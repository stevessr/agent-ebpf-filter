package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"
)

const (
	bashDecisionDatasetRevision    = "fcac513317e1487c75f5d3fd0f50f3ddb2ba325a"
	decisionTrainingStoreLimit     = 100000
	decisionTrainingMinPerClass    = 20
	decisionTrainingDatasetLicense = "CC-BY-SA-4.0"

	// Hyperparameters tuned by the offline sweep (scripts/ml-sweep.sh --mode full,
	// repeats=3) against this public dataset: stability validation accuracy
	// 97.3%±0.5 for 72 trees/depth 14 versus ~95% for the previous
	// 31-tree/depth-8 defaults.
	//
	// Second refinement round (random_forest_leaf/random_forest_refine profiles,
	// 3 repeats) kept these values: 72/14/1 raised raw validation accuracy to
	// 97.5%±0.6 but cut BLOCK recall to 92.9% (balanced 95.5%), while this
	// configuration holds 97.2% accuracy with 97.0% BLOCK recall and 97.1%
	// balanced accuracy on the fixed grouped holdout.
	decisionTrainingNumTrees       = 72
	decisionTrainingMaxDepth       = 14
	decisionTrainingMinSamplesLeaf = 3

	// decisionTrainingModelType is the promoted model family. Other catalog
	// families stay reachable through the CLI so candidate artifacts can be
	// A/B compared against the promoted one on the same holdout.
	decisionTrainingModelType = string(ml.ModelRandomForest)
)

// DecisionTrainingHyperparameters are the model family and knobs selected
// offline by the sweep harness. They are recorded in the manifest so a
// candidate artifact can be reproduced without reading this source file.
// ModelType is a built-in catalog entry; the three numeric knobs mean whatever
// that family means by them (trees/depth/leaf for forests, learning
// rate/iterations for the linear families), matching the runtime MLConfig
// convention.
type DecisionTrainingHyperparameters struct {
	ModelType      string `json:"modelType"`
	NumTrees       int    `json:"numTrees"`
	MaxDepth       int    `json:"maxDepth"`
	MinSamplesLeaf int    `json:"minSamplesLeaf"`
}

const (
	maxDecisionTrainingNumTrees       = 4096
	maxDecisionTrainingMaxDepth       = 64
	maxDecisionTrainingMinSamplesLeaf = 256
)

// DefaultDecisionTrainingHyperparameters returns the promoted sweep winner.
// Change it only together with the tuning evidence in
// docs/backend/ml-models-summary.md and a fresh stability comparison.
func DefaultDecisionTrainingHyperparameters() DecisionTrainingHyperparameters {
	return DecisionTrainingHyperparameters{
		ModelType:      decisionTrainingModelType,
		NumTrees:       decisionTrainingNumTrees,
		MaxDepth:       decisionTrainingMaxDepth,
		MinSamplesLeaf: decisionTrainingMinSamplesLeaf,
	}
}

// normalizeDecisionTrainingHyperparameters resolves the model family and
// validates explicit overrides. A zero numeric knob means "use the family
// default" (the promoted trio for the promoted forest, the builtin catalog
// preset for any other family), so it is validated only when actually set.
func normalizeDecisionTrainingHyperparameters(hyperparameters DecisionTrainingHyperparameters) (DecisionTrainingHyperparameters, error) {
	hyperparameters.ModelType = strings.TrimSpace(hyperparameters.ModelType)
	if hyperparameters.ModelType == "" {
		hyperparameters.ModelType = decisionTrainingModelType
	}
	if !decisionTrainingModelTypeKnown(hyperparameters.ModelType) {
		return hyperparameters, fmt.Errorf("unknown model type %q", hyperparameters.ModelType)
	}
	for _, knob := range []struct {
		name     string
		value    int
		maxValue int
	}{
		{name: "numTrees", value: hyperparameters.NumTrees, maxValue: maxDecisionTrainingNumTrees},
		{name: "maxDepth", value: hyperparameters.MaxDepth, maxValue: maxDecisionTrainingMaxDepth},
		{name: "minSamplesLeaf", value: hyperparameters.MinSamplesLeaf, maxValue: maxDecisionTrainingMinSamplesLeaf},
	} {
		if knob.value < 0 {
			return hyperparameters, fmt.Errorf("%s must not be negative, got %d", knob.name, knob.value)
		}
		if knob.value > knob.maxValue {
			return hyperparameters, fmt.Errorf("%s must be between 1 and %d, got %d", knob.name, knob.maxValue, knob.value)
		}
	}
	return hyperparameters, nil
}

func decisionTrainingModelTypeKnown(modelType string) bool {
	for _, profile := range ml.BuiltinModelProfiles {
		if string(profile.Type) == modelType {
			return true
		}
	}
	return false
}

// resolveDecisionTrainingConfig maps requested hyperparameters onto a runtime
// MLConfig and returns the effective values for the manifest. The promoted
// family keeps its tuned trio; every other catalog family starts from its
// builtin preset, and explicit non-zero knobs override either one.
func resolveDecisionTrainingConfig(hyperparameters DecisionTrainingHyperparameters) (ml.MLConfig, DecisionTrainingHyperparameters, error) {
	normalized, err := normalizeDecisionTrainingHyperparameters(hyperparameters)
	if err != nil {
		return ml.MLConfig{}, hyperparameters, err
	}
	cfg := ml.DefaultMLConfig()
	cfg.LlmEnabled = false
	cfg.AutoTrain = false
	cfg.ModelType = ml.ModelType(normalized.ModelType)
	if normalized.ModelType == decisionTrainingModelType {
		cfg.NumTrees = decisionTrainingNumTrees
		cfg.MaxDepth = decisionTrainingMaxDepth
		cfg.MinSamplesLeaf = decisionTrainingMinSamplesLeaf
	} else {
		cfg = ml.ApplyBuiltinModelPresetForType(cfg, cfg.ModelType)
	}
	if normalized.NumTrees > 0 {
		cfg.NumTrees = normalized.NumTrees
	}
	if normalized.MaxDepth > 0 {
		cfg.MaxDepth = normalized.MaxDepth
	}
	if normalized.MinSamplesLeaf > 0 {
		cfg.MinSamplesLeaf = normalized.MinSamplesLeaf
	}
	normalized.NumTrees = cfg.NumTrees
	normalized.MaxDepth = cfg.MaxDepth
	normalized.MinSamplesLeaf = cfg.MinSamplesLeaf
	return cfg, normalized, nil
}

// DecisionTrainingDataset identifies one labeled JSONL source for offline training.
// FallbackURL is optional and must serve byte-identical content (same pinned revision).
type DecisionTrainingDataset struct {
	Name        string
	URL         string
	FallbackURL string
}

// DecisionModelTrainingReport records dataset provenance and held-out validation
// results for a candidate artifact. The workflow must not deploy it automatically.
type DecisionModelTrainingReport struct {
	GeneratedAt                time.Time                       `json:"generatedAt"`
	CandidateOnly              bool                            `json:"candidateOnly"`
	ModelType                  string                          `json:"modelType"`
	Hyperparameters            DecisionTrainingHyperparameters `json:"hyperparameters"`
	ModelPath                  string                          `json:"modelPath"`
	DatasetLicense             string                          `json:"datasetLicense"`
	DatasetSources             []string                        `json:"datasetSources"`
	SourceRecords              int                             `json:"sourceRecords"`
	Samples                    int                             `json:"samples"`
	DuplicatesSkipped          int                             `json:"duplicatesSkipped"`
	LabelCounts                map[string]int                  `json:"labelCounts"`
	TrainingSamples            int                             `json:"trainingSamples"`
	ValidationSamples          int                             `json:"validationSamples"`
	ValidationAccuracy         float64                         `json:"validationAccuracy"`
	ValidationBalancedAccuracy float64                         `json:"validationBalancedAccuracy"`
	ValidationAllowRecall      float64                         `json:"validationAllowRecall"`
	ValidationBlockRecall      float64                         `json:"validationBlockRecall"`
	ArtifactReloadVerified     bool                            `json:"artifactReloadVerified"`
}

// DefaultDecisionTrainingDatasets returns the reviewed, immutable public JSONL
// inputs used by the scheduled candidate-training workflow. Each source lists a
// primary GitHub raw URL plus an equivalent jsDelivr mirror fallback so the
// training run still works when raw.githubusercontent.com is unreachable.
func DefaultDecisionTrainingDatasets() []DecisionTrainingDataset {
	const (
		safeName   = "taglio4_3k/taglio4_safe.jsonl"
		unsafeName = "taglio4_3k/taglio4_unsafe.jsonl"
	)
	rawBase := "https://raw.githubusercontent.com/rivoluzione-informatica/bash-datasets/" + bashDecisionDatasetRevision + "/data/taglio4_3k/"
	mirrorBase := "https://cdn.jsdelivr.net/gh/rivoluzione-informatica/bash-datasets@" + bashDecisionDatasetRevision + "/data/taglio4_3k/"
	return []DecisionTrainingDataset{
		{Name: safeName, URL: rawBase + "taglio4_safe.jsonl", FallbackURL: mirrorBase + "taglio4_safe.jsonl"},
		{Name: unsafeName, URL: rawBase + "taglio4_unsafe.jsonl", FallbackURL: mirrorBase + "taglio4_unsafe.jsonl"},
	}
}

// TrainDecisionModelFromPublicDatasets downloads labeled JSONL command records,
// converts them with the runtime feature extractor, trains the default random
// forest, and writes a runtime-loadable candidate model. It does not publish the
// model into the active runtime or change policy configuration.
func TrainDecisionModelFromPublicDatasets(sources []DecisionTrainingDataset, modelPath string, hyperparameters DecisionTrainingHyperparameters) (DecisionModelTrainingReport, error) {
	if len(sources) == 0 {
		return DecisionModelTrainingReport{}, fmt.Errorf("at least one public training dataset is required")
	}
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return DecisionModelTrainingReport{}, fmt.Errorf("model output path is required")
	}
	cfg, hyperparameters, err := resolveDecisionTrainingConfig(hyperparameters)
	if err != nil {
		return DecisionModelTrainingReport{}, err
	}
	store := ml.NewTrainingDataStore(decisionTrainingStoreLimit)
	extractor := &FeatureExtractor{history: newRecentHistoryBuffer(1)}
	seen := make(map[string]int32)
	labelCounts := map[string]int{"ALLOW": 0, "BLOCK": 0}
	report := DecisionModelTrainingReport{
		GeneratedAt:     time.Now().UTC(),
		CandidateOnly:   true,
		Hyperparameters: hyperparameters,
		ModelPath:       filepath.Clean(modelPath),
		DatasetLicense:  decisionTrainingDatasetLicense,
		DatasetSources:  make([]string, 0, len(sources)),
		LabelCounts:     labelCounts,
	}

	for _, source := range sources {
		name := strings.TrimSpace(source.Name)
		if name == "" {
			return report, fmt.Errorf("public training dataset name is required")
		}
		url := strings.TrimSpace(source.URL)
		if _, err := validateRemoteDatasetURL(url); err != nil {
			return report, fmt.Errorf("validate dataset %q URL: %w", name, err)
		}
		parsed, err := pullRemoteDataset(remoteDatasetRequest{
			URL:            url,
			SourceName:     name,
			Format:         "jsonl",
			Limit:          remoteDatasetAbsoluteRecordLimit,
			LabelMode:      "preserve",
			ImportAll:      true,
			CleanSensitive: true,
		})
		if err != nil && strings.TrimSpace(source.FallbackURL) != "" {
			parsed, err = pullRemoteDataset(remoteDatasetRequest{
				URL:            strings.TrimSpace(source.FallbackURL),
				SourceName:     name,
				Format:         "jsonl",
				Limit:          remoteDatasetAbsoluteRecordLimit,
				LabelMode:      "preserve",
				ImportAll:      true,
				CleanSensitive: true,
			})
		}
		if err != nil {
			return report, fmt.Errorf("download dataset %q: %w", name, err)
		}
		if parsed.Truncated || parsed.TotalIsLowerBound {
			return report, fmt.Errorf("dataset %q was truncated at %d records", name, parsed.RecordLimit)
		}
		if parsed.Total == 0 || len(parsed.Rows) != parsed.Total {
			return report, fmt.Errorf("dataset %q returned %d stored rows from %d records", name, len(parsed.Rows), parsed.Total)
		}
		report.SourceRecords += parsed.Total
		report.DatasetSources = append(report.DatasetSources, parsed.Source)

		for _, row := range parsed.Rows {
			sample := buildRemoteDatasetSampleWithExtractor(row, "preserve", true, extractor)
			if !sample.IsLabeled() {
				return report, fmt.Errorf("dataset %q row %d has unsupported or missing label %q", name, row.Row, row.Label)
			}
			if sample.Label != 0 && sample.Label != 1 {
				return report, fmt.Errorf("dataset %q row %d has non-binary action label %d", name, row.Row, sample.Label)
			}
			key := behavior.CommandKey(sample.Comm, sample.Args)
			if previous, exists := seen[key]; exists {
				if previous != sample.Label {
					return report, fmt.Errorf("conflicting labels for duplicate command in dataset %q", name)
				}
				report.DuplicatesSkipped++
				continue
			}
			seen[key] = sample.Label
			store.Add(sample)
			labelCounts[ml.ActionLabel[sample.Label]]++
		}
	}

	if labelCounts["ALLOW"] < decisionTrainingMinPerClass || labelCounts["BLOCK"] < decisionTrainingMinPerClass {
		return report, fmt.Errorf("need at least %d ALLOW and BLOCK samples; got %d/%d", decisionTrainingMinPerClass, labelCounts["ALLOW"], labelCounts["BLOCK"])
	}
	report.Samples = labelCounts["ALLOW"] + labelCounts["BLOCK"]

	model, result := ml.GlobalTrainer.TrainWithConfig(store, cfg)
	if result.Error != "" || model == nil {
		if result.Error != "" {
			return report, fmt.Errorf("train decision model: %s", result.Error)
		}
		return report, fmt.Errorf("train decision model returned no model")
	}
	validationSamples := ml.GlobalTrainer.LastValidationSamples()
	if len(validationSamples) == 0 {
		return report, fmt.Errorf("training produced no validation samples")
	}
	metrics := ml.EvaluateAutoTuneTrainingSampleMetrics(validationSamples, model)
	blockTotal, blockCorrect := 0, 0
	for _, sample := range validationSamples {
		if sample.Label != 1 {
			continue
		}
		blockTotal++
		if model.Predict(sample.Features).Action == sample.Label {
			blockCorrect++
		}
	}

	if err := model.Serialize(report.ModelPath); err != nil {
		return report, fmt.Errorf("serialize candidate model: %w", err)
	}
	loaded := tryLoadModel(report.ModelPath, cfg.ModelType)
	if loaded == nil {
		return report, fmt.Errorf("runtime could not reload candidate model at %q", report.ModelPath)
	}
	for _, sample := range validationSamples {
		if loaded.Predict(sample.Features).Action != model.Predict(sample.Features).Action {
			return report, fmt.Errorf("reloaded model predictions differ from trained model")
		}
	}

	report.ModelType = string(model.Type())
	report.TrainingSamples = result.TrainSamples
	report.ValidationSamples = result.ValidationSamples
	report.ValidationAccuracy = result.ValidationAccuracy
	report.ValidationBalancedAccuracy = metrics.BalancedAccuracy
	report.ValidationAllowRecall = metrics.AllowRecall
	if blockTotal > 0 {
		report.ValidationBlockRecall = float64(blockCorrect) / float64(blockTotal)
	}
	report.ArtifactReloadVerified = true
	return report, nil
}
