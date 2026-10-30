//go:build ignore
// Opt-in cgroup IPv4 classifier, including IPv4-mapped IPv6 sockets.
// Not attached by the normal backend.
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define FEATURE_COUNT 6
#define GUARD_SCHEMA 1
#define GUARD_SHADOW 1
#define GUARD_ENFORCE 2

struct guard_config {
    __u32 version;
    __u32 mode;
    __u32 generation;
    __s32 bias;
    __s32 threshold;
    __s32 weights[FEATURE_COUNT];
};

struct guard_stats {
    __u64 examined;
    __u64 reviewed;
    __u64 matched;
    __u64 blocked;
};

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct guard_config);
} kernel_guard_config SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 64);
    __type(key, __u32);
    __type(value, __u32);
} kernel_guard_ports SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct guard_stats);
} kernel_guard_stats SEC(".maps");

// Conservative external-candidate filter, NOT an authoritative IP reputation
// or exhaustive IANA classification. Reserved/documentation ranges are exempt.
static __always_inline int external_ipv4(__u32 ip)
{
    __u32 a = ip >> 24;
    __u32 b = (ip >> 16) & 255;
    __u32 d = (ip >> 8) & 255;
    if (a == 0 || a == 10 || a == 127 || a >= 224) return 0;
    if (a == 100 && b >= 64 && b <= 127) return 0;
    if (a == 169 && b == 254) return 0;
    if (a == 172 && b >= 16 && b <= 31) return 0;
    if (a == 192 && (b == 168 || (b == 0 && d == 2))) return 0;
    if (a == 198 && ((b == 18 || b == 19) || (b == 51 && d == 100))) return 0;
    if (a == 203 && b == 0 && d == 113) return 0;
    return 1;
}

static __always_inline int evaluate_ipv4(__u32 ip, __u32 port)
{
    __u32 zero = 0;
    struct guard_config *cfg = bpf_map_lookup_elem(&kernel_guard_config, &zero);
    if (!cfg || cfg->version != GUARD_SCHEMA ||
        (cfg->mode != GUARD_SHADOW && cfg->mode != GUARD_ENFORCE) ||
        !cfg->generation || cfg->threshold <= 0) return 1;

    struct guard_stats *stats = bpf_map_lookup_elem(&kernel_guard_stats, &zero);
    if (stats) stats->examined++;
    if (!external_ipv4(ip)) return 1;

    // Mandatory independent operator-reviewed target scope. The model alone
    // cannot deny generic outbound connections or modify the scope.
    __u32 *reviewed = bpf_map_lookup_elem(&kernel_guard_ports, &port);
    if (!reviewed || *reviewed != 1) return 1;
    if (stats) stats->reviewed++;

    __s32 score = cfg->bias + cfg->weights[0];
    if (port == 22) score += cfg->weights[1];
    if (port == 4444) score += cfg->weights[2];
    if (port >= 49152) score += cfg->weights[3];
    if (port == 443) score += cfg->weights[4];
    if (port == 53) score += cfg->weights[5];

    if (score < cfg->threshold) return 1;
    if (stats) stats->matched++;
    if (cfg->mode == GUARD_ENFORCE) {
        if (stats) stats->blocked++;
        return 0; // cgroup/connect / sendmsg: deny
    }
    return 1;
}

static __always_inline int evaluate4(struct bpf_sock_addr *ctx)
{
    if (ctx->family != 2) return 1; // AF_INET
    return evaluate_ipv4(bpf_ntohl(ctx->user_ip4),
                         bpf_ntohs(ctx->user_port));
}

// ::ffff:a.b.c.d passes through the same v4 scorer and port scope.
// Native IPv6 must not be silently classified as IPv4 or blocked.
static __always_inline int evaluate_mapped6(struct bpf_sock_addr *ctx)
{
    if (ctx->family != 10) return 1; // AF_INET6
    if (ctx->user_ip6[0] != 0 || ctx->user_ip6[1] != 0 ||
        bpf_ntohl(ctx->user_ip6[2]) != 0x0000ffff) return 1;
    return evaluate_ipv4(bpf_ntohl(ctx->user_ip6[3]),
                         bpf_ntohs(ctx->user_port));
}

SEC("cgroup/connect4")
int kernel_guard_connect4(struct bpf_sock_addr *ctx)
{
    return evaluate4(ctx);
}

SEC("cgroup/sendmsg4")
int kernel_guard_sendmsg4(struct bpf_sock_addr *ctx)
{
    return evaluate4(ctx);
}

SEC("cgroup/connect6")
int kernel_guard_connect6(struct bpf_sock_addr *ctx)
{
    return evaluate_mapped6(ctx);
}

SEC("cgroup/sendmsg6")
int kernel_guard_sendmsg6(struct bpf_sock_addr *ctx)
{
    return evaluate_mapped6(ctx);
}

char LICENSE[] SEC("license") = "GPL";
