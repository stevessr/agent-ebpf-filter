package main

import (
	"fmt"
	"sort"
	"time"
)

// Windows desktop sampling is deliberately independent of the Linux eBPF
// backend. A poll observes state, not a kernel trace: absent processes are
// "no longer observed", and established TCP rows do not prove connect().
type windowsProcessSample struct {
	PID int
	PPID int
	Name string
	Start uint64 // FILETIME, when accessible; zero for protected processes
	CPU uint64 // cumulative user + kernel time, 100 ns units
	WorkingSet uint64
}

type windowsTCPSample struct {
	PID int
	Local string
	Remote string
}

type windowsObservation struct {
	Processes map[int]windowsProcessSample
	Connections map[string]windowsTCPSample
	System systemSnapshot
	CPUIdle uint64
	CPUTotal uint64
}

const windowsMaxEventsPerPoll = 256

func windowsObservationEvents(prev, next windowsObservation, hasBaseline bool, at time.Time) ([]eventSummary, int) {
	if !hasBaseline {
		return nil, 0 // never mislabel the initial process/connection inventory as new activity
	}
	events := make([]eventSummary, 0, 24)
	add := func(e eventSummary) {
		e.ReceivedAtMS = at.UnixMilli()
		e.Decision = "OBSERVED"
		e.EventID = fmt.Sprintf("windows:%d:%d", at.UnixNano(), len(events))
		events = append(events, e)
	}
	ids := make([]int, 0, len(next.Processes))
	for pid := range next.Processes { ids = append(ids, pid) }
	sort.Ints(ids)
	for _, pid := range ids {
		p := next.Processes[pid]
		old, exists := prev.Processes[pid]
		if !exists || (old.Start != 0 && p.Start != 0 && old.Start != p.Start) {
			add(eventSummary{PID: p.PID, PPID: p.PPID, Comm: p.Name, Type: "process_newly_observed", Target: p.Name})
		}
	}
	ids = ids[:0]
	for pid := range prev.Processes { ids = append(ids, pid) }
	sort.Ints(ids)
	for _, pid := range ids {
		p := prev.Processes[pid]
		newer, exists := next.Processes[pid]
		if !exists || (p.Start != 0 && newer.Start != 0 && p.Start != newer.Start) {
			add(eventSummary{PID: p.PID, PPID: p.PPID, Comm: p.Name, Type: "process_disappeared", Target: p.Name})
		}
	}
	keys := make([]string, 0, len(next.Connections))
	for key := range next.Connections { keys = append(keys, key) }
	sort.Strings(keys)
	for _, key := range keys {
		conn := next.Connections[key]
		if _, exists := prev.Connections[key]; exists { continue }
		p := next.Processes[conn.PID]
		add(eventSummary{PID: conn.PID, PPID: p.PPID, Comm: p.Name,
			Type: "network_tcp_observed", Target: conn.Remote, Network: true})
	}
	if len(events) > windowsMaxEventsPerPoll {
		return events[:windowsMaxEventsPerPoll], len(events)-windowsMaxEventsPerPoll
	}
	return events, 0
}

func windowsCPUPercent(prev, next windowsObservation) float64 {
	if next.CPUTotal <= prev.CPUTotal || next.CPUIdle < prev.CPUIdle { return 0 }
	total := next.CPUTotal-prev.CPUTotal
	idle := next.CPUIdle-prev.CPUIdle
	if idle > total { return 0 }
	return 100 * float64(total-idle) / float64(total)
}

func windowsProcessCPUPercent(prev, next windowsProcessSample, elapsed time.Duration, cpuCount int) float64 {
	if cpuCount <= 0 || elapsed <= 0 || next.CPU < prev.CPU { return 0 }
	if prev.Start != 0 && next.Start != 0 && prev.Start != next.Start { return 0 }
	// FILETIME CPU time is in 100 ns increments and counts logical cores.
	value := float64(next.CPU-prev.CPU) / (float64(elapsed.Nanoseconds()) / 100) * 100 / float64(cpuCount)
	if value < 0 { return 0 }
	if value > 100 { return 100 }
	return value
}
