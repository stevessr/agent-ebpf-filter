package ml

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newGANTransformerTestStore(t testing.TB, perClass int) *TrainingDataStore {
	t.Helper()
	store := NewTrainingDataStore(ganTransformerClasses*perClass + 16)
	for c := 0; c < ganTransformerClasses; c++ {
		for i := 0; i < perClass; i++ {
			store.Add(TrainingSample{
				Features:    ganTransformerTestFeatures(c, i),
				Label:       int32(c),
				Comm:        fmt.Sprintf("gan-class-%d", c),
				CommandLine: fmt.Sprintf("gan-class-%d sample-%d", c, i),
				Timestamp:   time.Now(),
				UserLabel:   "test",
			})
		}
	}
	return store
}

func ganTransformerTestFeatures(classIdx, sampleIdx int) [FeatureDim]float64 {
	var features [FeatureDim]float64
	for d := 0; d < FeatureDim; d++ {
		block := (d / 8) % ganTransformerClasses
		value := 0.12
		if block == classIdx {
			value = 0.82
		}
		if d%ganTransformerClasses == classIdx {
			value += 0.08
		}
		jitter := float64((sampleIdx*(d+3)+classIdx*11+d)%13) / 200.0
		features[d] = Clamp01(value + jitter)
	}
	return features
}

func TestGANTransformerTrainPredictSerializeAndPerformance(t *testing.T) {
	store := newGANTransformerTestStore(t, 32)
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelGANTransformer
	cfg.NumTrees = 16
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 3
	cfg.ValidationSplitRatio = 0.25

	GlobalTrainer.ResetCancel()
	trainStart := time.Now()
	model, result := GlobalTrainer.TrainWithConfig(store, cfg)
	trainDuration := time.Since(trainStart)
	if result.Error != "" {
		t.Fatalf("TrainWithConfig failed: %s", result.Error)
	}
	if model == nil {
		t.Fatal("nil GAN Transformer model")
	}
	if model.Type() != ModelGANTransformer {
		t.Fatalf("wrong model type: %s", model.Type())
	}
	if result.TrainSamples == 0 || result.ValidationSamples == 0 {
		t.Fatalf("expected train/validation split, got %+v", result)
	}

	labeled := store.LabeledSamples()
	pred := model.Predict(labeled[0].Features)
	if pred.Action < 0 || pred.Action > 3 {
		t.Fatalf("invalid action: %+v", pred)
	}
	if pred.Confidence <= 0 || pred.Confidence > 1 {
		t.Fatalf("invalid confidence: %+v", pred)
	}

	gan, ok := UnwrapModelType(model).(*GANTransformerModel)
	if !ok {
		t.Fatalf("expected GANTransformerModel, got %T", UnwrapModelType(model))
	}
	generated := gan.Generate(0, 7)
	for i, value := range generated {
		if value < 0 || value > 1 {
			t.Fatalf("generated feature %d out of range: %f", i, value)
		}
	}

	duration, throughput, latencyMs, predictions := BenchmarkModelInference(model, labeled)
	if predictions == 0 || throughput <= 0 || latencyMs <= 0 || duration <= 0 {
		t.Fatalf("invalid performance metrics: duration=%f throughput=%f latency=%f predictions=%d",
			duration, throughput, latencyMs, predictions)
	}
	t.Logf("GAN+Transformer performance: train=%s trainAcc=%.2f%% valAcc=%.2f%% throughput=%.0f/s latency=%.4fms predictions=%d",
		trainDuration.Round(time.Millisecond), result.TrainAccuracy*100, result.ValidationAccuracy*100, throughput, latencyMs, predictions)

	path := filepath.Join(t.TempDir(), "gan_transformer.json")
	if err := model.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	loaded, err := DeserializeGANTransformer(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.Type() != ModelGANTransformer {
		t.Fatalf("loaded wrong type: %s", loaded.Type())
	}
	loadedPred := loaded.Predict(labeled[0].Features)
	if loadedPred.Action != pred.Action {
		t.Fatalf("prediction changed after reload: %d vs %d", pred.Action, loadedPred.Action)
	}
}

func BenchmarkGANTransformerPredict(b *testing.B) {
	store := newGANTransformerTestStore(b, 32)
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelGANTransformer
	cfg.NumTrees = 16
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 3
	model, result := GlobalTrainer.TrainWithConfig(store, cfg)
	if result.Error != "" {
		b.Fatalf("train: %s", result.Error)
	}
	samples := store.LabeledSamples()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.Predict(samples[i%len(samples)].Features)
	}
}
