import type { AgentEvent } from "../dashboard/useDashboard";
import type { RenewTone } from "./types";

export const normalizedDecision = (event: AgentEvent) =>
  String(event.decision || "").trim().toUpperCase();

export const isAgentEvent = (event: AgentEvent) =>
  Boolean(
    event.agentRunId ||
      event.conversationId ||
      event.toolCallId ||
      event.rootAgentPid ||
      (event.tag && event.tag !== "Unknown"),
  );

export const isAttentionEvent = (event: AgentEvent) => {
  const decision = normalizedDecision(event);
  return (
    decision.includes("BLOCK") ||
    decision.includes("DENY") ||
    decision.includes("ALERT") ||
    (event.riskScore ?? 0) >= 60 ||
    event.type === "semantic_alert" ||
    event.type === "agentsight_alert"
  );
};

export const describeEvent = (event: AgentEvent) => {
  const target = event.path || event.netEndpoint || event.extraPath || "";
  switch (event.type) {
    case "execve":
    case "process_exec":
      return target
        ? `${event.comm || "进程"} 启动了 ${target}`
        : `${event.comm || "进程"} 启动了一个程序`;
    case "openat":
    case "open":
    case "read":
      return target ? `读取 ${target}` : "读取了文件";
    case "write":
      return target ? `写入 ${target}` : "写入了文件";
    case "unlink":
      return target ? `删除 ${target}` : "删除了文件";
    case "rename":
      return target ? `重命名 ${target}` : "重命名了文件";
    case "network_connect":
    case "tcp_connect":
      return target ? `连接到 ${target}` : "建立了网络连接";
    case "dns_query":
      return target ? `查询域名 ${target}` : "发起了 DNS 查询";
    case "wrapper_intercept":
      return event.toolName
        ? `调用工具 ${event.toolName}`
        : "执行了一次受监控的工具调用";
    case "native_hook":
      return event.toolName
        ? `Agent 调用 ${event.toolName}`
        : "收到 Agent 生命周期事件";
    case "semantic_alert":
    case "agentsight_alert":
      return target ? `检测到异常：${target}` : "检测到需要关注的行为";
    default:
      return target
        ? `${event.comm || "进程"} · ${event.type} · ${target}`
        : `${event.comm || "进程"} · ${event.type}`;
  }
};

export const eventTime = (event: AgentEvent) => {
  if (event.receivedAtMs) {
    return new Date(event.receivedAtMs).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }
  return event.time || "刚刚";
};

export const eventTone = (event: AgentEvent): RenewTone => {
  const decision = normalizedDecision(event);
  if (
    decision.includes("BLOCK") ||
    decision.includes("DENY") ||
    (event.riskScore ?? 0) >= 80
  ) {
    return "danger";
  }
  if (
    decision.includes("ALERT") ||
    (event.riskScore ?? 0) >= 60 ||
    event.type === "semantic_alert" ||
    event.type === "agentsight_alert"
  ) {
    return "warning";
  }
  return "normal";
};

export const eventLabel = (event: AgentEvent) => {
  const decision = normalizedDecision(event);
  if (decision && decision !== "ALLOW") return decision;
  if ((event.riskScore ?? 0) >= 60) return `风险 ${event.riskScore}`;
  return event.tag && event.tag !== "Unknown" ? event.tag : event.type;
};
