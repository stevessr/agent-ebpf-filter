<script setup lang="ts">
import { ref, computed, defineAsyncComponent } from "vue";
import {
  ReloadOutlined,
  CheckCircleOutlined,
  ControlOutlined,
} from "@ant-design/icons-vue";
import type { useConfigML } from "../../../composables/config/useConfigML";
import { useAutoTuneElapsed } from "./useAutoTuneElapsed";
import { useModelTypeDisplay } from "./useModelTypeDisplay";
import FeatureSerializationDetail from "./FeatureSerializationDetail.vue";
import ConfigMLAutoTuneCard from "./ConfigMLAutoTuneCard.vue";

const VueApexCharts = defineAsyncComponent(
  async () => (await import("vue3-apexcharts")).default as any,
) as any;
const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
const emit = defineEmits<{ (e: "nav", tab: string): void }>();
const {
  modelType,
  builtinModelCatalog,
  selectedBuiltinModel,
  modelBaseType,
  cudaAvailable,
  cudaInfo,
  hyperParams,
  mlThresholds,
  mlTrainingConfig,
  autoTuneMode,
  modelTuneSelectedTypes,
  modelTuneParamSearch,
  modelTuneApplyBest,
  modelTuneResponse,
  modelTuneBest,
  modelTuneRecommendedTypes,
  autoTuneXAxis,
  autoTuneYAxis,
  autoTuneGridSize,
  autoTuneGranularity,
  autoTuneMetric,
  autoTuneMinX,
  autoTuneMaxX,
  autoTuneMinY,
  autoTuneMaxY,
  autoTuneAxisOptions,
  autoTuneLoading,
  autoTuneInProgress,
  autoTuneCompleted,
  autoTuneTotal,
  autoTuneMessage,
  autoTuneError,
  autoTuneResponse,
  autoTuneSelectedCell,
  autoTuneAxisLabel,
  autoTuneMetricLabel,
  autoTuneMetricFormat,
  autoTuneGranularityLabel,
  autoTuneScore,
  autoTuneHeatmapOptions,
  autoTuneHeatmapSeries,
  autoTuneBestCell,
  runAutoTune,
  applyAutoTuneCell,
  applyModelTuneBest,
  saveMLThresholds,
  saveMLModelType,
  trainingLogs,
} = props.ml;
const { autoTuneElapsed } = useAutoTuneElapsed(autoTuneInProgress);
const autoTuneJustCompleted = computed(
  () =>
    !autoTuneInProgress.value &&
    autoTuneResponse.value &&
    autoTuneLoading.value === false,
);
const {
  modelTypeLabel,
  modelTypeTagColor,
  modelTypeDescription,
  modelBaseLabel,
  modelCatalogGroups,
  isTreeLikeModel,
  isLinearModel,
  isPrototypeModel,
  hasCompactParams,
  modelTuneColumns,
} = useModelTypeDisplay(
  modelType,
  selectedBuiltinModel,
  builtinModelCatalog,
  modelBaseType,
);
const modelTuneBestType = computed(() => modelTuneBest.value?.modelType || "");
const modelTuneProgressTotal = computed(
  () =>
    autoTuneTotal.value ||
    (autoTuneMode.value === "models"
      ? modelTuneSelectedTypes.value.length
      : autoTuneGridSize.value * autoTuneGridSize.value),
);

const numTreesMin = computed(() => modelBaseType.value === "graph_learning" ? 8 : 5);
const numTreesMax = computed(() => modelBaseType.value === "graph_learning" ? 256 : 200);

const maxDepthMin = computed(() => modelBaseType.value === "graph_learning" ? 1 : 3);
const maxDepthMax = computed(() => modelBaseType.value === "graph_learning" ? 10 : 20);

const minSamplesLeafMin = computed(() => modelBaseType.value === "graph_learning" ? 10 : 1);
const minSamplesLeafMax = computed(() => modelBaseType.value === "graph_learning" ? 500 : 50);
</script>

<template>
  <!-- Model Type Selector -->
  <a-col :xs="24">
    <a-card size="small">
      <template #title>
        <span>Model Type</span>
        <a-tag :color="modelTypeTagColor" style="margin-left: 8px">
          {{ modelTypeLabel }}
        </a-tag>
      </template>
      <a-select
        v-model:value="modelType"
        show-search
        option-filter-prop="label"
        style="width: 100%; max-width: 760px"
        @change="saveMLModelType"
      >
        <a-select-opt-group
          v-for="group in modelCatalogGroups"
          :key="group.category"
          :label="group.category"
        >
          <a-select-option
            v-for="item in group.models"
            :key="item.value"
            :value="item.value"
            :label="`${item.label} ${item.value} ${item.tags?.join(' ') || ''}`"
          >
            <a-space>
              <span>{{ item.label }}</span>
              <a-tag v-if="item.recommended" color="green">推荐</a-tag>
              <a-tag color="default">{{ item.base }}</a-tag>
              <a-tag
                v-for="tag in item.tags || []"
                :key="`${item.value}-${tag}`"
                color="blue"
                >{{ tag }}</a-tag
              >
            </a-space>
          </a-select-option>
        </a-select-opt-group>
      </a-select>
      <a-alert
        type="info"
        show-icon
        style="margin-top: 10px"
        :message="modelTypeDescription"
        :description="`基础算法: ${modelBaseLabel}；切换内置模型会写入该 profile 的默认参数，随后可继续手动调参。`"
      />
      <a-space
        style="
          margin-top: 8px;
          display: flex;
          align-items: center;
          flex-wrap: wrap;
        "
      >
        <a-tag :color="cudaAvailable ? 'success' : 'warning'">
          {{
            cudaAvailable
              ? "CUDA: " + cudaInfo
              : "CPU 训练 (未检测到 NVIDIA GPU)"
          }}
        </a-tag>
        <a-typography-text type="secondary"
          >切换模型类型后会自动保存，训练和推理将使用所选模型。</a-typography-text
        >
      </a-space>
    </a-card>
  </a-col>

  <!-- Hyperparameters (model-type-aware) -->
  <a-col :xs="24">
    <a-card title="Model Hyperparameters" size="small">
      <template #extra>
        <a-tag color="geekblue">{{ modelTypeLabel }} 参数</a-tag>
      </template>
      <!-- Random Forest / GNN params -->
      <a-row v-if="isTreeLikeModel" :gutter="[24, 16]">
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">{{
            modelBaseType === "extra_trees"
              ? "Num Extra Trees (极随机树数量)"
              : modelBaseType === "graph_learning"
              ? "Hidden Dimension (图特征隐层维度)"
              : "Num Trees (树的数量)"
          }}</span>
          <a-slider
            v-model:value="hyperParams.numTrees"
            :min="numTreesMin"
            :max="numTreesMax"
            :step="1"
          />
          <a-input-number
            v-model:value="hyperParams.numTrees"
            :min="numTreesMin"
            :max="numTreesMax"
            size="small"
            style="width: 100%"
          />
          <div style="font-size: 11px; color: #6b7280">
            {{
              modelBaseType === "graph_learning"
                ? "图卷积节点投影表示特征维度。推荐 32-128"
                : "更多树 = 更高精度但更慢训练。推荐 31-101"
            }}
          </div>
        </a-col>
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">{{
            modelBaseType === "graph_learning"
              ? "Num Layers (消息传递网络层数)"
              : "Max Depth (最大深度)"
          }}</span>
          <a-slider
            v-model:value="hyperParams.maxDepth"
            :min="maxDepthMin"
            :max="maxDepthMax"
            :step="1"
          />
          <a-input-number
            v-model:value="hyperParams.maxDepth"
            :min="maxDepthMin"
            :max="maxDepthMax"
            size="small"
            style="width: 100%"
          />
          <div style="font-size: 11px; color: #6b7280">
            {{
              modelBaseType === "graph_learning"
                ? "图神经网络的迭代步数/层数。推荐 2-4"
                : "更深的树 = 更复杂决策边界。推荐 6-12"
            }}
          </div>
        </a-col>
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">{{
            modelBaseType === "graph_learning"
              ? "Epochs (网络迭代训练轮数)"
              : "Min Samples Leaf (叶节点最小样本)"
          }}</span>
          <a-slider
            v-model:value="hyperParams.minSamplesLeaf"
            :min="minSamplesLeafMin"
            :max="minSamplesLeafMax"
            :step="1"
          />
          <a-input-number
            v-model:value="hyperParams.minSamplesLeaf"
            :min="minSamplesLeafMin"
            :max="minSamplesLeafMax"
            size="small"
            style="width: 100%"
          />
          <div style="font-size: 11px; color: #6b7280">
            {{
              modelBaseType === "graph_learning"
                ? "GNN 训练完整数据集的轮数。推荐 100-300"
                : "更大值防止过拟合。推荐 2-10"
            }}
          </div>
        </a-col>
      </a-row>
      <!-- KNN params -->
      <a-row v-if="modelBaseType === 'knn'" :gutter="[24, 16]">
        <a-col :xs="24" :md="12">
          <span style="font-weight: 600">K (邻居数量)</span>
          <a-slider
            v-model:value="hyperParams.numTrees"
            :min="1"
            :max="31"
            :step="2"
          />
          <a-input-number
            v-model:value="hyperParams.numTrees"
            :min="1"
            :max="31"
            size="small"
            style="width: 100%"
          />
          <div style="font-size: 11px; color: #6b7280">
            较小的 K 对噪声敏感，较大的 K 决策边界更平滑。推荐 3-11
          </div>
        </a-col>
        <a-col :xs="24" :md="12">
          <span style="font-weight: 600">Distance (距离度量)</span>
          <a-select v-model:value="hyperParams.maxDepth" style="width: 100%">
            <a-select-option :value="8">Euclidean</a-select-option>
            <a-select-option :value="12">Manhattan</a-select-option>
            <a-select-option :value="16">Cosine</a-select-option>
          </a-select>
          <div style="font-size: 11px; color: #6b7280; margin-top: 8px">
            Euclidean 适合连续特征，Manhattan 适合高维稀疏数据
          </div>
        </a-col>
      </a-row>
      <!-- Logistic Regression params -->
      <a-row v-if="isLinearModel" :gutter="[24, 16]">
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">{{
            modelBaseType === "ridge"
              ? "Alpha ×100 (正则强度)"
              : "Learning Rate / C 编码"
          }}</span>
          <a-slider
            v-model:value="hyperParams.numTrees"
            :min="1"
            :max="100"
            :step="1"
          />
          <a-input-number
            v-model:value="hyperParams.numTrees"
            :min="1"
            :max="100"
            size="small"
            style="width: 100%"
            :formatter="(v: number) => (v / 1000).toFixed(3)"
            :parser="(v: string) => parseFloat(v) * 1000"
          />
          <div style="font-size: 11px; color: #6b7280">
            较小值收敛更稳定。推荐 0.005-0.05
          </div>
        </a-col>
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">Regularization (正则化)</span>
          <a-select v-model:value="hyperParams.maxDepth" style="width: 100%">
            <a-select-option :value="8">L2 (Ridge)</a-select-option>
            <a-select-option :value="12">L1 (Lasso)</a-select-option>
            <a-select-option :value="4">None</a-select-option>
          </a-select>
          <div style="font-size: 11px; color: #6b7280; margin-top: 8px">
            L2 防止大权重，L1 产生稀疏特征选择
          </div>
        </a-col>
        <a-col :xs="24" :md="8">
          <span style="font-weight: 600">Max Iterations (最大迭代)</span>
          <a-slider
            v-model:value="hyperParams.minSamplesLeaf"
            :min="100"
            :max="5000"
            :step="100"
          />
          <a-input-number
            v-model:value="hyperParams.minSamplesLeaf"
            :min="100"
            :max="5000"
            size="small"
            style="width: 100%"
          />
          <div style="font-size: 11px; color: #6b7280">
            SGD 最大迭代数。推荐 500-2000
          </div>
        </a-col>
      </a-row>
      <!-- Nearest Centroid params -->
      <a-row v-if="isPrototypeModel" :gutter="[24, 16]">
        <a-col :xs="24">
          <a-alert
            type="info"
            show-icon
            :message="`${modelTypeLabel} 使用距离/先验编码参数：numTrees 控制 metric，maxDepth 控制 prior，内置变体已预设常用组合。`"
          />
        </a-col>
      </a-row>
      <a-row
        v-if="hasCompactParams"
        :gutter="[24, 16]"
        style="margin-top: 12px"
      >
        <a-col :xs="24">
          <a-alert
            type="success"
            show-icon
            :message="`${modelTypeLabel} 已加载本地内置 profile`"
            :description="`当前基础算法为 ${modelBaseLabel}，默认参数 numTrees=${hyperParams.numTrees} / maxDepth=${hyperParams.maxDepth} / minSamplesLeaf=${hyperParams.minSamplesLeaf}。`"
          />
        </a-col>
      </a-row>
    </a-card>
  </a-col>

  <ConfigMLAutoTuneCard :ml="props.ml" @nav="emit('nav', $event)" />

  <!-- Feature Serialization (特征序列化展示) -->
  <FeatureSerializationDetail
    :model-base-type="modelBaseType"
    :model-type-label="modelTypeLabel"
  />

  <!-- Training / Validation Split -->
  <a-col :xs="24">

    <a-card title="Training / Validation Split" size="small">
      <template #extra
        ><a-tag color="purple"
          >训练后会自动切分验证集，并可选做 LLM 后训练复核</a-tag
        ></template
      >
      <a-row :gutter="[24, 16]">
        <a-col :xs="24" :md="12">
          <span>Validation Split Ratio</span>
          <a-slider
            v-model:value="mlTrainingConfig.validationSplitRatio"
            :min="0.1"
            :max="0.4"
            :step="0.05"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlTrainingConfig.validationSplitRatio"
            :min="0.1"
            :max="0.4"
            :step="0.05"
            size="small"
            style="width: 100%"
          />
        </a-col>
        <a-col :xs="24" :md="12">
          <div
            style="
              font-size: 13px;
              color: #4a4a4a;
              line-height: 1.7;
              margin-top: 24px;
            "
          >
            <div>
              • 训练时会先随机切分训练集 / 验证集，再分别记录 train / validation
              accuracy。
            </div>
            <div>• 后训练阶段可用外部 OpenAI 风格 LLM 对验证集做批量复核。</div>
            <div>
              • 若训练集打分选择"回写标签"，仅训练集会被改写，验证集只读。
            </div>
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
          <a-slider
            v-model:value="mlThresholds.blockConfidenceThreshold"
            :min="0.5"
            :max="1.0"
            :step="0.05"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlThresholds.blockConfidenceThreshold"
            :min="0.5"
            :max="1.0"
            :step="0.05"
            size="small"
            style="width: 100%"
          />
        </a-col>
        <a-col :xs="24" :md="8">
          <span>ML Minimum Confidence</span>
          <a-slider
            v-model:value="mlThresholds.mlMinConfidence"
            :min="0.3"
            :max="1.0"
            :step="0.05"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlThresholds.mlMinConfidence"
            :min="0.3"
            :max="1.0"
            :step="0.05"
            size="small"
            style="width: 100%"
          />
        </a-col>
        <a-col :xs="24" :md="8">
          <span>Rule Override Priority</span>
          <a-slider
            v-model:value="mlThresholds.ruleOverridePriority"
            :min="0"
            :max="200"
            :step="10"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlThresholds.ruleOverridePriority"
            :min="0"
            :max="200"
            :step="10"
            size="small"
            style="width: 100%"
          />
        </a-col>
        <a-col :xs="24" :md="8">
          <span>Low Anomaly Threshold (below = normal)</span>
          <a-slider
            v-model:value="mlThresholds.lowAnomalyThreshold"
            :min="0.0"
            :max="0.5"
            :step="0.05"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlThresholds.lowAnomalyThreshold"
            :min="0.0"
            :max="0.5"
            :step="0.05"
            size="small"
            style="width: 100%"
          />
        </a-col>
        <a-col :xs="24" :md="8">
          <span>High Anomaly Threshold (above = alert)</span>
          <a-slider
            v-model:value="mlThresholds.highAnomalyThreshold"
            :min="0.5"
            :max="1.0"
            :step="0.05"
            @afterChange="saveMLThresholds"
          />
          <a-input-number
            v-model:value="mlThresholds.highAnomalyThreshold"
            :min="0.5"
            :max="1.0"
            :step="0.05"
            size="small"
            style="width: 100%"
          />
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
