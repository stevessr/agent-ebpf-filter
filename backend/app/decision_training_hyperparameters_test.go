package app

import "testing"

func TestNormalizeDecisionTrainingHyperparameters(t *testing.T) {
	defaults := DefaultDecisionTrainingHyperparameters()
	if defaults.NumTrees < 1 || defaults.MaxDepth < 1 || defaults.MinSamplesLeaf < 1 {
		t.Fatalf("promoted defaults must be positive: %+v", defaults)
	}

	partial, err := normalizeDecisionTrainingHyperparameters(DecisionTrainingHyperparameters{MaxDepth: 9})
	if err != nil {
		t.Fatalf("normalize partial override: %v", err)
	}
	if partial.NumTrees != defaults.NumTrees || partial.MinSamplesLeaf != defaults.MinSamplesLeaf {
		t.Fatalf("zero fields must fall back to promoted defaults: %+v", partial)
	}
	if partial.MaxDepth != 9 {
		t.Fatalf("explicit override lost: %+v", partial)
	}

	tests := []struct {
		name            string
		hyperparameters DecisionTrainingHyperparameters
		wantErr         bool
	}{
		{name: "all zero uses defaults"},
		{name: "valid explicit", hyperparameters: DecisionTrainingHyperparameters{NumTrees: 10, MaxDepth: 4, MinSamplesLeaf: 2}},
		{name: "negative trees", hyperparameters: DecisionTrainingHyperparameters{NumTrees: -1}, wantErr: true},
		{name: "trees above cap", hyperparameters: DecisionTrainingHyperparameters{NumTrees: maxDecisionTrainingNumTrees + 1}, wantErr: true},
		{name: "negative depth", hyperparameters: DecisionTrainingHyperparameters{MaxDepth: -3}, wantErr: true},
		{name: "depth above cap", hyperparameters: DecisionTrainingHyperparameters{MaxDepth: maxDecisionTrainingMaxDepth + 1}, wantErr: true},
		{name: "negative leaf", hyperparameters: DecisionTrainingHyperparameters{MinSamplesLeaf: -2}, wantErr: true},
		{name: "leaf above cap", hyperparameters: DecisionTrainingHyperparameters{MinSamplesLeaf: maxDecisionTrainingMinSamplesLeaf + 1}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeDecisionTrainingHyperparameters(test.hyperparameters)
			if test.wantErr && err == nil {
				t.Fatalf("expected error for %+v", test.hyperparameters)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error for %+v: %v", test.hyperparameters, err)
			}
		})
	}
}
