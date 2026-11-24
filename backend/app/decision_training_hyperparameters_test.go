package app

import (
	"testing"

	"agent-ebpf-filter/app/ml"
)

func TestNormalizeDecisionTrainingHyperparameters(t *testing.T) {
	promoted := DefaultDecisionTrainingHyperparameters()
	if promoted.ModelType != decisionTrainingModelType {
		t.Fatalf("promoted model type = %q, want %q", promoted.ModelType, decisionTrainingModelType)
	}
	if promoted.NumTrees < 1 || promoted.MaxDepth < 1 || promoted.MinSamplesLeaf < 1 {
		t.Fatalf("promoted knobs must be positive: %+v", promoted)
	}

	empty, err := normalizeDecisionTrainingHyperparameters(DecisionTrainingHyperparameters{})
	if err != nil {
		t.Fatalf("normalize empty: %v", err)
	}
	if empty.ModelType != decisionTrainingModelType {
		t.Fatalf("empty model type must resolve to the promoted family, got %q", empty.ModelType)
	}
	if empty.NumTrees != 0 || empty.MaxDepth != 0 || empty.MinSamplesLeaf != 0 {
		t.Fatalf("zero knobs mean \"family default\" and must stay zero after normalize: %+v", empty)
	}

	tests := []struct {
		name            string
		hyperparameters DecisionTrainingHyperparameters
		wantErr         bool
	}{
		{name: "all zero uses family defaults"},
		{name: "valid explicit forest", hyperparameters: DecisionTrainingHyperparameters{ModelType: decisionTrainingModelType, NumTrees: 10, MaxDepth: 4, MinSamplesLeaf: 2}},
		{name: "valid catalog variant", hyperparameters: DecisionTrainingHyperparameters{ModelType: "perceptron_long"}},
		{name: "unknown model type", hyperparameters: DecisionTrainingHyperparameters{ModelType: "not_a_model"}, wantErr: true},
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

func TestResolveDecisionTrainingConfig(t *testing.T) {
	cfg, effective, err := resolveDecisionTrainingConfig(DecisionTrainingHyperparameters{})
	if err != nil {
		t.Fatalf("resolve defaults: %v", err)
	}
	if string(cfg.ModelType) != decisionTrainingModelType {
		t.Fatalf("default config type = %q, want %q", cfg.ModelType, decisionTrainingModelType)
	}
	if cfg.NumTrees != decisionTrainingNumTrees || cfg.MaxDepth != decisionTrainingMaxDepth || cfg.MinSamplesLeaf != decisionTrainingMinSamplesLeaf {
		t.Fatalf("promoted family must keep its tuned trio, got %+v", cfg)
	}
	if effective.NumTrees != cfg.NumTrees || effective.MaxDepth != cfg.MaxDepth || effective.MinSamplesLeaf != cfg.MinSamplesLeaf {
		t.Fatalf("manifest values must record the effective config: %+v vs %+v", effective, cfg)
	}
	if cfg.LlmEnabled || cfg.AutoTrain {
		t.Fatalf("candidate training must not enable LLM or auto-train: %+v", cfg)
	}

	overridden, _, err := resolveDecisionTrainingConfig(DecisionTrainingHyperparameters{NumTrees: 11})
	if err != nil {
		t.Fatalf("resolve override: %v", err)
	}
	if overridden.NumTrees != 11 {
		t.Fatalf("explicit knob must win over the promoted default, got %d", overridden.NumTrees)
	}
	if overridden.MaxDepth != decisionTrainingMaxDepth || overridden.MinSamplesLeaf != decisionTrainingMinSamplesLeaf {
		t.Fatalf("unset knobs must keep the promoted values, got %+v", overridden)
	}

	// A catalog variant keeps its identity and preset knobs; explicit knobs
	// override the preset but never the family.
	variant := catalogVariantWithDistinctKnobs(t)
	preset, effectivePreset, err := resolveDecisionTrainingConfig(DecisionTrainingHyperparameters{ModelType: string(variant)})
	if err != nil {
		t.Fatalf("resolve %s: %v", variant, err)
	}
	if preset.ModelType != variant {
		t.Fatalf("catalog variant %s was collapsed onto %s", variant, preset.ModelType)
	}
	if preset.NumTrees == 0 || preset.MaxDepth == 0 {
		t.Fatalf("catalog variant %s must apply its preset knobs, got %+v", variant, preset)
	}
	if effectivePreset.ModelType != string(variant) {
		t.Fatalf("manifest model type = %q, want %q", effectivePreset.ModelType, variant)
	}
	if effectivePreset.NumTrees != preset.NumTrees || effectivePreset.MaxDepth != preset.MaxDepth || effectivePreset.MinSamplesLeaf != preset.MinSamplesLeaf {
		t.Fatalf("manifest knobs must match the preset: %+v vs %+v", effectivePreset, preset)
	}

	forced, _, err := resolveDecisionTrainingConfig(DecisionTrainingHyperparameters{ModelType: string(variant), NumTrees: 7})
	if err != nil {
		t.Fatalf("resolve forced knob: %v", err)
	}
	if forced.NumTrees != 7 {
		t.Fatalf("explicit knob must override the catalog preset, got %d", forced.NumTrees)
	}
	if forced.ModelType != variant {
		t.Fatalf("explicit knob must not change the family, got %s", forced.ModelType)
	}
}

// catalogVariantWithDistinctKnobs returns a catalog entry whose preset knobs
// differ from MLConfig defaults, so a test can tell preset application apart
// from doing nothing.
func catalogVariantWithDistinctKnobs(t *testing.T) ml.ModelType {
	t.Helper()
	defaults := ml.DefaultMLConfig()
	for _, profile := range ml.BuiltinModelProfiles {
		if profile.Type == profile.Base {
			continue
		}
		numTrees := profile.Defaults["numTrees"]
		maxDepth := profile.Defaults["maxDepth"]
		if numTrees > 0 && maxDepth > 0 && (numTrees != defaults.NumTrees || maxDepth != defaults.MaxDepth) {
			return profile.Type
		}
	}
	t.Skip("no catalog variant with distinct preset knobs")
	return ""
}

func TestOfflineTrainingSeedIsRecordedAndDeterministic(t *testing.T) {
	defaults := DefaultDecisionTrainingHyperparameters()
	if defaults.Seed == 0 {
		t.Fatal("offline candidate default seed must be nonzero")
	}
	config, values, err := resolveDecisionTrainingConfig(DecisionTrainingHyperparameters{Seed: 12345})
	if err != nil {
		t.Fatal(err)
	}
	if values.Seed != 12345 || config.TrainingSeed != 12345 {
		t.Fatalf("explicit offline seed lost: config=%+v values=%+v", config.TrainingSeed, values)
	}
}
