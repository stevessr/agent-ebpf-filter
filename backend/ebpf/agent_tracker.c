// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;

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

static __always_inline u32 get_tag_id(u32 pid, char *comm, char *path) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (tag) return *tag;
    tag = bpf_map_lookup_elem(&tracked_comms, comm);
    if (tag) return *tag;
    if (path) {
        tag = bpf_map_lookup_elem(&tracked_paths, path);
        if (tag) return *tag;
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
    for (int i = 0; i < 16; i++) e->net_addr[i] = 0;
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

SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint__syscalls__sys_enter_execve(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    char path[MAX_PATH_LEN];
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(&path, sizeof(path), filename);

    u32 tag_id = get_tag_id(pid, comm, path);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_EXECVE;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = path[i];

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_openat")
int tracepoint__syscalls__sys_enter_openat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    char path[MAX_PATH_LEN];
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&path, sizeof(path), filename);

    u32 tag_id = get_tag_id(pid, comm, path);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_OPENAT;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = path[i];

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_connect")
int tracepoint__syscalls__sys_enter_connect(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_CONNECT;
    fill_network_endpoint(e, (const void *)ctx->args[1], NET_DIR_OUTGOING, 0);
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "socket connect");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_mkdirat")
int tracepoint__syscalls__sys_enter_mkdirat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    char path[MAX_PATH_LEN];
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&path, sizeof(path), filename);

    u32 tag_id = get_tag_id(pid, comm, path);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_MKDIRAT;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = path[i];

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_unlinkat")
int tracepoint__syscalls__sys_enter_unlinkat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    char path[MAX_PATH_LEN];
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&path, sizeof(path), filename);

    u32 tag_id = get_tag_id(pid, comm, path);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_UNLINKAT;
    for (int i = 0; i < MAX_PATH_LEN; i++) e->path[i] = path[i];

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_ioctl")
int tracepoint__syscalls__sys_enter_ioctl(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_IOCTL;
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "Special Resource Interaction (ioctl)");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_BIND;
    fill_network_endpoint(e, (const void *)ctx->args[1], NET_DIR_LISTEN, 0);
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "socket bind");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_sendto")
int tracepoint__syscalls__sys_enter_sendto(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_SENDTO;
    fill_network_endpoint(e, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "socket sendto");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_recvfrom")
int tracepoint__syscalls__sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_base_info(e, pid, tag_id, comm);
    e->type = TYPE_RECVFROM;
    fill_network_endpoint(e, (const void *)ctx->args[4], NET_DIR_INCOMING, (u32)ctx->args[2]);
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "socket recvfrom");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

char _license[] SEC("license") = "Dual MIT/GPL";
