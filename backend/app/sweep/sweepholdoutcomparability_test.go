package sweep

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasHoldoutSplitCounts(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name              string
		numSamples        int
		trainSamples      int
		validationSamples int
		want              bool
	}{
		// Training-only scores lack validation samples and cannot establish a holdout.
		{name: "training-only result has no validation samples", numSamples: 2187, trainSamples: 2187},
		{name: "no samples at all", numSamples: 0, trainSamples: 0, validationSamples: 0},
		{name: "holdout split", numSamples: 2187, trainSamples: 1749, validationSamples: 438, want: true},
		{name: "grouped holdout excludes validation groups", numSamples: 2187, trainSamples: 1600, validationSamples: 438, want: true},
		{name: "validation leaves training short", numSamples: 100, trainSamples: 80, validationSamples: 21},
		{name: "empty training split", numSamples: 100, validationSamples: 20},
		{name: "negative training count", numSamples: 100, trainSamples: -1, validationSamples: 20},
		{name: "overflowing counts", numSamples: maxInt, trainSamples: maxInt, validationSamples: maxInt},
		{name: "tight split", numSamples: 100, trainSamples: 80, validationSamples: 20, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasHoldoutSplitCounts(test.numSamples, test.trainSamples, test.validationSamples); got != test.want {
				t.Fatalf("hasHoldoutSplitCounts(%d, %d, %d) = %v, want %v",
					test.numSamples, test.trainSamples, test.validationSamples, got, test.want)
			}
		})
	}
}

func holdoutRun(validationSamples int) repeatRunResult {
	return repeatRunResult{
		Profile:           "all/model_catalog",
		ModelType:         "random_forest",
		NumSamples:        1000,
		TrainSamples:      1000 - validationSamples,
		ValidationSamples: validationSamples,
		HoldoutStrategy:   holdoutStrategyGroupedBehaviorV1,
	}
}

func noHoldoutRun() repeatRunResult {
	return repeatRunResult{
		Profile:      "all/model_catalog",
		ModelType:    "svm",
		NumSamples:   1000,
		TrainSamples: 1000,
	}
}

func TestAggregateRepeatRunsDerivesComparabilityFromSamples(t *testing.T) {
	honest, err := aggregateRepeatRuns([]repeatRunResult{holdoutRun(200), holdoutRun(200), holdoutRun(200)})
	if err != nil {
		t.Fatalf("aggregate holdout runs: %v", err)
	}
	if !honest.Comparable {
		t.Fatal("runs scored on a holdout must be comparable")
	}

	trainSetScored, err := aggregateRepeatRuns([]repeatRunResult{noHoldoutRun(), noHoldoutRun()})
	if err != nil {
		t.Fatalf("aggregate train-set runs: %v", err)
	}
	if trainSetScored.Comparable {
		t.Fatal("runs that fit and score every sample must not be comparable")
	}
}

func TestBestScreenSummaryPrefersHoldoutProfiles(t *testing.T) {
	trainSet := profileSummary{
		Profile: sweepProfile{Name: "all/svm"},
		Best: sweepResult{
			Profile: "all/svm", ModelType: "svm",
			NumSamples: 1000, TrainSamples: 1000,
			BalancedAccuracy: 0.99, ValidationAccuracy: 0.99,
		},
	}
	holdout := profileSummary{
		Profile: sweepProfile{Name: "all/random_forest"},
		Best: sweepResult{
			Profile: "all/random_forest", ModelType: "random_forest",
			NumSamples: 1000, TrainSamples: 800, ValidationSamples: 200,
			HoldoutStrategy:  holdoutStrategyGroupedBehaviorV1,
			BalancedAccuracy: 0.90, ValidationAccuracy: 0.90,
		},
	}
	best := bestScreenSummary([]profileSummary{trainSet, holdout})
	if best == nil || best.Profile.Name != "all/random_forest" {
		t.Fatalf("grid best must come from a holdout-scored profile, got %+v", best)
	}

	// A run with no honest alternative still reports something instead of nothing.
	only := bestScreenSummary([]profileSummary{trainSet})
	if only == nil || only.Profile.Name != "all/svm" {
		t.Fatalf("fallback must still report the train-set winner, got %+v", only)
	}
}

func TestMixedProfileBestAndStabilitySkipInSampleRows(t *testing.T) {
	rows := []sweepResult{
		{
			Profile: "all/model_catalog", ModelType: "svm", ConfigSummary: "in-sample-high-score",
			NumSamples: 100, TrainSamples: 100,
			BalancedAccuracy: 0.99, ValidationAccuracy: 0.99,
		},
		{
			Profile: "all/model_catalog", ModelType: "random_forest", ConfigSummary: "holdout-row",
			NumSamples: 100, TrainSamples: 80, ValidationSamples: 20,
			HoldoutStrategy:  holdoutStrategyGroupedBehaviorV1,
			BalancedAccuracy: 0.90, ValidationAccuracy: 0.90,
		},
	}
	if best := bestSweepResult(rows); best.ConfigSummary != "holdout-row" {
		t.Fatalf("bestSweepResult chose %q, want the holdout row", best.ConfigSummary)
	}
	tasks := selectTopRepeatConfigs(sweepProfile{Name: "all/model_catalog", Kind: "bar"}, rows, 2, nil, nil)
	if len(tasks) != 1 || tasks[0].Config.ConfigSummary != "holdout-row" {
		t.Fatalf("stability tasks = %+v, want only the holdout row", tasks)
	}
}

func TestAggregateRepeatRunsRejectsMixedHoldoutValidity(t *testing.T) {
	withMixedRuns, err := aggregateRepeatRuns([]repeatRunResult{holdoutRun(200), noHoldoutRun()})
	if err != nil {
		t.Fatalf("aggregate mixed runs: %v", err)
	}
	if withMixedRuns.Comparable {
		t.Fatal("a repeat summary with any successful train-set-scored run must not be comparable")
	}
}

func TestBestComparableSummarySkipsTrainSetScoredRuns(t *testing.T) {
	trainSet, err := aggregateRepeatRuns([]repeatRunResult{noHoldoutRun()})
	if err != nil {
		t.Fatalf("aggregate train-set runs: %v", err)
	}
	trainSet.BalancedMean = 0.99
	honest, err := aggregateRepeatRuns([]repeatRunResult{holdoutRun(200)})
	if err != nil {
		t.Fatalf("aggregate holdout runs: %v", err)
	}
	honest.BalancedMean = 0.90

	best := bestComparableSummary([]repeatSummary{trainSet, honest})
	if best == nil {
		t.Fatal("expected a comparable winner")
	}
	if best.BalancedMean != 0.90 {
		t.Fatalf("comparable best must ignore train-set-scored runs, got %+v", best)
	}
}

func TestHoldoutComparabilityRequiresGroupedStrategy(t *testing.T) {
	row := sweepResult{NumSamples: 100, TrainSamples: 80, ValidationSamples: 20}
	if sweepResultHasHoldout(row) {
		t.Fatal("counts without grouped holdout provenance must not be comparable")
	}
	row.HoldoutStrategy = "row_wise_v0"
	if sweepResultHasHoldout(row) {
		t.Fatal("unknown split strategy must not be comparable")
	}
	row.HoldoutStrategy = holdoutStrategyGroupedBehaviorV1
	if !sweepResultHasHoldout(row) {
		t.Fatal("current grouped holdout with valid counts must be comparable")
	}

	run := repeatRunResult{NumSamples: 100, TrainSamples: 80, ValidationSamples: 20}
	if repeatRunHasHoldout(run) {
		t.Fatal("repeat counts without grouped holdout provenance must not be comparable")
	}
	run.HoldoutStrategy = "row_wise_v0"
	if repeatRunHasHoldout(run) {
		t.Fatal("repeat with unknown split strategy must not be comparable")
	}
	run.HoldoutStrategy = holdoutStrategyGroupedBehaviorV1
	if !repeatRunHasHoldout(run) {
		t.Fatal("current grouped repeat holdout with valid counts must be comparable")
	}
}

func TestSweepResultsCSVCurrentSchemaRejectsLegacyRows(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{
			name:   "legacy holdout counts have no provenance",
			header: "allowPassRate,blockPassRate,balancedAccuracy",
		},
		{
			name:   "current grouped holdout schema",
			header: "allowPassRate,blockPassRate,balancedAccuracy,holdoutStrategy",
			want:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, test.name+".csv")
			if err := os.WriteFile(path, []byte(test.header+"\n"), 0o600); err != nil {
				t.Fatalf("write schema fixture: %v", err)
			}
			if got := sweepResultsCSVHasCurrentSchema(path); got != test.want {
				t.Fatalf("sweepResultsCSVHasCurrentSchema() = %v, want %v", got, test.want)
			}
		})
	}
}
