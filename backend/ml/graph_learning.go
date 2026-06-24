package ml

import (
	"fmt"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/ml/graph"
)

// GraphLearningModel wraps the real graph.GNNClassifier to implement the Model interface.
type GraphLearningModel struct {
	Classifier *graph.GNNClassifier
}

// Type returns the model type identifier.
func (g *GraphLearningModel) Type() core.ModelType {
	return core.ModelGraphLearning
}

// NewGraphLearningModel creates a new GNN-style model wrapper.
func NewGraphLearningModel() *GraphLearningModel {
	cfg := graph.DefaultGNNConfig()
	return &GraphLearningModel{
		Classifier: graph.NewGNNClassifier(cfg),
	}
}

// Predict runs inference using graph message passing.
func (g *GraphLearningModel) Predict(features [FeatureDim]float64) Prediction {
	if g.Classifier == nil || !g.Classifier.IsTrained {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0}
	}

	featSlice := features[:]
	bestClass, bestProb, anomalyScore := g.Classifier.PredictClass(featSlice)

	return Prediction{
		Action:       int32(bestClass),
		Confidence:   bestProb,
		AnomalyScore: anomalyScore,
	}
}

// Serialize saves the graph model to a binary file using Gob encoding.
func (g *GraphLearningModel) Serialize(path string) error {
	if g.Classifier == nil {
		return fmt.Errorf("classifier is not initialized")
	}
	return g.Classifier.Serialize(path)
}

// DeserializeGraphLearning loads a graph model from a binary file.
func DeserializeGraphLearning(path string) (*GraphLearningModel, error) {
	clf, err := graph.DeserializeGNNClassifier(path)
	if err != nil {
		return nil, err
	}
	return &GraphLearningModel{
		Classifier: clf,
	}, nil
}

func init() {
	RegisterModel(core.ModelGraphLearning, func() Model {
		m := NewGraphLearningModel()
		m.Classifier.IsTrained = true // allow running predictions (with default randomized weights)
		return m
	})
}
