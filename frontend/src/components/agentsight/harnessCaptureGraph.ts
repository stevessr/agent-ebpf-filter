import type {
  ExecutionGraphEdge,
  ExecutionGraphNode,
} from "../../types/executionGraph";
import type { AgentSightEvent } from "../../utils/agentsight/types";
import { isStdioSource } from "../../utils/agentsight/stdio";
import { AgentSightStdioStreamDecoder } from "../../utils/agentsight/stdio_stream";

export interface HarnessCaptureGraph {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  displayedEvents: number;
  omittedEvents: number;
}

export const captureEventKey = (event: AgentSightEvent): string =>
  JSON.stringify([event.source, event.id]);

function text(value: unknown, max = 160): string {
  if (value === undefined || value === null) return "";
  return String(value).trim().slice(0, max);
}

function riskOf(data: Record<string, any>): number {
  const value = Number(data.riskScore ?? data.risk_score ?? 0);
  return Number.isFinite(value) ? Math.max(0, value) : 0;
}

function decisionOf(data: Record<string, any>): string {
  return text(data.decision ?? data.verdict ?? data.action, 60).toUpperCase();
}

function priority(event: AgentSightEvent): number {
  const data = event.data ?? {};
  const decision = decisionOf(data);
  if (event.source === "policy" || /ALERT|BLOCK|DENY|DENIED|REJECT|DROP/.test(
    event.eventType + " " + decision,
  ) || data.alert === true || riskOf(data) >= 60) return 0;
  if (event.source === "stdio" || event.source === "agent") return 1;
  if (event.source === "file" || event.source === "process") return 2;
  return 3;
}

function eventKind(event: AgentSightEvent): string {
  const data = event.data ?? {};
  if (priority(event) === 0) return "policy_alert";
  if (isStdioSource(event.source)) return "hook_event";
  if (event.source === "ssl" || event.source === "http_parser" ||
      event.source === "sse_processor") return "network";
  if (event.source === "agent") return "wrapper_event";
  if (event.source === "system") return "hook_event";
  return "syscall";
}

/**
 * Transform the already normalized/filtered AgentSight buffer into a
 * presentation-only graph. No new socket, no retention changes, no raw
 * prompt/body/stdio content copied into graph metadata.
 *
 * When a capture is too large, keep alerts and recent Harness/MCP events
 * first. The omitted count is explicit and all original rows remain in
 * AgentSight's Timeline/Log/export views.
 */
export function buildHarnessCaptureGraph(
  events: AgentSightEvent[],
  maxEvents = 1000,
): HarnessCaptureGraph {
  const limit = Math.max(1, Math.min(2000, Math.floor(maxEvents) || 1000));
  const included = events.length > limit
    ? [...events]
      .sort((a, b) => priority(a) - priority(b) ||
        b.timestamp - a.timestamp ||
        captureEventKey(a).localeCompare(captureEventKey(b)))
      .slice(0, limit)
    : [...events];
  included.sort((a, b) => a.timestamp - b.timestamp ||
    captureEventKey(a).localeCompare(captureEventKey(b)));

  // The same bounded streaming decoder as AgentSight's process tree handles
  // Content-Length and newline-delimited MCP fragments in timestamp order.
  const stdioDecoder = new AgentSightStdioStreamDecoder({
    maxStreams: 128,
    maxPendingBytesPerStream: 128 * 1024,
    maxTotalPendingBytes: 1024 * 1024,
  });
  const nodes = new Map<string, ExecutionGraphNode>();
  const edges = new Map<string, ExecutionGraphEdge>();
  const addNode = (node: ExecutionGraphNode) => {
    const current = nodes.get(node.id);
    if (!current) {
      nodes.set(node.id, node);
    } else if ((node.riskScore ?? 0) > (current.riskScore ?? 0)) {
      nodes.set(node.id, { ...current, riskScore: node.riskScore });
    }
  };
  const addEdge = (source: string, target: string, kind: string) => {
    if (source === target) return;
    const id = JSON.stringify([source, target, kind]);
    if (!edges.has(id)) edges.set(id, { id, source, target, kind });
  };

  for (const event of included) {
    const data = event.data ?? {};
    const runId = text(data.agentRunId ?? data.agent_run_id, 256);
    const traceId = text(event.traceId || data.traceId || data.trace_id, 256);
    const scope = runId ? "run:" + runId : traceId ? "trace:" + traceId : "capture";
    const pid = Number.isFinite(event.pid) && event.pid > 0 ? event.pid : 0;
    const processId = pid ? `proc:${scope}:${pid}` : `collector:${event.source}`;
    const runNodeId = runId ? `run:${runId}` : traceId ? `trace:${traceId}` : "";
    const eventId = "captured:" + captureEventKey(event);
    const toolCallId = text(data.toolCallId ?? data.tool_call_id, 256);
    const toolName = text(data.toolName ?? data.tool_name, 100);
    const decision = decisionOf(data);
    const risk = riskOf(data);
    const path = text(data.path ?? data.filepath ?? data.file_path, 500);
    const endpoint = text(data.host ?? data.domain ?? data.net_endpoint ??
      data.netEndpoint ?? data.endpoint, 256);
    const kind = eventKind(event);

    addNode({
      id: processId,
      kind: "process",
      label: pid ? text(event.comm, 80) || `pid ${pid}` : `${event.source} collector`,
      pid,
      metadata: { pid: String(pid), comm: text(event.comm, 80), agentRunId: runId, traceId },
    });
    if (pid && Number.isFinite(event.ppid) && (event.ppid ?? 0) > 0 &&
        event.ppid !== pid) {
      const parentId = `proc:${scope}:${event.ppid}`;
      addNode({
        id: parentId, kind: "process", label: `pid ${event.ppid}`, pid: event.ppid,
        metadata: { pid: String(event.ppid), agentRunId: runId, traceId },
      });
      addEdge(parentId, processId, "parent_process");
    }
    if (runNodeId) {
      addNode({
        id: runNodeId,
        kind: "agent_run",
        label: runId ? `Agent ${runId.slice(0, 32)}` : `Trace ${traceId.slice(0, 32)}`,
        metadata: { agentRunId: runId, traceId },
      });
      addEdge(runNodeId, processId, "contains");
    }

    let rpcMethod = "";
    let rpcTool = "";
    let rpcId = "";
    let rpcProtocol = "";
    if (isStdioSource(event.source)) {
      // Decode only the bounded presentation sample, never the entire live
      // collector buffer. This is metadata-only, not a raw-payload overlay.
      try {
        const decoded = stdioDecoder.decode(event);
        rpcMethod = text(decoded.method, 100);
        rpcTool = text(decoded.toolName, 100);
        rpcId = text(decoded.id, 128);
        rpcProtocol = text(decoded.protocol, 24);
      } catch {
        // Malformed frames remain visible as ordinary captured events.
      }
    }
    const toolIdentity = toolCallId || (
      rpcId && rpcProtocol === "mcp"
        ? `mcp:${event.source}:${pid}:${rpcId}`
        : ""
    );
    if (toolIdentity) {
      const toolId = `tool:${scope}:${toolIdentity}`;
      addNode({
        id: toolId, kind: "tool_call",
        label: toolName || rpcTool || rpcMethod || "Tool call",
        metadata: {
          agentRunId: runId, traceId,
          toolCallId: toolIdentity,
          toolName: toolName || rpcTool,
        },
      });
      if (runNodeId) addEdge(runNodeId, toolId, "contains");
      addEdge(toolId, processId, "owns");
    }

    const type = text(event.eventType || data.type || event.source, 100);
    addNode({
      id: eventId,
      kind,
      label: text(rpcTool ? `${rpcMethod || "MCP"} · ${rpcTool}` :
        event.title || type, 140) || type,
      riskScore: risk,
      metadata: {
        type: kind === "hook_event" && event.source === "system"
          ? "system_metrics" : type,
        source: event.source,
        pid: String(pid),
        path, endpoint, decision,
        traceId, agentRunId: runId,
        toolCallId: toolIdentity,
        sourceNodeId: captureEventKey(event),
        receivedAt: Number.isFinite(event.timestamp)
          ? new Date(event.timestamp).toISOString() : "",
        redactionState: event.redactionState || "",
        rpcMethod, rpcTool,
        riskScore: String(risk),
      },
    });
    addEdge(processId, eventId, kind === "policy_alert" ? "alerted" : "observed");

    if (path && event.source === "file") {
      const fileId = "file:" + path;
      addNode({ id: fileId, kind: "file", label: path, metadata: { path } });
      addEdge(eventId, fileId, /WRITE|DELETE|UNLINK|RENAME/i.test(type)
        ? "wrote" : "read");
    }
    if (endpoint && (event.source === "ssl" || event.source === "http_parser" ||
        event.source === "sse_processor")) {
      const networkId = "net:" + endpoint;
      addNode({ id: networkId, kind: "network", label: endpoint,
        metadata: { endpoint } });
      addEdge(eventId, networkId, "connected");
    }
  }

  return {
    nodes: [...nodes.values()],
    edges: [...edges.values()],
    displayedEvents: included.length,
    omittedEvents: events.length - included.length,
  };
}
