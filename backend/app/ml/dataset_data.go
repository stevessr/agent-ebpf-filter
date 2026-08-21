package ml

import (
	"log"
	"path/filepath"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/behavior"
)

// ---- moved from backend/zz_merged_backend.go section dataset_data.go ----

const (
	trainingStoreMagic              = "AEF2"
	trainingStoreMaxSamples         = 100000
	trainingStoreMaxFileBytes int64 = 512 << 20
)

// TrainingSample represents one labeled wrapper intercept event for ML training
type TrainingSample struct {
	Features     [FeatureDim]float64
	Label        int32 // 0=ALLOW, 1=BLOCK, 2=REWRITE, 3=ALERT, -1=unlabeled
	CommandLine  string
	Comm         string
	Args         []string
	Category     string
	AnomalyScore float64
	Timestamp    time.Time
	UserLabel    string // "accepted", "rejected", "auto", ""
}

// IndexedTrainingSample keeps the ring-buffer slot alongside the sample data.
type IndexedTrainingSample struct {
	Index  int
	Sample TrainingSample
}

// IsLabeled returns true if the sample has a user-provided label
func (s *TrainingSample) IsLabeled() bool {
	return s.Label >= 0 && s.Label <= 3 && s.UserLabel != ""
}

// TrainingDataStore is a ring buffer of training samples with disk persistence
type TrainingDataStore struct {
	mu          sync.RWMutex
	flushMu     sync.Mutex
	samples     []TrainingSample
	maxSamples  int
	nextWrite   int
	totalAdded  int
	dataDir     string
	persistPath string
	dirtyCount  int // number of unsaved samples
	revision    uint64
}

var GlobalTrainingStore *TrainingDataStore

func NewTrainingDataStore(maxSamples int) *TrainingDataStore {
	maxSamples = NormalizeTrainingStoreCapacity(maxSamples)
	dataDir := filepath.Join(platform.GetRealHomeDir(), ".config", "agent-ebpf-filter")
	return &TrainingDataStore{
		samples:     make([]TrainingSample, maxSamples),
		maxSamples:  maxSamples,
		dataDir:     dataDir,
		persistPath: filepath.Join(dataDir, "ml_training_data.bin"),
	}
}

// InitTrainingStore initializes the global training data store
func InitTrainingStore(maxSamples int) {
	GlobalTrainingStore = NewTrainingDataStore(maxSamples)
	if err := GlobalTrainingStore.LoadFromDisk(); err != nil {
		log.Printf("[WARN] failed to load persisted ML training data: %v", err)
	}
}

// Add adds a training sample to the store
func (s *TrainingDataStore) Add(sample TrainingSample) {
	if s == nil || s.maxSamples <= 0 || len(s.samples) == 0 {
		return
	}
	sample = NormalizeTrainingSample(sample)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.samples[s.nextWrite] = sample
	s.nextWrite = (s.nextWrite + 1) % s.maxSamples
	s.totalAdded++
	s.dirtyCount++
	s.revision++
}

// Clear removes all samples from the store and resets the ring buffer state.
func (s *TrainingDataStore) Clear() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleared := 0
	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			cleared++
		}
		s.samples[i] = TrainingSample{}
	}
	s.nextWrite = 0
	s.totalAdded = 0
	s.dirtyCount++
	s.revision++
	return cleared
}

// LabeledSamples returns all samples with user labels
func (s *TrainingDataStore) LabeledSamples() []TrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []TrainingSample
	for i := range s.samples {
		if s.samples[i].IsLabeled() {
			out = append(out, cloneTrainingSample(s.samples[i]))
		}
	}
	return out
}

// AllSamples returns all samples (labeled and unlabeled)
func (s *TrainingDataStore) AllSamples() []TrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []TrainingSample
	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			out = append(out, cloneTrainingSample(s.samples[i]))
		}
	}
	return out
}

// RemoveSample removes a sample by index
func (s *TrainingDataStore) RemoveSample(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.samples) {
		return false
	}
	if s.samples[index].Timestamp.IsZero() {
		return false
	}
	s.samples[index] = TrainingSample{} // zero out
	s.dirtyCount++
	s.revision++
	return true
}

// UpdateSampleLabel updates the label of a sample by index
func (s *TrainingDataStore) UpdateSampleLabel(index int, label int32, userLabel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.samples) {
		return false
	}
	if s.samples[index].Timestamp.IsZero() {
		return false
	}
	if label < -1 || label > 3 {
		return false
	}
	s.samples[index].Label = label
	s.samples[index].UserLabel = normalizeTrainingMetadata(userLabel, trainingSampleMaxUserLabelBytes, true)
	s.dirtyCount++
	s.revision++
	return true
}

// UpdateSampleAnomaly updates the anomaly score of a sample by index
func (s *TrainingDataStore) UpdateSampleAnomaly(index int, anomalyScore float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.samples) {
		return false
	}
	if s.samples[index].Timestamp.IsZero() {
		return false
	}
	s.samples[index].AnomalyScore = normalizeTrainingScore(anomalyScore)
	s.dirtyCount++
	s.revision++
	return true
}

// ApplyFeedback applies user feedback to label matching samples
func (s *TrainingDataStore) ApplyFeedback(comm string, userAction string) int {
	label := int32(-1)
	switch userAction {
	case "accepted":
		label = 0 // ALLOW
	case "rejected":
		label = 1 // BLOCK
	case "alerted":
		label = 3 // ALERT
	default:
		return 0
	}
	userAction = normalizeTrainingMetadata(userAction, trainingSampleMaxUserLabelBytes, true)

	s.mu.Lock()
	defer s.mu.Unlock()

	matched := 0
	for i := range s.samples {
		if s.samples[i].Comm == comm && !s.samples[i].IsLabeled() {
			s.samples[i].Label = label
			s.samples[i].UserLabel = userAction
			s.dirtyCount++
			matched++
		}
	}
	if matched > 0 {
		s.revision++
	}
	return matched
}

// LabelSample labels a specific sample by its index in the ring buffer
func (s *TrainingDataStore) LabelSample(index int, label string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= s.maxSamples {
		return false
	}
	if s.samples[index].Timestamp.IsZero() {
		return false
	}

	labelInt := int32(-1)
	switch label {
	case "BLOCK":
		labelInt = 1
	case "ALERT":
		labelInt = 3
	case "ALLOW":
		labelInt = 0
	case "REWRITE":
		labelInt = 2
	default:
		return false
	}

	s.samples[index].Label = labelInt
	s.samples[index].UserLabel = "manual-index"
	s.dirtyCount++
	s.revision++
	return true
}

// AllSamplesWithIndex returns all non-zero samples with their ring buffer index
func (s *TrainingDataStore) AllSamplesWithIndex() []IndexedTrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []IndexedTrainingSample
	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			out = append(out, IndexedTrainingSample{Index: i, Sample: cloneTrainingSample(s.samples[i])})
		}
	}
	return out
}

// BoundedSamplesWithIndex returns at most limit samples without first copying
// the entire training ring. It is used by bounded API jobs such as LLM review.
func (s *TrainingDataStore) BoundedSamplesWithIndex(limit int, onlyUnlabeled bool) []IndexedTrainingSample {
	if s == nil || limit <= 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]IndexedTrainingSample, 0, min(limit, len(s.samples)))
	for i := range s.samples {
		sample := s.samples[i]
		if sample.Timestamp.IsZero() || (onlyUnlabeled && sample.IsLabeled()) {
			continue
		}
		sample = cloneTrainingSample(sample)
		out = append(out, IndexedTrainingSample{Index: i, Sample: sample})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ExactMatches returns all samples whose command and arguments exactly match.
func (s *TrainingDataStore) ExactMatches(comm string, args []string) []IndexedTrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []IndexedTrainingSample
	for i := range s.samples {
		if s.samples[i].Timestamp.IsZero() {
			continue
		}
		if s.samples[i].Comm == comm && behavior.SameStringSlice(s.samples[i].Args, args) {
			out = append(out, IndexedTrainingSample{Index: i, Sample: cloneTrainingSample(s.samples[i])})
		}
	}
	return out
}

// HasExactCommand reports whether an exact command sample already exists.
func (s *TrainingDataStore) HasExactCommand(comm string, args []string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.samples {
		if s.samples[i].Timestamp.IsZero() {
			continue
		}
		if s.samples[i].Comm == comm && behavior.SameStringSlice(s.samples[i].Args, args) {
			return true
		}
	}
	return false
}

// Status returns summary statistics
func (s *TrainingDataStore) Status() (totalSamples, labeledSamples int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			totalSamples++
			if s.samples[i].IsLabeled() {
				labeledSamples++
			}
		}
	}
	return
}

// Flush writes a stable snapshot without holding the sample mutex during disk I/O.
func (s *TrainingDataStore) Flush() error {
	if s == nil {
		return nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.RLock()
	if s.dirtyCount == 0 {
		s.mu.RUnlock()
		return nil
	}
	dirtySnapshot := s.dirtyCount
	samples := make([]TrainingSample, 0, len(s.samples))
	for _, sample := range s.samples {
		if !sample.Timestamp.IsZero() {
			samples = append(samples, cloneTrainingSample(sample))
		}
	}
	dataDir := s.dataDir
	persistPath := s.persistPath
	s.mu.RUnlock()

	if err := persistTrainingStoreSnapshot(dataDir, persistPath, samples); err != nil {
		return err
	}

	s.mu.Lock()
	if s.dirtyCount >= dirtySnapshot {
		s.dirtyCount -= dirtySnapshot
	} else {
		s.dirtyCount = 0
	}
	s.mu.Unlock()
	return nil
}

func (s *TrainingDataStore) LoadFromDisk() error {
	if s == nil {
		return nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.RLock()
	persistPath := s.persistPath
	maxSamples := s.maxSamples
	revision := s.revision
	s.mu.RUnlock()

	samples, err := readTrainingStoreFile(persistPath, maxSamples)
	if err != nil {
		return err
	}
	if samples == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revision != revision {
		return errTrainingStoreChangedDuringLoad
	}
	for index := range s.samples {
		s.samples[index] = TrainingSample{}
	}
	for index, sample := range samples {
		s.samples[index] = sample
	}
	if s.maxSamples > 0 {
		s.nextWrite = len(samples) % s.maxSamples
	} else {
		s.nextWrite = 0
	}
	s.totalAdded = len(samples)
	s.dirtyCount = 0
	s.revision++
	return nil
}

// MarkPristine clears the dirty counter after an in-memory-only bulk load so
// ephemeral stores (benchmarks, sweeps) never trigger a persistence flush.
func (s *TrainingDataStore) MarkPristine() {
	s.dirtyCount = 0
}

// SetPersistLocation redirects the store's data directory and snapshot path.
// Intended for tests and explicit relocation flows.
func (s *TrainingDataStore) SetPersistLocation(dir string) {
	s.dataDir = dir
	s.persistPath = filepath.Join(dir, "ml_training_data.bin")
}
