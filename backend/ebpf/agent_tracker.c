// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;
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
    u32 pid;
    u32 ppid;
    u32 uid;
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
    u32 extra1;
    u32 extra2;
    u64 extra3;
    char extra4[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

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
    for (int i = 0; i < 16; i++) e->net_addr[i] = 0;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = 0;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->extra4[i] = 0;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), comm);

    u64 uid_gid = bpf_get_current_uid_gid();
    e->uid = (u32)uid_gid;

    // Get PPID
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    e->ppid = BPF_CORE_READ(task, real_parent, tgid);
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

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
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

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
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

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
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

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
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

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: ioctl (comm-only, request at args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_ioctl")
int tracepoint__syscalls__sys_enter_ioctl(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_IOCTL;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // request

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "Special Resource Interaction (ioctl)", 38);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_ioctl")
int tracepoint__syscalls__sys_exit_ioctl(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: bind (comm-only, network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_BIND;
    meta.tag_id = tag_id;
    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_LISTEN, 0);

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket bind", 12);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_bind")
int tracepoint__syscalls__sys_exit_bind(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: sendto (comm-only, network from args[4], len at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_sendto")
int tracepoint__syscalls__sys_enter_sendto(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_SENDTO;
    meta.tag_id = tag_id;
    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);
    meta.extra3 = (u32)ctx->args[2]; // byte count

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendto", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_sendto")
int tracepoint__syscalls__sys_exit_sendto(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: recvfrom (comm-only, network from args[4], len at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_recvfrom")
int tracepoint__syscalls__sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_RECVFROM;
    meta.tag_id = tag_id;
    meta.extra3 = (u32)ctx->args[2]; // byte count
    meta.addr_ptr = ctx->args[4]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket recvfrom", 16);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvfrom")
int tracepoint__syscalls__sys_exit_recvfrom(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    // Read the address now that the syscall has completed
    if (meta.addr_ptr && ctx->ret > 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, (u32)ctx->ret);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: read (no path, fd at args[0], count at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_read")
int tracepoint__syscalls__sys_enter_read(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_READ;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.extra3 = (u32)ctx->args[2]; // count

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_read")
int tracepoint__syscalls__sys_exit_read(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: write (no path, fd at args[0], count at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_write")
int tracepoint__syscalls__sys_enter_write(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_WRITE;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.extra3 = (u32)ctx->args[2]; // count

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_write")
int tracepoint__syscalls__sys_exit_write(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: open (path at args[0], flags at args[1], mode at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_open")
int tracepoint__syscalls__sys_enter_open(struct trace_event_raw_sys_enter *ctx) {
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
    meta.type = TYPE_OPEN;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // flags
    meta.extra2 = (u32)ctx->args[2]; // mode

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_open")
int tracepoint__syscalls__sys_exit_open(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: chmod (path at args[0], mode at args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_chmod")
int tracepoint__syscalls__sys_enter_chmod(struct trace_event_raw_sys_enter *ctx) {
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
    meta.type = TYPE_CHMOD;
    meta.tag_id = tag_id;
    meta.extra2 = (u32)ctx->args[1]; // mode

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_chmod")
int tracepoint__syscalls__sys_exit_chmod(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: chown (path at args[0], uid at args[1], gid at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_chown")
int tracepoint__syscalls__sys_enter_chown(struct trace_event_raw_sys_enter *ctx) {
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
    meta.type = TYPE_CHOWN;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // uid
    meta.extra2 = (u32)ctx->args[2]; // gid

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_chown")
int tracepoint__syscalls__sys_exit_chown(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: rename (path at args[0]=oldpath, extra4=args[1]=newpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_rename")
int tracepoint__syscalls__sys_enter_rename(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *oldpath = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, oldpath);
    const char *newpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, newpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_RENAME;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_rename")
int tracepoint__syscalls__sys_exit_rename(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: link (path at args[0]=oldpath/target, extra4=args[1]=newpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_link")
int tracepoint__syscalls__sys_enter_link(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *oldpath = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, oldpath);
    const char *newpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, newpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_LINK;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_link")
int tracepoint__syscalls__sys_exit_link(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: symlink (path at args[0]=target, extra4=args[1]=linkpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_symlink")
int tracepoint__syscalls__sys_enter_symlink(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *target = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, target);
    const char *linkpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, linkpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_SYMLINK;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_symlink")
int tracepoint__syscalls__sys_exit_symlink(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: mknod (path at args[0], mode at args[1], dev at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_mknod")
int tracepoint__syscalls__sys_enter_mknod(struct trace_event_raw_sys_enter *ctx) {
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
    meta.type = TYPE_MKNOD;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // mode
    meta.extra2 = (u32)ctx->args[2]; // dev

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mknod")
int tracepoint__syscalls__sys_exit_mknod(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: clone (no path, flags at args[0])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_clone")
int tracepoint__syscalls__sys_enter_clone(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_CLONE;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // flags

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_clone")
int tracepoint__syscalls__sys_exit_clone(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: exit_group (no path, status at args[0])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_exit_group")
int tracepoint__syscalls__sys_enter_exit_group(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_EXIT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // status

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_exit_group")
int tracepoint__syscalls__sys_exit_exit_group(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: socket (no path, domain at args[0], type at args[1], protocol at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__syscalls__sys_enter_socket(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_SOCKET;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // domain
    meta.extra2 = (u32)ctx->args[1]; // type
    meta.extra3 = (u32)ctx->args[2]; // protocol

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
int tracepoint__syscalls__sys_exit_socket(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: accept (no path, fd at args[0], network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_accept")
int tracepoint__syscalls__sys_enter_accept(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_ACCEPT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.addr_ptr = ctx->args[1]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket accept", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_accept")
int tracepoint__syscalls__sys_exit_accept(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    if (meta.addr_ptr && ctx->ret >= 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, 0);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: accept4 (no path, fd at args[0], network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_accept4")
int tracepoint__syscalls__sys_enter_accept4(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_ACCEPT4;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.addr_ptr = ctx->args[1]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket accept4", 15);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_accept4")
int tracepoint__syscalls__sys_exit_accept4(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    if (meta.addr_ptr && ctx->ret >= 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, 0);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

char _license[] SEC("license") = "Dual MIT/GPL";
