package network

import (
	netcore "agent-ebpf-filter/internal/network"
	"context"
	"time"
)

const tcpNonTerminalMaxAge = 30 * time.Minute

// ---- moved from backend/zz_merged_backend.go section tcp_network.go ----

type TCPState = netcore.TCPState

type tcpConnectionState = netcore.TCPConnectionState

type tcpStateTracker struct {
	inner *netcore.TCPStateTracker
}

func tcpStateFromLinux(state uint8) TCPState {
	return netcore.TCPStateFromLinux(state)
}

func newTCPStateTracker() *tcpStateTracker {
	return &tcpStateTracker{inner: netcore.NewTCPStateTracker()}
}

func (t *tcpStateTracker) connKey(srcIP, dstIP string, srcPort, dstPort uint32) string {
	return netcore.TCPConnKey(srcIP, dstIP, srcPort, dstPort)
}

func (t *tcpStateTracker) RecordStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
}

func (t *tcpStateTracker) RecordConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.RecordConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
}

func (t *tcpStateTracker) RecordClose(srcIP, dstIP string, srcPort, dstPort uint32) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.RecordClose(srcIP, dstIP, srcPort, dstPort)
}

func (t *tcpStateTracker) Snapshot() []tcpConnectionState {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.Snapshot()
}

func (t *tcpStateTracker) EvictTerminalOlderThan(maxAge time.Duration) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.EvictTerminalOlderThan(maxAge)
}

func (t *tcpStateTracker) EvictStale(terminalMaxAge, activeMaxAge time.Duration) {
	if t == nil || t.inner == nil {
		return
	}
	t.inner.EvictStale(terminalMaxAge, activeMaxAge)
}

func runTCPStateTrackerGC(ctx context.Context, tracker *tcpStateTracker, interval, maxAge time.Duration) {
	if ctx == nil || tracker == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tracker.EvictStale(maxAge, tcpNonTerminalMaxAge)
		}
	}
}

func detectAppProtocol(port uint32, domain string) string {
	return netcore.DetectAppProtocol(port, domain)
}
