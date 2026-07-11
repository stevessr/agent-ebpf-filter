<script setup lang="ts">
import { computed } from "vue";
import {
  CloudDownloadOutlined,
  ImportOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type {
  ResearchTrainingDataset,
  ResearchTrainingImportResponse,
  ResearchTrainingLabelPolicy,
  ResearchTrainingSample,
} from "../../types/config";
import { trainingLabelColor } from "./researchViewUtils";

type TrainingExportFormat = "jsonl" | "csv";

interface Props {
  selectedSessionId: string;
  loadingResearchTraining: boolean;
  importingResearchTraining: boolean;
  exportingResearchTraining: boolean;
  researchTrainingDataset: ResearchTrainingDataset | null;
  researchTrainingImportResult: ResearchTrainingImportResponse | null;
  researchTrainingPreviewSamples: ResearchTrainingSample[];
  riskColor: (risk?: number) => string;
  formatTime: (value?: string | number) => string;
}

const props = defineProps<Props>();

const researchTrainingLabelPolicy = defineModel<ResearchTrainingLabelPolicy>(
  "researchTrainingLabelPolicy",
  { required: true },
);
const researchTrainingImportLimit = defineModel<number>(
  "researchTrainingImportLimit",
  { required: true },
);

const emit = defineEmits<{
  preview: [];
  importDataset: [];
  download: [format: TrainingExportFormat];
}>();

const fetchResearchTrainingDataset = () => emit("preview");
const importResearchTrainingDataset = () => emit("importDataset");
const downloadResearchTrainingDataset = (format: TrainingExportFormat) =>
  emit("download", format);

const trainingOutOfRangeValues = computed(() => {
  const normalization = props.researchTrainingDataset?.normalization;
  if (!normalization) return 0;
  return normalization.belowZeroValues + normalization.aboveOneValues;
});

const researchTrainingQualityWarnings = computed(
  () => props.researchTrainingDataset?.quality?.warnings || [],
);

const researchTrainingLabeledRatio = computed(() => {
  const dataset = props.researchTrainingDataset;
  if (!dataset?.sampleCount) return "0.0";
  return ((dataset.labeledCount / dataset.sampleCount) * 100).toFixed(1);
});

const researchTrainingSkippedReasonText = computed(
  () =>
    (props.researchTrainingImportResult?.skippedByReason || [])
      .map((item) => `${item.key}:${item.count}`)
      .join(", ") || "none",
);
</script>

<template>
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
</template>

<style scoped>
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
</style>
