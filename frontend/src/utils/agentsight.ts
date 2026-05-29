export interface AgentSightEventRecord {
  Event?: Record<string, any>;
  Timestamp?: number | string;
  Envelope?: Record<string, any>;
  event?: Record<string, any>;
  timestamp?: number | string;
  envelope?: Record<string, any>;
  [key: string]: any;
}

export interface AgentSightEvent {
  id: string;
  timestamp: number;
  source: string;
  rawSource: string;
  pid: number;
  ppid?: number;
  comm: string;
  eventType: string;
  traceId: string;
  spanId: string;
  redactionState: string;
  title: string;
  data: Record<string, any>;
  raw: any;
  envelope?: Record<string, any>;
  legacyEvent?: Record<string, any>;
}

export interface ProcessedAgentSightEvent extends AgentSightEvent {
  datetime: Date;
  formattedTime: string;
  sourceColor: string;
  sourceColorClass: string;
}

export type ParsedAgentSightEventType = 'prompt' | 'response' | 'ssl' | 'file' | 'process' | 'stdio' | 'system' | 'policy' | 'agent';

export interface ParsedAgentSightEvent {
  id: string;
  timestamp: number;
  type: ParsedAgentSightEventType;
  title: string;
  content: string;
  metadata: Record<string, any>;
  promptDiff?: {
    diff: string;
    summary: string;
    hasChanges: boolean;
    previousPromptId?: string;
  };
}

export interface AgentSightTimelineItem {
  type: 'event' | 'process';
  timestamp: number;
  event?: ParsedAgentSightEvent;
  process?: AgentSightProcessNode;
}

export interface AgentSightProcessNode {
  pid: number;
  ppid?: number;
  comm: string;
  children: AgentSightProcessNode[];
  events: ParsedAgentSightEvent[];
  timeline: AgentSightTimelineItem[];
}

export interface AgentSightProcessFilters {
  eventTypes: string[];
  models: string[];
  sources: string[];
  commands: string[];
  searchText: string;
  timeRange: {
    start?: number;
    end?: number;
  };
}

export interface AgentSightFilterOptions {
  eventTypes: string[];
  models: string[];
  sources: string[];
  commands: string[];
}

export interface DecodedStdioMessage {
  direction: string;
  fdRole: string;
  fdTarget: string;
  fd: number | null;
  length: number;
  truncated: boolean;
  rawPayload: string;
  parsedPayload: any | null;
  kind: 'request' | 'notification' | 'response' | 'error' | 'text' | 'unknown';
  method?: string;
  id?: string;
  toolName?: string;
  preview?: string;
  title: string;
  summary: string;
}

const SOURCE_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444', '#6366f1', '#ec4899', '#6b7280'];
const SOURCE_COLOR_CLASSES = ['blue', 'green', 'gold', 'purple', 'red', 'geekblue', 'magenta', 'default'];

const EVENT_TYPE_NAMES: Record<number, string> = {
  0: 'EXECVE',
  1: 'OPENAT',
  2: 'NETWORK_CONNECT',
  3: 'MKDIR',
  4: 'UNLINK',
  5: 'IOCTL',
  6: 'NETWORK_BIND',
  7: 'NETWORK_SENDTO',
  8: 'NETWORK_RECVFROM',
  9: 'READ',
  10: 'WRITE',
  11: 'OPEN',
  12: 'CHMOD',
  13: 'CHOWN',
  14: 'RENAME',
  15: 'LINK',
  16: 'SYMLINK',
  17: 'MKNOD',
  18: 'CLONE',
  19: 'EXIT',
  20: 'SOCKET',
  21: 'ACCEPT',
  22: 'ACCEPT4',
  23: 'WRAPPER_INTERCEPT',
  24: 'NATIVE_HOOK',
  25: 'GENERIC_SYSCALL',
  26: 'SCHED_PROCESS_FORK',
  27: 'SCHED_PROCESS_EXEC',
  28: 'SCHED_PROCESS_EXIT',
  29: 'WAIT4',
  30: 'SEMANTIC_ALERT',
  31: 'TCP_CONNECT',
  32: 'TCP_CLOSE',
  33: 'TCP_STATE_CHANGE',
  34: 'DNS_QUERY',
  35: 'TLS_PLAINTEXT',
  36: 'HTTP_MESSAGE',
  37: 'SSE_MESSAGE',
  38: 'STDIO',
  39: 'SYSTEM_METRIC',
  40: 'OTEL_SPAN',
  41: 'AGENTSIGHT_ALERT',
};

const payloadSources: Array<{ keys: string[]; source: string; eventType: string }> = [
  { keys: ['tls_event', 'tlsEvent'], source: 'ssl', eventType: 'TLS_PLAINTEXT' },
  { keys: ['http_event', 'httpEvent'], source: 'http_parser', eventType: 'HTTP_MESSAGE' },
  { keys: ['sse_event', 'sseEvent'], source: 'sse_processor', eventType: 'SSE_MESSAGE' },
  { keys: ['stdio_event', 'stdioEvent'], source: 'stdio', eventType: 'STDIO' },
  { keys: ['system_metric_event', 'systemMetricEvent'], source: 'system', eventType: 'SYSTEM_METRIC' },
  { keys: ['otel_span_event', 'otelSpanEvent'], source: 'otel', eventType: 'OTEL_SPAN' },
  { keys: ['agentsight_alert_event', 'agentsightAlertEvent'], source: 'policy', eventType: 'AGENTSIGHT_ALERT' },
  { keys: ['network_event', 'networkEvent'], source: 'network', eventType: 'NETWORK' },
  { keys: ['process_event', 'processEvent'], source: 'process', eventType: 'PROCESS' },
  { keys: ['file_event', 'fileEvent'], source: 'file', eventType: 'FILE' },
  { keys: ['policy_event', 'policyEvent'], source: 'policy', eventType: 'POLICY' },
  { keys: ['wrapper_event', 'wrapperEvent'], source: 'agent', eventType: 'WRAPPER_INTERCEPT' },
  { keys: ['hook_event', 'hookEvent'], source: 'agent', eventType: 'NATIVE_HOOK' },
  { keys: ['mcp_event', 'mcpEvent'], source: 'stdio', eventType: 'MCP' },
  { keys: ['exec_event', 'execEvent'], source: 'process', eventType: 'EXECVE' },
];

function readAny<T = any>(obj: any, keys: string[], fallback?: T): T {
  if (!obj || typeof obj !== 'object') return fallback as T;
  for (const key of keys) {
    if (obj[key] !== undefined && obj[key] !== null) return obj[key] as T;
  }
  return fallback as T;
}

function firstNonEmpty(...values: any[]) {
  for (const value of values) {
    if (value !== undefined && value !== null && String(value).trim() !== '') return value;
  }
  return '';
}

function toNumber(value: any, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function eventTypeName(value: any, fallback: string) {
  if (typeof value === 'number') return EVENT_TYPE_NAMES[value] || fallback;
  if (typeof value === 'string' && value.trim() !== '') return value;
  return fallback;
}

function parseTimestampMs(value: any, fallback = Date.now()) {
  if (typeof value === 'number') {
    if (value > 1_000_000_000_000_000) return Math.floor(value / 1_000_000);
    if (value > 10_000_000_000_000) return Math.floor(value / 1_000);
    return Math.floor(value);
  }
  if (typeof value === 'string' && value.trim() !== '') {
    const numeric = Number(value);
    if (Number.isFinite(numeric)) return parseTimestampMs(numeric, fallback);
    const parsed = Date.parse(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function stableID(prefix: string, parts: any[]) {
  return `${prefix}-${parts.map(value => String(value ?? '').replace(/\s+/g, '_').slice(0, 80)).join('-')}`;
}

function getEnvelopePayload(envelope: Record<string, any> | undefined) {
  if (!envelope) return { payload: undefined, source: '', eventType: '' };
  for (const entry of payloadSources) {
    for (const key of entry.keys) {
      if (envelope[key] !== undefined && envelope[key] !== null) {
        return { payload: envelope[key], source: entry.source, eventType: entry.eventType };
      }
    }
  }
  return { payload: undefined, source: '', eventType: '' };
}

function normalizeSource(source: string, eventType: string, legacyType: string, data: Record<string, any>) {
  const sourceLower = source.toLowerCase();
  const typeLower = `${eventType} ${legacyType} ${readAny(data, ['type'], '')}`.toLowerCase();
  if (sourceLower) return sourceLower;
  if (typeLower.includes('stdio') || typeLower.includes('mcp')) return 'stdio';
  if (typeLower.includes('system')) return 'system';
  if (typeLower.includes('tls') || typeLower.includes('http_request') || typeLower.includes('http_response')) return 'ssl';
  if (typeLower.includes('http')) return 'http_parser';
  if (typeLower.includes('sse')) return 'sse_processor';
  if (typeLower.includes('process') || typeLower.includes('exec') || typeLower.includes('fork') || typeLower.includes('clone') || typeLower.includes('exit')) return 'process';
  if (typeLower.includes('open') || typeLower.includes('file') || typeLower.includes('write') || typeLower.includes('read')) return 'file';
  if (typeLower.includes('policy') || typeLower.includes('alert') || typeLower.includes('semantic')) return 'policy';
  if (typeLower.includes('wrapper') || typeLower.includes('hook')) return 'agent';
  return 'unknown';
}

function normalizeTLSLikeData(payload: Record<string, any>, legacyEvent?: Record<string, any>) {
  const method = firstNonEmpty(readAny(payload, ['method']), readAny(legacyEvent, ['path', 'Path']));
  const url = firstNonEmpty(readAny(payload, ['url']), readAny(legacyEvent, ['extra_path', 'extraPath', 'ExtraPath']));
  const host = firstNonEmpty(readAny(payload, ['host']), readAny(legacyEvent, ['http_host', 'httpHost', 'HttpHost', 'net_endpoint', 'netEndpoint', 'NetEndpoint']));
  const status = toNumber(readAny(payload, ['status', 'status_code', 'statusCode']), 0);
  const direction = firstNonEmpty(readAny(payload, ['direction']), readAny(legacyEvent, ['net_direction', 'netDirection', 'NetDirection']));
  const bodySize = toNumber(readAny(payload, ['body_size', 'bodySize']), 0);
  return {
    ...payload,
    method,
    url,
    path: url,
    host,
    status_code: status,
    status,
    direction,
    body_size: bodySize,
    content_type: readAny(payload, ['content_type', 'contentType'], ''),
    message_type: status ? 'response' : method ? 'request' : readAny(payload, ['message_type', 'messageType'], ''),
    redaction_state: readAny(payload, ['redaction_state', 'redactionState'], ''),
    raw_available: readAny(payload, ['raw_available', 'rawAvailable'], false),
    prompt_digest: readAny(payload, ['prompt_digest', 'promptDigest'], ''),
    prompt_len: readAny(payload, ['prompt_len', 'promptLen'], 0),
    message_role: readAny(payload, ['message_role', 'messageRole'], ''),
    vendor: readAny(payload, ['vendor'], ''),
  };
}

function normalizeSystemData(payload: Record<string, any>, legacyEvent?: Record<string, any>) {
  const cpu = toNumber(readAny(payload, ['cpu_percent', 'cpuPercent']), 0);
  const memoryBytes = toNumber(readAny(payload, ['memory_bytes', 'memoryBytes'], readAny(legacyEvent, ['bytes', 'Bytes'], 0)), 0);
  const threads = toNumber(readAny(payload, ['threads']), 0);
  const children = toNumber(readAny(payload, ['children']), 0);
  return {
    ...payload,
    type: 'system_metrics',
    cpu: { percent: cpu, cores: readAny(payload, ['cores'], 0) },
    memory: { rss_mb: Math.round(memoryBytes / 1024 / 1024), vsz_mb: Math.round(memoryBytes / 1024 / 1024) },
    process: { threads, children, state: readAny(payload, ['process_state', 'processState'], '') },
    alert: Boolean(readAny(payload, ['alert'], '')),
  };
}

function normalizeProcessData(payload: Record<string, any>, event: AgentSightEventRecord, legacyEvent?: Record<string, any>) {
  const phase = firstNonEmpty(readAny(payload, ['phase']), readAny(legacyEvent, ['type', 'Type']));
  return {
    ...payload,
    event: String(phase || 'process').toUpperCase(),
    pid: readAny(event, ['pid', 'Pid'], readAny(legacyEvent, ['pid', 'Pid'], 0)),
    ppid: readAny(event, ['ppid', 'Ppid'], readAny(payload, ['parent_pid', 'parentPid'], readAny(legacyEvent, ['ppid', 'Ppid'], 0))),
    comm: readAny(event, ['comm', 'Comm'], readAny(legacyEvent, ['comm', 'Comm'], '')),
    child_pid: readAny(payload, ['child_pid', 'childPid'], 0),
    target_pid: readAny(payload, ['target_pid', 'targetPid'], 0),
    old_pid: readAny(payload, ['old_pid', 'oldPid'], 0),
  };
}

function normalizeFileData(payload: Record<string, any>, legacyEvent?: Record<string, any>) {
  return {
    ...payload,
    operation: firstNonEmpty(readAny(payload, ['operation']), readAny(legacyEvent, ['type', 'Type'])),
    path: firstNonEmpty(readAny(payload, ['path']), readAny(legacyEvent, ['path', 'Path'])),
    filepath: firstNonEmpty(readAny(payload, ['path']), readAny(legacyEvent, ['path', 'Path'])),
    size: readAny(payload, ['bytes'], readAny(legacyEvent, ['bytes', 'Bytes'], 0)),
  };
}

function normalizePayloadData(source: string, payload: Record<string, any>, normalized: AgentSightEventRecord, legacyEvent?: Record<string, any>) {
  if (source === 'ssl' || source === 'http_parser' || source === 'sse_processor') return normalizeTLSLikeData(payload, legacyEvent);
  if (source === 'system') return normalizeSystemData(payload, legacyEvent);
  if (source === 'process') return normalizeProcessData(payload, normalized, legacyEvent);
  if (source === 'file') return normalizeFileData(payload, legacyEvent);
  if (source === 'policy') {
    return {
      ...payload,
      event: firstNonEmpty(readAny(payload, ['decision']), readAny(legacyEvent, ['decision', 'Decision']), 'policy'),
      reason: firstNonEmpty(readAny(payload, ['reason']), readAny(legacyEvent, ['extra_info', 'extraInfo', 'ExtraInfo'])),
    };
  }
  return { ...payload };
}

function normalizeReferenceEvent(value: any, index: number): AgentSightEvent | null {
  if (!value || typeof value !== 'object') return null;
  if (value.data === undefined || value.timestamp === undefined || value.source === undefined) return null;
  const timestamp = parseTimestampMs(value.timestamp, Date.now() + index);
  const data = value.data && typeof value.data === 'object' ? value.data : { value: value.data };
  const source = normalizeSource(String(value.source || ''), String(data.event_type || data.type || ''), String(data.type || ''), data);
  const eventType = eventTypeName(data.event_type, String(data.type || source).toUpperCase());
  const pid = toNumber(value.pid, toNumber(data.pid, 0));
  const ppid = toNumber(value.ppid, toNumber(data.ppid, 0));
  const comm = String(firstNonEmpty(value.comm, data.comm, '—'));
  const title = buildEventTitle(source, eventType, data, value);
  return {
    id: String(firstNonEmpty(value.id, stableID('ref', [source, timestamp, pid, index]))),
    timestamp,
    source,
    rawSource: String(value.source || source),
    pid,
    ppid,
    comm,
    eventType,
    traceId: String(firstNonEmpty(value.trace_id, value.traceId, data.trace_id, data.traceId)),
    spanId: String(firstNonEmpty(value.span_id, value.spanId, data.span_id, data.spanId)),
    redactionState: String(firstNonEmpty(data.redaction_state, data.redactionState)),
    title,
    data,
    raw: value,
  };
}

function buildEventTitle(source: string, eventType: string, data: Record<string, any>, raw?: any) {
  if (source === 'stdio') return decodeStdioMessage(data).summary;
  if (source === 'ssl' || source === 'http_parser' || source === 'sse_processor') {
    const method = readAny(data, ['method'], '');
    const host = readAny(data, ['host'], '');
    const url = readAny(data, ['url', 'path'], '');
    const status = readAny(data, ['status', 'status_code', 'statusCode'], '');
    const sse = readAny(data, ['sse_event', 'sseEvent'], '');
    if (sse) return `SSE ${sse} ${readAny(data, ['sse_data_digest', 'sseDataDigest'], '')}`.trim();
    if (method) return `${method} ${host}${url}`.trim();
    if (status) return `${status} ${host}${url}`.trim();
    if (readAny(data, ['body', 'raw_hex_dump', 'rawHexDump'], '')) return String(readAny(data, ['body', 'raw_hex_dump', 'rawHexDump'], '')).slice(0, 120);
  }
  if (source === 'file') return `${readAny(data, ['operation', 'event'], eventType)} ${readAny(data, ['path', 'filepath'], '')}`.trim();
  if (source === 'process') return `${readAny(data, ['event', 'phase'], eventType)} ${readAny(data, ['comm'], readAny(raw, ['comm'], ''))}`.trim();
  if (source === 'system') return `${toNumber(readAny(data, ['cpu_percent', 'cpuPercent', 'cpu.percent']), 0).toFixed(1)}% CPU · ${readAny(data, ['process_state', 'processState'], '')}`;
  if (source === 'policy') return String(firstNonEmpty(readAny(data, ['reason']), readAny(data, ['decision']), eventType));
  return String(firstNonEmpty(readAny(data, ['url', 'host', 'method', 'phase', 'operation', 'decision', 'path', 'netEndpoint']), eventType, '—'));
}

function normalizeRecordLike(value: AgentSightEventRecord, index: number): AgentSightEvent | null {
  const reference = normalizeReferenceEvent(value, index);
  if (reference) return reference;

  const envelope = readAny<Record<string, any> | undefined>(value, ['Envelope', 'envelope']);
  const legacyEvent = readAny<Record<string, any> | undefined>(value, ['Event', 'event'], readAny(envelope, ['legacy_event', 'legacyEvent']));
  const payloadInfo = getEnvelopePayload(envelope);
  const payload = payloadInfo.payload && typeof payloadInfo.payload === 'object' ? payloadInfo.payload : {};
  const rawSource = String(firstNonEmpty(readAny(envelope, ['source']), readAny(legacyEvent, ['source', 'Source']), payloadInfo.source));
  const fallbackType = String(firstNonEmpty(readAny(legacyEvent, ['type', 'Type']), payloadInfo.eventType, rawSource)).toUpperCase();
  const eventType = eventTypeName(readAny(envelope, ['event_type', 'eventType'], readAny(legacyEvent, ['event_type', 'eventType', 'EventType'])), fallbackType);
  const source = normalizeSource(payloadInfo.source, eventType, String(readAny(legacyEvent, ['type', 'Type'], '')), payload);
  const timestamp = parseTimestampMs(readAny(value, ['Timestamp', 'timestamp'], readAny(envelope, ['timestamp_ns', 'timestampNs'])), Date.now() + index);
  const pid = toNumber(firstNonEmpty(readAny(envelope, ['pid']), readAny(legacyEvent, ['pid', 'Pid']), readAny(payload, ['pid'])), 0);
  const ppid = toNumber(firstNonEmpty(readAny(envelope, ['ppid']), readAny(legacyEvent, ['ppid', 'Ppid']), readAny(payload, ['ppid', 'parent_pid', 'parentPid'])), 0);
  const comm = String(firstNonEmpty(readAny(envelope, ['comm']), readAny(legacyEvent, ['comm', 'Comm']), readAny(payload, ['comm']), '—'));
  const normalizedShell: AgentSightEventRecord = { pid, ppid, comm };
  const data = normalizePayloadData(source, payload, normalizedShell, legacyEvent);
  const traceId = String(firstNonEmpty(readAny(envelope, ['trace_id', 'traceId']), readAny(legacyEvent, ['trace_id', 'traceId', 'TraceId'])));
  const spanId = String(firstNonEmpty(readAny(envelope, ['span_id', 'spanId']), readAny(legacyEvent, ['span_id', 'spanId', 'SpanId'])));
  const redactionState = String(firstNonEmpty(readAny(data, ['redaction_state', 'redactionState']), readAny(payload, ['redaction_state', 'redactionState'])));
  return {
    id: String(firstNonEmpty(readAny(envelope, ['event_id', 'eventId']), stableID('evt', [source, timestamp, pid, eventType, traceId, index]))),
    timestamp,
    source,
    rawSource: rawSource || source,
    pid,
    ppid,
    comm,
    eventType,
    traceId,
    spanId,
    redactionState,
    title: buildEventTitle(source, eventType, data, legacyEvent),
    data,
    raw: value,
    envelope,
    legacyEvent,
  };
}

export function normalizeAgentSightEvent(value: any, index = 0): AgentSightEvent | null {
  if (!value || typeof value !== 'object') return null;
  if (value.schema_version || value.schemaVersion || value.legacy_event || value.legacyEvent) {
    return normalizeRecordLike({ Envelope: value }, index);
  }
  return normalizeRecordLike(value as AgentSightEventRecord, index);
}

export function normalizeAgentSightEvents(values: any[]): AgentSightEvent[] {
  const seen = new Set<string>();
  const normalized: AgentSightEvent[] = [];
  values.forEach((value, index) => {
    const event = normalizeAgentSightEvent(value, index);
    if (!event) return;
    const key = event.id || stableID('dedupe', [event.source, event.timestamp, event.pid, event.title]);
    if (seen.has(key)) return;
    seen.add(key);
    normalized.push(event);
  });
  return normalized.sort((a, b) => b.timestamp - a.timestamp);
}

export function processAgentSightEvents(events: AgentSightEvent[]): ProcessedAgentSightEvent[] {
  const sourceColorMap = new Map<string, string>();
  const sourceClassMap = new Map<string, string>();
  let colorIndex = 0;
  return events.map(event => {
    if (!sourceColorMap.has(event.source)) {
      sourceColorMap.set(event.source, SOURCE_COLORS[colorIndex % SOURCE_COLORS.length]);
      sourceClassMap.set(event.source, SOURCE_COLOR_CLASSES[colorIndex % SOURCE_COLOR_CLASSES.length]);
      colorIndex += 1;
    }
    const datetime = new Date(event.timestamp);
    return {
      ...event,
      datetime,
      formattedTime: formatShortTime(event.timestamp),
      sourceColor: sourceColorMap.get(event.source) || SOURCE_COLORS[0],
      sourceColorClass: sourceClassMap.get(event.source) || SOURCE_COLOR_CLASSES[0],
    };
  });
}

export function formatShortTime(timestamp: number) {
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return String(timestamp);
  return `${date.toLocaleTimeString(undefined, { hour12: false })}.${date.getMilliseconds().toString().padStart(3, '0')}`;
}

export function formatFullTime(timestamp?: number) {
  if (!timestamp) return '—';
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? String(timestamp) : date.toLocaleString();
}

export function formatDuration(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return '0ms';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

export function formatBytes(bytes: number) {
  const value = Number(bytes || 0);
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
}

export function filterProcessedEvents(events: ProcessedAgentSightEvent[], filters: { source?: string; comm?: string; pid?: string; searchTerm?: string; eventType?: string; traceId?: string; redactionState?: string }) {
  let filtered = events;
  if (filters.source) filtered = filtered.filter(event => event.source === filters.source || event.rawSource === filters.source);
  if (filters.comm) filtered = filtered.filter(event => event.comm.toLowerCase().includes(filters.comm!.toLowerCase()));
  if (filters.pid) filtered = filtered.filter(event => String(event.pid) === String(filters.pid));
  if (filters.eventType) filtered = filtered.filter(event => event.eventType === filters.eventType);
  if (filters.traceId) filtered = filtered.filter(event => event.traceId.includes(filters.traceId!));
  if (filters.redactionState) filtered = filtered.filter(event => event.redactionState === filters.redactionState);
  if (filters.searchTerm) {
    const term = filters.searchTerm.toLowerCase();
    filtered = filtered.filter(event => [event.source, event.rawSource, event.id, event.comm, String(event.pid), event.eventType, event.title, JSON.stringify(event.data)]
      .some(value => String(value || '').toLowerCase().includes(term)));
  }
  return filtered;
}

function safeJsonParse(value: string): any | null {
  if (!value.trim()) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function truncateText(value: string, limit = 96) {
  const normalized = value.replace(/\s+/g, ' ').trim();
  return normalized.length <= limit ? normalized : `${normalized.slice(0, limit - 3)}...`;
}

function stringifyId(value: unknown) {
  if (value === null || value === undefined) return undefined;
  return String(value);
}

function extractToolName(parsedPayload: any) {
  const toolName = parsedPayload?.params?.name;
  return typeof toolName === 'string' && toolName.length > 0 ? toolName : undefined;
}

function extractStdioPreview(parsedPayload: any, kind: DecodedStdioMessage['kind']) {
  if (!parsedPayload || typeof parsedPayload !== 'object') return undefined;
  if (kind === 'request' || kind === 'notification') {
    const args = parsedPayload.params?.arguments;
    if (typeof args?.text === 'string' && args.text.length > 0) return truncateText(args.text);
    const method = parsedPayload.method;
    if (method === 'tools/call' && typeof parsedPayload.params?.name === 'string') return parsedPayload.params.name;
    if (method === 'tools/list') return 'list tools';
    if (method === 'initialize') return parsedPayload.params?.clientInfo?.name || 'initialize';
  }
  if (kind === 'response') {
    const content = parsedPayload.result?.content;
    if (Array.isArray(content) && typeof content[0]?.text === 'string') return truncateText(content[0].text);
    if (Array.isArray(parsedPayload.result?.tools)) return `${parsedPayload.result.tools.length} tools`;
    if (typeof parsedPayload.result?.protocolVersion === 'string') return parsedPayload.result.protocolVersion;
  }
  if (kind === 'error' && typeof parsedPayload.error?.message === 'string') return truncateText(parsedPayload.error.message);
  return undefined;
}

export function isStdioSource(source: string) {
  return String(source || '').toLowerCase().trim() === 'stdio';
}

export function decodeStdioMessage(data: any): DecodedStdioMessage {
  const rawPayload = typeof data?.data === 'string' ? data.data : typeof data?.payload === 'string' ? data.payload : '';
  const parsedPayload = safeJsonParse(rawPayload);
  const direction = String(data?.direction || data?.stream || '').toUpperCase();
  const fdRole = String(data?.fd_role || data?.fdRole || data?.stream || (data?.fd !== undefined ? `fd ${data.fd}` : 'stdio'));
  const fdTarget = String(data?.fd_target || data?.fdTarget || '');
  const fd = typeof data?.fd === 'number' ? data.fd : null;
  const length = Number(data?.len || data?.size || 0);
  const truncated = Boolean(data?.truncated);
  let kind: DecodedStdioMessage['kind'] = rawPayload.trim() ? 'text' : 'unknown';
  let method: string | undefined;
  let id: string | undefined;
  if (parsedPayload && typeof parsedPayload === 'object') {
    method = typeof parsedPayload.method === 'string' ? parsedPayload.method : undefined;
    id = stringifyId(parsedPayload.id);
    if (method) kind = id ? 'request' : 'notification';
    else if (parsedPayload.result !== undefined) kind = 'response';
    else if (parsedPayload.error !== undefined) kind = 'error';
    else kind = 'unknown';
  }
  const toolName = extractToolName(parsedPayload);
  const preview = parsedPayload ? extractStdioPreview(parsedPayload, kind) : truncateText(rawPayload);
  const title = kind === 'request'
    ? method === 'tools/call' && toolName ? `tools/call ${toolName}` : method ? `request ${method}` : 'stdio request'
    : kind === 'notification'
      ? method ? `notification ${method}` : 'stdio notification'
      : kind === 'response'
        ? id ? `response #${id}` : 'stdio response'
        : kind === 'error'
          ? id ? `error #${id}` : 'stdio error'
          : kind === 'text' ? 'stdio text' : 'stdio event';
  const directionLabel = direction || 'STDIO';
  const role = fdRole || 'fd';
  let summary = `${directionLabel} ${role}`;
  if (kind === 'request' || kind === 'notification') {
    summary = `${directionLabel} ${role} ${method || 'message'}${toolName && method === 'tools/call' ? ` ${toolName}` : ''}${preview && preview !== toolName ? ` · ${preview}` : ''}`;
  } else if (kind === 'response' || kind === 'error') {
    summary = `${directionLabel} ${role} ${kind}${id ? ` #${id}` : ''}${preview ? ` · ${preview}` : ''}`;
  } else if (preview) {
    summary = `${directionLabel} ${role} · ${preview}`;
  }
  return { direction, fdRole, fdTarget, fd, length, truncated, rawPayload, parsedPayload, kind, method, id, toolName, preview, title, summary };
}

export function formatStdioExpandedContent(decoded: DecodedStdioMessage) {
  const sections = [`Direction: ${decoded.direction || 'UNKNOWN'}`, `FD Role: ${decoded.fdRole || 'unknown'}`];
  if (decoded.fd !== null) sections.push(`FD: ${decoded.fd}`);
  if (decoded.fdTarget) sections.push(`Target: ${decoded.fdTarget}`);
  sections.push(`Kind: ${decoded.kind}`);
  if (decoded.method) sections.push(`Method: ${decoded.method}`);
  if (decoded.toolName) sections.push(`Tool: ${decoded.toolName}`);
  if (decoded.id) sections.push(`Message ID: ${decoded.id}`);
  sections.push(`Length: ${decoded.length}`);
  sections.push(`Truncated: ${decoded.truncated ? 'yes' : 'no'}`);
  const payload = decoded.parsedPayload !== null ? JSON.stringify(decoded.parsedPayload, null, 2) : decoded.rawPayload;
  return `${sections.join('\n')}\n\nPayload\n-------\n${payload}`;
}

function determineParsedType(event: AgentSightEvent): ParsedAgentSightEventType {
  const source = event.source.toLowerCase();
  const data = event.data;
  const typeText = `${event.eventType} ${readAny(data, ['type', 'event'], '')}`.toLowerCase();
  if (source === 'system' || typeText.includes('system')) return 'system';
  if (isStdioSource(source)) return 'stdio';
  if (isPromptEvent(data)) return 'prompt';
  if (isResponseEvent(source, data)) return 'response';
  if (source === 'file' || isFileData(data)) return 'file';
  if (source === 'process' || isProcessData(data)) return 'process';
  if (source === 'policy' || typeText.includes('alert') || typeText.includes('policy')) return 'policy';
  if (source === 'agent') return 'agent';
  return 'ssl';
}

function isPromptEvent(data: any) {
  return Boolean(data.model || data.messages || data.prompt || data.inputs || data.query || (data.method === 'POST' && data.message_type === 'request' && (String(data.path || data.url || '').includes('/v1/') || String(data.path || data.url || '').includes('/api/'))));
}

function isResponseEvent(source: string, data: any) {
  return Boolean(data.choices || data.completion || data.response || data.sse_events || data.delta || data.content_block || (source === 'sse_processor' && (data.sse_event || data.sse_data_digest)) || (data.message_type === 'response' && (data.model || data.usage || data.status_code)));
}

function isFileData(data: any) {
  return data.fd !== undefined || ['open', 'read', 'write', 'close'].includes(String(data.operation || '').toLowerCase()) || String(data.event || '').includes('FILE_') || data.filepath !== undefined || data.path !== undefined && data.operation !== undefined;
}

function isProcessData(data: any) {
  return data.exec !== undefined || data.exit !== undefined || ['EXEC', 'EXIT', 'FORK', 'CLONE', 'PROCESS'].includes(String(data.event || '').toUpperCase()) || data.ppid !== undefined || data.parent_pid !== undefined;
}

function contentString(value: any) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function parsePromptEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const data = event.data;
  let model = data.model || data.vendor || 'AI Request';
  let displayData = data;
  if (data.body && typeof data.body === 'string') {
    const parsedBody = safeJsonParse(data.body);
    if (parsedBody) {
      if (parsedBody.model) model = parsedBody.model;
      displayData = { ...data, body: parsedBody };
    }
  }
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: 'prompt',
    title: `${data.method || 'POST'} ${model}`,
    content: contentString(displayData),
    metadata: { ...data, model, method: data.method || 'POST', url: `${data.host || ''}${data.path || data.url || ''}`, raw: data, original_source: event.source },
  };
}

function parseResponseEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const data = event.data;
  let model = data.model || data.vendor || 'AI Response';
  if (Array.isArray(data.sse_events)) {
    for (const sseEvent of data.sse_events) {
      if (sseEvent.parsed_data?.message?.model) {
        model = sseEvent.parsed_data.message.model;
        break;
      }
    }
  }
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: 'response',
    title: model,
    content: contentString(data.body || data),
    metadata: { ...data, model, raw: data, original_source: event.source },
  };
}

function parseGenericEvent(event: AgentSightEvent, type: ParsedAgentSightEventType): ParsedAgentSightEvent {
  return {
    id: event.id,
    timestamp: event.timestamp,
    type,
    title: event.title,
    content: contentString(event.data),
    metadata: { ...event.data, original_source: event.source, event_type: event.eventType },
  };
}

function parseStdioEvent(event: AgentSightEvent): ParsedAgentSightEvent {
  const decoded = decodeStdioMessage(event.data);
  return {
    id: event.id,
    timestamp: event.timestamp,
    type: 'stdio',
    title: decoded.title,
    content: formatStdioExpandedContent(decoded),
    metadata: { ...event.data, original_source: event.source, stdio_kind: decoded.kind, rpc_method: decoded.method, rpc_id: decoded.id, tool_name: decoded.toolName, summary: decoded.summary, parsed_payload: decoded.parsedPayload },
  };
}

export function parseAgentSightEvent(event: AgentSightEvent): ParsedAgentSightEvent | null {
  const type = determineParsedType(event);
  if (type === 'system') return null;
  if (type === 'prompt') return parsePromptEvent(event);
  if (type === 'response') return parseResponseEvent(event);
  if (type === 'stdio') return parseStdioEvent(event);
  return parseGenericEvent(event, type);
}

function extractPromptContent(data: any) {
  if (data?.body && typeof data.body === 'string') return safeJsonParse(data.body);
  if (data?.messages || data?.prompt) return data;
  return null;
}

function formatPromptForDiff(obj: any) {
  if (!obj) return '';
  if (Array.isArray(obj.messages)) {
    return obj.messages.map((msg: any, index: number) => {
      const role = msg.role || 'unknown';
      const content = msg.content;
      if (Array.isArray(content)) {
        const text = content.filter(item => item.type === 'text').map(item => item.text).join('\n');
        return `[${index}] ${String(role).toUpperCase()}:\n${text}`;
      }
      return `[${index}] ${String(role).toUpperCase()}: ${typeof content === 'string' ? content : JSON.stringify(content)}`;
    }).join('\n\n---\n\n');
  }
  return JSON.stringify(obj, null, 2);
}

function generateSimpleDiff(oldText: string, newText: string) {
  if (oldText === newText) return 'No changes detected';
  const oldLines = oldText.split('\n');
  const newLines = newText.split('\n');
  const max = Math.max(oldLines.length, newLines.length);
  const out: string[] = [];
  for (let index = 0; index < max; index += 1) {
    const oldLine = oldLines[index];
    const newLine = newLines[index];
    if (oldLine === newLine) {
      if (out.length < 12) out.push(`  ${oldLine || ''}`);
      continue;
    }
    if (oldLine !== undefined) out.push(`- ${oldLine}`);
    if (newLine !== undefined) out.push(`+ ${newLine}`);
    if (out.length > 80) {
      out.push('  ...');
      break;
    }
  }
  return out.join('\n');
}

function comparePrompts(oldPrompt: any, newPrompt: any) {
  const oldContent = extractPromptContent(oldPrompt);
  const newContent = extractPromptContent(newPrompt);
  if (!oldContent || !newContent) return { diff: 'Unable to extract prompt content', summary: 'Unable to compare prompts', hasChanges: false };
  const oldText = formatPromptForDiff(oldContent);
  const newText = formatPromptForDiff(newContent);
  const hasChanges = oldText !== newText;
  return { diff: generateSimpleDiff(oldText, newText), summary: hasChanges ? 'Prompt changed from previous request' : 'No changes detected', hasChanges };
}

export function buildProcessTree(events: AgentSightEvent[]): AgentSightProcessNode[] {
  const processMap = new Map<number, AgentSightProcessNode>();
  const eventsByPid = new Map<number, ParsedAgentSightEvent[]>();
  const promptHistoryByPid = new Map<number, ParsedAgentSightEvent[]>();

  events.slice().sort((a, b) => a.timestamp - b.timestamp).forEach(event => {
    if (event.source === 'system' || event.pid === 0) return;
    if (!processMap.has(event.pid)) {
      processMap.set(event.pid, { pid: event.pid, ppid: event.ppid, comm: event.comm || 'unknown', children: [], events: [], timeline: [] });
    }
    const node = processMap.get(event.pid)!;
    if (!node.ppid && event.ppid) node.ppid = event.ppid;
    const parsed = parseAgentSightEvent(event);
    if (!parsed) return;
    if (parsed.type === 'prompt') {
      const history = promptHistoryByPid.get(event.pid) || [];
      if (history.length > 0) {
        const previous = history[history.length - 1];
        parsed.promptDiff = { ...comparePrompts(previous.metadata.raw, event.data), previousPromptId: previous.id };
      }
      history.push(parsed);
      if (history.length > 10) history.shift();
      promptHistoryByPid.set(event.pid, history);
    }
    if (!eventsByPid.has(event.pid)) eventsByPid.set(event.pid, []);
    eventsByPid.get(event.pid)!.push(parsed);
  });

  eventsByPid.forEach((parsedEvents, pid) => {
    const process = processMap.get(pid);
    if (process) process.events = parsedEvents.sort((a, b) => a.timestamp - b.timestamp);
  });

  const childProcesses = new Set<number>();
  processMap.forEach((process, pid) => {
    if (process.ppid && processMap.has(process.ppid)) {
      processMap.get(process.ppid)!.children.push(process);
      childProcesses.add(pid);
    }
  });

  processMap.forEach(process => {
    const timeline: AgentSightTimelineItem[] = process.events.map(event => ({ type: 'event', timestamp: event.timestamp, event }));
    process.children.forEach(child => timeline.push({ type: 'process', timestamp: getEarliestTimestamp(child), process: child }));
    process.timeline = timeline.sort((a, b) => a.timestamp - b.timestamp);
  });

  return Array.from(processMap.values()).filter(process => !childProcesses.has(process.pid)).sort((a, b) => getEarliestTimestamp(a) - getEarliestTimestamp(b));
}

function getEarliestTimestamp(process: AgentSightProcessNode): number {
  const candidates: number[] = [];
  if (process.events.length > 0) candidates.push(process.events[0].timestamp);
  process.children.forEach(child => candidates.push(getEarliestTimestamp(child)));
  return candidates.length ? Math.min(...candidates) : 0;
}

export function createDefaultProcessFilters(): AgentSightProcessFilters {
  return { eventTypes: [], models: [], sources: [], commands: [], searchText: '', timeRange: {} };
}

export function extractProcessFilterOptions(events: AgentSightEvent[]): AgentSightFilterOptions {
  const eventTypes = new Set<string>();
  const models = new Set<string>();
  const sources = new Set<string>();
  const commands = new Set<string>();
  events.forEach(event => {
    const parsed = parseAgentSightEvent(event);
    if (!parsed) return;
    eventTypes.add(parsed.type);
    sources.add(event.source);
    if (event.comm) commands.add(event.comm);
    const model = parsed.metadata?.model;
    if (model && model !== 'Unknown Model') models.add(model);
  });
  return {
    eventTypes: Array.from(eventTypes).sort(),
    models: Array.from(models).sort(),
    sources: Array.from(sources).sort(),
    commands: Array.from(commands).sort(),
  };
}

function parsedEventMatchesFilters(event: ParsedAgentSightEvent, process: AgentSightProcessNode, filters: AgentSightProcessFilters) {
  if (filters.eventTypes.length > 0 && !filters.eventTypes.includes(event.type)) return false;
  const source = String(event.metadata.original_source || '');
  if (filters.sources.length > 0 && !filters.sources.includes(source)) return false;
  if (filters.commands.length > 0 && !filters.commands.includes(process.comm)) return false;
  if (filters.models.length > 0 && !filters.models.includes(event.metadata?.model)) return false;
  if (filters.timeRange.start && event.timestamp < filters.timeRange.start) return false;
  if (filters.timeRange.end && event.timestamp > filters.timeRange.end) return false;
  if (filters.searchText) {
    const term = filters.searchText.toLowerCase();
    const searchable = [event.title, event.content, process.comm, source, event.metadata?.model, JSON.stringify(event.metadata)].filter(Boolean).join(' ').toLowerCase();
    if (!searchable.includes(term)) return false;
  }
  return true;
}

export function filterProcessTree(processTree: AgentSightProcessNode[], filters: AgentSightProcessFilters): AgentSightProcessNode[] {
  return processTree.map(process => {
    const events = process.events.filter(event => parsedEventMatchesFilters(event, process, filters));
    const children = filterProcessTree(process.children, filters);
    const timeline: AgentSightTimelineItem[] = [
      ...events.map(event => ({ type: 'event' as const, timestamp: event.timestamp, event })),
      ...children.map(child => ({ type: 'process' as const, timestamp: getEarliestTimestamp(child), process: child })),
    ].sort((a, b) => a.timestamp - b.timestamp);
    if (events.length > 0 || children.length > 0) return { ...process, events, children, timeline };
    return null;
  }).filter((process): process is AgentSightProcessNode => process !== null);
}

export function getTotalEventCount(processTree: AgentSightProcessNode[]): number {
  return processTree.reduce((total, process) => total + process.events.length + getTotalEventCount(process.children), 0);
}

export function parsedTypeColor(type: ParsedAgentSightEventType) {
  switch (type) {
    case 'prompt': return 'blue';
    case 'response': return 'green';
    case 'ssl': return 'orange';
    case 'file': return 'cyan';
    case 'process': return 'purple';
    case 'stdio': return 'geekblue';
    case 'policy': return 'red';
    case 'agent': return 'magenta';
    default: return 'default';
  }
}

export function buildParsedEventPreview(event: ParsedAgentSightEvent) {
  if (event.promptDiff?.summary) return `📝 ${event.promptDiff.summary}`;
  const content = event.content || event.title;
  return content.replace(/\s+/g, ' ').slice(0, 180) + (content.length > 180 ? '...' : '');
}
