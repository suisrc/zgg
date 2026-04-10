// Userspace 程序：读取 eBPF 上报的 TCP/UDP 负载，按 HTTP/HTTPS 进行解析后输出单行 JSON。
// 设计目标：
// 1. eBPF 侧只负责稳定地抓包和搬运原始负载
// 2. userspace 负责协议识别、HTTP 流缓冲、SNI 解析、进程/网段过滤
// 3. 每个事件只输出一次 JSON，避免控制台分段写入造成解析混乱

#define _GNU_SOURCE

#include <arpa/inet.h>
#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <limits.h>
#include <fcntl.h>
#include <net/ethernet.h>
#include <net/if.h>
#include <linux/if_packet.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <poll.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

extern unsigned char _binary_ebpf_capture_o_start[];
extern unsigned char _binary_ebpf_capture_o_end[];

#define CAP_PAYLOAD 4096
#define FLOW_BUCKETS 4096
#define COMM_LEN 16
#define FLOW_BUFFER_CAP 32768
#define SOCKET_CACHE_MAX_ITEMS 8192
#define FLOW_STATE_MAX_ITEMS 4096

#ifndef bpf_ntohs
#define bpf_ntohs(x) __builtin_bswap16(x)
#endif

struct event_t {
    uint64_t cookie;
    uint8_t family;
    uint8_t direction;
    uint8_t l4_proto;
    uint8_t pad;
    uint16_t sport;
    uint16_t dport;
    uint32_t payload_len;
    uint32_t cap_len;
    unsigned char saddr[16];
    unsigned char daddr[16];
    unsigned char payload[CAP_PAYLOAD];
} __attribute__((packed));

struct cidr_rule {
    bool deny;
    int family;
    uint8_t prefix_len;
    union {
        struct in_addr v4;
        struct in6_addr v6;
    } addr;
};

struct cidr_set {
    struct cidr_rule *items;
    size_t len;
    size_t cap;
};

struct proc_owner {
    unsigned long long inode;
    pid_t pid;
    char comm[COMM_LEN];
};

struct socket_meta {
    int family;
    int proto;
    unsigned char saddr[16];
    unsigned char daddr[16];
    uint16_t sport;
    uint16_t dport;
    unsigned long long inode;
    pid_t pid;
    char comm[COMM_LEN];
};

struct flow_owner_key {
    uint8_t family;
    uint8_t l4_proto;
    uint8_t pad[2];
    uint16_t sport;
    uint16_t dport;
    unsigned char saddr[16];
    unsigned char daddr[16];
};

struct flow_owner_value {
    uint64_t pid_tgid;
    char comm[COMM_LEN];
};

struct socket_cache {
    struct socket_meta *items;
    size_t len;
    size_t cap;
    size_t max_items;
    uint64_t last_refresh_ms;
};

struct byte_buffer {
    unsigned char *data;
    size_t len;
    size_t cap;
};

struct flow_key {
    uint8_t family;
    uint8_t l4_proto;
    uint8_t direction;
    uint8_t pad;
    uint16_t sport;
    uint16_t dport;
    unsigned char saddr[16];
    unsigned char daddr[16];
};

struct flow_state {
    struct flow_key key;
    struct byte_buffer tcp_buffer;
    uint64_t last_seen_ms;
    pid_t pid;
    char comm[COMM_LEN];
    bool tls_emitted;
    bool has_domain;
    char domain[256];
    struct flow_state *next;
};

struct monitor_config {
    const char *ifname;
    int ifindex;
    int direction_filter;
    bool have_pid_filter;
    pid_t pid_filter;
    bool have_comm_filter;
    char comm_filter[COMM_LEN];
    bool have_sport_filter;
    uint16_t sport_filter;
    bool have_dport_filter;
    uint16_t dport_filter;
    long long body_limit;
    struct cidr_set src_rules;
    struct cidr_set dst_rules;
};

struct monitor_state {
    struct flow_state *flows[FLOW_BUCKETS];
    struct socket_cache sockets;
    int flow_owner_map_fd;
    uint64_t last_gc_ms;
    size_t flow_count;
};

struct callback_ctx {
    struct monitor_state *state;
    struct monitor_config *cfg;
};

static volatile bool exiting = false;

static void sigint_handler(int sig)
{
    (void)sig;
    exiting = true;
}

static uint64_t now_ms(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000ULL + (uint64_t)ts.tv_nsec / 1000000ULL;
}

static uint64_t epoch_ms(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return (uint64_t)ts.tv_sec * 1000ULL + (uint64_t)ts.tv_nsec / 1000000ULL;
}


static void iso8601_ms(uint64_t ms, char *out, size_t out_len)
{
    time_t secs = (time_t)(ms / 1000ULL);
    struct tm tm;
    char date_buf[32];
    char tz_buf[8];
    
    localtime_r(&secs, &tm);
    strftime(date_buf, sizeof(date_buf), "%Y-%m-%dT%H:%M:%S", &tm);
    strftime(tz_buf, sizeof(tz_buf), "%z", &tm);
    
    if (strlen(tz_buf) == 5) {
        // 格式字符串补全最后一个%c，对应tz_buf[4]
        snprintf(out, out_len, "%s.%03llu%c%c%c:%c%c",
                 date_buf,
                 (unsigned long long)(ms % 1000ULL),
                 tz_buf[0],  // 符号：+/-
                 tz_buf[1],  // 小时第一位
                 tz_buf[2],  // 小时第二位
                 tz_buf[3],  // 分钟第一位
                 tz_buf[4]); // 分钟第二位 ✅ 现在占位符数量匹配
    } else {
        snprintf(out, out_len, "%s.%03llu%s",
                 date_buf,
                 (unsigned long long)(ms % 1000ULL),
                 tz_buf);
    }
}

static uint64_t rand64(void)
{
    uint64_t a = (uint64_t)rand();
    uint64_t b = (uint64_t)rand();
    uint64_t c = (uint64_t)rand();
    uint64_t d = (uint64_t)rand();
    return (a << 48) ^ (b << 32) ^ (c << 16) ^ d ^ (uint64_t)getpid();
}

static void make_record_id(char out[25], uint64_t ms)
{
    static unsigned long long seq = 0;
    unsigned long long left = (unsigned long long)(ms & 0x7ffffffffffULL);
    unsigned long long right = ((unsigned long long)__sync_fetch_and_add(&seq, 1) ^ rand64()) & 0x1ffffffffffffULL;
    snprintf(out, 25, "%011llx%013llx", left, right);
    out[24] = '\0';
}

static uint64_t fnv1a64_update(uint64_t hash, const void *data, size_t len);

static void flow_key_hex(const struct event_t *e, pid_t pid, char out[17])
{
    uint64_t h = 1469598103934665603ULL;
    size_t len = e->family == AF_INET ? 4 : 16;
    char pid_buf[32];
    snprintf(pid_buf, sizeof(pid_buf), "%d", pid);

    // canonicalize by comparing source/destination tuples so request/response share the same key.
    unsigned char left[24] = {0};
    unsigned char right[24] = {0};
    unsigned char a[24] = {0};
    unsigned char b[24] = {0};
    memcpy(a, e->saddr, len);
    memcpy(b, e->daddr, len);
    bool swap = false;
    int cmp = memcmp(a, b, len);
    if (cmp > 0 || (cmp == 0 && e->sport > e->dport)) swap = true;
    if (swap) {
        memcpy(left, e->daddr, len);
        memcpy(right, e->saddr, len);
        left[len] = (unsigned char)(e->dport >> 8);
        left[len + 1] = (unsigned char)e->dport;
        right[len] = (unsigned char)(e->sport >> 8);
        right[len + 1] = (unsigned char)e->sport;
    } else {
        memcpy(left, e->saddr, len);
        memcpy(right, e->daddr, len);
        left[len] = (unsigned char)(e->sport >> 8);
        left[len + 1] = (unsigned char)e->sport;
        right[len] = (unsigned char)(e->dport >> 8);
        right[len + 1] = (unsigned char)e->dport;
    }
    h = fnv1a64_update(h, left, len + 2);
    h = fnv1a64_update(h, right, len + 2);
    h = fnv1a64_update(h, &e->l4_proto, sizeof(e->l4_proto));
    h = fnv1a64_update(h, pid_buf, strlen(pid_buf));
    snprintf(out, 17, "%016llx", (unsigned long long)h);
    out[16] = '\0';
}

static void record_key_hex(const struct event_t *e, const struct socket_meta *meta, char out[17])
{
    pid_t pid = meta && meta->pid > 0 ? meta->pid : 0;
    if (e->cookie) {
        uint64_t h = 1469598103934665603ULL;
        char pid_buf[32];
        uint64_t cookie = e->cookie;
        snprintf(pid_buf, sizeof(pid_buf), "%d", pid);
        h = fnv1a64_update(h, &cookie, sizeof(cookie));
        h = fnv1a64_update(h, pid_buf, strlen(pid_buf));
        snprintf(out, 17, "%016llx", (unsigned long long)h);
        out[16] = '\0';
        return;
    }
    flow_key_hex(e, pid, out);
}

static void emit_json_line(const char *json)
{
    flockfile(stdout);
    fputs("EBPF_CAPTURE: ", stdout);
    fputs(json, stdout);
    fputc('\n', stdout);
    fflush(stdout);
    funlockfile(stdout);
}

static void buffer_free(struct byte_buffer *buf)
{
    free(buf->data);
    buf->data = NULL;
    buf->len = 0;
    buf->cap = 0;
}

static bool buffer_reserve(struct byte_buffer *buf, size_t need)
{
    if (need <= buf->cap) return true;
    size_t next_cap = buf->cap ? buf->cap : 1024;
    while (next_cap < need) next_cap *= 2;
    unsigned char *next = realloc(buf->data, next_cap);
    if (!next) return false;
    buf->data = next;
    buf->cap = next_cap;
    return true;
}

static bool buffer_append(struct byte_buffer *buf, const unsigned char *data, size_t len)
{
    if (len == 0) return true;

    if (buf->len + len > FLOW_BUFFER_CAP) {
        if (len >= FLOW_BUFFER_CAP) {
            data += len - FLOW_BUFFER_CAP;
            len = FLOW_BUFFER_CAP;
        }
        if (buf->len > FLOW_BUFFER_CAP - len) {
            size_t drop = buf->len + len - FLOW_BUFFER_CAP;
            if (drop >= buf->len) {
                buf->len = 0;
            } else {
                memmove(buf->data, buf->data + drop, buf->len - drop);
                buf->len -= drop;
            }
        }
    }

    if (!buffer_reserve(buf, buf->len + len)) return false;
    memcpy(buf->data + buf->len, data, len);
    buf->len += len;
    return true;
}

static void buffer_consume(struct byte_buffer *buf, size_t n)
{
    if (n == 0) return;
    if (n >= buf->len) {
        buf->len = 0;
        return;
    }
    memmove(buf->data, buf->data + n, buf->len - n);
    buf->len -= n;
}

static uint64_t fnv1a64(const void *data, size_t len)
{
    const unsigned char *p = data;
    uint64_t hash = 1469598103934665603ULL;
    for (size_t i = 0; i < len; ++i) {
        hash ^= p[i];
        hash *= 1099511628211ULL;
    }
    return hash;
}

static uint64_t fnv1a64_update(uint64_t hash, const void *data, size_t len)
{
    const unsigned char *p = data;
    for (size_t i = 0; i < len; ++i) {
        hash ^= p[i];
        hash *= 1099511628211ULL;
    }
    return hash;
}

static size_t flow_bucket(const struct flow_key *key)
{
    return (size_t)(fnv1a64(key, sizeof(*key)) % FLOW_BUCKETS);
}

static struct flow_state *flow_find(struct monitor_state *state, const struct flow_key *key)
{
    size_t bucket = flow_bucket(key);
    for (struct flow_state *flow = state->flows[bucket]; flow; flow = flow->next) {
        if (memcmp(&flow->key, key, sizeof(*key)) == 0) return flow;
    }
    return NULL;
}

static struct flow_state *flow_get(struct monitor_state *state, const struct flow_key *key)
{
    struct flow_state *flow = flow_find(state, key);
    if (flow) return flow;

    if (state->flow_count >= FLOW_STATE_MAX_ITEMS) {
        struct flow_state *victim = NULL;
        struct flow_state **victim_link = NULL;
        for (size_t i = 0; i < FLOW_BUCKETS; ++i) {
            struct flow_state **pp = &state->flows[i];
            while (*pp) {
                struct flow_state *candidate = *pp;
                if (!victim || candidate->last_seen_ms < victim->last_seen_ms) {
                    victim = candidate;
                    victim_link = pp;
                }
                pp = &candidate->next;
            }
        }
        if (victim && victim_link) {
            *victim_link = victim->next;
            buffer_free(&victim->tcp_buffer);
            free(victim);
            if (state->flow_count > 0) state->flow_count--;
        }
    }

    flow = calloc(1, sizeof(*flow));
    if (!flow) return NULL;
    flow->key = *key;
    size_t bucket = flow_bucket(key);
    flow->next = state->flows[bucket];
    state->flows[bucket] = flow;
    state->flow_count++;
    return flow;
}

static void flow_gc(struct monitor_state *state, uint64_t cutoff_ms)
{
    for (size_t i = 0; i < FLOW_BUCKETS; ++i) {
        struct flow_state **pp = &state->flows[i];
        while (*pp) {
            struct flow_state *flow = *pp;
            if (flow->last_seen_ms < cutoff_ms) {
                *pp = flow->next;
                buffer_free(&flow->tcp_buffer);
                free(flow);
                if (state->flow_count > 0) state->flow_count--;
                continue;
            }
            pp = &flow->next;
        }
    }
}

static void cidr_set_free(struct cidr_set *set)
{
    free(set->items);
    set->items = NULL;
    set->len = 0;
    set->cap = 0;
}

static bool cidr_set_push(struct cidr_set *set, const struct cidr_rule *rule)
{
    if (set->len == set->cap) {
        size_t next_cap = set->cap ? set->cap * 2 : 8;
        struct cidr_rule *next = realloc(set->items, next_cap * sizeof(*next));
        if (!next) return false;
        set->items = next;
        set->cap = next_cap;
    }
    set->items[set->len++] = *rule;
    return true;
}

static char *trim_ws(char *s)
{
    while (*s && isspace((unsigned char)*s)) s++;
    char *end = s + strlen(s);
    while (end > s && isspace((unsigned char)end[-1])) --end;
    *end = '\0';
    return s;
}

static bool parse_cidr_rule(const char *token, bool deny, struct cidr_rule *rule)
{
    memset(rule, 0, sizeof(*rule));
    rule->deny = deny;

    char *tmp = strdup(token);
    if (!tmp) return false;
    char *slash = strchr(tmp, '/');
    if (slash) *slash = '\0';
    char *addr_part = trim_ws(tmp);
    char *prefix_part = slash ? trim_ws(slash + 1) : NULL;

    if (strchr(addr_part, ':')) {
        rule->family = AF_INET6;
        rule->prefix_len = prefix_part ? (uint8_t)strtoul(prefix_part, NULL, 10) : 128;
        if (rule->prefix_len > 128) rule->prefix_len = 128;
        if (inet_pton(AF_INET6, addr_part, &rule->addr.v6) != 1) {
            free(tmp);
            return false;
        }
    } else {
        rule->family = AF_INET;
        rule->prefix_len = prefix_part ? (uint8_t)strtoul(prefix_part, NULL, 10) : 32;
        if (rule->prefix_len > 32) rule->prefix_len = 32;
        if (inet_pton(AF_INET, addr_part, &rule->addr.v4) != 1) {
            free(tmp);
            return false;
        }
    }

    free(tmp);
    return true;
}

static bool parse_cidr_list(const char *spec, struct cidr_set *set)
{
    if (!spec || !*spec) return true;
    char *tmp = strdup(spec);
    if (!tmp) return false;

    for (char *save = NULL, *tok = strtok_r(tmp, ",", &save); tok; tok = strtok_r(NULL, ",", &save)) {
        tok = trim_ws(tok);
        bool deny = false;
        if (*tok == '!') {
            deny = true;
            tok = trim_ws(tok + 1);
        }
        if (!*tok) continue;
        struct cidr_rule rule;
        if (!parse_cidr_rule(tok, deny, &rule) || !cidr_set_push(set, &rule)) {
            free(tmp);
            return false;
        }
    }

    free(tmp);
    return true;
}

static bool cidr_match_v4(const struct cidr_rule *rule, const unsigned char *addr)
{
    uint32_t want = ntohl(rule->addr.v4.s_addr);
    uint32_t got = ((uint32_t)addr[0] << 24) | ((uint32_t)addr[1] << 16) | ((uint32_t)addr[2] << 8) | (uint32_t)addr[3];
    uint32_t mask = rule->prefix_len == 0 ? 0 : (~0U << (32 - rule->prefix_len));
    return (want & mask) == (got & mask);
}

static bool cidr_match_v6(const struct cidr_rule *rule, const unsigned char *addr)
{
    int bits = rule->prefix_len;
    for (int i = 0; i < 16; ++i) {
        int remain = bits - i * 8;
        if (remain <= 0) break;
        uint8_t mask = remain >= 8 ? 0xff : (uint8_t)(0xff << (8 - remain));
        if ((rule->addr.v6.s6_addr[i] & mask) != (addr[i] & mask)) return false;
    }
    return true;
}

static bool cidr_set_accepts(const struct cidr_set *set, int family, const unsigned char *addr)
{
    bool has_allow = false;
    bool allow_hit = false;

    for (size_t i = 0; i < set->len; ++i) {
        const struct cidr_rule *rule = &set->items[i];
        if (rule->family != family) continue;
        bool hit = (family == AF_INET) ? cidr_match_v4(rule, addr) : cidr_match_v6(rule, addr);
        if (!hit) continue;
        if (rule->deny) return false;
        has_allow = true;
        allow_hit = true;
    }

    if (!has_allow) return true;
    return allow_hit;
}

static int hex_val(int c)
{
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return 10 + (c - 'a');
    if (c >= 'A' && c <= 'F') return 10 + (c - 'A');
    return -1;
}

static bool parse_proc_ipv4(const char *token, unsigned char out[4], uint16_t *port)
{
    char ip_hex[16] = {0};
    unsigned int p = 0;
    if (sscanf(token, "%8[0-9A-Fa-f]:%x", ip_hex, &p) != 2) return false;

    for (size_t i = 0; i < 4; ++i) {
        int hi = hex_val(ip_hex[i * 2]);
        int lo = hex_val(ip_hex[i * 2 + 1]);
        if (hi < 0 || lo < 0) return false;
        out[3 - i] = (unsigned char)((hi << 4) | lo);
    }
    *port = (uint16_t)p;
    return true;
}

static bool parse_proc_ipv6(const char *token, unsigned char out[16], uint16_t *port)
{
    char ip_hex[64] = {0};
    unsigned int p = 0;
    if (sscanf(token, "%32[0-9A-Fa-f]:%x", ip_hex, &p) != 2) return false;

    unsigned char raw[16] = {0};
    for (size_t i = 0; i < 16; ++i) {
        int hi = hex_val(ip_hex[i * 2]);
        int lo = hex_val(ip_hex[i * 2 + 1]);
        if (hi < 0 || lo < 0) return false;
        raw[i] = (unsigned char)((hi << 4) | lo);
    }

    for (size_t i = 0; i < 4; ++i) {
        out[i * 4 + 0] = raw[i * 4 + 3];
        out[i * 4 + 1] = raw[i * 4 + 2];
        out[i * 4 + 2] = raw[i * 4 + 1];
        out[i * 4 + 3] = raw[i * 4 + 0];
    }
    *port = (uint16_t)p;
    return true;
}

static bool parse_socket_token(int family, const char *token, unsigned char addr[16], uint16_t *port)
{
    if (family == AF_INET) {
        unsigned char v4[4] = {0};
        if (!parse_proc_ipv4(token, v4, port)) return false;
        memset(addr, 0, 16);
        memcpy(addr, v4, 4);
        return true;
    }
    return parse_proc_ipv6(token, addr, port);
}

static int owner_cmp_inode(const void *a, const void *b)
{
    const struct proc_owner *lhs = a;
    const struct proc_owner *rhs = b;
    if (lhs->inode < rhs->inode) return -1;
    if (lhs->inode > rhs->inode) return 1;
    return 0;
}

static const struct proc_owner *owner_find(const struct proc_owner *owners, size_t len, unsigned long long inode)
{
    struct proc_owner needle = {.inode = inode};
    return bsearch(&needle, owners, len, sizeof(*owners), owner_cmp_inode);
}

static bool inode_owner_add(struct proc_owner **owners, size_t *len, size_t *cap,
                            unsigned long long inode, pid_t pid, const char *comm)
{
    for (size_t i = 0; i < *len; ++i) {
        if ((*owners)[i].inode == inode) return true;
    }
    if (*len == *cap) {
        size_t next_cap = *cap ? *cap * 2 : 1024;
        struct proc_owner *next = realloc(*owners, next_cap * sizeof(**owners));
        if (!next) return false;
        *owners = next;
        *cap = next_cap;
    }
    (*owners)[*len].inode = inode;
    (*owners)[*len].pid = pid;
    strncpy((*owners)[*len].comm, comm, COMM_LEN - 1);
    (*owners)[*len].comm[COMM_LEN - 1] = '\0';
    (*len)++;
    return true;
}

static bool read_comm_file(pid_t pid, char comm[COMM_LEN])
{
    char path[64];
    snprintf(path, sizeof(path), "/proc/%d/comm", pid);
    FILE *fp = fopen(path, "r");
    if (!fp) return false;
    if (!fgets(comm, COMM_LEN, fp)) {
        fclose(fp);
        return false;
    }
    fclose(fp);
    comm[strcspn(comm, "\n")] = '\0';
    return true;
}

static bool build_inode_owners(struct proc_owner **owners_out, size_t *owners_len_out)
{
    DIR *proc = opendir("/proc");
    if (!proc) return false;

    struct proc_owner *owners = NULL;
    size_t owners_len = 0, owners_cap = 0;

    struct dirent *de;
    while ((de = readdir(proc)) != NULL) {
        if (!isdigit((unsigned char)de->d_name[0])) continue;
        char *end = NULL;
        long pid_long = strtol(de->d_name, &end, 10);
        if (!end || *end != '\0' || pid_long <= 0) continue;
        pid_t pid = (pid_t)pid_long;

        char comm[COMM_LEN] = {0};
        if (!read_comm_file(pid, comm)) continue;

        char fd_dir[64];
        snprintf(fd_dir, sizeof(fd_dir), "/proc/%d/fd", pid);
        DIR *fds = opendir(fd_dir);
        if (!fds) continue;

        struct dirent *fd_de;
        while ((fd_de = readdir(fds)) != NULL) {
            if (fd_de->d_name[0] == '.') continue;
            char link_path[128];
            char link_target[128];
            snprintf(link_path, sizeof(link_path), "%s/%s", fd_dir, fd_de->d_name);
            ssize_t n = readlink(link_path, link_target, sizeof(link_target) - 1);
            if (n <= 0) continue;
            link_target[n] = '\0';
            unsigned long long inode = 0;
            if (sscanf(link_target, "socket:[%llu]", &inode) == 1 && inode > 0) {
                if (!inode_owner_add(&owners, &owners_len, &owners_cap, inode, pid, comm)) {
                    closedir(fds);
                    closedir(proc);
                    free(owners);
                    return false;
                }
            }
        }
        closedir(fds);
    }

    closedir(proc);
    qsort(owners, owners_len, sizeof(*owners), owner_cmp_inode);
    *owners_out = owners;
    *owners_len_out = owners_len;
    return true;
}

static bool socket_cache_push(struct socket_cache *cache, const struct socket_meta *meta)
{
    if (cache->max_items > 0 && cache->len >= cache->max_items) {
        return true;
    }
    if (cache->len == cache->cap) {
        size_t next_cap = cache->cap ? cache->cap * 2 : 1024;
        struct socket_meta *next = realloc(cache->items, next_cap * sizeof(*next));
        if (!next) return false;
        cache->items = next;
        cache->cap = next_cap;
    }
    cache->items[cache->len++] = *meta;
    return true;
}

static bool parse_proc_net_table(const char *path, int family, int proto,
                                 const struct proc_owner *owners, size_t owners_len,
                                 struct socket_cache *cache)
{
    FILE *fp = fopen(path, "r");
    if (!fp) return false;

    char *line = NULL;
    size_t cap = 0;
    ssize_t nread;
    while ((nread = getline(&line, &cap, fp)) != -1) {
        (void)nread;
        char *save = NULL;
        char *tok = strtok_r(line, " \t\n", &save);
        if (!tok || !strchr(tok, ':')) continue;

        unsigned char local_addr[16] = {0};
        unsigned char remote_addr[16] = {0};
        uint16_t local_port = 0;
        uint16_t remote_port = 0;
        unsigned long long inode = 0;

        int column = 0;
        while ((tok = strtok_r(NULL, " \t\n", &save)) != NULL) {
            ++column;
            if (column == 1) {
                if (!parse_socket_token(family, tok, local_addr, &local_port)) goto next_line;
            } else if (column == 2) {
                if (!parse_socket_token(family, tok, remote_addr, &remote_port)) goto next_line;
            } else if (column == 9) {
                inode = strtoull(tok, NULL, 10);
            }
        }

        if (inode == 0) goto next_line;

        struct socket_meta meta;
        memset(&meta, 0, sizeof(meta));
        meta.family = family;
        meta.proto = proto;
        memcpy(meta.saddr, local_addr, 16);
        memcpy(meta.daddr, remote_addr, 16);
        meta.sport = local_port;
        meta.dport = remote_port;
        meta.inode = inode;

        const struct proc_owner *owner = owner_find(owners, owners_len, inode);
        if (owner) {
            meta.pid = owner->pid;
            strncpy(meta.comm, owner->comm, COMM_LEN - 1);
            meta.comm[COMM_LEN - 1] = '\0';
        }

        if (!socket_cache_push(cache, &meta)) {
            free(line);
            fclose(fp);
            return false;
        }

    next_line:
        ;
    }

    free(line);
    fclose(fp);
    return true;
}

static bool rebuild_socket_cache(struct socket_cache *cache)
{
    struct proc_owner *owners = NULL;
    size_t owners_len = 0;
    if (!build_inode_owners(&owners, &owners_len)) {
        free(owners);
        return false;
    }

    free(cache->items);
    cache->items = NULL;
    cache->len = 0;
    cache->cap = 0;
    if (cache->max_items == 0) cache->max_items = SOCKET_CACHE_MAX_ITEMS;

    bool ok = true;
    ok &= parse_proc_net_table("/proc/net/tcp", AF_INET, IPPROTO_TCP, owners, owners_len, cache);
    ok &= parse_proc_net_table("/proc/net/tcp6", AF_INET6, IPPROTO_TCP, owners, owners_len, cache);
    ok &= parse_proc_net_table("/proc/net/udp", AF_INET, IPPROTO_UDP, owners, owners_len, cache);
    ok &= parse_proc_net_table("/proc/net/udp6", AF_INET6, IPPROTO_UDP, owners, owners_len, cache);

    free(owners);
    cache->last_refresh_ms = now_ms();
    return ok;
}

static const struct socket_meta *socket_cache_lookup(const struct socket_cache *cache,
                                                     const struct event_t *e)
{
    for (size_t i = 0; i < cache->len; ++i) {
        const struct socket_meta *m = &cache->items[i];
        if (m->family != e->family || m->proto != e->l4_proto) continue;
        size_t addr_len = e->family == AF_INET ? 4 : 16;
        bool direct = m->sport == e->sport && m->dport == e->dport &&
                      memcmp(m->saddr, e->saddr, addr_len) == 0 &&
                      memcmp(m->daddr, e->daddr, addr_len) == 0;
        bool reverse = m->sport == e->dport && m->dport == e->sport &&
                       memcmp(m->saddr, e->daddr, addr_len) == 0 &&
                       memcmp(m->daddr, e->saddr, addr_len) == 0;
        if (direct || reverse) return m;
    }
    return NULL;
}

static const struct socket_meta *socket_cache_lookup_directional(const struct socket_cache *cache,
                                                                 const struct event_t *e)
{
    for (size_t i = 0; i < cache->len; ++i) {
        const struct socket_meta *m = &cache->items[i];
        if (m->family != e->family || m->proto != e->l4_proto) continue;
        size_t addr_len = e->family == AF_INET ? 4 : 16;
        bool direct = m->sport == e->sport && m->dport == e->dport &&
                      memcmp(m->saddr, e->saddr, addr_len) == 0 &&
                      memcmp(m->daddr, e->daddr, addr_len) == 0;
        bool reverse = m->sport == e->dport && m->dport == e->sport &&
                       memcmp(m->saddr, e->daddr, addr_len) == 0 &&
                       memcmp(m->daddr, e->saddr, addr_len) == 0;
        if (e->direction == 1) {
            if (direct) return m;
        } else {
            if (reverse) return m;
        }
    }
    return NULL;
}

static const struct socket_meta *socket_cache_lookup_listener(const struct socket_cache *cache,
                                                              const struct event_t *e)
{
    if (e->l4_proto != IPPROTO_TCP) return NULL;

    uint16_t listen_port = e->direction == 0 ? e->dport : e->sport;

    for (size_t i = 0; i < cache->len; ++i) {
        const struct socket_meta *m = &cache->items[i];
        bool family_match = m->family == e->family || (e->family == AF_INET && m->family == AF_INET6);
        if (!family_match || m->proto != e->l4_proto) continue;
        if (m->sport != listen_port) continue;
        if (m->dport != 0) continue;
        return m;
    }
    return NULL;
}

static void flow_owner_key_from_event(const struct event_t *e, bool reverse,
                                      struct flow_owner_key *key)
{
    memset(key, 0, sizeof(*key));
    key->family = e->family;
    key->l4_proto = e->l4_proto;

    size_t addr_len = e->family == AF_INET ? 4 : 16;
    if (reverse) {
        memcpy(key->saddr, e->daddr, addr_len);
        memcpy(key->daddr, e->saddr, addr_len);
        key->sport = e->dport;
        key->dport = e->sport;
    } else {
        memcpy(key->saddr, e->saddr, addr_len);
        memcpy(key->daddr, e->daddr, addr_len);
        key->sport = e->sport;
        key->dport = e->dport;
    }
}

static bool flow_owner_map_lookup(const struct monitor_state *state,
                                  const struct event_t *e,
                                  struct socket_meta *meta)
{
    if (state->flow_owner_map_fd < 0) return false;

    struct flow_owner_key key;
    struct flow_owner_value value;

    if (e->direction == 1) {
        flow_owner_key_from_event(e, false, &key);
    } else {
        flow_owner_key_from_event(e, true, &key);
    }
    if (bpf_map_lookup_elem(state->flow_owner_map_fd, &key, &value) != 0) return false;

    memset(meta, 0, sizeof(*meta));
    meta->family = e->family;
    meta->proto = e->l4_proto;
    meta->sport = e->sport;
    meta->dport = e->dport;
    meta->pid = (pid_t)(value.pid_tgid >> 32);
    strncpy(meta->comm, value.comm, COMM_LEN - 1);
    meta->comm[COMM_LEN - 1] = '\0';
    return true;
}

static const struct socket_meta *resolve_packet_meta(const struct monitor_state *state,
                                                     const struct event_t *e,
                                                     struct socket_meta *scratch)
{
    if (flow_owner_map_lookup(state, e, scratch)) return scratch;

    if (e->l4_proto == IPPROTO_TCP) {
        const struct socket_meta *listener = socket_cache_lookup_listener(&state->sockets, e);
        if (listener && listener->pid > 0) {
            *scratch = *listener;
            return scratch;
        }
    }

    const struct socket_meta *cached = socket_cache_lookup_directional(&state->sockets, e);
    if (cached) {
        *scratch = *cached;
        return scratch;
    }

    if (e->l4_proto == IPPROTO_TCP) {
        const struct socket_meta *listener = socket_cache_lookup_listener(&state->sockets, e);
        if (listener) {
            *scratch = *listener;
            return scratch;
        }
    }
    return NULL;
}

static void flow_reverse_key(const struct event_t *e, struct flow_key *key)
{
    memset(key, 0, sizeof(*key));
    key->family = e->family;
    key->l4_proto = e->l4_proto;
    key->direction = e->direction == 0 ? 1 : 0;

    size_t addr_len = e->family == AF_INET ? 4 : 16;
    memcpy(key->saddr, e->daddr, addr_len);
    memcpy(key->daddr, e->saddr, addr_len);
    key->sport = e->dport;
    key->dport = e->sport;
}

static void json_escape_file(FILE *out, const unsigned char *data, size_t len)
{
    for (size_t i = 0; i < len; ++i) {
        unsigned char c = data[i];
        switch (c) {
        case '"': fputs("\\\"", out); break;
        case '\\': fputs("\\\\", out); break;
        case '\b': fputs("\\b", out); break;
        case '\f': fputs("\\f", out); break;
        case '\n': fputs("\\n", out); break;
        case '\r': fputs("\\r", out); break;
        case '\t': fputs("\\t", out); break;
        default:
            if (c < 0x20 || c > 0x7e) {
                fprintf(out, "\\u%04x", c);
            } else {
                fputc(c, out);
            }
        }
    }
}

static bool is_http_request_start(const unsigned char *data, size_t len)
{
    if (len < 4) return false;
    return (!memcmp(data, "GET ", 4) || !memcmp(data, "POST", 4) || !memcmp(data, "HEAD", 4) ||
            !memcmp(data, "PUT ", 4) || !memcmp(data, "DELE", 4) || !memcmp(data, "OPTI", 4) ||
            !memcmp(data, "PATC", 4) || !memcmp(data, "CONN", 4) || !memcmp(data, "TRAC", 4));
}

static bool is_http_response_start(const unsigned char *data, size_t len)
{
    return len >= 5 && !memcmp(data, "HTTP/", 5);
}

static const unsigned char *find_bytes(const unsigned char *hay, size_t hay_len,
                                       const char *needle, size_t needle_len)
{
    if (needle_len == 0 || hay_len < needle_len) return NULL;
    for (size_t i = 0; i + needle_len <= hay_len; ++i) {
        if (memcmp(hay + i, needle, needle_len) == 0) return hay + i;
    }
    return NULL;
}

static size_t find_header_end(const unsigned char *data, size_t len)
{
    const unsigned char *p = find_bytes(data, len, "\r\n\r\n", 4);
    if (!p) return 0;
    return (size_t)(p - data) + 4;
}

static void extract_header_value(const unsigned char *headers, size_t len, const char *name,
                                char *out, size_t out_len)
{
    out[0] = '\0';
    size_t name_len = strlen(name);
    for (size_t off = 0; off + name_len < len; ++off) {
        if (strncasecmp((const char *)headers + off, name, name_len) != 0) continue;
        const unsigned char *p = headers + off + name_len;
        while (p < headers + len && (*p == ' ' || *p == '\t' || *p == ':')) p++;
        const unsigned char *end = p;
        while (end < headers + len && *end != '\r' && *end != '\n') end++;
        size_t copy_len = (size_t)(end - p);
        if (copy_len >= out_len) copy_len = out_len - 1;
        memcpy(out, p, copy_len);
        out[copy_len] = '\0';
        return;
    }
}

static int parse_content_length_header(const unsigned char *headers, size_t len)
{
    const char *needle = "Content-Length:";
    const unsigned char *p = find_bytes(headers, len, needle, strlen(needle));
    if (!p) return -1;
    p += strlen(needle);
    while (p < headers + len && isspace(*p)) p++;
    int value = 0;
    bool found = false;

    while (p < headers + len && isdigit(*p)) {
        found = true;
        value = value * 10 + (*p - '0');
        p++;
    }
    return found ? value : -1;
}

static bool header_has_chunked_encoding(const unsigned char *headers, size_t len)
{
    const char *needle = "Transfer-Encoding:";
    const unsigned char *p = find_bytes(headers, len, needle, strlen(needle));
    if (!p) return false;
    p += strlen(needle);
    while (p < headers + len && (*p == ' ' || *p == '\t' || *p == ':')) p++;
    while (p < headers + len && *p != '\r' && *p != '\n') {
        if (strncasecmp((const char *)p, "chunked", 7) == 0) return true;
        p++;
    }
    return false;
}

static bool parse_chunked_body(const unsigned char *data, size_t len, long long body_limit,
                               unsigned char **body_copy_out, size_t *body_len_out,
                               size_t *consumed_out)
{
    *body_copy_out = NULL;
    *body_len_out = 0;
    *consumed_out = 0;

    size_t keep_limit = 0;
    if (body_limit < 0) {
        keep_limit = 0;
    } else if (body_limit == 0) {
        keep_limit = SIZE_MAX;
    } else {
        keep_limit = (size_t)body_limit;
    }

    unsigned char *keep = NULL;
    size_t keep_len = 0;
    size_t keep_cap = 0;
    size_t pos = 0;

    for (;;) {
        const unsigned char *line_end = find_bytes(data + pos, len - pos, "\r\n", 2);
        if (!line_end) {
            free(keep);
            return false;
        }

        size_t line_len = (size_t)(line_end - (data + pos));
        if (line_len == 0) {
            free(keep);
            return false;
        }

        char size_buf[32];
        size_t copy_len = line_len < sizeof(size_buf) - 1 ? line_len : sizeof(size_buf) - 1;
        memcpy(size_buf, data + pos, copy_len);
        size_buf[copy_len] = '\0';
        char *semi = strchr(size_buf, ';');
        if (semi) *semi = '\0';

        errno = 0;
        char *endptr = NULL;
        unsigned long chunk_size = strtoul(size_buf, &endptr, 16);
        if (endptr == size_buf || errno != 0) {
            free(keep);
            return false;
        }

        pos += line_len + 2;
        if (chunk_size == 0) {
            if (len - pos >= 2 && data[pos] == '\r' && data[pos + 1] == '\n') {
                pos += 2;
            } else {
                const unsigned char *trail_end = find_bytes(data + pos, len - pos, "\r\n\r\n", 4);
                if (!trail_end) {
                    free(keep);
                    return false;
                }
                pos = (size_t)(trail_end - data) + 4;
            }
            break;
        }

        if (len - pos < chunk_size + 2) {
            free(keep);
            return false;
        }

        size_t chunk_copy = 0;
        if (keep_limit > 0) {
            if (keep_limit == SIZE_MAX) {
                chunk_copy = (size_t)chunk_size;
            } else if (keep_len < keep_limit) {
                chunk_copy = (size_t)chunk_size;
                if (chunk_copy > keep_limit - keep_len) chunk_copy = keep_limit - keep_len;
            }
        }

        if (chunk_copy > 0) {
            if (keep_len + chunk_copy > keep_cap) {
                size_t next_cap = keep_cap ? keep_cap * 2 : 1024;
                while (next_cap < keep_len + chunk_copy) next_cap *= 2;
                unsigned char *next = realloc(keep, next_cap);
                if (!next) {
                    free(keep);
                    return false;
                }
                keep = next;
                keep_cap = next_cap;
            }
            memcpy(keep + keep_len, data + pos, chunk_copy);
            keep_len += chunk_copy;
        }

        pos += chunk_size;
        if (data[pos] != '\r' || data[pos + 1] != '\n') {
            free(keep);
            return false;
        }
        pos += 2;
    }

    *body_copy_out = keep;
    *body_len_out = keep_len;
    *consumed_out = pos;
    return true;
}

static bool parse_tls_clienthello(const unsigned char *data, uint32_t len,
                                  char *sni, size_t sni_len,
                                  char *alpn, size_t alpn_len,
                                  uint16_t *tls_version_out)
{
    sni[0] = '\0';
    alpn[0] = '\0';

    if (len < 5) return false;
    if (data[0] != 0x16) return false;

    uint16_t record_version = (uint16_t)((data[1] << 8) | data[2]);
    if (tls_version_out) *tls_version_out = record_version;

    if (len < 9) return false;
    const unsigned char *p = data + 5;
    if (p[0] != 0x01) return false;

    if (5 + 4 + 2 + 32 > len) return false;
    const unsigned char *q = p + 4 + 2 + 32;
    if (q + 1 > data + len) return false;

    uint8_t sid_len = q[0];
    q += 1 + sid_len;
    if (q + 2 > data + len) return false;

    uint16_t cs_len = (uint16_t)((q[0] << 8) | q[1]);
    q += 2 + cs_len;
    if (q + 1 > data + len) return false;

    uint8_t comp_len = q[0];
    q += 1 + comp_len;
    if (q + 2 > data + len) return false;

    uint16_t ext_len = (uint16_t)((q[0] << 8) | q[1]);
    q += 2;
    const unsigned char *ext_end = q + ext_len;
    if (ext_end > data + len) return false;

    while (q + 4 <= ext_end) {
        uint16_t ext_type = (uint16_t)((q[0] << 8) | q[1]);
        uint16_t ext_size = (uint16_t)((q[2] << 8) | q[3]);
        q += 4;
        if (q + ext_size > ext_end) break;

        if (ext_type == 0x0000) {
            const unsigned char *es = q;
            if (es + 2 <= q + ext_size) {
                es += 2;
                const unsigned char *es_end = q + ext_size;
                while (es + 3 <= es_end) {
                    uint8_t name_type = es[0];
                    uint16_t name_len = (uint16_t)((es[1] << 8) | es[2]);
                    es += 3;
                    if (es + name_len > es_end) break;
                    if (name_type == 0) {
                        size_t cp = name_len < sni_len - 1 ? name_len : sni_len - 1;
                        memcpy(sni, es, cp);
                        sni[cp] = '\0';
                    }
                    es += name_len;
                }
            }
        } else if (ext_type == 0x0010) {
            const unsigned char *es = q;
            if (es + 2 <= q + ext_size) {
                es += 2;
                const unsigned char *es_end = q + ext_size;
                if (es + 1 <= es_end) {
                    uint8_t proto_len = es[0];
                    if (es + 1 + proto_len <= es_end) {
                        size_t cp = proto_len < alpn_len - 1 ? proto_len : alpn_len - 1;
                        memcpy(alpn, es + 1, cp);
                        alpn[cp] = '\0';
                    }
                }
            }
        } else if (ext_type == 0x002b) {
            if (ext_size >= 3 && q[0] == 0x02) {
                uint16_t negotiated = (uint16_t)((q[1] << 8) | q[2]);
                if (tls_version_out) *tls_version_out = negotiated;
            }
        }

        q += ext_size;
    }

    return sni[0] || alpn[0] || tls_version_out;
}

static bool parse_tls_serverhello(const unsigned char *data, uint32_t len,
                                  char *alpn, size_t alpn_len,
                                  uint16_t *tls_version_out)
{
    alpn[0] = '\0';
    if (len < 5 || data[0] != 0x16) return false;

    uint16_t record_version = (uint16_t)((data[1] << 8) | data[2]);
    if (tls_version_out) *tls_version_out = record_version;

    if (len < 9) return false;
    const unsigned char *p = data + 5;
    if (p[0] != 0x02) return false;

    const unsigned char *q = p + 4;
    if (q + 2 + 32 + 1 > data + len) return false;
    q += 2 + 32;
    uint8_t sid_len = q[0];
    q += 1 + sid_len;
    if (q + 2 + 1 + 2 > data + len) return false;
    q += 2 + 1 + 2;
    if (q + 2 > data + len) return false;
    uint16_t ext_len = (uint16_t)((q[0] << 8) | q[1]);
    q += 2;
    const unsigned char *ext_end = q + ext_len;
    if (ext_end > data + len) return false;

    while (q + 4 <= ext_end) {
        uint16_t ext_type = (uint16_t)((q[0] << 8) | q[1]);
        uint16_t ext_size = (uint16_t)((q[2] << 8) | q[3]);
        q += 4;
        if (q + ext_size > ext_end) break;

        if (ext_type == 0x0010) {
            if (ext_size >= 3) {
                uint16_t list_len = (uint16_t)((q[0] << 8) | q[1]);
                const unsigned char *es = q + 2;
                const unsigned char *es_end = q + 2 + list_len;
                if (es_end > q + ext_size) es_end = q + ext_size;
                if (es + 1 <= es_end) {
                    uint8_t proto_len = es[0];
                    if (es + 1 + proto_len <= es_end) {
                        size_t cp = proto_len < alpn_len - 1 ? proto_len : alpn_len - 1;
                        memcpy(alpn, es + 1, cp);
                        alpn[cp] = '\0';
                    }
                }
            }
        } else if (ext_type == 0x002b) {
            if (ext_size >= 3 && q[0] == 0x02) {
                uint16_t negotiated = (uint16_t)((q[1] << 8) | q[2]);
                if (tls_version_out) *tls_version_out = negotiated;
            }
        }

        q += ext_size;
    }

    return tls_version_out != NULL || alpn[0];
}

static const char *tls_version_name(uint16_t v)
{
    switch (v) {
    case 0x0301: return "TLS 1.0";
    case 0x0302: return "TLS 1.1";
    case 0x0303: return "TLS 1.2";
    case 0x0304: return "TLS 1.3";
    default: return "";
    }
}

static const char *http_version_from_alpn(const char *alpn)
{
    if (!alpn || !*alpn) return "";
    if (!strcmp(alpn, "h2")) return "HTTP/2";
    if (!strcmp(alpn, "h3")) return "HTTP/3";
    if (!strcmp(alpn, "http/1.1")) return "HTTP/1.1";
    if (!strcmp(alpn, "http/1.0")) return "HTTP/1.0";
    return alpn;
}

static int parse_quic_varint(const unsigned char *p, uint32_t len,
                             uint64_t *value, uint32_t *consumed)
{
    if (len < 1) return 0;
    uint8_t lead = p[0] >> 6;
    uint32_t n = 1U << lead;
    if (n > len) return 0;
    uint64_t v = p[0] & 0x3f;
    for (uint32_t i = 1; i < n; ++i) v = (v << 8) | p[i];
    *value = v;
    *consumed = n;
    return 1;
}

static int find_quic_tls_payload(const unsigned char *data, uint32_t len,
                                 const unsigned char **out, uint32_t *out_len)
{
    if (len < 6) return 0;
    if ((data[0] & 0x80) == 0) return 0;

    uint32_t pos = 1;
    if (pos + 4 > len) return 0;
    pos += 4;

    if (pos + 1 > len) return 0;
    uint8_t dcid_len = data[pos++];
    if (pos + dcid_len > len) return 0;
    pos += dcid_len;

    if (pos + 1 > len) return 0;
    uint8_t scid_len = data[pos++];
    if (pos + scid_len > len) return 0;
    pos += scid_len;

    uint64_t token_len = 0;
    uint32_t token_len_bytes = 0;
    if (!parse_quic_varint(data + pos, len - pos, &token_len, &token_len_bytes)) return 0;
    pos += token_len_bytes;
    if (pos + token_len > len) return 0;
    pos += (uint32_t)token_len;

    uint64_t payload_len = 0;
    uint32_t payload_len_bytes = 0;
    if (!parse_quic_varint(data + pos, len - pos, &payload_len, &payload_len_bytes)) return 0;
    pos += payload_len_bytes;
    if (pos > len) return 0;

    *out = data + pos;
    *out_len = len - pos;
    (void)payload_len;
    return 1;
}

struct http_message {
    bool response;
    char method[32];
    char path[1024];
    char status[256];
    char host[512];
    char version[64];
    size_t header_end;
    size_t body_len;
    size_t consumed;
    unsigned char *body_copy;
};

static void http_message_free(struct http_message *msg)
{
    free(msg->body_copy);
    msg->body_copy = NULL;
}

static bool parse_http_message(const unsigned char *data, size_t len, long long body_limit,
                               struct http_message *msg)
{
    memset(msg, 0, sizeof(*msg));

    if (len < 4) return false;
    bool is_resp = is_http_response_start(data, len);
    bool is_req = is_http_request_start(data, len);
    if (!is_resp && !is_req) return false;

    size_t header_end = find_header_end(data, len);
    if (header_end == 0) return false;

    const unsigned char *headers = data;
    size_t headers_len = header_end;
    size_t body_available = len - header_end;

    char *headers_copy = strndup((const char *)headers, headers_len);
    if (!headers_copy) return false;

    char *save = NULL;
    char *line = strtok_r(headers_copy, "\r\n", &save);
    if (!line) {
        free(headers_copy);
        return false;
    }

    if (is_resp) {
        msg->response = true;
        char *sp1 = strchr(line, ' ');
        if (sp1) {
            size_t version_len = (size_t)(sp1 - line);
            if (version_len >= sizeof(msg->version)) version_len = sizeof(msg->version) - 1;
            memcpy(msg->version, line, version_len);
            msg->version[version_len] = '\0';
            while (*sp1 == ' ') sp1++;
            char *sp2 = strchr(sp1, ' ');
            size_t code_len = sp2 ? (size_t)(sp2 - sp1) : strlen(sp1);
            if (code_len >= sizeof(msg->status)) code_len = sizeof(msg->status) - 1;
            memcpy(msg->status, sp1, code_len);
            msg->status[code_len] = '\0';
        } else {
            strncpy(msg->version, line, sizeof(msg->version) - 1);
        }
    } else {
        char *sp1 = strchr(line, ' ');
        if (!sp1) {
            free(headers_copy);
            return false;
        }
        size_t method_len = (size_t)(sp1 - line);
        if (method_len >= sizeof(msg->method)) method_len = sizeof(msg->method) - 1;
        memcpy(msg->method, line, method_len);
        msg->method[method_len] = '\0';

        while (*sp1 == ' ') sp1++;
        char *sp2 = strchr(sp1, ' ');
        if (sp2) {
            size_t path_len = (size_t)(sp2 - sp1);
            if (path_len >= sizeof(msg->path)) path_len = sizeof(msg->path) - 1;
            memcpy(msg->path, sp1, path_len);
            msg->path[path_len] = '\0';
            strncpy(msg->version, sp2 + 1, sizeof(msg->version) - 1);
        }
    }

    int content_length = parse_content_length_header((const unsigned char *)headers, headers_len);
    bool chunked = header_has_chunked_encoding((const unsigned char *)headers, headers_len);
    extract_header_value((const unsigned char *)headers, headers_len, "Host:", msg->host, sizeof(msg->host));

    bool no_body_response = msg->response &&
        (strncmp(msg->status, "1", 1) == 0 || strncmp(msg->status, "204", 3) == 0 || strncmp(msg->status, "304", 3) == 0);
    bool no_body_request = !msg->response &&
        (!strcasecmp(msg->method, "GET") || !strcasecmp(msg->method, "HEAD") ||
         !strcasecmp(msg->method, "OPTIONS") || !strcasecmp(msg->method, "TRACE"));

    msg->header_end = header_end;
    msg->body_len = 0;
    msg->body_copy = NULL;

    if (!no_body_request && !no_body_response) {
        if (chunked) {
            if (body_limit < 0) {
                msg->body_len = 0;
                msg->consumed = header_end;
                free(headers_copy);
                return true;
            }

            size_t chunked_consumed = 0;
            if (parse_chunked_body(data + header_end, len - header_end, body_limit,
                                   &msg->body_copy, &msg->body_len, &chunked_consumed)) {
                msg->consumed = header_end + chunked_consumed;
                if (msg->consumed > len) msg->consumed = len;
                free(headers_copy);
                return true;
            }
            free(headers_copy);
            return false;
        }

        size_t body_to_keep = 0;
        if (body_limit < 0) {
            body_to_keep = 0;
        } else if (body_limit == 0) {
            if (content_length >= 0) {
                if (body_available < (size_t)content_length) {
                    free(headers_copy);
                    return false;
                }
                body_to_keep = (size_t)content_length;
            } else {
                body_to_keep = body_available;
            }
        } else {
            size_t target = body_available;
            if (content_length >= 0 && (size_t)content_length < target) target = (size_t)content_length;
            if ((size_t)body_limit < target) target = (size_t)body_limit;
            if (body_available < target) {
                free(headers_copy);
                return false;
            }
            body_to_keep = target;
        }

        msg->body_len = body_to_keep;
        if (content_length >= 0) {
            msg->consumed = header_end + (size_t)content_length;
        } else if (body_to_keep > 0) {
            msg->consumed = header_end + body_to_keep;
        } else {
            msg->consumed = header_end;
        }
        if (msg->consumed > len) msg->consumed = len;
    } else {
        msg->consumed = header_end;
    }

    free(headers_copy);
    return true;
}

static const char *transport_name(uint8_t proto)
{
    return proto == IPPROTO_UDP ? "udp" : "tcp";
}

static const char *family_name(int family)
{
    return family == AF_INET6 ? "ipv6" : "ipv4";
}

static void ip_to_string(int family, const unsigned char *addr, char *out, size_t out_len)
{
    if (!inet_ntop(family, addr, out, out_len)) {
        snprintf(out, out_len, "<invalid>");
    }
}

static bool emit_json_http(const struct event_t *e, const struct socket_meta *meta,
                           const struct http_message *msg,
                           const unsigned char *headers, size_t headers_len,
                           const unsigned char *body, size_t body_len)
{
    char *json = NULL;
    size_t json_len = 0;
    FILE *out = open_memstream(&json, &json_len);
    if (!out) return false;

    char src[INET6_ADDRSTRLEN] = {0};
    char dst[INET6_ADDRSTRLEN] = {0};
    char time_buf[32] = {0};
    char id_buf[25] = {0};
    char key_buf[17] = {0};
    uint64_t ms = epoch_ms();
    iso8601_ms(ms, time_buf, sizeof(time_buf));
    make_record_id(id_buf, ms);
    record_key_hex(e, meta, key_buf);
    ip_to_string(e->family, e->saddr, src, sizeof(src));
    ip_to_string(e->family, e->daddr, dst, sizeof(dst));

    fprintf(out, "{");
    fprintf(out, "\"id\":\""); json_escape_file(out, (const unsigned char *)id_buf, strlen(id_buf)); fprintf(out, "\",");
    fprintf(out, "\"ts\":%llu,", (unsigned long long)ms);
    fprintf(out, "\"time\":\""); json_escape_file(out, (const unsigned char *)time_buf, strlen(time_buf)); fprintf(out, "\",");
    fprintf(out, "\"key\":\""); json_escape_file(out, (const unsigned char *)key_buf, strlen(key_buf)); fprintf(out, "\",");
    fprintf(out, "\"family\":\"%s\",", family_name(e->family));
    fprintf(out, "\"transport\":\"%s\",", transport_name(e->l4_proto));
    fprintf(out, "\"direction\":\"%s\",", e->direction == 0 ? "ingress" : "egress");
    fprintf(out, "\"src_ip\":\""); json_escape_file(out, (const unsigned char *)src, strlen(src)); fprintf(out, "\",");
    fprintf(out, "\"src_port\":%u,", e->sport);
    fprintf(out, "\"dst_ip\":\""); json_escape_file(out, (const unsigned char *)dst, strlen(dst)); fprintf(out, "\",");
    fprintf(out, "\"dst_port\":%u,", e->dport);

    if (meta) {
        fprintf(out, "\"pid\":%d,", meta->pid);
        fprintf(out, "\"comm\":\""); json_escape_file(out, (const unsigned char *)meta->comm, strlen(meta->comm)); fprintf(out, "\",");
    } else {
        fprintf(out, "\"pid\":0,\"comm\":\"\",");
    }

    fprintf(out, "\"domain\":\"");
    json_escape_file(out, (const unsigned char *)msg->host, strlen(msg->host));
    fprintf(out, "\",");
    fprintf(out, "\"proto\":\"http\",\"type\":\"");
    json_escape_file(out, (const unsigned char *)(msg->response ? "response" : "request"), msg->response ? 8 : 7);
    fprintf(out, "\",");
    fprintf(out, "\"http\":{");
    if (msg->response) {
        fprintf(out, "\"type\":\"response\",\"status\":\"");
        json_escape_file(out, (const unsigned char *)msg->status, strlen(msg->status));
        fprintf(out, "\",");
    } else {
        fprintf(out, "\"type\":\"request\",\"method\":\"");
        json_escape_file(out, (const unsigned char *)msg->method, strlen(msg->method));
        fprintf(out, "\",\"path\":\"");
        json_escape_file(out, (const unsigned char *)msg->path, strlen(msg->path));
        fprintf(out, "\",");
    }

    fprintf(out, "\"version\":\"");
    json_escape_file(out, (const unsigned char *)msg->version, strlen(msg->version));
    fprintf(out, "\",");
    fprintf(out, "\"payload_len\":%u,", e->payload_len);
    fprintf(out, "\"body_len\":%zu,", body_len);

    if (msg->host[0]) {
        fprintf(out, "\"host\":\"");
        json_escape_file(out, (const unsigned char *)msg->host, strlen(msg->host));
        fprintf(out, "\",");
    }

    fprintf(out, "\"headers_raw\":\"");
    json_escape_file(out, headers, headers_len);
    fprintf(out, "\"");

    if (body && body_len > 0) {
        fprintf(out, ",\"body\":\"");
        json_escape_file(out, body, body_len);
        fprintf(out, "\"");
    }

    fprintf(out, "}}\n");

    fclose(out);
    if (json && json_len > 0) {
        emit_json_line(json);
    }
    free(json);
    return true;
}

static bool emit_json_https(const struct event_t *e, const struct socket_meta *meta,
                            const char *type, const char *http_version, const char *tls_version,
                            const char *sni, const char *alpn, uint16_t record_version,
                            const unsigned char *payload, size_t payload_len,
                            const char *transport)
{
    char *json = NULL;
    size_t json_len = 0;
    FILE *out = open_memstream(&json, &json_len);
    if (!out) return false;

    char src[INET6_ADDRSTRLEN] = {0};
    char dst[INET6_ADDRSTRLEN] = {0};
    char time_buf[32] = {0};
    char id_buf[25] = {0};
    char key_buf[17] = {0};
    uint64_t ms = epoch_ms();
    iso8601_ms(ms, time_buf, sizeof(time_buf));
    make_record_id(id_buf, ms);
    record_key_hex(e, meta, key_buf);
    ip_to_string(e->family, e->saddr, src, sizeof(src));
    ip_to_string(e->family, e->daddr, dst, sizeof(dst));

    fprintf(out, "{");
    fprintf(out, "\"id\":\""); json_escape_file(out, (const unsigned char *)id_buf, strlen(id_buf)); fprintf(out, "\",");
    fprintf(out, "\"ts\":%llu,", (unsigned long long)ms);
    fprintf(out, "\"time\":\""); json_escape_file(out, (const unsigned char *)time_buf, strlen(time_buf)); fprintf(out, "\",");
    fprintf(out, "\"key\":\""); json_escape_file(out, (const unsigned char *)key_buf, strlen(key_buf)); fprintf(out, "\",");
    fprintf(out, "\"family\":\"%s\",", family_name(e->family));
    fprintf(out, "\"transport\":\"%s\",", transport);
    fprintf(out, "\"direction\":\"%s\",", e->direction == 0 ? "ingress" : "egress");
    fprintf(out, "\"src_ip\":\""); json_escape_file(out, (const unsigned char *)src, strlen(src)); fprintf(out, "\",");
    fprintf(out, "\"src_port\":%u,", e->sport);
    fprintf(out, "\"dst_ip\":\""); json_escape_file(out, (const unsigned char *)dst, strlen(dst)); fprintf(out, "\",");
    fprintf(out, "\"dst_port\":%u,", e->dport);

    if (meta) {
        fprintf(out, "\"pid\":%d,", meta->pid);
        fprintf(out, "\"comm\":\""); json_escape_file(out, (const unsigned char *)meta->comm, strlen(meta->comm)); fprintf(out, "\",");
    } else {
        fprintf(out, "\"pid\":0,\"comm\":\"\",");
    }

    fprintf(out, "\"domain\":\"");
    json_escape_file(out, (const unsigned char *)sni, strlen(sni));
    fprintf(out, "\",");
    fprintf(out, "\"proto\":\"https\",\"type\":\"");
    json_escape_file(out, (const unsigned char *)type, strlen(type));
    fprintf(out, "\",");
    fprintf(out, "\"https\":{");
    fprintf(out, "\"type\":\"");
    json_escape_file(out, (const unsigned char *)type, strlen(type));
    fprintf(out, "\",");
    fprintf(out, "\"version\":\"");
    json_escape_file(out, (const unsigned char *)http_version, strlen(http_version));
    fprintf(out, "\",");
    fprintf(out, "\"tls_version\":\"");
    json_escape_file(out, (const unsigned char *)tls_version, strlen(tls_version));
    fprintf(out, "\",");
    fprintf(out, "\"domain\":\"");
    json_escape_file(out, (const unsigned char *)sni, strlen(sni));
    fprintf(out, "\",");
    fprintf(out, "\"payload_len\":%zu", payload_len);
    if (alpn && *alpn) {
        fprintf(out, ",\"alpn\":\"");
        json_escape_file(out, (const unsigned char *)alpn, strlen(alpn));
        fprintf(out, "\"");
    }
    if (record_version) {
        fprintf(out, ",\"tls_record_version\":%u", record_version);
    }
    fprintf(out, "}}\n");

    fclose(out);
    if (json && json_len > 0) {
        emit_json_line(json);
    }
    free(json);
    (void)payload;
    return true;
}

static bool looks_like_tls(const unsigned char *data, size_t len)
{
    return len >= 5 && data[0] == 0x16 && data[1] == 0x03;
}

static bool parse_packet(const unsigned char *packet, size_t len, uint8_t direction,
                         struct event_t *e)
{
    memset(e, 0, sizeof(*e));
    e->direction = direction;

    if (len < sizeof(struct ethhdr)) return false;
    const unsigned char *cursor = packet;
    const unsigned char *end = packet + len;

    const struct ethhdr *eth = (const struct ethhdr *)cursor;
    __u16 h_proto = bpf_ntohs(eth->h_proto);
    cursor += sizeof(*eth);

    if (h_proto == ETH_P_8021Q || h_proto == ETH_P_8021AD) {
        struct vlan_hdr_local {
            __be16 h_vlan_TCI;
            __be16 h_vlan_encapsulated_proto;
        };
        if (cursor + sizeof(struct vlan_hdr_local) > end) return false;
        const struct vlan_hdr_local *vh = (const struct vlan_hdr_local *)cursor;
        h_proto = bpf_ntohs(vh->h_vlan_encapsulated_proto);
        cursor += sizeof(*vh);
    }

    if (h_proto == ETH_P_IP) {
        if (cursor + sizeof(struct iphdr) > end) return false;
        const struct iphdr *iph = (const struct iphdr *)cursor;
        if (iph->ihl < 5) return false;
        size_t ip_hdr_len = (size_t)iph->ihl * 4;
        if (cursor + ip_hdr_len > end) return false;

        e->family = AF_INET;
        memcpy(e->saddr, &iph->saddr, 4);
        memcpy(e->daddr, &iph->daddr, 4);
        e->l4_proto = iph->protocol;
        cursor += ip_hdr_len;

        if (iph->protocol == IPPROTO_TCP) {
            if (cursor + sizeof(struct tcphdr) > end) return false;
            const struct tcphdr *tcph = (const struct tcphdr *)cursor;
            size_t tcp_hdr_len = (size_t)tcph->doff * 4;
            if (tcp_hdr_len < sizeof(*tcph) || cursor + tcp_hdr_len > end) return false;
            e->sport = bpf_ntohs(tcph->source);
            e->dport = bpf_ntohs(tcph->dest);
            cursor += tcp_hdr_len;
        } else if (iph->protocol == IPPROTO_UDP) {
            if (cursor + sizeof(struct udphdr) > end) return false;
            const struct udphdr *udph = (const struct udphdr *)cursor;
            e->sport = bpf_ntohs(udph->source);
            e->dport = bpf_ntohs(udph->dest);
            cursor += sizeof(*udph);
        } else {
            return false;
        }
    } else if (h_proto == ETH_P_IPV6) {
        if (cursor + sizeof(struct ipv6hdr) > end) return false;
        const struct ipv6hdr *ip6 = (const struct ipv6hdr *)cursor;

        e->family = AF_INET6;
        memcpy(e->saddr, &ip6->saddr, 16);
        memcpy(e->daddr, &ip6->daddr, 16);
        e->l4_proto = ip6->nexthdr;
        cursor += sizeof(*ip6);

        __u8 next = ip6->nexthdr;
        while (next == IPPROTO_HOPOPTS || next == IPPROTO_ROUTING ||
               next == IPPROTO_FRAGMENT || next == IPPROTO_AH ||
               next == IPPROTO_DSTOPTS) {
            if (cursor + sizeof(struct ipv6_opt_hdr) > end) return false;
            const struct ipv6_opt_hdr *opt = (const struct ipv6_opt_hdr *)cursor;
            size_t hdr_len = (size_t)(opt->hdrlen + 1) * 8;
            if (hdr_len < sizeof(*opt) || cursor + hdr_len > end) return false;
            next = opt->nexthdr;
            cursor += hdr_len;
        }

        e->l4_proto = next;
        if (next == IPPROTO_TCP) {
            if (cursor + sizeof(struct tcphdr) > end) return false;
            const struct tcphdr *tcph = (const struct tcphdr *)cursor;
            size_t tcp_hdr_len = (size_t)tcph->doff * 4;
            if (tcp_hdr_len < sizeof(*tcph) || cursor + tcp_hdr_len > end) return false;
            e->sport = bpf_ntohs(tcph->source);
            e->dport = bpf_ntohs(tcph->dest);
            cursor += tcp_hdr_len;
        } else if (next == IPPROTO_UDP) {
            if (cursor + sizeof(struct udphdr) > end) return false;
            const struct udphdr *udph = (const struct udphdr *)cursor;
            e->sport = bpf_ntohs(udph->source);
            e->dport = bpf_ntohs(udph->dest);
            cursor += sizeof(*udph);
        } else {
            return false;
        }
    } else {
        return false;
    }

    size_t payload_len = (size_t)(end - cursor);
    e->payload_len = (uint32_t)payload_len;
    e->cap_len = payload_len < CAP_PAYLOAD ? (uint32_t)payload_len : CAP_PAYLOAD;
    if (e->cap_len > 0) memcpy(e->payload, cursor, e->cap_len);
    return true;
}

static void process_tcp_flow(struct monitor_state *state, struct monitor_config *cfg,
                             const struct event_t *e, const struct socket_meta *meta);

static void process_udp_packet(const struct event_t *e, const struct socket_meta *meta);

static bool packet_matches_filters(const struct event_t *e, const struct socket_meta *meta,
                                   const struct monitor_config *cfg);

static void handle_packet(struct callback_ctx *cb, const unsigned char *packet, size_t len, uint8_t direction)
{
    struct event_t e;
    if (!parse_packet(packet, len, direction, &e)) return;

    uint64_t now = now_ms();
    if (cb->state->sockets.last_refresh_ms == 0 || now - cb->state->sockets.last_refresh_ms > 1000) {
        rebuild_socket_cache(&cb->state->sockets);
    }

    struct socket_meta meta;
    memset(&meta, 0, sizeof(meta));
    struct socket_meta fallback_meta;
    if (resolve_packet_meta(cb->state, &e, &fallback_meta)) {
        meta = fallback_meta;
    } else if (rebuild_socket_cache(&cb->state->sockets) &&
               resolve_packet_meta(cb->state, &e, &fallback_meta)) {
        meta = fallback_meta;
    }

    if (!packet_matches_filters(&e, &meta, cb->cfg)) return;

    if (e.l4_proto == IPPROTO_TCP) {
        process_tcp_flow(cb->state, cb->cfg, &e, &meta);
    } else if (e.l4_proto == IPPROTO_UDP) {
        process_udp_packet(&e, &meta);
    }

    if (cb->state->last_gc_ms == 0 || now - cb->state->last_gc_ms > 2000) {
        flow_gc(cb->state, now - 120000ULL);
        cb->state->last_gc_ms = now;
    }
}

static void drain_packet_socket(int sock, struct callback_ctx *cb)
{
    unsigned char packet[65536];
    struct sockaddr_ll from;
    socklen_t from_len = sizeof(from);

    for (;;) {
        ssize_t n = recvfrom(sock, packet, sizeof(packet), 0, (struct sockaddr *)&from, &from_len);
        if (n >= 0) {
            uint8_t direction = from.sll_pkttype == PACKET_OUTGOING ? 1 : 0;
            handle_packet(cb, packet, (size_t)n, direction);
            from_len = sizeof(from);
            continue;
        }
        if (errno == EINTR) continue;
        if (errno == EAGAIN || errno == EWOULDBLOCK) break;
        if (errno == ENETDOWN || errno == ENOBUFS) break;
        perror("recvfrom");
        break;
    }
}

static void process_tcp_flow(struct monitor_state *state, struct monitor_config *cfg,
                             const struct event_t *e, const struct socket_meta *meta)
{
    struct flow_key key;
    memset(&key, 0, sizeof(key));
    key.family = e->family;
    key.l4_proto = e->l4_proto;
    key.direction = e->direction;
    key.sport = e->sport;
    key.dport = e->dport;
    memcpy(key.saddr, e->saddr, e->family == AF_INET ? 4 : 16);
    memcpy(key.daddr, e->daddr, e->family == AF_INET ? 4 : 16);

    struct flow_state *flow = flow_get(state, &key);
    if (!flow) return;
    flow->last_seen_ms = now_ms();

    if (meta && meta->pid > 0) {
        flow->pid = meta->pid;
        strncpy(flow->comm, meta->comm, sizeof(flow->comm) - 1);
        flow->comm[sizeof(flow->comm) - 1] = '\0';
    }

    struct flow_key reverse_key;
    flow_reverse_key(e, &reverse_key);
    struct flow_state *peer = flow_find(state, &reverse_key);

    struct socket_meta peer_meta;
    memset(&peer_meta, 0, sizeof(peer_meta));
    if (meta->pid <= 0 && peer && peer->pid > 0) {
        peer_meta.pid = peer->pid;
        strncpy(peer_meta.comm, peer->comm, sizeof(peer_meta.comm) - 1);
        peer_meta.comm[sizeof(peer_meta.comm) - 1] = '\0';
        meta = &peer_meta;
    }

    if (!buffer_append(&flow->tcp_buffer, e->payload, e->cap_len)) return;

    if (!flow->tls_emitted && looks_like_tls(flow->tcp_buffer.data, flow->tcp_buffer.len)) {
        char sni[256] = {0};
        char alpn[128] = {0};
        uint16_t ver = 0;
        if (parse_tls_clienthello(flow->tcp_buffer.data, (uint32_t)flow->tcp_buffer.len,
                                  sni, sizeof(sni), alpn, sizeof(alpn), &ver)) {
            if (sni[0]) {
                strncpy(flow->domain, sni, sizeof(flow->domain) - 1);
                flow->domain[sizeof(flow->domain) - 1] = '\0';
                flow->has_domain = true;
            }
            emit_json_https(e, meta, "request", http_version_from_alpn(alpn), tls_version_name(ver),
                            sni, alpn, ver, flow->tcp_buffer.data, flow->tcp_buffer.len, "tcp");
            flow->tls_emitted = true;
            buffer_consume(&flow->tcp_buffer, flow->tcp_buffer.len);
            return;
        }

        memset(alpn, 0, sizeof(alpn));
        ver = 0;
        if (parse_tls_serverhello(flow->tcp_buffer.data, (uint32_t)flow->tcp_buffer.len,
                                  alpn, sizeof(alpn), &ver)) {
            if (!sni[0] && peer && peer->has_domain) {
                strncpy(sni, peer->domain, sizeof(sni) - 1);
                sni[sizeof(sni) - 1] = '\0';
            }
            emit_json_https(e, meta, "response", http_version_from_alpn(alpn), tls_version_name(ver),
                            sni, alpn, ver, flow->tcp_buffer.data, flow->tcp_buffer.len, "tcp");
            flow->tls_emitted = true;
            buffer_consume(&flow->tcp_buffer, flow->tcp_buffer.len);
            return;
        }
    }

    while (flow->tcp_buffer.len > 0) {
        struct http_message msg;
        if (!parse_http_message(flow->tcp_buffer.data, flow->tcp_buffer.len, cfg->body_limit, &msg)) {
            break;
        }

        if (msg.consumed == 0 || msg.consumed > flow->tcp_buffer.len) break;

        if (!msg.host[0] && peer && peer->has_domain) {
            strncpy(msg.host, peer->domain, sizeof(msg.host) - 1);
            msg.host[sizeof(msg.host) - 1] = '\0';
        }

        if (msg.host[0]) {
            strncpy(flow->domain, msg.host, sizeof(flow->domain) - 1);
            flow->domain[sizeof(flow->domain) - 1] = '\0';
            flow->has_domain = true;
        }

        const unsigned char *body = NULL;
        size_t body_len = msg.body_len;
        if (msg.body_copy && msg.body_len > 0) {
            body = msg.body_copy;
        } else if (msg.body_len > 0 && msg.header_end + msg.body_len <= flow->tcp_buffer.len) {
            body = flow->tcp_buffer.data + msg.header_end;
        }

        emit_json_http(e, meta, &msg, flow->tcp_buffer.data, msg.header_end, body, body_len);
        http_message_free(&msg);
        buffer_consume(&flow->tcp_buffer, msg.consumed);
        flow->tls_emitted = false;
    }
}

static void process_udp_packet(const struct event_t *e, const struct socket_meta *meta)
{
    if (e->cap_len < 6) return;
    const unsigned char *payload = e->payload;
    uint32_t payload_len = e->cap_len;
    const unsigned char *tls_data = payload;
    uint32_t tls_len = payload_len;
    if (!find_quic_tls_payload(payload, payload_len, &tls_data, &tls_len)) return;

    char sni[256] = {0};
    char alpn[128] = {0};
    uint16_t ver = 0;
    if (parse_tls_clienthello(tls_data, tls_len, sni, sizeof(sni), alpn, sizeof(alpn), &ver)) {
        emit_json_https(e, meta, "request", http_version_from_alpn(alpn), tls_version_name(ver),
                        sni, alpn, ver, e->payload, e->cap_len, "udp");
    }
}

static bool packet_matches_filters(const struct event_t *e, const struct socket_meta *meta,
                                   const struct monitor_config *cfg)
{
    if (cfg->direction_filter >= 0 && e->direction != (uint8_t)cfg->direction_filter) return false;

    if (cfg->have_sport_filter && e->direction == 0 && e->sport != cfg->sport_filter) return false;
    if (cfg->have_dport_filter && e->direction == 1 && e->dport != cfg->dport_filter) return false;

    if (!cidr_set_accepts(&cfg->src_rules, e->family, e->saddr)) return false;
    if (!cidr_set_accepts(&cfg->dst_rules, e->family, e->daddr)) return false;

    if (cfg->have_pid_filter || cfg->have_comm_filter) {
        if (!meta) return false;
        if (cfg->have_pid_filter && meta->pid != cfg->pid_filter) return false;
        if (cfg->have_comm_filter && strncmp(meta->comm, cfg->comm_filter, COMM_LEN) != 0) return false;
    }

    return true;
}

static void usage(const char *prog)
{
    fprintf(stderr,
            "Usage: %s [-interface iface] [-direction ingress|egress] [-pid pid] [-comm comm] [-src spec] [-dst spec] [-sport port] [-dport port] [-max-body-size n]\n"
            "  -interface      monitor interface\n"
            "  -direction      ingress | egress, default both\n"
            "  -pid            process pid filter\n"
            "  -comm           process name filter (comm)\n"
            "  -src    source IP/CIDR list, !prefix means deny, deny wins\n"
            "  -dst    destination IP/CIDR list, !prefix means deny, deny wins\n"
            "  -sport  ingress only port filter\n"
            "  -dport  egress only port filter\n"
            "  -max-body-size  request/response body capture length, <0 no body, 0 unlimited, >0 truncated\n",
            prog);
}

static bool parse_long_long(const char *s, long long *out)
{
    char *end = NULL;
    errno = 0;
    long long v = strtoll(s, &end, 10);
    if (errno || !end || *end != '\0') return false;
    *out = v;
    return true;
}

static bool parse_port(const char *s, uint16_t *out)
{
    long long v = 0;
    if (!parse_long_long(s, &v)) return false;
    if (v < 0 || v > 65535) return false;
    *out = (uint16_t)v;
    return true;
}

static bool parse_args(int argc, char **argv, struct monitor_config *cfg)
{
    memset(cfg, 0, sizeof(*cfg));
    cfg->direction_filter = -1;
    cfg->body_limit = -1;

    for (int i = 1; i < argc; ++i) {
        const char *arg = argv[i];
        if (!strcmp(arg, "-h") || !strcmp(arg, "--help")) {
            usage(argv[0]);
            return false;
        } else if (!strcmp(arg, "-interface") && i + 1 < argc) {
            cfg->ifname = argv[++i];
        } else if (!strcmp(arg, "-direction") && i + 1 < argc) {
            const char *v = argv[++i];
            if (!strcmp(v, "ingress")) cfg->direction_filter = 0;
            else if (!strcmp(v, "egress")) cfg->direction_filter = 1;
            else {
                fprintf(stderr, "Invalid -direction value: %s\n", v);
                return false;
            }
        } else if (!strcmp(arg, "-pid") && i + 1 < argc) {
            long long pid = 0;
            if (!parse_long_long(argv[++i], &pid) || pid < 0) {
                fprintf(stderr, "Invalid pid\n");
                return false;
            }
            cfg->have_pid_filter = true;
            cfg->pid_filter = (pid_t)pid;
        } else if (!strcmp(arg, "-comm") && i + 1 < argc) {
            strncpy(cfg->comm_filter, argv[++i], COMM_LEN - 1);
            cfg->comm_filter[COMM_LEN - 1] = '\0';
            cfg->have_comm_filter = true;
        } else if (!strcmp(arg, "-src") && i + 1 < argc) {
            if (!parse_cidr_list(argv[++i], &cfg->src_rules)) {
                fprintf(stderr, "Invalid -src specification\n");
                return false;
            }
        } else if (!strcmp(arg, "-dst") && i + 1 < argc) {
            if (!parse_cidr_list(argv[++i], &cfg->dst_rules)) {
                fprintf(stderr, "Invalid -dst specification\n");
                return false;
            }
        } else if (!strcmp(arg, "-sport") && i + 1 < argc) {
            if (!parse_port(argv[++i], &cfg->sport_filter)) {
                fprintf(stderr, "Invalid -sport value\n");
                return false;
            }
            cfg->have_sport_filter = true;
        } else if (!strcmp(arg, "-dport") && i + 1 < argc) {
            if (!parse_port(argv[++i], &cfg->dport_filter)) {
                fprintf(stderr, "Invalid -dport value\n");
                return false;
            }
            cfg->have_dport_filter = true;
        } else if (!strcmp(arg, "-max-body-size") && i + 1 < argc) {
            if (!parse_long_long(argv[++i], &cfg->body_limit)) {
                fprintf(stderr, "Invalid -max-body-size value\n");
                return false;
            }
        } else {
            fprintf(stderr, "Unknown argument: %s\n", arg);
            usage(argv[0]);
            return false;
        }
    }

    return true;
}

static bool open_and_attach_bpf(const char *obj_file, int sock_fd,
                                struct bpf_object **obj_out,
                                struct bpf_link **tcp_connect_link_out,
                                struct bpf_link **tcp_accept_link_out,
                                struct bpf_link **tcp_sendmsg_link_out,
                                struct bpf_link **udp_sendmsg_link_out)
{
    struct bpf_object *obj = bpf_object__open_file(obj_file, NULL);
    if (libbpf_get_error(obj)) {
        fprintf(stderr, "Failed to open %s\n", obj_file);
        return false;
    }

    if (bpf_object__load(obj)) {
        fprintf(stderr, "Failed to load %s\n", obj_file);
        bpf_object__close(obj);
        return false;
    }
    struct bpf_program *tcp_connect_prog = bpf_object__find_program_by_name(obj, "track_tcp_connect");
    struct bpf_program *tcp_accept_prog = bpf_object__find_program_by_name(obj, "track_tcp_accept");
    struct bpf_program *tcp_send_prog = bpf_object__find_program_by_name(obj, "track_tcp_sendmsg");
    struct bpf_program *udp_send_prog = bpf_object__find_program_by_name(obj, "track_udp_sendmsg");
    if (!tcp_connect_prog || !tcp_accept_prog ||
        !tcp_send_prog || !udp_send_prog) {
        fprintf(stderr, "failed to find kprobe programs\n");
        bpf_object__close(obj);
        return false;
    }

    struct bpf_link *tcp_connect_link = bpf_program__attach_kprobe(tcp_connect_prog, false, "tcp_connect");
    if (libbpf_get_error(tcp_connect_link)) {
        fprintf(stderr, "failed to attach track_tcp_connect\n");
        bpf_object__close(obj);
        return false;
    }

    struct bpf_link *tcp_accept_link = bpf_program__attach_kprobe(tcp_accept_prog, true, "inet_csk_accept");
    if (libbpf_get_error(tcp_accept_link)) {
        fprintf(stderr, "failed to attach track_tcp_accept\n");
        bpf_link__destroy(tcp_connect_link);
        bpf_object__close(obj);
        return false;
    }

    struct bpf_link *tcp_send_link = bpf_program__attach_kprobe(tcp_send_prog, false, "tcp_sendmsg");
    if (libbpf_get_error(tcp_send_link)) {
        fprintf(stderr, "failed to attach track_tcp_sendmsg\n");
        bpf_link__destroy(tcp_accept_link);
        bpf_link__destroy(tcp_connect_link);
        bpf_object__close(obj);
        return false;
    }

    struct bpf_link *udp_send_link = bpf_program__attach_kprobe(udp_send_prog, false, "udp_sendmsg");
    if (libbpf_get_error(udp_send_link)) {
        fprintf(stderr, "failed to attach track_udp_sendmsg\n");
        bpf_link__destroy(tcp_send_link);
        bpf_link__destroy(tcp_accept_link);
        bpf_link__destroy(tcp_connect_link);
        bpf_link__destroy(tcp_send_link);
        bpf_object__close(obj);
        return false;
    }

    struct bpf_program *prog = bpf_object__find_program_by_name(obj, "capture_prog");
    if (!prog) {
        fprintf(stderr, "failed to find capture_prog\n");
        bpf_link__destroy(udp_send_link);
        bpf_link__destroy(tcp_send_link);
        bpf_object__close(obj);
        return false;
    }

    int prog_fd = bpf_program__fd(prog);
    if (setsockopt(sock_fd, SOL_SOCKET, SO_ATTACH_BPF, &prog_fd, sizeof(prog_fd)) < 0) {
        perror("SO_ATTACH_BPF");
        bpf_link__destroy(tcp_accept_link);
        bpf_link__destroy(tcp_connect_link);
        bpf_link__destroy(udp_send_link);
        bpf_link__destroy(tcp_send_link);
        bpf_object__close(obj);
        return false;
    }
    *obj_out = obj;
    if (tcp_connect_link_out) *tcp_connect_link_out = tcp_connect_link;
    if (tcp_accept_link_out) *tcp_accept_link_out = tcp_accept_link;
    if (tcp_sendmsg_link_out) *tcp_sendmsg_link_out = tcp_send_link;
    if (udp_sendmsg_link_out) *udp_sendmsg_link_out = udp_send_link;
    return true;
}

int main(int argc, char **argv)
{
    const char *obj_file = "ebpf_capture.o";
    struct monitor_config cfg;
    if (!parse_args(argc, argv, &cfg)) return 1;

    signal(SIGINT, sigint_handler);

    struct rlimit rl = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &rl)) {
        perror("setrlimit");
        return 1;
    }

    setvbuf(stdout, NULL, _IONBF, 0);

    int sock = socket(AF_PACKET, SOCK_RAW, htons(ETH_P_ALL));
    if (sock < 0) {
        perror("socket");
        return 1;
    }

    if (cfg.ifname) {
        cfg.ifindex = (int)if_nametoindex(cfg.ifname);
        if (cfg.ifindex == 0) {
            fprintf(stderr, "Invalid interface: %s\n", cfg.ifname);
            close(sock);
            return 1;
        }
        struct sockaddr_ll sll;
        memset(&sll, 0, sizeof(sll));
        sll.sll_family = AF_PACKET;
        sll.sll_protocol = htons(ETH_P_ALL);
        sll.sll_ifindex = cfg.ifindex;
        if (bind(sock, (struct sockaddr *)&sll, sizeof(sll)) < 0) {
            perror("bind");
            close(sock);
            return 1;
        }
    }

    int sock_flags = fcntl(sock, F_GETFL, 0);
    if (sock_flags >= 0) {
        fcntl(sock, F_SETFL, sock_flags | O_NONBLOCK);
    }

    struct bpf_object *obj = NULL;
    struct bpf_link *tcp_connect_link = NULL;
    struct bpf_link *tcp_accept_link = NULL;
    struct bpf_link *tcp_sendmsg_link = NULL;
    struct bpf_link *udp_sendmsg_link = NULL;
    if (!open_and_attach_bpf(obj_file, sock, &obj,
                             &tcp_connect_link,
                             &tcp_accept_link,
                             &tcp_sendmsg_link,
                             &udp_sendmsg_link)) {
        close(sock);
        return 1;
    }

    struct monitor_state state;
    memset(&state, 0, sizeof(state));
    state.flow_owner_map_fd = bpf_object__find_map_fd_by_name(obj, "flow_owner_map");
    state.last_gc_ms = 0;
    state.sockets.max_items = SOCKET_CACHE_MAX_ITEMS;
    if (!rebuild_socket_cache(&state.sockets)) {
        fprintf(stderr, "warning: failed to build socket cache\n");
    }

    struct callback_ctx cb_ctx = { .state = &state, .cfg = &cfg };

    fprintf(stderr, "listening for HTTP/HTTPS events on %s, Ctrl-C to exit\n",
            cfg.ifname ? cfg.ifname : "all interfaces");

    while (!exiting) {
        struct pollfd fds[1];
        memset(fds, 0, sizeof(fds));
        fds[0].fd = sock;
        fds[0].events = POLLIN;

        int ready = poll(fds, 1, 1000);
        if (ready < 0) {
            if (errno == EINTR) continue;
            perror("poll");
            break;
        }

        if (fds[0].revents & POLLIN) {
            drain_packet_socket(sock, &cb_ctx);
        }
    }

    bpf_link__destroy(tcp_accept_link);
    bpf_link__destroy(tcp_connect_link);
    bpf_link__destroy(udp_sendmsg_link);
    bpf_link__destroy(tcp_sendmsg_link);
    bpf_object__close(obj);
    close(sock);

    free(state.sockets.items);
    for (size_t i = 0; i < FLOW_BUCKETS; ++i) {
        struct flow_state *flow = state.flows[i];
        while (flow) {
            struct flow_state *next = flow->next;
            buffer_free(&flow->tcp_buffer);
            free(flow);
            flow = next;
        }
    }
    cidr_set_free(&cfg.src_rules);
    cidr_set_free(&cfg.dst_rules);
    return 0;
}