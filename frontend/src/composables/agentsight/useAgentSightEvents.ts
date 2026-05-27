import { computed, onMounted, onUnmounted, ref } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

import { pb } from '../../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../../utils/requestContext';

export interface AgentSightEventRecord {
  Event?: Record<string, any>;
  Timestamp?: number;
  Envelope?: Record<string, any>;
}

export interface AgentSightFilters {
  source: string;
  eventType: string;
  pid: string;
  comm: string;
  traceId: string;
  redactionState: string;
}

const AGENTSIGHT_CACHE_KEY = 'agent-ebpf.agentsight.importedRecords';

const defaultFilters = (): AgentSightFilters => ({
  source: '',
  eventType: '',
  pid: '',
  comm: '',
  traceId: '',
  redactionState: '',
});

const eventPayload = (record: AgentSightEventRecord) => record.Envelope?.tls_event || record.Envelope?.tlsEvent
  || record.Envelope?.http_event || record.Envelope?.httpEvent
  || record.Envelope?.sse_event || record.Envelope?.sseEvent
  || record.Envelope?.stdio_event || record.Envelope?.stdioEvent
  || record.Envelope?.system_metric_event || record.Envelope?.systemMetricEvent
  || record.Envelope?.otel_span_event || record.Envelope?.otelSpanEvent
  || record.Envelope?.agentsight_alert_event || record.Envelope?.agentsightAlertEvent
  || record.Envelope?.network_event || record.Envelope?.networkEvent
  || record.Envelope?.process_event || record.Envelope?.processEvent
  || record.Envelope?.file_event || record.Envelope?.fileEvent
  || record.Envelope?.policy_event || record.Envelope?.policyEvent
  || record.Envelope?.wrapper_event || record.Envelope?.wrapperEvent
  || record.Envelope?.hook_event || record.Envelope?.hookEvent
  || record.Envelope?.mcp_event || record.Envelope?.mcpEvent
  || {};

export const recordEventType = (record: AgentSightEventRecord) => record.Envelope?.event_type
  || record.Envelope?.eventType
  || record.Event?.eventType
  || record.Event?.type
  || 'UNKNOWN';

export const recordSource = (record: AgentSightEventRecord) => record.Envelope?.source || record.Event?.type || 'unknown';
export const recordPID = (record: AgentSightEventRecord) => Number(record.Envelope?.pid || record.Event?.pid || 0);
export const recordComm = (record: AgentSightEventRecord) => record.Envelope?.comm || record.Event?.comm || '—';
export const recordTraceID = (record: AgentSightEventRecord) => record.Envelope?.trace_id || record.Envelope?.traceId || record.Event?.traceId || '';
export const recordRedactionState = (record: AgentSightEventRecord) => eventPayload(record)?.redaction_state || eventPayload(record)?.redactionState || '';

export const recordTitle = (record: AgentSightEventRecord) => {
  const payload = eventPayload(record);
  return payload.url || payload.host || payload.method || payload.phase || payload.operation || payload.decision || record.Event?.path || record.Event?.netEndpoint || '—';
};

export const formatAgentSightTime = (value?: number) => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
};

function loadCachedAgentSightRecords(): AgentSightEventRecord[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(AGENTSIGHT_CACHE_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function cacheAgentSightRecords(records: AgentSightEventRecord[]) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(AGENTSIGHT_CACHE_KEY, JSON.stringify(records.slice(0, 2000)));
}

function normalizeImportedAgentSightRecord(value: any, index: number): AgentSightEventRecord | null {
  if (!value || typeof value !== 'object') return null;
  if (value.Event || value.Envelope || value.Timestamp) return value as AgentSightEventRecord;
  if (value.event || value.envelope || value.timestamp) {
    return { Event: value.event, Envelope: value.envelope, Timestamp: Number(value.timestamp || Date.now() + index) };
  }
  return { Event: value, Timestamp: Date.now() + index };
}

export function parseAgentSightRecordsText(text: string): AgentSightEventRecord[] {
  const trimmed = text.trim();
  if (!trimmed) return [];
  try {
    const parsed = JSON.parse(trimmed);
    const items = Array.isArray(parsed) ? parsed : parsed.events || parsed.records || [parsed];
    return items.map(normalizeImportedAgentSightRecord).filter(Boolean) as AgentSightEventRecord[];
  } catch {
    return trimmed.split('\n')
      .map(line => line.trim())
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

export function useAgentSightEvents() {
  const records = ref<AgentSightEventRecord[]>([]);
  const importedRecords = ref<AgentSightEventRecord[]>(loadCachedAgentSightRecords());
  const loading = ref(false);
  const limit = ref(500);
  const activeTab = ref('log');
  const filters = ref<AgentSightFilters>(defaultFilters());
  const isConnected = ref(false);
  let refreshTimer: ReturnType<typeof setInterval> | null = null;
  let ws: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let shouldReconnect = true;

  const queryString = computed(() => {
    const params = new URLSearchParams();
    params.set('limit', String(limit.value));
    if (filters.value.source) params.set('source', filters.value.source);
    if (filters.value.eventType) params.set('event_type', filters.value.eventType);
    if (filters.value.pid) params.set('pid', filters.value.pid);
    if (filters.value.comm) params.set('comm', filters.value.comm);
    if (filters.value.traceId) params.set('trace_id', filters.value.traceId);
    if (filters.value.redactionState) params.set('redaction_state', filters.value.redactionState);
    return params.toString();
  });

  const fetchEvents = async () => {
    loading.value = true;
    try {
      const response = await axios.get(`/events/recent?${queryString.value}`);
      const live = Array.isArray(response.data?.events) ? response.data.events.slice().reverse() : [];
      records.value = [...importedRecords.value, ...live].slice(0, limit.value);
    } catch (error: any) {
      message.error(error?.response?.data?.error || 'Failed to load AgentSight events');
    } finally {
      loading.value = false;
    }
  };

  const mergeEnvelopeRecords = (incoming: Record<string, any>[]) => {
    if (incoming.length === 0) return;
    const liveRecords = incoming.map(envelope => ({
      Event: envelope.legacy_event || envelope.legacyEvent,
      Timestamp: (envelope.timestamp_ns || envelope.timestampNs) ? Math.floor(Number(envelope.timestamp_ns || envelope.timestampNs) / 1_000_000) : Date.now(),
      Envelope: envelope,
    }));
    records.value = [...liveRecords.reverse(), ...records.value].slice(0, limit.value);
  };

  const connectWebSocket = () => {
    if (!shouldReconnect) return;
    if (ws) ws.close();
    const socket = new WebSocket(buildWebSocketUrl('/ws/envelopes'));
    ws = socket;
    socket.binaryType = 'arraybuffer';
    socket.onopen = () => {
      isConnected.value = true;
    };
    socket.onmessage = (event) => {
      try {
        const batch = pb.EventEnvelopeBatch.decode(new Uint8Array(event.data));
        const envelopes = JSON.parse(JSON.stringify(batch.envelopes || []));
        mergeEnvelopeRecords(envelopes);
      } catch (error) {
        console.error('AgentSight envelope websocket parse error', error);
      }
    };
    socket.onclose = () => {
      isConnected.value = false;
      if (shouldReconnect) reconnectTimer = setTimeout(connectWebSocket, 3000);
    };
    socket.onerror = () => {
      isConnected.value = false;
    };
  };

  const importRecordsText = (text: string) => {
    const parsed = parseAgentSightRecordsText(text);
    importedRecords.value = [...parsed, ...importedRecords.value].slice(0, 2000);
    cacheAgentSightRecords(importedRecords.value);
    records.value = [...parsed, ...records.value].slice(0, limit.value);
    return parsed.length;
  };

  const clearImportedRecords = () => {
    importedRecords.value = [];
    cacheAgentSightRecords([]);
    records.value = [];
    void fetchEvents();
  };

  const clearFilters = () => {
    filters.value = defaultFilters();
  };

  const eventTypeOptions = computed(() => Array.from(new Set(records.value.map(recordEventType))).filter(Boolean).sort().map(value => ({ label: value, value })));
  const sourceOptions = computed(() => Array.from(new Set(records.value.map(recordSource))).filter(Boolean).sort().map(value => ({ label: value, value })));

  const metrics = computed(() => {
    const total = records.value.length;
    const tls = records.value.filter(record => String(recordEventType(record)).includes('TLS')).length;
    const http = records.value.filter(record => String(recordEventType(record)).includes('HTTP') || Boolean((eventPayload(record) as any).url)).length;
    const sse = records.value.filter(record => String(recordEventType(record)).includes('SSE') || Boolean((eventPayload(record) as any).data_digest)).length;
    const sanitized = records.value.filter(record => recordRedactionState(record) === 'sanitized').length;
    const alerts = records.value.filter(record => String(recordEventType(record)).includes('ALERT') || record.Envelope?.agentsight_alert_event).length;
    const processes = new Set(records.value.map(recordPID).filter(Boolean)).size;
    return { total, tls, http, sse, sanitized, alerts, processes };
  });

  const processTree = computed(() => {
    const nodes = new Map<number, { pid: number; ppid: number; comm: string; events: number; children: number[] }>();
    for (const record of records.value) {
      const pid = recordPID(record);
      if (!pid) continue;
      const ppid = Number(record.Envelope?.ppid || record.Event?.ppid || 0);
      const existing = nodes.get(pid) || { pid, ppid, comm: recordComm(record), events: 0, children: [] };
      existing.events++;
      existing.ppid = existing.ppid || ppid;
      existing.comm = existing.comm || recordComm(record);
      nodes.set(pid, existing);
      if (ppid) {
        const parent = nodes.get(ppid) || { pid: ppid, ppid: 0, comm: 'parent', events: 0, children: [] };
        if (!parent.children.includes(pid)) parent.children.push(pid);
        nodes.set(ppid, parent);
      }
    }
    return Array.from(nodes.values()).sort((a, b) => a.pid - b.pid);
  });

  onMounted(() => {
    void fetchEvents();
    connectWebSocket();
    refreshTimer = setInterval(fetchEvents, 10000);
  });

  onUnmounted(() => {
    shouldReconnect = false;
    if (refreshTimer) clearInterval(refreshTimer);
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (ws) ws.close();
  });

  return {
    records,
    importedRecords,
    loading,
    isConnected,
    limit,
    filters,
    activeTab,
    eventTypeOptions,
    sourceOptions,
    metrics,
    processTree,
    fetchEvents,
    importRecordsText,
    clearImportedRecords,
    clearFilters,
  };
}
