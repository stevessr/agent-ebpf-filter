<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { message } from "ant-design-vue";
import {
  PlusOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  FileTextOutlined,
  DownOutlined,
  AlertOutlined,
  SafetyCertificateOutlined,
  FolderAddOutlined,
  FileAddOutlined,
  LinkOutlined,
  CloseCircleOutlined,
} from "@ant-design/icons-vue";
import { usePlugins } from "../../composables/usePlugins";

import PluginsVisualAiPanel from "./PluginsVisualAiPanel.vue";
import PluginsVisualMapPanel from "./PluginsVisualMapPanel.vue";
import PluginsVisualCodePanel from "./PluginsVisualCodePanel.vue";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

// Visual advanced blocks configurations
export interface VisualCondition {
  field: "comm" | "pid" | "uid" | "basename" | "port" | "ipv4" | "gid";
  operator: "==" | "!=" | "starts_with" | "ends_with";
  value: string;
}

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
const logicRelation = ref<"AND" | "OR">("AND");
const conditions = ref<VisualCondition[]>([
  { field: "comm", operator: "==", value: "nc" },
]);
const action = ref<"BLOCK" | "ALERT" | "KILL">("BLOCK");

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

const operatorOptions = [
  { value: "==", label: "等于 (==)" },
  { value: "!=", label: "不等于 (!=)" },
  { value: "starts_with", label: "前缀匹配 (starts_with)" },
  { value: "ends_with", label: "后缀匹配 (ends_with)" },
];

// Add/Remove condition blocks
const addCondition = () => {
  if (conditions.value.length >= 5) {
    message.warning(
      "为了防止 eBPF Verifier 复杂度限制，图形化条件最多限制为 5 个"
    );
    return;
  }
  conditions.value.push({ field: "comm", operator: "==", value: "" });
};

const removeCondition = (index: number) => {
  conditions.value.splice(index, 1);
};

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

  // Initialise matching logic based on logical OR or AND
  if (logicRelation.value === "AND") {
    body += `\n    u32 matched = 1;\n`;
  } else {
    body += `\n    u32 matched = 0;\n`;
  }

  // Iterate conditions
  conditions.value.forEach((cond) => {
    const val = cond.value.trim();
    if (!val) return;

    let expr = "";
    if (cond.field === "comm") {
      if (cond.operator === "==") {
        expr = `strcmp_const(comm, "${val}", sizeof(comm)) == 0`;
      } else if (cond.operator === "!=") {
        expr = `strcmp_const(comm, "${val}", sizeof(comm)) != 0`;
      } else if (cond.operator === "starts_with") {
        expr = `str_starts_with(comm, "${val}", sizeof(comm)) != 0`;
      } else if (cond.operator === "ends_with") {
        expr = `str_ends_with(comm, get_str_len(comm, sizeof(comm)), "${val}", ${val.length}) != 0`;
      }
    } else if (cond.field === "pid") {
      const pidNum = parseInt(val, 10) || 0;
      if (cond.operator === "==") expr = `pid == ${pidNum}`;
      else expr = `pid != ${pidNum}`;
    } else if (cond.field === "uid") {
      const uidNum = parseInt(val, 10) || 0;
      if (cond.operator === "==") expr = `uid == ${uidNum}`;
      else expr = `uid != ${uidNum}`;
    } else if (cond.field === "gid") {
      const gidNum = parseInt(val, 10) || 0;
      if (cond.operator === "==") expr = `gid == ${gidNum}`;
      else expr = `gid != ${gidNum}`;
    } else if (cond.field === "port") {
      const portNum = parseInt(val, 10) || 0;
      if (cond.operator === "==") expr = `dst_port == ${portNum}`;
      else expr = `dst_port != ${portNum}`;
    } else if (cond.field === "ipv4") {
      const hexIp = ipToHex(val);
      if (cond.operator === "==") expr = `dst_ipv4 == ${hexIp}`;
      else expr = `dst_ipv4 != ${hexIp}`;
    } else if (cond.field === "basename") {
      if (isKprobeUnlink) {
        expr = "0"; // unlink lacks dentry basename
      } else {
        if (cond.operator === "==") {
          expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) == 0`;
        } else if (cond.operator === "!=") {
          expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) != 0`;
        } else if (cond.operator === "starts_with") {
          expr = `str_starts_with(name_buf, "${val}", sizeof(name_buf)) != 0`;
        } else if (cond.operator === "ends_with") {
          expr = `str_ends_with(name_buf, get_str_len(name_buf, sizeof(name_buf)), "${val}", ${val.length}) != 0`;
        }
      }
    }

    if (expr) {
      if (logicRelation.value === "AND") {
        body += `    if (!(${expr})) matched = 0;\n`;
      } else {
        body += `    if (${expr}) matched = 1;\n`;
      }
    }
  });

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
  [trigger, conditions, action, logicRelation, mapMode, mapKey, mapLimit],
  () => {
    // Sanitize conditions depending on trigger
    if (trigger.value !== "socket_connect") {
      conditions.value.forEach((cond) => {
        if (cond.field === "port" || cond.field === "ipv4") {
          cond.field = "comm";
        }
      });
    }
    if (trigger.value === "unlink") {
      conditions.value.forEach((cond) => {
        if (cond.field === "basename") {
          cond.field = "comm";
        }
      });
    }

    const firstVal = conditions.value[0]?.value || "custom";
    const prefix = `visual-block-${trigger.value}-${firstVal.replace(
      /[^a-z0-9]/g,
      "-"
    )}`.toLowerCase();
    pluginId.value = prefix;
    pluginName.value = `积木插件(${trigger.value}-${firstVal})`;
    description.value = `由图形化积木拼装而成的内核 eBPF 过滤审计插件。入口: ${trigger.value}，Map状态: ${mapMode.value}，关系: ${logicRelation.value}，动作: ${action.value}。`;
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { deep: true, immediate: true }
);

// AI Translator callback
const handleAiTranslate = (payload: {
  trigger: any;
  action: "BLOCK" | "ALERT" | "KILL";
  conditions: any[];
  mapMode: "NONE" | "COUNTER" | "BLOCKLIST";
  mapKey: "uid" | "pid" | "comm";
  mapLimit: number;
}) => {
  trigger.value = payload.trigger;
  action.value = payload.action;
  conditions.value = payload.conditions;
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
</script>

<template>
  <div class="plugins-visual-tab">
    <a-row :gutter="20">
      <!-- Graphical programming layout -->
      <a-col :span="13">
        <div class="graphical-workspace">
          <div class="workspace-title">
            <h3>流程图高级规则拼接控制台 (Advanced Flow Designer)</h3>
            <span class="sub"
              >通过拼接多重高级匹配字段与触发点，在系统内核深层执行精密入侵侦测。</span
            >
          </div>

          <!-- AI COPILOT HELPER PANEL (BLOCK 0) -->
          <PluginsVisualAiPanel v-model="aiPrompt" @translate="handleAiTranslate" />

          <!-- BLOCK 1: EVENT TRIGGER -->
          <div class="block-card block-trigger">
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
          <div class="arrow-down">
            <DownOutlined />
          </div>

          <!-- BLOCK 2: DYNAMIC CONDITIONS & AND/OR RELATION -->
          <div class="block-card block-condition">
            <div
              class="block-header"
              style="
                display: flex;
                justify-content: space-between;
                align-items: center;
              "
            >
              <div>
                <span class="block-badge" style="background: #fa8c16"
                  >Block 2</span
                >
                <strong style="color: #fff"
                  >高级逻辑过滤条件 (Condition Block)</strong
                >
              </div>
              <!-- LOGICAL RELATION RELATION -->
              <div
                style="
                  background: rgba(255, 255, 255, 0.2);
                  border-radius: 4px;
                  padding: 2px 8px;
                "
              >
                <span
                  style="
                    color: white;
                    font-size: 11px;
                    font-weight: bold;
                    margin-right: 8px;
                  "
                  >条件关系:</span
                >
                <a-radio-group
                  v-model:value="logicRelation"
                  size="small"
                  button-style="solid"
                >
                  <a-radio-button value="AND">且 (AND)</a-radio-button>
                  <a-radio-button value="OR">或 (OR)</a-radio-button>
                </a-radio-group>
              </div>
            </div>
            <div class="block-body">
              <div class="desc-line">
                配置复杂的过滤多维属性判定（支持字符串前缀/后缀/PID/UID）：
              </div>

              <div class="conditions-list">
                <div
                  v-for="(cond, index) in conditions"
                  :key="index"
                  class="condition-row"
                >
                  <a-select v-model:value="cond.field" style="width: 32%">
                    <a-select-option
                      v-for="f in fieldOptions"
                      :key="f.value"
                      :value="f.value"
                      :disabled="
                        (trigger === 'unlink' && f.value === 'basename') ||
                        (trigger !== 'socket_connect' && (f.value === 'port' || f.value === 'ipv4'))
                      "
                    >
                      {{ f.label }}
                    </a-select-option>
                  </a-select>

                  <a-select v-model:value="cond.operator" style="width: 28%">
                    <a-select-option
                      v-for="o in operatorOptions"
                      :key="o.value"
                      :value="o.value"
                      :disabled="
                        (cond.field === 'pid' || cond.field === 'uid' || cond.field === 'port' || cond.field === 'ipv4' || cond.field === 'gid') &&
                        (o.value === 'starts_with' || o.value === 'ends_with')
                      "
                    >
                      {{ o.label }}
                    </a-select-option>
                  </a-select>

                  <a-input
                    v-model:value="cond.value"
                    placeholder="目标匹配值"
                    style="width: 32%"
                  />

                  <a-button
                    danger
                    type="text"
                    @click="removeCondition(index)"
                    :disabled="conditions.length === 1"
                    style="width: 8%"
                  >
                    <template #icon><DeleteOutlined /></template>
                  </a-button>
                </div>
              </div>

              <div
                style="
                  margin-top: 12px;
                  display: flex;
                  justify-content: flex-end;
                "
              >
                <a-button type="dashed" @click="addCondition" size="small">
                  <template #icon><PlusOutlined /></template>
                  添加高级判定分支
                </a-button>
              </div>
            </div>
          </div>

          <!-- CONNECTION ARROW -->
          <div class="arrow-down">
            <DownOutlined />
          </div>

          <!-- BLOCK 2.5: STATEFUL MAP OPERATIONS -->
          <PluginsVisualMapPanel
            v-model:mode="mapMode"
            v-model:key-field="mapKey"
            v-model:limit="mapLimit"
          />

          <!-- CONNECTION ARROW -->
          <div class="arrow-down">
            <DownOutlined />
          </div>

          <!-- BLOCK 3: TARGET ACTION -->
          <div class="block-card block-action">
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

      <!-- Code Preview Column -->
      <a-col :span="11">
        <PluginsVisualCodePanel
          :code="generatedBpfCode"
          :compiling="compiling"
          :compiled="isCompiled"
          :loading="loadingAction"
          :log="compileLogLocal"
          @load="handleLoad"
        />
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.plugins-visual-tab {
  min-height: 600px;
}
.graphical-workspace {
  background: #fafafa;
  border: 1px dashed #d9d9d9;
  border-radius: 8px;
  padding: 16px;
}
.workspace-title {
  margin-bottom: 20px;
  border-left: 4px solid #1890ff;
  padding-left: 10px;
}
.workspace-title h3 {
  margin: 0;
  font-weight: 600;
}
.workspace-title .sub {
  font-size: 12px;
  color: #8c8c8c;
}
.block-card {
  border-radius: 8px;
  overflow: hidden;
  box-shadow: 0 4px 10px rgba(0, 0, 0, 0.05);
  border: 1px solid #f0f0f0;
}
.block-trigger .block-header {
  background: #1890ff;
}
.block-condition .block-header {
  background: #fa8c16;
}
.block-action .block-header {
  background: #52c41a;
}
.block-header {
  padding: 8px 12px;
  display: flex;
  align-items: center;
}
.block-badge {
  background: rgba(0, 0, 0, 0.25);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: white;
  padding: 16px;
}
.desc-line {
  font-size: 13px;
  color: #595959;
  margin-bottom: 10px;
}
.arrow-down {
  text-align: center;
  font-size: 18px;
  color: #bfbfbf;
  margin: 10px 0;
}
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
</style>
