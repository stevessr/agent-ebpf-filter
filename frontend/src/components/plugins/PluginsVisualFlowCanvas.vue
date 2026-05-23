<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useTemplateRef } from "vue";
import { ReloadOutlined } from "@ant-design/icons-vue";
import type {
  VisualAction,
  VisualFlowNodeId,
  VisualHiddenNodeStates,
  VisualMapMode,
  VisualNodeLayout,
  VisualTrigger,
  VisualWireId,
  VisualWireStates,
} from "./types";

const props = defineProps<{
  nodeLayout: VisualNodeLayout;
  wireStates: VisualWireStates;
  selectedNodeId: VisualFlowNodeId;
  trigger: VisualTrigger;
  action: VisualAction;
  mapMode: VisualMapMode;
  conditionCount: number;
  treeDepth: number;
  codeLines: number;
  compileReady: boolean;
  hiddenNodes: VisualHiddenNodeStates;
}>();

const emit = defineEmits<{
  (e: "update:nodeLayout", value: VisualNodeLayout): void;
  (e: "update:wireStates", value: VisualWireStates): void;
  (e: "update:selectedNodeId", value: VisualFlowNodeId): void;
  (e: "reset-layout"): void;
  (e: "reset-wires"): void;
  (e: "delete-node", value: VisualFlowNodeId): void;
  (
    e: "drop-node-type",
    value: { category: string; value: string; x: number; y: number }
  ): void;
}>();

const nodeWidth = 160;
const nodeHeight = 72;
const gridSize = 24;
const defaultCanvasWidth = 920;
const defaultCanvasHeight = 420;
const minCanvasWidth = 720;
const minCanvasHeight = 340;
const maxCanvasWidth = 1800;
const maxCanvasHeight = 1100;
const canvasPadding = 12;
const viewportRef = useTemplateRef<HTMLElement>("viewport");
const canvasRef = useTemplateRef<HTMLElement>("canvas");
const viewportSize = ref({ width: defaultCanvasWidth });
const workspaceSize = ref({
  width: defaultCanvasWidth,
  height: defaultCanvasHeight,
});
let viewportResizeObserver: ResizeObserver | null = null;
const dragging = ref<{
  id: VisualFlowNodeId;
  offsetX: number;
  offsetY: number;
  startX: number;
  startY: number;
  moved: boolean;
} | null>(null);
const connectionDrag = ref<{
  from: VisualFlowNodeId;
  startX: number;
  startY: number;
  currentX: number;
  currentY: number;
  target: VisualFlowNodeId | null;
  valid: boolean;
} | null>(null);
const canvasResizeDrag = ref<{
  startX: number;
  startY: number;
  startWidth: number;
  startHeight: number;
} | null>(null);

const defaultLayout: VisualNodeLayout = {
  trigger: { x: 24, y: 38 },
  condition: { x: 196, y: 38 },
  map: { x: 368, y: 38 },
  action: { x: 540, y: 38 },
  code: { x: 368, y: 176 },
  compile: { x: 540, y: 176 },
};

const flowNodeIds: VisualFlowNodeId[] = [
  "trigger",
  "condition",
  "map",
  "action",
  "code",
  "compile",
];

const clamp = (value: number, min: number, max: number) =>
  Math.min(max, Math.max(min, value));

const snapToGrid = (value: number) => Math.round(value / gridSize) * gridSize;

const snapNodeX = (value: number) =>
  clamp(snapToGrid(Math.round(value)), canvasPadding, maxNodeX.value);

const snapNodeY = (value: number) =>
  clamp(snapToGrid(Math.round(value)), canvasPadding, maxNodeY.value);

const wireDefinitions: Array<{
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
  {
    id: "map-code",
    from: "map",
    to: "code",
    label: "map def",
    required: true,
  },
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

const defaultWireStates: Record<VisualWireId, boolean> = {
  "trigger-condition": true,
  "condition-map": true,
  "map-action": true,
  "condition-code": true,
  "map-code": true,
  "action-compile": true,
  "code-compile": true,
};

const mergedLayout = computed<VisualNodeLayout>(() => ({
  ...defaultLayout,
  ...props.nodeLayout,
}));

const mergedWireStates = computed<Record<VisualWireId, boolean>>(() => ({
  ...defaultWireStates,
  ...props.wireStates,
}));

const canvasSize = computed(() => ({
  width: Math.max(
    minCanvasWidth,
    workspaceSize.value.width,
    Math.round(viewportSize.value.width)
  ),
  height: workspaceSize.value.height,
}));

const canvasStyle = computed(() => ({
  width: `${canvasSize.value.width}px`,
  height: `${canvasSize.value.height}px`,
}));

const maxNodeX = computed(() =>
  Math.max(canvasPadding, canvasSize.value.width - nodeWidth - canvasPadding)
);

const maxNodeY = computed(() =>
  Math.max(canvasPadding, canvasSize.value.height - nodeHeight - canvasPadding)
);

const boundedLayout = computed<VisualNodeLayout>(() => {
  const next: VisualNodeLayout = {};
  flowNodeIds.forEach((id) => {
    const position = mergedLayout.value[id] || defaultLayout[id];
    next[id] = {
      x: clamp(Math.round(position.x), canvasPadding, maxNodeX.value),
      y: clamp(Math.round(position.y), canvasPadding, maxNodeY.value),
    };
  });
  return next;
});

const canvasViewBox = computed(
  () =>
    `0 0 ${Math.max(1, Math.round(canvasSize.value.width))} ${Math.max(
      1,
      Math.round(canvasSize.value.height)
    )}`
);

const flowNodes = computed(() => [
  {
    id: "trigger" as const,
    title: "Trigger Block",
    subtitle: props.trigger,
    badge: "HOOK",
    color: "#1890ff",
    hint: "选择 eBPF/LSM/cgroup 挂载入口，决定后续条件能读取哪些上下文。",
  },
  {
    id: "condition" as const,
    title: "Condition Tree",
    subtitle: `${props.conditionCount} 条件 / ${props.treeDepth} 层`,
    badge: "AND/OR",
    color: "#fa8c16",
    hint: "组合 comm、uid、basename、port、ipv4 等条件，生成内核布尔表达式。",
  },
  {
    id: "map" as const,
    title: "State Map",
    subtitle: props.mapMode,
    badge: "BPF MAP",
    color: "#722ed1",
    hint: "声明 COUNTER / BLOCKLIST 等状态 map，把一次性判断升级为状态化策略。",
  },
  {
    id: "action" as const,
    title: "Action Block",
    subtitle: props.action,
    badge: "DECISION",
    color:
      props.action === "ALERT"
        ? "#fadb14"
        : props.action === "KILL"
        ? "#ff4d4f"
        : "#52c41a",
    hint: "配置命中后的内核动作：告警、返回拒绝，或发送 SIGKILL。",
  },
  {
    id: "code" as const,
    title: "Generated C",
    subtitle: `${props.codeLines} lines`,
    badge: "SOURCE",
    color: "#13c2c2",
    hint: "实时预览由积木转译出的 libbpf C 源码和编译日志。",
  },
  {
    id: "compile" as const,
    title: "Compile Gate",
    subtitle: props.compileReady ? "ready" : "fix required",
    badge: props.compileReady ? "READY" : "ERROR",
    color: props.compileReady ? "#52c41a" : "#ff4d4f",
    hint: "检查插件元数据和 verifier 友好约束，通过后注册、编译并加载。",
  },
]);

const selectedNode = computed(
  () =>
    flowNodes.value.find((node) => node.id === props.selectedNodeId) ||
    flowNodes.value[0]
);

const isNodeHidden = (id: VisualFlowNodeId) => !!props.hiddenNodes[id];

const visibleFlowNodes = computed(() =>
  flowNodes.value.filter((node) => !isNodeHidden(node.id))
);

const visibleWireDefinitions = computed(() =>
  wireDefinitions.filter(
    (wire) => !isNodeHidden(wire.from) && !isNodeHidden(wire.to)
  )
);

const wires = computed(() => {
  return visibleWireDefinitions.value.map((wire) => {
    const start = boundedLayout.value[wire.from] || defaultLayout[wire.from];
    const end = boundedLayout.value[wire.to] || defaultLayout[wire.to];
    const x1 = start.x + nodeWidth;
    const y1 = start.y + nodeHeight / 2;
    const x2 = end.x;
    const y2 = end.y + nodeHeight / 2;
    const mid = Math.max(24, Math.abs(x2 - x1) / 2);
    return {
      ...wire,
      enabled: mergedWireStates.value[wire.id],
      d: `M ${x1} ${y1} C ${x1 + mid} ${y1}, ${x2 - mid} ${y2}, ${x2} ${y2}`,
      labelX: clamp(
        (x1 + x2) / 2 - 52,
        6,
        Math.max(6, canvasSize.value.width - 116)
      ),
      labelY: clamp(
        (y1 + y2) / 2 - 13,
        6,
        Math.max(6, canvasSize.value.height - 34)
      ),
    };
  });
});

const connectedWireCount = computed(
  () =>
    visibleWireDefinitions.value.filter((wire) => mergedWireStates.value[wire.id])
      .length
);

const connectionPreviewPath = computed(() => {
  if (!connectionDrag.value) return "";
  const { startX, startY, currentX, currentY } = connectionDrag.value;
  const mid = Math.max(28, Math.abs(currentX - startX) / 2);
  return `M ${startX} ${startY} C ${startX + mid} ${startY}, ${
    currentX - mid
  } ${currentY}, ${currentX} ${currentY}`;
});

const getNodePosition = (id: VisualFlowNodeId) =>
  boundedLayout.value[id] || defaultLayout[id];

const getNodePortPoint = (id: VisualFlowNodeId, side: "in" | "out") => {
  const position = getNodePosition(id);
  return {
    x: position.x + (side === "out" ? nodeWidth : 0),
    y: position.y + nodeHeight / 2,
  };
};

const canvasPointFromEvent = (event: PointerEvent) => {
  const canvas = canvasRef.value;
  if (!canvas) return null;
  const rect = canvas.getBoundingClientRect();
  return {
    x: clamp(event.clientX - rect.left, 0, rect.width),
    y: clamp(event.clientY - rect.top, 0, rect.height),
  };
};

const canvasPointFromDragEvent = (event: DragEvent) => {
  const canvas = canvasRef.value;
  if (!canvas) return null;
  const rect = canvas.getBoundingClientRect();
  const scaleX = rect.width > 0 ? canvasSize.value.width / rect.width : 1;
  const scaleY = rect.height > 0 ? canvasSize.value.height / rect.height : 1;
  return {
    x: clamp((event.clientX - rect.left) * scaleX, 0, canvasSize.value.width),
    y: clamp((event.clientY - rect.top) * scaleY, 0, canvasSize.value.height),
  };
};

const getWireForEndpoints = (from: VisualFlowNodeId, to: VisualFlowNodeId) =>
  wireDefinitions.find((wire) => wire.from === from && wire.to === to) || null;

const hasOutgoingWire = (id: VisualFlowNodeId) =>
  visibleWireDefinitions.value.some((wire) => wire.from === id);

const hasIncomingWire = (id: VisualFlowNodeId) =>
  visibleWireDefinitions.value.some((wire) => wire.to === id);

const canDeleteNode = (id: VisualFlowNodeId) =>
  id !== "trigger" && id !== "compile";

const getNearestInputNode = (
  point: { x: number; y: number },
  from: VisualFlowNodeId
): VisualFlowNodeId | null => {
  let nearestId: VisualFlowNodeId | null = null;
  let nearestDistance = Infinity;
  visibleFlowNodes.value.forEach((node) => {
    if (node.id === from || !hasIncomingWire(node.id)) return;
    const input = getNodePortPoint(node.id, "in");
    const distance = Math.hypot(point.x - input.x, point.y - input.y);
    if (distance <= 34 && distance < nearestDistance) {
      nearestId = node.id;
      nearestDistance = distance;
    }
  });
  return nearestId;
};

const updateNodePosition = (id: VisualFlowNodeId, x: number, y: number) => {
  const next = {
    ...mergedLayout.value,
    [id]: {
      x: snapNodeX(x),
      y: snapNodeY(y),
    },
  };
  emit("update:nodeLayout", next);
};

const handleCanvasDragOver = (event: DragEvent) => {
  event.preventDefault();
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = "move";
  }
};

const handleCanvasDrop = (event: DragEvent) => {
  event.preventDefault();
  event.stopPropagation();
  const point = canvasPointFromDragEvent(event);
  const rawData = event.dataTransfer?.getData("text/plain");
  if (!point || !rawData) return;

  try {
    const parsed = JSON.parse(rawData) as {
      category?: unknown;
      value?: unknown;
    };
    if (typeof parsed.category !== "string" || typeof parsed.value !== "string") {
      return;
    }
    emit("drop-node-type", {
      category: parsed.category,
      value: parsed.value,
      x: snapNodeX(point.x - nodeWidth / 2),
      y: snapNodeY(point.y - nodeHeight / 2),
    });
  } catch (err) {
    console.error("Failed to parse node type drop payload:", err);
  }
};

const toggleWire = (id: VisualWireId) => {
  emit("update:wireStates", {
    ...mergedWireStates.value,
    [id]: !mergedWireStates.value[id],
  });
};

const enableWire = (id: VisualWireId) => {
  if (mergedWireStates.value[id]) return;
  emit("update:wireStates", {
    ...mergedWireStates.value,
    [id]: true,
  });
};

const stopDragging = () => {
  if (dragging.value && !dragging.value.moved) {
    emit("update:selectedNodeId", dragging.value.id);
  }
  dragging.value = null;
  window.removeEventListener("pointermove", handlePointerMove);
  window.removeEventListener("pointerup", stopDragging);
};

const handlePointerMove = (event: PointerEvent) => {
  if (!dragging.value) return;
  const canvas = canvasRef.value;
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  if (
    Math.abs(event.clientX - dragging.value.startX) > 3 ||
    Math.abs(event.clientY - dragging.value.startY) > 3
  ) {
    dragging.value.moved = true;
  }
  updateNodePosition(
    dragging.value.id,
    event.clientX - rect.left - dragging.value.offsetX,
    event.clientY - rect.top - dragging.value.offsetY
  );
};

const handlePointerDown = (event: PointerEvent, id: VisualFlowNodeId) => {
  const target = event.currentTarget as HTMLElement;
  const rect = target.getBoundingClientRect();
  dragging.value = {
    id,
    offsetX: event.clientX - rect.left,
    offsetY: event.clientY - rect.top,
    startX: event.clientX,
    startY: event.clientY,
    moved: false,
  };
  window.addEventListener("pointermove", handlePointerMove);
  window.addEventListener("pointerup", stopDragging);
};

const handleConnectionPointerMove = (event: PointerEvent) => {
  if (!connectionDrag.value) return;
  const point = canvasPointFromEvent(event);
  if (!point) return;
  const target = getNearestInputNode(point, connectionDrag.value.from);
  connectionDrag.value = {
    ...connectionDrag.value,
    currentX: point.x,
    currentY: point.y,
    target,
    valid: !!target && !!getWireForEndpoints(connectionDrag.value.from, target),
  };
};

const stopConnectionDragging = () => {
  if (connectionDrag.value?.target && connectionDrag.value.valid) {
    const wire = getWireForEndpoints(
      connectionDrag.value.from,
      connectionDrag.value.target
    );
    if (wire) {
      enableWire(wire.id);
      emit("update:selectedNodeId", wire.to);
    }
  }
  connectionDrag.value = null;
  window.removeEventListener("pointermove", handleConnectionPointerMove);
  window.removeEventListener("pointerup", stopConnectionDragging);
};

const handleConnectionPointerDown = (event: PointerEvent, id: VisualFlowNodeId) => {
  if (!hasOutgoingWire(id)) return;
  const point = canvasPointFromEvent(event);
  const start = getNodePortPoint(id, "out");
  connectionDrag.value = {
    from: id,
    startX: start.x,
    startY: start.y,
    currentX: point?.x ?? start.x,
    currentY: point?.y ?? start.y,
    target: null,
    valid: false,
  };
  emit("update:selectedNodeId", id);
  window.addEventListener("pointermove", handleConnectionPointerMove);
  window.addEventListener("pointerup", stopConnectionDragging);
};

const applyWorkspaceSize = (width: number, height: number) => {
  workspaceSize.value = {
    width: clamp(Math.round(width), minCanvasWidth, maxCanvasWidth),
    height: clamp(Math.round(height), minCanvasHeight, maxCanvasHeight),
  };
};

const expandWorkspace = () => {
  applyWorkspaceSize(canvasSize.value.width + 260, canvasSize.value.height + 160);
};

const resetWorkspaceSize = () => {
  applyWorkspaceSize(defaultCanvasWidth, defaultCanvasHeight);
};

const stopCanvasResizing = () => {
  canvasResizeDrag.value = null;
  window.removeEventListener("pointermove", handleCanvasResizeMove);
  window.removeEventListener("pointerup", stopCanvasResizing);
};

const handleCanvasResizeMove = (event: PointerEvent) => {
  if (!canvasResizeDrag.value) return;
  const deltaX = event.clientX - canvasResizeDrag.value.startX;
  const deltaY = event.clientY - canvasResizeDrag.value.startY;
  applyWorkspaceSize(
    canvasResizeDrag.value.startWidth + deltaX,
    canvasResizeDrag.value.startHeight + deltaY
  );
};

const handleCanvasResizePointerDown = (event: PointerEvent) => {
  canvasResizeDrag.value = {
    startX: event.clientX,
    startY: event.clientY,
    startWidth: canvasSize.value.width,
    startHeight: canvasSize.value.height,
  };
  window.addEventListener("pointermove", handleCanvasResizeMove);
  window.addEventListener("pointerup", stopCanvasResizing);
};

const updateViewportSize = () => {
  const viewport = viewportRef.value;
  if (!viewport) return;
  const rect = viewport.getBoundingClientRect();
  viewportSize.value = {
    width: Math.max(minCanvasWidth, Math.round(rect.width)),
  };
};

onMounted(() => {
  updateViewportSize();
  if (typeof ResizeObserver !== "undefined" && viewportRef.value) {
    viewportResizeObserver = new ResizeObserver(updateViewportSize);
    viewportResizeObserver.observe(viewportRef.value);
  }
});

onBeforeUnmount(() => {
  stopDragging();
  stopCanvasResizing();
  connectionDrag.value = null;
  viewportResizeObserver?.disconnect();
  viewportResizeObserver = null;
  window.removeEventListener("pointermove", handleConnectionPointerMove);
  window.removeEventListener("pointerup", stopConnectionDragging);
});
</script>

<template>
  <div class="flow-shell">
    <div class="flow-header">
      <div>
        <h4>低代码节点拓扑画布 (Draggable Blueprint Canvas)</h4>
        <span>从左侧拖节点类型到画布会按 24px 网格吸附；拖节点端口可编辑路由，拖动底部手柄可拉开工作区。</span>
      </div>
      <a-space size="small" wrap>
        <a-tag color="blue">{{ canvasSize.width }}×{{ canvasSize.height }}</a-tag>
        <a-tag color="geekblue">24px snap</a-tag>
        <a-tag color="cyan">route edit</a-tag>
        <a-tag :color="connectedWireCount === visibleWireDefinitions.length ? 'green' : 'orange'">
          {{ connectedWireCount }}/{{ visibleWireDefinitions.length }} wires
        </a-tag>
        <a-button size="small" @click="expandWorkspace">展开画布</a-button>
        <a-button size="small" @click="resetWorkspaceSize">重置尺寸</a-button>
        <a-button size="small" @click="emit('reset-wires')">重连线缆</a-button>
        <a-button size="small" @click="emit('reset-layout')">
          <template #icon><ReloadOutlined /></template>
          自动布局
        </a-button>
      </a-space>
    </div>

    <div class="flow-inspector">
      <a-tag :color="selectedNode.color" class="inspector-tag">
        {{ selectedNode.badge }}
      </a-tag>
      <div>
        <strong>{{ selectedNode.title }}</strong>
        <span>{{ selectedNode.hint }}</span>
      </div>
    </div>

    <div ref="viewport" class="flow-canvas-viewport">
      <div
        ref="canvas"
        class="flow-canvas"
        :class="{ resizing: !!canvasResizeDrag }"
        :style="canvasStyle"
        @dragover="handleCanvasDragOver"
        @drop="handleCanvasDrop"
      >
        <div class="drop-target-hint">
          拖入节点类型创建/恢复节点 · 节点移动自动吸附网格
        </div>
        <svg class="flow-wires" :viewBox="canvasViewBox" preserveAspectRatio="none">
        <defs>
          <linearGradient id="flow-wire-gradient" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stop-color="#1890ff" />
            <stop offset="45%" stop-color="#722ed1" />
            <stop offset="100%" stop-color="#52c41a" />
          </linearGradient>
          <filter id="flow-glow">
            <feGaussianBlur stdDeviation="2" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <path
          v-for="wire in wires"
          :key="wire.id"
          :d="wire.d"
          :stroke="wire.enabled ? 'url(#flow-wire-gradient)' : 'rgba(100, 116, 139, 0.45)'"
          :stroke-width="wire.enabled ? 2 : 1.5"
          fill="none"
          :filter="wire.enabled ? 'url(#flow-glow)' : undefined"
          class="flow-wire"
          :class="{ disabled: !wire.enabled }"
        />
        <path
          v-if="connectionPreviewPath"
          :d="connectionPreviewPath"
          :stroke="connectionDrag?.valid ? '#22c55e' : '#f59e0b'"
          stroke-width="2.5"
          fill="none"
          class="flow-wire-preview"
          :class="{ invalid: connectionDrag && !connectionDrag.valid }"
        />
      </svg>

      <button
        v-for="wire in wires"
        :key="`${wire.id}-toggle`"
        type="button"
        class="wire-toggle"
        :class="{ disabled: !wire.enabled, required: wire.required }"
        :style="{ left: `${wire.labelX}px`, top: `${wire.labelY}px` }"
        :title="wire.enabled ? `断开 ${wire.id}` : `连接 ${wire.id}`"
        @pointerdown.stop
        @click.stop="toggleWire(wire.id)"
      >
        <span>{{ wire.label }}</span>
        <code>{{ wire.enabled ? "ON" : "OFF" }}</code>
      </button>

      <div v-if="connectionDrag" class="connection-hint">
        <strong>{{ connectionDrag.from }}</strong>
        <span v-if="connectionDrag.target && connectionDrag.valid">
          松开即可连接到 {{ connectionDrag.target }}
        </span>
        <span v-else-if="connectionDrag.target">
          {{ connectionDrag.from }} 不能直接连接 {{ connectionDrag.target }}
        </span>
        <span v-else>拖到另一个节点左侧输入端口</span>
      </div>

      <div
        v-for="node in visibleFlowNodes"
        :key="node.id"
        role="button"
        tabindex="0"
        class="flow-node"
        :class="{ selected: selectedNodeId === node.id }"
        :style="{
          left: `${boundedLayout[node.id].x}px`,
          top: `${boundedLayout[node.id].y}px`,
          borderColor: node.color,
          '--node-color': node.color,
        }"
        @pointerdown="handlePointerDown($event, node.id)"
      >
        <button
          v-if="canDeleteNode(node.id)"
          type="button"
          class="node-delete"
          title="删除节点"
          @pointerdown.stop
          @click.stop="emit('delete-node', node.id)"
        >
          ×
        </button>
        <span
          v-if="hasIncomingWire(node.id)"
          class="node-port port-in"
          :class="{
            candidate: connectionDrag?.target === node.id,
            valid: connectionDrag?.target === node.id && connectionDrag?.valid,
            invalid: connectionDrag?.target === node.id && !connectionDrag?.valid,
          }"
        ></span>
        <span
          v-if="hasOutgoingWire(node.id)"
          class="node-port port-out"
          title="拖拽到另一个节点输入端口以连接线缆"
          @pointerdown.stop.prevent="handleConnectionPointerDown($event, node.id)"
        ></span>
        <span class="node-badge">{{ node.badge }}</span>
        <strong>{{ node.title }}</strong>
        <code>{{ node.subtitle }}</code>
      </div>

      </div>
    </div>
    <div class="canvas-resize-footer">
      <span>画布超过可视区后可横向/纵向滚动，继续拖拽右侧手柄可拉得更开。</span>
      <button
        type="button"
        class="canvas-resize-handle"
        title="拖拽拉开画布"
        @pointerdown.stop.prevent="handleCanvasResizePointerDown"
      >
        <span>拖拽拉开</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.flow-shell {
  background: rgba(15, 23, 42, 0.78);
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 10px;
  padding: 14px;
  margin-bottom: 20px;
  box-shadow: inset 0 0 20px rgba(0, 0, 0, 0.3);
}

.flow-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.flow-header h4 {
  margin: 0;
  color: #f8fafc;
  font-size: 14px;
}

.flow-header span {
  color: #94a3b8;
  font-size: 12px;
}

.flow-inspector {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 12px;
  margin-bottom: 12px;
  border-radius: 8px;
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(2, 6, 23, 0.55);
}

.inspector-tag {
  margin: 0;
  min-width: 72px;
  text-align: center;
}

.flow-inspector strong {
  display: block;
  color: #f8fafc;
  font-size: 12px;
  line-height: 1.2;
}

.flow-inspector span {
  display: block;
  margin-top: 2px;
  color: #94a3b8;
  font-size: 11px;
  line-height: 1.4;
}

.flow-canvas-viewport {
  position: relative;
  max-width: 100%;
  overflow: auto;
  border-radius: 10px;
  border: 1px solid rgba(59, 130, 246, 0.2);
  background: rgba(2, 6, 23, 0.56);
  scrollbar-color: rgba(56, 189, 248, 0.5) rgba(15, 23, 42, 0.9);
}

.flow-canvas {
  position: relative;
  overflow: hidden;
  background-color: #07111f;
  background-image:
    linear-gradient(to right, rgba(56, 189, 248, 0.08) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(56, 189, 248, 0.08) 1px, transparent 1px),
    radial-gradient(circle at 20% 20%, rgba(24, 144, 255, 0.16), transparent 26%),
    radial-gradient(circle at 80% 70%, rgba(114, 46, 209, 0.16), transparent 30%);
  background-size: 24px 24px, 24px 24px, 100% 100%, 100% 100%;
  transition: width 0.12s ease, height 0.12s ease;
}

.flow-canvas.resizing {
  cursor: nwse-resize;
  transition: none;
}

.drop-target-hint {
  position: absolute;
  right: 12px;
  top: 12px;
  z-index: 5;
  max-width: min(360px, calc(100% - 24px));
  padding: 6px 10px;
  border: 1px dashed rgba(56, 189, 248, 0.45);
  border-radius: 999px;
  color: #bae6fd;
  background: rgba(15, 23, 42, 0.78);
  box-shadow: 0 8px 22px rgba(0, 0, 0, 0.28);
  font-size: 10.5px;
  pointer-events: none;
}

.flow-wires {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.flow-wire {
  stroke-dasharray: 8 8;
  animation: dash-flow 1.4s linear infinite;
  opacity: 0.78;
}

.flow-wire.disabled {
  stroke-dasharray: 4 8;
  animation: none;
  opacity: 0.42;
}

.flow-wire-preview {
  stroke-dasharray: 10 5;
  filter: drop-shadow(0 0 8px rgba(34, 197, 94, 0.65));
  pointer-events: none;
}

.flow-wire-preview.invalid {
  filter: drop-shadow(0 0 8px rgba(245, 158, 11, 0.65));
}

@keyframes dash-flow {
  to {
    stroke-dashoffset: -32;
  }
}

.flow-node {
  position: absolute;
  width: 160px;
  height: 72px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 4px;
  padding: 10px 12px 10px 14px;
  border: 1px solid var(--node-color);
  border-left-width: 4px;
  border-radius: 10px;
  color: #f8fafc;
  background: rgba(15, 23, 42, 0.92);
  box-shadow: 0 10px 26px rgba(0, 0, 0, 0.45);
  cursor: grab;
  user-select: none;
  touch-action: none;
  transition: box-shadow 0.18s ease, transform 0.18s ease;
}

.wire-toggle {
  position: absolute;
  z-index: 4;
  min-width: 104px;
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 4px 8px;
  border: 1px solid rgba(56, 189, 248, 0.45);
  border-radius: 999px;
  color: #dbeafe;
  background: rgba(15, 23, 42, 0.86);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.28);
  cursor: pointer;
  font-size: 10px;
  line-height: 1;
  transition: border-color 0.16s ease, opacity 0.16s ease, transform 0.16s ease;
}

.wire-toggle:hover {
  transform: translateY(-1px);
  border-color: #38bdf8;
}

.wire-toggle.required::before {
  content: "";
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 8px rgba(34, 197, 94, 0.7);
}

.wire-toggle.disabled {
  border-color: rgba(148, 163, 184, 0.25);
  color: #94a3b8;
  opacity: 0.72;
}

.wire-toggle.disabled::before {
  background: #64748b;
  box-shadow: none;
}

.wire-toggle code {
  color: inherit;
  font-size: 9px;
}

.connection-hint {
  position: absolute;
  left: 12px;
  bottom: 12px;
  z-index: 8;
  display: inline-flex;
  gap: 8px;
  align-items: center;
  max-width: calc(100% - 24px);
  padding: 7px 10px;
  border-radius: 999px;
  border: 1px solid rgba(56, 189, 248, 0.35);
  color: #dbeafe;
  background: rgba(15, 23, 42, 0.9);
  box-shadow: 0 8px 22px rgba(0, 0, 0, 0.34);
  font-size: 11px;
}

.connection-hint strong {
  color: #38bdf8;
}

.connection-hint span {
  color: #cbd5e1;
}

.canvas-resize-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 8px;
  color: #94a3b8;
  font-size: 11px;
}

.canvas-resize-handle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  border: 1px solid rgba(56, 189, 248, 0.5);
  border-radius: 999px 999px 4px 999px;
  color: #dbeafe;
  background: rgba(15, 23, 42, 0.9);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.34);
  cursor: nwse-resize;
  font-size: 10px;
  line-height: 1;
  user-select: none;
  touch-action: none;
}

.canvas-resize-handle::after {
  content: "⌟";
  color: #38bdf8;
  font-size: 14px;
  line-height: 0.8;
}

.canvas-resize-handle:hover {
  border-color: #38bdf8;
  color: #f8fafc;
}

.node-delete {
  position: absolute;
  top: 6px;
  right: 7px;
  z-index: 3;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: 1px solid rgba(248, 113, 113, 0.45);
  border-radius: 999px;
  color: #fecaca;
  background: rgba(127, 29, 29, 0.62);
  cursor: pointer;
  font-size: 13px;
  line-height: 1;
  opacity: 0;
  transition: opacity 0.16s ease, transform 0.16s ease, border-color 0.16s ease;
}

.flow-node:hover .node-delete,
.flow-node.selected .node-delete {
  opacity: 1;
}

.node-delete:hover {
  transform: scale(1.05);
  border-color: #fca5a5;
  color: #fff;
}

.flow-node:hover,
.flow-node.selected {
  transform: translateY(-1px);
  box-shadow: 0 0 0 1px var(--node-color), 0 12px 32px rgba(0, 0, 0, 0.55);
}

.flow-node:active {
  cursor: grabbing;
}

.node-badge {
  color: var(--node-color);
  font-size: 10px;
  font-family: "Consolas", "Courier New", monospace;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.flow-node strong {
  font-size: 12px;
  line-height: 1.15;
}

.flow-node code {
  max-width: 132px;
  color: #c4b5fd;
  font-size: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.node-port {
  position: absolute;
  top: 50%;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--node-color);
  box-shadow: 0 0 10px var(--node-color);
}

.node-port.candidate {
  width: 14px;
  height: 14px;
  box-shadow: 0 0 0 4px rgba(250, 204, 21, 0.2), 0 0 14px #facc15;
}

.node-port.valid {
  background: #22c55e;
  box-shadow: 0 0 0 4px rgba(34, 197, 94, 0.2), 0 0 14px #22c55e;
}

.node-port.invalid {
  background: #f59e0b;
  box-shadow: 0 0 0 4px rgba(245, 158, 11, 0.2), 0 0 14px #f59e0b;
}

.port-in {
  left: -6px;
  transform: translateY(-50%);
}

.port-out {
  right: -6px;
  transform: translateY(-50%);
}
</style>
