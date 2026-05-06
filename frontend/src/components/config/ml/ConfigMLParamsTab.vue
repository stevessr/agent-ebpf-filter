<script setup lang="ts">
import { ref, computed, watch, defineAsyncComponent } from 'vue';
import {
  ReloadOutlined, CheckCircleOutlined, ControlOutlined,
} from '@ant-design/icons-vue';
import type { useConfigML } from '../../../composables/useConfigML';

const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const emit = defineEmits<{ (e: 'nav', tab: string): void }>();

const {
  modelType, cudaAvailable, cudaInfo,
  hyperParams, mlThresholds, mlTrainingConfig,
  autoTuneXAxis, autoTuneYAxis, autoTuneGridSize, autoTuneGranularity, autoTuneMetric,
  autoTuneMinX, autoTuneMaxX, autoTuneMinY, autoTuneMaxY,
  autoTuneAxisOptions,
  autoTuneLoading, autoTuneInProgress, autoTuneCompleted, autoTuneTotal,
  autoTuneMessage, autoTuneError, autoTuneResponse, autoTuneSelectedCell,
  autoTuneAxisLabel, autoTuneMetricLabel, autoTuneMetricFormat, autoTuneGranularityLabel,
  autoTuneScore, autoTuneHeatmapOptions, autoTuneHeatmapSeries, autoTuneBestCell,
  runAutoTune, applyAutoTuneCell, saveMLThresholds,
  trainingLogs,
} = props.ml;

// Auto-tune elapsed time tracking (local to params tab)
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

const autoTuneJustCompleted = computed(() =>
  !autoTuneInProgress.value && autoTuneResponse.value && autoTuneLoading.value === false
);

const modelTypeLabel = computed(() => {
  switch (modelType.value) {
    case 'random_forest': return 'Random Forest';
    case 'extra_trees': return 'Extra Trees';
    case 'adaboost': return 'AdaBoost';
    case 'knn': return 'K-Nearest Neighbors';
    case 'naive_bayes': return 'Naive Bayes';
    case 'nearest_centroid': return 'Nearest Centroid';
    case 'logistic': return 'Logistic';
    case 'svm': return 'SVM';
    case 'ridge': return 'Ridge';
    case 'perceptron': return 'Perceptron';
    case 'passive_aggressive': return 'PA';
    default: return modelType.value;
  }
});

const modelTypeTagColor = computed(() => {
  switch (modelType.value) {
    case 'random_forest': return 'green';
    case 'extra_trees': return 'purple';
    case 'adaboost': return 'magenta';
    case 'knn': return 'blue';
    case 'naive_bayes': return 'gold';
    case 'nearest_centroid': return 'geekblue';
    case 'logistic': return 'cyan';
    case 'svm': return 'red';
    case 'ridge': return 'volcano';
    case 'perceptron': return 'orange';
    case 'passive_aggressive': return 'volcano';
    default: return 'purple';
  }
});
</script>

<template>
  <!-- Model Type Selector -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Model Type</span>
        <a-tag :color="modelTypeTagColor" style="margin-left: 8px;">
          {{ modelTypeLabel }}
        </a-tag>
      </template>
      <a-radio-group v-model:value="modelType" button-style="solid" size="small" @change="saveMLThresholds">
        <a-radio-button value="random_forest">Random Forest</a-radio-button>
        <a-radio-button value="extra_trees">Extra Trees</a-radio-button>
        <a-radio-button value="adaboost">AdaBoost</a-radio-button>
        <a-radio-button value="knn">KNN</a-radio-button>
        <a-radio-button value="naive_bayes">Naive Bayes</a-radio-button>
        <a-radio-button value="nearest_centroid">Nearest Centroid</a-radio-button>
        <a-radio-button value="logistic">Logistic</a-radio-button>
        <a-radio-button value="svm">SVM</a-radio-button>
        <a-radio-button value="ridge">Ridge</a-radio-button>
        <a-radio-button value="perceptron">Perceptron</a-radio-button>
        <a-radio-button value="passive_aggressive">PA</a-radio-button>
      </a-radio-group>
      <a-space style="margin-top: 8px; display: flex; align-items: center;">
        <a-tag :color="cudaAvailable ? 'success' : 'warning'">
          {{ cudaAvailable ? 'CUDA: ' + cudaInfo : 'CPU 训练 (未检测到 NVIDIA GPU)' }}
        </a-tag>
        <a-typography-text type="secondary">切换模型类型后会自动保存，训练和推理将使用所选模型。</a-typography-text>
      </a-space>
    </a-card>
  </a-col>

  <!-- Hyperparameters (model-type-aware) -->
  <a-col :xs="24">
    <a-card title="Model Hyperparameters" size="small">
      <template #extra>
        <a-tag color="geekblue">{{ modelTypeLabel }} 参数</a-tag>
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
      <!-- Nearest Centroid params -->
      <a-row v-if="modelType === 'nearest_centroid'" :gutter="[24, 16]">
        <a-col :xs="24">
          <a-alert
            type="info"
            show-icon
            message="Nearest Centroid uses the current defaults. The benchmark sweep explores euclidean / balanced / cosine variants offline."
          />
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <!-- Auto Parameter Tuning -->
  <a-col :xs="24">
    <a-card title="Auto Parameter Tuning" size="small">
      <template #extra>
        <a-space>
          <a-tag color="magenta">{{ autoTuneGridSize }}×{{ autoTuneGridSize }} 方阵</a-tag>
          <a-button size="small" type="primary" :loading="autoTuneLoading" @click="runAutoTune">
            <ControlOutlined /> 开始调优
          </a-button>
        </a-space>
      </template>
      <a-alert type="info" show-icon style="margin-bottom: 12px" :message="`选择两个参数做平方搜索，颜色越深表示所选指标越高。当前按「${autoTuneMetricLabel(autoTuneMetric)}」着色。`" />
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
              <a-typography-text type="secondary" style="display: block; margin-top: 4px">数值越大，搜索越细</a-typography-text>
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
            <a-alert type="warning" show-icon message="X/Y 轴不能相同；调优结果会直接更新到当前滑块。" />
            <!-- Auto-tune Progress -->
            <div v-if="autoTuneLoading || autoTuneInProgress || autoTuneMessage || autoTuneError" style="background: #fafafa; padding: 12px; border-radius: 8px; border: 1px solid #f0f0f0;">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
                <span style="font-weight: 600; font-size: 13px;">
                  <ReloadOutlined v-if="autoTuneLoading || autoTuneInProgress" spin style="margin-right: 4px;" />
                  {{ autoTuneLoading || autoTuneInProgress ? '调优进行中' : '调优完成' }}
                </span>
                <span v-if="autoTuneLoading || autoTuneInProgress" style="font-size: 12px; color: #999;">已用 {{ autoTuneElapsed }}</span>
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
              <a-alert v-if="autoTuneError" type="error" show-icon :message="autoTuneError" style="margin-top: 8px" />
              <a-alert v-if="autoTuneJustCompleted && autoTuneBestCell" type="success" show-icon style="margin-top: 8px">
                <template #message>
                  <span style="font-weight: 600;">最佳参数：</span>
                  树数={{ autoTuneBestCell.numTrees }}，
                  深度={{ autoTuneBestCell.maxDepth }}，
                  叶样本={{ autoTuneBestCell.minSamplesLeaf }}
                  <span style="margin-left: 8px; color: #52c41a;">{{ autoTuneMetricLabel(autoTuneMetric) }}={{ autoTuneMetricFormat(autoTuneScore(autoTuneBestCell)) }}</span>
                </template>
              </a-alert>
            </div>
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
          <div style="width: 100%; aspect-ratio: 1 / 1; min-height: 420px; background: #fff; border: 1px solid #f0f0f0; border-radius: 8px; padding: 8px;">
            <VueApexCharts
              v-if="autoTuneHeatmapSeries.length > 0"
              type="heatmap"
              :height="Math.max(360, autoTuneGridSize * 64)"
              :options="autoTuneHeatmapOptions"
              :series="autoTuneHeatmapSeries"
            />
            <a-empty v-else description="点击“开始调优”生成参数方阵" style="height: 100%; display: flex; align-items: center; justify-content: center" />
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
                <a-descriptions-item :label="autoTuneMetricLabel(autoTuneMetric)">{{ autoTuneMetricFormat(autoTuneScore(autoTuneSelectedCell)) }}</a-descriptions-item>
                <a-descriptions-item label="验证集准确率">{{ (autoTuneSelectedCell.validationAccuracy * 100).toFixed(1) }}%</a-descriptions-item>
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
                <a-descriptions-item :label="autoTuneMetricLabel(autoTuneMetric)"><b>{{ autoTuneMetricFormat(autoTuneScore(autoTuneBestCell)) }}</b></a-descriptions-item>
                <a-descriptions-item label="验证集准确率">{{ (autoTuneBestCell.validationAccuracy * 100).toFixed(1) }}%</a-descriptions-item>
                <a-descriptions-item label="推理速度">{{ autoTuneMetricFormat(autoTuneBestCell.inferenceThroughput, 'inferenceThroughput') }}</a-descriptions-item>
                <a-descriptions-item label="训练/评估耗时">{{ autoTuneBestCell.trainDuration.toFixed(2) }}s / {{ autoTuneBestCell.evalDuration.toFixed(2) }}s</a-descriptions-item>
              </a-descriptions>
            </template>
            <a-empty v-else description="运行后自动选出最佳结果" />
          </a-card>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-card size="small" title="应用操作">
            <a-space direction="vertical" style="width: 100%">
              <a-button type="primary" block :disabled="!autoTuneSelectedCell" @click="applyAutoTuneCell(autoTuneSelectedCell)"><ControlOutlined /> 应用选中参数</a-button>
              <a-button block :disabled="!autoTuneBestCell" @click="applyAutoTuneCell(autoTuneBestCell)">应用最佳参数</a-button>
              <a-button block @click="emit('nav', 'model')">前往训练页</a-button>
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

  <!-- Training / Validation Split -->
  <a-col :xs="24">
    <a-card title="Training / Validation Split" size="small">
      <template #extra><a-tag color="purple">训练后会自动切分验证集，并可选做 LLM 后训练复核</a-tag></template>
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
            <div>• 若训练集打分选择"回写标签"，仅训练集会被改写，验证集只读。</div>
          </div>
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <!-- Detection Thresholds -->
  <a-col :xs="24">
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
</template>
