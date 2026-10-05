import { describe, expect, test } from "bun:test";

import { presentRenewEventDetail } from "../src/composables/renew/eventDetailPresentation";

describe("Renew full event visualization", () => {
  test("turns snake_case event JSON into a readable network view", () => {
    const presentation = presentRenewEventDetail({
      Timestamp: 1791165600000,
      Event: {
        type: "network_connect",
        comm: "codex",
        pid: 4242,
        ppid: 41,
        net_endpoint: "api.openai.com:443",
        decision: "BLOCK",
        risk_score: 88,
        agent_run_id: "run-1",
        conversation_id: "conv-1",
        tool_name: "web",
        capture_source: "ebpf",
      },
      Envelope: {
        networkEvent: {
          endpoint: "api.openai.com:443",
          direction: "egress",
          bytes: 2048,
        },
      },
    });

    expect(presentation).not.toBeNull();
    expect(presentation?.category).toBe("network");
    expect(presentation?.title).toBe("建立网络连接");
    expect(presentation?.targetLabel).toBe("api.openai.com:443");
    expect(presentation?.outcome).toBe("已阻断");
    expect(presentation?.tone).toBe("danger");
    expect(presentation?.riskScore).toBe(88);
    expect(
      presentation?.sections.find((section) => section.key === "agent")?.fields
        .map((field) => field.value),
    ).toContain("run-1");
    expect(presentation?.envelopeFields.some((field) => field.value === "egress")).toBe(true);
  });

  test("accepts camelCase payloads and keeps raw protocol fields out of the title", () => {
    const presentation = presentRenewEventDetail({
      timestamp: 1791165600000,
      event: {
        type: "wrapper_intercept",
        comm: "claude",
        pid: 99,
        toolName: "mcp__github__search",
        toolCallId: "tool-7",
        riskScore: 35,
        decision: "ALLOW",
      },
      envelope: {
        wrapperEvent: {
          toolName: "mcp__github__search",
          commandDigest: "abc123",
        },
      },
    });

    expect(presentation?.category).toBe("agent");
    expect(presentation?.title).toBe("Agent 调用工具");
    expect(presentation?.outcome).toBe("已允许");
    expect(presentation?.tone).toBe("normal");
    expect(presentation?.processLabel).toBe("claude · PID 99");
    expect(
      presentation?.sections
        .flatMap((section) => section.fields)
        .some((field) => field.value === "tool-7"),
    ).toBe(true);
  });

  test("keeps missing risk scores distinct from an explicit zero score", () => {
    const unscored = presentRenewEventDetail({
      Timestamp: 1791165600000,
      Event: {
        type: "read",
        comm: "codex",
        pid: 12,
        path: "/tmp/a",
      },
    });
    const zeroRisk = presentRenewEventDetail({
      Timestamp: 1791165600000,
      Event: {
        type: "read",
        comm: "codex",
        pid: 12,
        path: "/tmp/a",
        risk_score: 0,
      },
    });

    expect(unscored?.riskAvailable).toBe(false);
    expect(unscored?.riskLabel).toBe("未评分");
    expect(zeroRisk?.riskAvailable).toBe(true);
    expect(zeroRisk?.riskLabel).toBe("未发现风险");
  });

  test("renders backend detail errors as a visual error state", () => {
    const presentation = presentRenewEventDetail({
      error: "event not found",
      eventId: "missing",
    });

    expect(presentation?.error).toBe("event not found");
    expect(presentation?.title).toBe("无法读取事件详情");
    expect(presentation?.sections).toHaveLength(0);
  });
});
