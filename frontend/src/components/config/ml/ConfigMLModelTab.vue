<script setup lang="ts">
import { computed } from 'vue';
import {
  ThunderboltOutlined, ReloadOutlined, StopOutlined,
  SearchOutlined, ControlOutlined,
} from '@ant-design/icons-vue';
import { highRiskPresets, type useConfigML } from '../../../composables/useConfigML';
import { mlModelCategoryColor } from '../../../data/mlModelCatalog';

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const modelTuneColumns = [
  { title: '模型', dataIndex: 'label', key: 'label' },
  { title: '基础算法', dataIndex: 'base', key: 'base' },
  { title: '验证准确率', dataIndex: 'validationAccuracy', key: 'validationAccuracy' },
  { title: '训练准确率', dataIndex: 'trainAccuracy', key: 'trainAccuracy' },
  { title: '推理速度', dataIndex: 'inferenceThroughput', key: 'inferenceThroughput' },
  { title: '参数', dataIndex: 'hyperParams', key: 'hyperParams' },
  { title: '状态', dataIndex: 'state', key: 'state' },
];

const {
  mlStatus, trainingModel, feedbackComm, feedbackAction,
  modelType, builtinModelCatalog, selectedBuiltinModel, modelBaseType, hyperParams,
  autoTuneMetric, autoTuneMetricLabel, autoTuneMetricFormat,
  autoTuneLoading, autoTuneInProgress, autoTuneCompleted, autoTuneTotal, autoTuneMessage, autoTuneError,
  modelTuneSelectedTypes, modelTuneParamSearch, modelTuneApplyBest, modelTuneResponse, modelTuneBest, modelTuneRecommendedTypes,
  cudaAvailable, cudaMemUsedMB, cudaMemTotalMB, mlCRuntime, cancellingTraining,
  backtestCommandLine, backtesting, backtestResult,
  trainWithParams, cancelTraining, submitFeedback, saveMLModelType, runModelTune, applyModelTuneBest,
  runBacktest, runBacktestPreset, riskLevelColor, riskMeterColor,
  getLabelColor, maskSensitiveData,
} = props.ml;

const modelCatalogGroups = computed(() => {
  const groups = new Map<string, typeof builtinModelCatalog.value>();
  for (const item of builtinModelCatalog.value) {
    const key = item.category || '其他模型';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)?.push(item);
  }
  return Array.from(groups.entries()).map(([category, models]) => ({ category, models }));
});

const modelTypeLabel = computed(() => selectedBuiltinModel.value?.label || modelType.value);
const modelTypeTagColor = computed(() => mlModelCategoryColor(selectedBuiltinModel.value?.category, modelBaseType.value));
const modelTuneBestType = computed(() => modelTuneBest.value?.modelType || '');
const modelTuneProgressTotal = computed(() => autoTuneTotal.value || modelTuneSelectedTypes.value.length || modelTuneRecommendedTypes.value.length);

const selectRecommendedModels = () => {
  modelTuneSelectedTypes.value = modelTuneRecommendedTypes.value.slice();
};

const selectModelCategory = (category: string) => {
  modelTuneSelectedTypes.value = builtinModelCatalog.value.filter((item) => item.category === category).map((item) => item.value);
};

type RuntimeBackendView = {
  id: string;
  label: string;
  available: boolean;
  accelerated: boolean;
  detail?: string;
};

const runtimeBackendColor = (backend: RuntimeBackendView) => {
  if (backend.accelerated) return 'green';
  if (backend.available) return 'blue';
  return 'default';
};

const runtimeBackendSuffix = (backend: RuntimeBackendView) => {
  if (backend.accelerated) return 'accelerated';
  if (backend.available) return 'available';
  return 'missing';
};

const runtimeBackendLabel = (backendId?: string) => {
  const labels: Record<string, string> = {
    c_cpu: 'Native C CPU',
    cuda: 'NVIDIA CUDA',
    intel_igpu: 'Intel iGPU',
  };
  return backendId ? labels[backendId] || backendId : '—';
};

const formatRuntimeMs = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '—';
  if (value < 0.001) return `${(value * 1000).toFixed(2)} µs`;
  if (value < 1) return `${value.toFixed(4)} ms`;
  return `${value.toFixed(2)} ms`;
};

const formatRuntimeSpeedup = (value?: number) => {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '—';
  return `${value.toFixed(2)}×`;
};
</script>

<template>
  <!-- Multi-model management -->
  <a-col :xs="24">
    <a-card title="Multi-model Management" size="small">
      <template #extra>
        <a-tag :color="modelTypeTagColor">当前：{{ modelTypeLabel }}</a-tag>
      </template>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :lg="10">
          <a-space direction="vertical" style="width: 100%">
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">Active Model</div>
              <a-select
                v-model:value="modelType"
                show-search
                option-filter-prop="label"
                style="width: 100%"
                @change="saveMLModelType"
              >
                <a-select-opt-group v-for="group in modelCatalogGroups" :key="group.category" :label="group.category">
                  <a-select-option v-for="item in group.models" :key="item.value" :value="item.value" :label="`${item.label} ${item.value} ${item.tags?.join(' ') || ''}`">
                    <a-space>
                      <span>{{ item.label }}</span>
                      <a-tag v-if="item.recommended" color="green">推荐</a-tag>
                      <a-tag color="default">{{ item.base }}</a-tag>
                      <a-tag v-for="tag in item.tags || []" :key="`${item.value}-${tag}`" color="blue">{{ tag }}</a-tag>
                    </a-space>
                  </a-select-option>
                </a-select-opt-group>
              </a-select>
            </div>
            <a-descriptions :column="1" size="small" bordered>
              <a-descriptions-item label="基础算法">{{ selectedBuiltinModel?.base || modelBaseType }}</a-descriptions-item>
              <a-descriptions-item label="当前参数">trees={{ hyperParams.numTrees }} / depth={{ hyperParams.maxDepth }} / leaf={{ hyperParams.minSamplesLeaf }}</a-descriptions-item>
              <a-descriptions-item label="说明">{{ selectedBuiltinModel?.description || '本地模型配置' }}</a-descriptions-item>
            </a-descriptions>
          </a-space>
        </a-col>
        <a-col :xs="24" :lg="14">
          <div style="font-weight: 600; margin-bottom: 6px">Model Catalog</div>
          <a-row :gutter="[8, 8]">
            <a-col v-for="group in modelCatalogGroups" :key="group.category" :xs="24" :md="12">
              <a-card size="small" :title="group.category" :body-style="{ padding: '8px' }">
                <a-space wrap size="small">
                  <a-tag v-for="item in group.models" :key="item.value" :color="item.value === modelType ? 'processing' : mlModelCategoryColor(item.category, item.base)">
                    {{ item.label }}
                  </a-tag>
                </a-space>
                <a-button size="small" type="link" @click="selectModelCategory(group.category)">选择本类参与调优</a-button>
              </a-card>
            </a-col>
          </a-row>
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <!-- Multi-type auto tuning -->
  <a-col :xs="24">
    <a-card title="Multi-type Model Auto Tuning" size="small">
      <template #extra>
        <a-space>
          <a-tag color="magenta">{{ modelTuneSelectedTypes.length || modelTuneRecommendedTypes.length }} 个候选</a-tag>
          <a-button size="small" type="primary" :loading="autoTuneLoading" @click="runModelTune">
            <ControlOutlined /> 开始多类型调优
          </a-button>
        </a-space>
      </template>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :lg="8">
          <a-space direction="vertical" style="width: 100%">
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">候选模型</div>
              <a-select
                v-model:value="modelTuneSelectedTypes"
                mode="multiple"
                show-search
                option-filter-prop="label"
                style="width: 100%"
                placeholder="默认使用推荐模型"
              >
                <a-select-option v-for="item in builtinModelCatalog" :key="item.value" :value="item.value" :label="`${item.label} ${item.base} ${item.tags?.join(' ') || ''}`">
                  <a-space>
                    <span>{{ item.label }}</span>
                    <a-tag v-if="item.recommended" color="green">推荐</a-tag>
                    <a-tag color="default">{{ item.base }}</a-tag>
                  </a-space>
                </a-select-option>
              </a-select>
              <a-space wrap style="margin-top: 6px">
                <a-button size="small" type="link" @click="selectRecommendedModels">选择推荐模型</a-button>
                <a-button size="small" type="link" @click="modelTuneSelectedTypes = []">清空选择</a-button>
              </a-space>
            </div>
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">排序指标</div>
              <a-radio-group v-model:value="autoTuneMetric" button-style="solid">
                <a-radio-button value="validationAccuracy">回测准确率</a-radio-button>
                <a-radio-button value="inferenceThroughput">推理速度</a-radio-button>
              </a-radio-group>
            </div>
            <a-checkbox v-model:checked="modelTuneParamSearch">每个模型再做参数方阵细调</a-checkbox>
            <a-checkbox v-model:checked="modelTuneApplyBest">完成后自动应用并保存最佳模型</a-checkbox>
            <div v-if="autoTuneLoading || autoTuneInProgress || autoTuneMessage || autoTuneError" style="background: #fafafa; padding: 12px; border-radius: 8px; border: 1px solid #f0f0f0;">
              <div style="display: flex; justify-content: space-between; gap: 8px; font-size: 12px; color: #666; margin-bottom: 6px">
                <span>{{ autoTuneMessage || (autoTuneInProgress ? '正在评估候选模型...' : '已完成') }}</span>
                <span>{{ autoTuneCompleted }} / {{ modelTuneProgressTotal }} 模型</span>
              </div>
              <a-progress :percent="modelTuneProgressTotal > 0 ? Math.round(autoTuneCompleted / modelTuneProgressTotal * 100) : 0" :status="autoTuneError ? 'exception' : (autoTuneInProgress ? 'active' : 'success')" />
              <a-alert v-if="autoTuneError" type="error" show-icon :message="autoTuneError" style="margin-top: 8px" />
            </div>
          </a-space>
        </a-col>
        <a-col :xs="24" :lg="16">
          <a-table
            size="small"
            :columns="modelTuneColumns"
            :data-source="modelTuneResponse?.candidates || []"
            :pagination="false"
            row-key="modelType"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'label'">
                <a-space>
                  <span>{{ record.label || record.modelType }}</span>
                  <a-tag v-if="record.modelType === modelTuneBestType" color="success">最佳</a-tag>
                  <a-tag v-if="record.recommended" color="green">推荐</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.key === 'validationAccuracy'">{{ autoTuneMetricFormat(record.validationAccuracy, 'validationAccuracy') }}</template>
              <template v-else-if="column.key === 'trainAccuracy'">{{ autoTuneMetricFormat(record.trainAccuracy, 'validationAccuracy') }}</template>
              <template v-else-if="column.key === 'inferenceThroughput'">{{ autoTuneMetricFormat(record.inferenceThroughput, 'inferenceThroughput') }}</template>
              <template v-else-if="column.key === 'hyperParams'">
                <a-space wrap>
                  <a-tag>trees={{ record.hyperParams?.numTrees ?? '—' }}</a-tag>
                  <a-tag>depth={{ record.hyperParams?.maxDepth ?? '—' }}</a-tag>
                  <a-tag>leaf={{ record.hyperParams?.minSamplesLeaf ?? '—' }}</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.key === 'state'">
                <a-tag v-if="record.error" color="error">{{ record.error }}</a-tag>
                <a-tag v-else-if="record.applied" color="processing">已应用</a-tag>
                <a-tag v-else color="success">完成</a-tag>
              </template>
            </template>
          </a-table>
        </a-col>
      </a-row>
      <a-divider />
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="16">
          <a-card size="small" title="最佳模型">
            <template v-if="modelTuneBest">
              <a-tag color="success" style="margin-bottom: 8px;">最优 {{ autoTuneMetricLabel(autoTuneMetric) }}</a-tag>
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item label="模型">{{ modelTuneBest.label || modelTuneBest.modelType }}</a-descriptions-item>
                <a-descriptions-item label="基础算法">{{ modelTuneBest.base }}</a-descriptions-item>
                <a-descriptions-item label="验证集准确率">{{ autoTuneMetricFormat(modelTuneBest.validationAccuracy, 'validationAccuracy') }}</a-descriptions-item>
                <a-descriptions-item label="推理速度">{{ autoTuneMetricFormat(modelTuneBest.inferenceThroughput, 'inferenceThroughput') }}</a-descriptions-item>
                <a-descriptions-item label="参数">trees={{ modelTuneBest.hyperParams?.numTrees }} / depth={{ modelTuneBest.hyperParams?.maxDepth }} / leaf={{ modelTuneBest.hyperParams?.minSamplesLeaf }}</a-descriptions-item>
              </a-descriptions>
            </template>
            <a-empty v-else description="运行后自动选出最佳模型" />
          </a-card>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-card size="small" title="应用操作">
            <a-space direction="vertical" style="width: 100%">
              <a-button type="primary" block :disabled="!modelTuneBest" @click="applyModelTuneBest"><ControlOutlined /> 应用最佳模型配置</a-button>
              <a-button block :disabled="!modelTuneBest" @click="trainWithParams">应用后重新训练当前模型</a-button>
            </a-space>
          </a-card>
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <!-- Training Controls -->
  <a-col :xs="24" :md="12">
    <a-card title="Training Controls" size="small">
      <a-space direction="vertical" style="width: 100%">
        <div v-if="mlStatus.training_in_progress" style="background: #f6ffed; border: 1px solid #b7eb8f; border-radius: 6px; padding: 8px 12px; font-size: 12px;">
          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 4px;">
            <span><ReloadOutlined spin style="margin-right: 4px; color: #52c41a;" /><b>训练中</b> {{ Math.round((mlStatus.training_progress || 0) * 100) }}%</span>
            <span v-if="cudaAvailable && cudaMemTotalMB > 0" style="color: #666;">GPU: {{ cudaMemUsedMB }} / {{ cudaMemTotalMB }} MB</span>
          </div>
          <a-progress :percent="Math.round((mlStatus.training_progress || 0) * 100)" :show-info="false" style="margin-top: 4px;" />
        </div>
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
            <a-button danger :loading="cancellingTraining"><StopOutlined /> 中止</a-button>
          </a-popconfirm>
        </div>
        <a-divider style="margin: 8px 0">Native C Runtime</a-divider>
        <div v-if="mlCRuntime" style="border: 1px solid #f0f0f0; border-radius: 6px; padding: 8px 10px; background: #fafafa;">
          <div style="display: flex; justify-content: space-between; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 8px;">
            <div style="font-weight: 600">
              C running time
              <a-tag :color="mlCRuntime.cSupported ? 'green' : 'orange'" style="margin-left: 6px">
                {{ mlCRuntime.cSupported ? 'kernel ready' : 'detect only' }}
              </a-tag>
            </div>
            <a-tag color="geekblue">active: {{ runtimeBackendLabel(mlCRuntime.activeBackend) }}</a-tag>
          </div>
          <a-space wrap size="small" style="margin-bottom: 8px">
            <a-tooltip v-for="backend in mlCRuntime.backends || []" :key="backend.id" :title="backend.detail || backend.label">
              <a-tag :color="runtimeBackendColor(backend)">
                {{ backend.label }} · {{ runtimeBackendSuffix(backend) }}
              </a-tag>
            </a-tooltip>
          </a-space>
          <a-descriptions :column="2" size="small" bordered>
            <a-descriptions-item label="Go inference">{{ formatRuntimeMs(mlCRuntime.goMsPerSample) }}/sample</a-descriptions-item>
            <a-descriptions-item label="C inference">{{ formatRuntimeMs(mlCRuntime.cMsPerSample) }}/sample</a-descriptions-item>
            <a-descriptions-item label="Speedup">{{ formatRuntimeSpeedup(mlCRuntime.speedup) }}</a-descriptions-item>
            <a-descriptions-item label="Samples">{{ mlCRuntime.sampleCount || 0 }}</a-descriptions-item>
            <a-descriptions-item label="Benchmark backend">{{ runtimeBackendLabel(mlCRuntime.benchmarkBackend) }}</a-descriptions-item>
            <a-descriptions-item label="Model">{{ mlCRuntime.modelType || mlStatus.model_type || '—' }}</a-descriptions-item>
          </a-descriptions>
          <div v-if="mlCRuntime.note" style="margin-top: 6px; color: #8c8c8c; font-size: 12px; line-height: 1.45">
            {{ mlCRuntime.note }}
          </div>
        </div>
        <a-alert v-else type="info" show-icon message="等待后端返回 C runtime / CUDA / Intel iGPU 状态" />
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

  <!-- Command Safety Assessment -->
  <a-col :xs="24">
    <a-card title="Command Safety Assessment" size="small">
      <template #extra><a-tag color="purple">输入完整命令进行安全性判断</a-tag></template>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="8">
          <div style="font-weight: 600; margin-bottom: 8px">待判断命令</div>
          <a-space direction="vertical" style="width: 100%">
            <a-textarea v-model:value="backtestCommandLine" placeholder="完整命令 (e.g. sudo systemctl disable firewalld)" :auto-size="{ minRows: 3, maxRows: 6 }" @keyup.ctrl.enter="runBacktest" />
            <a-button type="primary" @click="runBacktest" :loading="backtesting" block><SearchOutlined /> 判断安全性</a-button>
          </a-space>
          <div style="margin-top: 12px; font-size: 12px; color: #999">
            快速测试：
            <a v-for="(p, i) in highRiskPresets.slice(0, 5)" :key="i" @click="runBacktestPreset(p.comm, p.args)" style="margin-right: 8px; white-space: nowrap">{{ p.comm }}</a>
          </div>
        </a-col>
        <a-col :xs="24" :md="16">
          <div v-if="backtestResult" style="display: flex; flex-direction: column; gap: 16px">
            <div style="display: flex; align-items: center; gap: 16px">
              <div style="flex: 1">
                <div style="font-weight: 600; margin-bottom: 4px">
                  风险评分：{{ backtestResult.riskScore?.toFixed(0) || '-' }} / 100
                  <a-tag :color="riskLevelColor(backtestResult.riskLevel)" style="margin-left: 8px">{{ backtestResult.riskLevel }}</a-tag>
                </div>
                <div style="background: #f0f0f0; border-radius: 8px; height: 20px; overflow: hidden">
                  <div :style="{ width: (backtestResult.riskScore || 0) + '%', height: '100%', background: riskMeterColor(backtestResult.riskScore || 0), borderRadius: '8px', transition: 'width 0.5s ease' }"></div>
                </div>
              </div>
              <div style="text-align: center; min-width: 80px">
                <div style="font-size: 28px; font-weight: bold; color: riskMeterColor(backtestResult.riskScore || 0)">{{ backtestResult.riskScore?.toFixed(0) || 0 }}</div>
                <div style="font-size: 11px; color: #999">/ 100</div>
              </div>
            </div>
            <a-descriptions :column="3" size="small" bordered>
              <a-descriptions-item label="Command">{{ backtestResult.commandLine || backtestResult.comm }}</a-descriptions-item>
              <a-descriptions-item label="Args">{{ backtestResult.args?.join(' ') || '—' }}</a-descriptions-item>
              <a-descriptions-item label="Recommended Action">
                <a-tag :color="backtestResult.recommendedAction === 'BLOCK' ? 'red' : backtestResult.recommendedAction === 'ALERT' ? 'orange' : backtestResult.recommendedAction === 'REWRITE' ? 'blue' : 'green'">{{ backtestResult.recommendedAction }}</a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="Category"><a-tag>{{ backtestResult.classification?.primary_category || 'UNKNOWN' }}</a-tag></a-descriptions-item>
              <a-descriptions-item label="Classify Confidence">{{ backtestResult.classification?.confidence || '—' }}</a-descriptions-item>
              <a-descriptions-item label="Anomaly Score">
                <span :style="{ color: (backtestResult.anomalyScore ?? 0) > 0.7 ? '#d4380d' : (backtestResult.anomalyScore ?? 0) > 0.3 ? '#d48806' : '#52c41a' }">{{ backtestResult.anomalyScore?.toFixed(3) || '—' }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="ML Action">{{ backtestResult.mlPrediction?.action || '—' }}</a-descriptions-item>
              <a-descriptions-item label="ML Confidence">{{ backtestResult.mlPrediction?.confidence ? (backtestResult.mlPrediction.confidence * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
              <a-descriptions-item label="Reasoning" :span="3">{{ backtestResult.reasoning || '—' }}</a-descriptions-item>
            </a-descriptions>
            <div v-if="backtestResult.llmAssessment" style="margin-top: 8px">
              <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                <span>LLM 打分结果</span>
                <a-tag :color="backtestResult.llmAssessment.error ? 'red' : 'purple'">{{ backtestResult.llmAssessment.error ? 'Error' : 'OpenAI-style' }}</a-tag>
              </div>
              <a-alert v-if="backtestResult.llmAssessment.error" type="error" show-icon :message="backtestResult.llmAssessment.error" style="margin-bottom: 8px" />
              <a-descriptions v-else :column="3" size="small" bordered>
                <a-descriptions-item label="Model">{{ backtestResult.llmAssessment.model || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Risk Score">{{ backtestResult.llmAssessment.riskScore?.toFixed(0) || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Confidence">{{ backtestResult.llmAssessment.confidence ? (backtestResult.llmAssessment.confidence * 100).toFixed(0) + '%' : '—' }}</a-descriptions-item>
                <a-descriptions-item label="Recommended Action">
                  <a-tag :color="backtestResult.llmAssessment.recommendedAction === 'BLOCK' ? 'red' : backtestResult.llmAssessment.recommendedAction === 'ALERT' ? 'orange' : backtestResult.llmAssessment.recommendedAction === 'REWRITE' ? 'blue' : 'green'">{{ backtestResult.llmAssessment.recommendedAction }}</a-tag>
                </a-descriptions-item>
                <a-descriptions-item label="Reasoning" :span="2">{{ backtestResult.llmAssessment.reasoning || '—' }}</a-descriptions-item>
                <a-descriptions-item label="Signals" :span="3">
                  <a-space wrap>
                    <a-tag v-for="(signal, i) in backtestResult.llmAssessment.signals || []" :key="i" color="purple">{{ signal }}</a-tag>
                    <span v-if="(backtestResult.llmAssessment.signals || []).length === 0" style="color: #999">—</span>
                  </a-space>
                </a-descriptions-item>
              </a-descriptions>
            </div>
            <div v-if="backtestResult.sampleEvidence?.totalMatches > 0">
              <a-alert show-icon :type="backtestResult.sampleEvidence?.decision === 'BLOCK' ? 'error' : backtestResult.sampleEvidence?.decision === 'ALERT' ? 'warning' : 'info'"
                :message="`命中已有样本 ${backtestResult.sampleEvidence.totalMatches} 条，已标注 ${backtestResult.sampleEvidence.labeledMatches} 条`"
                :description="backtestResult.sampleEvidence?.decision ? `历史标注倾向：${backtestResult.sampleEvidence.decision}，置信度 ${(backtestResult.sampleEvidence.confidence * 100).toFixed(0)}%` : '暂无可直接用于判断的标注，但命令已存在于样本库。'"
                style="margin-bottom: 8px" />
              <a-table :dataSource="backtestResult.sampleMatches || []" :pagination="false" size="small" rowKey="index" :scroll="{ x: 700 }">
                <a-table-column title="#" dataIndex="index" :width="60" />
                <a-table-column title="Command" dataIndex="commandLine" :width="260" ellipsis>
                  <template #default="{ record }"><code>{{ maskSensitiveData(record.commandLine) }}</code></template>
                </a-table-column>
                <a-table-column title="Label" dataIndex="label" :width="90">
                  <template #default="{ record }"><a-tag :color="getLabelColor(record.label)" size="small">{{ record.label }}</a-tag></template>
                </a-table-column>
                <a-table-column title="User Label" dataIndex="userLabel" :width="120" />
                <a-table-column title="Anomaly" dataIndex="anomalyScore" :width="90">
                  <template #default="{ record }">{{ record.anomalyScore?.toFixed(2) }}</template>
                </a-table-column>
              </a-table>
            </div>
            <div v-if="backtestResult.networkAudit && backtestResult.networkAudit.findings?.length > 0" style="margin-top: 16px">
              <div style="font-weight: 600; margin-bottom: 8px; display: flex; align-items: center; gap: 8px">
                <span>网络审计发现</span>
                <a-tag :color="backtestResult.networkAudit.riskLevel === 'CRITICAL' ? 'red' : backtestResult.networkAudit.riskLevel === 'HIGH' ? 'orange' : backtestResult.networkAudit.riskLevel === 'MEDIUM' ? 'gold' : 'blue'">{{ backtestResult.networkAudit.riskLevel }}</a-tag>
                <span style="color: #999; font-size: 12px">风险分：{{ backtestResult.networkAudit.riskScore?.toFixed(0) }}</span>
              </div>
              <a-list size="small" bordered :data-source="backtestResult.networkAudit.findings">
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-list-item-meta>
                      <template #title>
                        <span style="display: flex; align-items: center; gap: 8px">
                          <a-tag :color="item.severity === 'critical' ? 'red' : item.severity === 'high' ? 'orange' : item.severity === 'medium' ? 'gold' : 'blue'" size="small">{{ item.severity.toUpperCase() }}</a-tag>
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
            输入命令并点击"判断安全性"查看评估结果；若已有完全匹配的标注样本，会优先作为判断证据。
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
