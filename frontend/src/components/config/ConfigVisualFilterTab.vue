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

// Auto update plugin metadata when type/value/action changes
watch(
  [
    () => currentConfig.value.type,
    () => currentConfig.value.value,
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

// Directly Apply as a core kernel blocking rule
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
        "内置高性能拦截引擎（LSM & cgroup）默认执行硬阻断（BLOCK）。如需实现定制化的“仅告警并打印内核日志”逻辑，请在右侧点击“编译并作为自定义 eBPF 插件装载”运行。",
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
    } else if (type === "file") {
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

// Compile and Load as a custom eBPF plugin
const handleCompilePlugin = async () => {
  compiling.value = true;
  compileLogLocal.value = "正在将生成的 C 代码写入系统临时文件...\n";
  try {
    // 1. Save or Update the plugin in the registry
    compileLogLocal.value += `正在将插件 [${currentConfig.value.pluginId}] 注册至项目 Manifest...\n`;
    await upsertPlugin({
      id: currentConfig.value.pluginId,
      name: currentConfig.value.pluginName,
      description: currentConfig.value.description,
      kind: "ebpf",
      enabled: false,
      attachKind:
        currentConfig.value.type === "process" ? "tracepoint" : "kprobe", // dummy values, compiling uses user SEC hooks directly
      attachTarget:
        currentConfig.value.type === "process"
          ? "syscalls/sys_enter_execve"
          : "do_unlinkat",
      programName:
        currentConfig.value.type === "process"
          ? "visual_process_filter"
          : "visual_file_filter",
      source: generatedCode.value,
    });

    // 2. Call backend compilation
    compileLogLocal.value +=
      "正在调用 Clang (target bpf) 编译为 ELF 字节码...\n";
    const success = await compileBpf(
      currentConfig.value.pluginId,
      generatedCode.value
    );
    if (success) {
      isCompiled.value = true;
      compileLogLocal.value +=
        "编译成功！已生成 program.o。支持点击“装载过滤器”载入内核。";
    } else {
      compileLogLocal.value += "编译失败。详情请查看控制台报错。";
    }
  } catch (err: any) {
    compileLogLocal.value += "\n[ERROR] 发生错误: " + err.message;
  } finally {
    compiling.value = false;
  }
};

const handleLoadPlugin = async () => {
  loadingAction.value = true;
  try {
    // Enable and Load BPF plugin
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

// Core rule unblocking helper
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
      <!-- Left side: Interactive Form Designer -->
      <a-col :span="12">
        <a-card
          title="可视化 eBPF 规则生成器 (Visual Rule Designer)"
          size="small"
        >
          <template #extra>
            <span style="color: #1677ff; font-weight: 500">
              <FireOutlined /> 内核层高级防护
            </span>
          </template>

          <a-form layout="vertical">
            <a-form-item
              label="1. 选择过滤防御对象 (Target object to intercept)"
            >
              <a-radio-group
                v-model:value="currentConfig.type"
                button-style="solid"
                style="width: 100%"
              >
                <a-radio-button
                  value="process"
                  style="width: 25%; text-align: center"
                >
                  <ThunderboltOutlined /> 进程运行
                </a-radio-button>
                <a-radio-button
                  value="file"
                  style="width: 25%; text-align: center"
                >
                  <SafetyCertificateOutlined /> 文件打开
                </a-radio-button>
                <a-radio-button
                  value="ip"
                  style="width: 25%; text-align: center"
                >
                  <GlobalOutlined /> 网络IP
                </a-radio-button>
                <a-radio-button
                  value="port"
                  style="width: 25%; text-align: center"
                >
                  <CodeOutlined /> 网络端口
                </a-radio-button>
              </a-radio-group>
            </a-form-item>

            <a-form-item label="2. 配置过滤目标参数 (Parameters)">
              <div v-if="currentConfig.type === 'process'">
                <a-input
                  v-model:value="currentConfig.value"
                  placeholder="例如: nc 或 /usr/bin/nc (支持 basename 或完整路径)"
                />
                <span class="helper-text"
                  >当内核检测到该名称或完整路径可执行文件试图运行时，硬阻断返回
                  EACCES (拒绝访问)。</span
                >
              </div>
              <div v-else-if="currentConfig.type === 'file'">
                <a-input
                  v-model:value="currentConfig.value"
                  placeholder="例如: id_rsa 或 shadow (文件名，不支持带/的路径)"
                />
                <span class="helper-text"
                  >当检测到任何进程视图读取、打开此 basename
                  匹配的文件时，直接在内核层阻断打开 fd。</span
                >
              </div>
              <div v-else-if="currentConfig.type === 'ip'">
                <a-input
                  v-model:value="currentConfig.value"
                  placeholder="例如: 8.8.8.8 或 127.0.0.1"
                />
                <span class="helper-text"
                  >在 cgroup/connect 阶段，直接拦截目的端 IP
                  出站网络包，拒绝建立 Socket。</span
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
                  >拦截发往外部目标主机的特定 TCP/UDP 端口。连接失败。</span
                >
              </div>
            </a-form-item>

            <a-form-item label="3. 选择动作级别 (Filter Action)">
              <a-radio-group
                v-model:value="currentConfig.action"
                button-style="solid"
              >
                <a-radio-button value="BLOCK" class="action-block-btn">
                  BLOCK (阻断拦截)
                </a-radio-button>
                <a-radio-button value="ALERT">
                  ALERT (仅告警不拦截)
                </a-radio-button>
              </a-radio-group>
              <span class="helper-text" style="display: block; margin-top: 4px">
                BLOCK：在内核触发点阻断并向容器/进程抛出错误；
                ALERT：允许操作但通过 eBPF `bpf_printk`
                在内核级记录安全审计事件。
              </span>
            </a-form-item>

            <a-form-item label="插件元数据预览 (Automatic Metadata)">
              <div class="metadata-preview">
                <div>
                  <strong>插件标识 ID:</strong>
                  <code>{{ currentConfig.pluginId }}</code>
                </div>
                <div>
                  <strong>展示名称 Name:</strong>
                  <span>{{ currentConfig.pluginName }}</span>
                </div>
                <div>
                  <strong>描述 Description:</strong>
                  <span style="font-size: 12px; color: #666">{{
                    currentConfig.description
                  }}</span>
                </div>
              </div>
            </a-form-item>

            <div style="margin-top: 24px">
              <a-space style="width: 100%; justify-content: flex-end">
                <!-- Deploy Direct Core rule -->
                <a-button
                  type="primary"
                  danger
                  :loading="loadingAction"
                  @click="handleApplyCoreRule"
                >
                  <template #icon><SafetyCertificateOutlined /></template>
                  直接应用为内核阻断规则
                </a-button>

                <!-- Compile as Custom plugin -->
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

        <!-- Rule enforcement engine status -->
        <a-card
          title="内核防御引擎就绪状态 (Enforcement Engine Status)"
          size="small"
          style="margin-top: 20px"
        >
          <a-descriptions bordered :column="1" size="small">
            <a-descriptions-item label="LSM 文件/进程拦截器 (LSM Enforcer)">
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
            <a-descriptions-item label="cgroup 网络拦截器 (cgroup Sandbox)">
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

      <!-- Right side: Code Generator & Compilation Logger -->
      <a-col :span="12">
        <a-card
          title="动态生成的 eBPF C 语言过滤器源码 (Generated C Code Preview)"
          size="small"
        >
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
              <span>Clang / LLVM 编译输出控制台</span>
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

    <!-- Block List Management Board -->
    <a-card
      title="活跃内核阻断规则与插件监控面板 (Active Kernel Blocks & Visual eBPF Monitor)"
      size="small"
      style="margin-top: 24px"
    >
      <a-tabs default-active-key="core-rules" size="small">
        <a-tab-pane
          key="core-rules"
          tab="活跃核心阻断规则 (Core Interceptor Lists)"
        >
          <div class="monitor-stats">
            <a-row :gutter="16" style="margin-bottom: 16px">
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="LSM 已拦截进程 (Execs Blocked)"
                    :value="security.lsmEnforcerStatus.value.stats.execBlocked"
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="LSM 已拦截文件 (Files Blocked)"
                    :value="security.lsmEnforcerStatus.value.stats.fileBlocked"
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="cgroup 拦截出站 (Net Blocked)"
                    :value="security.cgroupSandboxStatus.value.stats.blocked"
                  />
                </a-card>
              </a-col>
              <a-col :span="6">
                <a-card size="small" class="stat-card">
                  <a-statistic
                    title="cgroup 放行出站 (Net Allowed)"
                    :value="security.cgroupSandboxStatus.value.stats.allowed"
                  />
                </a-card>
              </a-col>
            </a-row>
          </div>

          <div style="margin-top: 12px">
            <a-list bordered size="small" header="当前拦截列表">
              <!-- LSM Exec Paths -->
              <a-list-item
                v-for="path in security.lsmEnforcerStatus.value
                  .blockedExecPaths"
                :key="`path-${path}`"
              >
                <div class="rule-item">
                  <a-tag color="red">LSM 路径拦截</a-tag>
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

              <!-- LSM Exec Names -->
              <a-list-item
                v-for="name in security.lsmEnforcerStatus.value
                  .blockedExecNames"
                :key="`name-${name}`"
              >
                <div class="rule-item">
                  <a-tag color="volcano">LSM 进程拦截</a-tag>
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

              <!-- LSM File Names -->
              <a-list-item
                v-for="file in security.lsmEnforcerStatus.value
                  .blockedFileNames"
                :key="`file-${file}`"
              >
                <div class="rule-item">
                  <a-tag color="orange">LSM 文件拦截</a-tag>
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
                v-for="ip in security.cgroupSandboxStatus.value.blockedIPs"
                :key="`ip-${ip}`"
              >
                <div class="rule-item">
                  <a-tag color="blue">网络 IP 拦截</a-tag>
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
                v-for="port in security.cgroupSandboxStatus.value.blockedPorts"
                :key="`port-${port}`"
              >
                <div class="rule-item">
                  <a-tag color="purple">网络端口拦截</a-tag>
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

              <!-- Empty state -->
              <div
                v-if="
                  !security.lsmEnforcerStatus.value.blockedExecPaths?.length &&
                  !security.lsmEnforcerStatus.value.blockedExecNames?.length &&
                  !security.lsmEnforcerStatus.value.blockedFileNames?.length &&
                  !security.cgroupSandboxStatus.value.blockedIPs?.length &&
                  !security.cgroupSandboxStatus.value.blockedPorts?.length
                "
                style="padding: 32px; text-align: center; color: #999"
              >
                <InfoCircleOutlined />
                暂无活跃内核阻断规则。在上方配置一个规则并点击“直接应用为内核阻断规则”即可部署在内核中。
              </div>
            </a-list>
          </div>
        </a-tab-pane>

        <a-tab-pane
          key="plugins"
          tab="已部署可视化 eBPF 插件 (Custom Filter Plugins)"
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
                    {{ item.loaded ? "拦截加载中 (Active)" : "已卸载" }}
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
              <InfoCircleOutlined />
              暂无已编译的可视化自定义插件。在上方配置并在右侧点击“生成并编译为
              BPF 插件”即可编译。
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
