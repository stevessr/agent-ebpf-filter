package ml

import (
	"sort"
	"strings"

	"agent-ebpf-filter/core"
)

// Research count helpers shared by the app training-readiness view and the
// research processing pipeline.

func IncrementResearchCount(counts map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	counts[key]++
}

func TopResearchCounts(counts map[string]int, limit int) []core.ResearchCount {
	items := make([]core.ResearchCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, core.ResearchCount{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
