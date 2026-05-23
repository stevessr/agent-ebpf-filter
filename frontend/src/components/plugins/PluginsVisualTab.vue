<script setup lang="ts">
import {
  ref,
  computed,
  watch,
  onMounted,
  onBeforeUnmount,
  h,
  nextTick,
  useTemplateRef,
} from "vue";
import { message, Modal } from "ant-design-vue";
import {
  ThunderboltOutlined,
  AlertOutlined,
  SafetyCertificateOutlined,
  CloseCircleOutlined,
} from "@ant-design/icons-vue";
import { usePlugins } from "../../composables/usePlugins";

import PluginsVisualAiPanel from "./PluginsVisualAiPanel.vue";
import PluginsVisualMapPanel from "./PluginsVisualMapPanel.vue";
import PluginsVisualCodePanel from "./PluginsVisualCodePanel.vue";
import PluginsVisualConditionTree from "./PluginsVisualConditionTree.vue";
import PluginsVisualFlowCanvas from "./PluginsVisualFlowCanvas.vue";
import PluginsVisualNodeInspector from "./PluginsVisualNodeInspector.vue";
import PluginsVisualNodeTypeLibrary from "./PluginsVisualNodeTypeLibrary.vue";
import PluginsVisualRecipePanel from "./PluginsVisualRecipePanel.vue";
import PluginsVisualSchematic from "./PluginsVisualSchematic.vue";
import { triggerOptions } from "./constants";
import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualLogicNode,
  VisualLogicGroup,
  VisualCondition,
  VisualMapKey,
  VisualMapMode,
  VisualHiddenNodeStates,
  VisualNodeLayout,
  VisualRecipe,
  VisualTrigger,
  VisualValidationIssue,
  VisualWireId,
  VisualWireStates,
  VisualWorkspaceSnapshot,
} from "./types";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

const trigger = ref<VisualTrigger>("process");

const logicRoot = ref<VisualLogicGroup>({
  id: "root",
  type: "AND",
  children: [
    {
      id: "cond-init",
      type: "CONDITION",
      field: "comm",
      operator: "==",
      value: "nc",
    },
  ],
});

const action = ref<VisualAction>("BLOCK");

// Recursive logic tree helpers
const countConditions = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children) return 0;
  return node.children.reduce((sum, child) => sum + countConditions(child), 0);
};

const getTreeDepth = (node: VisualLogicNode): number => {
  if (node.type === "CONDITION") return 1;
  if (!node.children || node.children.length === 0) return 1;
  return 1 + Math.max(...node.children.map(getTreeDepth));
};

const findNodeAndMutate = (
  root: VisualLogicGroup,
  targetId: string,
  mutateFn: (parent: VisualLogicGroup, index: number) => void
): boolean => {
  if (root.id === targetId) return false;
  for (let i = 0; i < root.children.length; i++) {
    const child = root.children[i];
    if (child.id === targetId) {
      mutateFn(root, i);
      return true;
    }
    if (child.type === "AND" || child.type === "OR") {
      const found = findNodeAndMutate(child, targetId, mutateFn);
      if (found) return true;
    }
  }
  return false;
};

const findNodeById = (root: VisualLogicNode, targetId: string): VisualLogicNode | null => {
  if (root.id === targetId) return root;
  if (root.type === "AND" || root.type === "OR") {
    if (root.children) {
      for (const child of root.children) {
        const found = findNodeById(child, targetId);
        if (found) return found;
      }
    }
  }
  return null;
};

const onDeleteNode = (id: string) => {
  if (id === "root") return;
  findNodeAndMutate(logicRoot.value, id, (parent, idx) => {
    parent.children.splice(idx, 1);
  });
};

const onAddRule = (groupId: string, field?: string) => {
  const currentCount = countConditions(logicRoot.value);
  if (currentCount >= 8) {
    message.warning("为了防止 eBPF Verifier 复杂度过高而加载失败，图形化条件最多限制为 8 个");
    return;
  }
  const targetGroup = findNodeById(logicRoot.value, groupId);
  if (targetGroup && (targetGroup.type === "AND" || targetGroup.type === "OR")) {
    const id = `cond-${Math.random().toString(36).substr(2, 9)}`;
    const fieldValue = isVisualConditionField(field) ? field : "comm";
    targetGroup.children.push({
      id,
      type: "CONDITION",
      field: fieldValue,
      operator: "==",
      value: "",
    });
  }
};

const onAddGroup = (groupId: string, type: "AND" | "OR") => {
  const targetGroup = findNodeById(logicRoot.value, groupId);
  if (targetGroup && (targetGroup.type === "AND" || targetGroup.type === "OR")) {
    const id = `group-${Math.random().toString(36).substr(2, 9)}`;
    targetGroup.children.push({
      id,
      type,
      children: [],
    });
  }
};

const onUpdateRule = (ruleId: string, updated: Partial<VisualCondition>) => {
  const targetNode = findNodeById(logicRoot.value, ruleId);
  if (targetNode && targetNode.type === "CONDITION") {
    Object.assign(targetNode, updated);
  }
};

const onUpdateGroupType = (groupId: string, type: "AND" | "OR") => {
  const targetNode = findNodeById(logicRoot.value, groupId);
  if (targetNode && (targetNode.type === "AND" || targetNode.type === "OR")) {
    targetNode.type = type;
  }
};

// Low-Code Stateful Map configurations
const mapMode = ref<VisualMapMode>("NONE");
const mapKey = ref<VisualMapKey>("pid");
const mapLimit = ref<number>(10);

// AI Copilot Helper configurations
const aiPrompt = ref("");

const pluginId = ref("visual-plugin-custom-block");
const pluginName = ref("可视化流插件(custom-block)");
const description = ref("利用图形化流式积木拼装自动生成的内核级 eBPF 拦截器。");

const compiling = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");
const isCompiled = ref(false);
const autosaveLabel = ref("本地草稿未加载");
const undoStack = ref<VisualWorkspaceSnapshot[]>([]);
const redoStack = ref<VisualWorkspaceSnapshot[]>([]);
const lastHistoryJson = ref("");
const isHistoryApplying = ref(false);
const maxHistoryDepth = 40;

const createDefaultNodeLayout = (): VisualNodeLayout => ({
  trigger: { x: 24, y: 38 },
  condition: { x: 196, y: 38 },
  map: { x: 368, y: 38 },
  action: { x: 540, y: 38 },
  code: { x: 368, y: 176 },
  compile: { x: 540, y: 176 },
});

const visualWireIds: VisualWireId[] = [
  "trigger-condition",
  "condition-map",
  "map-action",
  "condition-code",
  "map-code",
  "action-compile",
  "code-compile",
];

const visualWireLabels: Record<VisualWireId, string> = {
  "trigger-condition": "Trigger → Condition",
  "condition-map": "Condition → Map",
  "map-action": "Map → Action",
  "condition-code": "Condition → Code",
  "map-code": "Map → Code",
  "action-compile": "Action → Compile",
  "code-compile": "Code → Compile",
};

const visualWireEndpoints: Record<
  VisualWireId,
  { from: VisualFlowNodeId; to: VisualFlowNodeId }
> = {
  "trigger-condition": { from: "trigger", to: "condition" },
  "condition-map": { from: "condition", to: "map" },
  "map-action": { from: "map", to: "action" },
  "condition-code": { from: "condition", to: "code" },
  "map-code": { from: "map", to: "code" },
  "action-compile": { from: "action", to: "compile" },
  "code-compile": { from: "code", to: "compile" },
};

const visualFlowNodeIds: VisualFlowNodeId[] = [
  "trigger",
  "condition",
  "map",
  "action",
  "code",
  "compile",
];

const createDefaultWireStates = (): Record<VisualWireId, boolean> => ({
  "trigger-condition": true,
  "condition-map": true,
  "map-action": true,
  "condition-code": true,
  "map-code": true,
  "action-compile": true,
  "code-compile": true,
});

const createDefaultHiddenNodes = (): VisualHiddenNodeStates => ({});

const mergeWireStates = (states?: VisualWireStates): Record<VisualWireId, boolean> => {
  const merged = createDefaultWireStates();
  if (!states) return merged;
  visualWireIds.forEach((id) => {
    if (typeof states[id] === "boolean") {
      merged[id] = states[id] as boolean;
    }
  });
  return merged;
};

const nodeLayout = ref<VisualNodeLayout>(createDefaultNodeLayout());
const wireStates = ref<VisualWireStates>(createDefaultWireStates());
const hiddenFlowNodes = ref<VisualHiddenNodeStates>(createDefaultHiddenNodes());
const activeFlowNode = ref<VisualFlowNodeId>("trigger");
const designerSubtab = ref<"dify" | "map" | "nlp" | "source">("dify");
type FloatingDock = "left" | "right";
const floatingPanelEdgeMargin = 18;
const getInitialFloatingX = (dock: FloatingDock, width: number) => {
  const viewportWidth =
    typeof window === "undefined" ? 1280 : Math.max(480, window.innerWidth);
  return dock === "left"
    ? floatingPanelEdgeMargin
    : Math.max(floatingPanelEdgeMargin, viewportWidth - width - floatingPanelEdgeMargin);
};
const nodeLibraryVisible = ref(true);
const nodeLibraryDock = ref<FloatingDock>("left");
const nodeLibraryPosition = ref({
  x: getInitialFloatingX("left", 320),
  y: 92,
});
const nodeLibraryDragging = ref<{
  startX: number;
  startY: number;
  originX: number;
  originY: number;
} | null>(null);
const recipePanelVisible = ref(true);
const recipePanelDock = ref<FloatingDock>("right");
const recipePanelPosition = ref({
  x: getInitialFloatingX("right", 360),
  y: 92,
});
const recipePanelDragging = ref<{
  startX: number;
  startY: number;
  originX: number;
  originY: number;
} | null>(null);
const recipePanelEdgeMargin = 18;
const recipePanelSnapDelayMs = 180;
const nodeLibrarySnapDelayMs = 180;
let recipePanelHideTimer: number | null = null;
let nodeLibraryHideTimer: number | null = null;
const triggerBlockRef = useTemplateRef<HTMLElement>("triggerBlock");
const conditionBlockRef = useTemplateRef<HTMLElement>("conditionBlock");
const mapBlockRef = useTemplateRef<HTMLElement>("mapBlock");
const actionBlockRef = useTemplateRef<HTMLElement>("actionBlock");
const compileBlockRef = useTemplateRef<HTMLElement>("compileBlock");
const codeBlockRef = useTemplateRef<HTMLElement>("codeBlock");
const recipeFloatingRef = useTemplateRef<HTMLElement>("recipeFloating");
const nodeLibraryFloatingRef = useTemplateRef<HTMLElement>("nodeLibraryFloating");

const flowNodeDetails: Record<VisualFlowNodeId, { label: string; focus: string }> = {
  trigger: {
    label: "Trigger Block",
    focus: "选择 LSM / kprobe / socket 等内核挂载点。",
  },
  condition: {
    label: "Condition Tree",
    focus: "编辑嵌套 AND/OR 条件树和字段匹配值。",
  },
  map: {
    label: "State Map",
    focus: "配置 COUNTER / BLOCKLIST 等 BPF Map 状态化逻辑。",
  },
  action: {
    label: "Action Block",
    focus: "设置 ALERT / BLOCK / KILL 命中动作。",
  },
  code: {
    label: "Generated C",
    focus: "查看由积木转译出的 eBPF C 源码和编译输出。",
  },
  compile: {
    label: "Compile Gate",
    focus: "确认插件元数据并执行注册、编译、加载。",
  },
};

const selectedFlowNodeDetail = computed(
  () => flowNodeDetails[activeFlowNode.value]
);

const nodeLibraryDockLabel = computed(() =>
  nodeLibraryDock.value === "left" ? "吸附右侧" : "吸附左侧"
);

const nodeLibraryHideArrow = computed(() =>
  nodeLibraryDock.value === "left" ? "‹" : "›"
);

const nodeLibraryRestoreArrow = computed(() =>
  nodeLibraryDock.value === "left" ? "›" : "‹"
);

const recipePanelDockLabel = computed(() =>
  recipePanelDock.value === "left" ? "吸附右侧" : "吸附左侧"
);

const recipeHideArrow = computed(() =>
  recipePanelDock.value === "left" ? "‹" : "›"
);

const recipeRestoreArrow = computed(() =>
  recipePanelDock.value === "left" ? "›" : "‹"
);

const recipePanelStyle = computed(() => ({
  left: `${recipePanelPosition.value.x}px`,
  top: `${recipePanelPosition.value.y}px`,
}));

const recipeTriggerStyle = computed(() => ({
  top: `${Math.max(88, Math.min(recipePanelPosition.value.y + 14, window.innerHeight - 120))}px`,
}));

const nodeLibraryStyle = computed(() => ({
  left: `${nodeLibraryPosition.value.x}px`,
  top: `${nodeLibraryPosition.value.y}px`,
}));

const nodeLibraryTriggerStyle = computed(() => ({
  top: `${Math.max(
    88,
    Math.min(nodeLibraryPosition.value.y + 14, window.innerHeight - 120)
  )}px`,
}));

const clampNumber = (value: number, min: number, max: number) =>
  Math.min(max, Math.max(min, value));

const getRecipePanelSize = () => {
  const element = recipeFloatingRef.value;
  return {
    width: element?.offsetWidth || 360,
    height: element?.offsetHeight || 520,
  };
};

const getNodeLibrarySize = () => {
  const element = nodeLibraryFloatingRef.value;
  return {
    width: element?.offsetWidth || 320,
    height: element?.offsetHeight || 560,
  };
};

const clearRecipePanelHideTimer = () => {
  if (recipePanelHideTimer === null) return;
  window.clearTimeout(recipePanelHideTimer);
  recipePanelHideTimer = null;
};

const clearNodeLibraryHideTimer = () => {
  if (nodeLibraryHideTimer === null) return;
  window.clearTimeout(nodeLibraryHideTimer);
  nodeLibraryHideTimer = null;
};

const getRecipePanelDockX = (dock: "left" | "right", width: number) =>
  dock === "left"
    ? recipePanelEdgeMargin
    : Math.max(recipePanelEdgeMargin, window.innerWidth - width - recipePanelEdgeMargin);

const isRecipePanelAtDock = (dock: "left" | "right", width: number) =>
  Math.abs(recipePanelPosition.value.x - getRecipePanelDockX(dock, width)) <= 2;

const getNodeLibraryDockX = (dock: FloatingDock, width: number) =>
  dock === "left"
    ? floatingPanelEdgeMargin
    : Math.max(floatingPanelEdgeMargin, window.innerWidth - width - floatingPanelEdgeMargin);

const isNodeLibraryAtDock = (dock: FloatingDock, width: number) =>
  Math.abs(nodeLibraryPosition.value.x - getNodeLibraryDockX(dock, width)) <= 2;

const snapNodeLibraryTo = (dock: FloatingDock) => {
  const { width, height } = getNodeLibrarySize();
  nodeLibraryDock.value = dock;
  nodeLibraryPosition.value = {
    x: getNodeLibraryDockX(dock, width),
    y: clampNumber(
      nodeLibraryPosition.value.y,
      floatingPanelEdgeMargin,
      Math.max(
        floatingPanelEdgeMargin,
        window.innerHeight - Math.min(height, window.innerHeight - 128) - floatingPanelEdgeMargin
      )
    ),
  };
};

const toggleNodeLibraryDock = () => {
  snapNodeLibraryTo(nodeLibraryDock.value === "left" ? "right" : "left");
};

const snapRecipePanelTo = (dock: "left" | "right") => {
  const { width, height } = getRecipePanelSize();
  recipePanelDock.value = dock;
  recipePanelPosition.value = {
    x: getRecipePanelDockX(dock, width),
    y: clampNumber(
      recipePanelPosition.value.y,
      recipePanelEdgeMargin,
      Math.max(
        recipePanelEdgeMargin,
        window.innerHeight - Math.min(height, window.innerHeight - 128) - recipePanelEdgeMargin
      )
    ),
  };
};

const toggleRecipePanelDock = () => {
  snapRecipePanelTo(recipePanelDock.value === "left" ? "right" : "left");
};

const handleRecipePanelPointerMove = (event: PointerEvent) => {
  if (!recipePanelDragging.value) return;
  const margin = 12;
  const { width, height } = getRecipePanelSize();
  recipePanelPosition.value = {
    x: clampNumber(
      recipePanelDragging.value.originX + event.clientX - recipePanelDragging.value.startX,
      margin,
      Math.max(margin, window.innerWidth - width - margin)
    ),
    y: clampNumber(
      recipePanelDragging.value.originY + event.clientY - recipePanelDragging.value.startY,
      margin,
      Math.max(margin, window.innerHeight - Math.min(height, window.innerHeight - 128) - margin)
    ),
  };
};

const stopRecipePanelDragging = () => {
  if (recipePanelDragging.value) {
    const snapThreshold = 96;
    const { width } = getRecipePanelSize();
    if (recipePanelPosition.value.x <= snapThreshold) {
      snapRecipePanelTo("left");
    } else if (recipePanelPosition.value.x + width >= window.innerWidth - snapThreshold) {
      snapRecipePanelTo("right");
    } else {
      recipePanelDock.value =
        recipePanelPosition.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
    }
  }
  recipePanelDragging.value = null;
  window.removeEventListener("pointermove", handleRecipePanelPointerMove);
  window.removeEventListener("pointerup", stopRecipePanelDragging);
};

const startRecipePanelDragging = (event: PointerEvent) => {
  clearRecipePanelHideTimer();
  recipePanelDragging.value = {
    startX: event.clientX,
    startY: event.clientY,
    originX: recipePanelPosition.value.x,
    originY: recipePanelPosition.value.y,
  };
  window.addEventListener("pointermove", handleRecipePanelPointerMove);
  window.addEventListener("pointerup", stopRecipePanelDragging);
};

const hideRecipePanel = () => {
  const { width } = getRecipePanelSize();
  const nearestDock =
    recipePanelPosition.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
  const wasAlreadyAtDock = isRecipePanelAtDock(nearestDock, width);
  clearRecipePanelHideTimer();
  snapRecipePanelTo(nearestDock);
  recipePanelHideTimer = window.setTimeout(
    () => {
      recipePanelVisible.value = false;
      recipePanelHideTimer = null;
    },
    wasAlreadyAtDock ? 0 : recipePanelSnapDelayMs
  );
};

const handleNodeLibraryPointerMove = (event: PointerEvent) => {
  if (!nodeLibraryDragging.value) return;
  const margin = 12;
  const { width, height } = getNodeLibrarySize();
  nodeLibraryPosition.value = {
    x: clampNumber(
      nodeLibraryDragging.value.originX + event.clientX - nodeLibraryDragging.value.startX,
      margin,
      Math.max(margin, window.innerWidth - width - margin)
    ),
    y: clampNumber(
      nodeLibraryDragging.value.originY + event.clientY - nodeLibraryDragging.value.startY,
      margin,
      Math.max(margin, window.innerHeight - Math.min(height, window.innerHeight - 128) - margin)
    ),
  };
};

const stopNodeLibraryDragging = () => {
  if (nodeLibraryDragging.value) {
    const snapThreshold = 96;
    const { width } = getNodeLibrarySize();
    if (nodeLibraryPosition.value.x <= snapThreshold) {
      snapNodeLibraryTo("left");
    } else if (nodeLibraryPosition.value.x + width >= window.innerWidth - snapThreshold) {
      snapNodeLibraryTo("right");
    } else {
      nodeLibraryDock.value =
        nodeLibraryPosition.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
    }
  }
  nodeLibraryDragging.value = null;
  window.removeEventListener("pointermove", handleNodeLibraryPointerMove);
  window.removeEventListener("pointerup", stopNodeLibraryDragging);
};

const startNodeLibraryDragging = (event: PointerEvent) => {
  clearNodeLibraryHideTimer();
  nodeLibraryDragging.value = {
    startX: event.clientX,
    startY: event.clientY,
    originX: nodeLibraryPosition.value.x,
    originY: nodeLibraryPosition.value.y,
  };
  window.addEventListener("pointermove", handleNodeLibraryPointerMove);
  window.addEventListener("pointerup", stopNodeLibraryDragging);
};

const hideNodeLibrary = () => {
  const { width } = getNodeLibrarySize();
  const nearestDock =
    nodeLibraryPosition.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
  const wasAlreadyAtDock = isNodeLibraryAtDock(nearestDock, width);
  clearNodeLibraryHideTimer();
  snapNodeLibraryTo(nearestDock);
  nodeLibraryHideTimer = window.setTimeout(
    () => {
      nodeLibraryVisible.value = false;
      nodeLibraryHideTimer = null;
    },
    wasAlreadyAtDock ? 0 : nodeLibrarySnapDelayMs
  );
};

const showNodeLibrary = () => {
  clearNodeLibraryHideTimer();
  nodeLibraryVisible.value = true;
};

const resetNodeLayout = () => {
  nodeLayout.value = createDefaultNodeLayout();
  message.success("已恢复低代码节点画布自动布局");
};

const nodeWireIds = (node: VisualFlowNodeId) =>
  visualWireIds.filter((id) => {
    const endpoint = visualWireEndpoints[id];
    return endpoint.from === node || endpoint.to === node;
  });

const resetWireStates = () => {
  wireStates.value = createDefaultWireStates();
  message.success("已重新连接全部低代码流程线缆");
};

const restoreFlowNode = (node: VisualFlowNodeId, reconnect = true) => {
  if (!hiddenFlowNodes.value[node]) return;
  const nextHidden = { ...hiddenFlowNodes.value };
  delete nextHidden[node];
  hiddenFlowNodes.value = nextHidden;
  if (reconnect) {
    const nextWires = { ...mergeWireStates(wireStates.value) };
    nodeWireIds(node).forEach((wireId) => {
      const endpoint = visualWireEndpoints[wireId];
      if (!nextHidden[endpoint.from] && !nextHidden[endpoint.to]) {
        nextWires[wireId] = true;
      }
    });
    wireStates.value = nextWires;
  }
};

const handleDeleteFlowNode = (node: VisualFlowNodeId) => {
  if (node === "trigger" || node === "compile") {
    message.warning("Trigger 入口和 Compile 出口是流程骨架，不能删除");
    return;
  }
  hiddenFlowNodes.value = {
    ...hiddenFlowNodes.value,
    [node]: true,
  };
  const nextWires = { ...mergeWireStates(wireStates.value) };
  nodeWireIds(node).forEach((wireId) => {
    nextWires[wireId] = false;
  });
  wireStates.value = nextWires;
  if (activeFlowNode.value === node) {
    activeFlowNode.value = "trigger";
  }
  message.warning(`已从画布删除 ${flowNodeDetails[node].label}，可从左侧节点类型库重新添加`);
};

const selectFlowNode = (node: VisualFlowNodeId) => {
  activeFlowNode.value = node;
};

const focusFlowNode = async (node: VisualFlowNodeId) => {
  restoreFlowNode(node);
  activeFlowNode.value = node;
  if (node === "code") {
    designerSubtab.value = "source";
  } else if (node === "map" || node === "condition" || node === "action") {
    designerSubtab.value = "map";
  } else {
    designerSubtab.value = "dify";
  }
  await nextTick();
  const targetMap: Record<VisualFlowNodeId, HTMLElement | null> = {
    trigger: triggerBlockRef.value,
    condition: conditionBlockRef.value,
    map: mapBlockRef.value,
    action: actionBlockRef.value,
    code: codeBlockRef.value,
    compile: compileBlockRef.value,
  };
  targetMap[node]?.scrollIntoView({ behavior: "smooth", block: "center" });
};

const flowSectionClass = (node: VisualFlowNodeId) => ({
  "flow-section-active": activeFlowNode.value === node,
});

const workspaceStorageKey = "agent-ebpf-filter.visual-ebpf.workspace.v1";
const visualFieldSet = new Set<VisualConditionField>([
  "comm",
  "pid",
  "uid",
  "basename",
  "port",
  "ipv4",
  "gid",
]);
const visualMapModeSet = new Set<VisualMapMode>([
  "NONE",
  "COUNTER",
  "BLOCKLIST",
]);
const visualMapKeySet = new Set<VisualMapKey>(["uid", "pid", "comm"]);
const visualActionSet = new Set<VisualAction>(["BLOCK", "ALERT", "KILL"]);

interface CanvasNodeTypeDropPayload {
  category: string;
  value: string;
  x: number;
  y: number;
}

const isVisualConditionField = (value: unknown): value is VisualConditionField =>
  typeof value === "string" && visualFieldSet.has(value as VisualConditionField);

const canUseLocalStorage = () =>
  typeof window !== "undefined" && typeof window.localStorage !== "undefined";

const getTimeLabel = () => new Date().toLocaleTimeString();

const visualRecipes: VisualRecipe[] = [
  {
    id: "process-nc-block",
    name: "阻断 nc 执行",
    description: "最小闭环：bprm_check_security + comm/name 条件 + BLOCK。",
    tags: ["process", "LSM", "BLOCK"],
    version: 1,
    trigger: "process",
    action: "BLOCK",
    mapMode: "NONE",
    mapKey: "pid",
    mapLimit: 10,
    conditions: {
      id: "root",
      type: "AND",
      children: [
        {
          id: "recipe-nc-comm",
          type: "CONDITION",
          field: "comm",
          operator: "==",
          value: "nc",
        },
      ],
    },
  },
  {
    id: "reverse-shell-ports",
    name: "反连端口强杀",
    description: "socket_connect 上组合 comm + 多端口 OR，并用 COUNTER 限频兜底。",
    tags: ["socket", "OR", "KILL", "COUNTER"],
    version: 1,
    trigger: "socket_connect",
    action: "KILL",
    mapMode: "COUNTER",
    mapKey: "pid",
    mapLimit: 3,
    conditions: {
      id: "root",
      type: "AND",
      children: [
        {
          id: "recipe-rev-comm",
          type: "CONDITION",
          field: "comm",
          operator: "==",
          value: "nc",
        },
        {
          id: "recipe-rev-ports",
          type: "OR",
          children: [
            {
              id: "recipe-rev-port-4444",
              type: "CONDITION",
              field: "port",
              operator: "==",
              value: "4444",
            },
            {
              id: "recipe-rev-port-5555",
              type: "CONDITION",
              field: "port",
              operator: "==",
              value: "5555",
            },
          ],
        },
      ],
    },
  },
  {
    id: "ssh-key-read-protect",
    name: "SSH 私钥读取保护",
    description: "file_open 上拦截非 root 对 id_rsa / id_ed25519 的读取打开。",
    tags: ["file_open", "AND", "OR"],
    version: 1,
    trigger: "file_open",
    action: "BLOCK",
    mapMode: "NONE",
    mapKey: "pid",
    mapLimit: 10,
    conditions: {
      id: "root",
      type: "AND",
      children: [
        {
          id: "recipe-ssh-uid",
          type: "CONDITION",
          field: "uid",
          operator: "!=",
          value: "0",
        },
        {
          id: "recipe-ssh-files",
          type: "OR",
          children: [
            {
              id: "recipe-ssh-rsa",
              type: "CONDITION",
              field: "basename",
              operator: "==",
              value: "id_rsa",
            },
            {
              id: "recipe-ssh-ed25519",
              type: "CONDITION",
              field: "basename",
              operator: "==",
              value: "id_ed25519",
            },
          ],
        },
      ],
    },
  },
  {
    id: "ransomware-rename-watch",
    name: "勒索重命名审计",
    description: "inode_rename 上关注 shadow / .env / .key 等敏感名称，先告警审计。",
    tags: ["rename", "ALERT", "OR"],
    version: 1,
    trigger: "inode_rename",
    action: "ALERT",
    mapMode: "NONE",
    mapKey: "pid",
    mapLimit: 10,
    conditions: {
      id: "root",
      type: "OR",
      children: [
        {
          id: "recipe-ren-shadow",
          type: "CONDITION",
          field: "basename",
          operator: "==",
          value: "shadow",
        },
        {
          id: "recipe-ren-env",
          type: "CONDITION",
          field: "basename",
          operator: "ends_with",
          value: ".env",
        },
        {
          id: "recipe-ren-key",
          type: "CONDITION",
          field: "basename",
          operator: "ends_with",
          value: ".key",
        },
      ],
    },
  },
  {
    id: "mprotect-rwx-kill",
    name: "RWX 内存强杀",
    description: "file_mprotect 上对脚本/解释器进程启用 KILL 响应。",
    tags: ["mprotect", "KILL", "OR"],
    version: 1,
    trigger: "file_mprotect",
    action: "KILL",
    mapMode: "COUNTER",
    mapKey: "comm",
    mapLimit: 2,
    conditions: {
      id: "root",
      type: "OR",
      children: [
        {
          id: "recipe-mprot-python",
          type: "CONDITION",
          field: "comm",
          operator: "starts_with",
          value: "python",
        },
        {
          id: "recipe-mprot-node",
          type: "CONDITION",
          field: "comm",
          operator: "==",
          value: "node",
        },
        {
          id: "recipe-mprot-perl",
          type: "CONDITION",
          field: "comm",
          operator: "==",
          value: "perl",
        },
      ],
    },
  },
];

const cloneLogicRoot = (root: VisualLogicGroup): VisualLogicGroup =>
  JSON.parse(JSON.stringify(root)) as VisualLogicGroup;

const assertValidConditionTree = (node: VisualLogicNode) => {
  if (node.type === "CONDITION") {
    if (!isVisualConditionField(node.field)) {
      throw new Error(`不支持的条件字段: ${String(node.field)}`);
    }
    return;
  }
  if (node.type !== "AND" && node.type !== "OR") {
    throw new Error("逻辑分组必须是 AND 或 OR");
  }
  if (!Array.isArray(node.children)) {
    throw new Error("逻辑分组 children 必须是数组");
  }
  node.children.forEach(assertValidConditionTree);
};

const createWorkspaceSnapshot = (): VisualWorkspaceSnapshot => ({
  version: 1,
  trigger: trigger.value,
  action: action.value,
  conditions: cloneLogicRoot(logicRoot.value),
  mapMode: mapMode.value,
  mapKey: mapKey.value,
  mapLimit: mapLimit.value,
  nodeLayout: { ...nodeLayout.value },
  wireStates: { ...mergeWireStates(wireStates.value) },
  hiddenNodes: { ...hiddenFlowNodes.value },
  pluginId: pluginId.value,
  pluginName: pluginName.value,
  description: description.value,
});

const cloneWorkspaceSnapshot = (
  snapshot: VisualWorkspaceSnapshot
): VisualWorkspaceSnapshot =>
  JSON.parse(JSON.stringify(snapshot)) as VisualWorkspaceSnapshot;

const serializeWorkspaceSnapshot = (snapshot: VisualWorkspaceSnapshot) =>
  JSON.stringify(snapshot);

const applyWorkspaceSnapshot = (snapshot: VisualWorkspaceSnapshot) => {
  const validTrigger = triggerOptions.some((item) => item.value === snapshot.trigger);
  if (!validTrigger) throw new Error(`不支持的挂载点: ${snapshot.trigger}`);
  if (!snapshot.conditions || !Array.isArray(snapshot.conditions.children)) {
    throw new Error("conditions 必须是包含 children 的逻辑根节点");
  }
  assertValidConditionTree(snapshot.conditions);
  const conditionTotal = countConditions(snapshot.conditions);
  if (conditionTotal > 8) {
    throw new Error(`条件数量 ${conditionTotal} 超过 eBPF Verifier 友好上限 8`);
  }
  if (!visualMapModeSet.has(snapshot.mapMode)) {
    throw new Error(`不支持的 Map 模式: ${String(snapshot.mapMode)}`);
  }
  if (!visualMapKeySet.has(snapshot.mapKey)) {
    throw new Error(`不支持的 Map Key: ${String(snapshot.mapKey)}`);
  }

  trigger.value = snapshot.trigger;
  action.value =
    snapshot.trigger === "unlink" && snapshot.action === "BLOCK"
      ? "ALERT"
      : snapshot.action;
  logicRoot.value = cloneLogicRoot(snapshot.conditions);
  mapMode.value = snapshot.mapMode || "NONE";
  mapKey.value = snapshot.mapKey || "pid";
  mapLimit.value = Number(snapshot.mapLimit) || 10;
  if (snapshot.pluginId) pluginId.value = snapshot.pluginId;
  if (snapshot.pluginName) pluginName.value = snapshot.pluginName;
  if (snapshot.description) description.value = snapshot.description;
  nodeLayout.value = snapshot.nodeLayout
    ? { ...createDefaultNodeLayout(), ...snapshot.nodeLayout }
    : createDefaultNodeLayout();
  wireStates.value = mergeWireStates(snapshot.wireStates);
  hiddenFlowNodes.value = snapshot.hiddenNodes
    ? { ...createDefaultHiddenNodes(), ...snapshot.hiddenNodes }
    : createDefaultHiddenNodes();
};

const syncHistoryBaseline = () => {
  lastHistoryJson.value = serializeWorkspaceSnapshot(createWorkspaceSnapshot());
};

const recordWorkspaceHistory = () => {
  if (isHistoryApplying.value) return;
  const nextSnapshot = createWorkspaceSnapshot();
  const nextJson = serializeWorkspaceSnapshot(nextSnapshot);
  if (!lastHistoryJson.value) {
    lastHistoryJson.value = nextJson;
    return;
  }
  if (nextJson === lastHistoryJson.value) return;
  undoStack.value.push(JSON.parse(lastHistoryJson.value) as VisualWorkspaceSnapshot);
  if (undoStack.value.length > maxHistoryDepth) {
    undoStack.value.shift();
  }
  redoStack.value = [];
  lastHistoryJson.value = nextJson;
};

const applyHistorySnapshot = async (snapshot: VisualWorkspaceSnapshot) => {
  isHistoryApplying.value = true;
  applyWorkspaceSnapshot(cloneWorkspaceSnapshot(snapshot));
  lastHistoryJson.value = serializeWorkspaceSnapshot(createWorkspaceSnapshot());
  await nextTick();
  isHistoryApplying.value = false;
  saveWorkspaceDraft(true);
};

const undoWorkspace = async () => {
  const previous = undoStack.value.pop();
  if (!previous) {
    message.info("当前没有可撤销的积木历史");
    return;
  }
  redoStack.value.push(cloneWorkspaceSnapshot(createWorkspaceSnapshot()));
  await applyHistorySnapshot(previous);
  message.success("已撤销上一步积木编辑");
};

const redoWorkspace = async () => {
  const next = redoStack.value.pop();
  if (!next) {
    message.info("当前没有可重做的积木历史");
    return;
  }
  undoStack.value.push(cloneWorkspaceSnapshot(createWorkspaceSnapshot()));
  await applyHistorySnapshot(next);
  message.success("已重做积木编辑");
};

const selectTriggerNodeType = (value: VisualTrigger) => {
  trigger.value = value;
  void focusFlowNode("trigger");
  message.success(`已从节点类型库选择入口: ${value}`);
};

const addConditionNodeType = (value: VisualConditionField) => {
  onAddRule("root", value);
  void focusFlowNode("condition");
  message.success(`已从节点类型库添加条件: ${value}`);
};

const addLogicNodeType = (value: "AND" | "OR") => {
  onAddGroup("root", value);
  void focusFlowNode("condition");
  message.success(`已从节点类型库添加逻辑组: ${value}`);
};

const setMapNodeType = (value: VisualMapMode) => {
  mapMode.value = value;
  void focusFlowNode("map");
  message.success(`已从节点类型库设置状态节点: ${value}`);
};

const setActionNodeType = (value: VisualAction) => {
  if (trigger.value === "unlink" && value === "BLOCK") {
    message.error("unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL");
    return;
  }
  action.value = value;
  void focusFlowNode("action");
  message.success(`已从节点类型库设置动作: ${value}`);
};

const applyRecipe = (recipeId: string) => {
  const recipe = visualRecipes.find((item) => item.id === recipeId);
  if (!recipe) return;
  applyWorkspaceSnapshot(recipe);
  message.success(`已套用积木模板：${recipe.name}`);
};

const resetWorkspace = () => {
  applyRecipe("process-nc-block");
};

const exportWorkspace = async () => {
  const json = JSON.stringify(createWorkspaceSnapshot(), null, 2);
  try {
    await navigator.clipboard.writeText(json);
    message.success("当前积木工作台 JSON 已复制到剪贴板");
  } catch {
    Modal.info({
      title: "当前积木工作台 JSON",
      width: 720,
      content: h("pre", { class: "workspace-json-preview" }, json),
    });
  }
};

const importWorkspace = () => {
  const raw = window.prompt("粘贴由“导出 JSON”生成的积木工作台配置：");
  if (!raw) return;
  try {
    const snapshot = JSON.parse(raw) as VisualWorkspaceSnapshot;
    applyWorkspaceSnapshot(snapshot);
    message.success("已导入积木工作台配置");
  } catch (err: any) {
    message.error(`导入失败: ${err?.message || "JSON 格式错误"}`);
  }
};

const saveWorkspaceDraft = (silent = false) => {
  if (!canUseLocalStorage()) {
    autosaveLabel.value = "当前环境不支持 localStorage 草稿";
    return;
  }
  window.localStorage.setItem(
    workspaceStorageKey,
    JSON.stringify(createWorkspaceSnapshot())
  );
  autosaveLabel.value = `草稿已保存 ${getTimeLabel()}`;
  if (!silent) message.success("低代码积木草稿已保存到浏览器本地");
};

const clearWorkspaceDraft = () => {
  if (!canUseLocalStorage()) return;
  window.localStorage.removeItem(workspaceStorageKey);
  autosaveLabel.value = "草稿已清除；继续编辑会自动重新保存";
  message.success("已清除浏览器本地积木草稿");
};

const restoreWorkspaceDraft = () => {
  if (!canUseLocalStorage()) return;
  const raw = window.localStorage.getItem(workspaceStorageKey);
  if (!raw) {
    autosaveLabel.value = "尚无浏览器本地草稿";
    return;
  }
  try {
    applyWorkspaceSnapshot(JSON.parse(raw) as VisualWorkspaceSnapshot);
    autosaveLabel.value = `已恢复本地草稿 ${getTimeLabel()}`;
    message.info("已恢复上次未完成的低代码积木草稿");
  } catch (err: any) {
    autosaveLabel.value = "本地草稿损坏，已忽略";
    console.warn("Failed to restore visual eBPF workspace draft:", err);
  }
};

const conditionCount = computed(() => countConditions(logicRoot.value));
const treeDepth = computed(() => getTreeDepth(logicRoot.value));

const allConditions = computed(() => {
  const result: VisualCondition[] = [];
  const collect = (node: VisualLogicNode) => {
    if (node.type === "CONDITION") {
      result.push(node);
      return;
    }
    node.children?.forEach(collect);
  };
  collect(logicRoot.value);
  return result;
});

const isValidIPv4 = (value: string) => {
  const parts = value.split(".");
  return (
    parts.length === 4 &&
    parts.every((part) => {
      if (!/^\d+$/.test(part)) return false;
      const num = Number(part);
      return num >= 0 && num <= 255;
    })
  );
};

const validationIssues = computed<VisualValidationIssue[]>(() => {
  const issues: VisualValidationIssue[] = [];

  if (!/^[a-z0-9][a-z0-9-]{2,63}$/.test(pluginId.value.trim())) {
    issues.push({
      id: "plugin-id",
      severity: "error",
      title: "插件 ID 不合法",
      detail: "请使用 3-64 位小写字母、数字或中划线，且以字母/数字开头。",
    });
  }

  const currentWireStates = mergeWireStates(wireStates.value);
  const deletedNodes = visualFlowNodeIds.filter((id) => hiddenFlowNodes.value[id]);
  if (deletedNodes.length > 0) {
    issues.push({
      id: "flow-node-deleted",
      severity: "error",
      title: "低代码流程节点已删除",
      detail: `请从左侧节点类型库恢复: ${deletedNodes
        .map((id) => flowNodeDetails[id].label)
        .join("、")}。`,
    });
  }

  const disconnectedWires = visualWireIds.filter((id) => {
    const endpoint = visualWireEndpoints[id];
    if (hiddenFlowNodes.value[endpoint.from] || hiddenFlowNodes.value[endpoint.to]) {
      return false;
    }
    return !currentWireStates[id];
  });
  if (disconnectedWires.length > 0) {
    issues.push({
      id: "wire-flow-disconnected",
      severity: "error",
      title: "低代码流程线缆未闭合",
      detail: `请在画布中重新连接: ${disconnectedWires
        .map((id) => visualWireLabels[id])
        .join("、")}。`,
    });
  }

  if (conditionCount.value === 0) {
    issues.push({
      id: "no-conditions",
      severity: "error",
      title: "条件积木为空",
      detail: "至少需要一个 CONDITION 积木，否则生成的规则没有明确匹配边界。",
    });
  }

  if (conditionCount.value > 8) {
    issues.push({
      id: "condition-limit",
      severity: "error",
      title: "条件积木过多",
      detail: "当前限制为 8 个条件，避免 eBPF Verifier 复杂度过高。",
    });
  }

  if (treeDepth.value > 5) {
    issues.push({
      id: "tree-depth",
      severity: "warning",
      title: "逻辑嵌套较深",
      detail: "建议把深层 AND/OR 拆成多个插件，便于审计和 verifier 排错。",
    });
  }

  if (trigger.value === "unlink" && action.value === "BLOCK") {
    issues.push({
      id: "unlink-block",
      severity: "error",
      title: "unlink Kprobe 不支持 BLOCK",
      detail: "Kprobe 只做观测/信号动作，不能直接改变 LSM 返回值；请改用 ALERT 或 KILL。",
    });
  }

  if (mapMode.value === "COUNTER" && (!Number.isFinite(mapLimit.value) || mapLimit.value < 1)) {
    issues.push({
      id: "map-limit",
      severity: "error",
      title: "COUNTER 阈值无效",
      detail: "计数器模式需要大于 0 的最大命中次数。",
    });
  }

  if (mapMode.value === "BLOCKLIST") {
    issues.push({
      id: "blocklist-runtime",
      severity: "warning",
      title: "BLOCKLIST 需要运行时填表",
      detail: "当前 UI 只声明 map 和查表逻辑；后续还需要通过 bpftool/API 写入具体 key。",
    });
  }

  allConditions.value.forEach((condition, index) => {
    const label = `条件 #${index + 1}`;
    const value = condition.value.trim();
    if (!value) {
      issues.push({
        id: `${condition.id}-empty`,
        severity: "error",
        title: `${label} 缺少匹配值`,
        detail: "空值会退化为无边界匹配，已阻止编译。",
      });
      return;
    }

    if (/"|\\|\r|\n/.test(value)) {
      issues.push({
        id: `${condition.id}-unsafe`,
        severity: "error",
        title: `${label} 包含不安全 C 字符`,
        detail: "当前可视化生成器暂不接受引号、反斜杠或换行，请改用纯文本匹配值。",
      });
    }

    if (trigger.value !== "socket_connect" && (condition.field === "port" || condition.field === "ipv4")) {
      issues.push({
        id: `${condition.id}-socket-field`,
        severity: "error",
        title: `${label} 字段不适用于当前 Hook`,
        detail: "port / ipv4 仅在 socket_connect 挂载点中有可用上下文。",
      });
    }

    if (trigger.value === "unlink" && condition.field === "basename") {
      issues.push({
        id: `${condition.id}-unlink-basename`,
        severity: "error",
        title: `${label} 无法读取 unlink basename`,
        detail: "当前 unlink 走 kprobe/do_unlinkat，生成器没有安全读取 dentry 名称。",
      });
    }

    if (condition.field === "pid" || condition.field === "uid" || condition.field === "gid") {
      if (!/^\d+$/.test(value)) {
        issues.push({
          id: `${condition.id}-numeric`,
          severity: "error",
          title: `${label} 需要数字值`,
          detail: `${condition.field} 条件只能填写非负整数。`,
        });
      }
    }

    if (condition.field === "port") {
      const port = Number(value);
      if (!/^\d+$/.test(value) || port < 1 || port > 65535) {
        issues.push({
          id: `${condition.id}-port`,
          severity: "error",
          title: `${label} 端口范围无效`,
          detail: "目标端口必须位于 1..65535。",
        });
      }
    }

    if (condition.field === "ipv4" && !isValidIPv4(value)) {
      issues.push({
        id: `${condition.id}-ipv4`,
        severity: "error",
        title: `${label} IPv4 格式无效`,
        detail: "请使用类似 203.0.113.10 的点分十进制地址。",
      });
    }
  });

  if (action.value === "KILL") {
    issues.push({
      id: "kill-action",
      severity: "info",
      title: "KILL 会发送 SIGKILL",
      detail: "生成的程序会调用 bpf_send_signal(9)，建议先用 ALERT 模式演练命中范围。",
    });
  }

  return issues;
});

const validationErrors = computed(() =>
  validationIssues.value.filter((issue) => issue.severity === "error")
);

const isWorkspaceValid = computed(() => validationErrors.value.length === 0);



const ipToHex = (ip: string): string => {
  const parts = ip.split(".").map((p) => parseInt(p, 10));
  if (parts.length !== 4 || parts.some(isNaN)) return "0x00000000";
  return (
    "0x" +
    parts
      .map((p) => Math.min(255, Math.max(0, p)).toString(16).padStart(2, "0"))
      .join("")
  );
};

// Dynamic eBPF C Code Compiler Transpiler
const generatedBpfCode = computed(() => {
  const isKprobeUnlink = trigger.value === "unlink";

  const isKill = action.value === "KILL";
  const returnValLsm = (action.value === "BLOCK" || action.value === "KILL") ? "-EACCES" : "0";
  const logPrefix = isKill ? "Killed" : (action.value === "BLOCK" ? "Blocked" : "Alert");

  let headers = `#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";
#define EACCES 13

#ifndef bpf_ntohs
#define bpf_ntohs(x) __builtin_bswap16(x)
#endif
#ifndef bpf_ntohl
#define bpf_ntohl(x) __builtin_bswap32(x)
#endif

static __always_inline int strcmp_const(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s1[i] != s2[i]) return 1;
        if (s1[i] == '\\0') return 0;
    }
    return 0;
}

static __always_inline int str_starts_with(const char *s1, const char *s2, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s2[i] == '\\0') return 1;
        if (s1[i] != s2[i]) return 0;
    }
    return 0;
}

static __always_inline int get_str_len(const char *s, int max_len) {
    for (int i = 0; i < max_len; i++) {
        if (s[i] == '\\0') return i;
    }
    return max_len;
}

static __always_inline int str_ends_with(const char *s1, int s1_len, const char *s2, int s2_len) {
    if (s1_len < s2_len) return 0;
    int offset = s1_len - s2_len;
    for (int i = 0; i < 64; i++) {
        if (i >= s2_len) break;
        if (s1[offset + i] != s2[i]) return 0;
    }
    return 1;
}
`;

  let body = "";

  // 1. Hook function header
  if (trigger.value === "process") {
    body = `
SEC("lsm/bprm_check_security")
int BPF_PROG(visual_custom_plugin, struct linux_binprm *bprm, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *exec_name = BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (exec_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), exec_name);
    }
`;
  } else if (trigger.value === "file_open") {
    body = `
SEC("lsm/file_open")
int BPF_PROG(visual_custom_plugin, struct file *file, int ret) {
    if (ret != 0) return ret;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (
    trigger.value === "mkdir" ||
    trigger.value === "file_create" ||
    trigger.value === "rmdir" ||
    trigger.value === "symlink"
  ) {
    const secName =
      trigger.value === "mkdir"
        ? "lsm/inode_mkdir"
        : trigger.value === "file_create"
        ? "lsm/inode_create"
        : trigger.value === "rmdir"
        ? "lsm/inode_rmdir"
        : "lsm/inode_symlink";

    const funcArgs =
      trigger.value === "mkdir"
        ? "struct inode *dir, struct dentry *dentry, umode_t mode"
        : trigger.value === "file_create"
        ? "struct inode *dir, struct dentry *dentry, umode_t mode"
        : trigger.value === "rmdir"
        ? "struct inode *dir, struct dentry *dentry"
        : "struct inode *dir, struct dentry *dentry, const char *old_name";

    body = `
SEC("${secName}")
int BPF_PROG(visual_custom_plugin, ${funcArgs}) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger.value === "socket_connect") {
    body = `
SEC("lsm/socket_connect")
int BPF_PROG(visual_custom_plugin, struct socket *sock, struct sockaddr *address, int addrlen) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    short family = 0;
    if (address) {
        bpf_probe_read_kernel(&family, sizeof(family), &address->sa_family);
    }

    u16 dst_port = 0;
    u32 dst_ipv4 = 0;
    if (family == 2) { // AF_INET
        struct sockaddr_in addr_in = {};
        bpf_probe_read_kernel(&addr_in, sizeof(addr_in), address);
        dst_port = bpf_ntohs(addr_in.sin_port);
        dst_ipv4 = bpf_ntohl(addr_in.sin_addr.s_addr);
    }
`;
  } else if (trigger.value === "inode_mknod") {
    body = `
SEC("lsm/inode_mknod")
int BPF_PROG(visual_custom_plugin, struct inode *dir, struct dentry *dentry, umode_t mode, dev_t dev) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (trigger.value === "file_mprotect") {
    body = `
SEC("lsm/file_mprotect")
int BPF_PROG(visual_custom_plugin, struct vm_area_struct *vma, unsigned long reqprot, unsigned long prot, int ret) {
    if (ret != 0) return ret;
    if (!vma) return 0;

    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    struct file *file = BPF_CORE_READ(vma, vm_file);
    char name_buf[64] = {};
    if (file) {
        const unsigned char *file_name = BPF_CORE_READ(file, f_path.dentry, d_name.name);
        if (file_name) {
            bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
        }
    }
`;
  } else if (trigger.value === "inode_rename") {
    body = `
SEC("lsm/inode_rename")
int BPF_PROG(visual_custom_plugin, struct inode *old_dir, struct dentry *old_dentry, struct inode *new_dir, struct dentry *new_dentry) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;

    const unsigned char *file_name = BPF_CORE_READ(old_dentry, d_name.name);
    char name_buf[64] = {};
    if (file_name) {
        bpf_probe_read_kernel_str(name_buf, sizeof(name_buf), file_name);
    }
`;
  } else if (isKprobeUnlink) {
    body = `
SEC("kprobe/do_unlinkat")
int BPF_PROG(visual_custom_plugin, struct pt_regs *ctx) {
    char comm[16] = {};
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    u32 uid = bpf_get_current_uid_gid() & 0xffffffff;
    u32 gid = bpf_get_current_uid_gid() >> 32;
    char name_buf[64] = {}; // kprobe lacks dentry
`;
  }

  const lines: string[] = [];
  const generateNodeCExpression = (node: VisualLogicNode): string => {
    if (node.type === "CONDITION") {
      const val = (node.value || "").trim();
      if (!val) {
        return "1"; // safe default
      }
      let expr = "0";
      if (node.field === "comm") {
        if (node.operator === "==") {
          expr = `strcmp_const(comm, "${val}", sizeof(comm)) == 0`;
        } else if (node.operator === "!=") {
          expr = `strcmp_const(comm, "${val}", sizeof(comm)) != 0`;
        } else if (node.operator === "starts_with") {
          expr = `str_starts_with(comm, "${val}", sizeof(comm)) != 0`;
        } else if (node.operator === "ends_with") {
          expr = `str_ends_with(comm, get_str_len(comm, sizeof(comm)), "${val}", ${val.length}) != 0`;
        }
      } else if (node.field === "pid") {
        const pidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `pid == ${pidNum}`;
        else expr = `pid != ${pidNum}`;
      } else if (node.field === "uid") {
        const uidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `uid == ${uidNum}`;
        else expr = `uid != ${uidNum}`;
      } else if (node.field === "gid") {
        const gidNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `gid == ${gidNum}`;
        else expr = `gid != ${gidNum}`;
      } else if (node.field === "port") {
        const portNum = parseInt(val, 10) || 0;
        if (node.operator === "==") expr = `dst_port == ${portNum}`;
        else expr = `dst_port != ${portNum}`;
      } else if (node.field === "ipv4") {
        const hexIp = ipToHex(val);
        if (node.operator === "==") expr = `dst_ipv4 == ${hexIp}`;
        else expr = `dst_ipv4 != ${hexIp}`;
      } else if (node.field === "basename") {
        if (isKprobeUnlink) {
          expr = "0";
        } else {
          if (node.operator === "==") {
            expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) == 0`;
          } else if (node.operator === "!=") {
            expr = `strcmp_const(name_buf, "${val}", sizeof(name_buf)) != 0`;
          } else if (node.operator === "starts_with") {
            expr = `str_starts_with(name_buf, "${val}", sizeof(name_buf)) != 0`;
          } else if (node.operator === "ends_with") {
            expr = `str_ends_with(name_buf, get_str_len(name_buf, sizeof(name_buf)), "${val}", ${val.length}) != 0`;
          }
        }
      }
      const varId = node.id.replace(/[^a-zA-Z0-9]/g, "_");
      const varName = `cond_${varId}`;
      lines.push(`    u32 ${varName} = ${expr};`);
      return varName;
    } else {
      const childVarNames: string[] = [];
      if (node.children && node.children.length > 0) {
        node.children.forEach(child => {
          childVarNames.push(generateNodeCExpression(child));
        });
      }
      const varId = node.id.replace(/[^a-zA-Z0-9]/g, "_");
      const varName = `group_${varId}`;
      if (childVarNames.length === 0) {
        lines.push(`    u32 ${varName} = 1;`);
      } else {
        const op = node.type === "AND" ? "&&" : "||";
        lines.push(`    u32 ${varName} = ${childVarNames.map(name => `(${name})`).join(` ${op} `)};`);
      }
      return varName;
    }
  };

  const rootVarName = generateNodeCExpression(logicRoot.value);
  body += `\n${lines.join("\n")}\n    u32 matched = ${rootVarName};\n`;

  // Finish function body
  let mapDefinitions = "";
  let mapLookupBody = "";

  if (mapMode.value === "COUNTER") {
    if (mapKey.value === "comm") {
      mapDefinitions = `
struct block_key {
    char name[64];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct block_key);
    __type(value, u64);
} rate_limit_map SEC(".maps");
`;
      mapLookupBody = `
        struct block_key m_key = {};
        bpf_probe_read_kernel_str(m_key.name, sizeof(m_key.name), comm);
        u64 *count = bpf_map_lookup_elem(&rate_limit_map, &m_key);
        u64 init_val = 1;
        if (count) {
            __sync_fetch_and_add(count, 1);
            if (*count > ${mapLimit.value}) {
                matched = 1;
            } else {
                matched = 0;
            }
        } else {
            bpf_map_update_elem(&rate_limit_map, &m_key, &init_val, BPF_ANY);
            matched = 0;
        }
`;
    } else {
      const keyVar = mapKey.value === "uid" ? "uid" : "pid";
      mapDefinitions = `
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u64);
} rate_limit_map SEC(".maps");
`;
      mapLookupBody = `
        u32 m_key = ${keyVar};
        u64 *count = bpf_map_lookup_elem(&rate_limit_map, &m_key);
        u64 init_val = 1;
        if (count) {
            __sync_fetch_and_add(count, 1);
            if (*count > ${mapLimit.value}) {
                matched = 1;
            } else {
                matched = 0;
            }
        } else {
            bpf_map_update_elem(&rate_limit_map, &m_key, &init_val, BPF_ANY);
            matched = 0;
        }
`;
    }
  } else if (mapMode.value === "BLOCKLIST") {
    if (mapKey.value === "comm") {
      mapDefinitions = `
struct block_key {
    char name[64];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, struct block_key);
    __type(value, u32);
} blocklist_map SEC(".maps");
`;
      mapLookupBody = `
        struct block_key m_key = {};
        bpf_probe_read_kernel_str(m_key.name, sizeof(m_key.name), comm);
        u32 *is_blocked = bpf_map_lookup_elem(&blocklist_map, &m_key);
        if (is_blocked && *is_blocked) {
            matched = 1;
        } else {
            matched = 0;
        }
`;
    } else {
      const keyVar = mapKey.value === "uid" ? "uid" : "pid";
      mapDefinitions = `
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u32);
    __type(value, u32);
} blocklist_map SEC(".maps");
`;
      mapLookupBody = `
        u32 m_key = ${keyVar};
        u32 *is_blocked = bpf_map_lookup_elem(&blocklist_map, &m_key);
        if (is_blocked && *is_blocked) {
            matched = 1;
        } else {
            matched = 0;
        }
`;
    }
  }

  if (isKprobeUnlink) {
    body += `
    if (matched) {
        ${mapMode.value !== "NONE" ? `// Run stateful Map operation checks\n` + mapLookupBody.trim() + `\n\n        if (matched) {` : ""}
        bpf_printk("[Visual Plugin] matched unlink event: process %s (pid %d, uid %d, gid %d) deleted file\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        ${mapMode.value !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  } else {
    body += `
    if (matched) {
        ${mapMode.value !== "NONE" ? `// Run stateful Map operation checks\n` + mapLookupBody.trim() + `\n\n        if (matched) {` : ""}
        bpf_printk("[Visual Plugin] ${logPrefix} matched rule! process %s (pid %d, uid %d, gid %d)\\n", comm, pid, uid, gid);
        ${isKill ? "bpf_send_signal(9);\n" : ""}
        return ${returnValLsm};
        ${mapMode.value !== "NONE" ? "}" : ""}
    }
    return 0;
}
`;
  }

  return headers + mapDefinitions + body;
});

const generatedLineCount = computed(
  () => generatedBpfCode.value.split(/\r?\n/).length
);

// Watch inputs to auto-sync Manifest fields
watch(
  [trigger, logicRoot, action, mapMode, mapKey, mapLimit],
  () => {
    if (isHistoryApplying.value) {
      isCompiled.value = false;
      compileLogLocal.value = "";
      return;
    }
    // Sanitize conditions recursively
    const sanitizeNode = (node: VisualLogicNode) => {
      if (node.type === "CONDITION") {
        if (trigger.value !== "socket_connect") {
          if (node.field === "port" || node.field === "ipv4") {
            node.field = "comm";
          }
        }
        if (trigger.value === "unlink") {
          if (node.field === "basename") {
            node.field = "comm";
          }
        }
      } else if (node.children) {
        node.children.forEach(sanitizeNode);
      }
    };
    sanitizeNode(logicRoot.value);

    // Extract first condition value to make descriptive id
    const leaves: VisualCondition[] = [];
    const findLeaves = (n: VisualLogicNode) => {
      if (n.type === "CONDITION") leaves.push(n);
      else if (n.children) n.children.forEach(findLeaves);
    };
    findLeaves(logicRoot.value);

    const firstVal = leaves[0]?.value || "custom";
    const prefix = `visual-block-${trigger.value}-${firstVal.replace(
      /[^a-z0-9]/g,
      "-"
    )}`.toLowerCase();
    pluginId.value = prefix;
    pluginName.value = `积木插件(${trigger.value}-${firstVal})`;
    description.value = `由图形化积木拼装而成的内核 eBPF 过滤审计插件。入口: ${trigger.value}，Map状态: ${mapMode.value}，嵌套层数: ${getTreeDepth(logicRoot.value)}，动作: ${action.value}。`;
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { deep: true, immediate: true }
);

watch(
  [
    trigger,
    logicRoot,
    action,
    mapMode,
    mapKey,
    mapLimit,
    nodeLayout,
    wireStates,
    hiddenFlowNodes,
    pluginId,
    pluginName,
    description,
  ],
  () => {
    saveWorkspaceDraft(true);
    recordWorkspaceHistory();
  },
  { deep: true }
);

watch(
  wireStates,
  () => {
    isCompiled.value = false;
    compileLogLocal.value = "";
  },
  { deep: true }
);

// AI Translator callback
const handleAiTranslate = (payload: {
  trigger: VisualTrigger;
  action: VisualAction;
  conditions: VisualLogicGroup;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
}) => {
  trigger.value = payload.trigger;
  action.value = payload.action;
  logicRoot.value = payload.conditions;
  mapMode.value = payload.mapMode;
  mapKey.value = payload.mapKey;
  mapLimit.value = payload.mapLimit;
  hiddenFlowNodes.value = createDefaultHiddenNodes();
  wireStates.value = createDefaultWireStates();
};

const isTextEditingTarget = (target: EventTarget | null) => {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return (
    tag === "input" ||
    tag === "textarea" ||
    target.isContentEditable ||
    !!target.closest(".ant-select")
  );
};

const handleHistoryShortcut = (event: KeyboardEvent) => {
  const key = event.key.toLowerCase();
  const isModifier = event.ctrlKey || event.metaKey;
  if (!isModifier || isTextEditingTarget(event.target)) return;
  if (key === "z" && !event.shiftKey) {
    event.preventDefault();
    void undoWorkspace();
  } else if ((key === "z" && event.shiftKey) || key === "y") {
    event.preventDefault();
    void redoWorkspace();
  }
};

onMounted(async () => {
  restoreWorkspaceDraft();
  syncHistoryBaseline();
  window.addEventListener("keydown", handleHistoryShortcut);
  await fetchPlugins();
});

onBeforeUnmount(() => {
  stopNodeLibraryDragging();
  stopRecipePanelDragging();
  clearNodeLibraryHideTimer();
  clearRecipePanelHideTimer();
  window.removeEventListener("keydown", handleHistoryShortcut);
});

// Compile and upsert
const handleCompileAndRegister = async () => {
  if (!isWorkspaceValid.value) {
    compileLogLocal.value = [
      "已阻止编译：当前积木工作台存在错误。",
      ...validationErrors.value.map(
        (issue) => `[${issue.severity.toUpperCase()}] ${issue.title}${issue.detail ? ` - ${issue.detail}` : ""}`
      ),
    ].join("\n");
    message.error("请先修复左侧“编译前验证”中的错误");
    return;
  }
  compiling.value = true;
  compileLogLocal.value = "正在将高阶规则积木块转译为标准的 BPF C 源码...\n";
  try {
    compileLogLocal.value += `正在注册插件 Manifest [${pluginId.value}] 至本地仓库...\n`;
    await upsertPlugin({
      id: pluginId.value,
      name: pluginName.value,
      description: description.value,
      kind: "ebpf",
      enabled: false,
      attachKind: trigger.value === "unlink" ? "kprobe" : "none",
      attachTarget: trigger.value === "unlink" ? "do_unlinkat" : "",
      programName: "visual_custom_plugin",
      source: generatedBpfCode.value,
    });

    compileLogLocal.value +=
      "正在调用 LLVM/Clang 将源码编译为 ELF 内核字节码...\n";
    const success = await compileBpf(pluginId.value, generatedBpfCode.value);
    if (success) {
      isCompiled.value = true;
      compileLogLocal.value +=
        "\n[SUCCESS] 编译成功！点击下方按钮即可一键挂载至内核运行生效。";
    } else {
      compileLogLocal.value +=
        "\n[ERROR] 编译失败，请排查过滤表达式是否在内核 Verifier 安全范围内。";
    }
  } catch (err: any) {
    compileLogLocal.value += `\n[ERROR] 错误: ${err.message}`;
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
const moveFlowNodeTo = (node: VisualFlowNodeId, x: number, y: number) => {
  nodeLayout.value = {
    ...nodeLayout.value,
    [node]: {
      x: Math.round(x),
      y: Math.round(y),
    },
  };
};

const applyNodeTypeDrop = (
  category: string,
  value: string,
  position?: { x: number; y: number }
) => {
  let targetNode: VisualFlowNodeId | null = null;
  let statusText = "";

  if (category === "trigger") {
    if (!triggerOptions.some((item) => item.value === value)) return;
    targetNode = "trigger";
    restoreFlowNode(targetNode);
    trigger.value = value as VisualTrigger;
    statusText = `已切换事件挂载点为: ${value}`;
  } else if (category === "condition") {
    if (!isVisualConditionField(value)) return;
    targetNode = "condition";
    restoreFlowNode(targetNode);
    onAddRule("root", value);
    statusText = `已拖动添加匹配过滤: ${value}`;
  } else if (category === "logic_group") {
    if (value !== "AND" && value !== "OR") return;
    targetNode = "condition";
    restoreFlowNode(targetNode);
    onAddGroup("root", value);
    statusText = `已拖动添加逻辑运算组: ${value}`;
  } else if (category === "map") {
    if (!visualMapModeSet.has(value as VisualMapMode)) return;
    targetNode = "map";
    restoreFlowNode(targetNode);
    mapMode.value = value as VisualMapMode;
    statusText = `已配置 Map 状态存储为: ${value}`;
  } else if (category === "action") {
    if (!visualActionSet.has(value as VisualAction)) return;
    if (trigger.value === "unlink" && value === "BLOCK") {
      message.error("unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL");
      return;
    }
    targetNode = "action";
    restoreFlowNode(targetNode);
    action.value = value as VisualAction;
    statusText = `已更新拦截响应动作为: ${value}`;
  } else if (category === "focus") {
    if (!visualFlowNodeIds.includes(value as VisualFlowNodeId)) return;
    targetNode = value as VisualFlowNodeId;
    restoreFlowNode(targetNode);
    statusText = `已拖入并聚焦节点: ${flowNodeDetails[targetNode].label}`;
  }

  if (!targetNode) return;
  if (position) {
    moveFlowNodeTo(targetNode, position.x, position.y);
  }
  activeFlowNode.value = targetNode;
  designerSubtab.value = "dify";
  message.success(
    position ? `${statusText}，已吸附到画布网格` : statusText
  );
};

const handleCanvasNodeTypeDrop = (payload: CanvasNodeTypeDropPayload) => {
  applyNodeTypeDrop(payload.category, payload.value, {
    x: payload.x,
    y: payload.y,
  });
};

const handleWorkspaceDrop = (event: DragEvent) => {
  event.preventDefault();
  if (!event.dataTransfer) return;
  try {
    const rawData = event.dataTransfer.getData("text/plain");
    if (!rawData) return;
    const { category, value } = JSON.parse(rawData) as {
      category: string;
      value: string;
    };
    applyNodeTypeDrop(category, value);
  } catch (e) {
    console.error("Drop parsing failed:", e);
  }
};


</script>

<template>
  <div class="plugins-visual-tab">
    <a-row :gutter="16">
      <!-- Workspace (Designer Canvas) -->
      <a-col :span="24">
        <div class="graphical-workspace" @dragover.prevent @drop="handleWorkspaceDrop">
          <div class="workspace-title">
            <h3>流程图高级规则拼接控制台 (Advanced Flow Designer)</h3>
            <span class="sub"
              >通过拼接多重高级匹配字段与触发点，在系统内核深层执行精密入侵侦测。</span
            >
          </div>

          <a-tabs v-model:active-key="designerSubtab" class="dify-workspace-tabs">
            <a-tab-pane key="dify" tab="Dify Workflow">
              <div class="dify-workflow-shell">
                <div class="dify-workflow-hero">
                  <div>
                    <a-tag color="blue">Dify Style</a-tag>
                    <h4>节点工作流编排</h4>
                    <p>主视图只保留悬浮节点类型库、拖线画布和节点 Inspector；可从悬浮窗拖拽节点类型到画布，自动吸附到网格，并通过端口拖线/线缆开关编辑路由。</p>
                  </div>
                  <a-space size="small" wrap>
                    <a-tag :color="isWorkspaceValid ? 'green' : 'red'">
                      {{ isWorkspaceValid ? 'READY' : 'FIX REQUIRED' }}
                    </a-tag>
                    <a-tag color="purple">{{ conditionCount }} filters</a-tag>
                    <a-tag color="cyan">{{ generatedLineCount }} C lines</a-tag>
                  </a-space>
                </div>
          <PluginsVisualFlowCanvas
            v-model:node-layout="nodeLayout"
            v-model:wire-states="wireStates"
            :selected-node-id="activeFlowNode"
            :trigger="trigger"
            :action="action"
            :map-mode="mapMode"
            :condition-count="conditionCount"
            :tree-depth="treeDepth"
            :code-lines="generatedLineCount"
            :compile-ready="isWorkspaceValid"
            :hidden-nodes="hiddenFlowNodes"
            @update:selected-node-id="selectFlowNode"
            @reset-layout="resetNodeLayout"
            @reset-wires="resetWireStates"
            @delete-node="handleDeleteFlowNode"
            @drop-node-type="handleCanvasNodeTypeDrop"
          />

          <div class="selected-flow-panel">
            <a-tag color="blue" class="selected-flow-tag">
              {{ selectedFlowNodeDetail.label }}
            </a-tag>
            <span>{{ selectedFlowNodeDetail.focus }}</span>
            <a-space size="small" wrap>
              <a-button size="small" @click="focusFlowNode('trigger')">Trigger</a-button>
              <a-button size="small" @click="focusFlowNode('condition')">Condition</a-button>
              <a-button size="small" @click="focusFlowNode('map')">Map</a-button>
              <a-button size="small" @click="focusFlowNode('action')">Action</a-button>
              <a-button size="small" @click="focusFlowNode('code')">Code</a-button>
              <a-button size="small" @click="focusFlowNode('compile')">Compile</a-button>
            </a-space>
          </div>

          <PluginsVisualNodeInspector
            :selected-node-id="activeFlowNode"
            v-model:trigger="trigger"
            v-model:action="action"
            v-model:map-mode="mapMode"
            v-model:map-key="mapKey"
            v-model:map-limit="mapLimit"
            v-model:plugin-id="pluginId"
            v-model:plugin-name="pluginName"
            v-model:description="description"
            :condition-count="conditionCount"
            :tree-depth="treeDepth"
            :code-lines="generatedLineCount"
            :compile-ready="isWorkspaceValid"
            :compiling="compiling"
            :validation-issues="validationIssues"
            @add-condition="onAddRule('root', $event)"
            @add-group="onAddGroup('root', $event)"
            @compile="handleCompileAndRegister"
          />
              </div>
            </a-tab-pane>
            <a-tab-pane key="map" tab="Map / Blueprint Details">
              <div class="map-workspace-shell">
                <div class="map-workspace-notice">
                  <a-tag color="purple">二级选项卡</a-tag>
                  <span>原先偏 map / blueprint 的块状配置、条件树和状态 Map 面板集中在这里，主编辑体验保持 Dify 工作流风格。</span>
                </div>
          <!-- BLOCK 1: EVENT TRIGGER -->
          <div
            ref="triggerBlock"
            class="block-card block-trigger"
            :class="flowSectionClass('trigger')"
          >
            <!-- Node port -->
            <div class="node-port port-output trigger-port"></div>

            <div class="block-header">
              <span class="block-badge">Block 1</span>
              <strong style="color: #fff"
                >防御拦截挂载点积木 (Trigger Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">选择安全管控的内核底层事件拦截入口：</div>
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

          <!-- CONNECTION ARROW -->
          <div class="blueprint-wire-container">
            <div class="blueprint-wire-line wire-1-to-2"></div>
            <div class="blueprint-wire-pulse pulse-1-to-2"></div>
          </div>

          <!-- BLOCK 2: DYNAMIC CONDITIONS & AND/OR RELATION -->
          <div
            ref="conditionBlock"
            class="block-card block-condition"
            :class="flowSectionClass('condition')"
          >
            <!-- Node ports -->
            <div class="node-port port-input condition-port-in"></div>
            <div class="node-port port-output condition-port-out"></div>

            <div class="block-header">
              <div>
                <span class="block-badge" style="background: #fa8c16">Block 2</span>
                <strong style="color: #fff">高级嵌套逻辑过滤条件 (Nested Condition Block)</strong>
              </div>
            </div>
            <div class="block-body">
              <a-row :gutter="16">
                <!-- Condition Tree -->
                <a-col :span="15">
                  <div class="desc-line" style="margin-bottom: 16px;">
                    支持无限嵌套的逻辑运算组，可从左侧拖拽条件或逻辑组至目标块内：
                  </div>
                  
                  <div class="conditions-list-tree" style="max-height: 380px; overflow-y: auto; padding-right: 4px;">
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

                <!-- Blueprint Logic Gate Visualizer (Fully integrated SVG tree) -->
                <a-col :span="9" style="border-left: 1px dashed rgba(255, 255, 255, 0.1); padding-left: 16px;">
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

          <!-- BLOCK 2.5: STATEFUL MAP OPERATIONS -->
          <div ref="mapBlock" :class="flowSectionClass('map')">
            <PluginsVisualMapPanel
              v-model:mode="mapMode"
              v-model:key-field="mapKey"
              v-model:limit="mapLimit"
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
            <!-- Node port -->
            <div class="node-port port-input action-port-in"></div>

            <div class="block-header">
              <span class="block-badge" style="background: #52c41a"
                >Block 3</span
              >
              <strong style="color: #fff"
                >安全管控响应积木 (Action Block)</strong
              >
            </div>
            <div class="block-body">
              <div class="desc-line">
                当上述过滤组合触发成功时，内核要执行的安全响应动作：
              </div>
              <a-radio-group
                v-model:value="action"
                button-style="solid"
                style="width: 100%"
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
                style="color: #fa8c16; margin-top: 8px"
              >
                * 物理文件 unlink 挂载于 Kprobe 上，不改变内核决策链，仅支持 ALERT 或 KILL 动作。其他 LSM 挂载点支持完整的 BLOCK、ALERT 与 KILL 动作。
              </div>
            </div>
          </div>

          <!-- Plugin Details Panel -->
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
                        v-model:value="pluginId"
                        placeholder="例如 custom-visual-lsm"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :span="12">
                    <a-form-item label="规则插件显示名">
                      <a-input v-model:value="pluginName" />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-form-item label="详细说明描述" style="margin-bottom: 0">
                  <a-textarea v-model:value="description" :rows="2" />
                </a-form-item>
              </a-form>

              <div
                style="margin-top: 20px; display: flex; justify-content: flex-end"
              >
                <a-button
                  type="primary"
                  :loading="compiling"
                  :disabled="!isWorkspaceValid"
                  @click="handleCompileAndRegister"
                >
                  <template #icon><ThunderboltOutlined /></template>
                  一键编译并注册为 BPF 插件
                </a-button>
              </div>
            </a-card>
          </div>
              </div>
            </a-tab-pane>
            <a-tab-pane key="nlp" tab="NLP Blocks Compiler">
              <div class="nlp-workspace-shell">
                <div class="nlp-workspace-notice">
                  <a-tag color="purple">NLP Blocks Compiler</a-tag>
                  <span>用自然语言描述内核防御意图，自动编译成 Trigger / Condition / Map / Action 积木流；生成后可回到 Dify Workflow 继续拖线、删节点、调 Inspector。</span>
                </div>
                <PluginsVisualAiPanel v-model="aiPrompt" @translate="handleAiTranslate" />
              </div>
            </a-tab-pane>
            <a-tab-pane key="source" tab="Generated eBPF C">
              <div ref="codeBlock" class="source-workspace-shell" :class="flowSectionClass('code')">
                <div class="source-workspace-notice">
                  <a-tag color="cyan">独立源码 Tab</a-tag>
                  <span>动态生成的 eBPF C 语言高阶过滤器源码、Clang 编译日志和加载入口集中在这里，主画布只负责 Dify 风格节点编排。</span>
                </div>
                <PluginsVisualCodePanel
                  :code="generatedBpfCode"
                  :compiling="compiling"
                  :compiled="isCompiled"
                  :loading="loadingAction"
                  :log="compileLogLocal"
                  @load="handleLoad"
                />
              </div>
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-col>

    </a-row>

    <transition name="node-library-float">
      <div
        v-if="nodeLibraryVisible"
        ref="nodeLibraryFloating"
        class="node-library-floating-window"
        :class="[`dock-${nodeLibraryDock}`, { dragging: !!nodeLibraryDragging }]"
        :style="nodeLibraryStyle"
      >
        <div class="node-library-floating-toolbar">
          <button
            type="button"
            class="node-library-direction-button"
            :title="nodeLibraryDock === 'left' ? '贴左边缘隐藏节点类型' : '贴右边缘隐藏节点类型'"
            @pointerdown.stop
            @click="hideNodeLibrary"
          >
            {{ nodeLibraryHideArrow }}
          </button>
          <div
            class="node-library-floating-drag-handle"
            title="拖拽移动；靠近左右边缘自动吸附"
            @pointerdown.prevent="startNodeLibraryDragging"
          >
            <a-tag color="blue">节点类型</a-tag>
            <span>拖拽移动，靠近边缘自动吸附</span>
          </div>
          <a-button
            size="small"
            class="node-library-dock-toggle"
            @pointerdown.stop
            @click="toggleNodeLibraryDock"
          >
            {{ nodeLibraryDockLabel }}
          </a-button>
        </div>
        <PluginsVisualNodeTypeLibrary
          @select-trigger="selectTriggerNodeType"
          @add-condition="addConditionNodeType"
          @add-group="addLogicNodeType"
          @set-map="setMapNodeType"
          @set-action="setActionNodeType"
          @focus-node="focusFlowNode"
        />
      </div>
    </transition>
    <transition name="node-library-trigger">
      <button
        v-if="!nodeLibraryVisible"
        type="button"
        class="node-library-floating-trigger"
        :class="`dock-${nodeLibraryDock}`"
        :style="nodeLibraryTriggerStyle"
        :title="nodeLibraryDock === 'left' ? '从左边缘展开节点类型' : '从右边缘展开节点类型'"
        @click="showNodeLibrary"
      >
        <span>{{ nodeLibraryRestoreArrow }}</span>
        节点类型
      </button>
    </transition>

    <transition name="recipe-float">
      <div
        v-if="recipePanelVisible"
        ref="recipeFloating"
        class="recipe-floating-window"
        :class="[`dock-${recipePanelDock}`, { dragging: !!recipePanelDragging }]"
        :style="recipePanelStyle"
      >
        <div class="recipe-floating-toolbar">
          <button
            type="button"
            class="recipe-direction-button"
            :title="recipePanelDock === 'left' ? '贴左边缘隐藏' : '贴右边缘隐藏'"
            @pointerdown.stop
            @click="hideRecipePanel"
          >
            {{ recipeHideArrow }}
          </button>
          <div
            class="recipe-floating-drag-handle"
            title="拖拽移动；靠近左右边缘自动吸附"
            @pointerdown.prevent="startRecipePanelDragging"
          >
            <a-tag color="green">场景积木</a-tag>
            <span>拖拽移动，靠近边缘自动吸附</span>
          </div>
          <a-button
            size="small"
            class="recipe-dock-toggle"
            @pointerdown.stop
            @click="toggleRecipePanelDock"
          >
            {{ recipePanelDockLabel }}
          </a-button>
        </div>
        <PluginsVisualRecipePanel
          :recipes="visualRecipes"
          :trigger="trigger"
          :action="action"
          :map-mode="mapMode"
          :condition-count="conditionCount"
          :tree-depth="treeDepth"
          :plugin-id="pluginId"
          :code-lines="generatedLineCount"
          :validation-issues="validationIssues"
          :compile-ready="isWorkspaceValid"
          :autosave-label="autosaveLabel"
          :undo-count="undoStack.length"
          :redo-count="redoStack.length"
          @apply-recipe="applyRecipe"
          @reset-workspace="resetWorkspace"
          @export-workspace="exportWorkspace"
          @import-workspace="importWorkspace"
          @save-draft="() => saveWorkspaceDraft(false)"
          @clear-draft="clearWorkspaceDraft"
          @undo-workspace="undoWorkspace"
          @redo-workspace="redoWorkspace"
        />
      </div>
    </transition>
    <transition name="recipe-trigger">
      <button
        v-if="!recipePanelVisible"
        type="button"
        class="recipe-floating-trigger"
        :class="`dock-${recipePanelDock}`"
        :style="recipeTriggerStyle"
        :title="recipePanelDock === 'left' ? '从左边缘展开场景积木' : '从右边缘展开场景积木'"
        @click="recipePanelVisible = true"
      >
        <span>{{ recipeRestoreArrow }}</span>
        场景积木
      </button>
    </transition>
  </div>
</template>

<style scoped>
.plugins-visual-tab {
  min-height: 600px;
}
.palette-stack {
  margin-top: 16px;
}
.dify-workspace-tabs {
  margin-top: 8px;
}

.dify-workflow-shell,
.map-workspace-shell,
.nlp-workspace-shell,
.source-workspace-shell {
  padding-top: 8px;
}

.dify-workflow-hero,
.map-workspace-notice,
.nlp-workspace-notice,
.source-workspace-notice {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid rgba(59, 130, 246, 0.26);
  background: rgba(15, 23, 42, 0.78);
  color: #cbd5e1;
}

.dify-workflow-hero h4 {
  margin: 8px 0 4px;
  color: #f8fafc;
  font-size: 15px;
}

.dify-workflow-hero p,
.map-workspace-notice span,
.nlp-workspace-notice span,
.source-workspace-notice span {
  margin: 0;
  color: #94a3b8;
  font-size: 12px;
  line-height: 1.45;
}

.map-workspace-notice {
  align-items: center;
  justify-content: flex-start;
  border-color: rgba(114, 46, 209, 0.28);
}

.source-workspace-notice {
  align-items: center;
  justify-content: flex-start;
  border-color: rgba(19, 194, 194, 0.32);
}

.nlp-workspace-notice {
  align-items: center;
  justify-content: flex-start;
  border-color: rgba(114, 46, 209, 0.36);
}

.node-library-floating-window {
  position: fixed;
  z-index: 34;
  display: flex;
  flex-direction: column;
  width: min(320px, calc(100vw - 32px));
  max-height: calc(100vh - 128px);
  min-height: 420px;
  overflow: hidden;
  padding: 10px;
  border: 1px solid rgba(56, 189, 248, 0.32);
  border-radius: 14px;
  background: rgba(2, 6, 23, 0.84);
  box-shadow: 0 20px 55px rgba(0, 0, 0, 0.48);
  backdrop-filter: blur(14px);
  --node-library-panel-exit-x: calc(-100% - 24px);
  --node-library-edge-button-hover: -4px;
  transition:
    left 0.18s ease,
    top 0.18s ease,
    opacity 0.2s ease,
    transform 0.2s ease,
    box-shadow 0.18s ease;
}

.node-library-floating-window.dragging {
  cursor: grabbing;
  transition: none;
  box-shadow: 0 24px 68px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(56, 189, 248, 0.3);
}

.node-library-floating-window.dock-right {
  --node-library-panel-exit-x: calc(100% + 24px);
  --node-library-edge-button-hover: 4px;
}

.node-library-floating-toolbar {
  position: sticky;
  top: -10px;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex: 0 0 auto;
  margin: -10px -10px 10px;
  padding: 8px 10px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.94);
}

.node-library-floating-window.dock-left .node-library-direction-button {
  order: 0;
}

.node-library-floating-window.dock-left .node-library-floating-drag-handle {
  order: 1;
}

.node-library-floating-window.dock-left .node-library-dock-toggle {
  order: 2;
}

.node-library-floating-window.dock-right .node-library-dock-toggle {
  order: 0;
}

.node-library-floating-window.dock-right .node-library-floating-drag-handle {
  order: 1;
}

.node-library-floating-window.dock-right .node-library-direction-button {
  order: 2;
}

.node-library-floating-drag-handle {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  min-width: 0;
  color: #94a3b8;
  font-size: 11px;
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.node-library-floating-drag-handle span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-library-floating-window.dragging .node-library-floating-drag-handle {
  cursor: grabbing;
}

.node-library-dock-toggle {
  flex: 0 0 auto;
}

.node-library-direction-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 24px;
  border: 1px solid rgba(56, 189, 248, 0.36);
  border-radius: 999px;
  color: #dbeafe;
  background: rgba(30, 64, 175, 0.32);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  transition: transform 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.node-library-direction-button:hover {
  transform: translateX(var(--node-library-edge-button-hover));
  border-color: rgba(125, 211, 252, 0.7);
  background: rgba(30, 64, 175, 0.56);
}

.node-library-floating-window :deep(.dify-node-library) {
  flex: 1 1 auto;
  min-height: 0;
}

.node-library-floating-trigger {
  position: fixed;
  z-index: 35;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid rgba(56, 189, 248, 0.42);
  color: #dbeafe;
  background: rgba(15, 23, 42, 0.92);
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.38);
  cursor: pointer;
  font-size: 12px;
  transition:
    transform 0.18s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.18s ease,
    border-color 0.16s ease;
}

.node-library-floating-trigger:hover {
  transform: translateX(var(--node-library-trigger-hover, 4px));
  border-color: rgba(125, 211, 252, 0.72);
}

.node-library-floating-trigger span {
  font-size: 18px;
  line-height: 0.8;
}

.node-library-floating-trigger.dock-left {
  left: 0;
  border-left: 0;
  border-radius: 0 999px 999px 0;
  --node-library-trigger-enter-x: -100%;
  --node-library-trigger-hover: 4px;
}

.node-library-floating-trigger.dock-right {
  right: 0;
  border-right: 0;
  border-radius: 999px 0 0 999px;
  --node-library-trigger-enter-x: 100%;
  --node-library-trigger-hover: -4px;
}

.node-library-float-enter-active,
.node-library-float-leave-active,
.node-library-trigger-enter-active,
.node-library-trigger-leave-active {
  transition: opacity 0.22s ease, transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}

.node-library-float-enter-from,
.node-library-float-leave-to {
  opacity: 0;
  transform: translateX(var(--node-library-panel-exit-x)) scale(0.98);
}

.node-library-trigger-enter-from,
.node-library-trigger-leave-to {
  opacity: 0;
  transform: translateX(var(--node-library-trigger-enter-x)) scale(0.98);
}

.recipe-floating-window {
  position: fixed;
  z-index: 30;
  width: min(360px, calc(100vw - 32px));
  max-height: calc(100vh - 128px);
  overflow: auto;
  padding: 10px;
  border: 1px solid rgba(34, 197, 94, 0.28);
  border-radius: 14px;
  background: rgba(2, 6, 23, 0.82);
  box-shadow: 0 20px 55px rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(14px);
  --recipe-panel-exit-x: calc(-100% - 24px);
  --recipe-edge-button-hover: -4px;
  transition:
    left 0.18s ease,
    top 0.18s ease,
    opacity 0.2s ease,
    transform 0.2s ease,
    box-shadow 0.18s ease;
}

.recipe-floating-window.dragging {
  cursor: grabbing;
  transition: none;
  box-shadow: 0 24px 68px rgba(0, 0, 0, 0.58), 0 0 0 1px rgba(34, 197, 94, 0.28);
}

.recipe-floating-window.dock-right {
  --recipe-panel-exit-x: calc(100% + 24px);
  --recipe-edge-button-hover: 4px;
}

.recipe-floating-toolbar {
  position: sticky;
  top: -10px;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin: -10px -10px 10px;
  padding: 8px 10px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.94);
}

.recipe-floating-window.dock-left .recipe-direction-button {
  order: 0;
}

.recipe-floating-window.dock-left .recipe-floating-drag-handle {
  order: 1;
}

.recipe-floating-window.dock-left .recipe-dock-toggle {
  order: 2;
}

.recipe-floating-window.dock-right .recipe-dock-toggle {
  order: 0;
}

.recipe-floating-window.dock-right .recipe-floating-drag-handle {
  order: 1;
}

.recipe-floating-window.dock-right .recipe-direction-button {
  order: 2;
}

.recipe-floating-drag-handle {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 auto;
  min-width: 0;
  color: #94a3b8;
  font-size: 11px;
  cursor: grab;
  user-select: none;
  touch-action: none;
}

.recipe-floating-drag-handle span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recipe-floating-window.dragging .recipe-floating-drag-handle {
  cursor: grabbing;
}

.recipe-dock-toggle {
  flex: 0 0 auto;
}

.recipe-direction-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 24px;
  border: 1px solid rgba(34, 197, 94, 0.32);
  border-radius: 999px;
  color: #dcfce7;
  background: rgba(22, 101, 52, 0.32);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
  transition: transform 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.recipe-direction-button:hover {
  transform: translateX(var(--recipe-edge-button-hover));
  border-color: rgba(134, 239, 172, 0.65);
  background: rgba(22, 101, 52, 0.55);
}

.recipe-floating-trigger {
  position: fixed;
  z-index: 31;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid rgba(34, 197, 94, 0.38);
  border-radius: 999px;
  color: #dcfce7;
  background: rgba(15, 23, 42, 0.92);
  box-shadow: 0 14px 34px rgba(0, 0, 0, 0.36);
  cursor: pointer;
  font-size: 12px;
  transition:
    transform 0.18s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.18s ease,
    border-color 0.16s ease;
}

.recipe-floating-trigger:hover {
  transform: translateX(var(--recipe-trigger-hover, 4px));
  border-color: rgba(134, 239, 172, 0.65);
}

.recipe-floating-trigger span {
  font-size: 18px;
  line-height: 0.8;
}

.recipe-floating-trigger.dock-left {
  left: 0;
  border-left: 0;
  border-radius: 0 999px 999px 0;
  --recipe-trigger-enter-x: -100%;
  --recipe-trigger-hover: 4px;
}

.recipe-floating-trigger.dock-right {
  right: 0;
  border-right: 0;
  border-radius: 999px 0 0 999px;
  --recipe-trigger-enter-x: 100%;
  --recipe-trigger-hover: -4px;
}

.recipe-float-enter-active,
.recipe-float-leave-active,
.recipe-trigger-enter-active,
.recipe-trigger-leave-active {
  transition: opacity 0.22s ease, transform 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}

.recipe-float-enter-from,
.recipe-float-leave-to {
  opacity: 0;
  transform: translateX(var(--recipe-panel-exit-x)) scale(0.98);
}

.recipe-trigger-enter-from,
.recipe-trigger-leave-to {
  opacity: 0;
  transform: translateX(var(--recipe-trigger-enter-x)) scale(0.98);
}

:deep(.dify-workspace-tabs .ant-tabs-nav) {
  margin-bottom: 12px;
}

:deep(.dify-workspace-tabs .ant-tabs-tab) {
  padding: 8px 14px;
  border-radius: 999px;
  color: #94a3b8;
  background: rgba(15, 23, 42, 0.62);
}

:deep(.dify-workspace-tabs .ant-tabs-tab-active) {
  background: rgba(37, 99, 235, 0.18);
}

:deep(.dify-workspace-tabs .ant-tabs-tab-active .ant-tabs-tab-btn) {
  color: #dbeafe;
}

:deep(.dify-workspace-tabs .ant-tabs-ink-bar) {
  background: #38bdf8;
}
.selected-flow-panel {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid rgba(24, 144, 255, 0.24);
  background: rgba(15, 23, 42, 0.72);
  color: #cbd5e1;
  font-size: 12px;
}

.selected-flow-tag {
  margin: 0;
}

.flow-section-active {
  outline: 2px solid rgba(56, 189, 248, 0.82);
  outline-offset: 4px;
  box-shadow: 0 0 0 1px rgba(56, 189, 248, 0.25), 0 0 24px rgba(56, 189, 248, 0.2);
  border-radius: 10px;
  transition: outline-color 0.2s ease, box-shadow 0.2s ease;
}
.graphical-workspace {
  background-color: #0b132b;
  background-image: 
    linear-gradient(to right, rgba(28, 37, 65, 0.4) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(28, 37, 65, 0.4) 1px, transparent 1px),
    linear-gradient(to right, rgba(28, 37, 65, 0.15) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(28, 37, 65, 0.15) 1px, transparent 1px);
  background-size: 40px 40px, 40px 40px, 10px 10px, 10px 10px;
  border: 1px solid #1c2541;
  border-radius: 12px;
  padding: 24px;
  box-shadow: inset 0 0 40px rgba(0, 0, 0, 0.8);
  position: relative;
}
.workspace-title {
  margin-bottom: 20px;
  border-left: 4px solid #1890ff;
  padding-left: 10px;
}
.workspace-title h3 {
  margin: 0;
  font-weight: 600;
  color: #ffffff;
}
.workspace-title .sub {
  font-size: 12px;
  color: #94a3b8;
}

/* Blueprint nodes styling */
.block-card {
  border-radius: 8px;
  overflow: visible; /* to show ports */
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  background: rgba(13, 19, 33, 0.85);
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.7);
}

.block-trigger {
  border-color: rgba(24, 144, 255, 0.35);
}
.block-trigger:hover {
  border-color: rgba(24, 144, 255, 0.7);
  box-shadow: 0 0 15px rgba(24, 144, 255, 0.2);
}
.block-trigger .block-header {
  background: linear-gradient(135deg, #1890ff, #0050b3);
}

.block-condition {
  border-color: rgba(250, 140, 22, 0.35);
}
.block-condition:hover {
  border-color: rgba(250, 140, 22, 0.7);
  box-shadow: 0 0 15px rgba(250, 140, 22, 0.2);
}
.block-condition .block-header {
  background: linear-gradient(135deg, #fa8c16, #ad4e00);
}

.block-action {
  border-color: rgba(82, 196, 26, 0.35);
}
.block-action:hover {
  border-color: rgba(82, 196, 26, 0.7);
  box-shadow: 0 0 15px rgba(82, 196, 26, 0.2);
}
.block-action .block-header {
  background: linear-gradient(135deg, #52c41a, #237804);
}

.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}
.block-badge {
  background: rgba(0, 0, 0, 0.35);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: #0f172a;
  padding: 18px;
  color: #cbd5e1;
}
.desc-line {
  font-size: 13px;
  color: #94a3b8;
  margin-bottom: 12px;
}

/* Blueprint wires */
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
  background: linear-gradient(180deg, #1890ff, #fa8c16);
}
.wire-2-to-2-5 {
  background: linear-gradient(180deg, #fa8c16, #722ed1);
}
.wire-2-5-to-3 {
  background: linear-gradient(180deg, #722ed1, #52c41a);
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
  background: #1890ff;
  box-shadow: 0 0 8px #1890ff, 0 0 15px #1890ff;
}
.pulse-2-to-2-5 {
  background: #fa8c16;
  box-shadow: 0 0 8px #fa8c16, 0 0 15px #fa8c16;
}
.pulse-2-5-to-3 {
  background: #722ed1;
  box-shadow: 0 0 8px #722ed1, 0 0 15px #722ed1;
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

/* Node ports */
.node-port {
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  border: 1px solid rgba(255, 255, 255, 0.6);
}
.port-input {
  top: -5px;
}
.port-output {
  bottom: -5px;
}

.trigger-port {
  background: #1890ff;
  border-color: #1890ff;
  box-shadow: 0 0 8px #1890ff;
}
.condition-port-in {
  background: #1890ff;
  border-color: #1890ff;
  box-shadow: 0 0 8px #1890ff;
}
.condition-port-out {
  background: #fa8c16;
  border-color: #fa8c16;
  box-shadow: 0 0 8px #fa8c16;
}
.action-port-in {
  background: #722ed1;
  border-color: #722ed1;
  box-shadow: 0 0 8px #722ed1;
}

/* Condition inputs and layout */
.condition-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.helper-text {
  font-size: 11px;
}
.block-red.ant-radio-button-wrapper-checked {
  background: #f5222d;
  border-color: #f5222d;
  color: white;
}

/* Deep input styling for dark mode */
:deep(.graphical-workspace .ant-select-selector),
:deep(.graphical-workspace .ant-input),
:deep(.graphical-workspace .ant-input-number),
:deep(.graphical-workspace .ant-radio-button-wrapper) {
  background-color: #1e293b !important;
  border-color: #334155 !important;
  color: #f1f5f9 !important;
}
:deep(.graphical-workspace .ant-select-arrow) {
  color: #94a3b8 !important;
}
:deep(.graphical-workspace .ant-radio-button-wrapper-checked) {
  background-color: #1890ff !important;
  color: #ffffff !important;
  border-color: #1890ff !important;
}
:deep(.graphical-workspace .ant-radio-button-wrapper-checked.block-red) {
  background-color: #ef4444 !important;
  border-color: #ef4444 !important;
}
:deep(.graphical-workspace .ant-btn-dashed) {
  background: rgba(255, 255, 255, 0.03) !important;
  border-color: #475569 !important;
  color: #94a3b8 !important;
}
:deep(.graphical-workspace .ant-btn-dashed:hover) {
  border-color: #fa8c16 !important;
  color: #fa8c16 !important;
}
:deep(.graphical-workspace .ant-card) {
  background: #0f172a !important;
  border-color: rgba(255, 255, 255, 0.05) !important;
}
:deep(.graphical-workspace .ant-card-head) {
  border-bottom-color: rgba(255, 255, 255, 0.05) !important;
  color: #ffffff !important;
  background: #1e293b !important;
}


</style>
