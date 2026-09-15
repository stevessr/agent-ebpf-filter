package ml

import "agent-ebpf-filter/core"

// ModelFamily is the algorithmic family of a classifier. It deliberately stays
// orthogonal to feature engineering: e.g. RF + Mamba is still a tree model,
// while its feature class is sequence/state-space.
type ModelFamily string

const (
	ModelFamilyTree          ModelFamily = "tree"
	ModelFamilyLinear        ModelFamily = "linear"
	ModelFamilyOnlineLinear  ModelFamily = "online_linear"
	ModelFamilyDistance      ModelFamily = "distance"
	ModelFamilyProbabilistic ModelFamily = "probabilistic"
	ModelFamilyBoosting      ModelFamily = "boosting"
	ModelFamilyEnsemble      ModelFamily = "ensemble"
	ModelFamilyGraph         ModelFamily = "graph"
	ModelFamilyAttention     ModelFamily = "attention"
	ModelFamilySequence      ModelFamily = "sequence"
	ModelFamilyGenerative    ModelFamily = "generative"
	ModelFamilyOther         ModelFamily = "other"
)

// ModelFeatureClass is the primary feature/representation strategy used by a
// model. FeatureProfiles provides finer-grained, many-to-many tags.
type ModelFeatureClass string

const (
	ModelFeatureTabular128       ModelFeatureClass = "tabular_128"
	ModelFeatureAttentionContext ModelFeatureClass = "attention_context"
	ModelFeatureSequenceState    ModelFeatureClass = "sequence_state_space"
	ModelFeatureSequenceNGram    ModelFeatureClass = "sequence_ngram"
	ModelFeatureGraphContext     ModelFeatureClass = "graph_context"
	ModelFeatureSyntheticSeq     ModelFeatureClass = "synthetic_sequence"
)

// Feature source IDs mirror FeatureExtractor's stable 128-dim layout. They are
// shared by normal tabular models and remain visible even when a model adds an
// attention, sequence, graph, or N-Gram representation on top.
const (
	FeatureSourceCommandProcess = "command_process" // [0,31]
	FeatureSourceArgumentStats  = "argument_stats"  // [32,63]
	FeatureSourceEmbedding      = "embedding"       // [64,95]
	FeatureSourceHistory        = "history"         // [96,111]
	FeatureSourceTemporal       = "temporal_rate"   // [112,119]
	FeatureSourceNetworkAudit   = "network_audit"   // [120,127]
)

var baseFeatureSources = []string{
	FeatureSourceCommandProcess,
	FeatureSourceArgumentStats,
	FeatureSourceEmbedding,
	FeatureSourceHistory,
	FeatureSourceTemporal,
	FeatureSourceNetworkAudit,
}

// ModelTaxonomyItem is the normalized two-dimensional classification exposed
// to APIs/UI and used for family/feature based model selection.
type ModelTaxonomyItem struct {
	ModelType         string   `json:"modelType"`
	Family            string   `json:"family"`
	FamilyLabel       string   `json:"familyLabel"`
	FeatureClass      string   `json:"featureClass"`
	FeatureClassLabel string   `json:"featureClassLabel"`
	FeatureProfiles   []string `json:"featureProfiles"`
	FeatureSources    []string `json:"featureSources"`
}

func ModelTaxonomy(t ModelType) ModelTaxonomyItem {
	family := modelFamilyForType(t)
	featureClass, profiles := modelFeatureProfileForType(t)
	return ModelTaxonomyItem{
		ModelType:         string(t),
		Family:            string(family),
		FamilyLabel:       modelFamilyLabel(family),
		FeatureClass:      string(featureClass),
		FeatureClassLabel: modelFeatureClassLabel(featureClass),
		FeatureProfiles:   profiles,
		FeatureSources:    append([]string(nil), baseFeatureSources...),
	}
}

func BuiltinModelTaxonomy() []ModelTaxonomyItem {
	types := AllModelTypes()
	items := make([]ModelTaxonomyItem, 0, len(types))
	for _, t := range types {
		items = append(items, ModelTaxonomy(t))
	}
	return items
}

// ModelTypesByTaxonomy returns registered model IDs matching all non-empty
// filters. A feature profile matches either the primary feature class or one of
// the detailed feature profiles.
func ModelTypesByTaxonomy(families, featureProfiles []string) []ModelType {
	familySet := stringSet(families)
	featureSet := stringSet(featureProfiles)
	out := make([]ModelType, 0)
	for _, t := range AllModelTypes() {
		if _, ok := ModelRegistry[t]; !ok {
			continue
		}
		meta := ModelTaxonomy(t)
		if len(familySet) > 0 {
			if _, ok := familySet[meta.Family]; !ok {
				continue
			}
		}
		if len(featureSet) > 0 && !taxonomyFeatureMatches(meta, featureSet) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func taxonomyFeatureMatches(meta ModelTaxonomyItem, wanted map[string]struct{}) bool {
	if _, ok := wanted[meta.FeatureClass]; ok {
		return true
	}
	for _, profile := range meta.FeatureProfiles {
		if _, ok := wanted[profile]; ok {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func modelFamilyForType(t ModelType) ModelFamily {
	switch t {
	case ModelRandomForest, ModelRandomForestFast, ModelRandomForestShallow, ModelRandomForestStable, ModelRandomForestDeep, ModelRandomForestWide,
		ModelExtraTrees, ModelExtraTreesFast, ModelExtraTreesDeep, ModelExtraTreesWide,
		core.ModelRandomForestAttention,
		ModelRandomForestScaledDotProduct, ModelRandomForestMultiHead, ModelRandomForestRWKV, ModelRandomForestMamba,
		core.ModelNGramRandomForest:
		return ModelFamilyTree

	case ModelLogisticRegression, ModelLogisticFast, ModelLogisticNone, ModelLogisticL1, ModelLogisticBalanced, ModelLogisticL1Balanced,
		ModelSVM, ModelSVMLong, ModelSVMBalanced,
		ModelRidge, ModelRidgeLight, ModelRidgeStrong,
		core.ModelLogisticAttention,
		ModelLogisticScaledDotProduct, ModelLogisticMultiHead, ModelLogisticRWKV, ModelLogisticMamba,
		core.ModelNGramLogistic:
		return ModelFamilyLinear

	case ModelPerceptron, ModelPerceptronLong, ModelPerceptronBalanced,
		ModelPassiveAggressive, ModelPassiveAggressiveLong, ModelPassiveAggressiveBalanced:
		return ModelFamilyOnlineLinear

	case ModelKNN, ModelKNNManhattan, ModelKNNCosine, ModelKNNDistance,
		ModelNearestCentroid, ModelNearestCentroidBalanced, ModelNearestCentroidCosine, ModelNearestCentroidManhattan,
		core.ModelKNNAttention,
		ModelKNNScaledDotProduct, ModelKNNMultiHead, ModelKNNRWKV, ModelKNNMamba,
		core.ModelNGramKNN:
		return ModelFamilyDistance

	case ModelNaiveBayes, ModelNaiveBayesBalanced:
		return ModelFamilyProbabilistic
	case ModelAdaBoost, ModelAdaBoostFast, ModelAdaBoostLarge:
		return ModelFamilyBoosting
	case ModelEnsemble, ModelEnsembleSoft, ModelEnsembleHard, ModelEnsembleStacked:
		return ModelFamilyEnsemble
	case core.ModelGraphLearning:
		return ModelFamilyGraph
	case ModelAdditiveAttention, ModelScaledDotProductAttention, ModelMultiHeadAttention:
		return ModelFamilyAttention
	case ModelRWKVAttention, ModelMambaAttention:
		return ModelFamilySequence
	case ModelGANTransformer:
		return ModelFamilyGenerative
	default:
		return ModelFamilyOther
	}
}

func modelFeatureProfileForType(t ModelType) (ModelFeatureClass, []string) {
	switch t {
	case core.ModelNGramRandomForest, core.ModelNGramLogistic, core.ModelNGramKNN:
		return ModelFeatureSequenceNGram, []string{"tabular", "sequence", "ngram"}

	case core.ModelGraphLearning:
		return ModelFeatureGraphContext, []string{"tabular", "graph", "contextual"}

	case ModelGANTransformer:
		return ModelFeatureSyntheticSeq, []string{"tabular", "sequence", "transformer", "synthetic_augmentation", "generative"}

	case ModelRWKVAttention, ModelMambaAttention,
		ModelRandomForestRWKV, ModelLogisticRWKV, ModelKNNRWKV,
		ModelRandomForestMamba, ModelLogisticMamba, ModelKNNMamba:
		profiles := []string{"tabular", "sequence", "contextual"}
		if t == ModelMambaAttention || t == ModelRandomForestMamba || t == ModelLogisticMamba || t == ModelKNNMamba {
			profiles = append(profiles, "state_space", "mamba")
		} else {
			profiles = append(profiles, "rwkv")
		}
		return ModelFeatureSequenceState, profiles

	case ModelAdditiveAttention, ModelScaledDotProductAttention, ModelMultiHeadAttention,
		core.ModelRandomForestAttention, core.ModelLogisticAttention, core.ModelKNNAttention,
		ModelRandomForestScaledDotProduct, ModelLogisticScaledDotProduct, ModelKNNScaledDotProduct,
		ModelRandomForestMultiHead, ModelLogisticMultiHead, ModelKNNMultiHead:
		return ModelFeatureAttentionContext, []string{"tabular", "attention", "contextual"}

	default:
		return ModelFeatureTabular128, []string{"tabular"}
	}
}

func modelFamilyLabel(family ModelFamily) string {
	switch family {
	case ModelFamilyTree:
		return "树模型"
	case ModelFamilyLinear:
		return "线性模型"
	case ModelFamilyOnlineLinear:
		return "在线线性模型"
	case ModelFamilyDistance:
		return "距离/原型模型"
	case ModelFamilyProbabilistic:
		return "概率模型"
	case ModelFamilyBoosting:
		return "Boosting 模型"
	case ModelFamilyEnsemble:
		return "集成模型"
	case ModelFamilyGraph:
		return "图模型"
	case ModelFamilyAttention:
		return "注意力模型"
	case ModelFamilySequence:
		return "序列/状态空间模型"
	case ModelFamilyGenerative:
		return "生成式模型"
	default:
		return "其他模型"
	}
}

func modelFeatureClassLabel(featureClass ModelFeatureClass) string {
	switch featureClass {
	case ModelFeatureAttentionContext:
		return "注意力上下文特征"
	case ModelFeatureSequenceState:
		return "序列/状态空间特征"
	case ModelFeatureSequenceNGram:
		return "N-Gram 序列特征"
	case ModelFeatureGraphContext:
		return "图结构上下文特征"
	case ModelFeatureSyntheticSeq:
		return "生成增强序列特征"
	default:
		return "128 维表格上下文特征"
	}
}
