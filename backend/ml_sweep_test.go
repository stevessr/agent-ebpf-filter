package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type sweepProfile struct {
	Name       string
	ModelType  ModelType
	Comparable bool
	Kind       string // "heatmap" or "bar"
	XName      string
	YName      string
	XValues    []int
	YValues    []int
	XLabel     func(int) string
	YLabel     func(int) string
	Build      func(x, y int) MLConfig
	Summary    func(cfg MLConfig) string

	// Parameter metadata is used by the comprehensive axis-sweep verifier.
	// Numeric parameters must expose RequiredDiscretePoints unique values; small
	// categorical parameters use ParameterKind="categorical" and a lower/zero
	// requirement because there are only a few meaningful choices.
	ParameterName          string
	ParameterKind          string
	RequiredDiscretePoints int
	DatasetName            string
	DatasetDescription     string
}

type sweepResult struct {
	Profile             string
	Dataset             string
	BaseProfile         string
	ModelType           ModelType
	ParameterName       string
	ParameterKind       string
	RequiredPoints      int
	ConfiguredPoints    int
	XValue              int
	YValue              int
	ConfigSummary       string
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type profileSummary struct {
	Profile sweepProfile
	Best    sweepResult
	Results []sweepResult
	Chart   string
}

type repeatRunResult struct {
	Profile             string
	ModelType           ModelType
	XValue              int
	YValue              int
	ConfigSummary       string
	RunIndex            int
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type repeatSummary struct {
	Profile              string
	ModelType            ModelType
	Comparable           bool
	XValue               int
	YValue               int
	ConfigSummary        string
	Runs                 int
	SuccessRuns          int
	FailureRuns          int
	TrainMean            float64
	TrainStd             float64
	ValidationMean       float64
	ValidationStd        float64
	ValidationMin        float64
	ValidationMax        float64
	AllowMean            float64
	AllowStd             float64
	AllowMin             float64
	AllowMax             float64
	DurationMean         float64
	DurationStd          float64
	InferenceMean        float64
	InferenceStd         float64
	InferenceMin         float64
	InferenceMax         float64
	InferenceLatencyMean float64
	InferenceLatencyStd  float64
	TrainMin             float64
	TrainMax             float64
	MemoryMean           float64
	MemoryStd            float64
	MemoryMin            float64
	MemoryMax            float64
	SuccessRate          float64
}

type stabilityTask struct {
	Profile          sweepProfile
	Config           sweepResult
	Store            *TrainingDataStore
	BenchmarkSamples []TrainingSample
}

type sweepDataset struct {
	Name        string
	Description string
	Samples     []TrainingSample
}

func TestComprehensiveSweepProfilesCoverThousandPointsPerNumericParameter(t *testing.T) {
	profiles := profilesForMode("comprehensive")
	seen := make(map[ModelType]map[string]int)
	for _, profile := range profiles {
		if profile.ParameterName == "" {
			t.Fatalf("profile %s missing parameter metadata", profile.Name)
		}
		if profile.ParameterKind != "numeric" {
			if unique := uniqueIntCount(profile.XValues); profile.RequiredDiscretePoints != unique {
				t.Fatalf("%s categorical/fixed requirement=%d, want unique count %d", profile.Name, profile.RequiredDiscretePoints, unique)
			}
			continue
		}
		unique := uniqueIntCount(profile.XValues)
		if unique < 1000 {
			t.Fatalf("%s has %d unique points, want >=1000", profile.Name, unique)
		}
		if seen[profile.ModelType] == nil {
			seen[profile.ModelType] = make(map[string]int)
		}
		seen[profile.ModelType][profile.ParameterName] = unique
	}
	for _, modelType := range AllModelTypes() {
		for _, param := range numericSweepParametersForModel(modelType) {
			if seen[modelType][param] < 1000 {
				t.Fatalf("%s/%s coverage = %d, want >=1000 discrete points", modelType, param, seen[modelType][param])
			}
		}
	}
}

func TestComprehensiveSweepDefaultsToMultipleDatasets(t *testing.T) {
	samples := make([]TrainingSample, 0, 30)
	for i := 0; i < 12; i++ {
		samples = append(samples, sweepTestSample(0, "allow"))
	}
	for i := 0; i < 10; i++ {
		samples = append(samples, sweepTestSample(1, "block"))
	}
	for i := 0; i < 8; i++ {
		samples = append(samples, sweepTestSample(3, "alert"))
	}

	datasets := datasetProfilesForMode(samples, "comprehensive", nil)
	if len(datasets) < 2 {
		t.Fatalf("comprehensive datasets = %d, want at least 2", len(datasets))
	}
	if datasets[0].Name != "all" || len(datasets[0].Samples) != len(samples) {
		t.Fatalf("first dataset = %s/%d, want all/%d", datasets[0].Name, len(datasets[0].Samples), len(samples))
	}
	foundBalanced := false
	for _, dataset := range datasets {
		if dataset.Name == "label-balanced" {
			foundBalanced = true
			if len(dataset.Samples) != 24 {
				t.Fatalf("label-balanced samples = %d, want 24", len(dataset.Samples))
			}
		}
	}
	if !foundBalanced {
		t.Fatalf("expected label-balanced dataset, got %#v", datasets)
	}
}

func sweepTestSample(label int32, userLabel string) TrainingSample {
	return TrainingSample{
		Label:     label,
		UserLabel: userLabel,
		Timestamp: time.Now(),
		Comm:      "cmd",
		Args:      []string{fmt.Sprintf("%d", label)},
	}
}

func TestMLSweep(t *testing.T) {
	if os.Getenv("ML_SWEEP") != "1" {
		t.Skip("set ML_SWEEP=1 to run the offline ML sweep report generator")
	}
	if err := runMLSweepReport(); err != nil {
		t.Fatalf("ml sweep failed: %v", err)
	}
}

func runMLSweepReport() error {
	if parseBoolEnv(os.Getenv("ML_SWEEP_QUIET_LOGS")) {
		origLogOutput := log.Writer()
		log.SetOutput(io.Discard)
		defer log.SetOutput(origLogOutput)
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("ML_SWEEP_MODE")))
	if mode == "" {
		mode = "quick"
	}
	if mode != "quick" && mode != "full" && mode != "comprehensive" {
		return fmt.Errorf("unsupported ML_SWEEP_MODE %q", mode)
	}
	repeats := parsePositiveInt(os.Getenv("ML_SWEEP_REPEATS"), 100)
	stabilityTop := parsePositiveInt(os.Getenv("ML_SWEEP_STABILITY_TOP"), 1)
	if repeats < 1 {
		repeats = 1
	}
	if stabilityTop < 1 {
		stabilityTop = 1
	}
	pointsPerParam := parsePositiveInt(os.Getenv("ML_SWEEP_POINTS_PER_PARAM"), 1000)
	workers := parsePositiveInt(os.Getenv("ML_SWEEP_WORKERS"), 1)

	selectedModels := parseModelFilter(os.Getenv("ML_SWEEP_MODELS"))
	selectedDatasets := parseNameFilter(os.Getenv("ML_SWEEP_DATASETS"))
	resumeSweep := parseBoolEnv(os.Getenv("ML_SWEEP_RESUME"))
	outDir := strings.TrimSpace(os.Getenv("ML_SWEEP_OUTDIR"))
	if outDir == "" {
		outDir = filepath.Join("..", "reports", "ml-sweep-"+time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	resultsPath := filepath.Join(outDir, "results.csv")
	if !resumeSweep {
		_ = os.Remove(resultsPath)
	}

	// Load the persisted dataset directly so the sweep can run even if the live
	// backend is busy or temporarily unavailable.
	InitTrainingStore(100000)
	if globalTrainingStore == nil {
		return fmt.Errorf("training store not initialized")
	}
	labeled := globalTrainingStore.LabeledSamples()
	if len(labeled) == 0 {
		return fmt.Errorf("no labeled samples found in the persisted training store")
	}
	datasets := datasetProfilesForMode(labeled, mode, selectedDatasets)
	if len(datasets) == 0 {
		return fmt.Errorf("no sweep datasets selected")
	}

	origMLConfig := mlConfig
	defer func() {
		mlConfig = origMLConfig
	}()

	mlConfig = DefaultMLConfig()
	mlConfig.ValidationSplitRatio = 0.2
	mlConfig.LlmEnabled = false
	mlConfig.LlmBaseURL = ""
	mlConfig.LlmModel = ""
	mlConfig.LlmAPIKey = ""

	profiles := profilesForModeWithPoints(mode, pointsPerParam)
	if len(selectedModels) > 0 {
		filtered := make([]sweepProfile, 0, len(profiles))
		for _, p := range profiles {
			if modelFilterMatches(selectedModels, p.ModelType) {
				filtered = append(filtered, p)
			}
		}
		profiles = filtered
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no sweep profiles selected")
	}

	fmt.Printf("[ml-sweep] dataset=%d labeled samples, mode=%s, datasets=%d, pointsPerParam=%d, workers=%d, out=%s\n", len(labeled), mode, len(datasets), pointsPerParam, workers, outDir)

	summaries := make([]profileSummary, 0, len(profiles))
	allResults := make([]sweepResult, 0, 4096)
	stabilityCandidates := make([]stabilityTask, 0, len(profiles)*stabilityTop)

	for _, dataset := range datasets {
		store := trainingStoreFromSamples(dataset.Samples)
		benchmarkSamples := selectBenchmarkSamples(dataset.Samples, 64)
		fmt.Printf("[ml-sweep] dataset=%-18s samples=%d (%s)\n", dataset.Name, len(dataset.Samples), dataset.Description)
		for _, baseProfile := range profiles {
			profile := profileForDataset(baseProfile, dataset)
			profileResultsPath := filepath.Join(outDir, slug(profile.Name)+"-grid.csv")
			var results []sweepResult
			var best sweepResult
			if resumeSweep {
				if cached, err := readSweepResultsCSV(profileResultsPath); err == nil && len(cached) >= expectedProfileResultCount(profile) {
					results = annotateSweepResults(profile, cached)
					best = bestSweepResult(results)
					fmt.Printf("[ml-sweep] %-32s resume=%d rows\n", profile.Name, len(results))
					if err := writeCSV(profileResultsPath, results); err != nil {
						return err
					}
				}
			}
			if len(results) == 0 {
				var err error
				results, best, err = runProfile(profile, store, benchmarkSamples, workers)
				if err != nil {
					return fmt.Errorf("%s: %w", profile.Name, err)
				}
				results = annotateSweepResults(profile, results)
				if err := writeCSV(profileResultsPath, results); err != nil {
					return err
				}
				if err := appendSweepResultsCSV(resultsPath, results); err != nil {
					return err
				}
			}
			allResults = append(allResults, results...)
			chart, err := renderProfileChart(profile, results)
			if err != nil {
				return fmt.Errorf("%s chart: %w", profile.Name, err)
			}
			if err := os.WriteFile(filepath.Join(outDir, slug(profile.Name)+".svg"), []byte(chart), 0o644); err != nil {
				return err
			}
			inferenceChart, err := renderProfileInferenceChart(profile, results)
			if err != nil {
				return fmt.Errorf("%s inference chart: %w", profile.Name, err)
			}
			if err := os.WriteFile(filepath.Join(outDir, slug(profile.Name)+"-inference.svg"), []byte(inferenceChart), 0o644); err != nil {
				return err
			}
			summaries = append(summaries, profileSummary{
				Profile: profile,
				Best:    best,
				Results: results,
				Chart:   chart,
			})
			stabilityCandidates = append(stabilityCandidates, selectTopRepeatConfigs(profile, results, stabilityTop, store, benchmarkSamples)...)
			fmt.Printf("[ml-sweep] %-32s best=%s val=%.2f%% train=%.2f%%\n",
				profile.Name, best.ConfigSummary, best.ValidationAccuracy*100, best.TrainAccuracy*100)
		}
	}

	if err := writeCSV(resultsPath, allResults); err != nil {
		return err
	}

	stabilityRuns, stabilitySummaries, err := runStabilityPhase(stabilityCandidates, repeats)
	if err != nil {
		return err
	}
	if err := writeRepeatCSV(filepath.Join(outDir, "stability-runs.csv"), stabilityRuns); err != nil {
		return err
	}
	if err := writeRepeatSummaryCSV(filepath.Join(outDir, "stability-summary.csv"), stabilitySummaries); err != nil {
		return err
	}
	coverage := buildSweepCoverage(datasets, profiles, allResults, pointsPerParam)
	if err := writeCoverageJSON(filepath.Join(outDir, "coverage.json"), coverage); err != nil {
		return err
	}

	overall := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		overall = append(overall, barItem{
			Label: shortProfileLabel(s.Profile.Name),
			Value: s.Best.ValidationAccuracy,
			Title: fmt.Sprintf("%s | %s | val=%.2f%%", s.Profile.Name, s.Best.ConfigSummary, s.Best.ValidationAccuracy*100),
		})
	}

	bestChart, err := renderBarChart("Best validation accuracy by model", "higher is better", overall, 0.0, 1.0)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "overall_best.svg"), []byte(bestChart), 0o644); err != nil {
		return err
	}
	speedChart, err := renderOverallSpeedChart(summaries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "overall_speed.svg"), []byte(speedChart), 0o644); err != nil {
		return err
	}

	stabilityChart, err := renderStabilityChart(stabilitySummaries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "stability_best.svg"), []byte(stabilityChart), 0o644); err != nil {
		return err
	}
	stabilitySpeedChart, err := renderStabilitySpeedChart(stabilitySummaries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "stability_speed.svg"), []byte(stabilitySpeedChart), 0o644); err != nil {
		return err
	}

	screenBest := bestScreenSummary(summaries)
	if screenBest != nil {
		if err := writeCSV(filepath.Join(outDir, slug(screenBest.Profile.Name)+"-grid.csv"), screenBest.Results); err != nil {
			return err
		}
		bestDurationChart, err := renderProfileDurationChart(screenBest.Profile, screenBest.Results)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, slug(screenBest.Profile.Name)+"-duration.svg"), []byte(bestDurationChart), 0o644); err != nil {
			return err
		}
		bestInferenceChart, err := renderProfileInferenceChart(screenBest.Profile, screenBest.Results)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, slug(screenBest.Profile.Name)+"-inference.svg"), []byte(bestInferenceChart), 0o644); err != nil {
			return err
		}
	}

	if err := writeReportHTML(filepath.Join(outDir, "index.html"), summaries, stabilitySummaries, repeats, stabilityTop); err != nil {
		return err
	}

	bestJSON := map[string]any{
		"datasetSize":    len(labeled),
		"datasets":       coverage.Datasets,
		"mode":           mode,
		"pointsPerParam": pointsPerParam,
		"workers":        workers,
		"repeats":        repeats,
		"stabilityTop":   stabilityTop,
		"outDir":         outDir,
		"coverage":       coverage.Summary,
		"screenBest": map[string]any{
			"profile":                  screenBest.Profile.Name,
			"modelType":                screenBest.Profile.ModelType,
			"configSummary":            screenBest.Best.ConfigSummary,
			"trainAccuracy":            screenBest.Best.TrainAccuracy,
			"validationAccuracy":       screenBest.Best.ValidationAccuracy,
			"allowPassRate":            screenBest.Best.AllowPassRate,
			"durationSeconds":          screenBest.Best.Duration,
			"inferenceDurationSeconds": screenBest.Best.InferenceDuration,
			"inferenceSamples":         screenBest.Best.InferenceSamples,
			"inferenceLatencyMs":       screenBest.Best.InferenceLatencyMs,
			"inferenceThroughput":      screenBest.Best.InferenceThroughput,
		},
		"stableBest": stabilityBestJSON(stabilitySummaries),
	}
	if bestComparable := bestComparableSummary(stabilitySummaries); bestComparable != nil {
		bestJSON["best"] = map[string]any{
			"profile":              bestComparable.Profile,
			"modelType":            bestComparable.ModelType,
			"configSummary":        bestComparable.ConfigSummary,
			"trainMean":            bestComparable.TrainMean,
			"validationMean":       bestComparable.ValidationMean,
			"validationStd":        bestComparable.ValidationStd,
			"allowMean":            bestComparable.AllowMean,
			"allowStd":             bestComparable.AllowStd,
			"allowMin":             bestComparable.AllowMin,
			"allowMax":             bestComparable.AllowMax,
			"durationMean":         bestComparable.DurationMean,
			"inferenceMean":        bestComparable.InferenceMean,
			"inferenceStd":         bestComparable.InferenceStd,
			"inferenceLatencyMean": bestComparable.InferenceLatencyMean,
			"inferenceLatencyStd":  bestComparable.InferenceLatencyStd,
			"successRate":          bestComparable.SuccessRate,
		}
	} else if len(stabilitySummaries) > 0 {
		bestJSON["best"] = map[string]any{
			"profile":              stabilitySummaries[0].Profile,
			"modelType":            stabilitySummaries[0].ModelType,
			"configSummary":        stabilitySummaries[0].ConfigSummary,
			"trainMean":            stabilitySummaries[0].TrainMean,
			"validationMean":       stabilitySummaries[0].ValidationMean,
			"validationStd":        stabilitySummaries[0].ValidationStd,
			"allowMean":            stabilitySummaries[0].AllowMean,
			"allowStd":             stabilitySummaries[0].AllowStd,
			"allowMin":             stabilitySummaries[0].AllowMin,
			"allowMax":             stabilitySummaries[0].AllowMax,
			"durationMean":         stabilitySummaries[0].DurationMean,
			"inferenceMean":        stabilitySummaries[0].InferenceMean,
			"inferenceStd":         stabilitySummaries[0].InferenceStd,
			"inferenceLatencyMean": stabilitySummaries[0].InferenceLatencyMean,
			"inferenceLatencyStd":  stabilitySummaries[0].InferenceLatencyStd,
			"successRate":          stabilitySummaries[0].SuccessRate,
		}
	}
	data, _ := json.MarshalIndent(bestJSON, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "best.json"), data, 0o644); err != nil {
		return err
	}

	fmt.Printf("[ml-sweep] report written to %s\n", filepath.Join(outDir, "index.html"))
	if bestComparable := bestComparableSummary(stabilitySummaries); bestComparable != nil {
		fmt.Printf("[ml-sweep] comparable best: %s | %s | val=%.2f%% ± %.2f%% | allow=%.2f%% ± %.2f%% (%dx)\n",
			bestComparable.Profile, bestComparable.ConfigSummary, bestComparable.ValidationMean*100, bestComparable.ValidationStd*100, bestComparable.AllowMean*100, bestComparable.AllowStd*100, bestComparable.Runs)
	}
	return nil
}

func runProfile(profile sweepProfile, store *TrainingDataStore, benchmarkSamples []TrainingSample, workers int) ([]sweepResult, sweepResult, error) {
	if len(profile.XValues) == 0 {
		return nil, sweepResult{}, fmt.Errorf("profile %s has no x-values", profile.Name)
	}
	if profile.Kind == "heatmap" && len(profile.YValues) == 0 {
		return nil, sweepResult{}, fmt.Errorf("profile %s has no y-values", profile.Name)
	}
	if canRunIncrementalCountProfile(profile) {
		return runIncrementalCountProfile(profile, store, benchmarkSamples, workers)
	}

	points := profileGridPoints(profile)
	results := make([]sweepResult, len(points))
	if workers <= 1 || len(points) <= 1 {
		for _, point := range points {
			row, err := runSingleConfig(profile, store, point.X, point.Y, benchmarkSamples)
			if err != nil {
				return nil, sweepResult{}, err
			}
			results[point.Index] = row
		}
		return profileRunBest(profile, results)
	}

	if workers > len(points) {
		workers = len(points)
	}

	type profileJob struct {
		Index int
		X     int
		Y     int
	}
	type profileJobResult struct {
		Index int
		Row   sweepResult
		Err   error
	}
	jobs := make(chan profileJob)
	resultCh := make(chan profileJobResult)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				row, err := runSingleConfig(profile, store, job.X, job.Y, benchmarkSamples)
				resultCh <- profileJobResult{Index: job.Index, Row: row, Err: err}
			}
		}()
	}
	go func() {
		for _, point := range points {
			jobs <- profileJob{Index: point.Index, X: point.X, Y: point.Y}
		}
		close(jobs)
		wg.Wait()
		close(resultCh)
	}()

	var firstErr error
	for result := range resultCh {
		if result.Err != nil && firstErr == nil {
			firstErr = result.Err
		}
		results[result.Index] = result.Row
	}
	if firstErr != nil {
		return nil, sweepResult{}, firstErr
	}
	return profileRunBest(profile, results)
}

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

func allocMem() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func parseBoolEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseNameFilter(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func modelFilterMatches(selected map[ModelType]bool, modelType ModelType) bool {
	if len(selected) == 0 {
		return true
	}
	return selected[modelType] || selected[baseModelType(modelType)]
}

func datasetProfilesForMode(labeled []TrainingSample, mode string, selected map[string]bool) []sweepDataset {
	candidates := buildSweepDatasetCandidates(labeled)
	if len(candidates) == 0 {
		return nil
	}
	if len(selected) > 0 {
		out := make([]sweepDataset, 0, len(selected))
		for _, ds := range candidates {
			if selected[strings.ToLower(ds.Name)] {
				out = append(out, ds)
			}
		}
		return out
	}
	if mode != "comprehensive" {
		return candidates[:1]
	}
	out := []sweepDataset{candidates[0]}
	for _, ds := range candidates[1:] {
		out = append(out, ds)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func buildSweepDatasetCandidates(labeled []TrainingSample) []sweepDataset {
	if len(labeled) == 0 {
		return nil
	}
	out := []sweepDataset{{
		Name:        "all",
		Description: "all persisted labeled samples",
		Samples:     append([]TrainingSample(nil), labeled...),
	}}
	if balanced := balancedLabelDataset(labeled); len(balanced) >= 10 && len(balanced) < len(labeled) {
		out = append(out, sweepDataset{
			Name:        "label-balanced",
			Description: "deterministic class-balanced subset capped by the smallest present label",
			Samples:     balanced,
		})
	}
	if allowBlock := filterSamplesByLabel(labeled, map[int32]bool{0: true, 1: true}); len(allowBlock) >= 10 && len(allowBlock) < len(labeled) {
		out = append(out, sweepDataset{
			Name:        "allow-block",
			Description: "binary ALLOW/BLOCK subset for false-block and miss-block sensitivity",
			Samples:     allowBlock,
		})
	}
	if even := deterministicIndexSubset(labeled, 0); len(even) >= 10 && len(out) < 3 {
		out = append(out, sweepDataset{
			Name:        "even-index",
			Description: "deterministic even-index subset used when label-derived subsets are unavailable",
			Samples:     even,
		})
	}
	if odd := deterministicIndexSubset(labeled, 1); len(odd) >= 10 && len(out) < 3 {
		out = append(out, sweepDataset{
			Name:        "odd-index",
			Description: "deterministic odd-index subset used when label-derived subsets are unavailable",
			Samples:     odd,
		})
	}
	return out
}

func balancedLabelDataset(samples []TrainingSample) []TrainingSample {
	byLabel := make(map[int32][]TrainingSample)
	for _, sample := range samples {
		byLabel[sample.Label] = append(byLabel[sample.Label], sample)
	}
	if len(byLabel) < 2 {
		return nil
	}
	minCount := int(^uint(0) >> 1)
	labels := make([]int, 0, len(byLabel))
	for label, group := range byLabel {
		if len(group) == 0 {
			continue
		}
		if len(group) < minCount {
			minCount = len(group)
		}
		labels = append(labels, int(label))
	}
	if minCount <= 0 || len(labels) < 2 {
		return nil
	}
	sort.Ints(labels)
	out := make([]TrainingSample, 0, minCount*len(labels))
	for _, label := range labels {
		group := byLabel[int32(label)]
		if len(group) > minCount {
			group = group[:minCount]
		}
		out = append(out, group...)
	}
	return out
}

func filterSamplesByLabel(samples []TrainingSample, labels map[int32]bool) []TrainingSample {
	out := make([]TrainingSample, 0, len(samples))
	for _, sample := range samples {
		if labels[sample.Label] {
			out = append(out, sample)
		}
	}
	return out
}

func deterministicIndexSubset(samples []TrainingSample, parity int) []TrainingSample {
	out := make([]TrainingSample, 0, (len(samples)+1)/2)
	for i, sample := range samples {
		if i%2 == parity {
			out = append(out, sample)
		}
	}
	return out
}

func trainingStoreFromSamples(samples []TrainingSample) *TrainingDataStore {
	maxSamples := len(samples)
	if maxSamples < 1 {
		maxSamples = 1
	}
	store := &TrainingDataStore{
		samples:    make([]TrainingSample, maxSamples),
		maxSamples: maxSamples,
	}
	for _, sample := range samples {
		store.Add(sample)
	}
	store.dirtyCount = 0
	return store
}

func profileForDataset(profile sweepProfile, dataset sweepDataset) sweepProfile {
	scoped := profile
	scoped.Name = dataset.Name + "/" + profile.Name
	scoped.DatasetName = dataset.Name
	scoped.DatasetDescription = dataset.Description
	return scoped
}

func profilesForMode(mode string) []sweepProfile {
	return profilesForModeWithPoints(mode, 1000)
}

func profilesForModeWithPoints(mode string, pointsPerParam int) []sweepProfile {
	if pointsPerParam < 1 {
		pointsPerParam = 1000
	}
	if mode == "comprehensive" {
		return comprehensiveAxisSweepProfiles(pointsPerParam)
	}
	quick := map[string]bool{
		string(ModelRandomForest):       true,
		string(ModelExtraTrees):         true,
		string(ModelKNN):                true,
		string(ModelNaiveBayes):         true,
		string(ModelAdaBoost):           true,
		string(ModelLogisticRegression): true,
		string(ModelSVM):                true,
		string(ModelRidge):              true,
		string(ModelPerceptron):         true,
		string(ModelPassiveAggressive):  true,
	}
	_ = quick

	// ── Parameter grids ──────────────────────────────────────────
	// quick: ~9-50 points per model
	// full: ~50-200 points per model
	// comprehensive: 1000+ points per model

	// Helper: linspaceInt generates count evenly-spaced integers from min to max
	linspaceInt := func(minVal, maxVal, count int) []int {
		if count <= 0 {
			return nil
		}
		if count == 1 {
			return []int{(minVal + maxVal) / 2}
		}
		out := make([]int, count)
		for i := 0; i < count; i++ {
			out[i] = minVal + (maxVal-minVal)*i/(count-1)
		}
		return out
	}

	rfX, rfY := []int{15, 31, 51}, []int{6, 8, 10}
	etX, etY := []int{15, 31, 51}, []int{6, 8, 10}
	logX, logY := []int{5, 10, 20, 50}, []int{4, 8, 12}
	linearX, linearY := []int{5, 10, 20, 50}, []int{500, 1000, 2000}
	knnX := []int{1, 3, 5, 7, 9}
	ridgeX := []int{5, 10, 25, 50, 100}
	adaX := []int{10, 25, 50, 100}
	nbX := []int{0}

	switch mode {
	case "full":
		rfX, rfY = linspaceInt(3, 100, 15), linspaceInt(2, 16, 10)
		etX, etY = linspaceInt(3, 100, 15), linspaceInt(2, 16, 10)
		logX, logY = linspaceInt(1, 200, 20), []int{4, 8, 12}
		linearX, linearY = linspaceInt(1, 200, 20), linspaceInt(100, 5000, 10)
		knnX = linspaceInt(1, 31, 15)
		ridgeX = linspaceInt(1, 500, 20)
		adaX = linspaceInt(5, 300, 20)
	}

	return []sweepProfile{
		{
			Name:       "random_forest",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    rfX,
			YValues:    rfY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "extra_trees",
			ModelType:  ModelExtraTrees,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    etX,
			YValues:    etY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelExtraTrees
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    logX,
			YValues:    logY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				reg := "l2"
				switch cfg.MaxDepth {
				case 4:
					reg = "none"
				case 12:
					reg = "l1"
				}
				return fmt.Sprintf("lr=%.3f reg=%s iter=%d", float64(cfg.NumTrees)/1000.0, reg, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "knn",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    knnX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 8
				cfg.MinSamplesLeaf = 5
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "ridge",
			ModelType:  ModelRidge,
			Comparable: false,
			Kind:       "bar",
			XName:      "alpha×100",
			XValues:    ridgeX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRidge
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.2f", float64(v)/100.0) },
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
			},
		},
		{
			Name:       "adaboost",
			ModelType:  ModelAdaBoost,
			Comparable: false,
			Kind:       "bar",
			XName:      "estimators",
			XValues:    adaX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelAdaBoost
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("estimators=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "naive_bayes",
			ModelType:  ModelNaiveBayes,
			Comparable: false,
			Kind:       "bar",
			XName:      "preset",
			XValues:    nbX,
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNaiveBayes
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "default" },
			Summary: func(cfg MLConfig) string {
				return "default"
			},
		},
		{
			Name:       "naive_bayes_balanced",
			ModelType:  ModelNaiveBayes,
			Comparable: false,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNaiveBayes
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "balanced" },
			Summary: func(cfg MLConfig) string {
				return "balanced-prior"
			},
		},
		{
			Name:       "logistic_balanced",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{3, 5, 10, 20, 50, 100},
			YValues:    []int{1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 12
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=l1 balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm_balanced",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron_balanced",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive_balanced",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "nearest_centroid",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "euclidean" },
			Summary: func(cfg MLConfig) string {
				return "metric=euclidean prior=empirical"
			},
		},
		{
			Name:       "nearest_centroid_balanced",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "euclidean + uniform prior" },
			Summary: func(cfg MLConfig) string {
				return "metric=euclidean prior=uniform"
			},
		},
		{
			Name:       "nearest_centroid_cosine",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 4
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "cosine + uniform prior" },
			Summary: func(cfg MLConfig) string {
				return "metric=cosine prior=uniform"
			},
		},
		{
			Name:       "knn_cosine",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    []int{1, 3, 5, 7, 9, 11, 15, 21, 31},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 16
				cfg.MinSamplesLeaf = 10
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d distance=cosine weight=distance", cfg.NumTrees)
			},
		},
		{
			Name:       "random_forest_fast",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{5, 9, 13, 17, 21},
			YValues:    []int{3, 4, 5, 6, 7},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 2
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "random_forest_deep",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{31, 41, 51, 61, 71},
			YValues:    []int{8, 10, 12, 14},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "extra_trees_deep",
			ModelType:  ModelExtraTrees,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{31, 41, 51, 61, 71},
			YValues:    []int{8, 10, 12, 14},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelExtraTrees
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic_none",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{1, 3, 5, 10, 20, 50},
			YValues:    []int{500, 1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 4
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=none iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic_l1",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{3, 5, 10, 20, 50, 100},
			YValues:    []int{1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 12
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=l1 iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm_long",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron_long",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive_long",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "knn_distance",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    []int{1, 3, 5, 7, 9, 11, 15, 21, 31},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 12
				cfg.MinSamplesLeaf = 10
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d distance=manhattan weight=distance", cfg.NumTrees)
			},
		},
		{
			Name:       "ridge_strong",
			ModelType:  ModelRidge,
			Comparable: false,
			Kind:       "bar",
			XName:      "alpha×100",
			XValues:    []int{100, 150, 200, 250, 300, 400, 500},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRidge
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.2f", float64(v)/100.0) },
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
			},
		},
		{
			Name:       "adaboost_large",
			ModelType:  ModelAdaBoost,
			Comparable: false,
			Kind:       "bar",
			XName:      "estimators",
			XValues:    []int{50, 100, 150, 200, 250, 300, 400},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelAdaBoost
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("estimators=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "ensemble",
			ModelType:  ModelEnsemble,
			Comparable: true,
			Kind:       "bar",
			XName:      "voting",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelEnsemble
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "soft" },
			Summary: func(_ MLConfig) string {
				return "soft-vote ensemble"
			},
		},
	}
}

func comprehensiveAxisSweepProfiles(pointsPerParam int) []sweepProfile {
	if pointsPerParam < 1 {
		pointsPerParam = 1000
	}
	profiles := make([]sweepProfile, 0, len(AllModelTypes())*3)
	for _, mt := range AllModelTypes() {
		base := baseModelType(mt)
		switch base {
		case ModelRandomForest, ModelExtraTrees:
			profiles = append(profiles,
				numericAxisProfile(mt, "numTrees", "numTrees", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "maxDepth", "maxDepth", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }),
				numericAxisProfile(mt, "minSamplesLeaf", "minSamplesLeaf", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelLogisticRegression:
			profiles = append(profiles,
				numericAxisProfile(mt, "learningRate", "learningRate×1000", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "maxIter", "maxIter", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
				categoricalAxisProfile(mt, "regularization", "regularization", []int{4, 8, 12}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch v {
					case 4:
						return "none"
					case 12:
						return "l1"
					default:
						return "l2"
					}
				}),
			)
		case ModelSVM, ModelPerceptron:
			profiles = append(profiles,
				numericAxisProfile(mt, "learningRate", "learningRate×1000", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "iterations", "iterations", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelPassiveAggressive:
			profiles = append(profiles,
				numericAxisProfile(mt, "aggressivenessC", "C×10", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "iterations", "iterations", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelKNN:
			profiles = append(profiles,
				numericAxisProfile(mt, "k", "k", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				categoricalAxisProfile(mt, "distance", "distance selector", []int{8, 12, 16}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch {
					case v >= 16:
						return "cosine"
					case v >= 12:
						return "manhattan"
					default:
						return "euclidean"
					}
				}),
				categoricalAxisProfile(mt, "weight", "weight selector", []int{5, 8}, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }, func(v int) string {
					if v >= 8 {
						return "distance"
					}
					return "uniform"
				}),
			)
		case ModelRidge:
			profiles = append(profiles,
				numericAxisProfile(mt, "alpha", "alpha×100", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
			)
		case ModelAdaBoost:
			profiles = append(profiles,
				numericAxisProfile(mt, "estimators", "estimators", intRange(10, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
			)
		case ModelNearestCentroid:
			profiles = append(profiles,
				categoricalAxisProfile(mt, "metric", "metric selector", []int{4, 8, 12}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch v {
					case 4:
						return "cosine"
					case 12:
						return "manhattan"
					default:
						return "euclidean"
					}
				}),
				categoricalAxisProfile(mt, "classPrior", "class prior", []int{0, 1}, func(cfg *MLConfig, v int) { cfg.BalanceClasses = v == 1 }, func(v int) string {
					if v == 1 {
						return "uniform"
					}
					return "empirical"
				}),
			)
		case ModelNaiveBayes:
			profiles = append(profiles,
				categoricalAxisProfile(mt, "classPrior", "class prior", []int{0, 1}, func(cfg *MLConfig, v int) { cfg.BalanceClasses = v == 1 }, func(v int) string {
					if v == 1 {
						return "uniform"
					}
					return "empirical"
				}),
			)
		case ModelEnsemble:
			profiles = append(profiles, fixedAxisProfile(mt, "voting", "soft-vote ensemble"))
		}
	}
	return profiles
}

func numericSweepParametersForModel(modelType ModelType) []string {
	switch baseModelType(modelType) {
	case ModelRandomForest, ModelExtraTrees:
		return []string{"numTrees", "maxDepth", "minSamplesLeaf"}
	case ModelLogisticRegression:
		return []string{"learningRate", "maxIter"}
	case ModelSVM, ModelPerceptron:
		return []string{"learningRate", "iterations"}
	case ModelPassiveAggressive:
		return []string{"aggressivenessC", "iterations"}
	case ModelKNN:
		return []string{"k"}
	case ModelRidge:
		return []string{"alpha"}
	case ModelAdaBoost:
		return []string{"estimators"}
	default:
		return nil
	}
}

func numericAxisProfile(modelType ModelType, paramName, xName string, values []int, required int, apply func(*MLConfig, int)) sweepProfile {
	return axisProfile(modelType, paramName, "numeric", xName, values, required, apply, strconv.Itoa)
}

func categoricalAxisProfile(modelType ModelType, paramName, xName string, values []int, apply func(*MLConfig, int), label func(int) string) sweepProfile {
	return axisProfile(modelType, paramName, "categorical", xName, values, len(values), apply, label)
}

func fixedAxisProfile(modelType ModelType, paramName, summary string) sweepProfile {
	return sweepProfile{
		Name:                   string(modelType) + "/" + paramName,
		ModelType:              modelType,
		Comparable:             profileComparable(string(modelType)),
		Kind:                   "bar",
		XName:                  paramName,
		XValues:                []int{0},
		XLabel:                 func(int) string { return "default" },
		ParameterName:          paramName,
		ParameterKind:          "fixed",
		RequiredDiscretePoints: 1,
		Build: func(_, _ int) MLConfig {
			cfg := defaultSweepConfigForModel(modelType)
			return cfg
		},
		Summary: func(MLConfig) string { return summary },
	}
}

func axisProfile(modelType ModelType, paramName, paramKind, xName string, values []int, required int, apply func(*MLConfig, int), label func(int) string) sweepProfile {
	if label == nil {
		label = strconv.Itoa
	}
	return sweepProfile{
		Name:                   string(modelType) + "/" + paramName,
		ModelType:              modelType,
		Comparable:             profileComparable(string(modelType)),
		Kind:                   "bar",
		XName:                  xName,
		XValues:                values,
		XLabel:                 label,
		ParameterName:          paramName,
		ParameterKind:          paramKind,
		RequiredDiscretePoints: required,
		Build: func(x, _ int) MLConfig {
			cfg := defaultSweepConfigForModel(modelType)
			if apply != nil {
				apply(&cfg, x)
			}
			return cfg
		},
		Summary: summarizeSweepConfig,
	}
}

func defaultSweepConfigForModel(modelType ModelType) MLConfig {
	cfg := DefaultMLConfig()
	cfg.ModelType = modelType
	cfg.ValidationSplitRatio = 0.2
	cfg.LlmEnabled = false
	cfg.LlmBaseURL = ""
	cfg.LlmModel = ""
	cfg.LlmAPIKey = ""
	for _, profile := range builtinModelProfiles {
		if profile.Type != modelType {
			continue
		}
		if v := profile.Defaults["numTrees"]; v > 0 {
			cfg.NumTrees = v
		}
		if v := profile.Defaults["maxDepth"]; v > 0 {
			cfg.MaxDepth = v
		}
		if v := profile.Defaults["minSamplesLeaf"]; v > 0 {
			cfg.MinSamplesLeaf = v
		}
		if profile.Apply != nil {
			cfg = profile.Apply(cfg)
		}
		break
	}
	return cfg
}

func summarizeSweepConfig(cfg MLConfig) string {
	switch baseModelType(cfg.ModelType) {
	case ModelRandomForest, ModelExtraTrees:
		return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
	case ModelLogisticRegression:
		reg := "l2"
		switch cfg.MaxDepth {
		case 4:
			reg = "none"
		case 12:
			reg = "l1"
		}
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("lr=%.3f reg=%s%s iter=%d", float64(cfg.NumTrees)/1000.0, reg, balanced, cfg.MinSamplesLeaf)
	case ModelSVM, ModelPerceptron:
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("lr=%.3f%s iter=%d", float64(cfg.NumTrees)/1000.0, balanced, cfg.MinSamplesLeaf)
	case ModelPassiveAggressive:
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("C=%.2f%s iter=%d", float64(cfg.NumTrees)/10.0, balanced, cfg.MinSamplesLeaf)
	case ModelKNN:
		distance := "euclidean"
		if cfg.MaxDepth >= 16 {
			distance = "cosine"
		} else if cfg.MaxDepth >= 12 {
			distance = "manhattan"
		}
		weight := "uniform"
		if cfg.MinSamplesLeaf >= 8 {
			weight = "distance"
		}
		return fmt.Sprintf("k=%d distance=%s weight=%s", cfg.NumTrees, distance, weight)
	case ModelRidge:
		return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
	case ModelAdaBoost:
		return fmt.Sprintf("estimators=%d", cfg.NumTrees)
	case ModelNearestCentroid:
		metric := "euclidean"
		switch cfg.MaxDepth {
		case 4:
			metric = "cosine"
		case 12:
			metric = "manhattan"
		}
		prior := "empirical"
		if cfg.BalanceClasses {
			prior = "uniform"
		}
		return fmt.Sprintf("metric=%s prior=%s", metric, prior)
	case ModelNaiveBayes:
		if cfg.BalanceClasses {
			return "balanced-prior"
		}
		return "empirical-prior"
	case ModelEnsemble:
		return "soft-vote ensemble"
	default:
		return string(cfg.ModelType)
	}
}

func intRange(minVal, count int) []int {
	if count <= 0 {
		return nil
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		out[i] = minVal + i
	}
	return out
}

func linspaceIntGlobal(minVal, maxVal, count int) []int {
	if count <= 0 {
		return nil
	}
	if count == 1 {
		return []int{(minVal + maxVal) / 2}
	}
	out := make([]int, count)
	seen := make(map[int]bool, count)
	for i := 0; i < count; i++ {
		v := minVal + (maxVal-minVal)*i/(count-1)
		for seen[v] {
			v++
		}
		out[i] = v
		seen[v] = true
	}
	return out
}

func parseModelFilter(raw string) map[ModelType]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[ModelType]bool)
	for _, part := range strings.Split(raw, ",") {
		mt := ModelType(strings.TrimSpace(part))
		if mt != "" {
			out[mt] = true
		}
	}
	return out
}

type barItem struct {
	Label string
	Value float64
	Title string
}

func renderProfileChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.ValidationAccuracy,
				Title: fmt.Sprintf("%s | %s | val=%.2f%%", profile.Name, r.ConfigSummary, r.ValidationAccuracy*100),
			})
		}
		maxV := 1.0
		return renderBarChart(profile.Name+" validation accuracy", profile.XName, items, 0, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.ValidationAccuracy
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\ntrain=%.2f%%\ninfer=%.0f/s (%.2fms)",
			r.ConfigSummary, r.ValidationAccuracy*100, r.TrainAccuracy*100, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderHeatmap(profile.Name+" validation accuracy", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderProfileDurationChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.Duration,
				Title: fmt.Sprintf("%s | %s | duration=%.2fs", profile.Name, r.ConfigSummary, r.Duration),
			})
		}
		minV, maxV := minMax(func() []float64 {
			values := make([]float64, 0, len(results))
			for _, r := range results {
				values = append(values, r.Duration)
			}
			return values
		}())
		return renderBarChart(profile.Name+" training duration", profile.XName, items, minV, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.Duration
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\nduration=%.2fs\ninfer=%.0f/s (%.2fms)",
			r.ConfigSummary, r.ValidationAccuracy*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderDurationHeatmap(profile.Name+" training duration", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderProfileInferenceChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.InferenceThroughput,
				Title: fmt.Sprintf("%s | %s | infer=%.0f/s (%.2fms)", profile.Name, r.ConfigSummary, r.InferenceThroughput, r.InferenceLatencyMs),
			})
		}
		maxV := 0.0
		values := make([]float64, 0, len(results))
		for _, r := range results {
			values = append(values, r.InferenceThroughput)
		}
		_, maxV = minMax(values)
		return renderBarChart(profile.Name+" inference throughput", profile.XName, items, 0, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.InferenceThroughput
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\ntrain=%.2f%%\ninfer=%.0f/s\nlatency=%.2fms",
			r.ConfigSummary, r.ValidationAccuracy*100, r.TrainAccuracy*100, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderThroughputHeatmap(profile.Name+" inference throughput", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderBarChart(title, subtitle string, items []barItem, minV, maxV float64) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("empty bar chart")
	}
	width, height := 960, 420
	left, right, top, bottom := 80, 30, 60, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	maxVal := maxV
	if maxVal <= minV {
		maxVal = minV + 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.grid { stroke: #eee; stroke-width: 1; }
		.label { font-size: 12px; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.bar { fill: #1890ff; }
		.bar-best { fill: #52c41a; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	if subtitle != "" {
		fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">%s</text>`, left, html.EscapeString(subtitle))
	}
	for i := 0; i <= 5; i++ {
		v := minV + (maxVal-minV)*float64(i)/5.0
		y := float64(top) + plotH - (v-minV)/(maxVal-minV)*plotH
		fmt.Fprintf(&b, `<line class="grid" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, width-right, y, y)
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-8, y+4, fmt.Sprintf("%.0f%%", v*100))
	}
	fmt.Fprintf(&b, `<line class="axis" x1="%d" x2="%d" y1="%d" y2="%d"/>`, left, width-right, top+int(plotH), top+int(plotH))
	fmt.Fprintf(&b, `<line class="axis" x1="%d" x2="%d" y1="%d" y2="%d"/>`, left, left, top, top+int(plotH))

	barGap := 0.2
	barW := plotW / float64(len(items))
	bestIdx := 0
	bestVal := items[0].Value
	for i, item := range items {
		if item.Value > bestVal {
			bestVal = item.Value
			bestIdx = i
		}
	}
	for i, item := range items {
		x := float64(left) + float64(i)*barW + barW*barGap/2
		w := barW * (1 - barGap)
		h := 0.0
		if maxVal > minV {
			h = (item.Value - minV) / (maxVal - minV) * plotH
		}
		y := float64(top) + plotH - h
		fill := colorForScore(item.Value, minV, maxVal)
		if i == bestIdx {
			fill = "#52c41a"
		}
		fmt.Fprintf(&b, `<g><title>%s: %.2f%%</title><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" class="bar" fill="%s"/></g>`,
			html.EscapeString(item.Title), item.Value*100, x, y, w, h, fill)
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`,
			x+w/2, top+int(plotH)+22, html.EscapeString(item.Label))
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+w/2, y-6, fmt.Sprintf("%.1f%%", item.Value*100))
	}
	fmt.Fprintf(&b, `<text class="text label" x="%d" y="%d">%s</text>`, left, height-22, html.EscapeString(subtitle))
	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			fill := colorForScore(val, minV, maxV)
			highlight := ""
			if val >= maxV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.1f%%", val*100))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderDurationHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			score := maxV - val
			fill := colorForScore(score, 0, maxV-minV)
			highlight := ""
			if val <= minV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.2fs", val))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderThroughputHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			fill := colorForScore(val, minV, maxV)
			highlight := ""
			if val >= maxV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.0f/s", val))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func writeCSV(path string, results []sweepResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(sweepResultCSVHeader()); err != nil {
		return err
	}
	for _, r := range results {
		if err := w.Write(sweepResultCSVRow(r)); err != nil {
			return err
		}
	}
	return w.Error()
}

func appendSweepResultsCSV(path string, results []sweepResult) error {
	if len(results) == 0 {
		return nil
	}
	needsHeader := true
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		needsHeader = false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if needsHeader {
		if err := w.Write(sweepResultCSVHeader()); err != nil {
			return err
		}
	}
	for _, result := range results {
		if err := w.Write(sweepResultCSVRow(result)); err != nil {
			return err
		}
	}
	return w.Error()
}

func sweepResultCSVHeader() []string {
	return []string{
		"profile", "dataset", "baseProfile", "modelType",
		"parameterName", "parameterKind", "requiredDiscretePoints", "configuredDiscretePoints",
		"xValue", "yValue", "configSummary",
		"trainAccuracy", "validationAccuracy", "allowPassRate", "durationSeconds",
		"inferenceDurationSeconds", "inferenceSamples", "inferenceLatencyMs", "inferenceThroughput",
		"memoryBytes",
		"numSamples", "trainSamples", "validationSamples", "error",
	}
}

func sweepResultCSVRow(r sweepResult) []string {
	return []string{
		r.Profile,
		r.Dataset,
		r.BaseProfile,
		string(r.ModelType),
		r.ParameterName,
		r.ParameterKind,
		strconv.Itoa(r.RequiredPoints),
		strconv.Itoa(r.ConfiguredPoints),
		strconv.Itoa(r.XValue),
		strconv.Itoa(r.YValue),
		r.ConfigSummary,
		fmt.Sprintf("%.6f", r.TrainAccuracy),
		fmt.Sprintf("%.6f", r.ValidationAccuracy),
		fmt.Sprintf("%.6f", r.AllowPassRate),
		fmt.Sprintf("%.6f", r.Duration),
		fmt.Sprintf("%.6f", r.InferenceDuration),
		strconv.Itoa(r.InferenceSamples),
		fmt.Sprintf("%.6f", r.InferenceLatencyMs),
		fmt.Sprintf("%.6f", r.InferenceThroughput),
		strconv.Itoa(int(r.MemoryBytes)),
		strconv.Itoa(r.NumSamples),
		strconv.Itoa(r.TrainSamples),
		strconv.Itoa(r.ValidationSamples),
		r.Error,
	}
}

func readSweepResultsCSV(path string) ([]sweepResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("no cached sweep rows in %s", path)
	}
	header := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		header[name] = i
	}
	out := make([]sweepResult, 0, len(rows)-1)
	for _, row := range rows[1:] {
		out = append(out, sweepResult{
			Profile:             csvString(row, header, "profile"),
			Dataset:             csvString(row, header, "dataset"),
			BaseProfile:         csvString(row, header, "baseProfile"),
			ModelType:           ModelType(csvString(row, header, "modelType")),
			ParameterName:       csvString(row, header, "parameterName"),
			ParameterKind:       csvString(row, header, "parameterKind"),
			RequiredPoints:      csvInt(row, header, "requiredDiscretePoints"),
			ConfiguredPoints:    csvInt(row, header, "configuredDiscretePoints"),
			XValue:              csvInt(row, header, "xValue"),
			YValue:              csvInt(row, header, "yValue"),
			ConfigSummary:       csvString(row, header, "configSummary"),
			TrainAccuracy:       csvFloat(row, header, "trainAccuracy"),
			ValidationAccuracy:  csvFloat(row, header, "validationAccuracy"),
			AllowPassRate:       csvFloat(row, header, "allowPassRate"),
			Duration:            csvFloat(row, header, "durationSeconds"),
			InferenceDuration:   csvFloat(row, header, "inferenceDurationSeconds"),
			InferenceSamples:    csvInt(row, header, "inferenceSamples"),
			InferenceLatencyMs:  csvFloat(row, header, "inferenceLatencyMs"),
			InferenceThroughput: csvFloat(row, header, "inferenceThroughput"),
			MemoryBytes:         int64(csvInt(row, header, "memoryBytes")),
			NumSamples:          csvInt(row, header, "numSamples"),
			TrainSamples:        csvInt(row, header, "trainSamples"),
			ValidationSamples:   csvInt(row, header, "validationSamples"),
			Error:               csvString(row, header, "error"),
		})
	}
	return out, nil
}

func csvString(row []string, header map[string]int, name string) string {
	idx, ok := header[name]
	if !ok || idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}

func csvInt(row []string, header map[string]int, name string) int {
	value, _ := strconv.Atoi(csvString(row, header, name))
	return value
}

func csvFloat(row []string, header map[string]int, name string) float64 {
	value, _ := strconv.ParseFloat(csvString(row, header, name), 64)
	return value
}

type sweepCoverage struct {
	Summary  map[string]any         `json:"summary"`
	Datasets []map[string]any       `json:"datasets"`
	Profiles []sweepCoverageProfile `json:"profiles"`
}

type sweepCoverageProfile struct {
	Dataset                  string `json:"dataset"`
	Profile                  string `json:"profile"`
	ModelType                string `json:"modelType"`
	Parameter                string `json:"parameter"`
	ParameterKind            string `json:"parameterKind"`
	RequiredDiscretePoints   int    `json:"requiredDiscretePoints"`
	ConfiguredDiscretePoints int    `json:"configuredDiscretePoints"`
	TestedRows               int    `json:"testedRows"`
	Passed                   bool   `json:"passed"`
}

func buildSweepCoverage(datasets []sweepDataset, profiles []sweepProfile, results []sweepResult, pointsPerParam int) sweepCoverage {
	rowCounts := make(map[string]int)
	for _, result := range results {
		rowCounts[result.Profile]++
	}
	entries := make([]sweepCoverageProfile, 0, len(datasets)*len(profiles))
	passed := 0
	required := 0
	for _, dataset := range datasets {
		for _, profile := range profiles {
			scoped := profileForDataset(profile, dataset)
			configured := configuredProfilePointCount(profile)
			req := profile.RequiredDiscretePoints
			if req < 1 {
				req = configured
			}
			ok := rowCounts[scoped.Name] >= configured && configured >= req
			if profile.ParameterKind == "categorical" || profile.ParameterKind == "fixed" {
				ok = rowCounts[scoped.Name] >= configured && configured == req
			}
			required++
			if ok {
				passed++
			}
			entries = append(entries, sweepCoverageProfile{
				Dataset:                  dataset.Name,
				Profile:                  scoped.Name,
				ModelType:                string(profile.ModelType),
				Parameter:                profile.ParameterName,
				ParameterKind:            profile.ParameterKind,
				RequiredDiscretePoints:   req,
				ConfiguredDiscretePoints: configured,
				TestedRows:               rowCounts[scoped.Name],
				Passed:                   ok,
			})
		}
	}
	datasetRows := make([]map[string]any, 0, len(datasets))
	for _, dataset := range datasets {
		datasetRows = append(datasetRows, map[string]any{
			"name":        dataset.Name,
			"description": dataset.Description,
			"samples":     len(dataset.Samples),
		})
	}
	return sweepCoverage{
		Summary: map[string]any{
			"datasets":               len(datasets),
			"profiles":               len(profiles),
			"coverageEntries":        required,
			"passedEntries":          passed,
			"pointsPerParam":         pointsPerParam,
			"numericRequirementNote": "numeric comprehensive axis profiles require at least pointsPerParam unique tested values per tunable parameter; categorical/fixed profiles enumerate all meaningful values",
		},
		Datasets: datasetRows,
		Profiles: entries,
	}
}

func writeCoverageJSON(path string, coverage sweepCoverage) error {
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func uniqueIntCount(values []int) int {
	if len(values) == 0 {
		return 0
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	return len(seen)
}

func writeRepeatCSV(path string, runs []repeatRunResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"profile", "modelType", "xValue", "yValue", "runIndex", "configSummary",
		"trainAccuracy", "validationAccuracy", "allowPassRate", "durationSeconds",
		"inferenceDurationSeconds", "inferenceSamples", "inferenceLatencyMs", "inferenceThroughput",
		"memoryBytes",
		"numSamples", "trainSamples", "validationSamples", "error",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range runs {
		row := []string{
			r.Profile,
			string(r.ModelType),
			strconv.Itoa(r.XValue),
			strconv.Itoa(r.YValue),
			strconv.Itoa(r.RunIndex),
			r.ConfigSummary,
			fmt.Sprintf("%.6f", r.TrainAccuracy),
			fmt.Sprintf("%.6f", r.ValidationAccuracy),
			fmt.Sprintf("%.6f", r.AllowPassRate),
			fmt.Sprintf("%.6f", r.Duration),
			fmt.Sprintf("%.6f", r.InferenceDuration),
			strconv.Itoa(r.InferenceSamples),
			fmt.Sprintf("%.6f", r.InferenceLatencyMs),
			fmt.Sprintf("%.6f", r.InferenceThroughput),
			strconv.Itoa(int(r.MemoryBytes)),
			strconv.Itoa(r.NumSamples),
			strconv.Itoa(r.TrainSamples),
			strconv.Itoa(r.ValidationSamples),
			r.Error,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeRepeatSummaryCSV(path string, summaries []repeatSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"profile", "modelType", "comparable", "xValue", "yValue", "configSummary",
		"runs", "successRuns", "failureRuns", "successRate",
		"trainMean", "trainStd", "validationMean", "validationStd",
		"validationMin", "validationMax", "allowMean", "allowStd", "allowMin", "allowMax",
		"durationMean", "durationStd",
		"inferenceMean", "inferenceStd", "inferenceMin", "inferenceMax", "inferenceLatencyMean", "inferenceLatencyStd",
		"memoryMean", "memoryStd", "memoryMin", "memoryMax",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, s := range summaries {
		row := []string{
			s.Profile,
			string(s.ModelType),
			strconv.FormatBool(s.Comparable),
			strconv.Itoa(s.XValue),
			strconv.Itoa(s.YValue),
			s.ConfigSummary,
			strconv.Itoa(s.Runs),
			strconv.Itoa(s.SuccessRuns),
			strconv.Itoa(s.FailureRuns),
			fmt.Sprintf("%.6f", s.SuccessRate),
			fmt.Sprintf("%.6f", s.TrainMean),
			fmt.Sprintf("%.6f", s.TrainStd),
			fmt.Sprintf("%.6f", s.ValidationMean),
			fmt.Sprintf("%.6f", s.ValidationStd),
			fmt.Sprintf("%.6f", s.ValidationMin),
			fmt.Sprintf("%.6f", s.ValidationMax),
			fmt.Sprintf("%.6f", s.AllowMean),
			fmt.Sprintf("%.6f", s.AllowStd),
			fmt.Sprintf("%.6f", s.AllowMin),
			fmt.Sprintf("%.6f", s.AllowMax),
			fmt.Sprintf("%.6f", s.DurationMean),
			fmt.Sprintf("%.6f", s.DurationStd),
			fmt.Sprintf("%.6f", s.InferenceMean),
			fmt.Sprintf("%.6f", s.InferenceStd),
			fmt.Sprintf("%.6f", s.InferenceMin),
			fmt.Sprintf("%.6f", s.InferenceMax),
			fmt.Sprintf("%.6f", s.InferenceLatencyMean),
			fmt.Sprintf("%.6f", s.InferenceLatencyStd),
			fmt.Sprintf("%.0f", s.MemoryMean),
			fmt.Sprintf("%.0f", s.MemoryStd),
			fmt.Sprintf("%.0f", s.MemoryMin),
			fmt.Sprintf("%.0f", s.MemoryMax),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func renderStabilityChart(summaries []repeatSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no stability summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortProfileLabel(s.Profile),
			Value: s.ValidationMean,
			Title: fmt.Sprintf("%s | %s | mean=%.2f%% ± %.2f%% | success=%.0f%%",
				s.Profile, s.ConfigSummary, s.ValidationMean*100, s.ValidationStd*100, s.SuccessRate*100),
		})
	}
	return renderBarChart("100-run mean validation accuracy", "higher is better", items, 0.0, 1.0)
}

func renderOverallSpeedChart(summaries []profileSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no sweep summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortProfileLabel(s.Profile.Name),
			Value: s.Best.InferenceThroughput,
			Title: fmt.Sprintf("%s | %s | infer=%.0f/s (%.2fms) | val=%.2f%%",
				s.Profile.Name, s.Best.ConfigSummary, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, s.Best.ValidationAccuracy*100),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	values := make([]float64, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}
	minV, maxV := minMax(values)
	return renderBarChart("Best inference throughput by model", "higher is better", items, minV, maxV)
}

func renderStabilitySpeedChart(summaries []repeatSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no stability summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortModelLabel(s.ModelType),
			Value: s.InferenceMean,
			Title: fmt.Sprintf("%s | %s | infer=%.0f/s ± %.0f/s | mean val=%.2f%%",
				s.Profile, s.ConfigSummary, s.InferenceMean, s.InferenceStd, s.ValidationMean*100),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	values := make([]float64, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}
	minV, maxV := minMax(values)
	return renderBarChart("100-run mean inference throughput", "higher is better", items, minV, maxV)
}

func writeReportHTML(path string, summaries []profileSummary, repeats []repeatSummary, repeatCount, stabilityTop int) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>ML Sweep Report</title>`)
	b.WriteString(`<style>
		body { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 24px; color: #222; }
		h1, h2, h3 { margin: 0.2em 0 0.4em; }
		p, li { line-height: 1.5; }
		table { border-collapse: collapse; width: 100%; margin: 16px 0 28px; }
		th, td { border: 1px solid #ddd; padding: 8px 10px; vertical-align: top; }
		th { background: #fafafa; text-align: left; position: sticky; top: 0; }
		.small { color: #666; font-size: 12px; }
		.card { border: 1px solid #e8e8e8; border-radius: 10px; padding: 16px; margin: 20px 0; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
		.chart { max-width: 100%; overflow-x: auto; }
		.chart-row { display: flex; gap: 16px; flex-wrap: wrap; }
		.chart-row .chart { flex: 1 1 440px; }
		code { background: #f6f8fa; padding: 2px 4px; border-radius: 4px; }
	</style></head><body>`)

	best := bestScreenSummary(summaries)
	if best == nil {
		return fmt.Errorf("no sweep summaries")
	}
	stabilityBest := bestComparableSummary(repeats)

	fmt.Fprintf(&b, `<h1>ML Sweep Report</h1>`)
	fmt.Fprintf(&b, `<p class="small">Generated at %s. Results are based on the persisted local training store used by the running backend.</p>`, html.EscapeString(time.Now().Format(time.RFC3339)))
	fmt.Fprintf(&b, `<div class="card"><h2>Grid best</h2><p><b>%s</b> — %s — validation <b>%.2f%%</b>, ALLOW pass <b>%.2f%%</b>, train <b>%.2f%%</b>, infer <b>%.0f/s</b> (%.2fms)</p><p class="small">Charts: <code>overall_best.svg</code> and <code>overall_speed.svg</code>; raw CSV: <code>results.csv</code>; JSON summary: <code>best.json</code></p><div class="chart-row"><div class="chart"><img src="overall_best.svg" alt="Overall best chart" style="max-width:100%%;height:auto"></div><div class="chart"><img src="overall_speed.svg" alt="Overall speed chart" style="max-width:100%%;height:auto"></div></div></div>`,
		html.EscapeString(best.Profile.Name), html.EscapeString(best.Best.ConfigSummary), best.Best.ValidationAccuracy*100, best.Best.AllowPassRate*100, best.Best.TrainAccuracy*100, best.Best.InferenceThroughput, best.Best.InferenceLatencyMs)

	if stabilityBest != nil {
		fmt.Fprintf(&b, `<div class="card"><h2>100-run stability best</h2><p><b>%s</b> — %s — mean validation <b>%.2f%%</b> ± <b>%.2f%%</b>, mean ALLOW pass <b>%.2f%%</b> ± <b>%.2f%%</b>; mean speed <b>%.0f/s</b> ± <b>%.0f/s</b> across %d runs</p><p class="small">Charts: <code>stability_best.svg</code> and <code>stability_speed.svg</code>; raw runs: <code>stability-runs.csv</code>; summary CSV: <code>stability-summary.csv</code></p><div class="chart-row"><div class="chart"><img src="stability_best.svg" alt="Stability chart" style="max-width:100%%;height:auto"></div><div class="chart"><img src="stability_speed.svg" alt="Stability speed chart" style="max-width:100%%;height:auto"></div></div></div>`,
			html.EscapeString(stabilityBest.Profile), html.EscapeString(stabilityBest.ConfigSummary), stabilityBest.ValidationMean*100, stabilityBest.ValidationStd*100, stabilityBest.AllowMean*100, stabilityBest.AllowStd*100, stabilityBest.InferenceMean, stabilityBest.InferenceStd, repeatCount)
	}

	if best != nil {
		bf := slug(best.Profile.Name)
		paramRows := append([]sweepResult(nil), best.Results...)
		sort.Slice(paramRows, func(i, j int) bool {
			if paramRows[i].ValidationAccuracy != paramRows[j].ValidationAccuracy {
				return paramRows[i].ValidationAccuracy > paramRows[j].ValidationAccuracy
			}
			if paramRows[i].InferenceThroughput != paramRows[j].InferenceThroughput {
				return paramRows[i].InferenceThroughput > paramRows[j].InferenceThroughput
			}
			if paramRows[i].Duration != paramRows[j].Duration {
				return paramRows[i].Duration < paramRows[j].Duration
			}
			if paramRows[i].XValue != paramRows[j].XValue {
				return paramRows[i].XValue < paramRows[j].XValue
			}
			return paramRows[i].YValue < paramRows[j].YValue
		})
		fmt.Fprintf(&b, `<div class="card"><h2>Best model parameter sweep</h2><p><b>%s</b> — grid best <b>%s</b>. The charts below show <b>validation accuracy</b>, <b>training duration</b>, <b>inference throughput</b>, and <b>ALLOW pass rate</b> for every tested parameter point.</p><p class="small">Artifacts: <code>%s.svg</code>, <code>%s-duration.svg</code>, <code>%s-inference.svg</code>, <code>%s-grid.csv</code></p><div class="chart-row"><div class="chart"><img src="%s.svg" alt="%s validation heatmap" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-duration.svg" alt="%s duration heatmap" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-inference.svg" alt="%s inference heatmap" style="max-width:100%%;height:auto"></div></div>`,
			html.EscapeString(best.Profile.Name), html.EscapeString(best.Best.ConfigSummary), bf, bf, bf, bf, bf, html.EscapeString(best.Profile.Name), bf, html.EscapeString(best.Profile.Name), bf, html.EscapeString(best.Profile.Name))
		fmt.Fprintf(&b, `<table><thead><tr><th>Config</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Duration</th><th>Infer/s</th><th>Latency</th><th>X</th><th>Y</th></tr></thead><tbody>`)
		for _, r := range paramRows {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2fs</td><td>%.0f/s</td><td>%.2fms</td><td>%d</td><td>%d</td></tr>`,
				html.EscapeString(r.ConfigSummary), r.TrainAccuracy*100, r.ValidationAccuracy*100, r.AllowPassRate*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs, r.XValue, r.YValue)
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<h2>Profile details</h2>`)
	for _, s := range summaries {
		fmt.Fprintf(&b, `<div class="card"><h3>%s</h3>`, html.EscapeString(s.Profile.Name))
		fmt.Fprintf(&b, `<p class="small">Best grid point: <b>%s</b> — validation <b>%.2f%%</b> / ALLOW pass <b>%.2f%%</b> / train <b>%.2f%%</b> / infer <b>%.0f/s</b> (%.2fms) (%s)</p>`,
			html.EscapeString(s.Best.ConfigSummary), s.Best.ValidationAccuracy*100, s.Best.AllowPassRate*100, s.Best.TrainAccuracy*100, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, ternary(s.Profile.Comparable, "holdout-comparable", "train-set / optimistic"))
		fmt.Fprintf(&b, `<div class="chart-row"><div class="chart"><img src="%s.svg" alt="%s" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-inference.svg" alt="%s inference" style="max-width:100%%;height:auto"></div></div>`, slug(s.Profile.Name), html.EscapeString(s.Profile.Name), slug(s.Profile.Name), html.EscapeString(s.Profile.Name))
		topRows := append([]sweepResult(nil), s.Results...)
		sort.Slice(topRows, func(i, j int) bool {
			if topRows[i].ValidationAccuracy != topRows[j].ValidationAccuracy {
				return topRows[i].ValidationAccuracy > topRows[j].ValidationAccuracy
			}
			if topRows[i].AllowPassRate != topRows[j].AllowPassRate {
				return topRows[i].AllowPassRate > topRows[j].AllowPassRate
			}
			if topRows[i].InferenceThroughput != topRows[j].InferenceThroughput {
				return topRows[i].InferenceThroughput > topRows[j].InferenceThroughput
			}
			return topRows[i].Duration < topRows[j].Duration
		})
		if len(topRows) > 5 {
			topRows = topRows[:5]
		}
		fmt.Fprintf(&b, `<table><thead><tr><th>Config</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Duration</th><th>Infer/s</th><th>Latency</th><th>Error</th></tr></thead><tbody>`)
		for _, r := range topRows {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2fs</td><td>%.0f/s</td><td>%.2fms</td><td>%s</td></tr>`,
				html.EscapeString(r.ConfigSummary), r.TrainAccuracy*100, r.ValidationAccuracy*100, r.AllowPassRate*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs, html.EscapeString(r.Error))
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<div class="card"><h2>Grid summary</h2><table><thead><tr><th>Model</th><th>Best config</th><th>Comparable</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Infer/s</th><th>Latency</th><th>Runs</th></tr></thead><tbody>`)
	for _, s := range summaries {
		fmt.Fprintf(&b, `<tr><td>%s</td><td><code>%s</code></td><td>%s</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.0f/s</td><td>%.2fms</td><td>%d</td></tr>`,
			html.EscapeString(s.Profile.Name), html.EscapeString(s.Best.ConfigSummary), ternary(s.Profile.Comparable, "yes", "no"), s.Best.TrainAccuracy*100, s.Best.ValidationAccuracy*100, s.Best.AllowPassRate*100, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, len(s.Results))
	}
	fmt.Fprintf(&b, `</tbody></table></div>`)

	if len(repeats) > 0 {
		fmt.Fprintf(&b, `<div class="card"><h2>100-run stability summary</h2><table><thead><tr><th>Model</th><th>Config</th><th>Comparable</th><th>Mean val</th><th>Std val</th><th>Mean ALLOW</th><th>Std ALLOW</th><th>Mean speed</th><th>Std speed</th><th>Success</th><th>Runs</th></tr></thead><tbody>`)
		for _, s := range repeats {
			fmt.Fprintf(&b, `<tr><td>%s</td><td><code>%s</code></td><td>%s</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.0f/s</td><td>%.0f/s</td><td>%.0f%%</td><td>%d</td></tr>`,
				html.EscapeString(s.Profile), html.EscapeString(s.ConfigSummary), ternary(s.Comparable, "yes", "no"), s.ValidationMean*100, s.ValidationStd*100, s.AllowMean*100, s.AllowStd*100, s.InferenceMean, s.InferenceStd, s.SuccessRate*100, s.Runs)
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<div class="card"><h2>Notes</h2><ul>`)
	fmt.Fprintf(&b, `<li><code>random_forest</code> / <code>extra_trees</code> sweep trees × depth with leaf fixed at 3.</li>`)
	fmt.Fprintf(&b, `<li><code>logistic</code> uses <code>numTrees</code> as learning-rate × 1000 and <code>maxDepth</code> as regularization selector.</li>`)
	fmt.Fprintf(&b, `<li><code>svm</code>, <code>perceptron</code>, and <code>passive_aggressive</code> use <code>numTrees</code> as learning-rate × 1000 and <code>minSamplesLeaf</code> as iterations.</li>`)
	fmt.Fprintf(&b, `<li>Phase 1 runs a horizontal grid sweep; phase 2 repeats each profile's top <code>%d</code> grid point(s) <code>%d</code> times for stability.</li>`, stabilityTop, repeatCount)
	fmt.Fprintf(&b, `<li>Inference speed is benchmarked on a fixed cached sample slice from the persisted dataset, so throughput and latency are comparable across all families.</li>`)
	fmt.Fprintf(&b, `<li><code>random_forest</code>, <code>extra_trees</code>, <code>logistic</code>, <code>svm</code>, <code>perceptron</code>, <code>passive_aggressive</code>, and <code>nearest_centroid</code> are holdout-comparable in this repo; <code>knn</code>, <code>ridge</code>, <code>adaboost</code>, and <code>naive_bayes</code> currently report training-set-based scores in their trainers.</li>`)
	fmt.Fprintf(&b, `<li>We now track <strong>ALLOW pass rate</strong> alongside overall accuracy so the sweep does not over-optimize on catching bad commands while accidentally blocking good ones.</li>`)
	fmt.Fprintf(&b, `<li>The sweep runs offline against the persisted dataset, so it does not require the live backend to be free.</li>`)
	fmt.Fprintf(&b, `</ul></div>`)

	fmt.Fprintf(&b, `</body></html>`)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func indexOf(xs []int, target int) int {
	for i, v := range xs {
		if v == target {
			return i
		}
	}
	return -1
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"(", "",
		")", "",
	)
	return repl.Replace(s)
}

func shortModelLabel(mt ModelType) string {
	switch mt {
	case ModelRandomForest:
		return "RF"
	case ModelExtraTrees:
		return "ET"
	case ModelKNN:
		return "KNN"
	case ModelNaiveBayes:
		return "NB"
	case ModelAdaBoost:
		return "Ada"
	case ModelLogisticRegression:
		return "LR"
	case ModelSVM:
		return "SVM"
	case ModelRidge:
		return "Ridge"
	case ModelPerceptron:
		return "Perc"
	case ModelPassiveAggressive:
		return "PA"
	default:
		return string(mt)
	}
}

func shortProfileLabel(profile string) string {
	label := strings.ReplaceAll(strings.TrimSpace(profile), "_", " ")
	repl := strings.NewReplacer(
		"random forest", "RF",
		"extra trees", "ET",
		"nearest centroid cosine", "NC cos",
		"nearest centroid balanced", "NC bal",
		"nearest centroid", "NC",
		"logistic regression", "LR",
		"logistic balanced", "LR bal",
		"logistic", "LR",
		"passive aggressive", "PA",
		"passive aggressive balanced", "PA bal",
		"perceptron", "Perc",
		"perceptron balanced", "Perc bal",
		"knn", "KNN",
		"knn cosine", "KNN cos",
		"adaboost", "Ada",
		"naive bayes", "NB",
		"naive bayes balanced", "NB bal",
		"ensemble", "Ens",
	)
	return repl.Replace(label)
}

func colorForScore(v, minV, maxV float64) string {
	if maxV <= minV {
		return "#1890ff"
	}
	t := (v - minV) / (maxV - minV)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	red := [3]float64{245, 34, 45}
	yellow := [3]float64{250, 173, 20}
	green := [3]float64{82, 196, 26}
	var c [3]float64
	if t < 0.5 {
		u := t * 2
		for i := 0; i < 3; i++ {
			c[i] = red[i] + (yellow[i]-red[i])*u
		}
	} else {
		u := (t - 0.5) * 2
		for i := 0; i < 3; i++ {
			c[i] = yellow[i] + (green[i]-yellow[i])*u
		}
	}
	return fmt.Sprintf("#%02x%02x%02x", int(c[0]+0.5), int(c[1]+0.5), int(c[2]+0.5))
}

func contrastColor(fill string) string {
	if len(fill) != 7 || !strings.HasPrefix(fill, "#") {
		return "#111"
	}
	r, _ := strconv.ParseInt(fill[1:3], 16, 64)
	g, _ := strconv.ParseInt(fill[3:5], 16, 64)
	b, _ := strconv.ParseInt(fill[5:7], 16, 64)
	luma := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luma < 150 {
		return "#fff"
	}
	return "#111"
}

func ternary(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
