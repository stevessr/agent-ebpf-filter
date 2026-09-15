package ml

import (
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section ensemble_builder.go ----

const ensembleTrainingSeed int64 = 0x4147454e54

// ── Ensemble Builder ────────────────────────────────────────────────

// BuildEnsembleFromStore trains multiple fast models and returns an ensemble.
// Only uses fast-training models to avoid excessive training time.
func BuildEnsembleFromStore(store *TrainingDataStore) *EnsembleModel {
	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil
	}

	// Keep the newest tail completely unseen by the base learners. The previous
	// implementation trained on all labeled samples and then called the last 20%
	// a validation set, which leaked validation examples into every model.
	trainSet, validationSet := splitEnsembleCalibrationHoldout(labeled)

	models := make([]Model, 0, 5)
	modelNames := make([]string, 0, 5)

	// 1. Logistic Regression with class weights for imbalance
	Xs, Ys := extractFeaturesLabels(trainSet)
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
	for _, s := range trainSet {
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
		nb.Priors[c] = float64(counts[c]) / float64(len(trainSet))
		if counts[c] > 0 {
			for d := 0; d < FeatureDim; d++ {
				nb.Means[c][d] /= float64(counts[c])
			}
		}
	}
	for _, s := range trainSet {
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
	if len(trainSet) >= 10 {
		k := int(math.Sqrt(float64(len(trainSet))))
		if k < 3 {
			k = 3
		}
		if k > 15 {
			k = 15
		}
		knn := NewKNNModel(k, "euclidean", "distance")
		knn.NumClasses = 4
		knn.Samples = make([][FeatureDim]float64, len(trainSet))
		knn.Labels = make([]int32, len(trainSet))
		for i, s := range trainSet {
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
	for _, s := range trainSet {
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

	// 5. Lightweight Random Forest (5 trees, depth 6) for fast inference.
	// A fixed local seed makes retraining reproducible without touching the
	// process-global PRNG or changing the online inference path.
	if len(trainSet) >= 20 {
		samples := ToTrainSamples(trainSet)
		lightRF := NewDecisionForest(5, 6, 4)
		rng := rand.New(rand.NewSource(ensembleTrainingSeed))
		fCount := int(math.Sqrt(float64(FeatureDim)))
		if fCount < 1 {
			fCount = 1
		}
		for ti := 0; ti < 5; ti++ {
			bootstrap := make([]TrainSample, len(samples))
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

	// Assign weights from a genuinely unseen hold-out. Accuracy alone is a poor
	// security objective, so weight calibration combines balanced accuracy,
	// BLOCK/ALERT recall, benign false-positive rate, and confidence reliability.
	weights := make([]float64, len(models))
	for i := range weights {
		weights[i] = 1.0
	}
	if len(validationSet) > 0 {
		for i, model := range models {
			weights[i] = calibratedEnsembleWeight(evaluateRiskModel(model, validationSet))
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
	}

	log.Printf("[ML] Ensemble built: %s, train=%d, calibration=%d, weights=%.2f", strings.Join(modelNames, "+"), len(trainSet), len(validationSet), weights)
	return NewEnsembleModel(models, "soft", weights)
}

func splitEnsembleCalibrationHoldout(labeled []TrainingSample) (trainSet, validationSet []TrainingSample) {
	ordered := chronologicalTrainingSamples(labeled)
	if len(ordered) < 30 {
		return ordered, nil
	}
	splitIdx := len(ordered) * 4 / 5
	if splitIdx < 10 || splitIdx >= len(ordered) {
		return ordered, nil
	}
	return ordered[:splitIdx], ordered[splitIdx:]
}

func chronologicalTrainingSamples(samples []TrainingSample) []TrainingSample {
	ordered := append([]TrainingSample(nil), samples...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].Timestamp
		right := ordered[j].Timestamp
		if left.IsZero() != right.IsZero() {
			// Unknown timestamps stay on the training side rather than entering the
			// newest validation window and pretending to be recent observations.
			return left.IsZero()
		}
		return left.Before(right)
	})
	return ordered
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
	ModelType               string  `json:"modelType"`
	Accuracy                float64 `json:"accuracy"`
	BalancedAccuracy        float64 `json:"balancedAccuracy"`
	HighRiskRecall          float64 `json:"highRiskRecall"`
	BenignFalsePositiveRate float64 `json:"benignFalsePositiveRate"`
	ConfidenceBrier         float64 `json:"confidenceBrier"`
	ECE                     float64 `json:"expectedCalibrationError"`
	TrainDuration           float64 `json:"trainDurationSeconds"`
	InferenceTimeUs         float64 `json:"inferenceTimeUs"`
	MemoryBytes             int64   `json:"memoryBytes,omitempty"`
}

// BenchmarkAllModels trains and evaluates all registered model types.
func BenchmarkAllModels(store *TrainingDataStore) []ModelBenchmark {
	labeled := store.LabeledSamples()
	if len(labeled) < 20 {
		return nil
	}
	labeled = chronologicalTrainingSamples(labeled)

	allTypes := AllModelTypes()
	results := make([]ModelBenchmark, 0, len(allTypes))

	// Use a chronological 80/20 split so benchmark quality is closer to the
	// online setting and does not benefit from future samples.
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
	tmpStore := NewTrainingDataStore(len(trainSet))
	for i := range trainSet {
		tmpStore.samples[i] = trainSet[i]
	}
	tmpStore.nextWrite = len(trainSet)

	trainStart := time.Now()
	model, result := GlobalTrainer.TrainWithConfig(tmpStore, cfg)
	bench.TrainDuration = time.Since(trainStart).Seconds()

	if result.Error != "" || model == nil {
		bench.Accuracy = 0
		return bench
	}

	// Measure inference speed independently from metric calculation.
	testFeatures := make([][FeatureDim]float64, len(testSet))
	for i, s := range testSet {
		testFeatures[i] = s.Features
	}

	// Warm up
	for i := 0; i < min(10, len(testSet)); i++ {
		model.Predict(testFeatures[i])
	}

	infStart := time.Now()
	for _, feat := range testFeatures {
		model.Predict(feat)
	}
	infElapsed := time.Since(infStart)
	if len(testFeatures) > 0 {
		bench.InferenceTimeUs = float64(infElapsed.Microseconds()) / float64(len(testFeatures))
	}

	metrics := evaluateRiskModel(model, testSet)
	bench.Accuracy = metrics.Accuracy
	bench.BalancedAccuracy = metrics.BalancedAccuracy
	bench.HighRiskRecall = metrics.HighRiskRecall
	bench.BenignFalsePositiveRate = metrics.BenignFalsePositiveRate
	bench.ConfidenceBrier = metrics.ConfidenceBrier
	bench.ECE = metrics.ECE

	return bench
}
