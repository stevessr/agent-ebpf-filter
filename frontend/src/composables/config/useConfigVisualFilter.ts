import { ref, computed } from "vue";

export interface VisualFilterConfig {
  type:
    | "process"
    | "file"
    | "ip"
    | "port"
    | "mkdir"
    | "file_create"
    | "rmdir"
    | "symlink";
  value: string;
  operator: "==" | "!=" | "starts_with" | "ends_with";
  action: "BLOCK" | "ALERT";
  pluginId: string;
  pluginName: string;
  description: string;
}

export function useConfigVisualFilter() {
  const currentConfig = ref<VisualFilterConfig>({
    type: "process",
    value: "nc",
    operator: "==",
    action: "BLOCK",
    pluginId: "visual-process-nc-block",
    pluginName: "可视化进程阻断 (nc)",
    description:
      "通过 eBPF LSM 拦截可执行文件 nc 的运行，返回 EACCES 阻断其执行。",
  });

  // Helper to convert IP address or subnet to Hex mask representation (Host-byte-order)
  const parseCIDR = (val: string) => {
    const parts = val.trim().split("/");
    const ip = parts[0];
    const cidr = parts[1] ? parseInt(parts[1], 10) : 32;

    const ipParts = ip.split(".");
    if (ipParts.length !== 4)
      return { subnetHex: "0x00000000", maskHex: "0xffffffff" };

    const ipNum = ipParts.reduce((acc, part) => {
      const v = parseInt(part, 10);
      if (isNaN(v) || v < 0 || v > 255) return acc;
      return (acc << 8) | v;
    }, 0);

    const mask = cidr === 0 ? 0 : (0xffffffff << (32 - cidr)) >>> 0;
    return {
      subnetHex: "0x" + (ipNum >>> 0).toString(16).padStart(8, "0"),
      maskHex: "0x" + mask.toString(16).padStart(8, "0"),
    };
  };

  // Helper code definitions to inject
  const bpfHelpers = `
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

  // Dynamic eBPF C Code Generator with Advanced matching support
  const generatedCode = computed(() => {
    const type = currentConfig.value.type;
    const value = currentConfig.value.value.trim();
    const operator = currentConfig.value.operator;
    const action = currentConfig.value.action;
    const returnValLsm = action === "BLOCK" ? "-EACCES" : "0";
    const returnValCgroup = action === "BLOCK" ? "0" : "1";
    const logPrefix = action === "BLOCK" ? "Blocked" : "Alert";

    let condition = "";
    if (operator === "==") {
      condition = `strcmp_const(name_buf, "${value}", sizeof(name_buf)) == 0`;
    } else if (operator === "!=") {
      condition = `strcmp_const(name_buf, "${value}", sizeof(name_buf)) != 0`;
    } else if (operator === "starts_with") {
      condition = `str_starts_with(name_buf, "${value}", sizeof(name_buf)) != 0`;
    } else if (operator === "ends_with") {
      condition = `str_ends_with(name_buf, get_str_len(name_buf, sizeof(name_buf)), "${value}", ${value.length}) != 0`;
    }

    if (type === "process") {
      let pathCondition = "";
      if (operator === "==") {
        pathCondition = `strcmp_const(path_buf, "${value}", sizeof(path_buf)) == 0`;
      } else if (operator === "!=") {
        pathCondition = `strcmp_const(path_buf, "${value}", sizeof(path_buf)) != 0`;
      } else if (operator === "starts_with") {
        pathCondition = `str_starts_with(path_buf, "${value}", sizeof(path_buf)) != 0`;
      } else if (operator === "ends_with") {
        pathCondition = `str_ends_with(path_buf, get_str_len(path_buf, sizeof(path_buf)), "${value}", ${value.length}) != 0`;
      }

      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";
#define EACCES 13
${bpfHelpers}

SEC("lsm/bprm_check_security")
int BPF_PROG(visual_process_filter, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    // Check complete path
    const char *filename = BPF_CORE_READ(bprm, filename);
    if (filename) {
        char path_buf[128] = {};
        bpf_probe_read_kernel_str(path_buf, sizeof(path_buf), filename);
        if (${pathCondition}) {
            bpf_printk("[eBPF Filter] ${logPrefix} execution path: %s\\n", path_buf);
            return ${returnValLsm};
        }
    }

    // Check executable basename
    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    if (exec_name) {
        char name_buf[64] = {};
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), exec_name);
        if (${condition}) {
            bpf_printk("[eBPF Filter] ${logPrefix} executable filename: %s\\n", name_buf);
            return ${returnValLsm};
        }
    }

    return 0;
}
`;
    }

    if (type === "file") {
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";
#define EACCES 13
${bpfHelpers}

SEC("lsm/file_open")
int BPF_PROG(visual_file_filter, struct file *file, int ret) {
    if (ret != 0) return ret;

    const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    if (name) {
        char name_buf[64] = {};
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), name);
        if (${condition}) {
            bpf_printk("[eBPF Filter] ${logPrefix} file open basename: %s\\n", name_buf);
            return ${returnValLsm};
        }
    }

    return 0;
}
`;
    }

    // New LSM hooks for directory/file creations & deletions
    if (
      type === "mkdir" ||
      type === "file_create" ||
      type === "rmdir" ||
      type === "symlink"
    ) {
      const secName =
        type === "mkdir"
          ? "lsm/inode_mkdir"
          : type === "file_create"
            ? "lsm/inode_create"
            : type === "rmdir"
              ? "lsm/inode_rmdir"
              : "lsm/inode_symlink";

      const funcArgs =
        type === "mkdir"
          ? "struct inode *dir, struct dentry *dentry, umode_t mode"
          : type === "file_create"
            ? "struct inode *dir, struct dentry *dentry, umode_t mode"
            : type === "rmdir"
              ? "struct inode *dir, struct dentry *dentry"
              : "struct inode *dir, struct dentry *dentry, const char *old_name";

      const typeDesc =
        type === "mkdir"
          ? "folder creation"
          : type === "file_create"
            ? "file creation"
            : type === "rmdir"
              ? "folder deletion"
              : "symlink creation";

      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";
#define EACCES 13
${bpfHelpers}

SEC("${secName}")
int BPF_PROG(visual_lsm_${type}, ${funcArgs}) {
    const unsigned char *name = BPF_CORE_READ(dentry, d_name.name);
    if (name) {
        char name_buf[64] = {};
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), name);
        if (${condition}) {
            bpf_printk("[eBPF Filter] ${logPrefix} ${typeDesc}: %s\\n", name_buf);
            return ${returnValLsm};
        }
    }

    return 0;
}
`;
    }

    if (type === "ip") {
      const { subnetHex, maskHex } = parseCIDR(value);
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

SEC("cgroup/connect4")
int visual_ip_filter(struct bpf_sock_addr *ctx) {
    if (ctx->family != 2) { // AF_INET
        return 1; // allow
    }

    u32 dst_ip = bpf_ntohl(ctx->user_ip4);
    
    // Subnet CIDR match: ${value}
    if ((dst_ip & ${maskHex}) == (${subnetHex} & ${maskHex})) {
        bpf_printk("[eBPF Filter] ${logPrefix} connection to IP/CIDR: %d\\n", dst_ip);
        return ${returnValCgroup};
    }

    return 1; // allow
}
`;
    }

    if (type === "port") {
      const portNum = parseInt(value, 10) || 0;
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

SEC("cgroup/connect4")
int visual_port_filter(struct bpf_sock_addr *ctx) {
    if (ctx->family != 2) { // AF_INET
        return 1; // allow
    }

    u32 dst_port = bpf_ntohs(ctx->user_port);
    
    if (dst_port == ${portNum}) {
        bpf_printk("[eBPF Filter] ${logPrefix} connection to port: %d\\n", dst_port);
        return ${returnValCgroup};
    }

    return 1; // allow
}
`;
    }

    return "";
  });

  const updateMetadata = () => {
    const val = currentConfig.value.value.trim();
    const type = currentConfig.value.type;
    const actionText = currentConfig.value.action === "BLOCK" ? "阻断" : "告警";

    currentConfig.value.pluginId = `visual-${type}-${val.replace(
      /[^a-z0-9]/g,
      "-",
    )}-${currentConfig.value.action.toLowerCase()}`;
    currentConfig.value.pluginName = `可视化${
      type === "process"
        ? "进程"
        : type === "file"
          ? "文件打开"
          : type === "mkdir"
            ? "新建目录"
            : type === "file_create"
              ? "新建文件"
              : type === "rmdir"
                ? "删除目录"
                : type === "symlink"
                  ? "符号链接"
                  : type === "ip"
                    ? "网络 IP"
                    : "网络端口"
    }${actionText}(${val})`;

    if (type === "process") {
      currentConfig.value.description = `通过 eBPF LSM 拦截可执行文件 [${val}] 的运行。在内核决策点返回 EACCES 以拦截执行。`;
    } else if (type === "file") {
      currentConfig.value.description = `通过 eBPF LSM 拦截对文件或目录 [${val}] 的打开和读取操作。`;
    } else if (type === "mkdir") {
      currentConfig.value.description = `通过 eBPF LSM 拦截创建名为 [${val}] 的新文件夹。`;
    } else if (type === "file_create") {
      currentConfig.value.description = `通过 eBPF LSM 拦截创建名为 [${val}] 的新物理文件。`;
    } else if (type === "rmdir") {
      currentConfig.value.description = `通过 eBPF LSM 拦截删除名为 [${val}] 的现有文件夹。`;
    } else if (type === "symlink") {
      currentConfig.value.description = `通过 eBPF LSM 拦截创建名为 [${val}] 的符号软链接。`;
    } else if (type === "ip") {
      currentConfig.value.description = `通过 eBPF cgroupv2 socket 过滤器拦截发往目标 IP 网段 [${val}] 的出站网络连接。`;
    } else if (type === "port") {
      currentConfig.value.description = `通过 eBPF cgroupv2 socket 过滤器拦截发往目标端口 [${val}] 的出站网络连接。`;
    }
  };

  return {
    currentConfig,
    generatedCode,
    updateMetadata,
  };
}
