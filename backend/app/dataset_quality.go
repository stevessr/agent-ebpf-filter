package app

import (
	"fmt"
	"strings"
)

type DatasetQualitySummary struct {
	ImportableCount     int      `json:"importableCount"`
	LabeledCount        int      `json:"labeledCount"`
	UnlabeledCount      int      `json:"unlabeledCount"`
	DuplicateCount      int      `json:"duplicateCount"`
	DominantLabel       string   `json:"dominantLabel,omitempty"`
	DominantLabelRatio  float64  `json:"dominantLabelRatio,omitempty"`
	ClassImbalance      bool     `json:"classImbalance"`
	FeatureOutOfRange   int      `json:"featureOutOfRange"`
	NormalizationStatus string   `json:"normalizationStatus"`
	Warnings            []string `json:"warnings,omitempty"`
}

type remoteDatasetParseWarning struct {
	Source string `json:"source,omitempty"`
	Row    int    `json:"row,omitempty"`
	Reason string `json:"reason"`
	Count  int    `json:"count,omitempty"`
}

func normalizedDatasetLabelKey(raw string) string {
	if label := normalizeActionLabel(raw); label != "" {
		return label
	}
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "-", "UNLABELED", "UNKNOWN", "NONE":
		return "UNLABELED"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func normalizedDatasetCategoryKey(raw string) string {
	category := strings.TrimSpace(raw)
	if category == "" || category == "-" {
		return "UNCATEGORIZED"
	}
	return category
}

func datasetQualityFromRows(rows []remoteDatasetRow, normalization FeatureNormalizationReport) DatasetQualitySummary {
	labels := map[string]int{}
	importable := 0
	labeled := 0
	unlabeled := 0
	duplicates := 0
	for _, row := range rows {
		label := normalizedDatasetLabelKey(row.Label)
		labels[label]++
		if label == "UNLABELED" {
			unlabeled++
		} else {
			labeled++
		}
		if row.Duplicate {
			duplicates++
			continue
		}
		if strings.TrimSpace(row.Comm) != "" {
			importable++
		}
	}
	return buildDatasetQualitySummary(len(rows), importable, labeled, unlabeled, duplicates, labels, normalization)
}

func buildDatasetQualitySummary(total, importable, labeled, unlabeled, duplicates int, labels map[string]int, normalization FeatureNormalizationReport) DatasetQualitySummary {
	dominantLabel, dominantCount := dominantDatasetLabel(labels)
	dominantRatio := 0.0
	if labeled > 0 && dominantLabel != "" && dominantLabel != "UNLABELED" {
		dominantRatio = float64(dominantCount) / float64(labeled)
	}
	outOfRange := normalization.BelowZeroValues + normalization.AboveOneValues
	quality := DatasetQualitySummary{
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

func dominantDatasetLabel(labels map[string]int) (string, int) {
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
