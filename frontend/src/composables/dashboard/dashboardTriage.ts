import type { AgentEvent } from "./dashboardConstants";

export type TriagePriority = "attention" | "failed" | "routine";
export interface TriageGroup {
  key: string;
  priority: TriagePriority;
  reason: string;
  count: number;
  records: number;
  latest: AgentEvent;
}
const blocked = new Set([
  "block",
  "blocked",
  "deny",
  "denied",
  "reject",
  "rejected",
]);
export function classifyEvent(event: AgentEvent): {
  priority: TriagePriority;
  reason: string;
} {
  if (blocked.has((event.decision ?? "").toLowerCase()))
    return { priority: "attention", reason: "策略阻断" };
  if (["semantic_alert", "agentsight_alert"].includes(event.type))
    return { priority: "attention", reason: "告警事件" };
  if ((event.riskScore ?? 0) >= 70)
    return { priority: "attention", reason: "风险评分 ≥ 70" };
  if ((event.retval ?? 0) < 0)
    return { priority: "failed", reason: "负返回值（不等于安全威胁）" };
  return { priority: "routine", reason: "常规活动" };
}
export function eventOccurrences(event: AgentEvent): number {
  const count = event.occurrenceCount;
  return count !== undefined && Number.isFinite(count) && count >= 1
    ? Math.floor(count)
    : 1;
}

// Group only within the current bounded buffer. Preserve security outcomes and
// process/session boundaries; raw evidence is never removed by summarization.
export function buildDashboardTriage(
  events: readonly AgentEvent[],
  limit = 50,
) {
  const groups = new Map<string, TriageGroup>();
  const counts: Record<TriagePriority, number> = {
    attention: 0,
    failed: 0,
    routine: 0,
  };
  for (const event of events) {
    const { priority, reason } = classifyEvent(event);
    const weight = eventOccurrences(event);
    counts[priority] += weight;
    const key = JSON.stringify([
      priority,
      reason,
      event.agentRunId,
      event.conversationId,
      event.pid,
      event.ppid,
      event.comm,
      event.type,
      event.path,
      event.netEndpoint,
      event.netDirection,
      event.toolName,
      event.decision,
      event.retval,
    ]);
    const group = groups.get(key);
    if (group) {
      group.count += weight;
      group.records++;
      if ((event.receivedAtMs ?? 0) > (group.latest.receivedAtMs ?? 0))
        group.latest = event;
    } else
      groups.set(key, {
        key,
        priority,
        reason,
        count: weight,
        records: 1,
        latest: event,
      });
  }
  const rank = { attention: 0, failed: 1, routine: 2 };
  const ordered = [...groups.values()].sort(
    (a, b) =>
      rank[a.priority] - rank[b.priority] ||
      b.count - a.count ||
      (b.latest.receivedAtMs ?? 0) - (a.latest.receivedAtMs ?? 0),
  );
  return {
    counts,
    total: counts.attention + counts.failed + counts.routine,
    records: events.length,
    groupCount: groups.size,
    foldedRecords: events.length - groups.size,
    groups: ordered.slice(0, Math.max(0, limit)),
  };
}
