import type { ExecutionGraphNode } from "../../types/executionGraph";

/**
 * Pure helper functions for the execution graph canvas.
 * Extracted from ExecutionGraphCanvas.vue for maintainability.
 */

export const kindColor = (kind: string) => {
  const colorMap: Record<string, string> = {
    agent_run: "#7c3aed",
    tool_call: "#2563eb",
    process: "#10b981",
    syscall: "#f59e0b",
    wrapper_event: "#0891b2",
    hook_event: "#0f766e",
    file: "#64748b",
    network: "#ef4444",
    policy_decision: "#111827",
    policy_alert: "#dc2626",
    exit_status: "#6b7280",
  };
  return colorMap[kind] ?? "#94a3b8";
};

export const nodeRadius = (node: ExecutionGraphNode) => {
  const eventCount = Number(node.metadata?.eventCount ?? 1);
  const aggregateBoost =
    Number.isFinite(eventCount) && eventCount > 1
      ? Math.min(8, Math.log2(eventCount) * 2)
      : 0;
  switch (node.kind) {
    case "agent_run":
      return 18 + aggregateBoost;
    case "tool_call":
      return 16 + aggregateBoost;
    case "process":
      return 14 + aggregateBoost;
    case "policy_alert":
    case "policy_decision":
      return 12 + aggregateBoost;
    default:
      return 10 + aggregateBoost;
  }
};

export const truncate = (value: string, max = 28) => {
  if (value.length <= max) return value;
  return `${value.slice(0, max - 1)}…`;
};

export const linkStrokeWidth = (kind: string) =>
  kind === "alerted" || kind === "blocked" ? 2.4 : 1.4;

export const linkStrokeColor = (kind: string) => {
  if (kind === "alerted" || kind === "blocked") return "#dc2626";
  if (kind === "rewritten") return "#7c3aed";
  if (kind === "child_process" || kind === "parent_process") return "#059669";
  if (kind === "exec_chain") return "#2563eb";
  return "#cbd5e1";
};

export const processTreeEdgeKinds = new Set([
  "child_process",
  "parent_process",
  "exec_chain",
  "spawned",
]);
export const activityEdgeKinds = new Set([
  "observed",
  "execed",
  "waited",
  "exited",
  "reviewed",
  "alerted",
]);

export const linkDistance = (kind: string) => {
  switch (kind) {
    case "contains":
    case "owns":
      return 95;
    case "child_process":
    case "parent_process":
    case "exec_chain":
      return 80;
    case "spawned":
    case "waited":
      return 88;
    case "connected":
    case "opened":
    case "read":
    case "wrote":
    case "deleted":
      return 110;
    default:
      return 120;
  }
};

export const linkStrength = (kind: string) =>
  processTreeEdgeKinds.has(kind) ? 0.85 : kind === "contains" ? 0.55 : 0.35;

export const processSortValue = (pid: number | undefined) => {
  if (pid !== undefined && Number.isFinite(pid)) return pid;
  return Number.MAX_SAFE_INTEGER;
};

export const processDisplayLabel = (node: ExecutionGraphNode | undefined) => {
  if (!node) return "";
  const pid = String(node.metadata?.pid ?? node.pid ?? "").trim();
  const label = /^pid \d+$/.test(node.label.trim()) ? "process" : node.label;
  return pid && !label.endsWith(`(${pid})`) ? `${label} (${pid})` : label;
};
