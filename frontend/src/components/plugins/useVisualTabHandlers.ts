import { message } from "ant-design-vue";

import type {
  VisualAction,
  VisualConditionField,
  VisualFlowNodeId,
  VisualMapMode,
  VisualMapKey,
  VisualTrigger,
} from "./types";

import { isVisualConditionField } from "./validation";
import { triggerOptions } from "./constants";
import {
  visualFlowNodeIds,
  flowNodeDetails,
} from "./useFlowNodeManager";

export interface CanvasNodeTypeDropPayload {
  category: string;
  value: string;
  x: number;
  y: number;
}

interface UseVisualTabHandlersOptions {
  trigger: import("vue").Ref<VisualTrigger>;
  action: import("vue").Ref<VisualAction>;
  mapMode: import("vue").Ref<VisualMapMode>;
  activeFlowNode: import("vue").Ref<VisualFlowNodeId>;
  designerSubtab: import("vue").Ref<string>;
  onAddRule: (parent: string, field: VisualConditionField) => void;
  onAddGroup: (parent: string, type: "AND" | "OR") => void;
  restoreFlowNode: (id: VisualFlowNodeId) => void;
  focusFlowNode: (id: string) => Promise<void>;
  moveFlowNodeTo: (id: VisualFlowNodeId, x: number, y: number) => void;
}

export function useVisualTabHandlers(options: UseVisualTabHandlersOptions) {
  const {
    trigger,
    action,
    mapMode,
    activeFlowNode,
    designerSubtab,
    onAddRule,
    onAddGroup,
    restoreFlowNode,
    focusFlowNode,
    moveFlowNodeTo,
  } = options;

  // --- Node type library handlers ---
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

  // --- AI translate handler ---
  const handleAiTranslate = (payload: {
    trigger: VisualTrigger;
    action: VisualAction;
    conditions: import("./types").VisualLogicGroup;
    mapMode: VisualMapMode;
    mapKey: VisualMapKey;
    mapLimit: number;
  }, applyFn: (payload: {
    trigger: VisualTrigger;
    action: VisualAction;
    conditions: import("./types").VisualLogicGroup;
    mapMode: VisualMapMode;
    mapKey: VisualMapKey;
    mapLimit: number;
  }) => void) => {
    applyFn(payload);
  };

  // --- Canvas drop handler ---
  const applyNodeTypeDrop = (
    category: string,
    value: string,
    position?: { x: number; y: number }
  ) => {
    let targetNode: VisualFlowNodeId | null = null;
    let statusText = "";

    const visualMapModeSet = new Set<VisualMapMode>(["NONE", "COUNTER", "BLOCKLIST"]);
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

  // --- Keyboard shortcuts ---
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

  const createHistoryShortcutHandler = (
    undoWorkspace: () => Promise<void>,
    redoWorkspace: () => Promise<void>
  ) => {
    return (event: KeyboardEvent) => {
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
  };

  return {
    selectTriggerNodeType,
    addConditionNodeType,
    addLogicNodeType,
    setMapNodeType,
    setActionNodeType,
    handleAiTranslate,
    applyNodeTypeDrop,
    handleCanvasNodeTypeDrop,
    handleWorkspaceDrop,
    createHistoryShortcutHandler,
  };
}
