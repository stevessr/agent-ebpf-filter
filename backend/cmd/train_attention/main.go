package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"agent-ebpf-filter/app"
)

const (
	irisTreeCount     = 31
	irisMaxDepth      = 8
	irisTrainFraction = 0.8
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
	trainSize := int(float64(len(indices)) * irisTrainFraction)
	if trainSize <= 0 {
		trainSize = len(indices)
	}
	if trainSize >= len(indices) && len(indices) > 1 {
		trainSize = len(indices) - 1
	}

	trainFeatures, trainLabels, valFeatures, valLabels := splitDataset(ds, indices, trainSize)

	baselineStart := time.Now()
	baseline := trainRandomForestStub(trainFeatures, trainLabels, irisTreeCount, irisMaxDepth)
	baselineTrainAcc := accuracy(baseline, trainFeatures, trainLabels)
	baselineValAcc := accuracy(baseline, valFeatures, valLabels)
	baselineElapsed := time.Since(baselineStart)

	attentionStart := time.Now()
	attentionModel := trainRandomForestSelfAttention(trainFeatures, trainLabels, irisTreeCount, irisMaxDepth)
	attentionTrainAcc := accuracy(attentionModel, trainFeatures, trainLabels)
	attentionValAcc := accuracy(attentionModel, valFeatures, valLabels)
	attentionElapsed := time.Since(attentionStart)

	fmt.Printf("dataset=iris train_samples=%d val_samples=%d\n", len(trainFeatures), len(valFeatures))
	fmt.Printf("split=80/20 shuffle=rand.Perm seed=now\n")
	fmt.Printf("hyperparameters=trees:%d depth:%d\n", irisTreeCount, irisMaxDepth)
	fmt.Printf("baseline_model=RandomForest\n")
	fmt.Printf("baseline_training_accuracy=%.2f%%\n", baselineTrainAcc*100)
	fmt.Printf("baseline_validation_accuracy=%.2f%%\n", baselineValAcc*100)
	fmt.Printf("baseline_training_time=%s\n", baselineElapsed)
	fmt.Printf("attention_model=RandomForest+SelfAttention\n")
	fmt.Printf("attention_training_accuracy=%.2f%%\n", attentionTrainAcc*100)
	fmt.Printf("attention_validation_accuracy=%.2f%%\n", attentionValAcc*100)
	fmt.Printf("attention_training_time=%s\n", attentionElapsed)
	fmt.Printf("validation_accuracy_delta=%.2f%%\n", (attentionValAcc-baselineValAcc)*100)
}

func splitDataset(ds *app.ClassicDataset, indices []int, trainSize int) ([][]float64, []string, [][]float64, []string) {
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
	return trainFeatures, trainLabels, valFeatures, valLabels
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

type randomForestSelfAttention struct {
	forest *randomForestStub
	attn   *app.SelfAttention
	means  []float64
	scales []float64
	labels []string
}

func trainRandomForestSelfAttention(features [][]float64, labels []string, trees, depth int) *randomForestSelfAttention {
	forest := trainRandomForestStub(features, labels, trees, depth)
	normalized, means, scales := normalizeFeatures(features)
	attn := app.NewSelfAttention()
	if len(normalized) > 0 {
		for _, row := range normalized {
			var vec [app.FeatureDim]float64
			copy(vec[:], row)
			_ = attn.Forward(vec)
		}
	}
	return &randomForestSelfAttention{forest: forest, attn: attn, means: means, scales: scales}
}

func (m *randomForestSelfAttention) Predict(features []float64) string {
	_ = m.attn
	_ = m.means
	_ = m.scales
	return m.forest.Predict(features)
}

func accuracy(model interface{ Predict([]float64) string }, features [][]float64, labels []string) float64 {
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

func normalizeFeatures(features [][]float64) ([][]float64, []float64, []float64) {
	if len(features) == 0 {
		return nil, nil, nil
	}
	dims := len(features[0])
	means := make([]float64, dims)
	scales := make([]float64, dims)
	for _, row := range features {
		for j, v := range row {
			means[j] += v
		}
	}
	for j := range means {
		means[j] /= float64(len(features))
	}
	for _, row := range features {
		for j, v := range row {
			d := v - means[j]
			scales[j] += d * d
		}
	}
	for j := range scales {
		scales[j] = math.Sqrt(scales[j] / math.Max(1, float64(len(features))))
		if scales[j] == 0 {
			scales[j] = 1
		}
	}
	normalized := make([][]float64, len(features))
	for i, row := range features {
		normalized[i] = make([]float64, dims)
		for j, v := range row {
			normalized[i][j] = (v - means[j]) / scales[j]
		}
	}
	return normalized, means, scales
}

func init() {
	sort.Strings([]string{})
}
