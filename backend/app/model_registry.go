package app

import (
	"agent-ebpf-filter/core"
	"fmt"
)

// ---- moved from backend/zz_merged_backend.go section model_registry.go ----

// AllModelTypes returns all registered local built-in model IDs in UI order.
func AllModelTypes() []ModelType {
	return []ModelType{
		ModelRandomForest, ModelRandomForestFast, ModelRandomForestShallow, ModelRandomForestStable, ModelRandomForestDeep, ModelRandomForestWide, core.ModelRandomForestAttention,
		ModelExtraTrees, ModelExtraTreesFast, ModelExtraTreesDeep, ModelExtraTreesWide,
		ModelLogisticRegression, ModelLogisticFast, ModelLogisticNone, ModelLogisticL1, ModelLogisticBalanced, ModelLogisticL1Balanced, core.ModelLogisticAttention,
		ModelSVM, ModelSVMLong, ModelSVMBalanced,
		ModelPerceptron, ModelPerceptronLong, ModelPerceptronBalanced,
		ModelPassiveAggressive, ModelPassiveAggressiveLong, ModelPassiveAggressiveBalanced,
		ModelKNN, ModelKNNManhattan, ModelKNNCosine, ModelKNNDistance, core.ModelKNNAttention,
		ModelNearestCentroid, ModelNearestCentroidBalanced, ModelNearestCentroidCosine, ModelNearestCentroidManhattan,
		ModelNaiveBayes, ModelNaiveBayesBalanced,
		ModelRidge, ModelRidgeLight, ModelRidgeStrong,
		ModelAdaBoost, ModelAdaBoostFast, ModelAdaBoostLarge,
		ModelEnsemble, ModelEnsembleSoft, ModelEnsembleHard, ModelEnsembleStacked,
		core.ModelGraphLearning,
		ModelAdditiveAttention,
		ModelScaledDotProductAttention,
		ModelMultiHeadAttention,
		ModelRWKVAttention,
		ModelMambaAttention,
		ModelRandomForestScaledDotProduct, ModelLogisticScaledDotProduct, ModelKNNScaledDotProduct,
		ModelRandomForestMultiHead, ModelLogisticMultiHead, ModelKNNMultiHead,
		ModelRandomForestRWKV, ModelLogisticRWKV, ModelKNNRWKV,
		ModelRandomForestMamba, ModelLogisticMamba, ModelKNNMamba,
		core.ModelNGramRandomForest, core.ModelNGramLogistic, core.ModelNGramKNN,
	}
}

// Model is the interface that all ML models must implement.
type Model interface {
	Predict(features [FeatureDim]float64) Prediction
	Serialize(path string) error
	Type() ModelType
}

// ModelFactory creates a new untrained model instance.
type ModelFactory func() Model

var modelRegistry = map[ModelType]ModelFactory{}

// RegisterModel registers a model type and its factory function.
func RegisterModel(t ModelType, factory ModelFactory) {
	modelRegistry[t] = factory
}

// NewModel creates a new model instance of the given type.
func NewModel(t ModelType) (Model, error) {
	factory, ok := modelRegistry[t]
	if !ok {
		return nil, fmt.Errorf("unknown model type: %s", t)
	}
	return factory(), nil
}

func init() {
	RegisterModel(ModelRandomForest, func() Model { return NewDecisionForest(31, 8, 4) })
}

// modelName returns a human-readable name for a model type.
func modelName(t ModelType) string {
	if label, ok := builtinModelDisplayName(t); ok {
		return label
	}
	switch t {
	case ModelRandomForest:
		return "Random Forest"
	case core.ModelRandomForestAttention:
		return "Random Forest Attention"
	case ModelKNN:
		return "K-Nearest Neighbors"
	case core.ModelKNNAttention:
		return "KNN Attention"
	case ModelLogisticRegression:
		return "Logistic Regression"
	case core.ModelLogisticAttention:
		return "Logistic Attention"
	case ModelNaiveBayes:
		return "Naive Bayes"
	case ModelNearestCentroid:
		return "Nearest Centroid"
	case ModelExtraTrees:
		return "Extra Trees"
	case ModelAdaBoost:
		return "AdaBoost"
	case ModelSVM:
		return "Linear SVM"
	case ModelRidge:
		return "Ridge Classifier"
	case ModelPerceptron:
		return "Perceptron"
	case ModelPassiveAggressive:
		return "Passive-Aggressive"
	case ModelEnsemble:
		return "Ensemble"
	case ModelEnsembleSoft:
		return "Soft-vote Ensemble"
	case ModelEnsembleHard:
		return "Hard-vote Ensemble"
	case ModelEnsembleStacked:
		return "Risk-stacked Ensemble"
	case ModelScaledDotProductAttention:
		return "Scaled Dot-Product Attention"
	case ModelMultiHeadAttention:
		return "Multi-Head Attention"
	case ModelRWKVAttention:
		return "RWKV Attention"
	case ModelMambaAttention:
		return "Mamba Attention"
	case ModelRandomForestScaledDotProduct:
		return "Random Forest + Scaled Dot-Product"
	case ModelLogisticScaledDotProduct:
		return "Logistic + Scaled Dot-Product"
	case ModelKNNScaledDotProduct:
		return "KNN + Scaled Dot-Product"
	case ModelRandomForestMultiHead:
		return "Random Forest + Multi-Head"
	case ModelLogisticMultiHead:
		return "Logistic + Multi-Head"
	case ModelKNNMultiHead:
		return "KNN + Multi-Head"
	case ModelRandomForestRWKV:
		return "Random Forest + RWKV"
	case ModelLogisticRWKV:
		return "Logistic + RWKV"
	case ModelKNNRWKV:
		return "KNN + RWKV"
	case ModelRandomForestMamba:
		return "Random Forest + Mamba"
	case ModelLogisticMamba:
		return "Logistic + Mamba"
	case ModelKNNMamba:
		return "KNN + Mamba"
	case core.ModelGraphLearning:
		return "Graph Neural Network"
	default:
		return string(t)
	}
}
