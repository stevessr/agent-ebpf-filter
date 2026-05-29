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

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 256 * 1024);
} tls_events SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 8192);
	__type(key, __u64);
	__type(value, struct retprobe_ctx);
} retprobe_buf SEC(".maps");

static __always_inline int emit_tls_fragment(const void *buf, __u32 original_len, __u8 lib, __u8 dir, __u8 function)
{
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

	for (__u32 i = 0; i < frag_count32; i++) {

		struct tls_fragment *f = bpf_ringbuf_reserve(&tls_events, sizeof(*f), 0);
		if (!f) {
			break;
		}

		__u32 offset = i * TLS_FRAG_SIZE;
		__u32 chunk = total_len - offset;
		if (chunk > TLS_FRAG_SIZE) {
			chunk = TLS_FRAG_SIZE;
		}

		f->timestamp_ns = now_ns;
		f->pid = (__u32)pid_tgid;
		f->tgid = (__u32)(pid_tgid >> 32);
		f->data_len = chunk;
		f->total_len = total_len;
		f->original_len = original_len;
		f->frag_index = (__u16)i;
		f->frag_count = (__u16)frag_count32;
		f->lib_type = lib;
		f->direction = dir;
		f->flags = flags;
		f->function = function;
		bpf_get_current_comm(&f->comm, sizeof(f->comm));

		if (bpf_probe_read_user(f->data, chunk, (const char *)buf + offset) < 0) {
			bpf_ringbuf_discard(f, 0);
			break;
		}
		bpf_ringbuf_submit(f, 0);
	}

	return 0;
}

static __always_inline int save_retprobe_ctx(void *buf, const void *len_ptr, __u32 len, __u8 lib, __u8 dir, __u8 function)
{
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

static __always_inline int emit_retprobe_payload(__u32 len)
{
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc)) {
		return 0;
	}
	return emit_tls_fragment((const void *)rc.buf, len, rc.lib_type, rc.direction, rc.function);
}

SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_OPENSSL, TLS_DIR_SEND, TLS_FUNC_SSL_WRITE);
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
		return emit_tls_fragment((const void *)rc.buf, (__u32)written, rc.lib_type, rc.direction, rc.function);
	}
	return emit_tls_fragment((const void *)rc.buf, rc.len, rc.lib_type, rc.direction, rc.function);
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
	return emit_retprobe_payload((__u32)ret);
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
	return emit_tls_fragment((const void *)rc.buf, (__u32)read_len, rc.lib_type, rc.direction, rc.function);
}

SEC("uprobe/gnutls_record_send")
int uprobe_gnutls_record_send(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_GNUTLS, TLS_DIR_SEND, TLS_FUNC_GNUTLS_RECORD_SEND);
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
	return emit_retprobe_payload((__u32)ret);
}

SEC("uprobe/PR_Write")
int uprobe_pr_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_NSS, TLS_DIR_SEND, TLS_FUNC_PR_WRITE);
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
	return emit_retprobe_payload((__u32)ret);
}

SEC("uprobe/crypto_tls_Conn_Write")
int uprobe_crypto_tls_conn_write(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
#else
	const void *buf = 0;
	__u32 len = 0;
#endif
	return emit_tls_fragment(buf, len, TLS_LIB_GO, TLS_DIR_SEND, TLS_FUNC_GO_CONN_WRITE);
}

SEC("uprobe/crypto_tls_Conn_Read")
int uprobe_crypto_tls_conn_read(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	return save_retprobe_ctx((void *)PT_REGS_PARM2(ctx), 0, (__u32)PT_REGS_PARM3(ctx), TLS_LIB_GO, TLS_DIR_RECV, TLS_FUNC_GO_CONN_READ);
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
	return emit_retprobe_payload((__u32)ret);
}
