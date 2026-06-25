package network

import (
	netcore "agent-ebpf-filter/internal/network"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section tcp_network.go ----

type TCPState = netcore.TCPState

const (
	TCPStateUnknown     = netcore.TCPStateUnknown
	TCPStateEstablished = netcore.TCPStateEstablished
	TCPStateSynSent     = netcore.TCPStateSynSent
	TCPStateSynRecv     = netcore.TCPStateSynRecv
	TCPStateFinWait1    = netcore.TCPStateFinWait1
	TCPStateFinWait2    = netcore.TCPStateFinWait2
	TCPStateTimeWait    = netcore.TCPStateTimeWait
	TCPStateClose       = netcore.TCPStateClose
	TCPStateCloseWait   = netcore.TCPStateCloseWait
	TCPStateLastAck     = netcore.TCPStateLastAck
	TCPStateListen      = netcore.TCPStateListen
	TCPStateClosing     = netcore.TCPStateClosing
	TCPStateClosed      = netcore.TCPStateClosed
)

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

var tcpTracker = newTCPStateTracker()

func startTCPStateTrackerGC() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			tcpTracker.EvictTerminalOlderThan(1 * time.Minute)
		}
	}()
}

func detectAppProtocol(port uint32, domain string) string {
	return netcore.DetectAppProtocol(port, domain)
}
