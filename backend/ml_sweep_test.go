package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

type sweepProfile struct {
	Name       string
	ModelType  ModelType
	Comparable bool
	Kind       string // "heatmap" or "bar"
	XName      string
	YName      string
	XValues    []int
	YValues    []int
	XLabel     func(int) string
	YLabel     func(int) string
	Build      func(x, y int) MLConfig
	Summary    func(cfg MLConfig) string

	// Parameter metadata is used by the comprehensive axis-sweep verifier.
	// Numeric parameters must expose RequiredDiscretePoints unique values; small
	// categorical parameters use ParameterKind="categorical" and a lower/zero
	// requirement because there are only a few meaningful choices.
	ParameterName          string
	ParameterKind          string
	RequiredDiscretePoints int
	DatasetName            string
	DatasetDescription     string
}

type sweepResult struct {
	Profile             string
	Dataset             string
	BaseProfile         string
	ModelType           ModelType
	ParameterName       string
	ParameterKind       string
	RequiredPoints      int
	ConfiguredPoints    int
	XValue              int
	YValue              int
	ConfigSummary       string
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type profileSummary struct {
	Profile sweepProfile
	Best    sweepResult
	Results []sweepResult
	Chart   string
}

type repeatRunResult struct {
	Profile             string
	ModelType           ModelType
	XValue              int
	YValue              int
	ConfigSummary       string
	RunIndex            int
	TrainAccuracy       float64
	ValidationAccuracy  float64
	AllowPassRate       float64
	NumSamples          int
	TrainSamples        int
	ValidationSamples   int
	Duration            float64
	InferenceDuration   float64
	InferenceSamples    int
	InferenceLatencyMs  float64
	InferenceThroughput float64
	MemoryBytes         int64
	Error               string
}

type repeatSummary struct {
	Profile              string
	ModelType            ModelType
	Comparable           bool
	XValue               int
	YValue               int
	ConfigSummary        string
	Runs                 int
	SuccessRuns          int
	FailureRuns          int
	TrainMean            float64
	TrainStd             float64
	ValidationMean       float64
	ValidationStd        float64
	ValidationMin        float64
	ValidationMax        float64
	AllowMean            float64
	AllowStd             float64
	AllowMin             float64
	AllowMax             float64
	DurationMean         float64
	DurationStd          float64
	InferenceMean        float64
	InferenceStd         float64
	InferenceMin         float64
	InferenceMax         float64
	InferenceLatencyMean float64
	InferenceLatencyStd  float64
	TrainMin             float64
	TrainMax             float64
	MemoryMean           float64
	MemoryStd            float64
	MemoryMin            float64
	MemoryMax            float64
	SuccessRate          float64
}

type stabilityTask struct {
	Profile          sweepProfile
	Config           sweepResult
	Store            *TrainingDataStore
	BenchmarkSamples []TrainingSample
}

type sweepDataset struct {
	Name        string
	Description string
	Samples     []TrainingSample
}

func TestComprehensiveSweepProfilesCoverThousandPointsPerNumericParameter(t *testing.T) {
	profiles := profilesForMode("comprehensive")
	seen := make(map[ModelType]map[string]int)
	for _, profile := range profiles {
		if profile.ParameterName == "" {
			t.Fatalf("profile %s missing parameter metadata", profile.Name)
		}
		if profile.ParameterKind != "numeric" {
			if unique := uniqueIntCount(profile.XValues); profile.RequiredDiscretePoints != unique {
				t.Fatalf("%s categorical/fixed requirement=%d, want unique count %d", profile.Name, profile.RequiredDiscretePoints, unique)
			}
			continue
		}
		unique := uniqueIntCount(profile.XValues)
		if unique < 1000 {
			t.Fatalf("%s has %d unique points, want >=1000", profile.Name, unique)
		}
		if seen[profile.ModelType] == nil {
			seen[profile.ModelType] = make(map[string]int)
		}
		seen[profile.ModelType][profile.ParameterName] = unique
	}
	for _, modelType := range AllModelTypes() {
		for _, param := range numericSweepParametersForModel(modelType) {
			if seen[modelType][param] < 1000 {
				t.Fatalf("%s/%s coverage = %d, want >=1000 discrete points", modelType, param, seen[modelType][param])
			}
		}
	}
}

func TestComprehensiveSweepDefaultsToMultipleDatasets(t *testing.T) {
	samples := make([]TrainingSample, 0, 30)
	for i := 0; i < 12; i++ {
		samples = append(samples, sweepTestSample(0, "allow"))
	}
	for i := 0; i < 10; i++ {
		samples = append(samples, sweepTestSample(1, "block"))
	}
	for i := 0; i < 8; i++ {
		samples = append(samples, sweepTestSample(3, "alert"))
	}

	datasets := datasetProfilesForMode(samples, "comprehensive", nil)
	if len(datasets) < 2 {
		t.Fatalf("comprehensive datasets = %d, want at least 2", len(datasets))
	}
	if datasets[0].Name != "all" || len(datasets[0].Samples) != len(samples) {
		t.Fatalf("first dataset = %s/%d, want all/%d", datasets[0].Name, len(datasets[0].Samples), len(samples))
	}
	foundBalanced := false
	for _, dataset := range datasets {
		if dataset.Name == "label-balanced" {
			foundBalanced = true
			if len(dataset.Samples) != 24 {
				t.Fatalf("label-balanced samples = %d, want 24", len(dataset.Samples))
			}
		}
	}
	if !foundBalanced {
		t.Fatalf("expected label-balanced dataset, got %#v", datasets)
	}
}

func sweepTestSample(label int32, userLabel string) TrainingSample {
	return TrainingSample{
		Label:     label,
		UserLabel: userLabel,
		Timestamp: time.Now(),
		Comm:      "cmd",
		Args:      []string{fmt.Sprintf("%d", label)},
	}
}

func TestMLSweep(t *testing.T) {
	if os.Getenv("ML_SWEEP") != "1" {
		t.Skip("set ML_SWEEP=1 to run the offline ML sweep report generator")
	}
	if err := runMLSweepReport(); err != nil {
		t.Fatalf("ml sweep failed: %v", err)
	}
}
