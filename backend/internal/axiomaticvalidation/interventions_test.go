package axiomaticvalidation

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"agent-ebpf-filter/internal/agentidentity"
	"agent-ebpf-filter/internal/agentscope"
	"agent-ebpf-filter/internal/detectionengineering"
	"agent-ebpf-filter/internal/eventnoise"
	"agent-ebpf-filter/internal/eventqueue"
)

// Finite fixture coverage, not independent random trials or production effects.
// The experiment deliberately changes one input at a time at the pure-package
// boundary. It does not instantiate the app broadcaster or attach BPF.
func TestScopeIntervention(t *testing.T) {
	const owner = "agent-fixture"
	store := agentidentity.NewStoreWithClock(func() time.Time { return time.Unix(100, 0) })
	store.Observe(agentidentity.Evidence{PID: 1, RootPID: 1, Comm: owner, RunID: "run"})
	var cases, captureChanges, monitorChanges, ownerChanges int
	for _, comm := range []string{"agent-fixture", "shell", "tool"} {
		for _, tag := range []string{"agent-fixture", "other"} {
			for _, run := range []string{"run", "different", ""} {
				e := agentidentity.Evidence{PID: 2, RootPID: 1, Comm: comm, RunID: run}
				wantOwner := ""
				if run == "run" {
					wantOwner = owner
				}
				gotOwner := store.OwnerComm(e)
				if gotOwner != wantOwner {
					t.Fatalf("owner mismatch: %+v: %q", e, gotOwner)
				}
				for _, cMode := range []string{agentscope.ModeBlacklist, agentscope.ModeWhitelist} {
					for _, mMode := range []string{agentscope.ModeBlacklist, agentscope.ModeWhitelist} {
						p := agentscope.Policy{
							Capture: agentscope.List{Mode: cMode, Entries: []string{owner}},
							Monitor: agentscope.List{Mode: mMode, Entries: []string{owner}},
						}
						baseC := agentscope.AllowsWithOwner(p.Capture, comm, tag, gotOwner)
						baseM := agentscope.AllowsWithOwner(p.Monitor, comm, tag, gotOwner)
						match := comm == owner || tag == owner || wantOwner == owner
						wantC := match == (cMode == agentscope.ModeWhitelist)
						wantM := match == (mMode == agentscope.ModeWhitelist)
						if baseC != wantC || baseM != wantM {
							t.Fatalf("scope oracle mismatch: %+v", p)
						}
						// Intervene on capture and monitor separately. The other
						// raw predicate must not change; effective downstream
						// monitoring is capture AND monitor and is not invariant.
						pc := p
						pc.Capture.Mode = flipMode(cMode)
						pm := p
						pm.Monitor.Mode = flipMode(mMode)
						if agentscope.AllowsWithOwner(pc.Monitor, comm, tag, gotOwner) != baseM ||
							agentscope.AllowsWithOwner(pm.Capture, comm, tag, gotOwner) != baseC {
							t.Fatal("cross-predicate change")
						}
						if agentscope.AllowsWithOwner(pc.Capture, comm, tag, gotOwner) != baseC {
							captureChanges++
						}
						if agentscope.AllowsWithOwner(pm.Monitor, comm, tag, gotOwner) != baseM {
							monitorChanges++
						}
						if store.OwnerComm(e) != gotOwner {
							ownerChanges++
						}
						cases++
					}
				}
			}
		}
	}
	if cases != 72 || captureChanges != cases || monitorChanges != cases || ownerChanges != 0 {
		t.Fatal("scope intervention coverage mismatch")
	}
	t.Logf("AD_FIXTURE scope cases=%d capture_changes=%d monitor_changes=%d owner_changes=%d", cases, captureChanges, monitorChanges, ownerChanges)
}

func flipMode(mode string) string {
	if mode == agentscope.ModeWhitelist {
		return agentscope.ModeBlacklist
	}
	return agentscope.ModeWhitelist
}

func TestNoiseIntervention(t *testing.T) {
	var cases, protectedSuppressed, ordinaryChanged int
	for _, path := range []string{"/tmp/item", "/tmp-other/item", "/proc/item", "/home/item"} {
		for _, decision := range []string{"ALLOW", "BLOCK", "DENY", "ALERT"} {
			for _, risk := range []int64{0, 59, 60, 100} {
				for _, kind := range []string{"read", "semantic_alert"} {
					e := eventnoise.Event{Type: kind, Path: path, Decision: decision, RiskScore: risk}
					before := eventnoise.ShouldIgnore(e, nil)
					after := eventnoise.ShouldIgnore(e, []string{"/tmp", "/proc"})
					protected := decision != "ALLOW" || risk >= 60 || kind == "semantic_alert"
					want := !protected && (path == "/tmp/item" || path == "/proc/item")
					if before || after != want {
						t.Fatalf("noise oracle mismatch: %+v: %v -> %v", e, before, after)
					}
					if protected && after {
						protectedSuppressed++
					}
					if !protected && after != before {
						ordinaryChanged++
					}
					cases++
				}
			}
		}
	}
	if cases != 128 || protectedSuppressed != 0 || ordinaryChanged != 4 {
		t.Fatal("noise intervention coverage mismatch")
	}
	t.Logf("AD_FIXTURE noise cases=%d protected_suppressed=%d ordinary_changed=%d", cases, protectedSuppressed, ordinaryChanged)
}

func TestQueueCapacityIntervention(t *testing.T) {
	var cases, offered, admitted, rejected, consumed int
	for _, capacity := range []int{0, 1, 2, 4, 8, 16} {
		for _, count := range []int{0, 1, 2, 4, 8, 16, 32} {
			q := make(chan *int, capacity)
			want := count
			if capacity < want {
				want = capacity
			}
			var a, drop int
			for i := 0; i < count; i++ {
				item := i
				switch eventqueue.Offer(q, &item) {
				case eventqueue.Accepted:
					a++
				case eventqueue.QueueFull:
					drop++
				default:
					t.Fatal("unexpected offer result")
				}
			}
			if a != want || a+drop != count {
				t.Fatalf("queue conservation: cap=%d count=%d accepted=%d dropped=%d", capacity, count, a, drop)
			}
			for i := 0; i < a; i++ {
				if got := <-q; *got != i {
					t.Fatalf("FIFO mismatch: %d != %d", *got, i)
				}
				consumed++
			}
			if len(q) != 0 {
				t.Fatal("unexpected residual")
			}
			cases++
			offered += count
			admitted += a
			rejected += drop
		}
	}
	if cases != 42 || admitted != consumed || offered != admitted+rejected {
		t.Fatal("aggregate conservation")
	}
	t.Logf("AD_FIXTURE queue cases=%d offered=%d admitted=%d rejected=%d consumed=%d", cases, offered, admitted, rejected, consumed)
}

func TestReplayWindowIntervention(t *testing.T) {
	var cases, detected, mutation, enforcement int
	for _, gap := range []int64{0, 1, 99, 100, 101, 999, 1000, 1001} {
		for _, window := range []int64{100, 1000} {
			for _, sameTrace := range []bool{false, true} {
				trace := "different"
				if sameTrace {
					trace = "fixture"
				}
				events := []detectionengineering.Event{
					{ID: "a", Timestamp: 10000, TraceID: "fixture", EventType: "openat", Comm: "bash"},
					{ID: "b", Timestamp: 10000 + gap, TraceID: trace, EventType: "connect", Comm: "curl"},
				}
				snapshot := append([]detectionengineering.Event(nil), events...)
				rule := detectionengineering.Rule{
					ID: "fixture", Scope: "trace", WindowMS: window, MinSignals: 2,
					Signals: []detectionengineering.Signal{{Field: "eventType", Value: "openat"}, {Field: "comm", Value: "curl"}},
				}
				report := detectionengineering.Replay(rule, events, nil)
				if !report.Valid {
					t.Fatalf("fixture rejected: %+v", report)
				}
				want := sameTrace && gap <= window
				if (report.DetectedScopes == 1) != want {
					t.Fatalf("replay oracle: gap=%d window=%d sameTrace=%t report=%+v", gap, window, sameTrace, report)
				}
				if want {
					detected++
				}
				if !reflect.DeepEqual(events, snapshot) {
					mutation++
				}
				if report.EnforcementApplied {
					enforcement++
				}
				cases++
			}
		}
	}
	if cases != 32 || mutation != 0 || enforcement != 0 {
		t.Fatal(fmt.Sprintf("replay invariant: cases=%d mutation=%d enforcement=%d", cases, mutation, enforcement))
	}
	t.Logf("AD_FIXTURE replay cases=%d detected=%d input_mutations=%d enforcement_claims=%d", cases, detected, mutation, enforcement)
}
