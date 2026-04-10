// eBPF 探针：只识别 HTTP/HTTPS 流量并放行到 userspace。
// 这个文件的职责非常窄：
// 1. 在内核里尽量早地筛掉无关流量；
// 2. 在抓到相关 socket 时，把“这个 socket 属于谁”记录下来；
// 3. 把 userspace 需要的最小结构写进 map，后面由 userspace 负责更重的协议解析。
// 注意：这里不做 HTTP 解析、不重组 payload、不输出 JSON，所有复杂逻辑都留给 userspace。

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <linux/ptrace.h>
#include <linux/socket.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#ifndef bpf_ntohs
#define bpf_ntohs(x) __builtin_bswap16(x)
#endif

#define PROBE_BYTES 512
#ifndef TCP_ESTABLISHED
#define TCP_ESTABLISHED 1
#endif
#ifndef AF_INET
#define AF_INET 2
#endif
#ifndef AF_INET6
#define AF_INET6 10
#endif

#ifndef CAP_PAYLOAD
#define CAP_PAYLOAD 256
#endif

struct ns_common {
    // namespace 公共头部，里面的 inum 是 namespace 的 inode 号。
    __u32 inum;
};

struct pid_namespace {
    // PID namespace 对象，里面嵌着 ns_common。
    struct ns_common ns;
};

struct upid {
    // upid 表示“某个 PID 在某一层 namespace 里的编号”。
    int nr;
    struct pid_namespace *ns;
};

struct pid {
    // level 表示当前进程在 PID namespace 栈里的层级。
    // numbers[0] 是最外层，numbers[level] 是当前 task 所处 namespace 里的 pid。
    int level;
    struct upid numbers[1];
};

struct task_struct {
    // thread_pid 指向当前线程对应的 pid 结构体。
    struct pid *thread_pid;
};

struct socket_owner_key {
    __u64 sk_addr;
};

struct socket_owner_value {
    __u64 pid_tgid;
    char comm[16];
};

struct flow_owner_key {
    __u8 family;
    __u8 l4_proto;
    __u8 pad[2];
    __u16 sport;
    __u16 dport;
    unsigned char saddr[16];
    unsigned char daddr[16];
};

struct flow_owner_value {
    __u64 pid_tgid;
    __u32 uid;
    __u64 cr_id;
    __u32 cr_pid;
    char comm[16];
};

struct tracked_flow_key {
    __u8 family;
    __u8 l4_proto;
    __u8 pad[2];
    __u16 sport;
    __u16 dport;
    unsigned char saddr[16];
    unsigned char daddr[16];
};

struct socket {
    void *sk;
};

struct sock_common_bpf {
    union {
        __u64 skc_addrpair;
        struct {
            __be32 skc_daddr;
            __be32 skc_rcv_saddr;
        };
    };
    union {
        __u32 skc_hash;
        __u16 skc_u16hashes[2];
    };
    union {
        __u32 skc_portpair;
        struct {
            __be16 skc_dport;
            __u16 skc_num;
        };
    };
    unsigned short skc_family;
    volatile unsigned char skc_state;
    unsigned char skc_reuse:4;
    unsigned char skc_reuseport:1;
    unsigned char skc_ipv6only:1;
    unsigned char skc_net_refcnt:1;
    int skc_bound_dev_if;
    union {
        struct { void *next; void *pprev; } skc_bind_node;
        struct { void *next; void *pprev; } skc_portaddr_node;
    };
    void *skc_prot;
    void *skc_net;
    struct in6_addr skc_v6_daddr;
    struct in6_addr skc_v6_rcv_saddr;
};

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 32768);
    __type(key, struct flow_owner_key);
    __type(value, struct flow_owner_value);
} flow_owner_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 32768);
    __type(key, struct tracked_flow_key);
    __type(value, __u8);
} tracked_flow_map SEC(".maps");

static __always_inline int record_socket_owner(void *sk, __u8 l4_proto)
{
    struct flow_owner_key flow_key = {};
    struct flow_owner_value flow_value = {};
    struct sock_common_bpf common = {};
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    struct pid *pid = NULL;
    int level = 0;

    if (!sk) return 0;

    // 记录当前执行上下文对应的宿主进程信息：
    // pid_tgid 用于保留宿主视角的进程身份；uid 用于按用户过滤；comm 用于按进程名过滤。
    flow_value.pid_tgid = bpf_get_current_pid_tgid();
    flow_value.uid = (__u32)bpf_get_current_uid_gid();
    // 先把容器字段置零：
    // 如果后面发现这是宿主机进程，或者 namespace 读取失败，就保持 0，避免输出脏值。
    flow_value.cr_pid = 0;
    flow_value.cr_id = 0;

    // 读取当前进程 task_struct 的 thread_pid 字段，指向进程的 PID 结构体。
    pid = BPF_CORE_READ(task, thread_pid);
    if (pid) {
        // 读取 PID 结构体里的 level 字段：表示 PID namespace 的嵌套层级。
        // level=0 表示宿主机根 namespace；level>0 表示处在更深层的 namespace 中。
        level = BPF_CORE_READ(pid, level);
        if (level > 0) {
            // 读取当前 namespace 下的 PID，也就是容器视角看到的 pid。
            flow_value.cr_pid = BPF_CORE_READ(pid, numbers[level].nr);
            // 读取当前 PID namespace 的 inode 号，作为容器 namespace 的稳定标识。
            flow_value.cr_id = BPF_CORE_READ(pid, numbers[level].ns, ns.inum);
        }
        // level=0 时保持默认值 0：这里代表宿主机进程，不额外改写容器字段。
    }

    // comm 是进程名，后面 userspace 会把它作为 filter / 展示字段使用。
    if (bpf_get_current_comm(&flow_value.comm, sizeof(flow_value.comm)) < 0) return 0;

    // 这里只接受 TCP/UDP，其他 L4 协议直接丢弃。
    if (l4_proto != IPPROTO_TCP && l4_proto != IPPROTO_UDP) return 0;

    // 从 socket 读取五元组相关的基础信息，作为 map key 的来源。
    if (bpf_probe_read_kernel(&common, sizeof(common), sk) < 0) return 0;

    flow_key.family = common.skc_family;
    flow_key.l4_proto = l4_proto;

    // IPv4 场景：
    // skc_num 是本地端口，skc_dport / skc_rcv_saddr / skc_daddr 分别是对端端口、源地址和目的地址。
    if (common.skc_family == AF_INET) {
        flow_key.sport = common.skc_num;
        flow_key.dport = bpf_ntohs(common.skc_dport);
        __builtin_memcpy(flow_key.saddr, &common.skc_rcv_saddr, 4);
        __builtin_memcpy(flow_key.daddr, &common.skc_daddr, 4);
        // 把“这个五元组属于谁”写入 BPF map，后续 userspace 会按相同 key 反查。
        bpf_map_update_elem(&flow_owner_map, &flow_key, &flow_value, BPF_ANY);
    // IPv6 场景：
    // 和 IPv4 的逻辑一致，只是地址长度扩展成 16 字节。
    } else if (common.skc_family == AF_INET6) {
        flow_key.sport = common.skc_num;
        flow_key.dport = bpf_ntohs(common.skc_dport);
        __builtin_memcpy(flow_key.saddr, &common.skc_v6_rcv_saddr, 16);
        __builtin_memcpy(flow_key.daddr, &common.skc_v6_daddr, 16);
        bpf_map_update_elem(&flow_owner_map, &flow_key, &flow_value, BPF_ANY);
    }
    return 0;
}

// 探针1：跟踪 TCP 连接建立（客户端 connect）。
// 这个点能拿到“发起连接的一方”正在使用的 socket，因此适合记录客户端侧归属。
SEC("kprobe/tcp_connect")
int BPF_KPROBE(track_tcp_connect, void *sk) {
    if (!sk) return 0;
    return record_socket_owner(sk, IPPROTO_TCP);
}

// 探针2：跟踪 TCP 连接接受（inet_csk_accept 返回新建的连接 socket）。
// 这个点代表服务端 accept 之后拿到的新连接，因此适合记录服务端侧归属。
SEC("kretprobe/inet_csk_accept")
int track_tcp_accept(struct pt_regs *ctx) {
    void *new_sk = (void *)PT_REGS_RC(ctx);
    if (!new_sk) return 0;
    return record_socket_owner(new_sk, IPPROTO_TCP);
}

// tcp_sendmsg / udp_sendmsg 作为发送侧 owner 更新点。
// 发送路径上经常能拿到更接近“真正发包者”的上下文，因此这里作为补充更新点。
SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(track_tcp_sendmsg, void *sk, void *msg, unsigned long size) {
    if (!sk) return 0;
    return record_socket_owner(sk, IPPROTO_TCP);
}
SEC("kprobe/udp_sendmsg")
int BPF_KPROBE(track_udp_sendmsg, void *sk, void *msg, unsigned long size) {
    if (!sk) return 0;
    return record_socket_owner(sk, IPPROTO_UDP);
}

static __always_inline int is_http_https_port(__u16 port)
{
    // 端口级别的粗筛：80/443 是最典型的 HTTP/HTTPS 入口。
    return port == 80 || port == 443;
}

static __always_inline int is_vlan_proto(__u16 proto)
{
    // VLAN 封装会在 Ethernet 和真正的 IP 头之间插一层额外头部。
    return proto == ETH_P_8021Q || proto == ETH_P_8021AD;
}

static __always_inline int is_http_probe(const unsigned char *data, __u32 len)
{
    // 只看报文开头几个字节，判断它是不是 HTTP 请求行。
    // 这里故意只做轻量识别，不做完整解析，减少 eBPF 侧复杂度。
    if (len < 4) return 0;
    return (data[0] == 'G' && data[1] == 'E' && data[2] == 'T' && data[3] == ' ') ||
           (data[0] == 'P' && data[1] == 'O' && data[2] == 'S' && data[3] == 'T') ||
           (data[0] == 'H' && data[1] == 'E' && data[2] == 'A' && data[3] == 'D') ||
           (data[0] == 'P' && data[1] == 'U' && data[2] == 'T' && data[3] == ' ') ||
           (data[0] == 'D' && data[1] == 'E' && data[2] == 'L' && data[3] == 'E') ||
           (data[0] == 'O' && data[1] == 'P' && data[2] == 'T' && data[3] == 'I') ||
           (data[0] == 'P' && data[1] == 'A' && data[2] == 'T' && data[3] == 'C') ||
           (data[0] == 'C' && data[1] == 'O' && data[2] == 'N' && data[3] == 'N') ||
           (data[0] == 'T' && data[1] == 'R' && data[2] == 'A' && data[3] == 'C') ||
           (data[0] == 'H' && data[1] == 'T' && data[2] == 'T' && data[3] == 'P');
}

static __always_inline int is_tls_probe(const unsigned char *data, __u32 len)
{
    // TLS 记录层通常以 0x16 开头，后面跟版本号 0x03xx。
    // 这里只做非常轻的前缀判断，用于快速把明显的 TLS 流量标记出来。
    if (len < 5) return 0;
    return data[0] == 0x16 && data[1] == 0x03 && (data[2] <= 0x04);
}

static __always_inline int is_quic_probe(const unsigned char *data, __u32 len)
{
    // QUIC Initial 包的高位通常会被置位，这里只做最粗的启发式判断。
    if (len < 1) return 0;
    return (data[0] & 0x80) != 0;
}

static __always_inline int tracked_flow_lookup(__u8 family, __u8 l4_proto,
                                               const unsigned char *saddr,
                                               const unsigned char *daddr,
                                               __u16 sport, __u16 dport)
{
    // 构造流 key，并在 tracked_flow_map 里检查：
    // 如果这条流已经被确认是 HTTP/HTTPS/QUIC 相关流，就直接放行，不再重复判断。
    struct tracked_flow_key key = {};
    key.family = family;
    key.l4_proto = l4_proto;
    key.sport = sport;
    key.dport = dport;
    if (family == AF_INET) {
        __builtin_memcpy(key.saddr, saddr, 4);
        __builtin_memcpy(key.daddr, daddr, 4);
    } else if (family == AF_INET6) {
        __builtin_memcpy(key.saddr, saddr, 16);
        __builtin_memcpy(key.daddr, daddr, 16);
    } else {
        return 0;
    }
    return bpf_map_lookup_elem(&tracked_flow_map, &key) != NULL;
}

static __always_inline void tracked_flow_store(__u8 family, __u8 l4_proto,
                                               const unsigned char *saddr,
                                               const unsigned char *daddr,
                                               __u16 sport, __u16 dport)
{
    // 同时写正向和反向 key：
    // 这样请求方向和响应方向都能命中同一条已追踪流，避免只认一个方向。
    struct tracked_flow_key key = {};
    struct tracked_flow_key reverse = {};
    __u8 value = 1;

    key.family = family;
    key.l4_proto = l4_proto;
    key.sport = sport;
    key.dport = dport;
    reverse.family = family;
    reverse.l4_proto = l4_proto;
    reverse.sport = dport;
    reverse.dport = sport;

    if (family == AF_INET) {
        __builtin_memcpy(key.saddr, saddr, 4);
        __builtin_memcpy(key.daddr, daddr, 4);
        __builtin_memcpy(reverse.saddr, daddr, 4);
        __builtin_memcpy(reverse.daddr, saddr, 4);
    } else if (family == AF_INET6) {
        __builtin_memcpy(key.saddr, saddr, 16);
        __builtin_memcpy(key.daddr, daddr, 16);
        __builtin_memcpy(reverse.saddr, daddr, 16);
        __builtin_memcpy(reverse.daddr, saddr, 16);
    } else {
        return;
    }

    bpf_map_update_elem(&tracked_flow_map, &key, &value, BPF_ANY);
    bpf_map_update_elem(&tracked_flow_map, &reverse, &value, BPF_ANY);
}

SEC("socket")
int capture_prog(struct __sk_buff *skb)
{
    // socket filter 主入口：
    // 它不负责解析 payload 内容，只负责判断“这是不是我们关心的流量”。
    struct ethhdr eth;
    __u64 offset = 0;
    __u16 h_proto;
    __u8 family = 0;
    __u8 l4_proto = 0;
    unsigned char saddr[16] = {};
    unsigned char daddr[16] = {};

    if (bpf_skb_load_bytes(skb, 0, &eth, sizeof(eth)) < 0) return 0;
    h_proto = bpf_ntohs(eth.h_proto);
    offset = sizeof(eth);

    // 如果是 VLAN 封装，先跳过 VLAN 头，再继续判断真正的上层协议。
    if (is_vlan_proto(h_proto)) {
        struct {
            __be16 vlan_tci;
            __be16 encapsulated_proto;
        } vlan;
        if (bpf_skb_load_bytes(skb, offset, &vlan, sizeof(vlan)) < 0) return 0;
        h_proto = bpf_ntohs(vlan.encapsulated_proto);
        offset += sizeof(vlan);
    }

    if (h_proto == ETH_P_IP) {
        // IPv4：读取 IP 头，确认上层是 TCP/UDP，再提取源/目的地址。
        struct iphdr iph;
        __u32 ihl = 0;
        if (bpf_skb_load_bytes(skb, offset, &iph, sizeof(iph)) < 0) return 0;
        ihl = iph.ihl * 4;
        if (ihl < sizeof(iph)) return 0;
        if (iph.protocol != IPPROTO_TCP && iph.protocol != IPPROTO_UDP) return 0;
        family = AF_INET;
        l4_proto = iph.protocol;
        __builtin_memcpy(saddr, &iph.saddr, 4);
        __builtin_memcpy(daddr, &iph.daddr, 4);
        offset += ihl;
    } else if (h_proto == ETH_P_IPV6) {
        // IPv6：逻辑和 IPv4 类似，只是地址更长，且可能夹带扩展头。
        struct ipv6hdr ip6;
        if (bpf_skb_load_bytes(skb, offset, &ip6, sizeof(ip6)) < 0) return 0;
        if (ip6.nexthdr != IPPROTO_TCP && ip6.nexthdr != IPPROTO_UDP) return 0;
        family = AF_INET6;
        l4_proto = ip6.nexthdr;
        __builtin_memcpy(saddr, &ip6.saddr, 16);
        __builtin_memcpy(daddr, &ip6.daddr, 16);
        offset += sizeof(ip6);
    } else {
        return 0;
    }

    if (h_proto == ETH_P_IP) {
        // 这里是一个很轻的长度检查，避免明显不完整的包继续往下判断。
        if (offset + sizeof(struct tcphdr) > skb->len && offset + sizeof(struct udphdr) > skb->len) return 0;
    }

    if (h_proto == ETH_P_IP || h_proto == ETH_P_IPV6) {
        // 先拿到首部后的两个端口字段，作为最便宜的分类依据。
        __be16 ports[2] = {};
        if (bpf_skb_load_bytes(skb, offset, ports, sizeof(ports)) < 0) return 0;
        __u16 sport = bpf_ntohs(ports[0]);
        __u16 dport = bpf_ntohs(ports[1]);
        // 如果这条流已经在 tracked_flow_map 里，直接放行。
        if (tracked_flow_lookup(family, l4_proto, saddr, daddr, sport, dport)) return skb->len;
        // 80/443 端口是最典型的 HTTP/HTTPS，先把它们记住，后续包直接放行。
        if (is_http_https_port(sport) || is_http_https_port(dport)) {
            tracked_flow_store(family, l4_proto, saddr, daddr, sport, dport);
            return skb->len;
        }
    }

    // 如果端口不是 80/443，再看 payload 前几个字节，尝试识别 HTTP/TLS/QUIC。
    unsigned char probe[5] = {};
    __u32 want = sizeof(probe);
    if (bpf_skb_load_bytes(skb, offset, probe, want) < 0) return 0;

    if (is_http_probe(probe, want) || is_tls_probe(probe, want) || is_quic_probe(probe, want)) {
        // 命中协议特征后，把对应五元组写入 tracked_flow_map。
        __be16 ports[2] = {};
        if (bpf_skb_load_bytes(skb, offset, ports, sizeof(ports)) < 0) return 0;
        tracked_flow_store(family, l4_proto, saddr, daddr, bpf_ntohs(ports[0]), bpf_ntohs(ports[1]));
        // 返回 skb->len 表示放行整个包到 userspace。
        return skb->len;
    }
    // 既不是典型端口，也没有命中协议前缀，就丢弃。
    return 0;
}

char LICENSE[] SEC("license") = "GPL";