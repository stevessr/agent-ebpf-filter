import { describe, expect, test } from "bun:test";
import { selectBoundedCaptureEvents } from "../src/components/agentsight/harnessCaptureSelection";

type Sample = { id: string; timestamp: number; priority: number };
const priority = (item: Sample) => item.priority;
const identity = (item: Sample) => item.id;
const compareRank = (a: Sample, b: Sample) =>
  a.priority - b.priority ||
  b.timestamp - a.timestamp ||
  a.id.localeCompare(b.id);
const chrono = (a: Sample, b: Sample) =>
  a.timestamp - b.timestamp || a.id.localeCompare(b.id);
const reference = (samples: Sample[], budget: number) =>
  samples.slice().sort(compareRank).slice(0, budget).sort(chrono);

describe("bounded Harness capture selection", () => {
  test("exactly matches a full stable priority sort with ties and shuffling", () => {
    const data: Sample[] = Array.from({ length: 1234 }, (_, i) => ({
      id: `event-${String(i).padStart(5, "0")}`,
      timestamp: 1000 + (i % 27),
      priority: i % 7 === 0 ? 0 : i % 3,
    }));
    for (const limit of [1, 3, 10, 200, 1000, 1234, 2048]) {
      expect(selectBoundedCaptureEvents(data, limit, priority, identity))
        .toEqual(reference(data, limit));
      expect(selectBoundedCaptureEvents(data.slice().reverse(), limit, priority, identity))
        .toEqual(reference(data, limit));
    }
    expect(data[0].id).toBe("event-00000");
  });

  test("retains old critical events over thousands of recent benign observations", () => {
    const data: Sample[] = [
      { id: "very-old-alert", timestamp: 1, priority: 0 },
      ...Array.from({ length: 40000 }, (_, i) => ({
        id: `noise-${i}`, timestamp: i + 2, priority: 3,
      })),
    ];
    const selected = selectBoundedCaptureEvents(data, 200, priority, identity);
    expect(selected).toHaveLength(200);
    expect(selected[0].id).toBe("very-old-alert");
    expect(selected.at(-1)?.id).toBe("noise-39999");
  });

  test("fast path matches full sort for ascending and descending chronological streams", () => {
    const samples: Sample[] = Array.from({ length: 25000 }, (_, i) => ({
      id: "stream-" + String(i).padStart(5, "0"),
      timestamp: 123_000 + i,
      priority: i % 17 === 0 ? 0 : i % 13 === 0 ? 1 : i % 5 === 0 ? 2 : 3,
    }));
    for (const limit of [1, 19, 1000, 2500]) {
      const expected = reference(samples, limit);
      expect(selectBoundedCaptureEvents(samples, limit, priority, identity))
        .toEqual(expected);
      expect(selectBoundedCaptureEvents(samples.slice().reverse(), limit, priority, identity))
        .toEqual(expected);
    }
  });

  test("repeated timestamps trigger deterministic identity tie-breaker", () => {
    const samples: Sample[] = [
      { id: "z", timestamp: 100, priority: 0 },
      { id: "b", timestamp: 100, priority: 0 },
      { id: "a", timestamp: 100, priority: 0 },
      { id: "c", timestamp: 101, priority: 2 },
      { id: "d", timestamp: 102, priority: 3 },
    ];
    expect(selectBoundedCaptureEvents(samples, 2, priority, identity)
      .map((item) => item.id)).toEqual(["a", "b"]);
    expect(selectBoundedCaptureEvents(samples.slice().reverse(), 2, priority, identity)
      .map((item) => item.id)).toEqual(["a", "b"]);
  });

  test("unexpected priority classes fall back without dropping observations", () => {
    const samples: Sample[] = Array.from({ length: 300 }, (_, i) => ({
      id: "other-" + i, timestamp: i,
      priority: i % 23 === 0 ? 5 : i % 3,
    }));
    expect(selectBoundedCaptureEvents(samples, 50, priority, identity))
      .toEqual(reference(samples, 50));
  });

  test("bounded selection handles empty, malformed budget and clock skew", () => {
    const data: Sample[] = [
      { id: "bad-clock", timestamp: Number.NaN, priority: 2 },
      { id: "normal", timestamp: 1, priority: 1 },
    ];
    expect(selectBoundedCaptureEvents(data, 0, priority, identity)).toEqual([]);
    expect(selectBoundedCaptureEvents(data, Number.NaN, priority, identity)).toEqual([]);
    expect(selectBoundedCaptureEvents([], 100, priority, identity)).toEqual([]);
    expect(selectBoundedCaptureEvents(data, 1, priority, identity)
      .map((item) => item.id)).toEqual(["normal"]);
  });
});
