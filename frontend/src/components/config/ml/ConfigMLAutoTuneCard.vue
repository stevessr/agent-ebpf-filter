<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";
import {
  CheckCircleOutlined,
  ControlOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type { useConfigML } from "../../../composables/config/useConfigML";
import { useAutoTuneElapsed } from "./useAutoTuneElapsed";
import { useModelTypeDisplay } from "./useModelTypeDisplay";

const VueApexCharts = defineAsyncComponent(
  async () => (await import("vue3-apexcharts")).default as any,
) as any;
const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
const emit = defineEmits<{ (e: "nav", tab: string): void }>();
const {
  mlStatus,
  modelType,
  builtinModelCatalog,
  selectedBuiltinModel,
  modelBaseType,
  hyperParams,
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
  trainingLogs,
} = props.ml;
const { autoTuneElapsed } = useAutoTuneElapsed(autoTuneInProgress);
const autoTuneJustCompleted = computed(
  () =>
    !autoTuneInProgress.value &&
    autoTuneResponse.value &&
    autoTuneLoading.value === false,
);
const { modelTuneColumns } = useModelTypeDisplay(
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
</script>

<template>
<!-- Auto Parameter Tuning -->
  <a-col :xs="24">
    <a-card title="Auto Parameter Tuning" size="small">
      <template #extra>
        <a-space>
          <a-tag
            v-if="mlStatus.auto_tune_runtime"
            :color="mlStatus.auto_tune_runtime.closed ? 'red' : 'blue'"
          >
            queue {{ mlStatus.auto_tune_runtime.queueLen }}/{{
              mlStatus.auto_tune_runtime.queueCap
            }}
          </a-tag>
          <a-tag color="magenta">{{
            autoTuneMode === "models"
              ? `${modelTuneSelectedTypes.length || modelTuneRecommendedTypes.length} 个模型`
              : `${autoTuneGridSize}×${autoTuneGridSize} 方阵`
          }}</a-tag>
          <a-button
            size="small"
            type="primary"
            :loading="autoTuneLoading"
            @click="runAutoTune"
          >
            <ControlOutlined />
            {{ autoTuneMode === "models" ? "开始模型调优" : "开始调优" }}
          </a-button>
        </a-space>
      </template>
      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 12px"
        :message="
          autoTuneMode === 'models'
            ? `自动试训多个候选模型，按「${autoTuneMetricLabel(autoTuneMetric)}」选择最佳模型。`
            : `选择两个参数做平方搜索，颜色越深表示所选指标越高。当前按「${autoTuneMetricLabel(autoTuneMetric)}」着色。`
        "
      />
      <a-radio-group
        v-model:value="autoTuneMode"
        button-style="solid"
        style="margin-bottom: 12px"
      >
        <a-radio-button value="params">参数方阵调优</a-radio-button>
        <a-radio-button value="models">跨模型自动选择</a-radio-button>
      </a-radio-group>
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="6">
          <a-space direction="vertical" style="width: 100%">
            <div v-if="autoTuneMode === 'models'">
              <div style="font-weight: 600; margin-bottom: 6px">候选模型</div>
              <a-select
                v-model:value="modelTuneSelectedTypes"
                mode="multiple"
                show-search
                option-filter-prop="label"
                style="width: 100%"
                placeholder="默认使用推荐模型"
              >
                <a-select-option
                  v-for="item in builtinModelCatalog"
                  :key="item.value"
                  :value="item.value"
                  :label="`${item.label} ${item.base} ${item.tags?.join(' ') || ''}`"
                >
                  <a-space>
                    <span>{{ item.label }}</span>
                    <a-tag v-if="item.recommended" color="green">推荐</a-tag>
                    <a-tag color="default">{{ item.base }}</a-tag>
                  </a-space>
                </a-select-option>
              </a-select>
              <a-button
                size="small"
                type="link"
                @click="
                  modelTuneSelectedTypes = modelTuneRecommendedTypes.slice()
                "
                >选择推荐模型</a-button
              >
            </div>
            <div v-if="autoTuneMode === 'params' || modelTuneParamSearch">
              <div style="font-weight: 600; margin-bottom: 6px">X 轴参数</div>
              <a-select v-model:value="autoTuneXAxis" style="width: 100%">
                <a-select-option
                  v-for="opt in autoTuneAxisOptions"
                  :key="opt.value"
                  :value="opt.value"
                  >{{ opt.label }}</a-select-option
                >
              </a-select>
            </div>
            <div v-if="autoTuneMode === 'params' || modelTuneParamSearch">
              <div style="font-weight: 600; margin-bottom: 6px">Y 轴参数</div>
              <a-select v-model:value="autoTuneYAxis" style="width: 100%">
                <a-select-option
                  v-for="opt in autoTuneAxisOptions"
                  :key="opt.value"
                  :value="opt.value"
                  >{{ opt.label }}</a-select-option
                >
              </a-select>
            </div>
            <div v-if="autoTuneMode === 'params' || modelTuneParamSearch">
              <div style="font-weight: 600; margin-bottom: 6px">方阵大小</div>
              <a-radio-group
                v-model:value="autoTuneGridSize"
                button-style="solid"
                size="small"
              >
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
                v-if="![3, 5, 7, 9, 11, 15, 21, 31].includes(autoTuneGridSize)"
                v-model:value="autoTuneGridSize"
                :min="3"
                :max="51"
                :step="2"
                placeholder="自定义 (3-51)"
                style="width: 100%; margin-top: 4px"
              />
            </div>
            <div v-if="autoTuneMode === 'params' || modelTuneParamSearch">
              <div style="font-weight: 600; margin-bottom: 6px">颗粒度</div>
              <a-radio-group
                v-model:value="autoTuneGranularity"
                button-style="solid"
              >
                <a-radio-button :value="1">1x</a-radio-button>
                <a-radio-button :value="2">2x</a-radio-button>
                <a-radio-button :value="4">4x</a-radio-button>
              </a-radio-group>
              <a-typography-text
                type="secondary"
                style="display: block; margin-top: 4px"
                >数值越大，搜索越细</a-typography-text
              >
            </div>
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">着色指标</div>
              <a-radio-group
                v-model:value="autoTuneMetric"
                button-style="solid"
              >
                <a-radio-button value="validationAccuracy"
                  >回测准确率</a-radio-button
                >
                <a-radio-button value="balancedAccuracy"
                  >平衡准确率</a-radio-button
                >
                <a-radio-button value="allowRecall">合法召回</a-radio-button>
                <a-radio-button value="inferenceThroughput"
                  >推理速度</a-radio-button
                >
              </a-radio-group>
            </div>
            <div v-if="autoTuneMode === 'models'">
              <a-checkbox v-model:checked="modelTuneParamSearch"
                >对每个模型再做参数方阵细调</a-checkbox
              >
              <a-checkbox v-model:checked="modelTuneApplyBest"
                >完成后自动应用并保存最佳模型</a-checkbox
              >
            </div>
            <a-collapse
              v-if="autoTuneMode === 'params' || modelTuneParamSearch"
              :bordered="false"
              style="background: transparent"
            >
              <a-collapse-panel key="range" header="展开：自定义参数范围">
                <div
                  style="
                    display: flex;
                    gap: 8px;
                    align-items: center;
                    margin-bottom: 8px;
                  "
                >
                  <span style="font-size: 12px; width: 50px">{{
                    autoTuneAxisLabel(autoTuneXAxis)
                  }}</span>
                  <a-input-number
                    v-model:value="autoTuneMinX"
                    :min="1"
                    size="small"
                    placeholder="最小"
                    style="width: 70px"
                  />
                  <span style="font-size: 12px">~</span>
                  <a-input-number
                    v-model:value="autoTuneMaxX"
                    :min="1"
                    size="small"
                    placeholder="最大"
                    style="width: 70px"
                  />
                  <a-button
                    size="small"
                    type="link"
                    @click="
                      autoTuneMinX = undefined;
                      autoTuneMaxX = undefined;
                    "
                    >自动</a-button
                  >
                </div>
                <div style="display: flex; gap: 8px; align-items: center">
                  <span style="font-size: 12px; width: 50px">{{
                    autoTuneAxisLabel(autoTuneYAxis)
                  }}</span>
                  <a-input-number
                    v-model:value="autoTuneMinY"
                    :min="1"
                    size="small"
                    placeholder="最小"
                    style="width: 70px"
                  />
                  <span style="font-size: 12px">~</span>
                  <a-input-number
                    v-model:value="autoTuneMaxY"
                    :min="1"
                    size="small"
                    placeholder="最大"
                    style="width: 70px"
                  />
                  <a-button
                    size="small"
                    type="link"
                    @click="
                      autoTuneMinY = undefined;
                      autoTuneMaxY = undefined;
                    "
                    >自动</a-button
                  >
                </div>
              </a-collapse-panel>
            </a-collapse>
            <a-alert
              type="warning"
              show-icon
              :message="
                autoTuneMode === 'models'
                  ? '跨模型调优会逐个训练候选模型；若不勾选自动应用，只会展示最佳结果。'
                  : 'X/Y 轴不能相同；调优结果会直接更新到当前滑块。'
              "
            />
            <!-- Auto-tune Progress -->
            <div
              v-if="
                autoTuneLoading ||
                autoTuneInProgress ||
                autoTuneMessage ||
                autoTuneError
              "
              style="
                background: #fafafa;
                padding: 12px;
                border-radius: 8px;
                border: 1px solid #f0f0f0;
              "
            >
              <div
                style="
                  display: flex;
                  justify-content: space-between;
                  align-items: center;
                  margin-bottom: 8px;
                "
              >
                <span style="font-weight: 600; font-size: 13px">
                  <ReloadOutlined
                    v-if="autoTuneLoading || autoTuneInProgress"
                    spin
                    style="margin-right: 4px"
                  />
                  {{
                    autoTuneLoading || autoTuneInProgress
                      ? "调优进行中"
                      : "调优完成"
                  }}
                </span>
                <span
                  v-if="autoTuneLoading || autoTuneInProgress"
                  style="font-size: 12px; color: #6b7280"
                  >已用 {{ autoTuneElapsed }}</span
                >
              </div>
              <a-progress
                :percent="
                  autoTuneTotal > 0
                    ? Math.round((autoTuneCompleted / autoTuneTotal) * 100)
                    : autoTuneInProgress
                      ? 0
                      : 100
                "
                :status="
                  autoTuneError
                    ? 'exception'
                    : autoTuneInProgress
                      ? 'active'
                      : 'success'
                "
                style="margin-bottom: 4px"
              />
              <div
                style="
                  display: flex;
                  justify-content: space-between;
                  gap: 12px;
                  font-size: 12px;
                  color: #4a4a4a;
                "
              >
                <span>{{
                  autoTuneMessage ||
                  (autoTuneInProgress ? "正在评估参数组合..." : "已完成")
                }}</span>
                <span
                  >{{ autoTuneCompleted }} / {{ modelTuneProgressTotal }}
                  {{ autoTuneMode === "models" ? "模型" : "格" }}</span
                >
              </div>
              <a-alert
                v-if="autoTuneError"
                type="error"
                show-icon
                :message="autoTuneError"
                style="margin-top: 8px"
              />
              <a-alert
                v-if="
                  autoTuneMode === 'params' &&
                  autoTuneJustCompleted &&
                  autoTuneBestCell
                "
                type="success"
                show-icon
                style="margin-top: 8px"
              >
                <template #message>
                  <span style="font-weight: 600">最佳参数：</span>
                  树数={{ autoTuneBestCell.numTrees }}，深度={{
                    autoTuneBestCell.maxDepth
                  }}，叶样本={{ autoTuneBestCell.minSamplesLeaf }}
                  <span style="margin-left: 8px; color: #52c41a"
                    >{{ autoTuneMetricLabel(autoTuneMetric) }}={{
                      autoTuneMetricFormat(autoTuneScore(autoTuneBestCell))
                    }}</span
                  >
                </template>
              </a-alert>
            </div>
            <details
              v-if="autoTuneInProgress && trainingLogs.length > 0"
              style="margin-top: 4px"
            >
              <summary style="cursor: pointer; font-size: 12px; color: #4b5563">
                查看调优日志 ({{ trainingLogs.length }})
              </summary>
              <div
                style="
                  max-height: 160px;
                  overflow-y: auto;
                  background: #1e1e1e;
                  color: #d4d4d4;
                  font-family: monospace;
                  font-size: 11px;
                  padding: 8px;
                  border-radius: 4px;
                  margin-top: 4px;
                "
              >
                <div
                  v-for="(log, i) in trainingLogs.slice(-50)"
                  :key="i"
                  :style="{
                    color: log.message.includes('ERROR')
                      ? '#f48771'
                      : log.message.includes('完成') ||
                          log.message.includes('best')
                        ? '#89d185'
                        : '#d4d4d4',
                  }"
                >
                  <span style="color: #4b5563">{{ log.time }}</span>
                  {{ log.message }}
                </div>
              </div>
            </details>
          </a-space>
        </a-col>
        <a-col :xs="24" :md="18">
          <a-table
            v-if="autoTuneMode === 'models'"
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
                  <a-tag
                    v-if="record.modelType === modelTuneBestType"
                    color="success"
                    >最佳</a-tag
                  >
                  <a-tag v-if="record.recommended" color="green">推荐</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.key === 'validationAccuracy'">{{
                autoTuneMetricFormat(
                  record.validationAccuracy,
                  "validationAccuracy",
                )
              }}</template>
              <template v-else-if="column.key === 'trainAccuracy'">{{
                autoTuneMetricFormat(record.trainAccuracy, "validationAccuracy")
              }}</template>
              <template v-else-if="column.key === 'inferenceThroughput'">{{
                autoTuneMetricFormat(
                  record.inferenceThroughput,
                  "inferenceThroughput",
                )
              }}</template>
              <template v-else-if="column.key === 'hyperParams'">
                <a-space wrap>
                  <a-tag>trees={{ record.hyperParams?.numTrees ?? "—" }}</a-tag>
                  <a-tag>depth={{ record.hyperParams?.maxDepth ?? "—" }}</a-tag>
                  <a-tag
                    >leaf={{ record.hyperParams?.minSamplesLeaf ?? "—" }}</a-tag
                  >
                </a-space>
              </template>
              <template v-else-if="column.key === 'state'">
                <a-tag v-if="record.error" color="error">{{
                  record.error
                }}</a-tag>
                <a-tag v-else-if="record.applied" color="processing"
                  >已应用</a-tag
                >
                <a-tag v-else color="success">完成</a-tag>
              </template>
            </template>
          </a-table>
          <div
            v-else
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
              description="点击「开始调优」生成参数方阵"
              style="
                height: 100%;
                display: flex;
                align-items: center;
                justify-content: center;
              "
            />
          </div>
        </a-col>
      </a-row>
      <a-divider />
      <a-row v-if="autoTuneMode === 'models'" :gutter="[16, 16]">
        <a-col :xs="24" :md="16">
          <a-card size="small" title="最佳模型">
            <template v-if="modelTuneBest">
              <a-tag color="success" style="margin-bottom: 8px"
                >最优 {{ autoTuneMetricLabel(autoTuneMetric) }}</a-tag
              >
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item label="模型">{{
                  modelTuneBest.label || modelTuneBest.modelType
                }}</a-descriptions-item>
                <a-descriptions-item label="基础算法">{{
                  modelTuneBest.base
                }}</a-descriptions-item>
                <a-descriptions-item label="验证集准确率">{{
                  autoTuneMetricFormat(
                    modelTuneBest.validationAccuracy,
                    "validationAccuracy",
                  )
                }}</a-descriptions-item>
                <a-descriptions-item label="推理速度">{{
                  autoTuneMetricFormat(
                    modelTuneBest.inferenceThroughput,
                    "inferenceThroughput",
                  )
                }}</a-descriptions-item>
                <a-descriptions-item label="参数"
                  >trees={{ modelTuneBest.hyperParams?.numTrees }} / depth={{
                    modelTuneBest.hyperParams?.maxDepth
                  }}
                  / leaf={{
                    modelTuneBest.hyperParams?.minSamplesLeaf
                  }}</a-descriptions-item
                >
              </a-descriptions>
            </template>
            <a-empty v-else description="运行后自动选出最佳模型" />
          </a-card>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-card size="small" title="应用操作">
            <a-space direction="vertical" style="width: 100%">
              <a-button
                type="primary"
                block
                :disabled="!modelTuneBest"
                @click="applyModelTuneBest"
                ><ControlOutlined /> 应用最佳模型配置</a-button
              >
              <a-button block @click="emit('nav', 'model')"
                >前往训练页</a-button
              >
            </a-space>
          </a-card>
        </a-col>
      </a-row>
      <a-row v-else :gutter="[16, 16]">
        <a-col :xs="24" :md="8">
          <a-card size="small" title="当前选中">
            <template v-if="autoTuneSelectedCell">
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item
                  :label="autoTuneAxisLabel(autoTuneXAxis)"
                  >{{ autoTuneSelectedCell.xValue }}</a-descriptions-item
                >
                <a-descriptions-item
                  :label="autoTuneAxisLabel(autoTuneYAxis)"
                  >{{ autoTuneSelectedCell.yValue }}</a-descriptions-item
                >
                <a-descriptions-item
                  :label="autoTuneMetricLabel(autoTuneMetric)"
                  >{{
                    autoTuneMetricFormat(autoTuneScore(autoTuneSelectedCell))
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="验证集准确率"
                  >{{
                    (autoTuneSelectedCell.validationAccuracy * 100).toFixed(1)
                  }}%</a-descriptions-item
                >
              </a-descriptions>
            </template>
            <a-empty v-else description="暂无选中项" />
          </a-card>
        </a-col>
        <a-col :xs="24" :md="8">
          <a-card size="small" title="最佳结果">
            <template v-if="autoTuneBestCell">
              <a-tag color="success" style="margin-bottom: 8px"
                >最优 {{ autoTuneMetricLabel(autoTuneMetric) }}</a-tag
              >
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item
                  :label="autoTuneAxisLabel(autoTuneXAxis)"
                  >{{ autoTuneBestCell.xValue }}</a-descriptions-item
                >
                <a-descriptions-item
                  :label="autoTuneAxisLabel(autoTuneYAxis)"
                  >{{ autoTuneBestCell.yValue }}</a-descriptions-item
                >
                <a-descriptions-item
                  :label="autoTuneMetricLabel(autoTuneMetric)"
                  ><b>{{
                    autoTuneMetricFormat(autoTuneScore(autoTuneBestCell))
                  }}</b></a-descriptions-item
                >
                <a-descriptions-item label="验证集准确率"
                  >{{
                    (autoTuneBestCell.validationAccuracy * 100).toFixed(1)
                  }}%</a-descriptions-item
                >
                <a-descriptions-item label="推理速度">{{
                  autoTuneMetricFormat(
                    autoTuneBestCell.inferenceThroughput,
                    "inferenceThroughput",
                  )
                }}</a-descriptions-item>
                <a-descriptions-item label="训练/评估耗时"
                  >{{ autoTuneBestCell.trainDuration.toFixed(2) }}s /
                  {{
                    autoTuneBestCell.evalDuration.toFixed(2)
                  }}s</a-descriptions-item
                >
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
                ><ControlOutlined /> 应用选中参数</a-button
              >
              <a-button
                block
                :disabled="!autoTuneBestCell"
                @click="applyAutoTuneCell(autoTuneBestCell)"
                >应用最佳参数</a-button
              >
              <a-button block @click="emit('nav', 'model')"
                >前往训练页</a-button
              >
            </a-space>
          </a-card>
        </a-col>
      </a-row>
      <div
        v-if="modelTuneResponse"
        style="
          margin-top: 12px;
          padding: 8px 12px;
          background: #f6ffed;
          border: 1px solid #b7eb8f;
          border-radius: 6px;
          font-size: 12px;
        "
      >
        <CheckCircleOutlined style="color: #52c41a; margin-right: 6px" />
        共评估 <b>{{ modelTuneResponse.candidates.length }}</b> 个候选模型，样本
        <b>{{ modelTuneResponse.sampleCount }}</b
        >，最佳模型
        <b>{{
          modelTuneResponse.best?.label ||
          modelTuneResponse.best?.modelType ||
          "—"
        }}</b
        >， {{ autoTuneMetricLabel(modelTuneResponse.metric) }}
        <b>{{
          modelTuneResponse.best
            ? autoTuneMetricFormat(
                modelTuneResponse.best.score,
                modelTuneResponse.metric,
              )
            : "—"
        }}</b
        >，总用时 <b>{{ modelTuneResponse.totalDuration.toFixed(1) }}s</b>
      </div>
      <div
        v-if="autoTuneMode === 'params' && autoTuneResponse"
        style="
          margin-top: 12px;
          padding: 8px 12px;
          background: #f6ffed;
          border: 1px solid #b7eb8f;
          border-radius: 6px;
          font-size: 12px;
        "
      >
        <CheckCircleOutlined style="color: #52c41a; margin-right: 6px" />
        共评估 <b>{{ autoTuneResponse.cells.length }}</b> 个参数组合（{{
          autoTuneResponse.gridSize
        }}×{{ autoTuneResponse.gridSize }} 方阵，颗粒度
        {{ autoTuneGranularityLabel(autoTuneResponse.granularity) }}），样本
        <b>{{ autoTuneResponse.sampleCount }}</b
        >，验证集 <b>{{ autoTuneResponse.validationCount }}</b
        >，总用时 <b>{{ autoTuneResponse.totalDuration.toFixed(1) }}s</b>
      </div>
    </a-card>
  </a-col>
</template>
