package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

// TrainingLogEntry is a single timestamped log line during training
type TrainingLogEntry struct {
	Timestamp time.Time
	Message   string
}

// TrainingHistoryEntry records metrics from a single training run
type TrainingHistoryEntry struct {
	Timestamp             time.Time `json:"timestamp"`
	Accuracy              float64   `json:"accuracy"`
	TrainAccuracy         float64   `json:"trainAccuracy,omitempty"`
	ValidationAccuracy    float64   `json:"validationAccuracy,omitempty"`
	NumTrees              int       `json:"numTrees"`
	NumSamples            int       `json:"numSamples"`
	TrainSamples          int       `json:"trainSamples,omitempty"`
	ValidationSamples     int       `json:"validationSamples,omitempty"`
	ValidationSplitRatio   float64   `json:"validationSplitRatio,omitempty"`
	LLMScoredSamples      int       `json:"llmScoredSamples,omitempty"`
	LLMAverageRiskScore   float64   `json:"llmAverageRiskScore,omitempty"`
	LLMAgreement          float64   `json:"llmAgreement,omitempty"`
	Duration              float64   `json:"duration"` // seconds
}

// ModelTrainer builds and evaluates random forest models
type ModelTrainer struct {
	mu        chan struct{} // single-training mutex via channel
	cancelCh  chan struct{} // closed to request cancellation
	isRunning bool
	progress  float64
	lastError string
	lastTrain time.Time
	accuracy  float64
	trainAccuracy      float64
	validationAccuracy float64
	validationRatio    float64
	// Training log ring buffer
	logMu      sync.RWMutex
	logs       []TrainingLogEntry
	logMaxSize int
	logNext    int
	logTotal   int
	// Training history
	historyMu sync.RWMutex
	history   []TrainingHistoryEntry
	splitMu   sync.RWMutex
	lastTrainSamples      []TrainingSample
	lastValidationSamples []TrainingSample
	lastLLMReview         *LLMReviewSummary
}

// CancelTraining signals any running training to stop.
func (t *ModelTrainer) CancelTraining() {
	if t.isRunning {
		t.logf("训练中止请求已接收")
		close(t.cancelCh)
	}
}

// IsCancelled returns true if cancellation has been requested.
func (t *ModelTrainer) IsCancelled() bool {
	select {
	case <-t.cancelCh:
		return true
	default:
		return false
	}
}

// ResetCancel prepares a new cancel channel for the next training run.
func (t *ModelTrainer) ResetCancel() {
	t.cancelCh = make(chan struct{})
}

var globalTrainer = &ModelTrainer{
	mu:         make(chan struct{}, 1),
	cancelCh:   make(chan struct{}),
	logMaxSize: 200,
}

func (t *ModelTrainer) logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("[ML-Train] %s", msg)

	t.logMu.Lock()
	entry := TrainingLogEntry{Timestamp: time.Now(), Message: msg}
	if len(t.logs) < t.logMaxSize {
		t.logs = append(t.logs, entry)
	} else {
		t.logs[t.logNext] = entry
	}
	t.logNext = (t.logNext + 1) % t.logMaxSize
	t.logTotal++
	t.logMu.Unlock()
}

// GetLogs returns recent training log entries (newest last)
func (t *ModelTrainer) GetLogs(limit int) []TrainingLogEntry {
	t.logMu.RLock()
	defer t.logMu.RUnlock()

	n := len(t.logs)
	if limit <= 0 || limit > n {
		limit = n
	}
	if n == 0 {
		return nil
	}
	// Return in chronological order
	out := make([]TrainingLogEntry, limit)
	copy(out, t.logs[max(0, n-limit):])
	return out
}

// GetHistory returns training history entries
func (t *ModelTrainer) GetHistory() []TrainingHistoryEntry {
	t.historyMu.RLock()
	defer t.historyMu.RUnlock()
	out := make([]TrainingHistoryEntry, len(t.history))
	copy(out, t.history)
	return out
}

// addHistory records a training run to history
func (t *ModelTrainer) addHistory(entry TrainingHistoryEntry) {
	t.historyMu.Lock()
	defer t.historyMu.Unlock()
	t.history = append(t.history, entry)
	// Keep last 100 entries
	if len(t.history) > 100 {
		t.history = t.history[len(t.history)-100:]
	}
}

// TrainResult holds the outcome of a training run
type TrainResult struct {
	Accuracy           float64
	TrainAccuracy      float64
	ValidationAccuracy float64
	NumTrees           int
	NumSamples         int
	TrainSamples       int
	ValidationSamples   int
	LLMScoredSamples   int
	LLMAverageRiskScore float64
	LLMAgreement       float64
	Error              string
}

// splitPoint represents a candidate feature split during training
type splitPoint struct {
	featureIdx int
	threshold  float64
	giniGain   float64
}

// trainSample labels are [0,3] for ALLOW/BLOCK/REWRITE/ALERT
type trainSample struct {
	features [FeatureDim]float64
	label    int32
}

// Train builds a random forest from labeled training data.
// Uses bootstrap aggregating (bagging) with Gini impurity splitting.
func (t *ModelTrainer) Train(store *TrainingDataStore, numTrees, maxDepth, minSamplesLeaf int) (*DecisionForest, TrainResult) {
	// Acquire training mutex
	select {
	case t.mu <- struct{}{}:
		defer func() { <-t.mu }()
	default:
		return nil, TrainResult{Error: "training already in progress"}
	}

	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	trainStart := time.Now()
	t.logf("══════ Training started ══════")
	t.logf("Config: trees=%d, maxDepth=%d, minSamplesLeaf=%d", numTrees, maxDepth, minSamplesLeaf)
	defer func() {
		t.isRunning = false
		t.progress = 1.0
	}()

	labeled := store.LabeledSamples()
	if len(labeled) < minSamplesLeaf*10 {
		msg := fmt.Sprintf("Insufficient labeled samples: need >=%d, have %d", minSamplesLeaf*10, len(labeled))
		t.logf("ERROR: %s", msg)
		return nil, TrainResult{Error: msg}
	}
	t.logf("Labeled samples loaded: %d", len(labeled))

	// Convert to internal format
	samples := make([]trainSample, len(labeled))
	classDist := make(map[int32]int)
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
		classDist[s.Label]++
	}
	t.logf("Class distribution: ALLOW=%d, BLOCK=%d, ALERT=%d, REWRITE=%d",
		classDist[0], classDist[1], classDist[3], classDist[2])

	// Train/validation split
	validationRatio := mlConfig.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
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
	trainSet := samples[:trainCount]
	validationSet := samples[trainCount:]
	trainRaw := append([]TrainingSample(nil), shuffledRaw[:trainCount]...)
	validationRaw := append([]TrainingSample(nil), shuffledRaw[trainCount:]...)
	t.logf("Data split: train=%d, validation=%d (ratio=%.2f)", len(trainSet), len(validationSet), validationRatio)

	// Build forest
	t.logf("Building random forest with %d trees...", numTrees)
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	featureSampleCount := int(math.Sqrt(float64(FeatureDim))) // sqrt(F) features per split
	t.logf("Feature sampling: %d of %d per split", featureSampleCount, FeatureDim)

	totalNodes := 0
	treeStart := time.Now()
	for ti := 0; ti < numTrees; ti++ {
		if t.IsCancelled() {
			t.logf("训练已中止")
			return nil, TrainResult{Error: "cancelled"}
		}
		t.progress = float64(ti) / float64(numTrees)
		tStart := time.Now()

		// Bootstrap sample
		bootstrap := make([]trainSample, len(trainSet))
		for i := range bootstrap {
			bootstrap[i] = trainSet[rand.Intn(len(trainSet))]
		}

		// Build tree
		nodes := buildTree(bootstrap, 0, maxDepth, minSamplesLeaf, featureSampleCount)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
		totalNodes += len(nodes)

		elapsed := time.Since(tStart)
		if ti%10 == 0 || ti == numTrees-1 {
			t.logf("Tree %d/%d built: %d nodes, %s (%.0f%%)",
				ti+1, numTrees, len(nodes), elapsed.Round(time.Microsecond), t.progress*100)
		}
	}
	treeElapsed := time.Since(treeStart)
	t.logf("All %d trees built in %s, total nodes: %d, avg nodes/tree: %d",
		numTrees, treeElapsed.Round(time.Millisecond), totalNodes, totalNodes/numTrees)

	forest.IsTrained = true

	// Evaluate on train and validation sets
	t.logf("Evaluating model on %d train samples and %d validation samples...", len(trainSet), len(validationSet))
	evalStart := time.Now()
	trainAccuracy := evaluateForest(forest, trainSet)
	validationAccuracy := evaluateForest(forest, validationSet)
	evalElapsed := time.Since(evalStart)

	// Per-class metrics
	perClassCorrect := make(map[int32]int)
	perClassTotal := make(map[int32]int)
	for _, s := range validationSet {
		pred := forest.Predict(s.features)
		perClassTotal[s.label]++
		if pred.Action == s.label {
			perClassCorrect[s.label]++
		}
	}
	t.logf("Evaluation complete in %s", evalElapsed.Round(time.Millisecond))
	for _, lbl := range []int32{0, 1, 2, 3} {
		if perClassTotal[lbl] > 0 {
			acc := float64(perClassCorrect[lbl]) / float64(perClassTotal[lbl]) * 100
			t.logf("  %s: %d/%d correct (%.1f%%)", actionLabel[lbl], perClassCorrect[lbl], perClassTotal[lbl], acc)
		}
	}
	t.logf("Train accuracy: %.2f%%", trainAccuracy*100)
	t.logf("Validation accuracy: %.2f%%", validationAccuracy*100)

	llmReviewSamples := 0
	llmAverageRiskScore := 0.0
	llmAgreement := 0.0
	if mlConfig.LlmEnabled {
		if review, err := t.reviewValidationWithLLM(validationRaw); err != nil {
			t.logf("WARN: LLM post-training review failed: %v", err)
		} else if review != nil {
			llmReviewSamples = review.ScoredSamples
			llmAverageRiskScore = review.AverageRiskScore
			llmAgreement = review.Agreement
			t.logf("LLM post-training review: %d samples, avg risk %.1f, agreement %.1f%%", review.ScoredSamples, review.AverageRiskScore, review.Agreement*100)
			t.setLastLLMReview(review)
		}
	}

	t.accuracy = validationAccuracy
	t.trainAccuracy = trainAccuracy
	t.validationAccuracy = validationAccuracy
	t.validationRatio = validationRatio
	t.lastTrain = time.Now()
	t.logf("══════ Training complete in %s ══════", treeElapsed.Round(time.Millisecond))
	t.setLastSplit(trainRaw, validationRaw)

	// Record to history
	t.addHistory(TrainingHistoryEntry{
		Timestamp:           trainStart,
		Accuracy:            validationAccuracy,
		TrainAccuracy:       trainAccuracy,
		ValidationAccuracy:  validationAccuracy,
		NumTrees:            numTrees,
		NumSamples:          len(labeled),
		TrainSamples:        len(trainRaw),
		ValidationSamples:   len(validationRaw),
		ValidationSplitRatio: validationRatio,
		LLMScoredSamples:    llmReviewSamples,
		LLMAverageRiskScore: llmAverageRiskScore,
		LLMAgreement:        llmAgreement,
		Duration:            time.Since(trainStart).Seconds(),
	})

	result := TrainResult{
		Accuracy:           validationAccuracy,
		TrainAccuracy:      trainAccuracy,
		ValidationAccuracy: validationAccuracy,
		NumTrees:           numTrees,
		NumSamples:         len(labeled),
		TrainSamples:       len(trainRaw),
		ValidationSamples:  len(validationRaw),
		LLMScoredSamples:   llmReviewSamples,
		LLMAverageRiskScore: llmAverageRiskScore,
		LLMAgreement:       llmAgreement,
	}

	return forest, result
}

func (t *ModelTrainer) setLastSplit(trainSamples, validationSamples []TrainingSample) {
	t.splitMu.Lock()
	defer t.splitMu.Unlock()

	t.lastTrainSamples = append(t.lastTrainSamples[:0], trainSamples...)
	t.lastValidationSamples = append(t.lastValidationSamples[:0], validationSamples...)
}

func (t *ModelTrainer) setLastLLMReview(review *LLMReviewSummary) {
	t.splitMu.Lock()
	defer t.splitMu.Unlock()

	if review == nil {
		t.lastLLMReview = nil
		return
	}
	copyReview := *review
	t.lastLLMReview = &copyReview
}

func (t *ModelTrainer) LastValidationSamples() []TrainingSample {
	t.splitMu.RLock()
	defer t.splitMu.RUnlock()

	out := make([]TrainingSample, len(t.lastValidationSamples))
	copy(out, t.lastValidationSamples)
	return out
}

func (t *ModelTrainer) LastTrainSamples() []TrainingSample {
	t.splitMu.RLock()
	defer t.splitMu.RUnlock()

	out := make([]TrainingSample, len(t.lastTrainSamples))
	copy(out, t.lastTrainSamples)
	return out
}

func (t *ModelTrainer) LastLLMReview() *LLMReviewSummary {
	t.splitMu.RLock()
	defer t.splitMu.RUnlock()

	if t.lastLLMReview == nil {
		return nil
	}
	copyReview := *t.lastLLMReview
	return &copyReview
}

func (t *ModelTrainer) SplitMetrics() (trainAccuracy, validationAccuracy, validationRatio float64, trainSamples, validationSamples int) {
	t.splitMu.RLock()
	defer t.splitMu.RUnlock()

	return t.trainAccuracy, t.validationAccuracy, t.validationRatio, len(t.lastTrainSamples), len(t.lastValidationSamples)
}

// buildTree recursively builds a decision tree using Gini impurity
func buildTree(samples []trainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int) []DecisionNode {
	// Check termination conditions
	if depth >= maxDepth || len(samples) < minSamplesLeaf*2 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	// Check if all same class
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

	// Find best split
	best := findBestSplit(samples, featureSampleCount)
	if best.giniGain <= 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	// Partition samples
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

	// Build children
	leftNodes := buildTree(leftSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount)
	rightNodes := buildTree(rightSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount)

	// Rebase child pointers from subtree-relative to absolute positions
	leftOffset := 1
	rightOffset := 1 + len(leftNodes)

	for i := range leftNodes {
		n := &leftNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(leftOffset)
			n.RightChild += int16(leftOffset)
		}
	}
	for i := range rightNodes {
		n := &rightNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(rightOffset)
			n.RightChild += int16(rightOffset)
		}
	}

	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    int16(leftOffset),
		RightChild:   int16(rightOffset),
		LeafValue:    0,
	}

	nodes := []DecisionNode{root}
	nodes = append(nodes, leftNodes...)
	nodes = append(nodes, rightNodes...)

	return nodes
}

// findBestSplit finds the best feature and threshold using Gini impurity
func findBestSplit(samples []trainSample, featureSampleCount int) splitPoint {
	best := splitPoint{giniGain: -1}
	parentGini := giniImpurity(samples)

	// Random feature selection
	features := make([]int, FeatureDim)
	for i := range features {
		features[i] = i
	}
	rand.Shuffle(len(features), func(i, j int) { features[i], features[j] = features[j], features[i] })
	selectedFeatures := features[:featureSampleCount]

	for _, fi := range selectedFeatures {
		// Sort by this feature
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].features[fi] < samples[j].features[fi]
		})

		// Try thresholds between distinct values
		for i := 1; i < len(samples); i++ {
			if samples[i].features[fi] == samples[i-1].features[fi] {
				continue
			}
			threshold := (samples[i].features[fi] + samples[i-1].features[fi]) / 2.0

			leftSamples := samples[:i]
			rightSamples := samples[i:]

			if len(leftSamples) < 1 || len(rightSamples) < 1 {
				continue
			}

			leftWeight := float64(len(leftSamples)) / float64(len(samples))
			gain := parentGini - leftWeight*giniImpurity(leftSamples) -
				(1-leftWeight)*giniImpurity(rightSamples)

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

// giniImpurity computes Gini impurity for a set of samples
func giniImpurity(samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	counts := make(map[int32]float64)
	for _, s := range samples {
		counts[s.label]++
	}
	var impurity float64
	n := float64(len(samples))
	for _, c := range counts {
		p := c / n
		impurity += p * (1 - p)
	}
	return impurity
}

// majorityClass returns the most common class label as float32
func majorityClass(samples []trainSample) float32 {
	if len(samples) == 0 {
		return 0
	}
	counts := make(map[int32]int)
	for _, s := range samples {
		counts[s.label]++
	}
	best := int32(0)
	bestCount := 0
	for label, count := range counts {
		if count > bestCount {
			bestCount = count
			best = label
		}
	}
	return float32(best)
}

// evaluateForest computes accuracy on a test set
func evaluateForest(forest *DecisionForest, testSet []trainSample) float64 {
	if len(testSet) == 0 {
		return 1.0
	}
	correct := 0
	for _, s := range testSet {
		pred := forest.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(testSet))
}

// GetStatus returns training status for the API
func (t *ModelTrainer) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"isRunning": t.isRunning,
		"progress":  t.progress,
		"lastError": t.lastError,
		"lastTrain": t.lastTrain.Format(time.RFC3339),
		"accuracy":  t.accuracy,
	}
}

// mlReasoning builds a human-readable explanation of the ML prediction
func mlReasoning(pred Prediction, anomalyScore float64, classification *pb.BehaviorClassification) string {
	parts := make([]string, 0, 3)

	if pred.Confidence >= 0.85 {
		parts = append(parts, "high-confidence ML prediction")
	} else if pred.Confidence >= 0.60 {
		parts = append(parts, "moderate-confidence ML prediction")
	} else {
		parts = append(parts, "low-confidence ML prediction")
	}

	parts = append(parts, "action="+actionLabel[pred.Action])

	if anomalyScore > 0.7 {
		parts = append(parts, "highly anomalous")
	} else if anomalyScore > 0.3 {
		parts = append(parts, "moderately anomalous")
	} else {
		parts = append(parts, "behavior within normal range")
	}

	if classification != nil && classification.PrimaryCategory != "" {
		parts = append(parts, "category="+classification.PrimaryCategory)
	}

	return strings.Join(parts, "; ")
}

// TrainWithConfig trains a model based on the MLConfig.ModelType.
// Returns a Model interface so callers don't need to know the concrete type.
func (t *ModelTrainer) TrainWithConfig(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	switch cfg.ModelType {
	case ModelRandomForest:
		forest, result := t.Train(store, cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
		return forest, result
	case ModelKNN:
		return t.trainKNN(store, cfg)
	case ModelLogisticRegression:
		return t.trainLogistic(store, cfg)
	default:
		forest, result := t.Train(store, cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
		return forest, result
	}
}

func (t *ModelTrainer) trainKNN(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()

	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) == 0 {
		return nil, TrainResult{Error: "no labeled samples available"}
	}

	k := cfg.NumTrees
	if k < 1 {
		k = 5
	}
	if k > len(labeled) {
		k = len(labeled)
	}

	model := NewKNNModel(k, "euclidean", "uniform")
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	t.logf("KNN 训练完成: k=%d, samples=%d", k, len(labeled))

	correct := 0
	for _, s := range labeled {
		pred := model.Predict(s.Features)
		if pred.Action == s.Label {
			correct++
		}
	}
	accuracy := float64(correct) / float64(len(labeled))

	t.lastTrain = time.Now()
	t.accuracy = accuracy
	t.trainAccuracy = accuracy
	t.validationAccuracy = accuracy

	t.addHistory(TrainingHistoryEntry{
		Timestamp:  t.lastTrain,
		Accuracy:   accuracy,
		NumSamples: len(labeled),
	})

	return model, TrainResult{
		Accuracy:          accuracy,
		TrainAccuracy:     accuracy,
		ValidationAccuracy: accuracy,
		NumSamples:        len(labeled),
		TrainSamples:      len(labeled),
	}
}

func (t *ModelTrainer) trainLogistic(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()

	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need at least 10 labeled samples for logistic regression"}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffled := make([]TrainingSample, len(labeled))
	copy(shuffled, labeled)
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	splitIdx := int(float64(len(shuffled)) * (1.0 - cfg.ValidationSplitRatio))
	if splitIdx < 1 {
		splitIdx = 1
	}

	trainSamples := make([][FeatureDim]float64, splitIdx)
	trainLabels := make([]int32, splitIdx)
	for i := 0; i < splitIdx; i++ {
		trainSamples[i] = shuffled[i].Features
		trainLabels[i] = shuffled[i].Label
	}

	model := NewLogisticModel(0.01, "l2", 1000)
	model.NumClasses = 4
	model.Train(trainSamples, trainLabels)

	t.logf("逻辑回归训练完成: samples=%d", splitIdx)

	trainCorrect := 0
	for i := 0; i < splitIdx; i++ {
		if pred := model.Predict(trainSamples[i]); pred.Action == trainLabels[i] {
			trainCorrect++
		}
	}
	trainAcc := float64(trainCorrect) / float64(splitIdx)

	valAcc := trainAcc
	valSamples := 0
	if splitIdx < len(shuffled) {
		valSamples = len(shuffled) - splitIdx
		valCorrect := 0
		for i := splitIdx; i < len(shuffled); i++ {
			if pred := model.Predict(shuffled[i].Features); pred.Action == shuffled[i].Label {
				valCorrect++
			}
		}
		valAcc = float64(valCorrect) / float64(valSamples)
	}

	t.lastTrain = time.Now()
	t.accuracy = valAcc
	t.trainAccuracy = trainAcc
	t.validationAccuracy = valAcc

	t.addHistory(TrainingHistoryEntry{
		Timestamp:  t.lastTrain,
		Accuracy:   valAcc,
		NumSamples: len(labeled),
	})

	return model, TrainResult{
		Accuracy:           valAcc,
		TrainAccuracy:      trainAcc,
		ValidationAccuracy: valAcc,
		NumSamples:         len(labeled),
		TrainSamples:       splitIdx,
		ValidationSamples:  valSamples,
	}
}
