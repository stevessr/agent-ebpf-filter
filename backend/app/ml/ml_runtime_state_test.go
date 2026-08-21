package ml

import (
	"sync"
	"testing"
)

func TestMLRuntimeSnapshotsPublishModelAndMetadataAtomically(t *testing.T) {
	previous := SnapshotMLRuntime()
	t.Cleanup(func() { ReplaceMLRuntime(previous) })

	modelA := makeDummyLinearModel()
	modelB := makeDummyLinearModel()
	stateA := MLRuntimeSnapshot{
		Engine:      modelA,
		Config:      MLConfig{ModelPath: "state-a"},
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	}
	stateB := MLRuntimeSnapshot{
		Engine:      modelB,
		Config:      MLConfig{ModelPath: "state-b"},
		Enabled:     false,
		ModelLoaded: true,
		ModelType:   ModelSVM,
	}
	ReplaceMLRuntime(stateA)

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
				snapshot := SnapshotMLRuntime()
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
			ReplaceMLRuntime(stateB)
		} else {
			ReplaceMLRuntime(stateA)
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
	previous := SnapshotMLRuntime()
	t.Cleanup(func() { ReplaceMLRuntime(previous) })

	ReplaceMLRuntime(MLRuntimeSnapshot{Config: DefaultMLConfig(), Enabled: true})
	globalPredictionCache.Set("stale", Prediction{Action: 1, Confidence: 1})
	PublishMLRuntimeModel(makeDummyLinearModel(), ModelLogisticRegression)
	if _, exists := globalPredictionCache.Get("stale"); exists {
		t.Fatal("model publication retained a prediction from the previous engine")
	}
	snapshot := SnapshotMLRuntime()
	if snapshot.Engine == nil || !snapshot.ModelLoaded || snapshot.ModelType != ModelLogisticRegression {
		t.Fatalf("published runtime = %+v", snapshot)
	}
}

func TestUpdateMLRuntimeConfigPreservesPublishedModel(t *testing.T) {
	previous := SnapshotMLRuntime()
	t.Cleanup(func() { ReplaceMLRuntime(previous) })

	model := makeDummyLinearModel()
	ReplaceMLRuntime(MLRuntimeSnapshot{
		Engine:      model,
		Config:      MLConfig{ModelPath: "before"},
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	})
	cfg := MLConfig{ModelPath: "after", ModelType: ModelSVM}
	UpdateMLRuntimeConfig(cfg, false)
	snapshot := SnapshotMLRuntime()
	if snapshot.Engine != model || !snapshot.ModelLoaded || snapshot.ModelType != ModelLogisticRegression {
		t.Fatalf("config update replaced published model metadata: %+v", snapshot)
	}
	if snapshot.Config != cfg || snapshot.Enabled {
		t.Fatalf("config update = %+v enabled=%t, want %+v/false", snapshot.Config, snapshot.Enabled, cfg)
	}
}


func BenchmarkSnapshotMLRuntime(b *testing.B) {
	previous := SnapshotMLRuntime()
	b.Cleanup(func() { ReplaceMLRuntime(previous) })
	ReplaceMLRuntime(MLRuntimeSnapshot{
		Engine:      makeDummyLinearModel(),
		Config:      DefaultMLConfig(),
		Enabled:     true,
		ModelLoaded: true,
		ModelType:   ModelLogisticRegression,
	})
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = SnapshotMLRuntime()
	}
}

func makeDummyLinearModel() *LogisticModel {
	model := &LogisticModel{NumClasses: 4}
	model.Weights = make([][FeatureDim + 1]float64, model.NumClasses)
	for c := 0; c < model.NumClasses; c++ {
		for d := 0; d <= FeatureDim; d++ {
			model.Weights[c][d] = float64((c+1)*(d+3)) / 1000.0
		}
	}
	return model
}
