//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define CODEX_FRAG_SIZE 960

struct codex_syscall_event {
	__u64 timestamp_ns;
	__u32 pid;
	__u32 data_len;
	__u8 direction;
	char comm[16];
	char data[CODEX_FRAG_SIZE];
};

const struct codex_syscall_event *codex_event_anchor __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 256 * 1024);
} codex_events SEC(".maps");

static __always_inline bool is_codex() {
	char comm[16];
	bpf_get_current_comm(&comm, sizeof(comm));
	return comm[0] == 'c' && comm[1] == 'o' && comm[2] == 'd' &&
	       comm[3] == 'e' && comm[4] == 'x' && comm[5] == '\0';
}

SEC("tracepoint/syscalls/sys_enter_write")
int trace_codex_write(struct trace_event_raw_sys_enter *ctx) {
	if (!is_codex())
		return 0;

	unsigned long buf_ptr = ctx->args[1];
	unsigned long count = ctx->args[2];

	if (count == 0 || count > CODEX_FRAG_SIZE)
		return 0;

	struct codex_syscall_event *evt = bpf_ringbuf_reserve(&codex_events, sizeof(*evt), 0);
	if (!evt)
		return 0;

	evt->timestamp_ns = bpf_ktime_get_ns();
	evt->pid = bpf_get_current_pid_tgid() >> 32;
	evt->data_len = count;
	evt->direction = 1;
	bpf_get_current_comm(&evt->comm, sizeof(evt->comm));
	bpf_probe_read_user(evt->data, evt->data_len, (const void *)buf_ptr);

	bpf_ringbuf_submit(evt, 0);
	return 0;
}

SEC("tracepoint/syscalls/sys_exit_read")
int trace_codex_read(struct trace_event_raw_sys_exit *ctx) {
	if (!is_codex())
		return 0;

	long ret = ctx->ret;
	if (ret <= 0 || ret > CODEX_FRAG_SIZE)
		return 0;

	struct codex_syscall_event *evt = bpf_ringbuf_reserve(&codex_events, sizeof(*evt), 0);
	if (!evt)
		return 0;

	evt->timestamp_ns = bpf_ktime_get_ns();
	evt->pid = bpf_get_current_pid_tgid() >> 32;
	evt->data_len = ret;
	evt->direction = 0;
	bpf_get_current_comm(&evt->comm, sizeof(evt->comm));

	bpf_ringbuf_submit(evt, 0);
	return 0;
}
