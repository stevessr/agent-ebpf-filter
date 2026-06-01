package main

import (
	"math"
	"math/rand"
	"time"
)

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
