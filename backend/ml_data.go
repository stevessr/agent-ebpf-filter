package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const trainingStoreMagic = "AEF2"

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
	samples     []TrainingSample
	maxSamples  int
	nextWrite   int
	totalAdded  int
	dataDir     string
	persistPath string
	dirtyCount  int // number of unsaved samples
}

var globalTrainingStore *TrainingDataStore

func newTrainingDataStore(maxSamples int) *TrainingDataStore {
	dataDir := filepath.Join(getRealHomeDir(), ".config", "agent-ebpf-filter")
	return &TrainingDataStore{
		samples:     make([]TrainingSample, maxSamples),
		maxSamples:  maxSamples,
		dataDir:     dataDir,
		persistPath: filepath.Join(dataDir, "ml_training_data.bin"),
	}
}

// InitTrainingStore initializes the global training data store
func InitTrainingStore(maxSamples int) {
	globalTrainingStore = newTrainingDataStore(maxSamples)
	globalTrainingStore.loadFromDisk()
}

// Add adds a training sample to the store
func (s *TrainingDataStore) Add(sample TrainingSample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.samples[s.nextWrite] = sample
	s.nextWrite = (s.nextWrite + 1) % s.maxSamples
	s.totalAdded++
	s.dirtyCount++
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
	return cleared
}

// LabeledSamples returns all samples with user labels
func (s *TrainingDataStore) LabeledSamples() []TrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []TrainingSample
	for i := range s.samples {
		if s.samples[i].IsLabeled() {
			out = append(out, s.samples[i])
		}
	}
	// Ensure at least one empty check
	for i := range s.samples {
		if s.samples[i].Timestamp.IsZero() {
			continue
		}
		_ = i
		break
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
			out = append(out, s.samples[i])
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
	s.samples[index].Label = label
	s.samples[index].UserLabel = userLabel
	s.dirtyCount++
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
	s.samples[index].AnomalyScore = anomalyScore
	s.dirtyCount++
	return true
}

// ApplyFeedback applies user feedback to label matching samples
func (s *TrainingDataStore) ApplyFeedback(comm string, userAction string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	label := int32(-1)
	switch userAction {
	case "accepted":
		label = 0 // ALLOW
	case "rejected":
		label = 1 // BLOCK
	case "alerted":
		label = 3 // ALERT
	}

	matched := 0
	for i := range s.samples {
		if s.samples[i].Comm == comm && !s.samples[i].IsLabeled() {
			s.samples[i].Label = label
			s.samples[i].UserLabel = userAction
			s.dirtyCount++
			matched++
		}
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
	return true
}

// AllSamplesWithIndex returns all non-zero samples with their ring buffer index
func (s *TrainingDataStore) AllSamplesWithIndex() []IndexedTrainingSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []IndexedTrainingSample
	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			out = append(out, IndexedTrainingSample{Index: i, Sample: s.samples[i]})
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
		if s.samples[i].Comm == comm && sameStringSlice(s.samples[i].Args, args) {
			out = append(out, IndexedTrainingSample{Index: i, Sample: s.samples[i]})
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
		if s.samples[i].Comm == comm && sameStringSlice(s.samples[i].Args, args) {
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

// Flush writes dirty samples to disk
func (s *TrainingDataStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dirtyCount == 0 {
		return nil
	}
	if err := s.persistLocked(); err != nil {
		return err
	}
	s.dirtyCount = 0
	return nil
}

func (s *TrainingDataStore) persistLocked() error {
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return err
	}

	tmpPath := s.persistPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Format: [4 bytes magic][4 bytes count][records...]
	// Each record: timestamp(8), label(4), anomaly_score(8),
	//             command_line_len(2), command_line_bytes...,
	//             comm_len(2), comm_bytes...,
	//             args_len(2), args_json_bytes..., features(128*8)
	count := uint32(0)
	// Count valid entries first
	for i := range s.samples {
		if !s.samples[i].Timestamp.IsZero() {
			count++
		}
	}

	if _, err := f.Write([]byte(trainingStoreMagic)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, count); err != nil {
		return err
	}

	for i := range s.samples {
		sample := &s.samples[i]
		if sample.Timestamp.IsZero() {
			continue
		}
		if err := binary.Write(f, binary.LittleEndian, sample.Timestamp.UnixNano()); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, sample.Label); err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, float64(sample.AnomalyScore)); err != nil {
			return err
		}
		commandLine := strings.TrimSpace(sample.CommandLine)
		if commandLine == "" {
			commandLine = joinCommandLine(sample.Comm, sample.Args)
		}
		commandLineBytes := []byte(commandLine)
		if err := binary.Write(f, binary.LittleEndian, uint16(len(commandLineBytes))); err != nil {
			return err
		}
		if _, err := f.Write(commandLineBytes); err != nil {
			return err
		}
		commBytes := []byte(sample.Comm)
		if err := binary.Write(f, binary.LittleEndian, uint16(len(commBytes))); err != nil {
			return err
		}
		if _, err := f.Write(commBytes); err != nil {
			return err
		}
		argsBytes, err := json.Marshal(sample.Args)
		if err != nil {
			return err
		}
		if err := binary.Write(f, binary.LittleEndian, uint16(len(argsBytes))); err != nil {
			return err
		}
		if _, err := f.Write(argsBytes); err != nil {
			return err
		}
		// Write features
		var featureBytes [FeatureDim * 8]byte
		for fi, v := range sample.Features {
			binary.LittleEndian.PutUint64(featureBytes[fi*8:(fi+1)*8], math.Float64bits(v))
		}
		if _, err := f.Write(featureBytes[:]); err != nil {
			return err
		}
	}

	return os.Rename(tmpPath, s.persistPath)
}

func (s *TrainingDataStore) loadFromDisk() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) < 4 {
		return fmt.Errorf("training data file too short")
	}

	versioned := len(data) >= 8 && bytes.Equal(data[0:4], []byte(trainingStoreMagic))
	offset := 4
	count := binary.LittleEndian.Uint32(data[0:4])
	if versioned {
		count = binary.LittleEndian.Uint32(data[4:8])
		offset = 8
	}

	loaded := 0
	for i := uint32(0); i < count && loaded < s.maxSamples; i++ {
		var sample TrainingSample
		if offset+24 > len(data) {
			break
		}

		sample.Timestamp = time.Unix(0, int64(binary.LittleEndian.Uint64(data[offset:offset+8])))
		offset += 8
		sample.Label = int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4
		sample.AnomalyScore = math.Float64frombits(binary.LittleEndian.Uint64(data[offset : offset+8]))
		offset += 8

		if versioned {
			commandLineLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+commandLineLen > len(data) {
				break
			}
			commandLine := string(data[offset : offset+commandLineLen])
			offset += commandLineLen

			commLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+commLen > len(data) {
				break
			}
			sample.Comm = string(data[offset : offset+commLen])
			offset += commLen

			argsLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+argsLen > len(data) {
				break
			}
			// Prefer JSON array encoding, but keep compatibility with older bracketed string records.
			argsRaw := strings.TrimSpace(string(data[offset : offset+argsLen]))
			if argsRaw != "" {
				var parsedArgs []string
				if err := json.Unmarshal([]byte(argsRaw), &parsedArgs); err == nil {
					sample.Args = parsedArgs
				} else {
					fallback := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(argsRaw, "]"), "["))
					if fallback != "" {
						sample.Args = splitCommandLine(fallback)
					}
				}
			}
			offset += argsLen
			sample.CommandLine = strings.TrimSpace(commandLine)
		} else {
			commLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+commLen > len(data) {
				break
			}
			sample.Comm = string(data[offset : offset+commLen])
			offset += commLen

			argsLen := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+argsLen > len(data) {
				break
			}
			// Prefer JSON array encoding, but keep compatibility with older bracketed string records.
			argsRaw := strings.TrimSpace(string(data[offset : offset+argsLen]))
			if argsRaw != "" {
				var parsedArgs []string
				if err := json.Unmarshal([]byte(argsRaw), &parsedArgs); err == nil {
					sample.Args = parsedArgs
				} else {
					fallback := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(argsRaw, "]"), "["))
					if fallback != "" {
						sample.Args = splitCommandLine(fallback)
					}
				}
			}
			offset += argsLen
			sample.CommandLine = joinCommandLine(sample.Comm, sample.Args)
		}

		if strings.TrimSpace(sample.CommandLine) == "" {
			sample.CommandLine = joinCommandLine(sample.Comm, sample.Args)
		}

		if offset+FeatureDim*8 > len(data) {
			break
		}
		for fi := 0; fi < FeatureDim; fi++ {
			sample.Features[fi] = math.Float64frombits(
				binary.LittleEndian.Uint64(data[offset+fi*8 : offset+(fi+1)*8]))
		}
		offset += FeatureDim * 8

		// Derive UserLabel from label
		if sample.Label >= 0 {
			sample.UserLabel = "loaded"
		}

		s.samples[s.nextWrite] = sample
		s.nextWrite = (s.nextWrite + 1) % s.maxSamples
		loaded++
	}
	s.totalAdded = loaded

	return nil
}
