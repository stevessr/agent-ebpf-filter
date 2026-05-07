package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"agent-ebpf-filter/cuda"
	"github.com/gin-gonic/gin"
)

type MLAutoTuneRequest struct {
	XAxis                string  `json:"xAxis"`
	YAxis                string  `json:"yAxis"`
	GridSize             int     `json:"gridSize"`
	Granularity          float64 `json:"granularity"`
	Metric               string  `json:"metric"`
	ValidationSplitRatio float64 `json:"validationSplitRatio"`
	MinX                 *int    `json:"minX,omitempty"`
	MaxX                 *int    `json:"maxX,omitempty"`
	MinY                 *int    `json:"minY,omitempty"`
	MaxY                 *int    `json:"maxY,omitempty"`
}

type MLAutoTuneCell struct {
	XIndex               int     `json:"xIndex"`
	YIndex               int     `json:"yIndex"`
	XValue               int     `json:"xValue"`
	YValue               int     `json:"yValue"`
	NumTrees             int     `json:"numTrees"`
	MaxDepth             int     `json:"maxDepth"`
	MinSamplesLeaf       int     `json:"minSamplesLeaf"`
	TrainAccuracy        float64 `json:"trainAccuracy"`
	ValidationAccuracy   float64 `json:"validationAccuracy"`
	InferenceThroughput  float64 `json:"inferenceThroughput"`
	InferenceMsPerSample float64 `json:"inferenceMsPerSample"`
	TrainDuration        float64 `json:"trainDuration"`
	EvalDuration         float64 `json:"evalDuration"`
	Score                float64 `json:"score"`
}

type MLAutoTuneResponse struct {
	XAxis           string           `json:"xAxis"`
	YAxis           string           `json:"yAxis"`
	Metric          string           `json:"metric"`
	Granularity     float64          `json:"granularity"`
	GridSize        int              `json:"gridSize"`
	XValues         []int            `json:"xValues"`
	YValues         []int            `json:"yValues"`
	SampleCount     int              `json:"sampleCount"`
	ValidationCount int              `json:"validationCount"`
	TotalDuration   float64          `json:"totalDuration"`
	Cells           []MLAutoTuneCell `json:"cells"`
	Best            *MLAutoTuneCell  `json:"best,omitempty"`
}

type MLAutoTuneState struct {
	JobID      string              `json:"jobId"`
	Running    bool                `json:"running"`
	Progress   float64             `json:"progress"`
	Completed  int                 `json:"completed"`
	Total      int                 `json:"total"`
	Message    string              `json:"message,omitempty"`
	Error      string              `json:"error,omitempty"`
	StartedAt  string              `json:"startedAt,omitempty"`
	FinishedAt string              `json:"finishedAt,omitempty"`
	Result     *MLAutoTuneResponse `json:"result,omitempty"`
}

type autoTuneRuntime struct {
	mu     sync.RWMutex
	state  MLAutoTuneState
	result *MLAutoTuneResponse
}

var globalAutoTuneState = &autoTuneRuntime{}

func (s *autoTuneRuntime) begin(jobID string, total int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = MLAutoTuneState{
		JobID:     jobID,
		Running:   true,
		Progress:  0,
		Completed: 0,
		Total:     total,
		Message:   message,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	s.result = nil
}

func (s *autoTuneRuntime) tryBegin(jobID string, total int, message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Running {
		return false
	}
	s.state = MLAutoTuneState{
		JobID:     jobID,
		Running:   true,
		Progress:  0,
		Completed: 0,
		Total:     total,
		Message:   message,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	s.result = nil
	return true
}

func (s *autoTuneRuntime) update(jobID string, completed, total int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobID != "" && s.state.JobID != "" && s.state.JobID != jobID {
		return
	}
	s.state.JobID = jobID
	s.state.Running = true
	s.state.Completed = completed
	s.state.Total = total
	if total > 0 {
		s.state.Progress = math.Max(0, math.Min(1, float64(completed)/float64(total)))
	} else {
		s.state.Progress = 0
	}
	s.state.Message = message
	s.state.Error = ""
}

func (s *autoTuneRuntime) setError(jobID string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Running = false
	s.state.Error = errMsg
	s.state.Message = "调优失败"
}

func autoTuneBestScore(resp *MLAutoTuneResponse) float64 {
	if resp == nil || resp.Best == nil {
		return 0
	}
	return resp.Best.Score
}

func (s *autoTuneRuntime) finish(jobID string, result *MLAutoTuneResponse, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if jobID != "" && s.state.JobID != "" && s.state.JobID != jobID {
		return
	}
	s.state.Running = false
	s.state.FinishedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		s.state.Error = err.Error()
		s.state.Message = "自动调参失败"
		s.result = nil
		s.state.Result = nil
		return
	}
	s.state.Progress = 1
	if result != nil {
		s.result = result
		s.state.Result = result
	}
	s.state.Error = ""
	if s.state.Message == "" {
		s.state.Message = "自动调参完成"
	}
}

func (s *autoTuneRuntime) snapshot() MLAutoTuneState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.state
	if s.result != nil {
		state.Result = s.result
	}
	return state
}

func handleMLTunePost(c *gin.Context) {
	if !mlEnabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	var req MLAutoTuneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if globalAutoTuneState.snapshot().Running {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}
	if globalTrainer.isRunning {
		c.JSON(409, gin.H{"error": "training already in progress"})
		return
	}

	jobID := fmt.Sprintf("tune-%d", time.Now().UnixNano())
	if !globalAutoTuneState.tryBegin(jobID, 0, "自动调参任务已接收") {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}

	go func(jobID string, req MLAutoTuneRequest) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ML] Auto-tune panic: %v", r)
				globalAutoTuneState.setError(jobID, fmt.Sprintf("panic: %v", r))
			}
		}()
		log.Printf("[ML] Auto-tune started: jobID=%s, model=%s, grid=%dx%d, x=%s, y=%s",
			jobID, mlConfig.ModelType, req.GridSize, req.GridSize, req.XAxis, req.YAxis)
		resp, err := globalTrainer.AutoTune(globalTrainingStore, req, func(completed, total int, message string) {
			if completed%5 == 0 || completed == total {
				log.Printf("[ML] Auto-tune progress: %d/%d — %s", completed, total, message)
			}
			globalAutoTuneState.update(jobID, completed, total, message)
		})
		if err != nil {
			log.Printf("[ML] Auto-tune error: %v", err)
		} else {
			log.Printf("[ML] Auto-tune done: %d cells, best score=%.4f", len(resp.Cells), autoTuneBestScore(resp))
		}
		globalAutoTuneState.finish(jobID, resp, err)
	}(jobID, req)

	c.JSON(202, gin.H{
		"jobId":   jobID,
		"started": true,
		"message": "自动调参已开始",
	})
}

func (t *ModelTrainer) AutoTune(store *TrainingDataStore, req MLAutoTuneRequest, progressCb func(completed, total int, message string)) (*MLAutoTuneResponse, error) {
	select {
	case t.mu <- struct{}{}:
		defer func() { <-t.mu }()
	default:
		return nil, errors.New("training already in progress")
	}

	xAxis := normalizeAutoTuneAxis(req.XAxis)
	yAxis := normalizeAutoTuneAxis(req.YAxis)
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
	metric := normalizeAutoTuneMetric(req.Metric)
	if metric == "" {
		metric = "validationAccuracy"
	}

	effectiveCfg := applyBuiltinModelPreset(mlConfig)
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

	trainSet, validationSet, _, validationRaw, err := prepareAutoTuneSplit(labeled, validationRatio)
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
	requestedModelType := mlConfig.ModelType
	if requestedModelType == "" {
		requestedModelType = ModelRandomForest
	}
	mt := effectiveCfg.ModelType
	if mt == "" {
		mt = ModelRandomForest
	}
	globalTrainer.logf("══════ 自动调参开始 ══════")
	globalTrainer.logf("模型类型: %s, 方阵: %dx%d, 轴: %s×%s", modelName(requestedModelType), gridSize, gridSize, xAxis, yAxis)
	if cuda.IsAvailable() {
		globalTrainer.logf("CUDA 加速已启用: %s", cuda.DeviceInfo())
	} else {
		globalTrainer.logf("CPU 模式（无 CUDA 设备）")
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
			if globalTrainer.IsCancelled() {
				globalTrainer.logf("自动调参已中止: %d/%d 格完成", done, totalCombos)
				return nil, errors.New("cancelled")
			}
			numTrees, maxDepth, minLeaf := baseNumTrees, baseMaxDepth, baseMinSamplesLeaf
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(xAxis, xValue, numTrees, maxDepth, minLeaf)
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(yAxis, yValue, numTrees, maxDepth, minLeaf)

			cellStart := time.Now()
			var trainAccuracy, validationAccuracy, throughput, msPerSample float64
			var evalDuration time.Duration
			var evalStart time.Time

			switch mt {
			case ModelRandomForest:
				if len(labeled) < minLeaf*10 {
					done++
					if progressCb != nil {
						progressCb(done, totalCombos, fmt.Sprintf("跳过 %d/%d (RF 样本不足)", done, totalCombos))
					}
					continue
				}
				seed := int64((yi+1)*100000 + (xi+1)*1000 + numTrees*31 + maxDepth*17 + minLeaf*13)
				forest := buildAutoTuneForest(trainSet, numTrees, maxDepth, minLeaf, seed)
				trainAccuracy = evaluateForest(forest, trainSet)
				evalStart = time.Now()
				validationAccuracy = evaluateForest(forest, validationSet)
				evalDuration = time.Since(evalStart)

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

			case ModelLogisticRegression:
				lr := float64(numTrees) / 1000.0
				if lr < 0.001 {
					lr = 0.01
				}
				reg := "l2"
				if maxDepth == 12 {
					reg = "l1"
				} else if maxDepth == 4 {
					reg = "none"
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
				trainAccuracy = evalModelSamples(nb, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalModelSamples(nb, validationSet)
				evalDuration = time.Since(evalStart)

			case ModelExtraTrees:
				seed := int64((yi+1)*100000 + (xi+1)*1000 + numTrees*31 + maxDepth*17 + minLeaf*13)
				et := buildExtraTrees(trainSet, numTrees, maxDepth, minLeaf, seed)
				trainAccuracy = evalModelSamples(&ExtraTreesModel{Forest: et}, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalModelSamples(&ExtraTreesModel{Forest: et}, validationSet)
				evalDuration = time.Since(evalStart)

			case ModelAdaBoost:
				trainS, trainL := extractTrainData(trainSet)
				ab := trainAdaBoostFromData(trainS, trainL, numTrees)
				trainAccuracy = evalModelSamples(ab, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalModelSamples(ab, validationSet)
				evalDuration = time.Since(evalStart)

			case ModelEnsemble:
				tmpStore := newTrainingDataStore(len(trainSet))
				for i := range trainSet {
					tmpStore.samples[i] = TrainingSample{
						Features: trainSet[i].features,
						Label:    trainSet[i].label,
					}
				}
				tmpStore.nextWrite = len(trainSet)
				ens := buildEnsembleFromStore(tmpStore)
				if ens == nil {
					done++
					if progressCb != nil {
						progressCb(done, totalCombos, fmt.Sprintf("跳过 %d/%d (ensemble 样本不足)", done, totalCombos))
					}
					continue
				}
				trainAccuracy = evalModelSamples(ens, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalModelSamples(ens, validationSet)
				evalDuration = time.Since(evalStart)

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
				trainAccuracy = evalModelSamples(model, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalModelSamples(model, validationSet)
				evalDuration = time.Since(evalStart)

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
					ridgeFit(W, 4, labeled[:len(trainSet)], float64(numTrees)/100.0+0.1)
				} else {
					labeledSubset := make([]TrainingSample, len(trainSet))
					for i, s := range trainSet {
						labeledSubset[i] = TrainingSample{Features: s.features, Label: s.label}
					}
					trainSGD(W, 4, labeledSubset, lr, maxIter, C, loss, nil, globalTrainer)
				}
				trainAccuracy = evalLinearModel(W, 4, trainSet)
				evalStart = time.Now()
				validationAccuracy = evalLinearModel(W, 4, validationSet)
				evalDuration = time.Since(evalStart)
			}

			cellDuration := time.Since(cellStart)
			if len(validationSet) > 0 && evalDuration > 0 {
				throughput = float64(len(validationSet)) / evalDuration.Seconds()
				msPerSample = evalDuration.Seconds() * 1000 / float64(len(validationSet))
			}

			score := validationAccuracy
			if metric == "inferenceThroughput" {
				score = throughput
			}

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
			if done%3 == 0 || done == totalCombos {
				globalTrainer.logf("%s 调优: %d/%d 格 (准确率 %.1f%%)", modelName(requestedModelType), done, totalCombos, validationAccuracy*100)
			}
			if progressCb != nil {
				progressCb(done, totalCombos, fmt.Sprintf("%s 评估 %d/%d%s", modelName(requestedModelType), done, totalCombos, cudaLog))
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
		Cells:           cells,
		Best:            best,
	}, nil
}

func prepareAutoTuneSplit(labeled []TrainingSample, validationRatio float64) ([]trainSample, []trainSample, []TrainingSample, []TrainingSample, error) {
	if len(labeled) < 2 {
		return nil, nil, nil, nil, errors.New("need at least 2 labeled samples for tuning")
	}
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}

	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	shuffledRaw := append([]TrainingSample(nil), labeled...)
	rand.Shuffle(len(samples), func(i, j int) {
		samples[i], samples[j] = samples[j], samples[i]
		shuffledRaw[i], shuffledRaw[j] = shuffledRaw[j], shuffledRaw[i]
	})

	validationCount := int(math.Round(float64(len(samples)) * validationRatio))
	if validationCount < 1 {
		validationCount = 1
	}
	if validationCount >= len(samples) {
		validationCount = len(samples) - 1
	}

	trainCount := len(samples) - validationCount
	trainSet := append([]trainSample(nil), samples[:trainCount]...)
	validationSet := append([]trainSample(nil), samples[trainCount:]...)
	trainRaw := append([]TrainingSample(nil), shuffledRaw[:trainCount]...)
	validationRaw := append([]TrainingSample(nil), shuffledRaw[trainCount:]...)
	return trainSet, validationSet, trainRaw, validationRaw, nil
}

func buildAutoTuneForest(trainSet []trainSample, numTrees, maxDepth, minSamplesLeaf int, seed int64) *DecisionForest {
	if numTrees < 1 {
		numTrees = 1
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	if minSamplesLeaf < 1 {
		minSamplesLeaf = 1
	}

	rng := rand.New(rand.NewSource(seed))
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	featureSampleCount := int(math.Sqrt(float64(FeatureDim)))
	if featureSampleCount < 1 {
		featureSampleCount = 1
	}

	for ti := 0; ti < numTrees; ti++ {
		bootstrap := make([]trainSample, len(trainSet))
		for i := range bootstrap {
			bootstrap[i] = trainSet[rng.Intn(len(trainSet))]
		}
		nodes := buildAutoTuneTree(bootstrap, 0, maxDepth, minSamplesLeaf, featureSampleCount, rng)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
	}

	forest.IsTrained = true
	return forest
}

func buildAutoTuneTree(samples []trainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) []DecisionNode {
	if depth >= maxDepth || len(samples) < minSamplesLeaf*2 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	allSame := true
	firstLabel := samples[0].label
	for _, s := range samples[1:] {
		if s.label != firstLabel {
			allSame = false
			break
		}
	}
	if allSame {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: float32(firstLabel)}}
	}

	best := findAutoTuneBestSplit(samples, featureSampleCount, rng)
	if best.giniGain <= 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	var leftSamples, rightSamples []trainSample
	for _, s := range samples {
		if s.features[best.featureIdx] < best.threshold {
			leftSamples = append(leftSamples, s)
		} else {
			rightSamples = append(rightSamples, s)
		}
	}
	if len(leftSamples) == 0 || len(rightSamples) == 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	leftNodes := buildAutoTuneTree(leftSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)
	rightNodes := buildAutoTuneTree(rightSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)

	// Build flat array: [root] + [left subtree] + [right subtree]
	// Rebase child pointers from subtree-relative (0-based) to absolute positions.
	leftOffset := 1
	rightOffset := 1 + len(leftNodes)

	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    int16(leftOffset),
		RightChild:   int16(rightOffset),
		LeafValue:    0,
	}
	nodes := []DecisionNode{root}

	// Rebase left subtree indices
	for i := range leftNodes {
		n := &leftNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(leftOffset)
			n.RightChild += int16(leftOffset)
		}
	}
	nodes = append(nodes, leftNodes...)

	// Rebase right subtree indices
	for i := range rightNodes {
		n := &rightNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(rightOffset)
			n.RightChild += int16(rightOffset)
		}
	}
	nodes = append(nodes, rightNodes...)
	return nodes
}

func findAutoTuneBestSplit(samples []trainSample, featureSampleCount int, rng *rand.Rand) splitPoint {
	best := splitPoint{giniGain: -1}
	parentGini := giniImpurity(samples)

	features := make([]int, FeatureDim)
	for i := range features {
		features[i] = i
	}
	rng.Shuffle(len(features), func(i, j int) { features[i], features[j] = features[j], features[i] })
	if featureSampleCount > len(features) {
		featureSampleCount = len(features)
	}
	selectedFeatures := features[:featureSampleCount]

	for _, fi := range selectedFeatures {
		sorted := append([]trainSample(nil), samples...)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].features[fi] < sorted[j].features[fi]
		})

		for i := 1; i < len(sorted); i++ {
			if sorted[i].features[fi] == sorted[i-1].features[fi] {
				continue
			}
			threshold := (sorted[i].features[fi] + sorted[i-1].features[fi]) / 2.0
			leftSamples := sorted[:i]
			rightSamples := sorted[i:]
			if len(leftSamples) < 1 || len(rightSamples) < 1 {
				continue
			}

			leftWeight := float64(len(leftSamples)) / float64(len(sorted))
			gain := parentGini - leftWeight*giniImpurity(leftSamples) - (1-leftWeight)*giniImpurity(rightSamples)
			if gain > best.giniGain {
				best = splitPoint{
					featureIdx: fi,
					threshold:  threshold,
					giniGain:   gain,
				}
			}
		}
	}

	return best
}
