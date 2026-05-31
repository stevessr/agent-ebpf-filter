import { computed, nextTick, useTemplateRef } from "vue";
import { message } from "ant-design-vue";
import type {
  VisualFlowNodeId,
  VisualWireId,
  VisualWireStates,
  VisualHiddenNodeStates,
  VisualNodeLayout,
  VisualTrigger,
  VisualAction,
} from "./types";

// --- Constants ---

export const visualWireIds: VisualWireId[] = [
  "trigger-condition",
  "condition-map",
  "map-action",
  "condition-code",
  "map-code",
  "action-compile",
  "code-compile",
];

export const visualWireLabels: Record<VisualWireId, string> = {
  "trigger-condition": "Trigger → Condition",
  "condition-map": "Condition → Map",
  "map-action": "Map → Action",
  "condition-code": "Condition → Code",
  "map-code": "Map → Code",
  "action-compile": "Action → Compile",
  "code-compile": "Code → Compile",
};

export const visualWireEndpoints: Record<
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

export const visualFlowNodeIds: VisualFlowNodeId[] = [
  "trigger",
  "condition",
  "map",
  "action",
  "code",
  "compile",
];

export const flowNodeDetails: Record<
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

export const createDefaultNodeLayout = (): Record<
  VisualFlowNodeId,
  { x: number; y: number }
> => ({
  trigger: { x: 24, y: 38 },
  condition: { x: 196, y: 38 },
  map: { x: 368, y: 38 },
  action: { x: 540, y: 38 },
  code: { x: 368, y: 176 },
  compile: { x: 540, y: 176 },
});

export const createDefaultWireStates = (): Record<VisualWireId, boolean> => ({
  "trigger-condition": true,
  "condition-map": true,
  "map-action": true,
  "condition-code": true,
  "map-code": true,
  "action-compile": true,
  "code-compile": true,
});

export const createDefaultHiddenNodes = (): VisualHiddenNodeStates => ({});

export const mergeWireStates = (
  states?: VisualWireStates,
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

// --- Composable ---

export interface UseFlowNodeManagerOptions {
  wireStates: { value: VisualWireStates };
  hiddenFlowNodes: { value: VisualHiddenNodeStates };
  activeFlowNode: { value: VisualFlowNodeId };
  designerSubtab: { value: "dify" | "map" | "nlp" | "source" };
  nodeLayout: { value: VisualNodeLayout };
  trigger: { value: VisualTrigger };
  action: { value: VisualAction };
}

export function useFlowNodeManager(opts: UseFlowNodeManagerOptions) {
  const triggerBlockRef = useTemplateRef<HTMLElement>("triggerBlock");
  const conditionBlockRef = useTemplateRef<HTMLElement>("conditionBlock");
  const mapBlockRef = useTemplateRef<HTMLElement>("mapBlock");
  const actionBlockRef = useTemplateRef<HTMLElement>("actionBlock");
  const compileBlockRef = useTemplateRef<HTMLElement>("compileBlock");
  const codeBlockRef = useTemplateRef<HTMLElement>("codeBlock");

  const selectedFlowNodeDetail = computed(
    () => flowNodeDetails[opts.activeFlowNode.value],
  );

  const nodeWireIds = (node: VisualFlowNodeId) =>
    visualWireIds.filter((id) => {
      const endpoint = visualWireEndpoints[id];
      return endpoint.from === node || endpoint.to === node;
    });

  const resetNodeLayout = () => {
    opts.nodeLayout.value = createDefaultNodeLayout();
    message.success("已恢复低代码节点画布自动布局");
  };

  const resetWireStates = () => {
    opts.wireStates.value = createDefaultWireStates();
    message.success("已重新连接全部低代码流程线缆");
  };

  const restoreFlowNode = (node: VisualFlowNodeId, reconnect = true) => {
    if (!opts.hiddenFlowNodes.value[node]) return;
    const nextHidden = { ...opts.hiddenFlowNodes.value };
    delete nextHidden[node];
    opts.hiddenFlowNodes.value = nextHidden;
    if (reconnect) {
      const nextWires = { ...mergeWireStates(opts.wireStates.value) };
      nodeWireIds(node).forEach((wireId) => {
        const endpoint = visualWireEndpoints[wireId];
        if (!nextHidden[endpoint.from] && !nextHidden[endpoint.to]) {
          nextWires[wireId] = true;
        }
      });
      opts.wireStates.value = nextWires;
    }
  };

  const handleDeleteFlowNode = (node: VisualFlowNodeId) => {
    if (node === "trigger" || node === "compile") {
      message.warning("Trigger 入口和 Compile 出口是流程骨架，不能删除");
      return;
    }
    opts.hiddenFlowNodes.value = {
      ...opts.hiddenFlowNodes.value,
      [node]: true,
    };
    const nextWires = { ...mergeWireStates(opts.wireStates.value) };
    nodeWireIds(node).forEach((wireId) => {
      nextWires[wireId] = false;
    });
    opts.wireStates.value = nextWires;
    if (opts.activeFlowNode.value === node) {
      opts.activeFlowNode.value = "trigger";
    }
    message.warning(
      `已从画布删除 ${flowNodeDetails[node].label}，可从左侧节点类型库重新添加`,
    );
  };

  const selectFlowNode = (node: VisualFlowNodeId) => {
    opts.activeFlowNode.value = node;
  };

  const focusFlowNode = async (node: VisualFlowNodeId) => {
    restoreFlowNode(node);
    opts.activeFlowNode.value = node;
    if (node === "code") {
      opts.designerSubtab.value = "source";
    } else if (node === "map" || node === "condition" || node === "action") {
      opts.designerSubtab.value = "map";
    } else {
      opts.designerSubtab.value = "dify";
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
    "flow-section-active": opts.activeFlowNode.value === node,
  });

  const moveFlowNodeTo = (node: VisualFlowNodeId, x: number, y: number) => {
    opts.nodeLayout.value = {
      ...opts.nodeLayout.value,
      [node]: { x: Math.round(x), y: Math.round(y) },
    };
  };

  return {
    triggerBlockRef,
    conditionBlockRef,
    mapBlockRef,
    actionBlockRef,
    compileBlockRef,
    codeBlockRef,
    selectedFlowNodeDetail,
    resetNodeLayout,
    resetWireStates,
    restoreFlowNode,
    handleDeleteFlowNode,
    selectFlowNode,
    focusFlowNode,
    flowSectionClass,
    moveFlowNodeTo,
    nodeWireIds,
  };
}
