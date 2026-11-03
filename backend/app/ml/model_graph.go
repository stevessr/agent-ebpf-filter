package ml

import (
	"fmt"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/ml/graph"
)

// GraphLearningModel wraps the graph.GNNClassifier to implement app.Model.
type GraphLearningModel struct {
	Classifier *graph.GNNClassifier
}

func (m *GraphLearningModel) Type() ModelType {
	return core.ModelGraphLearning
}

func (m *GraphLearningModel) Predict(features [FeatureDim]float64) Prediction {
	if m.Classifier == nil || !m.Classifier.IsTrained {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0}
	}

	featSlice := features[:]
	bestClass, bestProb, anomalyScore := m.Classifier.PredictClass(featSlice)

	return Prediction{
		Action:       int32(bestClass),
		Confidence:   bestProb,
		AnomalyScore: anomalyScore,
	}
}

func (m *GraphLearningModel) Serialize(path string) error {
	if m.Classifier == nil {
		return fmt.Errorf("classifier is not initialized")
	}
	return m.Classifier.Serialize(path)
}

func DeserializeGraphLearning(path string) (*GraphLearningModel, error) {
	clf, err := graph.DeserializeGNNClassifier(path)
	if err != nil {
		return nil, err
	}
	return &GraphLearningModel{
		Classifier: clf,
	}, nil
}

// trainGraph trains the GNN classifier on the stored dataset.
func (t *ModelTrainer) trainGraph(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}
	_, _, trainRaw, validationRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: fmt.Sprintf("independent holdout: %v", err)}
	}

	trainFeats := make([][]float64, len(trainRaw))
	trainLabels := make([]int, len(trainRaw))
	for i, sample := range trainRaw {
		trainFeats[i] = make([]float64, FeatureDim)
		copy(trainFeats[i], sample.Features[:])
		trainLabels[i] = int(sample.Label)
	}
	valFeats := make([][]float64, len(validationRaw))
	valLabels := make([]int, len(validationRaw))
	for i, sample := range validationRaw {
		valFeats[i] = make([]float64, FeatureDim)
		copy(valFeats[i], sample.Features[:])
		valLabels[i] = int(sample.Label)
	}

	gnnCfg := graph.DefaultGNNConfig()
	if cfg.NumTrees > 0 {
		gnnCfg.HiddenDim = cfg.NumTrees
	}
	if cfg.MaxDepth > 0 {
		gnnCfg.NumLayers = cfg.MaxDepth
	}
	if cfg.MinSamplesLeaf > 0 {
		gnnCfg.Epochs = cfg.MinSamplesLeaf
	}

	clf := graph.NewGNNClassifier(gnnCfg)
	m := &GraphLearningModel{Classifier: clf}
	t.Logf("Training Graph Neural Network: hiddenDim=%d, layers=%d, epochs=%d, batchSize=%d, samples=%d",
		gnnCfg.HiddenDim, gnnCfg.NumLayers, gnnCfg.Epochs, gnnCfg.BatchSize, len(trainFeats))

	finalLoss := clf.Train(trainFeats, trainLabels, gnnCfg.Epochs, gnnCfg.BatchSize, func(p float64) {
		t.setTrainingProgress(p)
		percent := int(p * 100)
		if percent%10 == 0 && percent > 0 {
			t.Logf("GNN training progress: %d%%...", percent)
		}
	})
	t.Logf("GNN training complete. Final cross-entropy loss: %.4f", finalLoss)

	trainCorrect := 0
	for i, features := range trainFeats {
		class, _, _ := clf.PredictClass(features)
		if class == trainLabels[i] {
			trainCorrect++
		}
	}
	trainAccuracy := float64(trainCorrect) / float64(len(trainFeats))
	validationCorrect := 0
	for i, features := range valFeats {
		class, _, _ := clf.PredictClass(features)
		if class == valLabels[i] {
			validationCorrect++
		}
	}
	validationAccuracy := float64(validationCorrect) / float64(len(valFeats))

	t.Logf("GNN accuracy: train=%.2f%%, validation=%.2f%%", trainAccuracy*100, validationAccuracy*100)
	t.finishMetrics(validationAccuracy, trainAccuracy, validationAccuracy, len(labeled), len(trainRaw), len(validationRaw))
	t.setLastSplit(trainRaw, validationRaw)
	return m, TrainResult{
		Accuracy:           validationAccuracy,
		TrainAccuracy:      trainAccuracy,
		ValidationAccuracy: validationAccuracy,
		NumSamples:         len(labeled),
		TrainSamples:       len(trainRaw),
		ValidationSamples:  len(validationRaw),
	}
}

func init() {
	RegisterModel(core.ModelGraphLearning, func() Model {
		cfg := graph.DefaultGNNConfig()
		clf := graph.NewGNNClassifier(cfg)
		clf.IsTrained = true // default to trained for the basic registered instance
		return &GraphLearningModel{Classifier: clf}
	})
}
