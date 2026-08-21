package ml

import (
	"agent-ebpf-filter/cuda"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// AutoTuneWithConfig runs a grid search against an immutable caller-provided
// configuration so candidate jobs never have to mutate process-wide ML state.
func (t *ModelTrainer) AutoTuneWithConfig(store *TrainingDataStore, cfg MLConfig, req MLAutoTuneRequest, progressCb func(completed, total int, message string)) (*MLAutoTuneResponse, error) {
	select {
	case t.mu <- struct{}{}:
		defer func() { <-t.mu }()
	default:
		return nil, errors.New("training already in progress")
	}
	t.BeginTraining()
	defer t.finishTraining()

	xAxis := NormalizeAutoTuneAxis(req.XAxis)
	yAxis := NormalizeAutoTuneAxis(req.YAxis)
	if xAxis == "" {
		xAxis = "numTrees"
	}
	if yAxis == "" {
		yAxis = "maxDepth"
	}
	if xAxis == yAxis {
		return nil, fmt.Errorf("xAxis and yAxis must be different")
	}

	gridSize := normalizeAutoTuneGridSize(req.GridSize)
	granularity := normalizeAutoTuneGranularity(req.Granularity)
	metric := NormalizeAutoTuneMetric(req.Metric)
	if metric == "" {
		metric = "validationAccuracy"
	}

	effectiveCfg := ApplyBuiltinModelPreset(cfg)
	baseNumTrees := effectiveCfg.NumTrees
	if baseNumTrees <= 0 {
		baseNumTrees = 31
	}
	baseMaxDepth := effectiveCfg.MaxDepth
	if baseMaxDepth <= 0 {
		baseMaxDepth = 8
	}
	baseMinSamplesLeaf := effectiveCfg.MinSamplesLeaf
	if baseMinSamplesLeaf <= 0 {
		baseMinSamplesLeaf = 5
	}

	validationRatio := req.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = effectiveCfg.ValidationSplitRatio
	}
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}

	labeled := store.LabeledSamples()
	if len(labeled) < baseMinSamplesLeaf*10 {
		msg := fmt.Sprintf("Insufficient labeled samples: need >=%d, have %d", baseMinSamplesLeaf*10, len(labeled))
		return nil, errors.New(msg)
	}

	trainSet, validationSet, _, validationRaw, err := PrepareAutoTuneSplit(labeled, validationRatio)
	if err != nil {
		return nil, err
	}

	xValues := autoTuneAxisValuesWithRange(xAxis, gridSize, granularity, baseNumTrees, baseMaxDepth, baseMinSamplesLeaf, req.MinX, req.MaxX)
	yValues := autoTuneAxisValuesWithRange(yAxis, gridSize, granularity, baseNumTrees, baseMaxDepth, baseMinSamplesLeaf, req.MinY, req.MaxY)
	maxRequiredLeaf := autoTuneMaxInt(baseMinSamplesLeaf, maxAxisValue(xAxis, xValues, yAxis, yValues, "minSamplesLeaf"))
	if len(labeled) < maxRequiredLeaf*10 {
		msg := fmt.Sprintf("Insufficient labeled samples for tuning: need >=%d, have %d", maxRequiredLeaf*10, len(labeled))
		return nil, errors.New(msg)
	}

	totalCombos := len(xValues) * len(yValues)
	if totalCombos <= 0 {
		return nil, errors.New("no valid parameter combinations found for tuning")
	}
	requestedModelType := cfg.ModelType
	if requestedModelType == "" {
		requestedModelType = ModelRandomForest
	}
	mt := effectiveCfg.ModelType
	if mt == "" {
		mt = ModelRandomForest
	}
	t.Logf("══════ 自动调参开始 ══════")
	t.Logf("模型类型: %s, 方阵: %dx%d, 轴: %s×%s", ModelName(requestedModelType), gridSize, gridSize, xAxis, yAxis)
	if cuda.IsAvailable() {
		t.Logf("CUDA 加速已启用: %s", cuda.DeviceInfo())
	} else {
		t.Logf("CPU 模式（无 CUDA 设备）")
	}

	if progressCb != nil {
		startMsg := "开始评估自动调参方阵"
		if cuda.IsAvailable() {
			startMsg += fmt.Sprintf(" [CUDA: %s]", cuda.DeviceInfo())
		}
		progressCb(0, totalCombos, startMsg)
	}

	start := time.Now()
	cells := make([]MLAutoTuneCell, 0, totalCombos)
	var best *MLAutoTuneCell
	bestScore := math.Inf(-1)

	cudaLog := ""
	if cuda.IsAvailable() {
		cudaLog = fmt.Sprintf(" [CUDA: %s]", cuda.DeviceInfo())
	}

	done := 0
	for yi, yValue := range yValues {
		for xi, xValue := range xValues {
			if t.IsCancelled() {
				t.Logf("自动调参已中止：%d/%d 格完成", done, totalCombos)
				return nil, errors.New("cancelled")
			}
			numTrees, maxDepth, minLeaf := baseNumTrees, baseMaxDepth, baseMinSamplesLeaf
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(xAxis, xValue, numTrees, maxDepth, minLeaf)
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(yAxis, yValue, numTrees, maxDepth, minLeaf)

			cellStart := time.Now()
			var trainAccuracy, validationAccuracy, allowRecall, balancedAccuracy, throughput, msPerSample float64
			var evalDuration time.Duration
			var evalStart time.Time
			var predictValidation func([FeatureDim]float64) int32

			switch mt {
			case ModelRandomForest:
				if len(labeled) < minLeaf*10 {
					done++
					t.setTrainingProgress(float64(done) / float64(totalCombos))
					if progressCb != nil {
						progressCb(done, totalCombos, fmt.Sprintf("跳过 %d/%d (RF 样本不足)", done, totalCombos))
					}
					continue
				}
				seed := int64((yi+1)*100000 + (xi+1)*1000 + numTrees*31 + maxDepth*17 + minLeaf*13)
				forest := BuildAutoTuneForest(trainSet, numTrees, maxDepth, minLeaf, seed)
				trainAccuracy = EvaluateForest(forest, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvaluateForest(forest, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return forest.Predict(features).Action
				}

			case ModelKNN:
				k := numTrees
				if k < 1 {
					k = 5
				}
				if k > len(trainSet) {
					k = len(trainSet)
				}
				model := NewKNNModel(k, "euclidean", "uniform")
				model.NumClasses = 4
				model.Samples = make([][FeatureDim]float64, len(trainSet))
				model.Labels = make([]int32, len(trainSet))
				for i, s := range trainSet {
					model.Samples[i] = s.features
					model.Labels[i] = s.label
				}
				trainAccuracy = evalKNNModel(model, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalKNNModel(model, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return model.Predict(features).Action
				}

			case ModelLogisticRegression:
				lr := float64(numTrees) / 1000.0
				if lr < 0.001 {
					lr = 0.01
				}
				var reg string
				switch maxDepth {
				case 12:
					reg = "l1"
				case 4:
					reg = "none"
				default:
					reg = "l2"
				}
				maxIter := minLeaf
				if maxIter < 100 {
					maxIter = 1000
				}
				trainS, trainL := extractTrainData(trainSet)
				lrModel := NewLogisticModel(lr, reg, maxIter)
				lrModel.NumClasses = 4
				lrModel.Train(trainS, trainL)
				trainAccuracy = evalLogisticModel(lrModel, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalLogisticModel(lrModel, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return lrModel.Predict(features).Action
				}

			case ModelNaiveBayes:
				nb := NewNaiveBayes()
				nb.Means = make([][FeatureDim]float64, 4)
				nb.Vars = make([][FeatureDim]float64, 4)
				nb.Priors = make([]float64, 4)
				counts := make([]int, 4)
				for _, s := range trainSet {
					c := s.label
					counts[c]++
					for d := 0; d < FeatureDim; d++ {
						nb.Means[c][d] += s.features[d]
					}
				}
				for c := 0; c < 4; c++ {
					nb.Priors[c] = float64(counts[c]) / float64(len(trainSet))
					if counts[c] > 0 {
						for d := 0; d < FeatureDim; d++ {
							nb.Means[c][d] /= float64(counts[c])
						}
					}
				}
				for _, s := range trainSet {
					c := s.label
					for d := 0; d < FeatureDim; d++ {
						diff := s.features[d] - nb.Means[c][d]
						nb.Vars[c][d] += diff * diff
					}
				}
				for c := 0; c < 4; c++ {
					if counts[c] > 1 {
						for d := 0; d < FeatureDim; d++ {
							nb.Vars[c][d] /= float64(counts[c] - 1)
						}
					}
				}
				trainAccuracy = EvalModelSamples(nb, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(nb, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return nb.Predict(features).Action
				}

			case ModelExtraTrees:
				seed := int64((yi+1)*100000 + (xi+1)*1000 + numTrees*31 + maxDepth*17 + minLeaf*13)
				et := BuildExtraTrees(trainSet, numTrees, maxDepth, minLeaf, seed)
				trainAccuracy = EvalModelSamples(&ExtraTreesModel{Forest: et}, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(&ExtraTreesModel{Forest: et}, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return et.Predict(features).Action
				}

			case ModelAdaBoost:
				trainS, trainL := extractTrainData(trainSet)
				ab := trainAdaBoostFromData(trainS, trainL, numTrees)
				trainAccuracy = EvalModelSamples(ab, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(ab, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return ab.Predict(features).Action
				}

			case ModelEnsemble:
				tmpStore := NewTrainingDataStore(len(trainSet))
				for i := range trainSet {
					tmpStore.samples[i] = TrainingSample{
						Features: trainSet[i].features,
						Label:    trainSet[i].label,
					}
				}
				tmpStore.nextWrite = len(trainSet)
				ens := BuildEnsembleFromStore(tmpStore)
				if ens == nil {
					done++
					t.setTrainingProgress(float64(done) / float64(totalCombos))
					if progressCb != nil {
						progressCb(done, totalCombos, fmt.Sprintf("跳过 %d/%d (ensemble 样本不足)", done, totalCombos))
					}
					continue
				}
				trainAccuracy = EvalModelSamples(ens, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(ens, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return ens.Predict(features).Action
				}

			case ModelGANTransformer:
				modelCfg := DefaultMLConfig()
				modelCfg.ModelType = ModelGANTransformer
				modelCfg.NumTrees = numTrees
				modelCfg.MaxDepth = maxDepth
				modelCfg.MinSamplesLeaf = minLeaf
				modelCfg.ValidationSplitRatio = validationRatio
				model := NewGANTransformerModel(numTrees, maxDepth*4, minLeaf*2)
				model.Train(ToTrainingSamples(trainSet), modelCfg)
				trainAccuracy = EvalModelSamples(model, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(model, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return model.Predict(features).Action
				}

			case ModelNearestCentroid:
				metric := "euclidean"
				switch {
				case numTrees <= 24:
					metric = "cosine"
				case numTrees >= 36:
					metric = "manhattan"
				}
				balanced := maxDepth >= 8
				model := NewNearestCentroid(metric, balanced)
				model.Classes = 4
				model.Centroids = make([][FeatureDim]float64, model.Classes)
				model.Priors = make([]float64, model.Classes)
				counts := make([]int, model.Classes)
				for _, s := range trainSet {
					if s.label < 0 || int(s.label) >= model.Classes {
						continue
					}
					c := int(s.label)
					counts[c]++
					for d := 0; d < FeatureDim; d++ {
						model.Centroids[c][d] += s.features[d]
					}
				}
				nonEmpty := 0
				for _, count := range counts {
					if count > 0 {
						nonEmpty++
					}
				}
				for c := 0; c < model.Classes; c++ {
					if counts[c] > 0 {
						for d := 0; d < FeatureDim; d++ {
							model.Centroids[c][d] /= float64(counts[c])
						}
					}
					if balanced && nonEmpty > 0 && counts[c] > 0 {
						model.Priors[c] = 1.0 / float64(nonEmpty)
					} else if len(trainSet) > 0 {
						model.Priors[c] = float64(counts[c]) / float64(len(trainSet))
					}
				}
				trainAccuracy = EvalModelSamples(model, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalModelSamples(model, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					return model.Predict(features).Action
				}

			case ModelSVM, ModelPerceptron, ModelPassiveAggressive, ModelRidge:
				W := make([][FeatureDim + 1]float64, 4)
				for c := range W {
					for d := range W[c] {
						W[c][d] = (rand.Float64() - 0.5) * 0.01
					}
				}
				lr := float64(numTrees) / 1000.0
				if lr < 0.001 {
					lr = 0.01
				}
				C := float64(numTrees) / 10.0
				if C < 0.1 {
					C = 1.0
				}
				maxIter := minLeaf
				if maxIter < 100 {
					maxIter = 1000
				}
				loss := "hinge"
				if mt == ModelPerceptron {
					loss = "perceptron"
				}
				if mt == ModelPassiveAggressive {
					loss = "pa"
				}
				if mt == ModelRidge {
					RidgeFit(W, 4, labeled[:len(trainSet)], float64(numTrees)/100.0+0.1)
				} else {
					labeledSubset := make([]TrainingSample, len(trainSet))
					for i, s := range trainSet {
						labeledSubset[i] = TrainingSample{Features: s.features, Label: s.label}
					}
					TrainSGD(W, 4, labeledSubset, lr, maxIter, C, loss, nil, t)
				}
				trainAccuracy = EvalLinearModel(W, 4, trainSet)
				evalStart = time.Now()
				validationAccuracy = EvalLinearModel(W, 4, validationSet)
				evalDuration = time.Since(evalStart)
				predictValidation = func(features [FeatureDim]float64) int32 {
					bestClass := int32(0)
					bestScore := math.Inf(-1)
					for c := 0; c < 4; c++ {
						score := W[c][FeatureDim]
						for d := 0; d < FeatureDim; d++ {
							score += W[c][d] * features[d]
						}
						if score > bestScore {
							bestScore = score
							bestClass = int32(c)
						}
					}
					return bestClass
				}
			}

			cellDuration := time.Since(cellStart)
			if len(validationSet) > 0 && evalDuration > 0 {
				throughput = float64(len(validationSet)) / evalDuration.Seconds()
				msPerSample = evalDuration.Seconds() * 1000 / float64(len(validationSet))
			}
			metrics := evaluateAutoTuneClassificationMetrics(validationSet, predictValidation)
			allowRecall = metrics.AllowRecall
			balancedAccuracy = metrics.BalancedAccuracy

			score := AutoTuneMetricScore(metric, validationAccuracy, throughput, metrics)

			cell := MLAutoTuneCell{
				XIndex:               xi,
				YIndex:               yi,
				XValue:               xValue,
				YValue:               yValue,
				NumTrees:             numTrees,
				MaxDepth:             maxDepth,
				MinSamplesLeaf:       minLeaf,
				TrainAccuracy:        trainAccuracy,
				ValidationAccuracy:   validationAccuracy,
				AllowRecall:          allowRecall,
				BalancedAccuracy:     balancedAccuracy,
				InferenceThroughput:  throughput,
				InferenceMsPerSample: msPerSample,
				TrainDuration:        cellDuration.Seconds(),
				EvalDuration:         evalDuration.Seconds(),
				Score:                score,
			}
			cells = append(cells, cell)
			if score > bestScore {
				copyCell := cell
				best = &copyCell
				bestScore = score
			}

			done++
			t.setTrainingProgress(float64(done) / float64(totalCombos))
			if done%3 == 0 || done == totalCombos {
				t.Logf("%s 调优: %d/%d 格 (准确率 %.1f%%)", ModelName(requestedModelType), done, totalCombos, validationAccuracy*100)
			}
			if progressCb != nil {
				progressCb(done, totalCombos, fmt.Sprintf("%s 评估 %d/%d%s", ModelName(requestedModelType), done, totalCombos, cudaLog))
			}
		}
	}

	if len(cells) == 0 {
		return nil, errors.New("no valid parameter combinations found for tuning")
	}

	if progressCb != nil {
		progressCb(totalCombos, totalCombos, "自动调参完成")
	}

	return &MLAutoTuneResponse{
		XAxis:           xAxis,
		YAxis:           yAxis,
		Metric:          metric,
		Granularity:     granularity,
		GridSize:        gridSize,
		XValues:         xValues,
		YValues:         yValues,
		SampleCount:     len(labeled),
		ValidationCount: len(validationRaw),
		TotalDuration:   time.Since(start).Seconds(),
		Normalization:   SummarizeFeatureNormalization(labeled),
		Cells:           cells,
		Best:            best,
	}, nil
}
