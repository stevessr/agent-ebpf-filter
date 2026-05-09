// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;

// BPF map: set of cgroup IDs that are BLOCKED from outbound connections.
// Key = cgroup_id (u64), Value = u32 (1 = blocked, 0 = allowed)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, u64);
    __type(value, u32);
} cgroup_blocklist SEC(".maps");

// BPF map: set of destination IPs (u32, IPv4) blocked for all tracked cgroups.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u32);
} ip_blocklist SEC(".maps");

// BPF map: set of destination ports blocked for all tracked cgroups.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, u32);
    __type(value, u32);
} port_blocklist SEC(".maps");

// Statistics for cgroup sandbox decisions
struct cgroup_sandbox_stats {
    u64 connect_checked;
    u64 connect_blocked;
    u64 connect_allowed;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct cgroup_sandbox_stats);
} cgroup_sandbox_stats SEC(".maps");

// IPv4 connect handler — attached to cgroup/connect4
SEC("cgroup/connect4")
int cgroup_sandbox_connect4(struct bpf_sock_addr *ctx) {
    // Only handle IPv4
    if (ctx->family != 2) { // AF_INET
        return 1; // allow
    }

    u64 cgroup_id = bpf_get_current_cgroup_id();

    // Check cgroup blocklist
    u32 *blocked = bpf_map_lookup_elem(&cgroup_blocklist, &cgroup_id);
    if (blocked && *blocked) {
        goto block;
    }

    // Check IP blocklist (destination IP in network byte order)
    u32 dst_ip = ctx->user_ip4;
    u32 *ip_blocked = bpf_map_lookup_elem(&ip_blocklist, &dst_ip);
    if (ip_blocked && *ip_blocked) {
        goto block;
    }

    // Check port blocklist (destination port in network byte order)
    u32 dst_port_be = ctx->user_port;
    u32 dst_port = ((dst_port_be & 0xFF) << 8) | ((dst_port_be >> 8) & 0xFF);
    u32 *port_blocked = bpf_map_lookup_elem(&port_blocklist, &dst_port);
    if (port_blocked && *port_blocked) {
        goto block;
    }

    // Update allow stats
    u32 zero = 0;
    struct cgroup_sandbox_stats *stats = bpf_map_lookup_elem(&cgroup_sandbox_stats, &zero);
    if (stats) {
        stats->connect_checked++;
        stats->connect_allowed++;
    }

    return 1; // allow

block:
    // Update block stats
    u32 key = 0;
    struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
    if (s) {
        s->connect_checked++;
        s->connect_blocked++;
    }

    return 0; // block
}

// IPv6 connect handler — attached to cgroup/connect6
SEC("cgroup/connect6")
int cgroup_sandbox_connect6(struct bpf_sock_addr *ctx) {
    // Only handle IPv6
    if (ctx->family != 10) { // AF_INET6
        return 1; // allow
    }

    u64 cgroup_id = bpf_get_current_cgroup_id();

    // Check cgroup blocklist
    u32 *blocked = bpf_map_lookup_elem(&cgroup_blocklist, &cgroup_id);
    if (blocked && *blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    u32 key = 0;
    struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
    if (s) {
        s->connect_checked++;
        s->connect_allowed++;
    }

    return 1; // allow
}

// Optional: file_open LSM hook for filesystem sandboxing
// This is experimental and requires CONFIG_BPF_LSM=y in the kernel
SEC("lsm/file_open")
int BPF_PROG(lsm_file_open, struct file *file, int ret) {
    // Allow by default (this is a LSM hook — return 0 to allow, negative to deny)
    // In production, check file path against a blocklist map

    // For now, just pass through (audit-only mode)
    return 0;
}

char _license[] SEC("license") = "Dual MIT/GPL";
