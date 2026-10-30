// Package agentpipeline correlates sanitized Agent messages on the existing
// single-owner event stream. It never retains prompts, arguments or outputs.
package agentpipeline

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxSessions = 4096
	defaultMaxCalls    = 64
	defaultTTL         = 5 * time.Minute
	completionGrace   = 2 * time.Second
	maxFieldBytes      = 256
)

// Message is the common, transport-independent representation of a captured
// hook, tool invocation, model interaction or OS side effect. At is ingest
// time, not a potentially untrusted kernel/producer timestamp.
type Message struct {
	At              time.Time
	Kind            string // tool_call, tool_result, model_input, session_end, process, file, network, other
	Action          string // read, write, execute, egress, none
	AgentRunID      string
	ConversationID  string
	TaskID          string
	TurnID          string
	TraceID         string
	ToolCallID      string
	ToolName        string
	RootPID         uint32
	PID             uint32
	SideEffectScope string // explicit tool declaration; never inferred from a name
	PermissionMode  string // explicit plan mode
	UntrustedInput  bool   // producer explicitly marked external/untrusted content
	HighRiskInput   bool   // independent detector explicitly marked input high-risk
	Target          string // only emitted in a finding; never retained in state
}

type Finding struct {
	Code      string
	Reason    string
	Target    string
	RiskScore float64 // 0..100, consistent with pb.Event
}

type Status struct {
	Sessions int `json:"sessions"`
	MaxSessions int `json:"maxSessions"`
	Evictions uint64 `json:"evictions"`
	Findings uint64 `json:"findings"`
	ActiveCalls int `json:"activeCalls"`
	CompletedCalls int `json:"completedCalls"`
	TaintedSessions int `json:"taintedSessions"`
}

type invocation struct {
	name     string
	readOnly bool
	planMode bool
	seenAt   time.Time
	turn     string
	completedAt time.Time
	reported uint8
}

type session struct {
	key           string
	touched       time.Time
	calls         map[string]*invocation
	taintedAt     time.Time
	taintedTurn   string
	taintReported bool
	element       *list.Element
}

// Pipeline is an in-process ordered stage, not a second lossy queue.
// The existing broadcaster owns message admission, sequencing, retention and
// delivery. State is bounded even under attacker-controlled context IDs.
type Pipeline struct {
	mu          sync.Mutex
	sessions    map[string]*session
	recency     list.List
	maxSessions int
	ttl         time.Duration
	evictions   uint64
	findings    uint64
}

func New(maxSessions int, ttl time.Duration) *Pipeline {
	if maxSessions <= 0 {
		maxSessions = defaultMaxSessions
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Pipeline{sessions: make(map[string]*session), maxSessions: maxSessions, ttl: ttl}
}

// Process consumes exactly one message in stream order and returns zero or more
// evidence-based alerts. It does not issue enforcement actions or mutate input.
func (p *Pipeline) Process(m Message) []Finding {
	if p == nil || m.Kind == "semantic_alert" {
		return nil
	}
	// Inert model frames do not create session state. A risk signal must be
	// explicitly marked by an upstream detector, not inferred from raw text.
	if m.Kind == "model_input" && !(m.UntrustedInput && m.HighRiskInput) {
		return nil
	}
	key := sessionKey(m)
	if key == "" {
		return nil
	}
	now := m.At
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sessions == nil {
		p.sessions = make(map[string]*session)
		if p.maxSessions <= 0 {
			p.maxSessions = defaultMaxSessions
		}
		if p.ttl <= 0 {
			p.ttl = defaultTTL
		}
	}
	s := p.sessions[key]
	if s != nil && (now.Sub(s.touched) > p.ttl || now.Before(s.touched.Add(-time.Second))) {
		p.remove(s)
		s = nil
	}
	// Lifecycle/end events never allocate a new scope. A syscall without a
	// prior declaration or independently flagged risk also has no state to
	// correlate; dropping it here protects the high-volume kernel path.
	if m.Kind == "session_end" {
		if s != nil {
			p.remove(s)
		}
		return nil
	}
	if s == nil && m.Kind != "tool_call" && !(m.UntrustedInput && m.HighRiskInput) {
		return nil
	}
	if s == nil {
		if len(p.sessions) >= p.maxSessions {
			if oldest := p.recency.Back(); oldest != nil {
				p.remove(oldest.Value.(*session))
				p.evictions++
			}
		}
		s = &session{key: key, calls: make(map[string]*invocation), touched: now}
		s.element = p.recency.PushFront(s)
		p.sessions[key] = s
	} else {
		p.recency.MoveToFront(s.element)
		s.touched = now
	}
	// Tool-call expiry is checked lazily for the touched call below;
	// do not sweep up to 64 calls for every hot-path process/network event.
	var findings []Finding
	emit := func(code, reason string, score float64) {
		findings = append(findings, Finding{Code: code, Reason: reason, Target: bound(m.Target), RiskScore: score})
		p.findings++
	}

	id := bound(m.ToolCallID)
	switch m.Kind {
	case "tool_call":
		if id != "" {
			call := s.calls[id]
			if call != nil && (now.Sub(call.seenAt) > p.ttl ||
				(call.turn != "" && m.TurnID != "" && call.turn != bound(m.TurnID)) ||
				!call.completedAt.IsZero()) {
				// A new invocation of a reused call ID must not inherit the
				// previous call's permission declaration or risk flags.
				delete(s.calls, id)
				call = nil
			}
			if call != nil && call.name != "" && m.ToolName != "" && call.name != bound(m.ToolName) {
				if call.reported&8 == 0 {
					emit("TOOL_CALL_ID_CONFLICT", "one scoped tool-call ID was reused for different tools", 83)
					call.reported |= 8
				}
			} else {
				if call == nil {
					if len(s.calls) >= defaultMaxCalls {
						// Evict the oldest call deterministically (tie-break on ID).
						oldestID := ""
						var oldest time.Time
						for cid, c := range s.calls {
							if oldestID == "" || c.seenAt.Before(oldest) || (c.seenAt.Equal(oldest) && cid < oldestID) {
								oldestID, oldest = cid, c.seenAt
							}
						}
						delete(s.calls, oldestID)
					}
					call = &invocation{}
					s.calls[id] = call
				}
				call.name = bound(m.ToolName)
				call.turn = bound(m.TurnID)
				call.seenAt = now
				call.readOnly = call.readOnly || isReadOnly(m.SideEffectScope)
				call.planMode = call.planMode || strings.EqualFold(strings.TrimSpace(m.PermissionMode), "plan")
			}
		}
	case "model_input", "tool_result":
		if m.Kind == "tool_result" && id != "" {
			if call := s.calls[id]; call != nil &&
				(call.turn == "" || m.TurnID == "" || call.turn == bound(m.TurnID)) {
				if call.completedAt.IsZero() {
					call.completedAt = now
				}
			}
		}
		if m.UntrustedInput && m.HighRiskInput {
			s.taintedAt = now
			s.taintedTurn = bound(m.TurnID)
			s.taintReported = false
		}
	}
	if m.Action == "write" || m.Action == "execute" || m.Action == "egress" {
		if call := s.calls[id]; id != "" && call != nil && now.Sub(call.seenAt) >= 0 && now.Sub(call.seenAt) <= p.ttl &&
			(call.turn == "" || m.TurnID == "" || call.turn == bound(m.TurnID)) &&
			(call.completedAt.IsZero() || (!now.Before(call.completedAt) && now.Sub(call.completedAt) <= completionGrace)) {
			if call.readOnly && call.reported&1 == 0 {
				emit("TOOL_SCOPE_VIOLATION", "tool declared read-only scope but produced a side effect", 96)
				call.reported |= 1
			}
			if call.planMode && call.reported&2 == 0 {
				emit("PLAN_MODE_SIDE_EFFECT", "tool declared plan mode but produced a side effect", 95)
				call.reported |= 2
			}
		}
		if !s.taintedAt.IsZero() && now.Sub(s.taintedAt) >= 0 && now.Sub(s.taintedAt) <= p.ttl && !s.taintReported &&
			(s.taintedTurn == "" || m.TurnID == "" || s.taintedTurn == bound(m.TurnID)) {
			emit("UNTRUSTED_INPUT_ACTION_CHAIN", "explicitly flagged high-risk untrusted input preceded an Agent side effect", 94)
			s.taintReported = true
		}
	}
	return findings
}

func (p *Pipeline) remove(s *session) {
	delete(p.sessions, s.key)
	p.recency.Remove(s.element)
}

func (p *Pipeline) Stats() Status {
	if p == nil {
		return Status{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	status := Status{Sessions: len(p.sessions), MaxSessions: p.maxSessions, Evictions: p.evictions, Findings: p.findings}
	for _, s := range p.sessions {
		if !s.taintedAt.IsZero() && !s.taintReported {
			status.TaintedSessions++
		}
		for _, call := range s.calls {
			if call.completedAt.IsZero() {
				status.ActiveCalls++
			} else {
				status.CompletedCalls++
			}
		}
	}
	return status
}

func isReadOnly(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "readonly", "read_only", "read-only", "read", "none":
		return true
	default:
		return false
	}
}

func sessionKey(m Message) string {
	// Never key by tool_call_id alone: IDs collide across independent Agents.
	// Require a parent identity; PID-only fallback is intentionally short-lived.
	switch {
	case m.AgentRunID != "":
		return "run:" + bound(m.AgentRunID) + "|task:" + bound(m.TaskID) + "|conv:" + bound(m.ConversationID) + "|root:" + strconv.FormatUint(uint64(m.RootPID), 10)
	case m.ConversationID != "":
		return "conv:" + bound(m.ConversationID) + "|root:" + strconv.FormatUint(uint64(m.RootPID), 10)
	case m.TraceID != "":
		return "trace:" + bound(m.TraceID) + "|root:" + strconv.FormatUint(uint64(m.RootPID), 10)
	case m.RootPID != 0:
		return "root:" + strconv.FormatUint(uint64(m.RootPID), 10)
	case m.PID != 0:
		return "pid:" + strconv.FormatUint(uint64(m.PID), 10)
	default:
		return ""
	}
}

func bound(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxFieldBytes {
		return s
	}
	digest := sha256.Sum256([]byte(s))
	return s[:maxFieldBytes-17] + "#" + hex.EncodeToString(digest[:8])
}
