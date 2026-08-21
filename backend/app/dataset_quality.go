package app

import (
	"strings"

	"agent-ebpf-filter/app/ml"
)

var (
	normalizedDatasetCategoryKey = ml.NormalizedDatasetCategoryKey
	buildDatasetQualitySummary   = ml.BuildDatasetQualitySummary
	dominantDatasetLabel         = ml.DominantDatasetLabel
)

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

func datasetQualityFromRows(rows []remoteDatasetRow, normalization ml.FeatureNormalizationReport) DatasetQualitySummary {
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
