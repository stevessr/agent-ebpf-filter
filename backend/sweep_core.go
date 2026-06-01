package main

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
)

func canRunIncrementalCountProfile(profile sweepProfile) bool {
	if profile.Kind != "bar" || profile.ParameterKind != "numeric" {
		return false
	}
	switch baseModelType(profile.ModelType) {
	case ModelRandomForest, ModelExtraTrees:
		return profile.ParameterName == "numTrees"
	case ModelAdaBoost:
		return profile.ParameterName == "estimators"
	default:
		return false
	}
}

type incrementalCountContext struct {
	Profile        sweepProfile
	MaxValue       int
	BuildDuration  float64
	MemoryBytes    int64
	NumSamples     int
	TrainSamples   int
	ValSamples     int
	TrainSet       []trainSample
	ValSet         []trainSample
	ValRaw         []TrainingSample
	Benchmark      []TrainingSample
	ModelForValue  func(int) Model
	ConfigForValue func(int) MLConfig
	SummaryForCfg  func(MLConfig) string
}

func runIncrementalCountProfile(profile sweepProfile, store *TrainingDataStore, benchmarkSamples []TrainingSample, workers int) ([]sweepResult, sweepResult, error) {
	ctx, err := buildIncrementalCountContext(profile, store, benchmarkSamples)
	if err != nil {
		return nil, sweepResult{}, err
	}
	results := make([]sweepResult, len(profile.XValues))
	if workers <= 1 || len(profile.XValues) <= 1 {
		for i, x := range profile.XValues {
			results[i] = runIncrementalCountValue(ctx, x)
		}
		return profileRunBest(profile, results)
	}
	if workers > len(profile.XValues) {
		workers = len(profile.XValues)
	}
	type job struct {
		Index int
		X     int
	}
	jobs := make(chan job)
	rows := make(chan struct {
		Index int
		Row   sweepResult
	})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				rows <- struct {
					Index int
					Row   sweepResult
				}{Index: j.Index, Row: runIncrementalCountValue(ctx, j.X)}
			}
		}()
	}
	go func() {
		for i, x := range profile.XValues {
			jobs <- job{Index: i, X: x}
		}
		close(jobs)
		wg.Wait()
		close(rows)
	}()
	for row := range rows {
		results[row.Index] = row.Row
	}
	return profileRunBest(profile, results)
}

func buildIncrementalCountContext(profile sweepProfile, store *TrainingDataStore, benchmarkSamples []TrainingSample) (incrementalCountContext, error) {
	maxValue := maxSweepInt(profile.XValues)
	if maxValue < 1 {
		return incrementalCountContext{}, fmt.Errorf("%s has no positive count values", profile.Name)
	}
	labeled := store.LabeledSamples()
	if len(labeled) == 0 {
		return incrementalCountContext{}, fmt.Errorf("%s has no labeled samples", profile.Name)
	}
	cfgMax := profile.Build(maxValue, 0)
	ctx := incrementalCountContext{
		Profile:        profile,
		MaxValue:       maxValue,
		NumSamples:     len(labeled),
		Benchmark:      benchmarkSamples,
		ConfigForValue: func(v int) MLConfig { return profile.Build(v, 0) },
		SummaryForCfg:  profile.Summary,
	}

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	start := time.Now()
	switch baseModelType(profile.ModelType) {
	case ModelRandomForest:
		if len(labeled) < cfgMax.MinSamplesLeaf*10 {
			return incrementalCountContext{}, fmt.Errorf("insufficient labeled samples: need >=%d, have %d", cfgMax.MinSamplesLeaf*10, len(labeled))
		}
		trainSet, valSet, _, valRaw, err := prepareAutoTuneSplit(labeled, cfgMax.ValidationSplitRatio)
		if err != nil {
			return incrementalCountContext{}, err
		}
		forest := buildAutoTuneForest(trainSet, maxValue, cfgMax.MaxDepth, cfgMax.MinSamplesLeaf, time.Now().UnixNano())
		ctx.TrainSet = trainSet
		ctx.ValSet = valSet
		ctx.ValRaw = valRaw
		ctx.TrainSamples = len(trainSet)
		ctx.ValSamples = len(valSet)
		ctx.ModelForValue = func(v int) Model {
			return prefixDecisionForest(forest, v)
		}
	case ModelExtraTrees:
		if len(labeled) < cfgMax.MinSamplesLeaf*10 {
			return incrementalCountContext{}, fmt.Errorf("insufficient labeled samples: need >=%d, have %d", cfgMax.MinSamplesLeaf*10, len(labeled))
		}
		trainSet, valSet, _, valRaw, err := prepareAutoTuneSplit(labeled, cfgMax.ValidationSplitRatio)
		if err != nil {
			return incrementalCountContext{}, err
		}
		allSamples := toTrainSamples(labeled)
		forest := buildExtraTrees(allSamples, maxValue, cfgMax.MaxDepth, cfgMax.MinSamplesLeaf, time.Now().UnixNano())
		ctx.TrainSet = trainSet
		ctx.ValSet = valSet
		ctx.ValRaw = valRaw
		ctx.TrainSamples = len(trainSet)
		ctx.ValSamples = len(valSet)
		ctx.ModelForValue = func(v int) Model {
			return &ExtraTreesModel{Forest: prefixDecisionForest(forest, v), NumTrees: clampCount(v, len(forest.Trees)), MaxDepth: cfgMax.MaxDepth}
		}
	case ModelAdaBoost:
		trainer := newSweepTrainer()
		model, result := trainer.TrainWithConfig(store, cfgMax)
		if result.Error != "" {
			return incrementalCountContext{}, fmt.Errorf("%s", result.Error)
		}
		ada, ok := unwrapModelType(model).(*AdaBoostModel)
		if !ok || ada == nil || len(ada.Stumps) == 0 {
			return incrementalCountContext{}, fmt.Errorf("expected AdaBoost model for %s", profile.Name)
		}
		allSamples := toTrainSamples(labeled)
		ctx.TrainSet = allSamples
		ctx.ValSet = allSamples
		ctx.ValRaw = labeled
		ctx.TrainSamples = len(allSamples)
		ctx.ValSamples = 0
		ctx.ModelForValue = func(v int) Model {
			return prefixAdaBoostModel(ada, v)
		}
	default:
		return incrementalCountContext{}, fmt.Errorf("%s is not an incremental count profile", profile.Name)
	}
	ctx.BuildDuration = time.Since(start).Seconds()
	runtime.ReadMemStats(&memAfter)
	if memAfter.Alloc > memBefore.Alloc {
		ctx.MemoryBytes = int64(memAfter.Alloc - memBefore.Alloc)
	}
	return ctx, nil
}

func runIncrementalCountValue(ctx incrementalCountContext, x int) sweepResult {
	cfg := ctx.ConfigForValue(x)
	model := ctx.ModelForValue(x)
	trainAccuracy := evalModelTrainSamples(model, ctx.TrainSet)
	validationAccuracy := trainAccuracy
	if len(ctx.ValSet) > 0 {
		validationAccuracy = evalModelTrainSamples(model, ctx.ValSet)
	}
	allowPassRate := 0.0
	if len(ctx.ValRaw) > 0 {
		allowPassRate = evaluateClassMetrics(model, ctx.ValRaw).AllowPassRate
	}
	inferenceDuration, inferenceThroughput, inferenceLatencyMs, inferenceSamples := benchmarkModelInference(model, ctx.Benchmark)
	ratio := float64(clampCount(x, ctx.MaxValue)) / float64(ctx.MaxValue)
	return sweepResult{
		Profile:             ctx.Profile.Name,
		Dataset:             ctx.Profile.DatasetName,
		BaseProfile:         baseProfileSegment(ctx.Profile.Name),
		ModelType:           cfg.ModelType,
		ParameterName:       ctx.Profile.ParameterName,
		ParameterKind:       ctx.Profile.ParameterKind,
		RequiredPoints:      ctx.Profile.RequiredDiscretePoints,
		ConfiguredPoints:    configuredProfilePointCount(ctx.Profile),
		XValue:              x,
		YValue:              0,
		ConfigSummary:       ctx.SummaryForCfg(cfg),
		TrainAccuracy:       trainAccuracy,
		ValidationAccuracy:  validationAccuracy,
		AllowPassRate:       allowPassRate,
		Duration:            ctx.BuildDuration * ratio,
		InferenceDuration:   inferenceDuration,
		InferenceSamples:    inferenceSamples,
		InferenceLatencyMs:  inferenceLatencyMs,
		InferenceThroughput: inferenceThroughput,
		MemoryBytes:         int64(float64(ctx.MemoryBytes) * ratio),
		NumSamples:          ctx.NumSamples,
		TrainSamples:        ctx.TrainSamples,
		ValidationSamples:   ctx.ValSamples,
	}
}

type profileGridPoint struct {
	Index int
	X     int
	Y     int
}

func profileGridPoints(profile sweepProfile) []profileGridPoint {
	points := make([]profileGridPoint, 0, configuredProfilePointCount(profile))
	for _, x := range profile.XValues {
		yValues := profile.YValues
		if profile.Kind == "bar" {
			yValues = []int{0}
		}
		for _, y := range yValues {
			points = append(points, profileGridPoint{Index: len(points), X: x, Y: y})
		}
	}
	return points
}

func profileRunBest(profile sweepProfile, results []sweepResult) ([]sweepResult, sweepResult, error) {
	best := bestSweepResult(results)
	if math.IsInf(best.ValidationAccuracy, -1) {
		return results, sweepResult{}, fmt.Errorf("profile %s produced no successful runs", profile.Name)
	}
	return results, best, nil
}

func bestSweepResult(results []sweepResult) sweepResult {
	var best sweepResult
	best.ValidationAccuracy = math.Inf(-1)
	for _, row := range results {
		if row.Error == "" && (row.ValidationAccuracy > best.ValidationAccuracy ||
			(row.ValidationAccuracy == best.ValidationAccuracy && row.AllowPassRate > best.AllowPassRate) ||
			(row.ValidationAccuracy == best.ValidationAccuracy && row.InferenceThroughput > best.InferenceThroughput) ||
			(row.ValidationAccuracy == best.ValidationAccuracy && row.AllowPassRate == best.AllowPassRate && row.InferenceThroughput == best.InferenceThroughput && row.Duration < best.Duration)) {
			best = row
		}
	}
	return best
}

func expectedProfileResultCount(profile sweepProfile) int {
	return configuredProfilePointCount(profile)
}

func configuredProfilePointCount(profile sweepProfile) int {
	if profile.Kind == "heatmap" {
		return uniqueIntCount(profile.XValues) * uniqueIntCount(profile.YValues)
	}
	return uniqueIntCount(profile.XValues)
}

func annotateSweepResults(profile sweepProfile, results []sweepResult) []sweepResult {
	configured := configuredProfilePointCount(profile)
	out := make([]sweepResult, len(results))
	for i, row := range results {
		row.Profile = profile.Name
		row.Dataset = profile.DatasetName
		row.BaseProfile = baseProfileSegment(profile.Name)
		row.ParameterName = profile.ParameterName
		row.ParameterKind = profile.ParameterKind
		row.RequiredPoints = profile.RequiredDiscretePoints
		row.ConfiguredPoints = configured
		out[i] = row
	}
	return out
}

func runSingleConfig(profile sweepProfile, store *TrainingDataStore, x, y int, benchmarkSamples []TrainingSample) (sweepResult, error) {
	cfg := profile.Build(x, y)
	trainer := newSweepTrainer()

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	start := time.Now()
	model, result := trainer.TrainWithConfig(store, cfg)
	duration := time.Since(start).Seconds()
	runtime.ReadMemStats(&memAfter)
	memUsed := int64(memAfter.Alloc - memBefore.Alloc)
	if memUsed < 0 {
		memUsed = 0
	}

	row := sweepResult{
		Profile:          profile.Name,
		Dataset:          profile.DatasetName,
		BaseProfile:      baseProfileSegment(profile.Name),
		ModelType:        cfg.ModelType,
		ParameterName:    profile.ParameterName,
		ParameterKind:    profile.ParameterKind,
		RequiredPoints:   profile.RequiredDiscretePoints,
		ConfiguredPoints: configuredProfilePointCount(profile),
		XValue:           x,
		YValue:           y,
		ConfigSummary:    profile.Summary(cfg),

		TrainAccuracy:      result.TrainAccuracy,
		ValidationAccuracy: result.ValidationAccuracy,
		NumSamples:         result.NumSamples,
		TrainSamples:       result.TrainSamples,
		ValidationSamples:  result.ValidationSamples,
		Duration:           duration,
		Error:              result.Error,
	}
	if result.Error == "" && model != nil {
		row.ModelType = model.Type()
		if row.ConfigSummary == "" {
			row.ConfigSummary = string(model.Type())
		}
		validationSamples := trainer.LastValidationSamples()
		if len(validationSamples) > 0 {
			row.AllowPassRate = evaluateClassMetrics(model, validationSamples).AllowPassRate
		}
		infStartMem := allocMem()
		row.InferenceDuration, row.InferenceThroughput, row.InferenceLatencyMs, row.InferenceSamples = benchmarkModelInference(model, benchmarkSamples)
		row.MemoryBytes = int64(allocMem()-infStartMem) + memUsed
	} else {
		row.MemoryBytes = memUsed
	}
	if row.Error != "" {
		row.ValidationAccuracy = 0
	}
	return row, nil
}

func newSweepTrainer() *ModelTrainer {
	return &ModelTrainer{
		mu:         make(chan struct{}, 1),
		cancelCh:   make(chan struct{}),
		logMaxSize: 64,
	}
}

func selectBenchmarkSamples(samples []TrainingSample, target int) []TrainingSample {
	if target <= 0 || len(samples) == 0 {
		return nil
	}
	if target >= len(samples) {
		return append([]TrainingSample(nil), samples...)
	}
	if target == 1 {
		return []TrainingSample{samples[len(samples)/2]}
	}
	out := make([]TrainingSample, 0, target)
	for i := 0; i < target; i++ {
		idx := int(math.Round(float64(i) * float64(len(samples)-1) / float64(target-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(samples) {
			idx = len(samples) - 1
		}
		out = append(out, samples[idx])
	}
	return out
}

func benchmarkModelInference(model Model, samples []TrainingSample) (float64, float64, float64, int) {
	if model == nil || len(samples) == 0 {
		return 0, 0, 0, 0
	}
	warmup := 8
	if warmup > len(samples) {
		warmup = len(samples)
	}
	for i := 0; i < warmup; i++ {
		_ = model.Predict(samples[i].Features)
	}

	const targetPredictions = 256
	rounds := targetPredictions / len(samples)
	if targetPredictions%len(samples) != 0 {
		rounds++
	}
	if rounds < 1 {
		rounds = 1
	}

	totalPredictions := 0
	start := time.Now()
	for r := 0; r < rounds; r++ {
		for _, sample := range samples {
			_ = model.Predict(sample.Features)
			totalPredictions++
		}
	}
	duration := time.Since(start).Seconds()
	if duration <= 0 {
		duration = 1e-9
	}
	throughput := float64(totalPredictions) / duration
	latencyMs := duration * 1000 / float64(totalPredictions)
	return duration, throughput, latencyMs, totalPredictions
}

type classMetrics struct {
	Accuracy      float64
	AllowPassRate float64
	AllowTotal    int
	AllowCorrect  int
}

func evaluateClassMetrics(model Model, samples []TrainingSample) classMetrics {
	if model == nil || len(samples) == 0 {
		return classMetrics{}
	}
	correct := 0
	allowTotal := 0
	allowCorrect := 0
	for _, sample := range samples {
		pred := model.Predict(sample.Features)
		if pred.Action == sample.Label {
			correct++
		}
		if sample.Label == 0 {
			allowTotal++
			if pred.Action == 0 {
				allowCorrect++
			}
		}
	}
	metrics := classMetrics{
		Accuracy:     float64(correct) / float64(len(samples)),
		AllowTotal:   allowTotal,
		AllowCorrect: allowCorrect,
	}
	if allowTotal > 0 {
		metrics.AllowPassRate = float64(allowCorrect) / float64(allowTotal)
	}
	return metrics
}
