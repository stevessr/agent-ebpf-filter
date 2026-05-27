import { ref, computed, onBeforeUnmount } from "vue";

export type FloatingDock = "left" | "right";

export interface FloatingPanelOptions {
  initialDock: FloatingDock;
  defaultWidth: number;
  defaultHeight: number;
  edgeMargin?: number;
  snapDelayMs?: number;
}

export function useFloatingPanel(options: FloatingPanelOptions) {
  const {
    initialDock,
    defaultWidth,
    defaultHeight,
    edgeMargin = 18,
    snapDelayMs = 180,
  } = options;

  const visible = ref(false);
  const dock = ref<FloatingDock>(initialDock);
  const position = ref({ x: getInitialX(initialDock), y: 92 });
  const dragging = ref<{
    startX: number;
    startY: number;
    originX: number;
    originY: number;
  } | null>(null);

  let hideTimer: number | null = null;

  // --- Internal helpers ---

  function getInitialX(d: FloatingDock) {
    const vw =
      typeof window === "undefined" ? 1280 : Math.max(480, window.innerWidth);
    return d === "left"
      ? edgeMargin
      : Math.max(edgeMargin, vw - defaultWidth - edgeMargin);
  }

  function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value));
  }

  function getPanelSize(): { width: number; height: number } {
    return { width: defaultWidth, height: defaultHeight };
  }

  function getDockX(d: FloatingDock, width: number) {
    return d === "left"
      ? edgeMargin
      : Math.max(edgeMargin, window.innerWidth - width - edgeMargin);
  }

  function isAtDock(d: FloatingDock, width: number) {
    return Math.abs(position.value.x - getDockX(d, width)) <= 2;
  }

  function clearHideTimer() {
    if (hideTimer === null) return;
    window.clearTimeout(hideTimer);
    hideTimer = null;
  }

  function snapTo(d: FloatingDock) {
    const { width, height } = getPanelSize();
    dock.value = d;
    position.value = {
      x: getDockX(d, width),
      y: clamp(
        position.value.y,
        edgeMargin,
        Math.max(
          edgeMargin,
          window.innerHeight -
            Math.min(height, window.innerHeight - 128) -
            edgeMargin
        )
      ),
    };
  }

  // --- Public API ---

  const dockLabel = computed(() =>
    dock.value === "left" ? "吸附右侧" : "吸附左侧"
  );

  const hideArrow = computed(() => (dock.value === "left" ? "‹" : "›"));

  const restoreArrow = computed(() =>
    dock.value === "left" ? "›" : "‹"
  );

  const panelStyle = computed(() => ({
    left: `${position.value.x}px`,
    top: `${position.value.y}px`,
  }));

  const triggerStyle = computed(() => ({
    top: `${Math.max(
      88,
      Math.min(position.value.y + 14, window.innerHeight - 120)
    )}px`,
  }));

  const toggleDock = () => {
    snapTo(dock.value === "left" ? "right" : "left");
  };

  const hide = () => {
    const { width } = getPanelSize();
    const nearestDock =
      position.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
    const wasAlreadyAtDock = isAtDock(nearestDock, width);
    clearHideTimer();
    snapTo(nearestDock);
    hideTimer = window.setTimeout(
      () => {
        visible.value = false;
        hideTimer = null;
      },
      wasAlreadyAtDock ? 0 : snapDelayMs
    );
  };

  const show = () => {
    clearHideTimer();
    visible.value = true;
  };

  const handlePointerMove = (event: PointerEvent) => {
    if (!dragging.value) return;
    const margin = 12;
    const { width, height } = getPanelSize();
    position.value = {
      x: clamp(
        dragging.value.originX + event.clientX - dragging.value.startX,
        margin,
        Math.max(margin, window.innerWidth - width - margin)
      ),
      y: clamp(
        dragging.value.originY + event.clientY - dragging.value.startY,
        margin,
        Math.max(
          margin,
          window.innerHeight - Math.min(height, window.innerHeight - 128) - margin
        )
      ),
    };
  };

  const stopDragging = () => {
    if (dragging.value) {
      const snapThreshold = 96;
      const { width } = getPanelSize();
      if (position.value.x <= snapThreshold) {
        snapTo("left");
      } else if (position.value.x + width >= window.innerWidth - snapThreshold) {
        snapTo("right");
      } else {
        dock.value =
          position.value.x + width / 2 < window.innerWidth / 2 ? "left" : "right";
      }
    }
    dragging.value = null;
    window.removeEventListener("pointermove", handlePointerMove);
    window.removeEventListener("pointerup", stopDragging);
  };

  const startDragging = (event: PointerEvent) => {
    clearHideTimer();
    dragging.value = {
      startX: event.clientX,
      startY: event.clientY,
      originX: position.value.x,
      originY: position.value.y,
    };
    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", stopDragging);
  };

  const cleanup = () => {
    stopDragging();
    clearHideTimer();
  };

  onBeforeUnmount(cleanup);

  return {
    visible,
    dock,
    position,
    dragging,
    dockLabel,
    hideArrow,
    restoreArrow,
    panelStyle,
    triggerStyle,
    toggleDock,
    hide,
    show,
    startDragging,
    cleanup,
  };
}
