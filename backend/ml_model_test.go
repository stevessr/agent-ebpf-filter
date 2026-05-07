package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// ── Helpers ────────────────────────────────────────────────────────

func seedRand() *rand.Rand { return rand.New(rand.NewSource(42)) }

func initMLTest(t *testing.T, nSamples int) {
	t.Helper()
	InitTrainingStore(100000)
	mlConfig = DefaultMLConfig()
	mlEnabled = true
	globalTrainer.ResetCancel()

	rng := seedRand()
	for i := 0; i < nSamples; i++ {
		var features [FeatureDim]float64
		for d := 0; d < FeatureDim; d++ {
			features[d] = rng.Float64()
		}
		globalTrainingStore.Add(TrainingSample{
			Features: features,
			Label:    int32(i % 4),
			Comm:     fmt.Sprintf("cmd-%d", i%4),
			Args:     []string{fmt.Sprintf("arg-%d", i)},
		})
	}
}

func tmpModelPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name+".bin")
}

// ── Decision Forest ────────────────────────────────────────────────

func TestDecisionForestTrainPredict(t *testing.T) {
	initMLTest(t, 300)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	forest := buildAutoTuneForest(samples, 11, 5, 3, 42)
	if len(forest.Trees) != 11 {
		t.Fatalf("expected 11 trees, got %d", len(forest.Trees))
	}
	if !forest.IsTrained {
		t.Fatal("forest not marked as trained")
	}

	// Predict all samples — should not hang
	correct := 0
	for _, s := range samples {
		pred := forest.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	acc := float64(correct) / float64(len(samples))
	t.Logf("RF accuracy: %.2f%% (%d/%d)", acc*100, correct, len(samples))
	if acc < 0.1 {
		t.Logf("warn: accuracy very low (random features), but test passes if no hang")
	}
}

func TestDecisionForestSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	forest := buildAutoTuneForest(samples, 7, 4, 2, 99)
	path := tmpModelPath(t, "rf_test")

	if err := forest.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeForest(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if len(loaded.Trees) != len(forest.Trees) {
		t.Fatalf("tree count mismatch: %d vs %d", len(loaded.Trees), len(forest.Trees))
	}

	// Verify predictions match
	for i, s := range samples[:10] {
		orig := forest.Predict(s.features)
		reloaded := loaded.Predict(s.features)
		if orig.Action != reloaded.Action {
			t.Errorf("sample %d: action mismatch %d vs %d", i, orig.Action, reloaded.Action)
		}
	}
	t.Logf("RF roundtrip OK: %d trees", len(loaded.Trees))
}

// ── KNN ────────────────────────────────────────────────────────────

func TestKNNPredict(t *testing.T) {
	initMLTest(t, 500)

	labeled := globalTrainingStore.LabeledSamples()
	model := NewKNNModel(5, "euclidean", "uniform")
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	pred := model.Predict(labeled[0].Features)
	if pred.Action < 0 || pred.Action > 3 {
		t.Errorf("invalid action: %d", pred.Action)
	}
	if pred.Confidence < 0 || pred.Confidence > 1 {
		t.Errorf("invalid confidence: %.3f", pred.Confidence)
	}
	t.Logf("KNN predict: action=%d, confidence=%.3f, anomaly=%.3f",
		pred.Action, pred.Confidence, pred.AnomalyScore)

	// Empty model should return safe defaults
	empty := NewKNNModel(3, "euclidean", "uniform")
	emptyPred := empty.Predict(labeled[0].Features)
	if emptyPred.Action != 0 {
		t.Errorf("empty model should return ALLOW(0), got %d", emptyPred.Action)
	}
}

func TestKNNSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	model := NewKNNModel(7, "manhattan", "distance")
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	path := tmpModelPath(t, "knn_test")
	if err := model.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeKNN(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.K != model.K {
		t.Fatalf("K mismatch: %d vs %d", loaded.K, model.K)
	}
	if loaded.Distance != model.Distance {
		t.Fatalf("distance mismatch: %s vs %s", loaded.Distance, model.Distance)
	}
	if len(loaded.Samples) != len(model.Samples) {
		t.Fatalf("sample count mismatch: %d vs %d", len(loaded.Samples), len(model.Samples))
	}

	// Verify predictions match
	for i := 0; i < 10; i++ {
		orig := model.Predict(labeled[i].Features)
		reloaded := loaded.Predict(labeled[i].Features)
		if orig.Action != reloaded.Action {
			t.Errorf("sample %d: action mismatch", i)
		}
	}
	t.Logf("KNN roundtrip OK: k=%d, samples=%d", loaded.K, len(loaded.Samples))
}

func TestKNNDistanceMetrics(t *testing.T) {
	initMLTest(t, 100)
	labeled := globalTrainingStore.LabeledSamples()

	euclidean := NewKNNModel(3, "euclidean", "uniform")
	manhattan := NewKNNModel(3, "manhattan", "uniform")
	for _, m := range []*KNNModel{euclidean, manhattan} {
		m.NumClasses = 4
		m.Samples = make([][FeatureDim]float64, len(labeled))
		m.Labels = make([]int32, len(labeled))
		for i, s := range labeled {
			m.Samples[i] = s.Features
			m.Labels[i] = s.Label
		}
	}

	// Both should produce valid predictions
	ePred := euclidean.Predict(labeled[0].Features)
	mPred := manhattan.Predict(labeled[0].Features)
	t.Logf("euclidean: action=%d conf=%.3f, manhattan: action=%d conf=%.3f",
		ePred.Action, ePred.Confidence, mPred.Action, mPred.Confidence)
}

// ── Logistic Regression ────────────────────────────────────────────

func TestLogisticTrainPredict(t *testing.T) {
	initMLTest(t, 500)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([][FeatureDim]float64, len(labeled))
	labels := make([]int32, len(labeled))
	for i, s := range labeled {
		samples[i] = s.Features
		labels[i] = s.Label
	}

	model := NewLogisticModel(0.01, "l2", 500)
	model.NumClasses = 4
	model.Train(samples, labels)

	if len(model.Weights) != 4 {
		t.Fatalf("expected 4 class weight sets, got %d", len(model.Weights))
	}

	// Predict all samples
	correct := 0
	for i, s := range samples {
		pred := model.Predict(s)
		if pred.Action == labels[i] {
			correct++
		}
	}
	acc := float64(correct) / float64(len(samples))
	t.Logf("Logistic accuracy: %.2f%% (%d/%d)", acc*100, correct, len(samples))
	if acc < 0.1 {
		t.Logf("warn: accuracy low (random features), but training should not hang")
	}
}

func TestLogisticSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([][FeatureDim]float64, len(labeled))
	labels := make([]int32, len(labeled))
	for i, s := range labeled {
		samples[i] = s.Features
		labels[i] = s.Label
	}

	model := NewLogisticModel(0.01, "l2", 300)
	model.NumClasses = 4
	model.Train(samples, labels)

	path := tmpModelPath(t, "lr_test")
	if err := model.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeLogistic(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.NumClasses != model.NumClasses {
		t.Fatalf("class count mismatch: %d vs %d", loaded.NumClasses, model.NumClasses)
	}

	// Verify predictions match
	match := 0
	for i := 0; i < 20; i++ {
		orig := model.Predict(samples[i])
		reloaded := loaded.Predict(samples[i])
		if orig.Action == reloaded.Action {
			match++
		}
	}
	if match < 18 {
		t.Errorf("prediction mismatch: %d/20 match", match)
	}
	t.Logf("Logistic roundtrip OK: classes=%d, match=%d/20", loaded.NumClasses, match)
}

// ── Model Registry ──────────────────────────────────────────────────

func TestModelRegistry(t *testing.T) {
	for _, mt := range AllModelTypes() {
		m, err := NewModel(mt)
		if err != nil {
			t.Fatalf("NewModel(%s): %v", mt, err)
		}
		if m == nil {
			t.Fatalf("NewModel(%s) returned nil", mt)
		}
		if m.Type() != mt {
			t.Fatalf("type mismatch: expected %s, got %s", mt, m.Type())
		}
		t.Logf("Model %s: type=%s ✓", modelName(mt), m.Type())
	}

	_, err := NewModel("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown model type")
	}
}

func TestModelTypeNames(t *testing.T) {
	if modelName(ModelRandomForest) == string(ModelRandomForest) {
		t.Error("modelName should return human-readable name, not the constant")
	}
	types := AllModelTypes()
	if len(types) < 3 {
		t.Fatalf("expected at least 3 model types, got %d", len(types))
	}
}

func TestBuiltinModelCatalogCoversAllModelTypes(t *testing.T) {
	types := AllModelTypes()
	catalog := BuiltinModelCatalog()
	if len(types) < 30 {
		t.Fatalf("expected at least 30 built-in model profiles, got %d", len(types))
	}
	if len(catalog) != len(types) {
		t.Fatalf("catalog/model type mismatch: %d catalog vs %d model types", len(catalog), len(types))
	}
	seen := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		if item.Value == "" || item.Label == "" || item.Base == "" || item.Category == "" {
			t.Fatalf("incomplete catalog item: %+v", item)
		}
		seen[item.Value] = true
	}
	for _, mt := range types {
		if !seen[string(mt)] {
			t.Fatalf("model type %s missing from built-in catalog", mt)
		}
	}
}

// ── Training Data Store ─────────────────────────────────────────────

func TestTrainingStoreAddAndQuery(t *testing.T) {
	store := initTestStore(500)
	total, labeled := store.Status()
	t.Logf("Store: total=%d, labeled=%d", total, labeled)
	if total < 1 {
		t.Fatal("store should have samples")
	}

	samples := store.LabeledSamples()
	if len(samples) != labeled {
		t.Fatalf("labeled count mismatch: %d vs %d", len(samples), labeled)
	}
}

func initTestStore(n int) *TrainingDataStore {
	InitTrainingStore(n + 10)
	rng := seedRand()
	for i := 0; i < n; i++ {
		var features [FeatureDim]float64
		for d := 0; d < FeatureDim; d++ {
			features[d] = rng.Float64()
		}
		globalTrainingStore.Add(TrainingSample{
			Features:    features,
			Label:       int32(i % 4),
			Comm:        "test",
			CommandLine: fmt.Sprintf("test-%d", i),
		})
	}
	return globalTrainingStore
}

// ── Trainer with different model types ──────────────────────────────

func TestTrainerAllModelTypes(t *testing.T) {
	models := []struct {
		modelType ModelType
		nTrees    int
		maxDepth  int
		minLeaf   int
		balance   bool
	}{
		{ModelRandomForest, 5, 3, 2, false},
		{ModelRandomForestFast, 31, 8, 5, false},
		{ModelKNN, 5, 8, 0, false}, // K uses nTrees, distance uses maxDepth
		{ModelKNNDistance, 31, 8, 5, false},
		{ModelLogisticRegression, 10, 8, 500, true}, // LR=0.01, L2, 500 iters
		{ModelLogisticL1, 31, 8, 5, false},
		{ModelSVM, 5, 8, 500, false},
		{ModelRidge, 5, 8, 5, false},
		{ModelPerceptron, 20, 8, 500, false},
		{ModelPassiveAggressive, 10, 8, 500, false},
		{ModelNearestCentroid, 0, 4, 0, true}, // cosine + uniform-prior path
		{ModelNearestCentroidCosine, 31, 8, 5, false},
		{ModelAdaBoostFast, 31, 8, 5, false},
		{ModelEnsemble, 31, 8, 5, false},
	}

	for _, tc := range models {
		t.Run(string(tc.modelType), func(t *testing.T) {
			initMLTest(t, 300)
			cfg := DefaultMLConfig()
			cfg.ModelType = tc.modelType
			cfg.NumTrees = tc.nTrees
			cfg.MaxDepth = tc.maxDepth
			cfg.MinSamplesLeaf = tc.minLeaf
			cfg.BalanceClasses = tc.balance
			mlConfig = cfg

			globalTrainer.ResetCancel()
			model, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
			if result.Error != "" {
				t.Fatalf("TrainWithConfig failed: %s", result.Error)
			}
			if model == nil {
				t.Fatal("nil model returned")
			}
			if model.Type() != tc.modelType {
				t.Fatalf("wrong type: %s vs %s", model.Type(), tc.modelType)
			}

			// Test prediction doesn't hang
			labeled := globalTrainingStore.LabeledSamples()
			pred := model.Predict(labeled[0].Features)
			t.Logf("%s: accuracy=%.2f%%, pred action=%d conf=%.3f",
				model.Type(), result.Accuracy*100, pred.Action, pred.Confidence)

			// Serialize roundtrip
			path := tmpModelPath(t, string(tc.modelType)+"_trainer")
			if err := model.Serialize(path); err != nil {
				t.Fatalf("serialize: %v", err)
			}
			_, err := os.Stat(path)
			if err != nil {
				t.Fatalf("model file missing: %v", err)
			}
			loaded := tryLoadModel(path, tc.modelType)
			if loaded == nil {
				t.Fatalf("failed to reload %s model from %s", tc.modelType, path)
			}
			if loaded.Type() != tc.modelType {
				t.Fatalf("loaded wrong type: %s vs %s", loaded.Type(), tc.modelType)
			}
			t.Logf("model file: %s exists and reloads", path)
		})
	}
}

// ── Trainer edge cases ──────────────────────────────────────────────

func TestTrainerEmptyStore(t *testing.T) {
	InitTrainingStore(10)
	store := globalTrainingStore
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest

	globalTrainer.ResetCancel()
	_, result := globalTrainer.TrainWithConfig(store, cfg)
	if result.Error == "" {
		t.Fatal("expected error for empty store")
	}
	t.Logf("empty store correctly rejected: %s", result.Error)
}

func TestTrainerDuplicateRun(t *testing.T) {
	initMLTest(t, 200)
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest
	cfg.NumTrees = 3
	mlConfig = cfg

	globalTrainer.ResetCancel()
	// First training should succeed
	_, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result.Error != "" {
		t.Fatalf("first train failed: %s", result.Error)
	}

	// Second should also work (mutex is released by defer)
	globalTrainer.ResetCancel()
	_, result2 := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result2.Error != "" {
		t.Fatalf("second train failed: %s", result2.Error)
	}
	t.Log("consecutive training runs OK")
}
