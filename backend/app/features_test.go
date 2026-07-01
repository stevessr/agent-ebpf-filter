package app

import (
	"math"
	"testing"
	"time"
)

func TestNormalizeFeatureVectorRepairsAndBounds(t *testing.T) {
	var f [FeatureDim]float64
	f[0] = -0.25
	f[1] = 1.25
	f[2] = math.NaN()
	f[116] = -1
	f[117] = 1

	got := normalizeFeatureVector(f)
	if got[0] != 0 {
		t.Fatalf("negative feature normalized to %f, want 0", got[0])
	}
	if got[1] != 1 {
		t.Fatalf("above-one feature normalized to %f, want 1", got[1])
	}
	if got[2] != 0 {
		t.Fatalf("non-finite feature normalized to %f, want 0", got[2])
	}
	if got[116] != 0 {
		t.Fatalf("sin cyclic feature normalized to %f, want 0", got[116])
	}
	if got[117] != 1 {
		t.Fatalf("cos cyclic feature normalized to %f, want 1", got[117])
	}
}

func TestFeatureExtractorProducesFiniteBoundedFeatures(t *testing.T) {
	fe := &FeatureExtractor{history: newRecentHistoryBuffer(16)}
	fe.AddHistory("git", "DEVELOPMENT", "ALLOW", 0.1, 100, "steve", 16, 2)
	fe.AddHistory("go", "DEVELOPMENT", "ALLOW", 0.2, 101, "steve", 24, 3)
	fe.AddHistory("curl", "NETWORK", "ALLOW", 0.3, 102, "steve", 32, 2)
	fe.AddHistory("grep", "FILE_READ", "ALLOW", 0.05, 103, "steve", 12, 2)

	features := fe.Extract("git", []string{"status", "--short"}, "steve", 104)
	for i, value := range features {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("feature %d is non-finite: %f", i, value)
		}
		if value < 0 || value > 1 {
			t.Fatalf("feature %d = %f, want [0,1]", i, value)
		}
	}
	if fe.sampleCount != 1 {
		t.Fatalf("sampleCount = %d, want 1", fe.sampleCount)
	}
}

func TestFeatureNormalizationSummaryFlagsOutOfRangeRawSamples(t *testing.T) {
	samples := []TrainingSample{
		{Features: normalizeFeatureVector([FeatureDim]float64{0: 0.2, 1: 0.8}), Timestamp: time.Now()},
		{Features: [FeatureDim]float64{0: -1, 1: 2, 2: math.Inf(1)}, Timestamp: time.Now()},
	}
	report := summarizeFeatureNormalization(samples)
	if report.SampleCount != 2 || report.FeatureDim != FeatureDim {
		t.Fatalf("report dimensions = %#v", report)
	}
	if report.BelowZeroValues == 0 || report.AboveOneValues == 0 || report.NonFiniteValues == 0 {
		t.Fatalf("report did not flag invalid values: %#v", report)
	}
}
