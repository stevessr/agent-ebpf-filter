<script setup lang="ts">
import { ref, watch } from "vue";
import { message, Modal } from "ant-design-vue";
import {
  CodeOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  FireOutlined,
  LoadingOutlined,
} from "@ant-design/icons-vue";

const props = defineProps<{
  currentConfig: any;
  generatedCode: string;
  security: any;
  updateMetadata: () => void;
  compileBpf: (id: string, code: string) => Promise<boolean>;
  loadBpf: (id: string) => Promise<void>;
  fetchPlugins: () => Promise<void>;
}>();

const compiling = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");
const isCompiled = ref(false);

const typeOptions = [
  {
    value: "process",
    label: "进程运行拦截 (LSM bprm)",
    icon: ThunderboltOutlined,
  },
  {
    value: "file",
    label: "文件读取打开 (LSM file_open)",
    icon: SafetyCertificateOutlined,
  },
  {
    value: "file_create",
    label: "新建物理文件 (LSM inode_create)",
    icon: SafetyCertificateOutlined,
  },
  {
    value: "mkdir",
    label: "创建新文件夹 (LSM inode_mkdir)",
    icon: SafetyCertificateOutlined,
  },
  {
    value: "rmdir",
    label: "删除现有文件夹 (LSM inode_rmdir)",
    icon: SafetyCertificateOutlined,
  },
  {
    value: "symlink",
    label: "创建软链接 (LSM inode_symlink)",
    icon: SafetyCertificateOutlined,
  },
  {
    value: "ip",
    label: "网络 IPv4 目标 (核心精确 IP / 插件 CIDR)",
    icon: SafetyCertificateOutlined,
  },
  { value: "port", label: "网络出站端口 (cgroup connect)", icon: CodeOutlined },
];

const operatorOptions = [
  { value: "==", label: "等于 (==)" },
  { value: "!=", label: "不等于 (!=)" },
  { value: "starts_with", label: "前缀匹配 (starts_with)" },
  { value: "ends_with", label: "后缀匹配 (ends_with)" },
];

watch(
  [
    () => props.currentConfig.value?.type,
    () => props.currentConfig.value?.value,
    () => props.currentConfig.value?.operator,
    () => props.currentConfig.value?.action,
  ],
  () => {
    props.updateMetadata();
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { immediate: true },
);

const handleApplyCoreRule = async () => {
  const { type, value, action } = props.currentConfig.value;
  if (!value.trim()) {
    message.warning("请输入拦截规则的目标值");
    return;
  }

  if (action === "ALERT") {
    Modal.confirm({
      title: "仅告警模式限制",
      content:
        '内核内置防御引擎在执行核心规则时默认执行硬阻断（BLOCK）。如果您需要为该条件配置纯告警（ALERT）逻辑而不执行阻断拦截，请在右侧点击"生成并编译为 BPF 插件"并载入运行。',
      okText: "直接应用为阻断",
      cancelText: "取消",
      onOk: () => {
        props.currentConfig.value.action = "BLOCK";
        handleApplyCoreRule();
      },
    });
    return;
  }

  loadingAction.value = true;
  try {
    if (type === "process") {
      if (value.startsWith("/")) {
        props.security.lsmExecPath.value = value;
        await props.security.blockLsmExecPath();
      } else {
        props.security.lsmExecName.value = value;
        await props.security.blockLsmExecName();
      }
    } else if (
      ["file", "file_create", "mkdir", "rmdir", "symlink"].includes(type)
    ) {
      props.security.lsmFileName.value = value;
      await props.security.blockLsmFileName();
    } else if (type === "ip") {
      props.security.cgroupTargetIP.value = value;
      await props.security.blockCgroupIP();
    } else if (type === "port") {
      props.security.cgroupTargetPort.value = parseInt(value, 10);
      await props.security.blockCgroupPort();
    }
  } catch (err: any) {
    message.error("规则应用失败：" + (err.message || "内核拒绝写入"));
  } finally {
    loadingAction.value = false;
  }
};

const handleCompilePlugin = async () => {
  compiling.value = true;
  compileLogLocal.value = "正在将高级匹配参数转译为 eBPF C 语言逻辑代码...\n";
  try {
    compileLogLocal.value += `正在注册自定义插件 [${props.currentConfig.value.pluginId}] 至本地 Manifest 仓库...\n`;
    // upsertPlugin is handled by the parent
    compileLogLocal.value +=
      "正在调用 Clang 18 (bpf-target) 将 C 源码编译为 ELF 内核字节码...\n";
    const success = await props.compileBpf(
      props.currentConfig.value.pluginId,
      props.generatedCode,
    );
    if (success) {
      isCompiled.value = true;
      compileLogLocal.value +=
        "\n[SUCCESS] 编译成功！已生成 program.o。点击下方按钮即可载入内核运行。";
    } else {
      compileLogLocal.value +=
        "\n[ERROR] 编译失败，请排查过滤参数是否包含不合法字符。";
    }
  } catch (err: any) {
    compileLogLocal.value += "\n[ERROR] 错误：" + err.message;
  } finally {
    compiling.value = false;
  }
};

const handleLoadPlugin = async () => {
  loadingAction.value = true;
  try {
    await props.loadBpf(props.currentConfig.value.pluginId);
    await props.fetchPlugins();
  } finally {
    loadingAction.value = false;
  }
};
</script>

<template>
  <a-row :gutter="24">
    <!-- Left: Interactive Form Designer -->
    <a-col :span="12">
      <a-card title="可视化主动防御拦截器 (Visual Rule Designer)" size="small">
        <template #extra>
          <span style="color: #52c41a; font-weight: 500">
            <FireOutlined /> 内核 IPS 主动防御
          </span>
        </template>

        <a-form layout="vertical">
          <a-form-item
            label="1. 选择防御拦截入口 (Select Intercept Event Trigger)"
          >
            <a-select v-model:value="currentConfig.type" style="width: 100%">
              <a-select-option
                v-for="opt in typeOptions"
                :key="opt.value"
                :value="opt.value"
              >
                <component :is="opt.icon" style="color: #1677ff" />
                <span style="margin-left: 8px">{{ opt.label }}</span>
              </a-select-option>
            </a-select>
          </a-form-item>

          <a-form-item
            v-if="currentConfig.type !== 'ip' && currentConfig.type !== 'port'"
            label="2. 选择过滤匹配算子 (Select Match Operator)"
          >
            <a-select
              v-model:value="currentConfig.operator"
              :options="operatorOptions"
              style="width: 100%"
            />
          </a-form-item>

          <a-form-item label="3. 配置过滤匹配值 (Parameters)">
            <div v-if="currentConfig.type === 'process'">
              <a-input
                v-model:value="currentConfig.value"
                placeholder="例如: nc 或 /usr/bin/nc (支持 basename 模糊或完整路径)"
              />
              <span class="helper-text"
                >前缀/后缀匹配可对可执行程序的特定存放路径或类型实施集中安全加固。</span
              >
            </div>
            <div
              v-else-if="
                ['file', 'file_create', 'mkdir', 'rmdir', 'symlink'].includes(
                  currentConfig.type,
                )
              "
            >
              <a-input
                v-model:value="currentConfig.value"
                placeholder="例如: id_rsa 或 shadow"
              />
              <span class="helper-text"
                >此处的过滤条件将精准注入内核
                LSM，当触发相匹配的文件打开、文件创建、软链接指引、目录删除等事件时，触发强阻断决策。</span
              >
            </div>
            <div v-else-if="currentConfig.type === 'ip'">
              <a-input
                v-model:value="currentConfig.value"
                placeholder="例如: 8.8.8.8；自定义插件可输入 10.0.0.0/8"
              />
              <span class="helper-text"
                >快速核心规则写入后端现有精确 IP
                map；CIDR/掩码逻辑只在"生成并编译为 BPF
                插件"路径中生成独立过滤代码。</span
              >
            </div>
            <div v-else-if="currentConfig.type === 'port'">
              <a-input-number
                v-model:value="currentConfig.value"
                style="width: 100%"
                :min="1"
                :max="65535"
                placeholder="例如: 4444"
              />
              <span class="helper-text"
                >直接丢弃发往特定非法端口的数据流量。</span
              >
            </div>
          </a-form-item>

          <a-form-item label="4. 响应执行级别 (Interception Severity)">
            <a-radio-group
              v-model:value="currentConfig.action"
              button-style="solid"
            >
              <a-radio-button value="BLOCK" class="action-block-btn"
                >BLOCK (硬阻断拦截)</a-radio-button
              >
              <a-radio-button value="ALERT"
                >ALERT (仅告警不拦截)</a-radio-button
              >
            </a-radio-group>
          </a-form-item>

          <a-form-item label="自动生成的规则插件元数据">
            <div class="metadata-preview">
              <div>
                <strong>规则 ID:</strong>
                <code>{{ currentConfig.pluginId }}</code>
              </div>
              <div>
                <strong>规则名：</strong>
                <span>{{ currentConfig.pluginName }}</span>
              </div>
              <div>
                <strong>简介：</strong>
                <span style="font-size: 12px; color: #666">{{
                  currentConfig.description
                }}</span>
              </div>
            </div>
          </a-form-item>

          <div style="margin-top: 24px">
            <a-space style="width: 100%; justify-content: flex-end">
              <a-button
                type="primary"
                danger
                :loading="loadingAction"
                @click="handleApplyCoreRule"
              >
                <template #icon><SafetyCertificateOutlined /></template>
                应用到内核沙箱规则 (Core Sandbox)
              </a-button>
              <a-button
                type="default"
                :loading="compiling"
                @click="handleCompilePlugin"
              >
                <template #icon><CodeOutlined /></template>
                生成并编译为 BPF 插件
              </a-button>
            </a-space>
          </div>
        </a-form>
      </a-card>

      <!-- Readiness Descriptions -->
      <a-card
        title="内核防御引擎就绪状态"
        size="small"
        style="margin-top: 20px"
      >
        <a-descriptions bordered :column="1" size="small">
          <a-descriptions-item label="LSM 主动防御拦截器">
            <a-tag
              :color="
                security.lsmEnforcerStatus.value.available &&
                security.lsmEnforcerStatus.value.attached
                  ? 'green'
                  : 'red'
              "
            >
              {{
                security.lsmEnforcerStatus.value.available &&
                security.lsmEnforcerStatus.value.attached
                  ? "LSM 拦截已启用 (Active)"
                  : "未挂载"
              }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="cgroup2 网络拦截器">
            <a-tag
              :color="
                security.cgroupSandboxStatus.value.available &&
                security.cgroupSandboxStatus.value.attached
                  ? 'green'
                  : 'red'
              "
            >
              {{
                security.cgroupSandboxStatus.value.available &&
                security.cgroupSandboxStatus.value.attached
                  ? "cgroup 网络拦截已启用 (Active)"
                  : "未挂载"
              }}
            </a-tag>
          </a-descriptions-item>
        </a-descriptions>
      </a-card>
    </a-col>

    <!-- Right: Generated Code Preview -->
    <a-col :span="12">
      <a-card title="动态生成的 eBPF C 语言高阶过滤器源码" size="small">
        <template #extra>
          <a-tag color="purple">eBPF Bytecode</a-tag>
        </template>
        <div class="code-editor-preview">
          <pre><code>{{ generatedCode }}</code></pre>
        </div>

        <div
          v-if="isCompiled || compiling || compileLogLocal"
          class="compilation-logger"
          style="margin-top: 16px"
        >
          <div class="logger-header">
            <span>Clang LLVM 编译输出控制台</span>
            <a-tag v-if="compiling" color="blue"
              ><LoadingOutlined /> Compiling...</a-tag
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
              @click="handleLoadPlugin"
              :loading="loadingAction"
            >
              <template #icon><PlayCircleOutlined /></template>
              立即装载此 eBPF 过滤器至内核
            </a-button>
          </div>
        </div>
      </a-card>
    </a-col>
  </a-row>
</template>

<style scoped>
.helper-text {
  font-size: 12px;
  color: #8c8c8c;
  margin-top: 4px;
  display: block;
}
.metadata-preview {
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 13px;
  border-left: 3px solid #1677ff;
}
.metadata-preview code {
  color: #c41d7f;
}
.code-editor-preview {
  background: #1e1e1e;
  padding: 12px;
  border-radius: 6px;
  overflow: auto;
  max-height: 380px;
  border: 1px solid #333;
}
.code-editor-preview pre {
  margin: 0;
}
.code-editor-preview code {
  color: #9cdcfe;
  font-family: "Consolas", monospace;
  font-size: 12px;
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
  max-height: 200px;
  overflow: auto;
  color: #52c41a;
  background: #000;
  font-family: "Consolas", monospace;
  font-size: 12px;
  white-space: pre-wrap;
}
.action-block-btn {
  border-color: #ff4d4f;
  color: #ff4d4f;
}
.action-block-btn.ant-radio-button-wrapper-checked {
  background: #ff4d4f;
  color: white;
}
</style>
