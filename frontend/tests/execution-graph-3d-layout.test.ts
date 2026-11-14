import { describe, expect, test } from "bun:test";
import type { ExecutionGraphNode } from "../src/types/executionGraph";
import { harnessLabelNodes, harnessPosition, harnessTopologySignature } from "../src/components/execution-graph/harness3dLayout";

const node = (id: string, kind: string, run = ""): ExecutionGraphNode => ({
  id, kind, label: id, metadata: { agentRunId: run },
});

describe("deterministic harness 3D layout", () => {
  test("stays anchored across reorder and replay growth", () => {
    const root = node("run-1", "agent_run", "trace-a");
    const process = node("proc:1", "process", "trace-a");
    const before = [root, process].map((n) => harnessPosition(n));
    const after = [node("extra", "syscall", "trace-a"), process, root];
    expect(harnessPosition(after[2])).toEqual(before[0]);
    expect(harnessPosition(after[1])).toEqual(before[1]);
  });

  test("lanes separate agent, process, resources and policy alerts", () => {
    const run = harnessPosition(node("run", "agent_run"));
    const process = harnessPosition(node("proc", "process"));
    const file = harnessPosition(node("file", "file"));
    const alert = harnessPosition(node("alert", "policy_alert"));
    expect(run.x).toBeLessThan(process.x);
    expect(process.x).toBeLessThan(file.x);
    expect(file.x).toBeLessThan(alert.x);
  });

  test("GPU signature ignores transport order but tracks layout and risk updates", () => {
    const nodes = [
      node("run", "agent_run", "run-a"),
      { ...node("call", "syscall", "run-a"), riskScore: 10 },
    ];
    const edges = [{
      id: "run->call", source: "run", target: "call", kind: "contains",
    }];
    const key = harnessTopologySignature(nodes, edges);
    expect(harnessTopologySignature(nodes.slice().reverse(), edges.slice().reverse())).toBe(key);
    expect(harnessTopologySignature([
      nodes[0], { ...nodes[1], riskScore: 95 },
    ], edges)).not.toBe(key);
    expect(harnessTopologySignature([
      nodes[0], { ...nodes[1], metadata: { agentRunId: "run-b" } },
    ], edges)).not.toBe(key);
    expect(harnessTopologySignature(nodes, [
      { ...edges[0], kind: "blocked" },
    ])).not.toBe(key);
  });

  test("bounded labels prioritize selected and alert evidence", () => {
    const samples: ExecutionGraphNode[] = Array.from({ length: 1000 }, (_, i) =>
      node("proc:" + i, "process"),
    );
    samples.push(node("alert", "policy_alert"));
    samples.push(node("selected", "syscall"));
    const labels = harnessLabelNodes(samples, "selected", 42);
    expect(labels).toHaveLength(42);
    expect(labels[0].id).toBe("selected");
    expect(labels[1].id).toBe("alert");
  });
});
