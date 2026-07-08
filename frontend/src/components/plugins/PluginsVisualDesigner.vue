<script setup lang="ts">
import {
  ThunderboltOutlined,
  SafetyCertificateOutlined,
  AlertOutlined,
  CloseCircleOutlined,
} from "@ant-design/icons-vue";
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
import "./PluginsVisualDesigner.css";

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
        <strong style="color: #fff">防御拦截挂载点积木 (Trigger Block)</strong>
      </div>
      <div class="block-body">
        <div class="desc-line">选择安全管控的内核底层事件拦截入口：</div>
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
            style="
              border-left: 1px dashed #d6e4ff;
              padding-left: 16px;
            "
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
        <strong style="color: #fff">安全管控响应积木 (Action Block)</strong>
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
          * 物理文件 unlink 挂载于 Kprobe 上，不改变内核决策链，仅支持 ALERT 或
          KILL 动作。其他 LSM 挂载点支持完整的 BLOCK、ALERT 与 KILL 动作。
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
