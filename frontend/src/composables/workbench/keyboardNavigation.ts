export type DockAxis = "horizontal" | "vertical";

export function dockTabNavigationIndex(key: string, current: number, length: number): number | null {
  if (!Number.isInteger(current) || current < 0 || current >= length || length <= 0) return null;
  if (key === "Home") return 0;
  if (key === "End") return length - 1;
  if (key === "ArrowRight") return (current + 1) % length;
  if (key === "ArrowLeft") return (current + length - 1) % length;
  return null;
}

export function dockSplitterRatio(axis: DockAxis, key: string, ratio: number): number | null {
  if (!Number.isFinite(ratio)) return null;
  const step = axis === "horizontal"
    ? (key === "ArrowLeft" ? -0.05 : key === "ArrowRight" ? 0.05 : null)
    : (key === "ArrowUp" ? -0.05 : key === "ArrowDown" ? 0.05 : null);
  if (step === null) return null;
  return Math.max(0.15, Math.min(0.85, Number((ratio + step).toFixed(4))));
}
 
export interface DockRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export function dockPointerRatio(
  axis: DockAxis,
  clientX: number,
  clientY: number,
  rect: DockRect,
): number | null {
  const length = axis === "horizontal" ? rect.width : rect.height;
  const coordinate = axis === "horizontal" ? clientX - rect.left : clientY - rect.top;
  if (!Number.isFinite(length) || length <= 0 || !Number.isFinite(coordinate)) return null;
  return Math.max(0.15, Math.min(0.85, Number((coordinate / length).toFixed(4))));
}
