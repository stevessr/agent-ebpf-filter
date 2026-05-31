import {
  decodeStdioMessage,
  formatBytes,
  formatDuration,
  type ProcessedAgentSightEvent,
} from "./agentsight";

export type AgentSightFlameMetric = "count" | "bytes" | "duration" | "risk";
export type AgentSightFlameDimensionPreset =
  | "execution"
  | "process"
  | "conversation";

export interface AgentSightFlameMetrics {
  count: number;
  bytes: number;
  duration: number;
  explicitDuration: number;
  inferredDuration: number;
  risk: number;
}

export interface AgentSightFlameSegment {
  key: string;
  label: string;
  level: string;
}

export interface AgentSightFlameNode {
  id: string;
  key: string;
  label: string;
  level: string;
  depth: number;
  metrics: AgentSightFlameMetrics;
  children: AgentSightFlameNode[];
  dominantSource: string;
  dominantColor: string;
  eventCount: number;
  latestTimestamp?: number;
  representativeEvent?: ProcessedAgentSightEvent | null;
  sampleEvents: ProcessedAgentSightEvent[];
}

export interface AgentSightFlameRect {
  node: AgentSightFlameNode;
  x: number;
  y: number;
  width: number;
  height: number;
  value: number;
  percentOfRoot: number;
  percentOfParent: number;
}

export interface AgentSightFlameLayoutOptions {
  width?: number;
  rowHeight?: number;
  barHeight?: number;
  gap?: number;
  minSegmentPx?: number;
  maxChildrenPerParent?: number;
}

interface MutableAgentSightFlameNode extends AgentSightFlameNode {
  sourceWeights: Map<string, { count: number; color: string }>;
}

const ROOT_ID = "root";
const SAMPLE_EVENT_LIMIT = 5;
const DEFAULT_LAYOUT_WIDTH = 1200;
const DEFAULT_ROW_HEIGHT = 30;
const DEFAULT_BAR_HEIGHT = 24;
const DEFAULT_GAP = 1;
const DEFAULT_MIN_SEGMENT_PX = 1.2;
const DEFAULT_MAX_CHILDREN = 80;

const emptyMetrics = (): AgentSightFlameMetrics => ({
  count: 0,
  bytes: 0,
  duration: 0,
  explicitDuration: 0,
  inferredDuration: 0,
  risk: 0,
});

function createNode(
  segment: AgentSightFlameSegment,
  depth: number,
  parentId: string,
): MutableAgentSightFlameNode {
  const id =
    parentId === ROOT_ID
      ? `${ROOT_ID}/${segment.key}`
      : `${parentId}/${segment.key}`;
  return {
    id,
    key: segment.key,
    label: segment.label,
    level: segment.level,
    depth,
    metrics: emptyMetrics(),
    children: [],
    dominantSource: "",
    dominantColor: "#64748b",
    eventCount: 0,
    latestTimestamp: undefined,
    representativeEvent: null,
    sampleEvents: [],
    sourceWeights: new Map(),
  };
}

function sanitizeKey(value: string) {
  return value.replace(/\s+/g, " ").trim().slice(0, 160) || "unknown";
}

function segment(
  level: string,
  value: unknown,
  fallback: string,
): AgentSightFlameSegment {
  const label = sanitizeKey(String(value ?? "").trim() || fallback);
  return { level, label, key: `${level}:${label}` };
}

function readAny(obj: any, keys: string[]) {
  if (!obj || typeof obj !== "object") return undefined;
  for (const key of keys) {
    const value = obj[key];
    if (value !== undefined && value !== null && String(value).trim() !== "")
      return value;
  }
  return undefined;
}

function readEventField(event: ProcessedAgentSightEvent, keys: string[]) {
  return (
    readAny(event.data, keys) ??
    readAny(event.raw, keys) ??
    readAny(event.legacyEvent, keys) ??
    readAny(event.envelope, keys) ??
    readAny(event.raw?.Event, keys) ??
    readAny(event.raw?.event, keys) ??
    readAny(event.raw?.Envelope, keys) ??
    readAny(event.raw?.envelope, keys)
  );
}

function toNumber(value: unknown, fallback = 0) {
  if (typeof value === "number")
    return Number.isFinite(value) ? value : fallback;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }
  if (value && typeof value === "object") {
    const maybeLong = value as { low?: number; toNumber?: () => number };
    if (typeof maybeLong.toNumber === "function") return maybeLong.toNumber();
    if (typeof maybeLong.low === "number") return maybeLong.low;
  }
  return fallback;
}

function positiveNumber(value: unknown) {
  const parsed = toNumber(value, 0);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

function readDurationMs(event: ProcessedAgentSightEvent) {
  const durationNs = positiveNumber(
    firstNonEmpty(
      readEventField(event, ["duration_ns", "durationNs", "DurationNs"]),
      readEventField(event, [
        "elapsed_ns",
        "elapsedNs",
        "latency_ns",
        "latencyNs",
      ]),
    ),
  );
  if (durationNs > 0) return durationNs / 1_000_000;

  const latencyMs = positiveNumber(
    firstNonEmpty(
      readEventField(event, ["latency_ms", "latencyMs", "LatencyMs"]),
      readEventField(event, [
        "duration_ms",
        "durationMs",
        "DurationMs",
        "duration",
      ]),
      readEventField(event, ["elapsed_ms", "elapsedMs", "ElapsedMs"]),
    ),
  );
  if (latencyMs > 0) return latencyMs;

  const seconds = positiveNumber(
    firstNonEmpty(
      readEventField(event, [
        "duration_s",
        "durationSec",
        "duration_seconds",
        "durationSeconds",
      ]),
      readEventField(event, [
        "elapsed_s",
        "elapsedSec",
        "elapsed_seconds",
        "elapsedSeconds",
      ]),
    ),
  );
  return seconds > 0 ? seconds * 1000 : 0;
}

function firstNonEmpty(...values: unknown[]) {
  for (const value of values) {
    if (value !== undefined && value !== null && String(value).trim() !== "")
      return String(value).trim();
  }
  return "";
}

function compactLabel(value: string, limit = 96) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length <= limit
    ? normalized
    : `${normalized.slice(0, limit - 3)}...`;
}

function processLabel(event: ProcessedAgentSightEvent) {
  const comm =
    event.comm || String(readEventField(event, ["comm", "Comm"]) || "process");
  const pid = event.pid || toNumber(readEventField(event, ["pid", "Pid"]), 0);
  return pid ? `${comm}#${pid}` : comm;
}

function executionScopeLabel(event: ProcessedAgentSightEvent) {
  const agentRunId = firstNonEmpty(
    readEventField(event, ["agent_run_id", "agentRunId", "AgentRunId"]),
  );
  if (agentRunId) return `agent ${agentRunId}`;
  const conversationId = firstNonEmpty(
    readEventField(event, [
      "conversation_id",
      "conversationId",
      "ConversationId",
    ]),
  );
  if (conversationId) return `conversation ${conversationId}`;
  const traceId =
    event.traceId ||
    firstNonEmpty(readEventField(event, ["trace_id", "traceId", "TraceId"]));
  if (traceId) return `trace ${traceId}`;
  return "unscoped";
}

function conversationLabel(event: ProcessedAgentSightEvent) {
  return (
    firstNonEmpty(
      readEventField(event, [
        "conversation_id",
        "conversationId",
        "ConversationId",
      ]),
      event.traceId,
    ) || "conversation unknown"
  );
}

function turnLabel(event: ProcessedAgentSightEvent) {
  return (
    firstNonEmpty(
      readEventField(event, ["turn_id", "turnId", "TurnId"]),
      readEventField(event, ["task_id", "taskId", "TaskId"]),
    ) || "turn unknown"
  );
}

function toolLabel(event: ProcessedAgentSightEvent) {
  const explicit = firstNonEmpty(
    readEventField(event, ["tool_name", "toolName", "ToolName"]),
    readEventField(event, ["tool_call_id", "toolCallId", "ToolCallId"]),
  );
  if (explicit) return compactLabel(explicit, 72);
  if (event.source === "stdio") {
    const decoded = decodeStdioMessage(event.data);
    return compactLabel(
      decoded.toolName ||
        decoded.method ||
        decoded.title ||
        decoded.fdRole ||
        "stdio",
      72,
    );
  }
  return event.source || "source unknown";
}

function eventLabel(event: ProcessedAgentSightEvent) {
  return `${event.source || "unknown"}:${event.eventType || "event"}`;
}

function networkEndpoint(event: ProcessedAgentSightEvent) {
  const dstIp = firstNonEmpty(
    readEventField(event, ["dst_ip", "dstIp", "DstIp"]),
  );
  const dstPort = firstNonEmpty(
    readEventField(event, ["dst_port", "dstPort", "DstPort"]),
  );
  if (dstIp && dstPort) return `${dstIp}:${dstPort}`;
  return firstNonEmpty(
    readEventField(event, [
      "domain",
      "dns_name",
      "dnsName",
      "sni",
      "http_host",
      "httpHost",
      "host",
    ]),
    readEventField(event, ["net_endpoint", "netEndpoint", "NetEndpoint"]),
    dstIp,
  );
}

export function extractAgentSightFlameTarget(event: ProcessedAgentSightEvent) {
  const source = String(event.source || "").toLowerCase();
  if (source === "stdio") {
    const decoded = decodeStdioMessage(event.data);
    return compactLabel(
      decoded.toolName ||
        decoded.method ||
        decoded.preview ||
        decoded.summary ||
        decoded.title ||
        event.title,
      96,
    );
  }
  if (source === "file") {
    return compactLabel(
      firstNonEmpty(
        readEventField(event, [
          "path",
          "Path",
          "filepath",
          "filePath",
          "file_path",
        ]),
        readEventField(event, ["extra_path", "extraPath", "ExtraPath"]),
        event.title,
      ),
      120,
    );
  }
  if (
    source === "network" ||
    source === "ssl" ||
    source === "http_parser" ||
    source === "sse_processor"
  ) {
    const host = firstNonEmpty(
      readEventField(event, [
        "host",
        "domain",
        "dns_name",
        "dnsName",
        "sni",
        "http_host",
        "httpHost",
      ]),
    );
    const path = firstNonEmpty(readEventField(event, ["url", "path", "Path"]));
    const method = firstNonEmpty(readEventField(event, ["method"]));
    const endpoint = networkEndpoint(event);
    return compactLabel(
      firstNonEmpty(
        host && path
          ? `${method ? `${method} ` : ""}${host}${path.startsWith("/") ? path : `/${path}`}`
          : "",
        endpoint,
        path,
        event.title,
      ),
      120,
    );
  }
  if (source === "policy") {
    return compactLabel(
      firstNonEmpty(
        readEventField(event, ["decision", "Decision"]),
        readEventField(event, ["reason", "message", "rule", "policy"]),
        event.title,
      ),
      96,
    );
  }
  if (source === "system") {
    return event.pid ? processLabel(event) : "system-wide";
  }
  if (source === "process") {
    return compactLabel(
      firstNonEmpty(
        readEventField(event, ["event", "type", "state"]),
        readEventField(event, ["path", "Path"]),
        event.title,
      ),
      96,
    );
  }
  return compactLabel(event.title || "event", 96);
}

export function extractAgentSightFlameMetrics(
  event: ProcessedAgentSightEvent,
  inferredDurationMs = 0,
): AgentSightFlameMetrics {
  const bodyBytes = toNumber(
    firstNonEmpty(readEventField(event, ["body_size", "bodySize"])),
    0,
  );
  const directBytes = toNumber(
    firstNonEmpty(
      readEventField(event, [
        "size",
        "bytes",
        "len",
        "length",
        "net_bytes",
        "netBytes",
      ]),
    ),
    0,
  );
  const directionalBytes =
    toNumber(firstNonEmpty(readEventField(event, ["bytes_in", "bytesIn"])), 0) +
    toNumber(
      firstNonEmpty(readEventField(event, ["bytes_out", "bytesOut"])),
      0,
    );
  const bytes =
    event.source === "system"
      ? 0
      : bodyBytes || directionalBytes || directBytes;
  const explicitDuration = readDurationMs(event);
  const inferredDuration =
    explicitDuration > 0 ? 0 : Math.max(0, inferredDurationMs);
  const explicitRisk = toNumber(
    firstNonEmpty(readEventField(event, ["risk_score", "riskScore", "risk"])),
    0,
  );
  const decision = String(
    firstNonEmpty(
      readEventField(event, ["decision", "Decision"]),
      readEventField(event, ["type", "event"]),
      event.eventType,
    ),
  ).toLowerCase();
  const risk =
    explicitRisk > 0
      ? explicitRisk <= 1
        ? explicitRisk * 100
        : explicitRisk
      : decision.includes("alert")
        ? 100
        : decision.includes("block") || decision.includes("deny")
          ? 80
          : decision.includes("warn")
            ? 50
            : 0;
  return {
    count: 1,
    bytes,
    duration: explicitDuration || inferredDuration,
    explicitDuration,
    inferredDuration,
    risk,
  };
}

export function buildAgentSightFlamePath(
  event: ProcessedAgentSightEvent,
  preset: AgentSightFlameDimensionPreset,
): AgentSightFlameSegment[] {
  if (preset === "process") {
    return [
      segment("process", processLabel(event), "process unknown"),
      segment("source", event.source, "source unknown"),
      segment("event", eventLabel(event), "event unknown"),
      segment("target", extractAgentSightFlameTarget(event), "target unknown"),
    ];
  }
  if (preset === "conversation") {
    return [
      segment("conversation", conversationLabel(event), "conversation unknown"),
      segment("turn", turnLabel(event), "turn unknown"),
      segment("tool", toolLabel(event), "tool unknown"),
      segment("process", processLabel(event), "process unknown"),
      segment("target", extractAgentSightFlameTarget(event), "target unknown"),
    ];
  }
  return [
    segment("agent", executionScopeLabel(event), "unscoped"),
    segment("tool", toolLabel(event), "tool unknown"),
    segment("process", processLabel(event), "process unknown"),
    segment("event", eventLabel(event), "event unknown"),
    segment("target", extractAgentSightFlameTarget(event), "target unknown"),
  ];
}

function addMetrics(
  target: AgentSightFlameMetrics,
  delta: AgentSightFlameMetrics,
) {
  target.count += delta.count;
  target.bytes += delta.bytes;
  target.duration += delta.duration;
  target.explicitDuration += delta.explicitDuration;
  target.inferredDuration += delta.inferredDuration;
  target.risk += delta.risk;
}

function metricValue(
  metrics: AgentSightFlameMetrics,
  metric: AgentSightFlameMetric,
) {
  return metrics[metric] || 0;
}

function updateNode(
  node: MutableAgentSightFlameNode,
  event: ProcessedAgentSightEvent,
  metrics: AgentSightFlameMetrics,
) {
  addMetrics(node.metrics, metrics);
  node.eventCount += 1;
  if (!node.latestTimestamp || event.timestamp > node.latestTimestamp)
    node.latestTimestamp = event.timestamp;
  if (
    !node.representativeEvent ||
    metricValue(metrics, "risk") >
      metricValue(
        extractAgentSightFlameMetrics(node.representativeEvent),
        "risk",
      ) ||
    event.timestamp > (node.representativeEvent.timestamp || 0)
  ) {
    node.representativeEvent = event;
  }
  if (node.sampleEvents.length < SAMPLE_EVENT_LIMIT)
    node.sampleEvents.push(event);
  const currentSource = node.sourceWeights.get(event.source) || {
    count: 0,
    color: event.sourceColor || "#64748b",
  };
  currentSource.count += 1;
  node.sourceWeights.set(event.source, currentSource);
}

function finalizeNode(node: MutableAgentSightFlameNode): AgentSightFlameNode {
  let dominant = {
    source: node.dominantSource,
    count: 0,
    color: node.dominantColor,
  };
  node.sourceWeights.forEach((value, source) => {
    if (value.count > dominant.count)
      dominant = { source, count: value.count, color: value.color };
  });
  node.dominantSource = dominant.source || node.dominantSource || "unknown";
  node.dominantColor = dominant.color || node.dominantColor || "#64748b";
  node.children = node.children
    .map((child) => finalizeNode(child as MutableAgentSightFlameNode))
    .sort((a, b) => b.metrics.count - a.metrics.count);
  const { sourceWeights, ...finalNode } = node;
  return finalNode;
}

function eventDurationScope(event: ProcessedAgentSightEvent) {
  return (
    firstNonEmpty(
      readEventField(event, ["agent_run_id", "agentRunId", "AgentRunId"]),
      readEventField(event, [
        "conversation_id",
        "conversationId",
        "ConversationId",
      ]),
      event.traceId,
      readEventField(event, ["trace_id", "traceId", "TraceId"]),
      event.pid ? `${event.comm || "process"}#${event.pid}` : "",
      event.comm,
    ) || "global"
  );
}

function buildInferredDurationMap(events: ProcessedAgentSightEvent[]) {
  const groups = new Map<string, ProcessedAgentSightEvent[]>();
  events.forEach((event) => {
    if (!Number.isFinite(event.timestamp) || event.timestamp <= 0) return;
    const key = eventDurationScope(event);
    const group = groups.get(key) || [];
    group.push(event);
    groups.set(key, group);
  });

  const inferred = new Map<string, number>();
  groups.forEach((group) => {
    const sorted = group.slice().sort((a, b) => a.timestamp - b.timestamp);
    sorted.forEach((event, index) => {
      const next = sorted[index + 1];
      if (!next) return;
      const delta = next.timestamp - event.timestamp;
      if (delta > 0 && delta <= 30_000) inferred.set(event.id, delta);
    });
  });
  return inferred;
}

export function buildAgentSightFlameTree(
  events: ProcessedAgentSightEvent[],
  preset: AgentSightFlameDimensionPreset,
): AgentSightFlameNode {
  const root: MutableAgentSightFlameNode = {
    id: ROOT_ID,
    key: ROOT_ID,
    label: "All events",
    level: "root",
    depth: 0,
    metrics: emptyMetrics(),
    children: [],
    dominantSource: "all",
    dominantColor: "#64748b",
    eventCount: 0,
    latestTimestamp: undefined,
    representativeEvent: null,
    sampleEvents: [],
    sourceWeights: new Map(),
  };

  const inferredDurations = buildInferredDurationMap(events);
  events.forEach((event) => {
    const metrics = extractAgentSightFlameMetrics(
      event,
      inferredDurations.get(event.id) || 0,
    );
    updateNode(root, event, metrics);
    let current = root;
    buildAgentSightFlamePath(event, preset).forEach((pathSegment, index) => {
      let child = current.children.find(
        (item) => item.key === pathSegment.key,
      ) as MutableAgentSightFlameNode | undefined;
      if (!child) {
        child = createNode(pathSegment, index + 1, current.id);
        current.children.push(child);
      }
      updateNode(child, event, metrics);
      current = child;
    });
  });

  return finalizeNode(root);
}

function cloneSyntheticOther(
  children: AgentSightFlameNode[],
  parent: AgentSightFlameNode,
  metric: AgentSightFlameMetric,
): AgentSightFlameNode {
  const other = children.reduce<AgentSightFlameNode>(
    (node, child) => {
      addMetrics(node.metrics, child.metrics);
      node.eventCount += child.eventCount;
      if (
        !node.latestTimestamp ||
        (child.latestTimestamp || 0) > node.latestTimestamp
      )
        node.latestTimestamp = child.latestTimestamp;
      if (!node.representativeEvent && child.representativeEvent)
        node.representativeEvent = child.representativeEvent;
      child.sampleEvents.forEach((event) => {
        if (node.sampleEvents.length < SAMPLE_EVENT_LIMIT)
          node.sampleEvents.push(event);
      });
      return node;
    },
    {
      id: `${parent.id}/other-${metric}`,
      key: `other-${metric}`,
      label: "Other",
      level: "other",
      depth: parent.depth + 1,
      metrics: emptyMetrics(),
      children: [],
      dominantSource: parent.dominantSource,
      dominantColor: "#94a3b8",
      eventCount: 0,
      latestTimestamp: undefined,
      representativeEvent: null,
      sampleEvents: [],
    },
  );
  return other;
}

function visibleChildren(
  node: AgentSightFlameNode,
  metric: AgentSightFlameMetric,
  parentWidth: number,
  minSegmentPx: number,
  maxChildren: number,
) {
  const total = Math.max(metricValue(node.metrics, metric), 0);
  if (total <= 0) return [];
  const sorted = node.children
    .slice()
    .sort(
      (a, b) => metricValue(b.metrics, metric) - metricValue(a.metrics, metric),
    );
  const visible: AgentSightFlameNode[] = [];
  const hidden: AgentSightFlameNode[] = [];
  sorted.forEach((child, index) => {
    const value = metricValue(child.metrics, metric);
    const width = parentWidth * (value / total);
    if (index >= maxChildren || width < minSegmentPx) hidden.push(child);
    else visible.push(child);
  });
  if (hidden.length > 0)
    visible.push(cloneSyntheticOther(hidden, node, metric));
  return visible;
}

export function layoutAgentSightFlamegraph(
  root: AgentSightFlameNode,
  metric: AgentSightFlameMetric,
  options: AgentSightFlameLayoutOptions = {},
): AgentSightFlameRect[] {
  const width = options.width ?? DEFAULT_LAYOUT_WIDTH;
  const rowHeight = options.rowHeight ?? DEFAULT_ROW_HEIGHT;
  const barHeight = options.barHeight ?? DEFAULT_BAR_HEIGHT;
  const gap = options.gap ?? DEFAULT_GAP;
  const minSegmentPx = options.minSegmentPx ?? DEFAULT_MIN_SEGMENT_PX;
  const maxChildren = options.maxChildrenPerParent ?? DEFAULT_MAX_CHILDREN;
  const rootValue = Math.max(metricValue(root.metrics, metric), 0);
  if (rootValue <= 0) return [];
  const rects: AgentSightFlameRect[] = [
    {
      node: root,
      x: 0,
      y: 0,
      width,
      height: barHeight,
      value: rootValue,
      percentOfRoot: 1,
      percentOfParent: 1,
    },
  ];

  const visit = (
    node: AgentSightFlameNode,
    x: number,
    y: number,
    nodeWidth: number,
    parentValue: number,
  ) => {
    const nodeValue = Math.max(metricValue(node.metrics, metric), 0);
    let offset = x;
    visibleChildren(node, metric, nodeWidth, minSegmentPx, maxChildren).forEach(
      (child) => {
        const childValue = Math.max(metricValue(child.metrics, metric), 0);
        if (childValue <= 0 || nodeValue <= 0) return;
        const childWidth = nodeWidth * (childValue / nodeValue);
        const rectWidth = Math.max(childWidth - gap, 0);
        rects.push({
          node: child,
          x: offset,
          y: y + rowHeight,
          width: rectWidth,
          height: barHeight,
          value: childValue,
          percentOfRoot: rootValue > 0 ? childValue / rootValue : 0,
          percentOfParent: nodeValue > 0 ? childValue / nodeValue : 0,
        });
        if (child.children.length > 0 && child.level !== "other")
          visit(child, offset, y + rowHeight, childWidth, childValue);
        offset += childWidth;
      },
    );
  };

  visit(root, 0, 0, width, rootValue);
  return rects;
}

export function findAgentSightFlameNode(
  root: AgentSightFlameNode,
  id: string,
): AgentSightFlameNode | null {
  if (root.id === id) return root;
  for (const child of root.children) {
    const result = findAgentSightFlameNode(child, id);
    if (result) return result;
  }
  return null;
}

export function agentSightFlameBreadcrumbs(
  root: AgentSightFlameNode,
  id: string,
): AgentSightFlameNode[] {
  const path: AgentSightFlameNode[] = [];
  const visit = (node: AgentSightFlameNode): boolean => {
    path.push(node);
    if (node.id === id) return true;
    if (node.children.some(visit)) return true;
    path.pop();
    return false;
  };
  visit(root);
  return path;
}

export function agentSightFlameMetricValue(
  node: AgentSightFlameNode,
  metric: AgentSightFlameMetric,
) {
  return metricValue(node.metrics, metric);
}

export function formatAgentSightFlameMetric(
  value: number,
  metric: AgentSightFlameMetric,
) {
  if (metric === "bytes") return formatBytes(value);
  if (metric === "duration") return formatDuration(value);
  if (metric === "risk")
    return value >= 10 ? value.toFixed(0) : value.toFixed(1);
  return Math.round(value).toLocaleString();
}
