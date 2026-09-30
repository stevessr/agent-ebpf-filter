package main

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// TLSModel holds the bounded TLS capture history and its statistics.
type TLSModel struct {
	mu          sync.Mutex
	ring        *Ring[*TLSEvent]
	filter      Filter
	dirty       bool
	state       ConnState
	stateDetail string
	evicted     uint64
	total       uint64
	requests    uint64
	responses   uint64
	bytes       uint64
	byHost      map[string]uint64
	byComm      map[string]uint64
	byVendor    map[string]uint64
	lastEventAt time.Time
}

func NewTLSModel(history int) *TLSModel {
	return &TLSModel{
		ring:     NewRing[*TLSEvent](history),
		state:    StateConnecting,
		byHost:   make(map[string]uint64),
		byComm:   make(map[string]uint64),
		byVendor: make(map[string]uint64),
	}
}

// OnTLSEvents implements TLSHandler.
func (m *TLSModel) OnTLSEvents(events []*TLSEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ev := range events {
		if ev != nil {
			m.push(ev)
		}
	}
	m.dirty = true
}

// OnTLSHistory implements TLSHandler; it only seeds an empty table.
func (m *TLSModel) OnTLSHistory(events []*TLSEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ring.Len() > 0 {
		return
	}
	for _, ev := range events {
		if ev != nil {
			m.push(ev)
		}
	}
	m.dirty = true
}

func (m *TLSModel) push(ev *TLSEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	if m.ring.Len() == m.ring.Cap() {
		m.evicted++
	}
	m.ring.Push(ev)
	m.total++
	m.lastEventAt = ev.Timestamp
	switch ev.Type {
	case "http_request":
		m.requests++
	case "http_response":
		m.responses++
	}
	m.bytes += uint64(max(ev.BodySize, ev.CapturedLen))
	if ev.Host != "" {
		m.byHost[ev.Host]++
	}
	if ev.Comm != "" {
		m.byComm[ev.Comm]++
	}
	if ev.Vendor != "" {
		m.byVendor[ev.Vendor]++
	}
}

// OnTLSState implements TLSHandler.
func (m *TLSModel) OnTLSState(state ConnState, detail string) {
	m.mu.Lock()
	m.state = state
	m.stateDetail = detail
	m.dirty = true
	m.mu.Unlock()
}

func (m *TLSModel) SetFilter(f Filter) {
	m.mu.Lock()
	m.filter = f
	m.dirty = true
	m.mu.Unlock()
}

func (m *TLSModel) Clear() {
	m.mu.Lock()
	m.ring.Clear()
	m.evicted, m.total, m.requests, m.responses, m.bytes = 0, 0, 0, 0, 0
	clear(m.byHost)
	clear(m.byComm)
	clear(m.byVendor)
	m.dirty = true
	m.mu.Unlock()
}

func (m *TLSModel) ConsumeDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	dirty := m.dirty
	m.dirty = false
	return dirty
}

// TLSSummary is the sidebar data for the TLS view.
type TLSSummary struct {
	State       ConnState
	StateDetail string
	Total       uint64
	Requests    uint64
	Responses   uint64
	Bytes       uint64
	Retained    int
	Capacity    int
	Evicted     uint64
	Filter      Filter
	LastEventAt time.Time
	TopHosts    []countedKey
	TopComms    []countedKey
	TopVendors  []countedKey
}

func (m *TLSModel) Summary(topN int) TLSSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return TLSSummary{
		State:       m.state,
		StateDetail: m.stateDetail,
		Total:       m.total,
		Requests:    m.requests,
		Responses:   m.responses,
		Bytes:       m.bytes,
		Retained:    m.ring.Len(),
		Capacity:    m.ring.Cap(),
		Evicted:     m.evicted,
		Filter:      m.filter,
		LastEventAt: m.lastEventAt,
		TopHosts:    topCounts(m.byHost, topN),
		TopComms:    topCounts(m.byComm, topN),
		TopVendors:  topCounts(m.byVendor, topN),
	}
}

// Snapshot appends the filtered history, oldest first, to dst[:0].
func (m *TLSModel) Snapshot(dst []*TLSEvent) []*TLSEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst = dst[:0]
	m.ring.Each(func(_ int, ev *TLSEvent) bool {
		if m.filter.MatchTLS(ev) {
			dst = append(dst, ev)
		}
		return true
	})
	return dst
}

// MatchTLS evaluates the filter against a TLS event. Free text matches
// host, URL, comm, method, type, vendor and content type; keyed terms use
// host: url: comm: method: status: type: vendor: pid: tool: run: dir:.
func (f Filter) MatchTLS(ev *TLSEvent) bool {
	for _, term := range f.terms {
		if term.matchesTLS(ev) == term.negate {
			return false
		}
	}
	return true
}

func (t filterTerm) matchesTLS(ev *TLSEvent) bool {
	switch t.key {
	case "":
		return containsFold(ev.Host, t.value) ||
			containsFold(ev.URL, t.value) ||
			containsFold(ev.Comm, t.value) ||
			containsFold(ev.Method, t.value) ||
			containsFold(ev.Type, t.value) ||
			containsFold(ev.Vendor, t.value) ||
			containsFold(ev.ContentType, t.value)
	case "host", "net", "endpoint":
		return containsFold(ev.Host, t.value)
	case "url", "path":
		return containsFold(ev.URL, t.value)
	case "comm":
		return containsFold(ev.Comm, t.value)
	case "method":
		return containsFold(ev.Method, t.value)
	case "type":
		return containsFold(ev.Type, t.value)
	case "vendor":
		return containsFold(ev.Vendor, t.value)
	case "dir", "direction":
		return containsFold(ev.Direction, t.value)
	case "tool":
		return containsFold(ev.ToolName, t.value)
	case "run":
		return containsFold(ev.AgentRunID, t.value)
	case "status":
		if ev.StatusCode == 0 {
			return false
		}
		return strings.HasPrefix(strconv.Itoa(ev.StatusCode), strings.TrimSpace(t.value))
	case "pid":
		return t.isNumber && (uint64(ev.PID) == t.numeric || uint64(ev.TGID) == t.numeric)
	default:
		return false
	}
}
