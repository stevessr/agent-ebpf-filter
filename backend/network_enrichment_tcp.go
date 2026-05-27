package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ── TCP State Machine (RFC 793, from rustnet) ────────────────────────

type TCPState uint8

const (
	TCPStateUnknown     TCPState = 0
	TCPStateEstablished TCPState = 1
	TCPStateSynSent     TCPState = 2
	TCPStateSynRecv     TCPState = 3
	TCPStateFinWait1    TCPState = 4
	TCPStateFinWait2    TCPState = 5
	TCPStateTimeWait    TCPState = 6
	TCPStateClose       TCPState = 7
	TCPStateCloseWait   TCPState = 8
	TCPStateLastAck     TCPState = 9
	TCPStateListen      TCPState = 10
	TCPStateClosing     TCPState = 11
	TCPStateClosed      TCPState = 12
)

var tcpStateDisplayNames = map[TCPState]string{
	TCPStateUnknown:     "UNKNOWN",
	TCPStateEstablished: "ESTABLISHED",
	TCPStateSynSent:     "SYN_SENT",
	TCPStateSynRecv:     "SYN_RECV",
	TCPStateFinWait1:    "FIN_WAIT1",
	TCPStateFinWait2:    "FIN_WAIT2",
	TCPStateTimeWait:    "TIME_WAIT",
	TCPStateClose:       "CLOSE",
	TCPStateCloseWait:   "CLOSE_WAIT",
	TCPStateLastAck:     "LAST_ACK",
	TCPStateListen:      "LISTEN",
	TCPStateClosing:     "CLOSING",
	TCPStateClosed:      "CLOSED",
}

func tcpStateFromLinux(state uint8) TCPState {
	switch state {
	case 1:
		return TCPStateEstablished
	case 2:
		return TCPStateSynSent
	case 3:
		return TCPStateSynRecv
	case 4:
		return TCPStateFinWait1
	case 5:
		return TCPStateFinWait2
	case 6:
		return TCPStateTimeWait
	case 7:
		return TCPStateClose
	case 8:
		return TCPStateCloseWait
	case 9:
		return TCPStateLastAck
	case 10:
		return TCPStateListen
	case 11:
		return TCPStateClosing
	default:
		return TCPStateUnknown
	}
}

func (s TCPState) String() string {
	if name, ok := tcpStateDisplayNames[s]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", s)
}

func (s TCPState) IsTerminal() bool {
	switch s {
	case TCPStateClose, TCPStateClosed, TCPStateTimeWait:
		return true
	default:
		return false
	}
}

func (s TCPState) IsEstablished() bool {
	return s == TCPStateEstablished
}

type tcpConnectionState struct {
	SrcIP      string
	DstIP      string
	SrcPort    uint32
	DstPort    uint32
	State      TCPState
	LastUpdate time.Time
	PID        uint32
	Comm       string
}

type tcpStateTracker struct {
	mu          sync.RWMutex
	connections map[string]*tcpConnectionState
}

func newTCPStateTracker() *tcpStateTracker {
	return &tcpStateTracker{
		connections: make(map[string]*tcpConnectionState),
	}
}

func (t *tcpStateTracker) connKey(srcIP, dstIP string, srcPort, dstPort uint32) string {
	return fmt.Sprintf("%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)
}

func (t *tcpStateTracker) RecordStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	newTCPState := tcpStateFromLinux(newState)

	t.mu.Lock()
	defer t.mu.Unlock()

	conn, ok := t.connections[key]
	if !ok {
		conn = &tcpConnectionState{
			SrcIP:   srcIP,
			DstIP:   dstIP,
			SrcPort: srcPort,
			DstPort: dstPort,
			PID:     pid,
			Comm:    comm,
		}
		t.connections[key] = conn
	}
	conn.State = newTCPState
	conn.LastUpdate = time.Now().UTC()
	if pid > 0 {
		conn.PID = pid
	}
	if comm != "" {
		conn.Comm = comm
	}
}

func (t *tcpStateTracker) RecordConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connections[key] = &tcpConnectionState{
		SrcIP:      srcIP,
		DstIP:      dstIP,
		SrcPort:    srcPort,
		DstPort:    dstPort,
		State:      TCPStateSynSent,
		LastUpdate: time.Now().UTC(),
		PID:        pid,
		Comm:       comm,
	}
}

func (t *tcpStateTracker) RecordClose(srcIP, dstIP string, srcPort, dstPort uint32) {
	if t == nil {
		return
	}
	key := t.connKey(srcIP, dstIP, srcPort, dstPort)
	t.mu.Lock()
	defer t.mu.Unlock()
	if conn, ok := t.connections[key]; ok {
		conn.State = TCPStateClosed
		conn.LastUpdate = time.Now().UTC()
	}
}

func (t *tcpStateTracker) Snapshot() []tcpConnectionState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	conns := make([]tcpConnectionState, 0, len(t.connections))
	for _, conn := range t.connections {
		conns = append(conns, *conn)
	}
	return conns
}

func (t *tcpStateTracker) EvictTerminalOlderThan(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge)
	for key, conn := range t.connections {
		if conn.State.IsTerminal() && conn.LastUpdate.Before(cutoff) {
			delete(t.connections, key)
		}
	}
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

// ── Application Protocol Hints ────────────────────────────────────────

func detectAppProtocol(port uint32, domain string) string {
	p := uint16(port)
	switch p {
	case 80:
		return "HTTP"
	case 443:
		if strings.Contains(strings.ToLower(domain), "quic") {
			return "QUIC"
		}
		return "HTTPS/TLS"
	case 22:
		return "SSH"
	case 53:
		return "DNS"
	case 123:
		return "NTP"
	case 161, 162:
		return "SNMP"
	case 1883:
		return "MQTT"
	case 3306:
		return "MySQL"
	case 5432:
		return "PostgreSQL"
	case 6379:
		return "Redis"
	case 27017:
		return "MongoDB"
	case 9092:
		return "Kafka"
	case 6443:
		return "Kubernetes"
	default:
		if service := lookupService(p); service != "" {
			return service
		}
		return "Unknown"
	}
}
