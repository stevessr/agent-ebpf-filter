import { onUnmounted, ref, type Ref } from "vue";
import axios from "axios";

import { pb } from "../../pb/tracker_pb.js";
import { buildWebSocketUrl } from "../../utils/requestContext";
import {
  eventTypeLabelMap,
  type AgentEvent,
} from "../dashboard/dashboardConstants";

const HISTORY_LIMIT = 240;
const MAX_SUMMARIES = 800;
const FLUSH_WINDOW_MS = 100;

const text = (value: unknown) =>
  typeof value === "string" ? value : value == null ? "" : String(value);

const numberValue = (value: unknown) => {
  const numeric = Number(value ?? 0);
  return Number.isFinite(numeric) ? numeric : 0;
};

function envelopeTimestampMs(envelope: any) {
  const ingest = numberValue(envelope.ingestTimestampNs);
  const capture = numberValue(envelope.captureTimestampNs);
  const timestamp = numberValue(envelope.timestampNs);
  const ns = ingest || capture || timestamp;
  return ns > 0 ? Math.floor(ns / 1_000_000) : Date.now();
}

function summaryFromEnvelope(envelope: any): AgentEvent | null {
  const eventID = text(envelope.eventId).trim();
  if (!eventID) return null;
  const legacy = envelope.legacyEvent || {};
  const eventType = numberValue(envelope.eventType ?? legacy.eventType);
  const receivedAtMs = envelopeTimestampMs(envelope);
  return {
    key: eventID,
    eventId: eventID,
    pid: numberValue(envelope.pid || legacy.pid),
    ppid: numberValue(envelope.ppid || legacy.ppid),
    uid: numberValue(envelope.uid || legacy.uid),
    type:
      text(legacy.type) ||
      eventTypeLabelMap[eventType] ||
      text(envelope.eventType) ||
      "event",
    eventType,
    tag: text(legacy.tag) || "Unknown",
    comm: text(envelope.comm || legacy.comm),
    path: text(legacy.path),
    extraPath: text(legacy.extraPath),
    netDirection: text(legacy.netDirection),
    netEndpoint: text(legacy.netEndpoint),
    netBytes: numberValue(legacy.netBytes),
    domain: text(legacy.domain),
    decision: text(envelope.policyDecision || legacy.decision),
    riskScore: numberValue(envelope.riskScore || legacy.riskScore),
    agentRunId: text(envelope.agentRunId || legacy.agentRunId),
    conversationId: text(envelope.conversationId || legacy.conversationId),
    turnId: text(envelope.turnId || legacy.turnId),
    toolCallId: text(envelope.toolCallId || legacy.toolCallId),
    toolName: text(envelope.toolName || legacy.toolName),
    traceId: text(envelope.traceId || legacy.traceId),
    spanId: text(envelope.spanId || legacy.spanId),
    time: new Date(receivedAtMs).toISOString(),
    receivedAtMs,
  };
}

function normalizeSummary(value: any): AgentEvent | null {
  const eventID = text(value?.eventId || value?.key).trim();
  if (!eventID) return null;
  const receivedAtMs =
    numberValue(value?.receivedAtMs) ||
    Date.parse(text(value?.time)) ||
    Date.now();
  return {
    key: eventID,
    eventId: eventID,
    pid: numberValue(value?.pid),
    ppid: numberValue(value?.ppid),
    uid: numberValue(value?.uid),
    type: text(value?.type) || "event",
    eventType: numberValue(value?.eventType),
    tag: text(value?.tag) || "Unknown",
    comm: text(value?.comm),
    path: text(value?.path),
    extraPath: text(value?.extraPath),
    netDirection: text(value?.netDirection),
    netEndpoint: text(value?.netEndpoint),
    netBytes: numberValue(value?.netBytes),
    domain: text(value?.domain),
    decision: text(value?.decision),
    riskScore: numberValue(value?.riskScore),
    agentRunId: text(value?.agentRunId),
    conversationId: text(value?.conversationId),
    turnId: text(value?.turnId),
    toolCallId: text(value?.toolCallId),
    toolName: text(value?.toolName),
    traceId: text(value?.traceId),
    spanId: text(value?.spanId),
    time: new Date(receivedAtMs).toISOString(),
    receivedAtMs,
  };
}

export function useRenewEventFeed(
  isPaused: Ref<boolean>,
  maxSummaries = MAX_SUMMARIES,
) {
  const events = ref<AgentEvent[]>([]);
  const isConnected = ref(false);
  const detailLoading = ref(false);
  const selectedEventDetail = ref<Record<string, unknown> | null>(null);
  const selectedEventID = ref("");

  let ws: WebSocket | null = null;
  let reconnectTimer: number | null = null;
  let flushTimer: number | null = null;
  let shouldReconnect = true;
  const buffer: AgentEvent[] = [];

  const mergeSummaries = (incoming: AgentEvent[]) => {
    if (incoming.length === 0) return;
    const byID = new Map<string, AgentEvent>();
    for (const event of events.value) byID.set(event.key, event);
    for (const event of incoming) byID.set(event.key, event);
    events.value = [...byID.values()]
      .sort((a, b) => (b.receivedAtMs || 0) - (a.receivedAtMs || 0))
      .slice(0, maxSummaries);
  };

  const flush = () => {
    flushTimer = null;
    if (buffer.length === 0) return;
    const incoming = buffer.splice(0, buffer.length);
    mergeSummaries(incoming);
  };

  const scheduleFlush = () => {
    if (flushTimer !== null) return;
    flushTimer = window.setTimeout(flush, FLUSH_WINDOW_MS);
  };

  const loadHistory = async () => {
    try {
      const response = await axios.get("/events/summaries", {
        params: { limit: HISTORY_LIMIT },
      });
      const summaries = Array.isArray(response.data?.events)
        ? response.data.events
            .map(normalizeSummary)
            .filter((event: AgentEvent | null): event is AgentEvent => event !== null)
        : [];
      mergeSummaries(summaries);
    } catch (error) {
      console.error("Renew: failed to load compact event summaries", error);
    }
  };

  const connect = () => {
    if (!shouldReconnect) return;
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.close();
    }
    const socket = new WebSocket(buildWebSocketUrl("/ws/envelopes"));
    ws = socket;
    socket.binaryType = "arraybuffer";

    socket.onopen = () => {
      if (ws !== socket) return;
      isConnected.value = true;
    };
    socket.onmessage = (message) => {
      if (ws !== socket || isPaused.value) return;
      try {
        const batch = pb.EventEnvelopeBatch.decode(
          new Uint8Array(message.data),
        );
        for (const envelope of batch.envelopes || []) {
          const summary = summaryFromEnvelope(envelope);
          if (summary) buffer.push(summary);
        }
        scheduleFlush();
      } catch (error) {
        console.error("Renew: failed to decode compact envelope stream", error);
      }
    };
    socket.onclose = () => {
      if (ws !== socket) return;
      ws = null;
      isConnected.value = false;
      if (!shouldReconnect) return;
      reconnectTimer = window.setTimeout(connect, 3000);
    };
  };

  const start = () => {
    shouldReconnect = true;
    void loadHistory();
    connect();
  };

  const stop = () => {
    shouldReconnect = false;
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (flushTimer !== null) {
      window.clearTimeout(flushTimer);
      flushTimer = null;
    }
    buffer.length = 0;
    if (ws) {
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.close();
      ws = null;
    }
    isConnected.value = false;
  };

  const loadEventDetail = async (eventID: string) => {
    const normalized = eventID.trim();
    if (!normalized) return;
    detailLoading.value = true;
    selectedEventID.value = normalized;
    try {
      const response = await axios.get(
        `/events/detail/${encodeURIComponent(normalized)}`,
      );
      if (selectedEventID.value === normalized) {
        selectedEventDetail.value = response.data;
      }
    } catch (error) {
      if (selectedEventID.value === normalized) {
        selectedEventDetail.value = {
          error: "无法从后端读取完整事件",
          eventId: normalized,
        };
      }
      console.error("Renew: failed to load full event", error);
    } finally {
      if (selectedEventID.value === normalized) {
        detailLoading.value = false;
      }
    }
  };

  const closeEventDetail = () => {
    selectedEventID.value = "";
    selectedEventDetail.value = null;
    detailLoading.value = false;
  };

  onUnmounted(stop);

  return {
    events,
    isConnected,
    detailLoading,
    selectedEventDetail,
    selectedEventID,
    start,
    stop,
    loadHistory,
    loadEventDetail,
    closeEventDetail,
  };
}
