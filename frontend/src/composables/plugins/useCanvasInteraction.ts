import { onBeforeUnmount, ref, useTemplateRef } from "vue";
import type {
  VisualFlowNodeId,
  VisualWireId,
} from "../../components/plugins/types";
import {
  NODE_WIDTH,
  NODE_HEIGHT,
  DEFAULT_CANVAS_WIDTH,
  DEFAULT_CANVAS_HEIGHT,
  MIN_CANVAS_WIDTH,
  MIN_CANVAS_HEIGHT,
  MAX_CANVAS_WIDTH,
  MAX_CANVAS_HEIGHT,
  clamp,
} from "./useCanvasLayout";

export interface UseCanvasInteractionEmit {
  (e: "update:selectedNodeId", value: VisualFlowNodeId): void;
  (e: "update:wireStates", value: Record<VisualWireId, boolean>): void;
  (
    e: "drop-node-type",
    value: { category: string; value: string; x: number; y: number },
  ): void;
}

export interface UseCanvasInteractionOptions {
  canvasSize: { width: number; height: number };
  mergedWireStates: Record<VisualWireId, boolean>;
  mergedLayout: Record<string, { x: number; y: number }>;
  visibleFlowNodes: Array<{ id: VisualFlowNodeId }>;
  visibleWireDefinitions: Array<{
    id: VisualWireId;
    from: VisualFlowNodeId;
    to: VisualFlowNodeId;
  }>;
  getNodePortPoint: (
    id: VisualFlowNodeId,
    side: "in" | "out",
  ) => { x: number; y: number };
  getWireForEndpoints: (
    from: VisualFlowNodeId,
    to: VisualFlowNodeId,
  ) => { id: VisualWireId; to: VisualFlowNodeId } | null;
  hasOutgoingWire: (id: VisualFlowNodeId) => boolean;
  hasIncomingWire: (id: VisualFlowNodeId) => boolean;
  snapNodeX: (v: number) => number;
  snapNodeY: (v: number) => number;
  emit: UseCanvasInteractionEmit;
}

export function useCanvasInteraction(opts: () => UseCanvasInteractionOptions) {
  const viewportRef = useTemplateRef<HTMLElement>("viewport");
  const canvasRef = useTemplateRef<HTMLElement>("canvas");
  const viewportSize = ref({ width: DEFAULT_CANVAS_WIDTH });

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

  let viewportResizeObserver: ResizeObserver | null = null;

  const workspaceSize = ref({
    width: DEFAULT_CANVAS_WIDTH,
    height: DEFAULT_CANVAS_HEIGHT,
  });

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
    const o = opts();
    if (!canvas) return null;
    const rect = canvas.getBoundingClientRect();
    const scaleX = rect.width > 0 ? o.canvasSize.width / rect.width : 1;
    const scaleY = rect.height > 0 ? o.canvasSize.height / rect.height : 1;
    return {
      x: clamp((event.clientX - rect.left) * scaleX, 0, o.canvasSize.width),
      y: clamp((event.clientY - rect.top) * scaleY, 0, o.canvasSize.height),
    };
  };

  const getNearestInputNode = (
    point: { x: number; y: number },
    from: VisualFlowNodeId,
  ): VisualFlowNodeId | null => {
    const o = opts();
    let nearestId: VisualFlowNodeId | null = null;
    let nearestDistance = Infinity;
    o.visibleFlowNodes.forEach((node) => {
      if (node.id === from || !o.hasIncomingWire(node.id)) return;
      const input = o.getNodePortPoint(node.id, "in");
      const distance = Math.hypot(point.x - input.x, point.y - input.y);
      if (distance <= 34 && distance < nearestDistance) {
        nearestId = node.id;
        nearestDistance = distance;
      }
    });
    return nearestId;
  };

  // Drag handlers
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
    const o = opts();
    const next = {
      ...o.mergedLayout,
      [dragging.value.id]: {
        x: o.snapNodeX(event.clientX - rect.left - dragging.value.offsetX),
        y: o.snapNodeY(event.clientY - rect.top - dragging.value.offsetY),
      },
    };
    return next;
  };

  const stopDragging = () => {
    const o = opts();
    if (dragging.value && !dragging.value.moved) {
      o.emit("update:selectedNodeId", dragging.value.id);
    }
    dragging.value = null;
    window.removeEventListener("pointermove", handlePointerMove);
    window.removeEventListener("pointerup", stopDragging);
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

  // Connection drag handlers
  const handleConnectionPointerMove = (event: PointerEvent) => {
    if (!connectionDrag.value) return;
    const point = canvasPointFromEvent(event);
    if (!point) return;
    const o = opts();
    const target = getNearestInputNode(point, connectionDrag.value.from);
    connectionDrag.value = {
      ...connectionDrag.value,
      currentX: point.x,
      currentY: point.y,
      target,
      valid:
        !!target && !!o.getWireForEndpoints(connectionDrag.value.from, target),
    };
  };

  const stopConnectionDragging = () => {
    const o = opts();
    if (connectionDrag.value?.target && connectionDrag.value.valid) {
      const wire = o.getWireForEndpoints(
        connectionDrag.value.from,
        connectionDrag.value.target,
      );
      if (wire) {
        if (!o.mergedWireStates[wire.id]) {
          o.emit("update:wireStates", {
            ...o.mergedWireStates,
            [wire.id]: true,
          });
        }
        o.emit("update:selectedNodeId", wire.to);
      }
    }
    connectionDrag.value = null;
    window.removeEventListener("pointermove", handleConnectionPointerMove);
    window.removeEventListener("pointerup", stopConnectionDragging);
  };

  const handleConnectionPointerDown = (
    event: PointerEvent,
    id: VisualFlowNodeId,
  ) => {
    const o = opts();
    if (!o.hasOutgoingWire(id)) return;
    const point = canvasPointFromEvent(event);
    const start = o.getNodePortPoint(id, "out");
    connectionDrag.value = {
      from: id,
      startX: start.x,
      startY: start.y,
      currentX: point?.x ?? start.x,
      currentY: point?.y ?? start.y,
      target: null,
      valid: false,
    };
    o.emit("update:selectedNodeId", id);
    window.addEventListener("pointermove", handleConnectionPointerMove);
    window.addEventListener("pointerup", stopConnectionDragging);
  };

  // Canvas resize
  const applyWorkspaceSize = (width: number, height: number) => {
    workspaceSize.value = {
      width: clamp(Math.round(width), MIN_CANVAS_WIDTH, MAX_CANVAS_WIDTH),
      height: clamp(Math.round(height), MIN_CANVAS_HEIGHT, MAX_CANVAS_HEIGHT),
    };
  };

  const expandWorkspace = () => {
    const o = opts();
    applyWorkspaceSize(o.canvasSize.width + 260, o.canvasSize.height + 160);
  };

  const resetWorkspaceSize = () => {
    applyWorkspaceSize(DEFAULT_CANVAS_WIDTH, DEFAULT_CANVAS_HEIGHT);
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
      canvasResizeDrag.value.startHeight + deltaY,
    );
  };

  const handleCanvasResizePointerDown = (event: PointerEvent) => {
    const o = opts();
    canvasResizeDrag.value = {
      startX: event.clientX,
      startY: event.clientY,
      startWidth: o.canvasSize.width,
      startHeight: o.canvasSize.height,
    };
    window.addEventListener("pointermove", handleCanvasResizeMove);
    window.addEventListener("pointerup", stopCanvasResizing);
  };

  // Drag & drop for node types
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
    const o = opts();
    try {
      const parsed = JSON.parse(rawData) as {
        category?: unknown;
        value?: unknown;
      };
      if (
        typeof parsed.category !== "string" ||
        typeof parsed.value !== "string"
      ) {
        return;
      }
      o.emit("drop-node-type", {
        category: parsed.category,
        value: parsed.value,
        x: o.snapNodeX(point.x - NODE_WIDTH / 2),
        y: o.snapNodeY(point.y - NODE_HEIGHT / 2),
      });
    } catch (err) {
      console.error("Failed to parse node type drop payload:", err);
    }
  };

  const updateViewportSize = () => {
    const viewport = viewportRef.value;
    if (!viewport) return;
    const rect = viewport.getBoundingClientRect();
    viewportSize.value = {
      width: Math.max(MIN_CANVAS_WIDTH, Math.round(rect.width)),
    };
  };

  const initResizeObserver = () => {
    updateViewportSize();
    if (typeof ResizeObserver !== "undefined" && viewportRef.value) {
      viewportResizeObserver = new ResizeObserver(updateViewportSize);
      viewportResizeObserver.observe(viewportRef.value);
    }
  };

  const connectionPreviewPath = () => {
    if (!connectionDrag.value) return "";
    const { startX, startY, currentX, currentY } = connectionDrag.value;
    const mid = Math.max(28, Math.abs(currentX - startX) / 2);
    return `M ${startX} ${startY} C ${startX + mid} ${startY}, ${
      currentX - mid
    } ${currentY}, ${currentX} ${currentY}`;
  };

  const cleanupInteraction = () => {
    stopDragging();
    stopCanvasResizing();
    connectionDrag.value = null;
    viewportResizeObserver?.disconnect();
    viewportResizeObserver = null;
    window.removeEventListener("pointermove", handleConnectionPointerMove);
    window.removeEventListener("pointerup", stopConnectionDragging);
  };

  onBeforeUnmount(cleanupInteraction);

  return {
    viewportRef,
    canvasRef,
    viewportSize,
    workspaceSize,
    dragging,
    connectionDrag,
    canvasResizeDrag,
    canvasPointFromEvent,
    canvasPointFromDragEvent,
    handlePointerDown,
    handlePointerMove,
    stopDragging,
    handleConnectionPointerDown,
    handleConnectionPointerMove,
    stopConnectionDragging,
    handleCanvasDragOver,
    handleCanvasDrop,
    handleCanvasResizePointerDown,
    connectionPreviewPath,
    applyWorkspaceSize,
    expandWorkspace,
    resetWorkspaceSize,
    updateViewportSize,
    initResizeObserver,
    cleanupInteraction,
  };
}
