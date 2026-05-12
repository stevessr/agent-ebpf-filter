// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;

struct ip6_block_key {
    u32 addr[4];
};

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

// BPF map: set of destination IPv6 addresses blocked for all tracked cgroups.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct ip6_block_key);
    __type(value, u32);
} ip6_blocklist SEC(".maps");

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

static __always_inline int ipv6_is_v4_mapped(struct ip6_block_key *addr)
{
    return addr->addr[0] == 0 && addr->addr[1] == 0 && addr->addr[2] == 0x0000ffff;
}

static __always_inline int mapped_v4_is_blocked(struct ip6_block_key *addr)
{
    if (!ipv6_is_v4_mapped(addr)) {
        return 0;
    }

    u32 mapped_v4 = addr->addr[3];
    u32 *ip_blocked = bpf_map_lookup_elem(&ip_blocklist, &mapped_v4);
    return ip_blocked && *ip_blocked;
}

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
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    // Check IP blocklist (host-order IPv4 key)
    u32 dst_ip = bpf_ntohl(ctx->user_ip4);
    u32 *ip_blocked = bpf_map_lookup_elem(&ip_blocklist, &dst_ip);
    if (ip_blocked && *ip_blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    // Check port blocklist (host-order TCP/UDP port key)
    u32 dst_port = bpf_ntohs(ctx->user_port);
    u32 *port_blocked = bpf_map_lookup_elem(&port_blocklist, &dst_port);
    if (port_blocked && *port_blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    // Update allow stats
    u32 zero = 0;
    struct cgroup_sandbox_stats *stats = bpf_map_lookup_elem(&cgroup_sandbox_stats, &zero);
    if (stats) {
        stats->connect_checked++;
        stats->connect_allowed++;
    }

    return 1; // allow
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

    // Check IPv6 destination blocklist (host-order IPv6 u32 words).
    struct ip6_block_key dst_ip6 = {};
    dst_ip6.addr[0] = bpf_ntohl(ctx->user_ip6[0]);
    dst_ip6.addr[1] = bpf_ntohl(ctx->user_ip6[1]);
    dst_ip6.addr[2] = bpf_ntohl(ctx->user_ip6[2]);
    dst_ip6.addr[3] = bpf_ntohl(ctx->user_ip6[3]);
    u32 *ip6_blocked = bpf_map_lookup_elem(&ip6_blocklist, &dst_ip6);
    if (ip6_blocked && *ip6_blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }
    // Also honor IPv4 block entries for IPv4-mapped IPv6 sockets
    // (::ffff:a.b.c.d), otherwise AF_INET6 clients can bypass IPv4-only
    // destination blocks for the same endpoint.
    if (mapped_v4_is_blocked(&dst_ip6)) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    // Check port blocklist (host-order TCP/UDP port key)
    u32 dst_port = bpf_ntohs(ctx->user_port);
    u32 *port_blocked = bpf_map_lookup_elem(&port_blocklist, &dst_port);
    if (port_blocked && *port_blocked) {
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

// IPv4 UDP sendmsg handler — attached to cgroup/sendmsg4. This closes the
// unconnected UDP sendto()/sendmsg() gap that cgroup/connect4 cannot see.
SEC("cgroup/sendmsg4")
int cgroup_sandbox_sendmsg4(struct bpf_sock_addr *ctx) {
    if (ctx->family != 2) { // AF_INET
        return 1; // allow
    }

    u64 cgroup_id = bpf_get_current_cgroup_id();

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

    u32 dst_ip = bpf_ntohl(ctx->user_ip4);
    u32 *ip_blocked = bpf_map_lookup_elem(&ip_blocklist, &dst_ip);
    if (ip_blocked && *ip_blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    u32 dst_port = bpf_ntohs(ctx->user_port);
    u32 *port_blocked = bpf_map_lookup_elem(&port_blocklist, &dst_port);
    if (port_blocked && *port_blocked) {
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

// IPv6 UDP sendmsg handler — attached to cgroup/sendmsg6. This closes the
// unconnected UDP sendto()/sendmsg() gap that cgroup/connect6 cannot see.
SEC("cgroup/sendmsg6")
int cgroup_sandbox_sendmsg6(struct bpf_sock_addr *ctx) {
    if (ctx->family != 10) { // AF_INET6
        return 1; // allow
    }

    u64 cgroup_id = bpf_get_current_cgroup_id();

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

    struct ip6_block_key dst_ip6 = {};
    dst_ip6.addr[0] = bpf_ntohl(ctx->user_ip6[0]);
    dst_ip6.addr[1] = bpf_ntohl(ctx->user_ip6[1]);
    dst_ip6.addr[2] = bpf_ntohl(ctx->user_ip6[2]);
    dst_ip6.addr[3] = bpf_ntohl(ctx->user_ip6[3]);
    u32 *ip6_blocked = bpf_map_lookup_elem(&ip6_blocklist, &dst_ip6);
    if (ip6_blocked && *ip6_blocked) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }
    // Also honor IPv4 block entries for IPv4-mapped IPv6 UDP destinations.
    if (mapped_v4_is_blocked(&dst_ip6)) {
        u32 key = 0;
        struct cgroup_sandbox_stats *s = bpf_map_lookup_elem(&cgroup_sandbox_stats, &key);
        if (s) {
            s->connect_checked++;
            s->connect_blocked++;
        }
        return 0; // block
    }

    u32 dst_port = bpf_ntohs(ctx->user_port);
    u32 *port_blocked = bpf_map_lookup_elem(&port_blocklist, &dst_port);
    if (port_blocked && *port_blocked) {
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

char _license[] SEC("license") = "Dual MIT/GPL";
