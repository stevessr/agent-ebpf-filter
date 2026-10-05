import { describe, expect, test } from "bun:test";

import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import {
  buildRenewActivityBuckets,
  buildRenewCategorySlices,
  buildRenewSafetySlices,
  renewEventCategory,
} from "../src/composables/renew/dashboardVisualization";

const now = 1_800_000_000_000;

function event(overrides: Partial<AgentEvent> = {}): AgentEvent {
  return {
    key: Math.random().toString(16),
    pid: 42,
    ppid: 1,
    uid: 1000,
    type: "read",
    tag: "Unknown",
    comm: "codex",
    path: "/tmp/a",
    time: new Date(now - 60_000).toISOString(),
    receivedAtMs: now - 60_000,
    ...overrides,
  };
}

describe("Renew dashboard visualization", () => {
  test("buckets the last hour without counting older history", () => {
    const events = [
      event({ key: "a", receivedAtMs: now - 60_000 }),
      event({ key: "b", receivedAtMs: now - 6 * 60_000, riskScore: 65 }),
      event({ key: "c", receivedAtMs: now - 61 * 60_000 }),
    ];

    const buckets = buildRenewActivityBuckets(events, now, 12, 5);
    expect(buckets).toHaveLength(12);
    expect(buckets.reduce((sum, bucket) => sum + bucket.count, 0)).toBe(2);
    expect(buckets.reduce((sum, bucket) => sum + bucket.attention, 0)).toBe(1);
  });

  test("classifies ordinary user-facing activity groups", () => {
    expect(renewEventCategory(event({ type: "write" }))).toBe("file");
    expect(renewEventCategory(event({ type: "tcp_connect" }))).toBe("network");
    expect(renewEventCategory(event({ type: "process_exec" }))).toBe("process");
    expect(renewEventCategory(event({ type: "native_hook" }))).toBe("agent");
    expect(renewEventCategory(event({ type: "semantic_alert" }))).toBe("alert");

    const slices = buildRenewCategorySlices([
      event({ key: "file", type: "read" }),
      event({ key: "network", type: "network_connect" }),
      event({ key: "agent", type: "wrapper_intercept" }),
    ]);
    expect(slices.find((slice) => slice.key === "file")?.count).toBe(1);
    expect(slices.find((slice) => slice.key === "network")?.count).toBe(1);
    expect(slices.find((slice) => slice.key === "agent")?.count).toBe(1);
  });

  test("separates normal, attention and high-risk outcomes", () => {
    const slices = buildRenewSafetySlices([
      event({ key: "normal", decision: "ALLOW" }),
      event({ key: "warning", riskScore: 65 }),
      event({ key: "danger", decision: "BLOCK" }),
    ]);

    expect(slices.find((slice) => slice.key === "normal")?.count).toBe(1);
    expect(slices.find((slice) => slice.key === "warning")?.count).toBe(1);
    expect(slices.find((slice) => slice.key === "danger")?.count).toBe(1);
  });
});
