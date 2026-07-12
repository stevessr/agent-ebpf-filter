package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section model_ensemble.go ----

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
	voting = normalizeEnsembleVoting(voting)
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
	if totalW > 0 {
		for i := range weights {
			weights[i] /= totalW
		}
	}
	return &EnsembleModel{Models: models, Voting: voting, Weights: weights}
}

func (m *EnsembleModel) Type() ModelType { return ModelEnsemble }

func (m *EnsembleModel) Predict(features [FeatureDim]float64) Prediction {
	if len(m.Models) == 0 {
		return Prediction{Action: 0, Confidence: 0, AnomalyScore: 0.5}
	}

	switch normalizeEnsembleVoting(m.Voting) {
	case "hard":
		return m.hardVote(features)
	case "stacked":
		return m.stackedVote(features)
	default:
		return m.softVote(features)
	}
}

func normalizeEnsembleVoting(voting string) string {
	switch strings.ToLower(strings.TrimSpace(voting)) {
	case "hard":
		return "hard"
	case "stacked", "risk_stacked", "risk-stacked":
		return "stacked"
	default:
		return "soft"
	}
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

func (m *EnsembleModel) stackedVote(features [FeatureDim]float64) Prediction {
	soft := m.softVote(features)
	bestRisk := soft
	bestRiskWeight := 0.0
	for i, model := range m.Models {
		pred := model.Predict(features)
		if pred.Action != 1 && pred.Action != 3 {
			continue
		}
		weight := 1.0
		if i < len(m.Weights) {
			weight = m.Weights[i]
		}
		weightedConfidence := pred.Confidence * weight
		if weightedConfidence > bestRiskWeight {
			bestRisk = pred
			bestRiskWeight = weightedConfidence
		}
	}
	if bestRisk.Action == 1 || bestRisk.Action == 3 {
		// Risk-stacked mode favours minority high-risk voters so rare BLOCK/ALERT
		// labels are not drowned out by benign majority votes.
		if bestRisk.Confidence < soft.Confidence && soft.Action == 0 {
			bestRisk.Confidence = math.Max(bestRisk.Confidence, soft.Confidence*0.75)
		}
		bestRisk.AnomalyScore = math.Max(bestRisk.AnomalyScore, 1-bestRisk.Confidence)
		return bestRisk
	}
	return soft
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
	raw, err := readBoundedMLModelFile(path, mlEnsembleManifestMaxBytes)
	if err != nil {
		return nil, err
	}
	var manifest ensembleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("invalid ensemble manifest: %w", err)
	}
	if manifest.Version != mlBinaryModelVersion {
		return nil, fmt.Errorf("unsupported ensemble version %d", manifest.Version)
	}
	memberCount := len(manifest.ModelFiles)
	if memberCount > mlMaxEnsembleMembers {
		return nil, fmt.Errorf("ensemble has %d members; maximum is %d", memberCount, mlMaxEnsembleMembers)
	}
	if len(manifest.ModelTypes) != memberCount || len(manifest.Weights) != memberCount {
		return nil, fmt.Errorf("ensemble manifest member metadata length mismatch")
	}
	if voting := strings.ToLower(strings.TrimSpace(manifest.Voting)); voting != "soft" && voting != "hard" && voting != "stacked" {
		return nil, fmt.Errorf("invalid ensemble voting mode %q", manifest.Voting)
	}
	totalWeight := 0.0
	for index, weight := range manifest.Weights {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 {
			return nil, fmt.Errorf("invalid ensemble weight %d", index)
		}
		totalWeight += weight
	}
	if memberCount > 0 && (totalWeight <= 0 || math.IsInf(totalWeight, 0)) {
		return nil, fmt.Errorf("ensemble weights must have a positive sum")
	}
	baseDir := filepath.Dir(path)
	models := make([]Model, 0, memberCount)
	for i, file := range manifest.ModelFiles {
		file = strings.TrimSpace(file)
		if file == "" || len(file) > 255 || filepath.IsAbs(file) || filepath.Base(file) != file || strings.ContainsAny(file, `/\\`) {
			return nil, fmt.Errorf("invalid ensemble model file %q", file)
		}
		if len(manifest.ModelTypes[i]) > 128 {
			return nil, fmt.Errorf("ensemble model type %d is too long", i)
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
