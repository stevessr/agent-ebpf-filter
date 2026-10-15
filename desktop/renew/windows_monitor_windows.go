//go:build windows

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Windows local monitoring is deliberately independent of the Linux eBPF
// backend. Only the default local origin falls back to Win32 snapshots; an
// explicitly reachable backend still uses the normal authenticated API.
func startLocalWindowsMonitoring(ctx context.Context, a *renewApp) bool {
	if a.backend != defaultBackendURL || backendAPIAvailable(ctx, a.backend) {
		return false
	}
	a.update(func() {
		a.localWindowsMonitoring = true
		a.starting = false
		a.connected = true
		a.healthReady = false
		a.configErr = "Windows 本机模式仅观测进程与 TCP 连接；eBPF、文件/网络阻断、策略编辑和完整事件回溯不可用。"
	})
	go a.runWindowsMonitoring(ctx)
	return true
}

func windowsAgentForPID(pid int, processes map[int]systemProcess) (int, string) {
	seen := make(map[int]bool, 8)
	for depth := 0; depth < 32 && pid > 0 && !seen[pid]; depth++ {
		seen[pid] = true
		proc, ok := processes[pid]
		if !ok {
			break
		}
		name := strings.ToLower(strings.TrimSuffix(proc.Name, ".exe"))
		for _, spec := range codingHarnessCatalog {
			if name == strings.ToLower(strings.TrimSuffix(spec.Command, ".exe")) {
				return pid, spec.Label
			}
		}
		// Windows process snapshots only include executable basenames.
		// Do not classify generic node.exe, cmd.exe or powershell.exe as
		// agent roots without actual process-lineage evidence.
		pid = proc.PPID
	}
	return 0, ""
}

func windowsConnectionKey(conn windowsTCPConnection) string {
	return fmt.Sprintf("%d|%s|%s|%d", conn.PID, conn.Local, conn.Remote, conn.State)
}

// runWindowsMonitoring polls native OS snapshots. It does not claim to see
// transient TCP connections between polls, file events, syscall arguments,
// HTTPS hostnames, process command lines, or kernel policy decisions.
func (a *renewApp) runWindowsMonitoring(ctx context.Context) {
	a.startEventUIBatcher(ctx)
	go a.runWindowsEBPFProbeLoop(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var previousProcesses map[int]systemProcess
	previousConnections := make(map[string]struct{})
	sequence := uint64(0)
	for ctx.Err() == nil {
		now := time.Now()
		processes, procErr := windowsProcessSnapshot()
		connections, tcpErr := windowsTCPSnapshot()
		if procErr != nil {
			a.update(func() {
				a.connected = true
				a.healthReady = true
				a.health.CaptureHealthy = false
				a.systemConnected = false
				a.eventStreamConnected = false
				a.systemErr = procErr.Error()
			})
		} else {
			snapshot := systemSnapshot{FetchedAt: now}
			snapshot.Processes = make([]systemProcess, 0, len(processes))
			for _, proc := range processes {
				snapshot.Processes = append(snapshot.Processes, proc)
			}
			sort.Slice(snapshot.Processes, func(i, j int) bool {
				return snapshot.Processes[i].PID < snapshot.Processes[j].PID
			})
			memTotal, memUsed, memPct, memErr := windowsMemorySnapshot()
			if memErr == nil {
				snapshot.MemTotal, snapshot.MemUsed, snapshot.MemPercent = memTotal, memUsed, memPct
			}

			nextConnections := make(map[string]struct{}, len(connections))
			events := make([]eventSummary, 0, 32)
			if previousProcesses != nil {
				for pid, proc := range processes {
					if _, wasRunning := previousProcesses[pid]; wasRunning {
						continue
					}
					root, label := windowsAgentForPID(pid, processes)
					if root != pid || len(events) >= 200 {
						continue
					}
					sequence++
					events = append(events, eventSummary{
						EventID: fmt.Sprintf("win-process-%d-%d", now.UnixNano(), sequence),
						PID: pid, PPID: proc.PPID, RootAgentPID: root,
						Type: "WINDOWS_PROCESS_OBSERVED", Comm: proc.Name, Tag: label,
						Target: proc.Name, Decision: "OBSERVED", HasAgentContext: true,
						ReceivedAtMS: now.UnixMilli(),
					})
				}
			}
			for _, conn := range connections {
				key := windowsConnectionKey(conn)
				nextConnections[key] = struct{}{}
				if _, known := previousConnections[key]; known || len(events) >= 200 {
					continue
				}
				root, label := windowsAgentForPID(conn.PID, processes)
				if root == 0 {
					continue
				}
				sequence++
				proc := processes[conn.PID]
				events = append(events, eventSummary{
					EventID: fmt.Sprintf("win-tcp-%d-%d", now.UnixNano(), sequence),
					PID: conn.PID, PPID: proc.PPID, RootAgentPID: root,
					Type: "WINDOWS_TCP_OBSERVED", Comm: proc.Name, Tag: label,
					Target: conn.Remote, NetEndpoint: conn.Remote, Network: true,
					Decision: "OBSERVED", HasAgentContext: true,
					ReceivedAtMS: now.UnixMilli(),
				})
			}
			previousProcesses = processes
			if tcpErr == nil {
				previousConnections = nextConnections
			}
			issue := ""
			if tcpErr != nil {
				issue = tcpErr.Error()
			}
			if memErr != nil {
				if issue != "" {
					issue += "; "
				}
				issue += memErr.Error()
			}
			a.update(func() {
				a.system = snapshot
				a.connected = true
				a.healthReady = true
				a.health.CaptureHealthy = tcpErr == nil
				a.systemConnected = true
				a.eventStreamConnected = tcpErr == nil
				a.systemErr = issue
				a.eventStreamErr = issue
				a.lastSync = now
				a.historyInitialized = true
			})
			if len(events) > 0 {
				a.queueEventSummaries(events)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *renewApp) runWindowsEBPFProbeLoop(ctx context.Context) {
    ticker := time.NewTicker(20 * time.Second)
    defer ticker.Stop()
    for ctx.Err() == nil {
        probe := probeWindowsEBPF(ctx)
        if ctx.Err() != nil { return }
        a.update(func() { a.windowsEBPFSummary = " · " + probe.Summary() })
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
        }
    }
}
