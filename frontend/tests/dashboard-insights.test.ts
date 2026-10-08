import { describe, expect, test } from "bun:test";

import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import { buildDashboardInsights } from "../src/composables/dashboard/dashboardInsights";

let nextKey = 0;

function event(overrides: Partial<AgentEvent> = {}): AgentEvent {
  nextKey += 1;
  return {
    key: `event-${nextKey}`,
    pid: 1000 + nextKey,
    ppid: 1,
    uid: 1000,
    type: "read",
    tag: "test",
    comm: "agent",
    path: "",
    time: `2026-09-13T14:00:${String(nextKey).padStart(2, "0")}Z`,
    ...overrides,
  };
}

describe("buildDashboardInsights", () => {
  test("aggregates runs, tool calls, MCP calls, network targets and risk", () => {
    const insights = buildDashboardInsights([
      event({
        type: "native_hook",
        agentRunId: "run-a",
        toolCallId: "tool-1",
        toolName: "mcp__github__create_issue",
        traceId: "trace-a",
      }),
      event({
        type: "network_connect",
        agentRunId: "run-a",
        toolCallId: "tool-1",
        netDirection: "outgoing",
        netEndpoint: "api.github.com:443",
      }),
      event({
        type: "semantic_alert",
        agentRunId: "run-a",
        decision: "BLOCK",
        riskScore: 92,
      }),
      event({
        type: "read",
        agentRunId: "run-a",
        path: "/home/steve/.ssh/id_ed25519",
      }),
    ]);

    expect(insights.agentRuns).toBe(1);
    expect(insights.toolCalls).toBe(1);
    expect(insights.mcpCalls).toBe(1);
    expect(insights.mcpServers).toEqual([
      { key: "github", label: "github", count: 1 },
    ]);
    expect(insights.networkDestinations).toBe(1);
    expect(insights.topDestinations[0]).toEqual({
      destination: "api.github.com",
      count: 1,
    });
    expect(insights.alerts).toBe(1);
    expect(insights.blocked).toBe(1);
    expect(insights.highRisk).toBe(1);
    expect(insights.sensitiveFileTouches).toBe(1);
    expect(insights.recentRuns[0]?.toolCalls).toBe(1);
    expect(insights.recentRuns[0]?.destinations).toBe(1);
  });

  test("classifies file, package installation, git and gh operations", () => {
    const insights = buildDashboardInsights([
      event({ type: "read", path: "/workspace/README.md" }),
      event({ type: "write", path: "/workspace/out.txt" }),
      event({ type: "unlink", path: "/workspace/old.txt" }),
      event({ type: "rename", path: "/workspace/a", extraPath: "/workspace/b" }),
      event({
        type: "execve",
        comm: "pip",
        path: "/usr/bin/pip",
        extraInfo: "install ruff",
      }),
      event({
        type: "execve",
        comm: "pacman",
        path: "/usr/bin/pacman",
        extraInfo: "-S --needed jq",
      }),
      event({
        type: "execve",
        comm: "npm",
        path: "/usr/bin/npm",
        extraInfo: "install -g typescript",
      }),
      event({
        type: "execve",
        comm: "cargo",
        path: "/usr/bin/cargo",
        extraInfo: "install ripgrep",
      }),
      event({
        type: "execve",
        comm: "git",
        path: "/usr/bin/git",
        extraInfo: "push origin main",
      }),
      event({
        type: "execve",
        comm: "git",
        path: "/usr/bin/git",
        extraInfo: "fetch --all",
      }),
      event({
        type: "execve",
        comm: "gh",
        path: "/usr/bin/gh",
        extraInfo: "pr create",
      }),
    ]);

    expect(Object.fromEntries(insights.fileOperations.map((item) => [item.key, item.count]))).toEqual({
      read: 1,
      write: 1,
      delete: 1,
      mutate: 1,
    });
    expect(
      Object.fromEntries(insights.installOperations.map((item) => [item.key, item.count])),
    ).toEqual({ python: 1, system: 1, node: 1, other: 1 });
    expect(Object.fromEntries(insights.gitOperations.map((item) => [item.key, item.count]))).toEqual({
      push: 1,
      clone: 0,
      commit: 0,
      sync: 1,
      gh: 1,
      other: 0,
    });
  });

  test("reports semantic gaps only for correlatable kernel executions", () => {
    const insights = buildDashboardInsights([
      event({
        type: "native_hook",
        toolCallId: "tool-paired",
        traceId: "trace-paired",
      }),
      event({ type: "execve", toolCallId: "tool-paired", traceId: "trace-paired" }),
      event({ type: "process_exec", toolCallId: "tool-missing", traceId: "trace-missing" }),
      event({ type: "execve" }),
    ]);

    expect(insights.correlatedKernelExecs).toBe(2);
    expect(insights.pairedKernelExecs).toBe(1);
    expect(insights.semanticGaps).toBe(1);
    expect(insights.dualLayerCoverage).toBe(50);
  });

  test("returns an undefined coverage signal when no correlated exec events exist", () => {
    const insights = buildDashboardInsights([
      event({ type: "read", path: "/workspace/file.txt" }),
      event({ type: "network_bind", netDirection: "listening", netEndpoint: "0.0.0.0:8080" }),
    ]);

    expect(insights.correlatedKernelExecs).toBe(0);
    expect(insights.semanticGaps).toBe(0);
    expect(insights.dualLayerCoverage).toBeNull();
    expect(insights.networkDestinations).toBe(0);
  });
});
