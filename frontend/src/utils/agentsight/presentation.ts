import { SOURCE_COLOR_CLASSES, SOURCE_COLORS } from "./constants";
import type { AgentSightEvent, ProcessedAgentSightEvent } from "./types";

// Memoization cache for processed events
const processedEventsCache = new WeakMap<AgentSightEvent[], ProcessedAgentSightEvent[]>();

export function processAgentSightEvents(
  events: AgentSightEvent[],
): ProcessedAgentSightEvent[] {
  if (!Array.isArray(events) || events.length === 0) return [];

  // Check cache first
  const cached = processedEventsCache.get(events);
  if (cached) return cached;

  const sourceColorMap = new Map<string, string>();
  const sourceClassMap = new Map<string, string>();
  let colorIndex = 0;

  const processed = events.map((event) => {
    if (!sourceColorMap.has(event.source)) {
      sourceColorMap.set(
        event.source,
        SOURCE_COLORS[colorIndex % SOURCE_COLORS.length],
      );
      sourceClassMap.set(
        event.source,
        SOURCE_COLOR_CLASSES[colorIndex % SOURCE_COLOR_CLASSES.length],
      );
      colorIndex += 1;
    }
    const datetime = new Date(event.timestamp);
    return {
      ...event,
      datetime,
      formattedTime: formatShortTime(event.timestamp),
      sourceColor: sourceColorMap.get(event.source)!,
      sourceColorClass: sourceClassMap.get(event.source)!,
    };
  });

  // Cache the result
  processedEventsCache.set(events, processed);
  return processed;
}

export function formatShortTime(timestamp: number) {
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return String(timestamp);
  return `${date.toLocaleTimeString(undefined, { hour12: false })}.${date.getMilliseconds().toString().padStart(3, "0")}`;
}

export function formatFullTime(timestamp?: number) {
  if (!timestamp) return "—";
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime())
    ? String(timestamp)
    : date.toLocaleString();
}

export function formatDuration(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "0ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${(ms / 60000).toFixed(1)}m`;
}

export function formatBytes(bytes: number) {
  const value = Number(bytes || 0);
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
}

// Optimize filterProcessedEvents with early returns and reduced iterations
export function filterProcessedEvents(
  events: ProcessedAgentSightEvent[],
  filters: {
    source?: string;
    comm?: string;
    pid?: string;
    searchTerm?: string;
    eventType?: string;
    traceId?: string;
    redactionState?: string;
  },
) {
  // Early return if no filters applied
  if (!filters || Object.values(filters).every(v => !v)) {
    return events;
  }

  // Prepare filter conditions once
  const hasSourceFilter = !!filters.source;
  const hasCommFilter = !!filters.comm;
  const hasEventTypeFilter = !!filters.eventType;
  const hasTraceIdFilter = !!filters.traceId;
  const hasRedactionFilter = !!filters.redactionState;
  const hasPidFilter = !!filters.pid;
  const hasSearchFilter = !!filters.searchTerm;

  const commLower = filters.comm?.toLowerCase();
  const searchLower = filters.searchTerm?.toLowerCase();
  const pidStr = String(filters.pid);

  return events.filter((event) => {
    // Quick checks first (most restrictive)
    if (hasSourceFilter && event.source !== filters.source && event.rawSource !== filters.source) {
      return false;
    }
    if (hasPidFilter && String(event.pid) !== pidStr) {
      return false;
    }
    if (hasEventTypeFilter && event.eventType !== filters.eventType) {
      return false;
    }
    if (hasRedactionFilter && event.redactionState !== filters.redactionState) {
      return false;
    }

    // String contains checks (more expensive)
    if (hasCommFilter && !event.comm.toLowerCase().includes(commLower!)) {
      return false;
    }
    if (hasTraceIdFilter && !event.traceId.includes(filters.traceId!)) {
      return false;
    }

    // Search term check (most expensive - done last)
    if (hasSearchFilter) {
      const searchableText = `${event.source} ${event.rawSource} ${event.id} ${event.comm} ${event.pid} ${event.eventType} ${event.title}`;
      if (!searchableText.toLowerCase().includes(searchLower!)) {
        // Fallback to JSON search if quick check fails
        try {
          if (!JSON.stringify(event.data).toLowerCase().includes(searchLower!)) {
            return false;
          }
        } catch {
          return false;
        }
      }
    }

    return true;
  });
}
