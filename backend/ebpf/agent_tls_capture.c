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

struct tls_fragment {
	__u64 timestamp_ns;
	__u32 pid;
	__u32 tgid;
	__u32 data_len;
	__u32 total_len;
	__u16 frag_index;
	__u16 frag_count;
	__u8 lib_type;
	__u8 direction;
	char comm[16];
	char data[TLS_FRAG_SIZE];
};

struct retprobe_ctx {
	__u64 buf;
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

static __always_inline int emit_tls_fragment(const void *buf, __u32 total_len, __u8 lib, __u8 dir)
{
	if (!buf || total_len == 0 || total_len > TLS_MAX_CAPTURE_SIZE) {
		return 0;
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
		f->frag_index = (__u16)i;
		f->frag_count = (__u16)frag_count32;
		f->lib_type = lib;
		f->direction = dir;
		bpf_get_current_comm(&f->comm, sizeof(f->comm));

		if (bpf_probe_read_user(f->data, chunk, (const char *)buf + offset) < 0) {
			bpf_ringbuf_discard(f, 0);
			break;
		}
		bpf_ringbuf_submit(f, 0);
	}

	return 0;
}

static __always_inline int save_retprobe_buf(void *buf)
{
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx rc = {
		.buf = (__u64)buf,
	};
	return bpf_map_update_elem(&retprobe_buf, &pid_tgid, &rc, BPF_ANY);
}

static __always_inline void *load_retprobe_buf(void)
{
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &pid_tgid);
	if (!rc) {
		return 0;
	}
	void *buf = (void *)rc->buf;
	bpf_map_delete_elem(&retprobe_buf, &pid_tgid);
	return buf;
}

SEC("uprobe/SSL_write")
int uprobe_ssl_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_OPENSSL, TLS_DIR_SEND);
}

SEC("uprobe/SSL_read")
int uprobe_ssl_read(struct pt_regs *ctx)
{
	return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/SSL_read")
int uretprobe_ssl_read(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_OPENSSL, TLS_DIR_RECV);
}

SEC("uprobe/gnutls_record_send")
int uprobe_gnutls_record_send(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_GNUTLS, TLS_DIR_SEND);
}

SEC("uprobe/gnutls_record_recv")
int uprobe_gnutls_record_recv(struct pt_regs *ctx)
{
	return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/gnutls_record_recv")
int uretprobe_gnutls_record_recv(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_GNUTLS, TLS_DIR_RECV);
}

SEC("uprobe/PR_Write")
int uprobe_pr_write(struct pt_regs *ctx)
{
	const void *buf = (const void *)PT_REGS_PARM2(ctx);
	__u32 len = (__u32)PT_REGS_PARM3(ctx);
	return emit_tls_fragment(buf, len, TLS_LIB_NSS, TLS_DIR_SEND);
}

SEC("uprobe/PR_Read")
int uprobe_pr_read(struct pt_regs *ctx)
{
	return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
}

SEC("uretprobe/PR_Read")
int uretprobe_pr_read(struct pt_regs *ctx)
{
	__s32 ret = (__s32)PT_REGS_RC(ctx);
	if (ret <= 0) {
		return 0;
	}
	return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_NSS, TLS_DIR_RECV);
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
	return emit_tls_fragment(buf, len, TLS_LIB_GO, TLS_DIR_SEND);
}

SEC("uprobe/crypto_tls_Conn_Read")
int uprobe_crypto_tls_conn_read(struct pt_regs *ctx)
{
#if defined(__TARGET_ARCH_x86)
	return save_retprobe_buf((void *)PT_REGS_PARM2(ctx));
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
	return emit_tls_fragment(load_retprobe_buf(), (__u32)ret, TLS_LIB_GO, TLS_DIR_RECV);
}
