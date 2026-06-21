package main

import (
	"fmt"
	"math"
	"sort"

	"agent-ebpf-filter/app"
)

type datasetSplit struct {
	trainX [][]float64
	trainY []int32
	valX   [][]float64
	valY   []int32
}

func main() {
	fmt.Println("=== Wine 数据集 Logistic+Attention 训练对比 ===\n")

	ds, err := app.LoadClassicDataset("wine")
	if err != nil {
		panic(err)
	}

	fmt.Printf("数据集: %s\n", ds.Name)
	fmt.Printf("样本数: %d, 特征数: %d\n", len(ds.Features), len(ds.Features[0]))
	fmt.Printf("类别数: %d\n\n", classCount(ds.Labels))

	split := makeSplit(ds.Features, ds.Labels, 0.8)
	fmt.Printf("数据划分: %d 训练 / %d 验证\n\n", len(split.trainX), len(split.valX))

	logistic := app.NewLogisticModel(0.02, "l2", 800)
	logistic.NumClasses = classCount(ds.Labels)
	logistic.Train(toFixed(split.trainX), split.trainY)

	baseTrainAcc, baseValAcc := evaluateLogistic(logistic, split)

	attnBase := app.NewLogisticModel(0.02, "l2", 800)
	attnBase.NumClasses = classCount(ds.Labels)
	attnModel := newAttentionLogisticModel(attnBase)
	attnModel.Train(toFixed(split.trainX), split.trainY)

	attnTrainAcc, attnValAcc := evaluateAttentionModel(attnModel, split)

	fmt.Println("模型对比:")
	fmt.Printf("  %-18s 训练准确率: %6.2f%%  验证准确率: %6.2f%%\n", "Logistic", baseTrainAcc*100, baseValAcc*100)
	fmt.Printf("  %-18s 训练准确率: %6.2f%%  验证准确率: %6.2f%%\n", "Logistic+Attention", attnTrainAcc*100, attnValAcc*100)
	fmt.Printf("  %-18s 验证集提升: %+.2f%%\n\n", "Attention Gain", (attnValAcc-baseValAcc)*100)

	best := "Logistic"
	bestAcc := baseValAcc
	if attnValAcc > bestAcc {
		best = "Logistic+Attention"
		bestAcc = attnValAcc
	}

	fmt.Println("总结:")
	fmt.Printf("  最优模型: %s\n", best)
	fmt.Printf("  最佳验证准确率: %.2f%%\n", bestAcc*100)
}

func classCount(labels []string) int {
	seen := map[string]int{}
	for _, label := range labels {
		if _, ok := seen[label]; !ok {
			seen[label] = len(seen)
		}
	}
	return len(seen)
}

func makeSplit(features [][]float64, labels []string, trainRatio float64) datasetSplit {
	idx := make([]int, len(features))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return idx[i] < idx[j] })
	trainN := int(math.Round(float64(len(features)) * trainRatio))
	if trainN <= 0 {
		trainN = 1
	}
	if trainN >= len(features) {
		trainN = len(features) - 1
	}

	split := datasetSplit{}
	labelMap := map[string]int32{}
	for _, label := range labels {
		if _, ok := labelMap[label]; !ok {
			labelMap[label] = int32(len(labelMap))
		}
	}
	for i, row := range features {
		if i < trainN {
			split.trainX = append(split.trainX, row)
			split.trainY = append(split.trainY, labelMap[labels[i]])
		} else {
			split.valX = append(split.valX, row)
			split.valY = append(split.valY, labelMap[labels[i]])
		}
	}
	return split
}

func toFixed(xs [][]float64) [][app.FeatureDim]float64 {
	out := make([][app.FeatureDim]float64, len(xs))
	for i, row := range xs {
		for j := 0; j < app.FeatureDim && j < len(row); j++ {
			out[i][j] = row[j]
		}
	}
	return out
}

func evaluateLogistic(m *app.LogisticModel, split datasetSplit) (float64, float64) {
	return accuracyLogistic(m, toFixed(split.trainX), split.trainY), accuracyLogistic(m, toFixed(split.valX), split.valY)
}

func accuracyLogistic(m *app.LogisticModel, xs [][app.FeatureDim]float64, ys []int32) float64 {
	if len(xs) == 0 {
		return 0
	}
	correct := 0
	for i, x := range xs {
		if m.Predict(x).Action == ys[i] {
			correct++
		}
	}
	return float64(correct) / float64(len(xs))
}

type attentionLogisticModel struct {
	attention *app.SelfAttention
	base      *app.LogisticModel
}

func newAttentionLogisticModel(base *app.LogisticModel) *attentionLogisticModel {
	return &attentionLogisticModel{attention: app.NewSelfAttention(), base: base}
}

func (m *attentionLogisticModel) Train(samples [][app.FeatureDim]float64, labels []int32) {
	m.base.Train(samples, labels)
}

func (m *attentionLogisticModel) Predict(x [app.FeatureDim]float64) app.Prediction {
	return m.base.Predict(m.attention.Forward(x))
}

func evaluateAttentionModel(m *attentionLogisticModel, split datasetSplit) (float64, float64) {
	return accuracyAttention(m, toFixed(split.trainX), split.trainY), accuracyAttention(m, toFixed(split.valX), split.valY)
}

func accuracyAttention(m *attentionLogisticModel, xs [][app.FeatureDim]float64, ys []int32) float64 {
	if len(xs) == 0 {
		return 0
	}
	correct := 0
	for i, x := range xs {
		if m.Predict(x).Action == ys[i] {
			correct++
		}
	}
	return float64(correct) / float64(len(xs))
}
