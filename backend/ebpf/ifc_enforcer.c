//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_endian.h>

#define EACCES 13
#define AF_UNIX 1
#define AF_INET 2
#define AF_INET6 10
#define AF_NETLINK 16

#define O_WRONLY 00000001
#define O_RDWR 00000002
#define O_TRUNC 00001000
#define O_APPEND 00002000

#define MAY_WRITE 2
#define MAY_READ 4

// Confidentiality taint lattice bits.
#define CONF_SECRET (1U << 0)
#define CONF_PRIVATE (1U << 1)

// Integrity taint lattice bits.
#define INTEG_UNTRUSTED (1U << 0)
#define INTEG_EXTERNAL_NET (1U << 1)
#define INTEG_TOOL_OUTPUT (1U << 2)

// Monotonic capability lease mask bits.
#define CAP_NET_EGRESS (1U << 0)
#define CAP_UNTRUSTED_EGRESS (1U << 1)
#define CAP_FILE_WRITE (1U << 2)
#define CAP_FILE_UNLINK (1U << 3)
#define CAP_EXEC_UNTRUSTED (1U << 4)
#define CAP_SECRET_READ (1U << 5)

// Enforcement flags.
#define IFC_FLAG_ENFORCE (1U << 0)
#define IFC_FLAG_AUDIT_ONLY (1U << 1)
#define IFC_FLAG_TRACKED (1U << 2)

struct ifc_task_label {
	__u32 conf_taint;
	__u32 integ_taint;
	__u32 cap_mask;
	__u32 spawn_budget;
	__u64 run_id_hash;
	__u32 flags;
	__u32 generation;
};

struct ifc_inode_label {
	__u32 conf_taint;
	__u32 integ_taint;
	__u64 writer_run_hash;
};

struct ifc_inode_key {
	__u32 s_dev;
	__u32 pad;
	__u64 i_ino;
};

struct ifc_stats {
	__u64 task_inherits;
	__u64 inode_taint_writes;
	__u64 task_taint_reads;
	__u64 trifecta_egress_blocks;
	__u64 untrusted_egress_blocks;
	__u64 untrusted_unlink_blocks;
	__u64 untrusted_exec_blocks;
	__u64 spawn_budget_blocks;
};

const struct ifc_task_label *ifc_task_label_type_anchor __attribute__((unused));
const struct ifc_inode_label *ifc_inode_label_type_anchor __attribute__((unused));
const struct ifc_inode_key *ifc_inode_key_type_anchor __attribute__((unused));
const struct ifc_stats *ifc_stats_type_anchor __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_TASK_STORAGE);
	__uint(map_flags, BPF_F_NO_PREALLOC);
	__type(key, int);
	__type(value, struct ifc_task_label);
} ifc_task_labels SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_INODE_STORAGE);
	__uint(map_flags, BPF_F_NO_PREALLOC);
	__type(key, int);
	__type(value, struct ifc_inode_label);
} ifc_inode_labels SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 4096);
	__type(key, __u32);
	__type(value, struct ifc_task_label);
} ifc_pid_seed SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1024);
	__type(key, struct ifc_inode_key);
	__type(value, struct ifc_inode_label);
} ifc_classified_inodes SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct ifc_stats);
} ifc_stats_map SEC(".maps");

static __always_inline struct ifc_stats *get_ifc_stats(void)
{
	__u32 key = 0;
	return bpf_map_lookup_elem(&ifc_stats_map, &key);
}

static __always_inline struct ifc_task_label *get_or_seed_current_task_label(void)
{
	struct task_struct *cur = bpf_get_current_task_btf();
	if (!cur) {
		return NULL;
	}
	struct ifc_task_label *lbl = bpf_task_storage_get(&ifc_task_labels, cur, 0, 0);
	if (lbl) {
		return lbl;
	}
	__u32 tgid = (__u32)(bpf_get_current_pid_tgid() >> 32);
	struct ifc_task_label *seed = bpf_map_lookup_elem(&ifc_pid_seed, &tgid);
	if (!seed) {
		return NULL;
	}
	return bpf_task_storage_get(&ifc_task_labels, cur, seed, BPF_LOCAL_STORAGE_GET_F_CREATE);
}

static __always_inline void read_combined_inode_taint(struct inode *inode,
						      __u32 *out_conf,
						      __u32 *out_integ)
{
	*out_conf = 0;
	*out_integ = 0;
	if (!inode) {
		return;
	}

	struct ifc_inode_label *dyn = bpf_inode_storage_get(&ifc_inode_labels, inode, 0, 0);
	if (dyn) {
		*out_conf |= dyn->conf_taint;
		*out_integ |= dyn->integ_taint;
	}

	struct super_block *sb = BPF_CORE_READ(inode, i_sb);
	if (!sb) {
		return;
	}
	struct ifc_inode_key key = {};
	key.s_dev = BPF_CORE_READ(sb, s_dev);
	key.i_ino = BPF_CORE_READ(inode, i_ino);
	if (key.i_ino == 0) {
		return;
	}
	struct ifc_inode_label *cls = bpf_map_lookup_elem(&ifc_classified_inodes, &key);
	if (cls) {
		*out_conf |= cls->conf_taint;
		*out_integ |= cls->integ_taint;
	}
}

static __always_inline int propagate_file_ifc(struct ifc_task_label *lbl,
					      struct inode *inode,
					      bool is_read,
					      bool is_write,
					      struct ifc_stats *stats)
{
	if (!lbl || !inode) {
		return 0;
	}

	__u32 in_conf = 0;
	__u32 in_integ = 0;
	read_combined_inode_taint(inode, &in_conf, &in_integ);

	if (is_write) {
		if ((lbl->flags & IFC_FLAG_ENFORCE) &&
		    (lbl->integ_taint & INTEG_UNTRUSTED) &&
		    !(lbl->cap_mask & CAP_FILE_WRITE)) {
			return -EACCES;
		}
		if (lbl->conf_taint || lbl->integ_taint) {
			struct ifc_inode_label *dlbl = bpf_inode_storage_get(
				&ifc_inode_labels, inode, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
			if (dlbl) {
				dlbl->conf_taint |= lbl->conf_taint;
				dlbl->integ_taint |= lbl->integ_taint;
				dlbl->writer_run_hash = lbl->run_id_hash;
				if (stats) {
					stats->inode_taint_writes++;
				}
			}
		}
	}

	if (is_read) {
		if ((in_conf & CONF_SECRET) &&
		    (lbl->flags & IFC_FLAG_ENFORCE) &&
		    !(lbl->cap_mask & CAP_SECRET_READ)) {
			return -EACCES;
		}
		if ((in_conf & ~lbl->conf_taint) || (in_integ & ~lbl->integ_taint)) {
			lbl->conf_taint |= in_conf;
			lbl->integ_taint |= in_integ;
			if (stats) {
				stats->task_taint_reads++;
			}
		}
	}
	return 0;
}

static __always_inline bool is_loopback_ipv4(__be32 addr)
{
	return ((bpf_ntohl(addr) >> 24) & 0xff) == 127;
}

static __always_inline bool is_loopback_ip6_words(const __be32 w[4])
{
	// ::1
	if (w[0] == 0 && w[1] == 0 && w[2] == 0 && bpf_ntohl(w[3]) == 1) {
		return true;
	}
	// ::ffff:127.0.0.0/104
	if (w[0] == 0 && w[1] == 0 && bpf_ntohl(w[2]) == 0x0000ffffU && is_loopback_ipv4(w[3])) {
		return true;
	}
	return false;
}

static __always_inline int check_egress_policy(struct ifc_task_label *lbl, struct ifc_stats *stats)
{
	if (!lbl || !(lbl->flags & IFC_FLAG_ENFORCE)) {
		return 0;
	}
	if ((lbl->conf_taint & CONF_SECRET) && (lbl->integ_taint & INTEG_UNTRUSTED)) {
		if (stats) {
			stats->trifecta_egress_blocks++;
		}
		return -EACCES;
	}
	if (!(lbl->cap_mask & CAP_NET_EGRESS) ||
	    ((lbl->integ_taint & INTEG_UNTRUSTED) && !(lbl->cap_mask & CAP_UNTRUSTED_EGRESS))) {
		if (stats) {
			stats->untrusted_egress_blocks++;
		}
		return -EACCES;
	}
	return 0;
}

SEC("lsm/task_alloc")
int BPF_PROG(ifc_enforce_task_alloc, struct task_struct *task, unsigned long clone_flags, int ret)
{
	if (ret != 0 || !task) {
		return ret;
	}
	struct ifc_task_label *parent = get_or_seed_current_task_label();
	if (!parent) {
		return 0;
	}
	struct ifc_stats *stats = get_ifc_stats();
	if ((parent->flags & IFC_FLAG_ENFORCE) && parent->spawn_budget == 0) {
		if (stats) {
			stats->spawn_budget_blocks++;
		}
		return -EACCES;
	}
	__u32 child_budget = 0;
	if (parent->spawn_budget > 0) {
		parent->spawn_budget--;
		child_budget = parent->spawn_budget;
	}
	struct ifc_task_label init = *parent;
	init.spawn_budget = child_budget;
	struct ifc_task_label *child = bpf_task_storage_get(
		&ifc_task_labels, task, &init, BPF_LOCAL_STORAGE_GET_F_CREATE);
	if (child) {
		child->conf_taint = parent->conf_taint;
		child->integ_taint = parent->integ_taint;
		child->cap_mask = parent->cap_mask;
		child->spawn_budget = child_budget;
		child->run_id_hash = parent->run_id_hash;
		child->flags = parent->flags;
		child->generation = parent->generation;
		if (stats) {
			stats->task_inherits++;
		}
	}
	return 0;
}

SEC("lsm/file_open")
int BPF_PROG(ifc_enforce_file_open, struct file *file, int ret)
{
	if (ret != 0 || !file) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	struct inode *inode = file->f_inode;
	if (!inode) {
		return 0;
	}
	__u32 fflags = BPF_CORE_READ(file, f_flags);
	bool is_write = (fflags & (O_WRONLY | O_RDWR | O_TRUNC | O_APPEND)) != 0;
	bool is_read = (fflags & O_WRONLY) == 0;
	return propagate_file_ifc(lbl, inode, is_read, is_write, get_ifc_stats());
}

SEC("lsm/file_permission")
int BPF_PROG(ifc_enforce_file_permission, struct file *file, int mask, int ret)
{
	if (ret != 0 || !file) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	struct inode *inode = file->f_inode;
	if (!inode) {
		return 0;
	}
	bool is_read = (mask & MAY_READ) != 0;
	bool is_write = (mask & MAY_WRITE) != 0;
	if (!is_read && !is_write) {
		return 0;
	}
	return propagate_file_ifc(lbl, inode, is_read, is_write, get_ifc_stats());
}

SEC("lsm/bprm_check_security")
int BPF_PROG(ifc_enforce_bprm_check, struct linux_binprm *bprm, int ret)
{
	if (ret != 0 || !bprm) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	struct file *file = bprm->file;
	if (!file) {
		return 0;
	}
	struct inode *inode = file->f_inode;
	if (!inode) {
		return 0;
	}
	__u32 in_conf = 0;
	__u32 in_integ = 0;
	read_combined_inode_taint(inode, &in_conf, &in_integ);

	struct ifc_stats *stats = get_ifc_stats();
	if ((in_conf & ~lbl->conf_taint) || (in_integ & ~lbl->integ_taint)) {
		lbl->conf_taint |= in_conf;
		lbl->integ_taint |= in_integ;
		if (stats) {
			stats->task_taint_reads++;
		}
	}
	if ((lbl->flags & IFC_FLAG_ENFORCE) &&
	    ((in_integ & INTEG_UNTRUSTED) || (lbl->integ_taint & INTEG_UNTRUSTED)) &&
	    !(lbl->cap_mask & CAP_EXEC_UNTRUSTED)) {
		if (stats) {
			stats->untrusted_exec_blocks++;
		}
		return -EACCES;
	}
	return 0;
}

SEC("lsm/inode_unlink")
int BPF_PROG(ifc_enforce_inode_unlink, struct inode *dir, struct dentry *dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	if ((lbl->flags & IFC_FLAG_ENFORCE) &&
	    (lbl->integ_taint & INTEG_UNTRUSTED) &&
	    !(lbl->cap_mask & CAP_FILE_UNLINK)) {
		struct ifc_stats *stats = get_ifc_stats();
		if (stats) {
			stats->untrusted_unlink_blocks++;
		}
		return -EACCES;
	}
	return 0;
}

SEC("lsm/inode_rename")
int BPF_PROG(ifc_enforce_inode_rename, struct inode *old_dir, struct dentry *old_dentry,
	     struct inode *new_dir, struct dentry *new_dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	struct ifc_stats *stats = get_ifc_stats();
	if ((lbl->flags & IFC_FLAG_ENFORCE) &&
	    (lbl->integ_taint & INTEG_UNTRUSTED) &&
	    !(lbl->cap_mask & CAP_FILE_UNLINK)) {
		if (stats) {
			stats->untrusted_unlink_blocks++;
		}
		return -EACCES;
	}
	if (old_dentry) {
		struct inode *old_inode = old_dentry->d_inode;
		if (old_inode && (lbl->conf_taint || lbl->integ_taint)) {
			struct ifc_inode_label *olbl = bpf_inode_storage_get(
				&ifc_inode_labels, old_inode, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
			if (olbl) {
				olbl->conf_taint |= lbl->conf_taint;
				olbl->integ_taint |= lbl->integ_taint;
				olbl->writer_run_hash = lbl->run_id_hash;
			}
		}
	}
	if (new_dentry) {
		struct inode *new_inode = new_dentry->d_inode;
		if (new_inode && (lbl->conf_taint || lbl->integ_taint)) {
			struct ifc_inode_label *nlbl = bpf_inode_storage_get(
				&ifc_inode_labels, new_inode, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
			if (nlbl) {
				nlbl->conf_taint |= lbl->conf_taint;
				nlbl->integ_taint |= lbl->integ_taint;
				nlbl->writer_run_hash = lbl->run_id_hash;
			}
		}
	}
	return 0;
}

SEC("lsm/socket_connect")
int BPF_PROG(ifc_enforce_socket_connect, struct socket *sock, struct sockaddr *address,
	     int addrlen, int ret)
{
	if (ret != 0 || !address || addrlen < (int)sizeof(__u16)) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	__u16 fam = 0;
	if (bpf_probe_read_kernel(&fam, sizeof(fam), address) != 0) {
		return 0;
	}
	if (fam == AF_UNIX || fam == AF_NETLINK) {
		return 0;
	}
	if (fam == AF_INET && addrlen >= (int)sizeof(struct sockaddr_in)) {
		struct sockaddr_in sin = {};
		if (bpf_probe_read_kernel(&sin, sizeof(sin), address) == 0 &&
		    is_loopback_ipv4(sin.sin_addr.s_addr)) {
			return 0;
		}
	} else if (fam == AF_INET6 && addrlen >= (int)sizeof(struct sockaddr_in6)) {
		struct sockaddr_in6 sin6 = {};
		if (bpf_probe_read_kernel(&sin6, sizeof(sin6), address) == 0 &&
		    is_loopback_ip6_words(sin6.sin6_addr.in6_u.u6_addr32)) {
			return 0;
		}
	} else if (fam != AF_INET && fam != AF_INET6) {
		return 0;
	}
	return check_egress_policy(lbl, get_ifc_stats());
}

SEC("lsm/socket_sendmsg")
int BPF_PROG(ifc_enforce_socket_sendmsg, struct socket *sock, struct msghdr *msg,
	     int size, int ret)
{
	if (ret != 0 || !sock) {
		return ret;
	}
	struct ifc_task_label *lbl = get_or_seed_current_task_label();
	if (!lbl) {
		return 0;
	}
	struct sock *sk = sock->sk;
	if (!sk) {
		return 0;
	}
	__u16 fam = BPF_CORE_READ(sk, __sk_common.skc_family);
	if (fam != AF_INET && fam != AF_INET6) {
		return 0;
	}
	if (msg) {
		void *mname = BPF_CORE_READ(msg, msg_name);
		int mlen = BPF_CORE_READ(msg, msg_namelen);
		if (mname && fam == AF_INET && mlen >= (int)sizeof(struct sockaddr_in)) {
			struct sockaddr_in sin = {};
			if (bpf_probe_read_kernel(&sin, sizeof(sin), mname) == 0 &&
			    sin.sin_family == AF_INET && is_loopback_ipv4(sin.sin_addr.s_addr)) {
				return 0;
			}
		} else if (mname && fam == AF_INET6 && mlen >= (int)sizeof(struct sockaddr_in6)) {
			struct sockaddr_in6 sin6 = {};
			if (bpf_probe_read_kernel(&sin6, sizeof(sin6), mname) == 0 &&
			    sin6.sin6_family == AF_INET6 &&
			    is_loopback_ip6_words(sin6.sin6_addr.in6_u.u6_addr32)) {
				return 0;
			}
		}
	}
	if (fam == AF_INET) {
		__be32 daddr = BPF_CORE_READ(sk, __sk_common.skc_daddr);
		if (daddr != 0 && is_loopback_ipv4(daddr)) {
			return 0;
		}
	} else {
		__be32 w[4] = {};
		if (BPF_CORE_READ_INTO(&w, sk, __sk_common.skc_v6_daddr.in6_u.u6_addr32) == 0 &&
		    is_loopback_ip6_words(w)) {
			return 0;
		}
	}
	return check_egress_policy(lbl, get_ifc_stats());
}

char LICENSE[] SEC("license") = "GPL";
