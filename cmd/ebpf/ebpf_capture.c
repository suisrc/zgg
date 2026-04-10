// eBPF 探针：只识别 HTTP/HTTPS 流量并放行到 userspace。
// 其他流量直接丢弃，避免 userspace 承担无关包处理压力。
// 这里仅做协议类型判断，不截取 payload，也不做转发。

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

    if (!sk) return 0;

    flow_value.pid_tgid = bpf_get_current_pid_tgid();
    if (bpf_get_current_comm(&flow_value.comm, sizeof(flow_value.comm)) < 0) return 0;

    if (l4_proto != IPPROTO_TCP && l4_proto != IPPROTO_UDP) return 0;
    if (bpf_probe_read_kernel(&common, sizeof(common), sk) < 0) return 0;

    flow_key.family = common.skc_family;
    flow_key.l4_proto = l4_proto;

    if (common.skc_family == AF_INET) {
        flow_key.sport = common.skc_num;
        flow_key.dport = bpf_ntohs(common.skc_dport);
        __builtin_memcpy(flow_key.saddr, &common.skc_rcv_saddr, 4);
        __builtin_memcpy(flow_key.daddr, &common.skc_daddr, 4);
        bpf_map_update_elem(&flow_owner_map, &flow_key, &flow_value, BPF_ANY);
    } else if (common.skc_family == AF_INET6) {
        flow_key.sport = common.skc_num;
        flow_key.dport = bpf_ntohs(common.skc_dport);
        __builtin_memcpy(flow_key.saddr, &common.skc_v6_rcv_saddr, 16);
        __builtin_memcpy(flow_key.daddr, &common.skc_v6_daddr, 16);
        bpf_map_update_elem(&flow_owner_map, &flow_key, &flow_value, BPF_ANY);
    }
    return 0;
}

// 探针1：跟踪TCP连接建立（客户端connect）
SEC("kprobe/tcp_connect")
int BPF_KPROBE(track_tcp_connect, void *sk) {
    if (!sk) return 0;
    return record_socket_owner(sk, IPPROTO_TCP);
}

// 探针2：跟踪TCP连接接受（inet_csk_accept 返回新建的连接socket）
SEC("kretprobe/inet_csk_accept")
int track_tcp_accept(struct pt_regs *ctx) {
    void *new_sk = (void *)PT_REGS_RC(ctx);
    if (!new_sk) return 0;
    return record_socket_owner(new_sk, IPPROTO_TCP);
}

// tcp_sendmsg/udp_sendmsg 作为发送侧 owner 更新点
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
    return port == 80 || port == 443;
}

static __always_inline int is_vlan_proto(__u16 proto)
{
    return proto == ETH_P_8021Q || proto == ETH_P_8021AD;
}

static __always_inline int is_http_probe(const unsigned char *data, __u32 len)
{
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
    if (len < 5) return 0;
    return data[0] == 0x16 && data[1] == 0x03 && (data[2] <= 0x04);
}

static __always_inline int is_quic_probe(const unsigned char *data, __u32 len)
{
    if (len < 1) return 0;
    return (data[0] & 0x80) != 0;
}

static __always_inline int tracked_flow_lookup(__u8 family, __u8 l4_proto,
                                               const unsigned char *saddr,
                                               const unsigned char *daddr,
                                               __u16 sport, __u16 dport)
{
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
        if (offset + sizeof(struct tcphdr) > skb->len && offset + sizeof(struct udphdr) > skb->len) return 0;
    }

    if (h_proto == ETH_P_IP || h_proto == ETH_P_IPV6) {
        __be16 ports[2] = {};
        if (bpf_skb_load_bytes(skb, offset, ports, sizeof(ports)) < 0) return 0;
        __u16 sport = bpf_ntohs(ports[0]);
        __u16 dport = bpf_ntohs(ports[1]);
        if (tracked_flow_lookup(family, l4_proto, saddr, daddr, sport, dport)) return skb->len;
        if (is_http_https_port(sport) || is_http_https_port(dport)) {
            tracked_flow_store(family, l4_proto, saddr, daddr, sport, dport);
            return skb->len;
        }
    }

    unsigned char probe[5] = {};
    __u32 want = sizeof(probe);
    if (bpf_skb_load_bytes(skb, offset, probe, want) < 0) return 0;

    if (is_http_probe(probe, want) || is_tls_probe(probe, want) || is_quic_probe(probe, want)) {
        __be16 ports[2] = {};
        if (bpf_skb_load_bytes(skb, offset, ports, sizeof(ports)) < 0) return 0;
        tracked_flow_store(family, l4_proto, saddr, daddr, bpf_ntohs(ports[0]), bpf_ntohs(ports[1]));
        return skb->len;
    }
    return 0;
}

char LICENSE[] SEC("license") = "GPL";