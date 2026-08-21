package ml

import (
	"fmt"
	"strings"

	"agent-ebpf-filter/core"
)

// Dataset-quality builders shared by the app dataset pipeline and the
// research training views.

func NormalizedDatasetCategoryKey(raw string) string {
	category := strings.TrimSpace(raw)
	if category == "" || category == "-" {
		return "UNCATEGORIZED"
	}
	return category
}

func BuildDatasetQualitySummary(total, importable, labeled, unlabeled, duplicates int, labels map[string]int, normalization FeatureNormalizationReport) core.DatasetQualitySummary {
	dominantLabel, dominantCount := DominantDatasetLabel(labels)
	dominantRatio := 0.0
	if labeled > 0 && dominantLabel != "" && dominantLabel != "UNLABELED" {
		dominantRatio = float64(dominantCount) / float64(labeled)
	}
	outOfRange := normalization.BelowZeroValues + normalization.AboveOneValues
	quality := core.DatasetQualitySummary{
		ImportableCount:     importable,
		LabeledCount:        labeled,
		UnlabeledCount:      unlabeled,
		DuplicateCount:      duplicates,
		DominantLabel:       dominantLabel,
		DominantLabelRatio:  dominantRatio,
		ClassImbalance:      labeled >= 5 && dominantRatio >= 0.80,
		FeatureOutOfRange:   outOfRange,
		NormalizationStatus: normalization.Mode,
	}
	if total == 0 {
		quality.Warnings = append(quality.Warnings, "no_samples")
	}
	if labeled == 0 {
		quality.Warnings = append(quality.Warnings, "no_labeled_samples")
	}
	if quality.ClassImbalance {
		quality.Warnings = append(quality.Warnings, fmt.Sprintf("class_imbalance:%s:%.2f", dominantLabel, dominantRatio))
	}
	if duplicates > 0 {
		quality.Warnings = append(quality.Warnings, fmt.Sprintf("duplicate_commands:%d", duplicates))
	}
	if outOfRange > 0 {
		quality.Warnings = append(quality.Warnings, fmt.Sprintf("feature_out_of_range:%d", outOfRange))
	}
	return quality
}

func DominantDatasetLabel(labels map[string]int) (string, int) {
	bestLabel := ""
	bestCount := 0
	for label, count := range labels {
		if label == "UNLABELED" {
			continue
		}
		if count > bestCount || (count == bestCount && label < bestLabel) {
			bestLabel = label
			bestCount = count
		}
	}
	return bestLabel, bestCount
}
