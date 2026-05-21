<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { message } from "ant-design-vue";
import {
  PlayCircleOutlined,
  PlusOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  FileTextOutlined,
  DownOutlined,
  AlertOutlined,
  SafetyCertificateOutlined,
  LoadingOutlined,
} from "@ant-design/icons-vue";
import { usePlugins } from "../../composables/usePlugins";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

// Visual block configurations
export interface VisualCondition {
  field: "comm" | "pid" | "basename";
  operator: "==" | "!=";
  value: string;
}

const trigger = ref<"process" | "file_open" | "unlink">("process");
const conditions = ref<VisualCondition[]>([
  { field: "comm", operator: "==", value: "nc" },
]);
const action = ref<"BLOCK" | "ALERT">("BLOCK");

const pluginId = ref("visual-plugin-nc-block");
const pluginName = ref("可视化积木插件(nc-block)");
const description = ref("利用图形化积木拼装自动生成的 eBPF 过滤保护插件。");

const compiling = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");
const isCompiled = ref(false);

const triggerOptions = [
  {
    value: "process",
    label: "进程运行事件 (LSM bprm_check_security)",
    icon: ThunderboltOutlined,
    color: "#1890ff",
  },
  {
    value: "file_open",
    label: "文件打开事件 (LSM file_open)",
    icon: FileTextOutlined,
    color: "#fa8c16",
  },
  {
    value: "unlink",
    label: "文件删除事件 (Kprobe do_unlinkat)",
    icon: AlertOutlined,
    color: "#f5222d",
  },
];

const fieldOptions = [
  { value: "comm", label: "当前进程名称 (Comm)" },
  { value: "pid", label: "当前进程 PID" },
  { value: "basename", label: "操作目标文件名 (Basename)" },
];

const operatorOptions = [
  { value: "==", label: "等于 (==)" },
  { value: "!=", label: "不等于 (!=)" },
];

// Add/Remove condition blocks
const addCondition = () => {
  if (conditions.value.length >= 5) {
    message.warning(
      "为了防止 eBPF Verifier 越界校验失败，图形化条件最多限制为 5 个"
    );
    return;
  }
  conditions.value.push({ field: "comm", operator: "==", value: "" });
};

const removeCondition = (index: number) => {
  conditions.value.splice(index, 1);
};

// Generate BPF Code dynamically based on custom blocks
const generatedBpfCode = computed(() => {
  const isLsmBprm = trigger.value === "process";
  const isLsmFile = trigger.value === "file_open";
  const isKprobeUnlink = trigger.value === "unlink";

  const returnValLsm = action.value === "BLOCK" ? "-EACCES" : "0";
  const logPrefix = action.value === "BLOCK" ? "Blocked" : "Alert";

  let headers = `#include "vmlinux.h"
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
`;

  let body = "";

  if (isLsmBprm) {
    body = `
SEC("lsm/bprm_check_security")
int BPF_PROG(visual_custom_plugin, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    // Check process executable name
    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    char exec_buf[64] = {};
    if (exec_name) {
        bpf_probe_read_kernel_str(exec_buf, sizeof(exec_buf), exec_name);
    }

    u32 matched = 1;
`;
  } else if (isLsmFile) {
    body = `
SEC("lsm/file_open")
int BPF_PROG(visual_custom_plugin, struct file *file, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    char file_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(file_buf, sizeof(file_buf), file_name);
    }

    u32 matched = 1;
`;
  } else if (isKprobeUnlink) {
    body = `
SEC("kprobe/do_unlinkat")
int BPF_PROG(visual_custom_plugin, struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;

    u32 matched = 1;
`;
  }

  // Generate dynamic condition check statements
  let conditionsStatements = "";
  conditions.value.forEach((cond) => {
    const val = cond.value.trim();
    if (!val) return;

    if (cond.field === "comm") {
      if (cond.operator === "==") {
        conditionsStatements += `    if (strcmp_const(comm, "${val}", sizeof(comm)) != 0) matched = 0;\n`;
      } else {
        conditionsStatements += `    if (strcmp_const(comm, "${val}", sizeof(comm)) == 0) matched = 0;\n`;
      }
    } else if (cond.field === "pid") {
      const pidNum = parseInt(val, 10) || 0;
      if (cond.operator === "==") {
        conditionsStatements += `    if (pid != ${pidNum}) matched = 0;\n`;
      } else {
        conditionsStatements += `    if (pid == ${pidNum}) matched = 0;\n`;
      }
    } else if (cond.field === "basename") {
      const targetVar = isLsmFile ? "file_buf" : "exec_buf";
      if (isKprobeUnlink) {
        // Warning kprobes do unlink basename
        conditionsStatements += `    // kprobe trigger has limited filename support; skipping path\n`;
      } else {
        if (cond.operator === "==") {
          conditionsStatements += `    if (strcmp_const(${targetVar}, "${val}", sizeof(${targetVar})) != 0) matched = 0;\n`;
        } else {
          conditionsStatements += `    if (strcmp_const(${targetVar}, "${val}", sizeof(${targetVar})) == 0) matched = 0;\n`;
        }
      }
    }
  });

  body += conditionsStatements;

  // Append outcome block
  if (isKprobeUnlink) {
    body += `
    if (matched) {
        bpf_printk("[Visual Plugin] alert delete event: process %s (pid %d) unlink\\n", comm, pid);
    }
    return 0;
}
`;
  } else {
    body += `
    if (matched) {
        bpf_printk("[Visual Plugin] ${logPrefix} match rule: process %s (pid %d) matched!\\n", comm, pid);
        return ${returnValLsm};
    }
    return 0;
}
`;
  }

  return headers + body;
});

// Auto-sync IDs and names
watch(
  [trigger, conditions, action],
  () => {
    const firstVal = conditions.value[0]?.value || "custom";
    const prefix = `visual-block-${trigger.value}-${firstVal.replace(
      /[^a-z0-9]/g,
      "-"
    )}`.toLowerCase();
    pluginId.value = prefix;
    pluginName.value = `图形化插件(${trigger.value}-${firstVal})`;
    description.value = `由类似 Scratch/图形化编程方块自动转译生成的 eBPF 过滤审计插件。触发挂载: ${trigger.value}，动作: ${action.value}。`;
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { deep: true, immediate: true }
);

// Compile & load custom block plugin
const handleCompileAndRegister = async () => {
  compiling.value = true;
  compileLogLocal.value = "正在将您的流程图积木块转译为 C 源码...\n";
  try {
    // 1. Create/Upsert Manifest
    compileLogLocal.value += `正在将插件 [${pluginId.value}] 保存并注册至本地清单...\n`;
    await upsertPlugin({
      id: pluginId.value,
      name: pluginName.value,
      description: description.value,
      kind: "ebpf",
      enabled: false,
      attachKind: trigger.value === "unlink" ? "kprobe" : "none", // LSM is load-time SEC bound, trigger unlink uses kprobe
      attachTarget: trigger.value === "unlink" ? "do_unlinkat" : "",
      programName: "visual_custom_plugin",
      source: generatedBpfCode.value,
    });

    // 2. Compile BPF
    compileLogLocal.value +=
      "正在调用 Clang BPF 编译器生成 eBPF 字节码 (ELF 格式)...\n";
    const success = await compileBpf(pluginId.value, generatedBpfCode.value);
    if (success) {
      isCompiled.value = true;
      compileLogLocal.value +=
        "\n[SUCCESS] 编译成功！您可以直接点击下方的“载入内核并生效”！";
    } else {
      compileLogLocal.value += "\n[ERROR] 编译失败，请在控制台排查逻辑块。";
    }
  } catch (err: any) {
    compileLogLocal.value += `\n[ERROR] 发生不可期错误: ${err.message}`;
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
      <!-- Graphical Coding Column -->
      <a-col :span="13">
        <div class="graphical-workspace">
          <div class="workspace-title">
            <h3>积木块工作流面板 (Graphical Blocks Builder)</h3>
            <span class="sub"
              >通过流程方块的组合与拼装，在内核级实现极其定制的事件审计过滤。</span
            >
          </div>

          <!-- BLOCK 1: TRIGGER -->
          <div class="block-card block-trigger">
            <div class="block-header">
              <span class="block-badge">Block 1</span>
              <strong style="color: #fff"
                >事件触发积木 (Event Trigger Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">
                选择您要拦截或审计的内核系统调用入口：
              </div>
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

          <!-- CONNECTING ARROW -->
          <div class="arrow-down">
            <DownOutlined />
          </div>

          <!-- BLOCK 2: CONDITIONS -->
          <div class="block-card block-condition">
            <div class="block-header">
              <span class="block-badge" style="background: #fa8c16"
                >Block 2</span
              >
              <strong style="color: #fff"
                >组合过滤条件组 (Condition Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">
                当以下<strong>所有条件同时满足</strong>时，触发响应动作 (逻辑
                AND)：
              </div>

              <div class="conditions-list">
                <div
                  v-for="(cond, index) in conditions"
                  :key="index"
                  class="condition-row"
                >
                  <a-select v-model:value="cond.field" style="width: 35%">
                    <a-select-option
                      v-for="f in fieldOptions"
                      :key="f.value"
                      :value="f.value"
                      :disabled="trigger === 'unlink' && f.value === 'basename'"
                    >
                      {{ f.label }}
                    </a-select-option>
                  </a-select>

                  <a-select v-model:value="cond.operator" style="width: 20%">
                    <a-select-option
                      v-for="o in operatorOptions"
                      :key="o.value"
                      :value="o.value"
                    >
                      {{ o.label }}
                    </a-select-option>
                  </a-select>

                  <a-input
                    v-model:value="cond.value"
                    placeholder="过滤目标值"
                    style="width: 35%"
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
                  添加过滤分支
                </a-button>
              </div>
            </div>
          </div>

          <!-- CONNECTING ARROW -->
          <div class="arrow-down">
            <DownOutlined />
          </div>

          <!-- BLOCK 3: ACTION -->
          <div class="block-card block-action">
            <div class="block-header">
              <span class="block-badge" style="background: #52c41a"
                >Block 3</span
              >
              <strong style="color: #fff"
                >安全防护响应积木 (Action Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">
                一旦过滤积木完全匹配，内核应执行的操作等级：
              </div>
              <a-radio-group
                v-model:value="action"
                button-style="solid"
                :disabled="trigger === 'unlink'"
                style="width: 100%"
              >
                <a-radio-button
                  value="BLOCK"
                  class="block-red"
                  style="width: 50%; text-align: center"
                >
                  <SafetyCertificateOutlined /> BLOCK (硬拦截并向应用报错)
                </a-radio-button>
                <a-radio-button
                  value="ALERT"
                  style="width: 50%; text-align: center"
                >
                  <AlertOutlined /> ALERT (放行并打印内核审计日志)
                </a-radio-button>
              </a-radio-group>
              <div
                v-if="trigger === 'unlink'"
                class="helper-text"
                style="color: #fa8c16; margin-top: 8px"
              >
                * 文件删除挂载于 Kprobe 探针上，默认执行安全审计 ALERT
                告警动作。
              </div>
            </div>
          </div>

          <!-- Persistence config panel -->
          <a-card
            title="规则插件命名与保存 (Plugin Config)"
            size="small"
            style="margin-top: 24px"
          >
            <a-form layout="vertical">
              <a-row :gutter="12">
                <a-col :span="12">
                  <a-form-item label="自定义插件 ID">
                    <a-input
                      v-model:value="pluginId"
                      placeholder="例如 custom-visual-lsm"
                    />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item label="插件展示名称">
                    <a-input v-model:value="pluginName" />
                  </a-form-item>
                </a-col>
              </a-row>
              <a-form-item label="插件简介描述" style="margin-bottom: 0">
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

      <!-- Preview Code Column -->
      <a-col :span="11">
        <a-card
          title="转译后的高性能内核 eBPF 源码 (Transpiled BPF C Code)"
          size="small"
        >
          <template #extra>
            <a-tag color="purple">Pure C / Libbpf</a-tag>
          </template>

          <div class="generated-code-box">
            <pre><code>{{ generatedBpfCode }}</code></pre>
          </div>

          <!-- Compilation Logger -->
          <div
            v-if="compiling || isCompiled || compileLogLocal"
            class="compilation-logger"
            style="margin-top: 16px"
          >
            <div class="logger-header">
              <span>Clang (target bpf) 内置日志台</span>
              <a-tag v-if="compiling" color="blue"
                ><LoadingOutlined /> 正在构建...</a-tag
              >
              <a-tag v-else-if="isCompiled" color="green">SUCCESS</a-tag>
            </div>
            <pre class="logger-body"><code>{{ compileLogLocal }}</code></pre>

            <div
              v-if="isCompiled"
              style="margin-top: 12px; display: flex; justify-content: flex-end"
            >
              <a-button
                type="primary"
                color="green"
                @click="handleLoad"
                :loading="loadingAction"
              >
                <template #icon><PlayCircleOutlined /></template>
                载入内核并立即生效规则
              </a-button>
            </div>
          </div>
        </a-card>
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
.generated-code-box {
  background: #1e1e1e;
  border-radius: 6px;
  padding: 12px;
  overflow: auto;
  max-height: 400px;
  border: 1px solid #333;
}
.generated-code-box pre {
  margin: 0;
}
.generated-code-box code {
  font-family: "Consolas", monospace;
  font-size: 12px;
  color: #9cdcfe;
}
.helper-text {
  font-size: 11px;
}
.compilation-logger {
  background: #141414;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  overflow: hidden;
}
.logger-header {
  background: #262626;
  padding: 6px 12px;
  color: #fafafa;
  font-size: 13px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.logger-body {
  margin: 0;
  padding: 12px;
  max-height: 180px;
  overflow: auto;
  color: #52c41a;
  background: #000;
  font-family: "Consolas", monospace;
  font-size: 12px;
  white-space: pre-wrap;
}
.block-red.ant-radio-button-wrapper-checked {
  background: #f5222d;
  border-color: #f5222d;
  color: white;
}
</style>
