import { payloadSources } from "./constants";
import {
  eventTypeName,
  firstNonEmpty,
  parseTimestampMs,
  readAny,
  stableID,
  toNumber,
} from "./shared";
import { decodeStdioMessage } from "./stdio";
import type { AgentSightEvent, AgentSightEventRecord } from "./types";

function getEnvelopePayload(envelope: Record<string, any> | undefined) {
  if (!envelope) return { payload: undefined, source: "", eventType: "" };
  for (const entry of payloadSources) {
    for (const key of entry.keys) {
      if (envelope[key] !== undefined && envelope[key] !== null) {
        return {
          payload: envelope[key],
          source: entry.source,
          eventType: entry.eventType,
        };
      }
    }
  }
  return { payload: undefined, source: "", eventType: "" };
}

function normalizeSource(
  source: string,
  eventType: string,
  legacyType: string,
  data: Record<string, any>,
) {
  const sourceLower = source.toLowerCase();
  const typeLower =
    `${eventType} ${legacyType} ${readAny(data, ["type"], "")}`.toLowerCase();
  if (sourceLower) return sourceLower;
  if (typeLower.includes("stdio") || typeLower.includes("mcp")) return "stdio";
  if (typeLower.includes("system")) return "system";
  if (
    typeLower.includes("tls") ||
    typeLower.includes("http_request") ||
    typeLower.includes("http_response")
  )
    return "ssl";
  if (typeLower.includes("http")) return "http_parser";
  if (typeLower.includes("sse")) return "sse_processor";
  if (
    typeLower.includes("process") ||
    typeLower.includes("exec") ||
    typeLower.includes("fork") ||
    typeLower.includes("clone") ||
    typeLower.includes("exit")
  )
    return "process";
  if (
    typeLower.includes("open") ||
    typeLower.includes("file") ||
    typeLower.includes("write") ||
    typeLower.includes("read")
  )
    return "file";
  if (
    typeLower.includes("policy") ||
    typeLower.includes("alert") ||
    typeLower.includes("semantic")
  )
    return "policy";
  if (typeLower.includes("wrapper") || typeLower.includes("hook"))
    return "agent";
  return "unknown";
}

function normalizeTLSLikeData(
  payload: Record<string, any>,
  legacyEvent?: Record<string, any>,
) {
  const method = firstNonEmpty(
    readAny(payload, ["method"]),
    readAny(legacyEvent, ["path", "Path"]),
  );
  const url = firstNonEmpty(
    readAny(payload, ["url"]),
    readAny(legacyEvent, ["extra_path", "extraPath", "ExtraPath"]),
  );
  const host = firstNonEmpty(
    readAny(payload, ["host"]),
    readAny(legacyEvent, [
      "http_host",
      "httpHost",
      "HttpHost",
      "net_endpoint",
      "netEndpoint",
      "NetEndpoint",
    ]),
  );
  const status = toNumber(
    readAny(payload, ["status", "status_code", "statusCode"]),
    0,
  );
  const direction = firstNonEmpty(
    readAny(payload, ["direction"]),
    readAny(legacyEvent, ["net_direction", "netDirection", "NetDirection"]),
  );
  const bodySize = toNumber(readAny(payload, ["body_size", "bodySize"]), 0);
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
    content_type: readAny(payload, ["content_type", "contentType"], ""),
    message_type: status
      ? "response"
      : method
        ? "request"
        : readAny(payload, ["message_type", "messageType"], ""),
    redaction_state: readAny(
      payload,
      ["redaction_state", "redactionState"],
      "",
    ),
    raw_available: readAny(payload, ["raw_available", "rawAvailable"], false),
    prompt_digest: readAny(payload, ["prompt_digest", "promptDigest"], ""),
    prompt_len: readAny(payload, ["prompt_len", "promptLen"], 0),
    message_role: readAny(payload, ["message_role", "messageRole"], ""),
    vendor: readAny(payload, ["vendor"], ""),
  };
}

function normalizeSystemData(
  payload: Record<string, any>,
  legacyEvent?: Record<string, any>,
) {
  const cpu = toNumber(readAny(payload, ["cpu_percent", "cpuPercent"]), 0);
  const memoryBytes = toNumber(
    readAny(
      payload,
      ["memory_bytes", "memoryBytes"],
      readAny(legacyEvent, ["bytes", "Bytes"], 0),
    ),
    0,
  );
  const threads = toNumber(readAny(payload, ["threads"]), 0);
  const children = toNumber(readAny(payload, ["children"]), 0);
  return {
    ...payload,
    type: "system_metrics",
    cpu: { percent: cpu, cores: readAny(payload, ["cores"], 0) },
    memory: {
      rss_mb: Math.round(memoryBytes / 1024 / 1024),
      vsz_mb: Math.round(memoryBytes / 1024 / 1024),
    },
    process: {
      threads,
      children,
      state: readAny(payload, ["process_state", "processState"], ""),
    },
    alert: Boolean(readAny(payload, ["alert"], "")),
  };
}

function normalizeProcessData(
  payload: Record<string, any>,
  event: AgentSightEventRecord,
  legacyEvent?: Record<string, any>,
) {
  const phase = firstNonEmpty(
    readAny(payload, ["phase"]),
    readAny(legacyEvent, ["type", "Type"]),
  );
  return {
    ...payload,
    event: String(phase || "process").toUpperCase(),
    pid: readAny(
      event,
      ["pid", "Pid"],
      readAny(legacyEvent, ["pid", "Pid"], 0),
    ),
    ppid: readAny(
      event,
      ["ppid", "Ppid"],
      readAny(
        payload,
        ["parent_pid", "parentPid"],
        readAny(legacyEvent, ["ppid", "Ppid"], 0),
      ),
    ),
    comm: readAny(
      event,
      ["comm", "Comm"],
      readAny(legacyEvent, ["comm", "Comm"], ""),
    ),
    child_pid: readAny(payload, ["child_pid", "childPid"], 0),
    target_pid: readAny(payload, ["target_pid", "targetPid"], 0),
    old_pid: readAny(payload, ["old_pid", "oldPid"], 0),
  };
}

function normalizeFileData(
  payload: Record<string, any>,
  legacyEvent?: Record<string, any>,
) {
  return {
    ...payload,
    operation: firstNonEmpty(
      readAny(payload, ["operation"]),
      readAny(legacyEvent, ["type", "Type"]),
    ),
    path: firstNonEmpty(
      readAny(payload, ["path"]),
      readAny(legacyEvent, ["path", "Path"]),
    ),
    filepath: firstNonEmpty(
      readAny(payload, ["path"]),
      readAny(legacyEvent, ["path", "Path"]),
    ),
    size: readAny(
      payload,
      ["bytes"],
      readAny(legacyEvent, ["bytes", "Bytes"], 0),
    ),
  };
}

function normalizePayloadData(
  source: string,
  payload: Record<string, any>,
  normalized: AgentSightEventRecord,
  legacyEvent?: Record<string, any>,
) {
  if (
    source === "ssl" ||
    source === "http_parser" ||
    source === "sse_processor"
  )
    return normalizeTLSLikeData(payload, legacyEvent);
  if (source === "system") return normalizeSystemData(payload, legacyEvent);
  if (source === "process")
    return normalizeProcessData(payload, normalized, legacyEvent);
  if (source === "file") return normalizeFileData(payload, legacyEvent);
  if (source === "policy") {
    return {
      ...payload,
      event: firstNonEmpty(
        readAny(payload, ["decision"]),
        readAny(legacyEvent, ["decision", "Decision"]),
        "policy",
      ),
      reason: firstNonEmpty(
        readAny(payload, ["reason"]),
        readAny(legacyEvent, ["extra_info", "extraInfo", "ExtraInfo"]),
      ),
    };
  }
  return { ...payload };
}

function normalizeReferenceEvent(
  value: any,
  index: number,
): AgentSightEvent | null {
  if (!value || typeof value !== "object") return null;
  if (
    value.data === undefined ||
    value.timestamp === undefined ||
    value.source === undefined
  )
    return null;
  const timestamp = parseTimestampMs(value.timestamp, Date.now() + index);
  const data =
    value.data && typeof value.data === "object"
      ? value.data
      : { value: value.data };
  const source = normalizeSource(
    String(value.source || ""),
    String(data.event_type || data.type || ""),
    String(data.type || ""),
    data,
  );
  const eventType = eventTypeName(
    data.event_type,
    String(data.type || source).toUpperCase(),
  );
  const pid = toNumber(value.pid, toNumber(data.pid, 0));
  const ppid = toNumber(value.ppid, toNumber(data.ppid, 0));
  const comm = String(firstNonEmpty(value.comm, data.comm, "—"));
  const title = buildEventTitle(source, eventType, data, value);
  return {
    id: String(
      firstNonEmpty(value.id, stableID("ref", [source, timestamp, pid, index])),
    ),
    timestamp,
    source,
    rawSource: String(value.source || source),
    pid,
    ppid,
    comm,
    eventType,
    traceId: String(
      firstNonEmpty(value.trace_id, value.traceId, data.trace_id, data.traceId),
    ),
    spanId: String(
      firstNonEmpty(value.span_id, value.spanId, data.span_id, data.spanId),
    ),
    redactionState: String(
      firstNonEmpty(data.redaction_state, data.redactionState),
    ),
    title,
    data,
    raw: value,
  };
}

function buildEventTitle(
  source: string,
  eventType: string,
  data: Record<string, any>,
  raw?: any,
) {
  if (source === "stdio") return decodeStdioMessage(data).summary;
  if (
    source === "ssl" ||
    source === "http_parser" ||
    source === "sse_processor"
  ) {
    const method = readAny(data, ["method"], "");
    const host = readAny(data, ["host"], "");
    const url = readAny(data, ["url", "path"], "");
    const status = readAny(data, ["status", "status_code", "statusCode"], "");
    const sse = readAny(data, ["sse_event", "sseEvent"], "");
    if (sse)
      return `SSE ${sse} ${readAny(data, ["sse_data_digest", "sseDataDigest"], "")}`.trim();
    if (method) return `${method} ${host}${url}`.trim();
    if (status) return `${status} ${host}${url}`.trim();
    if (readAny(data, ["body", "raw_hex_dump", "rawHexDump"], ""))
      return String(
        readAny(data, ["body", "raw_hex_dump", "rawHexDump"], ""),
      ).slice(0, 120);
  }
  if (source === "file")
    return `${readAny(data, ["operation", "event"], eventType)} ${readAny(data, ["path", "filepath"], "")}`.trim();
  if (source === "process")
    return `${readAny(data, ["event", "phase"], eventType)} ${readAny(data, ["comm"], readAny(raw, ["comm"], ""))}`.trim();
  if (source === "system")
    return `${toNumber(readAny(data, ["cpu_percent", "cpuPercent", "cpu.percent"]), 0).toFixed(1)}% CPU · ${readAny(data, ["process_state", "processState"], "")}`;
  if (source === "policy")
    return String(
      firstNonEmpty(
        readAny(data, ["reason"]),
        readAny(data, ["decision"]),
        eventType,
      ),
    );
  return String(
    firstNonEmpty(
      readAny(data, [
        "url",
        "host",
        "method",
        "phase",
        "operation",
        "decision",
        "path",
        "netEndpoint",
      ]),
      eventType,
      "—",
    ),
  );
}

function normalizeRecordLike(
  value: AgentSightEventRecord,
  index: number,
): AgentSightEvent | null {
  const reference = normalizeReferenceEvent(value, index);
  if (reference) return reference;

  const envelope = readAny<Record<string, any> | undefined>(value, [
    "Envelope",
    "envelope",
  ]);
  const legacyEvent = readAny<Record<string, any> | undefined>(
    value,
    ["Event", "event"],
    readAny(envelope, ["legacy_event", "legacyEvent"]),
  );
  const payloadInfo = getEnvelopePayload(envelope);
  const payload =
    payloadInfo.payload && typeof payloadInfo.payload === "object"
      ? payloadInfo.payload
      : {};
  const rawSource = String(
    firstNonEmpty(
      readAny(envelope, ["source"]),
      readAny(legacyEvent, ["source", "Source"]),
      payloadInfo.source,
    ),
  );
  const fallbackType = String(
    firstNonEmpty(
      readAny(legacyEvent, ["type", "Type"]),
      payloadInfo.eventType,
      rawSource,
    ),
  ).toUpperCase();
  const eventType = eventTypeName(
    readAny(
      envelope,
      ["event_type", "eventType"],
      readAny(legacyEvent, ["event_type", "eventType", "EventType"]),
    ),
    fallbackType,
  );
  const source = normalizeSource(
    payloadInfo.source,
    eventType,
    String(readAny(legacyEvent, ["type", "Type"], "")),
    payload,
  );
  const timestamp = parseTimestampMs(
    readAny(
      value,
      ["Timestamp", "timestamp"],
      readAny(envelope, ["timestamp_ns", "timestampNs"]),
    ),
    Date.now() + index,
  );
  const pid = toNumber(
    firstNonEmpty(
      readAny(envelope, ["pid"]),
      readAny(legacyEvent, ["pid", "Pid"]),
      readAny(payload, ["pid"]),
    ),
    0,
  );
  const ppid = toNumber(
    firstNonEmpty(
      readAny(envelope, ["ppid"]),
      readAny(legacyEvent, ["ppid", "Ppid"]),
      readAny(payload, ["ppid", "parent_pid", "parentPid"]),
    ),
    0,
  );
  const comm = String(
    firstNonEmpty(
      readAny(envelope, ["comm"]),
      readAny(legacyEvent, ["comm", "Comm"]),
      readAny(payload, ["comm"]),
      "—",
    ),
  );
  const normalizedShell: AgentSightEventRecord = { pid, ppid, comm };
  const data = normalizePayloadData(
    source,
    payload,
    normalizedShell,
    legacyEvent,
  );
  const traceId = String(
    firstNonEmpty(
      readAny(envelope, ["trace_id", "traceId"]),
      readAny(legacyEvent, ["trace_id", "traceId", "TraceId"]),
    ),
  );
  const spanId = String(
    firstNonEmpty(
      readAny(envelope, ["span_id", "spanId"]),
      readAny(legacyEvent, ["span_id", "spanId", "SpanId"]),
    ),
  );
  const redactionState = String(
    firstNonEmpty(
      readAny(data, ["redaction_state", "redactionState"]),
      readAny(payload, ["redaction_state", "redactionState"]),
    ),
  );
  return {
    id: String(
      firstNonEmpty(
        readAny(envelope, ["event_id", "eventId"]),
        stableID("evt", [source, timestamp, pid, eventType, traceId, index]),
      ),
    ),
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

export function normalizeAgentSightEvent(
  value: any,
  index = 0,
): AgentSightEvent | null {
  if (!value || typeof value !== "object") return null;
  if (
    value.schema_version ||
    value.schemaVersion ||
    value.legacy_event ||
    value.legacyEvent
  ) {
    return normalizeRecordLike({ Envelope: value }, index);
  }
  return normalizeRecordLike(value as AgentSightEventRecord, index);
}

export function normalizeAgentSightEvents(values: any[]): AgentSightEvent[] {
  if (!Array.isArray(values) || values.length === 0) return [];

  const seen = new Set<string>();
  const normalized: AgentSightEvent[] = [];

  for (let index = 0; index < values.length; index++) {
    const event = normalizeAgentSightEvent(values[index], index);
    if (!event) continue;

    const key =
      event.id ||
      stableID("dedupe", [
        event.source,
        event.timestamp,
        event.pid,
        event.title,
      ]);

    if (seen.has(key)) continue;
    seen.add(key);
    normalized.push(event);
  }

  return normalized.sort((a, b) => b.timestamp - a.timestamp);
}

// Merge multiple already-normalized, timestamp-descending streams without
// re-normalizing or re-sorting their unchanged members. This keeps hot TLS or
// system updates from forcing a full O(N log N) pass over every AgentSight
// source. Duplicate event IDs are removed across streams while merging.
export function mergeSortedAgentSightEvents(
  groups: readonly AgentSightEvent[][],
  limit = Number.MAX_SAFE_INTEGER,
): AgentSightEvent[] {
  if (groups.length === 0 || limit <= 0) return [];

  const positions = new Array<number>(groups.length).fill(0);
  const seen = new Set<string>();
  const merged: AgentSightEvent[] = [];

  while (merged.length < limit) {
    let bestGroup = -1;
    let bestEvent: AgentSightEvent | undefined;

    for (let groupIndex = 0; groupIndex < groups.length; groupIndex++) {
      const candidate = groups[groupIndex][positions[groupIndex]];
      if (!candidate) continue;
      if (!bestEvent || candidate.timestamp > bestEvent.timestamp) {
        bestEvent = candidate;
        bestGroup = groupIndex;
      }
    }

    if (bestGroup < 0 || !bestEvent) break;
    positions[bestGroup]++;

    const key =
      bestEvent.id ||
      stableID("dedupe", [
        bestEvent.source,
        bestEvent.timestamp,
        bestEvent.pid,
        bestEvent.title,
      ]);
    if (seen.has(key)) continue;
    seen.add(key);
    merged.push(bestEvent);
  }

  return merged;
}
