//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define LSM_PATH_LEN 256
#define LSM_NAME_LEN 64
#define EACCES 13

struct lsm_path_key {
	char path[LSM_PATH_LEN];
};

struct lsm_name_key {
	char name[LSM_NAME_LEN];
};

struct lsm_enforcer_stats {
	__u64 exec_checked;
	__u64 exec_blocked;
	__u64 file_checked;
	__u64 file_blocked;
};

const struct lsm_enforcer_stats *lsm_enforcer_stats_type_anchor __attribute__((unused));

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 512);
	__type(key, struct lsm_path_key);
	__type(value, __u32);
} lsm_blocked_exec_paths SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 512);
	__type(key, struct lsm_name_key);
	__type(value, __u32);
} lsm_blocked_exec_names SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 512);
	__type(key, struct lsm_name_key);
	__type(value, __u32);
} lsm_blocked_file_names SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, struct lsm_enforcer_stats);
} lsm_enforcer_stats_map SEC(".maps");

static __always_inline struct lsm_enforcer_stats *get_lsm_stats(void)
{
	__u32 key = 0;
	return bpf_map_lookup_elem(&lsm_enforcer_stats_map, &key);
}

static __always_inline int lsm_file_name_is_blocked(const unsigned char *name)
{
	if (!name) {
		return 0;
	}

	struct lsm_name_key key = {};
	if (bpf_probe_read_kernel_str(key.name, sizeof(key.name), name) <= 0) {
		return 0;
	}

	__u32 *blocked = bpf_map_lookup_elem(&lsm_blocked_file_names, &key);
	return blocked && *blocked;
}

SEC("lsm/bprm_check_security")
int BPF_PROG(lsm_enforce_bprm_check, struct linux_binprm *bprm, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->exec_checked++;
	}

	const char *filename = BPF_CORE_READ(bprm, filename);
	if (!filename) {
		return 0;
	}

	struct lsm_path_key key = {};
	if (bpf_probe_read_kernel_str(key.path, sizeof(key.path), filename) <= 0) {
		return 0;
	}

	__u32 *blocked = bpf_map_lookup_elem(&lsm_blocked_exec_paths, &key);
	if (blocked && *blocked) {
		if (stats) {
			stats->exec_blocked++;
		}
		return -EACCES;
	}

	const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
	if (!exec_name) {
		return 0;
	}

	struct lsm_name_key name_key = {};
	if (bpf_probe_read_kernel_str(name_key.name, sizeof(name_key.name), exec_name) <= 0) {
		return 0;
	}

	__u32 *name_blocked = bpf_map_lookup_elem(&lsm_blocked_exec_names, &name_key);
	if (name_blocked && *name_blocked) {
		if (stats) {
			stats->exec_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/file_open")
int BPF_PROG(lsm_enforce_file_open, struct file *file, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/file_permission")
int BPF_PROG(lsm_enforce_file_permission, struct file *file, int mask, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/mmap_file")
int BPF_PROG(lsm_enforce_mmap_file, struct file *file, unsigned long reqprot,
	     unsigned long prot, unsigned long flags, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/file_mprotect")
int BPF_PROG(lsm_enforce_file_mprotect, struct vm_area_struct *vma, unsigned long reqprot,
	     unsigned long prot, int ret)
{
	if (ret != 0) {
		return ret;
	}

	if (!vma) {
		return 0;
	}

	struct file *file = BPF_CORE_READ(vma, vm_file);
	if (!file) {
		return 0;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_setattr")
int BPF_PROG(lsm_enforce_inode_setattr, struct mnt_idmap *idmap, struct dentry *dentry,
	     struct iattr *attr, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_create")
int BPF_PROG(lsm_enforce_inode_create, struct inode *dir, struct dentry *dentry, umode_t mode, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_link")
int BPF_PROG(lsm_enforce_inode_link, struct dentry *old_dentry, struct inode *dir,
	     struct dentry *new_dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *old_name = BPF_CORE_READ(old_dentry, d_name.name);
	if (lsm_file_name_is_blocked(old_name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	const unsigned char *new_name = BPF_CORE_READ(new_dentry, d_name.name);
	if (lsm_file_name_is_blocked(new_name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_unlink")
int BPF_PROG(lsm_enforce_inode_unlink, struct inode *dir, struct dentry *dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_symlink")
int BPF_PROG(lsm_enforce_inode_symlink, struct inode *dir, struct dentry *dentry,
	     const char *old_name, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_mkdir")
int BPF_PROG(lsm_enforce_inode_mkdir, struct inode *dir, struct dentry *dentry, umode_t mode, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_rmdir")
int BPF_PROG(lsm_enforce_inode_rmdir, struct inode *dir, struct dentry *dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_mknod")
int BPF_PROG(lsm_enforce_inode_mknod, struct inode *dir, struct dentry *dentry, umode_t mode,
	     dev_t dev, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
	if (lsm_file_name_is_blocked(name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

SEC("lsm/inode_rename")
int BPF_PROG(lsm_enforce_inode_rename, struct inode *old_dir, struct dentry *old_dentry,
	     struct inode *new_dir, struct dentry *new_dentry, int ret)
{
	if (ret != 0) {
		return ret;
	}

	struct lsm_enforcer_stats *stats = get_lsm_stats();
	if (stats) {
		stats->file_checked++;
	}

	const unsigned char *old_name = BPF_CORE_READ(old_dentry, d_name.name);
	if (lsm_file_name_is_blocked(old_name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	const unsigned char *new_name = BPF_CORE_READ(new_dentry, d_name.name);
	if (lsm_file_name_is_blocked(new_name)) {
		if (stats) {
			stats->file_blocked++;
		}
		return -EACCES;
	}

	return 0;
}

char LICENSE[] SEC("license") = "GPL";
