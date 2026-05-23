<script setup lang="ts">
import {
  ref,
  computed,
  onMounted,
  onBeforeUnmount,
  h,
  nextTick,
  useTemplateRef,
} from "vue";
import { message, Modal } from "ant-design-vue";
import { ThunderboltOutlined } from "@ant-design/icons-vue";
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
  VisualLogicGroup,
  VisualMapMode,
  VisualMapKey,
  VisualRecipe,
  VisualTrigger,
  VisualWireId,
  VisualWireStates,
} from "./types";

import { generateBpfCode } from "./transpiler";
import { validateWorkspace, isVisualConditionField } from "./validation";
import { useVisualWorkspace } from "./useVisualWorkspace";

const { compileBpf, loadBpf, upsertPlugin, fetchPlugins } = usePlugins();

const {
  trigger,
  logicRoot,
  action,
  mapMode,
  mapKey,
  mapLimit,
  aiPrompt,
  pluginId,
  pluginName,
  description,
  isCompiled,
  autosaveLabel,
  undoStack,
  redoStack,
  nodeLayout,
  wireStates,
  hiddenFlowNodes,
  activeFlowNode,
  designerSubtab,
  countConditions,
  treeDepth,
  onDeleteNode,
  onAddRule,
  onAddGroup,
  onUpdateRule,
  onUpdateGroupType,
  createWorkspaceSnapshot,
  applyWorkspaceSnapshot,
  saveWorkspaceDraft,
  clearWorkspaceDraft,
  restoreWorkspaceDraft,
  syncHistoryBaseline,
  undoWorkspace,
  redoWorkspace,
} = useVisualWorkspace();

const compiling = ref(false);
const loadingAction = ref(false);
const compileLogLocal = ref("");

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

const createDefaultNodeLayout = () => ({
  trigger: { x: 24, y: 38 },
  condition: { x: 196, y: 38 },
  map: { x: 368, y: 38 },
  action: { x: 540, y: 38 },
  code: { x: 368, y: 176 },
  compile: { x: 540, y: 176 },
});

const createDefaultWireStates = (): Record<VisualWireId, boolean> => ({
  "trigger-condition": true,
  "condition-map": true,
  "map-action": true,
  "condition-code": true,
  "map-code": true,
  "action-compile": true,
  "code-compile": true,
});

const createDefaultHiddenNodes = () => ({});

const mergeWireStates = (
  states?: VisualWireStates
): Record<VisualWireId, boolean> => {
  const merged = createDefaultWireStates();
  if (!states) return merged;
  visualWireIds.forEach((id) => {
    if (typeof states[id] === "boolean") {
      merged[id] = states[id] as boolean;
    }
  });
  return merged;
};

// Floating panels docks & drag logic
type FloatingDock = "left" | "right";
const floatingPanelEdgeMargin = 18;
const getInitialFloatingX = (dock: FloatingDock, width: number) => {
  const viewportWidth =
    typeof window === "undefined" ? 1280 : Math.max(480, window.innerWidth);
  return dock === "left"
    ? floatingPanelEdgeMargin
    : Math.max(
        floatingPanelEdgeMargin,
        viewportWidth - width - floatingPanelEdgeMargin
      );
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
const nodeLibraryFloatingRef = useTemplateRef<HTMLElement>(
  "nodeLibraryFloating"
);

const flowNodeDetails: Record<
  VisualFlowNodeId,
  { label: string; focus: string }
> = {
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
  top: `${Math.max(
    88,
    Math.min(recipePanelPosition.value.y + 14, window.innerHeight - 120)
  )}px`,
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
    : Math.max(
        recipePanelEdgeMargin,
        window.innerWidth - width - recipePanelEdgeMargin
      );

const isRecipePanelAtDock = (dock: "left" | "right", width: number) =>
  Math.abs(recipePanelPosition.value.x - getRecipePanelDockX(dock, width)) <= 2;

const getNodeLibraryDockX = (dock: FloatingDock, width: number) =>
  dock === "left"
    ? floatingPanelEdgeMargin
    : Math.max(
        floatingPanelEdgeMargin,
        window.innerWidth - width - floatingPanelEdgeMargin
      );

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
        window.innerHeight -
          Math.min(height, window.innerHeight - 128) -
          floatingPanelEdgeMargin
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
        window.innerHeight -
          Math.min(height, window.innerHeight - 128) -
          recipePanelEdgeMargin
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
      recipePanelDragging.value.originX +
        event.clientX -
        recipePanelDragging.value.startX,
      margin,
      Math.max(margin, window.innerWidth - width - margin)
    ),
    y: clampNumber(
      recipePanelDragging.value.originY +
        event.clientY -
        recipePanelDragging.value.startY,
      margin,
      Math.max(
        margin,
        window.innerHeight - Math.min(height, window.innerHeight - 128) - margin
      )
    ),
  };
};

const stopRecipePanelDragging = () => {
  if (recipePanelDragging.value) {
    const snapThreshold = 96;
    const { width } = getRecipePanelSize();
    if (recipePanelPosition.value.x <= snapThreshold) {
      snapRecipePanelTo("left");
    } else if (
      recipePanelPosition.value.x + width >=
      window.innerWidth - snapThreshold
    ) {
      snapRecipePanelTo("right");
    } else {
      recipePanelDock.value =
        recipePanelPosition.value.x + width / 2 < window.innerWidth / 2
          ? "left"
          : "right";
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
    recipePanelPosition.value.x + width / 2 < window.innerWidth / 2
      ? "left"
      : "right";
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
      nodeLibraryDragging.value.originX +
        event.clientX -
        nodeLibraryDragging.value.startX,
      margin,
      Math.max(margin, window.innerWidth - width - margin)
    ),
    y: clampNumber(
      nodeLibraryDragging.value.originY +
        event.clientY -
        nodeLibraryDragging.value.startY,
      margin,
      Math.max(
        margin,
        window.innerHeight - Math.min(height, window.innerHeight - 128) - margin
      )
    ),
  };
};

const stopNodeLibraryDragging = () => {
  if (nodeLibraryDragging.value) {
    const snapThreshold = 96;
    const { width } = getNodeLibrarySize();
    if (nodeLibraryPosition.value.x <= snapThreshold) {
      snapNodeLibraryTo("left");
    } else if (
      nodeLibraryPosition.value.x + width >=
      window.innerWidth - snapThreshold
    ) {
      snapNodeLibraryTo("right");
    } else {
      nodeLibraryDock.value =
        nodeLibraryPosition.value.x + width / 2 < window.innerWidth / 2
          ? "left"
          : "right";
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
    nodeLibraryPosition.value.x + width / 2 < window.innerWidth / 2
      ? "left"
      : "right";
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
  message.warning(
    `已从画布删除 ${flowNodeDetails[node].label}，可从左侧节点类型库重新添加`
  );
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

interface CanvasNodeTypeDropPayload {
  category: string;
  value: string;
  x: number;
  y: number;
}

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
    description:
      "socket_connect 上组合 comm + 多端口 OR，并用 COUNTER 限频兜底。",
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
    description:
      "inode_rename 上关注 shadow / .env / .key 等敏感名称，先告警审计。",
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
    message.error(
      "unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL"
    );
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
    const snapshot = JSON.parse(raw);
    applyWorkspaceSnapshot(snapshot);
    message.success("已导入积木工作台配置");
  } catch (err: any) {
    message.error(`导入失败: ${err?.message || "JSON 格式错误"}`);
  }
};

const validationIssues = computed(() => {
  const currentSnapshot = createWorkspaceSnapshot();
  return validateWorkspace(
    currentSnapshot,
    flowNodeDetails,
    visualWireLabels,
    visualWireEndpoints,
    visualFlowNodeIds,
    visualWireIds
  );
});

const validationErrors = computed(() =>
  validationIssues.value.filter((issue) => issue.severity === "error")
);

const isWorkspaceValid = computed(() => validationErrors.value.length === 0);

const generatedBpfCode = computed(() => {
  const currentSnapshot = createWorkspaceSnapshot();
  return generateBpfCode(currentSnapshot);
});

const generatedLineCount = computed(
  () => generatedBpfCode.value.split(/\r?\n/).length
);

const visualAttachKind = computed(() =>
  trigger.value === "unlink" ? "kprobe" : "lsm"
);

const visualAttachTarget = computed(() => {
  switch (trigger.value) {
    case "process":
      return "lsm/bprm_check_security";
    case "file_open":
      return "lsm/file_open";
    case "mkdir":
      return "lsm/inode_mkdir";
    case "file_create":
      return "lsm/inode_create";
    case "rmdir":
      return "lsm/inode_rmdir";
    case "symlink":
      return "lsm/inode_symlink";
    case "socket_connect":
      return "lsm/socket_connect";
    case "inode_mknod":
      return "lsm/inode_mknod";
    case "file_mprotect":
      return "lsm/file_mprotect";
    case "inode_rename":
      return "lsm/inode_rename";
    case "unlink":
      return "do_unlinkat";
    default:
      return "";
  }
});

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
  designerSubtab.value = "dify";
  activeFlowNode.value = "condition";
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

const handleCompileAndRegister = async () => {
  if (!isWorkspaceValid.value) {
    compileLogLocal.value = [
      "已阻止编译：当前积木工作台存在错误。",
      ...validationErrors.value.map(
        (issue) =>
          `[${issue.severity.toUpperCase()}] ${issue.title}${
            issue.detail ? ` - ${issue.detail}` : ""
          }`
      ),
    ].join("\n");
    message.error("请先修复左侧“编译前验证”中的错误");
    return;
  }
  compiling.value = true;
  compileLogLocal.value = "正在将高阶规则积木块转译为标准的 BPF C 源码...\n";
  try {
    compileLogLocal.value += `正在注册插件 Manifest [${pluginId.value}] 至本地仓库...\n`;
    compileLogLocal.value += `挂载方式: ${visualAttachKind.value} / ${visualAttachTarget.value} / program=visual_custom_plugin\n`;
    await upsertPlugin({
      id: pluginId.value,
      name: pluginName.value,
      description: description.value,
      kind: "ebpf",
      enabled: false,
      attachKind: visualAttachKind.value,
      attachTarget: visualAttachTarget.value,
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

  const visualMapModeSet = new Set<VisualMapMode>([
    "NONE",
    "COUNTER",
    "BLOCKLIST",
  ]);
  const visualActionSet = new Set<VisualAction>(["BLOCK", "ALERT", "KILL"]);

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
      message.error(
        "unlink (Kprobe) 挂载点不支持 BLOCK 动作，请选择 ALERT 或 KILL"
      );
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
  message.success(position ? `${statusText}，已吸附到画布网格` : statusText);
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
        <div
          class="graphical-workspace"
          @dragover.prevent
          @drop="handleWorkspaceDrop"
        >
          <div class="workspace-title">
            <h3>流程图高级规则拼接控制台 (Advanced Flow Designer)</h3>
            <span class="sub"
              >通过拼接多重高级匹配字段与触发点，在系统内核深层执行精密入侵侦测。</span
            >
          </div>

          <a-tabs
            v-model:active-key="designerSubtab"
            class="dify-workspace-tabs"
          >
            <a-tab-pane key="dify" tab="Dify Workflow">
              <div class="dify-workflow-shell">
                <div class="dify-workflow-hero">
                  <div>
                    <a-tag color="blue">Dify Style</a-tag>
                    <h4>节点工作流编排</h4>
                    <p>
                      主视图只保留悬浮节点类型库、拖线画布和节点
                      Inspector；可从悬浮窗拖拽节点类型到画布，自动吸附到网格，并通过端口拖线/线缆开关编辑路由。
                    </p>
                  </div>
                  <a-space size="small" wrap>
                    <a-tag :color="isWorkspaceValid ? 'green' : 'red'">
                      {{ isWorkspaceValid ? "READY" : "FIX REQUIRED" }}
                    </a-tag>
                    <a-tag color="purple">{{ countConditions }} filters</a-tag>
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
                  :condition-count="countConditions"
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
                    <a-button size="small" @click="focusFlowNode('trigger')"
                      >Trigger</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('condition')"
                      >Condition</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('map')"
                      >Map</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('action')"
                      >Action</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('code')"
                      >Code</a-button
                    >
                    <a-button size="small" @click="focusFlowNode('compile')"
                      >Compile</a-button
                    >
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
                  :condition-count="countConditions"
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
                  <!-- Node port -->
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
                    <a-select v-model:value="trigger" style="width: 100%">
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
                      <span class="block-badge" style="background: #fa8c16"
                        >Block 2</span
                      >
                      <strong style="color: #fff"
                        >高级嵌套逻辑过滤条件 (Nested Condition Block)</strong
                      >
                    </div>
                  </div>
                  <div class="block-body">
                    <a-row :gutter="16">
                      <!-- Condition Tree -->
                      <a-col :span="15">
                        <div class="desc-line" style="margin-bottom: 16px">
                          支持无限嵌套的逻辑运算组，可从左侧拖拽条件或逻辑组至目标块内：
                        </div>

                        <div
                          class="conditions-list-tree"
                          style="
                            max-height: 380px;
                            overflow-y: auto;
                            padding-right: 4px;
                          "
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

                      <!-- Blueprint Logic Gate Visualizer (Fully integrated SVG tree) -->
                      <a-col
                        :span="9"
                        style="
                          border-left: 1px dashed rgba(255, 255, 255, 0.1);
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
                      * 物理文件 unlink 挂载于 Kprobe
                      上，不改变内核决策链，仅支持 ALERT 或 KILL 动作。其他 LSM
                      挂载点支持完整的 BLOCK、ALERT 与 KILL 动作。
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
                      <a-form-item
                        label="详细说明描述"
                        style="margin-bottom: 0"
                      >
                        <a-textarea v-model:value="description" :rows="2" />
                      </a-form-item>
                    </a-form>

                    <div
                      style="
                        margin-top: 20px;
                        display: flex;
                        justify-content: flex-end;
                      "
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
                  <a-tag color="purple">LLM Blocks Compiler</a-tag>
                  <span
                    >用后端 OpenAI 兼容 LLM 将自然语言内核防御意图编译成 Trigger
                    / Condition / Map / Action 积木流；生成后会自动回到 Dify
                    Workflow，继续拖线、删节点、调 Inspector。</span
                  >
                </div>
                <PluginsVisualAiPanel
                  v-model="aiPrompt"
                  @translate="handleAiTranslate"
                />
              </div>
            </a-tab-pane>
            <a-tab-pane key="source" tab="Generated eBPF C">
              <div
                ref="codeBlock"
                class="source-workspace-shell"
                :class="flowSectionClass('code')"
              >
                <div class="source-workspace-notice">
                  <a-tag color="cyan">独立源码 Tab</a-tag>
                  <span
                    >动态生成的 eBPF C 语言高阶过滤器源码、Clang
                    编译日志和加载入口集中在这里，主画布只负责 Dify
                    风格节点编排。</span
                  >
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
        :class="[
          `dock-${nodeLibraryDock}`,
          { dragging: !!nodeLibraryDragging },
        ]"
        :style="nodeLibraryStyle"
      >
        <div class="node-library-floating-toolbar">
          <button
            type="button"
            class="node-library-direction-button"
            :title="
              nodeLibraryDock === 'left'
                ? '贴左边缘隐藏节点类型'
                : '贴右边缘隐藏节点类型'
            "
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
        :title="
          nodeLibraryDock === 'left'
            ? '从左边缘展开节点类型'
            : '从右边缘展开节点类型'
        "
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
        :class="[
          `dock-${recipePanelDock}`,
          { dragging: !!recipePanelDragging },
        ]"
        :style="recipePanelStyle"
      >
        <div class="recipe-floating-toolbar">
          <button
            type="button"
            class="recipe-direction-button"
            :title="
              recipePanelDock === 'left' ? '贴左边缘隐藏' : '贴右边缘隐藏'
            "
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
          :condition-count="countConditions"
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
        :title="
          recipePanelDock === 'left'
            ? '从左边缘展开场景积木'
            : '从右边缘展开场景积木'
        "
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
  transition: left 0.18s ease, top 0.18s ease, opacity 0.2s ease,
    transform 0.2s ease, box-shadow 0.18s ease;
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
  transition: transform 0.16s ease, border-color 0.16s ease,
    background 0.16s ease;
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
  transition: transform 0.18s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease,
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
  transition: left 0.18s ease, top 0.18s ease, opacity 0.2s ease,
    transform 0.2s ease, box-shadow 0.18s ease;
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
  transition: transform 0.16s ease, border-color 0.16s ease,
    background 0.16s ease;
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
  transition: transform 0.18s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.18s ease,
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
  box-shadow: 0 0 0 1px rgba(56, 189, 248, 0.25),
    0 0 24px rgba(56, 189, 248, 0.2);
  border-radius: 10px;
  transition: outline-color 0.2s ease, box-shadow 0.2s ease;
}

.graphical-workspace {
  background-color: #0b132b;
  background-image: linear-gradient(
      to right,
      rgba(28, 37, 65, 0.4) 1px,
      transparent 1px
    ),
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
