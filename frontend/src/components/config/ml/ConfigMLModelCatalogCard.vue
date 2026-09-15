<script setup lang="ts">
import { computed } from "vue";
import type { MLBuiltinModelCatalogItem } from "../../../types/config";
import type { useConfigML } from "../../../composables/config/useConfigML";
import { mlModelCategoryColor } from "../../../data/mlModelCatalog";
import {
  modelFamilyLabel,
  modelFeatureClass,
  modelFeatureClassLabel,
  modelFeatureProfiles,
} from "../../../data/mlModelTaxonomy";

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
    const key = modelFamilyLabel(item);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)?.push(item);
  }
  return Array.from(groups.entries()).map(([family, models]) => ({
    family,
    models,
  }));
});

const featureCatalogGroups = computed(() => {
  const groups = new Map<
    string,
    { featureClass: string; label: string; models: MLBuiltinModelCatalogItem[] }
  >();
  for (const item of builtinModelCatalog.value) {
    const featureClass = modelFeatureClass(item);
    const label = modelFeatureClassLabel(item);
    if (!groups.has(featureClass)) {
      groups.set(featureClass, { featureClass, label, models: [] });
    }
    groups.get(featureClass)?.models.push(item);
  }
  return Array.from(groups.values());
});

const modelTypeLabel = computed(
  () => selectedBuiltinModel.value?.label || modelType.value,
);
const selectedFamilyLabel = computed(() =>
  selectedBuiltinModel.value
    ? modelFamilyLabel(selectedBuiltinModel.value)
    : "其他模型",
);
const selectedFeatureLabel = computed(() =>
  selectedBuiltinModel.value
    ? modelFeatureClassLabel(selectedBuiltinModel.value)
    : "128维表格上下文",
);
const selectedFeatureProfiles = computed(() =>
  selectedBuiltinModel.value
    ? modelFeatureProfiles(selectedBuiltinModel.value)
    : ["tabular"],
);
const modelTypeTagColor = computed(() =>
  mlModelCategoryColor(selectedFamilyLabel.value, modelBaseType.value),
);

const selectModelFamily = (family: string) => {
  modelTuneSelectedTypes.value = builtinModelCatalog.value
    .filter((item) => modelFamilyLabel(item) === family)
    .map((item) => item.value);
};

const selectModelFeature = (featureClass: string) => {
  modelTuneSelectedTypes.value = builtinModelCatalog.value
    .filter((item) => modelFeatureClass(item) === featureClass)
    .map((item) => item.value);
};

const visibleTags = (item: MLBuiltinModelCatalogItem) =>
  (item.tags || []).filter(
    (tag) =>
      !tag.startsWith("family:") &&
      !tag.startsWith("feature:") &&
      !tag.startsWith("feature-profile:"),
  );
</script>

<template>
  <!-- Multi-model management -->
  <a-col :xs="24">
    <a-card title="Multi-model Management" size="small">
      <template #extra>
        <a-space size="small">
          <a-tag :color="modelTypeTagColor">当前：{{ modelTypeLabel }}</a-tag>
          <a-tag color="cyan">{{ selectedFamilyLabel }}</a-tag>
          <a-tag color="geekblue">{{ selectedFeatureLabel }}</a-tag>
        </a-space>
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
                  :key="group.family"
                  :label="group.family"
                >
                  <a-select-option
                    v-for="item in group.models"
                    :key="item.value"
                    :value="item.value"
                    :label="`${item.label} ${item.value} ${modelFeatureClassLabel(item)} ${visibleTags(item).join(' ')}`"
                  >
                    <a-space>
                      <span>{{ item.label }}</span>
                      <a-tag v-if="item.recommended" color="green">推荐</a-tag>
                      <a-tag color="geekblue">{{ modelFeatureClassLabel(item) }}</a-tag>
                      <a-tag color="default">{{ item.base }}</a-tag>
                    </a-space>
                  </a-select-option>
                </a-select-opt-group>
              </a-select>
            </div>
            <a-descriptions :column="1" size="small" bordered>
              <a-descriptions-item label="模型家族">{{
                selectedFamilyLabel
              }}</a-descriptions-item>
              <a-descriptions-item label="特征表示">{{
                selectedFeatureLabel
              }}</a-descriptions-item>
              <a-descriptions-item label="特征 Profile">
                <a-space wrap size="small">
                  <a-tag
                    v-for="profile in selectedFeatureProfiles"
                    :key="profile"
                    color="blue"
                    >{{ profile }}</a-tag
                  >
                </a-space>
              </a-descriptions-item>
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
          <div style="font-weight: 600; margin-bottom: 6px">
            Feature Classification
          </div>
          <a-space wrap size="small" style="margin-bottom: 12px">
            <a-button
              v-for="feature in featureCatalogGroups"
              :key="feature.featureClass"
              size="small"
              @click="selectModelFeature(feature.featureClass)"
            >
              {{ feature.label }} ({{ feature.models.length }})
            </a-button>
          </a-space>

          <div style="font-weight: 600; margin-bottom: 6px">
            Model Families
          </div>
          <a-row :gutter="[8, 8]">
            <a-col
              v-for="group in modelCatalogGroups"
              :key="group.family"
              :xs="24"
              :md="12"
            >
              <a-card
                size="small"
                :title="group.family"
                :body-style="{ padding: '8px' }"
              >
                <a-space direction="vertical" size="small" style="width: 100%">
                  <div v-for="item in group.models" :key="item.value">
                    <a-space wrap size="small">
                      <a-tag
                        :color="
                          item.value === modelType
                            ? 'processing'
                            : mlModelCategoryColor(group.family, item.base)
                        "
                      >
                        {{ item.label }}
                      </a-tag>
                      <a-tag color="geekblue">{{ modelFeatureClassLabel(item) }}</a-tag>
                    </a-space>
                  </div>
                </a-space>
                <a-button
                  size="small"
                  type="link"
                  @click="selectModelFamily(group.family)"
                  >选择本家族参与调优</a-button
                >
              </a-card>
            </a-col>
          </a-row>
        </a-col>
      </a-row>
    </a-card>
  </a-col>
</template>
