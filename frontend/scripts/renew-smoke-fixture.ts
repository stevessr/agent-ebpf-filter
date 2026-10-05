// Local-only, deterministic browser smoke fixture. Never imported by the app.
// bun scripts/renew-smoke-fixture.ts; VITE_AGENT_BACKEND_URL=http://127.0.0.1:48783 bun run dev
import { pb } from "../src/pb/tracker_pb.js";
const runtime: any = {
  disabledEventTypes: [],
  loopDetection: { enabled: true },
  signalProcessing: { enabled: true },
  researchProcessing: { enabled: false },
  tlsCaptureEnabled: false,
  logPersistenceEnabled: true,
};
const rules: Record<string, any> = {};
const updates: any[] = [];
let failNext = false;
const events = [
  {
    key: "codex-network",
    eventId: "codex-network",
    pid: 100,
    ppid: 1,
    rootAgentPid: 100,
    tag: "Codex",
    comm: "codex",
    agentRunId: "same-run",
    type: "network_connect",
    netEndpoint: "codex.test:443",
    netDirection: "OUT",
    netBytes: 100,
  },
  {
    key: "claude-network",
    eventId: "claude-network",
    pid: 200,
    ppid: 1,
    rootAgentPid: 200,
    tag: "Claude Code",
    comm: "node",
    agentRunId: "same-run",
    type: "network_connect",
    netEndpoint: "claude.test:443",
    netDirection: "OUT",
    netBytes: 200,
  },
  {
    key: "gemini-file",
    eventId: "gemini-file",
    pid: 300,
    ppid: 1,
    tag: "Gemini CLI",
    comm: "node",
    agentRunId: "gemini-run",
    type: "read",
    path: "/tmp/gemini.txt",
  },
  {
    key: "unknown",
    eventId: "unknown",
    pid: 400,
    tag: "Unknown",
    comm: "bun",
    type: "read",
    path: "/tmp/codex-is-not-identity",
  },
].map((e, i) => ({ ...e, receivedAtMs: Date.now() - i * 1000 }));
const processes = [
  {
    pid: 100,
    ppid: 1,
    name: "codex",
    cmdline: "/usr/bin/codex",
    cpu: 12,
    mem: 100000000,
    user: "fixture",
  },
  {
    pid: 101,
    ppid: 100,
    name: "curl",
    cmdline: "curl codex.test",
    cpu: 1,
    mem: 1000000,
    user: "fixture",
  },
  {
    pid: 200,
    ppid: 1,
    name: "node",
    cmdline: "node /opt/node_modules/@anthropic-ai/claude-code/cli.js",
    cpu: 8,
    mem: 80000000,
    user: "fixture",
  },
  {
    pid: 300,
    ppid: 1,
    name: "node",
    cmdline: "node /opt/node_modules/@google/gemini-cli/bin/gemini.js",
    cpu: 5,
    mem: 50000000,
    user: "fixture",
  },
  {
    pid: 400,
    ppid: 1,
    name: "bun",
    cmdline: "bun app.ts",
    cpu: 1,
    mem: 10000000,
    user: "fixture",
  },
];
const port = Number(process.env.RENEW_FIXTURE_PORT || 48783);
Bun.serve<{ path: string }>({
  hostname: "127.0.0.1",
  port,
  async fetch(req, server) {
    const url = new URL(req.url);
    const path = url.pathname;
    if (path.startsWith("/ws/")) {
      if (server.upgrade(req, { data: { path } })) return;
      return new Response("upgrade failed", { status: 400 });
    }
    if (path === "/__fixture/fail-next") {
      failNext = true;
      return Response.json({ ok: true });
    }
    if (path === "/__fixture/state")
      return Response.json({ runtime, rules, updates });
    if (path === "/config/runtime") {
      if (req.method === "PUT") {
        if (failNext) {
          failNext = false;
          return Response.json(
            { error: "fixture rejected update" },
            { status: 403 },
          );
        }
        const patch = await req.json();
        updates.push(patch);
        Object.assign(runtime, patch);
      }
      return Response.json({ runtime });
    }
    if (path === "/config/event-types")
      return Response.json({
        disabled_event_types: runtime.disabledEventTypes,
      });
    if (path === "/events/summaries") return Response.json({ events });
    if (path.startsWith("/events/detail/"))
      return Response.json({
        event: events.find(
          (e) => e.eventId === decodeURIComponent(path.split("/").at(-1)!),
        ),
      });
    if (path === "/system/collector-health")
      return Response.json({ captureHealthy: true, ringbufDroppedTotal: 0 });
    if (path === "/system/tracked-comms")
      return Response.json(["codex", "node"]);
    if (path === "/config/rules") {
      if (req.method === "POST") {
        const rule = await req.json();
        rules[rule.comm] = rule;
      }
      return Response.json(rules);
    }
    if (path.startsWith("/config/rules/") && req.method === "DELETE") {
      delete rules[decodeURIComponent(path.split("/").at(-1)!)];
      return Response.json({ ok: true });
    }
    return Response.json({});
  },
  websocket: {
    open(ws) {
      if (ws.data.path === "/ws/system")
        ws.send(
          pb.SystemStats.encode(
            pb.SystemStats.fromObject({
              processes,
              cpu: { total: 25 },
              memory: { total: 16000000000, used: 4000000000, percent: 25 },
            }),
          ).finish(),
        );
      else ws.send(JSON.stringify({ events }));
    },
    message() {},
  },
});
console.log(
  `Renew smoke fixture: http://127.0.0.1:${port} (synthetic data, no root)`,
);
