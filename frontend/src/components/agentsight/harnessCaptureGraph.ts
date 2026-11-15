import type {
  ExecutionGraphEdge,
  ExecutionGraphNode,
} from "../../types/executionGraph";
import type { AgentSightEvent } from "../../utils/agentsight/types";
import { isStdioSource } from "../../utils/agentsight/stdio";
import { AgentSightStdioStreamDecoder } from "../../utils/agentsight/stdio_stream";
import { selectBoundedCaptureEvents } from "./harnessCaptureSelection";

export interface HarnessCaptureGraph {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  displayedEvents: number;
  omittedEvents: number;
}

/**
 * Transport IDs can be reused across replay imports or recorder restarts.
 * The occurrence key includes time, PID and trace/span, so two observations
 * with the same producer event_id remain independently inspectable.
 */
export const captureEventKey = (event: AgentSightEvent): string =>
  JSON.stringify([
    event.source, event.id, event.timestamp, event.pid,
    event.traceId, event.spanId,
  ]);

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

const isSecurityOutcome = (event: AgentSightEvent): boolean => {
  const data = event.data ?? {};
  const decision = decisionOf(data);
  return /ALERT|BLOCK|DENY|DENIED|REJECT|DROP|KILL|QUARANTINE/i.test(
    event.eventType + " " + decision,
  ) || data.alert === true || data.alert === "true";
};

function hasFailureOutcome(event: AgentSightEvent): boolean {
  const data = event.data ?? {};
  const code = text(data.errno ?? data.errorCode ?? data.error_code).trim();
  return (code !== "" && !/^(0|none|ok|success)$/i.test(code)) ||
    data.success === false || data.success === "false" ||
    data.failed === true || data.failed === "true";
}

function priority(event: AgentSightEvent): number {
  const data = event.data ?? {};
  if (isSecurityOutcome(event) || riskOf(data) >= 60 ||
      hasFailureOutcome(event)) return 0;
  if (event.source === "stdio" || event.source === "agent") return 1;
  if (event.source === "policy" ||
      event.source === "file" || event.source === "process") return 2;
  return 3;
}

function eventKind(event: AgentSightEvent): string {
  // An ALLOW decision is a policy decision, not an alert. Critical flags
  // determine risk color and priority, not the original event category.
  if (event.source === "policy") {
    return isSecurityOutcome(event) ? "policy_alert" : "policy_decision";
  }
  if (isSecurityOutcome(event)) return "policy_alert";
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
  const included = selectBoundedCaptureEvents(
    events, limit, priority, captureEventKey,
  );

  // The same bounded streaming decoder as AgentSight's process tree handles
  // Content-Length and newline-delimited MCP fragments in timestamp order.
  const stdioDecoder = new AgentSightStdioStreamDecoder({
    maxStreams: 128,
    maxPendingBytesPerStream: 128 * 1024,
    maxTotalPendingBytes: 1024 * 1024,
  });
  // Correlate only requests actually observed in the bounded capture window.
  // Completed calls are removed so a reused JSON-RPC id starts a new call.
  // null denotes an ambiguous JSON-RPC ID concurrently in use by more
  // than one request. In that case do not guess which response belongs to
  // which call; both request nodes remain available for forensic review.
  const pendingRpc = new Map<string, {
    toolIdentity: string;
    toolName: string;
  } | null>();
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
    let rpcKind = "";
    if (isStdioSource(event.source)) {
      // Limit parsing oversized stdout chunks in the visualization; they
      // remain available unchanged in AgentSight's evidence drawer and logs.
      const rawChunk = data.data ?? data.payload;
      const chunkTooLarge = typeof rawChunk === "string" &&
        rawChunk.length > 128 * 1024;
      try {
        if (chunkTooLarge) throw new Error("visualization decode budget");
        const decoded = stdioDecoder.decode(event);
        rpcMethod = text(decoded.method, 100);
        rpcTool = text(decoded.toolName, 100);
        rpcId = text(decoded.id, 128);
        rpcProtocol = text(decoded.protocol, 24);
        rpcKind = text(decoded.kind, 24);
      } catch {
        // Malformed frames remain visible as ordinary captured events.
      }
    }
    // Use only explicitly bidirectional session identities. Per-direction
    // fd_target and stream_id can differ between stdin and stdout and
    // would silently break legitimate request/response correlations.
    const session = text(data.sessionId ?? data.session_id ??
      data.connectionId ?? data.connection_id ??
      data.transportSessionId ?? data.transport_session_id, 128);
    const rpcKey = rpcId
      ? JSON.stringify([scope, pid, session, event.source, rpcId])
      : "";
    let toolIdentity = toolCallId;
    let correlatedToolName = toolName || rpcTool;
    let toolRelation = "observed";
    if (rpcKey && rpcKind === "request" && rpcMethod === "tools/call" &&
        rpcProtocol === "mcp") {
      // Do not merge consecutive tools/call operations that reuse the same
      // JSON-RPC ID. The request occurrence determines the unique identity.
      const generated = `mcp:${captureEventKey(event)}`;
      toolIdentity ||= generated;
      pendingRpc.set(rpcKey, pendingRpc.has(rpcKey)
        ? null
        : { toolIdentity, toolName: correlatedToolName });
      toolRelation = "invoked";
    } else if (rpcKey && (rpcKind === "response" || rpcKind === "error")) {
      const request = pendingRpc.get(rpcKey);
      if (request) {
        toolIdentity ||= request.toolIdentity;
        correlatedToolName ||= request.toolName;
        toolRelation = rpcKind === "error" ? "failed" : "returned";
      }
      // Consume even ambiguous IDs without ever attributing an unknown
      // response to an arbitrary tool request.
      pendingRpc.delete(rpcKey);
    }
    if (toolIdentity) {
      const toolId = `tool:${scope}:${toolIdentity}`;
      addNode({
        id: toolId, kind: "tool_call",
        label: correlatedToolName || rpcMethod || "Tool call",
        metadata: {
          agentRunId: runId, traceId,
          toolCallId: toolIdentity,
          toolName: correlatedToolName,
        },
      });
      if (runNodeId) addEdge(runNodeId, toolId, "contains");
      addEdge(toolId, processId, "owns");
      addEdge(toolId, eventId, toolRelation);
    }

    const type = text(event.eventType || data.type || event.source, 100);
    addNode({
      id: eventId,
      kind,
      // A human-facing event.title may contain a prompt/response preview.
      // Render only the protocol/type/tool identity in the shared graph;
      // inspect full content explicitly through AgentSightEventDetails.
      label: text(rpcTool ? `${rpcMethod || "MCP"} · ${rpcTool}` :
        rpcMethod ? `RPC · ${rpcMethod}` :
        `${event.source.toUpperCase()} · ${type}`, 140) || type,
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
        receivedAt: Number.isFinite(event.timestamp) &&
          Math.abs(event.timestamp) <= 8.64e15
          ? new Date(event.timestamp).toISOString() : "",
        redactionState: event.redactionState || "",
        rpcMethod, rpcTool, rpcKind, rpcId,
        // Outcome metadata is required by the evidence-critical filter.
        errno: text(data.errno ?? data.errorCode ?? data.error_code),
        error: text(data.error, 120),
        ret: text(data.returnCode ?? data.return_code ?? data.ret),
        success: text(data.success),
        failed: text(data.failed),
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

/**
 * Drill down from an aggregate resource/process/tool/run into captured event
 * evidence without changing the original AgentSight filter or retention.
 * One extra hop is allowed for a Run -> Process -> Event chain.
 */
export function relatedCaptureEvidence(
  graph: HarnessCaptureGraph,
  selectedId: string,
  limit = 12,
): ExecutionGraphNode[] {
  if (!selectedId) return [];
  const byId = new Map(graph.nodes.map((node) => [node.id, node]));
  const selected = byId.get(selectedId);
  if (!selected) return [];
  const maxDepth = selected.kind === "agent_run" ? 2 : 1;
  const adjacency = new Map<string, string[]>();
  for (const edge of graph.edges) {
    const a = adjacency.get(edge.source) ?? [];
    a.push(edge.target);
    adjacency.set(edge.source, a);
    const b = adjacency.get(edge.target) ?? [];
    b.push(edge.source);
    adjacency.set(edge.target, b);
  }
  const visited = new Set<string>([selectedId]);
  const queue: Array<{ id: string; depth: number }> = [
    { id: selectedId, depth: 0 },
  ];
  const matched: ExecutionGraphNode[] = [];
  for (let cursor = 0; cursor < queue.length; cursor++) {
    const { id, depth } = queue[cursor];
    if (depth >= maxDepth) continue;
    for (const neighbor of adjacency.get(id) ?? []) {
      if (visited.has(neighbor)) continue;
      visited.add(neighbor);
      const node = byId.get(neighbor);
      if (!node) continue;
      if (node.metadata?.sourceNodeId) matched.push(node);
      if (depth + 1 < maxDepth && node.kind === "process") {
        queue.push({ id: neighbor, depth: depth + 1 });
      }
    }
  }
  matched.sort((a, b) =>
    Number(b.riskScore ?? 0) - Number(a.riskScore ?? 0) ||
    (b.metadata?.receivedAt ?? "").localeCompare(a.metadata?.receivedAt ?? "") ||
    a.id.localeCompare(b.id),
  );
  return matched.slice(0, Math.max(0, Math.min(100, limit)));
}
