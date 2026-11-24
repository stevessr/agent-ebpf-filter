package main

import (
	"sort"
	"strings"
)

func mergeEventSummaries(existing, incoming []eventSummary, limit int) []eventSummary {
	return mergeEventSummariesInto(nil, existing, incoming, limit)
}

// eventMergeWorkspace owns bounded, reusable scratch buffers. The map and
// sort slice are never returned; only the independent output event window is.
type eventMergeWorkspace struct {
	replacements  map[string]eventSummary
	fresh         []eventSummary
	riskDelta     [3]int
	filterAdded   []eventSummary
	filterRemoved map[string]struct{}
	// Removed original records are captured before the merge scratch is recycled.
	aggregateRemoved []eventSummary
}

func mergeEventSummariesInto(dst, existing, incoming []eventSummary, limit int) []eventSummary {
	return mergeEventSummariesIntoWorkspace(dst, existing, incoming, limit, nil, false, false)
}

// mergeEventSummariesIntoWorkspace reuses the per-window deduplication map and
// incoming sort buffer. When trackRisk is true, its riskDelta describes only
// added, replaced and evicted events, including timestamps changed on a
// duplicate EventID. The caller can apply this to an already-warm risk cache.
// The output remains a separate backing buffer from existing.
func mergeEventSummariesIntoWorkspace(dst, existing, incoming []eventSummary, limit int, workspace *eventMergeWorkspace, trackRisk, trackFilter bool, trackAggregates ...bool) []eventSummary {
	trackAggregate := len(trackAggregates) != 0 && trackAggregates[0]
	if limit <= 0 {
		limit = 1200
	}
	if workspace == nil {
		workspace = &eventMergeWorkspace{}
	}
	workspace.riskDelta = [3]int{}
	workspace.filterAdded = workspace.filterAdded[:0]
	workspace.aggregateRemoved = workspace.aggregateRemoved[:0]
	if trackFilter {
		if workspace.filterRemoved == nil {
			workspace.filterRemoved = make(map[string]struct{}, min(len(incoming)+16, limit))
		}
		clear(workspace.filterRemoved)
	}
	if len(incoming) == 0 {
		if len(existing) > limit {
			if trackRisk {
				for _, e := range existing[limit:] {
					workspace.riskDelta[eventRiskBucket(e)]--
				}
			}
			if trackFilter {
				for _, e := range existing[limit:] {
					workspace.filterRemoved[e.EventID] = struct{}{}
				}
			}
			if trackAggregate {
				workspace.aggregateRemoved = append(workspace.aggregateRemoved, existing[limit:]...)
			}
			return existing[:limit]
		}
		return existing
	}

	if workspace.replacements == nil {
		workspace.replacements = make(map[string]eventSummary, min(len(incoming), limit))
	}
	replacements := workspace.replacements
	// Clear at entry as well as exit; a workspace never owns the retained
	// event window, and stale IDs must not influence a subsequent merge.
	clear(replacements)
	for _, event := range incoming {
		id := strings.TrimSpace(event.EventID)
		if id == "" {
			continue
		}
		event.EventID = id
		// A flood may contain repeated revisions of the same EventID.
		// Build derived full-text search only after last-update-wins
		// deduplication; superseded revisions need no search corpus.
		replacements[id] = event
	}
	if len(replacements) == 0 {
		return existing
	}
	// Release string/target references as soon as a batch has been merged.
	defer clear(replacements)

	fresh := workspace.fresh[:0]
	if cap(fresh) < len(replacements) {
		fresh = make([]eventSummary, 0, len(replacements))
	}
	for _, event := range replacements {
		event.SearchText = buildEventSearchText(event)
		fresh = append(fresh, event)
	}
	workspace.fresh = fresh
	defer clear(fresh)
	sort.Slice(fresh, func(i, j int) bool { return eventSummaryBefore(fresh[i], fresh[j]) })

	capacity := len(existing) + len(fresh)
	if capacity > limit {
		capacity = limit
	}
	var out []eventSummary
	if cap(dst) >= capacity {
		out = dst[:0]
	} else {
		out = make([]eventSummary, 0, limit)
	}
	i, j := 0, 0
	addFresh := func(event eventSummary) {
		out = append(out, event)
		if trackRisk {
			workspace.riskDelta[eventRiskBucket(event)]++
		}
		if trackFilter || trackAggregate {
			workspace.filterAdded = append(workspace.filterAdded, event)
		}
	}
	for len(out) < limit && (i < len(fresh) || j < len(existing)) {
		for j < len(existing) {
			if _, replaced := replacements[strings.TrimSpace(existing[j].EventID)]; replaced {
				if trackRisk {
					workspace.riskDelta[eventRiskBucket(existing[j])]--
				}
				if trackFilter {
					workspace.filterRemoved[existing[j].EventID] = struct{}{}
				}
				if trackAggregate {
					workspace.aggregateRemoved = append(workspace.aggregateRemoved, existing[j])
				}
				j++
				continue
			}
			break
		}
		if i >= len(fresh) {
			if j >= len(existing) {
				break
			}
			event := existing[j]
			if event.SearchText == "" {
				event.SearchText = buildEventSearchText(event)
			}
			out = append(out, event)
			j++
			continue
		}
		if j >= len(existing) {
			addFresh(fresh[i])
			i++
			continue
		}

		existingEvent := existing[j]
		if existingEvent.SearchText == "" {
			existingEvent.SearchText = buildEventSearchText(existingEvent)
		}
		if eventSummaryBefore(fresh[i], existingEvent) {
			addFresh(fresh[i])
			i++
		} else {
			out = append(out, existingEvent)
			j++
		}
	}
	// The tail was evicted by the bounded window, even when an older
	// replacement moved outside the visible time range.
	if trackRisk || trackFilter || trackAggregate {
		for ; j < len(existing); j++ {
			if trackRisk {
				workspace.riskDelta[eventRiskBucket(existing[j])]--
			}
			if trackFilter {
				workspace.filterRemoved[existing[j].EventID] = struct{}{}
			}
			if trackAggregate {
				workspace.aggregateRemoved = append(workspace.aggregateRemoved, existing[j])
			}
		}
	}
	return out
}

// eventRiskBucket is derived from the same classification used by riskCounts
// and native risk pills. No risk-score or policy decision is modified.
func eventRiskBucket(e eventSummary) int {
	switch eventRisk(e) {
	case "高风险":
		return 2
	case "需关注":
		return 1
	default:
		return 0
	}
}

func eventSummaryBefore(a, b eventSummary) bool {
	if a.ReceivedAtMS != b.ReceivedAtMS {
		return a.ReceivedAtMS > b.ReceivedAtMS
	}
	return a.EventID > b.EventID
}

func matchesEventDecision(event eventSummary, filter string) bool {
	decision := strings.ToUpper(strings.TrimSpace(event.Decision))
	switch filter {
	case "已阻断":
		return decision == "BLOCK" || decision == "DENY"
	case "告警":
		return decision == "ALERT"
	case "已允许":
		return decision == "ALLOW"
	default:
		return true
	}
}

func uniqueEventTypes(events []eventSummary) []string {
	set := make(map[string]struct{})
	for _, event := range events {
		if strings.TrimSpace(event.Type) != "" {
			set[event.Type] = struct{}{}
		}
	}
	out := make([]string, 0, len(set)+1)
	out = append(out, "")
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out[1:])
	return out
}

func uniqueEventSessions(events []eventSummary) []string {
	set := make(map[string]struct{})
	for _, event := range events {
		set[eventSessionKey(event)] = struct{}{}
	}
	out := make([]string, 0, len(set)+1)
	out = append(out, "")
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out[1:])
	return out
}
