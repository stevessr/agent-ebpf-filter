package ml

import (
	"math"
	"math/rand"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section autotunehelpers.go ----

func NormalizeAutoTuneAxis(axis string) string {
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

func NormalizeAutoTuneMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "", "validationaccuracy", "accuracy", "backtestaccuracy", "backtest", "validation":
		return "validationAccuracy"
	case "balancedaccuracy", "balanced", "macrorecall", "macro_recall", "balanced_accuracy":
		return "balancedAccuracy"
	case "allowrecall", "legalrecall", "benignrecall", "safecommandrecall", "safe_command_recall", "falsepositive", "false_positive":
		return "allowRecall"
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
	if size > 31 {
		size = 31
	}
	if size%2 == 0 {
		size++
		if size > 31 {
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

func extractTrainData(samples []TrainSample) ([][FeatureDim]float64, []int32) {
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
		bestStump := AdaBoostStump{Feature: -1}
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
				bestStump = AdaBoostStump{Feature: fi, Threshold: thresh, LeftVote: 1, RightVote: 0}
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
func evalKNNModel(model *KNNModel, samples []TrainSample) float64 {
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
func evalLogisticModel(model *LogisticModel, samples []TrainSample) float64 {
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

type AutoTuneClassificationMetrics struct {
	Accuracy         float64
	AllowRecall      float64
	BalancedAccuracy float64
}

func evaluateAutoTuneClassificationMetrics(samples []TrainSample, predict func([FeatureDim]float64) int32) AutoTuneClassificationMetrics {
	if len(samples) == 0 || predict == nil {
		return AutoTuneClassificationMetrics{}
	}
	var correct int
	var perClassTotal [4]int
	var perClassCorrect [4]int
	for _, sample := range samples {
		predicted := predict(sample.features)
		if sample.label >= 0 && sample.label < 4 {
			perClassTotal[sample.label]++
			if predicted == sample.label {
				perClassCorrect[sample.label]++
			}
		}
		if predicted == sample.label {
			correct++
		}
	}

	seenClasses := 0
	balancedSum := 0.0
	for cls := 0; cls < 4; cls++ {
		if perClassTotal[cls] == 0 {
			continue
		}
		seenClasses++
		balancedSum += float64(perClassCorrect[cls]) / float64(perClassTotal[cls])
	}

	allowRecall := 0.0
	if perClassTotal[0] > 0 {
		allowRecall = float64(perClassCorrect[0]) / float64(perClassTotal[0])
	}
	balancedAccuracy := 0.0
	if seenClasses > 0 {
		balancedAccuracy = balancedSum / float64(seenClasses)
	}
	return AutoTuneClassificationMetrics{
		Accuracy:         float64(correct) / float64(len(samples)),
		AllowRecall:      allowRecall,
		BalancedAccuracy: balancedAccuracy,
	}
}

func EvaluateAutoTuneTrainingSampleMetrics(samples []TrainingSample, model Model) AutoTuneClassificationMetrics {
	if model == nil {
		return AutoTuneClassificationMetrics{}
	}
	return evaluateAutoTuneClassificationMetrics(ToTrainSamples(samples), func(features [FeatureDim]float64) int32 {
		return model.Predict(features).Action
	})
}

func AutoTuneMetricScore(metric string, validationAccuracy, throughput float64, metrics AutoTuneClassificationMetrics) float64 {
	switch metric {
	case "inferenceThroughput":
		return throughput
	case "allowRecall":
		return metrics.AllowRecall
	case "balancedAccuracy":
		return metrics.BalancedAccuracy
	default:
		return validationAccuracy
	}
}
