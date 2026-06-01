package main

import (
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func allocMem() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func parseBoolEnv(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseNameFilter(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func modelFilterMatches(selected map[ModelType]bool, modelType ModelType) bool {
	if len(selected) == 0 {
		return true
	}
	return selected[modelType] || selected[baseModelType(modelType)]
}

func datasetProfilesForMode(labeled []TrainingSample, mode string, selected map[string]bool) []sweepDataset {
	candidates := buildSweepDatasetCandidates(labeled)
	if len(candidates) == 0 {
		return nil
	}
	if len(selected) > 0 {
		out := make([]sweepDataset, 0, len(selected))
		for _, ds := range candidates {
			if selected[strings.ToLower(ds.Name)] {
				out = append(out, ds)
			}
		}
		return out
	}
	if mode != "comprehensive" {
		return candidates[:1]
	}
	out := []sweepDataset{candidates[0]}
	for _, ds := range candidates[1:] {
		out = append(out, ds)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func buildSweepDatasetCandidates(labeled []TrainingSample) []sweepDataset {
	if len(labeled) == 0 {
		return nil
	}
	out := []sweepDataset{{
		Name:        "all",
		Description: "all persisted labeled samples",
		Samples:     append([]TrainingSample(nil), labeled...),
	}}
	if balanced := balancedLabelDataset(labeled); len(balanced) >= 10 && len(balanced) < len(labeled) {
		out = append(out, sweepDataset{
			Name:        "label-balanced",
			Description: "deterministic class-balanced subset capped by the smallest present label",
			Samples:     balanced,
		})
	}
	if allowBlock := filterSamplesByLabel(labeled, map[int32]bool{0: true, 1: true}); len(allowBlock) >= 10 && len(allowBlock) < len(labeled) {
		out = append(out, sweepDataset{
			Name:        "allow-block",
			Description: "binary ALLOW/BLOCK subset for false-block and miss-block sensitivity",
			Samples:     allowBlock,
		})
	}
	if even := deterministicIndexSubset(labeled, 0); len(even) >= 10 && len(out) < 3 {
		out = append(out, sweepDataset{
			Name:        "even-index",
			Description: "deterministic even-index subset used when label-derived subsets are unavailable",
			Samples:     even,
		})
	}
	if odd := deterministicIndexSubset(labeled, 1); len(odd) >= 10 && len(out) < 3 {
		out = append(out, sweepDataset{
			Name:        "odd-index",
			Description: "deterministic odd-index subset used when label-derived subsets are unavailable",
			Samples:     odd,
		})
	}
	return out
}

func balancedLabelDataset(samples []TrainingSample) []TrainingSample {
	byLabel := make(map[int32][]TrainingSample)
	for _, sample := range samples {
		byLabel[sample.Label] = append(byLabel[sample.Label], sample)
	}
	if len(byLabel) < 2 {
		return nil
	}
	minCount := int(^uint(0) >> 1)
	labels := make([]int, 0, len(byLabel))
	for label, group := range byLabel {
		if len(group) == 0 {
			continue
		}
		if len(group) < minCount {
			minCount = len(group)
		}
		labels = append(labels, int(label))
	}
	if minCount <= 0 || len(labels) < 2 {
		return nil
	}
	sort.Ints(labels)
	out := make([]TrainingSample, 0, minCount*len(labels))
	for _, label := range labels {
		group := byLabel[int32(label)]
		if len(group) > minCount {
			group = group[:minCount]
		}
		out = append(out, group...)
	}
	return out
}

func filterSamplesByLabel(samples []TrainingSample, labels map[int32]bool) []TrainingSample {
	out := make([]TrainingSample, 0, len(samples))
	for _, sample := range samples {
		if labels[sample.Label] {
			out = append(out, sample)
		}
	}
	return out
}

func deterministicIndexSubset(samples []TrainingSample, parity int) []TrainingSample {
	out := make([]TrainingSample, 0, (len(samples)+1)/2)
	for i, sample := range samples {
		if i%2 == parity {
			out = append(out, sample)
		}
	}
	return out
}

func trainingStoreFromSamples(samples []TrainingSample) *TrainingDataStore {
	maxSamples := len(samples)
	if maxSamples < 1 {
		maxSamples = 1
	}
	store := &TrainingDataStore{
		samples:    make([]TrainingSample, maxSamples),
		maxSamples: maxSamples,
	}
	for _, sample := range samples {
		store.Add(sample)
	}
	store.dirtyCount = 0
	return store
}
