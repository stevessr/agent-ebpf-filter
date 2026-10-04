<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  AlertOutlined,
  AppstoreOutlined,
  ArrowRightOutlined,
  CheckCircleFilled,
  DashboardOutlined,
  GlobalOutlined,
  NodeIndexOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
} from "@ant-design/icons-vue";

import { useDashboard, type AgentEvent } from "../../composables/dashboard/useDashboard";
import { useMonitorData } from "../../composables/monitor/useMonitorData";

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
  if (attentionEvents.value.length > 0) return "warning";
  return "normal";
});

const formatRate = (bytes: number) => `${formatBytesWithUnit(bytes)}/s`;

function describeEvent(event: AgentEvent) {
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
}

function eventTime(event: AgentEvent) {
  if (event.receivedAtMs) {
    return new Date(event.receivedAtMs).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }
  return event.time || "刚刚";
}

function eventTone(event: AgentEvent) {
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
}

function eventLabel(event: AgentEvent) {
  if (normalizedDecision(event)) return normalizedDecision(event);
  if ((event.riskScore ?? 0) >= 60) return `风险 ${event.riskScore}`;
  return event.tag && event.tag !== "Unknown" ? event.tag : event.type;
}

const go = (name: string, params?: Record<string, string>) => {
  void router.push({ name, params });
};
</script>

<template>
  <div class="renew-shell">
    <aside class="renew-sidebar">
      <div class="renew-brand">
        <div class="renew-brand__mark">R</div>
        <div>
          <strong>Renew</strong>
          <span>Agent Monitor</span>
        </div>
      </div>

      <nav class="renew-nav" aria-label="Renew navigation">
        <button class="renew-nav__item renew-nav__item--active">
          <DashboardOutlined />
          <span>概览</span>
        </button>
        <button class="renew-nav__item" @click="go('Dashboard')">
          <AppstoreOutlined />
          <span>事件</span>
        </button>
        <button class="renew-nav__item" @click="go('NetworkFlow', { tab: 'overview' })">
          <GlobalOutlined />
          <span>网络</span>
        </button>
        <button class="renew-nav__item" @click="go('Monitor', { tab: 'processes' })">
          <NodeIndexOutlined />
          <span>进程</span>
        </button>
        <button class="renew-nav__item" @click="go('Config', { tab: 'security' })">
          <SafetyCertificateOutlined />
          <span>规则</span>
        </button>
      </nav>

      <div class="renew-sidebar__footer">
        <button class="renew-nav__item" @click="go('Config', { tab: 'runtime' })">
          <SettingOutlined />
          <span>设置</span>
        </button>
        <button class="renew-professional" @click="go('Dashboard')">
          打开专业工作台
          <ArrowRightOutlined />
        </button>
      </div>
    </aside>

    <main class="renew-main">
      <header class="renew-header">
        <div>
          <div class="renew-eyebrow">DAILY MONITORING</div>
          <h1>今天的 Agent 活动</h1>
          <p>把内核事件整理成日常可读的状态、异常和最近动作。</p>
        </div>
        <div class="renew-header__actions">
          <div class="renew-live" :class="{ 'renew-live--offline': !isConnected }">
            <span class="renew-live__dot" />
            {{ connectionLabel }}
          </div>
          <button
            class="renew-icon-button"
            :title="isPaused ? '继续事件流' : '暂停事件流'"
            @click="isPaused = !isPaused"
          >
            <PlayCircleOutlined v-if="isPaused" />
            <PauseCircleOutlined v-else />
          </button>
        </div>
      </header>

      <section class="renew-metrics" aria-label="System overview">
        <article class="renew-metric">
          <div class="renew-metric__label">采集状态</div>
          <div class="renew-metric__value">
            {{ isConnected ? "Online" : "Offline" }}
          </div>
          <div class="renew-metric__meta">
            当前缓冲区 {{ events.length }} 条 · Agent {{ agentEventCount }} 条
          </div>
        </article>

        <article class="renew-metric">
          <div class="renew-metric__label">CPU</div>
          <div class="renew-metric__value">
            {{ systemStats.cpuTotal.toFixed(1) }}%
          </div>
          <div class="renew-meter">
            <span :style="{ width: Math.min(100, systemStats.cpuTotal) + '%' }" />
          </div>
          <div class="renew-metric__meta">{{ processes.length }} 个进程</div>
        </article>

        <article class="renew-metric">
          <div class="renew-metric__label">内存</div>
          <div class="renew-metric__value">
            {{ systemStats.memPercent.toFixed(1) }}%
          </div>
          <div class="renew-meter">
            <span :style="{ width: Math.min(100, systemStats.memPercent) + '%' }" />
          </div>
          <div class="renew-metric__meta">
            {{ formatBytesWithUnit(systemStats.memUsed) }} /
            {{ formatBytesWithUnit(systemStats.memTotal) }}
          </div>
        </article>

        <article class="renew-metric" :class="`renew-metric--${alertTone}`">
          <div class="renew-metric__label">需要关注</div>
          <div class="renew-metric__value">{{ attentionEvents.length }}</div>
          <div class="renew-metric__meta">
            已阻断 {{ blockedCount }} ·
            {{ attentionEvents.length ? "建议查看" : "暂时正常" }}
          </div>
        </article>
      </section>

      <section class="renew-toolbar">
        <a-input
          v-model:value="search"
          allow-clear
          placeholder="搜索 Agent、动作、文件或网络目标"
          class="renew-search"
        >
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <label class="renew-filter">
          <span>只看 Agent</span>
          <a-switch v-model:checked="onlyAgents" size="small" />
        </label>
        <div class="renew-throughput">
          ↓ {{ formatRate(systemStats.totalNetRecv) }}
          <span>↑ {{ formatRate(systemStats.totalNetSent) }}</span>
        </div>
      </section>

      <div class="renew-grid">
        <section class="renew-panel renew-panel--activity">
          <div class="renew-panel__header">
            <div>
              <h2>最近活动</h2>
              <p>默认隐藏底层字段，只保留“谁做了什么”。</p>
            </div>
            <button class="renew-link" @click="go('Dashboard')">
              完整事件流 <ArrowRightOutlined />
            </button>
          </div>

          <div v-if="recentEvents.length" class="renew-activity-list">
            <button
              v-for="event in recentEvents"
              :key="event.key"
              class="renew-activity"
              @click="go('Dashboard')"
            >
              <span class="renew-activity__status" :class="`is-${eventTone(event)}`" />
              <span class="renew-activity__body">
                <span class="renew-activity__top">
                  <strong>{{ event.tag && event.tag !== "Unknown" ? event.tag : event.comm }}</strong>
                  <span>{{ eventTime(event) }}</span>
                </span>
                <span class="renew-activity__text">{{ describeEvent(event) }}</span>
                <span class="renew-activity__meta">
                  PID {{ event.pid }}
                  <template v-if="event.toolName"> · {{ event.toolName }}</template>
                  <template v-if="event.riskScore"> · risk {{ event.riskScore }}</template>
                </span>
              </span>
              <span class="renew-activity__badge" :class="`is-${eventTone(event)}`">
                {{ eventLabel(event) }}
              </span>
            </button>
          </div>
          <div v-else class="renew-empty">
            <CheckCircleFilled />
            <strong>还没有匹配的 Agent 活动</strong>
            <span>启动已接入的 Agent 后，这里会自动出现行为摘要。</span>
          </div>
        </section>

        <section class="renew-panel renew-panel--attention">
          <div class="renew-panel__header">
            <div>
              <h2>需要关注</h2>
              <p>只有真的值得处理时才强调颜色。</p>
            </div>
            <AlertOutlined />
          </div>

          <div v-if="attentionEvents.length" class="renew-attention-list">
            <button
              v-for="event in attentionEvents"
              :key="event.key"
              class="renew-attention-item"
              @click="go('Dashboard')"
            >
              <div>
                <strong>{{ eventLabel(event) }}</strong>
                <span>{{ eventTime(event) }}</span>
              </div>
              <p>{{ describeEvent(event) }}</p>
            </button>
          </div>
          <div v-else class="renew-empty renew-empty--compact">
            <CheckCircleFilled />
            <strong>没有待处理异常</strong>
            <span>阻断、高风险和语义告警会集中显示在这里。</span>
          </div>
        </section>

        <section class="renew-panel">
          <div class="renew-panel__header">
            <div>
              <h2>Agent 会话</h2>
              <p>按 run / conversation / 根进程归并。</p>
            </div>
          </div>
          <div v-if="activeAgents.length" class="renew-session-list">
            <div v-for="agent in activeAgents" :key="agent.key" class="renew-session">
              <div class="renew-session__icon">{{ agent.label.slice(0, 1).toUpperCase() }}</div>
              <div class="renew-session__body">
                <div>
                  <strong>{{ agent.label }}</strong>
                  <span>{{ agent.events }} 个动作</span>
                </div>
                <p>{{ agent.lastAction }}</p>
              </div>
              <span v-if="agent.alerts" class="renew-session__alert">
                {{ agent.alerts }}
              </span>
            </div>
          </div>
          <div v-else class="renew-empty renew-empty--compact">
            <span>暂无可归并的 Agent 会话</span>
          </div>
        </section>

        <section class="renew-panel">
          <div class="renew-panel__header">
            <div>
              <h2>系统速览</h2>
              <p>日常只保留最常看的进程与网络目标。</p>
            </div>
          </div>
          <div class="renew-snapshot">
            <div>
              <h3>高 CPU 进程</h3>
              <button
                v-for="process in topProcesses"
                :key="process.pid"
                class="renew-row"
                @click="go('Monitor', { tab: 'processes' })"
              >
                <span>{{ process.name }}</span>
                <strong>{{ process.cpu.toFixed(1) }}%</strong>
              </button>
              <span v-if="!topProcesses.length" class="renew-muted">等待系统指标…</span>
            </div>
            <div>
              <h3>常见网络目标</h3>
              <button
                v-for="item in topDestinations"
                :key="item.endpoint"
                class="renew-row"
                @click="go('NetworkFlow', { tab: 'overview' })"
              >
                <span>{{ item.endpoint }}</span>
                <strong>{{ item.count }}</strong>
              </button>
              <span v-if="!topDestinations.length" class="renew-muted">暂无网络事件</span>
            </div>
          </div>
          <div v-if="trackedProcesses.length" class="renew-tracked">
            当前跟踪 {{ trackedProcesses.length }} 个已配置进程
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<style scoped>
.renew-shell {
  --renew-bg: #f6f7f9;
  --renew-panel: #ffffff;
  --renew-border: #e7e9ee;
  --renew-text: #17191f;
  --renew-muted: #737986;
  --renew-accent: #e97932;
  --renew-accent-soft: #fff3ea;
  --renew-warning: #c98a1b;
  --renew-danger: #d84a4a;
  min-height: 100vh;
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  background: var(--renew-bg);
  color: var(--renew-text);
}

.renew-sidebar {
  position: sticky;
  top: 0;
  height: 100vh;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  padding: 18px 14px;
  border-right: 1px solid var(--renew-border);
  background: rgba(255, 255, 255, 0.96);
}

.renew-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 8px 18px;
}

.renew-brand__mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  background: var(--renew-accent);
  color: #fff;
  font-weight: 800;
}

.renew-brand strong,
.renew-brand span {
  display: block;
}

.renew-brand strong {
  font-size: 16px;
}

.renew-brand span {
  margin-top: 1px;
  color: var(--renew-muted);
  font-size: 11px;
}

.renew-nav {
  display: grid;
  gap: 4px;
}

.renew-nav__item {
  width: 100%;
  border: 0;
  border-radius: 10px;
  padding: 10px 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  background: transparent;
  color: #555c68;
  cursor: pointer;
  text-align: left;
  transition: 0.18s ease;
}

.renew-nav__item:hover {
  background: #f5f5f6;
  color: var(--renew-text);
}

.renew-nav__item--active {
  background: var(--renew-accent-soft);
  color: #b84f13;
  font-weight: 650;
}

.renew-sidebar__footer {
  margin-top: auto;
  display: grid;
  gap: 8px;
}

.renew-professional {
  border: 1px solid var(--renew-border);
  border-radius: 10px;
  padding: 10px 11px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  color: #4c515b;
  cursor: pointer;
}

.renew-main {
  min-width: 0;
  padding: 30px clamp(18px, 3vw, 44px) 48px;
}

.renew-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.renew-eyebrow {
  margin-bottom: 5px;
  color: var(--renew-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
}

.renew-header h1,
.renew-panel h2,
.renew-snapshot h3 {
  margin: 0;
}

.renew-header h1 {
  font-size: clamp(26px, 3vw, 34px);
  line-height: 1.15;
  letter-spacing: -0.03em;
}

.renew-header p,
.renew-panel__header p {
  margin: 6px 0 0;
  color: var(--renew-muted);
  font-size: 13px;
}

.renew-header__actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.renew-live,
.renew-icon-button {
  height: 34px;
  border: 1px solid var(--renew-border);
  border-radius: 999px;
  background: #fff;
}

.renew-live {
  padding: 0 12px;
  display: flex;
  align-items: center;
  gap: 7px;
  color: #4c515b;
  font-size: 12px;
  font-weight: 600;
}

.renew-live__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #46a86d;
  box-shadow: 0 0 0 3px rgba(70, 168, 109, 0.12);
}

.renew-live--offline .renew-live__dot {
  background: #a4a7ae;
  box-shadow: none;
}

.renew-icon-button {
  width: 34px;
  display: grid;
  place-items: center;
  color: #5d626d;
  cursor: pointer;
}

.renew-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.renew-metric,
.renew-panel {
  border: 1px solid var(--renew-border);
  background: var(--renew-panel);
  box-shadow: 0 1px 2px rgba(19, 25, 38, 0.02);
}

.renew-metric {
  min-height: 122px;
  border-radius: 14px;
  padding: 17px;
}

.renew-metric--danger {
  border-color: rgba(216, 74, 74, 0.34);
}

.renew-metric--warning {
  border-color: rgba(201, 138, 27, 0.34);
}

.renew-metric__label {
  color: var(--renew-muted);
  font-size: 12px;
}

.renew-metric__value {
  margin-top: 11px;
  font-size: 27px;
  line-height: 1;
  font-weight: 750;
  letter-spacing: -0.035em;
}

.renew-metric__meta {
  margin-top: 11px;
  color: var(--renew-muted);
  font-size: 11px;
}

.renew-meter {
  height: 4px;
  margin-top: 12px;
  overflow: hidden;
  border-radius: 999px;
  background: #f0f1f3;
}

.renew-meter span {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: #8d929b;
  transition: width 0.3s ease;
}

.renew-toolbar {
  margin: 18px 0 12px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.renew-search {
  max-width: 420px;
}

.renew-filter,
.renew-throughput {
  height: 32px;
  display: inline-flex;
  align-items: center;
  gap: 9px;
  color: var(--renew-muted);
  font-size: 12px;
}

.renew-filter {
  padding: 0 10px;
  border: 1px solid var(--renew-border);
  border-radius: 8px;
  background: #fff;
}

.renew-throughput {
  margin-left: auto;
}

.renew-throughput span {
  margin-left: 5px;
}

.renew-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.55fr) minmax(300px, 0.85fr);
  gap: 12px;
  align-items: start;
}

.renew-panel {
  border-radius: 14px;
  padding: 18px;
  min-width: 0;
}

.renew-panel__header {
  min-height: 42px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 13px;
}

.renew-panel h2 {
  font-size: 15px;
}

.renew-link {
  border: 0;
  background: transparent;
  color: #6b7079;
  font-size: 12px;
  cursor: pointer;
}

.renew-activity-list,
.renew-attention-list,
.renew-session-list {
  display: grid;
}

.renew-activity {
  width: 100%;
  min-width: 0;
  display: grid;
  grid-template-columns: 8px minmax(0, 1fr) auto;
  gap: 11px;
  align-items: center;
  padding: 11px 0;
  border: 0;
  border-top: 1px solid #f0f1f3;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.renew-activity:hover {
  background: #fafafa;
}

.renew-activity__status {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #b4b7bd;
}

.renew-activity__status.is-warning {
  background: var(--renew-warning);
}

.renew-activity__status.is-danger {
  background: var(--renew-danger);
}

.renew-activity__body {
  min-width: 0;
  display: grid;
  gap: 3px;
}

.renew-activity__top {
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.renew-activity__top strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.renew-activity__top span,
.renew-activity__meta {
  color: #90949c;
  font-size: 10px;
}

.renew-activity__text {
  overflow: hidden;
  color: #40454e;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.renew-activity__badge {
  max-width: 110px;
  overflow: hidden;
  border-radius: 999px;
  padding: 3px 7px;
  background: #f3f4f5;
  color: #727781;
  font-size: 9px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.renew-activity__badge.is-warning {
  background: #fff7e8;
  color: #a56d0d;
}

.renew-activity__badge.is-danger {
  background: #fff0f0;
  color: #bd3737;
}

.renew-attention-item {
  padding: 11px 0;
  border: 0;
  border-top: 1px solid #f0f1f3;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.renew-attention-item div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.renew-attention-item strong {
  color: #ba3a3a;
  font-size: 11px;
}

.renew-attention-item span,
.renew-attention-item p {
  color: var(--renew-muted);
  font-size: 10px;
}

.renew-attention-item p {
  margin: 5px 0 0;
  color: #4b5059;
  font-size: 11px;
}

.renew-session {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
  padding: 9px 0;
  border-top: 1px solid #f0f1f3;
}

.renew-session__icon {
  width: 32px;
  height: 32px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  background: #f1f2f4;
  color: #5e636c;
  font-size: 11px;
  font-weight: 800;
}

.renew-session__body {
  min-width: 0;
}

.renew-session__body > div {
  display: flex;
  align-items: center;
  gap: 8px;
}

.renew-session__body strong {
  font-size: 11px;
}

.renew-session__body span,
.renew-session__body p {
  color: var(--renew-muted);
  font-size: 10px;
}

.renew-session__body p {
  margin: 2px 0 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.renew-session__alert {
  min-width: 20px;
  height: 20px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: #fff0f0;
  color: #bd3737 !important;
  font-weight: 700;
}

.renew-snapshot {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18px;
}

.renew-snapshot h3 {
  margin-bottom: 8px;
  color: #666c76;
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.renew-row {
  width: 100%;
  min-width: 0;
  padding: 6px 0;
  border: 0;
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  color: #555b65;
  cursor: pointer;
  text-align: left;
}

.renew-row span {
  overflow: hidden;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.renew-row strong {
  color: #2f333a;
  font-size: 10px;
}

.renew-muted,
.renew-tracked {
  color: var(--renew-muted);
  font-size: 10px;
}

.renew-tracked {
  margin-top: 11px;
  padding-top: 10px;
  border-top: 1px solid #f0f1f3;
}

.renew-empty {
  min-height: 210px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 7px;
  color: #9ca0a7;
  text-align: center;
}

.renew-empty :deep(.anticon) {
  color: #68a97e;
  font-size: 24px;
}

.renew-empty strong {
  color: #585d66;
  font-size: 12px;
}

.renew-empty span {
  max-width: 360px;
  font-size: 10px;
}

.renew-empty--compact {
  min-height: 150px;
}

@media (max-width: 1080px) {
  .renew-shell {
    grid-template-columns: 74px minmax(0, 1fr);
  }

  .renew-brand > div:last-child,
  .renew-nav__item span,
  .renew-professional {
    display: none;
  }

  .renew-brand {
    justify-content: center;
  }

  .renew-nav__item {
    justify-content: center;
  }

  .renew-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .renew-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 700px) {
  .renew-shell {
    display: block;
  }

  .renew-sidebar {
    position: static;
    width: 100%;
    height: auto;
    padding: 10px 14px;
    flex-direction: row;
    align-items: center;
    border-right: 0;
    border-bottom: 1px solid var(--renew-border);
  }

  .renew-brand {
    padding: 0;
  }

  .renew-nav {
    margin-left: auto;
    display: flex;
  }

  .renew-nav__item {
    width: 36px;
    padding: 9px;
  }

  .renew-sidebar__footer {
    display: none;
  }

  .renew-main {
    padding: 20px 14px 36px;
  }

  .renew-header {
    display: grid;
  }

  .renew-header__actions {
    justify-content: space-between;
  }

  .renew-metrics {
    grid-template-columns: 1fr 1fr;
  }

  .renew-toolbar {
    align-items: stretch;
    flex-wrap: wrap;
  }

  .renew-search {
    max-width: none;
    flex: 1 0 100%;
  }

  .renew-throughput {
    margin-left: 0;
  }

  .renew-snapshot {
    grid-template-columns: 1fr;
  }
}
</style>
