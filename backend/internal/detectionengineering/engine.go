// Package detectionengineering provides bounded, read-only detection linting and
// telemetry replay. It deliberately has no kernel, policy-write or LLM imports.
package detectionengineering

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersion = "detection-replay.v1"
	MaxEvents     = 20000
	MaxSignals    = 8
	MaxFindings   = 256
	MaxLabels     = 1000
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var signalValuePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}// Package detectionengineering provides bounded, read-only detection linting and
// telemetry replay. It deliberately has no kernel, policy-write or LLM imports.
package detectionengineering

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersion = "detection-replay.v1"
	MaxEvents     = 20000
	MaxSignals    = 8
	MaxFindings   = 256
	MaxLabels     = 1000
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
)
var hostLikePattern = regexp.MustCompile(`(?i)^[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:com|net|org|io|dev|edu|gov|ai|cn|uk|local)// Package detectionengineering provides bounded, read-only detection linting and
// telemetry replay. It deliberately has no kernel, policy-write or LLM imports.
package detectionengineering

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersion = "detection-replay.v1"
	MaxEvents     = 20000
	MaxSignals    = 8
	MaxFindings   = 256
	MaxLabels     = 1000
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
)

var supportedFields = map[string]bool{
	"eventType": true,
	"source":    true,
	"comm":      true,
}

// Rule operates on observable, redacted metadata, not prompt contents, raw
// targets, IP addresses or names tied to a specific machine.
type Rule struct {
	ID          string   `json:"id"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope"` // trace or pid
	WindowMS    int64    `json:"windowMs"`
	MinSignals  int      `json:"minSignals"`
	Signals     []Signal `json:"signals"`
}

type Signal struct {
	Field string `json:"field"` // eventType, source, comm
	Value string `json:"value"` // case-insensitive exact match
}

type Event struct {
	ID        string
	Timestamp int64 // Unix milliseconds
	TraceID   string
	PID       uint32
	EventType string
	Source    string
	Comm      string
}

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Finding struct {
	Scope            string   `json:"scope"`
	FirstTimestamp   int64    `json:"firstTimestamp"`
	LastTimestamp    int64    `json:"lastTimestamp"`
	MatchedSignals   []int    `json:"matchedSignals"`
	EvidenceEventIDs []string `json:"evidenceEventIds"`
}

type Metrics struct {
	AuthoringAttacks  int      `json:"authoringAttacks"`
	AuthoringDetected int      `json:"authoringDetected"`
	HoldoutAttacks    int      `json:"holdoutAttacks"`
	HoldoutDetected   int      `json:"holdoutDetected"`
	BenignScopes      int      `json:"benignScopes"`
	BenignFlagged     int      `json:"benignFlagged"`
	HoldoutRecall     *float64 `json:"holdoutRecall,omitempty"`
	FalsePositiveRate *float64 `json:"falsePositiveRate,omitempty"`
	Precision         *float64 `json:"precision,omitempty"`
}

// Labels must be human-supplied per observed scope. Risk scores and existing
// ALLOW/BLOCK decisions MUST NOT be recycled as attack ground truth.
// "holdout_attack" is operator-asserted; the engine cannot prove independence.
type Report struct {
	SchemaVersion          string    `json:"schemaVersion"`
	RuleID                 string    `json:"ruleId"`
	Valid                  bool      `json:"valid"`
	Issues                 []Issue   `json:"issues"`
	ObservedScopes         []string  `json:"observedScopes"`
	SkippedUnscoped        int       `json:"skippedUnscoped"`
	Findings               []Finding `json:"findings"`
	FindingsTruncated      bool      `json:"findingsTruncated"`
	DetectedScopes         int       `json:"detectedScopes"`
	UnlabeledScopes        int       `json:"unlabeledScopes"`
	Metrics                Metrics   `json:"metrics"`
	EligibleForHumanReview bool      `json:"eligibleForHumanReview"`
	EnforcementApplied     bool      `json:"enforcementApplied"`
}

func Lint(rule Rule) []Issue {
	issues := make([]Issue, 0)
	add := func(code, detail string) { issues = append(issues, Issue{Code: code, Message: detail}) }
	if !ruleIDPattern.MatchString(rule.ID) {
		add("invalid_id", "id must be 1-64 characters: letters, digits, dot, underscore or dash")
	}
	if rule.Scope != "trace" && rule.Scope != "pid" {
		add("invalid_scope", "scope must be trace or pid; missing identity is never treated as global")
	}
	if rule.WindowMS < 10 || rule.WindowMS > 600000 {
		add("invalid_window", "windowMs must be between 10 and 600000 milliseconds")
	}
	if len(rule.Signals) < 2 || len(rule.Signals) > MaxSignals {
		add("invalid_signal_count", "rules require 2-8 signals")
	}
	if rule.MinSignals < 2 || rule.MinSignals > len(rule.Signals) {
		add("invalid_threshold", "minSignals must be between 2 and the number of signals")
	}
	uniqueFields := map[string]bool{}
	seen := map[string]bool{}
	for i, signal := range rule.Signals {
		if !supportedFields[signal.Field] {
			add("unknown_field", fmt.Sprintf("signals[%d]: unsupported field; use eventType, source or comm", i))
			continue
		}
		uniqueFields[signal.Field] = true
		if !signalValuePattern.MatchString(signal.Value) || net.ParseIP(signal.Value) != nil || hostLikePattern.MatchString(signal.Value) {
			add("unstable_identifier", fmt.Sprintf("signals[%d]: use a short behavioral token, not a path, host, IP, URL or wildcard", i))
		}
		key := signal.Field + ":" + strings.ToLower(signal.Value)
		if seen[key] {
			add("duplicate_signal", fmt.Sprintf("signals[%d]: duplicate predicate", i))
		}
		seen[key] = true
	}
	if len(uniqueFields) < 2 {
		add("single_dimension", "a correlated rule must use at least two different telemetry fields")
	}
	return issues
}

func eventScope(rule Rule, event Event) string {
	switch rule.Scope {
	case "trace":
		if strings.TrimSpace(event.TraceID) != "" {
			return "trace:" + event.TraceID
		}
	case "pid":
		if event.PID != 0 {
			return fmt.Sprintf("pid:%d", event.PID)
		}
	}
	return ""
}

func matches(signal Signal, event Event) bool {
	switch signal.Field {
	case "eventType":
		return strings.EqualFold(signal.Value, event.EventType)
	case "source":
		return strings.EqualFold(signal.Value, event.Source)
	case "comm":
		return strings.EqualFold(signal.Value, event.Comm)
	default:
		return false
	}
}

type indexedEvent struct {
	event Event
	mask  uint16
	order int
}

// Replay uses an O(N log N + N*S) sorted, sliding-window correlation engine
// where S <= 8. Every candidate is evaluated without modifying runtime policy.
// Labels: scope -> authoring_attack | holdout_attack | benign.
func Replay(rule Rule, events []Event, labels map[string]string) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		RuleID: rule.ID,
		Issues: make([]Issue, 0),
		ObservedScopes: make([]string, 0),
		Findings: make([]Finding, 0),
	}
	report.Issues = append(report.Issues, Lint(rule)...)
	if len(events) > MaxEvents {
		report.Issues = append(report.Issues, Issue{"too_many_events", fmt.Sprintf("at most %d events per replay", MaxEvents)})
	}
	if len(labels) > MaxLabels {
		report.Issues = append(report.Issues, Issue{"too_many_labels", fmt.Sprintf("at most %d labels per replay", MaxLabels)})
	}
	for scope, label := range labels {
		if label != "authoring_attack" && label != "holdout_attack" && label != "benign" {
			report.Issues = append(report.Issues, Issue{"invalid_label", fmt.Sprintf("scope %q: unsupported label", scope)})
		}
	}
	if len(report.Issues) > 0 {
		return report
	}
	report.Valid = true
	groups := make(map[string][]indexedEvent)
	for i, event := range events {
		scope := eventScope(rule, event)
		if scope == "" || event.Timestamp <= 0 {
			report.SkippedUnscoped++
			continue
		}
		var mask uint16
		for j, signal := range rule.Signals {
			if matches(signal, event) {
				mask |= 1 << uint(j)
			}
		}
		groups[scope] = append(groups[scope], indexedEvent{event: event, mask: mask, order: i})
	}
	for scope := range groups {
		report.ObservedScopes = append(report.ObservedScopes, scope)
	}
	sort.Strings(report.ObservedScopes)
	for scope := range labels {
		if _, ok := groups[scope]; !ok {
			report.Issues = append(report.Issues, Issue{"label_without_events", fmt.Sprintf("label scope %q does not exist in replay", scope)})
		}
	}
	if len(report.Issues) > 0 {
		report.Valid = false
		return report
	}
	detected := make(map[string]bool)
	for _, scope := range report.ObservedScopes {
		group := groups[scope]
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].event.Timestamp == group[j].event.Timestamp {
				return group[i].order < group[j].order
			}
			return group[i].event.Timestamp < group[j].event.Timestamp
		})
		counts := make([]int, len(rule.Signals))
		left := 0
		active := false
		for right, item := range group {
			cutoff := item.event.Timestamp - rule.WindowMS
			// Expire previous evidence BEFORE adding the current event. Otherwise
			// an all-signals current event can hide a gap between distinct episodes.
			for left < right && group[left].event.Timestamp < cutoff {
				for j := range counts {
					if group[left].mask&(1<<uint(j)) != 0 {
						counts[j]--
					}
				}
				left++
			}
			if active {
				live, dimensions := 0, map[string]bool{}
				for j, count := range counts {
					if count > 0 {
						live++
						dimensions[rule.Signals[j].Field] = true
					}
				}
				if live < rule.MinSignals || len(dimensions) < 2 {
					active = false
				}
			}
			for j := range counts {
				if item.mask&(1<<uint(j)) != 0 {
					counts[j]++
				}
			}
			var selected []int
			dimensions := make(map[string]bool)
			for j, count := range counts {
				if count > 0 {
					selected = append(selected, j)
					dimensions[rule.Signals[j].Field] = true
				}
			}
			isMatch := len(selected) >= rule.MinSignals && len(dimensions) >= 2
			if !isMatch {
				active = false
				continue
			}
			if active {
				continue
			}
			active = true
			detected[scope] = true
			if len(report.Findings) == MaxFindings {
				report.FindingsTruncated = true
				continue
			}
			// Evidence is limited to one event for each selected predicate.
			evidence, seen := make([]string, 0, len(selected)), map[string]bool{}
			for _, j := range selected {
				for k := right; k >= left; k-- {
					if group[k].mask&(1<<uint(j)) != 0 {
						id := group[k].event.ID
						if id != "" && !seen[id] {
							evidence = append(evidence, id)
							seen[id] = true
						}
						break
					}
				}
			}
			report.Findings = append(report.Findings, Finding{
				Scope: scope, FirstTimestamp: group[left].event.Timestamp,
				LastTimestamp: item.event.Timestamp,
				MatchedSignals: selected, EvidenceEventIDs: evidence,
			})
		}
	}
	report.DetectedScopes = len(detected)
	for _, scope := range report.ObservedScopes {
		switch labels[scope] {
		case "authoring_attack":
			report.Metrics.AuthoringAttacks++
			if detected[scope] { report.Metrics.AuthoringDetected++ }
		case "holdout_attack":
			report.Metrics.HoldoutAttacks++
			if detected[scope] { report.Metrics.HoldoutDetected++ }
		case "benign":
			report.Metrics.BenignScopes++
			if detected[scope] { report.Metrics.BenignFlagged++ }
		default:
			report.UnlabeledScopes++
		}
	}
	m := &report.Metrics
	if m.HoldoutAttacks > 0 {
		v := float64(m.HoldoutDetected) / float64(m.HoldoutAttacks)
		m.HoldoutRecall = &v
	}
	if m.BenignScopes > 0 {
		v := float64(m.BenignFlagged) / float64(m.BenignScopes)
		m.FalsePositiveRate = &v
	}
	if m.HoldoutDetected+m.BenignFlagged > 0 {
		v := float64(m.HoldoutDetected) / float64(m.HoldoutDetected+m.BenignFlagged)
		m.Precision = &v
	}
	// This gate merely qualifies the rule for independent HUMAN review.
	// Deployment must occur through separate authenticated policy management.
	report.EligibleForHumanReview = m.HoldoutAttacks > 0 && m.BenignScopes > 0 &&
		m.HoldoutDetected == m.HoldoutAttacks && m.BenignFlagged == 0
	return report
}
