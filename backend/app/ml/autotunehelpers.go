package ml

import (
	"math"
	"math/rand"
	"strings"
)

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

// trainAdaBoostFromData is the compatibility entry point for the AutoTune
// candidate grid. The learner uses genuine four-class SAMME, not the former
// binary BLOCK-vs-other stump loss (which could never predict REWRITE/ALERT).
func trainAdaBoostFromData(X [][FeatureDim]float64, Y []int32, nEst int) *AdaBoostModel {
	return trainAdaBoostFromDataBalanced(X, Y, nEst, false)
}

func trainAdaBoostFromDataBalanced(X [][FeatureDim]float64, Y []int32, nEst int, balance bool) *AdaBoostModel {
	if nEst < 10 {
		nEst = 50
	}
	model := NewAdaBoost(nEst)
	n := len(X)
	if n < 2 || n != len(Y) {
		return model
	}
	var counts [4]int
	for _, label := range Y {
		if label < 0 || label >= 4 {
			return model
		}
		counts[label]++
	}
	weights := make([]float64, n)
	normalize := func() bool {
		total := 0.0
		for _, weight := range weights {
			total += weight
		}
		if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
			return false
		}
		for i := range weights {
			weights[i] /= total
		}
		return true
	}
	for i, label := range Y {
		weights[i] = 1
		if balance {
			weights[i] = 1 / float64(counts[label])
		}
	}
	if !normalize() {
		return model
	}
	// Use a local stable RNG so comparing grid cells does not depend on
	// request timing or the process-global random stream.
	rng := rand.New(rand.NewSource(0x414441424f4f5354))
	for e := 0; e < nEst; e++ {
		best := AdaBoostStump{Feature: -1}
		bestError := math.Inf(1)
		for trial := 0; trial < 50; trial++ {
			feature := rng.Intn(FeatureDim)
			threshold := X[rng.Intn(n)][feature]
			var sides [2][4]float64
			var totals [2]float64
			for i, sample := range X {
				side := 0
				if sample[feature] >= threshold {
					side = 1
				}
				sides[side][Y[i]] += weights[i]
				totals[side] += weights[i]
			}
			if totals[0] <= 0 || totals[1] <= 0 {
				continue
			}
			votes := [2]int{}
			correct := 0.0
			for side := 0; side < 2; side++ {
				for c := 1; c < 4; c++ {
					if sides[side][c] > sides[side][votes[side]] {
						votes[side] = c
					}
				}
				correct += sides[side][votes[side]]
			}
			err := 1 - correct
			if err < bestError {
				bestError = err
				best = AdaBoostStump{Feature: feature, Threshold: threshold,
					LeftVote: float64(votes[0]), RightVote: float64(votes[1])}
			}
		}
		if best.Feature < 0 || bestError >= 1-1/float64(model.Classes) {
			continue
		}
		err := math.Max(1e-10, bestError)
		alpha := math.Log((1-err)/err) + math.Log(float64(model.Classes-1))
		if alpha <= 0 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
			continue
		}
		model.Stumps = append(model.Stumps, best)
		model.Alphas = append(model.Alphas, alpha)
		penalty := math.Exp(alpha)
		for i, sample := range X {
			vote := int32(best.RightVote)
			if sample[best.Feature] < best.Threshold {
				vote = int32(best.LeftVote)
			}
			if vote != Y[i] {
				weights[i] *= penalty
			}
		}
		if !normalize() {
			break
		}
	}
	return model
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
