import { describe, expect, test } from "bun:test";

import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import {
  matchesDecisionFilter,
  passesExplorerTriage,
  type DecisionFilter,
} from "../src/composables/renew/explorerTriage";
import { isAttentionEvent } from "../src/composables/renew/eventPresentation";

function event(overrides: Partial<AgentEvent> = {}): AgentEvent {
  return {
    key: "renew-triage-test",
    pid: 42,
    ppid: 1,
    uid: 1000,
    type: "read",
    tag: "Unknown",
    comm: "codex",
    path: "",
    time: "2026-10-04T16:00:00Z",
    ...overrides,
  };
}

describe("Renew explorer triage filters", () => {
  test("blocked filter matches BLOCK and DENY, never ALLOW", () => {
    expect(matchesDecisionFilter(event({ decision: "BLOCK" }), "blocked")).toBe(
      true,
    );
    expect(matchesDecisionFilter(event({ decision: "DENY" }), "blocked")).toBe(
      true,
    );
    expect(matchesDecisionFilter(event({ decision: "BLOCK" }), "allowed")).toBe(
      false,
    );
    expect(matchesDecisionFilter(event({ decision: "DENY" }), "allowed")).toBe(
      false,
    );
  });

  test("alert filter matches ALERT decisions only", () => {
    expect(matchesDecisionFilter(event({ decision: "ALERT" }), "alert")).toBe(
      true,
    );
    expect(matchesDecisionFilter(event({ decision: "ALERT" }), "blocked")).toBe(
      false,
    );
    expect(matchesDecisionFilter(event({ decision: "ALERT" }), "allowed")).toBe(
      false,
    );
  });

  test("allowed filter matches only ALLOW decisions", () => {
    expect(matchesDecisionFilter(event({ decision: "ALLOW" }), "allowed")).toBe(
      true,
    );
    expect(matchesDecisionFilter(event({ decision: "ALLOW" }), "blocked")).toBe(
      false,
    );
    expect(matchesDecisionFilter(event({ decision: "ALLOW" }), "alert")).toBe(
      false,
    );
  });

  test("blank decision value means no decision filtering", () => {
    const samples: AgentEvent[] = [
      event({ decision: "BLOCK" }),
      event({ decision: "ALERT" }),
      event({ decision: "ALLOW" }),
      event({ decision: "REWRITE" }),
      event(),
    ];
    for (const sample of samples) {
      expect(matchesDecisionFilter(sample, "")).toBe(true);
      expect(passesExplorerTriage(sample, "", false)).toBe(true);
    }
  });

  test("decision parsing is normalized through normalizedDecision", () => {
    expect(matchesDecisionFilter(event({ decision: "block" }), "blocked")).toBe(
      true,
    );
    expect(
      matchesDecisionFilter(event({ decision: " allow " }), "allowed"),
    ).toBe(true);
    expect(matchesDecisionFilter(event({ decision: "allow" }), "blocked")).toBe(
      false,
    );
  });

  test("attention toggle keeps attention events and drops normal events", () => {
    const blocked = event({ decision: "BLOCK" });
    const alert = event({ decision: "ALERT" });
    const allowed = event({ decision: "ALLOW" });
    const routine = event({ riskScore: 10 });

    expect(isAttentionEvent(blocked)).toBe(true);
    expect(isAttentionEvent(alert)).toBe(true);
    expect(isAttentionEvent(routine)).toBe(false);

    expect(passesExplorerTriage(blocked, "", true)).toBe(true);
    expect(passesExplorerTriage(alert, "", true)).toBe(true);
    expect(passesExplorerTriage(allowed, "", true)).toBe(false);
    expect(passesExplorerTriage(routine, "", true)).toBe(false);
    expect(passesExplorerTriage(routine, "", false)).toBe(true);
  });

  test("attention toggle composes with the decision filter using AND, not OR", () => {
    const blocked = event({ decision: "BLOCK" });
    const alert = event({ decision: "ALERT" });
    const allowed = event({ decision: "ALLOW" });

    // Both satisfied → passes.
    expect(passesExplorerTriage(blocked, "blocked", true)).toBe(true);
    expect(passesExplorerTriage(alert, "alert", true)).toBe(true);

    // Matches decision but not attention → excluded (OR would keep it).
    expect(passesExplorerTriage(allowed, "allowed", true)).toBe(false);

    // Matches attention but not the decision → excluded (OR would keep it).
    expect(passesExplorerTriage(blocked, "alert", true)).toBe(false);
    expect(passesExplorerTriage(alert, "blocked", true)).toBe(false);
  });
});
