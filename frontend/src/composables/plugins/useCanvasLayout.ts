import { computed, ref } from "vue";
import type {
  VisualAction,
  VisualFlowNodeId,
  VisualHiddenNodeStates,
  VisualMapMode,
  VisualNodeLayout,
  VisualTrigger,
  VisualWireId,
  VisualWireStates,
} from "../../components/plugins/types";
import { visualWorkflowTheme } from "../../components/plugins/theme";

export const NODE_WIDTH = 160;
export const NODE_HEIGHT = 72;
export const GRID_SIZE = 24;
export const DEFAULT_CANVAS_WIDTH = 920;
export const DEFAULT_CANVAS_HEIGHT = 420;
export const MIN_CANVAS_WIDTH = 720;
export const MIN_CANVAS_HEIGHT = 340;
export const MAX_CANVAS_WIDTH = 1800;
export const MAX_CANVAS_HEIGHT = 1100;
export const CANVAS_PADDING = 12;

export const DEFAULT_LAYOUT: VisualNodeLayout = {
  trigger: { x: 24, y: 38 },
  condition: { x: 196, y: 38 },
  map: { x: 368, y: 38 },
  action: { x: 540, y: 38 },
  code: { x: 368, y: 176 },
  compile: { x: 540, y: 176 },
};

export const FLOW_NODE_IDS: VisualFlowNodeId[] = [
  "trigger",
  "condition",
  "map",
  "action",
  "code",
  "compile",
];

export const WIRE_DEFINITIONS: Array<{
  id: VisualWireId;
  from: VisualFlowNodeId;
  to: VisualFlowNodeId;
  label: string;
  required: boolean;
}> = [
  {
    id: "trigger-condition",
    from: "trigger",
    to: "condition",
    label: "event ctx",
    required: true,
  },
  {
    id: "condition-map",
    from: "condition",
    to: "map",
    label: "match",
    required: true,
  },
  {
    id: "map-action",
    from: "map",
    to: "action",
    label: "state",
    required: true,
  },
  {
    id: "condition-code",
    from: "condition",
    to: "code",
    label: "expr",
    required: true,
  },
  { id: "map-code", from: "map", to: "code", label: "map def", required: true },
  {
    id: "action-compile",
    from: "action",
    to: "compile",
    label: "decision",
    required: true,
  },
  {
    id: "code-compile",
    from: "code",
    to: "compile",
    label: "source",
    required: true,
  },
];

export const DEFAULT_WIRE_STATES: Record<VisualWireId, boolean> = {
  "trigger-condition": true,
  "condition-map": true,
  "map-action": true,
  "condition-code": true,
  "map-code": true,
  "action-compile": true,
  "code-compile": true,
};

export const clamp = (value: number, min: number, max: number) =>
  Math.min(max, Math.min(min, value));

export const snapToGrid = (value: number) =>
  Math.round(value / GRID_SIZE) * GRID_SIZE;

export interface UseCanvasLayoutParams {
  nodeLayout: VisualNodeLayout;
  wireStates: VisualWireStates;
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  conditionCount: number;
  treeDepth: number;
  codeLines: number;
  compileReady: boolean;
  hiddenNodes: VisualHiddenNodeStates;
}

export function useCanvasLayout(params: () => UseCanvasLayoutParams) {
  const theme = visualWorkflowTheme;

  const mergedLayout = computed<VisualNodeLayout>(() => ({
    ...DEFAULT_LAYOUT,
    ...params().nodeLayout,
  }));

  const mergedWireStates = computed<Record<VisualWireId, boolean>>(() => ({
    ...DEFAULT_WIRE_STATES,
    ...params().wireStates,
  }));

  const canvasSize = ref({
    width: DEFAULT_CANVAS_WIDTH,
    height: DEFAULT_CANVAS_HEIGHT,
  });

  const maxNodeX = computed(() =>
    Math.max(
      CANVAS_PADDING,
      canvasSize.value.width - NODE_WIDTH - CANVAS_PADDING,
    ),
  );

  const maxNodeY = computed(() =>
    Math.max(
      CANVAS_PADDING,
      canvasSize.value.height - NODE_HEIGHT - CANVAS_PADDING,
    ),
  );

  const snapNodeX = (value: number) =>
    clamp(snapToGrid(Math.round(value)), CANVAS_PADDING, maxNodeX.value);

  const snapNodeY = (value: number) =>
    clamp(snapToGrid(Math.round(value)), CANVAS_PADDING, maxNodeY.value);

  const boundedLayout = computed<VisualNodeLayout>(() => {
    const next: VisualNodeLayout = {};
    FLOW_NODE_IDS.forEach((id) => {
      const position = mergedLayout.value[id] || DEFAULT_LAYOUT[id];
      next[id] = {
        x: clamp(Math.round(position.x), CANVAS_PADDING, maxNodeX.value),
        y: clamp(Math.round(position.y), CANVAS_PADDING, maxNodeY.value),
      };
    });
    return next;
  });

  const canvasViewBox = computed(
    () =>
      `0 0 ${Math.max(1, Math.round(canvasSize.value.width))} ${Math.max(
        1,
        Math.round(canvasSize.value.height),
      )}`,
  );

  const flowNodes = computed(() => {
    const p = params();
    return [
      {
        id: "trigger" as const,
        title: "Trigger Block",
        subtitle: p.trigger,
        badge: "HOOK",
        color: theme.primary,
        hint: "选择 eBPF/LSM/cgroup 挂载入口，决定后续条件能读取哪些上下文。",
      },
      {
        id: "condition" as const,
        title: "Condition Tree",
        subtitle: `${p.conditionCount} 条件 / ${p.treeDepth} 层`,
        badge: "AND/OR",
        color: theme.primaryHover,
        hint: "组合 comm、uid、basename、port、ipv4 等条件，生成内核布尔表达式。",
      },
      {
        id: "map" as const,
        title: "State Map",
        subtitle: p.mapMode,
        badge: "BPF MAP",
        color: theme.primary,
        hint: "声明 COUNTER / BLOCKLIST 等状态 map，把一次性判断升级为状态化策略。",
      },
      {
        id: "action" as const,
        title: "Action Block",
        subtitle: p.action,
        badge: "DECISION",
        color:
          p.action === "ALERT"
            ? theme.warning
            : p.action === "KILL"
              ? theme.danger
              : theme.primary,
        hint: "配置命中后的内核动作：告警、返回拒绝，或发送 SIGKILL。",
      },
      {
        id: "code" as const,
        title: "Generated C",
        subtitle: `${p.codeLines} lines`,
        badge: "SOURCE",
        color: theme.primaryHover,
        hint: "实时预览由积木转译出的 libbpf C 源码和编译日志。",
      },
      {
        id: "compile" as const,
        title: "Compile Gate",
        subtitle: p.compileReady ? "ready" : "fix required",
        badge: p.compileReady ? "READY" : "ERROR",
        color: p.compileReady ? theme.success : theme.danger,
        hint: "检查插件元数据和 verifier 友好约束，通过后注册、编译并加载。",
      },
    ];
  });

  const isNodeHidden = (id: VisualFlowNodeId) => !!params().hiddenNodes[id];

  const visibleFlowNodes = computed(() =>
    flowNodes.value.filter((node) => !isNodeHidden(node.id)),
  );

  const visibleWireDefinitions = computed(() =>
    WIRE_DEFINITIONS.filter(
      (wire) => !isNodeHidden(wire.from) && !isNodeHidden(wire.to),
    ),
  );

  const wires = computed(() => {
    return visibleWireDefinitions.value.map((wire) => {
      const start = boundedLayout.value[wire.from] || DEFAULT_LAYOUT[wire.from];
      const end = boundedLayout.value[wire.to] || DEFAULT_LAYOUT[wire.to];
      const x1 = start.x + NODE_WIDTH;
      const y1 = start.y + NODE_HEIGHT / 2;
      const x2 = end.x;
      const y2 = end.y + NODE_HEIGHT / 2;
      const mid = Math.max(24, Math.abs(x2 - x1) / 2);
      return {
        ...wire,
        enabled: mergedWireStates.value[wire.id],
        d: `M ${x1} ${y1} C ${x1 + mid} ${y1}, ${x2 - mid} ${y2}, ${x2} ${y2}`,
        labelX: clamp(
          (x1 + x2) / 2 - 52,
          6,
          Math.max(6, canvasSize.value.width - 116),
        ),
        labelY: clamp(
          (y1 + y2) / 2 - 13,
          6,
          Math.max(6, canvasSize.value.height - 34),
        ),
      };
    });
  });

  const connectedWireCount = computed(
    () =>
      visibleWireDefinitions.value.filter(
        (wire) => mergedWireStates.value[wire.id],
      ).length,
  );

  const getNodePosition = (id: VisualFlowNodeId) =>
    boundedLayout.value[id] || DEFAULT_LAYOUT[id];

  const getNodePortPoint = (id: VisualFlowNodeId, side: "in" | "out") => {
    const position = getNodePosition(id);
    return {
      x: position.x + (side === "out" ? NODE_WIDTH : 0),
      y: position.y + NODE_HEIGHT / 2,
    };
  };

  const getWireForEndpoints = (from: VisualFlowNodeId, to: VisualFlowNodeId) =>
    WIRE_DEFINITIONS.find((wire) => wire.from === from && wire.to === to) ||
    null;

  const hasOutgoingWire = (id: VisualFlowNodeId) =>
    visibleWireDefinitions.value.some((wire) => wire.from === id);

  const hasIncomingWire = (id: VisualFlowNodeId) =>
    visibleWireDefinitions.value.some((wire) => wire.to === id);

  const canDeleteNode = (id: VisualFlowNodeId) =>
    id !== "trigger" && id !== "compile";

  return {
    theme,
    mergedLayout,
    mergedWireStates,
    canvasSize,
    maxNodeX,
    maxNodeY,
    snapNodeX,
    snapNodeY,
    boundedLayout,
    canvasViewBox,
    flowNodes,
    visibleFlowNodes,
    visibleWireDefinitions,
    wires,
    connectedWireCount,
    getNodePosition,
    getNodePortPoint,
    getWireForEndpoints,
    hasOutgoingWire,
    hasIncomingWire,
    canDeleteNode,
    isNodeHidden,
  };
}
