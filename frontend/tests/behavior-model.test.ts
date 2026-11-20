import { describe, expect, test } from "bun:test";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import { pb } from "../src/pb/tracker_pb.js";
import { BEHAVIOR_LANES, buildBehaviorSnapshot, createBehaviorDemo, isAttentionEvent, laneForEvent } from "../src/composables/behavior/behaviorModel";

function event(overrides: Partial<AgentEvent> = {}): AgentEvent {
  return {
    key: "event", pid: 10, ppid: 1, uid: 1000, type: "unknown", tag: "test", comm: "node", path: "",
    time: "", receivedAtMs: 10_000, ...overrides,
  };
}

describe("behavior model", () => {
  test("keeps inclusive window boundary, rejects invalid and future timestamps, sorts without mutating input", () => {
    const input = [
      event({ key: "mid", receivedAtMs: 8_000 }),
      event({ key: "edge", receivedAtMs: 5_000 }),
      event({ key: "future-tolerated", receivedAtMs: 10_030 }),
      event({ key: "too-future", receivedAtMs: 40_001 }),
      event({ key: "invalid", receivedAtMs: Number.NaN }),
      event({ key: "missing", receivedAtMs: undefined }),
      event({ key: "old", receivedAtMs: 4_999 }),
    ];
    const originalOrder = input.map((item) => item.key);
    const snapshot = buildBehaviorSnapshot(input, 10_000, 5_000);
    expect(snapshot.events.map((item) => item.key)).toEqual(["mid", "edge"]);
    expect(input.map((item) => item.key)).toEqual(originalOrder);
    expect(snapshot.histogram.reduce((sum, count) => sum + count, 0)).toBe(snapshot.events.length);
  });

  test("filters by PID before deriving process and lane counts", () => {
    const snapshot = buildBehaviorSnapshot([
      event({ pid: 7, comm: "git", type: "execve", receivedAtMs: 9_000 }),
      event({ pid: 8, comm: "python", type: "write", receivedAtMs: 9_100 }),
      event({ pid: 7, comm: "git", type: "read", receivedAtMs: 9_200 }),
    ], 10_000, 2_000, 7);
    expect(snapshot.events).toHaveLength(2);
    expect(snapshot.processes).toEqual([{ pid: 7, comm: "git", count: 2, attention: 0 }]);
    expect(snapshot.lanes).toEqual([{ key: "file", count: 1 }, { key: "network", count: 0 }, { key: "process", count: 1 }, { key: "other", count: 0 }]);
  });

  test("classifies known numeric values and conservative string fallbacks", () => {
    expect(laneForEvent(event({ eventType: 1, type: "totally_unknown" }))).toBe("file");
    expect(laneForEvent(event({ eventType: 31, type: "unknown" }))).toBe("network");
    expect(laneForEvent(event({ eventType: 27, type: "unknown" }))).toBe("process");
    expect(laneForEvent(event({ type: "NETWORK_CONNECT" }))).toBe("network");
    expect(laneForEvent(event({ type: "arbitrary_read_like_string" }))).toBe("other");
    expect(laneForEvent(event({ eventType: 999, type: "write" }))).toBe("file");
    expect(laneForEvent(event({ eventType: 25, type: "write" }))).toBe("other");
    expect(BEHAVIOR_LANES).toEqual(["file", "network", "process", "other"]);
  });

  test("separates attention policy from execution outcome", () => {
    expect(isAttentionEvent(event({ type: "semantic_alert" }))).toBe(true);
    expect(isAttentionEvent(event({ eventType: pb.EventType.AGENTSIGHT_ALERT }))).toBe(true);
    expect(isAttentionEvent(event({ riskScore: 59.99 }))).toBe(false);
    expect(isAttentionEvent(event({ riskScore: 60 }))).toBe(true);
    expect(isAttentionEvent(event({ decision: " deny " }))).toBe(true);
    expect(isAttentionEvent(event({ decision: "BLOCK", retval: 0 }))).toBe(true);
    expect(isAttentionEvent(event({ decision: "ALLOW", retval: -1 }))).toBe(false);
    const snapshot = buildBehaviorSnapshot([
      event({ key: "policy-only", decision: "BLOCK", retval: 0 }),
      event({ key: "risk", riskScore: 80, receivedAtMs: 9_000 }),
    ], 10_000, 2_000);
    expect(snapshot.attentionCount).toBe(2);
    expect(snapshot.processes[0]?.attention).toBe(2);
    // The model reports the recorded decision; it never rewrites retval or infers success.
    expect(snapshot.events.find((item) => item.key === "policy-only")?.retval).toBe(0);
  });

  test("aggregates non-negative bytes, bucket counts and window rate from retained records only", () => {
    const snapshot = buildBehaviorSnapshot([
      event({ key: "net", netBytes: 100, bytes: 900, receivedAtMs: 10_000 }),
      event({ key: "fallback", netBytes: -3, bytes: 25, receivedAtMs: 4_000 }),
      event({ key: "negative", netBytes: -1, bytes: -9, receivedAtMs: 4_999 }),
      event({ key: "outside", netBytes: 1_000, receivedAtMs: -1 }),
    ], 10_000, 10_000);
    expect(snapshot.byteCount).toBe(125);
    expect(snapshot.histogram).toHaveLength(30);
    expect(snapshot.histogram.reduce((sum, count) => sum + count, 0)).toBe(3);
    expect(snapshot.rate).toBe(0.2); // one event in the most recent five seconds
    expect(snapshot.maxRisk).toBeNull();
  });

  test("demo is deterministic, namespaced, populated, and spans the preceding minute", () => {
    const first = createBehaviorDemo(100_000);
    expect(first).toEqual(createBehaviorDemo(100_000));
    expect(first).toHaveLength(150);
    expect(first.every((item) => item.key.startsWith("demo-") && item.eventId?.startsWith("demo-"))).toBe(true);
    expect(new Set(first.map((item) => item.comm))).toEqual(new Set(["node", "claude", "git", "python"]));
    expect(Math.min(...first.map((item) => item.receivedAtMs!))).toBeGreaterThanOrEqual(40_000);
    expect(Math.max(...first.map((item) => item.receivedAtMs!))).toBe(100_000);
  });
});
