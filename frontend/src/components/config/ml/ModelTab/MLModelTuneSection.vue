<script setup lang="ts">
import { computed } from "vue";
import { ControlOutlined } from "@ant-design/icons-vue";
import type { useConfigML } from "../../../../composables/config/useConfigML";
import ConfigMLModelCatalogCard from "../ConfigMLModelCatalogCard.vue";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();

const modelTuneColumns = [
  { title: "模型", dataIndex: "label", key: "label" },
  { title: "基础算法", dataIndex: "base", key: "base" },
  {
    title: "验证准确率",
    dataIndex: "validationAccuracy",
    key: "validationAccuracy",
  },
  { title: "训练准确率", dataIndex: "trainAccuracy", key: "trainAccuracy" },
  {
    title: "推理速度",
    dataIndex: "inferenceThroughput",
    key: "inferenceThroughput",
  },
  { title: "参数", dataIndex: "hyperParams", key: "hyperParams" },
  { title: "状态", dataIndex: "state", key: "state" },
];

const {
  modelType,
  builtinModelCatalog,
  hyperParams,
  mlStatus,
  autoTuneMetric,
  autoTuneMetricLabel,
  autoTuneMetricFormat,
  autoTuneLoading,
  autoTuneInProgress,
  autoTuneCompleted,
  autoTuneTotal,
  autoTuneMessage,
  autoTuneError,
  modelTuneSelectedTypes,
  modelTuneParamSearch,
  modelTuneApplyBest,
  modelTuneResponse,
  modelTuneBest,
  modelTuneRecommendedTypes,
  trainWithParams,
  runModelTune,
  applyModelTuneBest,
} = props.ml;

const trainingReadinessBlocked = computed(() => {
  const readiness = mlStatus.value.training_readiness;
  return Boolean(readiness && !readiness.ready);
});
const modelTuneBestType = computed(() => modelTuneBest.value?.modelType || "");
const modelTuneProgressTotal = computed(
  () =>
    autoTuneTotal.value ||
    modelTuneSelectedTypes.value.length ||
    modelTuneRecommendedTypes.value.length,
);

const selectRecommendedModels = () => {
  modelTuneSelectedTypes.value = modelTuneRecommendedTypes.value.slice();
};
</script>

<template>
  <ConfigMLModelCatalogCard :ml="props.ml" />

  <!-- Multi-type auto tuning -->
  <a-col :xs="24">
    <a-card title="Multi-type Model Auto Tuning" size="small">
      <template #extra>
        <a-space>
          <a-tag color="magenta"
            >{{
              modelTuneSelectedTypes.length || modelTuneRecommendedTypes.length
            }}
            个候选</a-tag
          >
          <a-button
            size="small"
            type="primary"
            :loading="autoTuneLoading"
            @click="runModelTune"
          >
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
              <a-space wrap style="margin-top: 6px">
                <a-button
                  size="small"
                  type="link"
                  @click="selectRecommendedModels"
                  >选择推荐模型</a-button
                >
                <a-button
                  size="small"
                  type="link"
                  @click="modelTuneSelectedTypes = []"
                  >清空选择</a-button
                >
              </a-space>
            </div>
            <div>
              <div style="font-weight: 600; margin-bottom: 6px">排序指标</div>
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
            <a-checkbox v-model:checked="modelTuneParamSearch"
              >每个模型再做参数方阵细调</a-checkbox
            >
            <a-checkbox v-model:checked="modelTuneApplyBest"
              >完成后自动应用并保存最佳模型</a-checkbox
            >
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
                  gap: 8px;
                  font-size: 12px;
                  color: #4a4a4a;
                  margin-bottom: 6px;
                "
              >
                <span>{{
                  autoTuneMessage ||
                  (autoTuneInProgress ? "正在评估候选模型..." : "已完成")
                }}</span>
                <span
                  >{{ autoTuneCompleted }} /
                  {{ modelTuneProgressTotal }} 模型</span
                >
              </div>
              <a-progress
                :percent="
                  modelTuneProgressTotal > 0
                    ? Math.round(
                        (autoTuneCompleted / modelTuneProgressTotal) * 100,
                      )
                    : 0
                "
                :status="
                  autoTuneError
                    ? 'exception'
                    : autoTuneInProgress
                      ? 'active'
                      : 'success'
                "
              />
              <a-alert
                v-if="autoTuneError"
                type="error"
                show-icon
                :message="autoTuneError"
                style="margin-top: 8px"
              />
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
        </a-col>
      </a-row>
      <a-divider />
      <a-row :gutter="[16, 16]">
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
              <a-button
                block
                :disabled="!modelTuneBest || trainingReadinessBlocked"
                @click="trainWithParams"
                >应用后重新训练当前模型</a-button
              >
            </a-space>
          </a-card>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
