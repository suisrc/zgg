// eBPF socket filter：最小化实现，只放行所有包给 userspace。
// 这样可以稳定加载，HTTP/HTTPS 的识别和过滤全部放到 userspace 完成。

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

SEC("socket")
int capture_prog(struct __sk_buff *skb)
{
    return skb->len;
}

char LICENSE[] SEC("license") = "GPL";