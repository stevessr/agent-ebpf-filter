package main

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func selectTopRepeatConfigs(profile sweepProfile, results []sweepResult, topN int, store *TrainingDataStore, benchmarkSamples []TrainingSample) []stabilityTask {
	if topN < 1 {
		topN = 1
	}
	filtered := make([]sweepResult, 0, len(results))
	for _, r := range results {
		if r.Error == "" {
			filtered = append(filtered, r)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].ValidationAccuracy != filtered[j].ValidationAccuracy {
			return filtered[i].ValidationAccuracy > filtered[j].ValidationAccuracy
		}
		if filtered[i].AllowPassRate != filtered[j].AllowPassRate {
			return filtered[i].AllowPassRate > filtered[j].AllowPassRate
		}
		if filtered[i].TrainAccuracy != filtered[j].TrainAccuracy {
			return filtered[i].TrainAccuracy > filtered[j].TrainAccuracy
		}
		if filtered[i].InferenceThroughput != filtered[j].InferenceThroughput {
			return filtered[i].InferenceThroughput > filtered[j].InferenceThroughput
		}
		if filtered[i].Duration != filtered[j].Duration {
			return filtered[i].Duration < filtered[j].Duration
		}
		if filtered[i].XValue != filtered[j].XValue {
			return filtered[i].XValue < filtered[j].XValue
		}
		return filtered[i].YValue < filtered[j].YValue
	})
	if len(filtered) > topN {
		filtered = filtered[:topN]
	}
	out := make([]stabilityTask, 0, len(filtered))
	for _, r := range filtered {
		out = append(out, stabilityTask{Profile: profile, Config: r, Store: store, BenchmarkSamples: benchmarkSamples})
	}
	return out
}

func runStabilityPhase(tasks []stabilityTask, repeats int) ([]repeatRunResult, []repeatSummary, error) {
	if repeats < 1 {
		repeats = 1
	}
	if len(tasks) == 0 {
		return nil, nil, fmt.Errorf("no stability tasks selected")
	}

	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}

	type job struct {
		Task  stabilityTask
		Index int
	}

	jobs := make(chan job)
	resultsCh := make(chan repeatRunResult)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				resultsCh <- runSingleRepeat(j.Task, j.Index)
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	go func() {
		for _, task := range tasks {
			for i := 1; i <= repeats; i++ {
				jobs <- job{Task: task, Index: i}
			}
		}
		close(jobs)
	}()

	rawRuns := make([]repeatRunResult, 0, len(tasks)*repeats)
	grouped := make(map[string][]repeatRunResult)
	order := make([]string, 0, len(tasks))
	seen := make(map[string]bool)

	for run := range resultsCh {
		rawRuns = append(rawRuns, run)
		key := repeatKey(run.Profile, run.ModelType, run.XValue, run.YValue, run.ConfigSummary)
		grouped[key] = append(grouped[key], run)
		if !seen[key] {
			seen[key] = true
			order = append(order, key)
		}
	}

	summaries := make([]repeatSummary, 0, len(order))
	for _, key := range order {
		runs := grouped[key]
		if len(runs) == 0 {
			continue
		}
		summary, err := aggregateRepeatRuns(runs)
		if err != nil {
			return rawRuns, summaries, err
		}
		summaries = append(summaries, summary)
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Comparable != summaries[j].Comparable {
			return summaries[i].Comparable && !summaries[j].Comparable
		}
		if summaries[i].ValidationMean != summaries[j].ValidationMean {
			return summaries[i].ValidationMean > summaries[j].ValidationMean
		}
		if summaries[i].SuccessRate != summaries[j].SuccessRate {
			return summaries[i].SuccessRate > summaries[j].SuccessRate
		}
		if summaries[i].ValidationStd != summaries[j].ValidationStd {
			return summaries[i].ValidationStd < summaries[j].ValidationStd
		}
		if summaries[i].InferenceMean != summaries[j].InferenceMean {
			return summaries[i].InferenceMean > summaries[j].InferenceMean
		}
		return summaries[i].DurationMean < summaries[j].DurationMean
	})

	return rawRuns, summaries, nil
}

func runSingleRepeat(task stabilityTask, repeatIndex int) repeatRunResult {
	row, err := runSingleConfig(task.Profile, task.Store, task.Config.XValue, task.Config.YValue, task.BenchmarkSamples)
	return repeatRunResult{
		Profile:             row.Profile,
		ModelType:           row.ModelType,
		XValue:              row.XValue,
		YValue:              row.YValue,
		ConfigSummary:       row.ConfigSummary,
		RunIndex:            repeatIndex,
		TrainAccuracy:       row.TrainAccuracy,
		ValidationAccuracy:  row.ValidationAccuracy,
		AllowPassRate:       row.AllowPassRate,
		NumSamples:          row.NumSamples,
		TrainSamples:        row.TrainSamples,
		ValidationSamples:   row.ValidationSamples,
		Duration:            row.Duration,
		InferenceDuration:   row.InferenceDuration,
		InferenceSamples:    row.InferenceSamples,
		InferenceLatencyMs:  row.InferenceLatencyMs,
		InferenceThroughput: row.InferenceThroughput,
		MemoryBytes:         row.MemoryBytes,
		Error:               errIfAny(row.Error, err),
	}
}

func errIfAny(rowErr string, err error) string {
	if rowErr != "" {
		return rowErr
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func aggregateRepeatRuns(runs []repeatRunResult) (repeatSummary, error) {
	if len(runs) == 0 {
		return repeatSummary{}, fmt.Errorf("no repeat runs to aggregate")
	}
	summary := repeatSummary{
		Profile:       runs[0].Profile,
		ModelType:     runs[0].ModelType,
		Comparable:    profileComparable(runs[0].Profile),
		XValue:        runs[0].XValue,
		YValue:        runs[0].YValue,
		ConfigSummary: runs[0].ConfigSummary,
		Runs:          len(runs),
	}

	trainVals := make([]float64, 0, len(runs))
	valVals := make([]float64, 0, len(runs))
	allowVals := make([]float64, 0, len(runs))
	durations := make([]float64, 0, len(runs))
	inferenceVals := make([]float64, 0, len(runs))
	inferenceLatencyVals := make([]float64, 0, len(runs))
	memoryVals := make([]float64, 0, len(runs))
	for _, r := range runs {
		durations = append(durations, r.Duration)
		memoryVals = append(memoryVals, float64(r.MemoryBytes))
		if r.Error != "" {
			summary.FailureRuns++
			continue
		}
		summary.SuccessRuns++
		trainVals = append(trainVals, r.TrainAccuracy)
		valVals = append(valVals, r.ValidationAccuracy)
		allowVals = append(allowVals, r.AllowPassRate)
		if r.InferenceThroughput > 0 {
			inferenceVals = append(inferenceVals, r.InferenceThroughput)
		}
		if r.InferenceLatencyMs > 0 {
			inferenceLatencyVals = append(inferenceLatencyVals, r.InferenceLatencyMs)
		}
	}

	if summary.SuccessRuns == 0 {
		return repeatSummary{}, fmt.Errorf("%s produced no successful repeat runs", summary.ConfigSummary)
	}

	summary.FailureRuns = summary.Runs - summary.SuccessRuns
	summary.TrainMean, summary.TrainStd = meanStd(trainVals)
	summary.ValidationMean, summary.ValidationStd = meanStd(valVals)
	summary.AllowMean, summary.AllowStd = meanStd(allowVals)
	summary.AllowMin, summary.AllowMax = minMax(allowVals)
	summary.DurationMean, summary.DurationStd = meanStd(durations)
	summary.InferenceMean, summary.InferenceStd = meanStd(inferenceVals)
	summary.InferenceLatencyMean, summary.InferenceLatencyStd = meanStd(inferenceLatencyVals)
	summary.InferenceMin, summary.InferenceMax = minMax(inferenceVals)
	summary.MemoryMean, summary.MemoryStd = meanStd(memoryVals)
	summary.MemoryMin, summary.MemoryMax = minMax(memoryVals)
	summary.TrainMin, summary.TrainMax = minMax(trainVals)
	summary.ValidationMin, summary.ValidationMax = minMax(valVals)
	summary.SuccessRate = float64(summary.SuccessRuns) / float64(summary.Runs)
	return summary, nil
}

func repeatKey(profile string, modelType ModelType, xValue, yValue int, configSummary string) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s", profile, modelType, xValue, yValue, configSummary)
}

func profileComparable(profile string) bool {
	// The sweep report uses the profile name to infer whether the model
	// currently evaluates against a holdout split in this codebase.
	profile = baseProfileSegment(profile)
	switch {
	case strings.HasPrefix(profile, "random_forest"),
		strings.HasPrefix(profile, "extra_trees"),
		strings.HasPrefix(profile, "logistic"),
		strings.HasPrefix(profile, "svm"),
		strings.HasPrefix(profile, "perceptron"),
		strings.HasPrefix(profile, "passive_aggressive"),
		strings.HasPrefix(profile, "nearest_centroid"),
		strings.HasPrefix(profile, "ensemble"):
		return true
	default:
		return false
	}
}

func baseProfileSegment(profile string) string {
	profile = strings.TrimSpace(profile)
	if strings.Contains(profile, "/") {
		parts := strings.Split(profile, "/")
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	return profile
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	var ss float64
	for _, v := range values {
		diff := v - mean
		ss += diff * diff
	}
	std := 0.0
	if len(values) > 1 {
		std = math.Sqrt(ss / float64(len(values)-1))
	}
	return mean, std
}

func minMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minV, maxV := values[0], values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func maxSweepInt(values []int) int {
	maxV := 0
	for _, value := range values {
		if value > maxV {
			maxV = value
		}
	}
	return maxV
}

func clampCount(value, maxValue int) int {
	if value < 1 {
		return 1
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func prefixDecisionForest(forest *DecisionForest, count int) *DecisionForest {
	if forest == nil || len(forest.Trees) == 0 {
		return &DecisionForest{NumClasses: 4, NumFeatures: FeatureDim}
	}
	count = clampCount(count, len(forest.Trees))
	return &DecisionForest{
		Trees:       forest.Trees[:count],
		NumClasses:  forest.NumClasses,
		MaxDepth:    forest.MaxDepth,
		NumFeatures: forest.NumFeatures,
		IsTrained:   true,
	}
}

func prefixAdaBoostModel(model *AdaBoostModel, count int) *AdaBoostModel {
	if model == nil || len(model.Stumps) == 0 {
		return NewAdaBoost(10)
	}
	count = clampCount(count, len(model.Stumps))
	return &AdaBoostModel{
		Stumps:  model.Stumps[:count],
		Alphas:  model.Alphas[:count],
		NEst:    count,
		Classes: model.Classes,
	}
}

func evalModelTrainSamples(model Model, samples []trainSample) float64 {
	if model == nil || len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, sample := range samples {
		if pred := model.Predict(sample.features); pred.Action == sample.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func bestScreenSummary(summaries []profileSummary) *profileSummary {
	if len(summaries) == 0 {
		return nil
	}
	best := &summaries[0]
	for i := 1; i < len(summaries); i++ {
		if summaries[i].Best.ValidationAccuracy > best.Best.ValidationAccuracy ||
			(summaries[i].Best.ValidationAccuracy == best.Best.ValidationAccuracy && summaries[i].Best.AllowPassRate > best.Best.AllowPassRate) ||
			(summaries[i].Best.ValidationAccuracy == best.Best.ValidationAccuracy && summaries[i].Best.InferenceThroughput > best.Best.InferenceThroughput) ||
			(summaries[i].Best.ValidationAccuracy == best.Best.ValidationAccuracy && summaries[i].Best.AllowPassRate == best.Best.AllowPassRate && summaries[i].Best.InferenceThroughput == best.Best.InferenceThroughput && summaries[i].Best.Duration < best.Best.Duration) {
			best = &summaries[i]
		}
	}
	return best
}

func bestComparableSummary(summaries []repeatSummary) *repeatSummary {
	var best *repeatSummary
	for i := range summaries {
		if !summaries[i].Comparable {
			continue
		}
		if best == nil ||
			summaries[i].ValidationMean > best.ValidationMean ||
			(summaries[i].ValidationMean == best.ValidationMean && summaries[i].AllowMean > best.AllowMean) ||
			(summaries[i].ValidationMean == best.ValidationMean && summaries[i].SuccessRate > best.SuccessRate) ||
			(summaries[i].ValidationMean == best.ValidationMean && summaries[i].AllowMean == best.AllowMean && summaries[i].SuccessRate == best.SuccessRate && summaries[i].ValidationStd < best.ValidationStd) ||
			(summaries[i].ValidationMean == best.ValidationMean && summaries[i].AllowMean == best.AllowMean && summaries[i].SuccessRate == best.SuccessRate && summaries[i].ValidationStd == best.ValidationStd && summaries[i].InferenceMean > best.InferenceMean) ||
			(summaries[i].ValidationMean == best.ValidationMean && summaries[i].AllowMean == best.AllowMean && summaries[i].SuccessRate == best.SuccessRate && summaries[i].ValidationStd == best.ValidationStd && summaries[i].InferenceMean == best.InferenceMean && summaries[i].DurationMean < best.DurationMean) {
			copy := summaries[i]
			best = &copy
		}
	}
	return best
}

func stabilityBestJSON(summaries []repeatSummary) map[string]any {
	if best := bestComparableSummary(summaries); best != nil {
		return map[string]any{
			"profile":              best.Profile,
			"modelType":            best.ModelType,
			"configSummary":        best.ConfigSummary,
			"trainMean":            best.TrainMean,
			"trainStd":             best.TrainStd,
			"validationMean":       best.ValidationMean,
			"validationStd":        best.ValidationStd,
			"validationMin":        best.ValidationMin,
			"validationMax":        best.ValidationMax,
			"allowMean":            best.AllowMean,
			"allowStd":             best.AllowStd,
			"allowMin":             best.AllowMin,
			"allowMax":             best.AllowMax,
			"durationMean":         best.DurationMean,
			"durationStd":          best.DurationStd,
			"inferenceMean":        best.InferenceMean,
			"inferenceStd":         best.InferenceStd,
			"inferenceMin":         best.InferenceMin,
			"inferenceMax":         best.InferenceMax,
			"inferenceLatencyMean": best.InferenceLatencyMean,
			"inferenceLatencyStd":  best.InferenceLatencyStd,
			"successRate":          best.SuccessRate,
			"runs":                 best.Runs,
		}
	}
	if len(summaries) == 0 {
		return nil
	}
	best := summaries[0]
	return map[string]any{
		"profile":              best.Profile,
		"modelType":            best.ModelType,
		"configSummary":        best.ConfigSummary,
		"trainMean":            best.TrainMean,
		"trainStd":             best.TrainStd,
		"validationMean":       best.ValidationMean,
		"validationStd":        best.ValidationStd,
		"validationMin":        best.ValidationMin,
		"validationMax":        best.ValidationMax,
		"allowMean":            best.AllowMean,
		"allowStd":             best.AllowStd,
		"allowMin":             best.AllowMin,
		"allowMax":             best.AllowMax,
		"durationMean":         best.DurationMean,
		"durationStd":          best.DurationStd,
		"inferenceMean":        best.InferenceMean,
		"inferenceStd":         best.InferenceStd,
		"inferenceMin":         best.InferenceMin,
		"inferenceMax":         best.InferenceMax,
		"inferenceLatencyMean": best.InferenceLatencyMean,
		"inferenceLatencyStd":  best.InferenceLatencyStd,
		"successRate":          best.SuccessRate,
		"runs":                 best.Runs,
	}
}
