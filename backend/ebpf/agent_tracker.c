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
    u32 type; // 0 for execve, 1 for openat
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

SEC("tracepoint/syscalls/sys_enter_execve")
int tracepoint__syscalls__sys_enter_execve(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;

    u8 *is_agent = bpf_map_lookup_elem(&agent_pids, &pid);
    if (!is_agent) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = 0; // execve
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_openat")
int tracepoint__syscalls__sys_enter_openat(struct trace_event_raw_sys_enter *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;

    u8 *is_agent = bpf_map_lookup_elem(&agent_pids, &pid);
    if (!is_agent) return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->pid = pid;
    e->type = 1; // openat
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    
    // For openat, the filename is the second argument (args[1])
    const char *filename = (const char *)ctx->args[1];
    bpf_probe_read_user_str(&e->path, sizeof(e->path), filename);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

char _license[] SEC("license") = "Dual MIT/GPL";
