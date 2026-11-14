import type { ExecutionGraphEdge, ExecutionGraphNode } from "../../types/executionGraph";

/**
 * Presentation-only noise reduction. The full snapshot is retained for
 * recording, replay, inspection and backend enforcement.
 */
export type GraphNoiseMode = "off" | "balanced" | "strict";

export interface GraphNoiseResult {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  hiddenNodes: number;
  hiddenEdges: number;
}

const ROUTINE_SYSCALLS = new Set([
  "getpid", "getppid", "gettid", "clock_gettime", "gettimeofday",
  "sched_yield", "futex", "nanosleep", "rt_sigprocmask",
]);
const EXTRA_ROUTINE_SYSCALLS = new Set([
  "epoll_wait", "epoll_pwait", "poll", "ppoll", "select",
  "pselect6", "getrandom", "brk",
]);
const READ_ONLY_SYSCALLS = new Set([
  "read", "pread64", "readv", "open", "openat", "openat2",
  "stat", "lstat", "fstat", "newfstatat", "statx",
  "access", "faccessat", "faccessat2", "readlink", "readlinkat",
  "getdents", "getdents64",
]);
const ROUTINE_PATHS = [
  "/proc", "/sys", "/dev/null", "/dev/zero", "/dev/random",
  "/dev/urandom", "/etc/ld.so.cache", "/etc/localtime",
  "/usr/share/zoneinfo", "/usr/share/locale", "/usr/lib/locale",
];
const SENSITIVE_PATH = /(?:^|\/)(?:\.ssh|\.aws|\.gnupg|\.kube|\.config|environ|shadow|passwd|credentials|secrets?)(?:\/|$)/i;
const RISK_BYPASS = 60;

function pathFromNode(node: ExecutionGraphNode): string {
  const metadata = node.metadata ?? {};
  const candidate =
    metadata.path || metadata.filePath || metadata.file_path ||
    metadata.targetPath || metadata.target_path || (node.kind === "file" ? node.label : "");
  return candidate.trim().split(/[?#]/, 1)[0] ?? "";
}

function routinePath(path: string): boolean {
  if (!path.startsWith("/") || SENSITIVE_PATH.test(path)) return false;
  const normalized = path.replace(/\/+$/, "");
  return ROUTINE_PATHS.some((prefix) =>
    normalized === prefix || normalized.startsWith(prefix + "/"),
  );
}

function syscallName(node: ExecutionGraphNode): string {
  const raw = (
    node.metadata?.type || node.metadata?.syscall ||
    node.metadata?.eventType || node.label.split("·").pop() || ""
  ).trim().toLowerCase();
  return /^[a-z0-9_]+/.exec(raw)?.[0] ?? "";
}

function isProtected(node: ExecutionGraphNode): boolean {
  if (node.kind === "policy_alert" || node.kind === "policy_decision") return true;
  if ((node.riskScore ?? 0) >= RISK_BYPASS) return true;
  const metadata = node.metadata ?? {};
  const decision = [
    metadata.decision, metadata.action, metadata.policyDecision,
    metadata.policy_decision, metadata.verdict,
  ].filter(Boolean).join(" ").toUpperCase();
  return /BLOCK|DENY|ALERT|REWRITE|REJECT|QUARANTINE/.test(decision);
}

function isNoise(node: ExecutionGraphNode, mode: GraphNoiseMode): boolean {
  if (isProtected(node)) return false;
  const path = pathFromNode(node);
  if (node.kind === "syscall") {
    const name = syscallName(node);
    if (ROUTINE_SYSCALLS.has(name)) return true;
    if (mode === "strict" && EXTRA_ROUTINE_SYSCALLS.has(name)) return true;
    return READ_ONLY_SYSCALLS.has(name) && routinePath(path);
  }
  if (node.kind === "file" && mode === "strict" && routinePath(path)) {
    const operation = (
      node.metadata?.operation || node.metadata?.type || ""
    ).toLowerCase();
    // A write, create, delete or unknown operation remains evidence.
    return /^(read|open|stat|access|readlink|metadata)$/.test(operation);
  }
  if (mode === "strict" && node.kind === "exit_status") {
    const status = node.metadata?.exitCode ?? node.metadata?.exit_code ??
      node.metadata?.status;
    return status === "0";
  }
  if (mode === "strict" && (node.kind === "hook_event" || node.kind === "wrapper_event")) {
    return /^(heartbeat|healthcheck|ping)$/i.test(node.metadata?.type ?? "");
  }
  return false;
}

export function filterPresentationNoise(
  nodes: ExecutionGraphNode[],
  edges: ExecutionGraphEdge[],
  mode: GraphNoiseMode,
  selectedNodeId = "",
): GraphNoiseResult {
  if (mode === "off") {
    return { nodes, edges, hiddenNodes: 0, hiddenEdges: 0 };
  }

  const byId = new Map(nodes.map((node) => [node.id, node]));
  const preserved = new Set<string>();
  for (const node of nodes) {
    if (isProtected(node) || node.id === selectedNodeId) {
      preserved.add(node.id);
    }
  }
  // Preserve direct evidence attached to alerts / enforcement, not all
  // sibling syscalls of a protected process.
  for (const edge of edges) {
    if (!/^(alerted|blocked|rewritten|reviewed)$/.test(edge.kind)) continue;
    if (preserved.has(edge.source) && byId.has(edge.target)) preserved.add(edge.target);
    if (preserved.has(edge.target) && byId.has(edge.source)) preserved.add(edge.source);
  }
  const visibleNodes = nodes.filter((node) =>
    preserved.has(node.id) || !isNoise(node, mode),
  );
  const visibleIds = new Set(visibleNodes.map((node) => node.id));
  const visibleEdges = edges.filter((edge) =>
    visibleIds.has(edge.source) && visibleIds.has(edge.target),
  );
  return {
    nodes: visibleNodes,
    edges: visibleEdges,
    hiddenNodes: nodes.length - visibleNodes.length,
    hiddenEdges: edges.length - visibleEdges.length,
  };
}
