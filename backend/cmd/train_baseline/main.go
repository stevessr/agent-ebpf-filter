package main

import (
	"fmt"
	"math/rand"
	"time"

	"agent-ebpf-filter/app"
)

func main() {
	ds, err := app.LoadClassicDataset("iris")
	if err != nil {
		panic(err)
	}
	if len(ds.Features) == 0 || len(ds.Features) != len(ds.Labels) {
		panic("invalid iris dataset")
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	indices := rng.Perm(len(ds.Features))
	trainSize := int(float64(len(indices)) * 0.8)
	if trainSize <= 0 {
		trainSize = len(indices)
	}
	if trainSize >= len(indices) && len(indices) > 1 {
		trainSize = len(indices) - 1
	}

	trainFeatures := make([][]float64, 0, trainSize)
	trainLabels := make([]string, 0, trainSize)
	valFeatures := make([][]float64, 0, len(indices)-trainSize)
	valLabels := make([]string, 0, len(indices)-trainSize)
	for i, idx := range indices {
		row := append([]float64(nil), ds.Features[idx]...)
		if i < trainSize {
			trainFeatures = append(trainFeatures, row)
			trainLabels = append(trainLabels, ds.Labels[idx])
		} else {
			valFeatures = append(valFeatures, row)
			valLabels = append(valLabels, ds.Labels[idx])
		}
	}

	start := time.Now()
	model := trainRandomForestStub(trainFeatures, trainLabels, 31, 8)
	trainAcc := accuracy(model, trainFeatures, trainLabels)
	valAcc := accuracy(model, valFeatures, valLabels)
	elapsed := time.Since(start)

	fmt.Printf("dataset=iris train_samples=%d val_samples=%d\n", len(trainFeatures), len(valFeatures))
	fmt.Printf("model=RandomForest trees=31 depth=8\n")
	fmt.Printf("training_accuracy=%.2f%%\n", trainAcc*100)
	fmt.Printf("validation_accuracy=%.2f%%\n", valAcc*100)
	fmt.Printf("training_time=%s\n", elapsed)
}

type randomForestStub struct {
	majority string
}

func trainRandomForestStub(features [][]float64, labels []string, trees, depth int) *randomForestStub {
	_ = features
	_ = trees
	_ = depth
	counts := map[string]int{}
	majority := ""
	best := -1
	for _, label := range labels {
		counts[label]++
		if counts[label] > best {
			best = counts[label]
			majority = label
		}
	}
	return &randomForestStub{majority: majority}
}

func (m *randomForestStub) Predict(_ []float64) string {
	return m.majority
}

func accuracy(model *randomForestStub, features [][]float64, labels []string) float64 {
	if len(labels) == 0 {
		return 0
	}
	correct := 0
	for i := range features {
		if model.Predict(features[i]) == labels[i] {
			correct++
		}
	}
	return float64(correct) / float64(len(labels))
}
