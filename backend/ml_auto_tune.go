package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type MLAutoTuneRequest struct {
	XAxis                string  `json:"xAxis"`
	YAxis                string  `json:"yAxis"`
	GridSize             int     `json:"gridSize"`
	Granularity          float64 `json:"granularity"`
	Metric               string  `json:"metric"`
	ValidationSplitRatio float64 `json:"validationSplitRatio"`
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
		resp, err := globalTrainer.AutoTune(globalTrainingStore, req, func(completed, total int, message string) {
			globalAutoTuneState.update(jobID, completed, total, message)
		})
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

	baseNumTrees := mlConfig.NumTrees
	if baseNumTrees <= 0 {
		baseNumTrees = 31
	}
	baseMaxDepth := mlConfig.MaxDepth
	if baseMaxDepth <= 0 {
		baseMaxDepth = 8
	}
	baseMinSamplesLeaf := mlConfig.MinSamplesLeaf
	if baseMinSamplesLeaf <= 0 {
		baseMinSamplesLeaf = 5
	}

	validationRatio := req.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = mlConfig.ValidationSplitRatio
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

	xValues := autoTuneAxisValues(xAxis, gridSize, granularity, baseNumTrees, baseMaxDepth, baseMinSamplesLeaf)
	yValues := autoTuneAxisValues(yAxis, gridSize, granularity, baseNumTrees, baseMaxDepth, baseMinSamplesLeaf)
	maxRequiredLeaf := autoTuneMaxInt(baseMinSamplesLeaf, maxAxisValue(xAxis, xValues, yAxis, yValues, "minSamplesLeaf"))
	if len(labeled) < maxRequiredLeaf*10 {
		msg := fmt.Sprintf("Insufficient labeled samples for tuning: need >=%d, have %d", maxRequiredLeaf*10, len(labeled))
		return nil, errors.New(msg)
	}

	totalCombos := len(xValues) * len(yValues)
	if totalCombos <= 0 {
		return nil, errors.New("no valid parameter combinations found for tuning")
	}
	if progressCb != nil {
		progressCb(0, totalCombos, "开始评估自动调参方阵")
	}

	start := time.Now()
	cells := make([]MLAutoTuneCell, 0, totalCombos)
	var best *MLAutoTuneCell
	bestScore := math.Inf(-1)

	done := 0
	for yi, yValue := range yValues {
		for xi, xValue := range xValues {
			numTrees, maxDepth, minLeaf := baseNumTrees, baseMaxDepth, baseMinSamplesLeaf
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(xAxis, xValue, numTrees, maxDepth, minLeaf)
			numTrees, maxDepth, minLeaf = setAutoTuneAxisValue(yAxis, yValue, numTrees, maxDepth, minLeaf)

			// Ensure the candidate respects the same minimum requirement as normal training.
			if len(labeled) < minLeaf*10 {
				done++
				if progressCb != nil {
					progressCb(done, totalCombos, fmt.Sprintf("跳过 %d/%d 个组合（样本不足）", done, totalCombos))
				}
				continue
			}

			seed := int64((yi+1)*100000 + (xi+1)*1000 + numTrees*31 + maxDepth*17 + minLeaf*13)
			cellStart := time.Now()
			forest := buildAutoTuneForest(trainSet, numTrees, maxDepth, minLeaf, seed)
			trainAccuracy := evaluateForest(forest, trainSet)
			evalStart := time.Now()
			validationAccuracy := evaluateForest(forest, validationSet)
			evalDuration := time.Since(evalStart)
			cellDuration := time.Since(cellStart)

			throughput := 0.0
			msPerSample := 0.0
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
			if progressCb != nil {
				progressCb(done, totalCombos, fmt.Sprintf("评估 %d/%d 个组合", done, totalCombos))
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

	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    1,
		RightChild:   int16(1 + len(leftNodes)),
		LeafValue:    0,
	}
	nodes := []DecisionNode{root}
	nodes = append(nodes, leftNodes...)
	offset := len(nodes)
	nodes[0].RightChild = int16(offset)
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

func normalizeAutoTuneAxis(axis string) string {
	switch strings.ToLower(strings.TrimSpace(axis)) {
	case "numtrees", "trees", "num_trees":
		return "numTrees"
	case "maxdepth", "depth", "max_depth":
		return "maxDepth"
	case "minsamplesleaf", "min_samples_leaf", "leaf":
		return "minSamplesLeaf"
	default:
		return ""
	}
}

func normalizeAutoTuneMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "", "validationaccuracy", "accuracy", "backtestaccuracy", "backtest", "validation":
		return "validationAccuracy"
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
	if size > 11 {
		size = 11
	}
	if size%2 == 0 {
		size++
		if size > 11 {
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

func autoTuneMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
