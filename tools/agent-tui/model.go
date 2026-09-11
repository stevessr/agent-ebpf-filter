package main

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

// entry is one captured event plus its arrival time. The event pointer is
// shared with the decoded batch; nothing is copied into the ring.
type entry struct {
	at time.Time
	ev *pb.Event
}

// Model is the single source of truth behind the UI: a bounded event history,
// running statistics, the active filter, and connection status.
type Model struct {
	mu          sync.Mutex
	ring        *Ring[entry]
	stats       *Stats
	filter      Filter
	evicted     uint64
	dirty       bool
	state       ConnState
	stateDetail string
	lastEventAt time.Time
}

func NewModel(history int) *Model {
	return &Model{ring: NewRing[entry](history), stats: NewStats(), state: StateConnecting}
}

// OnEvents implements StreamHandler.
func (m *Model) OnEvents(events []*pb.Event, received time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ev := range events {
		if ev == nil {
			continue
		}
		m.push(entry{at: received, ev: ev})
	}
	m.lastEventAt = received
	m.dirty = true
}

// OnHistory implements StreamHandler. History only seeds an empty table so a
// reconnect never duplicates rows that already streamed in.
func (m *Model) OnHistory(records []*pb.CapturedEventRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ring.Len() > 0 {
		return
	}
	for _, record := range records {
		ev := record.GetEvent()
		if ev == nil {
			continue
		}
		at := time.UnixMilli(record.GetTimestamp())
		if record.GetTimestamp() == 0 {
			at = time.Now()
		}
		m.push(entry{at: at, ev: ev})
	}
	m.dirty = true
}

func (m *Model) push(e entry) {
	if m.ring.Len() == m.ring.Cap() {
		m.evicted++
	}
	m.ring.Push(e)
	m.stats.Observe(e.ev, e.at)
}

// OnState implements StreamHandler.
func (m *Model) OnState(state ConnState, detail string) {
	m.mu.Lock()
	m.state = state
	m.stateDetail = detail
	m.dirty = true
	m.mu.Unlock()
}

// SetFilter replaces the active filter.
func (m *Model) SetFilter(f Filter) {
	m.mu.Lock()
	m.filter = f
	m.dirty = true
	m.mu.Unlock()
}

// Clear forgets history and statistics but keeps the connection state.
func (m *Model) Clear() {
	m.mu.Lock()
	m.ring.Clear()
	m.stats.Reset()
	m.evicted = 0
	m.dirty = true
	m.mu.Unlock()
}

// ConsumeDirty reports whether anything changed since the previous call.
func (m *Model) ConsumeDirty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	dirty := m.dirty
	m.dirty = false
	return dirty
}

// Summary is the header/sidebar data captured under one lock acquisition.
type Summary struct {
	State       ConnState
	StateDetail string
	Total       uint64
	Risky       uint64
	Blocked     uint64
	Retained    int
	Capacity    int
	Evicted     uint64
	Rate1s      float64
	Rate10s     float64
	Sparkline   []uint32
	Filter      Filter
	LastEventAt time.Time
	TopTypes    []countedKey
	TopComms    []countedKey
	TopTags     []countedKey
}

func (m *Model) Summary(now time.Time, topN int) Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Summary{
		State:       m.state,
		StateDetail: m.stateDetail,
		Total:       m.stats.Total,
		Risky:       m.stats.Risky,
		Blocked:     m.stats.Blocked,
		Retained:    m.ring.Len(),
		Capacity:    m.ring.Cap(),
		Evicted:     m.evicted,
		Rate1s:      m.stats.Rate(now, 1),
		Rate10s:     m.stats.Rate(now, 10),
		Sparkline:   m.stats.Sparkline(now, 30),
		Filter:      m.filter,
		LastEventAt: m.lastEventAt,
		TopTypes:    m.stats.Top("type", topN),
		TopComms:    m.stats.Top("comm", topN),
		TopTags:     m.stats.Top("tag", topN),
	}
}

// Snapshot appends the filtered history, oldest first, to dst[:0] and returns
// it. Passing the previous result back avoids reallocating on every refresh.
func (m *Model) Snapshot(dst []entry) []entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst = dst[:0]
	m.ring.Each(func(_ int, e entry) bool {
		if m.filter.Match(e.ev) {
			dst = append(dst, e)
		}
		return true
	})
	return dst
}

// Filter is a parsed query: free-text terms match several fields, while
// key:value terms match one field. All terms must match.
type Filter struct {
	Raw   string
	terms []filterTerm
}

type filterTerm struct {
	key      string
	value    string // lower-cased
	minRisk  float64
	numeric  uint64
	negate   bool
	isNumber bool
}

// ParseFilter accepts terms like `node`, `type:openat`, `comm:python`,
// `pid:1234`, `tag:"AI Agent"`, `risk:>=40`, `decision:block`, `-type:read`.
func ParseFilter(raw string) Filter {
	f := Filter{Raw: strings.TrimSpace(raw)}
	for _, token := range splitFilterTokens(f.Raw) {
		term := filterTerm{}
		if strings.HasPrefix(token, "-") && len(token) > 1 {
			term.negate = true
			token = token[1:]
		}
		key, value, hasKey := strings.Cut(token, ":")
		if !hasKey {
			value = token
			key = ""
		}
		term.key = strings.ToLower(strings.TrimSpace(key))
		term.value = strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
		switch term.key {
		case "pid", "ppid", "uid":
			if n, err := strconv.ParseUint(term.value, 10, 32); err == nil {
				term.numeric = n
				term.isNumber = true
			}
		case "risk":
			term.minRisk = parseRiskBound(term.value)
		}
		if term.value == "" && !term.isNumber {
			continue
		}
		f.terms = append(f.terms, term)
	}
	return f
}

func splitFilterTokens(raw string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func parseRiskBound(value string) float64 {
	value = strings.TrimLeft(value, ">=")
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return n
}

// IsZero reports whether the filter accepts everything.
func (f Filter) IsZero() bool { return len(f.terms) == 0 }

func (f Filter) Match(ev *pb.Event) bool {
	for _, term := range f.terms {
		if term.matches(ev) == term.negate {
			return false
		}
	}
	return true
}

func (t filterTerm) matches(ev *pb.Event) bool {
	switch t.key {
	case "":
		return containsFold(ev.GetComm(), t.value) ||
			containsFold(ev.GetType(), t.value) ||
			containsFold(ev.GetPath(), t.value) ||
			containsFold(ev.GetNetEndpoint(), t.value) ||
			containsFold(ev.GetDomain(), t.value) ||
			containsFold(ev.GetTag(), t.value) ||
			containsFold(ev.GetExtraInfo(), t.value)
	case "type":
		return containsFold(ev.GetType(), t.value)
	case "comm":
		return containsFold(ev.GetComm(), t.value)
	case "path":
		return containsFold(ev.GetPath(), t.value) || containsFold(ev.GetExtraPath(), t.value)
	case "tag":
		return containsFold(ev.GetTag(), t.value)
	case "net", "endpoint", "host":
		return containsFold(ev.GetNetEndpoint(), t.value) || containsFold(ev.GetDomain(), t.value) ||
			containsFold(ev.GetDstIp(), t.value) || containsFold(ev.GetSni(), t.value)
	case "decision":
		return containsFold(ev.GetDecision(), t.value)
	case "tool":
		return containsFold(ev.GetToolName(), t.value)
	case "run":
		return containsFold(ev.GetAgentRunId(), t.value)
	case "pid":
		return t.isNumber && uint64(ev.GetPid()) == t.numeric
	case "ppid":
		return t.isNumber && uint64(ev.GetPpid()) == t.numeric
	case "uid":
		return t.isNumber && uint64(ev.GetUid()) == t.numeric
	case "risk":
		return ev.GetRiskScore() >= t.minRisk
	default:
		return containsFold(ev.GetExtraInfo(), t.key+"="+t.value)
	}
}

// containsFold is an allocation-free ASCII case-insensitive substring test;
// sub must already be lower-cased.
func containsFold(s, sub string) bool {
	if sub == "" {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	first := sub[0]
	for i := 0; i+len(sub) <= len(s); i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		match := true
		for j := 1; j < len(sub); j++ {
			if lowerASCII(s[i+j]) != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
