import { computed, type Ref } from "vue";

import type { AgentEvent } from "../dashboard/useDashboard";
import {
  describeEvent,
  isAgentEvent,
  isAttentionEvent,
  normalizedDecision,
} from "./eventPresentation";
import type {
  RenewAgentSession,
  RenewDestinationSummary,
  RenewTone,
} from "./types";

export function useRenewEventSummary(
  events: Ref<AgentEvent[]>,
  search: Ref<string>,
  onlyAgents: Ref<boolean>,
) {
  const recentEvents = computed(() => {
    const query = search.value.trim().toLowerCase();
    return events.value
      .filter((event) => !onlyAgents.value || isAgentEvent(event))
      .filter((event) => {
        if (!query) return true;
        return [
          event.tag,
          event.comm,
          event.type,
          event.path,
          event.netEndpoint,
          event.toolName,
          event.decision,
        ]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(query));
      })
      .slice(0, 16);
  });

  const attentionCount = computed(
    () => events.value.filter(isAttentionEvent).length,
  );

  const attentionEvents = computed(() =>
    events.value.filter(isAttentionEvent).slice(0, 8),
  );

  const blockedCount = computed(
    () =>
      events.value.filter((event) => {
        const decision = normalizedDecision(event);
        return decision.includes("BLOCK") || decision.includes("DENY");
      }).length,
  );

  const agentEventCount = computed(
    () => events.value.filter(isAgentEvent).length,
  );

  const activeAgents = computed<RenewAgentSession[]>(() => {
    const runs = new Map<string, RenewAgentSession>();

    for (const event of events.value) {
      if (!isAgentEvent(event)) continue;
      const key =
        event.agentRunId ||
        event.conversationId ||
        `${event.comm}:${event.rootAgentPid || event.pid}`;
      const previous = runs.get(key);
      const receivedAt = event.receivedAtMs || Date.now();
      const label =
        event.tag && event.tag !== "Unknown" ? event.tag : event.comm || "Agent";

      if (!previous) {
        runs.set(key, {
          key,
          label,
          events: 1,
          alerts: isAttentionEvent(event) ? 1 : 0,
          lastSeen: receivedAt,
          lastAction: describeEvent(event),
        });
        continue;
      }

      previous.events += 1;
      if (isAttentionEvent(event)) previous.alerts += 1;
      if (receivedAt >= previous.lastSeen) {
        previous.lastSeen = receivedAt;
        previous.lastAction = describeEvent(event);
      }
    }

    return [...runs.values()]
      .sort((a, b) => b.lastSeen - a.lastSeen)
      .slice(0, 6);
  });

  const topDestinations = computed<RenewDestinationSummary[]>(() => {
    const counts = new Map<string, number>();
    for (const event of events.value) {
      if (!event.netEndpoint) continue;
      counts.set(event.netEndpoint, (counts.get(event.netEndpoint) || 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([endpoint, count]) => ({ endpoint, count }));
  });

  const alertTone = computed<RenewTone>(() => {
    if (blockedCount.value > 0) return "danger";
    if (attentionCount.value > 0) return "warning";
    return "normal";
  });

  return {
    recentEvents,
    attentionCount,
    attentionEvents,
    blockedCount,
    agentEventCount,
    activeAgents,
    topDestinations,
    alertTone,
  };
}
