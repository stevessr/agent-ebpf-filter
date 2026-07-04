<script setup lang="ts">
import { computed, onMounted } from "vue";
import {
  BarChartOutlined,
  CloudDownloadOutlined,
  DeleteOutlined,
  ExperimentOutlined,
  ExportOutlined,
  FileSearchOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RetweetOutlined,
  StopOutlined,
} from "@ant-design/icons-vue";
import { useResearchWorkbench } from "../../composables/research/useResearchWorkbench";
import type { ResearchCount, ResearchEvent } from "../../types/config";

const research = useResearchWorkbench();

const {
  sessions,
  selectedSessionId,
  selectedSession,
  events,
  results,
  activeTask,
  artifactRefs,
  loadingSessions,
  creatingSession,
  deletingSession,
  loadingEvents,
  loadingResults,
  submittingTask,
  exportingArtifact,
  eventSearch,
  eventLimit,
  eventOffset,
  eventsTotal,
  compareWindowHours,
  createForm,
  canPageBack,
  canPageForward,
  refreshSessions,
  createSession,
  selectSession,
  deleteSelectedSession,
  buildSession,
  scanRecent,
  exportBundle,
  resetSession,
  compareRecentWindows,
  cancelActiveTask,
  fetchEvents,
  fetchResults,
  pageEvents,
  downloadArtifact,
  formatBytes,
  statusColor,
  riskColor,
  formatTime,
} = research;

const taskInFlight = computed(() =>
  ["queued", "running"].includes(activeTask.value?.status || ""),
);

const taskProgressPercent = computed(() =>
  Math.round((activeTask.value?.progress || 0) * 100),
);

const selectedSummary = computed(() => selectedSession.value?.summary);

const summaryCards = computed(() => [
  {
    title: "Events",
    value: selectedSummary.value?.eventCount || 0,
    suffix: "normalized",
  },
  {
    title: "Risk Alerts",
    value: selectedSummary.value?.riskAlerts || 0,
    suffix: "findings",
  },
  {
    title: "Loop Findings",
    value: selectedSummary.value?.loopFindings || 0,
    suffix: "loops",
  },
  {
    title: "Max Risk",
    value: Number(selectedSummary.value?.maxRiskScore || 0).toFixed(1),
    suffix: "score",
  },
]);

const topCounts = (items?: ResearchCount[]) => (items || []).slice(0, 8);

const eventFeaturePreview = (event: ResearchEvent) => {
  const features = event.features || {};
  const keys = Object.keys(features).slice(0, 5);
  if (!keys.length) return "—";
  return keys
    .map((key) => `${key}=${String(features[key]).slice(0, 40)}`)
    .join(", ");
};

const eventRowKey = (event: ResearchEvent) =>
  event.id || `${event.timestamp}:${event.source}:${event.eventType}`;

onMounted(async () => {
  await refreshSessions(true);
  if (selectedSessionId.value) {
    await Promise.all([fetchEvents(true), fetchResults(true)]);
  }
});
</script>

<template>
  <div class="research-page">
    <div class="research-page__header">
      <div>
        <a-typography-title :level="3" class="research-page__title">
          <ExperimentOutlined />
          Research Workbench
        </a-typography-title>
        <a-typography-paragraph class="research-page__description">
          将实时事件、AgentSight 兼容事件、TLS/网络/进程上下文统一沉淀为可查询、可复现、可导出的研究会话。
        </a-typography-paragraph>
      </div>
      <a-space wrap>
        <a-button @click="refreshSessions()" :loading="loadingSessions">
          <ReloadOutlined /> 刷新
        </a-button>
        <a-button
          type="primary"
          @click="buildSession()"
          :loading="submittingTask"
          :disabled="!selectedSessionId"
        >
          <PlayCircleOutlined /> Build Session
        </a-button>
      </a-space>
    </div>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="7">
        <a-card size="small" title="Create / Filter Session">
          <div class="research-form">
            <a-input v-model:value="createForm.name" placeholder="Session name" />
            <a-textarea
              v-model:value="createForm.description"
              placeholder="Description"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
            <a-input
              v-model:value="createForm.tags"
              placeholder="tags, comma separated"
            />
            <a-input
              v-model:value="createForm.query"
              placeholder="query target / comm / event text"
              allow-clear
            />
            <a-input
              v-model:value="createForm.sources"
              placeholder="sources: ebpf,agentsight,tls..."
              allow-clear
            />
            <a-input
              v-model:value="createForm.eventTypes"
              placeholder="event types: openat,wrapper_intercept..."
              allow-clear
            />
            <a-input
              v-model:value="createForm.comms"
              placeholder="commands: bash,curl,python"
              allow-clear
            />
            <div class="research-form__row">
              <div class="research-form__field">
                <span>Limit</span>
                <a-input-number
                  v-model:value="createForm.limit"
                  :min="1"
                  :max="50000"
                  style="width: 100%"
                />
              </div>
              <div class="research-form__field">
                <span>Since minutes</span>
                <a-input-number
                  v-model:value="createForm.sinceMinutes"
                  :min="0"
                  :max="10080"
                  style="width: 100%"
                />
              </div>
            </div>
            <a-space wrap>
              <a-button
                type="primary"
                @click="createSession()"
                :loading="creatingSession"
              >
                <FileSearchOutlined /> 创建会话
              </a-button>
              <a-button
                @click="scanRecent()"
                :loading="submittingTask"
                :disabled="!selectedSessionId"
              >
                <ReloadOutlined /> Scan Recent
              </a-button>
            </a-space>
          </div>
        </a-card>

        <a-card size="small" class="research-card" title="Sessions">
          <a-list
            :data-source="sessions"
            :loading="loadingSessions"
            size="small"
            item-layout="vertical"
          >
            <template #renderItem="{ item }">
              <a-list-item
                class="research-session-item"
                :class="{
                  'research-session-item--active': item.id === selectedSessionId,
                }"
                @click="selectSession(item.id)"
              >
                <div class="research-session-item__title">
                  <span>{{ item.name }}</span>
                  <a-tag :color="statusColor(item.status)">{{
                    item.status
                  }}</a-tag>
                </div>
                <div class="research-session-item__meta">
                  {{ item.summary?.eventCount || 0 }} events ·
                  {{ formatTime(item.updatedAt) }}
                </div>
                <a-space wrap class="research-session-item__tags">
                  <a-tag
                    v-for="tag in item.tags || []"
                    :key="`${item.id}:${tag}`"
                    size="small"
                  >
                    {{ tag }}
                  </a-tag>
                </a-space>
              </a-list-item>
            </template>
            <template #empty>
              <a-empty description="暂无研究会话" />
            </template>
          </a-list>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="17">
        <a-card size="small">
          <template #title>
            <span>
              <BarChartOutlined /> Session Overview
              <a-tag
                v-if="selectedSession"
                :color="statusColor(selectedSession.status)"
                style="margin-left: 8px"
              >
                {{ selectedSession.status }}
              </a-tag>
            </span>
          </template>
          <template #extra>
            <a-space wrap>
              <a-button
                size="small"
                @click="fetchResults()"
                :loading="loadingResults"
                :disabled="!selectedSessionId"
              >
                <ReloadOutlined /> Results
              </a-button>
              <a-button
                size="small"
                @click="exportBundle()"
                :loading="submittingTask"
                :disabled="!selectedSessionId"
              >
                <ExportOutlined /> 生成导出包
              </a-button>
              <a-popconfirm
                title="确定重置该研究会话的事件和结果吗？"
                @confirm="resetSession()"
              >
                <a-button
                  size="small"
                  :disabled="!selectedSessionId"
                  :loading="submittingTask"
                >
                  <RetweetOutlined /> Reset
                </a-button>
              </a-popconfirm>
              <a-popconfirm
                title="确定删除该研究会话及 artifacts 吗？"
                @confirm="deleteSelectedSession()"
              >
                <a-button
                  size="small"
                  danger
                  :disabled="!selectedSessionId"
                  :loading="deletingSession"
                >
                  <DeleteOutlined /> Delete
                </a-button>
              </a-popconfirm>
            </a-space>
          </template>

          <a-empty v-if="!selectedSession" description="请选择或创建研究会话" />
          <template v-else>
            <a-alert
              v-if="selectedSession.lastError"
              type="error"
              show-icon
              style="margin-bottom: 12px"
              :message="selectedSession.lastError"
            />
            <a-row :gutter="[12, 12]" class="research-stats">
              <a-col
                v-for="card in summaryCards"
                :key="card.title"
                :xs="12"
                :md="6"
              >
                <a-card size="small">
                  <a-statistic
                    :title="card.title"
                    :value="card.value"
                    :suffix="card.suffix"
                  />
                </a-card>
              </a-col>
            </a-row>

            <a-card v-if="activeTask" size="small" class="research-card">
              <div class="research-task">
                <div class="research-task__head">
                  <a-space wrap>
                    <strong>{{ activeTask.action }}</strong>
                    <a-tag :color="statusColor(activeTask.status)">
                      {{ activeTask.status }}
                    </a-tag>
                    <span>{{ activeTask.records || 0 }} records</span>
                    <span v-if="activeTask.error" class="research-task__error">
                      {{ activeTask.error }}
                    </span>
                  </a-space>
                  <a-button
                    v-if="taskInFlight"
                    size="small"
                    danger
                    @click="cancelActiveTask()"
                  >
                    <StopOutlined /> Cancel
                  </a-button>
                </div>
                <a-progress
                  :percent="taskProgressPercent"
                  size="small"
                  :status="activeTask.status === 'failed' ? 'exception' : undefined"
                />
              </div>
            </a-card>

            <a-tabs class="research-tabs">
              <a-tab-pane key="events" tab="Events">
                <div class="research-toolbar">
                  <a-space wrap>
                    <a-input
                      v-model:value="eventSearch"
                      placeholder="filter events"
                      size="small"
                      allow-clear
                      style="width: 220px"
                      @press-enter="fetchEvents()"
                    />
                    <a-input-number
                      v-model:value="eventLimit"
                      :min="10"
                      :max="5000"
                      size="small"
                      style="width: 96px"
                    />
                    <a-button
                      size="small"
                      @click="fetchEvents()"
                      :loading="loadingEvents"
                    >
                      <ReloadOutlined /> Load
                    </a-button>
                    <a-button
                      size="small"
                      @click="pageEvents('prev')"
                      :disabled="!canPageBack"
                    >
                      Prev
                    </a-button>
                    <a-button
                      size="small"
                      @click="pageEvents('next')"
                      :disabled="!canPageForward"
                    >
                      Next
                    </a-button>
                    <a-tag color="blue">
                      {{ eventOffset }} -
                      {{ Math.min(eventOffset + events.length, eventsTotal) }} /
                      {{ eventsTotal }}
                    </a-tag>
                  </a-space>
                </div>
                <a-table
                  :dataSource="events"
                  :loading="loadingEvents"
                  :pagination="{ pageSize: 12, showSizeChanger: true }"
                  :scroll="{ x: 1250 }"
                  :rowKey="eventRowKey"
                  size="small"
                >
                  <a-table-column title="Time" dataIndex="time" :width="180">
                    <template #default="{ record }">
                      <span class="research-muted">{{
                        formatTime(record.time)
                      }}</span>
                    </template>
                  </a-table-column>
                  <a-table-column title="Source" dataIndex="source" :width="110">
                    <template #default="{ record }">
                      <a-tag color="geekblue">{{ record.source }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column
                    title="Type"
                    dataIndex="eventType"
                    :width="160"
                  />
                  <a-table-column title="PID" dataIndex="pid" :width="80" />
                  <a-table-column title="Comm" dataIndex="comm" :width="120">
                    <template #default="{ record }">
                      <strong>{{ record.comm || "—" }}</strong>
                    </template>
                  </a-table-column>
                  <a-table-column title="Risk" dataIndex="riskScore" :width="90">
                    <template #default="{ record }">
                      <a-tag :color="riskColor(record.riskScore)">
                        {{ (record.riskScore || 0).toFixed(1) }}
                      </a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column
                    title="Decision"
                    dataIndex="decision"
                    :width="110"
                  />
                  <a-table-column
                    title="Target"
                    dataIndex="target"
                    :width="260"
                    ellipsis
                  >
                    <template #default="{ record }">
                      <code>{{ record.target || "—" }}</code>
                    </template>
                  </a-table-column>
                  <a-table-column title="Features" :width="280" ellipsis>
                    <template #default="{ record }">
                      <span class="research-muted">{{
                        eventFeaturePreview(record)
                      }}</span>
                    </template>
                  </a-table-column>
                </a-table>
              </a-tab-pane>

              <a-tab-pane key="results" tab="Aggregates">
                <a-row :gutter="[16, 16]">
                  <a-col :xs="24" :lg="8">
                    <a-card size="small" title="By Source">
                      <a-list
                        size="small"
                        :data-source="topCounts(results?.summary.bySource)"
                      >
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <span>{{ item.key }}</span>
                            <a-tag color="blue">{{ item.count }}</a-tag>
                          </a-list-item>
                        </template>
                      </a-list>
                    </a-card>
                  </a-col>
                  <a-col :xs="24" :lg="8">
                    <a-card size="small" title="Top Commands">
                      <a-list
                        size="small"
                        :data-source="topCounts(results?.summary.byComm)"
                      >
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <span>{{ item.key }}</span>
                            <a-tag color="purple">{{ item.count }}</a-tag>
                          </a-list-item>
                        </template>
                      </a-list>
                    </a-card>
                  </a-col>
                  <a-col :xs="24" :lg="8">
                    <a-card size="small" title="Top Targets">
                      <a-list
                        size="small"
                        :data-source="topCounts(results?.topTargets)"
                      >
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <span class="research-ellipsis">{{ item.key }}</span>
                            <a-tag color="cyan">{{ item.count }}</a-tag>
                          </a-list-item>
                        </template>
                      </a-list>
                    </a-card>
                  </a-col>
                  <a-col :xs="24">
                    <a-card size="small" title="Risk Alerts">
                      <a-table
                        :dataSource="results?.riskAlerts || []"
                        :pagination="{ pageSize: 6 }"
                        :scroll="{ x: 900 }"
                        rowKey="eventId"
                        size="small"
                      >
                        <a-table-column title="Time" dataIndex="time" :width="170">
                          <template #default="{ record }">
                            {{ formatTime(record.time) }}
                          </template>
                        </a-table-column>
                        <a-table-column title="Comm" dataIndex="comm" :width="120" />
                        <a-table-column
                          title="Type"
                          dataIndex="eventType"
                          :width="150"
                        />
                        <a-table-column title="Risk" dataIndex="riskScore" :width="90">
                          <template #default="{ record }">
                            <a-tag :color="riskColor(record.riskScore)">
                              {{ (record.riskScore || 0).toFixed(1) }}
                            </a-tag>
                          </template>
                        </a-table-column>
                        <a-table-column
                          title="Target"
                          dataIndex="target"
                          :width="320"
                          ellipsis
                        />
                      </a-table>
                    </a-card>
                  </a-col>
                </a-row>
              </a-tab-pane>

              <a-tab-pane key="export" tab="Exports">
                <div class="research-toolbar">
                  <a-space wrap>
                    <a-button
                      @click="exportBundle()"
                      :loading="submittingTask"
                      :disabled="!selectedSessionId"
                    >
                      <ExportOutlined /> 生成 JSONL/CSV/Bundle
                    </a-button>
                    <a-button
                      @click="downloadArtifact('jsonl')"
                      :loading="exportingArtifact"
                      :disabled="!selectedSessionId"
                    >
                      <CloudDownloadOutlined /> JSONL
                    </a-button>
                    <a-button
                      @click="downloadArtifact('csv')"
                      :loading="exportingArtifact"
                      :disabled="!selectedSessionId"
                    >
                      <CloudDownloadOutlined /> CSV
                    </a-button>
                    <a-button
                      type="primary"
                      @click="downloadArtifact('bundle')"
                      :loading="exportingArtifact"
                      :disabled="!selectedSessionId"
                    >
                      <CloudDownloadOutlined /> Bundle ZIP
                    </a-button>
                    <span class="research-muted">
                      Artifact 下载前需要先“生成导出包”。
                    </span>
                  </a-space>
                </div>
                <a-table
                  :dataSource="artifactRefs"
                  :pagination="false"
                  rowKey="format"
                  size="small"
                >
                  <a-table-column title="Format" dataIndex="format" :width="110">
                    <template #default="{ record }">
                      <a-tag color="blue">{{ record.format }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="Name" dataIndex="name" />
                  <a-table-column title="Bytes" dataIndex="bytes" :width="120">
                    <template #default="{ record }">
                      {{ formatBytes(record.bytes) }}
                    </template>
                  </a-table-column>
                  <a-table-column title="SHA256" dataIndex="sha256" :width="260">
                    <template #default="{ record }">
                      <code>{{ record.sha256?.slice(0, 24) }}…</code>
                    </template>
                  </a-table-column>
                  <a-table-column title="Created" dataIndex="createdAt" :width="180">
                    <template #default="{ record }">
                      {{ formatTime(record.createdAt) }}
                    </template>
                  </a-table-column>
                </a-table>
              </a-tab-pane>

              <a-tab-pane key="compare" tab="Compare">
                <a-alert
                  type="info"
                  show-icon
                  style="margin-bottom: 12px"
                  message="Compare Windows 使用当前 session events，在后端生成左右窗口聚合差异并写回 results.compareWindows。"
                />
                <a-space wrap>
                  <span>Window hours</span>
                  <a-input-number
                    v-model:value="compareWindowHours"
                    :min="1"
                    :max="168"
                    style="width: 120px"
                  />
                  <a-button
                    type="primary"
                    @click="compareRecentWindows()"
                    :loading="submittingTask"
                    :disabled="!selectedSessionId"
                  >
                    <RetweetOutlined /> 比较最近两个窗口
                  </a-button>
                </a-space>
                <pre class="research-json">{{
                  JSON.stringify(results?.compareWindows || {}, null, 2)
                }}</pre>
              </a-tab-pane>
            </a-tabs>
          </template>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.research-page {
  min-height: 100%;
}

.research-page__header {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 12px;
}

.research-page__title {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 4px;
}

.research-page__description {
  max-width: 920px;
  margin-bottom: 0;
  color: #667085;
}

.research-card {
  margin-top: 12px;
}

.research-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.research-form__row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 10px;
}

.research-form__field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #667085;
  font-size: 12px;
}

.research-session-item {
  padding: 10px !important;
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: 8px;
}

.research-session-item:hover,
.research-session-item--active {
  background: #f5f8ff;
  border-color: #91caff;
}

.research-session-item__title {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  font-weight: 600;
}

.research-session-item__meta {
  margin-top: 4px;
  color: #667085;
  font-size: 12px;
}

.research-session-item__tags {
  margin-top: 6px;
}

.research-stats {
  margin-bottom: 12px;
}

.research-task {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.research-task__head {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
}

.research-task__error {
  color: #cf1322;
}

.research-tabs {
  margin-top: 4px;
}

.research-toolbar {
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
}

.research-muted {
  color: #667085;
  font-size: 12px;
}

.research-ellipsis {
  display: inline-block;
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.research-json {
  max-height: 420px;
  padding: 12px;
  margin-top: 12px;
  overflow: auto;
  font-size: 12px;
  background: #0f172a;
  border-radius: 8px;
  color: #e2e8f0;
}
</style>
