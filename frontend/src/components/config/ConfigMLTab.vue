<script setup lang="ts">
import { ref, onMounted, defineAsyncComponent, watch, computed } from 'vue';
import {
  ThunderboltOutlined, ReloadOutlined, SearchOutlined, PlusOutlined,
  ImportOutlined, ExportOutlined, DownloadOutlined, CopyOutlined, DeleteOutlined,
  FileOutlined, StopOutlined, AlertOutlined, CheckCircleOutlined, ExclamationCircleOutlined,
  EyeOutlined, EyeInvisibleOutlined, BookOutlined, GlobalOutlined,
} from '@ant-design/icons-vue';
import { getCategoryColor } from '../../composables/useConfigRegistry';
import { classicSecurityDatasetPresets, highRiskPresets, safetyNetHighRiskPresets, type useConfigML } from '../../composables/useConfigML';
import { useMLStatusStream } from '../../composables/useMLStatusStream';

const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

const props = defineProps<{
  ml: ReturnType<typeof useConfigML>;
}>();

const {
  mlEnabled, mlStatus, trainingModel, feedbackComm, feedbackAction,
  mlThresholds, mlTrainingConfig, llmScoringConfig, llmBatchConfig,
  llmBatchResponse, llmBatchLoading, trainingLogs, wsActive,
  trainingHistory, hyperParams,
  autoTuneXAxis, autoTuneYAxis, autoTuneGridSize, autoTuneGranularity, autoTuneMetric,
  autoTuneMinX, autoTuneMaxX, autoTuneMinY, autoTuneMaxY,
  autoTuneAxisOptions,
  autoTuneLoading, autoTuneInProgress, autoTuneCompleted, autoTuneTotal, autoTuneMessage, autoTuneError,
  autoTuneResponse, autoTuneSelectedCell,
  autoTuneAxisLabel, autoTuneMetricLabel, autoTuneMetricFormat,
  autoTuneGranularityLabel,
  autoTuneScore, autoTuneHeatmapOptions, autoTuneHeatmapSeries, autoTuneBestCell,
  runAutoTune, applyAutoTuneCell,
  allSamples, loadingSamples, sampleTablePageSize, sampleSearchText,
  existingDataLimit, existingLabelMode, existingCommandCandidates,
  loadingExistingData, importingExistingData, existingDataSource,
  remoteDatasetUrl, remoteDatasetFormat, remoteDatasetLabelMode, remoteDatasetLimit,
  loadingRemoteDataset, importingRemoteDataset, remoteDatasetPreview, remoteDatasetMeta,
  llmProductionDatasetLimit, llmProductionAllowHeuristic, llmProductionDeduplicate,
  llmProductionLoading, llmProductionPreview, llmProductionMeta,
  trainingDatasetImportInput, importingClassicDataset, dataMaskEnabled,
  sampleCommandLine, sampleLabel, submittingSample,
  backtestCommandLine, backtesting, backtestResult,
  applyMLStatusResponse, fetchMLStatus, trainingChartOptions, trainingChartSeries,
  submitFeedback, saveMLThresholds, runLLMBatchScore, llmBatchRowKey, llmBatchCanApplyLabels,
  filteredSamples, existingDuplicateCount, importableExistingCount,
  fetchAllSamples, fetchExistingCommandData, importExistingCommandData,
  fetchRemoteDatasetPreview, importRemoteDataset,
  fetchLLMProductionDataset, exportLLMProductionDataset,
  importClassicDataset, openClassicSecurityDatasetPage, copyClassicSecurityDatasetPage,
  maskSensitiveData,
  labelSample, deleteSample, updateAnomaly,
  importTrainingDatasetFromFile, exportTrainingDataset, clearTrainingDataset,
  getLabelColor, trainWithParams,
  openTrainingDatasetImportPicker,
  submitManualSample, addPresetSample, importAllHighRiskPresets,
  importAllSafetyNetPresets,
  runBacktest, runBacktestPreset, riskLevelColor, riskMeterColor,
  llmApiKeyStatus, llmSaveStatus, saveLLMConfigNow, modelType, cudaAvailable, cudaInfo, cudaMemUsedMB, cudaMemTotalMB, cancelTraining, cancellingTraining,
} = props.ml;

void trainingDatasetImportInput;

// WebSocket status stream — replaces polling
const { connect: wsConnect } = useMLStatusStream(applyMLStatusResponse);

const mlSubTabKey = ref(localStorage.getItem('config_ml_subtab') || 'status');

// Persist sub-tab selection
watch(mlSubTabKey, (val) => {
  localStorage.setItem('config_ml_subtab', val);
});

// Auto-tune elapsed time tracking
const autoTuneStartTime = ref(0);
const autoTuneElapsed = ref('');
let autoTuneElapsedTimer: ReturnType<typeof setInterval> | null = null;

watch(autoTuneInProgress, (running) => {
  if (running) {
    autoTuneStartTime.value = Date.now();
    autoTuneElapsed.value = '0s';
    autoTuneElapsedTimer = setInterval(() => {
      const sec = Math.floor((Date.now() - autoTuneStartTime.value) / 1000);
      autoTuneElapsed.value = sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m${sec % 60}s`;
    }, 1000);
  } else {
    if (autoTuneElapsedTimer) { clearInterval(autoTuneElapsedTimer); autoTuneElapsedTimer = null; }
  }
});

// Computed: if auto-tune just completed (has result and was running)
const autoTuneJustCompleted = computed(() =>
  !autoTuneInProgress.value && autoTuneResponse.value && autoTuneLoading.value === false
);

onMounted(() => {
  wsActive.value = true;
  wsConnect();
});
</script>

<template>
        <a-tabs
          v-model:activeKey="mlSubTabKey"
          size="small"
          type="card"
          style="margin: 8px 0 16px"
        >
          <a-tab-pane key="status" tab="状况"></a-tab-pane>
          <a-tab-pane key="params" tab="参数"></a-tab-pane>
          <a-tab-pane key="model" tab="模型管理"></a-tab-pane>
          <a-tab-pane key="llm" tab="LLM 打分"></a-tab-pane>
          <a-tab-pane key="training" tab="训练集管理"></a-tab-pane>
        </a-tabs>
        <a-row :gutter="[24, 24]">
          <!-- Row 1: Model Status + Training Controls -->
          <a-col v-if="mlSubTabKey === 'status'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Model Status</span>
              </template>
              <template #extra>
                <a-space>
                  <a-button size="small" @click="mlSubTabKey = 'training'">
                    <ImportOutlined /> 导入
                  </a-button>
                  <a-button size="small" @click="exportTrainingDataset">
                    <ExportOutlined /> 下载
                  </a-button>
                  <a-button size="small" type="link" @click="fetchMLStatus">
                    <ReloadOutlined />
                  </a-button>
                </a-space>
              </template>
              <a-row :gutter="[12, 12]">
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="ML Engine" :value="mlEnabled ? 'Active' : 'Inactive'" :value-style="{ color: mlEnabled ? '#3f8600' : '#cf1322', fontSize: '18px' }" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Model Loaded" :value="mlStatus.model_loaded ? 'Yes' : 'No'" :value-style="{ color: mlStatus.model_loaded ? '#3f8600' : '#d48806', fontSize: '18px' }" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Model Type" :value="modelType === 'random_forest' ? 'RF' : modelType === 'knn' ? 'KNN' : modelType === 'logistic' ? 'LR' : modelType" :value-style="{ color: '#1890ff', fontSize: '18px' }" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Trees" :value="mlStatus.num_trees || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Train Accuracy" :value="mlStatus.train_accuracy ? (mlStatus.train_accuracy * 100).toFixed(1) : '—'" :suffix="mlStatus.train_accuracy ? '%' : ''" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Acc" :value="mlStatus.validation_accuracy ? (mlStatus.validation_accuracy * 100).toFixed(1) : '—'" :suffix="mlStatus.validation_accuracy ? '%' : ''" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Train Samples" :value="mlStatus.train_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Samples" :value="mlStatus.validation_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Labeled Samples" :value="mlStatus.num_labeled_samples || 0" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Validation Split" :value="((mlStatus.validation_split_ratio || 0) * 100).toFixed(0)" suffix="%" />
                  </a-card>
                </a-col>
                <a-col :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <a-statistic title="Last Trained" :value="mlStatus.last_trained || 'Never'" :value-style="{ fontSize: '14px' }" />
                  </a-card>
                </a-col>
                <a-col v-if="mlStatus.training_in_progress" :xs="12" :sm="8" :md="6">
                  <a-card size="small" hoverable style="text-align: center; aspect-ratio: 1; display: flex; flex-direction: column; justify-content: center; align-items: center">
                    <div style="font-weight: 600; margin-bottom: 8px; color: #999">Training</div>
                    <a-progress type="circle" :percent="Math.round((mlStatus.training_progress || 0) * 100)" :size="64" />
                  </a-card>
                </a-col>
              </a-row>
              <div v-if="mlStatus.model_path" style="margin-top: 12px; font-size: 12px; color: #999; word-break: break-all">
                Model Path: {{ mlStatus.model_path }}
              </div>
            </a-card>
          </a-col>
          <a-col v-if="mlSubTabKey === 'model'" :xs="24" :md="12">
            <a-card title="Training Controls" size="small">
              <a-space direction="vertical" style="width: 100%">
                <!-- Training resource status -->
                <div v-if="mlStatus.training_in_progress" style="background: #f6ffed; border: 1px solid #b7eb8f; border-radius: 6px; padding: 8px 12px; font-size: 12px;">
                  <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 4px;">
                    <span>
                      <ReloadOutlined spin style="margin-right: 4px; color: #52c41a;" />
                      <b>训练中</b> {{ Math.round((mlStatus.training_progress || 0) * 100) }}%
                    </span>
                    <span v-if="cudaAvailable && cudaMemTotalMB > 0" style="color: #666;">
                      GPU: {{ cudaMemUsedMB }} / {{ cudaMemTotalMB }} MB
                    </span>
                  </div>
                  <a-progress :percent="Math.round((mlStatus.training_progress || 0) * 100)" :show-info="false" style="margin-top: 4px;" />
                </div>
                <!-- Train / Cancel buttons -->
                <div style="display: flex; gap: 8px;">
                  <a-button type="primary" @click="trainWithParams" :loading="trainingModel" :disabled="mlStatus.training_in_progress" style="flex: 1">
                    <ThunderboltOutlined /> Train Model Now
                  </a-button>
                  <a-popconfirm
                    v-if="mlStatus.training_in_progress"
                    title="确定要中止当前训练吗？"
                    @confirm="cancelTraining"
                    ok-text="中止训练"
                    cancel-text="继续等待"
                    ok-type="danger"
                  >
                    <a-button danger :loading="cancellingTraining">
                      <StopOutlined /> 中止
                    </a-button>
                  </a-popconfirm>
                </div>
                <a-divider style="margin: 8px 0">Batch Feedback</a-divider>
                <a-input-group compact>
                  <a-input v-model:value="feedbackComm" placeholder="Command (e.g. rm)" style="width: 40%" />
                  <a-select v-model:value="feedbackAction" style="width: 30%">
                    <a-select-option value="accepted">Accepted (ALLOW)</a-select-option>
                    <a-select-option value="rejected">Rejected (BLOCK)</a-select-option>
                    <a-select-option value="alerted">Alerted (ALERT)</a-select-option>
                  </a-select>
                  <a-button type="dashed" @click="submitFeedback" style="width: 30%">Submit</a-button>
                </a-input-group>
              </a-space>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'llm'" :xs="24" :md="12">
            <a-card title="LLM Scoring" size="small">
              <template #extra>
                <a-space>
                  <a-tag v-if="llmSaveStatus === 'saving'" color="processing"><ReloadOutlined spin /> Saving...</a-tag>
                  <a-tag v-else-if="llmSaveStatus === 'saved'" color="success"><CheckCircleOutlined /> Saved</a-tag>
                  <a-tag v-else-if="llmSaveStatus === 'error'" color="error"><ExclamationCircleOutlined /> Save Failed</a-tag>
                  <a-tag color="purple">OpenAI-compatible API</a-tag>
                </a-space>
              </template>
              <a-space direction="vertical" style="width: 100%">
                <a-alert
                  type="info"
                  show-icon
                  message="修改自动保存到浏览器本地并同步后端。API Key 留空则保留后端已保存的密钥。"
                />
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24">
                    <a-space align="center" wrap>
                      <a-switch v-model:checked="llmScoringConfig.enabled" />
                      <span>启用 LLM 打分</span>
                      <a-tag :color="llmApiKeyStatus.color">{{ llmApiKeyStatus.text }}</a-tag>
                    </a-space>
                  </a-col>
                  <a-col :xs="24">
                    <div style="font-weight: 600; margin-bottom: 6px">API Base URL</div>
                    <a-input v-model:value="llmScoringConfig.baseUrl" placeholder="https://api.openai.com" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <div style="font-weight: 600; margin-bottom: 6px">Model</div>
                    <a-input v-model:value="llmScoringConfig.model" placeholder="gpt-4o-mini / gpt-5 / 自建兼容模型" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <div style="font-weight: 600; margin-bottom: 6px">API Key</div>
                    <a-input-password v-model:value="llmScoringConfig.apiKey" placeholder="留空则保留后端现有密钥" allow-clear />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Timeout (s)</div>
                    <a-input-number v-model:value="llmScoringConfig.timeoutSeconds" :min="5" :max="300" :step="5" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Temperature</div>
                    <a-input-number v-model:value="llmScoringConfig.temperature" :min="0" :max="2" :step="0.1" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Max Tokens</div>
                    <a-input-number v-model:value="llmScoringConfig.maxTokens" :min="32" :max="4096" :step="32" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24">
                    <div style="font-weight: 600; margin-bottom: 6px">System Prompt</div>
                    <a-textarea
                      v-model:value="llmScoringConfig.systemPrompt"
                      :auto-size="{ minRows: 3, maxRows: 8 }"
                      placeholder="你是安全行为分析器，只返回严格 JSON ..."
                    />
                  </a-col>
                </a-row>
                <div style="display: flex; align-items: center; gap: 8px;">
                  <a-button size="small" type="primary" @click="saveLLMConfigNow"
                    :loading="llmSaveStatus === 'saving'">
                    <CheckCircleOutlined /> 保存 LLM 配置
                  </a-button>
                  <span v-if="llmSaveStatus === 'saved'" style="color: #52c41a; font-size: 12px;">已保存到后端</span>
                  <span v-else-if="llmSaveStatus === 'error'" style="color: #ff4d4f; font-size: 12px;">保存失败，请重试</span>
                  <span v-else-if="llmSaveStatus === 'idle'" style="color: #999; font-size: 12px;">修改后自动保存</span>
                </div>
                <a-divider style="margin: 8px 0">批量打分 / 后训练复核</a-divider>
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">数据源</div>
                    <a-select v-model:value="llmBatchConfig.source" style="width: 100%">
                      <a-select-option value="training">生成数据（生成新样本并打分）</a-select-option>
                      <a-select-option value="validation">已有数据（对现有样本重打分）</a-select-option>
                    </a-select>
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">Limit</div>
                    <a-input-number v-model:value="llmBatchConfig.limit" :min="1" :max="5000" :step="1" style="width: 100%" />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">只看未标注</div>
                    <a-switch v-model:checked="llmBatchConfig.onlyUnlabeled" />
                  </a-col>
                  <a-col :xs="24">
                    <a-space align="center" wrap>
                      <a-switch v-model:checked="llmBatchConfig.applyLabels" :disabled="!llmBatchCanApplyLabels" />
                      <span>把 LLM 结果回写为训练标签</span>
                      <a-tag v-if="!llmBatchCanApplyLabels" color="default">仅训练集可回写</a-tag>
                    </a-space>
                  </a-col>
                </a-row>
                <a-button type="primary" @click="runLLMBatchScore" :loading="llmBatchLoading" block>
                  <ThunderboltOutlined /> 开始批量打分
                </a-button>

                <div v-if="llmBatchResponse" style="display: flex; flex-direction: column; gap: 12px">
                  <a-alert
                    type="success"
                    show-icon
                    :message="`已处理 ${llmBatchResponse.scored}/${llmBatchResponse.total} 条，平均风险 ${(llmBatchResponse.averageRiskScore ?? 0).toFixed(1)}，一致性 ${(llmBatchResponse.agreement * 100).toFixed(0)}%`"
                    :description="llmBatchResponse.review?.validationSplitRatio !== undefined ? `验证集切分比例 ${(llmBatchResponse.review.validationSplitRatio * 100).toFixed(0)}%` : 'LLM 批量复核已完成。'"
                  />
                  <a-space wrap>
                    <a-tag color="blue">source: {{ llmBatchResponse.source }}</a-tag>
                    <a-tag color="geekblue">model: {{ llmBatchResponse.model }}</a-tag>
                    <a-tag color="green">applied: {{ llmBatchResponse.applied }}</a-tag>
                    <a-tag color="orange">skipped: {{ llmBatchResponse.skipped }}</a-tag>
                  </a-space>
                  <a-table
                    :dataSource="llmBatchResponse.entries"
                    :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }"
                    size="small"
                    :rowKey="llmBatchRowKey"
                    :scroll="{ x: 980 }"
                  >
                    <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
                      <template #default="{ record }">
                        <code>{{ maskSensitiveData(record.commandLine) }}</code>
                      </template>
                    </a-table-column>
                    <a-table-column title="Label" dataIndex="currentLabel" :width="100">
                      <template #default="{ record }">
                        <a-tag :color="getLabelColor(record.currentLabel || '-')">{{ record.currentLabel || '—' }}</a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Risk" dataIndex="riskScore" :width="90">
                      <template #default="{ record }">
                        {{ record.riskScore?.toFixed(0) }}
                      </template>
                    </a-table-column>
                    <a-table-column title="Action" dataIndex="recommendedAction" :width="110">
                      <template #default="{ record }">
                        <a-tag :color="record.recommendedAction === 'BLOCK' ? 'red' : record.recommendedAction === 'ALERT' ? 'orange' : record.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                          {{ record.recommendedAction }}
                        </a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Confidence" dataIndex="confidence" :width="110">
                      <template #default="{ record }">
                        {{ record.confidence ? (record.confidence * 100).toFixed(0) + '%' : '—' }}
                      </template>
                    </a-table-column>
                    <a-table-column title="State" dataIndex="applied" :width="100">
                      <template #default="{ record }">
                        <a-tag v-if="record.error" color="red">Error</a-tag>
                        <a-tag v-else-if="record.applied" color="green">Applied</a-tag>
                        <a-tag v-else color="blue">Scored</a-tag>
                      </template>
                    </a-table-column>
                    <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
                      <template #default="{ record }">
                        <span>{{ record.reasoning || record.error || '—' }}</span>
                      </template>
                    </a-table-column>
                  </a-table>
                </div>
              </a-space>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'llm'" :xs="24">
            <a-card title="LLM 生产训练集" size="small">
              <template #extra>
                <a-tag color="green">来源：/config/ml/training</a-tag>
              </template>
              <a-space direction="vertical" style="width: 100%">
                <a-alert
                  type="info"
                  show-icon
                  message="直接从当前训练存储生成 OpenAI chat JSONL，不抓网页 HTML，也不会把未标注样本洗进训练集。默认只保留已标注样本，并按 commandLine + label 去重；如确实需要噪声样本，可手动打开启发式标签。"
                />
                <a-row :gutter="[12, 12]">
                  <a-col :xs="24" :md="8">
                    <div style="font-weight: 600; margin-bottom: 6px">样本上限</div>
                    <a-input-number
                      v-model:value="llmProductionDatasetLimit"
                      :min="1"
                      :max="5000"
                      :step="1"
                      style="width: 100%"
                    />
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-space direction="vertical" style="width: 100%">
                      <a-space align="center" wrap>
                        <a-switch v-model:checked="llmProductionDeduplicate" />
                        <span>命令 + 标签去重</span>
                      </a-space>
                      <a-space align="center" wrap>
                        <a-switch v-model:checked="llmProductionAllowHeuristic" />
                        <span>允许启发式 / LLM 自动标签</span>
                      </a-space>
                    </a-space>
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-space direction="vertical" style="width: 100%">
                      <a-button type="primary" @click="fetchLLMProductionDataset" :loading="llmProductionLoading" block>
                        <ReloadOutlined /> 拉取当前训练集
                      </a-button>
                      <a-button @click="exportLLMProductionDataset" :disabled="llmProductionPreview.length === 0 || llmProductionLoading" block>
                        <DownloadOutlined /> 导出 JSONL
                      </a-button>
                    </a-space>
                  </a-col>
                </a-row>

                <a-space wrap>
                  <a-tag v-if="llmProductionMeta" color="blue">source: {{ llmProductionMeta.source }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="cyan">format: {{ llmProductionMeta.format }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="geekblue">total: {{ llmProductionMeta.total }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="green">included: {{ llmProductionMeta.included }}</a-tag>
                  <a-tag v-if="llmProductionMeta" color="default">skip unlabeled: {{ llmProductionMeta.skippedUnlabeled }}</a-tag>
                  <a-tag v-if="llmProductionMeta && llmProductionMeta.skippedHeuristic > 0" color="orange">skip noisy: {{ llmProductionMeta.skippedHeuristic }}</a-tag>
                  <a-tag v-if="llmProductionMeta && llmProductionMeta.skippedDuplicates > 0" color="gold">skip dup: {{ llmProductionMeta.skippedDuplicates }}</a-tag>
                  <a-tag v-if="llmProductionMeta?.truncated" color="red">truncated</a-tag>
                </a-space>

                <a-alert
                  v-if="llmProductionMeta"
                  type="success"
                  show-icon
                  :message="`已生成 ${llmProductionMeta.included} 条 LLM 生产训练样本`"
                  :description="`导出 JSONL 每行仅包含 messages，适合 chat fine-tuning；系统提示词来自当前 LLM 配置：${llmProductionMeta.systemPrompt}`"
                />
                <a-alert
                  v-else
                  type="warning"
                  show-icon
                  message="点击“拉取当前训练集”后，会直接从训练存储生成可训练的 chat JSONL 预览。"
                />

                <a-table
                  v-if="llmProductionPreview.length > 0"
                  :dataSource="llmProductionPreview"
                  :pagination="{ pageSize: 5, showSizeChanger: true, pageSizeOptions: ['5', '10', '20'] }"
                  :scroll="{ x: 1280 }"
                  size="small"
                  rowKey="index"
                >
                  <a-table-column title="#" dataIndex="index" :width="70" />
                  <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                    <template #default="{ record }">
                      <code>{{ maskSensitiveData(record.commandLine) }}</code>
                    </template>
                  </a-table-column>
                  <a-table-column title="Label" dataIndex="label" :width="100">
                    <template #default="{ record }">
                      <a-tag :color="getLabelColor(record.label)">{{ record.label }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="Risk" dataIndex="targetRiskScore" :width="90">
                    <template #default="{ record }">
                      {{ record.targetRiskScore?.toFixed(0) }}
                    </template>
                  </a-table-column>
                  <a-table-column title="Confidence" dataIndex="targetConfidence" :width="110">
                    <template #default="{ record }">
                      {{ record.targetConfidence ? (record.targetConfidence * 100).toFixed(0) + '%' : '—' }}
                    </template>
                  </a-table-column>
                  <a-table-column title="Source" dataIndex="userLabel" :width="140">
                    <template #default="{ record }">
                      <a-tag color="purple">{{ record.userLabel || '—' }}</a-tag>
                    </template>
                  </a-table-column>
                  <a-table-column title="Signals" dataIndex="signals" :width="220">
                    <template #default="{ record }">
                      <a-space wrap size="small">
                        <a-tag v-for="(signal, i) in record.signals || []" :key="i" color="purple" size="small">
                          {{ signal }}
                        </a-tag>
                      </a-space>
                    </template>
                  </a-table-column>
                  <a-table-column title="Reasoning" dataIndex="reasoning" ellipsis>
                    <template #default="{ record }">
                      <span>{{ record.reasoning || '—' }}</span>
                    </template>
                  </a-table-column>
                </a-table>

                <a-card v-if="llmProductionPreview.length > 0" size="small" title="首条样本 JSON 预览">
                  <a-descriptions :column="2" size="small" bordered>
                    <a-descriptions-item label="Command">{{ maskSensitiveData(llmProductionPreview[0].commandLine) }}</a-descriptions-item>
                    <a-descriptions-item label="Label">
                      <a-tag :color="getLabelColor(llmProductionPreview[0].label)">{{ llmProductionPreview[0].label }}</a-tag>
                    </a-descriptions-item>
                    <a-descriptions-item label="Prompt" :span="2">
                      <a-textarea
                        :value="maskSensitiveData(llmProductionPreview[0].prompt)"
                        :auto-size="{ minRows: 4, maxRows: 8 }"
                        readonly
                      />
                    </a-descriptions-item>
                    <a-descriptions-item label="Completion" :span="2">
                      <a-textarea
                        :value="maskSensitiveData(llmProductionPreview[0].completion)"
                        :auto-size="{ minRows: 4, maxRows: 8 }"
                        readonly
                      />
                    </a-descriptions-item>
                  </a-descriptions>
                </a-card>
              </a-space>
            </a-card>
          </a-col>

          <!-- Row: Model Logs (always visible) -->
          <a-col v-if="mlSubTabKey === 'status'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>模型日志</span>
                <a-tag v-if="mlStatus.training_in_progress" color="processing" style="margin-left: 8px">
                  <ReloadOutlined spin style="margin-right: 2px;" />训练中...
                </a-tag>
                <a-tag v-else-if="trainingLogs.length > 0" color="green" style="margin-left: 8px">
                  {{ trainingLogs.length }} 条
                </a-tag>
                <a-tag v-else style="margin-left: 8px">等待训练</a-tag>
              </template>
              <!-- Progress bar (only when training) -->
              <div v-if="mlStatus.training_in_progress" style="margin-bottom: 12px;">
                <div style="display: flex; justify-content: space-between; font-size: 12px; color: #888; margin-bottom: 4px;">
                  <span>训练进度</span>
                  <span>{{ Math.round((mlStatus.training_progress || 0) * 100) }}%</span>
                </div>
                <a-progress
                  :percent="Math.round((mlStatus.training_progress || 0) * 100)"
                  :status="'active'"
                />
              </div>
              <!-- Log viewer -->
              <div
                ref="logContainer"
                style="background: #1e1e1e; color: #d4d4d4; border-radius: 6px; padding: 10px 14px; max-height: 320px; overflow-y: auto; font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace; font-size: 12px; line-height: 1.6"
              >
                <template v-if="trainingLogs.length > 0">
                  <div v-for="(line, i) in trainingLogs" :key="i" style="white-space: pre-wrap; word-break: break-all">
                    <span style="color: #6a9955">{{ line.time }}</span>
                    <span v-if="line.message.includes('ERROR') || line.message.startsWith('ERROR')" style="color: #f44747">{{ ' ' + line.message }}</span>
                    <span v-else-if="line.message.startsWith('═══')" style="color: #569cd6; font-weight: bold">{{ ' ' + line.message }}</span>
                    <span v-else-if="line.message.includes('完成') || line.message.includes('complete') || line.message.includes('accuracy')" style="color: #89d185">{{ ' ' + line.message }}</span>
                    <span v-else style="color: #d4d4d4">{{ ' ' + line.message }}</span>
                  </div>
                </template>
                <div v-else style="color: #888; text-align: center; padding: 20px 0;">
                  {{ mlStatus.training_in_progress ? '等待训练开始...' : '暂无日志，点击训练按钮开始模型的训练和评估' }}
                </div>
              </div>
            </a-card>
          </a-col>

          <!-- Row: Training Curve Visualization -->
          <a-col v-if="mlSubTabKey === 'status' && trainingHistory.length > 0" :xs="24">
            <a-card title="Training History" size="small">
              <template #extra>
                <a-tag color="blue">{{ trainingHistory.length }} runs</a-tag>
              </template>
              <Suspense>
                <VueApexCharts
                  type="line"
                  height="280"
                  :options="trainingChartOptions"
                  :series="trainingChartSeries"
                />
                <template #fallback>
                  <div style="text-align: center; padding: 40px; color: #999">Loading chart...</div>
                </template>
              </Suspense>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'status' && mlStatus.llm_review" :xs="24">
            <a-card title="LLM Post-Training Review" size="small">
              <template #extra>
                <a-tag color="purple">OpenAI-style batch review</a-tag>
              </template>
              <a-descriptions :column="3" size="small" bordered>
                <a-descriptions-item label="Source">{{ mlStatus.llm_review?.source || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Model">{{ mlStatus.llm_review?.model || llmScoringConfig.model || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Scored Samples">{{ mlStatus.llm_review?.scoredSamples ?? 0 }}</a-descriptions-item>
                <a-descriptions-item label="Average Risk">
                  {{ mlStatus.llm_review ? mlStatus.llm_review.averageRiskScore.toFixed(1) : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Agreement">
                  {{ mlStatus.llm_review ? (mlStatus.llm_review.agreement * 100).toFixed(0) + '%' : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Validation Split">
                  {{ mlStatus.llm_review?.validationSplitRatio !== undefined ? (mlStatus.llm_review.validationSplitRatio * 100).toFixed(0) + '%' : '—' }}
                </a-descriptions-item>
                <a-descriptions-item label="Reviewed At" :span="3">
                  {{ mlStatus.llm_review?.reviewedAt ? new Date(mlStatus.llm_review.reviewedAt).toLocaleString() : '—' }}
                </a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>

          <!-- Row: Classic OS Security Datasets -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span><BookOutlined /> 经典 OS 安全数据集</span>
                <a-tag color="green" style="margin-left: 8px">支持一键导入</a-tag>
              </template>
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="有下载链接的数据集可一键导入；无下载链接的会跳转官方页面，下载后用“导入本地文件”上传。导入器支持 zip, gz, tar, tgz, bz2 等归档及 JSON, JSONL, CSV, TSV, 纯文本。"
              />

              <a-list :data-source="classicSecurityDatasetPresets" :split="false" size="small">
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-card size="small" style="width: 100%">
                      <a-space direction="vertical" style="width: 100%">
                        <div
                          style="
                            display: flex;
                            justify-content: space-between;
                            gap: 12px;
                            align-items: flex-start;
                            flex-wrap: wrap;
                          "
                        >
                          <div>
                            <div style="font-weight: 600">{{ item.name }}</div>
                            <div style="color: #666; font-size: 12px">
                              {{ item.note }}
                            </div>
                          </div>
                          <a-space wrap>
                            <a-tag color="blue">{{ item.family }}</a-tag>
                            <a-tag color="geekblue">{{ item.platform }}</a-tag>
                          </a-space>
                        </div>
                        <a-space wrap>
                          <a-button
                            size="small"
                            type="primary"
                            :loading="importingClassicDataset"
                            @click="importClassicDataset(item)"
                          >
                            <ImportOutlined /> {{ item.downloadUrl ? '一键导入' : '前往下载' }}
                          </a-button>
                          <a-button size="small" @click="openClassicSecurityDatasetPage(item)">
                            <GlobalOutlined /> 打开官网
                          </a-button>
                          <a-button size="small" @click="copyClassicSecurityDatasetPage(item)">
                            <CopyOutlined /> 复制链接
                          </a-button>
                        </a-space>
                      </a-space>
                    </a-card>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>
          </a-col>

          <!-- Row: Internet Dataset Import -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span><GlobalOutlined /> 互联网数据集拉取</span>
                <a-tag color="blue" style="margin-left: 8px">HTTP/HTTPS JSON、JSONL、CSV、TSV、文本</a-tag>
              </template>
              <template #extra>
                  <a-space>
                  <input
                    type="file"
                    ref="trainingDatasetImportInput"
                    @change="importTrainingDatasetFromFile"
                    style="display: none"
                    accept=".json,.jsonl,.ndjson,.csv,.tsv,.txt,.log,.zip,.gz,.tgz,.tar,.bz2,.tbz,.tbz2,.txz"
                  />
                  <a-button size="small" @click="fetchRemoteDatasetPreview" :loading="loadingRemoteDataset">
                    <ReloadOutlined /> 拉取预览
                  </a-button>
                  <a-button size="small" @click="openTrainingDatasetImportPicker" :loading="importingRemoteDataset">
                    <FileOutlined /> 导入本地文件
                  </a-button>
                  <a-button size="small" type="primary" @click="importRemoteDataset" :loading="importingRemoteDataset">
                    <ImportOutlined /> 导入训练集
                  </a-button>
                </a-space>
              </template>

              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="后端只接受可直接 GET 到的原始数据文件；如果地址返回的是 HTML 介绍页、下载页或归档页，会直接报错。也可以用“导入本地文件”上传 JSON, JSONL, CSV, TSV, 纯文本或常见压缩包，后端会自动尝试解压 zip, gz, tar, tar.gz, tgz, bz2 等归档。"
              />

              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="10">
                  <div style="display: flex; flex-direction: column; gap: 12px">
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">数据集 URL</div>
                      <a-input
                        v-model:value="remoteDatasetUrl"
                        placeholder="https://example.com/dataset.jsonl"
                        allow-clear
                      />
                    </div>
                    <div style="display: flex; gap: 12px; flex-wrap: wrap">
                      <div style="flex: 1; min-width: 180px">
                        <div style="font-weight: 600; margin-bottom: 6px">格式</div>
                        <a-select v-model:value="remoteDatasetFormat" style="width: 100%">
                          <a-select-option value="auto">自动识别</a-select-option>
                          <a-select-option value="json">JSON</a-select-option>
                          <a-select-option value="jsonl">JSONL / NDJSON</a-select-option>
                          <a-select-option value="csv">CSV</a-select-option>
                          <a-select-option value="tsv">TSV</a-select-option>
                          <a-select-option value="text">纯文本命令行</a-select-option>
                        </a-select>
                      </div>
                      <div style="flex: 1; min-width: 180px">
                        <div style="font-weight: 600; margin-bottom: 6px">标签模式</div>
                        <a-select v-model:value="remoteDatasetLabelMode" style="width: 100%">
                          <a-select-option value="preserve">保留原始标签</a-select-option>
                          <a-select-option value="unlabeled">统一未标注</a-select-option>
                          <a-select-option value="heuristic">按规则自动标注</a-select-option>
                        </a-select>
                      </div>
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">拉取条数</div>
                      <a-input-number
                        v-model:value="remoteDatasetLimit"
                        :min="1"
                        :max="5000"
                        :step="1"
                        style="width: 100%"
                      />
                    </div>
                    <a-typography-text type="secondary">
                      支持公开数据集、实验室内网数据集或你自己的样本仓库，只要 URL 可直接 GET 访问即可。
                    </a-typography-text>
                  </div>
                </a-col>
                <a-col :xs="24" :md="14">
                  <div style="display: flex; flex-direction: column; gap: 10px">
                    <a-space wrap>
                      <a-tag v-if="remoteDatasetMeta" color="blue">source: {{ remoteDatasetMeta.source }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="cyan">format: {{ remoteDatasetMeta.format }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="geekblue">type: {{ remoteDatasetMeta.contentType || 'unknown' }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="purple">total: {{ remoteDatasetMeta.total }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta?.truncated" color="orange">truncated</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="green">imported: {{ remoteDatasetMeta.imported ?? 0 }}</a-tag>
                      <a-tag v-if="remoteDatasetMeta" color="gold">skipped: {{ remoteDatasetMeta.skipped ?? 0 }}</a-tag>
                    </a-space>
                    <a-alert
                      v-if="remoteDatasetMeta"
                      type="success"
                      show-icon
                      :message="`已拉取 ${remoteDatasetMeta.total} 条，当前预览显示 ${remoteDatasetPreview.length} 条`"
                      :description="remoteDatasetMeta.truncated ? '列表已按 Limit 截断，导入时也会使用同样的条数上限。' : '列表展示的是当前请求返回的全部可见数据。'"
                    />
                    <a-alert
                      v-else
                      type="warning"
                      show-icon
                      message="输入数据集 URL 后点击“拉取预览”，即可先查看格式识别和样本解析情况。"
                    />
                    <a-table
                      :dataSource="remoteDatasetPreview"
                      :pagination="{ pageSize: 6, showSizeChanger: true, pageSizeOptions: ['6', '10', '20'] }"
                      :scroll="{ x: 980 }"
                      size="small"
                      rowKey="row"
                    >
                      <a-table-column title="#" dataIndex="row" :width="60" />
                      <a-table-column title="Command" dataIndex="commandLine" :width="280" ellipsis>
                        <template #default="{ record }">
                          <code>{{ maskSensitiveData(record.commandLine) }}</code>
                        </template>
                      </a-table-column>
                      <a-table-column title="Label" dataIndex="label" :width="100">
                        <template #default="{ record }">
                          <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column title="Category" dataIndex="category" :width="120">
                        <template #default="{ record }">
                          <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                          <span v-else style="color: #999">—</span>
                        </template>
                      </a-table-column>
                      <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                        <template #default="{ record }">
                          {{ record.anomalyScore?.toFixed(2) }}
                        </template>
                      </a-table-column>
                      <a-table-column title="State" dataIndex="duplicate" :width="100">
                        <template #default="{ record }">
                          <a-tag :color="record.duplicate ? 'default' : 'green'" size="small">
                            {{ record.duplicate ? '已存在' : '可导入' }}
                          </a-tag>
                        </template>
                      </a-table-column>
                      <a-table-column title="Time" dataIndex="timestamp" :width="180">
                        <template #default="{ record }">
                          <span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span>
                        </template>
                      </a-table-column>
                    </a-table>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row: Pull existing command data -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Existing Command Data</span>
                <a-tag color="cyan" style="margin-left: 8px">拉取已有 wrapper / hook 事件</a-tag>
              </template>
              <template #extra>
                <a-space wrap>
                  <span style="font-size: 12px; color: #666">Limit</span>
                  <a-input-number v-model:value="existingDataLimit" :min="10" :max="5000" size="small" style="width: 100px" />
                  <a-select v-model:value="existingLabelMode" size="small" style="width: 150px">
                    <a-select-option value="unlabeled">导入为未标注</a-select-option>
                    <a-select-option value="heuristic">按安全判断标注</a-select-option>
                  </a-select>
                  <a-button size="small" @click="fetchExistingCommandData" :loading="loadingExistingData">
                    <ReloadOutlined /> 拉取已有数据
                  </a-button>
                  <a-button
                    size="small"
                    type="primary"
                    @click="importExistingCommandData"
                    :loading="importingExistingData"
                    :disabled="importableExistingCount <= 0"
                  >
                    <ImportOutlined /> 导入 {{ importableExistingCount }}
                  </a-button>
                </a-space>
              </template>
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                message="从 /events/recent 读取历史 wrapper_intercept / native_hook 命令。默认导入为未标注样本；选择“按安全判断标注”会用当前规则/ML/网络审计结果自动给出 ALLOW/ALERT/BLOCK 标签。"
              />
              <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 8px; flex-wrap: wrap">
                <a-tag v-if="existingDataSource" color="blue">source: {{ existingDataSource }}</a-tag>
                <a-tag color="purple">{{ existingCommandCandidates.length }} pulled</a-tag>
                <a-tag color="default">{{ existingDuplicateCount }} duplicates</a-tag>
              </div>
              <a-table
                :dataSource="existingCommandCandidates"
                :pagination="{ pageSize: 8, showSizeChanger: true, pageSizeOptions: ['8','15','30'] }"
                :scroll="{ x: 900 }"
                size="small"
                rowKey="commandLine"
              >
                <a-table-column title="Command" dataIndex="commandLine" :width="300" ellipsis>
                  <template #default="{ record }">
                    <code>{{ maskSensitiveData(record.commandLine) }}</code>
                  </template>
                </a-table-column>
                <a-table-column title="Event" dataIndex="eventType" :width="120">
                  <template #default="{ record }">
                    <a-tag size="small" color="geekblue">{{ record.eventType }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Category" dataIndex="category" :width="120">
                  <template #default="{ record }">
                    <a-tag v-if="record.category" :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                    <span v-else style="color: #999">—</span>
                  </template>
                </a-table-column>
                <a-table-column title="Time" dataIndex="timestamp" :width="180">
                  <template #default="{ record }">
                    <span style="font-size: 12px; color: #666">{{ record.timestamp ? new Date(record.timestamp).toLocaleString() : '—' }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="State" dataIndex="duplicate" :width="100">
                  <template #default="{ record }">
                    <a-tag :color="record.duplicate ? 'default' : 'green'" size="small">
                      {{ record.duplicate ? '已存在' : '可导入' }}
                    </a-tag>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>

          <!-- Row: Sample Data Browser -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Training Data Browser</span>
                <a-tag color="purple" style="margin-left: 8px">{{ filteredSamples.length }} / {{ allSamples.length }}</a-tag>
              </template>
              <template #extra>
                <a-space wrap>
                  <a-button 
                    size="small" 
                    @click="dataMaskEnabled = !dataMaskEnabled"
                    :type="dataMaskEnabled ? 'primary' : 'default'"
                  >
                    <component :is="dataMaskEnabled ? EyeInvisibleOutlined : EyeOutlined" />
                    {{ dataMaskEnabled ? '脱敏' : '明文' }}
                  </a-button>
                  <a-button size="small" @click="exportTrainingDataset">
                    <ExportOutlined /> 导出训练集
                  </a-button>
                  <a-popconfirm title="确定要清空当前训练集吗？" @confirm="clearTrainingDataset">
                    <a-button size="small" danger>
                      <DeleteOutlined /> 清空训练集
                    </a-button>
                  </a-popconfirm>
                  <a-input 
                    v-model:value="sampleSearchText" 
                    placeholder="搜索命令或参数..." 
                    size="small" 
                    style="width: 200px"
                    allow-clear
                  >
                    <template #prefix><SearchOutlined /></template>
                  </a-input>
                  <a-button size="small" @click="fetchAllSamples" :loading="loadingSamples">
                    <ReloadOutlined /> Refresh
                  </a-button>
                </a-space>
              </template>
              <a-table
                :dataSource="filteredSamples"
                :pagination="{ pageSize: sampleTablePageSize, showSizeChanger: true, pageSizeOptions: ['10','15','30','50'], showTotal: (t:number) => `${t} samples` }"
                :scroll="{ x: 1100 }"
                size="small"
                rowKey="index"
              >
                <a-table-column title="#" dataIndex="index" :width="50" />
                <a-table-column title="Command" dataIndex="commandLine" :width="240" ellipsis>
                  <template #default="{ record }">
                    <code>{{ maskSensitiveData(record.commandLine || [record.comm, ...(record.args || [])].filter(Boolean).join(' ')) }}</code>
                  </template>
                </a-table-column>
                <a-table-column title="Comm" dataIndex="comm" :width="100">
                  <template #default="{ record }">
                    <strong>{{ record.comm }}</strong>
                  </template>
                </a-table-column>
                <a-table-column title="Args" dataIndex="args" :width="200" ellipsis>
                  <template #default="{ record }">
                    <span style="font-size: 12px; color: #666">{{ maskSensitiveData((record.args || []).join(' ')) || '—' }}</span>
                  </template>
                </a-table-column>
                <a-table-column title="Category" dataIndex="category" :width="110">
                  <template #default="{ record }">
                    <a-tag :color="getCategoryColor(record.category)" size="small">{{ record.category }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="100">
                  <template #default="{ record }">
                    <a-input-number 
                      v-model:value="record.anomalyScore" 
                      :min="0" 
                      :max="1" 
                      :step="0.01" 
                      :precision="2"
                      size="small"
                      style="width: 70px"
                      @change="updateAnomaly(record.index, record.anomalyScore)"
                    />
                  </template>
                </a-table-column>
                <a-table-column title="Label" dataIndex="label" :width="90">
                  <template #default="{ record }">
                    <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                  </template>
                </a-table-column>
                <a-table-column title="Actions" :width="240">
                  <template #default="{ record }">
                    <a-space :size="4">
                      <a-button size="small" type="primary" ghost @click="labelSample(record.index, 'ALLOW')" :disabled="record.label === 'ALLOW'">ALLOW</a-button>
                      <a-button size="small" style="border-color: #faad14; color: #d48806" ghost @click="labelSample(record.index, 'ALERT')" :disabled="record.label === 'ALERT'">ALERT</a-button>
                      <a-button size="small" danger ghost @click="labelSample(record.index, 'BLOCK')" :disabled="record.label === 'BLOCK'">BLOCK</a-button>
                      <a-button size="small" danger type="text" @click="deleteSample(record.index)">
                        <DeleteOutlined />
                      </a-button>
                    </a-space>
                  </template>
                </a-table-column>
              </a-table>
            </a-card>
          </a-col>

          <!-- Row: Model Type Selector + Hyperparameters -->
          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Model Type</span>
                <a-tag :color="modelType === 'random_forest' ? 'green' : modelType === 'knn' ? 'blue' : 'purple'" style="margin-left: 8px;">
                  {{ modelType === 'random_forest' ? 'Random Forest' : modelType === 'knn' ? 'K-Nearest Neighbors' : modelType === 'logistic' ? 'Logistic Regression' : modelType }}
                </a-tag>
              </template>
              <a-radio-group v-model:value="modelType" button-style="solid" @change="saveMLThresholds">
                <a-radio-button value="random_forest">Random Forest</a-radio-button>
                <a-radio-button value="knn">KNN</a-radio-button>
                <a-radio-button value="logistic">Logistic Regression</a-radio-button>
              </a-radio-group>
              <a-space style="margin-top: 8px; display: flex; align-items: center;">
                <a-tag :color="cudaAvailable ? 'success' : 'warning'">
                  {{ cudaAvailable ? 'CUDA: ' + cudaInfo : 'CPU 训练 (未检测到 NVIDIA GPU)' }}
                </a-tag>
                <a-typography-text type="secondary">
                  切换模型类型后会自动保存，训练和推理将使用所选模型。
                </a-typography-text>
              </a-space>
            </a-card>
          </a-col>

          <!-- Hyperparameters (model-type-aware) -->
          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Model Hyperparameters" size="small">
              <template #extra>
                <a-tag color="geekblue">{{ modelType === 'random_forest' ? 'Random Forest' : modelType === 'knn' ? 'KNN' : 'Logistic' }} 参数</a-tag>
              </template>
              <!-- Random Forest params -->
              <a-row v-if="modelType === 'random_forest'" :gutter="[24, 16]">
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Num Trees (树的数量)</span>
                  <a-slider v-model:value="hyperParams.numTrees" :min="5" :max="200" :step="1" />
                  <a-input-number v-model:value="hyperParams.numTrees" :min="5" :max="200" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更多树 = 更高精度但更慢训练。推荐 31-101</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Max Depth (最大深度)</span>
                  <a-slider v-model:value="hyperParams.maxDepth" :min="3" :max="20" :step="1" />
                  <a-input-number v-model:value="hyperParams.maxDepth" :min="3" :max="20" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更深的树 = 更复杂决策边界。推荐 6-12</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Min Samples Leaf (叶节点最小样本)</span>
                  <a-slider v-model:value="hyperParams.minSamplesLeaf" :min="1" :max="50" :step="1" />
                  <a-input-number v-model:value="hyperParams.minSamplesLeaf" :min="1" :max="50" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">更大值防止过拟合。推荐 2-10</div>
                </a-col>
              </a-row>
              <!-- KNN params -->
              <a-row v-if="modelType === 'knn'" :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <span style="font-weight: 600">K (邻居数量)</span>
                  <a-slider v-model:value="hyperParams.numTrees" :min="1" :max="31" :step="2" />
                  <a-input-number v-model:value="hyperParams.numTrees" :min="1" :max="31" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">较小的 K 对噪声敏感，较大的 K 决策边界更平滑。推荐 3-11</div>
                </a-col>
                <a-col :xs="24" :md="12">
                  <span style="font-weight: 600">Distance (距离度量)</span>
                  <a-select v-model:value="hyperParams.maxDepth" style="width: 100%">
                    <a-select-option :value="8">Euclidean</a-select-option>
                    <a-select-option :value="16">Manhattan</a-select-option>
                  </a-select>
                  <div style="font-size: 11px; color: #999; margin-top: 8px;">Euclidean 适合连续特征，Manhattan 适合高维稀疏数据</div>
                </a-col>
              </a-row>
              <!-- Logistic Regression params -->
              <a-row v-if="modelType === 'logistic'" :gutter="[24, 16]">
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Learning Rate (学习率)</span>
                  <a-slider v-model:value="hyperParams.numTrees" :min="1" :max="100" :step="1" />
                  <a-input-number v-model:value="hyperParams.numTrees" :min="1" :max="100" size="small" style="width: 100%" :formatter="(v: number) => (v / 1000).toFixed(3)" :parser="(v: string) => parseFloat(v) * 1000" />
                  <div style="font-size: 11px; color: #999">较小值收敛更稳定。推荐 0.005-0.05</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Regularization (正则化)</span>
                  <a-select v-model:value="hyperParams.maxDepth" style="width: 100%">
                    <a-select-option :value="8">L2 (Ridge)</a-select-option>
                    <a-select-option :value="12">L1 (Lasso)</a-select-option>
                    <a-select-option :value="4">None</a-select-option>
                  </a-select>
                  <div style="font-size: 11px; color: #999; margin-top: 8px;">L2 防止大权重，L1 产生稀疏特征选择</div>
                </a-col>
                <a-col :xs="24" :md="8">
                  <span style="font-weight: 600">Max Iterations (最大迭代)</span>
                  <a-slider v-model:value="hyperParams.minSamplesLeaf" :min="100" :max="5000" :step="100" />
                  <a-input-number v-model:value="hyperParams.minSamplesLeaf" :min="100" :max="5000" size="small" style="width: 100%" />
                  <div style="font-size: 11px; color: #999">SGD 最大迭代数。推荐 500-2000</div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row 4: Manual Training Data -->
          <a-col v-if="mlSubTabKey === 'training'" :xs="24">
            <a-card size="small">
              <template #title>
                <span>Add Labeled Training Data</span>
                <a-tag color="blue" style="margin-left: 8px">手动添加标注样本</a-tag>
              </template>
              <a-row :gutter="[16, 16]">
                <!-- Quick presets -->
                <a-col :xs="24" :md="14">
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                    <div style="font-weight: 600">高危行为预设（点击即可添加已标注样本）</div>
                    <a-button size="small" type="link" @click="importAllHighRiskPresets">一键导入全部预设</a-button>
                  </div>
                  <a-space wrap>
                    <a-tag
                      v-for="(p, i) in highRiskPresets"
                      :key="i"
                      :color="p.label === 'BLOCK' ? 'red' : 'orange'"
                      style="cursor: pointer; padding: 4px 8px; font-size: 13px"
                      @click="addPresetSample(p)"
                    >
                      {{ p.comm }} {{ p.args ? p.args.slice(0, 30) + '…' : '' }}
                      <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
                    </a-tag>
                  </a-space>
                </a-col>

                <!-- Safety Net presets -->
                <a-col :xs="24" :md="10">
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
                    <div style="font-weight: 600">Claude Code Safety Net 预设</div>
                    <a-button size="small" type="link" @click="importAllSafetyNetPresets">一键导入全部</a-button>
                  </div>
                  <a-space wrap>
                    <a-tag
                      v-for="(p, i) in safetyNetHighRiskPresets"
                      :key="'sn'+i"
                      :color="p.label === 'BLOCK' ? 'red' : p.label === 'ALLOW' ? 'green' : 'orange'"
                      style="cursor: pointer; padding: 4px 8px; font-size: 12px"
                      @click="addPresetSample(p)"
                    >
                      <code>{{ p.comm }}</code> {{ p.args.slice(0, 35) }}{{ p.args.length > 35 ? '…' : '' }}
                      <span style="opacity: 0.7; margin-left: 4px">→ {{ p.desc }}</span>
                    </a-tag>
                  </a-space>
                </a-col>

                <!-- Manual form with explicit labeling -->
                <a-col :xs="24" :md="10">
                  <div style="font-weight: 600; margin-bottom: 8px">Step 1: 输入完整命令行</div>
                  <a-input 
                    v-model:value="sampleCommandLine" 
                    placeholder="完整命令 (支持管道: cat file.txt | grep error | wc -l)" 
                    size="small" 
                    style="margin-bottom: 10px"
                    @keyup.enter="submitManualSample"
                  />

                  <div style="font-weight: 600; margin-bottom: 8px">Step 2: 标注行为 <a-tag color="processing" size="small">选择标签</a-tag></div>
                  <div style="display: flex; gap: 8px; margin-bottom: 6px">
                    <a-radio-group v-model:value="sampleLabel" button-style="solid" size="small">
                      <a-radio-button value="BLOCK" style="border-color: #ff4d4f; color: #ff4d4f">
                        <StopOutlined /> BLOCK 拦截
                      </a-radio-button>
                      <a-radio-button value="ALERT" style="border-color: #faad14; color: #d48806">
                        <AlertOutlined /> ALERT 警报
                      </a-radio-button>
                      <a-radio-button value="ALLOW" style="border-color: #52c41a; color: #52c41a">
                        <span style="font-size: 11px">&#10003;</span> ALLOW 放行
                      </a-radio-button>
                    </a-radio-group>
                  </div>
                  <div style="background: #fffbe6; border: 1px solid #ffe58f; border-radius: 4px; padding: 6px 10px; margin-bottom: 8px; font-size: 13px" v-if="sampleCommandLine.trim()">
                    <div v-for="(cmd, idx) in sampleCommandLine.trim().split('|').map(c => c.trim()).filter(c => c)" :key="idx" style="margin-bottom: 2px">
                      <span style="color: #666">{{ idx + 1 }}. </span>
                      <strong>{{ cmd.split(/\s+/)[0] }}</strong>
                      <span v-if="cmd.split(/\s+/).length > 1" style="color: #666"> {{ cmd.split(/\s+/).slice(1).join(' ').slice(0, 30) }}{{ cmd.split(/\s+/).slice(1).join(' ').length > 30 ? '…' : '' }}</span>
                      <span style="color: #666"> → </span>
                      <a-tag :color="sampleLabel === 'BLOCK' ? 'red' : sampleLabel === 'ALERT' ? 'orange' : 'green'" size="small">{{ sampleLabel }}</a-tag>
                    </div>
                  </div>

                  <a-button type="primary" @click="submitManualSample" :loading="submittingSample" block>
                    <PlusOutlined /> 添加此标注样本
                  </a-button>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <!-- Row 5: Command Safety Assessment -->
          <a-col v-if="mlSubTabKey === 'model'" :xs="24">
            <a-card title="Command Safety Assessment" size="small">
              <template #extra>
                <a-tag color="purple">输入完整命令进行安全性判断</a-tag>
              </template>
              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="8">
                  <div style="font-weight: 600; margin-bottom: 8px">待判断命令</div>
                  <a-space direction="vertical" style="width: 100%">
                    <a-textarea
                      v-model:value="backtestCommandLine"
                      placeholder="完整命令 (e.g. sudo systemctl disable firewalld)"
                      :auto-size="{ minRows: 3, maxRows: 6 }"
                      @keyup.ctrl.enter="runBacktest"
                    />
                    <a-button type="primary" @click="runBacktest" :loading="backtesting" block>
                      <SearchOutlined /> 判断安全性
                    </a-button>
                  </a-space>
                  <div style="margin-top: 12px; font-size: 12px; color: #999">
                    快速测试：
                    <a v-for="(p, i) in highRiskPresets.slice(0, 5)" :key="i" @click="runBacktestPreset(p.comm, p.args)" style="margin-right: 8px; white-space: nowrap">{{ p.comm }}</a>
                  </div>
                </a-col>

                <a-col :xs="24" :md="16">
                  <div v-if="backtestResult" style="display: flex; flex-direction: column; gap: 16px">
                    <!-- Risk gauge -->
                    <div style="display: flex; align-items: center; gap: 16px">
                      <div style="flex: 1">
                        <div style="font-weight: 600; margin-bottom: 4px">
                          风险评分：{{ backtestResult.riskScore?.toFixed(0) || '-' }} / 100
                          <a-tag :color="riskLevelColor(backtestResult.riskLevel)" style="margin-left: 8px">
                            {{ backtestResult.riskLevel }}
                          </a-tag>
                        </div>
                        <div style="background: #f0f0f0; border-radius: 8px; height: 20px; overflow: hidden">
                          <div
                            :style="{
                              width: (backtestResult.riskScore || 0) + '%',
                              height: '100%',
                              background: riskMeterColor(backtestResult.riskScore || 0),
                              borderRadius: '8px',
                              transition: 'width 0.5s ease',
                            }"
                          ></div>
                        </div>
                      </div>
                      <div style="text-align: center; min-width: 80px">
                        <div style="font-size: 28px; font-weight: bold; color: riskMeterColor(backtestResult.riskScore || 0)">
                          {{ backtestResult.riskScore?.toFixed(0) || 0 }}
                        </div>
                        <div style="font-size: 11px; color: #999">/ 100</div>
                      </div>
                    </div>

                    <!-- Detail breakdown -->
                    <a-descriptions :column="3" size="small" bordered>
                      <a-descriptions-item label="Command">{{ backtestResult.commandLine || backtestResult.comm }}</a-descriptions-item>
                      <a-descriptions-item label="Args">{{ backtestResult.args?.join(' ') || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="Recommended Action">
                        <a-tag :color="backtestResult.recommendedAction === 'BLOCK' ? 'red' : backtestResult.recommendedAction === 'ALERT' ? 'orange' : backtestResult.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                          {{ backtestResult.recommendedAction }}
                        </a-tag>
                      </a-descriptions-item>
                      <a-descriptions-item label="Category">
                        <a-tag>{{ backtestResult.classification?.primary_category || 'UNKNOWN' }}</a-tag>
                      </a-descriptions-item>
                      <a-descriptions-item label="Classify Confidence">{{ backtestResult.classification?.confidence || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="Anomaly Score">
                        <span :style="{ color: (backtestResult.anomalyScore ?? 0) > 0.7 ? '#d4380d' : (backtestResult.anomalyScore ?? 0) > 0.3 ? '#d48806' : '#52c41a' }">
                          {{ backtestResult.anomalyScore?.toFixed(3) || '—' }}
                        </span>
                      </a-descriptions-item>
                      <a-descriptions-item label="ML Action">{{ backtestResult.mlPrediction?.action || '—' }}</a-descriptions-item>
                      <a-descriptions-item label="ML Confidence">
                        {{ backtestResult.mlPrediction?.confidence ? (backtestResult.mlPrediction.confidence * 100).toFixed(0) + '%' : '—' }}
                      </a-descriptions-item>
                      <a-descriptions-item label="Reasoning" :span="3">{{ backtestResult.reasoning || '—' }}</a-descriptions-item>
                    </a-descriptions>

                    <!-- LLM scoring breakdown -->
                    <div v-if="backtestResult.llmAssessment" style="margin-top: 8px">
                      <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                        <span>LLM 打分结果</span>
                        <a-tag :color="backtestResult.llmAssessment.error ? 'red' : 'purple'">
                          {{ backtestResult.llmAssessment.error ? 'Error' : 'OpenAI-style' }}
                        </a-tag>
                      </div>
                      <a-alert
                        v-if="backtestResult.llmAssessment.error"
                        type="error"
                        show-icon
                        :message="backtestResult.llmAssessment.error"
                        style="margin-bottom: 8px"
                      />
                      <a-descriptions v-else :column="3" size="small" bordered>
                        <a-descriptions-item label="Model">{{ backtestResult.llmAssessment.model || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Risk Score">{{ backtestResult.llmAssessment.riskScore?.toFixed(0) || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Confidence">
                          {{ backtestResult.llmAssessment.confidence ? (backtestResult.llmAssessment.confidence * 100).toFixed(0) + '%' : '—' }}
                        </a-descriptions-item>
                        <a-descriptions-item label="Recommended Action">
                          <a-tag :color="backtestResult.llmAssessment.recommendedAction === 'BLOCK' ? 'red' : backtestResult.llmAssessment.recommendedAction === 'ALERT' ? 'orange' : backtestResult.llmAssessment.recommendedAction === 'REWRITE' ? 'blue' : 'green'">
                            {{ backtestResult.llmAssessment.recommendedAction }}
                          </a-tag>
                        </a-descriptions-item>
                        <a-descriptions-item label="Reasoning" :span="2">{{ backtestResult.llmAssessment.reasoning || '—' }}</a-descriptions-item>
                        <a-descriptions-item label="Signals" :span="3">
                          <a-space wrap>
                            <a-tag v-for="(signal, i) in backtestResult.llmAssessment.signals || []" :key="i" color="purple">
                              {{ signal }}
                            </a-tag>
                            <span v-if="(backtestResult.llmAssessment.signals || []).length === 0" style="color: #999">—</span>
                          </a-space>
                        </a-descriptions-item>
                      </a-descriptions>
                    </div>

                    <!-- Existing labeled sample evidence -->
                    <div v-if="backtestResult.sampleEvidence?.totalMatches > 0">
                      <a-alert
                        show-icon
                        :type="backtestResult.sampleEvidence?.decision === 'BLOCK' ? 'error' : backtestResult.sampleEvidence?.decision === 'ALERT' ? 'warning' : 'info'"
                        :message="`命中已有样本 ${backtestResult.sampleEvidence.totalMatches} 条，已标注 ${backtestResult.sampleEvidence.labeledMatches} 条`"
                        :description="backtestResult.sampleEvidence?.decision ? `历史标注倾向：${backtestResult.sampleEvidence.decision}，置信度 ${(backtestResult.sampleEvidence.confidence * 100).toFixed(0)}%` : '暂无可直接用于判断的标注，但命令已存在于样本库。'"
                        style="margin-bottom: 8px"
                      />
                      <a-table
                        :dataSource="backtestResult.sampleMatches || []"
                        :pagination="false"
                        size="small"
                        rowKey="index"
                        :scroll="{ x: 700 }"
                      >
                        <a-table-column title="#" dataIndex="index" :width="60" />
                        <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                          <template #default="{ record }">
                            <code>{{ maskSensitiveData(record.commandLine) }}</code>
                          </template>
                        </a-table-column>
                        <a-table-column title="Label" dataIndex="label" :width="90">
                          <template #default="{ record }">
                            <a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag>
                          </template>
                        </a-table-column>
                        <a-table-column title="User Label" dataIndex="userLabel" :width="120" />
                        <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                          <template #default="{ record }">
                            {{ record.anomalyScore?.toFixed(2) }}
                          </template>
                        </a-table-column>
                      </a-table>
                    </div>

                    <!-- Network Audit Findings -->
                    <div v-if="backtestResult.networkAudit && backtestResult.networkAudit.findings?.length > 0" style="margin-top: 16px">
                      <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                        <span>网络审计发现</span>
                        <a-tag :color="backtestResult.networkAudit.riskLevel === 'CRITICAL' ? 'red' : backtestResult.networkAudit.riskLevel === 'HIGH' ? 'orange' : backtestResult.networkAudit.riskLevel === 'MEDIUM' ? 'gold' : 'blue'">
                          {{ backtestResult.networkAudit.riskLevel }}
                        </a-tag>
                        <span style="color: #999; font-size: 12px">风险分：{{ backtestResult.networkAudit.riskScore?.toFixed(0) }}</span>
                      </div>
                      <a-list size="small" bordered :data-source="backtestResult.networkAudit.findings">
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <a-list-item-meta>
                              <template #title>
                                <span style="display: flex; align-items: center; gap: 8px">
                                  <a-tag :color="item.severity === 'critical' ? 'red' : item.severity === 'high' ? 'orange' : item.severity === 'medium' ? 'gold' : 'blue'" size="small">
                                    {{ item.severity.toUpperCase() }}
                                  </a-tag>
                                  <span>{{ item.type }}</span>
                                </span>
                              </template>
                              <template #description>{{ item.description }}</template>
                            </a-list-item-meta>
                          </a-list-item>
                        </template>
                      </a-list>
                    </div>
                  </div>
                  <div v-else style="color: #999; text-align: center; padding: 40px">
                    输入命令并点击“判断安全性”查看评估结果；若已有完全匹配的标注样本，会优先作为判断证据。
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Auto Parameter Tuning" size="small">
              <template #extra>
                <a-space>
                  <a-tag color="magenta">{{ autoTuneGridSize }}×{{ autoTuneGridSize }} 方阵</a-tag>
                  <a-button size="small" type="primary" :loading="autoTuneLoading" @click="runAutoTune">
                    <ControlOutlined /> 开始调优
                  </a-button>
                </a-space>
              </template>

              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 12px"
                :message="`选择两个参数做平方搜索，颜色越深表示所选指标越高。当前按「${autoTuneMetricLabel(autoTuneMetric)}」着色。`"
              />

              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="6">
                  <a-space direction="vertical" style="width: 100%">
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">X 轴参数</div>
                      <a-select v-model:value="autoTuneXAxis" style="width: 100%">
                        <a-select-option v-for="opt in autoTuneAxisOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</a-select-option>
                      </a-select>
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">Y 轴参数</div>
                      <a-select v-model:value="autoTuneYAxis" style="width: 100%">
                        <a-select-option v-for="opt in autoTuneAxisOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</a-select-option>
                      </a-select>
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">方阵大小</div>
                      <a-radio-group v-model:value="autoTuneGridSize" button-style="solid" size="small">
                        <a-radio-button :value="3">3</a-radio-button>
                        <a-radio-button :value="5">5</a-radio-button>
                        <a-radio-button :value="7">7</a-radio-button>
                        <a-radio-button :value="9">9</a-radio-button>
                        <a-radio-button :value="11">11</a-radio-button>
                        <a-radio-button :value="15">15</a-radio-button>
                        <a-radio-button :value="21">21</a-radio-button>
                        <a-radio-button :value="31">31</a-radio-button>
                      </a-radio-group>
                      <a-input-number
                        v-if="![3,5,7,9,11,15,21,31].includes(autoTuneGridSize)"
                        v-model:value="autoTuneGridSize"
                        :min="3" :max="51" :step="2"
                        placeholder="自定义 (3-51)"
                        style="width: 100%; margin-top: 4px;"
                      />
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">颗粒度</div>
                      <a-radio-group v-model:value="autoTuneGranularity" button-style="solid">
                        <a-radio-button :value="1">1x</a-radio-button>
                        <a-radio-button :value="2">2x</a-radio-button>
                        <a-radio-button :value="4">4x</a-radio-button>
                      </a-radio-group>
                      <a-typography-text type="secondary" style="display: block; margin-top: 4px">
                        数值越大，搜索越细
                      </a-typography-text>
                    </div>
                    <div>
                      <div style="font-weight: 600; margin-bottom: 6px">着色指标</div>
                      <a-radio-group v-model:value="autoTuneMetric" button-style="solid">
                        <a-radio-button value="validationAccuracy">回测准确率</a-radio-button>
                        <a-radio-button value="inferenceThroughput">推理速度</a-radio-button>
                      </a-radio-group>
                    </div>
                    <a-collapse :bordered="false" style="background: transparent;">
                      <a-collapse-panel key="range" header="展开：自定义参数范围">
                        <div style="display: flex; gap: 8px; align-items: center; margin-bottom: 8px;">
                          <span style="font-size: 12px; width: 50px;">{{ autoTuneAxisLabel(autoTuneXAxis) }}</span>
                          <a-input-number v-model:value="autoTuneMinX" :min="1" size="small" placeholder="最小" style="width: 70px;" />
                          <span style="font-size: 12px;">~</span>
                          <a-input-number v-model:value="autoTuneMaxX" :min="1" size="small" placeholder="最大" style="width: 70px;" />
                          <a-button size="small" type="link" @click="autoTuneMinX = undefined; autoTuneMaxX = undefined;">自动</a-button>
                        </div>
                        <div style="display: flex; gap: 8px; align-items: center;">
                          <span style="font-size: 12px; width: 50px;">{{ autoTuneAxisLabel(autoTuneYAxis) }}</span>
                          <a-input-number v-model:value="autoTuneMinY" :min="1" size="small" placeholder="最小" style="width: 70px;" />
                          <span style="font-size: 12px;">~</span>
                          <a-input-number v-model:value="autoTuneMaxY" :min="1" size="small" placeholder="最大" style="width: 70px;" />
                          <a-button size="small" type="link" @click="autoTuneMinY = undefined; autoTuneMaxY = undefined;">自动</a-button>
                        </div>
                      </a-collapse-panel>
                    </a-collapse>
                    <a-alert
                      type="warning"
                      show-icon
                      message="X/Y 轴不能相同；调优结果会直接更新到当前滑块。"
                    />
                    <!-- Auto-tune Progress -->
                    <div v-if="autoTuneLoading || autoTuneInProgress || autoTuneMessage || autoTuneError" style="background: #fafafa; padding: 12px; border-radius: 8px; border: 1px solid #f0f0f0;">
                      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
                        <span style="font-weight: 600; font-size: 13px;">
                          <ReloadOutlined v-if="autoTuneLoading || autoTuneInProgress" spin style="margin-right: 4px;" />
                          {{ autoTuneLoading || autoTuneInProgress ? '调优进行中' : '调优完成' }}
                        </span>
                        <span v-if="autoTuneLoading || autoTuneInProgress" style="font-size: 12px; color: #999;">
                          已用 {{ autoTuneElapsed }}
                        </span>
                      </div>
                      <a-progress
                        :percent="autoTuneTotal > 0 ? Math.round(autoTuneCompleted / autoTuneTotal * 100) : (autoTuneInProgress ? 0 : 100)"
                        :status="autoTuneError ? 'exception' : (autoTuneInProgress ? 'active' : 'success')"
                        style="margin-bottom: 4px;"
                      />
                      <div style="display: flex; justify-content: space-between; gap: 12px; font-size: 12px; color: #666;">
                        <span>{{ autoTuneMessage || (autoTuneInProgress ? '正在评估参数组合...' : '已完成') }}</span>
                        <span>{{ autoTuneCompleted }} / {{ autoTuneTotal || autoTuneGridSize * autoTuneGridSize }} 格</span>
                      </div>
                      <a-alert
                        v-if="autoTuneError"
                        type="error"
                        show-icon
                        :message="autoTuneError"
                        style="margin-top: 8px"
                      />
                      <!-- Success summary after completion -->
                      <a-alert
                        v-if="autoTuneJustCompleted && autoTuneBestCell"
                        type="success"
                        show-icon
                        style="margin-top: 8px"
                      >
                        <template #message>
                          <span style="font-weight: 600;">最佳参数：</span>
                          树数={{ autoTuneBestCell.numTrees }}，
                          深度={{ autoTuneBestCell.maxDepth }}，
                          叶样本={{ autoTuneBestCell.minSamplesLeaf }}
                          <span style="margin-left: 8px; color: #52c41a;">
                            {{ autoTuneMetricLabel(autoTuneMetric) }}={{ autoTuneMetricFormat(autoTuneScore(autoTuneBestCell)) }}
                          </span>
                        </template>
                      </a-alert>
                    </div>
                    <!-- Training logs during auto-tune -->
                    <details v-if="autoTuneInProgress && trainingLogs.length > 0" style="margin-top: 4px;">
                      <summary style="cursor: pointer; font-size: 12px; color: #888;">查看调优日志 ({{ trainingLogs.length }})</summary>
                      <div style="max-height: 160px; overflow-y: auto; background: #1e1e1e; color: #d4d4d4; font-family: monospace; font-size: 11px; padding: 8px; border-radius: 4px; margin-top: 4px;">
                        <div v-for="(log, i) in trainingLogs.slice(-50)" :key="i"
                          :style="{ color: log.message.includes('ERROR') ? '#f48771' : log.message.includes('完成') || log.message.includes('best') ? '#89d185' : '#d4d4d4' }">
                          <span style="color: #888;">{{ log.time }}</span> {{ log.message }}
                        </div>
                      </div>
                    </details>
                  </a-space>
                </a-col>

                <a-col :xs="24" :md="18">
                  <div
                    style="
                      width: 100%;
                      aspect-ratio: 1 / 1;
                      min-height: 420px;
                      background: #fff;
                      border: 1px solid #f0f0f0;
                      border-radius: 8px;
                      padding: 8px;
                    "
                  >
                    <VueApexCharts
                      v-if="autoTuneHeatmapSeries.length > 0"
                      type="heatmap"
                      :height="Math.max(360, autoTuneGridSize * 64)"
                      :options="autoTuneHeatmapOptions"
                      :series="autoTuneHeatmapSeries"
                    />
                    <a-empty
                      v-else
                      description="点击“开始调优”生成参数方阵"
                      style="height: 100%; display: flex; align-items: center; justify-content: center"
                    />
                  </div>
                </a-col>
              </a-row>

              <a-divider />

              <a-row :gutter="[16, 16]">
                <a-col :xs="24" :md="8">
                  <a-card size="small" title="当前选中">
                    <template v-if="autoTuneSelectedCell">
                      <a-descriptions :column="1" size="small" bordered>
                        <a-descriptions-item :label="autoTuneAxisLabel(autoTuneXAxis)">{{ autoTuneSelectedCell.xValue }}</a-descriptions-item>
                        <a-descriptions-item :label="autoTuneAxisLabel(autoTuneYAxis)">{{ autoTuneSelectedCell.yValue }}</a-descriptions-item>
                        <a-descriptions-item label="树数">{{ autoTuneSelectedCell.numTrees }}</a-descriptions-item>
                        <a-descriptions-item label="最大深度">{{ autoTuneSelectedCell.maxDepth }}</a-descriptions-item>
                        <a-descriptions-item label="叶节点样本">{{ autoTuneSelectedCell.minSamplesLeaf }}</a-descriptions-item>
                        <a-descriptions-item :label="autoTuneMetricLabel(autoTuneMetric)">
                          {{ autoTuneMetricFormat(autoTuneScore(autoTuneSelectedCell)) }}
                        </a-descriptions-item>
                      </a-descriptions>
                    </template>
                    <a-empty v-else description="暂无选中项" />
                  </a-card>
                </a-col>

                <a-col :xs="24" :md="8">
                  <a-card size="small" title="最佳结果">
                    <template v-if="autoTuneBestCell">
                      <a-tag color="success" style="margin-bottom: 8px;">最优 {{ autoTuneMetricLabel(autoTuneMetric) }}</a-tag>
                      <a-descriptions :column="1" size="small" bordered>
                        <a-descriptions-item :label="autoTuneAxisLabel(autoTuneXAxis)">{{ autoTuneBestCell.xValue }}</a-descriptions-item>
                        <a-descriptions-item :label="autoTuneAxisLabel(autoTuneYAxis)">{{ autoTuneBestCell.yValue }}</a-descriptions-item>
                        <a-descriptions-item label="树数">{{ autoTuneBestCell.numTrees }}</a-descriptions-item>
                        <a-descriptions-item label="最大深度">{{ autoTuneBestCell.maxDepth }}</a-descriptions-item>
                        <a-descriptions-item label="叶节点样本">{{ autoTuneBestCell.minSamplesLeaf }}</a-descriptions-item>
                        <a-descriptions-item :label="autoTuneMetricLabel(autoTuneMetric)">
                          <b>{{ autoTuneMetricFormat(autoTuneScore(autoTuneBestCell)) }}</b>
                        </a-descriptions-item>
                        <a-descriptions-item label="验证集准确率">
                          {{ (autoTuneBestCell.validationAccuracy * 100).toFixed(1) }}%
                        </a-descriptions-item>
                        <a-descriptions-item label="推理速度">
                          {{ autoTuneMetricFormat(autoTuneBestCell.inferenceThroughput, 'inferenceThroughput') }}
                        </a-descriptions-item>
                        <a-descriptions-item label="训练/评估耗时">
                          {{ autoTuneBestCell.trainDuration.toFixed(2) }}s / {{ autoTuneBestCell.evalDuration.toFixed(2) }}s
                        </a-descriptions-item>
                      </a-descriptions>
                    </template>
                    <a-empty v-else description="运行后自动选出最佳结果" />
                  </a-card>
                </a-col>

                <a-col :xs="24" :md="8">
                  <a-card size="small" title="应用操作">
                    <a-space direction="vertical" style="width: 100%">
                      <a-button
                        type="primary"
                        block
                        :disabled="!autoTuneSelectedCell"
                        @click="applyAutoTuneCell(autoTuneSelectedCell)"
                      >
                        <ControlOutlined /> 应用选中参数
                      </a-button>
                      <a-button
                        block
                        :disabled="!autoTuneBestCell"
                        @click="applyAutoTuneCell(autoTuneBestCell)"
                      >
                        应用最佳参数
                      </a-button>
                      <a-button block @click="mlSubTabKey = 'model'">
                        前往训练页
                      </a-button>
                    </a-space>
                  </a-card>
                </a-col>
              </a-row>

              <div v-if="autoTuneResponse" style="margin-top: 12px; padding: 8px 12px; background: #f6ffed; border: 1px solid #b7eb8f; border-radius: 6px; font-size: 12px;">
                <CheckCircleOutlined style="color: #52c41a; margin-right: 6px;" />
                共评估 <b>{{ autoTuneResponse.cells.length }}</b> 个参数组合（{{ autoTuneResponse.gridSize }}×{{ autoTuneResponse.gridSize }} 方阵，颗粒度 {{ autoTuneGranularityLabel(autoTuneResponse.granularity) }}），
                样本 <b>{{ autoTuneResponse.sampleCount }}</b>，验证集 <b>{{ autoTuneResponse.validationCount }}</b>，
                总用时 <b>{{ autoTuneResponse.totalDuration.toFixed(1) }}s</b>
              </div>
            </a-card>
          </a-col>

          <!-- Row 6: Detection Thresholds -->
          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Training / Validation Split" size="small">
              <template #extra>
                <a-tag color="purple">训练后会自动切分验证集，并可选做 LLM 后训练复核</a-tag>
              </template>
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="12">
                  <span>Validation Split Ratio</span>
                  <a-slider v-model:value="mlTrainingConfig.validationSplitRatio" :min="0.1" :max="0.4" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlTrainingConfig.validationSplitRatio" :min="0.1" :max="0.4" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="12">
                  <div style="font-size: 13px; color: #666; line-height: 1.7; margin-top: 24px">
                    <div>• 训练时会先随机切分训练集 / 验证集，再分别记录 train / validation accuracy。</div>
                    <div>• 后训练阶段可用外部 OpenAI 风格 LLM 对验证集做批量复核。</div>
                    <div>• 若训练集打分选择“回写标签”，仅训练集会被改写，验证集只读。</div>
                  </div>
                </a-col>
              </a-row>
            </a-card>
          </a-col>

          <a-col v-if="mlSubTabKey === 'params'" :xs="24">
            <a-card title="Detection Thresholds" size="small">
              <a-row :gutter="[24, 16]">
                <a-col :xs="24" :md="8">
                  <span>Block Confidence Threshold</span>
                  <a-slider v-model:value="mlThresholds.blockConfidenceThreshold" :min="0.5" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.blockConfidenceThreshold" :min="0.5" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>ML Minimum Confidence</span>
                  <a-slider v-model:value="mlThresholds.mlMinConfidence" :min="0.3" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.mlMinConfidence" :min="0.3" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>Rule Override Priority</span>
                  <a-slider v-model:value="mlThresholds.ruleOverridePriority" :min="0" :max="200" :step="10" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.ruleOverridePriority" :min="0" :max="200" :step="10" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>Low Anomaly Threshold (below = normal)</span>
                  <a-slider v-model:value="mlThresholds.lowAnomalyThreshold" :min="0.0" :max="0.5" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.lowAnomalyThreshold" :min="0.0" :max="0.5" :step="0.05" size="small" style="width: 100%" />
                </a-col>
                <a-col :xs="24" :md="8">
                  <span>High Anomaly Threshold (above = alert)</span>
                  <a-slider v-model:value="mlThresholds.highAnomalyThreshold" :min="0.5" :max="1.0" :step="0.05" @afterChange="saveMLThresholds" />
                  <a-input-number v-model:value="mlThresholds.highAnomalyThreshold" :min="0.5" :max="1.0" :step="0.05" size="small" style="width: 100%" />
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>
</template>
