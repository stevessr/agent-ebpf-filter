package sweep

import (
	"agent-ebpf-filter/app/ml"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section sweepcorereport.go ----

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

	ml.InitTrainingStore(100000)
	if ml.GlobalTrainingStore == nil {
		return fmt.Errorf("training store not initialized")
	}
	labeled := ml.GlobalTrainingStore.LabeledSamples()
	if len(labeled) == 0 {
		return fmt.Errorf("no labeled samples found in the persisted training store")
	}
	datasets := datasetProfilesForMode(labeled, mode, selectedDatasets)
	if len(datasets) == 0 {
		return fmt.Errorf("no sweep datasets selected")
	}

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

func runProfile(profile sweepProfile, store *ml.TrainingDataStore, benchmarkSamples []ml.TrainingSample, workers int) ([]sweepResult, sweepResult, error) {
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
