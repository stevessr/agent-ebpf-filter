package app

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section autotunemodel.go ----

func runModelAutoTune(store *TrainingDataStore, req MLModelTuneRequest, modelTypes []ModelType, progressCb func(completed, total int, message string)) (*MLModelTuneResponse, error) {
	return runModelAutoTuneWithCancel(store, req, modelTypes, progressCb, nil)
}

func runModelAutoTuneWithCancel(store *TrainingDataStore, req MLModelTuneRequest, modelTypes []ModelType, progressCb func(completed, total int, message string), isCanceled func() bool) (*MLModelTuneResponse, error) {
	labeled := store.LabeledSamples()
	if len(labeled) < 2 {
		return nil, errors.New("need at least 2 labeled samples for model tuning")
	}
	metric := normalizeAutoTuneMetric(req.Metric)
	if metric == "" {
		metric = "validationAccuracy"
	}
	validationRatio := req.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = currentMLConfig().ValidationSplitRatio
	}
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}

	start := time.Now()
	candidates := make([]MLModelTuneCandidate, 0, len(modelTypes))
	var best *MLModelTuneCandidate
	var bestModel Model
	bestScore := math.Inf(-1)
	benchmarkSamples := selectBenchmarkSamples(labeled, 64)
	baseCfg := currentMLConfig()

	if progressCb != nil {
		progressCb(0, len(modelTypes), "开始跨模型自动调优")
	}

	for i, modelType := range modelTypes {
		if (isCanceled != nil && isCanceled()) || globalTrainer.IsCancelled() {
			return nil, errors.New("cancelled")
		}
		label, base, recommended := modelTuneCatalogInfo(modelType)
		cfg := baseCfg
		cfg.ModelType = modelType
		cfg.ValidationSplitRatio = validationRatio
		effectiveCfg := applyBuiltinModelPreset(cfg)
		candidate := MLModelTuneCandidate{
			ModelType:   string(modelType),
			Label:       label,
			Base:        base,
			Recommended: recommended,
			HyperParams: map[string]int{
				"numTrees":       effectiveCfg.NumTrees,
				"maxDepth":       effectiveCfg.MaxDepth,
				"minSamplesLeaf": effectiveCfg.MinSamplesLeaf,
			},
			SampleCount: len(labeled),
		}
		if progressCb != nil {
			progressCb(i, len(modelTypes), fmt.Sprintf("训练 %s", label))
		}

		trainStart := time.Now()
		model, result := globalTrainer.TrainWithConfig(store, cfg)
		candidate.TrainDuration = time.Since(trainStart).Seconds()
		if result.Error != "" {
			candidate.Error = result.Error
			candidates = append(candidates, candidate)
			if progressCb != nil {
				progressCb(i+1, len(modelTypes), fmt.Sprintf("跳过 %s: %s", label, result.Error))
			}
			continue
		}

		candidate.TrainAccuracy = result.TrainAccuracy
		candidate.ValidationAccuracy = result.ValidationAccuracy
		candidate.ValidationCount = result.ValidationSamples
		if candidate.ValidationCount == 0 {
			candidate.ValidationCount = len(globalTrainer.LastValidationSamples())
		}
		validationMetrics := evaluateAutoTuneTrainingSampleMetrics(globalTrainer.LastValidationSamples(), model)
		candidate.AllowRecall = validationMetrics.AllowRecall
		candidate.BalancedAccuracy = validationMetrics.BalancedAccuracy

		if req.TuneParams {
			paramReq := MLAutoTuneRequest{
				XAxis:                req.XAxis,
				YAxis:                req.YAxis,
				GridSize:             req.GridSize,
				Granularity:          req.Granularity,
				Metric:               metric,
				ValidationSplitRatio: validationRatio,
				MinX:                 req.MinX,
				MaxX:                 req.MaxX,
				MinY:                 req.MinY,
				MaxY:                 req.MaxY,
			}
			paramResp, err := globalTrainer.AutoTuneWithConfig(store, cfg, paramReq, nil)
			if err == nil && paramResp != nil && paramResp.Best != nil {
				candidate.ParamTune = paramResp
				cfg.NumTrees = paramResp.Best.NumTrees
				cfg.MaxDepth = paramResp.Best.MaxDepth
				cfg.MinSamplesLeaf = paramResp.Best.MinSamplesLeaf
				candidate.HyperParams["numTrees"] = cfg.NumTrees
				candidate.HyperParams["maxDepth"] = cfg.MaxDepth
				candidate.HyperParams["minSamplesLeaf"] = cfg.MinSamplesLeaf
				trainStart = time.Now()
				model, result = globalTrainer.TrainWithConfig(store, cfg)
				candidate.TrainDuration += time.Since(trainStart).Seconds()
				if result.Error == "" {
					candidate.TrainAccuracy = result.TrainAccuracy
					candidate.ValidationAccuracy = result.ValidationAccuracy
					candidate.ValidationCount = result.ValidationSamples
					validationMetrics = evaluateAutoTuneTrainingSampleMetrics(globalTrainer.LastValidationSamples(), model)
					candidate.AllowRecall = validationMetrics.AllowRecall
					candidate.BalancedAccuracy = validationMetrics.BalancedAccuracy
				} else {
					candidate.Error = result.Error
				}
			}
		}

		evalDuration, throughput, latencyMs, _ := benchmarkModelInference(model, benchmarkSamples)
		candidate.EvalDuration = evalDuration
		candidate.InferenceThroughput = throughput
		candidate.InferenceMsPerSample = latencyMs
		candidate.Score = autoTuneMetricScore(metric, candidate.ValidationAccuracy, candidate.InferenceThroughput, autoTuneClassificationMetrics{
			AllowRecall:      candidate.AllowRecall,
			BalancedAccuracy: candidate.BalancedAccuracy,
		})
		candidates = append(candidates, candidate)
		if candidate.Error == "" && candidate.Score > bestScore {
			copyCandidate := candidate
			best = &copyCandidate
			bestModel = model
			bestScore = candidate.Score
		}
		globalTrainer.logf("模型调优: %s 验证准确率 %.1f%% 推理 %.0f/s", label, candidate.ValidationAccuracy*100, candidate.InferenceThroughput)
		if progressCb != nil {
			progressCb(i+1, len(modelTypes), fmt.Sprintf("完成 %s (%d/%d)", label, i+1, len(modelTypes)))
		}
	}

	if isCanceled != nil && isCanceled() {
		return nil, errors.New("cancelled")
	}
	if best == nil {
		return &MLModelTuneResponse{Metric: metric, SampleCount: len(labeled), TotalDuration: time.Since(start).Seconds(), Candidates: candidates}, errors.New("no model candidate trained successfully")
	}
	if req.ApplyBest {
		if err := applyModelTuneBest(*best, bestModel, validationRatio); err != nil {
			return nil, err
		}
		best.Applied = true
		for i := range candidates {
			if candidates[i].ModelType == best.ModelType {
				candidates[i].Applied = true
			}
		}
	}

	return &MLModelTuneResponse{
		Metric:          metric,
		SampleCount:     len(labeled),
		ValidationCount: best.ValidationCount,
		TotalDuration:   time.Since(start).Seconds(),
		Candidates:      candidates,
		Best:            best,
	}, nil
}

func applyModelTuneBest(best MLModelTuneCandidate, model Model, validationRatio float64) error {
	if model == nil {
		return errors.New("best model is not available")
	}
	settings := runtimeSettingsStore.Snapshot()
	settings.MLConfig.ModelType = ModelType(best.ModelType)
	settings.MLConfig.ValidationSplitRatio = validationRatio
	if v, ok := best.HyperParams["numTrees"]; ok && v > 0 {
		settings.MLConfig.NumTrees = v
	}
	if v, ok := best.HyperParams["maxDepth"]; ok && v > 0 {
		settings.MLConfig.MaxDepth = v
	}
	if v, ok := best.HyperParams["minSamplesLeaf"]; ok && v > 0 {
		settings.MLConfig.MinSamplesLeaf = v
	}
	if _, err := runtimeSettingsStore.Replace(settings); err != nil {
		return err
	}
	currentModelType = settings.MLConfig.ModelType
	mlEngine = model
	mlModelLoaded = true
	modelPath := settings.MLConfig.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}
	if err := model.Serialize(modelPath); err != nil {
		return fmt.Errorf("model selected but failed to save: %w", err)
	}
	return nil
}
