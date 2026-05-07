import type { MLBuiltinModelCatalogItem } from '../types/config';

export const defaultMLBuiltinModelCatalog: MLBuiltinModelCatalogItem[] = [
  { value: 'random_forest', label: 'Random Forest', base: 'random_forest', category: '树模型', description: '稳健默认随机森林，适合作为主力 baseline。', recommended: true, defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['holdout', 'stable'] },
  { value: 'random_forest_fast', label: 'Random Forest Fast', base: 'random_forest', category: '树模型', description: '少量浅树，优先推理速度与正确命令放行延迟。', recommended: true, defaults: { numTrees: 5, maxDepth: 4, minSamplesLeaf: 2 }, tags: ['fast', 'allow'] },
  { value: 'random_forest_shallow', label: 'Random Forest Shallow', base: 'random_forest', category: '树模型', description: '浅层森林，降低过拟合与误伤风险。', defaults: { numTrees: 15, maxDepth: 4, minSamplesLeaf: 3 }, tags: ['low-overfit'] },
  { value: 'random_forest_stable', label: 'Random Forest Stable', base: 'random_forest', category: '树模型', description: '中等深度 + 较多树，偏稳定性和多次重复表现。', recommended: true, defaults: { numTrees: 51, maxDepth: 10, minSamplesLeaf: 3 }, tags: ['stable'] },
  { value: 'random_forest_deep', label: 'Random Forest Deep', base: 'random_forest', category: '树模型', description: '更深森林，用于探索高上限但训练/推理成本更高。', defaults: { numTrees: 31, maxDepth: 12, minSamplesLeaf: 3 }, tags: ['high-capacity'] },
  { value: 'random_forest_wide', label: 'Random Forest Wide', base: 'random_forest', category: '树模型', description: '更多树的宽森林，降低方差。', defaults: { numTrees: 71, maxDepth: 8, minSamplesLeaf: 3 }, tags: ['wide'] },
  { value: 'extra_trees', label: 'Extra Trees', base: 'extra_trees', category: '树模型', description: '随机阈值的极随机树，对照随机森林泛化。', defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['randomized'] },
  { value: 'extra_trees_fast', label: 'Extra Trees Fast', base: 'extra_trees', category: '树模型', description: '少量 Extra Trees，速度优先。', defaults: { numTrees: 9, maxDepth: 5, minSamplesLeaf: 3 }, tags: ['fast'] },
  { value: 'extra_trees_deep', label: 'Extra Trees Deep', base: 'extra_trees', category: '树模型', description: '深层 Extra Trees，高容量探索。', defaults: { numTrees: 31, maxDepth: 12, minSamplesLeaf: 3 }, tags: ['high-capacity'] },
  { value: 'extra_trees_wide', label: 'Extra Trees Wide', base: 'extra_trees', category: '树模型', description: '更多 Extra Trees，偏低方差对照。', defaults: { numTrees: 71, maxDepth: 10, minSamplesLeaf: 3 }, tags: ['wide'] },
  { value: 'logistic', label: 'Logistic L2', base: 'logistic', category: '线性模型', description: 'L2 正则逻辑回归，轻量可解释。', defaults: { numTrees: 10, maxDepth: 8, minSamplesLeaf: 1000 }, tags: ['interpretable'] },
  { value: 'logistic_fast', label: 'Logistic Fast', base: 'logistic', category: '线性模型', description: '少迭代逻辑回归，快速基线。', defaults: { numTrees: 20, maxDepth: 8, minSamplesLeaf: 500 }, tags: ['fast'] },
  { value: 'logistic_none', label: 'Logistic None', base: 'logistic', category: '线性模型', description: '无正则逻辑回归，观察正则化影响。', defaults: { numTrees: 50, maxDepth: 4, minSamplesLeaf: 2000 }, tags: ['ablation'] },
  { value: 'logistic_l1', label: 'Logistic L1', base: 'logistic', category: '线性模型', description: 'L1 稀疏逻辑回归，便于解释关键特征。', recommended: true, defaults: { numTrees: 100, maxDepth: 12, minSamplesLeaf: 4000 }, tags: ['interpretable', 'sparse'] },
  { value: 'logistic_balanced', label: 'Logistic Balanced', base: 'logistic', category: '线性模型', description: '类别加权 L2 逻辑回归，面向稀缺高风险类。', defaults: { numTrees: 20, maxDepth: 8, minSamplesLeaf: 2000 }, tags: ['balanced'] },
  { value: 'logistic_l1_balanced', label: 'Logistic L1 Balanced', base: 'logistic', category: '线性模型', description: 'L1 + 类别加权，兼顾稀疏解释与不平衡。', defaults: { numTrees: 50, maxDepth: 12, minSamplesLeaf: 4000 }, tags: ['balanced', 'sparse'] },
  { value: 'svm', label: 'Linear SVM', base: 'svm', category: '线性模型', description: '线性 hinge-loss 基线。', defaults: { numTrees: 5, maxDepth: 8, minSamplesLeaf: 500 }, tags: ['margin'] },
  { value: 'svm_long', label: 'SVM Long', base: 'svm', category: '线性模型', description: '更长迭代 SVM，用于收敛性探索。', defaults: { numTrees: 5, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['long'] },
  { value: 'svm_balanced', label: 'SVM Balanced', base: 'svm', category: '线性模型', description: '类别加权 SVM，侧重少数类召回。', defaults: { numTrees: 10, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['balanced'] },
  { value: 'perceptron', label: 'Perceptron', base: 'perceptron', category: '在线线性模型', description: '在线感知机基线。', defaults: { numTrees: 20, maxDepth: 8, minSamplesLeaf: 1000 }, tags: ['online'] },
  { value: 'perceptron_long', label: 'Perceptron Long', base: 'perceptron', category: '在线线性模型', description: '更长迭代感知机。', defaults: { numTrees: 150, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['online', 'long'] },
  { value: 'perceptron_balanced', label: 'Perceptron Balanced', base: 'perceptron', category: '在线线性模型', description: '类别加权感知机。', defaults: { numTrees: 50, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['balanced', 'online'] },
  { value: 'passive_aggressive', label: 'Passive-Aggressive', base: 'passive_aggressive', category: '在线线性模型', description: 'PA 在线更新模型，适合流式对照。', defaults: { numTrees: 10, maxDepth: 8, minSamplesLeaf: 1000 }, tags: ['online'] },
  { value: 'passive_aggressive_long', label: 'PA Long', base: 'passive_aggressive', category: '在线线性模型', description: '更长迭代 PA 模型。', defaults: { numTrees: 20, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['online', 'long'] },
  { value: 'passive_aggressive_balanced', label: 'PA Balanced', base: 'passive_aggressive', category: '在线线性模型', description: '类别加权 PA 模型。', defaults: { numTrees: 20, maxDepth: 8, minSamplesLeaf: 4000 }, tags: ['balanced', 'online'] },
  { value: 'knn', label: 'KNN Euclidean', base: 'knn', category: '近邻/原型', description: '欧氏距离 KNN，训练快但推理随样本数增长。', defaults: { numTrees: 5, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['instance-based'] },
  { value: 'knn_manhattan', label: 'KNN Manhattan', base: 'knn', category: '近邻/原型', description: '曼哈顿距离 KNN，高维稀疏特征对照。', defaults: { numTrees: 7, maxDepth: 12, minSamplesLeaf: 5 }, tags: ['distance'] },
  { value: 'knn_cosine', label: 'KNN Cosine', base: 'knn', category: '近邻/原型', description: '余弦距离 KNN，关注命令模式方向相似。', defaults: { numTrees: 7, maxDepth: 16, minSamplesLeaf: 5 }, tags: ['distance'] },
  { value: 'knn_distance', label: 'KNN Distance Weighted', base: 'knn', category: '近邻/原型', description: '距离加权 KNN，近邻影响更大。', defaults: { numTrees: 5, maxDepth: 12, minSamplesLeaf: 8 }, tags: ['distance', 'weighted'] },
  { value: 'nearest_centroid', label: 'Nearest Centroid', base: 'nearest_centroid', category: '近邻/原型', description: '欧氏最近质心，极快且解释清晰。', defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['fast', 'interpretable'] },
  { value: 'nearest_centroid_balanced', label: 'Nearest Centroid Balanced', base: 'nearest_centroid', category: '近邻/原型', description: '最近质心 + 均匀先验，减少多数类偏置。', defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['balanced', 'fast'] },
  { value: 'nearest_centroid_cosine', label: 'Nearest Centroid Cosine', base: 'nearest_centroid', category: '近邻/原型', description: '余弦质心，适合模式方向相似度。', defaults: { numTrees: 20, maxDepth: 4, minSamplesLeaf: 5 }, tags: ['cosine', 'fast'] },
  { value: 'nearest_centroid_manhattan', label: 'Nearest Centroid Manhattan', base: 'nearest_centroid', category: '近邻/原型', description: '曼哈顿质心，适合稀疏/离散特征。', defaults: { numTrees: 40, maxDepth: 12, minSamplesLeaf: 5 }, tags: ['manhattan', 'fast'] },
  { value: 'naive_bayes', label: 'Naive Bayes', base: 'naive_bayes', category: '概率模型', description: '高斯朴素贝叶斯，快速概率 baseline。', defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['probabilistic'] },
  { value: 'naive_bayes_balanced', label: 'Naive Bayes Balanced', base: 'naive_bayes', category: '概率模型', description: '均匀类先验朴素贝叶斯。', defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['balanced', 'probabilistic'] },
  { value: 'ridge', label: 'Ridge', base: 'ridge', category: '线性模型', description: 'Ridge 分类器，速度快。', defaults: { numTrees: 5, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['linear'] },
  { value: 'ridge_light', label: 'Ridge Light', base: 'ridge', category: '线性模型', description: '低正则 Ridge，快速探索。', defaults: { numTrees: 1, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['linear', 'light'] },
  { value: 'ridge_strong', label: 'Ridge Strong', base: 'ridge', category: '线性模型', description: '强正则 Ridge，观察过拟合/欠拟合边界。', defaults: { numTrees: 200, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['linear', 'regularized'] },
  { value: 'adaboost', label: 'AdaBoost', base: 'adaboost', category: 'Boosting/集成', description: '决策桩 AdaBoost，对照 boosting 方向。', defaults: { numTrees: 100, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['boosting'] },
  { value: 'adaboost_fast', label: 'AdaBoost Fast', base: 'adaboost', category: 'Boosting/集成', description: '少估计器 AdaBoost，速度优先。', defaults: { numTrees: 25, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['boosting', 'fast'] },
  { value: 'adaboost_large', label: 'AdaBoost Large', base: 'adaboost', category: 'Boosting/集成', description: '更多估计器 AdaBoost，容量对照。', defaults: { numTrees: 200, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['boosting', 'large'] },
  { value: 'ensemble', label: 'Soft-vote Ensemble', base: 'ensemble', category: 'Boosting/集成', description: '本地软投票集成，融合 RF/Logistic/NB/KNN/Centroid/LightRF。', recommended: true, defaults: { numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 }, tags: ['ensemble', 'stable'] },
];

export const findMLBuiltinModel = (
  catalog: MLBuiltinModelCatalogItem[],
  value: string,
): MLBuiltinModelCatalogItem | undefined => catalog.find((item) => item.value === value);

export const mlModelCategoryColor = (category?: string, base?: string) => {
  if (category?.includes('树')) return 'green';
  if (category?.includes('线性')) return base === 'svm' ? 'red' : 'cyan';
  if (category?.includes('在线')) return 'orange';
  if (category?.includes('近邻')) return 'blue';
  if (category?.includes('概率')) return 'gold';
  if (category?.includes('Boosting') || category?.includes('集成')) return 'magenta';
  return 'purple';
};
