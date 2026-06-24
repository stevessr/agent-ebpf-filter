package attention

import "agent-ebpf-filter/core"

func init() {
	// Original additive attention models
	RegisterModel(core.ModelRandomForestAttention, func() Model { return newAttentionEnhancedModel(core.ModelRandomForestAttention, NewDecisionForest(31, 8, 4)) })
	RegisterModel(core.ModelLogisticAttention, func() Model { return newAttentionEnhancedModel(core.ModelLogisticAttention, NewLogisticModel(0.01, "l2", 1000)) })
	RegisterModel(core.ModelKNNAttention, func() Model { return newAttentionEnhancedModel(core.ModelKNNAttention, NewKNNModel(5, "euclidean", "uniform")) })

	// Standalone attention mechanisms
	RegisterModel(ModelAdditiveAttention, func() Model { return newStandaloneAttentionModel(ModelAdditiveAttention, NewAdditiveAttention()) })
	RegisterModel(ModelScaledDotProductAttention, func() Model { return newStandaloneAttentionModel(ModelScaledDotProductAttention, NewScaledDotProductAttention()) })
	RegisterModel(ModelMultiHeadAttention, func() Model { return newStandaloneAttentionModel(ModelMultiHeadAttention, NewMultiHeadAttention(4)) })
	RegisterModel(ModelRWKVAttention, func() Model { return newStandaloneAttentionModel(ModelRWKVAttention, NewRWKVAttention()) })
	RegisterModel(ModelMambaAttention, func() Model { return newStandaloneAttentionModel(ModelMambaAttention, NewMambaAttention()) })

	// Scaled Dot-Product Attention enhanced models
	RegisterModel(ModelRandomForestScaledDotProduct, func() Model { return newAttentionEnhancedModelWithLayer(ModelRandomForestScaledDotProduct, NewDecisionForest(31, 8, 4), NewScaledDotProductAttention()) })
	RegisterModel(ModelLogisticScaledDotProduct, func() Model { return newAttentionEnhancedModelWithLayer(ModelLogisticScaledDotProduct, NewLogisticModel(0.01, "l2", 1000), NewScaledDotProductAttention()) })
	RegisterModel(ModelKNNScaledDotProduct, func() Model { return newAttentionEnhancedModelWithLayer(ModelKNNScaledDotProduct, NewKNNModel(5, "euclidean", "uniform"), NewScaledDotProductAttention()) })

	// Multi-Head Attention enhanced models
	RegisterModel(ModelRandomForestMultiHead, func() Model { return newAttentionEnhancedModelWithLayer(ModelRandomForestMultiHead, NewDecisionForest(31, 8, 4), NewMultiHeadAttention(4)) })
	RegisterModel(ModelLogisticMultiHead, func() Model { return newAttentionEnhancedModelWithLayer(ModelLogisticMultiHead, NewLogisticModel(0.01, "l2", 1000), NewMultiHeadAttention(4)) })
	RegisterModel(ModelKNNMultiHead, func() Model { return newAttentionEnhancedModelWithLayer(ModelKNNMultiHead, NewKNNModel(5, "euclidean", "uniform"), NewMultiHeadAttention(4)) })

	// RWKV Attention enhanced models
	RegisterModel(ModelRandomForestRWKV, func() Model { return newAttentionEnhancedModelWithLayer(ModelRandomForestRWKV, NewDecisionForest(31, 8, 4), NewRWKVAttention()) })
	RegisterModel(ModelLogisticRWKV, func() Model { return newAttentionEnhancedModelWithLayer(ModelLogisticRWKV, NewLogisticModel(0.01, "l2", 1000), NewRWKVAttention()) })
	RegisterModel(ModelKNNRWKV, func() Model { return newAttentionEnhancedModelWithLayer(ModelKNNRWKV, NewKNNModel(5, "euclidean", "uniform"), NewRWKVAttention()) })

	// Mamba Attention enhanced models
	RegisterModel(ModelRandomForestMamba, func() Model { return newAttentionEnhancedModelWithLayer(ModelRandomForestMamba, NewDecisionForest(31, 8, 4), NewMambaAttention()) })
	RegisterModel(ModelLogisticMamba, func() Model { return newAttentionEnhancedModelWithLayer(ModelLogisticMamba, NewLogisticModel(0.01, "l2", 1000), NewMambaAttention()) })
	RegisterModel(ModelKNNMamba, func() Model { return newAttentionEnhancedModelWithLayer(ModelKNNMamba, NewKNNModel(5, "euclidean", "uniform"), NewMambaAttention()) })
}

