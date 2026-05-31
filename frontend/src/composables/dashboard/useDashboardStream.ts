import { ref, type Ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import { pb } from "../../pb/tracker_pb.js";
import { buildWebSocketUrl, fetchProto } from "../../utils/requestContext";
import {
  decodeIncomingEvents,
  buildAgentEvent,
  normalizeHistoryRecord,
} from "./dashboardHelpers";
import type { AgentEvent } from "./dashboardConstants";

export type DashboardStreamDeps = {
  events: Ref<AgentEvent[]>;
  isConnected: Ref<boolean>;
  isPaused: Ref<boolean>;
  maxEvents: Ref<number>;
  getFilteredEvents: () => AgentEvent[];
};

const EVENT_BATCH_WINDOW_MS = 80;
const HISTORY_LOAD_LIMIT = 200;
const HISTORY_LOAD_BATCH_SIZE = 24;
const HISTORY_LOAD_BATCH_DELAY_MS = 24;

export function useDashboardStream(deps: DashboardStreamDeps) {
  // ── Internal mutable state ──
  let ws: WebSocket | null = null;
  let reconnectTimer: number | null = null;
  let shouldReconnect = true;
  const eventBuffer: AgentEvent[] = [];
  let flushTimer: number | null = null;
  let historyLoadTimer: number | null = null;
  let historyLoadToken = 0;
  const pendingLiveEvents: AgentEvent[] = [];
  const historyLoaded = ref(false);

  // ── Event buffer management ──

  const markRecentRowsRef = ref<(keys: string[]) => void>(() => {});

  const flushEventBuffer = () => {
    if (eventBuffer.length === 0) return;

    const bufferedEvents = [...eventBuffer];
    const newEvents = [...bufferedEvents.reverse(), ...deps.events.value];
    if (newEvents.length > deps.maxEvents.value) {
      newEvents.length = deps.maxEvents.value;
    }
    deps.events.value = newEvents;
    eventBuffer.length = 0;

    markRecentRowsRef.value(
      newEvents.slice(0, bufferedEvents.length).map((event) => event.key),
    );
  };

  const scheduleEventBufferFlush = () => {
    if (flushTimer !== null) return;
    flushTimer = window.setTimeout(() => {
      flushTimer = null;
      flushEventBuffer();
    }, EVENT_BATCH_WINDOW_MS);
  };

  // ── History loading ──

  const clearHistoryLoadTimer = () => {
    if (historyLoadTimer !== null) {
      window.clearTimeout(historyLoadTimer);
      historyLoadTimer = null;
    }
  };

  const clearPendingLiveEvents = () => {
    pendingLiveEvents.length = 0;
  };

  const flushPendingLiveEvents = () => {
    if (pendingLiveEvents.length === 0) {
      return;
    }
    eventBuffer.push(...pendingLiveEvents);
    clearPendingLiveEvents();
    flushEventBuffer();
  };

  const animateHistoryRecords = (records: AgentEvent[], token: number) =>
    new Promise<void>((resolve) => {
      if (records.length === 0 || token !== historyLoadToken) {
        resolve();
        return;
      }

      let index = 0;
      const pump = () => {
        if (token !== historyLoadToken) {
          resolve();
          return;
        }

        const chunk = records.slice(index, index + HISTORY_LOAD_BATCH_SIZE);
        if (chunk.length === 0) {
          resolve();
          return;
        }

        eventBuffer.push(...chunk);
        flushEventBuffer();
        index += chunk.length;

        if (index < records.length) {
          historyLoadTimer = window.setTimeout(
            pump,
            HISTORY_LOAD_BATCH_DELAY_MS,
          );
          return;
        }

        historyLoadTimer = null;
        resolve();
      };

      clearHistoryLoadTimer();
      pump();
    });

  const loadRecentEvents = async () => {
    const token = ++historyLoadToken;
    historyLoaded.value = false;
    clearPendingLiveEvents();
    clearHistoryLoadTimer();

    try {
      const response = await fetchProto(
        `/events/recent?limit=${HISTORY_LOAD_LIMIT}`,
        pb.EventHistoryResponse.decode,
      );
      if (token !== historyLoadToken) {
        return;
      }

      const rawEvents = ((response as any).events ??
        (response as any).Events ??
        []) as any[];
      const records = rawEvents
        .map((record) => normalizeHistoryRecord(record))
        .filter((record): record is AgentEvent => record !== null);

      await animateHistoryRecords(records, token);
    } catch (err) {
      if (token === historyLoadToken) {
        console.error("Failed to load recent dashboard events", err);
      }
    } finally {
      if (token === historyLoadToken) {
        historyLoaded.value = true;
        flushPendingLiveEvents();
        clearHistoryLoadTimer();
      }
    }
  };

  // ── WebSocket connection ──

  const connectWebSocket = () => {
    if (!shouldReconnect) return;
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.close();
    }
    const socket = new WebSocket(buildWebSocketUrl("/ws"));
    ws = socket;
    socket.binaryType = "arraybuffer";

    socket.onopen = () => {
      if (ws !== socket) return;
      deps.isConnected.value = true;
    };

    socket.onmessage = (ev) => {
      if (ws !== socket) return;
      if (deps.isPaused.value) return;
      try {
        const incomingEvents = decodeIncomingEvents(new Uint8Array(ev.data));
        const normalizedEvents = incomingEvents.map((data) =>
          buildAgentEvent(data as Record<string, unknown>, Date.now()),
        );
        if (!historyLoaded.value) {
          pendingLiveEvents.push(...normalizedEvents);
        } else {
          eventBuffer.push(...normalizedEvents);
          scheduleEventBufferFlush();
        }
      } catch (e) {
        console.error("Failed to parse message", e);
      }
    };

    socket.onclose = () => {
      if (ws !== socket) return;
      deps.isConnected.value = false;
      ws = null;
      if (!shouldReconnect) return;
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
      }
      reconnectTimer = window.setTimeout(() => {
        connectWebSocket();
      }, 3000);
    };
  };

  // ── Clear and export ──

  const clearEvents = async () => {
    try {
      await axios.post("/data/clear-events-memory");
      deps.events.value = [];
      eventBuffer.length = 0;
      clearPendingLiveEvents();
      clearHistoryLoadTimer();
      historyLoadToken += 1;
      historyLoaded.value = true;
      if (flushTimer !== null) {
        window.clearTimeout(flushTimer);
        flushTimer = null;
      }
      message.success("Event buffer cleared on backend");
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to clear events");
      deps.events.value = [];
      eventBuffer.length = 0;
      clearPendingLiveEvents();
      clearHistoryLoadTimer();
      historyLoadToken += 1;
      historyLoaded.value = true;
      if (flushTimer !== null) {
        window.clearTimeout(flushTimer);
        flushTimer = null;
      }
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
      message.success("Events exported as CSV");
    } catch (err) {
      message.error("Failed to export CSV");
    }
  };

  // ── Lifecycle helpers ──

  const startStream = () => {
    shouldReconnect = true;
    connectWebSocket();
    void loadRecentEvents();
  };

  const resetStreamState = () => {
    eventBuffer.length = 0;
    clearHistoryLoadTimer();
    clearPendingLiveEvents();
    historyLoadToken += 1;
    historyLoaded.value = false;
    if (flushTimer !== null) {
      window.clearTimeout(flushTimer);
      flushTimer = null;
    }
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.close();
      ws = null;
    }
  };

  const stopStream = () => {
    shouldReconnect = false;
    resetStreamState();
  };

  return {
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
