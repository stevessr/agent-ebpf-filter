import type { AgentEvent } from "./dashboardConstants";

export interface InsightBucket {
  key: string;
  label: string;
  count: number;
}

export interface DestinationInsight {
  destination: string;
  count: number;
}

export interface AgentRunInsight {
  id: string;
  label: string;
  events: number;
  toolCalls: number;
  alerts: number;
  blocked: number;
  destinations: number;
  lastSeenMs: number;
}

export interface DashboardInsights {
  totalEvents: number;
  observedPids: number;
  agentRuns: number;
  toolCalls: number;
  mcpCalls: number;
  mcpServers: InsightBucket[];
  alerts: number;
  blocked: number;
  highRisk: number;
  sensitiveFileTouches: number;
  networkDestinations: number;
  topDestinations: DestinationInsight[];
  fileOperations: InsightBucket[];
  installOperations: InsightBucket[];
  gitOperations: InsightBucket[];
  correlatedKernelExecs: number;
  pairedKernelExecs: number;
  semanticGaps: number;
  dualLayerCoverage: number | null;
  recentRuns: AgentRunInsight[];
}

type MutableRunInsight = {
  id: string;
  label: string;
  events: number;
  toolCallIds: Set<string>;
  anonymousToolCalls: number;
  alerts: number;
  blocked: number;
  destinations: Set<string>;
  lastSeenMs: number;
};

const HOOK_EVENT_TYPES = new Set(["wrapper_intercept", "native_hook"]);
const KERNEL_EXEC_EVENT_TYPES = new Set(["execve", "process_exec"]);
const COMMAND_EVENT_TYPES = new Set([
  "execve",
  "process_exec",
  "wrapper_intercept",
  "native_hook",
]);
const FILE_READ_EVENT_TYPES = new Set(["read"]);
const FILE_WRITE_EVENT_TYPES = new Set(["write"]);
const FILE_DELETE_EVENT_TYPES = new Set(["unlink"]);
const FILE_MUTATION_EVENT_TYPES = new Set([
  "rename",
  "chmod",
  "chown",
  "link",
  "symlink",
  "mknod",
  "mkdir",
]);
const ALERT_EVENT_TYPES = new Set(["semantic_alert", "agentsight_alert"]);
const BLOCK_DECISIONS = new Set([
  "block",
  "blocked",
  "deny",
  "denied",
  "reject",
  "rejected",
]);

const SENSITIVE_PATH_PATTERNS = [
  /(^|\/)\.ssh(\/|$)/i,
  /(^|\/)\.aws\/(?:credentials|config)(?:$|\/)/i,
  /(^|\/)\.kube\/config(?:$|\/)/i,
  /(^|\/)\.env(?:\.[^/]*)?$/i,
  /(^|\/)(?:credentials?|secrets?|tokens?)(?:\.|\/|$)/i,
  /^\/etc\/(?:shadow|sudoers(?:\.d\/)?|ssh\/)/i,
];

const INSTALL_BUCKETS: Array<{
  key: string;
  label: string;
  test: (text: string) => boolean;
}> = [
  {
    key: "python",
    label: "Python / uv",
    test: (text) => /\b(?:pip3?|uv\s+pip)\s+install\b/i.test(text),
  },
  {
    key: "system",
    label: "系统包管理器",
    test: (text) =>
      /\b(?:apt(?:-get)?|dnf|yum|zypper)\b[^;&|\n]*\binstall\b/i.test(
        text,
      ) || /\bpacman\b[^;&|\n]*\s-(?:[^\s]*s[^\s]*)\b/i.test(text),
  },
  {
    key: "node",
    label: "Node 全局",
    test: (text) =>
      /\bnpm\s+(?:install|i)\b[^;&|\n]*(?:\s-g\b|\s--global\b)/i.test(
        text,
      ) ||
      /\bpnpm\s+(?:add|install)\b[^;&|\n]*(?:\s-g\b|\s--global\b)/i.test(
        text,
      ) ||
      /\byarn\s+global\s+add\b/i.test(text),
  },
  {
    key: "other",
    label: "其它安装",
    test: (text) =>
      /\b(?:cargo|go|brew|flatpak|snap)\s+install\b/i.test(text),
  },
];

const GIT_BUCKETS: Array<{
  key: string;
  label: string;
  test: (text: string) => boolean;
}> = [
  { key: "push", label: "git push", test: (text) => gitSubcommand(text, "push") },
  { key: "clone", label: "git clone", test: (text) => gitSubcommand(text, "clone") },
  { key: "commit", label: "git commit", test: (text) => gitSubcommand(text, "commit") },
  {
    key: "sync",
    label: "git pull/fetch",
    test: (text) => gitSubcommand(text, "pull") || gitSubcommand(text, "fetch"),
  },
  {
    key: "gh",
    label: "gh CLI",
    test: (text) => /(?:^|[\s/])gh(?:\s|$)/i.test(text),
  },
  {
    key: "other",
    label: "其它 git",
    test: (text) =>
      /(?:^|[\s/])git(?:\s|$)/i.test(text) || /\/git(?:\s|$)/i.test(text),
  },
];

function normalizeEventType(event: AgentEvent): string {
  return String(event.type ?? "").trim().toLowerCase();
}

function eventWeight(event: AgentEvent): number {
  const count = Number(event.occurrenceCount ?? 1);
  return Number.isFinite(count) && count > 0 ? Math.floor(count) : 1;
}

function eventText(event: AgentEvent): string {
  return [
    event.comm,
    event.path,
    event.extraInfo,
    event.extraPath,
    event.toolName,
  ]
    .filter((value): value is string => Boolean(value))
    .join(" ")
    .replace(/\s+/g, " ")
    .trim();
}

function gitSubcommand(text: string, subcommand: string): boolean {
  const escaped = subcommand.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(?:^|[\\s/])git\\s+(?:-[^\\s]+\\s+)*${escaped}\\b`, "i").test(
    text,
  );
}

function isBlocked(event: AgentEvent): boolean {
  return BLOCK_DECISIONS.has(String(event.decision ?? "").trim().toLowerCase());
}

function isAlert(event: AgentEvent): boolean {
  return (
    ALERT_EVENT_TYPES.has(normalizeEventType(event)) ||
    isBlocked(event) ||
    (event.riskScore ?? 0) >= 70
  );
}

function isHighRisk(event: AgentEvent): boolean {
  return isBlocked(event) || (event.riskScore ?? 0) >= 80;
}

function isSensitiveFileEvent(event: AgentEvent): boolean {
  const type = normalizeEventType(event);
  if (
    !FILE_READ_EVENT_TYPES.has(type) &&
    !FILE_WRITE_EVENT_TYPES.has(type) &&
    !FILE_DELETE_EVENT_TYPES.has(type) &&
    !FILE_MUTATION_EVENT_TYPES.has(type) &&
    type !== "open" &&
    type !== "openat"
  ) {
    return false;
  }
  const value = `${event.path ?? ""}\n${event.extraPath ?? ""}`;
  return SENSITIVE_PATH_PATTERNS.some((pattern) => pattern.test(value));
}

function normalizeDestination(event: AgentEvent): string | null {
  const preferred = String(event.domain ?? "").trim();
  let value = preferred || String(event.netEndpoint ?? "").trim();
  if (!value) return null;

  value = value.replace(/^[a-z][a-z0-9+.-]*:\/\//i, "").split("/")[0]?.trim() ?? "";
  if (!value) return null;

  if (value.startsWith("[")) {
    const end = value.indexOf("]");
    if (end > 1) value = value.slice(1, end);
  } else {
    const lastColon = value.lastIndexOf(":");
    if (lastColon > 0 && /^\d+$/.test(value.slice(lastColon + 1))) {
      value = value.slice(0, lastColon);
    }
  }

  value = value.trim().toLowerCase();
  if (!value || value === "0.0.0.0" || value === "::") return null;
  return value;
}

function correlationKeys(event: AgentEvent): string[] {
  const keys: string[] = [];
  if (event.toolCallId) keys.push(`tool:${event.toolCallId}`);
  if (event.spanId) keys.push(`span:${event.spanId}`);
  if (event.traceId) keys.push(`trace:${event.traceId}`);
  return keys;
}

function runIdentity(event: AgentEvent): { id: string; label: string } | null {
  if (event.agentRunId) {
    return { id: `run:${event.agentRunId}`, label: event.agentRunId };
  }
  if (event.conversationId) {
    return { id: `conversation:${event.conversationId}`, label: event.conversationId };
  }
  if (event.rootAgentPid && event.rootAgentPid > 0) {
    return {
      id: `root-pid:${event.rootAgentPid}`,
      label: `root PID ${event.rootAgentPid}`,
    };
  }
  return null;
}

function eventTimestampMs(event: AgentEvent): number {
  if (event.receivedAtMs && Number.isFinite(event.receivedAtMs)) {
    return event.receivedAtMs;
  }
  const parsed = Date.parse(event.time);
  return Number.isFinite(parsed) ? parsed : 0;
}

function increment(map: Map<string, number>, key: string, amount = 1): void {
  map.set(key, (map.get(key) ?? 0) + amount);
}

function bucketList(
  counts: Map<string, number>,
  definitions: Array<{ key: string; label: string }>,
): InsightBucket[] {
  return definitions.map((definition) => ({
    key: definition.key,
    label: definition.label,
    count: counts.get(definition.key) ?? 0,
  }));
}

export function buildDashboardInsights(events: AgentEvent[]): DashboardInsights {
  const observedPids = new Set<number>();
  const runIds = new Set<string>();
  const toolCallIds = new Set<string>();
  let anonymousToolCalls = 0;
  const mcpCallKeys = new Set<string>();
  const mcpServerCounts = new Map<string, number>();
  const destinationCounts = new Map<string, number>();
  const fileCounts = new Map<string, number>();
  const installCounts = new Map<string, number>();
  const gitCounts = new Map<string, number>();
  const hookCorrelationKeys = new Set<string>();
  const runs = new Map<string, MutableRunInsight>();

  let totalEvents = 0;
  let alerts = 0;
  let blocked = 0;
  let highRisk = 0;
  let sensitiveFileTouches = 0;

  for (const event of events) {
    const type = normalizeEventType(event);
    const weight = eventWeight(event);
    totalEvents += weight;

    if (event.pid > 0) observedPids.add(event.pid);

    const blockedEvent = isBlocked(event);
    const alertEvent = isAlert(event);
    if (blockedEvent) blocked += weight;
    if (alertEvent) alerts += weight;
    if (isHighRisk(event)) highRisk += weight;
    if (isSensitiveFileEvent(event)) sensitiveFileTouches += weight;

    if (event.toolCallId) {
      toolCallIds.add(event.toolCallId);
    } else if (HOOK_EVENT_TYPES.has(type)) {
      anonymousToolCalls += weight;
    }

    if (HOOK_EVENT_TYPES.has(type)) {
      for (const key of correlationKeys(event)) hookCorrelationKeys.add(key);
    }

    const toolName = String(event.toolName ?? "").trim();
    const mcpMatch = toolName.match(/^mcp__(.+?)__(.+)$/i);
    if (mcpMatch) {
      const mcpKey = event.toolCallId || event.key;
      if (!mcpCallKeys.has(mcpKey)) {
        mcpCallKeys.add(mcpKey);
        increment(mcpServerCounts, mcpMatch[1], 1);
      }
    }

    const destination = normalizeDestination(event);
    if (
      destination &&
      event.netDirection !== "incoming" &&
      event.netDirection !== "listening"
    ) {
      increment(destinationCounts, destination, weight);
    }

    if (FILE_READ_EVENT_TYPES.has(type)) increment(fileCounts, "read", weight);
    if (FILE_WRITE_EVENT_TYPES.has(type)) increment(fileCounts, "write", weight);
    if (FILE_DELETE_EVENT_TYPES.has(type)) increment(fileCounts, "delete", weight);
    if (FILE_MUTATION_EVENT_TYPES.has(type)) increment(fileCounts, "mutate", weight);

    if (COMMAND_EVENT_TYPES.has(type)) {
      const text = eventText(event);
      const installMatch = INSTALL_BUCKETS.find((bucket) => bucket.test(text));
      if (installMatch) increment(installCounts, installMatch.key, weight);

      const gitMatch = GIT_BUCKETS.find((bucket) => bucket.test(text));
      if (gitMatch) increment(gitCounts, gitMatch.key, weight);
    }

    const run = runIdentity(event);
    if (run) {
      runIds.add(run.id);
      let summary = runs.get(run.id);
      if (!summary) {
        summary = {
          id: run.id,
          label: run.label,
          events: 0,
          toolCallIds: new Set<string>(),
          anonymousToolCalls: 0,
          alerts: 0,
          blocked: 0,
          destinations: new Set<string>(),
          lastSeenMs: 0,
        };
        runs.set(run.id, summary);
      }
      summary.events += weight;
      if (event.toolCallId) summary.toolCallIds.add(event.toolCallId);
      else if (HOOK_EVENT_TYPES.has(type)) summary.anonymousToolCalls += weight;
      if (alertEvent) summary.alerts += weight;
      if (blockedEvent) summary.blocked += weight;
      if (destination) summary.destinations.add(destination);
      summary.lastSeenMs = Math.max(summary.lastSeenMs, eventTimestampMs(event));
    }
  }

  let correlatedKernelExecs = 0;
  let pairedKernelExecs = 0;
  let semanticGaps = 0;

  for (const event of events) {
    if (!KERNEL_EXEC_EVENT_TYPES.has(normalizeEventType(event))) continue;
    const keys = correlationKeys(event);
    if (keys.length === 0) continue;
    correlatedKernelExecs += eventWeight(event);
    const paired = keys.some((key) => hookCorrelationKeys.has(key));
    if (paired) pairedKernelExecs += eventWeight(event);
    else semanticGaps += eventWeight(event);
  }

  const dualLayerCoverage =
    correlatedKernelExecs > 0
      ? Math.max(
          0,
          Math.min(100, (pairedKernelExecs / correlatedKernelExecs) * 100),
        )
      : null;

  const topDestinations = [...destinationCounts.entries()]
    .map(([destination, count]) => ({ destination, count }))
    .sort((a, b) => b.count - a.count || a.destination.localeCompare(b.destination))
    .slice(0, 8);

  const mcpServers = [...mcpServerCounts.entries()]
    .map(([key, count]) => ({ key, label: key, count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
    .slice(0, 6);

  const recentRuns = [...runs.values()]
    .map((run) => ({
      id: run.id,
      label: run.label,
      events: run.events,
      toolCalls: run.toolCallIds.size + run.anonymousToolCalls,
      alerts: run.alerts,
      blocked: run.blocked,
      destinations: run.destinations.size,
      lastSeenMs: run.lastSeenMs,
    }))
    .sort((a, b) => b.lastSeenMs - a.lastSeenMs || b.events - a.events)
    .slice(0, 5);

  return {
    totalEvents,
    observedPids: observedPids.size,
    agentRuns: runIds.size,
    toolCalls: toolCallIds.size + anonymousToolCalls,
    mcpCalls: mcpCallKeys.size,
    mcpServers,
    alerts,
    blocked,
    highRisk,
    sensitiveFileTouches,
    networkDestinations: destinationCounts.size,
    topDestinations,
    fileOperations: bucketList(fileCounts, [
      { key: "read", label: "读取" },
      { key: "write", label: "写入" },
      { key: "delete", label: "删除" },
      { key: "mutate", label: "元数据/改名" },
    ]),
    installOperations: bucketList(installCounts, INSTALL_BUCKETS),
    gitOperations: bucketList(gitCounts, GIT_BUCKETS),
    correlatedKernelExecs,
    pairedKernelExecs,
    semanticGaps,
    dualLayerCoverage,
    recentRuns,
  };
}
