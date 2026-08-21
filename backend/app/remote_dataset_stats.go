package app

import (
	"agent-ebpf-filter/app/ml"
	"strings"
)

func applyRemoteDatasetResponseStats(resp *remoteDatasetResponse, labelMode string, cleanSensitive bool) {
	if resp == nil {
		return
	}
	byLabel := map[string]int{}
	byCategory := map[string]int{}
	bySource := map[string]int{}
	samples := make([]ml.TrainingSample, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		incrementResearchCount(byLabel, normalizedDatasetLabelKey(row.Label))
		incrementResearchCount(byCategory, normalizedDatasetCategoryKey(row.Category))
		source := strings.TrimSpace(row.Source)
		if source == "" {
			source = strings.TrimSpace(resp.Source)
		}
		if source == "" {
			source = "inline"
		}
		incrementResearchCount(bySource, source)
		if strings.TrimSpace(row.Comm) == "" {
			continue
		}
		samples = append(samples, buildRemoteDatasetSample(row, labelMode, cleanSensitive))
	}
	resp.ByLabel = topResearchCounts(byLabel, 0)
	resp.ByCategory = topResearchCounts(byCategory, 0)
	resp.BySource = topResearchCounts(bySource, 10)
	resp.Normalization = ml.SummarizeFeatureNormalization(samples)
	resp.Quality = datasetQualityFromRows(resp.Rows, resp.Normalization)
}
