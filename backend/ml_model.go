package main

import "fmt"

// ModelType identifies the ML model algorithm
type ModelType string

const (
	ModelRandomForest        ModelType = "random_forest"
	ModelKNN                 ModelType = "knn"
	ModelLogisticRegression  ModelType = "logistic"
	ModelNaiveBayes          ModelType = "naive_bayes"
	ModelExtraTrees          ModelType = "extra_trees"
	ModelAdaBoost            ModelType = "adaboost"
	ModelSVM                 ModelType = "svm"
	ModelRidge               ModelType = "ridge"
	ModelPerceptron          ModelType = "perceptron"
	ModelPassiveAggressive   ModelType = "passive_aggressive"
	ModelEnsemble            ModelType = "ensemble"
)

// AllModelTypes returns all registered model types
func AllModelTypes() []ModelType {
	return []ModelType{
		ModelRandomForest, ModelKNN, ModelLogisticRegression,
		ModelNaiveBayes, ModelExtraTrees, ModelAdaBoost,
		ModelSVM, ModelRidge, ModelPerceptron, ModelPassiveAggressive, ModelEnsemble,
	}
}

// Model is the interface that all ML models must implement
type Model interface {
	Predict(features [FeatureDim]float64) Prediction
	Serialize(path string) error
	Type() ModelType
}

// ModelFactory creates a new untrained model instance
type ModelFactory func() Model

var modelRegistry = map[ModelType]ModelFactory{}

// RegisterModel registers a model type and its factory function
func RegisterModel(t ModelType, factory ModelFactory) {
	modelRegistry[t] = factory
}

// NewModel creates a new model instance of the given type
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

// modelName returns a human-readable name for a model type
func modelName(t ModelType) string {
	switch t {
	case ModelRandomForest:
		return "Random Forest"
	case ModelKNN:
		return "K-Nearest Neighbors"
	case ModelLogisticRegression:
		return "Logistic Regression"
	case ModelNaiveBayes:
		return "Naive Bayes"
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
