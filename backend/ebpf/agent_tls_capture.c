//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define TLS_FRAG_SIZE 960
#define TLS_MAX_FRAGS 18
#define TLS_MAX_CAPTURE_SIZE (TLS_FRAG_SIZE * TLS_MAX_FRAGS)

#define TLS_LIB_OPENSSL 0
#define TLS_LIB_GO 1
#define TLS_LIB_GNUTLS 2
#define TLS_LIB_NSS 3
#define TLS_LIB_RUSTLS 4 // rustls (static-pie Rust binaries: codex, cursor)

#define TLS_DIR_RECV 0
#define TLS_DIR_SEND 1

#define TLS_FLAG_TRUNCATED 1

#define TLS_FUNC_SSL_WRITE 1
#define TLS_FUNC_SSL_READ 2
#define TLS_FUNC_SSL_WRITE_EX 3
#define TLS_FUNC_SSL_READ_EX 4
#define TLS_FUNC_GNUTLS_RECORD_SEND 5
#define TLS_FUNC_GNUTLS_RECORD_RECV 6
#define TLS_FUNC_PR_WRITE 7
#define TLS_FUNC_PR_READ 8
#define TLS_FUNC_GO_CONN_WRITE 9
#define TLS_FUNC_GO_CONN_READ 10
#define TLS_FUNC_SSL_WRITE_EX2 11
#define TLS_FUNC_RUSTLS_ENCRYPT_OUTGOING 12 // send: rustls encrypt_outgoing

struct tls_fragment {
	__u64 timestamp_ns;
	__u32 pid;
	__u32 tgid;
	__u32 data_len;
	__u32 total_len;
	__u32 original_len;
	__u16 frag_index;
	__u16 frag_count;
	__u8 lib_type;
	__u8 direction;
	__u8 flags;
	__u8 function;
	char comm[16];
	char data[TLS_FRAG_SIZE];
};

struct retprobe_ctx {
	__u64 buf;
	__u64 len_ptr;
	__u32 len;
	__u8 lib_type;
	__u8 direction;
	__u8 function;
};

const struct tls_fragment *tls_fragment_type_anchor __attribute__((unused));

// Perf event array for submitting TLS fragment events to userspace.
// Replaces BPF_MAP_TYPE_RINGBUF which has kernel 7.1 epoll issues.
struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
	__uint(key_size, sizeof(__u32));
	__uint(value_size, sizeof(__u32));
	__uint(max_entries, 128);
} tls_events SEC(".maps");

// Per-CPU scratch buffer for building fragments before perf_event_output.
// Only one entry needed (per CPU); key=0 always.
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct tls_fragment);
} tls_scratch SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 8192);
	__type(key, __u64);
	__type(value, struct retprobe_ctx);
} retprobe_buf SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 16);
	__type(key, __u32);
	__type(value, __u64);
} tls_probe_hits SEC(".maps");

static __always_inline void inc_probe_hit(__u8 function) {
	__u32 idx = (__u32)function;
	__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &idx);
	if (cnt) {
		__sync_fetch_and_add(cnt, 1);
	}
}

// emit_tls_fragment builds a tls_fragment in per-CPU scratch buffer and
// sends it via bpf_perf_event_output.  The ctx pointer MUST come from the
// outermost uprobe/uretprobe handler (struct pt_regs *).
static __always_inline int emit_tls_fragment(void *ctx, const void *buf, __u32 original_len, __u8 lib, __u8 dir, __u8 function)
{
	__u32 diag_output_fail = 12;
	__u32 diag_read_fail  = 13;
	__u32 diag_submit_ok  = 14;
	__u32 zero = 0;

	inc_probe_hit(function);

	if (!buf || original_len == 0) {
		return 0;
	}

	__u32 total_len = original_len;
	__u8 flags = 0;
	if (total_len > TLS_MAX_CAPTURE_SIZE) {
		total_len = TLS_MAX_CAPTURE_SIZE;
		flags |= TLS_FLAG_TRUNCATED;
	}

	__u64 pid_tgid = bpf_get_current_pid_tgid();
	__u64 now_ns = bpf_ktime_get_ns();
	__u32 frag_count32 = (total_len + TLS_FRAG_SIZE - 1) / TLS_FRAG_SIZE;
	if (frag_count32 == 0 || frag_count32 > TLS_MAX_FRAGS) {
		return 0;
	}

	struct tls_fragment *scratch = bpf_map_lookup_elem(&tls_scratch, &zero);
	if (!scratch) {
		return 0;
	}

	for (__u32 i = 0; i < frag_count32; i++) {

		__u32 offset = i * TLS_FRAG_SIZE;
		__u32 chunk = total_len - offset;
		if (chunk > TLS_FRAG_SIZE) {
			chunk = TLS_FRAG_SIZE;
		}

		scratch->timestamp_ns = now_ns;
		scratch->pid = (__u32)pid_tgid;
		scratch->tgid = (__u32)(pid_tgid >> 32);
		scratch->data_len = chunk;
		scratch->total_len = total_len;
		scratch->original_len = original_len;
		scratch->frag_index = (__u16)i;
		scratch->frag_count = (__u16)frag_count32;
		scratch->lib_type = lib;
		scratch->direction = dir;
		scratch->flags = flags;
		scratch->function = function;
		bpf_get_current_comm(&scratch->comm, sizeof(scratch->comm));

		if (bpf_probe_read_user(scratch->data, chunk, (const char *)buf + offset) < 0) {
			__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &diag_read_fail);
			if (cnt) __sync_fetch_and_add(cnt, 1);
			break;
		}

		long ret = bpf_perf_event_output(ctx, &tls_events, BPF_F_CURRENT_CPU, scratch, sizeof(*scratch));
		if (ret < 0) {
			__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &diag_output_fail);
			if (cnt) __sync_fetch_and_add(cnt, 1);
		} else {
			__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &diag_submit_ok);
			if (cnt) __sync_fetch_and_add(cnt, 1);
		}
	}

	return 0;
}

static __always_inline int save_retprobe_ctx(void *buf, const void *len_ptr, __u32 len, __u8 lib, __u8 dir, __u8 function)
{
	inc_probe_hit(function);
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx rc = {
		.buf = (__u64)buf,
		.len_ptr = (__u64)len_ptr,
		.len = len,
		.lib_type = lib,
		.direction = dir,
		.function = function,
	};
	return bpf_map_update_elem(&retprobe_buf, &pid_tgid, &rc, BPF_ANY);
}

static __always_inline int load_retprobe_ctx(struct retprobe_ctx *out)
{
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &pid_tgid);
	if (!rc) {
		return 0;
	}
	*out = *rc;
	bpf_map_delete_elem(&retprobe_buf, &pid_tgid);
	return 1;
}

static __always_inline int emit_retprobe_payload(void *ctx, __u32 len)
{
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc)) {
		return 0;
	}
	return emit_tls_fragment(ctx, (const void *)rc.buf, len, rc.lib_type, rc.direction, rc.function);
}

SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(ctx, buf, len, TLS_LIB_OPENSSL, TLS_DIR_SEND, TLS_FUNC_SSL_WRITE);
}

SEC("uprobe/SSL_write_ex")
int uprobe_ssl_write_ex(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	const void *written = (const void *)PT_REGS_PARM4(ctx);
	return save_retprobe_ctx((void *)buf, written, len, TLS_LIB_OPENSSL, TLS_DIR_SEND, TLS_FUNC_SSL_WRITE_EX);
}

SEC("uretprobe/SSL_write_ex")
int uretprobe_ssl_write_ex(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc) || ret != 1) {
		return 0;
	}
	__u64 written = 0;
	if (rc.len_ptr && bpf_probe_read_user(&written, sizeof(written), (const void *)rc.len_ptr) == 0 && written > 0) {
		return emit_tls_fragment(ctx, (const void *)rc.buf, (__u32)written, rc.lib_type, rc.direction, rc.function);
	}
	return emit_tls_fragment(ctx, (const void *)rc.buf, rc.len, rc.lib_type, rc.direction, rc.function);
}

SEC("uprobe/SSL_write_ex2")
int uprobe_ssl_write_ex2(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	const void *written = (const void *)PT_REGS_PARM4(ctx);
	return save_retprobe_ctx((void *)buf, written, len, TLS_LIB_OPENSSL, TLS_DIR_SEND, TLS_FUNC_SSL_WRITE_EX2);
#else
	return 0;
#endif
}

SEC("uretprobe/SSL_write_ex2")
int uretprobe_ssl_write_ex2(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc) || ret != 1) {
		return 0;
	}
	__u64 written = 0;
	if (rc.len_ptr && bpf_probe_read_user(&written, sizeof(written), (const void *)rc.len_ptr) == 0 && written > 0) {
		return emit_tls_fragment(ctx, (const void *)rc.buf, (__u32)written, rc.lib_type, rc.direction, rc.function);
	}
	return emit_tls_fragment(ctx, (const void *)rc.buf, rc.len, rc.lib_type, rc.direction, rc.function);
#else
	return 0;
#endif
}

SEC("uprobe/SSL_read")
int uprobe_ssl_read(struct pt_regs *ctx)
{
	return save_retprobe_ctx((void *)PT_REGS_PARM2(ctx), 0, (__u32)PT_REGS_PARM3(ctx), TLS_LIB_OPENSSL, TLS_DIR_RECV, TLS_FUNC_SSL_READ);
}

SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_retprobe_payload(ctx, (__u32)ret);
}

SEC("uprobe/SSL_read_ex")
int uprobe_ssl_read_ex(struct pt_regs *ctx)
{
	return save_retprobe_ctx((void *)PT_REGS_PARM2(ctx), (const void *)PT_REGS_PARM4(ctx), (__u32)PT_REGS_PARM3(ctx), TLS_LIB_OPENSSL, TLS_DIR_RECV, TLS_FUNC_SSL_READ_EX);
}

SEC("uretprobe/SSL_read_ex")
int uretprobe_ssl_read_ex(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc) || ret != 1 || !rc.len_ptr) {
		return 0;
	}
	__u64 read_len = 0;
	if (bpf_probe_read_user(&read_len, sizeof(read_len), (const void *)rc.len_ptr) < 0 || read_len == 0) {
		return 0;
	}
	return emit_tls_fragment(ctx, (const void *)rc.buf, (__u32)read_len, rc.lib_type, rc.direction, rc.function);
}

SEC("uprobe/gnutls_record_send")
int uprobe_gnutls_record_send(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(ctx, buf, len, TLS_LIB_GNUTLS, TLS_DIR_SEND, TLS_FUNC_GNUTLS_RECORD_SEND);
}

SEC("uprobe/gnutls_record_recv")
int uprobe_gnutls_record_recv(struct pt_regs *ctx)
{
	return save_retprobe_ctx((void *)PT_REGS_PARM2(ctx), 0, (__u32)PT_REGS_PARM3(ctx), TLS_LIB_GNUTLS, TLS_DIR_RECV, TLS_FUNC_GNUTLS_RECORD_RECV);
}

SEC("uretprobe/gnutls_record_recv")
int uretprobe_gnutls_record_recv(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_retprobe_payload(ctx, (__u32)ret);
}

SEC("uprobe/PR_Write")
int uprobe_pr_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(ctx, buf, len, TLS_LIB_NSS, TLS_DIR_SEND, TLS_FUNC_PR_WRITE);
}

SEC("uprobe/PR_Read")
int uprobe_pr_read(struct pt_regs *ctx)
{
	return save_retprobe_ctx((void *)PT_REGS_PARM2(ctx), 0, (__u32)PT_REGS_PARM3(ctx), TLS_LIB_NSS, TLS_DIR_RECV, TLS_FUNC_PR_READ);
}

SEC("uretprobe/PR_Read")
int uretprobe_pr_read(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_retprobe_payload(ctx, (__u32)ret);
}

SEC("uprobe/crypto_tls_Conn_Write")
int uprobe_crypto_tls_conn_write(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	// Go internal register ABI (Go 1.17+):
	//   PARM1=rax(rcvr), PARM2=rbx(slice.ptr), PARM3=rcx(slice.len)
	// NOT the SysV ABI (rdi/rsi/rdx) used by PT_REGS_PARM* macros.
	const void *buf = (const void *)ctx->bx;
	__u32 len = (__u32)ctx->cx;
#else
	const void *buf = 0;
	__u32 len = 0;
#endif
	return emit_tls_fragment(ctx, buf, len, TLS_LIB_GO, TLS_DIR_SEND, TLS_FUNC_GO_CONN_WRITE);
}

SEC("uprobe/crypto_tls_Conn_Read")
int uprobe_crypto_tls_conn_read(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	// Go internal register ABI: PARM2=rbx(buf.ptr), PARM3=rcx(buf.len)
	return save_retprobe_ctx((void *)ctx->bx, 0, (__u32)ctx->cx, TLS_LIB_GO, TLS_DIR_RECV, TLS_FUNC_GO_CONN_READ);
#else
	return 0;
#endif
}

SEC("uretprobe/crypto_tls_Conn_Read")
int uretprobe_crypto_tls_conn_read(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_retprobe_payload(ctx, (__u32)ret);
}

// ---- rustls (static-pie Rust binaries: codex, cursor) ----
//
// rustls encrypts TLS records internally. The plaintext-touching function is
// `RecordLayer::encrypt_outgoing(&mut self, plain: OutboundPlainMessage)`
// (rustls/src/record_layer.rs). It returns `OutboundOpaqueMessage` by value,
// so the System V AMD64 ABI uses sret (return pointer in PARM1):
//
//   rdi = &OutboundOpaqueMessage  (sret return slot, caller-allocated)
//   rsi = &mut self               (RecordLayer)
//   rdx = &OutboundPlainMessage   (borrowed, on the caller's stack)
//
// OutboundPlainMessage {
//   typ: ContentType (u8),
//   version: ProtocolVersion (u16),
//   payload: OutboundChunks,
// }
//
// OutboundChunks is an enum:
//   Single(&[u8])   -> discriminant 0, then slice { ptr, len }
//   Multiple { .. } -> discriminant 1
//
// From the codex 0.142 disassembly, the compiler lays out the borrowed
// OutboundPlainMessage (Single variant, application-data path) as:
//
//   +0x00: u64  discriminant (0 = Single)
//   +0x08: *const u8  plaintext slice pointer
//   +0x10: usize      plaintext slice length
//   +0x20: u16  version
//   +0x22: u8   typ
//
// Verified from the caller (`ConnectionCommon` encrypt path at 0x7bef680):
//   mov %r12, 0x30(%rsp)   ; +0x08 = ptr (within the rsp+0x28 OPM base)
//   mov %rcx, 0x38(%rsp)   ; +0x10 = len
//   mov %ax,  0x48(%rsp)   ; +0x20 = version
//   mov %eax, 0x4a(%rsp)   ; +0x22 = typ
//   lea 0x28(%rsp), %rdx   ; rdx = &OPM
//   call encrypt_outgoing
//
// This uprobe fires once per TLS record being encrypted — i.e. per outbound
// plaintext fragment. We dereference the borrowed OutboundPlainMessage (in
// PARM3 = rdx) via bpf_probe_read_user and emit the plaintext slice.
SEC("uprobe/rustls_encrypt_outgoing")
int uprobe_rustls_encrypt_outgoing(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	// rdx (PARM3) = &OutboundPlainMessage. PT_REGS_PARM3 is correct for the
	// SysV ABI; encrypt_outgoing's sret return does not shift the parameter
	// registers (the return pointer occupies PARM1, self=PARM2, plain=PARM3).
	const void *opm = (const void *)PT_REGS_PARM3(ctx);

	// OutboundPlainMessage (Single-variant layout, as built by the caller):
	//   +0x00: u64 discriminant (0 = Single)
	//   +0x08: *const u8  plaintext slice pointer
	//   +0x10: usize      plaintext slice length
	__u64 ptr = 0, len = 0;
	if (bpf_probe_read_user(&ptr, sizeof(ptr), (const void *)((const char *)opm + 0x08)) < 0) {
		return 0;
	}
	if (bpf_probe_read_user(&len, sizeof(len), (const void *)((const char *)opm + 0x10)) < 0) {
		return 0;
	}
	return emit_tls_fragment(ctx, (const void *)ptr, (__u32)len,
		TLS_LIB_RUSTLS, TLS_DIR_SEND, TLS_FUNC_RUSTLS_ENCRYPT_OUTGOING);
#else
	return 0;
#endif
}
