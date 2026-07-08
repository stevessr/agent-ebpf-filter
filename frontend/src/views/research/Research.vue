<script setup lang="ts">
import { computed, onMounted } from "vue";
import {
  BarChartOutlined,
  CloudDownloadOutlined,
  DeleteOutlined,
  ExperimentOutlined,
  ExportOutlined,
  FileSearchOutlined,
  ImportOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RetweetOutlined,
  SecurityScanOutlined,
  StopOutlined,
} from "@ant-design/icons-vue";
import { useResearchWorkbench } from "../../composables/research/useResearchWorkbench";
import type {
  ResearchCount,
  ResearchEvent,
  ResearchSecurityEvaluationSampleRow,
} from "../../types/config";

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
  exportingSecurityEvaluation,
  loadingResearchTraining,
  importingResearchTraining,
  exportingResearchTraining,
  eventSearch,
  eventLimit,
  eventOffset,
  eventsTotal,
  compareWindowHours,
  researchTrainingLabelPolicy,
  researchTrainingImportLimit,
  researchTrainingDataset,
  researchTrainingImportResult,
  researchTrainingPreviewSamples,
  securityEvaluationMode,
  securityEvaluationLimit,
  securityEvaluationIncludeLLM,
  securityEvaluationLabelPolicy,
  securityEvaluation,
  securityEvaluationPreviewSamples,
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
  runSecurityEvaluation,
  cancelActiveTask,
  fetchEvents,
  fetchResults,
  pageEvents,
  downloadArtifact,
  downloadSecurityEvaluation,
  fetchResearchTrainingDataset,
  importResearchTrainingDataset,
  downloadResearchTrainingDataset,
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

const trainingLabelColor = (label?: string) => {
  switch ((label || "").toUpperCase()) {
    case "ALLOW":
      return "green";
    case "ALERT":
      return "orange";
    case "BLOCK":
      return "red";
    case "REWRITE":
      return "blue";
    case "UNLABELED":
      return "default";
    default:
      return "geekblue";
  }
};

const trainingOutOfRangeValues = computed(() => {
  const normalization = researchTrainingDataset.value?.normalization;
  if (!normalization) return 0;
  return normalization.belowZeroValues + normalization.aboveOneValues;
});

const researchTrainingQualityWarnings = computed(
  () => researchTrainingDataset.value?.quality?.warnings || [],
);

const researchTrainingLabeledRatio = computed(() => {
  const dataset = researchTrainingDataset.value;
  if (!dataset?.sampleCount) return "0.0";
  return ((dataset.labeledCount / dataset.sampleCount) * 100).toFixed(1);
});

const researchTrainingSkippedReasonText = computed(
  () =>
    (researchTrainingImportResult.value?.skippedByReason || [])
      .map((item) => `${item.key}:${item.count}`)
      .join(", ") || "none",
);

const securityMetricCards = computed(() => {
  const report = securityEvaluation.value;
  const metrics = report?.metrics;
  const totals = report?.totals;
  return [
    {
      title: "Accuracy",
      value: Number(metrics?.accuracy || 0).toFixed(1),
      suffix: "%",
    },
    {
      title: "False Positive",
      value: Number(metrics?.falsePositiveRate || 0).toFixed(1),
      suffix: "%",
    },
    {
      title: "False Negative",
      value: Number(metrics?.falseNegativeRate || 0).toFixed(1),
      suffix: "%",
    },
    {
      title: "Samples",
      value: totals?.total || 0,
      suffix: "rows",
    },
  ];
});

const securityConfusionActions = computed(() => {
  const matrix = securityEvaluation.value?.confusionMatrix || {};
  const actions = new Set(["ALLOW", "ALERT", "BLOCK", "REWRITE"]);
  Object.values(matrix).forEach((row) => {
    Object.keys(row || {}).forEach((action) => actions.add(action));
  });
  return [...actions].sort();
});

const securityConfusionRows = computed(() => {
  const matrix = securityEvaluation.value?.confusionMatrix || {};
  return Object.keys(matrix)
    .sort()
    .map((expected) => ({
      expected,
      ...(matrix[expected] || {}),
    }));
});

const securityPosture = computed(() => securityEvaluation.value?.posture);

const securityPostureAlertType = computed(() => {
  switch ((securityPosture.value?.status || "").toLowerCase()) {
    case "critical":
      return "error";
    case "needs_review":
      return "warning";
    case "pass":
      return "success";
    default:
      return "info";
  }
});

const securityPostureColor = computed(() => {
  switch ((securityPosture.value?.status || "").toLowerCase()) {
    case "critical":
      return "red";
    case "needs_review":
      return "orange";
    case "pass":
      return "green";
    default:
      return "blue";
  }
});

const securityPriorityColor = (priority?: string) => {
  switch ((priority || "").toLowerCase()) {
    case "critical":
      return "red";
    case "high":
      return "volcano";
    case "medium":
      return "orange";
    case "low":
      return "green";
    default:
      return "blue";
  }
};

const securityPostureDescription = computed(() => {
  const posture = securityPosture.value;
  if (!posture) return "";
  const blockers = posture.blockingReasons || [];
  if (blockers.length > 0) {
    return blockers.map(formatSecurityToken).join("；");
  }
  const warnings = posture.warnings || [];
  if (warnings.length > 0) {
    return warnings.map(formatSecurityToken).join("；");
  }
  return "当前评测未发现阻断项，可导出报告用于复现实验。";
});

const securityFindingGroups = computed(() => {
  const findings = securityEvaluation.value?.findings || {};
  return [
    {
      key: "false_positive",
      title: "False Positives",
      rows: findings.falsePositives || [],
    },
    {
      key: "false_negative",
      title: "False Negatives",
      rows: findings.falseNegatives || [],
    },
    {
      key: "policy_gap",
      title: "Policy Gaps",
      rows: findings.policyGaps || [],
    },
    {
      key: "high_confidence_disagreement",
      title: "High Confidence Disagreements",
      rows: findings.highConfidenceDisagreements || [],
    },
    {
      key: "unlabeled_high_risk",
      title: "Unlabeled High Risk",
      rows: findings.unlabeledHighRisk || [],
    },
  ];
});

const securityActionColor = (action?: string) => {
  switch ((action || "").toUpperCase()) {
    case "ALLOW":
      return "green";
    case "ALERT":
      return "orange";
    case "BLOCK":
      return "red";
    case "REWRITE":
      return "blue";
    case "UNLABELED":
      return "default";
    default:
      return "geekblue";
  }
};

const formatSecurityToken = (value: string) =>
  value.replaceAll("_", " ").replaceAll(":", ": ");

const securityFindingColor = (finding?: string) => {
  switch ((finding || "").toLowerCase()) {
    case "false_positive":
      return "gold";
    case "false_negative":
      return "red";
    case "policy_gap":
      return "purple";
    case "high_confidence_disagreement":
      return "magenta";
    case "unlabeled_high_risk":
      return "volcano";
    default:
      return "default";
  }
};

const securitySampleRowKey = (row: ResearchSecurityEvaluationSampleRow) =>
  row.id || `${row.source}:${row.commandLine}:${row.expectedAction}`;

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

              <a-tab-pane key="security" tab="Security Eval">
                <a-alert
                  type="info"
                  show-icon
                  style="margin-bottom: 12px"
                  message="安全评测套件默认只读：不会写入训练集，不会生成或应用策略，也不会触发 kernel policy mutation。"
                  description="评测样本默认来自内置 Agent 安全基准 + 当前 Research Session 事件；输出 FP/FN、混淆矩阵、策略空洞和高风险未标注事件。"
                />
                <div class="research-toolbar">
                  <a-space wrap>
                    <span>Corpus</span>
                    <a-select
                      v-model:value="securityEvaluationMode"
                      size="small"
                      style="width: 180px"
                    >
                      <a-select-option value="combined">
                        内置 + 会话
                      </a-select-option>
                      <a-select-option value="builtin">
                        仅内置基准
                      </a-select-option>
                      <a-select-option value="session">
                        仅当前会话
                      </a-select-option>
                    </a-select>
                    <span>Label</span>
                    <a-select
                      v-model:value="securityEvaluationLabelPolicy"
                      size="small"
                      style="width: 210px"
                    >
                      <a-select-option value="decision_then_heuristic">
                        decision + heuristic
                      </a-select-option>
                      <a-select-option value="decision">
                        decision only
                      </a-select-option>
                      <a-select-option value="heuristic">
                        heuristic
                      </a-select-option>
                      <a-select-option value="unlabeled">
                        unlabeled
                      </a-select-option>
                    </a-select>
                    <span>Limit</span>
                    <a-input-number
                      v-model:value="securityEvaluationLimit"
                      :min="1"
                      :max="50000"
                      size="small"
                      style="width: 110px"
                    />
                    <a-checkbox v-model:checked="securityEvaluationIncludeLLM">
                      Include LLM
                    </a-checkbox>
                    <a-button
                      type="primary"
                      size="small"
                      @click="runSecurityEvaluation()"
                      :loading="submittingTask"
                      :disabled="!selectedSessionId"
                    >
                      <SecurityScanOutlined /> 运行安全评测
                    </a-button>
                    <a-button
                      size="small"
                      @click="downloadSecurityEvaluation('json')"
                      :loading="exportingSecurityEvaluation"
                      :disabled="!selectedSessionId || !securityEvaluation"
                    >
                      <CloudDownloadOutlined /> JSON
                    </a-button>
                    <a-button
                      size="small"
                      @click="downloadSecurityEvaluation('jsonl')"
                      :loading="exportingSecurityEvaluation"
                      :disabled="!selectedSessionId || !securityEvaluation"
                    >
                      <CloudDownloadOutlined /> JSONL
                    </a-button>
                    <a-button
                      size="small"
                      @click="downloadSecurityEvaluation('csv')"
                      :loading="exportingSecurityEvaluation"
                      :disabled="!selectedSessionId || !securityEvaluation"
                    >
                      <CloudDownloadOutlined /> CSV
                    </a-button>
                  </a-space>
                </div>

                <a-empty
                  v-if="!securityEvaluation"
                  description="点击“运行安全评测”生成 Agent 安全研究报告"
                />
                <template v-else>
                  <a-space wrap class="research-training-tags">
                    <a-tag color="blue">
                      schema: {{ securityEvaluation.schemaVersion }}
                    </a-tag>
                    <a-tag color="cyan">
                      mode: {{ securityEvaluation.mode }}
                    </a-tag>
                    <a-tag color="purple">
                      label: {{ securityEvaluation.labelPolicy }}
                    </a-tag>
                    <a-tag color="geekblue">
                      generated: {{ formatTime(securityEvaluation.generatedAt) }}
                    </a-tag>
                  </a-space>
                  <a-row :gutter="[12, 12]" class="research-stats">
                    <a-col
                      v-for="card in securityMetricCards"
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

                  <a-card
                    v-if="securityPosture"
                    size="small"
                    class="research-card"
                    title="Security Posture & Suggested Actions"
                  >
                    <a-alert
                      show-icon
                      :type="securityPostureAlertType"
                      :message="`Posture: ${securityPosture.status.toUpperCase()} · risk ${Number(securityPosture.riskScore || 0).toFixed(1)}`"
                      :description="securityPostureDescription"
                      style="margin-bottom: 12px"
                    />
                    <a-space wrap size="small" style="margin-bottom: 8px">
                      <a-tag :color="securityPostureColor">
                        {{ securityPosture.status }}
                      </a-tag>
                      <a-tag
                        v-for="item in securityPosture.findingCounts || []"
                        :key="item.key"
                        :color="securityFindingColor(item.key)"
                      >
                        {{ formatSecurityToken(item.key) }} {{ item.count }}
                      </a-tag>
                    </a-space>
                    <div
                      v-if="(securityPosture.suggestedActions || []).length > 0"
                      class="research-muted"
                      style="margin-bottom: 8px"
                    >
                      Suggested actions
                    </div>
                    <a-space
                      v-if="(securityPosture.suggestedActions || []).length > 0"
                      wrap
                      size="small"
                    >
                      <a-tag
                        v-for="action in securityPosture.suggestedActions"
                        :key="action"
                        color="green"
                      >
                        {{ formatSecurityToken(action) }}
                      </a-tag>
                    </a-space>
                    <a-table
                      v-if="(securityPosture.remediationPlan || []).length > 0"
                      :dataSource="securityPosture.remediationPlan"
                      :pagination="false"
                      rowKey="id"
                      size="small"
                      :scroll="{ x: 1120 }"
                      style="margin-top: 12px"
                    >
                      <a-table-column title="Priority" dataIndex="priority" :width="100">
                        <template #default="{ record }">
                          <a-tag :color="securityPriorityColor(record.priority)">
                            {{ record.priority }}
                          </a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column title="Area" dataIndex="area" :width="160">
                        <template #default="{ record }">
                          {{ formatSecurityToken(record.area || "") }}
                        </template>
                      </a-table-column>
                      <a-table-column title="Action" dataIndex="action" :width="260">
                        <template #default="{ record }">
                          <code>{{ formatSecurityToken(record.action || "") }}</code>
                        </template>
                      </a-table-column>
                      <a-table-column title="Count" dataIndex="count" :width="80" />
                      <a-table-column title="Rationale" dataIndex="rationale" :width="420" ellipsis />
                      <a-table-column title="Related Commands" :width="320" ellipsis>
                        <template #default="{ record }">
                          <span class="research-muted">
                            {{ (record.relatedCommands || []).slice(0, 2).join(' | ') || '—' }}
                          </span>
                        </template>
                      </a-table-column>
                    </a-table>
                    <a-table
                      v-if="(securityPosture.topFailingCategories || []).length > 0"
                      :dataSource="securityPosture.topFailingCategories"
                      :pagination="false"
                      rowKey="key"
                      size="small"
                      style="margin-top: 12px"
                    >
                      <a-table-column title="Top failing category" dataIndex="key" />
                      <a-table-column title="Failed" dataIndex="failed" :width="90" />
                      <a-table-column
                        title="FP"
                        dataIndex="falsePositives"
                        :width="80"
                      />
                      <a-table-column
                        title="FN"
                        dataIndex="falseNegatives"
                        :width="80"
                      />
                      <a-table-column
                        title="Avg Risk"
                        dataIndex="avgRiskScore"
                        :width="100"
                      >
                        <template #default="{ record }">
                          <a-tag :color="riskColor(record.avgRiskScore)">
                            {{ (record.avgRiskScore || 0).toFixed(1) }}
                          </a-tag>
                        </template>
                      </a-table-column>
                    </a-table>
                  </a-card>

                  <a-row :gutter="[16, 16]">
                    <a-col :xs="24" :lg="12">
                      <a-card size="small" title="Confusion Matrix">
                        <a-table
                          :dataSource="securityConfusionRows"
                          :pagination="false"
                          rowKey="expected"
                          size="small"
                        >
                          <a-table-column
                            title="Expected"
                            dataIndex="expected"
                            :width="130"
                          >
                            <template #default="{ record }">
                              <a-tag :color="securityActionColor(record.expected)">
                                {{ record.expected }}
                              </a-tag>
                            </template>
                          </a-table-column>
                          <a-table-column
                            v-for="action in securityConfusionActions"
                            :key="action"
                            :title="action"
                            :dataIndex="action"
                            :width="90"
                          >
                            <template #default="{ record }">
                              {{ record[action] || 0 }}
                            </template>
                          </a-table-column>
                        </a-table>
                      </a-card>
                    </a-col>
                    <a-col :xs="24" :lg="12">
                      <a-card size="small" title="Top Failing Categories">
                        <a-table
                          :dataSource="securityEvaluation.byCategory || []"
                          :pagination="false"
                          rowKey="key"
                          size="small"
                        >
                          <a-table-column title="Category" dataIndex="key" />
                          <a-table-column
                            title="Total"
                            dataIndex="total"
                            :width="80"
                          />
                          <a-table-column
                            title="Failed"
                            dataIndex="failed"
                            :width="80"
                          />
                          <a-table-column
                            title="Avg Risk"
                            dataIndex="avgRiskScore"
                            :width="100"
                          >
                            <template #default="{ record }">
                              <a-tag :color="riskColor(record.avgRiskScore)">
                                {{ (record.avgRiskScore || 0).toFixed(1) }}
                              </a-tag>
                            </template>
                          </a-table-column>
                        </a-table>
                      </a-card>
                    </a-col>
                  </a-row>

                  <a-row :gutter="[12, 12]" class="research-card">
                    <a-col
                      v-for="group in securityFindingGroups"
                      :key="group.key"
                      :xs="24"
                      :lg="12"
                    >
                      <a-card size="small">
                        <template #title>
                          <span>
                            {{ group.title }}
                            <a-tag :color="securityFindingColor(group.key)">
                              {{ group.rows.length }}
                            </a-tag>
                          </span>
                        </template>
                        <a-list
                          size="small"
                          :data-source="group.rows.slice(0, 5)"
                        >
                          <template #renderItem="{ item }">
                            <a-list-item>
                              <div class="research-finding-row">
                                <div>
                                  <code>{{ item.commandLine }}</code>
                                  <div class="research-muted">
                                    {{ item.source }} · {{ item.category || "—" }}
                                  </div>
                                </div>
                                <a-space>
                                  <a-tag
                                    :color="
                                      securityActionColor(item.expectedAction)
                                    "
                                  >
                                    E: {{ item.expectedAction }}
                                  </a-tag>
                                  <a-tag
                                    :color="
                                      securityActionColor(item.observedAction)
                                    "
                                  >
                                    O: {{ item.observedAction }}
                                  </a-tag>
                                </a-space>
                              </div>
                            </a-list-item>
                          </template>
                          <template #empty>
                            <a-empty description="暂无发现" />
                          </template>
                        </a-list>
                      </a-card>
                    </a-col>
                  </a-row>

                  <a-card
                    size="small"
                    class="research-card"
                    title="Evaluation Samples"
                  >
                    <a-table
                      :dataSource="securityEvaluationPreviewSamples"
                      :pagination="{ pageSize: 10 }"
                      :scroll="{ x: 1380 }"
                      :rowKey="securitySampleRowKey"
                      size="small"
                    >
                      <a-table-column
                        title="Command"
                        dataIndex="commandLine"
                        :width="320"
                        ellipsis
                      >
                        <template #default="{ record }">
                          <code>{{ record.commandLine }}</code>
                        </template>
                      </a-table-column>
                      <a-table-column title="Source" dataIndex="source" :width="110">
                        <template #default="{ record }">
                          <a-tag color="geekblue">{{ record.source }}</a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Expected"
                        dataIndex="expectedAction"
                        :width="120"
                      >
                        <template #default="{ record }">
                          <a-tag :color="securityActionColor(record.expectedAction)">
                            {{ record.expectedAction }}
                          </a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Observed"
                        dataIndex="observedAction"
                        :width="120"
                      >
                        <template #default="{ record }">
                          <a-tag :color="securityActionColor(record.observedAction)">
                            {{ record.observedAction }}
                          </a-tag>
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
                        title="Finding"
                        dataIndex="findingType"
                        :width="180"
                      >
                        <template #default="{ record }">
                          <a-tag :color="securityFindingColor(record.findingType)">
                            {{ record.findingType || "pass" }}
                          </a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column
                        title="Reasoning"
                        dataIndex="reasoning"
                        :width="360"
                        ellipsis
                      />
                    </a-table>
                  </a-card>
                </template>
              </a-tab-pane>

              <a-tab-pane key="training" tab="Training Dataset">
                <a-alert
                  type="info"
                  show-icon
                  style="margin-bottom: 12px"
                  message="将当前 Research Session 的归一化事件转换为训练样本：固定 128 维 featureVector、feature names、label policy 与 normalization report。"
                  description="decision 策略只信任已有决策，适合直接导入 ML TrainingDataStore；heuristic 会补充风险启发式标签；unlabeled 适合离线标注和导出。"
                />
                <div class="research-toolbar">
                  <a-space wrap>
                    <span>Label policy</span>
                    <a-select
                      v-model:value="researchTrainingLabelPolicy"
                      size="small"
                      style="width: 180px"
                    >
                      <a-select-option value="decision">
                        decision（推荐导入）
                      </a-select-option>
                      <a-select-option value="heuristic">
                        heuristic
                      </a-select-option>
                      <a-select-option value="unlabeled">
                        unlabeled（仅导出）
                      </a-select-option>
                    </a-select>
                    <span>Import limit</span>
                    <a-input-number
                      v-model:value="researchTrainingImportLimit"
                      :min="0"
                      :max="50000"
                      size="small"
                      style="width: 110px"
                    />
                    <a-button
                      size="small"
                      @click="fetchResearchTrainingDataset()"
                      :loading="loadingResearchTraining"
                      :disabled="!selectedSessionId"
                    >
                      <ReloadOutlined /> 预览训练集
                    </a-button>
                    <a-button
                      size="small"
                      type="primary"
                      @click="importResearchTrainingDataset()"
                      :loading="importingResearchTraining"
                      :disabled="
                        !selectedSessionId ||
                        researchTrainingLabelPolicy === 'unlabeled'
                      "
                    >
                      <ImportOutlined /> 导入 ML 训练库
                    </a-button>
                    <a-button
                      size="small"
                      @click="downloadResearchTrainingDataset('jsonl')"
                      :loading="exportingResearchTraining"
                      :disabled="!selectedSessionId"
                    >
                      <CloudDownloadOutlined /> JSONL
                    </a-button>
                    <a-button
                      size="small"
                      @click="downloadResearchTrainingDataset('csv')"
                      :loading="exportingResearchTraining"
                      :disabled="!selectedSessionId"
                    >
                      <CloudDownloadOutlined /> CSV
                    </a-button>
                  </a-space>
                </div>
                <a-row :gutter="[12, 12]" class="research-stats">
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Samples"
                        :value="researchTrainingDataset?.sampleCount || 0"
                        suffix="rows"
                      />
                    </a-card>
                  </a-col>
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Labeled"
                        :value="researchTrainingDataset?.labeledCount || 0"
                        :suffix="`${researchTrainingLabeledRatio}%`"
                      />
                    </a-card>
                  </a-col>
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Importable"
                        :value="
                          researchTrainingDataset?.quality?.importableCount || 0
                        "
                        suffix="unique"
                      />
                    </a-card>
                  </a-col>
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Unlabeled"
                        :value="
                          researchTrainingDataset?.quality?.unlabeledCount || 0
                        "
                        suffix="rows"
                      />
                    </a-card>
                  </a-col>
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Feature Dim"
                        :value="researchTrainingDataset?.featureDim || 0"
                        suffix="dim"
                      />
                    </a-card>
                  </a-col>
                  <a-col :xs="12" :md="4">
                    <a-card size="small">
                      <a-statistic
                        title="Out-of-range"
                        :value="trainingOutOfRangeValues"
                        suffix="values"
                      />
                    </a-card>
                  </a-col>
                </a-row>
                <a-space
                  v-if="researchTrainingDataset"
                  wrap
                  class="research-training-tags"
                >
                  <a-tag color="blue">
                    schema: {{ researchTrainingDataset.schemaVersion }}
                  </a-tag>
                  <a-tag color="cyan">
                    norm: {{ researchTrainingDataset.normalization.mode }}
                  </a-tag>
                  <a-tag color="green">
                    min:
                    {{
                      researchTrainingDataset.normalization.minObserved.toFixed(3)
                    }}
                  </a-tag>
                  <a-tag color="green">
                    max:
                    {{
                      researchTrainingDataset.normalization.maxObserved.toFixed(3)
                    }}
                  </a-tag>
                  <a-tag
                    v-for="label in researchTrainingDataset.byLabel"
                    :key="label.key"
                    :color="trainingLabelColor(label.key)"
                  >
                    {{ label.key }}: {{ label.count }}
                  </a-tag>
                  <a-tag
                    v-for="category in researchTrainingDataset.byCategory || []"
                    :key="`training-category-${category.key}`"
                    color="purple"
                  >
                    {{ category.key }}: {{ category.count }}
                  </a-tag>
                  <a-tag
                    v-for="source in researchTrainingDataset.bySource || []"
                    :key="`training-source-${source.key}`"
                    color="geekblue"
                  >
                    src {{ source.key }}: {{ source.count }}
                  </a-tag>
                </a-space>
                <a-alert
                  v-if="researchTrainingQualityWarnings.length"
                  type="warning"
                  show-icon
                  style="margin-bottom: 12px"
                  message="训练可用性提示"
                  :description="researchTrainingQualityWarnings.join(', ')"
                />
                <a-alert
                  v-if="researchTrainingImportResult"
                  type="success"
                  show-icon
                  style="margin-bottom: 12px"
                  :message="`导入完成：新增 ${researchTrainingImportResult.imported}，跳过 ${researchTrainingImportResult.skipped}`"
                  :description="`当前训练库 total=${researchTrainingImportResult.totalSamples}, labeled=${researchTrainingImportResult.labeledSamples}; skipped=${researchTrainingSkippedReasonText}`"
                />
                <a-empty
                  v-if="!researchTrainingDataset"
                  description="点击“预览训练集”生成训练视图"
                />
                <a-table
                  v-else
                  :dataSource="researchTrainingPreviewSamples"
                  :pagination="false"
                  :scroll="{ x: 1180 }"
                  rowKey="sampleId"
                  size="small"
                >
                  <a-table-column
                    title="Command"
                    dataIndex="commandLine"
                    :width="300"
                    ellipsis
                  >
                    <template #default="{ record }">
                      <code>{{ record.commandLine }}</code>
                    </template>
                  </a-table-column>
                  <a-table-column title="Event" dataIndex="eventType" :width="140" />
                  <a-table-column title="Comm" dataIndex="comm" :width="120" />
                  <a-table-column title="Label" dataIndex="labelName" :width="110">
                    <template #default="{ record }">
                      <a-tag :color="trainingLabelColor(record.labelName)">
                        {{ record.labelName }}
                      </a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column
                    title="Source"
                    dataIndex="labelSource"
                    :width="160"
                  />
                  <a-table-column title="Risk" dataIndex="riskScore" :width="90">
                    <template #default="{ record }">
                      <a-tag :color="riskColor(record.riskScore)">
                        {{ (record.riskScore || 0).toFixed(1) }}
                      </a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column
                    title="Anomaly"
                    dataIndex="anomalyScore"
                    :width="100"
                  >
                    <template #default="{ record }">
                      {{ (record.anomalyScore || 0).toFixed(3) }}
                    </template>
                  </a-table-column>
                  <a-table-column title="Feature" :width="120">
                    <template #default="{ record }">
                      {{ record.featureVector?.length || 0 }} dim
                    </template>
                  </a-table-column>
                  <a-table-column title="Time" dataIndex="time" :width="180">
                    <template #default="{ record }">
                      <span class="research-muted">{{
                        formatTime(record.time)
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
  color: #475569;
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
  color: #475569;
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
  color: #475569;
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
  color: #475569;
  font-size: 12px;
}

.research-training-tags {
  margin-bottom: 12px;
}

.research-finding-row {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  justify-content: space-between;
  width: 100%;
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
