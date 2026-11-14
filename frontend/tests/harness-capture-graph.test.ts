import { describe, expect, test } from "bun:test";
import {
  buildHarnessCaptureGraph,
  captureEventKey,
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
