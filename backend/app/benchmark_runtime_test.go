package app

import (
	"fmt"
	"testing"

	"agent-ebpf-filter/app/handlers"
)

func TestBenchmarkWorkerCountIsBounded(t *testing.T) {
	tests := []struct {
		cases int
		cpus  int
		want  int
	}{
		{cases: 0, cpus: 8, want: 0},
		{cases: 1, cpus: 128, want: 1},
		{cases: 10, cpus: 4, want: 4},
		{cases: 10, cpus: 0, want: 1},
	}
	for _, tt := range tests {
		if got := benchmarkWorkerCount(tt.cases, tt.cpus); got != tt.want {
			t.Fatalf("benchmarkWorkerCount(%d, %d) = %d, want %d", tt.cases, tt.cpus, got, tt.want)
		}
	}
}

func TestBenchmarkEngineBoundsRunHistory(t *testing.T) {
	engine := newBenchmarkEngine()
	for i := 0; i < benchmarkRunHistoryLimit+5; i++ {
		engine.storeRun(benchmarkRun{Name: fmt.Sprintf("run-%d", i)})
	}
	runs := engine.runsSnapshot()
	if len(runs) != benchmarkRunHistoryLimit {
		t.Fatalf("run history length = %d, want %d", len(runs), benchmarkRunHistoryLimit)
	}
	if runs[0].Name != "run-5" || runs[len(runs)-1].Name != fmt.Sprintf("run-%d", benchmarkRunHistoryLimit+4) {
		t.Fatalf("unexpected bounded history range: first=%q last=%q", runs[0].Name, runs[len(runs)-1].Name)
	}
}

func TestBenchmarkHandlerDependenciesAreWired(t *testing.T) {
	if handlers.Deps.RunBenchmark == nil || handlers.Deps.GetBenchmarkResults == nil {
		t.Fatal("benchmark handler dependencies are not initialized")
	}
}
