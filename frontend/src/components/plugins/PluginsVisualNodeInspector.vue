<script setup lang="ts">
import { computed } from "vue";
import {
  AlertOutlined,
  ApiOutlined,
  CodeOutlined,
  ForkOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import { triggerOptions } from "./constants";
import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualMapKey,
  VisualMapMode,
  VisualTrigger,
  VisualValidationIssue,
} from "./types";

const props = defineProps<{
  selectedNodeId: VisualFlowNodeId;
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
  pluginId: string;
  pluginName: string;
  description: string;
  conditionCount: number;
  treeDepth: number;
  codeLines: number;
  compileReady: boolean;
  compiling: boolean;
  validationIssues: VisualValidationIssue[];
}>();

const emit = defineEmits<{
  (e: "update:trigger", value: VisualTrigger): void;
  (e: "update:action", value: VisualAction): void;
  (e: "update:mapMode", value: VisualMapMode): void;
  (e: "update:mapKey", value: VisualMapKey): void;
  (e: "update:mapLimit", value: number): void;
  (e: "update:pluginId", value: string): void;
  (e: "update:pluginName", value: string): void;
  (e: "update:description", value: string): void;
  (e: "add-condition", value: VisualConditionField): void;
  (e: "add-group", value: "AND" | "OR"): void;
  (e: "compile"): void;
}>();

const triggerModel = computed({
  get: () => props.trigger,
  set: (value: VisualTrigger) => emit("update:trigger", value),
});

const actionModel = computed({
  get: () => props.action,
  set: (value: VisualAction) => emit("update:action", value),
});

const mapModeModel = computed({
  get: () => props.mapMode,
  set: (value: VisualMapMode) => emit("update:mapMode", value),
});

const mapKeyModel = computed({
  get: () => props.mapKey,
  set: (value: VisualMapKey) => emit("update:mapKey", value),
});

const mapLimitModel = computed({
  get: () => props.mapLimit,
  set: (value: number | null) => emit("update:mapLimit", Number(value) || 1),
});

const pluginIdModel = computed({
  get: () => props.pluginId,
  set: (value: string) => emit("update:pluginId", value),
});

const pluginNameModel = computed({
  get: () => props.pluginName,
  set: (value: string) => emit("update:pluginName", value),
});

const descriptionModel = computed({
  get: () => props.description,
  set: (value: string) => emit("update:description", value),
});

const nodeCopy: Record<
  VisualFlowNodeId,
  { title: string; summary: string; icon: any }
> = {
  trigger: {
    title: "Trigger Inspector",
    summary: "从画布直接切换内核挂载点，后续条件字段会按入口自动收敛。",
    icon: ApiOutlined,
  },
  condition: {
    title: "Condition Inspector",
    summary: "快速追加字段条件或 AND/OR 分组，再到下方条件树做精细编辑。",
    icon: ForkOutlined,
  },
  map: {
    title: "Map Inspector",
    summary: "声明 BPF Map 状态积木，让规则支持计数、限频或运行时阻断表。",
    icon: SafetyCertificateOutlined,
  },
  action: {
    title: "Action Inspector",
    summary: "配置命中后的响应动作：只告警、返回拒绝，或强制结束进程。",
    icon: AlertOutlined,
  },
  code: {
    title: "Code Inspector",
    summary: "查看当前积木转译出的 C 源码规模，源码面板位于右侧。",
    icon: CodeOutlined,
  },
  compile: {
    title: "Compile Inspector",
    summary: "补齐插件元数据并在验证通过后注册、编译为可加载 eBPF 插件。",
    icon: ThunderboltOutlined,
  },
};

const selectedCopy = computed(() => nodeCopy[props.selectedNodeId]);

const conditionQuickActions: Array<{
  value: VisualConditionField;
  label: string;
  hint: string;
}> = [
  { value: "comm", label: "进程名 comm", hint: "如 nc / bash / python" },
  { value: "uid", label: "UID", hint: "限制用户范围" },
  { value: "basename", label: "文件名", hint: "适合 LSM 文件事件" },
  { value: "port", label: "端口", hint: "仅 socket_connect" },
  { value: "ipv4", label: "IPv4", hint: "仅 socket_connect" },
  { value: "gid", label: "GID", hint: "限制用户组" },
];

const selectedErrors = computed(() =>
  props.validationIssues
    .filter((issue) => issue.severity === "error")
    .slice(0, 3),
);

const selectedWarnings = computed(() =>
  props.validationIssues
    .filter((issue) => issue.severity !== "error")
    .slice(0, 2),
);

const canUseSocketFields = computed(() => props.trigger === "socket_connect");
const canUseBasename = computed(() => props.trigger !== "unlink");
</script>

<template>
  <div class="node-inspector">
    <div class="inspector-heading">
      <div class="heading-title">
        <component :is="selectedCopy.icon" class="heading-icon" />
        <div>
          <strong>{{ selectedCopy.title }}</strong>
          <span>{{ selectedCopy.summary }}</span>
        </div>
      </div>
      <a-tag :color="compileReady ? 'green' : 'red'">
        {{ compileReady ? "compile ready" : "needs fix" }}
      </a-tag>
    </div>

    <div v-if="selectedNodeId === 'trigger'" class="inspector-body">
      <a-form layout="vertical">
        <a-form-item label="内核事件入口积木">
          <a-select v-model:value="triggerModel">
            <a-select-option
              v-for="opt in triggerOptions"
              :key="opt.value"
              :value="opt.value"
            >
              <component :is="opt.icon" :style="{ color: opt.color }" />
              <span style="margin-left: 8px">{{ opt.label }}</span>
            </a-select-option>
          </a-select>
        </a-form-item>
      </a-form>
      <a-alert
        type="info"
        show-icon
        message="切换 Trigger 会自动清理当前 Hook 不支持的字段，例如非 socket 入口会移除 port / ipv4。"
      />
    </div>

    <div v-else-if="selectedNodeId === 'condition'" class="inspector-body">
      <div class="condition-stats">
        <a-statistic title="条件数量" :value="conditionCount" />
        <a-statistic title="嵌套层数" :value="treeDepth" />
      </div>
      <div class="quick-block-grid">
        <button
          v-for="item in conditionQuickActions"
          :key="item.value"
          type="button"
          class="quick-block"
          :disabled="
            ((item.value === 'port' || item.value === 'ipv4') &&
              !canUseSocketFields) ||
            (item.value === 'basename' && !canUseBasename)
          "
          @click="emit('add-condition', item.value)"
        >
          <strong>{{ item.label }}</strong>
          <span>{{ item.hint }}</span>
        </button>
      </div>
      <a-space wrap>
        <a-button size="small" type="dashed" @click="emit('add-group', 'AND')">
          追加 AND 分组
        </a-button>
        <a-button size="small" type="dashed" @click="emit('add-group', 'OR')">
          追加 OR 分组
        </a-button>
      </a-space>
    </div>

    <div v-else-if="selectedNodeId === 'map'" class="inspector-body">
      <a-form layout="vertical">
        <a-form-item label="状态 Map 模式">
          <a-radio-group v-model:value="mapModeModel" button-style="solid">
            <a-radio-button value="NONE">NONE</a-radio-button>
            <a-radio-button value="COUNTER">COUNTER</a-radio-button>
            <a-radio-button value="BLOCKLIST">BLOCKLIST</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="Map Key">
              <a-select v-model:value="mapKeyModel">
                <a-select-option value="pid">PID</a-select-option>
                <a-select-option value="uid">UID</a-select-option>
                <a-select-option value="comm">COMM</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="COUNTER 阈值">
              <a-input-number
                v-model:value="mapLimitModel"
                :min="1"
                :disabled="mapMode !== 'COUNTER'"
                style="width: 100%"
              />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </div>

    <div v-else-if="selectedNodeId === 'action'" class="inspector-body">
      <a-radio-group
        v-model:value="actionModel"
        button-style="solid"
        class="action-switch"
      >
        <a-radio-button value="BLOCK" :disabled="trigger === 'unlink'">
          BLOCK
        </a-radio-button>
        <a-radio-button value="ALERT">ALERT</a-radio-button>
        <a-radio-button value="KILL">KILL</a-radio-button>
      </a-radio-group>
      <a-alert
        v-if="trigger === 'unlink'"
        type="warning"
        show-icon
        message="unlink 当前走 kprobe/do_unlinkat，只能观测或发信号，不能直接返回 BLOCK。"
      />
      <a-alert
        v-else
        type="info"
        show-icon
        message="BLOCK/KILL 会返回 -EACCES；ALERT 仅打印内核事件日志。"
      />
    </div>

    <div
      v-else-if="selectedNodeId === 'code'"
      class="inspector-body code-summary"
    >
      <a-statistic title="Generated C Lines" :value="codeLines" />
      <p>
        右侧代码面板会随积木实时刷新；编译失败时可先检查这里生成的条件表达式与
        Map 查表逻辑。
      </p>
    </div>

    <div v-else class="inspector-body">
      <a-form layout="vertical">
        <a-form-item label="规则插件 ID">
          <a-input v-model:value="pluginIdModel" />
        </a-form-item>
        <a-form-item label="显示名">
          <a-input v-model:value="pluginNameModel" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="descriptionModel" :rows="2" />
        </a-form-item>
      </a-form>
      <div class="compile-footer">
        <div class="issue-list">
          <div
            v-for="issue in selectedErrors"
            :key="issue.id"
            class="issue error"
          >
            {{ issue.title }}
          </div>
          <div
            v-for="issue in selectedWarnings"
            :key="issue.id"
            class="issue warning"
          >
            {{ issue.title }}
          </div>
          <div
            v-if="selectedErrors.length === 0 && selectedWarnings.length === 0"
            class="issue ok"
          >
            当前积木配置通过编译前验证。
          </div>
        </div>
        <a-button
          type="primary"
          :loading="compiling"
          :disabled="!compileReady"
          @click="emit('compile')"
        >
          <template #icon><ThunderboltOutlined /></template>
          编译注册
        </a-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.node-inspector {
  margin-bottom: 20px;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid #d6e4ff;
  background: linear-gradient(135deg, #ffffff 0%, #f0f7ff 100%);
  color: #475569;
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
}

.inspector-heading,
.heading-title,
.compile-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.heading-title {
  align-items: flex-start;
}

.heading-icon {
  margin-top: 2px;
  color: #1677ff;
  font-size: 18px;
}

.heading-title strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

.heading-title span {
  display: block;
  margin-top: 2px;
  color: #64748b;
  font-size: 12px;
}

.inspector-body {
  margin-top: 14px;
}

.condition-stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}

.quick-block-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}

.quick-block {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-height: 58px;
  padding: 9px 10px;
  border-radius: 8px;
  border: 1px solid #d6e4ff;
  background: #ffffff;
  color: #0f172a;
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    transform 0.18s ease;
}

.quick-block:hover:not(:disabled) {
  transform: translateY(-1px);
  border-color: #1677ff;
}

.quick-block:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.quick-block strong {
  font-size: 12px;
}

.quick-block span {
  color: #64748b;
  font-size: 11px;
}

.action-switch {
  display: flex;
  width: 100%;
  margin-bottom: 12px;
}

.action-switch :deep(.ant-radio-button-wrapper) {
  flex: 1;
  text-align: center;
}

.code-summary {
  display: grid;
  grid-template-columns: 180px 1fr;
  align-items: center;
  gap: 12px;
}

.code-summary p {
  margin: 0;
  color: #64748b;
  font-size: 12px;
}

.issue-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.issue {
  font-size: 12px;
}

.issue.error {
  color: #cf1322;
}

.issue.warning {
  color: #ad6800;
}

.issue.ok {
  color: #237804;
}

:deep(.ant-statistic-title) {
  color: #64748b;
  font-size: 11px;
}

:deep(.ant-statistic-content) {
  color: #0f172a;
  font-size: 20px;
}

:deep(.ant-form-item-label > label) {
  color: #475569;
}

:deep(.ant-select-selector),
:deep(.ant-input),
:deep(.ant-input-number),
:deep(.ant-radio-button-wrapper) {
  background-color: #ffffff !important;
  border-color: #d9d9d9 !important;
  color: #0f172a !important;
}

:deep(.ant-radio-button-wrapper-checked) {
  background-color: #1677ff !important;
  border-color: #1677ff !important;
  color: #ffffff !important;
}
</style>
