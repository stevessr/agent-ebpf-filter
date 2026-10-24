package ml

import (
	"math"
	"math/rand"
	"time"
)

// ── Naive Bayes ────────────────────────────────────────────────────

func (t *ModelTrainer) trainNaiveBayes(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}
	trainSet, valSet, trainRaw, valRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: err.Error()}
	}

	m := NewNaiveBayes()
	m.Means = make([][FeatureDim]float64, m.Classes)
	m.Vars = make([][FeatureDim]float64, m.Classes)
	m.Priors = make([]float64, m.Classes)
	counts := make([]int, m.Classes)

	for _, s := range trainRaw {
		if s.Label < 0 || int(s.Label) >= m.Classes {
			continue
		}
		c := s.Label
		counts[c]++
		for d := 0; d < FeatureDim; d++ {
			m.Means[c][d] += s.Features[d]
		}
	}
	n := float64(len(trainRaw))
	nonEmptyClasses := 0
	for _, count := range counts {
		if count > 0 {
			nonEmptyClasses++
		}
	}
	for c := 0; c < m.Classes; c++ {
		if counts[c] > 0 {
			if cfg.BalanceClasses && nonEmptyClasses > 0 {
				m.Priors[c] = 1.0 / float64(nonEmptyClasses)
			} else {
				m.Priors[c] = float64(counts[c]) / n
			}
			for d := 0; d < FeatureDim; d++ {
				m.Means[c][d] /= float64(counts[c])
			}
		}
	}
	// Compute variances
	for _, s := range trainRaw {
		if s.Label < 0 || int(s.Label) >= m.Classes {
			continue
		}
		c := s.Label
		for d := 0; d < FeatureDim; d++ {
			diff := s.Features[d] - m.Means[c][d]
			m.Vars[c][d] += diff * diff
		}
	}
	for c := 0; c < m.Classes; c++ {
		if counts[c] > 1 {
			for d := 0; d < FeatureDim; d++ {
				m.Vars[c][d] /= float64(counts[c] - 1)
			}
		}
	}

	t.Logf("Naive Bayes 训练完成：classes=%d", m.Classes)
	trainAcc := EvalModelSamples(m, trainSet)
	valAcc := EvalModelSamples(m, valSet)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setLastSplit(trainRaw, valRaw)
	return m, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc,
		NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}

// ── Nearest Centroid ───────────────────────────────────────────────

func (t *ModelTrainer) trainNearestCentroid(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	trainSet, valSet, _, _, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: err.Error()}
	}

	metric := "euclidean"
	switch cfg.MaxDepth {
	case 4:
		metric = "cosine"
	case 12:
		metric = "manhattan"
	}

	m := NewNearestCentroid(metric, cfg.BalanceClasses)
	m.Classes = 4
	m.Centroids = make([][FeatureDim]float64, m.Classes)
	m.Priors = make([]float64, m.Classes)
	counts := make([]int, m.Classes)
	for _, s := range trainSet {
		if s.label < 0 || int(s.label) >= m.Classes {
			continue
		}
		c := int(s.label)
		counts[c]++
		for d := 0; d < FeatureDim; d++ {
			m.Centroids[c][d] += s.features[d]
		}
	}
	nonEmptyClasses := 0
	for _, count := range counts {
		if count > 0 {
			nonEmptyClasses++
		}
	}
	for c := 0; c < m.Classes; c++ {
		if counts[c] > 0 {
			for d := 0; d < FeatureDim; d++ {
				m.Centroids[c][d] /= float64(counts[c])
			}
		}
		if cfg.BalanceClasses && nonEmptyClasses > 0 {
			if counts[c] > 0 {
				m.Priors[c] = 1.0 / float64(nonEmptyClasses)
			}
		} else if len(trainSet) > 0 {
			m.Priors[c] = float64(counts[c]) / float64(len(trainSet))
		}
	}

	trainAcc := EvalModelSamples(m, trainSet)
	valAcc := EvalModelSamples(m, valSet)
	t.Logf("Nearest Centroid 训练完成: metric=%s, balanced=%t", metric, cfg.BalanceClasses)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setLastSplit(ToTrainingSamples(trainSet), ToTrainingSamples(valSet))
	return m, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc, NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}

// ── Extra Trees ────────────────────────────────────────────────────

func (t *ModelTrainer) trainExtraTrees(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < cfg.MinSamplesLeaf*10 {
		return nil, TrainResult{Error: "insufficient labeled samples"}
	}

	trainSet, valSet, trainRaw, validationRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: err.Error()}
	}

	nt := cfg.NumTrees
	if nt < 1 {
		nt = 31
	}
	md := cfg.MaxDepth
	if md < 1 {
		md = 8
	}
	ml := cfg.MinSamplesLeaf
	if ml < 1 {
		ml = 5
	}

	forest := BuildExtraTreesWithBalance(trainSet, nt, md, ml, time.Now().UnixNano(), cfg.BalanceClasses)
	m := &ExtraTreesModel{Forest: forest, NumTrees: nt, MaxDepth: md}

	t.Logf("Extra Trees 训练完成：trees=%d, depth=%d", nt, md)
	trainAcc := EvalModelSamples(m, trainSet)
	valAcc := EvalModelSamples(m, valSet)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setValidationRatio(float64(len(valSet)) / float64(len(labeled)))
	t.setLastSplit(trainRaw, validationRaw)
	return m, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc, NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}

// ── AdaBoost ───────────────────────────────────────────────────────

func (t *ModelTrainer) trainAdaBoost(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	nEst := cfg.NumTrees
	if nEst < 10 {
		nEst = 50
	}
	m := NewAdaBoost(nEst)

	// AdaBoost's previous TrainResult reported training accuracy as held-out
	// validation accuracy. Use the same duplicate-safe split as other models.
	trainSet, validationSet, trainRaw, validationRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: err.Error()}
	}
	samples := trainSet
	n := len(samples)
	weights := make([]float64, n)
	var classCounts [4]int
	for _, sample := range samples {
		if sample.label >= 0 && sample.label < 4 {
			classCounts[sample.label]++
		}
	}
	for i, sample := range samples {
		weights[i] = 1
		if cfg.BalanceClasses && sample.label >= 0 && sample.label < 4 && classCounts[sample.label] > 0 {
			weights[i] = 1 / float64(classCounts[sample.label])
		}
	}
	normalizeWeights := func() bool {
		total := 0.0
		for _, weight := range weights {
			total += weight
		}
		if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
			return false
		}
		for i := range weights {
			weights[i] /= total
		}
		return true
	}
	if !normalizeWeights() {
		return nil, TrainResult{Error: "AdaBoost requires valid labels and weights"}
	}

	// Discrete SAMME boosting supports all four security actions. The old
	// binary-only stumps could never predict REWRITE/ALERT and mislabeled their
	// samples as errors even when the feature evidence separated them.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for e := 0; e < nEst; e++ {
		t.setTrainingProgress(float64(e) / float64(nEst))
		if t.IsCancelled() {
			return nil, TrainResult{Error: "cancelled"}
		}
		bestStump := AdaBoostStump{Feature: -1}
		bestError := math.Inf(1)
		for trial := 0; trial < 50; trial++ {
			feature := rng.Intn(FeatureDim)
			threshold := samples[rng.Intn(n)].features[feature]
			var sideWeights [2][4]float64
			var sideTotals [2]float64
			for i, sample := range samples {
				if sample.label < 0 || sample.label >= 4 {
					continue
				}
				side := 0
				if sample.features[feature] >= threshold {
					side = 1
				}
				sideWeights[side][sample.label] += weights[i]
				sideTotals[side] += weights[i]
			}
			if sideTotals[0] <= 0 || sideTotals[1] <= 0 {
				continue
			}
			votes := [2]int{}
			correctWeight := 0.0
			for side := 0; side < 2; side++ {
				for c := 1; c < 4; c++ {
					if sideWeights[side][c] > sideWeights[side][votes[side]] {
						votes[side] = c
					}
				}
				correctWeight += sideWeights[side][votes[side]]
			}
			errorRate := 1 - correctWeight
			if errorRate < bestError {
				bestError = errorRate
				bestStump = AdaBoostStump{
					Feature: feature, Threshold: threshold,
					LeftVote: float64(votes[0]), RightVote: float64(votes[1]),
				}
			}
		}
		// SAMME requires a classifier better than 1/K random accuracy.
		if bestStump.Feature < 0 || bestError >= 1-1/float64(m.Classes) {
			continue
		}
		errorRate := math.Max(1e-10, bestError)
		alpha := math.Log((1-errorRate)/errorRate) + math.Log(float64(m.Classes-1))
		if alpha <= 0 || math.IsNaN(alpha) || math.IsInf(alpha, 0) {
			continue
		}
		m.Stumps = append(m.Stumps, bestStump)
		m.Alphas = append(m.Alphas, alpha)
		penalty := math.Exp(alpha)
		for i, sample := range samples {
			pred := int32(bestStump.RightVote)
			if sample.features[bestStump.Feature] < bestStump.Threshold {
				pred = int32(bestStump.LeftVote)
			}
			if pred != sample.label {
				weights[i] *= penalty
			}
		}
		if !normalizeWeights() {
			return nil, TrainResult{Error: "AdaBoost sample weights became non-finite"}
		}
	}

	if len(m.Stumps) == 0 {
		return nil, TrainResult{Error: "AdaBoost found no weak learner better than random"}
	}
	t.Logf("AdaBoost 训练完成：estimators=%d, train=%d, validation=%d", len(m.Stumps), len(trainSet), len(validationSet))
	trainAcc := EvalModelSamples(m, trainSet)
	valAcc := EvalModelSamples(m, validationSet)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(validationSet))
	t.setValidationRatio(float64(len(validationSet)) / float64(len(labeled)))
	t.setLastSplit(trainRaw, validationRaw)
	return m, TrainResult{
		Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc,
		NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(validationSet),
	}
}

// ── Ensemble ───────────────────────────────────────────────────────

func (t *ModelTrainer) trainEnsemble(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	// Weight calibration is nested inside the outer training partition.
	trainSet, valSet, trainRaw, valRaw, err := PrepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
	if err != nil {
		return nil, TrainResult{Error: err.Error()}
	}
	trainingStore := NewTrainingDataStore(len(trainRaw))
	for _, sample := range trainRaw {
		trainingStore.Add(sample)
	}
	model := BuildEnsembleFromStore(trainingStore)
	if model == nil {
		return nil, TrainResult{Error: "failed to build ensemble"}
	}
	model.Voting = normalizeEnsembleVoting(cfg.EnsembleVoting)
	trainAcc := EvalModelSamples(model, trainSet)
	valAcc := EvalModelSamples(model, valSet)
	t.Logf("Ensemble 训练完成: voting=%s, models=%d, train=%d, validation=%d",
		model.Voting, len(model.Models), len(trainSet), len(valSet))
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setLastSplit(trainRaw, valRaw)
	return model, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc,
		NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}
