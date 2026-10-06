import type { AgentEvent } from "../dashboard/dashboardConstants";
import { isAttentionEvent, normalizedDecision } from "./eventPresentation";

export type DecisionFilter = "" | "blocked" | "alert" | "allowed";

// Decision-based triage predicates for the explorer filter bar. Decision parsing
// is delegated to normalizedDecision so lower/upper-case decisions agree.
export function matchesDecisionFilter(
  event: AgentEvent,
  filter: DecisionFilter,
): boolean {
  if (!filter) return true;
  const decision = normalizedDecision(event);
  if (filter === "blocked") return decision === "BLOCK" || decision === "DENY";
  if (filter === "alert") return decision === "ALERT";
  return decision === "ALLOW";
}

export function passesExplorerTriage(
  event: AgentEvent,
  filter: DecisionFilter,
  attentionOnly: boolean,
): boolean {
  return (
    matchesDecisionFilter(event, filter) &&
    (!attentionOnly || isAttentionEvent(event))
  );
}
