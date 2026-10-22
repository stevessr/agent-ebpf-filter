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
)

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
	GeneratedAt                time.Time      `json:"generatedAt"`
	CandidateOnly              bool           `json:"candidateOnly"`
	ModelType                  string         `json:"modelType"`
	ModelPath                  string         `json:"modelPath"`
	DatasetLicense             string         `json:"datasetLicense"`
	DatasetSources             []string       `json:"datasetSources"`
	SourceRecords              int            `json:"sourceRecords"`
	Samples                    int            `json:"samples"`
	DuplicatesSkipped          int            `json:"duplicatesSkipped"`
	LabelCounts                map[string]int `json:"labelCounts"`
	TrainingSamples            int            `json:"trainingSamples"`
	ValidationSamples          int            `json:"validationSamples"`
	ValidationAccuracy         float64        `json:"validationAccuracy"`
	ValidationBalancedAccuracy float64        `json:"validationBalancedAccuracy"`
	ValidationAllowRecall      float64        `json:"validationAllowRecall"`
	ValidationBlockRecall      float64        `json:"validationBlockRecall"`
	ArtifactReloadVerified     bool           `json:"artifactReloadVerified"`
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
func TrainDecisionModelFromPublicDatasets(sources []DecisionTrainingDataset, modelPath string) (DecisionModelTrainingReport, error) {
	if len(sources) == 0 {
		return DecisionModelTrainingReport{}, fmt.Errorf("at least one public training dataset is required")
	}
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return DecisionModelTrainingReport{}, fmt.Errorf("model output path is required")
	}

	cfg := ml.DefaultMLConfig()
	cfg.ModelType = ml.ModelRandomForest
	cfg.LlmEnabled = false
	cfg.AutoTrain = false
	store := ml.NewTrainingDataStore(decisionTrainingStoreLimit)
	extractor := &FeatureExtractor{history: newRecentHistoryBuffer(1)}
	seen := make(map[string]int32)
	labelCounts := map[string]int{"ALLOW": 0, "BLOCK": 0}
	report := DecisionModelTrainingReport{
		GeneratedAt:    time.Now().UTC(),
		CandidateOnly:  true,
		ModelPath:      filepath.Clean(modelPath),
		DatasetLicense: decisionTrainingDatasetLicense,
		DatasetSources: make([]string, 0, len(sources)),
		LabelCounts:    labelCounts,
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
