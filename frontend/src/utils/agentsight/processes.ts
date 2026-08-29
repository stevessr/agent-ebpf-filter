import { comparePrompts, parseAgentSightEvent } from "./parsing";
import { AgentSightStdioStreamDecoder } from "./stdio_stream";
import type {
  AgentSightEvent,
  AgentSightFilterOptions,
  AgentSightProcessFilters,
  AgentSightProcessNode,
  AgentSightTimelineItem,
  ParsedAgentSightEvent,
  ParsedAgentSightEventType,
} from "./types";

// Optimized buildProcessTree with reduced allocations
export function buildProcessTree(
  events: AgentSightEvent[],
): AgentSightProcessNode[] {
  if (!Array.isArray(events) || events.length === 0) return [];

  const processMap = new Map<number, AgentSightProcessNode>();
  const eventsByPid = new Map<number, ParsedAgentSightEvent[]>();
  const promptHistoryByPid = new Map<number, ParsedAgentSightEvent[]>();
  // Stateful stdio framing is scoped to exactly one chronological tree build.
  // This lets split LSP/MCP frames reassemble without leaking state across UI
  // recomputations or separately loaded traces.
  const stdioDecoder = new AgentSightStdioStreamDecoder();

  // Sort once at the beginning
  const sortedEvents = events.slice().sort((a, b) => a.timestamp - b.timestamp);

  // First pass: build process nodes and parse events
  for (const event of sortedEvents) {
    if (event.source === "system" || event.pid === 0) continue;

    // Ensure process node exists
    if (!processMap.has(event.pid)) {
      processMap.set(event.pid, {
        pid: event.pid,
        ppid: event.ppid,
        comm: event.comm || "unknown",
        children: [],
        events: [],
        timeline: [],
      });
    }

    const node = processMap.get(event.pid)!;
    if (!node.ppid && event.ppid) node.ppid = event.ppid;

    const parsed = parseAgentSightEvent(event, stdioDecoder);
    if (!parsed) continue;

    // Handle prompt diff
    if (parsed.type === "prompt") {
      const history = promptHistoryByPid.get(event.pid);
      if (history && history.length > 0) {
        const previous = history[history.length - 1];
        parsed.promptDiff = {
          ...comparePrompts(previous.metadata.raw, event.data),
          previousPromptId: previous.id,
        };
        history.push(parsed);
        if (history.length > 10) history.shift();
      } else {
        promptHistoryByPid.set(event.pid, [parsed]);
      }
    }

    // Group events by PID
    if (!eventsByPid.has(event.pid)) {
      eventsByPid.set(event.pid, []);
    }
    eventsByPid.get(event.pid)!.push(parsed);
  }

  // Second pass: assign events to processes (already sorted)
  for (const [pid, parsedEvents] of eventsByPid) {
    const process = processMap.get(pid);
    if (process) {
      process.events = parsedEvents;
    }
  }

  // Third pass: build parent-child relationships
  const childProcesses = new Set<number>();
  for (const [pid, process] of processMap) {
    if (process.ppid && processMap.has(process.ppid)) {
      const parent = processMap.get(process.ppid)!;
      parent.children.push(process);
      childProcesses.add(pid);
    }
  }

  // Fourth pass: build timelines
  for (const process of processMap.values()) {
    const timeline: AgentSightTimelineItem[] = [
      ...process.events.map((event) => ({
        type: "event" as const,
        timestamp: event.timestamp,
        event,
      })),
      ...process.children.map((child) => ({
        type: "process" as const,
        timestamp: getEarliestTimestamp(child),
        process: child,
      })),
    ];
    process.timeline = timeline.sort((a, b) => a.timestamp - b.timestamp);
  }

  // Return root processes sorted by earliest timestamp
  const rootProcesses = Array.from(processMap.values()).filter(
    (process) => !childProcesses.has(process.pid),
  );
  return rootProcesses.sort(
    (a, b) => getEarliestTimestamp(a) - getEarliestTimestamp(b),
  );
}

function getEarliestTimestamp(process: AgentSightProcessNode): number {
  const candidates: number[] = [];
  if (process.events.length > 0) candidates.push(process.events[0].timestamp);
  process.children.forEach((child) =>
    candidates.push(getEarliestTimestamp(child)),
  );
  return candidates.length ? Math.min(...candidates) : 0;
}

export function createDefaultProcessFilters(): AgentSightProcessFilters {
  return {
    eventTypes: [],
    models: [],
    sources: [],
    commands: [],
    searchText: "",
    timeRange: {},
  };
}

export function extractProcessFilterOptions(
  events: AgentSightEvent[],
): AgentSightFilterOptions {
  const eventTypes = new Set<string>();
  const models = new Set<string>();
  const sources = new Set<string>();
  const commands = new Set<string>();
  events.forEach((event) => {
    const parsed = parseAgentSightEvent(event);
    if (!parsed) return;
    eventTypes.add(parsed.type);
    sources.add(event.source);
    if (event.comm) commands.add(event.comm);
    const model = parsed.metadata?.model;
    if (model && model !== "Unknown Model") models.add(model);
  });
  return {
    eventTypes: Array.from(eventTypes).sort(),
    models: Array.from(models).sort(),
    sources: Array.from(sources).sort(),
    commands: Array.from(commands).sort(),
  };
}

function parsedEventMatchesFilters(
  event: ParsedAgentSightEvent,
  process: AgentSightProcessNode,
  filters: AgentSightProcessFilters,
) {
  if (
    filters.eventTypes.length > 0 &&
    !filters.eventTypes.includes(event.type)
  )
    return false;
  const source = String(event.metadata.original_source || "");
  if (filters.sources.length > 0 && !filters.sources.includes(source))
    return false;
  if (
    filters.commands.length > 0 &&
    !filters.commands.includes(process.comm)
  )
    return false;
  if (
    filters.models.length > 0 &&
    !filters.models.includes(event.metadata?.model)
  )
    return false;
  if (filters.timeRange.start && event.timestamp < filters.timeRange.start)
    return false;
  if (filters.timeRange.end && event.timestamp > filters.timeRange.end)
    return false;
  if (filters.searchText) {
    const term = filters.searchText.toLowerCase();
    const searchable = [
      event.title,
      event.content,
      process.comm,
      source,
      event.metadata?.model,
      JSON.stringify(event.metadata),
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    if (!searchable.includes(term)) return false;
  }
  return true;
}

export function filterProcessTree(
  processTree: AgentSightProcessNode[],
  filters: AgentSightProcessFilters,
): AgentSightProcessNode[] {
  return processTree
    .map((process) => {
      const events = process.events.filter((event) =>
        parsedEventMatchesFilters(event, process, filters),
      );
      const children = filterProcessTree(process.children, filters);
      const timeline: AgentSightTimelineItem[] = [
        ...events.map((event) => ({
          type: "event" as const,
          timestamp: event.timestamp,
          event,
        })),
        ...children.map((child) => ({
          type: "process" as const,
          timestamp: getEarliestTimestamp(child),
          process: child,
        })),
      ].sort((a, b) => a.timestamp - b.timestamp);
      if (events.length > 0 || children.length > 0)
        return { ...process, events, children, timeline };
      return null;
    })
    .filter((process): process is AgentSightProcessNode => process !== null);
}

export function getTotalEventCount(
  processTree: AgentSightProcessNode[],
): number {
  return processTree.reduce(
    (total, process) =>
      total + process.events.length + getTotalEventCount(process.children),
    0,
  );
}

export function parsedTypeColor(type: ParsedAgentSightEventType) {
  switch (type) {
    case "prompt":
      return "blue";
    case "response":
      return "green";
    case "ssl":
      return "orange";
    case "file":
      return "cyan";
    case "process":
      return "purple";
    case "stdio":
      return "geekblue";
    case "policy":
      return "red";
    case "agent":
      return "magenta";
    default:
      return "default";
  }
}

export function buildParsedEventPreview(event: ParsedAgentSightEvent) {
  if (event.promptDiff?.summary) return `📝 ${event.promptDiff.summary}`;
  const content = event.content || event.title;
  return (
    content.replace(/\s+/g, " ").slice(0, 180) +
    (content.length > 180 ? "..." : "")
  );
}
