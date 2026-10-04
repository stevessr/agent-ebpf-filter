import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";

import { useDashboard, type AgentEvent } from "../dashboard/useDashboard";
import { useMonitorData } from "../monitor/useMonitorData";

export function useRenewDashboard() {
  const router = useRouter();

  const {
    events,
    isConnected,
    isPaused,
  } = useDashboard();

  const {
    processes,
    systemStats,
    trackedProcesses,
    formatBytesWithUnit,
    fetchTrackedComms,
    setup,
    teardown,
  } = useMonitorData();

  const search = ref("");
  const onlyAgents = ref(true);

  onMounted(() => {
    setup();
    void fetchTrackedComms();
  });

  onUnmounted(() => {
    teardown();
  });

  const normalizedDecision = (event: AgentEvent) =>
    String(event.decision || "").trim().toUpperCase();

  const isAgentEvent = (event: AgentEvent) =>
    Boolean(
      event.agentRunId ||
        event.conversationId ||
        event.toolCallId ||
        event.rootAgentPid ||
        (event.tag && event.tag !== "Unknown"),
    );

  const isAttentionEvent = (event: AgentEvent) => {
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

  const describeEvent = (event: AgentEvent) => {
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

  const recentEvents = computed(() => {
    const query = search.value.trim().toLowerCase();
    return events.value
      .filter((event) => !onlyAgents.value || isAgentEvent(event))
      .filter((event) => {
        if (!query) return true;
        return [
          event.tag,
          event.comm,
          event.type,
          event.path,
          event.netEndpoint,
          event.toolName,
          event.decision,
        ]
          .filter(Boolean)
          .some((value) => String(value).toLowerCase().includes(query));
      })
      .slice(0, 16);
  });

  const attentionCount = computed(
    () => events.value.filter(isAttentionEvent).length,
  );

  const attentionEvents = computed(() =>
    events.value.filter(isAttentionEvent).slice(0, 8),
  );

  const blockedCount = computed(
    () =>
      events.value.filter((event) => {
        const decision = normalizedDecision(event);
        return decision.includes("BLOCK") || decision.includes("DENY");
      }).length,
  );

  const agentEventCount = computed(
    () => events.value.filter(isAgentEvent).length,
  );

  const activeAgents = computed(() => {
    const runs = new Map<
      string,
      {
        key: string;
        label: string;
        events: number;
        alerts: number;
        lastSeen: number;
        lastAction: string;
      }
    >();

    for (const event of events.value) {
      if (!isAgentEvent(event)) continue;
      const key =
        event.agentRunId ||
        event.conversationId ||
        `${event.comm}:${event.rootAgentPid || event.pid}`;
      const previous = runs.get(key);
      const receivedAt = event.receivedAtMs || Date.now();
      const label =
        event.tag && event.tag !== "Unknown" ? event.tag : event.comm || "Agent";

      if (!previous) {
        runs.set(key, {
          key,
          label,
          events: 1,
          alerts: isAttentionEvent(event) ? 1 : 0,
          lastSeen: receivedAt,
          lastAction: describeEvent(event),
        });
        continue;
      }

      previous.events += 1;
      if (isAttentionEvent(event)) previous.alerts += 1;
      if (receivedAt >= previous.lastSeen) {
        previous.lastSeen = receivedAt;
        previous.lastAction = describeEvent(event);
      }
    }

    return [...runs.values()]
      .sort((a, b) => b.lastSeen - a.lastSeen)
      .slice(0, 6);
  });

  const topProcesses = computed(() =>
    [...processes.value].sort((a, b) => b.cpu - a.cpu).slice(0, 5),
  );

  const topDestinations = computed(() => {
    const counts = new Map<string, number>();
    for (const event of events.value) {
      if (!event.netEndpoint) continue;
      counts.set(event.netEndpoint, (counts.get(event.netEndpoint) || 0) + 1);
    }
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([endpoint, count]) => ({ endpoint, count }));
  });

  const connectionLabel = computed(() =>
    isConnected.value ? "实时采集中" : "等待采集端",
  );

  const alertTone = computed(() => {
    if (blockedCount.value > 0) return "danger";
    if (attentionCount.value > 0) return "warning";
    return "normal";
  });

  const formatRate = (bytes: number) => `${formatBytesWithUnit(bytes)}/s`;

  const eventTime = (event: AgentEvent) => {
    if (event.receivedAtMs) {
      return new Date(event.receivedAtMs).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
      });
    }
    return event.time || "刚刚";
  };

  const eventTone = (event: AgentEvent) => {
    const decision = normalizedDecision(event);
    if (
      decision.includes("BLOCK") ||
      decision.includes("DENY") ||
      (event.riskScore ?? 0) >= 80
    )
      return "danger";
    if (
      decision.includes("ALERT") ||
      (event.riskScore ?? 0) >= 60 ||
      event.type === "semantic_alert" ||
      event.type === "agentsight_alert"
    )
      return "warning";
    return "normal";
  };

  const eventLabel = (event: AgentEvent) => {
    const decision = normalizedDecision(event);
    if (decision && decision !== "ALLOW") return decision;
    if ((event.riskScore ?? 0) >= 60) return `风险 ${event.riskScore}`;
    return event.tag && event.tag !== "Unknown" ? event.tag : event.type;
  };

  const go = (name: string, params?: Record<string, string>) => {
    void router.push({ name, params });
  };

  return {
    events,
    isConnected,
    isPaused,
    processes,
    systemStats,
    trackedProcesses,
    search,
    onlyAgents,
    recentEvents,
    attentionEvents,
    attentionCount,
    blockedCount,
    agentEventCount,
    activeAgents,
    topProcesses,
    topDestinations,
    connectionLabel,
    alertTone,
    formatBytesWithUnit,
    formatRate,
    describeEvent,
    eventTime,
    eventTone,
    eventLabel,
    go,
  };
}
