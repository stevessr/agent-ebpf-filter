import { describe, expect, test } from "bun:test";
import {
  buildHarnessCaptureGraph,
  captureEventKey,
  relatedCaptureEvidence,
} from "../src/components/agentsight/harnessCaptureGraph";
import type { AgentSightEvent } from "../src/utils/agentsight/types";
import { filterPresentationNoise } from "../src/components/execution-graph/graphNoise";

function event(id: string, overrides: Partial<AgentSightEvent> = {}): AgentSightEvent {
  return {
    id,
    timestamp: 1_731_000_000_000,
    source: "stdio",
    rawSource: "stdio",
    pid: 200,
    ppid: 100,
    comm: "codex",
    eventType: "STDIO",
    traceId: "trace-test",
    spanId: "",
    redactionState: "sanitized",
    title: "STDIO capture",
    data: {},
    raw: {},
    ...overrides,
  };
}

describe("AgentSight Harness capture to causal topology", () => {
  test("links trace, parent/child PID and captured RPC event", () => {
    const source = event("rpc-request", {
      data: {
        data: JSON.stringify({
          jsonrpc: "2.0",
          id: 7,
          method: "tools/call",
          params: { name: "safe_search", arguments: { secret: "DO_NOT_EXPOSE" } },
        }) + "\n",
        direction: "write",
        fd: 0,
      },
    });
    const graph = buildHarnessCaptureGraph([source]);
    const node = graph.nodes.find((n) =>
      n.metadata?.sourceNodeId === captureEventKey(source),
    );
    expect(node).toBeDefined();
    expect(node?.kind).toBe("hook_event");
    expect(node?.label).toContain("safe_search");
    expect(graph.nodes.some((n) => n.id === "trace:trace-test")).toBe(true);
    expect(graph.nodes.some((n) => n.id === "proc:trace:trace-test:200")).toBe(true);
    expect(graph.edges.some((e) => e.kind === "parent_process")).toBe(true);
    expect(graph.edges.some((e) => e.kind === "owns")).toBe(true);
    expect(JSON.stringify(graph)).not.toContain("DO_NOT_EXPOSE");
  });

  test("reassembles a split MCP tools/call frame without emitting payload contents", () => {
    const raw = JSON.stringify({
      jsonrpc: "2.0", id: 12, method: "tools/call",
      params: { name: "repo_search", arguments: { token: "PRIVATE_TOKEN" } },
    }) + "\n";
    const first = event("part-1", {
      timestamp: 1000,
      data: { fd: 0, fd_role: "stdin", direction: "write", data: raw.slice(0, 13) },
    });
    const second = event("part-2", {
      timestamp: 1001,
      data: { fd: 0, fd_role: "stdin", direction: "write", data: raw.slice(13) },
    });
    const graph = buildHarnessCaptureGraph([second, first]);
    expect(graph.nodes.filter((n) => n.kind === "tool_call")).toHaveLength(1);
    expect(graph.nodes.some((n) => n.label.includes("repo_search"))).toBe(true);
    expect(JSON.stringify(graph)).not.toContain("PRIVATE_TOKEN");
  });

  test("correlates an MCP request and response to the same actual invocation", () => {
    const request = event("request", {
      timestamp: 100,
      data: {
        fd: 0, direction: "write",
        data: JSON.stringify({
          jsonrpc: "2.0", id: 9,
          method: "tools/call", params: { name: "grep" },
        }) + "\n",
      },
    });
    const response = event("response", {
      timestamp: 101,
      data: {
        fd: 1, direction: "read",
        data: JSON.stringify({
          jsonrpc: "2.0", id: 9,
          result: { content: [{ type: "text", text: "PRIVATE_RESULT" }] },
        }) + "\n",
      },
    });
    const graph = buildHarnessCaptureGraph([response, request]);
    const toolNodes = graph.nodes.filter((node) => node.kind === "tool_call");
    expect(toolNodes).toHaveLength(1);
    expect(toolNodes[0].label).toBe("grep");
    const linked = graph.edges.filter((edge) => edge.source === toolNodes[0].id);
    expect(linked.some((edge) => edge.kind === "invoked" &&
      graph.nodes.find((node) => node.id === edge.target)?.metadata?.sourceNodeId ===
      captureEventKey(request))).toBe(true);
    expect(linked.some((edge) => edge.kind === "returned" &&
      graph.nodes.find((node) => node.id === edge.target)?.metadata?.sourceNodeId ===
      captureEventKey(response))).toBe(true);
    expect(JSON.stringify(graph)).not.toContain("PRIVATE_RESULT");
  });

  test("JSON-RPC id reuse produces distinct tool calls, never a false shared invocation", () => {
    const records: AgentSightEvent[] = [];
    for (let i = 0; i < 2; i++) {
      records.push(event("request-" + i, {
        timestamp: 100 + i * 2,
        data: { fd: 0, direction: "write", data: JSON.stringify({
          jsonrpc: "2.0", id: 1, method: "tools/call",
          params: { name: i === 0 ? "list" : "fetch" },
        }) + "\n" },
      }));
      records.push(event("response-" + i, {
        timestamp: 101 + i * 2,
        data: { fd: 1, direction: "read", data: JSON.stringify({
          jsonrpc: "2.0", id: 1,
          result: { content: [{ type: "text", text: "done" }] },
        }) + "\n" },
      }));
    }
    const graph = buildHarnessCaptureGraph(records);
    const tools = graph.nodes.filter((node) => node.kind === "tool_call");
    expect(tools).toHaveLength(2);
    expect(tools.map((node) => node.label).sort()).toEqual(["fetch", "list"]);
    for (const tool of tools) {
      expect(graph.edges.filter((edge) => edge.source === tool.id &&
        edge.kind === "invoked")).toHaveLength(1);
      expect(graph.edges.filter((edge) => edge.source === tool.id &&
        edge.kind === "returned")).toHaveLength(1);
    }
  });

  test("concurrent duplicate IDs with no shared transport identity are not falsely paired", () => {
    const req = (id: string, time: number, tool: string) => event(id, {
      timestamp: time, data: { direction: "write", fd: 0,
        data: JSON.stringify({ jsonrpc: "2.0", id: 3,
          method: "tools/call", params: { name: tool } }) + "\n" },
    });
    const res = (id: string, time: number) => event(id, {
      timestamp: time, data: { direction: "read", fd: 1,
        data: JSON.stringify({ jsonrpc: "2.0", id: 3,
          result: { content: [{ type: "text", text: "ok" }] } }) + "\n" },
    });
    const graph = buildHarnessCaptureGraph([
      req("request-a", 100, "alpha"), req("request-b", 101, "beta"),
      res("response-a", 102), res("response-b", 103),
    ]);
    expect(graph.nodes.filter((node) => node.kind === "tool_call")).toHaveLength(2);
    expect(graph.edges.filter((edge) => edge.kind === "invoked")).toHaveLength(2);
    expect(graph.edges.filter((edge) => edge.kind === "returned")).toHaveLength(0);
  });

  test("explicit session identities disambiguate identical MCP IDs across transports", () => {
    const req = (id: string, time: number, sessionId: string, tool: string) =>
      event(id, { timestamp: time, data: {
        fd: 0, fd_target: "stdin", sessionId,
        data: JSON.stringify({ jsonrpc: "2.0", id: 5,
          method: "tools/call", params: { name: tool } }) + "\n",
      } });
    const res = (id: string, time: number, sessionId: string) =>
      event(id, { timestamp: time, data: {
        fd: 1, fd_target: "stdout", sessionId,
        data: JSON.stringify({ jsonrpc: "2.0", id: 5,
          result: { content: [{ type: "text", text: "ok" }] } }) + "\n",
      } });
    const graph = buildHarnessCaptureGraph([
      req("a-request", 100, "a", "alpha"), req("b-request", 101, "b", "beta"),
      res("b-response", 102, "b"), res("a-response", 103, "a"),
    ]);
    const tools = graph.nodes.filter((node) => node.kind === "tool_call");
    expect(tools).toHaveLength(2);
    for (const tool of tools) {
      const relations = graph.edges.filter((edge) => edge.source === tool.id);
      expect(relations.filter((edge) => edge.kind === "invoked")).toHaveLength(1);
      expect(relations.filter((edge) => edge.kind === "returned")).toHaveLength(1);
    }
  });

  test("orphan MCP responses remain visible but are not invented tool calls", () => {
    const response = event("orphan", {
      data: { fd: 1, direction: "read", data: JSON.stringify({
        jsonrpc: "2.0", id: 42,
        result: { content: [{ type: "text", text: "result" }] },
      }) + "\n" },
    });
    const graph = buildHarnessCaptureGraph([response]);
    expect(graph.nodes.filter((node) => node.kind === "tool_call")).toHaveLength(0);
    expect(graph.nodes.some((node) => node.metadata?.sourceNodeId === captureEventKey(response))).toBe(true);
  });

  test("repeated transport IDs with different timestamps or PIDs remain distinct observations", () => {
    const captures = [
      event("same", { timestamp: 100, pid: 100 }),
      event("same", { timestamp: 101, pid: 100 }),
      event("same", { timestamp: 100, pid: 200 }),
    ];
    const graph = buildHarnessCaptureGraph(captures);
    expect(new Set(captures.map(captureEventKey)).size).toBe(3);
    expect(graph.nodes.filter((node) => node.metadata?.sourceNodeId)).toHaveLength(3);
    expect(graph.displayedEvents).toBe(3);
  });

  test("policy ALLOW stays a decision and errno zero is not treated as an alert", () => {
    const allowed = event("allowed", {
      source: "policy", eventType: "POLICY",
      data: { decision: "ALLOW", riskScore: 0 },
    });
    const normal = event("errno-zero", {
      source: "system", eventType: "system_metrics",
      data: { errno: "0", type: "system_metrics" },
    });
    const denied = event("denied", {
      source: "policy", eventType: "POLICY",
      data: { decision: "BLOCK", riskScore: 90 },
    });
    const graph = buildHarnessCaptureGraph([allowed, normal, denied]);
    const bySource = (e: AgentSightEvent) =>
      graph.nodes.find((n) => n.metadata?.sourceNodeId === captureEventKey(e));
    expect(bySource(allowed)?.kind).toBe("policy_decision");
    expect(bySource(normal)?.kind).toBe("hook_event");
    expect(bySource(denied)?.kind).toBe("policy_alert");
    const narrowed = buildHarnessCaptureGraph([allowed, normal, denied], 1);
    expect(narrowed.nodes.find((n) => n.metadata?.sourceNodeId)?.metadata?.sourceNodeId)
      .toBe(captureEventKey(denied));
  });

  test("high-risk network evidence is prioritized without changing its semantic kind", () => {
    const evidence = event("high-risk-net", {
      source: "ssl",
      eventType: "TLS_DATA",
      data: { riskScore: 95, host: "example.test" },
    });
    const graph = buildHarnessCaptureGraph([evidence]);
    const captured = graph.nodes.find((node) =>
      node.metadata?.sourceNodeId === captureEventKey(evidence));
    expect(captured?.kind).toBe("network");
    expect(captured?.riskScore).toBe(95);
  });

  test("can drill down from process, run and file nodes to exact capture evidence", () => {
    const captured = event("file-scan", {
      timestamp: 100,
      source: "file",
      eventType: "openat",
      data: { operation: "read", path: "/tmp/case" },
    });
    const graph = buildHarnessCaptureGraph([captured]);
    const occurrence = captureEventKey(captured);
    const processId = "proc:trace:trace-test:200";
    const runId = "trace:trace-test";
    const fileId = "file:/tmp/case";
    for (const selected of [processId, runId, fileId]) {
      expect(relatedCaptureEvidence(graph, selected).map((node) =>
        node.metadata?.sourceNodeId)).toContain(occurrence);
    }
    expect(relatedCaptureEvidence(graph, "missing")).toEqual([]);
    expect(relatedCaptureEvidence(graph, processId, 0)).toEqual([]);
  });

  test("extracts multiple MCP tool requests from a single STDIO capture chunk", () => {
    const capture = event("batch", {
      timestamp: 1000,
      data: { fd: 0, direction: "write", data: [
        { jsonrpc: "2.0", id: 7, method: "tools/call",
          params: { name: "repo_search", arguments: { secret: "HIDDEN_BATCH" } } },
        { jsonrpc: "2.0", id: 8, method: "tools/call",
          params: { name: "read_file" } },
      ].map((item) => JSON.stringify(item) + "\n").join("") },
    });
    const graph = buildHarnessCaptureGraph([capture]);
    expect(graph.nodes.filter((node) => node.kind === "tool_call")
      .map((node) => node.label).sort()).toEqual(["read_file", "repo_search"]);
    expect(graph.edges.filter((edge) => edge.kind === "invoked")).toHaveLength(2);
    expect(graph.displayedEvents).toBe(1);
    expect(graph.omittedMessages).toBe(0);
    expect(graph.nodes.filter((node) => node.metadata?.sourceNodeId)).toHaveLength(1);
    expect(JSON.stringify(graph)).not.toContain("HIDDEN_BATCH");
  });

  test("pairs MCP request and response occurring within one collector event", () => {
    const capture = event("multi-rpc", {
      timestamp: 1000,
      data: { fd: 0, direction: "write", data: [
        { jsonrpc: "2.0", id: 13, method: "tools/call",
          params: { name: "check" } },
        { jsonrpc: "2.0", id: 13,
          result: { content: [{ type: "text", text: "SECRET" }] } },
      ].map((item) => JSON.stringify(item) + "\n").join("") },
    });
    const graph = buildHarnessCaptureGraph([capture]);
    const tools = graph.nodes.filter((node) => node.kind === "tool_call");
    expect(tools).toHaveLength(1);
    expect(tools[0].label).toBe("check");
    expect(graph.edges.filter((edge) =>
      edge.source === tools[0].id && edge.kind === "invoked")).toHaveLength(1);
    expect(graph.edges.filter((edge) =>
      edge.source === tools[0].id && edge.kind === "returned")).toHaveLength(1);
    expect(JSON.stringify(graph)).not.toContain("SECRET");
  });

  test("limits in-chunk frame relations and accounts for every omitted message", () => {
    const capture = event("many-frames", {
      data: {
        fd: 0, direction: "write",
        data: Array.from({ length: 40 }, (_, i) =>
          JSON.stringify({ jsonrpc: "2.0", id: i + 1, method: "tools/call",
            params: { name: "fetch" } }) + "\n").join(""),
      },
    });
    const graph = buildHarnessCaptureGraph([capture]);
    expect(graph.displayedEvents).toBe(1);
    expect(graph.omittedEvents).toBe(0);
    expect(graph.omittedMessages).toBe(8);
    expect(graph.nodes.filter((node) => node.kind === "tool_call")).toHaveLength(32);
    expect(graph.edges.filter((edge) => edge.kind === "invoked")).toHaveLength(32);
  });

  test("reuses matching tool and process nodes for same trace", () => {
    const captures = [
      event("first", { data: { toolCallId: "tool-1", agentRunId: "run-1" } }),
      event("second", { timestamp: 1_731_000_000_001,
        data: { toolCallId: "tool-1", agentRunId: "run-1" } }),
    ];
    const graph = buildHarnessCaptureGraph(captures);
    expect(graph.nodes.filter((n) => n.kind === "tool_call")).toHaveLength(1);
    expect(graph.nodes.filter((n) => n.kind === "agent_run")).toHaveLength(1);
    expect(graph.nodes.filter((n) => n.metadata?.sourceNodeId)).toHaveLength(2);
  });

  test("routine system metrics collapse in the graph while alerts remain visible", () => {
    const samples = [
      event("normal-metric", {
        source: "system",
        eventType: "system_metrics",
        data: { type: "system_metrics", alert: false },
      }),
      event("cpu-alert", {
        source: "system",
        eventType: "system_metrics",
        data: { type: "system_metrics", alert: true },
      }),
    ];
    const graph = buildHarnessCaptureGraph(samples);
    const filtered = filterPresentationNoise(graph.nodes, graph.edges, "balanced");
    expect(filtered.hiddenByReason["routine-telemetry"]).toBe(1);
    expect(filtered.nodes.some((n) => n.metadata?.sourceNodeId === captureEventKey(samples[1]))).toBe(true);
    expect(graph.nodes.some((n) => n.metadata?.sourceNodeId === captureEventKey(samples[0]))).toBe(true);
  });

  test("retains high-risk early records when bounded and reports omissions", () => {
    const records: AgentSightEvent[] = [
      event("alert-old", {
        timestamp: 100,
        source: "policy",
        eventType: "ALERT",
        data: { alert: true, decision: "BLOCK" },
      }),
      ...Array.from({ length: 30 }, (_, i) => event("sys-" + i, {
        timestamp: 200 + i,
        source: "system",
        eventType: "system_metrics",
      })),
    ];
    const graph = buildHarnessCaptureGraph(records, 5);
    expect(graph.displayedEvents).toBe(5);
    expect(graph.omittedEvents).toBe(26);
    expect(graph.nodes.some((n) => n.metadata?.sourceNodeId ===
      captureEventKey(records[0]))).toBe(true);
    const filtered = filterPresentationNoise(graph.nodes, graph.edges, "strict");
    expect(filtered.nodes.some((n) => n.kind === "policy_alert")).toBe(true);
  });

  test("separates identical PIDs from different agent runs", () => {
    const graph = buildHarnessCaptureGraph([
      event("one", { data: { agentRunId: "a" } }),
      event("two", { data: { agentRunId: "b" } }),
    ]);
    const processIds = graph.nodes.filter((n) => n.kind === "process" &&
      n.pid === 200).map((n) => n.id);
    expect(processIds).toEqual(["proc:run:a:200", "proc:run:b:200"]);
  });

  test("does not expose normalized prompt previews in shared graph labels", () => {
    const input = event("sensitive-title", {
      title: "Prompt: PRIVATE_SHOULD_NOT_RENDER",
      source: "http_parser",
      eventType: "HTTP_REQUEST",
      data: { method: "POST", host: "api.example.test" },
    });
    const graph = buildHarnessCaptureGraph([input]);
    expect(JSON.stringify(graph)).not.toContain("PRIVATE_SHOULD_NOT_RENDER");
    expect(graph.nodes.some((node) => node.metadata?.sourceNodeId ===
      captureEventKey(input))).toBe(true);
  });

  test("keeps event details inspectable by source identity", () => {
    const capture = event("log-123", {
      source: "http_parser",
      title: "HTTP request",
      data: { host: "example.test", path: "/v1/models" },
    });
    const graph = buildHarnessCaptureGraph([capture]);
    const eventNode = graph.nodes.find((n) =>
      n.metadata?.sourceNodeId === captureEventKey(capture),
    );
    expect(eventNode?.kind).toBe("network");
    expect(eventNode?.metadata?.redactionState).toBe("sanitized");
    expect(graph.edges.some((e) => e.kind === "connected")).toBe(true);
  });
});
