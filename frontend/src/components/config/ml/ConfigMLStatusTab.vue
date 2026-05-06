<script setup lang="ts">
import { defineAsyncComponent } from 'vue';
import {
  ImportOutlined, ExportOutlined, ReloadOutlined,
} from '@ant-design/icons-vue';
import type { useConfigML } from '../../../composables/useConfigML';

const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const emit = defineEmits<{ (e: 'nav', tab: string): void }>();

const {
  mlEnabled, mlStatus, modelType,
  trainingLogs,
  trainingHistory, trainingChartOptions, trainingChartSeries,
  llmScoringConfig,
  fetchMLStatus, exportTrainingDataset,
} = props.ml;

const modelTypeLabel = (type: string) => {
  switch (type) {
    case 'random_forest': return 'RF';
    case 'knn': return 'KNN';
    case 'logistic': return 'LR';
    case 'nearest_centroid': return 'NC';
    case 'naive_bayes': return 'NB';
    case 'adaboost': return 'Ada';
    case 'extra_trees': return 'ET';
    case 'svm': return 'SVM';
    case 'ridge': return 'Ridge';
    case 'perceptron': return 'Perc';
    case 'passive_aggressive': return 'PA';
    default: return type;
  }
};
</script>

<template>
  <!-- Model Status Cards -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title><span>Model Status</span></template>
      <template #extra>
        <a-space>
          <a-button size="small" @click="emit('nav', 'training')"><ImportOutlined /> 导入</a-button>
          <a-button size="small" @click="exportTrainingDataset"><ExportOutlined /> 下载</a-button>
          <a-button size="small" type="link" @click="fetchMLStatus"><ReloadOutlined /></a-button>
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
            <a-statistic title="Model Type" :value="modelTypeLabel(modelType)" :value-style="{ color: '#1890ff', fontSize: '18px' }" />
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

  <!-- Model Logs -->
  <a-col :xs="24">
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
      <div v-if="mlStatus.training_in_progress" style="margin-bottom: 12px;">
        <div style="display: flex; justify-content: space-between; font-size: 12px; color: #888; margin-bottom: 4px;">
          <span>训练进度</span>
          <span>{{ Math.round((mlStatus.training_progress || 0) * 100) }}%</span>
        </div>
        <a-progress :percent="Math.round((mlStatus.training_progress || 0) * 100)" :status="'active'" />
      </div>
      <div style="background: #1e1e1e; color: #d4d4d4; border-radius: 6px; padding: 10px 14px; max-height: 320px; overflow-y: auto; font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace; font-size: 12px; line-height: 1.6">
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

  <!-- Training History Chart -->
  <a-col v-if="trainingHistory.length > 0" :xs="24">
    <a-card title="Training History" size="small">
      <template #extra>
        <a-tag color="blue">{{ trainingHistory.length }} runs</a-tag>
      </template>
      <Suspense>
        <VueApexCharts type="line" height="280" :options="trainingChartOptions" :series="trainingChartSeries" />
        <template #fallback>
          <div style="text-align: center; padding: 40px; color: #999">Loading chart...</div>
        </template>
      </Suspense>
    </a-card>
  </a-col>

  <!-- LLM Post-Training Review -->
  <a-col v-if="mlStatus.llm_review" :xs="24">
    <a-card title="LLM Post-Training Review" size="small">
      <template #extra>
        <a-tag color="purple">OpenAI-style batch review</a-tag>
      </template>
      <a-descriptions :column="3" size="small" bordered>
        <a-descriptions-item label="Source">{{ mlStatus.llm_review?.source || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Model">{{ mlStatus.llm_review?.model || llmScoringConfig.model || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Scored Samples">{{ mlStatus.llm_review?.scoredSamples ?? 0 }}</a-descriptions-item>
        <a-descriptions-item label="Average Risk">{{ mlStatus.llm_review ? mlStatus.llm_review.averageRiskScore.toFixed(1) : '—' }}</a-descriptions-item>
        <a-descriptions-item label="Agreement">{{ mlStatus.llm_review ? (mlStatus.llm_review.agreement * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
        <a-descriptions-item label="Validation Split">{{ mlStatus.llm_review?.validationSplitRatio !== undefined ? (mlStatus.llm_review.validationSplitRatio * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
        <a-descriptions-item label="Reviewed At" :span="3">{{ mlStatus.llm_review?.reviewedAt ? new Date(mlStatus.llm_review.reviewedAt).toLocaleString() : '—' }}</a-descriptions-item>
      </a-descriptions>
    </a-card>
  </a-col>
</template>
