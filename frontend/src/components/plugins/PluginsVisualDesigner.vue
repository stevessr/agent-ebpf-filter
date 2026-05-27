<script setup lang="ts">
import { ThunderboltOutlined, SafetyCertificateOutlined, AlertOutlined, CloseCircleOutlined } from "@ant-design/icons-vue";
import PluginsVisualConditionTree from "./PluginsVisualConditionTree.vue";
import PluginsVisualMapPanel from "./PluginsVisualMapPanel.vue";
import PluginsVisualSchematic from "./PluginsVisualSchematic.vue";
import { triggerOptions } from "./constants";
import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualLogicGroup,
  VisualMapKey,
  VisualMapMode,
  VisualTrigger,
  VisualValidationIssue,
} from "./types";

const props = defineProps<{
  trigger: VisualTrigger;
  action: VisualAction;
  logicRoot: VisualLogicGroup;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
  pluginId: string;
  pluginName: string;
  description: string;
  compiling: boolean;
  isWorkspaceValid: boolean;
  validationIssues: VisualValidationIssue[];
  activeFlowNode: VisualFlowNodeId;
  countConditions: number;
  treeDepth: number;
  onDeleteNode: (id: string) => void;
  onAddRule: (groupId: string, field?: string) => void;
  onAddGroup: (groupId: string, type: "AND" | "OR") => void;
  onUpdateRule: (ruleId: string, updated: any) => void;
  onUpdateGroupType: (groupId: string, type: "AND" | "OR") => void;
  flowSectionClass: (node: VisualFlowNodeId) => Record<string, boolean>;
}>();

const emit = defineEmits<{
  "update:trigger": [value: VisualTrigger];
  "update:action": [value: VisualAction];
  "update:mapMode": [value: VisualMapMode];
  "update:mapKey": [value: VisualMapKey];
  "update:mapLimit": [value: number];
  "update:pluginId": [value: string];
  "update:pluginName": [value: string];
  "update:description": [value: string];
  "add-condition": [field: VisualConditionField];
  "add-group": [type: "AND" | "OR"];
  compile: [];
}>();
</script>

<template>
  <div class="map-workspace-shell">
    <div class="map-workspace-notice">
      <a-tag color="purple">二级选项卡</a-tag>
      <span
        >原先偏 map / blueprint 的块状配置、条件树和状态 Map
        面板集中在这里，主编辑体验保持 Dify 工作流风格。</span
      >
    </div>

    <!-- BLOCK 1: EVENT TRIGGER -->
    <div
      ref="triggerBlock"
      class="block-card block-trigger"
      :class="flowSectionClass('trigger')"
    >
      <div class="node-port port-output trigger-port"></div>
      <div class="block-header">
        <span class="block-badge">Block 1</span>
        <strong style="color: #fff"
          >防御拦截挂载点积木 (Trigger Block)</strong
        >
      </div>
      <div class="block-body">
        <div class="desc-line">
          选择安全管控的内核底层事件拦截入口：
        </div>
        <a-select
          :value="trigger"
          style="width: 100%"
          @update:value="emit('update:trigger', $event)"
        >
          <a-select-option
            v-for="opt in triggerOptions"
            :key="opt.value"
            :value="opt.value"
          >
            <component
              :is="opt.icon"
              :style="{ color: opt.color }"
            />
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

    <!-- BLOCK 2: DYNAMIC CONDITIONS -->
    <div
      ref="conditionBlock"
      class="block-card block-condition"
      :class="flowSectionClass('condition')"
    >
      <div class="node-port port-input condition-port-in"></div>
      <div class="node-port port-output condition-port-out"></div>
      <div class="block-header">
        <div>
          <span class="block-badge" style="background: #1677ff">Block 2</span>
          <strong style="color: #fff"
            >高级嵌套逻辑过滤条件 (Nested Condition Block)</strong
          >
        </div>
      </div>
      <div class="block-body">
        <a-row :gutter="16">
          <a-col :span="15">
            <div class="desc-line" style="margin-bottom: 16px">
              支持无限嵌套的逻辑运算组，可从左侧拖拽条件或逻辑组至目标块内：
            </div>
            <div
              class="conditions-list-tree"
              style="max-height: 380px; overflow-y: auto; padding-right: 4px"
            >
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
          <a-col
            :span="9"
            style="border-left: 1px dashed rgba(255, 255, 255, 0.1); padding-left: 16px"
          >
            <PluginsVisualSchematic :logic-root="logicRoot" />
          </a-col>
        </a-row>
      </div>
    </div>

    <!-- CONNECTION ARROW -->
    <div class="blueprint-wire-container">
      <div class="blueprint-wire-line wire-2-to-2-5"></div>
      <div class="blueprint-wire-pulse pulse-2-to-2-5"></div>
    </div>

    <!-- BLOCK 2.5: STATEFUL MAP -->
    <div ref="mapBlock" :class="flowSectionClass('map')">
      <PluginsVisualMapPanel
        :mode="mapMode"
        :key-field="mapKey"
        :limit="mapLimit"
        @update:mode="emit('update:mapMode', $event)"
        @update:key-field="emit('update:mapKey', $event)"
        @update:limit="emit('update:mapLimit', $event)"
      />
    </div>

    <!-- CONNECTION ARROW -->
    <div class="blueprint-wire-container">
      <div class="blueprint-wire-line wire-2-5-to-3"></div>
      <div class="blueprint-wire-pulse pulse-2-5-to-3"></div>
    </div>

    <!-- BLOCK 3: TARGET ACTION -->
    <div
      ref="actionBlock"
      class="block-card block-action"
      :class="flowSectionClass('action')"
    >
      <div class="node-port port-input action-port-in"></div>
      <div class="block-header">
        <span class="block-badge" style="background: #1677ff">Block 3</span>
        <strong style="color: #fff"
          >安全管控响应积木 (Action Block)</strong
        >
      </div>
      <div class="block-body">
        <div class="desc-line">
          当上述过滤组合触发成功时，内核要执行的安全响应动作：
        </div>
        <a-radio-group
          :value="action"
          button-style="solid"
          style="width: 100%"
          @update:value="emit('update:action', $event)"
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
          style="color: #ad6800; margin-top: 8px"
        >
          * 物理文件 unlink 挂载于 Kprobe
          上，不改变内核决策链，仅支持 ALERT 或 KILL 动作。其他 LSM
          挂载点支持完整的 BLOCK、ALERT 与 KILL 动作。
        </div>
      </div>
    </div>

    <!-- Plugin Metadata + Compile -->
    <div ref="compileBlock" :class="flowSectionClass('compile')">
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
                  :value="pluginId"
                  placeholder="例如 custom-visual-lsm"
                  @update:value="emit('update:pluginId', $event)"
                />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item label="规则插件显示名">
                <a-input
                  :value="pluginName"
                  @update:value="emit('update:pluginName', $event)"
                />
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item label="详细说明描述" style="margin-bottom: 0">
            <a-textarea
              :value="description"
              :rows="2"
              @update:value="emit('update:description', $event)"
            />
          </a-form-item>
        </a-form>
        <div style="margin-top: 20px; display: flex; justify-content: flex-end">
          <a-button
            type="primary"
            :loading="compiling"
            :disabled="!isWorkspaceValid"
            @click="emit('compile')"
          >
            <template #icon><ThunderboltOutlined /></template>
            一键编译并注册为 BPF 插件
          </a-button>
        </div>
      </a-card>
    </div>
  </div>
</template>

<style scoped>
.map-workspace-shell {
  padding-top: 8px;
}

.map-workspace-notice {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--workflow-border);
  background: linear-gradient(135deg, #ffffff 0%, var(--workflow-primary-subtle) 100%);
  color: var(--workflow-text-secondary);
  box-shadow: 0 8px 24px rgba(22, 119, 255, 0.08);
  border-color: var(--workflow-border);
}

.map-workspace-notice span {
  margin: 0;
  color: var(--workflow-text-muted);
  font-size: 12px;
  line-height: 1.45;
}

.flow-section-active {
  outline: 2px solid rgba(22, 119, 255, 0.62);
  outline-offset: 4px;
  box-shadow: 0 0 0 1px rgba(22, 119, 255, 0.2),
    0 0 24px rgba(22, 119, 255, 0.16);
  border-radius: 10px;
  transition: outline-color 0.2s ease, box-shadow 0.2s ease;
}

.block-card {
  border-radius: 8px;
  overflow: visible;
  box-shadow: 0 10px 28px rgba(22, 119, 255, 0.12);
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid var(--workflow-border);
}

.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 14px 38px rgba(22, 119, 255, 0.18);
}

.block-trigger {
  border-color: var(--workflow-primary-border);
}

.block-trigger:hover {
  border-color: var(--workflow-primary);
  box-shadow: 0 0 15px rgba(22, 119, 255, 0.18);
}

.block-trigger .block-header {
  background: linear-gradient(135deg, var(--workflow-primary), var(--workflow-primary-hover));
}

.block-condition {
  border-color: var(--workflow-primary-border);
}

.block-condition:hover {
  border-color: var(--workflow-primary);
  box-shadow: 0 0 15px rgba(22, 119, 255, 0.18);
}

.block-condition .block-header {
  background: linear-gradient(135deg, var(--workflow-primary), var(--workflow-primary-hover));
}

.block-action {
  border-color: var(--workflow-primary-border);
}

.block-action:hover {
  border-color: var(--workflow-primary);
  box-shadow: 0 0 15px rgba(22, 119, 255, 0.18);
}

.block-action .block-header {
  background: linear-gradient(135deg, var(--workflow-primary), var(--workflow-primary-hover));
}

.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(22, 119, 255, 0.12);
}

.block-badge {
  background: rgba(255, 255, 255, 0.22);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}

.block-body {
  background: #ffffff;
  padding: 18px;
  color: var(--workflow-text-secondary);
}

.desc-line {
  font-size: 13px;
  color: var(--workflow-text-muted);
  margin-bottom: 12px;
}

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
  background: linear-gradient(180deg, var(--workflow-primary), var(--workflow-primary-hover));
}

.wire-2-to-2-5 {
  background: linear-gradient(180deg, var(--workflow-primary-hover), var(--workflow-primary));
}

.wire-2-5-to-3 {
  background: linear-gradient(180deg, var(--workflow-primary), var(--workflow-primary-hover));
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
  background: var(--workflow-primary);
  box-shadow: 0 0 8px var(--workflow-primary), 0 0 15px rgba(22, 119, 255, 0.38);
}

.pulse-2-to-2-5 {
  background: var(--workflow-primary-hover);
  box-shadow: 0 0 8px var(--workflow-primary-hover), 0 0 15px rgba(64, 150, 255, 0.38);
}

.pulse-2-5-to-3 {
  background: var(--workflow-primary);
  box-shadow: 0 0 8px var(--workflow-primary), 0 0 15px rgba(22, 119, 255, 0.38);
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

.node-port {
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  border: 1px solid rgba(255, 255, 255, 0.8);
}

.port-input {
  top: -5px;
}

.port-output {
  bottom: -5px;
}

.trigger-port {
  background: var(--workflow-primary);
  border-color: var(--workflow-primary);
  box-shadow: 0 0 8px var(--workflow-primary);
}

.condition-port-in {
  background: var(--workflow-primary);
  border-color: var(--workflow-primary);
  box-shadow: 0 0 8px var(--workflow-primary);
}

.condition-port-out {
  background: var(--workflow-primary-hover);
  border-color: var(--workflow-primary-hover);
  box-shadow: 0 0 8px var(--workflow-primary-hover);
}

.action-port-in {
  background: var(--workflow-primary);
  border-color: var(--workflow-primary);
  box-shadow: 0 0 8px var(--workflow-primary);
}

.helper-text {
  font-size: 11px;
}

.block-red.ant-radio-button-wrapper-checked {
  background: #f5222d;
  border-color: #f5222d;
  color: white;
}

:deep(.ant-select-selector),
:deep(.ant-input),
:deep(.ant-input-number),
:deep(.ant-radio-button-wrapper) {
  background-color: #ffffff !important;
  border-color: #d9d9d9 !important;
  color: var(--workflow-text) !important;
}

:deep(.ant-select-arrow) {
  color: var(--workflow-text-muted) !important;
}

:deep(.ant-radio-button-wrapper-checked) {
  background-color: var(--workflow-primary) !important;
  color: #ffffff !important;
  border-color: var(--workflow-primary) !important;
}

:deep(.ant-radio-button-wrapper-checked.block-red) {
  background-color: #ef4444 !important;
  border-color: #ef4444 !important;
}

:deep(.ant-btn-dashed) {
  background: #ffffff !important;
  border-color: var(--workflow-border) !important;
  color: var(--workflow-text-muted) !important;
}

:deep(.ant-btn-dashed:hover) {
  border-color: var(--workflow-primary) !important;
  color: var(--workflow-primary) !important;
}

:deep(.ant-card) {
  background: #ffffff !important;
  border-color: var(--workflow-border) !important;
}

:deep(.ant-card-head) {
  border-bottom-color: var(--workflow-border) !important;
  color: var(--workflow-text) !important;
  background: var(--workflow-surface-soft) !important;
}
</style>
