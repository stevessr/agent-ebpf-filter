package app

import (
	"agent-ebpf-filter/app/ml"
	"errors"
	"fmt"
	"math"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section autotunemodel.go ----

func runModelAutoTune(store *ml.TrainingDataStore, req ml.MLModelTuneRequest, modelTypes []ModelType, progressCb func(completed, total int, message string)) (*ml.MLModelTuneResponse, error) {
	return runModelAutoTuneWithCancel(store, req, modelTypes, progressCb, nil)
}

func runModelAutoTuneWithCancel(store *ml.TrainingDataStore, req ml.MLModelTuneRequest, modelTypes []ModelType, progressCb func(completed, total int, message string), isCanceled func() bool) (*ml.MLModelTuneResponse, error) {
	labeled := store.LabeledSamples()
	if len(labeled) < 2 {
		return nil, errors.New("need at least 2 labeled samples for model tuning")
	}
	metric := ml.NormalizeSecurityAutoTuneMetric(req.Metric)
	if metric == "" {
		// Cross-model AutoML is security-first by default. Callers can still
		// explicitly request validationAccuracy, balancedAccuracy or throughput.
		metric = "securityUtility"
	}
	validationRatio := req.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = currentMLConfig().ValidationSplitRatio
	}
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}

	start := time.Now()
	candidates := make([]ml.MLModelTuneCandidate, 0, len(modelTypes))
	var best *ml.MLModelTuneCandidate
	var bestModel ml.Model
	bestScore := math.Inf(-1)
	benchmarkSamples := ml.SelectBenchmarkSamples(labeled, 64)
	baseCfg := currentMLConfig()

	if progressCb != nil {
		progressCb(0, len(modelTypes), "开始跨模型安全导向自动调优")
	}

	for i, modelType := range modelTypes {
		if (isCanceled != nil && isCanceled()) || ml.GlobalTrainer.IsCancelled() {
			return nil, errors.New("cancelled")
		}
		label, base, recommended := modelTuneCatalogInfo(modelType)
		taxonomy := ml.ModelTaxonomy(modelType)
		cfg := baseCfg
		cfg.ModelType = modelType
		cfg.ValidationSplitRatio = validationRatio
		effectiveCfg := ml.ApplyBuiltinModelPreset(cfg)
		candidate := ml.MLModelTuneCandidate{
			ModelType:       string(modelType),
			Label:           label,
			Base:            base,
			Family:          taxonomy.Family,
			FamilyLabel:     taxonomy.FamilyLabel,
			FeatureClass:    taxonomy.FeatureClass,
			FeatureProfiles: append([]string(nil), taxonomy.FeatureProfiles...),
			Recommended:     recommended,
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
		model, result := ml.GlobalTrainer.TrainWithConfig(store, cfg)
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
		validationSamples := ml.GlobalTrainer.LastValidationSamples()
		if candidate.ValidationCount == 0 {
			candidate.ValidationCount = len(validationSamples)
		}
		validationMetrics := ml.EvaluateAutoTuneTrainingSampleMetrics(validationSamples, model)
		candidate.AllowRecall = validationMetrics.AllowRecall
		candidate.BalancedAccuracy = validationMetrics.BalancedAccuracy
		candidate.AttackMetrics = ml.EvaluateAttackImpactMetrics(validationSamples, model)

		if req.TuneParams {
			paramMetric := metric
			// The inner grid tuner currently owns only conventional objectives.
			// For a security-first outer objective use balanced accuracy internally,
			// then re-rank the fully trained model by attack impact below.
			if ml.NormalizeAutoTuneMetric(paramMetric) == "" {
				paramMetric = "balancedAccuracy"
			}
			paramReq := ml.MLAutoTuneRequest{
				XAxis:                req.XAxis,
				YAxis:                req.YAxis,
				GridSize:             req.GridSize,
				Granularity:          req.Granularity,
				Metric:               paramMetric,
				ValidationSplitRatio: validationRatio,
				MinX:                 req.MinX,
				MaxX:                 req.MaxX,
				MinY:                 req.MinY,
				MaxY:                 req.MaxY,
			}
			paramResp, err := ml.GlobalTrainer.AutoTuneWithConfig(store, cfg, paramReq, nil)
			if err == nil && paramResp != nil && paramResp.Best != nil {
				candidate.ParamTune = paramResp
				cfg.NumTrees = paramResp.Best.NumTrees
				cfg.MaxDepth = paramResp.Best.MaxDepth
				cfg.MinSamplesLeaf = paramResp.Best.MinSamplesLeaf
				candidate.HyperParams["numTrees"] = cfg.NumTrees
				candidate.HyperParams["maxDepth"] = cfg.MaxDepth
				candidate.HyperParams["minSamplesLeaf"] = cfg.MinSamplesLeaf
				trainStart = time.Now()
				model, result = ml.GlobalTrainer.TrainWithConfig(store, cfg)
				candidate.TrainDuration += time.Since(trainStart).Seconds()
				if result.Error == "" {
					candidate.TrainAccuracy = result.TrainAccuracy
					candidate.ValidationAccuracy = result.ValidationAccuracy
					candidate.ValidationCount = result.ValidationSamples
					validationSamples = ml.GlobalTrainer.LastValidationSamples()
					validationMetrics = ml.EvaluateAutoTuneTrainingSampleMetrics(validationSamples, model)
					candidate.AllowRecall = validationMetrics.AllowRecall
					candidate.BalancedAccuracy = validationMetrics.BalancedAccuracy
					candidate.AttackMetrics = ml.EvaluateAttackImpactMetrics(validationSamples, model)
				} else {
					candidate.Error = result.Error
				}
			}
		}

		evalDuration, throughput, latencyMs, _ := ml.BenchmarkModelInference(model, benchmarkSamples)
		candidate.EvalDuration = evalDuration
		candidate.InferenceThroughput = throughput
		candidate.InferenceMsPerSample = latencyMs
		classificationMetrics := ml.AutoTuneClassificationMetrics{
			AllowRecall:      candidate.AllowRecall,
			BalancedAccuracy: candidate.BalancedAccuracy,
		}
		candidate.Score = ml.SecurityAutoTuneMetricScore(
			metric,
			candidate.ValidationAccuracy,
			candidate.InferenceThroughput,
			classificationMetrics,
			candidate.AttackMetrics,
		)
		candidates = append(candidates, candidate)
		if candidate.Error == "" && candidate.Score > bestScore {
			copyCandidate := candidate
			best = &copyCandidate
			bestModel = model
			bestScore = candidate.Score
		}
		ml.GlobalTrainer.Logf(
			"模型调优: %s [%s/%s] security=%.1f%% high-impact=%.1f%% destruction=%.1f%% catastrophic-miss=%.1f%% accuracy=%.1f%% 推理 %.0f/s",
			label,
			taxonomy.Family,
			taxonomy.FeatureClass,
			candidate.AttackMetrics.SecurityUtility*100,
			candidate.AttackMetrics.HighImpactRecall*100,
			candidate.AttackMetrics.DestructionRecall*100,
			candidate.AttackMetrics.CatastrophicMissRate*100,
			candidate.ValidationAccuracy*100,
			candidate.InferenceThroughput,
		)
		if progressCb != nil {
			progressCb(i+1, len(modelTypes), fmt.Sprintf("完成 %s (%d/%d)", label, i+1, len(modelTypes)))
		}
	}

	if isCanceled != nil && isCanceled() {
		return nil, errors.New("cancelled")
	}
	if best == nil {
		return &ml.MLModelTuneResponse{Metric: metric, SampleCount: len(labeled), TotalDuration: time.Since(start).Seconds(), Candidates: candidates}, errors.New("no model candidate trained successfully")
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

	return &ml.MLModelTuneResponse{
		Metric:          metric,
		SampleCount:     len(labeled),
		ValidationCount: best.ValidationCount,
		TotalDuration:   time.Since(start).Seconds(),
		Candidates:      candidates,
		Best:            best,
	}, nil
}

func applyModelTuneBest(best ml.MLModelTuneCandidate, model ml.Model, validationRatio float64) error {
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
	ml.PublishMLRuntimeModel(model, settings.MLConfig.ModelType)
	modelPath := settings.MLConfig.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}
	if err := model.Serialize(modelPath); err != nil {
		return fmt.Errorf("model selected but failed to save: %w", err)
	}
	return nil
}
