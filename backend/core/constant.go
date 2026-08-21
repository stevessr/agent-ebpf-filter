package core

// ── ML model type constants ──────────────────────────────────────────────────

type ModelType string

const (
	ModelRandomForest                 ModelType = "random_forest"
	ModelKNN                          ModelType = "knn"
	ModelLogisticRegression           ModelType = "logistic_regression"
	ModelNaiveBayes                   ModelType = "naive_bayes"
	ModelNearestCentroid              ModelType = "nearest_centroid"
	ModelExtraTrees                   ModelType = "extra_trees"
	ModelAdaBoost                     ModelType = "ada_boost"
	ModelSVM                          ModelType = "svm"
	ModelRidge                        ModelType = "ridge"
	ModelPerceptron                   ModelType = "perceptron"
	ModelPassiveAggressive            ModelType = "passive_aggressive"
	ModelEnsemble                     ModelType = "ensemble"
	ModelAdditiveAttention            ModelType = "additive_attention"
	ModelRandomForestAttention        ModelType = "random_forest_attention"
	ModelLogisticAttention            ModelType = "logistic_attention"
	ModelKNNAttention                 ModelType = "knn_attention"
	ModelGraphLearning                ModelType = "graph_learning"
	ModelGANTransformer               ModelType = "gan_transformer"
	ModelScaledDotProductAttention    ModelType = "scaled_dot_product_attention"
	ModelMultiHeadAttention           ModelType = "multi_head_attention"
	ModelRWKVAttention                ModelType = "rwkv_attention"
	ModelMambaAttention               ModelType = "mamba_attention"
	ModelRandomForestScaledDotProduct ModelType = "random_forest_scaled_dot_product"
	ModelLogisticScaledDotProduct     ModelType = "logistic_scaled_dot_product"
	ModelKNNScaledDotProduct          ModelType = "knn_scaled_dot_product"
	ModelRandomForestMultiHead        ModelType = "random_forest_multi_head"
	ModelLogisticMultiHead            ModelType = "logistic_multi_head"
	ModelKNNMultiHead                 ModelType = "knn_multi_head"
	ModelRandomForestRWKV             ModelType = "random_forest_rwkv"
	ModelLogisticRWKV                 ModelType = "logistic_rwkv"
	ModelKNNRWKV                      ModelType = "knn_rwkv"
	ModelRandomForestMamba            ModelType = "random_forest_mamba"
	ModelLogisticMamba                ModelType = "logistic_mamba"
	ModelKNNMamba                     ModelType = "knn_mamba"
	ModelNGramRandomForest            ModelType = "ngram_random_forest"
	ModelNGramLogistic                ModelType = "ngram_logistic"
	ModelNGramKNN                     ModelType = "ngram_knn"
	ModelRandomForestAttn             ModelType = "random_forest_attention_v2"
	ModelLogisticAttn                 ModelType = "logistic_attention_v2"
	ModelKNNAttn                      ModelType = "knn_attention_v2"
	ModelRandomForestFast             ModelType = "random_forest_fast"
	ModelRandomForestShallow          ModelType = "random_forest_shallow"
	ModelRandomForestStable           ModelType = "random_forest_stable"
	ModelRandomForestDeep             ModelType = "random_forest_deep"
	ModelRandomForestWide             ModelType = "random_forest_wide"
	ModelExtraTreesFast               ModelType = "extra_trees_fast"
	ModelExtraTreesDeep               ModelType = "extra_trees_deep"
	ModelExtraTreesWide               ModelType = "extra_trees_wide"
	ModelLogisticFast                 ModelType = "logistic_fast"
	ModelLogisticNone                 ModelType = "logistic_none"
	ModelLogisticL1                   ModelType = "logistic_l1"
	ModelLogisticBalanced             ModelType = "logistic_balanced"
	ModelLogisticL1Balanced           ModelType = "logistic_l1_balanced"
	ModelSVMLong                      ModelType = "svm_long"
	ModelSVMBalanced                  ModelType = "svm_balanced"
	ModelPerceptronLong               ModelType = "perceptron_long"
	ModelPerceptronBalanced           ModelType = "perceptron_balanced"
	ModelPassiveAggressiveLong        ModelType = "passive_aggressive_long"
	ModelPassiveAggressiveBalanced    ModelType = "passive_aggressive_balanced"
	ModelKNNManhattan                 ModelType = "knn_manhattan"
	ModelKNNCosine                    ModelType = "knn_cosine"
	ModelKNNDistance                  ModelType = "knn_distance"
	ModelNearestCentroidBalanced      ModelType = "nearest_centroid_balanced"
	ModelNearestCentroidCosine        ModelType = "nearest_centroid_cosine"
	ModelNearestCentroidManhattan     ModelType = "nearest_centroid_manhattan"
	ModelNaiveBayesBalanced           ModelType = "naive_bayes_balanced"
	ModelRidgeLong                    ModelType = "ridge_long"
	ModelRidgeBalanced                ModelType = "ridge_balanced"
	ModelRidgeLight                   ModelType = "ridge_light"
	ModelRidgeStrong                  ModelType = "ridge_strong"
	ModelAdaBoostLong                 ModelType = "ada_boost_long"
	ModelAdaBoostBalanced             ModelType = "ada_boost_balanced"
	ModelAdaBoostFast                 ModelType = "ada_boost_fast"
	ModelAdaBoostLarge                ModelType = "ada_boost_large"
	ModelEnsembleSoft                 ModelType = "ensemble_soft"
	ModelEnsembleHard                 ModelType = "ensemble_hard"
	ModelEnsembleStacked              ModelType = "ensemble_stacked"
)

// ── Other runtime constants ──────────────────────────────────────────────────

const (
	HookTypeNative  = "native"
	HookTypeWrapper = "wrapper"

	ConfigFormatJSON       = "json"
	ConfigFormatTOML       = "toml"
	ConfigFormatTypeScript = "typescript"
)

const FeatureDim = 128

// UDSPath is the UDS socket path for wrapper communication.
const UDSPATH = "/tmp/agent-ebpf.sock"

// EBPFPinRoot is the root path for eBPF pinned maps and programs.
const (
	EBPFPinRoot     = "/sys/fs/bpf/agent-ebpf"
	EBPFPinMapsDir  = EBPFPinRoot + "/maps"
	EBPFPinLinksDir = EBPFPinRoot + "/links"
)
