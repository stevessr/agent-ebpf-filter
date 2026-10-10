import { afterEach, describe, expect, test } from "bun:test";
import { effectScope, nextTick, ref } from "vue";
import { pb } from "../src/pb/tracker_pb.js";
import {
  normalizeEnvelope,
  normalizeHistoryRecord,
} from "../src/composables/dashboard/dashboardHelpers";
import { mergeDashboardRecords } from "../src/composables/dashboard/dashboardBuffer";
import { useDashboardStream } from "../src/composables/dashboard/useDashboardStream";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";

function envelope(id: string, ms = 100): pb.IEventEnvelope {
  return {
    eventId: id,
    ingestTimestampNs: ms * 1_000_000,
    legacyEvent: { pid: 42, path: "/same", comm: "node" },
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
class Socket {
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onmessage: ((message: { data: Uint8Array }) => void) | null = null;
  close() {}
  emit(...envelopes: pb.IEventEnvelope[]) {
    this.onmessage?.({
      data: pb.EventEnvelopeBatch.encode({ envelopes }).finish(),
    });
  }
}
const cleanup: (() => void)[] = [];
afterEach(() => {
  for (const stop of cleanup.splice(0)) stop();
});
function harness(capacity = 5) {
  const scope = effectScope();
  const sockets: Socket[] = [];
  const requests: (ReturnType<typeof deferred<pb.IEventHistoryResponse>> & {
    signal: AbortSignal;
  })[] = [];
  const clearRequest = deferred<unknown>();
  const notices: string[] = [];
  const deps = {
    events: ref<AgentEvent[]>([]),
    isConnected: ref(false),
    isPaused: ref(false),
    maxEvents: ref(capacity),
    getFilteredEvents: () => [],
  };
  const stream = scope.run(() =>
    useDashboardStream(deps, {
      open: () => {
        const socket = new Socket();
        sockets.push(socket);
        return socket as unknown as WebSocket;
      },
      history: (signal) => {
        const request = { ...deferred<pb.IEventHistoryResponse>(), signal };
        requests.push(request);
        return request.promise;
      },
      clear: () => clearRequest.promise,
      notify: (_, text) => {
        notices.push(text);
      },
    }),
  )!;
  cleanup.push(() => {
    stream.stopStream();
    scope.stop();
    for (const request of requests) request.resolve({ events: [] });
  });
  stream.startStream();
  return { ...deps, stream, sockets, requests, clearRequest, notices };
}
const settle = async () => {
  await Promise.resolve();
  await nextTick();
};

describe("dashboard continuity", () => {
  test("history and protobuf envelope share identity; identical distinct syscalls survive", () => {
    const wire = pb.EventEnvelope.decode(
      pb.EventEnvelope.encode(envelope("one")).finish(),
    );
    const live = normalizeEnvelope(wire)!;
    const history = normalizeHistoryRecord({ envelope: wire, timestamp: 100 })!;
    expect(live.key).toBe(history.key);
    const second = normalizeEnvelope(envelope("two"))!;
    expect(
      mergeDashboardRecords([live], [history, second], 5).records.map(
        (e) => e.eventId,
      ),
    ).toEqual(["one", "two"]);
    const legacy = normalizeHistoryRecord({
      event: { pid: 42 },
      timestamp: 100,
    })!;
    expect(mergeDashboardRecords([legacy], [legacy], 5).records).toHaveLength(
      2,
    );
  });
  test("server JSON fallback preserves snake-case envelope ID and timestamp", () => {
    const event = normalizeHistoryRecord({
      Envelope: {
        event_id: "json",
        ingest_timestamp_ns: "123000000",
        legacy_event: { pid: 42, path: "/same" },
      },
      Timestamp: 500,
    });
    expect(event?.eventId).toBe("json");
    expect(event?.receivedAtMs).toBe(123);
  });
  test("history protobuf roundtrip preserves envelope identity", () => {
    const decoded = pb.EventHistoryResponse.decode(
      pb.EventHistoryResponse.encode({
        events: [{ envelope: envelope("history-wire"), timestamp: 100 }],
      }).finish(),
    );
    expect(normalizeHistoryRecord(decoded.events[0])?.eventId).toBe(
      "history-wire",
    );
  });
  test("replayed batches cannot evict unique paused evidence", async () => {
    const h = harness(3);
    h.isPaused.value = true;
    await nextTick();
    h.sockets[0]!.emit(envelope("one", 1), envelope("two", 2));
    h.sockets[0]!.emit(
      ...Array.from({ length: 20 }, () => envelope("two", 2)),
      envelope("three", 3),
    );
    expect(h.stream.queuedRecords.value).toBe(3);
    expect(h.stream.droppedRecords.value).toBe(0);
    h.isPaused.value = false;
    await nextTick();
    expect(h.events.value.map((e) => e.eventId)).toEqual([
      "three",
      "two",
      "one",
    ]);
  });
  test("subscribe before snapshot; live/history overlap is displayed once", async () => {
    const h = harness();
    expect(h.requests).toHaveLength(0);
    h.sockets[0]!.onopen!();
    h.sockets[0]!.emit(envelope("overlap", 200));
    h.stream.flushEventBuffer();
    h.requests[0]!.resolve({
      events: [
        { envelope: envelope("old", 100) },
        { envelope: envelope("overlap", 200) },
      ],
    });
    await settle();
    expect(h.events.value.map((e) => e.eventId)).toEqual(["overlap", "old"]);
    expect(h.stream.evictedRecords.value).toBe(0);
  });
  test("pause freezes display while bounded reception continues, resume drains latest", async () => {
    const h = harness(3);
    h.isPaused.value = true;
    await nextTick();
    h.sockets[0]!.emit(...[1, 2, 3, 4, 5].map((n) => envelope(String(n), n)));
    h.stream.flushEventBuffer();
    expect(h.events.value).toHaveLength(0);
    expect(h.stream.queuedRecords.value).toBe(3);
    expect(h.stream.droppedRecords.value).toBe(2);
    h.isPaused.value = false;
    await nextTick();
    expect(h.events.value.map((e) => e.eventId)).toEqual(["5", "4", "3"]);
    expect(h.stream.queuedRecords.value).toBe(0);
    expect(h.stream.streamNotice.value).toContain("缓存累计淘汰 2");
  });
  test("capacity shrink bounds paused queue and rendered window separately", async () => {
    const h = harness();
    h.sockets[0]!.emit(...[1, 2, 3, 4].map((n) => envelope(String(n), n)));
    h.stream.flushEventBuffer();
    h.isPaused.value = true;
    h.sockets[0]!.emit(...[5, 6, 7, 8].map((n) => envelope(String(n), n)));
    h.maxEvents.value = 2;
    await nextTick();
    expect(h.events.value).toHaveLength(2);
    expect(h.stream.queuedRecords.value).toBe(2);
    expect(h.stream.droppedRecords.value).toBe(2);
    expect(h.stream.evictedRecords.value).toBe(2);
  });
  test("reconnect refetches, deduplicates and keeps an honest gap warning", async () => {
    const h = harness();
    h.sockets[0]!.onopen!();
    h.requests[0]!.resolve({ events: [{ envelope: envelope("old", 100) }] });
    await settle();
    h.sockets[0]!.onclose!();
    h.stream.connectWebSocket();
    h.sockets[1]!.onopen!();
    h.sockets[1]!.emit(envelope("new", 300));
    h.requests[1]!.resolve({
      events: ["old", "missed", "new"].map((id, i) => ({
        envelope: envelope(id, (i + 1) * 100),
      })),
    });
    await settle();
    expect(h.events.value.map((e) => e.eventId)).toEqual([
      "new",
      "missed",
      "old",
    ]);
    expect(h.stream.streamNotice.value).toContain("无法保证间隙完整");
  });
  test("stale snapshot and clear responses cannot cross session/cluster reset", async () => {
    const h = harness();
    h.sockets[0]!.onopen!();
    const clearing = h.stream.clearEvents();
    h.stream.stopStream();
    expect(h.requests[0]!.signal.aborted).toBe(true);
    h.stream.startStream();
    h.sockets[1]!.emit(envelope("new-session"));
    h.stream.flushEventBuffer();
    h.requests[0]!.resolve({ events: [{ envelope: envelope("stale") }] });
    h.clearRequest.resolve({});
    await clearing;
    await settle();
    expect(h.events.value.map((e) => e.eventId)).toEqual(["new-session"]);
  });
  test("history failure preserves live events and exposes retry; successful retry deduplicates", async () => {
    const h = harness();
    h.sockets[0]!.onopen!();
    h.sockets[0]!.emit(envelope("live"));
    h.stream.flushEventBuffer();
    h.requests[0]!.reject(new Error("offline"));
    await settle();
    expect(h.stream.recoveryState.value).toBe("error");
    expect(h.events.value).toHaveLength(1);
    const retry = h.stream.loadRecentEvents();
    h.requests[1]!.resolve({ events: [{ envelope: envelope("live") }] });
    await retry;
    expect(h.stream.recoveryState.value).toBe("ready");
    expect(h.events.value).toHaveLength(1);
  });
  test("failed clear retains local evidence and pending events", async () => {
    const h = harness();
    h.sockets[0]!.emit(envelope("shown"));
    h.stream.flushEventBuffer();
    h.isPaused.value = true;
    h.sockets[0]!.emit(envelope("pending"));
    const clearing = h.stream.clearEvents();
    h.clearRequest.reject(new Error("forbidden"));
    await clearing;
    expect(h.events.value).toHaveLength(1);
    expect(h.stream.queuedRecords.value).toBe(1);
    expect(h.notices[0]).toContain("retained");
  });
  test("successful clear invalidates an in-flight snapshot", async () => {
    const h = harness();
    h.sockets[0]!.onopen!();
    h.sockets[0]!.emit(envelope("old"));
    h.stream.flushEventBuffer();
    const clearing = h.stream.clearEvents();
    h.clearRequest.resolve({});
    await clearing;
    h.requests[0]!.resolve({ events: [{ envelope: envelope("stale") }] });
    await settle();
    expect(h.events.value).toEqual([]);
    expect(h.requests[0]!.signal.aborted).toBe(true);
  });
});
