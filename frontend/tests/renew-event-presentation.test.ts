import { describe, expect, test } from "bun:test";

import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import {
  describeEvent,
  eventLabel,
  eventTone,
  isAgentEvent,
  isAttentionEvent,
} from "../src/composables/renew/eventPresentation";

function event(overrides: Partial<AgentEvent> = {}): AgentEvent {
  return {
    key: "renew-test",
    pid: 42,
    ppid: 1,
    uid: 1000,
    type: "read",
    tag: "Unknown",
    comm: "codex",
    path: "",
    time: "2026-10-04T16:00:00Z",
    ...overrides,
  };
}

describe("Renew event presentation", () => {
  test("describes common kernel events in plain language", () => {
    expect(
      describeEvent(event({ type: "write", path: "/tmp/result.txt" })),
    ).toBe("写入 /tmp/result.txt");
    expect(
      describeEvent(
        event({
          type: "network_connect",
          netEndpoint: "api.openai.com:443",
        }),
      ),
    ).toBe("连接到 api.openai.com:443");
  });

  test("recognizes semantically correlated Agent activity", () => {
    expect(isAgentEvent(event({ agentRunId: "run-1" }))).toBe(true);
    expect(isAgentEvent(event({ toolCallId: "tool-1" }))).toBe(true);
    expect(isAgentEvent(event({ tag: "Codex" }))).toBe(true);
    expect(isAgentEvent(event())).toBe(false);
  });

  test("only elevates actionable decisions and risk", () => {
    expect(isAttentionEvent(event({ decision: "ALLOW" }))).toBe(false);
    expect(eventTone(event({ decision: "ALLOW" }))).toBe("normal");
    expect(eventLabel(event({ decision: "ALLOW" }))).toBe("read");

    expect(isAttentionEvent(event({ riskScore: 65 }))).toBe(true);
    expect(eventTone(event({ riskScore: 65 }))).toBe("warning");
    expect(eventLabel(event({ riskScore: 65 }))).toBe("风险 65");

    expect(isAttentionEvent(event({ decision: "BLOCK" }))).toBe(true);
    expect(eventTone(event({ decision: "BLOCK" }))).toBe("danger");
    expect(eventLabel(event({ decision: "BLOCK" }))).toBe("BLOCK");
  });
});
