package app

import (
	"math"
	"math/rand"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section trainer_more.go ----

// ── Naive Bayes ────────────────────────────────────────────────────

func (t *ModelTrainer) trainNaiveBayes(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	m := NewNaiveBayes()
	m.Means = make([][FeatureDim]float64, m.Classes)
	m.Vars = make([][FeatureDim]float64, m.Classes)
	m.Priors = make([]float64, m.Classes)
	counts := make([]int, m.Classes)

	for _, s := range labeled {
		if s.Label < 0 || int(s.Label) >= m.Classes {
			continue
		}
		c := s.Label
		counts[c]++
		for d := 0; d < FeatureDim; d++ {
			m.Means[c][d] += s.Features[d]
		}
	}
	n := float64(len(labeled))
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
	for _, s := range labeled {
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

	t.logf("Naive Bayes 训练完成：classes=%d", m.Classes)
	acc := evalModelLabeled(m, labeled)
	t.finishMetrics(acc, acc, acc, len(labeled), len(labeled), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(labeled)}
}

// ── Nearest Centroid ───────────────────────────────────────────────

func (t *ModelTrainer) trainNearestCentroid(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	trainSet, valSet, _, _, err := prepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)
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

	trainAcc := evalModelSamples(m, trainSet)
	valAcc := evalModelSamples(m, valSet)
	t.logf("Nearest Centroid 训练完成: metric=%s, balanced=%t", metric, cfg.BalanceClasses)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setLastSplit(toTrainingSamples(trainSet), toTrainingSamples(valSet))
	return m, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc, NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}

// ── Extra Trees ────────────────────────────────────────────────────

func (t *ModelTrainer) trainExtraTrees(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < cfg.MinSamplesLeaf*10 {
		return nil, TrainResult{Error: "insufficient labeled samples"}
	}

	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}
	trainSet, valSet, _, _, _ := prepareAutoTuneSplit(labeled, cfg.ValidationSplitRatio)

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

	forest := buildExtraTrees(samples, nt, md, ml, time.Now().UnixNano())
	m := &ExtraTreesModel{Forest: forest, NumTrees: nt, MaxDepth: md}

	t.logf("Extra Trees 训练完成：trees=%d, depth=%d", nt, md)
	trainAcc := evalModelSamples(m, trainSet)
	valAcc := evalModelSamples(m, valSet)
	t.finishMetrics(valAcc, trainAcc, valAcc, len(labeled), len(trainSet), len(valSet))
	t.setLastSplit(toTrainingSamples(trainSet), toTrainingSamples(valSet))
	return m, TrainResult{Accuracy: valAcc, TrainAccuracy: trainAcc, ValidationAccuracy: valAcc, NumSamples: len(labeled), TrainSamples: len(trainSet), ValidationSamples: len(valSet)}
}

// ── AdaBoost ───────────────────────────────────────────────────────

func (t *ModelTrainer) trainAdaBoost(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	nEst := cfg.NumTrees
	if nEst < 10 {
		nEst = 50
	}
	m := NewAdaBoost(nEst)

	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	n := len(samples)
	weights := make([]float64, n)
	for i := range weights {
		weights[i] = 1.0 / float64(n)
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for e := 0; e < nEst; e++ {
		t.progress = float64(e) / float64(nEst)
		if t.IsCancelled() {
			return nil, TrainResult{Error: "cancelled"}
		}

		// Weighted sampling
		cum := make([]float64, n)
		cum[0] = weights[0]
		for i := 1; i < n; i++ {
			cum[i] = cum[i-1] + weights[i]
		}
		totalW := cum[n-1]

		// Find best stump: sample a few random features and thresholds
		bestStump := adaboostStump{Feature: -1}
		bestErr := 1e9
		for tries := 0; tries < 50; tries++ {
			fi := rng.Intn(FeatureDim)
			si := rng.Intn(n)
			thresh := samples[si].features[fi]

			var leftErr, rightErr, leftW, rightW float64
			for i, s := range samples {
				if s.features[fi] < thresh {
					if s.label != 1 {
						leftErr += weights[i]
					}
					leftW += weights[i]
				} else {
					if s.label != 0 {
						rightErr += weights[i]
					}
					rightW += weights[i]
				}
			}
			err := (leftErr + rightErr) / totalW
			if err < bestErr {
				bestErr = err
				bestStump = adaboostStump{
					Feature: fi, Threshold: thresh,
					LeftVote: float64(1), RightVote: float64(0),
				}
				if leftErr/leftW > rightErr/rightW {
					bestStump.LeftVote = 0
					bestStump.RightVote = 1
				}
			}
		}
		if bestStump.Feature < 0 {
			continue
		}

		// Compute alpha
		err := math.Max(bestErr, 1e-10)
		alpha := 0.5 * math.Log((1-err)/err)
		if alpha <= 0 {
			continue
		}

		// Update weights
		for i, s := range samples {
			pred := 0
			if s.features[bestStump.Feature] < bestStump.Threshold {
				pred = int(bestStump.LeftVote)
			} else {
				pred = int(bestStump.RightVote)
			}
			if pred != int(s.label) {
				weights[i] *= math.Exp(alpha)
			}
		}

		m.Stumps = append(m.Stumps, bestStump)
		m.Alphas = append(m.Alphas, alpha)
	}

	t.logf("AdaBoost 训练完成：estimators=%d", len(m.Stumps))
	acc := evalModelSamples(m, samples)
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── Ensemble ───────────────────────────────────────────────────────

func (t *ModelTrainer) trainEnsemble(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.acquire()
	defer t.release()
	t.ResetCancel()
	t.isRunning = true
	t.progress = 0
	defer func() { t.isRunning = false; t.progress = 1.0 }()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need >=10 labeled samples"}
	}

	model := buildEnsembleFromStore(store)
	if model == nil {
		return nil, TrainResult{Error: "failed to build ensemble"}
	}
	model.Voting = normalizeEnsembleVoting(cfg.EnsembleVoting)

	samples := toTrainSamples(labeled)
	acc := evalModelSamples(model, samples)
	t.logf("Ensemble 训练完成: voting=%s, models=%d", model.Voting, len(model.Models))
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return model, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}
