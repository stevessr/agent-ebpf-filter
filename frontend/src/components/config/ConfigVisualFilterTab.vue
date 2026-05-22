<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { message, Modal } from "ant-design-vue";
import {
  CodeOutlined,
  PlayCircleOutlined,
  StopOutlined,
  SafetyCertificateOutlined,
  GlobalOutlined,
  ThunderboltOutlined,
  DeleteOutlined,
  InfoCircleOutlined,
  LoadingOutlined,
  FireOutlined,
  FolderAddOutlined,
  FileAddOutlined,
  LinkOutlined,
} from "@ant-design/icons-vue";
import { useConfigVisualFilter } from "../../composables/useConfigVisualFilter";
import { usePlugins } from "../../composables/usePlugins";

const props = defineProps<{
  security: any; // Passed from Config.vue
}>();

const { currentConfig, generatedCode, updateMetadata } =
  useConfigVisualFilter();
const { compileBpf, loadBpf, unloadBpf, upsertPlugin, fetchPlugins, plugins } =
  usePlugins();

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
    icon: FileAddOutlined,
  },
  {
    value: "mkdir",
    label: "创建新文件夹 (LSM inode_mkdir)",
    icon: FolderAddOutlined,
  },
  {
    value: "rmdir",
    label: "删除现有文件夹 (LSM inode_rmdir)",
    icon: DeleteOutlined,
  },
  {
    value: "symlink",
    label: "创建软链接 (LSM inode_symlink)",
    icon: LinkOutlined,
  },
  { value: "ip", label: "网络 IP/CIDR (cgroup connect)", icon: GlobalOutlined },
  { value: "port", label: "网络出站端口 (cgroup connect)", icon: CodeOutlined },
];

const operatorOptions = [
  { value: "==", label: "等于 (==)" },
  { value: "!=", label: "不等于 (!=)" },
  { value: "starts_with", label: "前缀匹配 (starts_with)" },
  { value: "ends_with", label: "后缀匹配 (ends_with)" },
];

// Auto update metadata on state changes
watch(
  [
    () => currentConfig.value.type,
    () => currentConfig.value.value,
    () => currentConfig.value.operator,
    () => currentConfig.value.action,
  ],
  () => {
    updateMetadata();
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { immediate: true }
);

onMounted(async () => {
  await fetchPlugins();
});

// Apply directly to core BPF enforcers
const handleApplyCoreRule = async () => {
  const { type, value, action } = currentConfig.value;
  if (!value.trim()) {
    message.warning("请输入拦截规则的目标值");
    return;
  }

  if (action === "ALERT") {
    Modal.confirm({
      title: "仅告警模式限制",
      content:
        "内核内置防御引擎在执行核心规则时默认执行硬阻断（BLOCK）。如果您需要为该条件配置纯告警（ALERT）逻辑而不执行阻断拦截，请在右侧点击“生成并编译为 BPF 插件”并载入运行。",
      okText: "直接应用为阻断",
      cancelText: "取消",
      onOk: () => {
        currentConfig.value.action = "BLOCK";
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
      type === "file" ||
      type === "file_create" ||
      type === "mkdir" ||
      type === "rmdir" ||
      type === "symlink"
    ) {
      // In core LSM enforcer, all file/directory attributes use lsm_blocked_file_names map!
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

// Compile and Register Custom Visual BPF Plugin
const handleCompilePlugin = async () => {
  compiling.value = true;
  compileLogLocal.value = "正在将高级匹配参数转译为 eBPF C 语言逻辑代码...\n";
  try {
    compileLogLocal.value += `正在注册自定义插件 [${currentConfig.value.pluginId}] 至本地 Manifest 仓库...\n`;
    await upsertPlugin({
      id: currentConfig.value.pluginId,
      name: currentConfig.value.pluginName,
      description: currentConfig.value.description,
      kind: "ebpf",
      enabled: false,
      attachKind: currentConfig.value.type === "process" ? "none" : "none", // LSM hooks load dynamically
      attachTarget: "",
      programName:
        currentConfig.value.type === "process"
          ? "visual_process_filter"
          : `visual_lsm_${currentConfig.value.type}`,
      source: generatedCode.value,
    });

    compileLogLocal.value +=
      "正在调用 Clang 18 (bpf-target) 将 C 源码编译为 ELF 内核字节码...\n";
    const success = await compileBpf(
      currentConfig.value.pluginId,
      generatedCode.value
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
    await loadBpf(currentConfig.value.pluginId);
    await fetchPlugins();
  } finally {
    loadingAction.value = false;
  }
};

const handleUnloadPlugin = async (id: string) => {
  loadingAction.value = true;
  try {
    await unloadBpf(id);
    await fetchPlugins();
  } finally {
    loadingAction.value = false;
  }
};

// Direct rule unblocking helpers
const handleRemoveLsmPath = async (path: string) => {
  props.security.lsmExecPath.value = path;
  await props.security.unblockLsmExecPath();
};

const handleRemoveLsmName = async (name: string) => {
  props.security.lsmExecName.value = name;
  await props.security.unblockLsmExecName();
};

const handleRemoveLsmFile = async (name: string) => {
  props.security.lsmFileName.value = name;
  await props.security.unblockLsmFileName();
};

const handleRemoveCgroupIP = async (ip: string) => {
  props.security.cgroupTargetIP.value = ip;
  await props.security.unblockCgroupIP();
};

const handleRemoveCgroupPort = async (port: number) => {
  props.security.cgroupTargetPort.value = port;
  await props.security.unblockCgroupPort();
};
</script>

<template>
  <div class="visual-filter-tab">
    <a-row :gutter="24">
      <!-- Left: Interactive Form Designer -->
      <a-col :span="12">
        <a-card
          title="可视化主动防御拦截器 (Visual Rule Designer)"
          size="small"
        >
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
              v-if="
                currentConfig.type !== 'ip' && currentConfig.type !== 'port'
              "
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
                  currentConfig.type === 'file' ||
                  currentConfig.type === 'file_create' ||
                  currentConfig.type === 'mkdir' ||
                  currentConfig.type === 'rmdir' ||
                  currentConfig.type === 'symlink'
                "
              >
                <a-input
                  v-model:value="currentConfig.value"
                  placeholder="例如: id_rsa 或 shadow"
                />
                <span class="helper-text">
                  此处的过滤条件将精准注入内核
                  LSM，当触发相匹配的文件打开、文件创建、软链接指引、目录删除等事件时，触发强阻断决策。
                </span>
              </div>
              <div v-else-if="currentConfig.type === 'ip'">
                <a-input
                  v-model:value="currentConfig.value"
                  placeholder="例如: 8.8.8.8 或 10.0.0.0/8 (支持 IP 网段掩码匹配)"
                />
                <span class="helper-text"
                  >在连接建立之初，拦截出站。支持子网掩码校验拦截。</span
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
                <a-radio-button value="BLOCK" class="action-block-btn">
                  BLOCK (硬阻断拦截)
                </a-radio-button>
                <a-radio-button value="ALERT">
                  ALERT (仅告警不拦截)
                </a-radio-button>
              </a-radio-group>
            </a-form-item>

            <a-form-item label="自动生成的规则插件元数据">
              <div class="metadata-preview">
                <div>
                  <strong>规则 ID:</strong>
                  <code>{{ currentConfig.pluginId }}</code>
                </div>
                <div>
                  <strong>规则名:</strong>
                  <span>{{ currentConfig.pluginName }}</span>
                </div>
                <div>
                  <strong>简介:</strong>
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
                  props.security.lsmEnforcerStatus.value.available &&
                  props.security.lsmEnforcerStatus.value.attached
                    ? 'green'
                    : 'red'
                "
              >
                {{
                  props.security.lsmEnforcerStatus.value.available &&
                  props.security.lsmEnforcerStatus.value.attached
                    ? "LSM 拦截已启用 (Active)"
                    : "未挂载"
                }}
              </a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="cgroup2 网络拦截器">
              <a-tag
                :color="
                  props.security.cgroupSandboxStatus.value.available &&
                  props.security.cgroupSandboxStatus.value.attached
                    ? 'green'
                    : 'red'
                "
              >
                {{
                  props.security.cgroupSandboxStatus.value.available &&
                  props.security.cgroupSandboxStatus.value.attached
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

    <!-- Monitoring Board -->
    <a-card
      title="活跃内核拦截状态与自定义插件监控"
      size="small"
      style="margin-top: 24px"
    >
      <a-tabs default-active-key="core-rules" size="small">
        <a-tab-pane key="core-rules" tab="内核核心阻断列表 (Core Lists)">
          <div class="monitor-stats">
            <a-row :gutter="16" style="margin-bottom: 16px">
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="LSM 检查总数"
                    :value="
                      props.security.lsmEnforcerStatus.value.stats.execChecked +
                      props.security.lsmEnforcerStatus.value.stats.fileChecked
                    "
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="LSM 执行阻断数"
                    :value="
                      props.security.lsmEnforcerStatus.value.stats.execBlocked
                    "
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="LSM 文件阻断数"
                    :value="
                      props.security.lsmEnforcerStatus.value.stats.fileBlocked
                    "
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="cgroup2 出站拦截"
                    :value="
                      props.security.cgroupSandboxStatus.value.stats.blocked
                    "
                  />
                </a-card>
              </a-col>
            </a-row>
          </div>

          <div style="margin-top: 12px">
            <a-list bordered size="small" header="当前激活的拦截项">
              <!-- LSM Paths -->
              <a-list-item
                v-for="path in props.security.lsmEnforcerStatus.value
                  .blockedExecPaths"
                :key="`path-${path}`"
              >
                <div class="rule-item">
                  <a-tag color="red">LSM 路径执行拦截</a-tag>
                  <code>{{ path }}</code>
                </div>
                <template #actions>
                  <a-button
                    size="small"
                    danger
                    type="text"
                    @click="handleRemoveLsmPath(path)"
                  >
                    <template #icon><DeleteOutlined /></template>解封
                  </a-button>
                </template>
              </a-list-item>

              <!-- LSM Names -->
              <a-list-item
                v-for="name in props.security.lsmEnforcerStatus.value
                  .blockedExecNames"
                :key="`name-${name}`"
              >
                <div class="rule-item">
                  <a-tag color="volcano">LSM Basename 执行拦截</a-tag>
                  <code>{{ name }}</code>
                </div>
                <template #actions>
                  <a-button
                    size="small"
                    danger
                    type="text"
                    @click="handleRemoveLsmName(name)"
                  >
                    <template #icon><DeleteOutlined /></template>解封
                  </a-button>
                </template>
              </a-list-item>

              <!-- LSM Files/Dirs -->
              <a-list-item
                v-for="file in props.security.lsmEnforcerStatus.value
                  .blockedFileNames"
                :key="`file-${file}`"
              >
                <div class="rule-item">
                  <a-tag color="orange"
                    >LSM 属性阻断 (创建/打开/软链/删除)</a-tag
                  >
                  <code>{{ file }}</code>
                </div>
                <template #actions>
                  <a-button
                    size="small"
                    danger
                    type="text"
                    @click="handleRemoveLsmFile(file)"
                  >
                    <template #icon><DeleteOutlined /></template>解封
                  </a-button>
                </template>
              </a-list-item>

              <!-- cgroup IPs -->
              <a-list-item
                v-for="ip in props.security.cgroupSandboxStatus.value
                  .blockedIPs"
                :key="`ip-${ip}`"
              >
                <div class="rule-item">
                  <a-tag color="blue">网络 IP/网段 拦截</a-tag>
                  <code>{{ ip }}</code>
                </div>
                <template #actions>
                  <a-button
                    size="small"
                    danger
                    type="text"
                    @click="handleRemoveCgroupIP(ip)"
                  >
                    <template #icon><DeleteOutlined /></template>解封
                  </a-button>
                </template>
              </a-list-item>

              <!-- cgroup Ports -->
              <a-list-item
                v-for="port in props.security.cgroupSandboxStatus.value
                  .blockedPorts"
                :key="`port-${port}`"
              >
                <div class="rule-item">
                  <a-tag color="purple">目的端口拦截</a-tag>
                  <code>dst_port == {{ port }}</code>
                </div>
                <template #actions>
                  <a-button
                    size="small"
                    danger
                    type="text"
                    @click="handleRemoveCgroupPort(port)"
                  >
                    <template #icon><DeleteOutlined /></template>解封
                  </a-button>
                </template>
              </a-list-item>

              <!-- Empty -->
              <div
                v-if="
                  !props.security.lsmEnforcerStatus.value.blockedExecPaths
                    ?.length &&
                  !props.security.lsmEnforcerStatus.value.blockedExecNames
                    ?.length &&
                  !props.security.lsmEnforcerStatus.value.blockedFileNames
                    ?.length &&
                  !props.security.cgroupSandboxStatus.value.blockedIPs
                    ?.length &&
                  !props.security.cgroupSandboxStatus.value.blockedPorts?.length
                "
                style="padding: 32px; text-align: center; color: #999"
              >
                <InfoCircleOutlined /> 暂无活跃内核阻断规则。
              </div>
            </a-list>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="plugins"
          tab="自生成的自定义 eBPF 过滤插件 (Block Plugins)"
        >
          <a-list
            bordered
            size="small"
            :data-source="plugins.filter((p) => p.id.startsWith('visual-'))"
          >
            <template #renderItem="{ item }">
              <a-list-item :key="item.id">
                <div class="plugin-item-info">
                  <a-tag color="purple">eBPF 插件</a-tag>
                  <strong style="margin-right: 12px">{{ item.name }}</strong>
                  <code style="font-size: 11px; margin-right: 12px">{{
                    item.id
                  }}</code>
                  <a-tag :color="item.loaded ? 'green' : 'orange'">
                    {{ item.loaded ? "挂载拦截中" : "未装载" }}
                  </a-tag>
                </div>
                <template #actions>
                  <a-button
                    v-if="item.loaded"
                    size="small"
                    danger
                    @click="handleUnloadPlugin(item.id)"
                  >
                    <template #icon><StopOutlined /></template>卸载
                  </a-button>
                  <a-button
                    v-else
                    size="small"
                    type="primary"
                    @click="loadBpf(item.id).then(() => fetchPlugins())"
                  >
                    <template #icon><PlayCircleOutlined /></template>装载
                  </a-button>
                </template>
              </a-list-item>
            </template>

            <div
              v-if="!plugins.filter((p) => p.id.startsWith('visual-')).length"
              style="padding: 32px; text-align: center; color: #999"
            >
              <InfoCircleOutlined /> 暂无自编译的高级过滤器插件。
            </div>
          </a-list>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </div>
</template>

<style scoped>
.visual-filter-tab {
  min-height: 500px;
}
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
.stat-card {
  border-radius: 6px;
  background: #fafafa;
}
.rule-item {
  display: flex;
  align-items: center;
  gap: 12px;
}
.plugin-item-info {
  display: flex;
  align-items: center;
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
