// +build ignore

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long long u64;

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

struct event {
    u32 pid;
    u32 type;
    char comm[TASK_COMM_LEN];
    char path[MAX_PATH_LEN];
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
    __type(value, u8); // 1 = registered
} agent_pids SEC(".maps");

// Map to store tracked command names (e.g., "git", "npm")
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, char[16]);
    __type(value, u8);
} tracked_comms SEC(".maps");

static __always_inline u8 is_tracked(u32 pid, char *comm) {
    u8 *res = bpf_map_lookup_elem(&agent_pids, &pid);
    if (res) return 1;
    res = bpf_map_lookup_elem(&tracked_comms, comm);
    if (res) return 1;
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint__syscalls__sys_enter_execve(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_EXECVE;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_openat")
int tracepoint__syscalls__sys_enter_openat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_OPENAT;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_connect")
int tracepoint__syscalls__sys_enter_connect(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_CONNECT;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    // For connect, we just log the action for now. 
    // Capturing IP/Port requires more complex parsing of sockaddr.
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "Network Connection");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_mkdirat")
int tracepoint__syscalls__sys_enter_mkdirat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_MKDIRAT;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_unlinkat")
int tracepoint__syscalls__sys_enter_unlinkat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_UNLINKAT;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_ioctl")
int tracepoint__syscalls__sys_enter_ioctl(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_IOCTL;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    // For ioctl, we log the command code in hex
    u64 cmd = ctx->args[1];
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "Special Resource (ioctl)");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    if (!is_tracked(pid, comm)) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = TYPE_BIND;
    bpf_probe_read_kernel(&e->comm, sizeof(e->comm), &comm);
    
    bpf_probe_read_kernel_str(&e->path, sizeof(e->path), "Network Bind");

    bpf_ringbuf_submit(e, 0);
    return 0;
}

char _license[] SEC("license") = "Dual MIT/GPL";
