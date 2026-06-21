<script setup lang="ts">
import { computed } from "vue";
import {
  DeleteOutlined,
  PlusOutlined,
  FolderAddOutlined,
  DragOutlined,
} from "@ant-design/icons-vue";
import type { VisualLogicNode, VisualCondition } from "./types";

interface Props {
  node: VisualLogicNode;
  trigger: string;
  depth?: number;
  onDeleteNode: (id: string) => void;
  onAddRule: (groupId: string, field?: string) => void;
  onAddGroup: (groupId: string, type: "AND" | "OR") => void;
  onUpdateRule: (ruleId: string, updated: Partial<VisualCondition>) => void;
  onUpdateGroupType: (groupId: string, type: "AND" | "OR") => void;
}

const props = withDefaults(defineProps<Props>(), {
  depth: 0,
});

const fieldOptions = [
  { value: "comm", label: "进程名称 (Comm)" },
  { value: "pid", label: "进程 PID" },
  { value: "uid", label: "用户 UID" },
  { value: "basename", label: "文件名 (Basename)" },
  { value: "port", label: "目标端口 (Port)" },
  { value: "ipv4", label: "IPv4 地址" },
  { value: "gid", label: "进程组 GID" },
];

const operatorOptions = [
  { value: "==", label: "等于 (==)" },
  { value: "!=", label: "不等于 (!=)" },
  { value: "starts_with", label: "前缀 (starts)" },
  { value: "ends_with", label: "后缀 (ends)" },
];

const conditionNode = computed(() => props.node as VisualCondition);

const handleDragStart = (event: DragEvent, nodeId: string) => {
  if (event.dataTransfer) {
    event.dataTransfer.setData(
      "text/plain",
      JSON.stringify({ category: "tree_node", value: nodeId }),
    );
    event.dataTransfer.effectAllowed = "move";
  }
};

const handleGroupDrop = (event: DragEvent, groupId: string) => {
  event.preventDefault();
  event.stopPropagation();
  if (!event.dataTransfer) return;
  try {
    const rawData = event.dataTransfer.getData("text/plain");
    if (!rawData) return;
    const { category, value } = JSON.parse(rawData);

    if (category === "condition") {
      props.onAddRule(groupId, value);
    } else if (category === "logic_group") {
      props.onAddGroup(groupId, value as "AND" | "OR");
    }
  } catch (e) {
    console.error("Drop inside recursive group failed:", e);
  }
};
</script>

<template>
  <!-- GROUP NODE -->
  <div
    v-if="node.type === 'AND' || node.type === 'OR'"
    class="logic-group-container"
    :class="[
      node.type === 'AND' ? 'group-and' : 'group-or',
      depth > 0 ? 'nested-group' : '',
    ]"
    @dragover.prevent
    @drop.stop="handleGroupDrop($event, node.id)"
  >
    <div class="group-header">
      <div class="header-left">
        <a-tag
          :color="node.type === 'AND' ? 'blue' : 'magenta'"
          style="font-weight: bold; font-family: monospace"
        >
          {{ node.type }}
        </a-tag>

        <a-radio-group
          :value="node.type"
          @change="(e: any) => onUpdateGroupType(node.id, e.target.value)"
          size="small"
          button-style="solid"
          class="group-radio-toggle"
        >
          <a-radio-button value="AND">且 (AND)</a-radio-button>
          <a-radio-button value="OR">或 (OR)</a-radio-button>
        </a-radio-group>
      </div>

      <div class="header-right">
        <a-space size="small">
          <a-button
            type="text"
            size="small"
            @click="() => onAddRule(node.id)"
            class="btn-action"
          >
            <template #icon><PlusOutlined /></template>
            +条件
          </a-button>
          <a-button
            type="text"
            size="small"
            @click="() => onAddGroup(node.id, 'AND')"
            class="btn-action"
          >
            <template #icon><FolderAddOutlined /></template>
            +子组
          </a-button>
          <a-button
            v-if="node.id !== 'root'"
            type="text"
            danger
            size="small"
            @click="() => onDeleteNode(node.id)"
            class="btn-delete"
          >
            <template #icon><DeleteOutlined /></template>
          </a-button>
        </a-space>
      </div>
    </div>

    <!-- Recursive children -->
    <div class="group-children">
      <div
        v-if="!node.children || node.children.length === 0"
        class="group-empty-placeholder"
      >
        拖拽组件或点击“+条件”向此逻辑分组添加内容
      </div>
      <div v-else class="children-list">
        <div
          v-for="child in node.children"
          :key="child.id"
          class="child-node-row"
        >
          <PluginsVisualConditionTree
            :node="child"
            :trigger="trigger"
            :depth="depth + 1"
            :on-delete-node="onDeleteNode"
            :on-add-rule="onAddRule"
            :on-add-group="onAddGroup"
            :on-update-rule="onUpdateRule"
            :on-update-group-type="onUpdateGroupType"
          />
        </div>
      </div>
    </div>
  </div>

  <!-- LEAF CONDITION NODE -->
  <div
    v-else
    class="logic-condition-row"
    draggable="true"
    @dragstart="handleDragStart($event, node.id)"
  >
    <div class="drag-handle-indicator">
      <DragOutlined />
    </div>

    <a-select
      :value="conditionNode.field"
      @change="(val: any) => onUpdateRule(node.id, { field: val })"
      style="width: 30%"
      class="rule-select"
    >
      <a-select-option
        v-for="f in fieldOptions"
        :key="f.value"
        :value="f.value"
        :disabled="
          (trigger === 'unlink' && f.value === 'basename') ||
          (trigger !== 'socket_connect' &&
            (f.value === 'port' || f.value === 'ipv4'))
        "
      >
        {{ f.label }}
      </a-select-option>
    </a-select>

    <a-select
      :value="conditionNode.operator"
      @change="(val: any) => onUpdateRule(node.id, { operator: val })"
      style="width: 25%"
      class="rule-select"
    >
      <a-select-option
        v-for="o in operatorOptions"
        :key="o.value"
        :value="o.value"
        :disabled="
          (conditionNode.field === 'pid' ||
            conditionNode.field === 'uid' ||
            conditionNode.field === 'port' ||
            conditionNode.field === 'ipv4' ||
            conditionNode.field === 'gid') &&
          (o.value === 'starts_with' || o.value === 'ends_with')
        "
      >
        {{ o.label }}
      </a-select-option>
    </a-select>

    <a-input
      :value="conditionNode.value"
      @input="(e: any) => onUpdateRule(node.id, { value: e.target.value })"
      placeholder="匹配值"
      style="width: 35%"
      class="rule-input"
    />

    <a-button
      danger
      type="text"
      size="small"
      @click="() => onDeleteNode(node.id)"
      class="btn-delete"
      style="width: 8%"
    >
      <template #icon><DeleteOutlined /></template>
    </a-button>
  </div>
</template>

<style scoped>
/* Group panel styling */
.logic-group-container {
  background: #f8fbff;
  border: 1px solid #d6e4ff;
  border-radius: 6px;
  padding: 10px 14px;
  position: relative;
  transition: all 0.25s ease;
}

.logic-group-container:hover {
  background: #f0f7ff;
}

.nested-group {
  margin-top: 6px;
  margin-bottom: 6px;
}

.group-and {
  border-left: 3px solid #1677ff;
  box-shadow: inset 2px 0 0 rgba(22, 119, 255, 0.08);
}

.group-or {
  border-left: 3px solid #4096ff;
  box-shadow: inset 2px 0 0 rgba(64, 150, 255, 0.08);
}

/* Header controls */
.group-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.group-radio-toggle :deep(.ant-radio-button-wrapper) {
  height: 24px;
  line-height: 22px;
  font-size: 11px;
  padding: 0 8px;
  background-color: #ffffff !important;
  border-color: #d9d9d9 !important;
  color: #475569 !important;
}

.group-radio-toggle :deep(.ant-radio-button-wrapper-checked) {
  background-color: #1677ff !important;
  color: #ffffff !important;
  border-color: #1677ff !important;
}

.group-and .group-radio-toggle :deep(.ant-radio-button-wrapper-checked) {
  background-color: #1677ff !important;
  border-color: #1677ff !important;
}

.group-or .group-radio-toggle :deep(.ant-radio-button-wrapper-checked) {
  background-color: #4096ff !important;
  border-color: #4096ff !important;
}

.btn-action {
  color: #64748b !important;
  font-size: 11px;
  height: 24px;
  padding: 0 6px;
}
.btn-action:hover {
  color: #1677ff !important;
  background: #e6f4ff;
}

.btn-delete {
  color: #ef4444 !important;
  height: 24px;
  width: 24px;
  padding: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.btn-delete:hover {
  background: #fff1f0;
}

/* Recursive children spacing */
.group-children {
  padding-left: 12px;
  border-left: 1px dashed #d6e4ff;
}

.group-empty-placeholder {
  padding: 12px;
  color: #64748b;
  font-size: 11px;
  text-align: center;
  border: 1px dashed #d6e4ff;
  border-radius: 4px;
  background: #ffffff;
}

.children-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* Leaf condition row styling */
.logic-condition-row {
  display: flex;
  align-items: center;
  gap: 6px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  padding: 4px 8px;
  transition: all 0.2s ease;
}

.logic-condition-row:hover {
  background: #f0f7ff;
  border-color: #91caff;
}

.drag-handle-indicator {
  color: #475569;
  cursor: grab;
  padding: 0 4px;
  display: flex;
  align-items: center;
}
.drag-handle-indicator:active {
  cursor: grabbing;
}

/* Theme input overrides for matching visual grid */
.rule-select :deep(.ant-select-selector),
.rule-input {
  background-color: #ffffff !important;
  border-color: #d9d9d9 !important;
  color: #0f172a !important;
  height: 28px !important;
  line-height: 26px !important;
}

.rule-select :deep(.ant-select-selection-item) {
  line-height: 26px !important;
  font-size: 12px;
}

.rule-input {
  padding: 0 8px;
  font-size: 12px;
}
</style>
