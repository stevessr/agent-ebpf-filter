package main

import (
	"math"
	"time"
)

func (t *ModelTrainer) acquire() { t.mu <- struct{}{} }
func (t *ModelTrainer) release() { <-t.mu }

func (t *ModelTrainer) finishMetrics(acc, trainAcc, valAcc float64, total, trainN, valN int) {
	t.lastTrain = time.Now()
	t.accuracy = acc
	t.trainAccuracy = trainAcc
	t.validationAccuracy = valAcc
	t.addHistory(TrainingHistoryEntry{Timestamp: t.lastTrain, Accuracy: acc, NumSamples: total})
}

func evalModelLabeled(model Model, labeled []TrainingSample) float64 {
	if len(labeled) == 0 {
		return 0
	}
	correct := 0
	for _, s := range labeled {
		if model.Predict(s.Features).Action == s.Label {
			correct++
		}
	}
	return float64(correct) / float64(len(labeled))
}

func evalModelSamples(model Model, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		if model.Predict(s.features).Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func evalLinearModel(W [][FeatureDim + 1]float64, nClasses int, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		bestC, bestS := 0, math.Inf(-1)
		for c := 0; c < nClasses; c++ {
			score := W[c][FeatureDim]
			for d := 0; d < FeatureDim; d++ {
				score += W[c][d] * s.features[d]
			}
			if score > bestS {
				bestS = score
				bestC = c
			}
		}
		if int32(bestC) == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func toTrainSamples(labeled []TrainingSample) []trainSample {
	out := make([]trainSample, len(labeled))
	for i, s := range labeled {
		out[i] = trainSample{features: s.Features, label: s.Label}
	}
	return out
}

func toTrainingSamples(samples []trainSample) []TrainingSample {
	out := make([]TrainingSample, len(samples))
	for i, s := range samples {
		out[i] = TrainingSample{Features: s.features, Label: s.label}
	}
	return out
}
