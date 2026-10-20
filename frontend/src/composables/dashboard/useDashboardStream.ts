import { computed, ref, watch, type Ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import { pb } from "../../pb/tracker_pb.js";
import { buildWebSocketUrl, fetchProto } from "../../utils/requestContext";
import { normalizeEnvelope, normalizeHistoryRecord } from "./dashboardHelpers";
import { appendBounded, mergeDashboardRecords } from "./dashboardBuffer";
import type { AgentEvent } from "./dashboardConstants";

export type DashboardStreamDeps = {
  events: Ref<AgentEvent[]>;
  isConnected: Ref<boolean>;
  isPaused: Ref<boolean>;
  maxEvents: Ref<number>;
  getFilteredEvents: () => AgentEvent[];
};

// Injectable transport keeps lifecycle tests independent of a browser/backend.
export type DashboardStreamTransport = {
  open: () => WebSocket;
  history: (signal: AbortSignal) => Promise<pb.IEventHistoryResponse>;
  clear: () => Promise<unknown>;
  notify: (kind: "success" | "error", text: string) => void;
};
const HISTORY_LOAD_LIMIT = 1000; // Server-side /events/recent maximum, not a cursor.
const EVENT_BATCH_WINDOW_MS = 80;

export function useDashboardStream(
  deps: DashboardStreamDeps,
  transport: DashboardStreamTransport = {
    open: () => new WebSocket(buildWebSocketUrl("/ws/envelopes")),
    history: (signal) =>
      fetchProto(
        `/events/recent?limit=${HISTORY_LOAD_LIMIT}`,
        pb.EventHistoryResponse.decode,
        signal,
      ),
    clear: () => axios.post("/data/clear-events-memory"),
    notify: (kind, text) => message[kind](text),
  },
) {
  let ws: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let flushTimer: ReturnType<typeof setTimeout> | null = null;
  let historyController: AbortController | null = null;
  let historyLoadToken = 0;
  let lifecycle = 0;
  let shouldReconnect = false;
  const eventBuffer: AgentEvent[] = [];
  let displayedIds = new Set<string>();
  const indexDisplayedIds = () => {
    displayedIds = new Set(
      deps.events.value.flatMap((event) =>
        event.eventId ? [event.eventId] : [],
      ),
    );
  };
  const historyLoaded = ref(false);
  const recoveryState = ref<"idle" | "loading" | "ready" | "error">("idle");
  const continuityGap = ref(false);
  const queuedRecords = ref(0);
  const droppedRecords = ref(0);
  const evictedRecords = ref(0);
  const markRecentRowsRef = ref<(keys: string[]) => void>(() => {});
  const streamNotice = computed(() => {
    const parts: string[] = [];
    if (!deps.isConnected.value) parts.push("实时流未连接，正在等待连接。");
    if (deps.isPaused.value)
      parts.push(
        `显示已暂停（不主动停止接收）；待显示 ${queuedRecords.value} 条，缓存上限 ${deps.maxEvents.value} 条。`,
      );
    if (recoveryState.value === "loading")
      parts.push("正在补取最近记录并按事件 ID 去重。");
    if (recoveryState.value === "error")
      parts.push("最近记录补取失败；已保留本地事件，可重试。");
    if (continuityGap.value)
      parts.push(
        "本次会话发生过断线或解码失败；每次仅补取最近 1000 条，无法保证间隙完整，请核对持久化审计记录。",
      );
    if (droppedRecords.value)
      parts.push(
        `待显示缓存累计淘汰 ${droppedRecords.value} 条（不代表后端丢失）。`,
      );
    return parts.join(" ");
  });

  const enqueue = (records: AgentEvent[]) => {
    const queuedIds = new Set(
      eventBuffer.flatMap((event) => (event.eventId ? [event.eventId] : [])),
    );
    const unique = records.filter((event) => {
      if (!event.eventId) return true;
      if (displayedIds.has(event.eventId) || queuedIds.has(event.eventId))
        return false;
      queuedIds.add(event.eventId);
      return true;
    });
    droppedRecords.value += appendBounded(
      eventBuffer,
      unique,
      deps.maxEvents.value,
    );
    queuedRecords.value = eventBuffer.length;
  };
  const flushEventBuffer = () => {
    if (deps.isPaused.value || eventBuffer.length === 0) return;
    const oldKeys = new Set(deps.events.value.map((event) => event.key));
    const merged = mergeDashboardRecords(
      deps.events.value,
      eventBuffer,
      deps.maxEvents.value,
    );
    evictedRecords.value += merged.evicted;
    deps.events.value = merged.records;
    indexDisplayedIds();
    eventBuffer.length = 0;
    queuedRecords.value = 0;
    markRecentRowsRef.value(
      merged.records
        .filter((event) => !oldKeys.has(event.key))
        .map((event) => event.key),
    );
  };
  const scheduleEventBufferFlush = () => {
    if (deps.isPaused.value || flushTimer !== null) return;
    flushTimer = setTimeout(() => {
      flushTimer = null;
      flushEventBuffer();
    }, EVENT_BATCH_WINDOW_MS);
  };
  watch(deps.isPaused, (paused) => {
    if (!paused) flushEventBuffer();
  });
  watch(deps.maxEvents, (capacity) => {
    droppedRecords.value += appendBounded(eventBuffer, [], capacity);
    queuedRecords.value = eventBuffer.length;
    if (deps.events.value.length > capacity) {
      evictedRecords.value += deps.events.value.length - capacity;
      deps.events.value = deps.events.value.slice(0, capacity);
      indexDisplayedIds();
    }
  });

  const cancelHistory = () => {
    historyLoadToken += 1;
    historyController?.abort();
    historyController = null;
  };
  const loadRecentEvents = async () => {
    if (!shouldReconnect) return;
    cancelHistory();
    const token = historyLoadToken;
    const controller = new AbortController();
    historyController = controller;
    const timeout = setTimeout(() => controller.abort(), 15_000);
    recoveryState.value = "loading";
    historyLoaded.value = false;
    try {
      const response = await transport.history(controller.signal);
      if (token !== historyLoadToken) return;
      const records = (response.events ?? [])
        .map(normalizeHistoryRecord)
        .filter(
          (record): record is AgentEvent =>
            record !== null &&
            (!record.eventId || !displayedIds.has(record.eventId)),
        );
      // Merge before truncation so the history/live overlap cannot evict unique
      // queued events. Keep the queue oldest-first for bounded live appends.
      const merged = mergeDashboardRecords(
        eventBuffer,
        records,
        deps.maxEvents.value,
      );
      droppedRecords.value += merged.evicted;
      eventBuffer.length = 0;
      for (const record of merged.records.reverse()) eventBuffer.push(record);
      queuedRecords.value = eventBuffer.length;
      recoveryState.value = "ready";
      flushEventBuffer();
    } catch {
      if (token === historyLoadToken) recoveryState.value = "error";
    } finally {
      clearTimeout(timeout);
      if (token === historyLoadToken) {
        historyController = null;
        historyLoaded.value = true;
      }
    }
  };

  const connectWebSocket = () => {
    if (!shouldReconnect) return;
    if (reconnectTimer !== null) clearTimeout(reconnectTimer);
    reconnectTimer = null;
    if (ws) {
      ws.onopen = ws.onmessage = ws.onclose = null;
      ws.close();
    }
    const socket = transport.open();
    ws = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      if (ws !== socket) return;
      deps.isConnected.value = true;
      // Subscribe first, then snapshot. Stable IDs remove overlap without a
      // blind window between the history request and WebSocket subscription.
      void loadRecentEvents();
    };
    socket.onmessage = (ev) => {
      if (ws !== socket) return;
      try {
        const envelopes = pb.EventEnvelopeBatch.decode(
          new Uint8Array(ev.data),
        ).envelopes;
        const records = envelopes
          .map((envelope) => normalizeEnvelope(envelope))
          .filter((record): record is AgentEvent => record !== null);
        enqueue(records);
        scheduleEventBufferFlush();
      } catch {
        continuityGap.value = true;
      }
    };
    socket.onclose = () => {
      if (ws !== socket) return;
      deps.isConnected.value = false;
      continuityGap.value = true;
      ws = null;
      cancelHistory();
      recoveryState.value = "idle";
      if (!shouldReconnect) return;
      reconnectTimer = setTimeout(connectWebSocket, 3000);
    };
  };

  const clearEvents = async () => {
    const session = lifecycle;
    try {
      await transport.clear();
      if (session !== lifecycle) return;
      cancelHistory();
      deps.events.value = [];
      displayedIds.clear();
      eventBuffer.length = 0;
      queuedRecords.value = 0;
      historyLoaded.value = true;
      recoveryState.value = "idle";
      transport.notify("success", "Event buffer cleared on backend");
    } catch {
      // A failed server operation must not silently destroy local evidence.
      if (session === lifecycle)
        transport.notify(
          "error",
          "Failed to clear events; local events retained",
        );
    }
  };

  const exportEvents = () => {
    try {
      const dataStr =
        "data:text/json;charset=utf-8," +
        encodeURIComponent(JSON.stringify(deps.events.value, null, 2));
      const downloadAnchorNode = document.createElement("a");
      downloadAnchorNode.setAttribute("href", dataStr);
      downloadAnchorNode.setAttribute(
        "download",
        `ebpf-events-${new Date().toISOString()}.json`,
      );
      document.body.appendChild(downloadAnchorNode);
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
      message.success("Events exported as JSON");
    } catch (err) {
      message.error("Failed to export events");
    }
  };

  const exportEventsCSV = () => {
    try {
      const headers = [
        "Time",
        "Tag",
        "PID",
        "PPID",
        "UID",
        "Command",
        "Event Type",
        "Path",
        "Net Direction",
        "Net Endpoint",
        "Net Bytes",
      ];
      const rows = deps
        .getFilteredEvents()
        .map((e) => [
          e.time,
          e.tag,
          e.pid,
          e.ppid,
          e.uid,
          e.comm,
          e.type,
          e.path,
          e.netDirection || "",
          e.netEndpoint || "",
          e.netBytes || 0,
        ]);
      const csvContent = [headers, ...rows]
        .map((r) =>
          r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","),
        )
        .join("\n");
      const blob = new Blob([csvContent], { type: "text/csv;charset=utf-8;" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.setAttribute("href", url);
      link.setAttribute(
        "download",
        `ebpf-events-${new Date().toISOString()}.csv`,
      );
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
      message.success("Events exported as CSV");
    } catch (err) {
      message.error("Failed to export CSV");
    }
  };

  // ── Lifecycle helpers ──
  const resetStreamState = () => {
    lifecycle += 1;
    indexDisplayedIds();
    cancelHistory();
    deps.isConnected.value = false;
    evictedRecords.value = droppedRecords.value = queuedRecords.value = 0;
    continuityGap.value = false;
    recoveryState.value = "idle";
    eventBuffer.length = 0;
    historyLoaded.value = false;
    if (flushTimer !== null) clearTimeout(flushTimer);
    if (reconnectTimer !== null) clearTimeout(reconnectTimer);
    flushTimer = reconnectTimer = null;
    if (ws) {
      ws.onopen = ws.onmessage = ws.onclose = null;
      ws.close();
      ws = null;
    }
  };
  const startStream = () => {
    resetStreamState();
    shouldReconnect = true;
    connectWebSocket();
  };
  const stopStream = () => {
    shouldReconnect = false;
    resetStreamState();
  };
  return {
    evictedRecords,
    droppedRecords,
    queuedRecords,
    streamNotice,
    recoveryState,
    historyLoaded,
    startStream,
    stopStream,
    resetStreamState,
    clearEvents,
    exportEvents,
    exportEventsCSV,
    connectWebSocket,
    loadRecentEvents,
    flushEventBuffer,
    markRecentRowsRef,
  };
}
