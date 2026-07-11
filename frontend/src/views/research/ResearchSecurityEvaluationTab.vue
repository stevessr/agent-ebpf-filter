<script setup lang="ts">
import { computed } from "vue";
import {
  CloudDownloadOutlined,
  SecurityScanOutlined,
} from "@ant-design/icons-vue";
import type {
  ResearchSecurityEvaluationReport,
  ResearchSecurityEvaluationSampleRow,
} from "../../types/config";
import {
  formatSecurityToken,
  securityActionColor,
  securityFindingColor,
  securityPriorityColor,
  securitySampleRowKey,
} from "./researchViewUtils";

type SecurityEvaluationMode = "combined" | "builtin" | "session";
type SecurityEvaluationExportFormat = "json" | "jsonl" | "csv";

interface Props {
  selectedSessionId: string;
  submittingTask: boolean;
  exportingSecurityEvaluation: boolean;
  securityEvaluation: ResearchSecurityEvaluationReport | null;
  securityEvaluationPreviewSamples: ResearchSecurityEvaluationSampleRow[];
  riskColor: (risk?: number) => string;
  formatTime: (value?: string | number) => string;
}

const props = defineProps<Props>();

const securityEvaluationMode = defineModel<SecurityEvaluationMode>(
  "securityEvaluationMode",
  { required: true },
);
const securityEvaluationLimit = defineModel<number>("securityEvaluationLimit", {
  required: true,
});
const securityEvaluationIncludeLLM = defineModel<boolean>(
  "securityEvaluationIncludeLLM",
  { required: true },
);
const securityEvaluationLabelPolicy = defineModel<string>(
  "securityEvaluationLabelPolicy",
  { required: true },
);

const emit = defineEmits<{
  run: [];
  download: [format: SecurityEvaluationExportFormat];
}>();

const runSecurityEvaluation = () => emit("run");
const downloadSecurityEvaluation = (format: SecurityEvaluationExportFormat) =>
  emit("download", format);

const securityMetricCards = computed(() => {
  const metrics = props.securityEvaluation?.metrics;
  const totals = props.securityEvaluation?.totals;
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
  const matrix = props.securityEvaluation?.confusionMatrix || {};
  const actions = new Set(["ALLOW", "ALERT", "BLOCK", "REWRITE"]);
  Object.values(matrix).forEach((row) => {
    Object.keys(row || {}).forEach((action) => actions.add(action));
  });
  return [...actions].sort();
});

const securityConfusionRows = computed(() => {
  const matrix = props.securityEvaluation?.confusionMatrix || {};
  return Object.keys(matrix)
    .sort()
    .map((expected) => ({
      expected,
      ...(matrix[expected] || {}),
    }));
});

const securityPosture = computed(() => props.securityEvaluation?.posture);

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
  const findings = props.securityEvaluation?.findings || {};
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
</script>

<template>
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
</template>

<style scoped>
.research-card {
  margin-top: 12px;
}

.research-stats {
  margin-bottom: 12px;
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
</style>
