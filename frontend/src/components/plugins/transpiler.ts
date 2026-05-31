import type { VisualLogicNode, VisualWorkspaceSnapshot } from "./types";

export const ipToHex = (ip: string): string => {
  const parts = ip.split(".").map((p) => parseInt(p, 10));
  if (parts.length !== 4 || parts.some(isNaN)) return "0x00000000";
  return (
    "0x" +
    parts
      .map((p) => Math.min(255, Math.max(0, p)).toString(16).padStart(2, "0"))
      .join("")
  );
};

export const generateBpfCode = (
  snapshot: VisualWorkspaceSnapshot,
  programName = "visual_custom_plugin",
): string => {
  const { trigger, action, conditions, mapMode, mapKey, mapLimit } = snapshot;
  const safeProgramName =
    programName.replace(/[^a-zA-Z0-9_]/g, "_").replace(/^[^a-zA-Z_]+/, "") ||
    "visual_custom_plugin";

  const isKprobeUnlink = trigger === "unlink";
  const isKill = action === "KILL";
  const returnValLsm =
    action === "BLOCK" || action === "KILL" ? "-EACCES" : "0";
  const logPrefix = isKill
    ? "Killed"
    : action === "BLOCK"
      ? "Blocked"
      : "Alert";

  let headers = `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

// 声明内核中 task_struct 的部分结构体以通过 core 编译
struct task_struct;
struct linux_binprm;
struct file;
struct inode;
struct dentry;
struct socket;
struct sockaddr;
struct vm_area_struct;
struct pt_regs;

char LICENSE[] SEC("license") = "GPL";
#define EACCES 13

#ifndef bpf_ntohs
#define bpf_ntohs(x) __builtin_bswap16(x)
#endif
#ifndef bpf_ntohl
#define bpf_ntohl(x) __builtin_bswap32(x)
#endif

static __always_inline int strcmp_const(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s1[i] != s2[i]) return 1;
        if (s1[i] == '\\0') return 0;
    }
    return 0;
}

static __always_inline int str_starts_with(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s2[i] == '\\0') return 1;
        if (s1[i] != s2[i]) return 0;
    }
    return 0;
}

static __always_inline int get_str_len(const char *s, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s[i] == '\\0') return i;
    }
    return max_len;
}

static __always_inline int str_ends_with(const char *s1, int s1_len, const char *s2, int s2_len) {
    if (s1_len < s2_len) return 0;
    int offset = s1_len - s2_len;
    for (int i = 0; i < 64; i++) {
        if (i >= s2_len) break;
        if (s1[offset + i] != s2[i]) return 0;
    }
    return 1;
}
`;

  let body = "";

  const readTaskFields = `
    // 从 task_struct 读取 ppid 和 loginuid
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    u32 ppid = 0;
    u32 loginuid = 0;
    if (task) {
        ppid = BPF_CORE_READ(task, real_parent, tgid);
        loginuid = BPF_CORE_READ(task, loginuid.val);
    }
  `;

  // 1. Hook function header
  if (trigger === "process") {
    body = `
SEC("lsm/bprm_check_security")
int BPF_PROG(${safeProgramName}, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (exec_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), exec_name);
    }
`;
  } else if (trigger === "file_open") {
    body = `
SEC("lsm/file_open")
int BPF_PROG(${safeProgramName}, struct file *file, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (
    trigger === "mkdir" ||
    trigger === "file_create" ||
    trigger === "rmdir" ||
    trigger === "symlink"
  ) {
    const secName =
      trigger === "mkdir"
        ? "lsm/inode_mkdir"
        : trigger === "file_create"
          ? "lsm/inode_create"
          : trigger === "rmdir"
            ? "lsm/inode_rmdir"
            : "lsm/inode_symlink";

    const funcArgs =
      trigger === "mkdir"
        ? "struct inode *dir, struct dentry *dentry, umode_t mode"
        : trigger === "file_create"
          ? "struct inode *dir, struct dentry *dentry, umode_t mode"
          : trigger === "rmdir"
            ? "struct inode *dir, struct dentry *dentry"
            : "struct inode *dir, struct dentry *dentry, const char *old_name";

    body = `
SEC("${secName}")
int BPF_PROG(${safeProgramName}, ${funcArgs}) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger === "socket_connect") {
    body = `
SEC("lsm/socket_connect")
int BPF_PROG(${safeProgramName}, struct socket *sock, struct sockaddr *address, int addrlen) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    short family = 0;
    if (address) {
        bpf_probe_read_kernel(&family, sizeof(family), &address->sa_family);
    }

    u16 dst_port = 0;
    u32 dst_ipv4 = 0;
    if (family == 2) { // AF_INET
        struct sockaddr_in addr_in = {};
        bpf_probe_read_kernel(&addr_in, sizeof(addr_in), address);
        dst_port = bpf_ntohs(addr_in.sin_port);
        dst_ipv4 = bpf_ntohl(addr_in.sin_addr.s_addr);
    }
`;
  } else if (trigger === "inode_mknod") {
    body = `
SEC("lsm/inode_mknod")
int BPF_PROG(${safeProgramName}, struct inode *dir, struct dentry *dentry, umode_t mode, dev_t dev) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger === "file_mprotect") {
    body = `
SEC("lsm/file_mprotect")
int BPF_PROG(${safeProgramName}, struct vm_area_struct *vma, unsigned long reqprot, unsigned long prot, int ret) {
    if (ret != 0) return ret;
    if (!vma) return 0;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    struct file *file = BPF_CORE_READ(vma, vm_file);
    char name_buf[64] = {};
    if (file) {
        const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
        if (file_name) {
            bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
        }
    }
`;
  } else if (trigger === "inode_rename") {
    body = `
SEC("lsm/inode_rename")
int BPF_PROG(${safeProgramName}, struct inode *old_dir, struct dentry *old_dentry, struct inode *new_dir, struct dentry *new_dentry) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}

    const unsigned char *file_name = BPF_CORE_READ(old_dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (isKprobeUnlink) {
    body = `
SEC("kprobe/do_unlinkat")
int BPF_PROG(${safeProgramName}, struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    ${readTaskFields.trim()}
    char name_buf[64] = {}; // kprobe lacks dentry
`;
  }

  const lines: string[] = [];
  const generateNodeCExpression = (node: VisualLogicNode): string => {
    if (node.type === "CONDITION") {
      const val = (node.value || "").trim();
      if (!val) {
        return "1"; // safe default
      }
      let expr = "0";
      if (node.field === "comm") {
        if (node.operator === "==") {
          expr = `strcmp_const(comm, "${val}", sizeof(comm)) == 0`;
        } else if (node.operator === "!=") {
          expr = `strcmp_const(comm, "${val}", sizeof(comm)) != 0`;
        } else if (node.operator === "starts_with") {
          expr = `str_starts_with(comm, "${val}", sizeof(comm)) != 0`;
        } else if (node.operator === "ends_with") {
          expr = `str_ends_with(comm, get_str_len(comm, sizeof(comm)), "${val}", ${val.length}) != 0`;
        }
      } else if (node.field === "pid") {
        const pidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `pid == ${pidNum}`;
        else expr = `pid != ${pidNum}`;
      } else if (node.field === "uid") {
        const uidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `uid == ${uidNum}`;
        else expr = `uid != ${uidNum}`;
      } else if (node.field === "gid") {
        const gidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `gid == ${gidNum}`;
        else expr = `gid != ${gidNum}`;
      } else if (node.field === "ppid") {
        const ppidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `ppid == ${ppidNum}`;
        else expr = `ppid != ${ppidNum}`;
      } else if (node.field === "loginuid") {
        const loginuidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `loginuid == ${loginuidNum}`;
        else expr = `loginuid != ${loginuidNum}`;
      } else if (node.field === "port") {
        const portNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `dst_port == ${portNum}`;
        else expr = `dst_port != ${portNum}`;
      } else if (node.field === "ipv4") {
        const hexIp = ipToHex(val);
        if (node.operator === "==") expr = `dst_ipv4 == ${hexIp}`;
        else expr = `dst_ipv4 != ${hexIp}`;
      } else if (node.field === "basename") {
        if (isKprobeUnlink) {
          expr = "0";
        } else {
          if (node.operator === "==") {
            expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) == 0`;
          } else if (node.operator === "!=") {
            expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) != 0`;
          } else if (node.operator === "starts_with") {
            expr = `str_starts_with(name_buf, "${val}", sizeof(name_buf)) != 0`;
          } else if (node.operator === "ends_with") {
            expr = `str_ends_with(name_buf, get_str_len(name_buf, sizeof(name_buf)), "${val}", ${val.length}) != 0`;
          }
        }
      }
      const varId = node.id.replace(/[^a-zA-Z0-9]/g, "_");
      const varName = `cond_${varId}`;
      lines.push(`    u32 ${varName} = ${expr};`);
      return varName;
    } else {
      const childVarNames: string[] = [];
      if (node.children && node.children.length > 0) {
        node.children.forEach((child) => {
          childVarNames.push(generateNodeCExpression(child));
        });
      }
      const varId = node.id.replace(/[^a-zA-Z0-9]/g, "_");
      const varName = `group_${varId}`;
      if (childVarNames.length === 0) {
        lines.push(`    u32 ${varName} = 1;`);
      } else {
        const op = node.type === "AND" ? "&&" : "||";
        lines.push(
          `    u32 ${varName} = ${childVarNames
            .map((name) => `(${name})`)
            .join(` ${op} `)};`,
        );
      }
      return varName;
    }
  };

  const rootVarName = generateNodeCExpression(conditions);
  body += `\n${lines.join("\n")}\n    u32 matched = ${rootVarName};\n`;

  // Finish function body
  let mapDefinitions = "";
  let mapLookupBody = "";

  if (mapMode === "COUNTER") {
    if (mapKey === "comm") {
      mapDefinitions = `
struct block_key {
    char name[64];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct block_key);
    __type(value, u64);
} rate_limit_map SEC(".maps");
`;
      mapLookupBody = `
        struct block_key m_key = {};
        bpf_probe_read_kernel_str(m_key.name, sizeof(m_key.name), comm);
        u64 *count = bpf_map_lookup_elem(&rate_limit_map, &m_key);
        u64 init_val = 1;
        if (count) {
            __sync_fetch_and_add(count, 1);
            if (*count > ${mapLimit}) {
                matched = 1;
            } else {
                matched = 0;
            }
        } else {
            bpf_map_update_elem(&rate_limit_map, &m_key, &init_val, BPF_ANY);
            matched = 0;
        }
`;
    } else {
      const keyVar = mapKey === "uid" ? "uid" : "pid";
      mapDefinitions = `
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u64);
} rate_limit_map SEC(".maps");
`;
      mapLookupBody = `
        u32 m_key = ${keyVar};
        u64 *count = bpf_map_lookup_elem(&rate_limit_map, &m_key);
        u64 init_val = 1;
        if (count) {
            __sync_fetch_and_add(count, 1);
            if (*count > ${mapLimit}) {
                matched = 1;
            } else {
                matched = 0;
            }
        } else {
            bpf_map_update_elem(&rate_limit_map, &m_key, &init_val, BPF_ANY);
            matched = 0;
        }
`;
    }
  } else if (mapMode === "BLOCKLIST") {
    if (mapKey === "comm") {
      mapDefinitions = `
struct block_key {
    char name[64];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct block_key);
    __type(value, u32);
} blocklist_map SEC(".maps");
`;
      mapLookupBody = `
        struct block_key m_key = {};
        bpf_probe_read_kernel_str(m_key.name, sizeof(m_key.name), comm);
        u32 *is_blocked = bpf_map_lookup_elem(&blocklist_map, &m_key);
        if (is_blocked && *is_blocked) {
            matched = 1;
        } else {
            matched = 0;
        }
`;
    } else {
      const keyVar = mapKey === "uid" ? "uid" : "pid";
      mapDefinitions = `
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u32);
} blocklist_map SEC(".maps");
`;
      mapLookupBody = `
        u32 m_key = ${keyVar};
        u32 *is_blocked = bpf_map_lookup_elem(&blocklist_map, &m_key);
        if (is_blocked && *is_blocked) {
            matched = 1;
        } else {
            matched = 0;
        }
`;
    }
  }

  if (isKprobeUnlink) {
    body += `
    if (matched) {
        ${
          mapMode !== "NONE"
            ? `// Run stateful Map operation checks\n` +
              mapLookupBody.trim() +
              `\n\n        if (matched) {`
            : ""
        }
        bpf_printk("[Visual Plugin] matched unlink event: process %s (pid %d, uid %d, gid %d) deleted file\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        ${mapMode !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  } else {
    body += `
    if (matched) {
        ${
          mapMode !== "NONE"
            ? `// Run stateful Map operation checks\n` +
              mapLookupBody.trim() +
              `\n\n        if (matched) {`
            : ""
        }
        bpf_printk("[Visual Plugin] ${logPrefix} matched rule! process %s (pid %d, uid %d, gid %d)\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        return ${returnValLsm};
        ${mapMode !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  }

  return headers + mapDefinitions + body;
};
