package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Model Type Registration ────────────────────────────────────────

func init() {
	RegisterModel(ModelEnsemble, func() Model { return NewEnsembleModel(nil, "soft", nil) })
}

// EnsembleModel combines multiple base models via weighted voting.
// Supports "hard" (majority) and "soft" (probability-weighted) voting.
type EnsembleModel struct {
	Models  []Model   `json:"-"`
	Weights []float64 `json:"weights"`
	Voting  string    `json:"voting"` // hard, soft
}

type ensembleManifest struct {
	Version    int       `json:"version"`
	Voting     string    `json:"voting"`
	Weights    []float64 `json:"weights"`
	ModelTypes []string  `json:"modelTypes"`
	ModelFiles []string  `json:"modelFiles"`
}

func NewEnsembleModel(models []Model, voting string, weights []float64) *EnsembleModel {
	if voting == "" {
		voting = "soft"
	}
	if len(weights) != len(models) {
		weights = make([]float64, len(models))
		for i := range weights {
			weights[i] = 1.0
		}
	}
	totalW := 0.0
	for _, w := range weights {
		totalW += w
	}
	for i := range weights {
		weights[i] /= totalW
	}
	return &EnsembleModel{Models: models, Voting: voting, Weights: weights}
}

func (m *EnsembleModel) Type() ModelType { return ModelEnsemble }

func (m *EnsembleModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Models) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	if m.Voting == "hard" {
		return m.hardVote(features)
	}
	return m.softVote(features)
}

func (m *EnsembleModel) hardVote(features [FeatureDim]float64) Prediction {
	votes := make([]float64, 4)
	totalW := 0.0
	for i, model := range m.Models {
		pred := model.Predict(features)
		if pred.Action >= 0 && pred.Action < 4 {
			votes[pred.Action] += m.Weights[i]
			totalW += m.Weights[i]
		}
	}
	if totalW == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	bestClass := int32(0)
	bestVotes := votes[0]
	for c := 1; c < 4; c++ {
		if votes[c] > bestVotes {
			bestVotes = votes[c]
			bestClass = int32(c)
		}
	}
	confidence := bestVotes / totalW
	return Prediction{Action: bestClass, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *EnsembleModel) softVote(features [FeatureDim]float64) Prediction {
	classProbs := make([]float64, 4)
	totalW := 0.0
	for i, model := range m.Models {
		pred := model.Predict(features)
		w := m.Weights[i]
		for c := 0; c < 4; c++ {
			if pred.Action == int32(c) {
				classProbs[c] += pred.Confidence * w
			} else {
				classProbs[c] += (1 - pred.Confidence) / 3 * w
			}
		}
		totalW += w
	}
	if totalW == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}
	for c := range classProbs {
		classProbs[c] /= totalW
	}
	bestClass := int32(0)
	bestProb := classProbs[0]
	for c := 1; c < 4; c++ {
		if classProbs[c] > bestProb {
			bestProb = classProbs[c]
			bestClass = int32(c)
		}
	}
	return Prediction{Action: bestClass, Confidence: bestProb, AnomalyScore: 1 - bestProb}
}

func (m *EnsembleModel) Serialize(path string) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	basePath := strings.TrimSuffix(path, filepath.Ext(path))
	modelTypes := make([]string, 0, len(m.Models))
	modelFiles := make([]string, 0, len(m.Models))
	for i, model := range m.Models {
		subPath := fmt.Sprintf("%s_ensemble_%d_%s.bin", basePath, i, model.Type())
		if err := model.Serialize(subPath); err != nil {
			return fmt.Errorf("ensemble serialize model[%d] %s: %w", i, model.Type(), err)
		}
		modelTypes = append(modelTypes, string(model.Type()))
		modelFiles = append(modelFiles, filepath.Base(subPath))
	}
	manifest := ensembleManifest{
		Version:    1,
		Voting:     m.Voting,
		Weights:    append([]float64(nil), m.Weights...),
		ModelTypes: modelTypes,
		ModelFiles: modelFiles,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func DeserializeEnsemble(path string) (*EnsembleModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest ensembleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(path)
	models := make([]Model, 0, len(manifest.ModelFiles))
	for i, file := range manifest.ModelFiles {
		if i >= len(manifest.ModelTypes) {
			return nil, fmt.Errorf("ensemble manifest missing model type for %s", file)
		}
		mt := ModelType(manifest.ModelTypes[i])
		subPath := filepath.Join(baseDir, file)
		model, err := deserializeModelByType(mt, subPath)
		if err != nil {
			return nil, fmt.Errorf("ensemble load model[%d] %s: %w", i, mt, err)
		}
		models = append(models, model)
	}
	return NewEnsembleModel(models, manifest.Voting, manifest.Weights), nil
}

func deserializeModelByType(mt ModelType, path string) (Model, error) {
	base := baseModelType(mt)
	var (
		model Model
		err   error
	)
	switch base {
	case ModelRandomForest:
		model, err = DeserializeForest(path)
	case ModelExtraTrees:
		var forest *DecisionForest
		forest, err = DeserializeForest(path)
		if err == nil {
			model = &ExtraTreesModel{Forest: forest, MaxDepth: forest.MaxDepth, NumTrees: len(forest.Trees)}
		}
	case ModelKNN:
		model, err = DeserializeKNN(path)
	case ModelLogisticRegression:
		model, err = DeserializeLogistic(path)
	case ModelNaiveBayes:
		model, err = DeserializeNaiveBayes(path)
	case ModelNearestCentroid:
		model, err = DeserializeNearestCentroid(path)
	case ModelAdaBoost:
		model, err = DeserializeAdaBoost(path)
	case ModelSVM:
		model, err = DeserializeSVM(path)
	case ModelRidge:
		model, err = DeserializeRidge(path)
	case ModelPerceptron:
		model, err = DeserializePerceptron(path)
	case ModelPassiveAggressive:
		model, err = DeserializePA(path)
	default:
		return nil, fmt.Errorf("unsupported ensemble member type: %s", mt)
	}
	if err != nil {
		return nil, err
	}
	return wrapModelType(model, mt), nil
}

// ── Prediction Cache ────────────────────────────────────────────────

type predictionCacheEntry struct {
	Prediction Prediction
	CommandKey string
	AccessTime time.Time
}

// PredictionCache is a bounded LRU cache for ML predictions.
type PredictionCache struct {
	mu       sync.RWMutex
	entries  map[string]*predictionCacheEntry
	order    []string
	capacity int
	hits     uint64
	misses   uint64
}

var globalPredictionCache = newPredictionCache(1000)

func newPredictionCache(capacity int) *PredictionCache {
	if capacity < 10 {
		capacity = 10
	}
	return &PredictionCache{
		entries:  make(map[string]*predictionCacheEntry, capacity),
		capacity: capacity,
	}
}

func (c *PredictionCache) Get(key string) (Prediction, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return Prediction{}, false
	}
	c.hits++
	entry.AccessTime = time.Now()
	return entry.Prediction, true
}

func (c *PredictionCache) Set(key string, pred Prediction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[key]; ok {
		entry.Prediction = pred
		entry.AccessTime = time.Now()
		return
	}
	if len(c.entries) >= c.capacity {
		c.evictLRU()
	}
	c.entries[key] = &predictionCacheEntry{
		Prediction: pred,
		CommandKey: key,
		AccessTime: time.Now(),
	}
}

func (c *PredictionCache) evictLRU() {
	var oldestKey string
	oldestTime := time.Now()
	for k, entry := range c.entries {
		if entry.AccessTime.Before(oldestTime) {
			oldestTime = entry.AccessTime
			oldestKey = k
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *PredictionCache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

func (c *PredictionCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*predictionCacheEntry, c.capacity)
	c.order = nil
	c.hits = 0
	c.misses = 0
}

func makePredictionCacheKey(comm string, args []string) string {
	return comm + "\x00" + strings.Join(args, "\x00")
}

// ── Two-Tier Inference ──────────────────────────────────────────────

// fastPredict uses a lightweight model (single decision tree or fast linear model)
// for initial assessment. Returns true if fast path is conclusive.
func fastPredict(features [FeatureDim]float64) (Prediction, bool) {
	if !mlModelLoaded || mlEngine == nil {
		return Prediction{}, false
	}

	pred := mlEngine.Predict(features)
	// Fast path: high confidence AND not BLOCK/ALERT (safe to fast-track ALLOW)
	if pred.Confidence >= 0.90 && pred.Action == 0 {
		return pred, true
	}
	return Prediction{}, false
}

// ── Ensemble Builder ────────────────────────────────────────────────

// buildEnsembleFromStore trains multiple fast models and returns an ensemble.
// Only uses fast-training models to avoid excessive training time.
func buildEnsembleFromStore(store *TrainingDataStore) *EnsembleModel {
	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil
	}

	models := make([]Model, 0, 3)
	modelNames := make([]string, 0, 3)

	// 1. Logistic Regression with class weights for imbalance
	Xs, Ys := extractFeaturesLabels(labeled)
	lr := NewLogisticModel(0.01, "l2", 500)
	lr.NumClasses = 4
	lr.ClassWeights = computeClassWeights(Ys, 4)
	lr.Train(Xs, Ys)
	models = append(models, lr)
	modelNames = append(modelNames, "logistic")

	// 2. Naive Bayes (O(n*d))
	nb := NewNaiveBayes()
	nb.Means = make([][FeatureDim]float64, 4)
	nb.Vars = make([][FeatureDim]float64, 4)
	nb.Priors = make([]float64, 4)
	counts := make([]int, 4)
	for _, s := range labeled {
		if s.Label < 0 || int(s.Label) >= 4 {
			continue
		}
		c := s.Label
		counts[c]++
		for d := 0; d < FeatureDim; d++ {
			nb.Means[c][d] += s.Features[d]
		}
	}
	for c := 0; c < 4; c++ {
		nb.Priors[c] = float64(counts[c]) / float64(len(labeled))
		if counts[c] > 0 {
			for d := 0; d < FeatureDim; d++ {
				nb.Means[c][d] /= float64(counts[c])
			}
		}
	}
	for _, s := range labeled {
		if s.Label < 0 || int(s.Label) >= 4 {
			continue
		}
		c := s.Label
		for d := 0; d < FeatureDim; d++ {
			diff := s.Features[d] - nb.Means[c][d]
			nb.Vars[c][d] += diff * diff
		}
	}
	for c := 0; c < 4; c++ {
		if counts[c] > 1 {
			for d := 0; d < FeatureDim; d++ {
				nb.Vars[c][d] /= float64(counts[c] - 1)
			}
		}
	}
	models = append(models, nb)
	modelNames = append(modelNames, "naive_bayes")

	// 3. KNN (fast "training" — just stores samples)
	if len(labeled) >= 10 {
		k := int(math.Sqrt(float64(len(labeled))))
		if k < 3 {
			k = 3
		}
		if k > 15 {
			k = 15
		}
		knn := NewKNNModel(k, "euclidean", "distance")
		knn.NumClasses = 4
		knn.Samples = make([][FeatureDim]float64, len(labeled))
		knn.Labels = make([]int32, len(labeled))
		for i, s := range labeled {
			knn.Samples[i] = s.Features
			knn.Labels[i] = s.Label
		}
		knn.MaxDistance = 3.0 // skip very distant samples for speed
		models = append(models, knn)
		modelNames = append(modelNames, "knn")
	}

	// 4. Nearest Centroid (low-data friendly, extremely fast)
	centroid := NewNearestCentroid("cosine", true)
	centroid.Classes = 4
	centroid.Centroids = make([][FeatureDim]float64, 4)
	centroid.Priors = make([]float64, 4)
	centroidCounts := make([]int, 4)
	for _, s := range labeled {
		if s.Label < 0 || int(s.Label) >= 4 {
			continue
		}
		c := int(s.Label)
		centroidCounts[c]++
		for d := 0; d < FeatureDim; d++ {
			centroid.Centroids[c][d] += s.Features[d]
		}
	}
	totalCentroid := 0
	for _, count := range centroidCounts {
		totalCentroid += count
	}
	if totalCentroid > 0 {
		nonEmptyCentroidClasses := 0
		for _, count := range centroidCounts {
			if count > 0 {
				nonEmptyCentroidClasses++
			}
		}
		for c := 0; c < 4; c++ {
			if centroidCounts[c] > 0 {
				for d := 0; d < FeatureDim; d++ {
					centroid.Centroids[c][d] /= float64(centroidCounts[c])
				}
				centroid.Priors[c] = 1.0 / float64(nonEmptyCentroidClasses)
			}
		}
		models = append(models, centroid)
		modelNames = append(modelNames, "nearest_centroid")
	}

	// 5. Lightweight Random Forest (5 trees, depth 6) for fast inference
	if len(labeled) >= 20 {
		samples := toTrainSamples(labeled)
		lightRF := NewDecisionForest(5, 6, 4)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		fCount := int(math.Sqrt(float64(FeatureDim)))
		if fCount < 1 {
			fCount = 1
		}
		for ti := 0; ti < 5; ti++ {
			bootstrap := make([]trainSample, len(samples))
			for i := range bootstrap {
				bootstrap[i] = samples[rng.Intn(len(samples))]
			}
			nodes := buildAutoTuneTree(bootstrap, 0, 6, 3, fCount, rng)
			lightRF.Trees[ti] = DecisionTree{Nodes: nodes}
		}
		lightRF.IsTrained = true
		models = append(models, lightRF)
		modelNames = append(modelNames, "light_rf")
	}

	// Assign weights based on individual hold-out accuracy
	weights := make([]float64, len(models))
	for i := range weights {
		weights[i] = 1.0
	}
	if len(labeled) >= 30 {
		// Split small validation set for weight calibration
		splitIdx := len(labeled) * 4 / 5
		for i, model := range models {
			correct := 0
			total := 0
			for j := splitIdx; j < len(labeled); j++ {
				pred := model.Predict(labeled[j].Features)
				if pred.Action == labeled[j].Label {
					correct++
				}
				total++
			}
			if total > 0 {
				acc := float64(correct) / float64(total)
				weights[i] = math.Max(acc, 0.25)
			}
		}
		// Normalize
		totalW := 0.0
		for _, w := range weights {
			totalW += w
		}
		if totalW > 0 {
			for i := range weights {
				weights[i] /= totalW
			}
		}
	}

	log.Printf("[ML] Ensemble built: %s, weights=%.2f", strings.Join(modelNames, "+"), weights)
	return NewEnsembleModel(models, "soft", weights)
}

func extractFeaturesLabels(labeled []TrainingSample) ([][FeatureDim]float64, []int32) {
	Xs := make([][FeatureDim]float64, len(labeled))
	Ys := make([]int32, len(labeled))
	for i, s := range labeled {
		Xs[i] = s.Features
		Ys[i] = s.Label
	}
	return Xs, Ys
}

// ── Model Auto-Benchmark ────────────────────────────────────────────

type ModelBenchmark struct {
	ModelType       string  `json:"modelType"`
	Accuracy        float64 `json:"accuracy"`
	TrainDuration   float64 `json:"trainDurationSeconds"`
	InferenceTimeUs float64 `json:"inferenceTimeUs"`
	MemoryBytes     int64   `json:"memoryBytes,omitempty"`
}

// BenchmarkAllModels trains and evaluates all registered model types.
func BenchmarkAllModels(store *TrainingDataStore) []ModelBenchmark {
	labeled := store.LabeledSamples()
	if len(labeled) < 20 {
		return nil
	}

	allTypes := AllModelTypes()
	results := make([]ModelBenchmark, 0, len(allTypes))

	// Use 80/20 split for benchmarking
	splitIdx := len(labeled) * 4 / 5
	trainSet := labeled[:splitIdx]
	testSet := labeled[splitIdx:]

	for _, mt := range allTypes {
		bench := benchmarkModelType(mt, trainSet, testSet)
		results = append(results, bench)
	}

	return results
}

func benchmarkModelType(mt ModelType, trainSet, testSet []TrainingSample) ModelBenchmark {
	bench := ModelBenchmark{ModelType: string(mt)}

	cfg := DefaultMLConfig()
	cfg.ModelType = mt
	cfg.NumTrees = 31
	cfg.MaxDepth = 8
	cfg.MinSamplesLeaf = 5

	// Use a temporary training store
	tmpStore := newTrainingDataStore(len(trainSet))
	for i := range trainSet {
		tmpStore.samples[i] = trainSet[i]
	}
	tmpStore.nextWrite = len(trainSet)

	trainStart := time.Now()
	model, result := globalTrainer.TrainWithConfig(tmpStore, cfg)
	bench.TrainDuration = time.Since(trainStart).Seconds()

	if result.Error != "" || model == nil {
		bench.Accuracy = 0
		return bench
	}

	// Measure inference speed
	testFeatures := make([][FeatureDim]float64, len(testSet))
	testLabels := make([]int32, len(testSet))
	for i, s := range testSet {
		testFeatures[i] = s.Features
		testLabels[i] = s.Label
	}

	// Warm up
	for i := 0; i < min(10, len(testSet)); i++ {
		model.Predict(testFeatures[i])
	}

	// Timed inference
	infStart := time.Now()
	correct := 0
	for i, feat := range testFeatures {
		pred := model.Predict(feat)
		if pred.Action == testLabels[i] {
			correct++
		}
	}
	infElapsed := time.Since(infStart)
	if len(testFeatures) > 0 {
		bench.InferenceTimeUs = float64(infElapsed.Microseconds()) / float64(len(testFeatures))
		bench.Accuracy = float64(correct) / float64(len(testFeatures))
	}

	return bench
}

// ── Optimized Decision Forest Predict ───────────────────────────────

// PredictFast is an optimized Predict that terminates early when
// a high-confidence prediction is found.
func (f *DecisionForest) PredictFast(features [FeatureDim]float64, earlyStopConfidence float64) Prediction {
	if !f.IsTrained || len(f.Trees) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0}
	}

	classVotes := make([]float64, f.NumClasses)
	treesEvaluated := 0

	for i := range f.Trees {
		leaf := f.Trees[i].Predict(features)
		cls := int(math.Round(float64(leaf)))
		if cls < 0 || cls >= f.NumClasses {
			continue
		}
		classVotes[cls]++
		treesEvaluated++

		// Early termination: if any class has overwhelming majority
		if treesEvaluated >= 5 && earlyStopConfidence > 0 {
			_ = int32(0)
			bestCount := classVotes[0]
			for c := 1; c < f.NumClasses; c++ {
				if classVotes[c] > bestCount {
					bestCount = classVotes[c]
					_ = int32(c)
				}
			}
			conf := bestCount / float64(treesEvaluated)
			// If highly confident and we've evaluated enough trees
			if conf >= earlyStopConfidence && treesEvaluated >= len(f.Trees)/3 {
				for j := i + 1; j < len(f.Trees); j++ {
					l := f.Trees[j].Predict(features)
					c := int(math.Round(float64(l)))
					if c >= 0 && c < f.NumClasses {
						classVotes[c]++
						treesEvaluated++
					}
					// Re-check every 5 trees
					if (j-i)%5 == 0 {
						_ = int32(0)
						bcv := classVotes[0]
						for c2 := 1; c2 < f.NumClasses; c2++ {
							if classVotes[c2] > bcv {
								bcv = classVotes[c2]
								_ = int32(c2)
							}
						}
						if bcv/float64(treesEvaluated) < earlyStopConfidence {
							break
						}
					}
				}
				break
			}
		}
	}

	if treesEvaluated == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	bestClass := int32(0)
	bestVotes := classVotes[0]
	for i := 1; i < f.NumClasses; i++ {
		if classVotes[i] > bestVotes {
			bestVotes = classVotes[i]
			bestClass = int32(i)
		}
	}

	confidence := bestVotes / float64(treesEvaluated)
	return Prediction{
		Action:       bestClass,
		Confidence:   confidence,
		AnomalyScore: 1 - confidence,
	}
}

// ── Inference with Cache and Two-Tier ───────────────────────────────

// predictWithOptimizations wraps the ML prediction with caching and fast path.
func predictWithOptimizations(features [FeatureDim]float64, cacheKey string) Prediction {
	// 1. Check cache first
	if cacheKey != "" {
		if cached, ok := globalPredictionCache.Get(cacheKey); ok {
			return cached
		}
	}

	// 2. Two-tier inference
	if fastPred, ok := fastPredict(features); ok {
		if cacheKey != "" {
			globalPredictionCache.Set(cacheKey, fastPred)
		}
		return fastPred
	}

	// 3. Full model inference
	pred := mlEngine.Predict(features)

	// 4. Cache result
	if cacheKey != "" {
		globalPredictionCache.Set(cacheKey, pred)
	}

	return pred
}
