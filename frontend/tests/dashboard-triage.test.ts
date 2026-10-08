import { describe, expect, test } from "bun:test";
import {
  buildDashboardTriage,
  classifyEvent,
} from "../src/composables/dashboard/dashboardTriage";
import { appendBounded } from "../src/composables/dashboard/dashboardBuffer";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
function event(extra: Partial<AgentEvent> = {}): AgentEvent {
  return {
    key: "sample",
    pid: 10,
    ppid: 1,
    uid: 1000,
    comm: "node",
    path: "/tmp/test",
    type: "read",
    tag: "agent",
    time: "12:00:00",
    receivedAtMs: 1,
    ...extra,
  };
}
describe("dashboard automatic triage", () => {
  test("folds repeats without mutating evidence; weights existing counts", () => {
    const source = [
      event(),
      event({ key: "latest", occurrenceCount: 4, receivedAtMs: 10 }),
    ];
    const before = JSON.stringify(source);
    const summary = buildDashboardTriage(source);
    expect(summary.total).toBe(5);
    expect(summary.groupCount).toBe(1);
    expect(summary.foldedRecords).toBe(1);
    expect(summary.groups[0]!.latest.key).toBe("latest");
    expect(JSON.stringify(source)).toBe(before);
  });
  test("preserves process, session, destination and outcome boundaries", () => {
    expect(
      buildDashboardTriage([
        event(),
        event({ pid: 11 }),
        event({ agentRunId: "another" }),
        event({ netEndpoint: "other:443" }),
        event({ decision: "BLOCK" }),
        event({ retval: -1 }),
      ]).groupCount,
    ).toBe(6);
  });
  test("puts rare alerts before noisy routine groups and does not equate failure with threat", () => {
    const summary = buildDashboardTriage(
      [
        event({ occurrenceCount: 10000 }),
        event({ retval: -2 }),
        event({ riskScore: 70 }),
      ],
      2,
    );
    expect(summary.groups.map((g) => g.priority)).toEqual([
      "attention",
      "failed",
    ]);
    expect(summary.groupCount).toBe(3);
    expect(summary.counts.routine).toBe(10000);
    expect(classifyEvent(event({ retval: -1 })).priority).toBe("failed");
    expect(classifyEvent(event({ type: "semantic_alert" })).priority).toBe(
      "attention",
    );
  });
  test("handles empty buffers and invalid occurrence counts", () => {
    expect(buildDashboardTriage([]).total).toBe(0);
    expect(
      buildDashboardTriage([
        event({ occurrenceCount: NaN }),
        event({ occurrenceCount: -5 }),
      ]).total,
    ).toBe(2);
  });
  test("50k repeated events produce one review group", () => {
    const summary = buildDashboardTriage(
      Array.from({ length: 50000 }, (_, i) => event({ key: String(i) })),
    );
    expect(summary.groups.length).toBe(1);
    expect(summary.total).toBe(50000);
  });
});
describe("bounded event queues", () => {
  test("keeps newest events, counts overflow, handles bursts without argument spreading", () => {
    const queue = [1, 2, 3];
    expect(appendBounded(queue, [4, 5], 4)).toBe(1);
    expect(queue).toEqual([2, 3, 4, 5]);
    expect(
      appendBounded(
        queue,
        Array.from({ length: 200000 }, (_, i) => i),
        3,
      ),
    ).toBe(200001);
    expect(queue).toEqual([199997, 199998, 199999]);
    expect(appendBounded(queue, [], 1)).toBe(2);
    expect(queue).toEqual([199999]);
  });
});
