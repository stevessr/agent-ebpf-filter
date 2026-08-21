package ml

import (
	"fmt"
	"math/rand"
	"time"

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

	features := make([][]float64, len(labeled))
	labels := make([]int, len(labeled))
	for i, s := range labeled {
		features[i] = make([]float64, FeatureDim)
		copy(features[i], s.Features[:])
		labels[i] = int(s.Label)
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

	// Split into train/validation sets
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	indices := rng.Perm(len(labeled))
	splitIdx := int(float64(len(labeled)) * (1.0 - cfg.ValidationSplitRatio))
	if splitIdx < 1 {
		splitIdx = 1
	}

	trainFeats := make([][]float64, splitIdx)
	trainLabels := make([]int, splitIdx)
	for i := 0; i < splitIdx; i++ {
		trainFeats[i] = features[indices[i]]
		trainLabels[i] = labels[indices[i]]
	}

	valFeats := make([][]float64, len(labeled)-splitIdx)
	valLabels := make([]int, len(labeled)-splitIdx)
	for i := splitIdx; i < len(labeled); i++ {
		valFeats[i-splitIdx] = features[indices[i]]
		valLabels[i-splitIdx] = labels[indices[i]]
	}

	clf := graph.NewGNNClassifier(gnnCfg)
	m := &GraphLearningModel{Classifier: clf}

	t.Logf("Training Graph Neural Network: hiddenDim=%d, layers=%d, epochs=%d, batchSize=%d, samples=%d",
		gnnCfg.HiddenDim, gnnCfg.NumLayers, gnnCfg.Epochs, gnnCfg.BatchSize, len(trainFeats))

	// Train GNN
	finalLoss := clf.Train(trainFeats, trainLabels, gnnCfg.Epochs, gnnCfg.BatchSize, func(p float64) {
		t.setTrainingProgress(p)
		// Log every 10%
		percent := int(p * 100)
		if percent%10 == 0 && percent > 0 {
			t.Logf("GNN training progress: %d%%...", percent)
		}
	})

	t.Logf("GNN training complete. Final cross-entropy loss: %.4f", finalLoss)

	// Evaluation
	trainCorrect := 0
	for i, feat := range trainFeats {
		cls, _, _ := clf.PredictClass(feat)
		if cls == trainLabels[i] {
			trainCorrect++
		}
	}
	trainAcc := float64(trainCorrect) / float64(len(trainFeats))

	valAcc := trainAcc
	if len(valFeats) > 0 {
		valCorrect := 0
		for i, feat := range valFeats {
			cls, _, _ := clf.PredictClass(feat)
			if cls == valLabels[i] {
				valCorrect++
			}
		}
		valAcc = float64(valCorrect) / float64(len(valFeats))
	}

	t.Logf("GNN accuracy: train=%.2f%%, validation=%.2f%%", trainAcc*100, valAcc*100)

	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainFeats), len(valFeats))

	shuffledLabeled := make([]TrainingSample, len(labeled))
	for i, idx := range indices {
		shuffledLabeled[i] = labeled[idx]
	}
	t.setLastSplit(shuffledLabeled[:splitIdx], shuffledLabeled[splitIdx:])

	return m, TrainResult{
		Accuracy:           valAcc,
		TrainAccuracy:      trainAcc,
		ValidationAccuracy: valAcc,
		NumSamples:         len(labeled),
		TrainSamples:       len(trainFeats),
		ValidationSamples:  len(valFeats),
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
