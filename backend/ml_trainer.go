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

// ModelTrainer builds and evaluates random forest models
type ModelTrainer struct {
	mu        chan struct{} // single-training mutex via channel
	isRunning bool
	progress  float64
	lastError string
	lastTrain time.Time
	accuracy  float64
	// Training log ring buffer
	logMu      sync.RWMutex
	logs       []TrainingLogEntry
	logMaxSize int
	logNext    int
	logTotal   int
}

var globalTrainer = &ModelTrainer{
	mu:         make(chan struct{}, 1),
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

// TrainResult holds the outcome of a training run
type TrainResult struct {
	Accuracy   float64
	NumTrees   int
	NumSamples int
	Error      string
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

	t.isRunning = true
	t.progress = 0
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

	// 80/20 train/test split
	rand.Shuffle(len(samples), func(i, j int) { samples[i], samples[j] = samples[j], samples[i] })
	split := len(samples) * 8 / 10
	trainSet := samples[:split]
	testSet := samples[split:]
	t.logf("Data split: train=%d, test=%d", len(trainSet), len(testSet))

	// Build forest
	t.logf("Building random forest with %d trees...", numTrees)
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	featureSampleCount := int(math.Sqrt(float64(FeatureDim))) // sqrt(F) features per split
	t.logf("Feature sampling: %d of %d per split", featureSampleCount, FeatureDim)

	totalNodes := 0
	treeStart := time.Now()
	for ti := 0; ti < numTrees; ti++ {
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

	// Evaluate on test set
	t.logf("Evaluating model on %d test samples...", len(testSet))
	evalStart := time.Now()
	accuracy := evaluateForest(forest, testSet)
	evalElapsed := time.Since(evalStart)

	// Per-class metrics
	perClassCorrect := make(map[int32]int)
	perClassTotal := make(map[int32]int)
	for _, s := range testSet {
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
	t.logf("Overall accuracy: %.2f%%", accuracy*100)

	t.accuracy = accuracy
	t.lastTrain = time.Now()
	t.logf("══════ Training complete in %s ══════", treeElapsed.Round(time.Millisecond))

	result := TrainResult{
		Accuracy:   accuracy,
		NumTrees:   numTrees,
		NumSamples: len(labeled),
	}

	return forest, result
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

	// Create split node (left child follows immediately, right after left subtree)
	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    1,                         // Next node in array
		RightChild:   int16(1 + len(leftNodes)), // After left subtree
		LeafValue:    0,
	}

	nodes := []DecisionNode{root}
	nodes = append(nodes, leftNodes...)
	offset := len(nodes)
	nodes[0].RightChild = int16(offset) // Update right child index
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
