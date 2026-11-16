import type { ExecutionGraphEdge, ExecutionGraphNode } from "../../types/executionGraph";

export interface Point3 {
  x: number;
  y: number;
  z: number;
}

const KIND_LANES: Record<string, number> = {
  agent_run: -5,
  tool_call: -4,
  process: -3,
  wrapper_event: -2,
  hook_event: -1,
  syscall: 0,
  exit_status: 1,
  file: 2,
  network: 3,
  policy_decision: 4,
  policy_alert: 5,
};

export function stableGraphHash(value: string): number {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i += 1) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

/**
 * Pure, bounded layout: coordinates depend on identities rather than array
 * indices, so socket updates and replay ordering cannot reshuffle old nodes.
 * Trace/run metadata provides Z separation without assuming that shared
 * file/network resources belong to only one agent.
 */
export function harnessPosition(node: ExecutionGraphNode): Point3 {
  const h = stableGraphHash(node.id);
  const lane = KIND_LANES[node.kind] ?? 0;
  const run = node.metadata?.agentRunId || node.metadata?.agent_run_id ||
    node.metadata?.traceId || node.metadata?.trace_id || "";
  const runOffset = run ? ((stableGraphHash(run) % 9) - 4) * 36 : 0;
  return {
    x: lane * 122,
    y: ((h & 0xffff) / 65535 - 0.5) * 500,
    z: (((h >>> 16) & 0xffff) / 65535 - 0.5) * 240 + runOffset,
  };
}

/** Limit HTML labels to protect responsiveness on 1000+ node captures. */
export function harnessLabelNodes(
  nodes: ExecutionGraphNode[],
  selectedId: string,
  limit = 42,
): ExecutionGraphNode[] {
  const priority = (node: ExecutionGraphNode): number => {
    if (node.id === selectedId || node.metadata?.sourceNodeId === selectedId) return 0;
    if (node.kind === "policy_alert" || (node.riskScore ?? 0) >= 60) return 1;
    if (node.kind === "agent_run") return 2;
    if (node.kind === "tool_call") return 3;
    if (node.kind === "process") return 4;
    if (node.kind === "policy_decision") return 5;
    return 6;
  };
  return nodes
    .filter((node) => priority(node) < 6)
    .sort((a, b) => priority(a) - priority(b) || a.id.localeCompare(b.id))
    .slice(0, Math.max(0, Math.min(limit, 80)));
}

/**
 * A stable content signature for GPU state. Both arrays are order-independent
 * because reordering a WebSocket snapshot must not reallocate WebGL objects.
 * Include every field which affects 3D color, label, position or selection
 * remapping, while leaving irrelevant raw payload fields out of the key.
 */
export function harnessTopologySignature(
  nodes: ExecutionGraphNode[],
  edges: ExecutionGraphEdge[],
): string {
  const nodeKeys = nodes.map((node) => {
    const m = node.metadata ?? {};
    return JSON.stringify([
      node.id, node.kind, node.label, node.subtitle ?? "",
      node.riskScore ?? 0, m.riskScore ?? m.risk_score ?? "",
      m.sourceNodeId ?? "", m.eventCount ?? "",
      m.agentRunId ?? m.agent_run_id ?? "",
      m.traceId ?? m.trace_id ?? "",
      m.decision ?? m.verdict ?? "",
    ]);
  }).sort();
  const edgeKeys = edges.map((edge) =>
    JSON.stringify([edge.id, edge.source, edge.target, edge.kind]),
  ).sort();
  return JSON.stringify([nodeKeys, edgeKeys]);
}
