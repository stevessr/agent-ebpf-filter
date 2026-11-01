package main

import (
	"sort"
	"strings"
)

// Rebuilding an aggregate's extrema (last event, maximum risk) and distinct
// PID/command sets requires the surviving events for that group. Rather than
// retaining a second indexed copy of the event window, re-evaluate only
// affected groups in one bounded pass over the current window.
//
// High-cardinality views benefit most: with a handful of changed targets/PIDs,
// the untouched aggregates survive unchanged. For small or broadly changed
// tables we deliberately leave the cache stale for the existing full rebuild.
const (
	incrementalAggregateMinGroups = 32
	incrementalAggregateMaxTouched = 24
)

func affectedNetworkTargets(removed, added []eventSummary) map[string]*networkAggregate {
	touched := make(map[string]*networkAggregate, len(removed)+len(added))
	for _, events := range [][]eventSummary{removed, added} {
		for _, e := range events {
			target := strings.TrimSpace(e.Target)
			if target == "" || !isNetworkEvent(e) {
				continue
			}
			if _, ok := touched[target]; !ok {
				touched[target] = &networkAggregate{
					Target: target,
					PIDs:   make(map[int]struct{}),
					Comms:  make(map[string]struct{}),
				}
			}
		}
	}
	return touched
}

func affectedProcessPIDs(removed, added []eventSummary) map[int]*processAggregate {
	touched := make(map[int]*processAggregate, len(removed)+len(added))
	for _, events := range [][]eventSummary{removed, added} {
		for _, e := range events {
			if e.PID > 0 {
				if _, ok := touched[e.PID]; !ok {
					touched[e.PID] = &processAggregate{PID: e.PID}
				}
			}
		}
	}
	return touched
}

func aggregateDeltaWorthwhile(existing, touched int) bool {
	// A small aggregation is already cheap to rebuild, while a large
	// change set can make local repair more expensive than full recompute.
	return existing >= incrementalAggregateMinGroups &&
		touched <= incrementalAggregateMaxTouched &&
		touched*4 <= existing
}

func incrementalNetworkAggregateRows(previous []networkAggregate, window, removed, added []eventSummary) ([]networkAggregate, bool) {
	touched := affectedNetworkTargets(removed, added)
	if len(touched) == 0 {
		return previous, true
	}
	if !aggregateDeltaWorthwhile(len(previous), len(touched)) {
		return nil, false
	}
	for _, e := range window {
		target := strings.TrimSpace(e.Target)
		row, affected := touched[target]
		if !affected || !isNetworkEvent(e) {
			continue
		}
		row.Events++
		if e.NetBytes > 0 {
			row.Bytes += e.NetBytes
		}
		if e.ReceivedAtMS > row.LastMS {
			row.LastMS = e.ReceivedAtMS
		}
		if e.RiskScore > row.Risk {
			row.Risk = e.RiskScore
		}
		if e.PID > 0 {
			row.PIDs[e.PID] = struct{}{}
		}
		if strings.TrimSpace(e.Comm) != "" {
			row.Comms[e.Comm] = struct{}{}
		}
	}
	rows := make([]networkAggregate, 0, len(previous)+len(touched))
	for _, row := range previous {
		if _, affected := touched[row.Target]; !affected {
			rows = append(rows, row)
		}
	}
	for _, row := range touched {
		if row.Events != 0 {
			rows = append(rows, *row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Events != rows[j].Events {
			return rows[i].Events > rows[j].Events
		}
		return rows[i].LastMS > rows[j].LastMS
	})
	return rows, true
}

func incrementalProcessAggregateRows(previous []processAggregate, window, removed, added []eventSummary) ([]processAggregate, bool) {
	touched := affectedProcessPIDs(removed, added)
	if len(touched) == 0 {
		return previous, true
	}
	if !aggregateDeltaWorthwhile(len(previous), len(touched)) {
		return nil, false
	}
	// Traverse newest-first, just like aggregateProcesses. PPID/Comm fields
	// intentionally take the last nonempty value encountered in that order;
	// sorting events by PID or choosing the newest name would change behavior.
	for _, e := range window {
		row, affected := touched[e.PID]
		if !affected {
			continue
		}
		row.Events++
		if e.ReceivedAtMS > row.LastMS {
			row.LastMS = e.ReceivedAtMS
		}
		if e.RiskScore > row.Risk {
			row.Risk = e.RiskScore
		}
		if e.PPID > 0 {
			row.PPID = e.PPID
		}
		if strings.TrimSpace(e.Comm) != "" {
			row.Comm = e.Comm
		}
	}
	rows := make([]processAggregate, 0, len(previous)+len(touched))
	for _, row := range previous {
		if _, affected := touched[row.PID]; !affected {
			rows = append(rows, row)
		}
	}
	for _, row := range touched {
		if row.Events != 0 {
			rows = append(rows, *row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastMS != rows[j].LastMS {
			return rows[i].LastMS > rows[j].LastMS
		}
		return rows[i].Events > rows[j].Events
	})
	return rows, true
}
