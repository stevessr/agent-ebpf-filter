import { describe, expect, test } from "bun:test";
import type {
  ExecutionGraphEdge,
  ExecutionGraphNode,
} from "../src/types/executionGraph";
import {
  filterPresentationNoise,
} from "../src/components/execution-graph/graphNoise";
import {
  buildDisplayGraph,
} from "../src/components/execution-graph/useDisplayGraphBuilder";

const process = (pid: number): ExecutionGraphNode => ({
  id: "proc:" + pid,
  kind: "process",
  label: "bash",
  pid,
  metadata: { comm: "bash", pid: String(pid) },
});
const syscall = (
  id: string,
  type: string,
  path = "",
  riskScore = 0,
): ExecutionGraphNode => ({
  id, kind: "syscall", label: type, riskScore, metadata: { type, path },
});
const edge = (source: string, target: string, kind = "observed"): ExecutionGraphEdge => ({
  id: source + "->" + target + ":" + kind,
  source, target, kind,
});

describe("harness process identity", () => {
  test("same executable with different PIDs keeps distinct process nodes and edges", () => {
    const graph = buildDisplayGraph(
      [process(101), process(202), syscall("call-a", "execve"), syscall("call-b", "execve")],
      [edge("proc:101", "call-a"), edge("proc:202", "call-b")],
    );
    const processes = graph.nodes.filter((node) => node.kind === "process");
    expect(processes).toHaveLength(2);
    expect(new Set(processes.map((node) => node.id))).toEqual(new Set(["proc:101", "proc:202"]));
    expect(graph.edges.map((item) => item.source).sort()).toEqual(["proc:101", "proc:202"]);
  });

  test("a PID-only neighbor cannot be merged into a different PID", () => {
    const graph = buildDisplayGraph(
      [process(101), { id: "proc:202", kind: "process", label: "PID 202", metadata: { pid: "202" } }],
      [edge("proc:101", "proc:202", "child_process")],
    );
    expect(graph.nodes.filter((node) => node.kind === "process")).toHaveLength(2);
    expect(graph.edges).toHaveLength(1);
  });
});

describe("presentation noise filter", () => {
  test("balanced mode hides routine calls without affecting process/tool evidence", () => {
    const nodes = [
      process(101),
      { id: "tool", kind: "tool_call", label: "bash" },
      syscall("getpid", "getpid"),
      { ...syscall("read-proc", "openat", "/proc/101/stat"), metadata: { type: "openat", path: "/proc/101/stat", openFlags: "O_RDONLY|O_CLOEXEC" } },
      syscall("read-user", "openat", "/workspace/.env"),
    ];
    const edges = nodes.slice(1).map((node) => edge("proc:101", node.id));
    const filtered = filterPresentationNoise(nodes, edges, "balanced");
    expect(filtered.nodes.map((node) => node.id)).toEqual(["proc:101", "tool", "read-user"]);
    expect(filtered.hiddenNodes).toBe(2);
    expect(filtered.hiddenEdges).toBe(2);
    expect(nodes).toHaveLength(5);
    expect(edges).toHaveLength(4);
  });

  test("never hides alerts, blocked decisions, high risk activity or sensitive proc reads", () => {
    const nodes = [
      syscall("high", "getpid", "", 80),
      { ...syscall("blocked", "futex"), metadata: { decision: "BLOCK", type: "futex" } },
      { id: "alert", kind: "policy_alert", label: "secret access" },
      syscall("sensitive", "openat", "/proc/101/environ"),
      syscall("tmp", "openat", "/tmp/sensitive.txt"),
    ];
    expect(filterPresentationNoise(nodes, [], "strict").hiddenNodes).toBe(0);
  });

  test("preserves the direct causal event linked to a policy alert", () => {
    const nodes = [
      process(1),
      syscall("getpid", "getpid"),
      { id: "alert", kind: "policy_alert", label: "risk" },
    ];
    const edges = [
      edge("proc:1", "getpid"),
      edge("getpid", "alert", "alerted"),
    ];
    const result = filterPresentationNoise(nodes, edges, "balanced");
    expect(result.hiddenNodes).toBe(0);
    expect(result.edges).toHaveLength(2);
  });

  test("strict mode additionally hides successful exits but selected nodes survive", () => {
    const nodes = [
      { id: "exit:0", kind: "exit_status", label: "exit", metadata: { exitCode: "0" } },
      { id: "exit:1", kind: "exit_status", label: "exit", metadata: { exitCode: "1" } },
    ];
    expect(filterPresentationNoise(nodes, [], "balanced").hiddenNodes).toBe(0);
    expect(filterPresentationNoise(nodes, [], "strict").nodes.map((n) => n.id)).toEqual(["exit:1"]);
    expect(filterPresentationNoise(nodes, [], "strict", "exit:0").hiddenNodes).toBe(0);
  });

  test("unknown or mutating open flags retain evidence on pseudo paths", () => {
    const base = syscall("raw-open", "openat", "/proc/222/stat");
    const candidates: ExecutionGraphNode[] = [
      base,
      { ...base, id: "write", metadata: { type: "openat", path: "/proc/222/stat", openFlags: "O_WRONLY" } },
      { ...base, id: "create", metadata: { type: "openat", path: "/proc/222/stat", openFlags: "O_RDONLY|O_CREAT" } },
      { ...base, id: "truncate", metadata: { type: "openat", path: "/proc/222/stat", flags: "512" } },
      { ...base, id: "read", metadata: { type: "openat", path: "/proc/222/stat", flags: "0" } },
    ];
    const filtered = filterPresentationNoise(candidates, [], "strict");
    expect(filtered.nodes.map((node) => node.id)).toEqual(["raw-open", "write", "create", "truncate"]);
  });

  test("does not collapse traversal, sensitive pseudo files or metadata risk", () => {
    const nodes = [
      syscall("traversal", "stat", "/proc/self/../102/environ"),
      syscall("kernel", "read", "/proc/kcore"),
      syscall("fd", "readlink", "/proc/self/fd/4"),
      { ...syscall("flagged", "getpid"), metadata: { type: "getpid", risk_score: "80" } },
      syscall("safe", "stat", "/proc/101/stat"),
    ];
    const filtered = filterPresentationNoise(nodes, [], "balanced");
    expect(filtered.nodes.map((node) => node.id)).toEqual(["traversal", "kernel", "fd", "flagged"]);
  });

  test("preserves evidence on generic links directly joined to policy decisions", () => {
    const nodes = [
      syscall("routine", "getpid"),
      { id: "policy", kind: "policy_decision", label: "deny" },
    ];
    const filtered = filterPresentationNoise(nodes, [edge("routine", "policy")], "strict");
    expect(filtered.hiddenNodes).toBe(0);
    expect(filtered.hiddenEdges).toBe(0);
  });

  test("off mode keeps the exact input snapshot and all links", () => {
    const nodes = [syscall("a", "getpid")];
    const edges = [edge("a", "a")];
    const result = filterPresentationNoise(nodes, edges, "off");
    expect(result.nodes).toBe(nodes);
    expect(result.edges).toBe(edges);
    expect(result.hiddenNodes).toBe(0);
  });
});
