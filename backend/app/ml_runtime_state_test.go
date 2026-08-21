package app

import (
	"sync"
	"testing"
)

func TestMLRuntimeSnapshotsPublishModelAndMetadataAtomically(t *testing.T) {
	previous := snapshotMLRuntime()
	t.Cleanup(func() { replaceMLRuntime(previous) })

	modelA := makeDummyLinearModel()
	modelB := makeDummyLinearModel()
	stateA := mlRuntimeSnapshot{
		Engine:      modelA,
		Config:      MLConfig{ModelPath: "state-a"},
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	}
	stateB := mlRuntimeSnapshot{
		Engine:      modelB,
		Config:      MLConfig{ModelPath: "state-b"},
		Enabled:     false,
		ModelLoaded: true,
		ModelType:   ModelSVM,
	}
	replaceMLRuntime(stateA)

	done := make(chan struct{})
	errors := make(chan string, 1)
	var readers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				snapshot := snapshotMLRuntime()
				validA := snapshot.Config.ModelPath == "state-a" && snapshot.Engine == modelA && snapshot.Enabled && snapshot.ModelLoaded && snapshot.ModelType == ModelLogisticRegression
				validB := snapshot.Config.ModelPath == "state-b" && snapshot.Engine == modelB && !snapshot.Enabled && snapshot.ModelLoaded && snapshot.ModelType == ModelSVM
				if !validA && !validB {
					select {
					case errors <- "observed a torn ML runtime snapshot":
					default:
					}
					return
				}
			}
		}()
	}
	for iteration := 0; iteration < 500; iteration++ {
		if iteration%2 == 0 {
			replaceMLRuntime(stateB)
		} else {
			replaceMLRuntime(stateA)
		}
	}
	close(done)
	readers.Wait()
	select {
	case message := <-errors:
		t.Fatal(message)
	default:
	}
}

func TestPublishMLRuntimeModelInvalidatesPredictionCache(t *testing.T) {
	previous := snapshotMLRuntime()
	t.Cleanup(func() { replaceMLRuntime(previous) })

	replaceMLRuntime(mlRuntimeSnapshot{Config: DefaultMLConfig(), Enabled: true})
	globalPredictionCache.Set("stale", Prediction{Action: 1, Confidence: 1})
	publishMLRuntimeModel(makeDummyLinearModel(), ModelLogisticRegression)
	if _, exists := globalPredictionCache.Get("stale"); exists {
		t.Fatal("model publication retained a prediction from the previous engine")
	}
	snapshot := snapshotMLRuntime()
	if snapshot.Engine == nil || !snapshot.ModelLoaded || snapshot.ModelType != ModelLogisticRegression {
		t.Fatalf("published runtime = %+v", snapshot)
	}
}

func TestUpdateMLRuntimeConfigPreservesPublishedModel(t *testing.T) {
	previous := snapshotMLRuntime()
	t.Cleanup(func() { replaceMLRuntime(previous) })

	model := makeDummyLinearModel()
	replaceMLRuntime(mlRuntimeSnapshot{
		Engine:      model,
		Config:      MLConfig{ModelPath: "before"},
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	})
	cfg := MLConfig{ModelPath: "after", ModelType: ModelSVM}
	updateMLRuntimeConfig(cfg, false)
	snapshot := snapshotMLRuntime()
	if snapshot.Engine != model || !snapshot.ModelLoaded || snapshot.ModelType != ModelLogisticRegression {
		t.Fatalf("config update replaced published model metadata: %+v", snapshot)
	}
	if snapshot.Config != cfg || snapshot.Enabled {
		t.Fatalf("config update = %+v enabled=%t, want %+v/false", snapshot.Config, snapshot.Enabled, cfg)
	}
}

func TestMLStatusUsesPublishedRuntimeConfig(t *testing.T) {
	const modelPath = "/tmp/published-ml-model.bin"
	status := mlStatusFromRuntime(mlRuntimeSnapshot{
		Config: MLConfig{ModelPath: modelPath},
	})
	if status.GetModelPath() != modelPath {
		t.Fatalf("model path = %q, want published runtime path %q", status.GetModelPath(), modelPath)
	}
}

func BenchmarkSnapshotMLRuntime(b *testing.B) {
	previous := snapshotMLRuntime()
	b.Cleanup(func() { replaceMLRuntime(previous) })
	replaceMLRuntime(mlRuntimeSnapshot{
		Engine:      makeDummyLinearModel(),
		Config:      DefaultMLConfig(),
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	})
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = snapshotMLRuntime()
	}
}
