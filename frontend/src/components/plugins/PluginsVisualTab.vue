<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { message } from "ant-design-vue";
import {
  DeleteOutlined,
  ThunderboltOutlined,
  FileTextOutlined,
  AlertOutlined,
  SafetyCertificateOutlined,
  FolderAddOutlined,
  FileAddOutlined,
  LinkOutlined,
  CloseCircleOutlined,
  DragOutlined,
} from "@ant-design/icons-vue";
import { usePlugins } from "../../composables/usePlugins";

import PluginsVisualAiPanel from "./PluginsVisualAiPanel.vue";
import PluginsVisualMapPanel from "./PluginsVisualMapPanel.vue";
import PluginsVisualCodePanel from "./PluginsVisualCodePanel.vue";
import PluginsVisualConditionTree from "./PluginsVisualConditionTree.vue";
import type { VisualLogicNode, VisualLogicGroup, VisualCondition } from "./types";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

const trigger = ref<
  | "process"
  | "file_open"
  | "mkdir"
  | "file_create"
  | "rmdir"
  | "symlink"
  | "unlink"
  | "socket_connect"
  | "inode_mknod"
  | "file_mprotect"
  | "inode_rename"
>("process");

const logicRoot = ref<VisualLogicGroup>({
  id: "root",
  type: "AND",
  children: [
    {
      id: "cond-init",
      type: "CONDITION",
      field: "comm",
      operator: "==",
      value: "nc",
    },
  ],
});

const action = ref<"BLOCK" | "ALERT" | "KILL">("BLOCK");

// Recursive logic tree helpers
const countConditions = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children) return 0;
  return node.children.reduce((sum, child) => sum + countConditions(child), 0);
};

const getTreeDepth = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children || node.children.length === 0) return 1;
  return 1 + Math.max(...node.children.map(getTreeDepth));
};

const findNodeAndMutate = (
  root: VisualLogicGroup,
  targetId: string,
  mutateFn: (parent: VisualLogicGroup, index: number) => void
): boolean => {
  if (root.id === targetId) return false;
  for (let i = 0; i < root.children.length; i++) {
    const child = root.children[i];
    if (child.id === targetId) {
      mutateFn(root, i);
      return true;
    }
    if (child.type === "AND" || child.type === "OR") {
      const found = findNodeAndMutate(child, targetId, mutateFn);
      if (found) return true;
    }
  }
  return false;
};

const findNodeById = (root: VisualLogicNode, targetId: string): VisualLogicNode | null => {
  if (root.id === targetId) return root;
  if (root.type === "AND" || root.type === "OR") {
    if (root.children) {
      for (const child of root.children) {
        const found = findNodeById(child, targetId);
        if (found) return found;
      }
    }
  }
  return null;
};

const onDeleteNode = (id: string) => {
  if (id === "root") return;
  findNodeAndMutate(logicRoot.value, id, (parent, idx) => {
    parent.children.splice(idx, 1);
  });
};

const onAddRule = (groupId: string, field?: string) => {
  const currentCount = countConditions(logicRoot.value);
  if (currentCount >= 8) {
    message.warning("为了防止 eBPF Verifier 复杂度过高而加载失败，图形化条件最多限制为 8 个");
    return;
  }
  const targetGroup = findNodeById(logicRoot.value, groupId);
  if (targetGroup && (targetGroup.type === "AND" || targetGroup.type === "OR")) {
    const id = `cond-${Math.random().toString(36).substr(2, 9)}`;
    targetGroup.children.push({
      id,
      type: "CONDITION",
      field: (field || "comm") as any,
      operator: "==",
      value: "",
    });
  }
};

const onAddGroup = (groupId: string, type: "AND" | "OR") => {
  const targetGroup = findNodeById(logicRoot.value, groupId);
  if (targetGroup && (targetGroup.type === "AND" || targetGroup.type === "OR")) {
    const id = `group-${Math.random().toString(36).substr(2, 9)}`;
    targetGroup.children.push({
      id,
      type,
      children: [],
    });
  }
};

const onUpdateRule = (ruleId: string, updated: Partial<VisualCondition>) => {
  const targetNode = findNodeById(logicRoot.value, ruleId);
  if (targetNode && targetNode.type === "CONDITION") {
    Object.assign(targetNode, updated);
  }
};

const onUpdateGroupType = (groupId: string, type: "AND" | "OR") => {
  const targetNode = findNodeById(logicRoot.value, groupId);
  if (targetNode && (targetNode.type === "AND" || targetNode.type === "OR")) {
    targetNode.type = type;
  }
};

// Low-Code Stateful Map configurations
const mapMode = ref<"NONE" | "COUNTER" | "BLOCKLIST">("NONE");
const mapKey = ref<"uid" | "pid" | "comm">("pid");
const mapLimit = ref<number>(10);

// AI Copilot Helper configurations
const aiPrompt = ref("");

const pluginId = ref("visual-plugin-custom-block");
const pluginName = ref("可视化流插件(custom-block)");
const description = ref("利用图形化流式积木拼装自动生成的内核级 eBPF 拦截器。");

const compiling = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");
const isCompiled = ref(false);

const triggerOptions = [
  {
    value: "process",
    label: "进程创建与加载 (LSM bprm_check)",
    icon: ThunderboltOutlined,
    color: "#1890ff",
  },
  {
    value: "file_open",
    label: "文件或目录被打开 (LSM file_open)",
    icon: FileTextOutlined,
    color: "#fa8c16",
  },
  {
    value: "file_create",
    label: "创建物理新文件 (LSM inode_create)",
    icon: FileAddOutlined,
    color: "#722ed1",
  },
  {
    value: "mkdir",
    label: "创建新目录文件夹 (LSM inode_mkdir)",
    icon: FolderAddOutlined,
    color: "#13c2c2",
  },
  {
    value: "rmdir",
    label: "删除已有文件夹 (LSM inode_rmdir)",
    icon: DeleteOutlined,
    color: "#eb2f96",
  },
  {
    value: "symlink",
    label: "创建软链接指引 (LSM inode_symlink)",
    icon: LinkOutlined,
    color: "#2f54eb",
  },
  {
    value: "unlink",
    label: "删除物理文件对象 (Kprobe unlink)",
    icon: AlertOutlined,
    color: "#f5222d",
  },
  {
    value: "socket_connect",
    label: "外发 socket 连接拦截 (LSM socket_connect)",
    icon: LinkOutlined,
    color: "#13c2c2",
  },
  {
    value: "inode_mknod",
    label: "物理特权设备节点创建 (LSM inode_mknod)",
    icon: FileAddOutlined,
    color: "#722ed1",
  },
  {
    value: "file_mprotect",
    label: "高危内存执行权限修改 (LSM file_mprotect)",
    icon: SafetyCertificateOutlined,
    color: "#eb2f96",
  },
  {
    value: "inode_rename",
    label: "关键文件路径重命名 (LSM inode_rename)",
    icon: FileTextOutlined,
    color: "#fa8c16",
  },
];

const fieldOptions = [
  { value: "comm", label: "当前进程名称 (Comm)" },
  { value: "pid", label: "当前进程 PID" },
  { value: "uid", label: "当前进程用户 UID" },
  { value: "basename", label: "操作目标文件名 (Basename)" },
  { value: "port", label: "目标网络端口 (Port)" },
  { value: "ipv4", label: "目标 IPv4 地址 (IPv4)" },
  { value: "gid", label: "当前进程组 GID" },
];


const ipToHex = (ip: string): string => {
  const parts = ip.split(".").map((p) => parseInt(p, 10));
  if (parts.length !== 4 || parts.some(isNaN)) return "0x00000000";
  return (
    "0x" +
    parts
      .map((p) => Math.min(255, Math.max(0, p)).toString(16).padStart(2, "0"))
      .join("")
  );
};

// Dynamic eBPF C Code Compiler Transpiler
const generatedBpfCode = computed(() => {
  const isKprobeUnlink = trigger.value === "unlink";

  const isKill = action.value === "KILL";
  const returnValLsm = (action.value === "BLOCK" || action.value === "KILL") ? "-EACCES" : "0";
  const logPrefix = isKill ? "Killed" : (action.value === "BLOCK" ? "Blocked" : "Alert");

  let headers = `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

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

  // 1. Hook function header
  if (trigger.value === "process") {
    body = `
SEC("lsm/bprm_check_security")
int BPF_PROG(visual_custom_plugin, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (exec_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), exec_name);
    }
`;
  } else if (trigger.value === "file_open") {
    body = `
SEC("lsm/file_open")
int BPF_PROG(visual_custom_plugin, struct file *file, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (
    trigger.value === "mkdir" ||
    trigger.value === "file_create" ||
    trigger.value === "rmdir" ||
    trigger.value === "symlink"
  ) {
    const secName =
      trigger.value === "mkdir"
        ? "lsm/inode_mkdir"
        : trigger.value === "file_create"
        ? "lsm/inode_create"
        : trigger.value === "rmdir"
        ? "lsm/inode_rmdir"
        : "lsm/inode_symlink";

    const funcArgs =
      trigger.value === "mkdir"
        ? "struct inode *dir, struct dentry *dentry, umode_t mode"
        : trigger.value === "file_create"
        ? "struct inode *dir, struct dentry *dentry, umode_t mode"
        : trigger.value === "rmdir"
        ? "struct inode *dir, struct dentry *dentry"
        : "struct inode *dir, struct dentry *dentry, const char *old_name";

    body = `
SEC("${secName}")
int BPF_PROG(visual_custom_plugin, ${funcArgs}) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger.value === "socket_connect") {
    body = `
SEC("lsm/socket_connect")
int BPF_PROG(visual_custom_plugin, struct socket *sock, struct sockaddr *address, int addrlen) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

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
  } else if (trigger.value === "inode_mknod") {
    body = `
SEC("lsm/inode_mknod")
int BPF_PROG(visual_custom_plugin, struct inode *dir, struct dentry *dentry, umode_t mode, dev_t dev) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger.value === "file_mprotect") {
    body = `
SEC("lsm/file_mprotect")
int BPF_PROG(visual_custom_plugin, struct vm_area_struct *vma, unsigned long reqprot, unsigned long prot, int ret) {
    if (ret != 0) return ret;
    if (!vma) return 0;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    struct file *file = BPF_CORE_READ(vma, vm_file);
    char name_buf[64] = {};
    if (file) {
        const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
        if (file_name) {
            bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
        }
    }
`;
  } else if (trigger.value === "inode_rename") {
    body = `
SEC("lsm/inode_rename")
int BPF_PROG(visual_custom_plugin, struct inode *old_dir, struct dentry *old_dentry, struct inode *new_dir, struct dentry *new_dentry) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(old_dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (isKprobeUnlink) {
    body = `
SEC("kprobe/do_unlinkat")
int BPF_PROG(visual_custom_plugin, struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
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
        node.children.forEach(child => {
          childVarNames.push(generateNodeCExpression(child));
        });
      }
      const varId = node.id.replace(/[^a-zA-Z0-9]/g, "_");
      const varName = `group_${varId}`;
      if (childVarNames.length === 0) {
        lines.push(`    u32 ${varName} = 1;`);
      } else {
        const op = node.type === "AND" ? "&&" : "||";
        lines.push(`    u32 ${varName} = ${childVarNames.map(name => `(${name})`).join(` ${op} `)};`);
      }
      return varName;
    }
  };

  const rootVarName = generateNodeCExpression(logicRoot.value);
  body += `\n${lines.join("\n")}\n    u32 matched = ${rootVarName};\n`;

  // Finish function body
  let mapDefinitions = "";
  let mapLookupBody = "";

  if (mapMode.value === "COUNTER") {
    if (mapKey.value === "comm") {
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
            if (*count > ${mapLimit.value}) {
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
      const keyVar = mapKey.value === "uid" ? "uid" : "pid";
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
            if (*count > ${mapLimit.value}) {
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
  } else if (mapMode.value === "BLOCKLIST") {
    if (mapKey.value === "comm") {
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
      const keyVar = mapKey.value === "uid" ? "uid" : "pid";
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
        ${mapMode.value !== "NONE" ? `// Run stateful Map operation checks\n` + mapLookupBody.trim() + `\n\n        if (matched) {` : ""}
        bpf_printk("[Visual Plugin] matched unlink event: process %s (pid %d, uid %d, gid %d) deleted file\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        ${mapMode.value !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  } else {
    body += `
    if (matched) {
        ${mapMode.value !== "NONE" ? `// Run stateful Map operation checks\n` + mapLookupBody.trim() + `\n\n        if (matched) {` : ""}
        bpf_printk("[Visual Plugin] ${logPrefix} matched rule! process %s (pid %d, uid %d, gid %d)\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        return ${returnValLsm};
        ${mapMode.value !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  }

  return headers + mapDefinitions + body;
});

// Watch inputs to auto-sync Manifest fields
watch(
  [trigger, logicRoot, action, mapMode, mapKey, mapLimit],
  () => {
    // Sanitize conditions recursively
    const sanitizeNode = (node: VisualLogicNode) => {
      if (node.type === "CONDITION") {
        if (trigger.value !== "socket_connect") {
          if (node.field === "port" || node.field === "ipv4") {
            node.field = "comm";
          }
        }
        if (trigger.value === "unlink") {
          if (node.field === "basename") {
            node.field = "comm";
          }
        }
      } else if (node.children) {
        node.children.forEach(sanitizeNode);
      }
    };
    sanitizeNode(logicRoot.value);

    // Extract first condition value to make descriptive id
    const leaves: VisualCondition[] = [];
    const findLeaves = (n: VisualLogicNode) => {
      if (n.type === "CONDITION") leaves.push(n);
      else if (n.children) n.children.forEach(findLeaves);
    };
    findLeaves(logicRoot.value);

    const firstVal = leaves[0]?.value || "custom";
    const prefix = `visual-block-${trigger.value}-${firstVal.replace(
      /[^a-z0-9]/g,
      "-"
    )}`.toLowerCase();
    pluginId.value = prefix;
    pluginName.value = `积木插件(${trigger.value}-${firstVal})`;
    description.value = `由图形化积木拼装而成的内核 eBPF 过滤审计插件。入口: ${trigger.value}，Map状态: ${mapMode.value}，嵌套层数: ${getTreeDepth(logicRoot.value)}，动作: ${action.value}。`;
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { deep: true, immediate: true }
);

// AI Translator callback
const handleAiTranslate = (payload: {
  trigger: any;
  action: "BLOCK" | "ALERT" | "KILL";
  conditions: VisualLogicGroup;
  mapMode: "NONE" | "COUNTER" | "BLOCKLIST";
  mapKey: "uid" | "pid" | "comm";
  mapLimit: number;
}) => {
  trigger.value = payload.trigger;
  action.value = payload.action;
  logicRoot.value = payload.conditions;
  mapMode.value = payload.mapMode;
  mapKey.value = payload.mapKey;
  mapLimit.value = payload.mapLimit;
};

onMounted(async () => {
  await fetchPlugins();
});

// Compile and upsert
const handleCompileAndRegister = async () => {
  compiling.value = true;
  compileLogLocal.value = "正在将高阶规则积木块转译为标准的 BPF C 源码...\n";
  try {
    compileLogLocal.value += `正在注册插件 Manifest [${pluginId.value}] 至本地仓库...\n`;
    await upsertPlugin({
      id: pluginId.value,
      name: pluginName.value,
      description: description.value,
      kind: "ebpf",
      enabled: false,
      attachKind: trigger.value === "unlink" ? "kprobe" : "none",
      attachTarget: trigger.value === "unlink" ? "do_unlinkat" : "",
      programName: "visual_custom_plugin",
      source: generatedBpfCode.value,
    });

    compileLogLocal.value +=
      "正在调用 LLVM/Clang 将源码编译为 ELF 内核字节码...\n";
    const success = await compileBpf(pluginId.value, generatedBpfCode.value);
    if (success) {
      isCompiled.value = true;
      compileLogLocal.value +=
        "\n[SUCCESS] 编译成功！点击下方按钮即可一键挂载至内核运行生效。";
    } else {
      compileLogLocal.value +=
        "\n[ERROR] 编译失败，请排查过滤表达式是否在内核 Verifier 安全范围内。";
    }
  } catch (err: any) {
    compileLogLocal.value += `\n[ERROR] 错误: ${err.message}`;
  } finally {
    compiling.value = false;
  }
};

const handleLoad = async () => {
  loadingAction.value = true;
  try {
    await loadBpf(pluginId.value);
    await fetchPlugins();
  } finally {
    loadingAction.value = false;
  }
};

const handleDragStart = (event: DragEvent, category: string, value: string) => {
  if (event.dataTransfer) {
    event.dataTransfer.setData("text/plain", JSON.stringify({ category, value }));
    event.dataTransfer.effectAllowed = "move";
  }
};

const handleWorkspaceDrop = (event: DragEvent) => {
  event.preventDefault();
  if (!event.dataTransfer) return;
  try {
    const rawData = event.dataTransfer.getData("text/plain");
    if (!rawData) return;
    const { category, value } = JSON.parse(rawData);

    if (category === "trigger") {
      trigger.value = value;
      message.success(`已切换事件挂载点为: ${value}`);
    } else if (category === "condition") {
      onAddRule("root", value);
      message.success(`已拖动添加匹配过滤: ${value}`);
    } else if (category === "logic_group") {
      onAddGroup("root", value as "AND" | "OR");
      message.success(`已拖动添加逻辑运算组: ${value}`);
    } else if (category === "map") {
      mapMode.value = value as any;
      message.success(`已配置 Map 状态存储为: ${value}`);
    } else if (category === "action") {
      if (trigger.value === "unlink" && value === "BLOCK") {
        message.error("unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL");
        return;
      }
      action.value = value as any;
      message.success(`已更新拦截响应动作为: ${value}`);
    }
  } catch (e) {
    console.error("Drop parsing failed:", e);
  }
};

// Dynamic tree layout algorithm for the logic gate schematic
const logicGateLayout = computed(() => {
  const elements: Array<{
    id: string;
    type: "condition" | "gate";
    label: string;
    x: number;
    y: number;
    field?: string;
    op?: string;
    value?: string;
  }> = [];

  const wires: Array<{
    d: string;
    color: string;
  }> = [];

  const leaves: VisualCondition[] = [];
  const findLeaves = (node: VisualLogicNode) => {
    if (node.type === "CONDITION") {
      leaves.push(node as VisualCondition);
    } else if (node.children) {
      node.children.forEach(findLeaves);
    }
  };
  findLeaves(logicRoot.value);

  const numLeaves = leaves.length;
  const leafMap = new Map<string, { x: number; y: number }>();

  leaves.forEach((leaf, idx) => {
    const x = 8;
    const y = numLeaves <= 1 ? 90 : 18 + (idx * (180 - 36)) / (numLeaves - 1);
    leafMap.set(leaf.id, { x, y });

    const opLabel = leaf.operator === '==' ? '=' : leaf.operator === '!=' ? '≠' : leaf.operator === 'starts_with' ? 'pref' : 'suff';
    elements.push({
      id: leaf.id,
      type: "condition",
      label: leaf.field,
      x,
      y,
      field: leaf.field,
      op: opLabel,
      value: leaf.value || '?',
    });
  });

  const nodePositionMap = new Map<string, { x: number; y: number }>();

  const positionNode = (node: VisualLogicNode, depth: number): { x: number; y: number } => {
    if (node.type === "CONDITION") {
      return leafMap.get(node.id) || { x: 8, y: 90 };
    }

    const childPosList: { x: number; y: number }[] = [];
    if (node.children && node.children.length > 0) {
      node.children.forEach(child => {
        childPosList.push(positionNode(child, depth + 1));
      });
    }

    let y = 90;
    if (childPosList.length > 0) {
      y = childPosList.reduce((sum, p) => sum + p.y, 0) / childPosList.length;
    }

    let x = 180;
    if (node.id !== "root") {
      x = Math.max(50, 180 - (depth) * 35);
    }

    nodePositionMap.set(node.id, { x, y });

    elements.push({
      id: node.id,
      type: "gate",
      label: node.type,
      x,
      y,
    });

    if (node.children && node.children.length > 0) {
      node.children.forEach(child => {
        const childPos = nodePositionMap.get(child.id) || leafMap.get(child.id) || { x: 8, y: 90 };
        const startX = childPos.x + (child.type === "CONDITION" ? 85 : 14);
        const startY = childPos.y;
        const endX = x - 14;
        const endY = y;

        const path = `M ${startX} ${startY} C ${startX + 15} ${startY}, ${endX - 15} ${endY}, ${endX} ${endY}`;
        const color = node.type === "AND" ? "url(#wire-gradient-and)" : "url(#wire-gradient-or)";
        wires.push({ d: path, color });
      });
    }

    return { x, y };
  };

  positionNode(logicRoot.value, 0);

  return { elements, wires };
});
</script>

<template>
  <div class="plugins-visual-tab">
    <a-row :gutter="16">
      <!-- Column 1: UE Blueprint Palette (Drag Source) -->
      <a-col :span="5">
        <div class="blueprint-palette">
          <div class="palette-header">
            <DragOutlined class="palette-icon" />
            <h4>蓝图组件库 (Palette)</h4>
          </div>
          <div class="palette-desc">
            拖拽下列组件到右侧画布即可快速拼接 eBPF 过滤流。
          </div>
          
          <!-- Category 1: Trigger Hooks -->
          <div class="palette-category">
            <div class="category-title">事件触发器 (Triggers)</div>
            <div class="palette-items">
              <div
                v-for="opt in triggerOptions"
                :key="opt.value"
                class="palette-item item-trigger"
                draggable="true"
                @dragstart="handleDragStart($event, 'trigger', opt.value)"
              >
                <component :is="opt.icon" :style="{ color: '#1890ff', marginRight: '6px' }" />
                <span class="item-text" :title="opt.label">{{ opt.value }}</span>
              </div>
            </div>
          </div>

          <!-- Category 2: Conditions -->
          <div class="palette-category">
            <div class="category-title">过滤条件 (Conditions)</div>
            <div class="palette-items">
              <div
                v-for="opt in fieldOptions"
                :key="opt.value"
                class="palette-item item-condition"
                draggable="true"
                @dragstart="handleDragStart($event, 'condition', opt.value)"
              >
                <span class="item-dot condition-dot"></span>
                <span class="item-text" :title="opt.label">{{ opt.value }}</span>
              </div>
            </div>
          </div>

          <!-- Category 2.5: Logic Groups -->
          <div class="palette-category">
            <div class="category-title">逻辑分组 (Logic Groups)</div>
            <div class="palette-items">
              <div
                class="palette-item item-group-and"
                style="border-left: 3px solid #1890ff;"
                draggable="true"
                @dragstart="handleDragStart($event, 'logic_group', 'AND')"
              >
                <span class="item-dot" style="background: #1890ff; box-shadow: 0 0 6px #1890ff;"></span>
                <span class="item-text" title="且运算组 (AND Group)">AND Group</span>
              </div>
              <div
                class="palette-item item-group-or"
                style="border-left: 3px solid #eb2f96;"
                draggable="true"
                @dragstart="handleDragStart($event, 'logic_group', 'OR')"
              >
                <span class="item-dot" style="background: #eb2f96; box-shadow: 0 0 6px #eb2f96;"></span>
                <span class="item-text" title="或运算组 (OR Group)">OR Group</span>
              </div>
            </div>
          </div>

          <!-- Category 3: Map Operations -->
          <div class="palette-category">
            <div class="category-title">状态机制 (State Maps)</div>
            <div class="palette-items">
              <div
                class="palette-item item-map"
                draggable="true"
                @dragstart="handleDragStart($event, 'map', 'COUNTER')"
              >
                <span class="item-dot map-dot"></span>
                <span class="item-text" title="计数器限频 (COUNTER)">COUNTER</span>
              </div>
              <div
                class="palette-item item-map"
                draggable="true"
                @dragstart="handleDragStart($event, 'map', 'BLOCKLIST')"
              >
                <span class="item-dot map-dot"></span>
                <span class="item-text" title="黑名单判定 (BLOCKLIST)">BLOCKLIST</span>
              </div>
              <div
                class="palette-item item-map"
                draggable="true"
                @dragstart="handleDragStart($event, 'map', 'NONE')"
              >
                <span class="item-dot map-dot-none"></span>
                <span class="item-text" title="无状态 (NONE)">NONE</span>
              </div>
            </div>
          </div>

          <!-- Category 4: Response Actions -->
          <div class="palette-category">
            <div class="category-title">响应动作 (Actions)</div>
            <div class="palette-items">
              <div
                class="palette-item item-action"
                draggable="true"
                @dragstart="handleDragStart($event, 'action', 'BLOCK')"
              >
                <span class="item-dot action-dot"></span>
                <span class="item-text" title="硬拦截 (BLOCK)">BLOCK</span>
              </div>
              <div
                class="palette-item item-action"
                draggable="true"
                @dragstart="handleDragStart($event, 'action', 'ALERT')"
              >
                <span class="item-dot action-dot"></span>
                <span class="item-text" title="告警审计 (ALERT)">ALERT</span>
              </div>
              <div
                class="palette-item item-action"
                draggable="true"
                @dragstart="handleDragStart($event, 'action', 'KILL')"
              >
                <span class="item-dot action-dot"></span>
                <span class="item-text" title="强制杀死 (KILL)">KILL</span>
              </div>
            </div>
          </div>
        </div>
      </a-col>

      <!-- Column 2: Workspace (Designer Canvas) -->
      <a-col :span="11">
        <div class="graphical-workspace" @dragover.prevent @drop="handleWorkspaceDrop">
          <div class="workspace-title">
            <h3>流程图高级规则拼接控制台 (Advanced Flow Designer)</h3>
            <span class="sub"
              >通过拼接多重高级匹配字段与触发点，在系统内核深层执行精密入侵侦测。</span
            >
          </div>

          <!-- BLOCK 1: EVENT TRIGGER -->
          <div class="block-card block-trigger">
            <!-- Node port -->
            <div class="node-port port-output trigger-port"></div>

            <div class="block-header">
              <span class="block-badge">Block 1</span>
              <strong style="color: #fff"
                >防御拦截挂载点积木 (Trigger Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">选择安全管控的内核底层事件拦截入口：</div>
              <a-select v-model:value="trigger" style="width: 100%">
                <a-select-option
                  v-for="opt in triggerOptions"
                  :key="opt.value"
                  :value="opt.value"
                >
                  <component :is="opt.icon" :style="{ color: opt.color }" />
                  <span style="margin-left: 8px">{{ opt.label }}</span>
                </a-select-option>
              </a-select>
            </div>
          </div>

          <!-- CONNECTION ARROW -->
          <div class="blueprint-wire-container">
            <div class="blueprint-wire-line wire-1-to-2"></div>
            <div class="blueprint-wire-pulse pulse-1-to-2"></div>
          </div>

          <!-- BLOCK 2: DYNAMIC CONDITIONS & AND/OR RELATION -->
          <div class="block-card block-condition">
            <!-- Node ports -->
            <div class="node-port port-input condition-port-in"></div>
            <div class="node-port port-output condition-port-out"></div>

            <div class="block-header">
              <div>
                <span class="block-badge" style="background: #fa8c16">Block 2</span>
                <strong style="color: #fff">高级嵌套逻辑过滤条件 (Nested Condition Block)</strong>
              </div>
            </div>
            <div class="block-body">
              <a-row :gutter="16">
                <!-- Condition Tree -->
                <a-col :span="15">
                  <div class="desc-line" style="margin-bottom: 16px;">
                    支持无限嵌套的逻辑运算组，可从左侧拖拽条件或逻辑组至目标块内：
                  </div>
                  
                  <div class="conditions-list-tree" style="max-height: 380px; overflow-y: auto; padding-right: 4px;">
                    <PluginsVisualConditionTree
                      :node="logicRoot"
                      :trigger="trigger"
                      :on-delete-node="onDeleteNode"
                      :on-add-rule="onAddRule"
                      :on-add-group="onAddGroup"
                      :on-update-rule="onUpdateRule"
                      :on-update-group-type="onUpdateGroupType"
                    />
                  </div>
                </a-col>

                <!-- Blueprint Logic Gate Visualizer (Fully integrated SVG tree) -->
                <a-col :span="9" style="border-left: 1px dashed rgba(255, 255, 255, 0.1); padding-left: 16px;">
                  <div style="font-size: 12px; font-weight: 600; color: #fa8c16; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; display: flex; align-items: center; justify-content: space-between;">
                    <span>逻辑拓扑树 (Logic Tree Gate)</span>
                    <a-tag size="small" color="orange" style="font-size: 10px; margin: 0; transform: scale(0.9);">Schematic</a-tag>
                  </div>
                  
                  <div class="logic-gate-canvas">
                    <div class="logic-gate-grid"></div>

                    <!-- SVG containing both dynamic bezier wires and gate/node elements -->
                    <svg class="logic-gate-wires" viewBox="0 0 200 180" width="100%" height="100%" preserveAspectRatio="xMidYMid meet">
                      <defs>
                        <linearGradient id="wire-gradient-and" x1="0%" y1="0%" x2="100%" y2="0%">
                          <stop offset="0%" stop-color="#1890ff" />
                          <stop offset="100%" stop-color="#0050b3" />
                        </linearGradient>
                        <linearGradient id="wire-gradient-or" x1="0%" y1="0%" x2="100%" y2="0%">
                          <stop offset="0%" stop-color="#eb2f96" />
                          <stop offset="100%" stop-color="#722ed1" />
                        </linearGradient>
                        <radialGradient id="gate-grad-and" cx="50%" cy="50%" r="50%">
                          <stop offset="0%" stop-color="#0077b6" />
                          <stop offset="100%" stop-color="#03045e" />
                        </radialGradient>
                        <radialGradient id="gate-grad-or" cx="50%" cy="50%" r="50%">
                          <stop offset="0%" stop-color="#d946ef" />
                          <stop offset="100%" stop-color="#701a75" />
                        </radialGradient>
                        <filter id="wire-glow">
                          <feGaussianBlur stdDeviation="1" result="coloredBlur"/>
                          <feMerge>
                            <feMergeNode in="coloredBlur"/>
                            <feMergeNode in="SourceGraphic"/>
                          </feMerge>
                        </filter>
                      </defs>
                      
                      <!-- Connections / Bezier wires -->
                      <path
                        v-for="(wire, idx) in logicGateLayout.wires"
                        :key="'wire-' + idx"
                        :d="wire.d"
                        :stroke="wire.color"
                        stroke-width="1.5"
                        fill="none"
                        filter="url(#wire-glow)"
                        opacity="0.8"
                      />

                      <!-- Logic Gates and Condition Badges -->
                      <g v-for="elem in logicGateLayout.elements" :key="elem.id">
                        <!-- Condition Nodes -->
                        <g v-if="elem.type === 'condition'">
                          <rect
                            :x="elem.x"
                            :y="elem.y - 10"
                            width="85"
                            height="20"
                            rx="3"
                            fill="#1e293b"
                            stroke="#334155"
                            stroke-width="1"
                          />
                          <text :x="elem.x + 4" :y="elem.y + 3" fill="#00b4d8" font-size="7" font-family="monospace" font-weight="bold">{{ elem.field }}</text>
                          <text :x="elem.x + 45" :y="elem.y + 3" fill="#fa8c16" font-size="7" font-family="monospace">{{ elem.op }}</text>
                          <text :x="elem.x + 58" :y="elem.y + 3" fill="#a78bfa" font-size="7" font-family="monospace">{{ elem.value }}</text>
                        </g>

                        <!-- Gate Nodes -->
                        <g v-else>
                          <circle
                            :cx="elem.x"
                            :cy="elem.y"
                            r="13"
                            :fill="elem.label === 'AND' ? 'url(#gate-grad-and)' : 'url(#gate-grad-or)'"
                            :stroke="elem.label === 'AND' ? '#00b4d8' : '#f472b6'"
                            stroke-width="1.5"
                          />
                          <text :x="elem.x" :y="elem.y - 1" text-anchor="middle" fill="#fff" font-size="8" font-family="monospace" font-weight="bold">{{ elem.label }}</text>
                          <text :x="elem.x" :y="elem.y + 7" text-anchor="middle" fill="rgba(255,255,255,0.6)" font-size="5" font-family="monospace">GATE</text>
                        </g>
                      </g>
                    </svg>
                  </div>
                </a-col>
              </a-row>
            </div>
          </div>

          <!-- CONNECTION ARROW -->
          <div class="blueprint-wire-container">
            <div class="blueprint-wire-line wire-2-to-2-5"></div>
            <div class="blueprint-wire-pulse pulse-2-to-2-5"></div>
          </div>

          <!-- BLOCK 2.5: STATEFUL MAP OPERATIONS -->
          <PluginsVisualMapPanel
            v-model:mode="mapMode"
            v-model:key-field="mapKey"
            v-model:limit="mapLimit"
          />

          <!-- CONNECTION ARROW -->
          <div class="blueprint-wire-container">
            <div class="blueprint-wire-line wire-2-5-to-3"></div>
            <div class="blueprint-wire-pulse pulse-2-5-to-3"></div>
          </div>

          <!-- BLOCK 3: TARGET ACTION -->
          <div class="block-card block-action">
            <!-- Node port -->
            <div class="node-port port-input action-port-in"></div>

            <div class="block-header">
              <span class="block-badge" style="background: #52c41a"
                >Block 3</span
              >
              <strong style="color: #fff"
                >安全管控响应积木 (Action Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">
                当上述过滤组合触发成功时，内核要执行的安全响应动作：
              </div>
              <a-radio-group
                v-model:value="action"
                button-style="solid"
                style="width: 100%"
              >
                <a-radio-button
                  value="BLOCK"
                  class="block-red"
                  :disabled="trigger === 'unlink'"
                  style="width: 33.3%; text-align: center"
                >
                  <SafetyCertificateOutlined /> BLOCK (硬拦截)
                </a-radio-button>
                <a-radio-button
                  value="ALERT"
                  style="width: 33.3%; text-align: center"
                >
                  <AlertOutlined /> ALERT (告警)
                </a-radio-button>
                <a-radio-button
                  value="KILL"
                  class="block-red"
                  style="width: 33.3%; text-align: center"
                >
                  <CloseCircleOutlined /> KILL (强制处死)
                </a-radio-button>
              </a-radio-group>
              <div
                v-if="trigger === 'unlink'"
                class="helper-text"
                style="color: #fa8c16; margin-top: 8px"
              >
                * 物理文件 unlink 挂载于 Kprobe 上，不改变内核决策链，仅支持 ALERT 或 KILL 动作。其他 LSM 挂载点支持完整的 BLOCK、ALERT 与 KILL 动作。
              </div>
            </div>
          </div>

          <!-- Plugin Details Panel -->
          <a-card
            title="规则插件注册配置 (Plugin Metadata)"
            size="small"
            style="margin-top: 24px"
          >
            <a-form layout="vertical">
              <a-row :gutter="12">
                <a-col :span="12">
                  <a-form-item label="自定义规则插件 ID">
                    <a-input
                      v-model:value="pluginId"
                      placeholder="例如 custom-visual-lsm"
                    />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item label="规则插件显示名">
                    <a-input v-model:value="pluginName" />
                  </a-form-item>
                </a-col>
              </a-row>
              <a-form-item label="详细说明描述" style="margin-bottom: 0">
                <a-textarea v-model:value="description" :rows="2" />
              </a-form-item>
            </a-form>

            <div
              style="margin-top: 20px; display: flex; justify-content: flex-end"
            >
              <a-button
                type="primary"
                :loading="compiling"
                @click="handleCompileAndRegister"
              >
                <template #icon><ThunderboltOutlined /></template>
                一键编译并注册为 BPF 插件
              </a-button>
            </div>
          </a-card>
        </div>
      </a-col>

      <!-- Column 3: AI Copilot & Code Preview (Stacked on the right) -->
      <a-col :span="8">
        <!-- AI COPILOT HELPER PANEL (BLOCK 0) -->
        <PluginsVisualAiPanel v-model="aiPrompt" @translate="handleAiTranslate" />

        <div style="margin-top: 16px;">
          <PluginsVisualCodePanel
            :code="generatedBpfCode"
            :compiling="compiling"
            :compiled="isCompiled"
            :loading="loadingAction"
            :log="compileLogLocal"
            @load="handleLoad"
          />
        </div>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.plugins-visual-tab {
  min-height: 600px;
}
.graphical-workspace {
  background-color: #0b132b;
  background-image: 
    linear-gradient(to right, rgba(28, 37, 65, 0.4) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(28, 37, 65, 0.4) 1px, transparent 1px),
    linear-gradient(to right, rgba(28, 37, 65, 0.15) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(28, 37, 65, 0.15) 1px, transparent 1px);
  background-size: 40px 40px, 40px 40px, 10px 10px, 10px 10px;
  border: 1px solid #1c2541;
  border-radius: 12px;
  padding: 24px;
  box-shadow: inset 0 0 40px rgba(0, 0, 0, 0.8);
  position: relative;
}
.workspace-title {
  margin-bottom: 20px;
  border-left: 4px solid #1890ff;
  padding-left: 10px;
}
.workspace-title h3 {
  margin: 0;
  font-weight: 600;
  color: #ffffff;
}
.workspace-title .sub {
  font-size: 12px;
  color: #94a3b8;
}

/* Blueprint nodes styling */
.block-card {
  border-radius: 8px;
  overflow: visible; /* to show ports */
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  background: rgba(13, 19, 33, 0.85);
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.7);
}

.block-trigger {
  border-color: rgba(24, 144, 255, 0.35);
}
.block-trigger:hover {
  border-color: rgba(24, 144, 255, 0.7);
  box-shadow: 0 0 15px rgba(24, 144, 255, 0.2);
}
.block-trigger .block-header {
  background: linear-gradient(135deg, #1890ff, #0050b3);
}

.block-condition {
  border-color: rgba(250, 140, 22, 0.35);
}
.block-condition:hover {
  border-color: rgba(250, 140, 22, 0.7);
  box-shadow: 0 0 15px rgba(250, 140, 22, 0.2);
}
.block-condition .block-header {
  background: linear-gradient(135deg, #fa8c16, #ad4e00);
}

.block-action {
  border-color: rgba(82, 196, 26, 0.35);
}
.block-action:hover {
  border-color: rgba(82, 196, 26, 0.7);
  box-shadow: 0 0 15px rgba(82, 196, 26, 0.2);
}
.block-action .block-header {
  background: linear-gradient(135deg, #52c41a, #237804);
}

.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}
.block-badge {
  background: rgba(0, 0, 0, 0.35);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: #0f172a;
  padding: 18px;
  color: #cbd5e1;
}
.desc-line {
  font-size: 13px;
  color: #94a3b8;
  margin-bottom: 12px;
}

/* Blueprint wires */
.blueprint-wire-container {
  height: 36px;
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
}
.blueprint-wire-line {
  width: 2px;
  height: 100%;
}
.wire-1-to-2 {
  background: linear-gradient(180deg, #1890ff, #fa8c16);
}
.wire-2-to-2-5 {
  background: linear-gradient(180deg, #fa8c16, #722ed1);
}
.wire-2-5-to-3 {
  background: linear-gradient(180deg, #722ed1, #52c41a);
}
.blueprint-wire-pulse {
  position: absolute;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  top: 0;
  animation: wire-pulse-run 1.5s infinite linear;
}
.pulse-1-to-2 {
  background: #1890ff;
  box-shadow: 0 0 8px #1890ff, 0 0 15px #1890ff;
}
.pulse-2-to-2-5 {
  background: #fa8c16;
  box-shadow: 0 0 8px #fa8c16, 0 0 15px #fa8c16;
}
.pulse-2-5-to-3 {
  background: #722ed1;
  box-shadow: 0 0 8px #722ed1, 0 0 15px #722ed1;
}

@keyframes wire-pulse-run {
  0% {
    top: 0%;
    opacity: 0;
  }
  10% {
    opacity: 1;
  }
  90% {
    opacity: 1;
  }
  100% {
    top: 100%;
    opacity: 0;
  }
}

/* Node ports */
.node-port {
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  border: 1px solid rgba(255, 255, 255, 0.6);
}
.port-input {
  top: -5px;
}
.port-output {
  bottom: -5px;
}

.trigger-port {
  background: #1890ff;
  border-color: #1890ff;
  box-shadow: 0 0 8px #1890ff;
}
.condition-port-in {
  background: #1890ff;
  border-color: #1890ff;
  box-shadow: 0 0 8px #1890ff;
}
.condition-port-out {
  background: #fa8c16;
  border-color: #fa8c16;
  box-shadow: 0 0 8px #fa8c16;
}
.action-port-in {
  background: #722ed1;
  border-color: #722ed1;
  box-shadow: 0 0 8px #722ed1;
}

/* Condition inputs and layout */
.condition-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.helper-text {
  font-size: 11px;
}
.block-red.ant-radio-button-wrapper-checked {
  background: #f5222d;
  border-color: #f5222d;
  color: white;
}

/* Deep input styling for dark mode */
:deep(.graphical-workspace .ant-select-selector),
:deep(.graphical-workspace .ant-input),
:deep(.graphical-workspace .ant-input-number),
:deep(.graphical-workspace .ant-radio-button-wrapper) {
  background-color: #1e293b !important;
  border-color: #334155 !important;
  color: #f1f5f9 !important;
}
:deep(.graphical-workspace .ant-select-arrow) {
  color: #94a3b8 !important;
}
:deep(.graphical-workspace .ant-radio-button-wrapper-checked) {
  background-color: #1890ff !important;
  color: #ffffff !important;
  border-color: #1890ff !important;
}
:deep(.graphical-workspace .ant-radio-button-wrapper-checked.block-red) {
  background-color: #ef4444 !important;
  border-color: #ef4444 !important;
}
:deep(.graphical-workspace .ant-btn-dashed) {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: #475569 !important;
  color: #94a3b8 !important;
}
:deep(.graphical-workspace .ant-btn-dashed:hover) {
  border-color: #fa8c16 !important;
  color: #fa8c16 !important;
}
:deep(.graphical-workspace .ant-card) {
  background: #0f172a !important;
  border-color: rgba(255, 255, 255, 0.05) !important;
}
:deep(.graphical-workspace .ant-card-head) {
  border-bottom-color: rgba(255, 255, 255, 0.05) !important;
  color: #ffffff !important;
  background: #1e293b !important;
}

/* Logic gate visualizer styles */
.logic-gate-canvas {
  height: 180px;
  position: relative;
  border: 1px solid rgba(250, 140, 22, 0.2);
  background: rgba(13, 19, 33, 0.4);
  border-radius: 6px;
  overflow: hidden;
  box-shadow: inset 0 0 15px rgba(0, 0, 0, 0.5);
}
.logic-gate-grid {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-image: 
    linear-gradient(to right, rgba(250, 140, 22, 0.05) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(250, 140, 22, 0.05) 1px, transparent 1px);
  background-size: 15px 15px;
  pointer-events: none;
}
.logic-gate-wires {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 1;
}
.logic-input-node {
  position: absolute;
  left: 8px;
  width: 95px;
  height: 28px;
  background: rgba(28, 37, 65, 0.85);
  border: 1px solid rgba(24, 144, 255, 0.4);
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 6px;
  font-family: monospace;
  font-size: 10px;
  color: #e2e8f0;
  z-index: 2;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
  transition: all 0.2s ease;
}
.logic-input-node:hover {
  border-color: #1890ff;
  box-shadow: 0 0 8px rgba(24, 144, 255, 0.4);
}
.node-field {
  color: #00b4d8;
  font-weight: bold;
}
.node-op {
  color: #fa8c16;
}
.node-val {
  color: #a78bfa;
  max-width: 42px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.logic-gate-node {
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  width: 44px;
  height: 44px;
  border-radius: 50%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  z-index: 2;
  font-family: 'Consolas', monospace;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
  border: 2px solid rgba(255, 255, 255, 0.15);
  transition: all 0.3s ease;
}
.logic-gate-node.and {
  background: radial-gradient(circle, #0077b6 0%, #03045e 100%);
  border-color: #00b4d8;
  box-shadow: 0 0 12px rgba(0, 180, 216, 0.5);
}
.logic-gate-node.or {
  background: radial-gradient(circle, #d946ef 0%, #701a75 100%);
  border-color: #f472b6;
  box-shadow: 0 0 12px rgba(244, 114, 182, 0.5);
}
.gate-icon {
  font-size: 11px;
  font-weight: 900;
  color: #fff;
  line-height: 1;
}
.gate-label {
  font-size: 7px;
  color: rgba(255, 255, 255, 0.7);
  margin-top: 2px;
  font-weight: bold;
}

/* UE Blueprint Palette Styling */
.blueprint-palette {
  background-color: #121620;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  padding: 16px;
  min-height: 550px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  font-family: monospace;
}

.palette-header {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  padding-bottom: 8px;
}

.palette-icon {
  font-size: 14px;
  margin-right: 6px;
  color: #fa8c16;
}

.palette-header h4 {
  margin: 0;
  color: #f1f5f9;
  font-size: 13px;
  font-weight: 600;
}

.palette-desc {
  font-size: 10px;
  color: #64748b;
  line-height: 1.4;
  margin-bottom: 16px;
}

.palette-category {
  margin-bottom: 16px;
}

.category-title {
  font-size: 11px;
  font-weight: bold;
  color: #94a3b8;
  margin-bottom: 8px;
  padding-bottom: 2px;
  border-bottom: 1px dashed rgba(255, 255, 255, 0.05);
  text-transform: uppercase;
}

.palette-items {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.palette-item {
  background: rgba(30, 41, 59, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.03);
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 11px;
  color: #cbd5e1;
  cursor: grab;
  display: flex;
  align-items: center;
  transition: all 0.2s ease;
  user-select: none;
}

.palette-item:active {
  cursor: grabbing;
}

.palette-item:hover {
  background: rgba(30, 41, 59, 0.95);
  transform: translateX(2px);
  color: #ffffff;
}

/* Color Coding for Palette Items (matching blueprint color accents) */
.item-trigger:hover {
  border-color: #1890ff;
  box-shadow: 0 0 8px rgba(24, 144, 255, 0.25);
}

.item-condition:hover {
  border-color: #fa8c16;
  box-shadow: 0 0 8px rgba(250, 140, 22, 0.25);
}

.item-map:hover {
  border-color: #722ed1;
  box-shadow: 0 0 8px rgba(114, 46, 209, 0.25);
}

.item-action:hover {
  border-color: #52c41a;
  box-shadow: 0 0 8px rgba(82, 196, 26, 0.25);
}

.item-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Dots and accents */
.item-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 8px;
  flex-shrink: 0;
}

.condition-dot {
  background: #fa8c16;
  box-shadow: 0 0 6px #fa8c16;
}

.map-dot {
  background: #722ed1;
  box-shadow: 0 0 6px #722ed1;
}

.map-dot-none {
  background: #94a3b8;
}

.action-dot {
  background: #52c41a;
  box-shadow: 0 0 6px #52c41a;
}
</style>
