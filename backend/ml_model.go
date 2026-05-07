package main

import "fmt"

// ModelType identifies the ML model algorithm or a local built-in profile.
type ModelType string

const (
	ModelRandomForest       ModelType = "random_forest"
	ModelKNN                ModelType = "knn"
	ModelLogisticRegression ModelType = "logistic"
	ModelNaiveBayes         ModelType = "naive_bayes"
	ModelNearestCentroid    ModelType = "nearest_centroid"
	ModelExtraTrees         ModelType = "extra_trees"
	ModelAdaBoost           ModelType = "adaboost"
	ModelSVM                ModelType = "svm"
	ModelRidge              ModelType = "ridge"
	ModelPerceptron         ModelType = "perceptron"
	ModelPassiveAggressive  ModelType = "passive_aggressive"
	ModelEnsemble           ModelType = "ensemble"

	// Local built-in profiles. They are first-class selectable model IDs in the
	// UI/config API, but train through their canonical base algorithm with
	// profile-specific parameters applied by applyBuiltinModelPreset.
	ModelRandomForestFast    ModelType = "random_forest_fast"
	ModelRandomForestShallow ModelType = "random_forest_shallow"
	ModelRandomForestStable  ModelType = "random_forest_stable"
	ModelRandomForestDeep    ModelType = "random_forest_deep"
	ModelRandomForestWide    ModelType = "random_forest_wide"

	ModelExtraTreesFast ModelType = "extra_trees_fast"
	ModelExtraTreesDeep ModelType = "extra_trees_deep"
	ModelExtraTreesWide ModelType = "extra_trees_wide"

	ModelLogisticFast       ModelType = "logistic_fast"
	ModelLogisticNone       ModelType = "logistic_none"
	ModelLogisticL1         ModelType = "logistic_l1"
	ModelLogisticBalanced   ModelType = "logistic_balanced"
	ModelLogisticL1Balanced ModelType = "logistic_l1_balanced"

	ModelSVMLong     ModelType = "svm_long"
	ModelSVMBalanced ModelType = "svm_balanced"

	ModelPerceptronLong     ModelType = "perceptron_long"
	ModelPerceptronBalanced ModelType = "perceptron_balanced"

	ModelPassiveAggressiveLong     ModelType = "passive_aggressive_long"
	ModelPassiveAggressiveBalanced ModelType = "passive_aggressive_balanced"

	ModelKNNManhattan ModelType = "knn_manhattan"
	ModelKNNCosine    ModelType = "knn_cosine"
	ModelKNNDistance  ModelType = "knn_distance"

	ModelNearestCentroidBalanced  ModelType = "nearest_centroid_balanced"
	ModelNearestCentroidCosine    ModelType = "nearest_centroid_cosine"
	ModelNearestCentroidManhattan ModelType = "nearest_centroid_manhattan"

	ModelNaiveBayesBalanced ModelType = "naive_bayes_balanced"

	ModelRidgeLight  ModelType = "ridge_light"
	ModelRidgeStrong ModelType = "ridge_strong"

	ModelAdaBoostFast  ModelType = "adaboost_fast"
	ModelAdaBoostLarge ModelType = "adaboost_large"
)

// AllModelTypes returns all registered local built-in model IDs in UI order.
func AllModelTypes() []ModelType {
	return []ModelType{
		ModelRandomForest, ModelRandomForestFast, ModelRandomForestShallow, ModelRandomForestStable, ModelRandomForestDeep, ModelRandomForestWide,
		ModelExtraTrees, ModelExtraTreesFast, ModelExtraTreesDeep, ModelExtraTreesWide,
		ModelLogisticRegression, ModelLogisticFast, ModelLogisticNone, ModelLogisticL1, ModelLogisticBalanced, ModelLogisticL1Balanced,
		ModelSVM, ModelSVMLong, ModelSVMBalanced,
		ModelPerceptron, ModelPerceptronLong, ModelPerceptronBalanced,
		ModelPassiveAggressive, ModelPassiveAggressiveLong, ModelPassiveAggressiveBalanced,
		ModelKNN, ModelKNNManhattan, ModelKNNCosine, ModelKNNDistance,
		ModelNearestCentroid, ModelNearestCentroidBalanced, ModelNearestCentroidCosine, ModelNearestCentroidManhattan,
		ModelNaiveBayes, ModelNaiveBayesBalanced,
		ModelRidge, ModelRidgeLight, ModelRidgeStrong,
		ModelAdaBoost, ModelAdaBoostFast, ModelAdaBoostLarge,
		ModelEnsemble,
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
	case ModelKNN:
		return "K-Nearest Neighbors"
	case ModelLogisticRegression:
		return "Logistic Regression"
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
	default:
		return string(t)
	}
}
