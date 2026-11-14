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

describe("behavior aggregation", () => {
  test("independent tool invocations must not collapse into one graph node", () => {
    const graph = buildDisplayGraph(
      [
        process(100),
        { id: "tool:a", kind: "tool_call", label: "shell", metadata: { toolCallId: "a" } },
        { id: "tool:b", kind: "tool_call", label: "shell", metadata: { toolCallId: "b" } },
      ],
      [edge("proc:100", "tool:a"), edge("proc:100", "tool:b")],
    );
    expect(graph.nodes.filter((node) => node.kind === "tool_call").map((node) => node.id).sort())
      .toEqual(["tool:a", "tool:b"]);
  });

  test("different file targets and per-tool traces remain distinct", () => {
    const nodes = [
      process(100),
      { ...syscall("scan-a", "openat", "/etc/passwd"), metadata: { type: "openat", path: "/etc/passwd", toolCallId: "a" } },
      { ...syscall("scan-b", "openat", "/etc/shadow"), metadata: { type: "openat", path: "/etc/shadow", toolCallId: "a" } },
      { ...syscall("scan-c", "openat", "/etc/passwd"), metadata: { type: "openat", path: "/etc/passwd", toolCallId: "b" } },
    ];
    const result = buildDisplayGraph(nodes, nodes.slice(1).map((node) => edge("proc:100", node.id)));
    expect(result.nodes.filter((node) => node.kind === "syscall")).toHaveLength(3);
  });

  test("blocked and high-risk events retain original source IDs", () => {
    const nodes = [
      process(100),
      syscall("normal", "openat", "/tmp/one"),
      { ...syscall("malicious", "openat", "/tmp/one", 93), metadata: { type: "openat", path: "/tmp/one" } },
      { ...syscall("denied", "openat", "/tmp/one"), metadata: { type: "openat", path: "/tmp/one", decision: "BLOCK" } },
    ];
    const result = buildDisplayGraph(nodes, nodes.slice(1).map((node) => edge("proc:100", node.id)));
    expect(result.nodes.some((node) => node.id === "malicious" && node.riskScore === 93)).toBe(true);
    expect(result.nodes.some((node) => node.id === "denied")).toBe(true);
    expect(result.edges.filter((item) => item.target === "malicious")).toHaveLength(1);
    expect(result.edges.filter((item) => item.target === "denied")).toHaveLength(1);
  });

  test("repeated harmless observations aggregate without dropping counts", () => {
    const nodes = [process(100), syscall("one", "futex"), syscall("two", "futex")];
    const result = buildDisplayGraph(nodes, nodes.slice(1).map((node) => edge("proc:100", node.id)));
    const grouped = result.nodes.filter((node) => node.kind === "syscall");
    expect(grouped).toHaveLength(1);
    expect(grouped[0].metadata?.eventCount).toBe("2");
    expect(grouped[0].metadata?.sourceNodeId).toBe("one");
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
    expect(filtered.hiddenByReason).toMatchObject({
      "routine-syscall": 1,
      "pseudo-file-read": 1,
      "successful-exit": 0,
      heartbeat: 0,
    });
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

  test("retains reconnaissance reads outside the narrow routine path allowlist", () => {
    const nodes = [
      syscall("cmdline", "read", "/proc/123/cmdline"),
      syscall("maps", "stat", "/proc/123/maps"),
      syscall("cgroup", "read", "/sys/fs/cgroup/user.slice/cgroup.procs"),
      syscall("proc-sys", "read", "/proc/sys/kernel/kptr_restrict"),
      syscall("safe-meta", "stat", "/proc/123/status"),
    ];
    const result = filterPresentationNoise(nodes, [], "strict");
    expect(result.nodes.map((node) => node.id)).toEqual([
      "cmdline", "maps", "cgroup", "proc-sys",
    ]);
    expect(result.hiddenByReason["pseudo-file-read"]).toBe(1);
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
