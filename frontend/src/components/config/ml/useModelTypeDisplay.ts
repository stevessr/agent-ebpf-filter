import { computed } from 'vue';
import type { Ref } from 'vue';
import { mlModelCategoryColor } from '../../../data/mlModelCatalog';

interface BuiltinModelItem {
  value: string;
  label: string;
  base: string;
  category?: string;
  description?: string;
  recommended?: boolean;
  tags?: string[];
}

export interface ModelTuneCandidate {
  modelType: string;
  label?: string;
  base?: string;
  recommended?: boolean;
  validationAccuracy?: number;
  trainAccuracy?: number;
  inferenceThroughput?: number;
  hyperParams?: { numTrees?: number; maxDepth?: number; minSamplesLeaf?: number };
  error?: string;
  applied?: boolean;
}

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
  const modelTypeTagColor = computed(() =>
    mlModelCategoryColor(
      selectedBuiltinModel.value?.category,
      modelBaseType.value,
    ),
  );
  const modelTypeDescription = computed(
    () => selectedBuiltinModel.value?.description || '本地模型配置',
  );
  const modelBaseLabel = computed(
    () => selectedBuiltinModel.value?.base || modelType.value,
  );

  const modelCatalogGroups = computed(() => {
    const groups = new Map<string, BuiltinModelItem[]>();
    for (const item of builtinModelCatalog.value) {
      const key = item.category || '其他模型';
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
      modelBaseType.value === 'random_forest' ||
      modelBaseType.value === 'extra_trees',
  );
  const isLinearModel = computed(() =>
    ['logistic', 'svm', 'perceptron', 'passive_aggressive'].includes(
      modelBaseType.value,
    ),
  );
  const isPrototypeModel = computed(
    () => modelBaseType.value === 'nearest_centroid',
  );
  const hasCompactParams = computed(() =>
    ['naive_bayes', 'ridge', 'adaboost', 'ensemble'].includes(
      modelBaseType.value,
    ),
  );

  const modelTuneColumns = [
    { title: '模型', dataIndex: 'label', key: 'label' },
    { title: '基础算法', dataIndex: 'base', key: 'base' },
    { title: '验证准确率', dataIndex: 'validationAccuracy', key: 'validationAccuracy' },
    { title: '训练准确率', dataIndex: 'trainAccuracy', key: 'trainAccuracy' },
    { title: '推理速度', dataIndex: 'inferenceThroughput', key: 'inferenceThroughput' },
    { title: '参数', dataIndex: 'hyperParams', key: 'hyperParams' },
    { title: '状态', dataIndex: 'state', key: 'state' },
  ];

  return {
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
  };
}
