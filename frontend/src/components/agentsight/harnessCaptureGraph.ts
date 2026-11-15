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
  omittedMessages: number;
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

/**
 * Endpoint labels are for topology, not raw forensics. Strip URL userinfo,
 * query strings and fragments that may embed API keys or bearer tokens.
 * The original destination remains untouched in AgentSight event details.
 */
function safeEndpoint(value: unknown): string {
  const raw = text(value, 256);
  if (!raw) return "";
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(raw)) {
    try {
      const parsed = new URL(raw);
      parsed.username = "";
      parsed.password = "";
      parsed.search = "";
      parsed.hash = "";
      return text(parsed.toString(), 256);
    } catch {
      // Treat malformed destinations as opaque and redact conservatively.
    }
  }
  return raw.replace(/^[^/@\s]+@/, "").split(/[?#]/, 1)[0];
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
  // Match complete verdict tokens, not innocent words like "unblocked"
  // or a resource path containing "block".
  const verdict = (event.eventType + " " + decision).replace(/[_-]/g, " ");
  return /(?:^|[^A-Z0-9])(?:ALERT|BLOCK|BLOCKED|DENY|DENIED|REJECT|REJECTED|DROP|DROPPED|KILL|KILLED|QUARANTINE|QUARANTINED)(?=$|[^A-Z0-9])/i.test(verdict) ||
    data.alert === true || data.alert === "true";
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
  let omittedMessages = 0;
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
    const endpoint = safeEndpoint(data.host ?? data.domain ?? data.net_endpoint ??
      data.netEndpoint ?? data.endpoint);
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

    type RpcFrame = {
      id: string;
      method: string;
      tool: string;
      kind: "request" | "response" | "error" | "other";
      protocol: string;
    };
    const rpcFrames: RpcFrame[] = [];
    let rpcMethod = "";
    let rpcTool = "";
    let rpcId = "";
    let rpcKind = "";
    if (isStdioSource(event.source)) {
      // Limit parsing oversized stdout chunks in the visualization; they
      // remain available unchanged in AgentSight's evidence drawer and logs.
      const rawChunk = data.data ?? data.payload;
      const chunkTooLarge = typeof rawChunk === "string" &&
        rawChunk.length > 128 * 1024;
      try {
        if (chunkTooLarge) {
          // An uninspected capture introduces an unknown byte gap. Reset
          // any pending fragment for this stream before the next event,
          // otherwise a later chunk could manufacture a false tool call.
          stdioDecoder.decode({
            ...event,
            data: { ...data, data: "", payload: "", truncated: true },
          });
          throw new Error("visualization decode budget");
        }
        const decoded = stdioDecoder.decode(event);
        // A truncated, overflowed or malformed frame is evidence of a
        // capture problem, not a trustworthy JSON-RPC invocation.
        if (decoded.truncated || decoded.framingError ||
            decoded.reassemblyReset) {
          throw new Error("untrusted or incomplete stdio framing");
        }
        rpcMethod = text(decoded.method, 100);
        rpcTool = text(decoded.toolName, 100);
        rpcId = text(decoded.id, 128);
        rpcKind = text(decoded.kind, 24);
        const messages = decoded.parsedMessages;
        // One collector event may contain many complete JSON-RPC frames.
        // Keep a bounded set of graph relations, while the original event
        // remains available with every byte in the evidence drawer.
        const maxFrames = 32;
        omittedMessages += Math.max(0, messages.length - maxFrames);
        for (const message of messages.slice(0, maxFrames)) {
          if (!message || typeof message !== "object") continue;
          const method = typeof message.method === "string"
            ? text(message.method, 100) : "";
          const id = message.id === null || message.id === undefined
            ? "" : text(message.id, 128);
          const tool = method === "tools/call" &&
            typeof message.params?.name === "string"
            ? text(message.params.name, 100) : "";
          const kind = method
            ? (id ? "request" : "other")
            : message.error !== undefined ? "error"
              : message.result !== undefined ? "response" : "other";
          rpcFrames.push({
            id, method, tool, kind,
            protocol: decoded.protocol,
          });
        }
        // Display the first recognized tool/method, but preserve all
        // individual RPC tool relations below.
        const first = rpcFrames[0];
        if (first) {
          rpcMethod = first.method;
          rpcTool = first.tool;
          rpcId = first.id;
          rpcKind = first.kind;
        }
      } catch {
        // Malformed frames remain visible as ordinary captured events.
      }
    }
    if (!rpcFrames.length) {
      rpcFrames.push({
        id: rpcId, method: rpcMethod, tool: rpcTool,
        kind: rpcKind === "request" || rpcKind === "response" ||
          rpcKind === "error" ? rpcKind : "other",
        protocol: "",
      });
    }

    // Use only explicitly bidirectional session identities. Per-direction
    // fd_target and stream_id can differ between stdin and stdout and
    // would silently break legitimate request/response correlations.
    const session = text(data.sessionId ?? data.session_id ??
      data.connectionId ?? data.connection_id ??
      data.transportSessionId ?? data.transport_session_id, 128);
    let eventToolIdentity = toolCallId;
    for (const [frameIndex, rpc] of rpcFrames.entries()) {
      const rpcKey = rpc.id
        ? JSON.stringify([scope, pid, session, event.source, rpc.id])
        : "";
      // A chunk can contain several *different* tool requests. An explicit
      // event-level toolCallId must not merge them together.
      let identity = rpcFrames.length === 1 ? toolCallId : "";
      let correlatedToolName = rpcFrames.length === 1 ? toolName || rpc.tool : rpc.tool;
      let relation = "observed";
      if (rpcKey && rpc.kind === "request" && rpc.method === "tools/call" &&
          rpc.protocol === "mcp") {
        identity ||= `mcp:${captureEventKey(event)}:${frameIndex}`;
        pendingRpc.set(rpcKey, pendingRpc.has(rpcKey)
          ? null : { toolIdentity: identity, toolName: correlatedToolName });
        relation = "invoked";
      } else if (rpcKey && rpc.kind === "request") {
        // Another JSON-RPC method can reuse a still-outstanding tool id.
        // Its response is ambiguous even when that other request is not
        // itself represented as a tool call in this graph.
        if (pendingRpc.has(rpcKey)) pendingRpc.set(rpcKey, null);
      } else if (rpcKey && (rpc.kind === "response" || rpc.kind === "error")) {
        const request = pendingRpc.get(rpcKey);
        if (request) {
          identity ||= request.toolIdentity;
          correlatedToolName ||= request.toolName;
          relation = rpc.kind === "error" ? "failed" : "returned";
        }
        // A reused or concurrent request id has no trustworthy attribution.
        pendingRpc.delete(rpcKey);
      }
      if (!identity) continue;
      if (frameIndex === 0) eventToolIdentity = identity;
      const toolId = `tool:${scope}:${identity}`;
      addNode({
        id: toolId, kind: "tool_call",
        label: correlatedToolName || rpc.method || "Tool call",
        metadata: {
          agentRunId: runId, traceId,
          toolCallId: identity,
          toolName: correlatedToolName,
        },
      });
      if (runNodeId) addEdge(runNodeId, toolId, "contains");
      addEdge(toolId, processId, "owns");
      addEdge(toolId, eventId, relation);
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
        toolCallId: eventToolIdentity,
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
    omittedMessages,
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
