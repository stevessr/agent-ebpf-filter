<script setup lang="ts">
import { computed } from "vue";

import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { buildDashboardInsights } from "../../composables/dashboard/dashboardInsights";

const props = defineProps<{
  events: AgentEvent[];
}>();

const insights = computed(() => buildDashboardInsights(props.events));

const formatCoverage = (coverage: number | null) =>
  coverage === null ? "—" : `${coverage.toFixed(1)}%`;

const formatRunLabel = (label: string) =>
  label.length > 34 ? `${label.slice(0, 18)}…${label.slice(-12)}` : label;

const formatLastSeen = (timestamp: number) => {
  if (!timestamp) return "—";
  return new Date(timestamp).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};
</script>

<template>
  <section class="activity-insights" aria-label="Agent activity insights">
    <div class="insights-header">
      <div>
        <div class="insights-title">Activity Insights</div>
        <div class="insights-subtitle">
          会话、工具、操作与双层观测关联概览
        </div>
      </div>
      <a-tag color="blue">current buffer · {{ insights.totalEvents }} events</a-tag>
    </div>

    <div class="metric-grid">
      <div class="metric-card">
        <span class="metric-label">Agent Runs</span>
        <strong>{{ insights.agentRuns }}</strong>
        <small>{{ insights.observedPids }} observed PIDs</small>
      </div>
      <div class="metric-card">
        <span class="metric-label">Tool Calls</span>
        <strong>{{ insights.toolCalls }}</strong>
        <small>{{ insights.mcpCalls }} MCP calls</small>
      </div>
      <div class="metric-card">
        <span class="metric-label">Alerts / Blocks</span>
        <strong>{{ insights.alerts }} / {{ insights.blocked }}</strong>
        <small>{{ insights.highRisk }} high-risk events</small>
      </div>
      <div class="metric-card">
        <span class="metric-label">Sensitive Files</span>
        <strong>{{ insights.sensitiveFileTouches }}</strong>
        <small>credential/config touches</small>
      </div>
      <div class="metric-card">
        <span class="metric-label">AI Trajectory</span>
        <strong>{{ insights.networkDestinations }}</strong>
        <small>unique network targets</small>
      </div>
      <div
        class="metric-card"
        :class="{
          'metric-card--danger': insights.semanticGaps > 0,
          'metric-card--good':
            insights.semanticGaps === 0 && insights.dualLayerCoverage !== null,
        }"
      >
        <span class="metric-label">Semantic Gap</span>
        <strong>{{ insights.semanticGaps }}</strong>
        <small>
          paired {{ insights.pairedKernelExecs }}/{{ insights.correlatedKernelExecs }} ·
          {{ formatCoverage(insights.dualLayerCoverage) }}
        </small>
      </div>
    </div>

    <div class="breakdown-grid">
      <div class="breakdown-card">
        <div class="breakdown-title">文件操作</div>
        <div class="tag-list">
          <a-tag v-for="item in insights.fileOperations" :key="item.key">
            {{ item.label }} {{ item.count }}
          </a-tag>
        </div>
      </div>

      <div class="breakdown-card">
        <div class="breakdown-title">软件安装</div>
        <div class="tag-list">
          <a-tag
            v-for="item in insights.installOperations"
            :key="item.key"
            :color="item.count > 0 ? 'orange' : 'default'"
          >
            {{ item.label }} {{ item.count }}
          </a-tag>
        </div>
      </div>

      <div class="breakdown-card">
        <div class="breakdown-title">Git / GitHub 操作</div>
        <div class="tag-list">
          <a-tag
            v-for="item in insights.gitOperations"
            :key="item.key"
            :color="item.count > 0 ? 'geekblue' : 'default'"
          >
            {{ item.label }} {{ item.count }}
          </a-tag>
        </div>
      </div>

      <div class="breakdown-card">
        <div class="breakdown-title">MCP Servers</div>
        <div v-if="insights.mcpServers.length" class="tag-list">
          <a-tag v-for="item in insights.mcpServers" :key="item.key" color="purple">
            {{ item.label }} {{ item.count }}
          </a-tag>
        </div>
        <span v-else class="empty-text">当前缓冲区没有 MCP 调用</span>
      </div>
    </div>

    <div class="trajectory-card">
      <div class="breakdown-title">AI 轨迹 · 网络目的地</div>
      <div v-if="insights.topDestinations.length" class="tag-list">
        <a-tag
          v-for="target in insights.topDestinations"
          :key="target.destination"
          color="cyan"
        >
          {{ target.destination }} ×{{ target.count }}
        </a-tag>
      </div>
      <span v-else class="empty-text">尚未捕获可归一化的网络目的地</span>
    </div>

    <div v-if="insights.recentRuns.length" class="runs-card">
      <div class="breakdown-title">最近 Agent Runs</div>
      <div class="runs-table" role="table" aria-label="Recent agent runs">
        <div class="run-row run-row--header" role="row">
          <span>Run</span>
          <span>Events</span>
          <span>Tools</span>
          <span>Alerts</span>
          <span>Targets</span>
          <span>Last seen</span>
        </div>
        <div
          v-for="run in insights.recentRuns"
          :key="run.id"
          class="run-row"
          role="row"
        >
          <span class="run-id" :title="run.label">{{ formatRunLabel(run.label) }}</span>
          <span>{{ run.events }}</span>
          <span>{{ run.toolCalls }}</span>
          <span :class="{ danger: run.alerts > 0 }">
            {{ run.alerts }}<template v-if="run.blocked"> / {{ run.blocked }} blocked</template>
          </span>
          <span>{{ run.destinations }}</span>
          <span>{{ formatLastSeen(run.lastSeenMs) }}</span>
        </div>
      </div>
    </div>

    <div class="insights-note">
      统计只覆盖 Dashboard 当前已加载事件；Semantic Gap 只检查带
      toolCallId/spanId/traceId 的 execve/process_exec，并与 wrapper/native hook
      事件交叉关联，因此未带语义关联字段的普通内核事件不会被误报为绕过。
    </div>
  </section>
</template>

<style scoped>
.activity-insights {
  margin: 4px 0 12px;
  padding: 14px;
  border: 1px solid #d9e4d1;
  border-radius: 8px;
  background: linear-gradient(180deg, #ffffff 0%, #f7fbf4 100%);
}

.insights-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.insights-title {
  color: #243b25;
  font-size: 15px;
  font-weight: 700;
}

.insights-subtitle,
.insights-note,
.empty-text {
  color: #6b7280;
  font-size: 12px;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 8px;
}

.metric-card,
.breakdown-card,
.trajectory-card,
.runs-card {
  border: 1px solid #e1e9dc;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.92);
}

.metric-card {
  min-width: 0;
  padding: 10px 11px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.metric-card strong {
  color: #1f3a1f;
  font-size: 20px;
  line-height: 1.2;
}

.metric-card small {
  color: #7a8578;
  font-size: 11px;
  line-height: 1.35;
}

.metric-card--danger {
  border-color: #ffccc7;
  background: #fff7f6;
}

.metric-card--danger strong {
  color: #cf1322;
}

.metric-card--good {
  border-color: #b7eb8f;
  background: #f6ffed;
}

.metric-card--good strong {
  color: #237804;
}

.metric-label,
.breakdown-title {
  color: #4b5f4a;
  font-size: 12px;
  font-weight: 700;
}

.breakdown-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
}

.breakdown-card,
.trajectory-card,
.runs-card {
  padding: 10px 11px;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 7px;
}

.tag-list :deep(.ant-tag) {
  margin-inline-end: 0;
}

.trajectory-card,
.runs-card {
  margin-top: 8px;
}

.runs-table {
  margin-top: 7px;
  overflow-x: auto;
}

.run-row {
  display: grid;
  grid-template-columns: minmax(180px, 2fr) repeat(5, minmax(72px, 0.7fr));
  gap: 8px;
  min-width: 720px;
  padding: 6px 4px;
  border-top: 1px solid #edf1ea;
  color: #334155;
  font-size: 12px;
  align-items: center;
}

.run-row--header {
  border-top: 0;
  color: #687568;
  font-weight: 700;
}

.run-id {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.danger {
  color: #cf1322;
  font-weight: 700;
}

.insights-note {
  margin-top: 10px;
  line-height: 1.5;
}

@media (max-width: 1280px) {
  .metric-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .breakdown-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .metric-grid,
  .breakdown-grid {
    grid-template-columns: 1fr;
  }

  .insights-header {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
