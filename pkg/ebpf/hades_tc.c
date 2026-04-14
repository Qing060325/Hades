// +build ignore

// hades_tc.c — Hades eBPF TC 程序
//
// 功能：在 TC ingress hook 处拦截网络数据包，
// 对匹配 DIRECT 规则的 IP-CIDR 流量直接放行（TC_ACT_OK），
// 对匹配 REJECT 规则的流量直接丢弃（TC_ACT_SHOT），
// 其余流量交由用户态代理处理（TC_ACT_OK 正常传递到 TUN）。
//
// BPF Maps：
//   - direct_cidr4: IPv4 CIDR 规则（前缀长度 + 网络地址）
//   - direct_cidr6: IPv6 CIDR 规则
//   - reject_set:   需要拒绝的 IP 集合
//   - stats:        包/字节统计计数器

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/ipv6.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

// ─── BPF Maps ─────────────────────────────────────────────

// IPv4 CIDR 规则：key = 网络字节序 IPv4 地址, value = 前缀长度
struct cidr4_key {
    __u32 prefixlen;   // BPF 树使用的前缀长度字段
    __u32 addr;        // 网络字节序的 IPv4 地址
};

struct cidr4_value {
    __u32 action;      // 1 = DIRECT (放行), 2 = REJECT (丢弃)
};

// IPv6 CIDR 规则
struct cidr6_key {
    __u32 prefixlen;
    __u32 addr[4];     // 网络字节序的 IPv6 地址
};

struct cidr6_value {
    __u32 action;      // 1 = DIRECT, 2 = REJECT
};

// LPM Trie 用于 CIDR 匹配
struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 10240);
    __type(key, struct cidr4_key);
    __type(value, struct cidr4_value);
    __uint(map_flags, BPF_F_NO_PREALLOC);
} direct_cidr4 SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 10240);
    __type(key, struct cidr6_key);
    __type(value, struct cidr6_value);
    __uint(map_flags, BPF_F_NO_PREALLOC);
} direct_cidr6 SEC(".maps");

// 统计计数器
struct stats_key {
    __u32 action;  // 1=direct, 2=reject, 3=proxy
};

struct stats_value {
    __u64 packets;
    __u64 bytes;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, struct stats_key);
    __type(value, struct stats_value);
} stats SEC(".maps");

// ─── 辅助函数 ────────────────────────────────────────────

static __always_inline void update_stats(__u32 action, __u64 pkt_bytes) {
    struct stats_key key = { .action = action };
    struct stats_value *val = bpf_map_lookup_elem(&stats, &key);
    if (val) {
        __sync_fetch_and_add(&val->packets, 1);
        __sync_fetch_and_add(&val->bytes, pkt_bytes);
    } else {
        struct stats_value new_val = { .packets = 1, .bytes = pkt_bytes };
        bpf_map_update_elem(&stats, &key, &new_val, BPF_ANY);
    }
}

// ─── TC Ingress 程序 ─────────────────────────────────────

SEC("tc")
int hades_tc_ingress(struct __sk_buff *skb) {
    void *data     = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;

    // 解析以太网头
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    // 只处理 IP 数据包
    if (eth->h_proto != bpf_htons(ETH_P_IP) && eth->h_proto != bpf_htons(ETH_P_IPV6))
        return TC_ACT_OK;

    __u64 pkt_bytes = skb->len;

    // ─── IPv4 处理 ───────────────────────────────────────
    if (eth->h_proto == bpf_htons(ETH_P_IP)) {
        struct iphdr *iph = (void *)(eth + 1);
        if ((void *)(iph + 1) > data_end)
            return TC_ACT_OK;

        // 在 LPM Trie 中查找目标 IP
        struct cidr4_key key = {};
        key.prefixlen = 32;
        key.addr = iph->daddr;

        // 尝试最长前缀匹配
        struct cidr4_value *val = bpf_map_lookup_elem(&direct_cidr4, &key);
        if (val) {
            if (val->action == 1) {
                // DIRECT: 直接放行，不经过用户态代理
                update_stats(1, pkt_bytes);
                return TC_ACT_OK;
            } else if (val->action == 2) {
                // REJECT: 直接丢弃
                update_stats(2, pkt_bytes);
                return TC_ACT_SHOT;
            }
        }
    }

    // ─── IPv6 处理 ───────────────────────────────────────
    if (eth->h_proto == bpf_htons(ETH_P_IPV6)) {
        struct ipv6hdr *ip6h = (void *)(eth + 1);
        if ((void *)(ip6h + 1) > data_end)
            return TC_ACT_OK;

        struct cidr6_key key = {};
        key.prefixlen = 128;
        __builtin_memcpy(key.addr, &ip6h->daddr, sizeof(key.addr));

        struct cidr6_value *val = bpf_map_lookup_elem(&direct_cidr6, &key);
        if (val) {
            if (val->action == 1) {
                update_stats(1, pkt_bytes);
                return TC_ACT_OK;
            } else if (val->action == 2) {
                update_stats(2, pkt_bytes);
                return TC_ACT_SHOT;
            }
        }
    }

    // 未匹配任何 eBPF 规则，交由用户态代理处理
    update_stats(3, pkt_bytes);
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
