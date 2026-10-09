package app

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestToolBaselineStoreBoundsToolsAndSamples(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store := newToolBaselineStore()

	for index := 0; index < toolBaselineMaxTools+20; index++ {
		store.observeAt(fmt.Sprintf("tool-%04d", index), "codex", "execve", now)
	}
	status := store.Status()
	store.mu.Lock()
	_, newestToolPresent := store.tools[fmt.Sprintf("tool-%04d", toolBaselineMaxTools+19)]
	store.mu.Unlock()
	if status.Tools != toolBaselineMaxTools || status.Samples != toolBaselineMaxTools {
		t.Fatalf("bounded tool status = %+v", status)
	}
	if status.CapacityEvictionsTotal != 20 {
		t.Fatalf("tool capacity evictions = %d, want 20", status.CapacityEvictionsTotal)
	}
	if !newestToolPresent {
		t.Fatal("newest tool baseline was evicted")
	}
	store.observeAt("tool-0020", "codex", "execve", now.Add(time.Second))
	store.observeAt(fmt.Sprintf("tool-%04d", toolBaselineMaxTools+20), "codex", "execve", now.Add(2*time.Second))
	store.mu.Lock()
	_, touchedToolPresent := store.tools["tool-0020"]
	_, nextOldestToolPresent := store.tools["tool-0021"]
	store.mu.Unlock()
	if !touchedToolPresent || nextOldestToolPresent {
		t.Fatalf("tool LRU did not retain a touched entry: touched=%v next-oldest=%v", touchedToolPresent, nextOldestToolPresent)
	}

	perTool := newToolBaselineStore()
	for index := 0; index < toolBaselineMaxSamplesPerTool+20; index++ {
		perTool.observeAt("codex", fmt.Sprintf("comm-%04d", index), "execve", now)
	}
	perToolStatus := perTool.Status()
	newestKey := toolBaselineBehaviorKey{
		Comm:      fmt.Sprintf("comm-%04d", toolBaselineMaxSamplesPerTool+19),
		EventType: "execve",
	}
	perTool.mu.Lock()
	_, newestSamplePresent := perTool.tools["codex"].samples[newestKey]
	perTool.mu.Unlock()
	if perToolStatus.Tools != 1 || perToolStatus.Samples != toolBaselineMaxSamplesPerTool {
		t.Fatalf("bounded per-tool status = %+v", perToolStatus)
	}
	if perToolStatus.CapacityEvictionsTotal != 20 {
		t.Fatalf("sample capacity evictions = %d, want 20", perToolStatus.CapacityEvictionsTotal)
	}
	if !newestSamplePresent {
		t.Fatal("newest tool sample was evicted")
	}
	perTool.observeAt("codex", "comm-0020", "execve", now.Add(time.Second))
	perTool.observeAt("codex", fmt.Sprintf("comm-%04d", toolBaselineMaxSamplesPerTool+20), "execve", now.Add(2*time.Second))
	perTool.mu.Lock()
	_, touchedSamplePresent := perTool.tools["codex"].samples[toolBaselineBehaviorKey{Comm: "comm-0020", EventType: "execve"}]
	_, nextOldestSamplePresent := perTool.tools["codex"].samples[toolBaselineBehaviorKey{Comm: "comm-0021", EventType: "execve"}]
	perTool.mu.Unlock()
	if !touchedSamplePresent || nextOldestSamplePresent {
		t.Fatalf("sample LRU did not retain a touched entry: touched=%v next-oldest=%v", touchedSamplePresent, nextOldestSamplePresent)
	}
}

func TestToolBaselineStoreEvictsExpiredAndBoundsKeys(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	longName := strings.Repeat("工", toolBaselineMaxNameRunes+20)
	longEvent := strings.Repeat("e", toolBaselineMaxEventRunes+20)
	store.observeAt(longName, longName, longEvent, now.Add(-toolBaselineTTL-time.Minute))

	store.mu.Lock()
	for toolName, tool := range store.tools {
		for key := range tool.samples {
			if len([]rune(toolName)) > toolBaselineMaxNameRunes || len([]rune(key.Comm)) > toolBaselineMaxNameRunes || len([]rune(key.EventType)) > toolBaselineMaxEventRunes {
				store.mu.Unlock()
				t.Fatal("stored baseline values exceeded configured bounds")
			}
		}
	}
	store.mu.Unlock()

	status := store.EvictExpired(now)
	if status.Tools != 0 || status.Samples != 0 || status.ExpiredEvictionsTotal != 1 {
		t.Fatalf("expired baseline remained after GC: %+v", status)
	}
	if status.TruncatedStateValuesTotal != 3 {
		t.Fatalf("truncated baseline values = %d, want 3", status.TruncatedStateValuesTotal)
	}
	if !status.LastSweepAt.Equal(now) {
		t.Fatalf("last sweep = %s, want %s", status.LastSweepAt, now)
	}
}

func TestBoundToolBaselineValueIsStableAndDistinct(t *testing.T) {
	prefix := strings.Repeat("a", toolBaselineMaxNameRunes*2)
	first, firstTruncated := boundToolBaselineValue(prefix+"-first", toolBaselineMaxNameRunes)
	firstRepeat, repeatTruncated := boundToolBaselineValue(prefix+"-first", toolBaselineMaxNameRunes)
	second, secondTruncated := boundToolBaselineValue(prefix+"-second", toolBaselineMaxNameRunes)
	if !firstTruncated || !repeatTruncated || !secondTruncated {
		t.Fatal("oversized baseline identifiers were not marked as bounded")
	}
	if first != firstRepeat || first == second {
		t.Fatalf("bounded identifiers are not stable/distinct: repeat=%v other=%v", first == firstRepeat, first == second)
	}
	if len([]rune(first)) > toolBaselineMaxNameRunes {
		t.Fatalf("bounded identifier has %d runes, want <= %d", len([]rune(first)), toolBaselineMaxNameRunes)
	}
}

func TestToolBaselineObserveDetectsThenRecordsDrift(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for index, behavior := range []toolBaselineBehaviorKey{
		{Comm: "git", EventType: "execve"},
		{Comm: "rg", EventType: "openat"},
		{Comm: "cat", EventType: "read"},
	} {
		if reason, drift := store.observeAt("review", behavior.Comm, behavior.EventType, now.Add(time.Duration(index)*time.Second)); drift {
			t.Fatalf("baseline warm-up produced drift: %q", reason)
		}
	}
	for observation := 3; observation < toolBaselineMinObservations; observation++ {
		if reason, drift := store.observeAt("review", "git", "execve", now.Add(time.Duration(observation)*time.Second)); drift {
			t.Fatalf("known behavior warm-up produced drift: %q", reason)
		}
	}

	reason, drift := store.observeAt("review", "curl", "execve", now.Add(toolBaselineMinObservations*time.Second))
	if !drift || !strings.Contains(reason, "curl/execve") {
		t.Fatalf("unexpected behavior drift = %v, reason = %q", drift, reason)
	}
	if reason, drift = store.observeAt("review", "curl", "execve", now.Add((toolBaselineMinObservations+1)*time.Second)); drift {
		t.Fatalf("recorded behavior alerted twice: %q", reason)
	}

	status := store.Status()
	if status.ObservationsTotal != toolBaselineMinObservations+2 || status.DriftsTotal != 1 || status.Samples != 4 {
		t.Fatalf("baseline observe counters mismatch: %+v", status)
	}
	store.mu.Lock()
	sample := store.tools["review"].samples[toolBaselineBehaviorKey{Comm: "curl", EventType: "execve"}]
	store.mu.Unlock()
	if sample == nil || sample.Count != 2 {
		t.Fatalf("recorded drift sample = %+v, want count 2", sample)
	}
}

func TestToolBaselineDoesNotDriftBeforeMaturity(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for index, behavior := range []toolBaselineBehaviorKey{
		{Comm: "git", EventType: "execve"},
		{Comm: "rg", EventType: "openat"},
		{Comm: "cat", EventType: "read"},
	} {
		store.observeAt("review", behavior.Comm, behavior.EventType, now.Add(time.Duration(index)*time.Second))
	}
	if reason, drift := store.observeAt("review", "sed", "write", now.Add(3*time.Second)); drift {
		t.Fatalf("immature baseline produced drift: %q", reason)
	}
	if status := store.Status(); status.Samples != 4 || status.DriftsTotal != 0 {
		t.Fatalf("immature behavior was not learned cleanly: %+v", status)
	}
}

func TestToolBaselineConcurrentNewBehaviorAlertsOnce(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for index, behavior := range []toolBaselineBehaviorKey{
		{Comm: "git", EventType: "execve"},
		{Comm: "rg", EventType: "openat"},
		{Comm: "cat", EventType: "read"},
	} {
		store.observeAt("review", behavior.Comm, behavior.EventType, now.Add(time.Duration(index)*time.Second))
	}
	for observation := 3; observation < toolBaselineMinObservations; observation++ {
		store.observeAt("review", "git", "execve", now.Add(time.Duration(observation)*time.Second))
	}

	const workers = 32
	var drifts atomic.Int32
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			if _, drift := store.observeAt("review", "curl", "execve", now.Add(time.Minute)); drift {
				drifts.Add(1)
			}
		}()
	}
	wait.Wait()

	if got := drifts.Load(); got != 1 {
		t.Fatalf("concurrent unseen behavior emitted %d drifts, want 1", got)
	}
	status := store.Status()
	if status.DriftsTotal != 1 || status.ObservationsTotal != workers+toolBaselineMinObservations {
		t.Fatalf("concurrent baseline status mismatch: %+v", status)
	}
}

func TestToolBaselineNovelBehaviorRequiresTemporalSupport(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	start := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	for i, behavior := range []toolBaselineBehaviorKey{
		{Comm: "git", EventType: "execve"},
		{Comm: "rg", EventType: "openat"},
		{Comm: "cat", EventType: "read"},
	} {
		store.observeAt("analysis", behavior.Comm, behavior.EventType, start.Add(time.Duration(i)*time.Second))
	}
	for i := 3; i < toolBaselineMinObservations; i++ {
		store.observeAt("analysis", "git", "execve", start.Add(time.Duration(i)*time.Second))
	}
	first := start.Add(toolBaselineMinObservations * time.Second)
	if reason, alert := store.observeAt("analysis", "jq", "read", first); alert {
		t.Fatalf("one unseen benign read should not alert: %q", reason)
	}
	if reason, alert := store.observeAt("analysis", "jq", "read", first.Add(100*time.Millisecond)); alert {
		t.Fatalf("burst duplicate should not confirm drift: %q", reason)
	}
	reason, alert := store.observeAt("analysis", "jq", "read", first.Add(toolBaselineConfirmationGap))
	if !alert || !strings.Contains(reason, "repeated novel behavior") {
		t.Fatalf("time-separated observations must confirm drift, got %v %q", alert, reason)
	}
	if reason, alert = store.observeAt("analysis", "jq", "read", first.Add(3*time.Second)); alert {
		t.Fatalf("confirmed novelty should only alert once: %q", reason)
	}
	status := store.Status()
	if status.DriftsTotal != 1 || status.ObservationsTotal != toolBaselineMinObservations+4 {
		t.Fatalf("model baseline counters disagree: %+v", status)
	}
}

func TestToolBaselineHighSignalNoveltyAndConcurrentBurst(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	start := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	for i, b := range []toolBaselineBehaviorKey{
		{Comm: "git", EventType: "execve"},
		{Comm: "rg", EventType: "openat"},
		{Comm: "cat", EventType: "read"},
	} {
		store.observeAt("review", b.Comm, b.EventType, start.Add(time.Duration(i)*time.Second))
	}
	for i := 3; i < toolBaselineMinObservations; i++ {
		store.observeAt("review", "git", "execve", start.Add(time.Duration(i)*time.Second))
	}
	if reason, alert := store.observeAt("review", "curl", "execve", start.Add(20*time.Second)); !alert ||
		!strings.Contains(reason, "high-signal") {
		t.Fatalf("suspicious new transport should alert immediately: %v %q", alert, reason)
	}
	const workers = 24
	var wg sync.WaitGroup
	var burstDrifts atomic.Int32
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, drift := store.observeAt("review", "jq", "read", start.Add(21*time.Second)); drift {
				burstDrifts.Add(1)
			}
		}()
	}
	wg.Wait()
	if burstDrifts.Load() != 0 {
		t.Fatalf("concurrent benign burst emitted %d false alerts", burstDrifts.Load())
	}
	if _, alert := store.observeAt("review", "jq", "read", start.Add(22*time.Second)); !alert {
		t.Fatal("subsequent time-separated behavior should confirm novelty")
	}
}

func TestToolBaselineRiskPriorIsNarrow(t *testing.T) {
	t.Parallel()
	for _, item := range []struct{ comm, event string; want bool }{
		{"curl", "execve", true},
		{"curl", "baseline_network", true},
		{"socat", "network_connect", true},
		{"fish", "write", false},
		{"git", "execve", false},
		{"node", "read", false},
		{"cursor", "network_connect", false},
	} {
		if got := highSignalToolBaselineBehavior(item.comm, item.event); got != item.want {
			t.Errorf("%s/%s high-signal=%v, want %v", item.comm, item.event, got, item.want)
		}
	}
}

func BenchmarkToolBaselineObserveKnownBehavior(b *testing.B) {
	store := newToolBaselineStore()
	for observation := 0; observation < toolBaselineMinObservations; observation++ {
		store.Observe("review", "git", "execve")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		store.Observe("review", "git", "execve")
	}
}
