package main

import (
	"math"
	"math/rand"
	"strings"
	"time"
)

func normalizeAutoTuneAxis(axis string) string {
	switch strings.ToLower(strings.TrimSpace(axis)) {
	case "numtrees", "trees", "num_trees", "k", "learningrate", "learning_rate", "alpha", "nestimators", "n_estimators":
		return "numTrees"
	case "maxdepth", "depth", "max_depth", "distance", "regularization":
		return "maxDepth"
	case "minsamplesleaf", "min_samples_leaf", "leaf", "weight", "weights", "maxiterations", "max_iterations", "iterations":
		return "minSamplesLeaf"
	default:
		return ""
	}
}

func normalizeAutoTuneMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "", "validationaccuracy", "accuracy", "backtestaccuracy", "backtest", "validation":
		return "validationAccuracy"
	case "inferencethroughput", "throughput", "speed", "inferencespeed":
		return "inferenceThroughput"
	default:
		return ""
	}
}

func normalizeAutoTuneGridSize(size int) int {
	if size < 3 {
		size = 3
	}
	if size > 11 {
		size = 11
	}
	if size%2 == 0 {
		size++
		if size > 11 {
			size -= 2
		}
	}
	if size < 3 {
		size = 3
	}
	return size
}

func normalizeAutoTuneGranularity(granularity float64) float64 {
	switch {
	case granularity >= 3:
		return 4
	case granularity >= 1.5:
		return 2
	default:
		return 1
	}
}

func autoTuneAxisValues(axis string, gridSize int, granularity float64, numTrees, maxDepth, minSamplesLeaf int) []int {
	center := axisCenter(axis, numTrees, maxDepth, minSamplesLeaf)
	minValue, maxValue := autoTuneAxisRange(axis, center, gridSize, granularity)
	return linspaceInt(minValue, maxValue, gridSize)
}

func autoTuneAxisValuesWithRange(axis string, gridSize int, granularity float64, numTrees, maxDepth, minSamplesLeaf int, minOverride, maxOverride *int) []int {
	if minOverride != nil && maxOverride != nil && *minOverride > 0 && *maxOverride >= *minOverride {
		return linspaceInt(*minOverride, *maxOverride, gridSize)
	}
	center := axisCenter(axis, numTrees, maxDepth, minSamplesLeaf)
	minValue, maxValue := autoTuneAxisRange(axis, center, gridSize, granularity)
	return linspaceInt(minValue, maxValue, gridSize)
}

func axisCenter(axis string, numTrees, maxDepth, minSamplesLeaf int) int {
	switch axis {
	case "maxDepth":
		return maxDepth
	case "minSamplesLeaf":
		return minSamplesLeaf
	default:
		return numTrees
	}
}

func autoTuneAxisRange(axis string, center, gridSize int, granularity float64) (int, int) {
	minBound, maxBound := autoTuneAxisBounds(axis)
	step := autoTuneAxisStep(axis, granularity)
	radius := gridSize / 2

	minValue := center - step*radius
	maxValue := center + step*radius

	if minValue < minBound {
		maxValue += minBound - minValue
		minValue = minBound
	}
	if maxValue > maxBound {
		minValue -= maxValue - maxBound
		maxValue = maxBound
	}

	minValue = autoTuneClampInt(minValue, minBound, maxBound)
	maxValue = autoTuneClampInt(maxValue, minBound, maxBound)
	if maxValue < minValue {
		maxValue = minValue
	}
	return minValue, maxValue
}

func autoTuneAxisStep(axis string, granularity float64) int {
	if granularity <= 0 {
		granularity = 1
	}
	base := 1
	if axis == "numTrees" {
		base = 5
	}
	step := int(math.Round(float64(base) / granularity))
	if step < 1 {
		step = 1
	}
	return step
}

func autoTuneAxisBounds(axis string) (int, int) {
	switch axis {
	case "maxDepth":
		return 3, 20
	case "minSamplesLeaf":
		return 1, 50
	default:
		return 5, 200
	}
}

func setAutoTuneAxisValue(axis string, value int, numTrees, maxDepth, minSamplesLeaf int) (int, int, int) {
	switch axis {
	case "numTrees":
		return value, maxDepth, minSamplesLeaf
	case "maxDepth":
		return numTrees, value, minSamplesLeaf
	case "minSamplesLeaf":
		return numTrees, maxDepth, value
	default:
		return numTrees, maxDepth, minSamplesLeaf
	}
}

func maxAxisValue(axisA string, valuesA []int, axisB string, valuesB []int, target string) int {
	maxValue := 0
	if axisA == target {
		for _, v := range valuesA {
			if v > maxValue {
				maxValue = v
			}
		}
	}
	if axisB == target {
		for _, v := range valuesB {
			if v > maxValue {
				maxValue = v
			}
		}
	}
	return maxValue
}

func linspaceInt(minValue, maxValue, count int) []int {
	if count <= 1 {
		return []int{minValue}
	}
	if maxValue < minValue {
		minValue, maxValue = maxValue, minValue
	}
	if minValue == maxValue {
		values := make([]int, count)
		for i := range values {
			values[i] = minValue
		}
		return values
	}

	values := make([]int, count)
	step := float64(maxValue-minValue) / float64(count-1)
	for i := 0; i < count; i++ {
		values[i] = int(math.Round(float64(minValue) + step*float64(i)))
	}
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			values[i] = values[i-1]
		}
	}
	return values
}

func autoTuneClampInt(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func extractTrainData(samples []trainSample) ([][FeatureDim]float64, []int32) {
	X := make([][FeatureDim]float64, len(samples))
	Y := make([]int32, len(samples))
	for i, s := range samples {
		X[i] = s.features
		Y[i] = s.label
	}
	return X, Y
}

func trainAdaBoostFromData(X [][FeatureDim]float64, Y []int32, nEst int) *AdaBoostModel {
	n := len(X)
	if nEst < 10 {
		nEst = 50
	}
	m := NewAdaBoost(nEst)
	weights := make([]float64, n)
	for i := range weights {
		weights[i] = 1.0 / float64(n)
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for e := 0; e < nEst; e++ {
		cum := make([]float64, n)
		cum[0] = weights[0]
		for i := 1; i < n; i++ {
			cum[i] = cum[i-1] + weights[i]
		}
		totalW := cum[n-1]
		bestStump := adaboostStump{Feature: -1}
		bestErr := 1e9
		for tries := 0; tries < 30; tries++ {
			fi := rng.Intn(FeatureDim)
			thresh := X[rng.Intn(n)][fi]
			var lErr, rErr, lW, rW float64
			for i := 0; i < n; i++ {
				cl := 0
				if Y[i] == 1 {
					cl = 1
				}
				if X[i][fi] < thresh {
					if cl != 1 {
						lErr += weights[i]
					}
					lW += weights[i]
				} else {
					if cl != 0 {
						rErr += weights[i]
					}
					rW += weights[i]
				}
			}
			err := (lErr + rErr) / totalW
			if err < bestErr {
				bestErr = err
				bestStump = adaboostStump{Feature: fi, Threshold: thresh, LeftVote: 1, RightVote: 0}
				if lErr/lW > rErr/rW {
					bestStump.LeftVote = 0
					bestStump.RightVote = 1
				}
			}
		}
		if bestStump.Feature < 0 {
			continue
		}
		err := math.Max(bestErr, 1e-10)
		alpha := 0.5 * math.Log((1-err)/err)
		if alpha <= 0 {
			continue
		}
		for i := 0; i < n; i++ {
			pred := 0
			if X[i][bestStump.Feature] < bestStump.Threshold {
				pred = int(bestStump.LeftVote)
			} else {
				pred = int(bestStump.RightVote)
			}
			cl := int(Y[i])
			if pred != cl {
				weights[i] *= math.Exp(alpha)
			}
		}
		m.Stumps = append(m.Stumps, bestStump)
		m.Alphas = append(m.Alphas, alpha)
	}
	return m
}

func autoTuneMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// evalKNNModel evaluates a KNN model on a set of samples.
func evalKNNModel(model *KNNModel, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		pred := model.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

// evalLogisticModel evaluates a logistic regression model on a set of samples.
func evalLogisticModel(model *LogisticModel, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		pred := model.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}
