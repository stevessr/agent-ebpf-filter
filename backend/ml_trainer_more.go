package main

import (
	"math"
	"math/rand"
	"time"
)

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

	t.logf("Naive Bayes 训练完成: classes=%d", m.Classes)
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

	t.logf("Extra Trees 训练完成: trees=%d, depth=%d", nt, md)
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

	t.logf("AdaBoost 训练完成: estimators=%d", len(m.Stumps))
	acc := evalModelSamples(m, samples)
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── Linear SVM ─────────────────────────────────────────────────────

func (t *ModelTrainer) trainSVM(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
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

	lr := 0.01
	if cfg.NumTrees > 0 {
		lr = float64(cfg.NumTrees) / 1000.0
	}
	maxIter := cfg.MinSamplesLeaf
	if maxIter < 100 {
		maxIter = 1000
	}

	m := NewSVMModel(lr, maxIter)
	m.Weights = make([][FeatureDim + 1]float64, m.Classes)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for c := range m.Weights {
		for d := range m.Weights[c] {
			m.Weights[c][d] = (rng.Float64() - 0.5) * 0.01
		}
	}

	var classWeights []float64
	if cfg.BalanceClasses {
		_, labels := extractFeaturesLabels(labeled)
		classWeights = computeClassWeights(labels, m.Classes)
	}
	trainSGD(m.Weights, m.Classes, labeled, lr, maxIter, m.C, "hinge", classWeights, t)
	t.logf("SVM 训练完成: lr=%.4f, iter=%d", lr, maxIter)

	samples := toTrainSamples(labeled)
	acc := evalLinearModel(m.Weights, m.Classes, samples)
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── Ridge Classifier ───────────────────────────────────────────────

func (t *ModelTrainer) trainRidge(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
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

	alpha := 1.0
	if cfg.NumTrees > 0 {
		alpha = float64(cfg.NumTrees) / 100.0
	}

	// One-vs-rest Ridge: closed-form (X^T X + αI)^-1 X^T Y
	m := NewRidgeModel(alpha)
	m.Weights = make([][FeatureDim + 1]float64, m.Classes)
	ridgeFit(m.Weights, m.Classes, labeled, alpha)

	t.logf("Ridge 训练完成: alpha=%.4f", alpha)
	samples := toTrainSamples(labeled)
	acc := evalLinearModel(m.Weights, m.Classes, samples)
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── Perceptron ─────────────────────────────────────────────────────

func (t *ModelTrainer) trainPerceptron(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
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

	lr := 0.01
	if cfg.NumTrees > 0 {
		lr = float64(cfg.NumTrees) / 1000.0
	}
	maxIter := cfg.MinSamplesLeaf
	if maxIter < 100 {
		maxIter = 1000
	}

	m := NewPerceptron(lr, maxIter)
	m.Weights = make([][FeatureDim + 1]float64, m.Classes)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for c := range m.Weights {
		for d := range m.Weights[c] {
			m.Weights[c][d] = (rng.Float64() - 0.5) * 0.01
		}
	}

	var classWeights []float64
	if cfg.BalanceClasses {
		_, labels := extractFeaturesLabels(labeled)
		classWeights = computeClassWeights(labels, m.Classes)
	}
	trainSGD(m.Weights, m.Classes, labeled, lr, maxIter, 0, "perceptron", classWeights, t)
	t.logf("Perceptron 训练完成: lr=%.4f, iter=%d", lr, maxIter)

	samples := toTrainSamples(labeled)
	acc := evalLinearModel(m.Weights, m.Classes, samples)
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return m, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── Passive Aggressive ─────────────────────────────────────────────

func (t *ModelTrainer) trainPA(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
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

	C := 1.0
	if cfg.NumTrees > 0 {
		C = float64(cfg.NumTrees) / 10.0
	}
	maxIter := cfg.MinSamplesLeaf
	if maxIter < 100 {
		maxIter = 1000
	}

	m := NewPAModel(C, maxIter)
	m.Weights = make([][FeatureDim + 1]float64, m.Classes)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for c := range m.Weights {
		for d := range m.Weights[c] {
			m.Weights[c][d] = (rng.Float64() - 0.5) * 0.01
		}
	}

	var classWeights []float64
	if cfg.BalanceClasses {
		_, labels := extractFeaturesLabels(labeled)
		classWeights = computeClassWeights(labels, m.Classes)
	}
	trainSGD(m.Weights, m.Classes, labeled, 1.0, maxIter, C, "pa", classWeights, t)
	t.logf("Passive-Aggressive 训练完成: C=%.2f, iter=%d", C, maxIter)

	samples := toTrainSamples(labeled)
	acc := evalLinearModel(m.Weights, m.Classes, samples)
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

	samples := toTrainSamples(labeled)
	acc := evalModelSamples(model, samples)
	t.logf("Ensemble 训练完成: voting=%s, models=%d", model.Voting, len(model.Models))
	t.finishMetrics(acc, acc, acc, len(labeled), len(samples), 0)
	t.setLastSplit(labeled, labeled)
	return model, TrainResult{Accuracy: acc, TrainAccuracy: acc, ValidationAccuracy: acc, NumSamples: len(labeled), TrainSamples: len(samples)}
}

// ── SGD Training Helper ─────────────────────────────────────────────

func trainSGD(W [][FeatureDim + 1]float64, nClasses int, labeled []TrainingSample, lr float64, maxIter int, C float64, loss string, classWeights []float64, t *ModelTrainer) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for iter := 0; iter < maxIter; iter++ {
		if t.IsCancelled() {
			return
		}
		eta := lr * (1.0 - float64(iter)/float64(maxIter)*0.95)
		order := rng.Perm(len(labeled))
		for _, idx := range order {
			s := labeled[idx]
			if s.Label < 0 || int(s.Label) >= nClasses {
				continue
			}

			scores := make([]float64, nClasses)
			for c := 0; c < nClasses; c++ {
				scores[c] = W[c][FeatureDim]
				for d := 0; d < FeatureDim; d++ {
					scores[c] += W[c][d] * s.Features[d]
				}
			}

			trueC := int(s.Label)
			bestWrongC := -1
			bestWrongScore := math.Inf(-1)
			for c := 0; c < nClasses; c++ {
				if c == trueC {
					continue
				}
				if scores[c] > bestWrongScore {
					bestWrongScore = scores[c]
					bestWrongC = c
				}
			}
			if bestWrongC < 0 {
				continue
			}
			margin := scores[trueC] - scores[bestWrongC]
			sampleWeight := 1.0
			if len(classWeights) == nClasses {
				sampleWeight = classWeights[trueC]
				if sampleWeight <= 0 {
					sampleWeight = 1.0
				}
			}

			switch loss {
			case "hinge": // SVM
				if margin < 1.0 {
					step := eta * sampleWeight
					for d := 0; d <= FeatureDim; d++ {
						v := 0.0
						if d == FeatureDim {
							v = 1.0
						} else {
							v = s.Features[d]
						}
						W[trueC][d] += step * v
						W[bestWrongC][d] -= step * v
					}
				}
			case "perceptron":
				if margin <= 0 {
					step := eta * sampleWeight
					for d := 0; d <= FeatureDim; d++ {
						v := 0.0
						if d == FeatureDim {
							v = 1.0
						} else {
							v = s.Features[d]
						}
						W[trueC][d] += step * v
						W[bestWrongC][d] -= step * v
					}
				}
			case "pa":
				if margin < 1.0 {
					normSq := 0.0
					for d := 0; d < FeatureDim; d++ {
						normSq += s.Features[d] * s.Features[d]
					}
					normSq += 1.0 // bias
					tau := (1.0 - margin) / (normSq + 1.0/(2*C))
					tau *= sampleWeight
					for d := 0; d <= FeatureDim; d++ {
						v := 0.0
						if d == FeatureDim {
							v = 1.0
						} else {
							v = s.Features[d]
						}
						W[trueC][d] += tau * v
						W[bestWrongC][d] -= tau * v
					}
				}
			}
		}
	}
}

func ridgeFit(W [][FeatureDim + 1]float64, nClasses int, labeled []TrainingSample, alpha float64) {
	_ = len(labeled)
	// One-vs-rest: for each class, solve (X^T X + αI) w = X^T y using SGD approximation
	// Simplified: iterative ridge via SGD
	for c := 0; c < nClasses; c++ {
		for iter := 0; iter < 500; iter++ {
			lr := 0.01 * (1.0 - float64(iter)/500.0*0.9)
			for _, s := range labeled {
				target := 0.0
				if int(s.Label) == c {
					target = 1.0
				}
				dot := W[c][FeatureDim]
				for d := 0; d < FeatureDim; d++ {
					dot += W[c][d] * s.Features[d]
				}
				err := dot - target
				for d := 0; d < FeatureDim; d++ {
					W[c][d] -= lr * (err*s.Features[d] + 2*alpha*W[c][d])
				}
				W[c][FeatureDim] -= lr * err
			}
		}
	}
}

// ── Helpers ────────────────────────────────────────────────────────

func (t *ModelTrainer) acquire() { t.mu <- struct{}{} }
func (t *ModelTrainer) release() { <-t.mu }

func (t *ModelTrainer) finishMetrics(acc, trainAcc, valAcc float64, total, trainN, valN int) {
	t.lastTrain = time.Now()
	t.accuracy = acc
	t.trainAccuracy = trainAcc
	t.validationAccuracy = valAcc
	t.addHistory(TrainingHistoryEntry{Timestamp: t.lastTrain, Accuracy: acc, NumSamples: total})
}

func evalModelLabeled(model Model, labeled []TrainingSample) float64 {
	if len(labeled) == 0 {
		return 0
	}
	correct := 0
	for _, s := range labeled {
		if model.Predict(s.Features).Action == s.Label {
			correct++
		}
	}
	return float64(correct) / float64(len(labeled))
}

func evalModelSamples(model Model, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		if model.Predict(s.features).Action == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func evalLinearModel(W [][FeatureDim + 1]float64, nClasses int, samples []trainSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	correct := 0
	for _, s := range samples {
		bestC, bestS := 0, math.Inf(-1)
		for c := 0; c < nClasses; c++ {
			score := W[c][FeatureDim]
			for d := 0; d < FeatureDim; d++ {
				score += W[c][d] * s.features[d]
			}
			if score > bestS {
				bestS = score
				bestC = c
			}
		}
		if int32(bestC) == s.label {
			correct++
		}
	}
	return float64(correct) / float64(len(samples))
}

func toTrainSamples(labeled []TrainingSample) []trainSample {
	out := make([]trainSample, len(labeled))
	for i, s := range labeled {
		out[i] = trainSample{features: s.Features, label: s.Label}
	}
	return out
}

func toTrainingSamples(samples []trainSample) []TrainingSample {
	out := make([]TrainingSample, len(samples))
	for i, s := range samples {
		out[i] = TrainingSample{Features: s.features, Label: s.label}
	}
	return out
}
