import type { AgentEvent } from "../dashboard/dashboardConstants";
import {
  eventTone,
  isAttentionEvent,
  normalizedDecision,
} from "./eventPresentation";

export type RenewActivityCategory =
  | "file"
  | "network"
  | "process"
  | "agent"
  | "alert"
  | "other";

export interface RenewActivityBucket {
  key: string;
  label: string;
  count: number;
  attention: number;
  normal: number;
}

export interface RenewCategorySlice {
  key: RenewActivityCategory;
  label: string;
  count: number;
  share: number;
}

export interface RenewSafetySlice {
  key: "normal" | "warning" | "danger";
  label: string;
  count: number;
  share: number;
}

const categoryOrder: Array<{ key: RenewActivityCategory; label: string }> = [
  { key: "file", label: "文件" },
  { key: "network", label: "网络" },
  { key: "process", label: "进程" },
  { key: "agent", label: "Agent / 工具" },
  { key: "alert", label: "告警" },
  { key: "other", label: "其他" },
];

export function renewEventCategory(event: AgentEvent): RenewActivityCategory {
  const type = String(event.type || "").toLowerCase();
  if (type.includes("alert")) return "alert";
  if (
    type.includes("network") ||
    type.startsWith("tcp_") ||
    type.includes("dns") ||
    type.includes("socket") ||
    type.includes("accept") ||
    type.includes("http") ||
    type.includes("tls") ||
    type.includes("sse")
  ) {
    return "network";
  }
  if (
    ["open", "openat", "read", "write", "unlink", "rename", "chmod", "chown", "link", "symlink", "mkdir", "mknod", "ioctl", "stdio"].includes(type)
  ) {
    return "file";
  }
  if (
    type.includes("exec") ||
    type.includes("fork") ||
    type.includes("clone") ||
    type.includes("exit") ||
    type === "wait4"
  ) {
    return "process";
  }
  if (
    type.includes("wrapper") ||
    type.includes("hook") ||
    type.includes("otel") ||
    event.toolName ||
    event.agentRunId ||
    event.conversationId
  ) {
    return "agent";
  }
  return "other";
}

const eventTimestamp = (event: AgentEvent) => {
  const explicit = Number(event.receivedAtMs || 0);
  if (Number.isFinite(explicit) && explicit > 0) return explicit;
  const parsed = Date.parse(event.time);
  return Number.isFinite(parsed) ? parsed : 0;
};

export function buildRenewActivityBuckets(
  events: AgentEvent[],
  now = Date.now(),
  bucketCount = 12,
  bucketMinutes = 5,
): RenewActivityBucket[] {
  const safeCount = Math.max(1, Math.min(48, Math.floor(bucketCount)));
  const safeMinutes = Math.max(1, Math.min(60, Math.floor(bucketMinutes)));
  const bucketMs = safeMinutes * 60_000;
  const start = now - safeCount * bucketMs;

  const buckets = Array.from({ length: safeCount }, (_, index) => {
    const bucketStart = start + index * bucketMs;
    return {
      key: String(bucketStart),
      label: new Date(bucketStart).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      }),
      count: 0,
      attention: 0,
      normal: 0,
    };
  });

  for (const event of events) {
    const timestamp = eventTimestamp(event);
    if (!timestamp || timestamp < start || timestamp > now) continue;
    const index = Math.min(
      safeCount - 1,
      Math.max(0, Math.floor((timestamp - start) / bucketMs)),
    );
    const bucket = buckets[index];
    bucket.count += 1;
    if (isAttentionEvent(event)) bucket.attention += 1;
    else bucket.normal += 1;
  }
  return buckets;
}

export function buildRenewCategorySlices(
  events: AgentEvent[],
): RenewCategorySlice[] {
  const counts = new Map<RenewActivityCategory, number>();
  for (const event of events) {
    const category = renewEventCategory(event);
    counts.set(category, (counts.get(category) || 0) + 1);
  }
  const total = Math.max(1, events.length);
  return categoryOrder.map(({ key, label }) => ({
    key,
    label,
    count: counts.get(key) || 0,
    share: ((counts.get(key) || 0) / total) * 100,
  }));
}

export function buildRenewSafetySlices(events: AgentEvent[]): RenewSafetySlice[] {
  const counts = { normal: 0, warning: 0, danger: 0 };
  for (const event of events) {
    const decision = normalizedDecision(event);
    const tone = eventTone(event);
    if (
      decision.includes("BLOCK") ||
      decision.includes("DENY") ||
      tone === "danger"
    ) {
      counts.danger += 1;
    } else if (tone === "warning") {
      counts.warning += 1;
    } else {
      counts.normal += 1;
    }
  }
  const total = Math.max(1, events.length);
  return [
    {
      key: "normal",
      label: "正常",
      count: counts.normal,
      share: (counts.normal / total) * 100,
    },
    {
      key: "warning",
      label: "需关注",
      count: counts.warning,
      share: (counts.warning / total) * 100,
    },
    {
      key: "danger",
      label: "高风险 / 阻断",
      count: counts.danger,
      share: (counts.danger / total) * 100,
    },
  ];
}
