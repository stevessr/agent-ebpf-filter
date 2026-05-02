package main

import "fmt"

// ModelType identifies the ML model algorithm
type ModelType string

const (
	ModelRandomForest      ModelType = "random_forest"
	ModelKNN               ModelType = "knn"
	ModelLogisticRegression ModelType = "logistic"
)

// AllModelTypes returns all registered model types
func AllModelTypes() []ModelType {
	return []ModelType{ModelRandomForest, ModelKNN, ModelLogisticRegression}
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
	default:
		return string(t)
	}
}
