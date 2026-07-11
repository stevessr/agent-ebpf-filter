<script setup lang="ts">
import { computed } from "vue";
import type { useConfigML } from "../../../composables/config/useConfigML";
import { mlModelCategoryColor } from "../../../data/mlModelCatalog";

const props = defineProps<{ ml: ReturnType<typeof useConfigML> }>();
const {
  modelType,
  builtinModelCatalog,
  selectedBuiltinModel,
  modelBaseType,
  hyperParams,
  modelTuneSelectedTypes,
  saveMLModelType,
} = props.ml;
const modelCatalogGroups = computed(() => {
  const groups = new Map<string, typeof builtinModelCatalog.value>();
  for (const item of builtinModelCatalog.value) {
    const key = item.category || "其他模型";
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)?.push(item);
  }
  return Array.from(groups.entries()).map(([category, models]) => ({
    category,
    models,
  }));
});
const modelTypeLabel = computed(
  () => selectedBuiltinModel.value?.label || modelType.value,
);
const modelTypeTagColor = computed(() =>
  mlModelCategoryColor(
    selectedBuiltinModel.value?.category,
    modelBaseType.value,
  ),
);
const selectModelCategory = (category: string) => {
  modelTuneSelectedTypes.value = builtinModelCatalog.value
    .filter((item) => item.category === category)
    .map((item) => item.value);
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
              <div style="font-weight: 600; margin-bottom: 6px">
                Active Model
              </div>
              <a-select
                v-model:value="modelType"
                show-search
                option-filter-prop="label"
                style="width: 100%"
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
            </div>
            <a-descriptions :column="1" size="small" bordered>
              <a-descriptions-item label="基础算法">{{
                selectedBuiltinModel?.base || modelBaseType
              }}</a-descriptions-item>
              <a-descriptions-item label="当前参数"
                >trees={{ hyperParams.numTrees }} / depth={{
                  hyperParams.maxDepth
                }}
                / leaf={{ hyperParams.minSamplesLeaf }}</a-descriptions-item
              >
              <a-descriptions-item label="说明">{{
                selectedBuiltinModel?.description || "本地模型配置"
              }}</a-descriptions-item>
            </a-descriptions>
          </a-space>
        </a-col>
        <a-col :xs="24" :lg="14">
          <div style="font-weight: 600; margin-bottom: 6px">Model Catalog</div>
          <a-row :gutter="[8, 8]">
            <a-col
              v-for="group in modelCatalogGroups"
              :key="group.category"
              :xs="24"
              :md="12"
            >
              <a-card
                size="small"
                :title="group.category"
                :body-style="{ padding: '8px' }"
              >
                <a-space wrap size="small">
                  <a-tag
                    v-for="item in group.models"
                    :key="item.value"
                    :color="
                      item.value === modelType
                        ? 'processing'
                        : mlModelCategoryColor(item.category, item.base)
                    "
                  >
                    {{ item.label }}
                  </a-tag>
                </a-space>
                <a-button
                  size="small"
                  type="link"
                  @click="selectModelCategory(group.category)"
                  >选择本类参与调优</a-button
                >
              </a-card>
            </a-col>
          </a-row>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
