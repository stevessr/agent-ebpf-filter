import { expect, test } from "bun:test";
import { ref } from "vue";
import {
  identifyHarness,
  attributeEvents,
  eventHarness,
  processHarness,
  attributeProcesses,
  sessionKey,
} from "../src/composables/renew/harness";
import { useRenewEventSummary } from "../src/composables/renew/useRenewEventSummary";
import type { AgentEvent } from "../src/composables/dashboard/dashboardConstants";
import type { ProcessInfo } from "../src/composables/monitor/useMonitorData";
const event = (patch: Partial<AgentEvent> = {}): AgentEvent => ({
  key: "a",
  pid: 42,
  comm: "node",
  tag: "Unknown",
  type: "read",
  time: "",
  ...patch,
});
const process = (patch: Partial<ProcessInfo> = {}): ProcessInfo => ({
  pid: 42,
  ppid: 1,
  name: "node",
  cmdline: "node app.js",
  cpu: 0,
  mem: 0,
  user: "me",
  gpuMem: 0,
  gpuId: 0,
  gpuUtil: 0,
  createTime: 100,
  minorFaults: 0,
  majorFaults: 0,
  ...patch,
});
test("identifies harnesses, not API providers or arbitrary file content", () => {
  expect(identifyHarness("Claude Code")).toBe("claude");
  expect(identifyHarness("Gemini CLI")).toBe("gemini");
  expect(identifyHarness("DeepSeek Harness")).toBe("dsh");
  expect(identifyHarness("openai")).toBe("unknown");
  expect(identifyHarness("constructor")).toBe("unknown");
  expect(
    eventHarness(event({ path: "/tmp/codex", domain: "api.anthropic.com" })),
  ).toBe("unknown");
  expect(eventHarness(event({ tag: "Claude Code", comm: "codex" }))).toBe(
    "claude",
  );
  expect(
    processHarness(
      process({ cmdline: "node /opt/node_modules/@openai/codex/bin/codex.js" }),
    ),
  ).toBe("codex");
  expect(
    processHarness(
      process({ name: "grep", cmdline: "grep @openai/codex/bin/codex.js" }),
    ),
  ).toBe("unknown");
  expect(
    processHarness(process({ cmdline: "node app.js --prompt=codex" })),
  ).toBe("unknown");
});
test("process descendants inherit nearest harness with bounded parent traversal", () => {
  const rows = attributeProcesses([
    process({ pid: 10, name: "codex" }),
    process({ pid: 20, ppid: 10, name: "bash" }),
    process({ pid: 30, ppid: 20, name: "curl" }),
    process({ pid: 40, name: "claude" }),
    process({ pid: 50, ppid: 50 }),
  ]);
  expect(rows[2]!.harness).toBe("codex");
  expect(rows[2]!.rootPid).toBe(10);
  expect(rows[3]!.harness).toBe("claude");
  expect(rows[4]!.harness).toBe("unknown");
  expect(
    attributeProcesses([
      process({ pid: 10, name: "codex", createTime: 200 }),
      process({ ppid: 10, createTime: 100 }),
    ])[1]!.harness,
  ).toBe("unknown");
});
test("same run IDs across harnesses stay separate; distinct sessions stay separate", () => {
  const events = [
    event({ tag: "Codex", agentRunId: "same" }),
    event({ key: "b", tag: "Claude Code", agentRunId: "same" }),
    event({ key: "c", tag: "Codex", agentRunId: "different" }),
  ];
  expect(sessionKey(events[0]!)).not.toBe(sessionKey(events[1]!));
  const summary = useRenewEventSummary(ref(events), ref(""), ref(true));
  expect(summary.activeAgents.value).toHaveLength(3);
  expect(summary.activeAgents.value.map((s) => s.label)).toContain(
    "Codex · different",
  );
});

test("generic descendants inherit explicit session identity; ambiguous contexts stay unknown", () => {
  const generic = event({ agentRunId: "run", rootAgentPid: 10, comm: "curl" });
  const identified = event({ ...generic, key: "native", tag: "Codex" });
  expect(eventHarness(attributeEvents([generic, identified])[0]!)).toBe(
    "codex",
  );
  expect(
    eventHarness(
      attributeEvents([
        generic,
        identified,
        event({ ...identified, tag: "Claude Code" }),
      ])[0]!,
    ),
  ).toBe("unknown");
  expect(
    eventHarness(
      attributeEvents([
        event({ ...generic, rootAgentPid: 20 }),
        identified,
      ])[0]!,
    ),
  ).toBe("unknown");
  expect(
    eventHarness(
      attributeEvents([
        event({ comm: "curl", rootAgentPid: 10 }),
        identified,
      ])[0]!,
    ),
  ).toBe("unknown");
});
