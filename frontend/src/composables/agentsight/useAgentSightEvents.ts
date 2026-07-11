import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import { pb } from "../../pb/tracker_pb.js";
import { buildWebSocketUrl } from "../../utils/requestContext";
import {
  normalizeAgentSightEvents,
  processAgentSightEvents,
  type AgentSightEvent,
  type AgentSightEventRecord,
} from "../../utils/agentsight";

export interface AgentSightFilters {
  source: string;
  eventType: string;
  pid: string;
  comm: string;
  traceId: string;
  redactionState: string;
  searchTerm: string;
}

const AGENTSIGHT_CACHE_KEY = "agent-ebpf.agentsight.importedRecords";
const AGENTSIGHT_SAMPLE_TRACE_URL = "/agentsight-sample-trace.jsonl";
const AGENTSIGHT_SYSTEM_TOP_PROCESSES = 40;
const AGENTSIGHT_DEFAULT_LIMIT = 500;
const AGENTSIGHT_UNLIMITED_LIMIT = 0;
export const AGENTSIGHT_IMPORT_MAX_BYTES = 16 * 1024 * 1024;
export const AGENTSIGHT_IMPORT_MAX_RECORDS = 10000;

export interface AgentSightImportResult {
  imported: number;
  retained: number;
  truncated: boolean;
}

const defaultFilters = (): AgentSightFilters => ({
  source: "",
  eventType: "",
  pid: "",
  comm: "",
  traceId: "",
  redactionState: "",
  searchTerm: "",
});

export interface UseAgentSightEventsOptions {
  initialPid?: string | number;
  initialComm?: string;
}

function loadCachedAgentSightRecords(): AgentSightEventRecord[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(AGENTSIGHT_CACHE_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    if (!Array.isArray(parsed)) return [];
    const retained = retainAgentSightRecords(parsed);
    if (retained.length !== parsed.length) {
      window.localStorage.setItem(
        AGENTSIGHT_CACHE_KEY,
        JSON.stringify(retained),
      );
    }
    return retained;
  } catch {
    return [];
  }
}

function cacheAgentSightRecords(records: AgentSightEventRecord[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(
      AGENTSIGHT_CACHE_KEY,
      JSON.stringify(retainAgentSightRecords(records)),
    );
  } catch {
    window.localStorage.removeItem(AGENTSIGHT_CACHE_KEY);
  }
}

function retainAgentSightRecords<T>(records: T[]): T[] {
  return records.slice(0, AGENTSIGHT_IMPORT_MAX_RECORDS);
}

function agentSightTextByteLength(text: string): number {
  if (typeof Blob !== "undefined") return new Blob([text]).size;
  return new TextEncoder().encode(text).byteLength;
}

function normalizeImportedAgentSightRecord(
  value: any,
  index: number,
): AgentSightEventRecord | null {
  if (!value || typeof value !== "object") return null;
  if (
    value.Event ||
    value.Envelope ||
    value.Timestamp ||
    value.event ||
    value.envelope ||
    value.timestamp ||
    value.source
  )
    return value as AgentSightEventRecord;
  return { Event: value, Timestamp: Date.now() + index };
}

export function parseAgentSightRecordsText(
  text: string,
): AgentSightEventRecord[] {
  return parseAgentSightRecordsTextWithLimit(text).records;
}

export function parseAgentSightRecordsTextWithLimit(
  text: string,
  maxRecords = AGENTSIGHT_IMPORT_MAX_RECORDS,
): { records: AgentSightEventRecord[]; truncated: boolean } {
  const trimmed = text.trim();
  if (!trimmed) return { records: [], truncated: false };
  try {
    const parsed = JSON.parse(trimmed);
    const items = Array.isArray(parsed)
      ? parsed
      : parsed.events || parsed.records || [parsed];
    const truncated = items.length > maxRecords;
    const records = items
      .slice(0, maxRecords)
      .map(normalizeImportedAgentSightRecord)
      .filter(Boolean) as AgentSightEventRecord[];
    return { records, truncated };
  } catch {
    const records: AgentSightEventRecord[] = [];
    let truncated = false;
    for (const line of trimmed.split("\n")) {
      const normalizedLine = line.trim();
      if (!normalizedLine) continue;
      if (records.length >= maxRecords) {
        truncated = true;
        break;
      }
      try {
        const record = normalizeImportedAgentSightRecord(
          JSON.parse(normalizedLine),
          records.length,
        );
        if (record) records.push(record);
      } catch {
        // Preserve the existing tolerant JSONL import behavior.
      }
    }
    return { records, truncated };
  }
}

function tlsCaptureEventToRecord(
  event: Record<string, any>,
  index = 0,
): AgentSightEventRecord {
  const timestamp = event.timestamp || Date.now() + index;
  const source =
    event.type === "sse_message"
      ? "sse_processor"
      : event.type === "http_request" || event.type === "http_response"
        ? "http_parser"
        : "ssl";
  return {
    id: `tls-${event.timestamp || index}-${event.pid || 0}-${event.method || event.status || event.type || "event"}`,
    timestamp,
    source,
    pid: event.pid,
    ppid: event.ppid,
    comm: event.comm,
    data: event,
  };
}

function numberValue(value: any, fallback = 0): number {
  if (value === undefined || value === null) return fallback;
  if (typeof value === "number")
    return Number.isFinite(value) ? value : fallback;
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }
  if (typeof value === "object") {
    if (typeof value.low === "number") return value.low;
    if (typeof value.toNumber === "function") return value.toNumber();
  }
  return fallback;
}

function systemStatsToRecords(
  stats: Record<string, any>,
  timestamp = Date.now(),
): AgentSightEventRecord[] {
  const memoryTotal = numberValue(stats.memory?.total, 0);
  const processes = Array.isArray(stats.processes) ? stats.processes : [];
  const ranked = processes
    .slice()
    .sort(
      (a: any, b: any) =>
        Math.max(numberValue(b.cpu), numberValue(b.mem)) -
        Math.max(numberValue(a.cpu), numberValue(a.mem)),
    )
    .slice(0, AGENTSIGHT_SYSTEM_TOP_PROCESSES);

  const records: AgentSightEventRecord[] = ranked.map(
    (process: any, index: number) => {
      const memPercent = numberValue(process.mem, 0);
      const memoryBytes =
        memoryTotal > 0 ? Math.round((memoryTotal * memPercent) / 100) : 0;
      const cpuPercent = numberValue(process.cpu, 0);
      const alert = cpuPercent >= 80 || memPercent >= 20;
      return {
        id: `system-${timestamp}-${process.pid || index}`,
        timestamp,
        source: "system",
        pid: numberValue(process.pid, 0),
        ppid: numberValue(process.ppid, 0),
        comm: process.name || process.comm || "process",
        data: {
          type: "system_metrics",
          pid: numberValue(process.pid, 0),
          comm: process.name || process.comm || "process",
          cpu: {
            percent: cpuPercent,
            cores: Array.isArray(stats.cpu?.cores) ? stats.cpu.cores.length : 0,
          },
          memory: {
            rss_mb: Math.round(memoryBytes / 1024 / 1024),
            rss_bytes: memoryBytes,
            percent: memPercent,
          },
          process: {
            state: "",
            user: process.user || "",
            cmdline: process.cmdline || "",
            create_time: process.createTime,
          },
          alert,
        },
      };
    },
  );

  if (stats.cpu || stats.memory) {
    records.unshift({
      id: `system-wide-${timestamp}`,
      timestamp,
      source: "system",
      pid: 0,
      ppid: 0,
      comm: "system",
      data: {
        type: "system_wide",
        cpu: {
          percent: numberValue(stats.cpu?.total, 0),
          cores: Array.isArray(stats.cpu?.cores) ? stats.cpu.cores.length : 0,
        },
        memory: {
          total_bytes: memoryTotal,
          used_bytes: numberValue(stats.memory?.used, 0),
          rss_bytes: numberValue(stats.memory?.used, 0),
          rss_mb: Math.round(numberValue(stats.memory?.used, 0) / 1024 / 1024),
          used_percent: numberValue(stats.memory?.percent, 0),
        },
        process: {
          children: processes.length,
        },
        alert:
          numberValue(stats.cpu?.total, 0) >= 80 ||
          numberValue(stats.memory?.percent, 0) >= 80,
      },
    });
  }

  return records;
}

function downloadText(filename: string, content: string, mime: string) {
  if (typeof window === "undefined") return;
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function csvCell(value: unknown) {
  const text = String(value ?? "");
  return `"${text.replaceAll('"', '""')}"`;
}

function applyAgentSightLimit<T>(items: T[], limit: number): T[] {
  return limit > AGENTSIGHT_UNLIMITED_LIMIT ? items.slice(0, limit) : items;
}

function agentSightLimitQuery(limit: number): string {
  return limit > AGENTSIGHT_UNLIMITED_LIMIT ? String(limit) : "all";
}

export function useAgentSightEvents(options?: UseAgentSightEventsOptions) {
  const liveRecords = ref<AgentSightEventRecord[]>([]);
  const tlsRecords = ref<AgentSightEventRecord[]>([]);
  const systemRecords = ref<AgentSightEventRecord[]>([]);
  const sampleRecords = ref<AgentSightEventRecord[]>([]);
  const importedRecords = ref<AgentSightEventRecord[]>(
    loadCachedAgentSightRecords(),
  );
  const loading = shallowRef(false);
  const tlsLoading = shallowRef(false);
  const sampleLoading = shallowRef(false);
  const limit = shallowRef(AGENTSIGHT_DEFAULT_LIMIT);
  const activeTab = shallowRef("flamegraph");
  const initialFilters = defaultFilters();
  if (options?.initialPid) {
    initialFilters.pid = String(options.initialPid);
  }
  if (options?.initialComm) {
    initialFilters.comm = options.initialComm;
  }
  const filters = ref<AgentSightFilters>(initialFilters);
  const isEnvelopeConnected = shallowRef(false);
  const isTLSConnected = shallowRef(false);
  const isSystemConnected = shallowRef(false);
  const sampleAttempted = shallowRef(false);
  // When paused, live feeds (envelope/TLS/system WebSockets + the periodic
  // refresh) keep their connections open but stop mutating the record buffers,
  // so the timeline and other views freeze for inspection. Manual Refresh still
  // works as an explicit override.
  const paused = shallowRef(false);

  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  let envelopeWS: WebSocket | null = null;
  let tlsWS: WebSocket | null = null;
  let systemWS: WebSocket | null = null;
  let envelopeReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let tlsReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let systemReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let shouldReconnect = true;
  let componentMounted = false;
  let tlsCaptureAvailable = true;
  let envelopeFetchController: AbortController | null = null;
  let tlsFetchController: AbortController | null = null;
  let envelopeFetchGeneration = 0;
  let tlsFetchGeneration = 0;

  const realRecords = computed(() => [
    ...importedRecords.value,
    ...systemRecords.value,
    ...tlsRecords.value,
    ...liveRecords.value,
  ]);
  const rawRecords = computed(() =>
    realRecords.value.length > 0 ? realRecords.value : sampleRecords.value,
  );
  const retainedEvents = computed<AgentSightEvent[]>(() =>
    normalizeAgentSightEvents(rawRecords.value).slice(
      0,
      AGENTSIGHT_IMPORT_MAX_RECORDS,
    ),
  );
  const events = computed<AgentSightEvent[]>(() =>
    applyAgentSightLimit(retainedEvents.value, limit.value),
  );
  const processedEvents = computed(() => processAgentSightEvents(events.value));

  const queryString = computed(() => {
    const params = new URLSearchParams();
    params.set("limit", agentSightLimitQuery(limit.value));
    if (filters.value.source) params.set("source", filters.value.source);
    if (filters.value.eventType)
      params.set("event_type", filters.value.eventType);
    if (filters.value.pid) params.set("pid", filters.value.pid);
    if (filters.value.comm) params.set("comm", filters.value.comm);
    if (filters.value.traceId) params.set("trace_id", filters.value.traceId);
    if (filters.value.redactionState)
      params.set("redaction_state", filters.value.redactionState);
    return params.toString();
  });

  const fetchEnvelopeEvents = async () => {
    envelopeFetchController?.abort();
    const controller = new AbortController();
    envelopeFetchController = controller;
    const generation = ++envelopeFetchGeneration;
    loading.value = true;
    try {
      const response = await axios.get(`/events/recent?${queryString.value}`, {
        signal: controller.signal,
      });
      if (!componentMounted || generation !== envelopeFetchGeneration) return;
      const live = Array.isArray(response.data?.events)
        ? response.data.events.slice().reverse()
        : [];
      liveRecords.value = retainAgentSightRecords(live);
    } catch (error: any) {
      if (controller.signal.aborted || generation !== envelopeFetchGeneration)
        return;
      message.error(
        error?.response?.data?.error ||
          "Failed to load AgentSight event envelopes",
      );
    } finally {
      if (generation === envelopeFetchGeneration) {
        loading.value = false;
        envelopeFetchController = null;
      }
    }
  };

  const fetchTLSEvents = async () => {
    tlsFetchController?.abort();
    const controller = new AbortController();
    tlsFetchController = controller;
    const generation = ++tlsFetchGeneration;
    tlsLoading.value = true;
    try {
      const response = await axios.get(
        `/tls-capture/recent?limit=${agentSightLimitQuery(limit.value)}`,
        { signal: controller.signal },
      );
      if (!componentMounted || generation !== tlsFetchGeneration) return;
      tlsCaptureAvailable = true;
      const tls = Array.isArray(response.data?.events)
        ? response.data.events.map(tlsCaptureEventToRecord)
        : [];
      tlsRecords.value = retainAgentSightRecords(tls);
      if (!tlsWS && shouldReconnect) connectTLSWebSocket();
    } catch (error: any) {
      if (controller.signal.aborted || generation !== tlsFetchGeneration) return;
      const status = error?.response?.status;
      if (status === 403 || status === 404) {
        tlsCaptureAvailable = false;
      } else if (status) {
        message.warning(
          error?.response?.data?.error ||
            "TLS capture history is not available",
        );
      }
      tlsRecords.value = [];
    } finally {
      if (generation === tlsFetchGeneration) {
        tlsLoading.value = false;
        tlsFetchController = null;
      }
    }
  };

  const fetchEvents = async () => {
    await Promise.all([fetchEnvelopeEvents(), fetchTLSEvents()]);
    if (componentMounted) await autoLoadSampleIfEmpty();
  };

  const mergeEnvelopeRecords = (incoming: Record<string, any>[]) => {
    if (paused.value) return;
    if (incoming.length === 0) return;
    const records = incoming.map((envelope) => ({
      Envelope: envelope,
      Timestamp:
        envelope.timestamp_ns || envelope.timestampNs
          ? Math.floor(
              Number(envelope.timestamp_ns || envelope.timestampNs) / 1_000_000,
            )
          : Date.now(),
    }));
    liveRecords.value = retainAgentSightRecords([
      ...records.reverse(),
      ...liveRecords.value,
    ]);
  };

  const disposeWebSocket = (socket: WebSocket | null) => {
    if (!socket) return;
    socket.onopen = null;
    socket.onmessage = null;
    socket.onclose = null;
    socket.onerror = null;
    if (
      socket.readyState === WebSocket.CONNECTING ||
      socket.readyState === WebSocket.OPEN
    ) {
      socket.close();
    }
  };

  const connectEnvelopeWebSocket = () => {
    if (!shouldReconnect) return;
    if (
      envelopeWS?.readyState === WebSocket.CONNECTING ||
      envelopeWS?.readyState === WebSocket.OPEN
    )
      return;
    disposeWebSocket(envelopeWS);
    if (envelopeReconnectTimer) {
      clearTimeout(envelopeReconnectTimer);
      envelopeReconnectTimer = null;
    }
    const socket = new WebSocket(buildWebSocketUrl("/ws/envelopes"));
    envelopeWS = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      if (envelopeWS !== socket) return;
      isEnvelopeConnected.value = true;
    };
    socket.onmessage = (event) => {
      if (envelopeWS !== socket) return;
      try {
        const batch = pb.EventEnvelopeBatch.decode(new Uint8Array(event.data));
        const envelopes = JSON.parse(JSON.stringify(batch.envelopes || []));
        mergeEnvelopeRecords(envelopes);
      } catch (error) {
        console.error("AgentSight envelope websocket parse error", error);
      }
    };
    socket.onclose = () => {
      if (envelopeWS !== socket) return;
      envelopeWS = null;
      isEnvelopeConnected.value = false;
      if (shouldReconnect) {
        if (envelopeReconnectTimer) clearTimeout(envelopeReconnectTimer);
        envelopeReconnectTimer = setTimeout(connectEnvelopeWebSocket, 3000);
      }
    };
    socket.onerror = () => {
      if (envelopeWS !== socket) return;
      isEnvelopeConnected.value = false;
    };
  };

  const connectTLSWebSocket = () => {
    if (!shouldReconnect || !tlsCaptureAvailable) return;
    if (
      tlsWS?.readyState === WebSocket.CONNECTING ||
      tlsWS?.readyState === WebSocket.OPEN
    )
      return;
    disposeWebSocket(tlsWS);
    if (tlsReconnectTimer) {
      clearTimeout(tlsReconnectTimer);
      tlsReconnectTimer = null;
    }
    const socket = new WebSocket(buildWebSocketUrl("/ws/tls-capture"));
    tlsWS = socket;
    socket.onopen = () => {
      if (tlsWS !== socket) return;
      isTLSConnected.value = true;
    };
    socket.onmessage = (event) => {
      if (tlsWS !== socket) return;
      if (paused.value) return;
      try {
        const payload = JSON.parse(String(event.data));
        tlsRecords.value = retainAgentSightRecords([
          tlsCaptureEventToRecord(payload),
          ...tlsRecords.value,
        ]);
      } catch (error) {
        console.error("AgentSight TLS websocket parse error", error);
      }
    };
    socket.onclose = () => {
      if (tlsWS !== socket) return;
      tlsWS = null;
      isTLSConnected.value = false;
      if (shouldReconnect && tlsCaptureAvailable) {
        if (tlsReconnectTimer) clearTimeout(tlsReconnectTimer);
        tlsReconnectTimer = setTimeout(connectTLSWebSocket, 3000);
      }
    };
    socket.onerror = () => {
      if (tlsWS !== socket) return;
      isTLSConnected.value = false;
    };
  };

  const connectSystemWebSocket = () => {
    if (!shouldReconnect) return;
    if (
      systemWS?.readyState === WebSocket.CONNECTING ||
      systemWS?.readyState === WebSocket.OPEN
    )
      return;
    disposeWebSocket(systemWS);
    if (systemReconnectTimer) {
      clearTimeout(systemReconnectTimer);
      systemReconnectTimer = null;
    }
    const socket = new WebSocket(
      buildWebSocketUrl("/ws/system", { interval: "2000" }),
    );
    systemWS = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      if (systemWS !== socket) return;
      isSystemConnected.value = true;
    };
    socket.onmessage = (event) => {
      if (systemWS !== socket) return;
      if (paused.value) return;
      try {
        const stats = pb.SystemStats.decode(new Uint8Array(event.data));
        const plain = JSON.parse(JSON.stringify(stats));
        const timestamp = Date.now();
        systemRecords.value = retainAgentSightRecords([
          ...systemStatsToRecords(plain, timestamp),
          ...systemRecords.value,
        ]);
      } catch (error) {
        console.error("AgentSight system websocket parse error", error);
      }
    };
    socket.onclose = () => {
      if (systemWS !== socket) return;
      systemWS = null;
      isSystemConnected.value = false;
      if (shouldReconnect) {
        if (systemReconnectTimer) clearTimeout(systemReconnectTimer);
        systemReconnectTimer = setTimeout(connectSystemWebSocket, 3000);
      }
    };
    socket.onerror = () => {
      if (systemWS !== socket) return;
      isSystemConnected.value = false;
    };
  };

  const importRecordsText = (text: string): AgentSightImportResult => {
    const byteLength = agentSightTextByteLength(text);
    if (byteLength > AGENTSIGHT_IMPORT_MAX_BYTES) {
      throw new RangeError(
        `AgentSight import exceeds ${AGENTSIGHT_IMPORT_MAX_BYTES} bytes`,
      );
    }
    const parsed = parseAgentSightRecordsTextWithLimit(text);
    const merged = [...parsed.records, ...importedRecords.value];
    importedRecords.value = retainAgentSightRecords(merged);
    cacheAgentSightRecords(importedRecords.value);
    return {
      imported: parsed.records.length,
      retained: importedRecords.value.length,
      truncated:
        parsed.truncated || merged.length > AGENTSIGHT_IMPORT_MAX_RECORDS,
    };
  };

  const loadSampleTrace = async () => {
    sampleLoading.value = true;
    sampleAttempted.value = true;
    try {
      const response = await axios.get(AGENTSIGHT_SAMPLE_TRACE_URL, {
        responseType: "text",
      });
      const parsed = parseAgentSightRecordsTextWithLimit(
        String(response.data || ""),
      );
      sampleRecords.value = parsed.records;
      return parsed.records.length;
    } catch {
      sampleRecords.value = [];
      return 0;
    } finally {
      sampleLoading.value = false;
    }
  };

  const autoLoadSampleIfEmpty = async () => {
    if (
      sampleAttempted.value ||
      realRecords.value.length > 0 ||
      sampleRecords.value.length > 0
    )
      return;
    await loadSampleTrace();
  };

  const clearImportedRecords = () => {
    importedRecords.value = [];
    cacheAgentSightRecords([]);
  };

  const clearAllRecords = () => {
    liveRecords.value = [];
    tlsRecords.value = [];
    systemRecords.value = [];
    sampleRecords.value = [];
    importedRecords.value = [];
    cacheAgentSightRecords([]);
  };

  const clearFilters = () => {
    filters.value = defaultFilters();
  };

  const togglePaused = () => {
    paused.value = !paused.value;
    // Resuming pulls a fresh snapshot so the views immediately reflect whatever
    // accumulated server-side while paused, instead of waiting for the next tick.
    if (!paused.value) void fetchEvents();
  };

  const eventTypeOptions = computed(() =>
    Array.from(new Set(events.value.map((event) => event.eventType)))
      .filter(Boolean)
      .sort()
      .map((value) => ({ label: value, value })),
  );
  const sourceOptions = computed(() =>
    Array.from(new Set(events.value.map((event) => event.source)))
      .filter(Boolean)
      .sort()
      .map((value) => ({ label: value, value })),
  );
  const redactionStateOptions = computed(() =>
    Array.from(new Set(events.value.map((event) => event.redactionState)))
      .filter(Boolean)
      .sort()
      .map((value) => ({ label: value, value })),
  );

  const visibleEvents = computed(() => {
    const f = filters.value;
    return events.value.filter((event) => {
      if (f.source && event.source !== f.source) return false;
      if (f.eventType && event.eventType !== f.eventType) return false;
      if (f.pid && String(event.pid) !== f.pid) return false;
      if (f.comm && !event.comm.toLowerCase().includes(f.comm.toLowerCase()))
        return false;
      if (f.traceId && !event.traceId.includes(f.traceId)) return false;
      if (f.redactionState && event.redactionState !== f.redactionState)
        return false;
      if (f.searchTerm) {
        const q = f.searchTerm.toLowerCase();
        if (
          ![
            event.title,
            event.source,
            event.eventType,
            event.comm,
            event.id,
            JSON.stringify(event.data),
          ].some((value) =>
            String(value || "")
              .toLowerCase()
              .includes(q),
          )
        )
          return false;
      }
      return true;
    });
  });

  const visibleProcessedEvents = computed(() =>
    processAgentSightEvents(visibleEvents.value),
  );

  const metrics = computed(() => {
    const list = visibleEvents.value;
    return {
      total: list.length,
      tls: list.filter((event) => event.source === "ssl").length,
      http: list.filter(
        (event) =>
          event.source === "http_parser" || event.eventType.includes("HTTP"),
      ).length,
      sse: list.filter(
        (event) =>
          event.source === "sse_processor" || event.eventType.includes("SSE"),
      ).length,
      sanitized: list.filter((event) => event.redactionState === "sanitized")
        .length,
      alerts: list.filter(
        (event) =>
          event.source === "policy" || event.eventType.includes("ALERT"),
      ).length,
      processes: new Set(list.map((event) => event.pid).filter(Boolean)).size,
      stdio: list.filter((event) => event.source === "stdio").length,
      imported: importedRecords.value.length,
      sample: realRecords.value.length > 0 ? 0 : sampleRecords.value.length,
      system: list.filter((event) => event.source === "system").length,
    };
  });

  const exportVisibleJSON = () => {
    downloadText(
      `agentsight-events-${Date.now()}.json`,
      JSON.stringify(visibleEvents.value, null, 2),
      "application/json;charset=utf-8",
    );
  };

  const exportVisibleJSONL = () => {
    downloadText(
      `agentsight-events-${Date.now()}.jsonl`,
      visibleEvents.value
        .map((event) => JSON.stringify(event.raw ?? event))
        .join("\n") + "\n",
      "application/x-ndjson;charset=utf-8",
    );
  };

  const exportVisibleCSV = () => {
    const rows = [
      [
        "timestamp",
        "source",
        "event_type",
        "pid",
        "ppid",
        "comm",
        "trace_id",
        "redaction_state",
        "summary",
      ],
      ...visibleEvents.value.map((event) => [
        event.timestamp,
        event.source,
        event.eventType,
        event.pid,
        event.ppid || "",
        event.comm,
        event.traceId,
        event.redactionState,
        event.title,
      ]),
    ];
    downloadText(
      `agentsight-events-${Date.now()}.csv`,
      rows.map((row) => row.map(csvCell).join(",")).join("\n") + "\n",
      "text/csv;charset=utf-8",
    );
  };

  watch(queryString, () => {
    if (!componentMounted || paused.value) return;
    void fetchEnvelopeEvents();
  });

  const runRefreshLoop = async () => {
    if (!componentMounted) return;
    if (!paused.value) await fetchEvents();
    if (componentMounted) {
      refreshTimer = setTimeout(() => void runRefreshLoop(), 10000);
    }
  };

  onMounted(() => {
    componentMounted = true;
    shouldReconnect = true;
    connectEnvelopeWebSocket();
    connectSystemWebSocket();
    void runRefreshLoop();
  });

  onUnmounted(() => {
    componentMounted = false;
    shouldReconnect = false;
    if (refreshTimer) clearTimeout(refreshTimer);
    if (envelopeReconnectTimer) clearTimeout(envelopeReconnectTimer);
    if (tlsReconnectTimer) clearTimeout(tlsReconnectTimer);
    if (systemReconnectTimer) clearTimeout(systemReconnectTimer);
    refreshTimer = null;
    envelopeReconnectTimer = null;
    tlsReconnectTimer = null;
    systemReconnectTimer = null;
    envelopeFetchGeneration++;
    tlsFetchGeneration++;
    envelopeFetchController?.abort();
    tlsFetchController?.abort();
    envelopeFetchController = null;
    tlsFetchController = null;
    loading.value = false;
    tlsLoading.value = false;
    disposeWebSocket(envelopeWS);
    disposeWebSocket(tlsWS);
    disposeWebSocket(systemWS);
    envelopeWS = null;
    tlsWS = null;
    systemWS = null;
  });

  return {
    rawRecords,
    events,
    processedEvents,
    visibleEvents,
    visibleProcessedEvents,
    importedRecords,
    sampleRecords,
    loading,
    tlsLoading,
    sampleLoading,
    isEnvelopeConnected,
    isTLSConnected,
    isSystemConnected,
    paused,
    togglePaused,
    limit,
    filters,
    activeTab,
    eventTypeOptions,
    sourceOptions,
    redactionStateOptions,
    metrics,
    fetchEvents,
    importRecordsText,
    loadSampleTrace,
    clearImportedRecords,
    clearAllRecords,
    clearFilters,
    exportVisibleJSON,
    exportVisibleJSONL,
    exportVisibleCSV,
  };
}
