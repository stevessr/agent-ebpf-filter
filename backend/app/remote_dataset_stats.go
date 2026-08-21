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
		ml.IncrementResearchCount(byLabel, normalizedDatasetLabelKey(row.Label))
		ml.IncrementResearchCount(byCategory, normalizedDatasetCategoryKey(row.Category))
		source := strings.TrimSpace(row.Source)
		if source == "" {
			source = strings.TrimSpace(resp.Source)
		}
		if source == "" {
			source = "inline"
		}
		ml.IncrementResearchCount(bySource, source)
		if strings.TrimSpace(row.Comm) == "" {
			continue
		}
		samples = append(samples, buildRemoteDatasetSample(row, labelMode, cleanSensitive))
	}
	resp.ByLabel = ml.TopResearchCounts(byLabel, 0)
	resp.ByCategory = ml.TopResearchCounts(byCategory, 0)
	resp.BySource = ml.TopResearchCounts(bySource, 10)
	resp.Normalization = ml.SummarizeFeatureNormalization(samples)
	resp.Quality = datasetQualityFromRows(resp.Rows, resp.Normalization)
}
