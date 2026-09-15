import type { MLBuiltinModelCatalogItem } from "../types/config";

export type MLModelFamily =
  | "tree"
  | "linear"
  | "online_linear"
  | "distance"
  | "probabilistic"
  | "boosting"
  | "ensemble"
  | "graph"
  | "attention"
  | "sequence"
  | "generative"
  | "other";

export type MLFeatureClass =
  | "tabular_128"
  | "attention_context"
  | "sequence_state_space"
  | "sequence_ngram"
  | "graph_context"
  | "synthetic_sequence";

const familyLabels: Record<MLModelFamily, string> = {
  tree: "树模型",
  linear: "线性模型",
  online_linear: "在线线性模型",
  distance: "距离/原型模型",
  probabilistic: "概率模型",
  boosting: "Boosting 模型",
  ensemble: "集成模型",
  graph: "图模型",
  attention: "注意力模型",
  sequence: "序列/状态空间模型",
  generative: "生成式模型",
  other: "其他模型",
};

const featureLabels: Record<MLFeatureClass, string> = {
  tabular_128: "128维表格上下文",
  attention_context: "注意力上下文",
  sequence_state_space: "序列/状态空间",
  sequence_ngram: "N-Gram 序列",
  graph_context: "图结构上下文",
  synthetic_sequence: "生成增强序列",
};

const taxonomyTag = (item: MLBuiltinModelCatalogItem, prefix: string) =>
  item.tags?.find((tag) => tag.startsWith(prefix))?.slice(prefix.length);

export const modelFamily = (item: MLBuiltinModelCatalogItem): MLModelFamily => {
  const tagged = taxonomyTag(item, "family:") as MLModelFamily | undefined;
  if (tagged && familyLabels[tagged]) return tagged;

  const value = item.value || "";
  const base = item.base || "";
  if (value === "additive_attention" || value === "scaled_dot_product_attention" || value === "multi_head_attention") return "attention";
  if (value === "rwkv_attention" || value === "mamba_attention") return "sequence";
  if (base === "random_forest" || base === "extra_trees" || value.includes("random_forest")) return "tree";
  if (base === "logistic" || base === "svm" || base === "ridge" || value.includes("logistic")) return "linear";
  if (base === "perceptron" || base === "passive_aggressive") return "online_linear";
  if (base === "knn" || base === "nearest_centroid" || value.includes("knn")) return "distance";
  if (base === "naive_bayes") return "probabilistic";
  if (base === "adaboost") return "boosting";
  if (base === "ensemble") return "ensemble";
  if (base === "graph_learning") return "graph";
  if (base === "gan_transformer") return "generative";
  return "other";
};

export const modelFamilyLabel = (item: MLBuiltinModelCatalogItem) =>
  familyLabels[modelFamily(item)];

export const modelFeatureClass = (
  item: MLBuiltinModelCatalogItem,
): MLFeatureClass => {
  const tagged = taxonomyTag(item, "feature:") as MLFeatureClass | undefined;
  if (tagged && featureLabels[tagged]) return tagged;

  const value = item.value || "";
  const tags = new Set(item.tags || []);
  if (value.includes("ngram") || tags.has("ngram")) return "sequence_ngram";
  if (item.base === "graph_learning" || tags.has("graph") || tags.has("gnn")) return "graph_context";
  if (item.base === "gan_transformer" || tags.has("gan") || tags.has("synthetic")) return "synthetic_sequence";
  if (
    value.includes("mamba") ||
    value.includes("rwkv") ||
    tags.has("mamba") ||
    tags.has("rwkv") ||
    tags.has("ssm")
  ) {
    return "sequence_state_space";
  }
  if (
    value.includes("attention") ||
    value.includes("multi_head") ||
    value.includes("scaled_dot_product") ||
    tags.has("attention")
  ) {
    return "attention_context";
  }
  return "tabular_128";
};

export const modelFeatureClassLabel = (item: MLBuiltinModelCatalogItem) =>
  featureLabels[modelFeatureClass(item)];

export const modelFeatureProfiles = (item: MLBuiltinModelCatalogItem) => {
  const profiles = (item.tags || [])
    .filter((tag) => tag.startsWith("feature-profile:"))
    .map((tag) => tag.slice("feature-profile:".length));
  if (profiles.length > 0) return profiles;

  switch (modelFeatureClass(item)) {
    case "attention_context":
      return ["tabular", "attention", "contextual"];
    case "sequence_state_space":
      return ["tabular", "sequence", "contextual"];
    case "sequence_ngram":
      return ["tabular", "sequence", "ngram"];
    case "graph_context":
      return ["tabular", "graph", "contextual"];
    case "synthetic_sequence":
      return ["tabular", "sequence", "transformer", "synthetic_augmentation"];
    default:
      return ["tabular"];
  }
};

export const familyLabelFromId = (family: MLModelFamily) => familyLabels[family];
export const featureLabelFromId = (feature: MLFeatureClass) => featureLabels[feature];
