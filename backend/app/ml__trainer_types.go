package app

import (
	"agent-ebpf-filter/pb"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section trainer_types.go ----

// TrainingLogEntry is a single timestamped log line during training
type TrainingLogEntry struct {
	Timestamp time.Time
	Message   string
}

// TrainingHistoryEntry records metrics from a single training run
type TrainingHistoryEntry struct {
	Timestamp            time.Time `json:"timestamp"`
	Accuracy             float64   `json:"accuracy"`
	TrainAccuracy        float64   `json:"trainAccuracy,omitempty"`
	ValidationAccuracy   float64   `json:"validationAccuracy,omitempty"`
	NumTrees             int       `json:"numTrees"`
	NumSamples           int       `json:"numSamples"`
	TrainSamples         int       `json:"trainSamples,omitempty"`
	ValidationSamples    int       `json:"validationSamples,omitempty"`
	ValidationSplitRatio float64   `json:"validationSplitRatio,omitempty"`
	LLMScoredSamples     int       `json:"llmScoredSamples,omitempty"`
	LLMAverageRiskScore  float64   `json:"llmAverageRiskScore,omitempty"`
	LLMAgreement         float64   `json:"llmAgreement,omitempty"`
	Duration             float64   `json:"duration"` // seconds
}

// ModelTrainer builds and evaluates random forest models
type ModelTrainer struct {
	mu                 chan struct{} // single-training mutex via channel
	cancelCh           chan struct{} // closed to request cancellation
	isRunning          bool
	progress           float64
	lastError          string
	lastTrain          time.Time
	accuracy           float64
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
	historyMu             sync.RWMutex
	history               []TrainingHistoryEntry
	splitMu               sync.RWMutex
	lastTrainSamples      []TrainingSample
	lastValidationSamples []TrainingSample
	lastLLMReview         *LLMReviewSummary
}

// TrainResult holds the outcome of a training run
type TrainResult struct {
	Accuracy            float64
	TrainAccuracy       float64
	ValidationAccuracy  float64
	NumTrees            int
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	LLMScoredSamples    int
	LLMAverageRiskScore float64
	LLMAgreement        float64
	Error               string
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

var globalTrainer = &ModelTrainer{
	mu:         make(chan struct{}, 1),
	cancelCh:   make(chan struct{}),
	logMaxSize: 200,
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
	if len(t.history) > 100 {
		t.history = t.history[len(t.history)-100:]
	}
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
