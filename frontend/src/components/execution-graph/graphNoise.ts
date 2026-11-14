import type { ExecutionGraphEdge, ExecutionGraphNode } from "../../types/executionGraph";

/**
 * Presentation-only noise reduction. The full snapshot is retained for
 * recording, replay, inspection and backend enforcement.
 */
export type GraphNoiseMode = "off" | "balanced" | "strict";

export type GraphNoiseReason =
  | "routine-syscall" | "pseudo-file-read"
  | "successful-exit" | "heartbeat";

export interface GraphNoiseResult {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  hiddenNodes: number;
  hiddenEdges: number;
  hiddenByReason: Record<GraphNoiseReason, number>;
}
const emptyReasonCounts = (): Record<GraphNoiseReason, number> => ({
  "routine-syscall": 0,
  "pseudo-file-read": 0,
  "successful-exit": 0,
  heartbeat: 0,
});

const ROUTINE_SYSCALLS = new Set([
  "getpid", "getppid", "gettid", "clock_gettime", "gettimeofday",
  "sched_yield", "futex", "nanosleep", "rt_sigprocmask",
]);
const EXTRA_ROUTINE_SYSCALLS = new Set([
  "epoll_wait", "epoll_pwait", "poll", "ppoll", "select",
  "pselect6", "getrandom", "brk",
]);
const OPEN_SYSCALLS = new Set(["open", "openat", "openat2"]);
const READ_ONLY_SYSCALLS = new Set([
  "read", "pread64", "readv",
  "stat", "lstat", "fstat", "newfstatat", "statx",
  "access", "faccessat", "faccessat2", "readlink", "readlinkat",
  "getdents", "getdents64",
]);
// These paths may expose secrets or kernel state even when opened read-only.
const SENSITIVE_PROC_SYS = /^\/(?:proc\/(?:\d+|self|thread-self)\/(?:mem|fd|map_files|environ|pagemap|stack)(?:\/|$)|proc\/(?:kcore|kallsyms|keys|key-users|sys|sysrq-trigger)(?:\/|$)|sys\/(?:kernel\/security|fs\/bpf)(?:\/|$))/i;
const KNOWN_READ_ONLY_OPEN_FLAGS = new Set([
  "O_RDONLY", "O_CLOEXEC", "O_DIRECTORY", "O_NONBLOCK",
  "O_NOFOLLOW", "O_LARGEFILE", "O_NOATIME",
]);
// Linux O_ACCMODE, O_CREAT, O_TRUNC, O_APPEND, O_TMPFILE.
// Unknown flags are retained; a mutating open must not become "noise".
const UNSAFE_OPEN_MASK = 3 | 0o100 | 0o1000 | 0o2000 | 0o20000000;

function verifiedReadOnlyOpen(node: ExecutionGraphNode): boolean {
  const m = node.metadata ?? {};
  const raw = (m.openFlags ?? m.open_flags ?? m.flags ?? "").trim();
  if (!raw) return false;
  if (/^(?:0x[0-9a-f]+|0o[0-7]+|\d+)$/i.test(raw)) {
    const numeric = Number(raw);
    return Number.isSafeInteger(numeric) && (numeric & UNSAFE_OPEN_MASK) === 0;
  }
  const flags = raw.toUpperCase().split(/[|,\s]+/).filter(Boolean);
  return flags.includes("O_RDONLY") &&
    flags.every((flag) => KNOWN_READ_ONLY_OPEN_FLAGS.has(flag));
}
// Avoid blanket /proc and /sys suppression: maps, cmdline, cgroups,
// debugfs, proc/sys and similar reads may be security-relevant reconnaissance.
const ROUTINE_PATHS = [
  "/sys/bus", "/sys/class", "/sys/devices", "/sys/kernel/mm",
  "/dev/null", "/dev/zero", "/dev/random", "/dev/urandom",
  "/etc/ld.so.cache", "/etc/localtime",
  "/usr/share/zoneinfo", "/usr/share/locale", "/usr/lib/locale",
];
const ROUTINE_PROC_FILES = new Set([
  "/proc/meminfo", "/proc/cpuinfo", "/proc/loadavg", "/proc/uptime",
  "/proc/version", "/proc/self/stat", "/proc/self/status",
]);
const SENSITIVE_PATH = /(?:^|\/)(?:\.ssh|\.aws|\.gnupg|\.kube|\.config|environ|shadow|passwd|credentials|secrets?)(?:\/|$)/i;
const RISK_BYPASS = 60;

function pathFromNode(node: ExecutionGraphNode): string {
  const metadata = node.metadata ?? {};
  const candidate =
    metadata.path || metadata.filePath || metadata.file_path ||
    metadata.targetPath || metadata.target_path || (node.kind === "file" ? node.label : "");
  return candidate.trim();
}

function routinePath(path: string): boolean {
  if (!path.startsWith("/") || SENSITIVE_PATH.test(path) ||
      SENSITIVE_PROC_SYS.test(path) || path.includes("\\") ||
      path.split("/").some((segment) => segment === "." || segment === "..")) return false;
  const normalized = path.replace(/\/+$/, "");
  return ROUTINE_PROC_FILES.has(normalized) ||
    /^\/proc\/(?:\d+|self|thread-self)\/(?:stat|status)$/.test(normalized) ||
    ROUTINE_PATHS.some((prefix) =>
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
  const metadata = node.metadata ?? {};
  const contextualRisk = Number(metadata.riskScore ?? metadata.risk_score ?? "0");
  if (Math.max(node.riskScore ?? 0, Number.isFinite(contextualRisk) ? contextualRisk : 0) >= RISK_BYPASS) {
    return true;
  }
  const decision = [
    metadata.decision, metadata.action, metadata.policyDecision,
    metadata.policy_decision, metadata.verdict,
  ].filter(Boolean).join(" ").toUpperCase();
  return /BLOCK|DENY|ALERT|REWRITE|REJECT|QUARANTINE/.test(decision);
}

function noiseReason(node: ExecutionGraphNode, mode: GraphNoiseMode): GraphNoiseReason | null {
  if (isProtected(node)) return null;
  const path = pathFromNode(node);
  if (node.kind === "syscall") {
    const name = syscallName(node);
    if (ROUTINE_SYSCALLS.has(name)) return "routine-syscall";
    if (mode === "strict" && EXTRA_ROUTINE_SYSCALLS.has(name)) return "routine-syscall";
    if (!routinePath(path)) return null;
    if (OPEN_SYSCALLS.has(name)) {
      return verifiedReadOnlyOpen(node) ? "pseudo-file-read" : null;
    }
    return READ_ONLY_SYSCALLS.has(name) ? "pseudo-file-read" : null;
  }
  if (node.kind === "file" && mode === "strict" && routinePath(path)) {
    const operation = (node.metadata?.operation || node.metadata?.type || "").toLowerCase();
    // Unknown file operations and mutations must remain visible.
    return /^(read|stat|access|readlink|metadata)$/.test(operation)
      ? "pseudo-file-read" : null;
  }
  if (mode === "strict" && node.kind === "exit_status") {
    const status = node.metadata?.exitCode ?? node.metadata?.exit_code ??
      node.metadata?.status;
    return status === "0" ? "successful-exit" : null;
  }
  if (mode === "strict" && (node.kind === "hook_event" || node.kind === "wrapper_event")) {
    return /^(heartbeat|healthcheck|ping)$/i.test(node.metadata?.type ?? "")
      ? "heartbeat" : null;
  }
  return null;
}

export function filterPresentationNoise(
  nodes: ExecutionGraphNode[],
  edges: ExecutionGraphEdge[],
  mode: GraphNoiseMode,
  selectedNodeId = "",
): GraphNoiseResult {
  if (mode === "off") {
    return { nodes, edges, hiddenNodes: 0, hiddenEdges: 0,
      hiddenByReason: emptyReasonCounts() };
  }

  const byId = new Map(nodes.map((node) => [node.id, node]));
  const preserved = new Set<string>();
  for (const node of nodes) {
    if (isProtected(node) || node.id === selectedNodeId) {
      preserved.add(node.id);
    }
  }
  // Preserve direct supporting evidence on policy edges, even when the
  // transport reports the relationship as a generic "observed" edge.
  // The initial protected IDs are immutable in this pass, so this does not
  // accidentally expand into an unbounded connected component.
  const protectedOrigins = new Set(preserved);
  for (const edge of edges) {
    const sourceProtected = protectedOrigins.has(edge.source);
    const targetProtected = protectedOrigins.has(edge.target);
    const policyEndpoint = [edge.source, edge.target].some((id) => {
      const kind = byId.get(id)?.kind;
      return kind === "policy_alert" || kind === "policy_decision";
    });
    if (!policyEndpoint && !/^(alerted|blocked|rewritten|reviewed)$/.test(edge.kind)) continue;
    if (sourceProtected && byId.has(edge.target)) preserved.add(edge.target);
    if (targetProtected && byId.has(edge.source)) preserved.add(edge.source);
  }
  const hiddenByReason = emptyReasonCounts();
  const visibleNodes = nodes.filter((node) => {
    if (preserved.has(node.id)) return true;
    const reason = noiseReason(node, mode);
    if (!reason) return true;
    hiddenByReason[reason] += 1;
    return false;
  });
  const visibleIds = new Set(visibleNodes.map((node) => node.id));
  const visibleEdges = edges.filter((edge) =>
    visibleIds.has(edge.source) && visibleIds.has(edge.target),
  );
  return {
    nodes: visibleNodes,
    edges: visibleEdges,
    hiddenNodes: nodes.length - visibleNodes.length,
    hiddenEdges: edges.length - visibleEdges.length,
    hiddenByReason,
  };
}
