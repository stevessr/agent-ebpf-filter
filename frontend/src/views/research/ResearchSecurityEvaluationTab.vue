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

type SecurityEvaluationMode =
  | "combined"
  | "builtin"
  | "session"
  | "session_outcome"
  | "combined_outcome";
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

const outcomeModeSelected = computed(() =>
  ["session_outcome", "combined_outcome"].includes(securityEvaluationMode.value),
);

const outcomeValidation = computed(
  () => props.securityEvaluation?.outcomeValidation,
);

const outcomeMetricCards = computed(() => {
  const outcome = outcomeValidation.value;
  if (!outcome?.enabled) return [];
  return [
    { title: "Unique actionable", value: outcome.uniqueActionable ?? outcome.actionable ?? 0 },
    { title: "Impact confirmed", value: outcome.impactConfirmed || 0 },
    { title: "Reproduced", value: outcome.reproduced || 0 },
    { title: "Reachable", value: outcome.reachable || 0 },
    { title: "Conflicted", value: outcome.conflicted || 0 },
    { title: "Rejected", value: outcome.rejected || 0 },
    { title: "Unauthorized", value: outcome.unauthorizedEvidence || 0 },
    { title: "Out of scope", value: outcome.outOfScope || 0 },
    { title: "Deduped variants", value: outcome.duplicateActionable || 0 },
    { title: "Unproven", value: outcome.unproven || 0 },
  ];
});

const outcomePolicyDescription = computed(() => {
  const outcome = outcomeValidation.value;
  if (!outcome?.enabled) return "";
  const parts = [
    `minimum=${outcome.minimumEvidence}`,
    `authorization=${outcome.requireAuthorization ? "required" : "disabled"}`,
    `independent-refutation=${outcome.requireIndependentRefutation ? "required" : "disabled"}`,
    `dedupe=${outcome.dedupeActionable ? "on" : "off"}`,
    `window=${outcome.correlationWindowSeconds || 30}s`,
  ];
  if (outcome.allowedValidatorSources?.length) {
    parts.push(`validators=${outcome.allowedValidatorSources.join(",")}`);
  }
  if (outcome.allowedAuthorizationIds?.length) {
    parts.push(`auth-ids=${outcome.allowedAuthorizationIds.join(",")}`);
  }
  if (outcome.allowedTargets?.length) {
    parts.push(`targets=${outcome.allowedTargets.join(",")}`);
  }
  return parts.join(" · ");
});

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
    :type="outcomeModeSelected ? 'warning' : 'info'"
    show-icon
    style="margin-bottom: 12px"
    :message="outcomeModeSelected
      ? '结果验证（Glasswing-inspired）已选择：默认只有 authorized + reproduced 或更强证据进入 actionable。'
      : '安全评测套件默认只读：不会写入训练集，不会生成或应用策略，也不会触发 kernel policy mutation。'"
    :description="outcomeModeSelected
      ? '默认要求授权证据、独立反证和结果去重；该模式只消费 Research Session 中的验证证据，不会自动对任意外部目标执行 exploit。'
      : '评测样本默认来自内置 Agent 安全基准 + 当前 Research Session 事件；输出 FP/FN、混淆矩阵、策略空洞和高风险未标注事件。'"
  />

  <div class="research-toolbar">
    <a-space wrap>
      <span>Corpus</span>
      <a-select
        v-model:value="securityEvaluationMode"
        size="small"
        style="width: 250px"
      >
        <a-select-option value="combined">内置 + 会话</a-select-option>
        <a-select-option value="builtin">仅内置基准</a-select-option>
        <a-select-option value="session">仅当前会话</a-select-option>
        <a-select-option value="session_outcome">当前会话 · 结果验证</a-select-option>
        <a-select-option value="combined_outcome">内置 + 会话 · 结果验证</a-select-option>
      </a-select>
      <span>Label</span>
      <a-select
        v-model:value="securityEvaluationLabelPolicy"
        size="small"
        style="width: 210px"
      >
        <a-select-option value="decision_then_heuristic">decision + heuristic</a-select-option>
        <a-select-option value="decision">decision only</a-select-option>
        <a-select-option value="heuristic">heuristic</a-select-option>
        <a-select-option value="unlabeled">unlabeled</a-select-option>
      </a-select>
      <span>Limit</span>
      <a-input-number
        v-model:value="securityEvaluationLimit"
        :min="1"
        :max="50000"
        size="small"
        style="width: 110px"
      />
      <a-checkbox v-model:checked="securityEvaluationIncludeLLM">Include LLM</a-checkbox>
      <a-button
        type="primary"
        size="small"
        :loading="submittingTask"
        :disabled="!selectedSessionId"
        @click="runSecurityEvaluation"
      >
        <template #icon><SecurityScanOutlined /></template>
        Run Security Eval
      </a-button>
      <a-dropdown :disabled="!securityEvaluation || exportingSecurityEvaluation">
        <a-button size="small" :loading="exportingSecurityEvaluation">
          <template #icon><CloudDownloadOutlined /></template>
          Export
        </a-button>
        <template #overlay>
          <a-menu>
            <a-menu-item key="json" @click="downloadSecurityEvaluation('json')">JSON</a-menu-item>
            <a-menu-item key="jsonl" @click="downloadSecurityEvaluation('jsonl')">JSONL</a-menu-item>
            <a-menu-item key="csv" @click="downloadSecurityEvaluation('csv')">CSV</a-menu-item>
          </a-menu>
        </template>
      </a-dropdown>
    </a-space>
  </div>

  <template v-if="securityEvaluation">
    <a-alert
      v-if="securityPosture"
      :type="securityPostureAlertType"
      show-icon
      style="margin: 12px 0"
      :message="`Security posture: ${securityPosture.status} · risk ${Number(securityPosture.riskScore || 0).toFixed(1)}`"
      :description="securityPostureDescription"
    />

    <a-alert
      v-if="outcomeValidation?.enabled"
      type="info"
      show-icon
      style="margin: 12px 0"
      :message="`Outcome validation · minimum evidence: ${outcomeValidation.minimumEvidence}`"
      :description="outcomePolicyDescription"
    />

    <a-row v-if="outcomeValidation?.enabled" :gutter="[8, 8]" style="margin-bottom: 12px">
      <a-col
        v-for="metric in outcomeMetricCards"
        :key="metric.title"
        :xs="12"
        :sm="8"
        :md="6"
        :lg="4"
      >
        <a-card size="small">
          <a-statistic :title="metric.title" :value="metric.value" />
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="[8, 8]" style="margin-bottom: 12px">
      <a-col
        v-for="metric in securityMetricCards"
        :key="metric.title"
        :xs="12"
        :sm="12"
        :md="6"
      >
        <a-card size="small">
          <a-statistic
            :title="metric.title"
            :value="metric.value"
            :suffix="metric.suffix"
          />
        </a-card>
      </a-col>
    </a-row>

    <a-card size="small" title="Confusion Matrix" style="margin-bottom: 12px">
      <a-table
        :data-source="securityConfusionRows"
        :pagination="false"
        size="small"
        row-key="expected"
        :scroll="{ x: 'max-content' }"
      >
        <a-table-column title="Expected" data-index="expected" key="expected" />
        <a-table-column
          v-for="action in securityConfusionActions"
          :key="action"
          :title="action"
        >
          <template #default="{ record }">
            {{ Number(record[action] || 0) }}
          </template>
        </a-table-column>
      </a-table>
    </a-card>

    <a-card
      v-if="securityPosture?.remediationPlan?.length"
      size="small"
      title="Remediation Plan"
      style="margin-bottom: 12px"
    >
      <a-list :data-source="securityPosture.remediationPlan" size="small">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-space direction="vertical" size="small" style="width: 100%">
              <a-space wrap>
                <a-tag :color="securityPriorityColor(item.priority)">{{ item.priority }}</a-tag>
                <strong>{{ formatSecurityToken(item.action) }}</strong>
                <span>× {{ item.count }}</span>
                <a-tag v-if="item.findingType">{{ formatSecurityToken(item.findingType) }}</a-tag>
              </a-space>
              <span>{{ item.rationale }}</span>
              <a-typography-text v-if="item.relatedCommands?.length" type="secondary">
                {{ item.relatedCommands.join(" · ") }}
              </a-typography-text>
            </a-space>
          </a-list-item>
        </template>
      </a-list>
    </a-card>

    <a-collapse v-if="securityFindingGroups.some((group) => group.rows.length)" style="margin-bottom: 12px">
      <a-collapse-panel
        v-for="group in securityFindingGroups.filter((item) => item.rows.length)"
        :key="group.key"
        :header="`${group.title} (${group.rows.length})`"
      >
        <a-table
          :data-source="group.rows"
          :pagination="false"
          size="small"
          :row-key="securitySampleRowKey"
          :scroll="{ x: 'max-content' }"
        >
          <a-table-column title="Time" key="time">
            <template #default="{ record }">{{ formatTime(record.time || record.timestamp) }}</template>
          </a-table-column>
          <a-table-column title="Command" key="command">
            <template #default="{ record }">
              <a-typography-text code>{{ record.commandLine || record.comm || '-' }}</a-typography-text>
            </template>
          </a-table-column>
          <a-table-column title="Expected" key="expected">
            <template #default="{ record }">
              <a-tag :color="securityActionColor(record.expectedAction)">{{ record.expectedAction }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="Observed" key="observed">
            <template #default="{ record }">
              <a-tag :color="securityActionColor(record.observedAction)">{{ record.observedAction }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="Finding" key="finding">
            <template #default="{ record }">
              <a-tag :color="securityFindingColor(record.findingType)">{{ formatSecurityToken(record.findingType || '-') }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="Outcome evidence" key="outcome">
            <template #default="{ record }">
              <a-space wrap>
                <a-tag v-if="record.validationStatus">{{ formatSecurityToken(record.validationStatus) }}</a-tag>
                <a-tag v-if="record.evidenceLevel" color="blue">{{ formatSecurityToken(record.evidenceLevel) }}</a-tag>
                <a-tag v-if="record.actionable" color="red">actionable</a-tag>
                <a-tag v-if="record.evidenceConflict" color="orange">conflict</a-tag>
              </a-space>
            </template>
          </a-table-column>
          <a-table-column title="Risk" key="risk">
            <template #default="{ record }">
              <a-tag :color="riskColor(record.riskScore)">{{ Number(record.riskScore || 0).toFixed(1) }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="Recommendation" data-index="recommendation" key="recommendation" />
        </a-table>
      </a-collapse-panel>
    </a-collapse>

    <a-card size="small" title="Sample Preview">
      <a-table
        :data-source="securityEvaluationPreviewSamples"
        :pagination="false"
        size="small"
        :row-key="securitySampleRowKey"
        :scroll="{ x: 'max-content' }"
      >
        <a-table-column title="Source" data-index="source" key="source" />
        <a-table-column title="Command" key="command">
          <template #default="{ record }">
            <a-typography-text code>{{ record.commandLine || record.comm || '-' }}</a-typography-text>
          </template>
        </a-table-column>
        <a-table-column title="Expected" key="expected">
          <template #default="{ record }">
            <a-tag :color="securityActionColor(record.expectedAction)">{{ record.expectedAction }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="Observed" key="observed">
          <template #default="{ record }">
            <a-tag :color="securityActionColor(record.observedAction)">{{ record.observedAction }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="Outcome" key="outcome">
          <template #default="{ record }">
            <a-space direction="vertical" size="small">
              <a-space wrap>
                <a-tag v-if="record.validationStatus">{{ formatSecurityToken(record.validationStatus) }}</a-tag>
                <a-tag v-if="record.evidenceLevel" color="blue">{{ formatSecurityToken(record.evidenceLevel) }}</a-tag>
                <a-tag v-if="record.actionable" color="red">actionable</a-tag>
                <a-tag v-if="record.evidenceConflict" color="orange">conflict</a-tag>
              </a-space>
              <a-typography-text v-if="record.findingKey" type="secondary" code>
                {{ record.findingKey }}
              </a-typography-text>
              <a-typography-text v-if="record.validatorReason" type="secondary">
                {{ record.validatorReason }}
              </a-typography-text>
            </a-space>
          </template>
        </a-table-column>
        <a-table-column title="Risk" key="risk">
          <template #default="{ record }">
            <a-tag :color="riskColor(record.riskScore)">{{ Number(record.riskScore || 0).toFixed(1) }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="Pass" key="passed">
          <template #default="{ record }">
            <a-tag :color="record.passed ? 'green' : 'red'">{{ record.passed ? 'PASS' : 'FAIL' }}</a-tag>
          </template>
        </a-table-column>
      </a-table>
    </a-card>
  </template>
</template>
