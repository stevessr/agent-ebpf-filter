// Package ml provides ML model types, training, and inference for the
// behavior classifier.
package ml

import (
	"fmt"

	"agent-ebpf-filter/core"
)

// ModelType aliases core.ModelType for convenience within the ml package.
type ModelType = core.ModelType

// MLConfig aliases core.MLConfig for convenience within the ml package.
type MLConfig = core.MLConfig

// FeatureDim is the number of features in the ML feature vector.
const FeatureDim = core.FeatureDim

// Prediction is the output of a model inference.
// Action: ALLOW=0, BLOCK=1, REWRITE=2, ALERT=3
type Prediction struct {
	Action       int32
	Confidence   float64
	AnomalyScore float64
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

// ModelName returns a human-readable name for a model type.
func ModelName(t ModelType) string {
	if label, ok := builtinModelDisplayName(t); ok {
		return label
	}
	switch t {
	case core.ModelRandomForest:
		return "Random Forest"
	case core.ModelKNN:
		return "K-Nearest Neighbors"
	case core.ModelLogisticRegression:
		return "Logistic Regression"
	case core.ModelNaiveBayes:
		return "Naive Bayes"
	case core.ModelNearestCentroid:
		return "Nearest Centroid"
	case core.ModelExtraTrees:
		return "Extra Trees"
	case core.ModelAdaBoost:
		return "AdaBoost"
	case core.ModelSVM:
		return "Linear SVM"
	case core.ModelRidge:
		return "Ridge Classifier"
	case core.ModelPerceptron:
		return "Perceptron"
	case core.ModelPassiveAggressive:
		return "Passive-Aggressive"
	case core.ModelEnsemble:
		return "Ensemble"
	default:
		return string(t)
	}
}

// builtinModelDisplayName returns the display name for built-in model profiles.
func builtinModelDisplayName(t ModelType) (string, bool) {
	// This will be populated by ml_builtin_models.go init()
	for _, p := range builtinModelProfiles {
		if p.Type == t {
			return p.Label, true
		}
	}
	return "", false
}

// builtinModelProfiles is populated by ml_builtin_models.go
var builtinModelProfiles []builtinModelProfile

type builtinModelProfile struct {
	Type        ModelType
	Base        ModelType
	Label       string
	Category    string
	Description string
	Recommended bool
	Defaults    map[string]int
	Tags        []string
	Apply       func(MLConfig) MLConfig
}
