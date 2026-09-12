package main

import (
	"sort"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

const rateWindowSeconds = 60

// Stats aggregates the live stream. It is owned by the UI model and guarded by
// the model's mutex, so it carries no locking of its own.
type Stats struct {
	Total   uint64
	Risky   uint64 // risk score >= riskyThreshold or an ALERT decision
	Blocked uint64 // BLOCK/DENY decisions

	byType map[string]uint64
	byComm map[string]uint64
	byTag  map[string]uint64

	// perSecond is a ring of event counts keyed by unix second; slot i holds
	// the count for second bucketAt[i]. Old buckets are lazily zeroed.
	perSecond [rateWindowSeconds]uint32
	bucketAt  [rateWindowSeconds]int64
}

const riskyThreshold = 40

func NewStats() *Stats {
	return &Stats{
		byType: make(map[string]uint64),
		byComm: make(map[string]uint64),
		byTag:  make(map[string]uint64),
	}
}

func (s *Stats) Observe(event *pb.Event, at time.Time) {
	s.Total++
	s.byType[event.GetType()]++
	if comm := event.GetComm(); comm != "" {
		s.byComm[comm]++
	}
	if tag := event.GetTag(); tag != "" {
		s.byTag[tag]++
	}
	switch decisionClass(event.GetDecision()) {
	case decisionBlock:
		s.Blocked++
		s.Risky++
	case decisionAlert:
		s.Risky++
	default:
		if event.GetRiskScore() >= riskyThreshold {
			s.Risky++
		}
	}
	sec := at.Unix()
	slot := int(sec % rateWindowSeconds)
	if s.bucketAt[slot] != sec {
		s.bucketAt[slot] = sec
		s.perSecond[slot] = 0
	}
	s.perSecond[slot]++
}

// Rate returns events per second averaged over the last window seconds.
func (s *Stats) Rate(now time.Time, window int) float64 {
	if window <= 0 || window > rateWindowSeconds {
		window = rateWindowSeconds
	}
	var sum uint64
	sec := now.Unix()
	for i := 0; i < window; i++ {
		t := sec - int64(i)
		slot := int(t % rateWindowSeconds)
		if s.bucketAt[slot] == t {
			sum += uint64(s.perSecond[slot])
		}
	}
	return float64(sum) / float64(window)
}

// Sparkline returns the last n per-second counts, oldest first.
func (s *Stats) Sparkline(now time.Time, n int) []uint32 {
	if n <= 0 || n > rateWindowSeconds {
		n = rateWindowSeconds
	}
	out := make([]uint32, n)
	sec := now.Unix()
	for i := 0; i < n; i++ {
		t := sec - int64(n-1-i)
		slot := int(t % rateWindowSeconds)
		if s.bucketAt[slot] == t {
			out[i] = s.perSecond[slot]
		}
	}
	return out
}

type countedKey struct {
	Key   string
	Count uint64
}

// Top returns the n most frequent keys of the requested histogram.
func (s *Stats) Top(kind string, n int) []countedKey {
	switch kind {
	case "comm":
		return topCounts(s.byComm, n)
	case "tag":
		return topCounts(s.byTag, n)
	default:
		return topCounts(s.byType, n)
	}
}

// topCounts returns the n largest entries of a histogram, ties broken by key.
func topCounts(src map[string]uint64, n int) []countedKey {
	out := make([]countedKey, 0, len(src))
	for key, count := range src {
		out = append(out, countedKey{Key: key, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (s *Stats) Reset() {
	*s = *NewStats()
}

type decisionKind int

const (
	decisionNone decisionKind = iota
	decisionAlert
	decisionBlock
)

func decisionClass(decision string) decisionKind {
	switch strings.ToUpper(strings.TrimSpace(decision)) {
	case "BLOCK", "DENY", "KILL", "REJECT":
		return decisionBlock
	case "ALERT", "WARN", "WARNING", "REVIEW":
		return decisionAlert
	default:
		return decisionNone
	}
}
