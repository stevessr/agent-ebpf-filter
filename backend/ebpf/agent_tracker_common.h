// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;
typedef int s32;
typedef long long s64;

#define AF_INET 2
#define AF_INET6 10

#define NET_DIR_OUTGOING 1
#define NET_DIR_INCOMING 2
#define NET_DIR_LISTEN 3

// CO-RE compatible task_struct definition
struct task_struct {
    struct task_struct *real_parent;
    int tgid;
} __attribute__((preserve_access_index));

#define MAX_PATH_LEN 256
#define TASK_COMM_LEN 16
#define LPM_PATH_LEN 64

// Event types
#define TYPE_EXECVE 0
#define TYPE_OPENAT 1
#define TYPE_CONNECT 2
#define TYPE_MKDIRAT 3
#define TYPE_UNLINKAT 4
#define TYPE_IOCTL 5
#define TYPE_BIND 6
#define TYPE_SENDTO 7
#define TYPE_RECVFROM 8
#define TYPE_READ     9
#define TYPE_WRITE    10
#define TYPE_OPEN     11
#define TYPE_CHMOD    12
#define TYPE_CHOWN    13
#define TYPE_RENAME   14
#define TYPE_LINK     15
#define TYPE_SYMLINK  16
#define TYPE_MKNOD    17
#define TYPE_CLONE    18
#define TYPE_EXIT     19
#define TYPE_SOCKET   20
#define TYPE_ACCEPT   21
#define TYPE_ACCEPT4  22
#define TYPE_GENERIC_SYSCALL 25
#define TYPE_PROCESS_FORK 26
#define TYPE_PROCESS_EXEC 27
#define TYPE_PROCESS_EXIT 28
#define TYPE_WAIT4 29
#define TYPE_SEMANTIC_ALERT 30
#define TYPE_TCP_CONNECT 31
#define TYPE_TCP_CLOSE 32
#define TYPE_TCP_STATE_CHANGE 33
#define TYPE_DNS_QUERY 34

struct trace_entry {
    short unsigned int type;
    unsigned char flags;
    unsigned char preempt_count;
    int pid;
};

struct trace_event_raw_sys_enter {
    struct trace_entry ent;
    long int id;
    long unsigned int args[6];
    char __data[0];
};

struct trace_event_raw_sys_exit {
    struct trace_entry ent;
    long int id;
    long int ret;
    char __data[0];
};

struct trace_event_raw_sched_process_fork {
    struct trace_entry ent;
    u32 parent_comm_loc;
    s32 parent_pid;
    u32 child_comm_loc;
    s32 child_pid;
};

struct trace_event_raw_sched_process_exec {
    struct trace_entry ent;
    u32 filename_loc;
    s32 pid;
    s32 old_pid;
};

struct trace_event_raw_sched_process_exit {
    struct trace_entry ent;
    char comm[TASK_COMM_LEN];
    s32 pid;
    s32 prio;
    u8 group_dead;
};

// TCP tracepoint structures for flow-level network tracking
struct trace_event_raw_tcp_event {
    struct trace_entry ent;
    u64 saddr_v6[2]; // IPv4 mapped: saddr_v6[0] holds saddr, or full IPv6
    u64 daddr_v6[2];
    u16 sport;
    u16 dport;
    u32 __pad;
};

struct trace_event_raw_inet_sock_set_state {
    struct trace_entry ent;
    u64 saddr_v6[2];
    u64 daddr_v6[2];
    u16 sport;
    u16 dport;
    u32 __pad;
    u8 protocol;
    u8 oldstate;
    u8 newstate;
    u8 __pad2;
};

struct in_addr {
    u32 s_addr;
};

struct in6_addr {
    u8 s6_addr[16];
};

struct sockaddr {
    u16 sa_family;
    char sa_data[14];
};

struct sockaddr_in {
    u16 sin_family;
    u16 sin_port;
    struct in_addr sin_addr;
    u8 sin_zero[8];
};

struct sockaddr_in6 {
    u16 sin6_family;
    u16 sin6_port;
    u32 sin6_flowinfo;
    struct in6_addr sin6_addr;
    u32 sin6_scope_id;
};

struct event {
    u32 pid;     // thread ID (tid)
    u32 tgid;    // thread group ID (process ID)
    u32 ppid;
    u32 uid;
    u32 gid;
    u32 type;
    u32 tag_id;
    char comm[TASK_COMM_LEN];
    char path[MAX_PATH_LEN];
    u32 net_family;
    u32 net_direction;
    u32 net_bytes;
    u32 net_port;
    char net_addr[16];
    s64 retval;
    u64 duration_ns;
    u64 cgroup_id;
    u32 extra1;
    u32 extra2;
    u64 extra3;
    char extra4[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

struct collector_stats {
    u64 ringbuf_events_total;
    u64 ringbuf_reserve_failed_total;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct collector_stats);
} collector_stats SEC(".maps");

// Map to store registered agent PIDs
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u32); // tag_id
} agent_pids SEC(".maps");

// Map to store tracked command names (e.g., "git", "npm")
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, char[16]);
    __type(value, u32); // tag_id
} tracked_comms SEC(".maps");

// Map to store tracked paths (exact match)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 512);
    __type(key, char[MAX_PATH_LEN]);
    __type(value, u32); // tag_id
} tracked_paths SEC(".maps");

// Exit context map: stores enter-side metadata for sys_exit correlation
struct exit_meta {
    u32 type;
    u32 tag_id;
    u32 extra1;
    u32 extra2;
    u64 extra3;
    u32 net_family;
    u32 net_direction;
    u32 net_bytes;
    u32 net_port;
    char net_addr[16];
    u64 addr_ptr;
    u64 start_ns;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, u64);
    __type(value, struct exit_meta);
} exit_ctx SEC(".maps");

// Exit path context map: stores path data for sys_exit (split due to 512-byte stack limit)
struct exit_path_data {
    char path[MAX_PATH_LEN];
    char extra4[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 2048);
    __type(key, u64);
    __type(value, struct exit_path_data);
} exit_path_ctx SEC(".maps");

static __always_inline void account_ringbuf_reserve_failed(void) {
    u32 key = 0;
    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);
    if (stats) {
        stats->ringbuf_reserve_failed_total++;
    }
}

static __always_inline struct event *reserve_event(void) {
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        account_ringbuf_reserve_failed();
    }
    return e;
}

static __always_inline void submit_event(struct event *e) {
    u32 key = 0;
    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);
    if (stats) {
        stats->ringbuf_events_total++;
    }
    bpf_ringbuf_submit(e, 0);
}

// Per-CPU buffer for exit_path_data (avoids 512-byte stack allocation)
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct exit_path_data);
} exit_path_buf SEC(".maps");

// LPM trie for path prefix matching
struct lpm_key {
    u32 prefix_len;
    u8 data[LPM_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 256);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, struct lpm_key);
    __type(value, u32);
} tracked_prefixes SEC(".maps");

static __always_inline u32 get_tag_id(u32 pid, char *comm, char *path) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (tag) return *tag;
    tag = bpf_map_lookup_elem(&tracked_comms, comm);
    if (tag) return *tag;
    if (path) {
        tag = bpf_map_lookup_elem(&tracked_paths, path);
        if (tag) return *tag;

        // LPM trie prefix match
        u32 path_len = 0;
        #pragma unroll
        for (path_len = 0; path_len < LPM_PATH_LEN; path_len++) {
            if (path[path_len] == '\0') break;
        }
        if (path_len > 0) {
            struct lpm_key lpmk = {};
            lpmk.prefix_len = path_len * 8;
            __builtin_memcpy(lpmk.data, path, LPM_PATH_LEN);
            tag = bpf_map_lookup_elem(&tracked_prefixes, &lpmk);
            if (tag) return *tag;
        }
    }
    return 0;
}

static __always_inline void read_tracepoint_data_loc_str(char *dst, u32 size, const void *ctx, u32 data_loc) {
    u32 offset = data_loc & 0xFFFF;
    if (offset == 0) {
        return;
    }
    const char *src = (const char *)ctx + offset;
    bpf_probe_read_kernel_str(dst, size, src);
}

static __always_inline void fill_base_info(struct event *e, u32 pid, u32 tag_id, char *comm) {
    e->pid = pid;
    e->tag_id = tag_id;
    e->net_family = 0;
    e->net_direction = 0;
    e->net_bytes = 0;
    e->net_port = 0;
    e->extra1 = 0;
    e->extra2 = 0;
    e->extra3 = 0;
    e->duration_ns = 0;
    for (int i = 0; i < 16; i++) e->net_addr[i] = 0;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = 0;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->extra4[i] = 0;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), comm);

    u64 uid_gid = bpf_get_current_uid_gid();
    e->uid = (u32)uid_gid;
    e->gid = (u32)(uid_gid >> 32);
    e->cgroup_id = bpf_get_current_cgroup_id();

    // Get PPID and TGID
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    e->ppid = BPF_CORE_READ(task, real_parent, tgid);
    e->tgid = BPF_CORE_READ(task, tgid);
}

static __always_inline void fill_network_endpoint(struct event *e, const void *user_addr, u32 direction, u32 bytes) {
    e->net_direction = direction;
    e->net_bytes = bytes;

    if (!user_addr) {
        return;
    }

    struct sockaddr sa = {};
    if (bpf_probe_read_user(&sa, sizeof(sa), user_addr) < 0) {
        return;
    }

    e->net_family = sa.sa_family;
    if (sa.sa_family == AF_INET) {
        struct sockaddr_in sin = {};
        if (bpf_probe_read_user(&sin, sizeof(sin), user_addr) < 0) {
            return;
        }
        e->net_port = (u32)__builtin_bswap16(sin.sin_port);
        for (int i = 0; i < 16; i++) e->net_addr[i] = 0;
        for (int i = 0; i < 4; i++) e->net_addr[i] = ((u8 *)&sin.sin_addr.s_addr)[i];
    } else if (sa.sa_family == AF_INET6) {
        struct sockaddr_in6 sin6 = {};
        if (bpf_probe_read_user(&sin6, sizeof(sin6), user_addr) < 0) {
            return;
        }
        e->net_port = (u32)__builtin_bswap16(sin6.sin6_port);
        for (int i = 0; i < 16; i++) {
            e->net_addr[i] = sin6.sin6_addr.s6_addr[i];
        }
    }
}

static __always_inline void fill_network_meta(struct exit_meta *meta, const void *user_addr, u32 direction, u32 bytes) {
    meta->net_direction = direction;
    meta->net_bytes = bytes;

    if (!user_addr) {
        return;
    }

    struct sockaddr sa = {};
    if (bpf_probe_read_user(&sa, sizeof(sa), user_addr) < 0) {
        return;
    }

    meta->net_family = sa.sa_family;
    if (sa.sa_family == AF_INET) {
        struct sockaddr_in sin = {};
        if (bpf_probe_read_user(&sin, sizeof(sin), user_addr) < 0) {
            return;
        }
        meta->net_port = (u32)__builtin_bswap16(sin.sin_port);
        for (int i = 0; i < 16; i++) meta->net_addr[i] = 0;
        for (int i = 0; i < 4; i++) meta->net_addr[i] = ((u8 *)&sin.sin_addr.s_addr)[i];
    } else if (sa.sa_family == AF_INET6) {
        struct sockaddr_in6 sin6 = {};
        if (bpf_probe_read_user(&sin6, sizeof(sin6), user_addr) < 0) {
            return;
        }
        meta->net_port = (u32)__builtin_bswap16(sin6.sin6_port);
        for (int i = 0; i < 16; i++) {
            meta->net_addr[i] = sin6.sin6_addr.s6_addr[i];
        }
    }
}

static __always_inline void store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
}

// Convenience inline for sys_exit handlers that only need pid_tgid correlation
// Returns 0 if no context was found (not a tracked syscall)
static __always_inline u32 consume_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    struct exit_meta *m = bpf_map_lookup_elem(&exit_ctx, &pid_tgid);
    if (!m) return 0;
    __builtin_memcpy(meta, m, sizeof(*meta));
    bpf_map_delete_elem(&exit_ctx, &pid_tgid);
    return meta->tag_id;
}

static __always_inline void fill_from_exit_meta(struct event *e, u64 pid_tgid, struct exit_meta *meta) {
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    fill_base_info(e, (u32)(pid_tgid >> 32), meta->tag_id, comm);
    e->type = meta->type;
    e->retval = 0;  // to be filled by caller
    e->duration_ns = 0;
    e->extra1 = meta->extra1;
    e->extra2 = meta->extra2;
    e->extra3 = meta->extra3;
    e->net_family = meta->net_family;
    e->net_direction = meta->net_direction;
    e->net_bytes = meta->net_bytes;
    e->net_port = meta->net_port;
    __builtin_memcpy(e->net_addr, meta->net_addr, 16);
}

// ============================================================
// sched tracepoints: process fork / exec / exit
// ============================================================
SEC("tracepoint/sched/sched_process_fork")
int tracepoint__sched__sched_process_fork(struct trace_event_raw_sched_process_fork *ctx) {
    u32 parent_pid = (u32)ctx->parent_pid;
    u32 child_pid = (u32)ctx->child_pid;
    if (parent_pid == 0 || child_pid == 0) return 0;

    u32 *tag = bpf_map_lookup_elem(&agent_pids, &parent_pid);
    if (!tag) return 0;

    bpf_map_update_elem(&agent_pids, &child_pid, tag, BPF_ANY);

    struct event *e = reserve_event();
    if (!e) return 0;

    char parent_comm[TASK_COMM_LEN] = {};
    read_tracepoint_data_loc_str(parent_comm, sizeof(parent_comm), ctx, ctx->parent_comm_loc);
    fill_base_info(e, parent_pid, *tag, parent_comm);
    e->type = TYPE_PROCESS_FORK;
    e->retval = child_pid;
    e->extra1 = child_pid;
    read_tracepoint_data_loc_str(e->path, MAX_PATH_LEN, ctx, ctx->child_comm_loc);
    submit_event(e);
    return 0;
}

SEC("tracepoint/sched/sched_process_exec")
int tracepoint__sched__sched_process_exec(struct trace_event_raw_sched_process_exec *ctx) {
    u32 pid = (u32)ctx->pid;
    u32 old_pid = (u32)ctx->old_pid;
    if (pid == 0) return 0;

    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (!tag && old_pid != 0) {
        tag = bpf_map_lookup_elem(&agent_pids, &old_pid);
    }
    if (!tag) return 0;

    if (old_pid != 0 && old_pid != pid) {
        bpf_map_update_elem(&agent_pids, &pid, tag, BPF_ANY);
        bpf_map_delete_elem(&agent_pids, &old_pid);
    }

    struct event *e = reserve_event();
    if (!e) return 0;

    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    fill_base_info(e, pid, *tag, comm);
    e->type = TYPE_PROCESS_EXEC;
    e->extra1 = old_pid;
    read_tracepoint_data_loc_str(e->path, MAX_PATH_LEN, ctx, ctx->filename_loc);
    submit_event(e);
    return 0;
}

SEC("tracepoint/sched/sched_process_exit")
int tracepoint__sched__sched_process_exit(struct trace_event_raw_sched_process_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tid = (u32)pid_tgid;
    u32 tgid = (u32)(pid_tgid >> 32);

    u32 *tag = bpf_map_lookup_elem(&agent_pids, &tgid);
    if (!tag) {
        tag = bpf_map_lookup_elem(&agent_pids, &tid);
    }
    if (!tag) return 0;

    struct event *e = reserve_event();
    if (e) {
        fill_base_info(e, tgid, *tag, ctx->comm);
        e->type = TYPE_PROCESS_EXIT;
        e->extra1 = ctx->group_dead ? 1 : 0;
        submit_event(e);
    }

    bpf_map_delete_elem(&agent_pids, &tid);
    if (ctx->group_dead) {
        bpf_map_delete_elem(&agent_pids, &tgid);
    }
    return 0;
}

// ============================================================
// TCP flow tracepoints for flow-level network attribution
// ============================================================

static __always_inline void format_ipv4_from_v6(char *buf, u64 saddr_v6_lo) {
    u32 ip = (u32)saddr_v6_lo;
    // Format as "x.x.x.x" using simple snprintf
    buf[0] = '0' + ((ip >> 0)  & 0xFF);
    // Use a compact hex representation since bpf_snprintf may not be available
    // Format: hex bytes for the address
    for (int i = 0; i < 4; i++) {
        u8 byte = (ip >> (i * 8)) & 0xFF;
        // Approximate formatting - store as raw bytes in network order
        buf[i] = (char)byte;
    }
    buf[4] = '\0';
}

static __always_inline int emit_tcp_flow_event(u32 pid, u32 tgid, u32 type,
                                                u64 saddr_lo, u64 daddr_lo,
                                                u16 sport, u16 dport,
                                                u8 oldstate, u8 newstate,
                                                u32 tag_id) {
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    struct event *e = reserve_event();
    if (!e) return 0;

    fill_base_info(e, tgid, tag_id, comm);
    e->type = type;

    // Pack flow info into the event
    e->net_family = (saddr_lo > 0xFFFFFFFFULL) ? AF_INET6 : AF_INET;
    e->net_direction = NET_DIR_OUTGOING;
    e->net_port = dport;
    e->extra1 = sport;           // source port
    e->extra2 = (u32)(saddr_lo); // source addr (lower 32 bits)
    e->extra3 = daddr_lo;        // dest addr
    // Store old/new state for inet_sock_set_state events
    if (oldstate || newstate) {
        e->duration_ns = ((u64)oldstate << 32) | newstate;
    }

    // Pack address bytes into net_addr (16 bytes available)
    // Store saddr and daddr as compact hex for display
    for (int i = 0; i < 4; i++) {
        e->net_addr[i] = (u8)((daddr_lo >> (i * 8)) & 0xFF);
    }
    // Port as net_bytes for convenience
    e->net_bytes = sport;

    submit_event(e);
    return 1;
}

SEC("tracepoint/tcp/tcp_connect")
int tracepoint__tcp__tcp_connect(struct trace_event_raw_tcp_event *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    u32 tid = (u32)pid_tgid;

    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (!tag_id) {
        tag_id = get_tag_id(tid, comm, NULL);
    }
    if (!tag_id) return 0;

    emit_tcp_flow_event(tid, tgid, TYPE_TCP_CONNECT,
                        ctx->saddr_v6[0], ctx->daddr_v6[0],
                        ctx->sport, ctx->dport, 0, 0, tag_id);
    return 0;
}

SEC("tracepoint/tcp/tcp_close")
int tracepoint__tcp__tcp_close(struct trace_event_raw_tcp_event *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    u32 tid = (u32)pid_tgid;

    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (!tag_id) {
        tag_id = get_tag_id(tid, comm, NULL);
    }
    if (!tag_id) return 0;

    emit_tcp_flow_event(tid, tgid, TYPE_TCP_CLOSE,
                        ctx->saddr_v6[0], ctx->daddr_v6[0],
                        ctx->sport, ctx->dport, 0, 0, tag_id);
    return 0;
}

SEC("tracepoint/sock/inet_sock_set_state")
int tracepoint__sock__inet_sock_set_state(struct trace_event_raw_inet_sock_set_state *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    u32 tid = (u32)pid_tgid;

    // Only track TCP state changes
    if (ctx->protocol != 6) return 0; // IPPROTO_TCP

    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (!tag_id) {
        tag_id = get_tag_id(tid, comm, NULL);
    }
    if (!tag_id) return 0;

    emit_tcp_flow_event(tid, tgid, TYPE_TCP_STATE_CHANGE,
                        ctx->saddr_v6[0], ctx->daddr_v6[0],
                        ctx->sport, ctx->dport,
                        ctx->oldstate, ctx->newstate, tag_id);
    return 0;
}

// DNS query extraction helper for UDP sendto to port 53
static __always_inline int detect_dns_query(struct event *e, const void *buf, u32 len, u32 dport) {
    if (dport != 53 || len < 20 || !buf) return 0;
    // DNS header is 12 bytes, try to read the QNAME
    // We need at least a minimal DNS packet
    char dns_data[64] = {};
    if (bpf_probe_read_user(dns_data, sizeof(dns_data) < len ? sizeof(dns_data) : len, buf) < 0) {
        return 0;
    }
    // Skip DNS header (12 bytes) and read QNAME
    // Simple detection: look for printable domain chars starting at offset 12
    u8 qname_len = dns_data[12];
    if (qname_len > 0 && qname_len < 64) {
        int pos = 13;
        int out_pos = 0;
        for (int i = 0; i < qname_len && pos < 63 && out_pos < 255; i++) {
            if (dns_data[pos] >= 0x20 && dns_data[pos] < 0x7F) {
                e->path[out_pos++] = dns_data[pos];
            }
            pos++;
        }
        e->path[out_pos] = '\0';
        return out_pos > 0 ? 1 : 0;
    }
    return 0;
}

// ============================================================
// sys_enter / sys_exit: execve (path at args[0])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint__syscalls__sys_enter_execve(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_EXECVE;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_execve")
int tracepoint__syscalls__sys_exit_execve(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: openat (path at args[1], flags at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_openat")
int tracepoint__syscalls__sys_enter_openat(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_OPENAT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[2]; // flags

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_openat")
int tracepoint__syscalls__sys_exit_openat(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: connect (comm-only, network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_connect")
int tracepoint__syscalls__sys_enter_connect(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_CONNECT;
    meta.tag_id = tag_id;
    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_OUTGOING, 0);

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket connect", 15);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")
int tracepoint__syscalls__sys_exit_connect(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: mkdirat (path at args[1], mode at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_mkdirat")
int tracepoint__syscalls__sys_enter_mkdirat(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_MKDIRAT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[2]; // mode

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mkdirat")
int tracepoint__syscalls__sys_exit_mkdirat(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: unlinkat (path at args[1], flags at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_unlinkat")
int tracepoint__syscalls__sys_enter_unlinkat(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_UNLINKAT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[2]; // flags

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_unlinkat")
int tracepoint__syscalls__sys_exit_unlinkat(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: ioctl (comm-only, request at args[1])
// ============================================================
