package ml

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestMultiHotCategoriesPreserveHighImpactAndMultipleThreatVectors(t *testing.T) {
	var f [FeatureDim]float64
	// A single command may both inspect files and remove them, touch network
	// resources and edit credentials. The first one-hot bit is not exclusive.
	for _, category := range []int{1, 3, 5, 9, 14} {
		f[category] = 1
	}
	if !featureHighImpact(f, 1) {
		t.Fatal("FILE_READ masked FILE_DELETE / SENSITIVE high-impact evidence")
	}
	if got := featureImpactWeight(f, 1); got < 12 {
		t.Fatalf("multi-category sample was assigned benign impact %.4f", got)
	}
	vectors := threatVectorsFromFeatures(f)
	want := []ThreatVector{ThreatIntrusion, ThreatDestruction, ThreatPersistence}
	if !reflect.DeepEqual(vectors, want) {
		t.Fatalf("multi-category threat evidence was lost: got %v want %v", vectors, want)
	}
	metrics := EvaluateAttackImpactTrainSamples([]TrainSample{{features: f, label: 1}},
		func([FeatureDim]float64) int32 { return 0 })
	if metrics.HighImpactSamples != 1 || metrics.DestructionSamples != 1 ||
		metrics.IntrusionSamples != 1 || metrics.PersistenceSamples != 1 ||
		metrics.CatastrophicMissRate != 1 {
		t.Fatalf("multi-hot catastrophic miss was not counted: %+v", metrics)
	}
}

func TestMultiHotRiskMetricsProtectBenignAndDestructiveDistinctly(t *testing.T) {
	normal := TrainSample{label: 0}
	normal.features[1] = 1 // FILE_READ
	threat := TrainSample{label: 1}
	threat.features[1] = 1 // benign-looking category comes first
	threat.features[3] = 1 // but FILE_DELETE is also active
	threat.features[5] = 1
	holdout := []TrainSample{normal, threat}
	missed := EvaluateAttackImpactTrainSamples(holdout, func([FeatureDim]float64) int32 { return 0 })
	defended := EvaluateAttackImpactTrainSamples(holdout, func(f [FeatureDim]float64) int32 {
		if f[3] != 0 {
			return 1
		}
		return 0
	})
	if missed.HighImpactSamples != 1 || missed.CatastrophicMissRate != 1 ||
		defended.CatastrophicMissRate != 0 || defended.BenignFalsePositive != 0 {
		t.Fatalf("invalid security-cost evidence: missed=%+v defended=%+v", missed, defended)
	}
	if !(defended.SecurityUtility > missed.SecurityUtility) {
		t.Fatalf("well-calibrated containment must win security utility: missed=%f defended=%f",
			missed.SecurityUtility, defended.SecurityUtility)
	}
}

func TestAutoTuneForestMirrorsProductionBalancedForest(t *testing.T) {
	var training []TrainSample
	for c, count := range []int{75, 8, 4, 3} {
		for i := 0; i < count; i++ {
			var features [FeatureDim]float64
			features[0] = float64(c) / 3.0
			features[1] = float64(i+1) / float64(count+1)
			features[2] = float64(c*i+1) / 300.0
			training = append(training, TrainSample{features: features, label: int32(c)})
		}
	}
	const trees, maxDepth, minLeaf, seed = 12, 5, 2, int64(20261102)
	got := BuildAutoTuneForest(training, trees, maxDepth, minLeaf, seed)
	rng := rand.New(rand.NewSource(seed))
	want := NewDecisionForest(trees, maxDepth, 4)
	featureCount := int(math.Sqrt(float64(FeatureDim)))
	for i := range want.Trees {
		bootstrap := make([]TrainSample, len(training))
		classStratifiedBootstrap(training, bootstrap, rng)
		want.Trees[i] = DecisionTree{
			Nodes: buildTree(bootstrap, 0, maxDepth, minLeaf, featureCount, rng),
		}
	}
	want.IsTrained = true
	if !reflect.DeepEqual(got, want) {
		t.Fatal("autotune forest must use identical balanced bootstrap and tree builder")
	}
	// Pruning belongs to the grid candidate scorer, not the shared builder,
	// because the incremental sweep consumes exact estimator prefixes.
	got.Prune(training)
	want.Prune(training)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("tuned forest differs from production after training-only pruning")
	}
}

func TestAutoTuneForestRejectsEmptyTrainingSet(t *testing.T) {
	got := BuildAutoTuneForest(nil, 10, 5, 2, 42)
	if got == nil || got.IsTrained || len(got.Trees) != 0 {
		t.Fatalf("no-sample candidate must stay unfitted: %+v", got)
	}
}

func TestBalancedForestLearnRareAttackSignal(t *testing.T) {
	// Each feature carries class evidence, avoiding a dependency on which
	// subset sqrt(128) happens to be selected for any individual split.
	training := make([]TrainSample, 100)
	for i := range training {
		label := int32(0)
		if i >= 98 {
			label = 1
		}
		training[i].label = label
		for d := range training[i].features {
			training[i].features[d] = float64(label)
		}
	}
	forest := BuildAutoTuneForest(training, 16, 4, 1, 2026)
	var benign, attack [FeatureDim]float64
	for d := range attack {
		attack[d] = 1
	}
	if got := forest.Predict(benign).Action; got != 0 {
		t.Fatalf("balanced forest false positive on separable benign behavior: %d", got)
	}
	if got := forest.Predict(attack).Action; got != 1 {
		t.Fatalf("balanced forest missed separable rare BLOCK class: %d", got)
	}
}

func TestBalancedTuningBootstrapImprovesRareAttackHoldoutRecall(t *testing.T) {
	// Paired, fixed-seed ablation against the *former* uniform tuning
	// bootstrap. A single BLOCK example among 49 ALLOW examples is easily
	// absent from a uniform bootstrap, while class-stratified sampling keeps
	// the observed rare action represented.
	const n = 50
	training := make([]TrainSample, n)
	for i := range training {
		label := int32(0)
		value := 0.05 + float64(i%5)/200.0
		if i == n-1 {
			label = 1
			value = 0.92
		}
		training[i].label = label
		for d := range training[i].features {
			training[i].features[d] = value
		}
	}
	var benign, attack [FeatureDim]float64
	for d := range benign {
		benign[d] = 0.06
		attack[d] = 0.88 // held-out value, not identical to training BLOCK
	}
	const trials = 48
	balancedCaught, uniformCaught := 0, 0
	balancedFP, uniformFP := 0, 0
	for seed := int64(1); seed <= trials; seed++ {
		balanced := BuildAutoTuneForest(training, 1, 3, 1, seed)
		if balanced.Predict(attack).Action == 1 {
			balancedCaught++
		}
		if balanced.Predict(benign).Action != 0 {
			balancedFP++
		}
		rng := rand.New(rand.NewSource(seed))
		legacy := NewDecisionForest(1, 3, 4)
		bootstrap := make([]TrainSample, len(training))
		for i := range bootstrap {
			bootstrap[i] = training[rng.Intn(len(training))]
		}
		legacy.Trees[0] = DecisionTree{Nodes: buildAutoTuneTree(bootstrap, 0, 3, 1,
			int(math.Sqrt(float64(FeatureDim))), rng)}
		legacy.IsTrained = true
		if legacy.Predict(attack).Action == 1 {
			uniformCaught++
		}
		if legacy.Predict(benign).Action != 0 {
			uniformFP++
		}
	}
	t.Logf("synthetic rare-BLOCK holdout, %d seeds: class-balanced recall %d/%d, original uniform recall %d/%d; benign FP %d vs %d",
		trials, balancedCaught, trials, uniformCaught, trials, balancedFP, uniformFP)
	if balancedCaught != trials || balancedFP != 0 ||
		uniformCaught > balancedCaught-6 || uniformFP > balancedFP {
		t.Fatalf("balanced tuning no longer recovers rare threats without extra false positives: balanced=%d uniform=%d FPs=%d/%d",
			balancedCaught, uniformCaught, balancedFP, uniformFP)
	}
}
