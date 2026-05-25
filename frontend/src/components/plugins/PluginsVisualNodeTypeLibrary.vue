<script setup lang="ts">
import { computed, ref } from "vue";
import {
  ApartmentOutlined,
  ApiOutlined,
  CodeOutlined,
  DatabaseOutlined,
  ForkOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import { triggerOptions, fieldOptions } from "./constants";
import { visualWorkflowTheme } from "./theme";
import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualMapMode,
  VisualTrigger,
} from "./types";

type VisualNodeCategory = "trigger" | "condition" | "logic" | "state" | "action" | "output";
type VisualNodeTypeKind = "trigger" | "condition" | "logic_group" | "map" | "action" | "focus";

interface VisualNodeTypeItem {
  id: string;
  category: VisualNodeCategory;
  kind: VisualNodeTypeKind;
  value: string;
  title: string;
  description: string;
  badge: string;
  color: string;
  icon: any;
  ports: string;
}

const emit = defineEmits<{
  (e: "select-trigger", value: VisualTrigger): void;
  (e: "add-condition", value: VisualConditionField): void;
  (e: "add-group", value: "AND" | "OR"): void;
  (e: "set-map", value: VisualMapMode): void;
  (e: "set-action", value: VisualAction): void;
  (e: "focus-node", value: VisualFlowNodeId): void;
}>();

const query = ref("");
const activeCategory = ref<VisualNodeCategory | "all">("all");
const theme = visualWorkflowTheme;

const categories: Array<{
  value: VisualNodeCategory | "all";
  label: string;
  icon: any;
}> = [
  { value: "all", label: "All", icon: ApartmentOutlined },
  { value: "trigger", label: "Start", icon: ThunderboltOutlined },
  { value: "condition", label: "Filter", icon: ForkOutlined },
  { value: "logic", label: "Logic", icon: ApartmentOutlined },
  { value: "state", label: "State", icon: DatabaseOutlined },
  { value: "action", label: "Action", icon: SafetyCertificateOutlined },
  { value: "output", label: "Output", icon: CodeOutlined },
];

const triggerNodeTypes = triggerOptions.map<VisualNodeTypeItem>((item) => ({
  id: `trigger-${item.value}`,
  category: "trigger",
  kind: "trigger",
  value: item.value,
  title: item.value,
  description: item.label,
  badge: "START",
  color: theme.primary,
  icon: item.icon,
  ports: "event → ctx",
}));

const conditionNodeTypes = fieldOptions.map<VisualNodeTypeItem>((item) => ({
  id: `condition-${item.value}`,
  category: "condition",
  kind: "condition",
  value: item.value,
  title: item.value,
  description: item.label,
  badge: "IF",
  color: theme.primaryHover,
  icon: ForkOutlined,
  ports: "ctx → bool",
}));

const nodeTypes: VisualNodeTypeItem[] = [
  ...triggerNodeTypes,
  ...conditionNodeTypes,
  {
    id: "logic-and",
    category: "logic",
    kind: "logic_group",
    value: "AND",
    title: "AND Group",
    description: "全部子条件命中后继续向下游输出 true。",
    badge: "LOGIC",
    color: theme.primary,
    icon: ApartmentOutlined,
    ports: "bool[] → bool",
  },
  {
    id: "logic-or",
    category: "logic",
    kind: "logic_group",
    value: "OR",
    title: "OR Group",
    description: "任意子条件命中即向下游输出 true。",
    badge: "LOGIC",
    color: theme.primaryHover,
    icon: ApartmentOutlined,
    ports: "bool[] → bool",
  },
  {
    id: "map-counter",
    category: "state",
    kind: "map",
    value: "COUNTER",
    title: "Counter Map",
    description: "按 pid/uid/comm 计数，超过阈值后才触发动作。",
    badge: "BPF MAP",
    color: theme.primary,
    icon: DatabaseOutlined,
    ports: "bool → state",
  },
  {
    id: "map-blocklist",
    category: "state",
    kind: "map",
    value: "BLOCKLIST",
    title: "Blocklist Map",
    description: "声明运行时可填充的 BPF HASH 阻断表。",
    badge: "BPF MAP",
    color: theme.primary,
    icon: DatabaseOutlined,
    ports: "key → bool",
  },
  {
    id: "map-none",
    category: "state",
    kind: "map",
    value: "NONE",
    title: "No State",
    description: "不生成状态 map，只使用当前事件上下文判断。",
    badge: "BYPASS",
    color: theme.neutral,
    icon: DatabaseOutlined,
    ports: "bool → bool",
  },
  {
    id: "action-block",
    category: "action",
    kind: "action",
    value: "BLOCK",
    title: "BLOCK",
    description: "命中后返回 -EACCES，阻断 LSM 决策链。",
    badge: "DENY",
    color: theme.danger,
    icon: SafetyCertificateOutlined,
    ports: "match → verdict",
  },
  {
    id: "action-alert",
    category: "action",
    kind: "action",
    value: "ALERT",
    title: "ALERT",
    description: "只打印内核日志并保留原始系统行为。",
    badge: "LOG",
    color: theme.warning,
    icon: SafetyCertificateOutlined,
    ports: "match → log",
  },
  {
    id: "action-kill",
    category: "action",
    kind: "action",
    value: "KILL",
    title: "KILL",
    description: "命中后发送 SIGKILL，并对 LSM 入口返回拒绝。",
    badge: "SIGNAL",
    color: theme.danger,
    icon: SafetyCertificateOutlined,
    ports: "match → signal",
  },
  {
    id: "output-code",
    category: "output",
    kind: "focus",
    value: "code",
    title: "Generated C",
    description: "跳转到生成源码与编译日志面板。",
    badge: "SOURCE",
    color: theme.primaryHover,
    icon: CodeOutlined,
    ports: "graph → c",
  },
  {
    id: "output-compile",
    category: "output",
    kind: "focus",
    value: "compile",
    title: "Compile Gate",
    description: "跳转到元数据、编译注册与加载入口。",
    badge: "BUILD",
    color: theme.success,
    icon: PlayCircleOutlined,
    ports: "c → elf",
  },
];

const filteredNodeTypes = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  return nodeTypes.filter((item) => {
    const matchesCategory = activeCategory.value === "all" || item.category === activeCategory.value;
    const matchesQuery =
      !keyword ||
      item.title.toLowerCase().includes(keyword) ||
      item.description.toLowerCase().includes(keyword) ||
      item.badge.toLowerCase().includes(keyword) ||
      item.value.toLowerCase().includes(keyword);
    return matchesCategory && matchesQuery;
  });
});

const emitNodeType = (item: VisualNodeTypeItem) => {
  if (item.kind === "trigger") {
    emit("select-trigger", item.value as VisualTrigger);
  } else if (item.kind === "condition") {
    emit("add-condition", item.value as VisualConditionField);
  } else if (item.kind === "logic_group") {
    emit("add-group", item.value as "AND" | "OR");
  } else if (item.kind === "map") {
    emit("set-map", item.value as VisualMapMode);
  } else if (item.kind === "action") {
    emit("set-action", item.value as VisualAction);
  } else if (item.kind === "focus") {
    emit("focus-node", item.value as VisualFlowNodeId);
  }
};

const handleDragStart = (event: DragEvent, item: VisualNodeTypeItem) => {
  if (!event.dataTransfer) return;
  event.dataTransfer.setData(
    "text/plain",
    JSON.stringify({ category: item.kind, value: item.value })
  );
  event.dataTransfer.effectAllowed = "move";
};
</script>

<template>
  <div class="dify-node-library">
    <div class="library-header">
      <div>
        <h4>节点类型库</h4>
        <span>Dify-style Node Types</span>
      </div>
      <a-tag color="blue">{{ filteredNodeTypes.length }}</a-tag>
    </div>

    <a-input
      v-model:value="query"
      size="small"
      allow-clear
      placeholder="搜索 hook / map / action"
      class="node-search"
    >
      <template #prefix><SearchOutlined /></template>
    </a-input>

    <div class="category-tabs">
      <button
        v-for="category in categories"
        :key="category.value"
        type="button"
        class="category-tab"
        :class="{ active: activeCategory === category.value }"
        @click="activeCategory = category.value"
      >
        <component :is="category.icon" />
        <span>{{ category.label }}</span>
      </button>
    </div>

    <div class="node-type-list">
      <button
        v-for="item in filteredNodeTypes"
        :key="item.id"
        type="button"
        draggable="true"
        class="node-type-card"
        :style="{ '--node-type-color': item.color }"
        @click="emitNodeType(item)"
        @dragstart="handleDragStart($event, item)"
      >
        <span class="node-icon"><component :is="item.icon" /></span>
        <span class="node-main">
          <span class="node-title-row">
            <strong>{{ item.title }}</strong>
            <code>{{ item.badge }}</code>
          </span>
          <span class="node-desc">{{ item.description }}</span>
          <span class="node-ports">
            <ApiOutlined /> {{ item.ports }}
          </span>
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.dify-node-library {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
  background: transparent;
  border: 0;
  border-radius: 0;
  padding: 0;
  color: #0f172a;
  box-shadow: none;
  scrollbar-color: rgba(22, 119, 255, 0.45) #e6f4ff;
}

.library-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 10px;
}

.library-header h4 {
  margin: 0;
  color: #0f172a;
  font-size: 13px;
}

.library-header span {
  color: #64748b;
  font-size: 11px;
}

.node-search {
  margin-bottom: 10px;
}

.category-tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
  margin-bottom: 10px;
}

.category-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-height: 28px;
  border: 1px solid #d6e4ff;
  border-radius: 7px;
  background: #f8fbff;
  color: #475569;
  font-size: 10px;
  cursor: pointer;
}

.category-tab.active {
  border-color: #1677ff;
  color: #0958d9;
  background: #e6f4ff;
}

.node-type-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding-right: 2px;
}

.node-type-card {
  width: 100%;
  display: flex;
  align-items: flex-start;
  gap: 9px;
  padding: 10px;
  border: 1px solid #e2e8f0;
  border-left: 3px solid var(--node-type-color);
  border-radius: 9px;
  background: #ffffff;
  color: inherit;
  text-align: left;
  cursor: grab;
  transition: transform 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.node-type-card:hover {
  transform: translateX(2px);
  border-color: var(--node-type-color);
  background: #f0f7ff;
}

.node-type-card:active {
  cursor: grabbing;
}

.node-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  border-radius: 8px;
  color: var(--node-type-color);
  background: color-mix(in srgb, var(--node-type-color) 14%, transparent);
}

.node-main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.node-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.node-title-row strong {
  color: #0f172a;
  font-size: 12px;
  line-height: 1.1;
}

.node-title-row code {
  color: var(--node-type-color);
  font-size: 9px;
  white-space: nowrap;
}

.node-desc {
  color: #64748b;
  font-size: 10.5px;
  line-height: 1.35;
}

.node-ports {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #1677ff;
  font-size: 10px;
}

:deep(.ant-input) {
  background: #ffffff;
  border-color: #d9d9d9;
  color: #0f172a;
}

:deep(.ant-input-prefix) {
  color: #64748b;
}
</style>
