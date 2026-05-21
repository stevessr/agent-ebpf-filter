import { ref, computed } from 'vue';

export interface VisualFilterConfig {
  type: 'process' | 'file' | 'ip' | 'port';
  value: string;
  action: 'BLOCK' | 'ALERT';
  pluginId: string;
  pluginName: string;
  description: string;
}

export function useConfigVisualFilter() {
  const currentConfig = ref<VisualFilterConfig>({
    type: 'process',
    value: 'nc',
    action: 'BLOCK',
    pluginId: 'visual-process-nc-block',
    pluginName: '可视化进程阻断 (nc)',
    description: '通过 eBPF LSM 拦截可执行文件 nc 的运行，返回 EACCES 阻断其执行。',
  });

  // Helper to convert IP address to Hex representation (Host-byte-order)
  const ipToHex = (ipStr: string): string => {
    const parts = ipStr.trim().split('.');
    if (parts.length !== 4) return '0x00000000';
    const num = parts.reduce((acc, part) => {
      const val = parseInt(part, 10);
      if (isNaN(val) || val < 0 || val > 255) return acc;
      return (acc << 8) | val;
    }, 0);
    // Unsigned right shift to convert to unsigned 32-bit int hex
    return '0x' + (num >>> 0).toString(16).padStart(8, '0');
  };

  // Dynamic eBPF C Code Generator
  const generatedCode = computed(() => {
    const type = currentConfig.value.type;
    const value = currentConfig.value.value.trim();
    const action = currentConfig.value.action;
    const returnValLsm = action === 'BLOCK' ? '-EACCES' : '0';
    const returnValCgroup = action === 'BLOCK' ? '0' : '1';
    const logPrefix = action === 'BLOCK' ? 'Blocked' : 'Alert';

    if (type === 'process') {
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define EACCES 13

static __always_inline int strcmp_const(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s1[i] != s2[i]) return 1;
        if (s1[i] == '\\0') return 0;
    }
    return 0;
}

SEC("lsm/bprm_check_security")
int BPF_PROG(visual_process_filter, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    // Check complete path (e.g. /usr/bin/nc)
    const char *filename = BPF_CORE_READ(bprm, filename);
    if (filename) {
        char path_buf[128] = {};
        bpf_probe_read_kernel_str(path_buf, sizeof(path_buf), filename);
        if (strcmp_const(path_buf, "${value}", sizeof(path_buf)) == 0) {
            bpf_printk("[eBPF Filter] ${logPrefix} execution path: %s\\n", path_buf);
            return ${returnValLsm};
        }
    }

    // Check executable basename (e.g. nc)
    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    if (exec_name) {
        char name_buf[64] = {};
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), exec_name);
        if (strcmp_const(name_buf, "${value}", sizeof(name_buf)) == 0) {
            bpf_printk("[eBPF Filter] ${logPrefix} executable filename: %s\\n", name_buf);
            return ${returnValLsm};
        }
    }

    return 0;
}
`;
    }

    if (type === 'file') {
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

#define EACCES 13

static __always_inline int strcmp_const(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s1[i] != s2[i]) return 1;
        if (s1[i] == '\\0') return 0;
    }
    return 0;
}

SEC("lsm/file_open")
int BPF_PROG(visual_file_filter, struct file *file, int ret) {
    if (ret != 0) return ret;

    // Check directory entry basename (e.g. id_rsa, shadow)
    const unsigned char *name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    if (name) {
        char name_buf[64] = {};
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), name);
        if (strcmp_const(name_buf, "${value}", sizeof(name_buf)) == 0) {
            bpf_printk("[eBPF Filter] ${logPrefix} file open basename: %s\\n", name_buf);
            return ${returnValLsm};
        }
    }

    return 0;
}
`;
    }

    if (type === 'ip') {
      const hexIP = ipToHex(value);
      return `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

SEC("cgroup/connect4")
int visual_ip_filter(struct bpf_sock_addr *ctx) {
    if (ctx->family != 2) { // AF_INET
        return 1; // allow
    }

    // Destination IP: convert from network-byte-order to host-byte-order
    u32 dst_ip = bpf_ntohl(ctx->user_ip4);
    
    // Hex representations: ${value}
    if (dst_ip == ${hexIP}) {
        bpf_printk("[eBPF Filter] ${logPrefix} connection to IP: %d\\n", dst_ip);
        return ${returnValCgroup}; // ${action === 'BLOCK' ? 'drop/block' : 'allow'}
    }

    return 1; // allow
}
`;
    }

    if (type === 'port') {
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

    // Destination Port: convert from network-byte-order to host-byte-order
    u32 dst_port = bpf_ntohs(ctx->user_port);
    
    if (dst_port == ${portNum}) {
        bpf_printk("[eBPF Filter] ${logPrefix} connection to port: %d\\n", dst_port);
        return ${returnValCgroup}; // ${action === 'BLOCK' ? 'drop/block' : 'allow'}
    }

    return 1; // allow
}
`;
    }

    return '';
  });

  // Watch config to keep metadata and helper descriptions clean
  const updateMetadata = () => {
    const val = currentConfig.value.value.trim();
    const type = currentConfig.value.type;
    const actionText = currentConfig.value.action === 'BLOCK' ? '阻断' : '告警';
    
    currentConfig.value.pluginId = `visual-${type}-${val.replace(/[^a-z0-9]/g, '-')}-${currentConfig.value.action.toLowerCase()}`;
    currentConfig.value.pluginName = `可视化${type === 'process' ? '进程' : type === 'file' ? '文件' : type === 'ip' ? '网络 IP' : '网络端口'}${actionText}(${val})`;
    
    if (type === 'process') {
      currentConfig.value.description = `通过 eBPF LSM 拦截可执行文件 [${val}] 的运行。在内核决策点返回 EACCES 以拦截执行。`;
    } else if (type === 'file') {
      currentConfig.value.description = `通过 eBPF LSM 拦截对文件或目录 basename [${val}] 的读取和打开操作。`;
    } else if (type === 'ip') {
      currentConfig.value.description = `通过 eBPF cgroupv2 socket 过滤器拦截发往目标 IP 地址 [${val}] 的出站网络连接。`;
    } else if (type === 'port') {
      currentConfig.value.description = `通过 eBPF cgroupv2 socket 过滤器拦截发往目标端口 [${val}] 的出站网络连接。`;
    }
  };

  return {
    currentConfig,
    generatedCode,
    updateMetadata,
  };
}
