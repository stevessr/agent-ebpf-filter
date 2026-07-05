package app

import (
	"strings"
	"testing"
	"time"
)

func readinessTestConfig() MLConfig {
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelLogisticRegression
	cfg.MinSamplesForTraining = 4
	cfg.MinSamplesLeaf = 1
	return cfg
}

func addReadinessSample(store *TrainingDataStore, label int32, userLabel string, comm string, value float64) {
	var features [FeatureDim]float64
	for i := range features {
		features[i] = value
	}
	store.Add(TrainingSample{
		Features:    features,
		Label:       label,
		CommandLine: comm + " --test",
		Comm:        comm,
		Args:        []string{"--test"},
		Category:    "test-category",
		Timestamp:   time.Now(),
		UserLabel:   userLabel,
	})
}

func TestBuildMLTrainingReadinessEmptyStore(t *testing.T) {
	store := newTrainingDataStore(8)
	readiness := buildMLTrainingReadiness(store, readinessTestConfig())
	if readiness.Ready {
		t.Fatal("empty store should not be training-ready")
	}
	if readiness.SampleCount != 0 || readiness.LabeledCount != 0 {
		t.Fatalf("unexpected sample counts: %+v", readiness)
	}
	if !hasPrefix(readiness.BlockingReasons, "insufficient_labeled_samples:") {
		t.Fatalf("expected insufficient_labeled_samples blocking reason, got %v", readiness.BlockingReasons)
	}
	if !hasExact(readiness.BlockingReasons, "no_labeled_samples") {
		t.Fatalf("expected no_labeled_samples blocking reason, got %v", readiness.BlockingReasons)
	}
	if !hasExact(readiness.SuggestedActions, "import_agent_legal_dataset_or_selinux_policy_dataset") {
		t.Fatalf("expected import suggestion, got %v", readiness.SuggestedActions)
	}
}

func TestBuildMLTrainingReadinessBalancedSamplesReady(t *testing.T) {
	store := newTrainingDataStore(8)
	for i := 0; i < 2; i++ {
		addReadinessSample(store, 0, "test", "allow-cmd", 0.10+float64(i)*0.01)
		addReadinessSample(store, 1, "test", "block-cmd", 0.70+float64(i)*0.01)
	}

	readiness := buildMLTrainingReadiness(store, readinessTestConfig())
	if !readiness.Ready {
		t.Fatalf("expected balanced labeled data to be ready: %+v", readiness)
	}
	if readiness.LabeledCount != 4 || readiness.ClassCount != 2 || readiness.MinSamples != 4 {
		t.Fatalf("unexpected readiness counts: %+v", readiness)
	}
	if !hasCount(readiness.ByLabel, "ALLOW", 2) || !hasCount(readiness.ByLabel, "BLOCK", 2) {
		t.Fatalf("unexpected label rollup: %+v", readiness.ByLabel)
	}
	if readiness.Normalization.NonFiniteValues != 0 || readiness.Normalization.BelowZeroValues != 0 || readiness.Normalization.AboveOneValues != 0 {
		t.Fatalf("expected normalized feature range, got %+v", readiness.Normalization)
	}
}

func TestBuildMLTrainingReadinessDetectsSingleClassAndOutOfRange(t *testing.T) {
	store := newTrainingDataStore(8)
	for i := 0; i < 5; i++ {
		value := 0.25
		if i == 4 {
			value = 1.5
		}
		addReadinessSample(store, 0, "test", "allow-cmd", value)
	}

	readiness := buildMLTrainingReadiness(store, readinessTestConfig())
	if readiness.Ready {
		t.Fatalf("single-class out-of-range data should not be ready: %+v", readiness)
	}
	if !hasExact(readiness.BlockingReasons, "single_class_training_data") {
		t.Fatalf("expected single_class_training_data, got %v", readiness.BlockingReasons)
	}
	if !hasPrefix(readiness.BlockingReasons, "feature_values_out_of_range:") {
		t.Fatalf("expected feature_values_out_of_range, got %v", readiness.BlockingReasons)
	}
	if !readiness.Quality.ClassImbalance {
		t.Fatalf("expected class imbalance quality flag: %+v", readiness.Quality)
	}
	if !hasExact(readiness.SuggestedActions, "add_counter_class_samples_for_allow_block_alert_rewrite") {
		t.Fatalf("expected class balance suggestion, got %v", readiness.SuggestedActions)
	}
}

func hasExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasPrefix(values []string, wantPrefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, wantPrefix) {
			return true
		}
	}
	return false
}

func hasCount(values []researchCount, key string, count int) bool {
	for _, value := range values {
		if value.Key == key && value.Count == count {
			return true
		}
	}
	return false
}
