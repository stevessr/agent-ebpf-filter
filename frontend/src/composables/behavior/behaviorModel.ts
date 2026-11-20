import { eventCategories, type AgentEvent } from "../dashboard/dashboardConstants";
import { pb } from "../../pb/tracker_pb.js";

export type BehaviorLane = "file" | "network" | "process" | "other";
export const BEHAVIOR_LANES: readonly BehaviorLane[] = ["file", "network", "process", "other"];

export interface BehaviorSnapshot {
  events: AgentEvent[];
  lanes: { key: BehaviorLane; count: number }[];
  processes: { pid: number; comm: string; count: number; attention: number }[];
  histogram: number[];
  rate: number;
  attentionCount: number;
  byteCount: number;
  maxRisk: number | null;
}

// Share schema-backed categories with the dashboard rather than duplicating enum values.
const numericLanes = new Map<number, BehaviorLane>();
for (const lane of ["file", "network", "process"] as const) {
  for (const type of eventCategories[lane] ?? []) numericLanes.set(type, lane);
}
for (const type of [pb.EventType.TLS_PLAINTEXT, pb.EventType.HTTP_MESSAGE, pb.EventType.SSE_MESSAGE]) numericLanes.set(type, "network");
const stringLanes = new Map<string, BehaviorLane>();
for (const type of ["openat", "open", "read", "write", "mkdir", "unlink", "ioctl", "chmod", "chown", "rename", "link", "symlink", "mknod"]) stringLanes.set(type, "file");
for (const type of ["network_connect", "network_bind", "network_sendto", "network_recvfrom", "accept", "accept4", "socket", "tcp_connect", "tcp_close", "tcp_state_change", "dns_query", "tls_plaintext", "http_message", "sse_message", "socket_http"]) stringLanes.set(type, "network");
for (const type of ["execve", "clone", "exit", "process_fork", "process_exec", "process_exit", "wait4"]) stringLanes.set(type, "process");

/** Classify only known event identities; generic and unknown events remain other. */
export function laneForEvent(event: AgentEvent): BehaviorLane {
  if (typeof event.eventType === "number" && Number.isInteger(event.eventType)) {
    const lane = numericLanes.get(event.eventType);
    if (lane !== undefined) return lane;
    if (pb.EventType[event.eventType] !== undefined) return "other";
  }
  return stringLanes.get(event.type.trim().toLowerCase()) ?? "other";
}

/** Marks noteworthy risk/policy evidence, without claiming execution was blocked. */
export function isAttentionEvent(event: AgentEvent): boolean {
  const decision = event.decision?.trim().toUpperCase();
  if (event.eventType === pb.EventType.SEMANTIC_ALERT || event.eventType === pb.EventType.AGENTSIGHT_ALERT || /^(semantic_alert|agentsight_alert)$/.test(event.type.trim().toLowerCase())) return true;
  return (typeof event.riskScore === "number" && Number.isFinite(event.riskScore) && event.riskScore >= 60) || decision === "ALERT" || decision === "BLOCK" || decision === "DENY";
}

const BUCKETS = 30;
function nonNegative(value: number | undefined): number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : 0;
}

/** Return an immutable-input, time-bounded projection suitable for behavior charts. */
export function buildBehaviorSnapshot(events: readonly AgentEvent[], nowMs: number, windowMs: number, pid?: number): BehaviorSnapshot {
  const duration = Number.isFinite(windowMs) && windowMs > 0 ? windowMs : 0;
  const start = nowMs - duration;
  const retained = events.filter((event) => {
    const time = event.receivedAtMs;
    return Number.isFinite(time) && time! >= start && time! <= nowMs && (pid === undefined || event.pid === pid);
  }).slice().sort((a, b) => b.receivedAtMs! - a.receivedAtMs!);
  const laneCounts = new Map<BehaviorLane, number>(BEHAVIOR_LANES.map((lane) => [lane, 0]));
  const byPid = new Map<number, { comm: string; count: number; attention: number }>();
  const histogram = Array<number>(BUCKETS).fill(0);
  let attentionCount = 0;
  let byteCount = 0;
  let maxRisk: number | null = null;

  for (const event of retained) {
    const lane = laneForEvent(event);
    laneCounts.set(lane, laneCounts.get(lane)! + 1);
    const proc = byPid.get(event.pid) ?? { comm: event.comm, count: 0, attention: 0 };
    proc.count++;
    if (isAttentionEvent(event)) { proc.attention++; attentionCount++; }
    byPid.set(event.pid, proc);
    const chartTime = Math.min(event.receivedAtMs!, nowMs);
    const bucket = Math.max(0, Math.min(BUCKETS - 1, duration === 0 ? 0 : Math.floor(((chartTime - start) / duration) * BUCKETS)));
    histogram[bucket]++;
    const netBytes = event.netBytes;
    byteCount += typeof netBytes === "number" && Number.isFinite(netBytes) && netBytes >= 0
      ? netBytes
      : nonNegative(event.bytes);
    if (typeof event.riskScore === "number" && Number.isFinite(event.riskScore)) maxRisk = maxRisk === null ? event.riskScore : Math.max(maxRisk, event.riskScore);
  }

  const rateWindow = Math.min(5_000, duration);
  const recent = rateWindow > 0 ? retained.filter((event) => event.receivedAtMs! >= nowMs - rateWindow && event.receivedAtMs! <= nowMs).length : 0;
  const processes = [...byPid].map(([processPid, value]) => ({ pid: processPid, ...value }))
    .sort((a, b) => b.count - a.count || a.pid - b.pid || a.comm.localeCompare(b.comm));
  return {
    events: retained,
    lanes: BEHAVIOR_LANES.map((key) => ({ key, count: laneCounts.get(key)! })),
    processes,
    histogram,
    rate: rateWindow > 0 ? recent / (rateWindow / 1_000) : 0,
    attentionCount,
    byteCount,
    maxRisk,
  };
}

/** Build deterministic, prefixed sample events, wholly separate from live input. */
export function createBehaviorDemo(nowMs: number): AgentEvent[] {
  const programs = [{ pid: 4101, comm: "node" }, { pid: 4102, comm: "claude" }, { pid: 4103, comm: "git" }, { pid: 4104, comm: "python" }];
  const patterns: { type: string; path: string; bytes: number; netBytes: number; riskScore?: number; decision?: string }[] = [
    { type: "openat", path: "/workspace/src/index.ts", bytes: 0, netBytes: 0 },
    { type: "read", path: "/workspace/package.json", bytes: 1536, netBytes: 0 },
    { type: "write", path: "/tmp/agent-output.json", bytes: 720, netBytes: 0 },
    { type: "network_connect", path: "", bytes: 0, netBytes: 256 },
    { type: "network_sendto", path: "", bytes: 0, netBytes: 2048 },
    { type: "execve", path: "/usr/bin/git", bytes: 0, netBytes: 0 },
    { type: "process_fork", path: "", bytes: 0, netBytes: 0 },
    { type: "unlink", path: "/tmp/old-cache", bytes: 0, netBytes: 0 },
    { type: "network_recvfrom", path: "", bytes: 0, netBytes: 4096 },
    { type: "openat", path: "/home/user/.ssh/config", bytes: 0, netBytes: 0, riskScore: 78, decision: "ALERT" },
    { type: "network_connect", path: "", bytes: 0, netBytes: 128, riskScore: 91, decision: "BLOCK" },
    { type: "write", path: "/workspace/generated.py", bytes: 2400, netBytes: 0, riskScore: 63 },
    { type: "stdio", path: "", bytes: 512, netBytes: 0 },
  ];
  return Array.from({ length: 150 }, (_, index) => {
    const pattern = patterns[index % patterns.length]!;
    const program = programs[(index * 7 + Math.floor(index / patterns.length)) % programs.length]!;
    const receivedAtMs = nowMs - Math.floor(index * 60_000 / 150);
    return {
      key: "demo-behavior-" + index,
      eventId: "demo-behavior-event-" + index,
      pid: program.pid, ppid: 1, uid: 1000,
      type: pattern.type, tag: program.comm === "git" ? "Git" : "AI Agent", comm: program.comm,
      path: pattern.path, bytes: pattern.bytes, netBytes: pattern.type.startsWith("network_") ? pattern.netBytes : undefined,
      netEndpoint: pattern.type.startsWith("network_") ? (pattern.decision === "BLOCK" ? "203.0.113.42:443" : "api.example.test:443") : undefined,
      riskScore: pattern.riskScore, decision: pattern.decision,
      time: new Date(receivedAtMs).toISOString(), receivedAtMs,
    };
  });
}
