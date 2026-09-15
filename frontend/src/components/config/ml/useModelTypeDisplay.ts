import { computed } from "vue";
import type { Ref } from "vue";
import {
  modelFamilyColor,
  modelFamilyLabel,
  modelFeatureClassLabel,
} from "../../../data/mlModelTaxonomy";

interface BuiltinModelItem {
  value: string;
  label: string;
  base: string;
  category?: string;
  description?: string;
  recommended?: boolean;
  tags?: string[];
}

interface AttackImpactMetrics {
  scoredSamples?: number;
  benignSamples?: number;
  attackSamples?: number;
  highImpactSamples?: number;
  intrusionSamples?: number;
  destructionSamples?: number;
  exfiltrationSamples?: number;
  persistenceSamples?: number;
  attackRecall?: number;
  highImpactRecall?: number;
  intrusionRecall?: number;
  destructionRecall?: number;
  exfiltrationRecall?: number;
  persistenceRecall?: number;
  catastrophicMissRate?: number;
  benignFalsePositiveRate?: number;
  riskWeightedRecall?: number;
  securityUtility?: number;
  threatVectorCoverage?: number;
}

export interface ModelTuneCandidate {
  modelType: string;
  label?: string;
  base?: string;
  family?: string;
  familyLabel?: string;
  featureClass?: string;
  featureProfiles?: string[];
  recommended?: boolean;
  validationAccuracy?: number;
  trainAccuracy?: number;
  inferenceThroughput?: number;
  attackMetrics?: AttackImpactMetrics;
  hyperParams?: {
    numTrees?: number;
    maxDepth?: number;
    minSamplesLeaf?: number;
  };
  error?: string;
  applied?: boolean;
}

const percent = (value?: number) =>
  Number.isFinite(value) ? `${((value || 0) * 100).toFixed(1)}%` : "—";

const vectorSummary = (metrics?: AttackImpactMetrics) => {
  if (!metrics) return "—";
  return [
    `I ${percent(metrics.intrusionRecall)}(${metrics.intrusionSamples ?? 0})`,
    `D ${percent(metrics.destructionRecall)}(${metrics.destructionSamples ?? 0})`,
    `E ${percent(metrics.exfiltrationRecall)}(${metrics.exfiltrationSamples ?? 0})`,
    `P ${percent(metrics.persistenceRecall)}(${metrics.persistenceSamples ?? 0})`,
  ].join(" / ");
};

/**
 * Computed display properties for the ML model type selector.
 * Extracted from ConfigMLParamsTab.vue.
 */
export function useModelTypeDisplay(
  modelType: Ref<string>,
  selectedBuiltinModel: Ref<BuiltinModelItem | undefined>,
  builtinModelCatalog: Ref<BuiltinModelItem[]>,
  modelBaseType: Ref<string>,
) {
  const modelTypeLabel = computed(
    () => selectedBuiltinModel.value?.label || modelType.value,
  );
  const modelFamily = computed(() =>
    selectedBuiltinModel.value
      ? modelFamilyLabel(selectedBuiltinModel.value)
      : "其他模型",
  );
  const modelFeature = computed(() =>
    selectedBuiltinModel.value
      ? modelFeatureClassLabel(selectedBuiltinModel.value)
      : "128维表格上下文",
  );
  const modelTypeTagColor = computed(() =>
    selectedBuiltinModel.value
      ? modelFamilyColor(selectedBuiltinModel.value)
      : "default",
  );
  const modelTypeDescription = computed(
    () => selectedBuiltinModel.value?.description || "本地模型配置",
  );
  const modelBaseLabel = computed(
    () => selectedBuiltinModel.value?.base || modelType.value,
  );

  const modelCatalogGroups = computed(() => {
    const groups = new Map<string, BuiltinModelItem[]>();
    for (const item of builtinModelCatalog.value) {
      const key = modelFamilyLabel(item);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key)?.push(item);
    }
    return Array.from(groups.entries()).map(([category, models]) => ({
      category,
      models,
    }));
  });

  const isTreeLikeModel = computed(
    () =>
      modelBaseType.value === "random_forest" ||
      modelBaseType.value === "extra_trees" ||
      modelBaseType.value === "graph_learning",
  );
  const isLinearModel = computed(() =>
    ["logistic", "svm", "perceptron", "passive_aggressive"].includes(
      modelBaseType.value,
    ),
  );
  const isPrototypeModel = computed(
    () => modelBaseType.value === "nearest_centroid",
  );
  const hasCompactParams = computed(() =>
    ["naive_bayes", "ridge", "adaboost", "ensemble"].includes(
      modelBaseType.value,
    ),
  );

  const modelTuneColumns = [
    { title: "模型", dataIndex: "label", key: "label" },
    { title: "模型家族", dataIndex: "familyLabel", key: "familyLabel" },
    { title: "特征分类", dataIndex: "featureClass", key: "featureClass" },
    {
      title: "安全效用",
      key: "securityUtility",
      customRender: ({ record }: { record: ModelTuneCandidate }) =>
        percent(record.attackMetrics?.securityUtility),
    },
    {
      title: "高影响召回",
      key: "highImpactRecall",
      customRender: ({ record }: { record: ModelTuneCandidate }) =>
        `${percent(record.attackMetrics?.highImpactRecall)} (${record.attackMetrics?.highImpactSamples ?? 0})`,
    },
    {
      title: "攻击向量 I/D/E/P",
      key: "attackVectors",
      customRender: ({ record }: { record: ModelTuneCandidate }) =>
        vectorSummary(record.attackMetrics),
    },
    {
      title: "灾难漏报",
      key: "catastrophicMissRate",
      customRender: ({ record }: { record: ModelTuneCandidate }) =>
        percent(record.attackMetrics?.catastrophicMissRate),
    },
    {
      title: "正常误报",
      key: "benignFalsePositiveRate",
      customRender: ({ record }: { record: ModelTuneCandidate }) =>
        percent(record.attackMetrics?.benignFalsePositiveRate),
    },
    {
      title: "验证准确率",
      dataIndex: "validationAccuracy",
      key: "validationAccuracy",
    },
    {
      title: "推理速度",
      dataIndex: "inferenceThroughput",
      key: "inferenceThroughput",
    },
    { title: "参数", dataIndex: "hyperParams", key: "hyperParams" },
    { title: "状态", dataIndex: "state", key: "state" },
  ];

  return {
    modelTypeLabel,
    modelFamily,
    modelFeature,
    modelTypeTagColor,
    modelTypeDescription,
    modelBaseLabel,
    modelCatalogGroups,
    isTreeLikeModel,
    isLinearModel,
    isPrototypeModel,
    hasCompactParams,
    modelTuneColumns,
  };
}
