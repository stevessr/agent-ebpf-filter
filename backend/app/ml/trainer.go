package ml

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"agent-ebpf-filter/core"
)

// LLMReviewHook lets the application layer plug in an LLM-assisted
// post-training dataset review without coupling this package to any HTTP
// client. When nil (or cfg.LlmEnabled is false) review is skipped.
var LLMReviewHook func(t *ModelTrainer, samples []TrainingSample, validationRatio float64) (*LLMReviewSummary, error)

// CancellationContext derives a context that aborts when training stops,
// so long-running auxiliary work (e.g. LLM scoring) can be cancelled.
func (t *ModelTrainer) CancellationContext(parent context.Context) (context.Context, func()) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	if t == nil {
		return ctx, cancel
	}
	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-t.cancellationSignal():
			cancel()
		case <-stopWatch:
		}
	}()
	var stopOnce sync.Once
	return ctx, func() {
		stopOnce.Do(func() { close(stopWatch) })
		cancel()
		<-watchDone
	}
}

// ---- moved from backend/zz_merged_backend.go section trainer.go ----

func (t *ModelTrainer) trainForestWithConfig(store *TrainingDataStore, numTrees, maxDepth, minSamplesLeaf int, cfg MLConfig) (*DecisionForest, TrainResult) {
	select {
	case t.mu <- struct{}{}:
		defer func() { <-t.mu }()
	default:
		return nil, TrainResult{Error: "training already in progress"}
	}

	t.BeginTraining()
	trainStart := time.Now()
	t.Logf("══════ Training started ══════")
	t.Logf("Config: trees=%d, maxDepth=%d, minSamplesLeaf=%d", numTrees, maxDepth, minSamplesLeaf)
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < minSamplesLeaf*10 {
		msg := fmt.Sprintf("Insufficient labeled samples: need >=%d, have %d", minSamplesLeaf*10, len(labeled))
		t.Logf("ERROR: %s", msg)
		return nil, TrainResult{Error: msg}
	}
	t.Logf("Labeled samples loaded: %d", len(labeled))

	samples := make([]TrainSample, len(labeled))
	classDist := make(map[int32]int)
	for i, s := range labeled {
		samples[i] = TrainSample{features: s.Features, label: s.Label}
		classDist[s.Label]++
	}
	t.Logf("Class distribution: ALLOW=%d, BLOCK=%d, ALERT=%d, REWRITE=%d",
		classDist[0], classDist[1], classDist[3], classDist[2])

	validationRatio := cfg.ValidationSplitRatio
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}
	shuffledRaw := append([]TrainingSample(nil), labeled...)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(samples), func(i, j int) {
		samples[i], samples[j] = samples[j], samples[i]
		shuffledRaw[i], shuffledRaw[j] = shuffledRaw[j], shuffledRaw[i]
	})
	validationCount := int(math.Round(float64(len(samples)) * validationRatio))
	if validationCount < 1 {
		validationCount = 1
	}
	if validationCount >= len(samples) {
		validationCount = len(samples) - 1
	}
	trainCount := len(samples) - validationCount
	trainSet := samples[:trainCount]
	validationSet := samples[trainCount:]
	trainRaw := append([]TrainingSample(nil), shuffledRaw[:trainCount]...)
	validationRaw := append([]TrainingSample(nil), shuffledRaw[trainCount:]...)
	t.Logf("Data split: train=%d, validation=%d (ratio=%.2f)", len(trainSet), len(validationSet), validationRatio)

	t.Logf("Building random forest with %d trees...", numTrees)
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	featureSampleCount := int(math.Sqrt(float64(FeatureDim)))
	t.Logf("Feature sampling: %d of %d per split", featureSampleCount, FeatureDim)

	totalNodes := 0
	treeStart := time.Now()
	for ti := 0; ti < numTrees; ti++ {
		if t.IsCancelled() {
			t.Logf("训练已中止")
			return nil, TrainResult{Error: "cancelled"}
		}
		progress := float64(ti) / float64(numTrees)
		t.setTrainingProgress(progress)
		tStart := time.Now()

		bootstrap := make([]TrainSample, len(trainSet))
		classStratifiedBootstrap(trainSet, bootstrap, rng)

		nodes := buildTree(bootstrap, 0, maxDepth, minSamplesLeaf, featureSampleCount, rng)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
		totalNodes += len(nodes)

		elapsed := time.Since(tStart)
		if ti%10 == 0 || ti == numTrees-1 {
			t.Logf("Tree %d/%d built: %d nodes, %s (%.0f%%)",
				ti+1, numTrees, len(nodes), elapsed.Round(time.Microsecond), progress*100)
		}
	}
	treeElapsed := time.Since(treeStart)
	t.Logf("All %d trees built in %s, total nodes: %d, avg nodes/tree: %d",
		numTrees, treeElapsed.Round(time.Millisecond), totalNodes, totalNodes/numTrees)

	forest.IsTrained = true

	pruned := forest.Prune(trainSet)
	if pruned > 0 {
		t.Logf("Pruned %d underperforming trees, %d remaining", pruned, len(forest.Trees))
	}

	t.Logf("Evaluating model on %d train samples and %d validation samples...", len(trainSet), len(validationSet))
	evalStart := time.Now()
	trainAccuracy := EvaluateForest(forest, trainSet)
	validationAccuracy := EvaluateForest(forest, validationSet)
	evalElapsed := time.Since(evalStart)

	perClassCorrect := make(map[int32]int)
	perClassTotal := make(map[int32]int)
	for _, s := range validationSet {
		pred := forest.Predict(s.features)
		perClassTotal[s.label]++
		if pred.Action == s.label {
			perClassCorrect[s.label]++
		}
	}
	t.Logf("Evaluation complete in %s", evalElapsed.Round(time.Millisecond))
	for _, lbl := range []int32{0, 1, 2, 3} {
		if perClassTotal[lbl] > 0 {
			acc := float64(perClassCorrect[lbl]) / float64(perClassTotal[lbl]) * 100
			t.Logf("  %s: %d/%d correct (%.1f%%)", ActionLabel[lbl], perClassCorrect[lbl], perClassTotal[lbl], acc)
		}
	}
	t.Logf("Train accuracy: %.2f%%", trainAccuracy*100)
	t.Logf("Validation accuracy: %.2f%%", validationAccuracy*100)

	llmReviewSamples := 0
	llmAverageRiskScore := 0.0
	llmAgreement := 0.0
	if cfg.LlmEnabled && LLMReviewHook != nil {
		if review, err := LLMReviewHook(t, validationRaw, validationRatio); err != nil {
			t.Logf("WARN: LLM post-training review failed: %v", err)
		} else if review != nil {
			llmReviewSamples = review.ScoredSamples
			llmAverageRiskScore = review.AverageRiskScore
			llmAgreement = review.Agreement
			t.Logf("LLM post-training review: %d samples, avg risk %.1f, agreement %.1f%%", review.ScoredSamples, review.AverageRiskScore, review.Agreement*100)
			t.SetLastLLMReview(review)
		}
	}

	t.setTrainingResult(time.Now(), validationAccuracy, trainAccuracy, validationAccuracy)
	t.setValidationRatio(validationRatio)
	t.Logf("══════ Training complete in %s ══════", treeElapsed.Round(time.Millisecond))
	t.setLastSplit(trainRaw, validationRaw)

	t.addHistory(TrainingHistoryEntry{
		Timestamp:            trainStart,
		Accuracy:             validationAccuracy,
		TrainAccuracy:        trainAccuracy,
		ValidationAccuracy:   validationAccuracy,
		NumTrees:             numTrees,
		NumSamples:           len(labeled),
		TrainSamples:         len(trainRaw),
		ValidationSamples:    len(validationRaw),
		ValidationSplitRatio: validationRatio,
		LLMScoredSamples:     llmReviewSamples,
		LLMAverageRiskScore:  llmAverageRiskScore,
		LLMAgreement:         llmAgreement,
		Duration:             time.Since(trainStart).Seconds(),
	})

	result := TrainResult{
		Accuracy:            validationAccuracy,
		TrainAccuracy:       trainAccuracy,
		ValidationAccuracy:  validationAccuracy,
		NumTrees:            numTrees,
		NumSamples:          len(labeled),
		TrainSamples:        len(trainRaw),
		ValidationSamples:   len(validationRaw),
		LLMScoredSamples:    llmReviewSamples,
		LLMAverageRiskScore: llmAverageRiskScore,
		LLMAgreement:        llmAgreement,
	}

	return forest, result
}

// TrainWithConfig trains a model based on the MLConfig.ModelType.
func (t *ModelTrainer) TrainWithConfig(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	requestedType := cfg.ModelType
	if requestedType == "" {
		requestedType = ModelRandomForest
	}
	effectiveCfg := ApplyBuiltinModelPreset(cfg)

	var (
		model  Model
		result TrainResult
	)
	switch effectiveCfg.ModelType {
	case ModelRandomForest:
		model, result = t.trainForestWithConfig(store, effectiveCfg.NumTrees, effectiveCfg.MaxDepth, effectiveCfg.MinSamplesLeaf, effectiveCfg)
	case ModelKNN:
		model, result = t.trainKNN(store, effectiveCfg)
	case ModelLogisticRegression:
		model, result = t.trainLogistic(store, effectiveCfg)
	case ModelNaiveBayes:
		model, result = t.trainNaiveBayes(store, effectiveCfg)
	case ModelNearestCentroid:
		model, result = t.trainNearestCentroid(store, effectiveCfg)
	case ModelExtraTrees:
		model, result = t.trainExtraTrees(store, effectiveCfg)
	case ModelAdaBoost:
		model, result = t.trainAdaBoost(store, effectiveCfg)
	case ModelSVM:
		model, result = t.trainSVM(store, effectiveCfg)
	case ModelRidge:
		model, result = t.trainRidge(store, effectiveCfg)
	case ModelPerceptron:
		model, result = t.trainPerceptron(store, effectiveCfg)
	case ModelPassiveAggressive:
		model, result = t.trainPA(store, effectiveCfg)
	case ModelEnsemble:
		model, result = t.trainEnsemble(store, effectiveCfg)
	case core.ModelGraphLearning:
		model, result = t.trainGraph(store, effectiveCfg)
	case ModelGANTransformer:
		model, result = t.trainGANTransformer(store, effectiveCfg)
	default:
		model, result = t.trainForestWithConfig(store, effectiveCfg.NumTrees, effectiveCfg.MaxDepth, effectiveCfg.MinSamplesLeaf, effectiveCfg)
	}
	return WrapModelType(model, requestedType), result
}

func (t *ModelTrainer) trainKNN(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()

	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) == 0 {
		return nil, TrainResult{Error: "no labeled samples available"}
	}

	k := cfg.NumTrees
	if k < 1 {
		k = 5
	}
	if k > len(labeled) {
		k = len(labeled)
	}

	distance := "euclidean"
	if cfg.MaxDepth >= 16 {
		distance = "cosine"
	} else if cfg.MaxDepth >= 12 {
		distance = "manhattan"
	}
	weight := "uniform"
	if cfg.MinSamplesLeaf >= 8 {
		weight = "distance"
	}

	model := NewKNNModel(k, distance, weight)
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	t.Logf("KNN 训练完成: k=%d, distance=%s, weight=%s, samples=%d", k, distance, weight, len(labeled))

	correct := 0
	for _, s := range labeled {
		pred := model.Predict(s.Features)
		if pred.Action == s.Label {
			correct++
		}
	}
	accuracy := float64(correct) / float64(len(labeled))

	trainedAt := time.Now()
	t.setTrainingResult(trainedAt, accuracy, accuracy, accuracy)

	t.addHistory(TrainingHistoryEntry{
		Timestamp:  trainedAt,
		Accuracy:   accuracy,
		NumSamples: len(labeled),
	})
	t.setLastSplit(labeled, labeled)

	return model, TrainResult{
		Accuracy:           accuracy,
		TrainAccuracy:      accuracy,
		ValidationAccuracy: accuracy,
		NumSamples:         len(labeled),
		TrainSamples:       len(labeled),
	}
}

func (t *ModelTrainer) trainLogistic(store *TrainingDataStore, cfg MLConfig) (Model, TrainResult) {
	t.mu <- struct{}{}
	defer func() { <-t.mu }()

	t.BeginTraining()
	defer t.finishTraining()

	labeled := store.LabeledSamples()
	if len(labeled) < 10 {
		return nil, TrainResult{Error: "need at least 10 labeled samples for logistic regression"}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	shuffled := make([]TrainingSample, len(labeled))
	copy(shuffled, labeled)
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	splitIdx := int(float64(len(shuffled)) * (1.0 - cfg.ValidationSplitRatio))
	if splitIdx < 1 {
		splitIdx = 1
	}

	learningRate := 0.01
	if cfg.NumTrees > 0 {
		learningRate = float64(cfg.NumTrees) / 1000.0
	}

	regularization := "l2"
	switch cfg.MaxDepth {
	case 4:
		regularization = "none"
	case 12:
		regularization = "l1"
	}

	maxIter := cfg.MinSamplesLeaf
	if maxIter < 100 {
		maxIter = 1000
	}

	trainSamples := make([][FeatureDim]float64, splitIdx)
	trainLabels := make([]int32, splitIdx)
	for i := 0; i < splitIdx; i++ {
		trainSamples[i] = shuffled[i].Features
		trainLabels[i] = shuffled[i].Label
	}

	model := NewLogisticModel(learningRate, regularization, maxIter)
	model.NumClasses = 4
	if cfg.BalanceClasses {
		model.ClassWeights = computeClassWeights(trainLabels, model.NumClasses)
	}
	model.Train(trainSamples, trainLabels)

	t.Logf("逻辑回归训练完成: lr=%.4f, reg=%s, iter=%d, samples=%d", learningRate, regularization, maxIter, splitIdx)

	trainCorrect := 0
	for i := 0; i < splitIdx; i++ {
		if pred := model.Predict(trainSamples[i]); pred.Action == trainLabels[i] {
			trainCorrect++
		}
	}
	trainAcc := float64(trainCorrect) / float64(splitIdx)

	valAcc := trainAcc
	valSamples := 0
	if splitIdx < len(shuffled) {
		valSamples = len(shuffled) - splitIdx
		valCorrect := 0
		for i := splitIdx; i < len(shuffled); i++ {
			if pred := model.Predict(shuffled[i].Features); pred.Action == shuffled[i].Label {
				valCorrect++
			}
		}
		valAcc = float64(valCorrect) / float64(valSamples)
	}

	trainedAt := time.Now()
	t.setTrainingResult(trainedAt, valAcc, trainAcc, valAcc)

	t.addHistory(TrainingHistoryEntry{
		Timestamp:  trainedAt,
		Accuracy:   valAcc,
		NumSamples: len(labeled),
	})
	t.setLastSplit(shuffled[:splitIdx], shuffled[splitIdx:])

	return model, TrainResult{
		Accuracy:           valAcc,
		TrainAccuracy:      trainAcc,
		ValidationAccuracy: valAcc,
		NumSamples:         len(labeled),
		TrainSamples:       splitIdx,
		ValidationSamples:  valSamples,
	}
}
