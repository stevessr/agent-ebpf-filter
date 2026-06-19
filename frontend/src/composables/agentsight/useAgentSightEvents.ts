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
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function cacheAgentSightRecords(records: AgentSightEventRecord[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(AGENTSIGHT_CACHE_KEY, JSON.stringify(records));
  } catch {
    window.localStorage.removeItem(AGENTSIGHT_CACHE_KEY);
  }
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
  const trimmed = text.trim();
  if (!trimmed) return [];
  try {
    const parsed = JSON.parse(trimmed);
    const items = Array.isArray(parsed)
      ? parsed
      : parsed.events || parsed.records || [parsed];
    return items
      .map(normalizeImportedAgentSightRecord)
      .filter(Boolean) as AgentSightEventRecord[];
  } catch {
    return trimmed
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line, index) => {
        try {
          return normalizeImportedAgentSightRecord(JSON.parse(line), index);
        } catch {
          return null;
        }
      })
      .filter(Boolean) as AgentSightEventRecord[];
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

  let refreshTimer: ReturnType<typeof setInterval> | null = null;
  let envelopeWS: WebSocket | null = null;
  let tlsWS: WebSocket | null = null;
  let systemWS: WebSocket | null = null;
  let envelopeReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let tlsReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let systemReconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let shouldReconnect = true;

  const realRecords = computed(() => [
    ...importedRecords.value,
    ...systemRecords.value,
    ...tlsRecords.value,
    ...liveRecords.value,
  ]);
  const rawRecords = computed(() =>
    realRecords.value.length > 0 ? realRecords.value : sampleRecords.value,
  );
  const events = computed<AgentSightEvent[]>(() =>
    applyAgentSightLimit(
      normalizeAgentSightEvents(rawRecords.value),
      limit.value,
    ),
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
    loading.value = true;
    try {
      const response = await axios.get(`/events/recent?${queryString.value}`);
      const live = Array.isArray(response.data?.events)
        ? response.data.events.slice().reverse()
        : [];
      liveRecords.value = live;
    } catch (error: any) {
      message.error(
        error?.response?.data?.error ||
          "Failed to load AgentSight event envelopes",
      );
    } finally {
      loading.value = false;
    }
  };

  const fetchTLSEvents = async () => {
    tlsLoading.value = true;
    try {
      const response = await axios.get(
        `/tls-capture/recent?limit=${agentSightLimitQuery(limit.value)}`,
      );
      const tls = Array.isArray(response.data?.events)
        ? response.data.events.map(tlsCaptureEventToRecord)
        : [];
      tlsRecords.value = tls;
    } catch (error: any) {
      if (error?.response?.status && error.response.status !== 404) {
        message.warning(
          error?.response?.data?.error ||
            "TLS capture history is not available",
        );
      }
      tlsRecords.value = [];
    } finally {
      tlsLoading.value = false;
    }
  };

  const fetchEvents = async () => {
    await Promise.all([fetchEnvelopeEvents(), fetchTLSEvents()]);
    await autoLoadSampleIfEmpty();
  };

  const mergeEnvelopeRecords = (incoming: Record<string, any>[]) => {
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
    liveRecords.value = applyAgentSightLimit(
      [...records.reverse(), ...liveRecords.value],
      limit.value,
    );
  };

  const connectEnvelopeWebSocket = () => {
    if (!shouldReconnect) return;
    if (envelopeWS) envelopeWS.close();
    const socket = new WebSocket(buildWebSocketUrl("/ws/envelopes"));
    envelopeWS = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      isEnvelopeConnected.value = true;
    };
    socket.onmessage = (event) => {
      try {
        const batch = pb.EventEnvelopeBatch.decode(new Uint8Array(event.data));
        const envelopes = JSON.parse(JSON.stringify(batch.envelopes || []));
        mergeEnvelopeRecords(envelopes);
      } catch (error) {
        console.error("AgentSight envelope websocket parse error", error);
      }
    };
    socket.onclose = () => {
      isEnvelopeConnected.value = false;
      if (shouldReconnect)
        envelopeReconnectTimer = setTimeout(connectEnvelopeWebSocket, 3000);
    };
    socket.onerror = () => {
      isEnvelopeConnected.value = false;
    };
  };

  const connectTLSWebSocket = () => {
    if (!shouldReconnect) return;
    if (tlsWS) tlsWS.close();
    const socket = new WebSocket(buildWebSocketUrl("/ws/tls-capture"));
    tlsWS = socket;
    socket.onopen = () => {
      isTLSConnected.value = true;
    };
    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(String(event.data));
        tlsRecords.value = applyAgentSightLimit(
          [tlsCaptureEventToRecord(payload), ...tlsRecords.value],
          limit.value,
        );
      } catch (error) {
        console.error("AgentSight TLS websocket parse error", error);
      }
    };
    socket.onclose = () => {
      isTLSConnected.value = false;
      if (shouldReconnect)
        tlsReconnectTimer = setTimeout(connectTLSWebSocket, 3000);
    };
    socket.onerror = () => {
      isTLSConnected.value = false;
    };
  };

  const connectSystemWebSocket = () => {
    if (!shouldReconnect) return;
    if (systemWS) systemWS.close();
    const socket = new WebSocket(
      buildWebSocketUrl("/ws/system", { interval: "2000" }),
    );
    systemWS = socket;
    socket.binaryType = "arraybuffer";
    socket.onopen = () => {
      isSystemConnected.value = true;
    };
    socket.onmessage = (event) => {
      try {
        const stats = pb.SystemStats.decode(new Uint8Array(event.data));
        const plain = JSON.parse(JSON.stringify(stats));
        const timestamp = Date.now();
        systemRecords.value = applyAgentSightLimit(
          [...systemStatsToRecords(plain, timestamp), ...systemRecords.value],
          limit.value,
        );
      } catch (error) {
        console.error("AgentSight system websocket parse error", error);
      }
    };
    socket.onclose = () => {
      isSystemConnected.value = false;
      if (shouldReconnect)
        systemReconnectTimer = setTimeout(connectSystemWebSocket, 3000);
    };
    socket.onerror = () => {
      isSystemConnected.value = false;
    };
  };

  const importRecordsText = (text: string) => {
    const parsed = parseAgentSightRecordsText(text);
    importedRecords.value = [...parsed, ...importedRecords.value];
    cacheAgentSightRecords(importedRecords.value);
    return parsed.length;
  };

  const loadSampleTrace = async () => {
    sampleLoading.value = true;
    sampleAttempted.value = true;
    try {
      const response = await axios.get(AGENTSIGHT_SAMPLE_TRACE_URL, {
        responseType: "text",
      });
      const parsed = parseAgentSightRecordsText(String(response.data || ""));
      sampleRecords.value = parsed;
      return parsed.length;
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
    void fetchEnvelopeEvents();
  });

  onMounted(() => {
    void fetchEvents();
    connectEnvelopeWebSocket();
    connectTLSWebSocket();
    connectSystemWebSocket();
    refreshTimer = setInterval(fetchEvents, 10000);
  });

  onUnmounted(() => {
    shouldReconnect = false;
    if (refreshTimer) clearInterval(refreshTimer);
    if (envelopeReconnectTimer) clearTimeout(envelopeReconnectTimer);
    if (tlsReconnectTimer) clearTimeout(tlsReconnectTimer);
    if (systemReconnectTimer) clearTimeout(systemReconnectTimer);
    if (envelopeWS) envelopeWS.close();
    if (tlsWS) tlsWS.close();
    if (systemWS) systemWS.close();
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
