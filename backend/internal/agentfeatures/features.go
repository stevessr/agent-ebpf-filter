// Package agentfeatures defines an offline, metadata-only feature space for
// Agent context and harness events. It is not wired to runtime policy decisions.
package agentfeatures

import (
	"errors"
	"sort"
	"strings"
)

const (
	SchemaVersion   = "agent-context-harness.v1"
	MaxBatchEvents  = 20000
)

// Evidence describes how an association was established. Observing an OS
// syscall does NOT make an inherited tool-call attribution "observed".
type Evidence string

const (
	Observed Evidence = "observed"
	Derived  Evidence = "derived"
	Inferred Evidence = "inferred"
	Unknown  Evidence = "unknown"
)

// Event is a deliberately small, already-redacted input contract. Upstream
// adapters must supply wall-clock Unix nanoseconds, never raw BPF monotonic
// timestamps. No prompt, command argument, file path, token, or response body
// belongs here. An empty RunID stays unknown and is never synthesized.
type Event struct {
	ID                  string   `json:"id"`
	HostID              string   `json:"hostId"`
	RunID               string   `json:"runId"`
	ToolCallID          string   `json:"toolCallId,omitempty"`
	ProcessKey          string   `json:"processKey,omitempty"`
	Source              string   `json:"source"`
	Kind                string   `json:"kind"` // tool, process_exec, file_write, network_connect, other
	Phase               string   `json:"phase,omitempty"` // start, result (tool only)
	Outcome             string   `json:"outcome,omitempty"` // success, failure (tool result only)
	EventTimeNS         int64    `json:"eventTimeNs"`
	EventEvidence       Evidence `json:"eventEvidence"`
	AttributionEvidence Evidence `json:"attributionEvidence,omitempty"`
	CaptureAudited      bool     `json:"captureAudited"`
	CaptureLoss         bool     `json:"captureLoss"`
}

// Window has inclusive start and exclusive end in the same Unix-ns clock
// domain. The extractor is stateless, event-time based and replayable.
type Window struct {
	StartNS int64 `json:"startNs"`
	EndNS   int64 `json:"endNs"`
}

type Definition struct {
	ID          string `json:"id"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
}

var definitions = []Definition{
	{"events.total", "count", "Accepted, source-scoped deduplicated metadata events"},
	{"tool.start_events", "count", "Observed tool-start signals; not distinct causal calls"},
	{"tool.result_events", "count", "Observed tool-result signals with explicit outcome"},
	{"tool.failure_ratio", "ratio", "Failure ratio among observed tool-result signals"},
	{"os.process_exec", "count", "Observed OS process-exec signals"},
	{"os.file_write", "count", "Observed OS file-write signals"},
	{"os.network_connect", "count", "Observed OS network-connect signals"},
	{"os.explicit_tool_ratio", "ratio", "OS signals with explicitly observed tool attribution among OS signals with attribution evidence"},
	{"evidence.inferred_ratio", "ratio", "Events marked inferred among events with known event evidence"},
	{"capture.loss_ratio", "ratio", "Loss-marked signals among samples with explicit capture audit"},
}

// Catalog is versioned and ordered. Its IDs are NOT offsets into the old
// 128-dim command FeatureExtractor and must not alter persisted ML models.
func Catalog() []Definition {
	return append([]Definition(nil), definitions...)
}

type Value struct {
	ID      string  `json:"id"`
	Value   float64 `json:"value"`
	Present bool    `json:"present"`
	Reason  string  `json:"reason,omitempty"`
}

type Frame struct {
	SchemaVersion string  `json:"schemaVersion"`
	HostID        string  `json:"hostId"`
	RunID         string  `json:"runId"`
	Window        Window  `json:"window"`
	Values        []Value `json:"values"`
}

type Report struct {
	SchemaVersion       string  `json:"schemaVersion"`
	Frames              []Frame `json:"frames"`
	SkippedUnknownScope int     `json:"skippedUnknownScope"`
	SkippedNoTime       int     `json:"skippedNoTime"`
	SkippedOutOfWindow  int     `json:"skippedOutOfWindow"`
	Duplicates          int     `json:"duplicates"`
}

type scope struct{ host, run string }
type eventKey struct{ scope scope; source, id string }
type counts struct {
	total, starts, results, failures int
	exec, writes, connects          int
	osAttributed, osKnown           int
	knownEvidence, inferred         int
	audited, loss                   int
}

func (c *counts) observe(e Event) {
	c.total++
	switch e.EventEvidence {
	case Observed, Derived, Inferred:
		c.knownEvidence++
		if e.EventEvidence == Inferred {
			c.inferred++
		}
	}
	if e.CaptureAudited {
		c.audited++
		if e.CaptureLoss {
			c.loss++
		}
	}
	// Tool events with unknown or inferred event evidence do not establish
	// a confirmed tool lifecycle. A missing outcome is NOT a success.
	if e.Kind == "tool" && e.EventEvidence == Observed {
		switch e.Phase {
		case "start":
			c.starts++
		case "result":
			if e.Outcome == "success" || e.Outcome == "failure" {
				c.results++
				if e.Outcome == "failure" {
					c.failures++
				}
			}
		}
	}
	isOS := e.Kind == "process_exec" || e.Kind == "file_write" || e.Kind == "network_connect"
	if !isOS || e.EventEvidence != Observed {
		return
	}
	switch e.Kind {
	case "process_exec":
		c.exec++
	case "file_write":
		c.writes++
	case "network_connect":
		c.connects++
	}
	if e.AttributionEvidence == Observed || e.AttributionEvidence == Derived || e.AttributionEvidence == Inferred {
		c.osKnown++
		// ToolCallID alone is insufficient: only explicitly observed
		// attribution counts as exact tool-to-OS linkage.
		if e.AttributionEvidence == Observed && strings.TrimSpace(e.ToolCallID) != "" {
			c.osAttributed++
		}
	}
}

func scalar(id string, v int) Value { return Value{ID: id, Value: float64(v), Present: true} }
func ratio(id string, numerator, denominator int) Value {
	if denominator == 0 {
		return Value{ID: id, Present: false, Reason: "denominator_unobserved"}
	}
	return Value{ID: id, Value: float64(numerator) / float64(denominator), Present: true}
}

func (c *counts) frame(k scope, window Window) Frame {
	return Frame{
		SchemaVersion: SchemaVersion, HostID: k.host, RunID: k.run, Window: window,
		Values: []Value{
			scalar("events.total", c.total),
			scalar("tool.start_events", c.starts),
			scalar("tool.result_events", c.results),
			ratio("tool.failure_ratio", c.failures, c.results),
			scalar("os.process_exec", c.exec),
			scalar("os.file_write", c.writes),
			scalar("os.network_connect", c.connects),
			ratio("os.explicit_tool_ratio", c.osAttributed, c.osKnown),
			ratio("evidence.inferred_ratio", c.inferred, c.knownEvidence),
			ratio("capture.loss_ratio", c.loss, c.audited),
		},
	}
}

// Extract builds independent host+run frames. It does not modify system state,
// infer run identities, parse private content, or use policy/model scores.
// A stable (host,run,source,event ID) deduplicates transport replays only;
// event IDs from different sources are intentionally not assumed equivalent.
func Extract(events []Event, window Window) (Report, error) {
	if window.StartNS < 0 || window.EndNS <= window.StartNS {
		return Report{}, errors.New("invalid event-time window")
	}
	if len(events) > MaxBatchEvents {
		return Report{}, errors.New("too many events in offline feature batch")
	}
	report := Report{SchemaVersion: SchemaVersion, Frames: []Frame{}}
	groups := make(map[scope]*counts)
	seen := make(map[eventKey]struct{}, len(events))
	for _, e := range events {
		if e.EventTimeNS <= 0 {
			report.SkippedNoTime++
			continue
		}
		if e.EventTimeNS < window.StartNS || e.EventTimeNS >= window.EndNS {
			report.SkippedOutOfWindow++
			continue
		}
		if strings.TrimSpace(e.HostID) == "" || strings.TrimSpace(e.RunID) == "" {
			report.SkippedUnknownScope++
			continue
		}
		k := scope{e.HostID, e.RunID}
		if e.ID != "" {
			dk := eventKey{k, e.Source, e.ID}
			if _, ok := seen[dk]; ok {
				report.Duplicates++
				continue
			}
			seen[dk] = struct{}{}
		}
		c := groups[k]
		if c == nil {
			c = &counts{}
			groups[k] = c
		}
		c.observe(e)
	}
	keys := make([]scope, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].host == keys[j].host {
			return keys[i].run < keys[j].run
		}
		return keys[i].host < keys[j].host
	})
	for _, k := range keys {
		report.Frames = append(report.Frames, groups[k].frame(k, window))
	}
	return report, nil
}
