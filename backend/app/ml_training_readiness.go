package app

import (
	"fmt"
	"math"
	"strings"

	"agent-ebpf-filter/core"
)

// MLTrainingReadiness explains whether the persisted training store is ready
// for a supervised model build.  It is intentionally derived from the training
// data store at request time so the UI, API clients and async import paths share
// one source of truth for pre-train quality gates.
type MLTrainingReadiness struct {
	Ready            bool                       `json:"ready"`
	SampleCount      int                        `json:"sampleCount"`
	LabeledCount     int                        `json:"labeledCount"`
	UnlabeledCount   int                        `json:"unlabeledCount"`
	MinSamples       int                        `json:"minSamples"`
	MinClasses       int                        `json:"minClasses"`
	ClassCount       int                        `json:"classCount"`
	FeatureDim       int                        `json:"featureDim"`
	ByLabel          []researchCount            `json:"byLabel,omitempty"`
	ByCategory       []researchCount            `json:"byCategory,omitempty"`
	Normalization    FeatureNormalizationReport `json:"normalization"`
	Quality          DatasetQualitySummary      `json:"quality"`
	BlockingReasons  []string                   `json:"blockingReasons,omitempty"`
	Warnings         []string                   `json:"warnings,omitempty"`
	SuggestedActions []string                   `json:"suggestedActions,omitempty"`
}

func buildMLTrainingReadiness(store *TrainingDataStore, cfg MLConfig) MLTrainingReadiness {
	minSamples := mlTrainingReadinessMinSamples(cfg)
	readiness := MLTrainingReadiness{
		Ready:         false,
		MinSamples:    minSamples,
		MinClasses:    2,
		FeatureDim:    FeatureDim,
		Normalization: summarizeFeatureNormalization(nil),
		Quality:       buildDatasetQualitySummary(0, 0, 0, 0, 0, nil, summarizeFeatureNormalization(nil)),
	}
	if store == nil {
		readiness.BlockingReasons = append(readiness.BlockingReasons, "training_store_unavailable")
		readiness.Warnings = append(readiness.Warnings, "training_store_unavailable")
		readiness.SuggestedActions = append(readiness.SuggestedActions, "restart_backend_or_initialize_ml_training_store")
		return readiness
	}

	allSamples := store.AllSamples()
	labeledSamples := make([]TrainingSample, 0, len(allSamples))
	byLabel := map[string]int{}
	byCategory := map[string]int{}
	classLabels := map[string]int{}
	for _, sample := range allSamples {
		label := normalizedTrainingSampleLabelKey(sample)
		incrementResearchCount(byLabel, label)
		incrementResearchCount(byCategory, normalizedDatasetCategoryKey(sample.Category))
		if label == "UNLABELED" {
			readiness.UnlabeledCount++
			continue
		}
		labeledSamples = append(labeledSamples, sample)
		classLabels[label]++
	}

	readiness.SampleCount = len(allSamples)
	readiness.LabeledCount = len(labeledSamples)
	readiness.ClassCount = len(classLabels)
	readiness.ByLabel = topResearchCounts(byLabel, 0)
	readiness.ByCategory = topResearchCounts(byCategory, 10)
	readiness.Normalization = summarizeFeatureNormalization(labeledSamples)
	readiness.Quality = datasetQualityFromTrainingSamples(allSamples, readiness.Normalization)

	if readiness.LabeledCount < minSamples {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("insufficient_labeled_samples:%d/%d", readiness.LabeledCount, minSamples))
	}
	if readiness.LabeledCount == 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, "no_labeled_samples")
	}
	if readiness.LabeledCount > 0 && readiness.ClassCount < readiness.MinClasses {
		readiness.BlockingReasons = append(readiness.BlockingReasons, "single_class_training_data")
	}
	if readiness.Normalization.NonFiniteValues > 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("non_finite_feature_values:%d", readiness.Normalization.NonFiniteValues))
	}
	outOfRange := readiness.Normalization.BelowZeroValues + readiness.Normalization.AboveOneValues
	if outOfRange > 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("feature_values_out_of_range:%d", outOfRange))
	}

	readiness.Warnings = append(readiness.Warnings, readiness.Quality.Warnings...)
	if readiness.UnlabeledCount > 0 {
		readiness.Warnings = append(readiness.Warnings, fmt.Sprintf("unlabeled_samples:%d", readiness.UnlabeledCount))
	}
	if readiness.Normalization.ZeroVarianceFeatures > int(math.Round(float64(FeatureDim)*0.90)) && readiness.LabeledCount >= 2 {
		readiness.Warnings = append(readiness.Warnings, fmt.Sprintf("low_feature_variance:%d", readiness.Normalization.ZeroVarianceFeatures))
	}
	readiness.Warnings = uniqueStringsPreserveOrder(readiness.Warnings)

	readiness.SuggestedActions = mlTrainingReadinessSuggestedActions(readiness)
	readiness.Ready = len(readiness.BlockingReasons) == 0
	return readiness
}

func mlTrainingReadinessMinSamples(cfg MLConfig) int {
	defaults := DefaultMLConfig()
	minSamples := cfg.MinSamplesForTraining
	if minSamples <= 0 {
		minSamples = defaults.MinSamplesForTraining
	}
	effective := applyBuiltinModelPreset(cfg)
	if effective.ModelType == "" {
		effective.ModelType = defaults.ModelType
	}
	if effective.MinSamplesLeaf <= 0 {
		effective.MinSamplesLeaf = defaults.MinSamplesLeaf
	}

	modelMin := 2
	switch effective.ModelType {
	case ModelRandomForest, ModelExtraTrees, ModelAdaBoost, ModelEnsemble, core.ModelGraphLearning:
		modelMin = effective.MinSamplesLeaf * 10
	}
	if modelMin > minSamples {
		return modelMin
	}
	return minSamples
}

func normalizedTrainingSampleLabelKey(sample TrainingSample) string {
	if sample.IsLabeled() {
		if label, ok := actionLabel[sample.Label]; ok {
			return label
		}
	}
	return "UNLABELED"
}

func datasetQualityFromTrainingSamples(samples []TrainingSample, normalization FeatureNormalizationReport) DatasetQualitySummary {
	labels := map[string]int{}
	seenCommands := map[string]int{}
	importable := 0
	labeled := 0
	unlabeled := 0
	duplicates := 0
	for _, sample := range samples {
		label := normalizedTrainingSampleLabelKey(sample)
		labels[label]++
		if label == "UNLABELED" {
			unlabeled++
		} else {
			labeled++
		}
		if strings.TrimSpace(sample.Comm) != "" {
			importable++
		}
		key := trainingSampleDuplicateKey(sample)
		if key == "" {
			continue
		}
		seenCommands[key]++
		if seenCommands[key] > 1 {
			duplicates++
		}
	}
	return buildDatasetQualitySummary(len(samples), importable, labeled, unlabeled, duplicates, labels, normalization)
}

func trainingSampleDuplicateKey(sample TrainingSample) string {
	comm := strings.TrimSpace(sample.Comm)
	if comm == "" {
		return ""
	}
	args := append([]string(nil), sample.Args...)
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
	}
	return comm + "\x00" + strings.Join(args, "\x00")
}

func mlTrainingReadinessSuggestedActions(readiness MLTrainingReadiness) []string {
	actions := []string{}
	reasons := strings.Join(readiness.BlockingReasons, "|")
	warnings := strings.Join(readiness.Warnings, "|")
	if strings.Contains(reasons, "training_store_unavailable") {
		actions = append(actions, "restart_backend_or_initialize_ml_training_store")
	}
	if strings.Contains(reasons, "insufficient_labeled_samples") || strings.Contains(reasons, "no_labeled_samples") {
		actions = append(actions,
			"import_agent_legal_dataset_or_selinux_policy_dataset",
			"build_research_session_training_dataset",
			"add_manual_or_feedback_labeled_samples",
		)
	}
	if strings.Contains(reasons, "single_class_training_data") || strings.Contains(warnings, "class_imbalance") {
		actions = append(actions,
			"add_counter_class_samples_for_allow_block_alert_rewrite",
			"run_heuristic_or_llm_label_review_before_training",
		)
	}
	if readiness.UnlabeledCount > 0 {
		actions = append(actions, "review_or_auto_label_unlabeled_samples")
	}
	if strings.Contains(reasons, "feature_values_out_of_range") || strings.Contains(reasons, "non_finite_feature_values") || strings.Contains(warnings, "feature_out_of_range") {
		actions = append(actions, "reimport_dataset_with_feature_normalization_enabled")
	}
	if readiness.Quality.DuplicateCount > 0 {
		actions = append(actions, "deduplicate_exact_command_samples")
	}
	if len(actions) == 0 {
		actions = append(actions, "training_data_ready_review_metrics_then_train")
	}
	return uniqueStringsPreserveOrder(actions)
}

func uniqueStringsPreserveOrder(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
