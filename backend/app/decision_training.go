package app

import (
	"crypto/sha256"
	"fmt"
	"os"
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
	decisionTrainingSeed           = int64(20261108)
	decisionTrainingModelType      = string(ml.ModelRandomForest)
)

// DecisionTrainingHyperparameters records the model family, effective knobs,
// and deterministic offline seed for reproducible candidate-only artifacts.
// Non-forest families retain the built-in catalog defaults.
type DecisionTrainingHyperparameters struct {
	ModelType      string `json:"modelType"`
	NumTrees       int    `json:"numTrees"`
	MaxDepth       int    `json:"maxDepth"`
	MinSamplesLeaf int    `json:"minSamplesLeaf"`
	Seed           int64  `json:"seed"`
}

const (
	maxDecisionTrainingNumTrees       = 4096
	maxDecisionTrainingMaxDepth       = 64
	maxDecisionTrainingMinSamplesLeaf = 256
)

// DefaultDecisionTrainingHyperparameters returns the promoted sweep winner.
// Change it only together with the tuning evidence in
// docs/backend/ml-models-summary.md and a fresh stability comparison.
// Neutralize wall-clock-only dimensions when a reviewed command corpus lacks
// real event times. Otherwise all offline samples receive the training host's
// current hour/day, so a model can learn spurious clock correlations and then
// fail after deployment at another time. Runtime extraction is unchanged.
// Group E: hour, weekday, circular hour, weekend in features [114, 118].
func neutralizeOfflineDecisionClockFeatures(sample *ml.TrainingSample) {
	for i := 114; i <= 118; i++ {
		sample.Features[i] = 0
	}
}

func DefaultDecisionTrainingHyperparameters() DecisionTrainingHyperparameters {
	return DecisionTrainingHyperparameters{
		ModelType:      decisionTrainingModelType,
		NumTrees:       decisionTrainingNumTrees,
		MaxDepth:       decisionTrainingMaxDepth,
		MinSamplesLeaf: decisionTrainingMinSamplesLeaf,
		Seed:           decisionTrainingSeed,
	}
}

// normalizeDecisionTrainingHyperparameters treats zero fields as "use the
// promoted default" so callers can override only the knobs they are testing.
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
	if hyperparameters.Seed == 0 {
		hyperparameters.Seed = decisionTrainingSeed
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
	cfg.TrainingSeed = normalized.Seed
	return cfg, normalized, nil
}

// DecisionTrainingDataset identifies one labeled JSONL source for offline training.
// FallbackURL is optional and must serve byte-identical content (same pinned revision).
type DecisionTrainingDataset struct {
	Name           string
	URL            string
	FallbackURL    string
	LocalPath      string
	ExpectedSHA256 string
	License        string
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
	DatasetDigests             map[string]string               `json:"datasetDigests,omitempty"`
	DatasetLicenses            map[string]string               `json:"datasetLicenses,omitempty"`
	ValidationBlockPrecision  float64                         `json:"validationBlockPrecision"`
	ValidationBenignFPR        float64                         `json:"validationBenignFalsePositiveRate"`
	ValidationMacroF1          float64                         `json:"validationMacroF1"`
	ValidationConfusion        map[string]map[string]int       `json:"validationConfusion"`
	ExternalTest               *DecisionExternalEvaluation       `json:"externalTest,omitempty"`
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

// TrainDecisionModelFromPublicDatasets loads pinned, explicitly labeled commands,
// extracts canonical runtime features, and trains an offline candidate model.
// No artifact is promoted or enforced automatically.
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
		DatasetDigests:  make(map[string]string),
		DatasetLicenses: make(map[string]string),
		LabelCounts:     labelCounts,
	}

	for _, source := range sources {
		name := strings.TrimSpace(source.Name)
		if name == "" {
			return report, fmt.Errorf("public training dataset name is required")
		}
		if _, duplicate := report.DatasetLicenses[name]; duplicate {
			return report, fmt.Errorf("duplicate source name %q", name)
		}
		license := strings.TrimSpace(source.License)
		localPath := strings.TrimSpace(source.LocalPath)
		if license == "" {
			if localPath != "" {
				return report, fmt.Errorf("local dataset %q requires an explicit license", name)
			}
			license = decisionTrainingDatasetLicense
		}
		report.DatasetLicenses[name] = license
		if len(report.DatasetLicenses) == 1 {
			report.DatasetLicense = license
		} else if report.DatasetLicense != license {
			report.DatasetLicense = "MIXED (see datasetLicenses)"
		}

		request := remoteDatasetRequest{
			SourceName: name, Format: "jsonl", Limit: remoteDatasetAbsoluteRecordLimit,
			LabelMode: "preserve", ImportAll: true, CleanSensitive: true,
		}
		var parsed *remoteDatasetResponse
		var err error
		if localPath != "" {
			if strings.TrimSpace(source.URL) != "" || strings.TrimSpace(source.FallbackURL) != "" {
				return report, fmt.Errorf("dataset %q must use either local file or URL, not both", name)
			}
			expected := strings.ToLower(strings.TrimSpace(source.ExpectedSHA256))
			if len(expected) != 64 {
				return report, fmt.Errorf("local dataset %q requires a 64-character SHA-256 digest", name)
			}
			for _, char := range expected {
				if !strings.ContainsRune("0123456789abcdef", char) {
					return report, fmt.Errorf("invalid SHA-256 for dataset %q", name)
				}
			}
			info, statErr := os.Stat(localPath)
			if statErr != nil {
				return report, fmt.Errorf("stat local dataset %q: %w", name, statErr)
			}
			if !info.Mode().IsRegular() || info.Size() > remoteDatasetUploadLimitBytes {
				return report, fmt.Errorf("local dataset %q must be regular and <= %d bytes", name, remoteDatasetUploadLimitBytes)
			}
			raw, readErr := os.ReadFile(localPath)
			if readErr != nil {
				return report, fmt.Errorf("read local dataset %q: %w", name, readErr)
			}
			if len(raw) > int(remoteDatasetUploadLimitBytes) {
				return report, fmt.Errorf("local dataset %q exceeds %d bytes", name, remoteDatasetUploadLimitBytes)
			}
			actual := fmt.Sprintf("%x", sha256.Sum256(raw))
			if actual != expected {
				return report, fmt.Errorf("SHA-256 mismatch for %q: expected %s, received %s", name, expected, actual)
			}
			report.DatasetDigests[name] = actual
			request.Content = string(raw)
			parsed, err = pullRemoteDataset(request)
		} else {
			url := strings.TrimSpace(source.URL)
			if _, validateErr := validateRemoteDatasetURL(url); validateErr != nil {
				return report, fmt.Errorf("validate dataset %q URL: %w", name, validateErr)
			}
			request.URL = url
			parsed, err = pullRemoteDataset(request)
			if err != nil && strings.TrimSpace(source.FallbackURL) != "" {
				request.URL = strings.TrimSpace(source.FallbackURL)
				if _, fallbackErr := validateRemoteDatasetURL(request.URL); fallbackErr != nil {
					return report, fmt.Errorf("validate fallback URL for dataset %q: %w", name, fallbackErr)
				}
				parsed, err = pullRemoteDataset(request)
			}
		}
		if err != nil {
			return report, fmt.Errorf("load dataset %q: %w", name, err)
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
			// Attack windows and filenames are not trustworthy action labels.
			if localPath != "" && row.LabelSource != "dataset" {
				return report, fmt.Errorf("local dataset %q row %d lacks explicit action label", name, row.Row)
			}
			sample := buildRemoteDatasetSampleWithExtractor(row, "preserve", true, extractor)
			neutralizeOfflineDecisionClockFeatures(&sample)
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
	allowTotal, allowCorrect, blockPredicted, benignFalsePositive := 0, 0, 0, 0
	confusion := map[string]map[string]int{
		"ALLOW": {"ALLOW": 0, "BLOCK": 0, "REWRITE": 0, "ALERT": 0},
		"BLOCK": {"ALLOW": 0, "BLOCK": 0, "REWRITE": 0, "ALERT": 0},
	}
	for _, sample := range validationSamples {
		predicted := model.Predict(sample.Features).Action
		if label, valid := ml.ActionLabel[sample.Label]; valid {
			if prediction, valid := ml.ActionLabel[predicted]; valid {
				confusion[label][prediction]++
			}
		}
		if predicted == 1 {
			blockPredicted++
		}
		if sample.Label == 0 {
			allowTotal++
			if predicted == 0 {
				allowCorrect++
			} else {
				benignFalsePositive++
			}
		}
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
	if blockPredicted > 0 {
		report.ValidationBlockPrecision = float64(blockCorrect) / float64(blockPredicted)
	}
	if allowTotal > 0 {
		report.ValidationBenignFPR = float64(benignFalsePositive) / float64(allowTotal)
	}
	allowPredicted := 0
	for _, counts := range confusion {
		allowPredicted += counts["ALLOW"]
	}
	allowF1, blockF1 := 0.0, 0.0
	if denom := allowTotal + allowPredicted; denom > 0 {
		allowF1 = 2 * float64(allowCorrect) / float64(denom)
	}
	if denom := blockTotal + blockPredicted; denom > 0 {
		blockF1 = 2 * float64(blockCorrect) / float64(denom)
	}
	report.ValidationMacroF1 = (allowF1 + blockF1) / 2
	report.ValidationConfusion = confusion
	report.ArtifactReloadVerified = true
	return report, nil
}
