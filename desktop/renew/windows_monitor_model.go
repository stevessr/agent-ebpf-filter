package main

import (
    "fmt"
    "sort"
    "time"
)

// Windows state sampling is not a syscall, ETW or connect()/bind() event
// stream. Keep evidence types explicit and never invent a security decision.
type windowsProcessSample struct {
    PID int
    PPID int
    Name string
    ImagePath string // executable image path, not a command line
    Start uint64 // FILETIME; zero when protected
    CPU uint64   // user + kernel, cumulative 100ns ticks
    WorkingSet uint64
    HasCPU bool
    HasMemory bool
}

type windowsTCPSample struct {
    PID int
    Local string
    Remote string
}

// UDP owner tables show local *bindings*, not peers or UDP packet flows.
type windowsUDPSample struct {
    PID int
    Local string
}

type windowsObservation struct {
    Processes map[int]windowsProcessSample
    Connections map[string]windowsTCPSample
    UDPBindings map[string]windowsUDPSample
    System systemSnapshot
    CPUIdle uint64
    CPUTotal uint64
}

const windowsMaxEventsPerPoll = 256

func windowsObservationEvents(prev, next windowsObservation, hasBaseline bool, at time.Time) ([]eventSummary, int) {
    if !hasBaseline { return nil, 0 }
    events := make([]eventSummary, 0, 24)
    dropped := 0
    add := func(e eventSummary) {
        // Bound allocation as well as return count during extreme churn.
        if len(events) >= windowsMaxEventsPerPoll {
            dropped++
            return
        }
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
        before, exists := prev.Processes[pid]
        if !exists || windowsProcessIdentityChanged(before, p) {
            add(eventSummary{PID:p.PID, PPID:p.PPID, Comm:p.Name,
                Type:"process_newly_observed", Target:windowsProcessTarget(p)})
        }
    }
    ids = ids[:0]
    for pid := range prev.Processes { ids = append(ids, pid) }
    sort.Ints(ids)
    for _, pid := range ids {
        p := prev.Processes[pid]
        after, exists := next.Processes[pid]
        if !exists || windowsProcessIdentityChanged(p, after) {
            add(eventSummary{PID:p.PID, PPID:p.PPID, Comm:p.Name,
                Type:"process_disappeared", Target:windowsProcessTarget(p)})
        }
    }
    tcpKeys := make([]string, 0, len(next.Connections))
    for key := range next.Connections { tcpKeys = append(tcpKeys, key) }
    sort.Strings(tcpKeys)
    for _, key := range tcpKeys {
        conn := next.Connections[key]
        previousConn, existed := prev.Connections[key]
        currentProc, currentOK := next.Processes[conn.PID]
        previousProc, previousOK := prev.Processes[conn.PID]
        // A recycled PID with the same 4-tuple belongs to a different
        // process, even when its network-table key looks unchanged.
        if existed && previousConn == conn &&
            (!currentOK || !previousOK || !windowsProcessIdentityChanged(previousProc, currentProc)) {
            continue
        }
        add(eventSummary{PID:conn.PID, PPID:currentProc.PPID, Comm:currentProc.Name,
            Type:"network_tcp_observed", Target:conn.Remote, Network:true})
    }
    udpKeys := make([]string, 0, len(next.UDPBindings))
    for key := range next.UDPBindings { udpKeys = append(udpKeys, key) }
    sort.Strings(udpKeys)
    for _, key := range udpKeys {
        binding := next.UDPBindings[key]
        previousBinding, existed := prev.UDPBindings[key]
        currentProc, currentOK := next.Processes[binding.PID]
        previousProc, previousOK := prev.Processes[binding.PID]
        if existed && previousBinding == binding &&
            (!currentOK || !previousOK || !windowsProcessIdentityChanged(previousProc, currentProc)) {
            continue
        }
        add(eventSummary{PID:binding.PID, PPID:currentProc.PPID, Comm:currentProc.Name,
            Type:"network_udp_binding_observed", Target:binding.Local, Network:true})
    }
    return events, dropped
}

func windowsProcessIdentityChanged(before, after windowsProcessSample) bool {
    // Start timestamp is the most reliable PID-reuse discriminator.
    if before.Start != 0 && after.Start != 0 {
        return before.Start != after.Start
    }
    return before.Name != after.Name || (before.ImagePath != "" && after.ImagePath != "" && before.ImagePath != after.ImagePath) || before.PPID != after.PPID
}

func windowsProcessTarget(process windowsProcessSample) string {
    if process.ImagePath != "" { return process.ImagePath }
    return process.Name
}

func windowsCPUPercent(prev, next windowsObservation) float64 {
    if next.CPUTotal <= prev.CPUTotal || next.CPUIdle < prev.CPUIdle { return 0 }
    total := next.CPUTotal - prev.CPUTotal
    idle := next.CPUIdle - prev.CPUIdle
    if idle > total { return 0 }
    return 100 * float64(total-idle) / float64(total)
}

func windowsProcessCPUPercent(prev, next windowsProcessSample, elapsed time.Duration, cpuCount int) float64 {
    if cpuCount <= 0 || elapsed <= 0 || next.CPU < prev.CPU { return 0 }
    if windowsProcessIdentityChanged(prev, next) { return 0 }
    value := float64(next.CPU-prev.CPU)/(float64(elapsed.Nanoseconds())/100)*100/float64(cpuCount)
    if value < 0 { return 0 }
    if value > 100 { return 100 }
    return value
}
