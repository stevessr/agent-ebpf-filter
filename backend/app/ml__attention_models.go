package app

import "agent-ebpf-filter/core"

func init() {
	RegisterModel(core.ModelRandomForestAttention, func() Model { return newAttentionEnhancedModel(core.ModelRandomForestAttention, NewDecisionForest(31, 8, 4)) })
	RegisterModel(core.ModelLogisticAttention, func() Model { return newAttentionEnhancedModel(core.ModelLogisticAttention, NewLogisticModel(0.01, "l2", 1000)) })
	RegisterModel(core.ModelKNNAttention, func() Model { return newAttentionEnhancedModel(core.ModelKNNAttention, NewKNNModel(5, "euclidean", "uniform")) })
}
