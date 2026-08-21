package app

import (
	"agent-ebpf-filter/app/ml"
	"fmt"
	"os"
	"testing"
	"time"
)

// ---- moved from backend/zz_merged_backend_test.go section sweep_test.go ----

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
	for _, modelType := range ml.AllModelTypes() {
		for _, param := range numericSweepParametersForModel(modelType) {
			if seen[modelType][param] < 1000 {
				t.Fatalf("%s/%s coverage = %d, want >=1000 discrete points", modelType, param, seen[modelType][param])
			}
		}
	}
}

func TestComprehensiveSweepDefaultsToMultipleDatasets(t *testing.T) {
	samples := make([]ml.TrainingSample, 0, 30)
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

func sweepTestSample(label int32, userLabel string) ml.TrainingSample {
	return ml.TrainingSample{
		Label:     label,
		UserLabel: userLabel,
		Timestamp: time.Now(),
		Comm:      "cmd",
		Args:      []string{fmt.Sprintf("%d", label)},
	}
}

func TestStabilityWorkerCountBoundsIdleWorkers(t *testing.T) {
	tests := []struct {
		name                string
		tasks, repeats, cpu int
		want                int
	}{
		{name: "no tasks", tasks: 0, repeats: 3, cpu: 8, want: 0},
		{name: "single job", tasks: 1, repeats: 1, cpu: 128, want: 1},
		{name: "bounded by jobs", tasks: 2, repeats: 2, cpu: 128, want: 4},
		{name: "bounded by cpu", tasks: 10, repeats: 3, cpu: 8, want: 8},
		{name: "minimum parallelism", tasks: 10, repeats: 3, cpu: 0, want: 2},
		{name: "normalize repeats", tasks: 1, repeats: 0, cpu: 0, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stabilityWorkerCount(tt.tasks, tt.repeats, tt.cpu); got != tt.want {
				t.Fatalf("stabilityWorkerCount(%d, %d, %d) = %d, want %d", tt.tasks, tt.repeats, tt.cpu, got, tt.want)
			}
		})
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
