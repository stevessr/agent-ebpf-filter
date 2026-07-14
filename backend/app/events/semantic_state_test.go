package events

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestBoundedSemanticStateMapEvictsLeastRecentlyUsed(t *testing.T) {
	state := newBoundedSemanticStateMap[int](2)
	state.Set("first", 1)
	state.Set("second", 2)

	if value, ok := state.Get("first"); !ok || value != 1 {
		t.Fatalf("Get(first) = %d, %v; want 1, true", value, ok)
	}
	if evicted := state.Set("third", 3); !evicted {
		t.Fatal("Set(third) did not report a capacity eviction")
	}
	if _, ok := state.Get("second"); ok {
		t.Fatal("least-recently-used entry was retained")
	}
	if value, ok := state.Get("first"); !ok || value != 1 {
		t.Fatalf("recently read entry was evicted: %d, %v", value, ok)
	}
	if value, ok := state.Get("third"); !ok || value != 3 {
		t.Fatalf("new entry missing: %d, %v", value, ok)
	}
	if got := state.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}

func TestSemanticAlertStateSkipsUnrelatedAgenticEvents(t *testing.T) {
	state := NewSemanticAlertState()
	event := &pb.Event{
		Type:       "execve",
		ToolCallId: "tool-call-unrelated",
		Comm:       "git",
	}

	if target, reason, alert := state.ObserveAgenticResourceLoop(event, time.Now().UTC()); target != "" || reason != "" || alert {
		t.Fatalf("unrelated event produced observation result: target=%q reason=%q alert=%v", target, reason, alert)
	}
	status := state.Status()
	if status.AgenticLoopWindows != 0 || status.Entries != 0 {
		t.Fatalf("unrelated event allocated semantic state: %+v", status)
	}
}

func TestSemanticAlertStateEnforcesContextCapacity(t *testing.T) {
	state := NewSemanticAlertState()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i <= SemanticStateMaxContextEntries; i++ {
		state.RememberSecret(&pb.Event{ToolCallId: fmt.Sprintf("context-%05d", i)}, "secret", now)
	}

	status := state.Status()
	if status.RecentSecrets != SemanticStateMaxContextEntries {
		t.Fatalf("recent secret entries = %d, want %d", status.RecentSecrets, SemanticStateMaxContextEntries)
	}
	if status.Entries > status.MaxEntries {
		t.Fatalf("semantic entries exceeded combined capacity: %+v", status)
	}
	if status.CapacityEvictionsTotal != 1 {
		t.Fatalf("capacity evictions = %d, want 1", status.CapacityEvictionsTotal)
	}
	if _, ok := state.RecentSecretTarget(&pb.Event{ToolCallId: "context-00000"}, now); ok {
		t.Fatal("oldest semantic context was not evicted")
	}
	if target, ok := state.RecentSecretTarget(&pb.Event{ToolCallId: fmt.Sprintf("context-%05d", SemanticStateMaxContextEntries)}, now); !ok || target != "secret" {
		t.Fatalf("newest semantic context = %q, %v; want secret, true", target, ok)
	}
}

func TestSemanticStateBoundsOversizedIdentifiersAndValues(t *testing.T) {
	contextA := strings.Repeat("a", SemanticStateMaxContextBytes*4) + "-one"
	contextB := strings.Repeat("a", SemanticStateMaxContextBytes*4) + "-two"
	keyA, truncatedA := semanticAlertContextKeyBounded(&pb.Event{ToolCallId: contextA})
	keyARepeat, truncatedARepeat := semanticAlertContextKeyBounded(&pb.Event{ToolCallId: contextA})
	keyB, truncatedB := semanticAlertContextKeyBounded(&pb.Event{ToolCallId: contextB})
	if !truncatedA || !truncatedARepeat || !truncatedB {
		t.Fatal("oversized context identifiers were not marked as bounded")
	}
	if len(keyA) > SemanticStateMaxContextBytes || keyA != keyARepeat || keyA == keyB {
		t.Fatalf("bounded context keys are not stable and distinct: len=%d equal-repeat=%v equal-other=%v", len(keyA), keyA == keyARepeat, keyA == keyB)
	}

	pathA, pathTruncatedA := normalizeSemanticPath(strings.Repeat("p", SemanticStateMaxPathBytes*2)+"-one", "/workspace")
	pathB, pathTruncatedB := normalizeSemanticPath(strings.Repeat("p", SemanticStateMaxPathBytes*2)+"-two", "/workspace")
	if !pathTruncatedA || !pathTruncatedB || len(pathA) > SemanticStateMaxPathBytes || pathA == pathB {
		t.Fatalf("oversized paths were not bounded distinctly: len=%d truncated=%v/%v equal=%v", len(pathA), pathTruncatedA, pathTruncatedB, pathA == pathB)
	}

	state := NewSemanticAlertState()
	now := time.Now().UTC()
	state.RememberSecret(
		&pb.Event{ToolCallId: contextA},
		strings.Repeat("s", SemanticStateMaxValueBytes*4),
		now,
	)
	state.mu.Lock()
	observation, ok := state.recentSecrets.Get(keyA)
	state.mu.Unlock()
	if !ok || len(observation.Target) > SemanticStateMaxValueBytes {
		t.Fatalf("stored target was not bounded: found=%v len=%d", ok, len(observation.Target))
	}
	if status := state.Status(); status.TruncatedStateValuesTotal < 2 {
		t.Fatalf("truncation counter = %d, want at least context and target", status.TruncatedStateValuesTotal)
	}
}

func TestSemanticAlertStateEvictsExpiredEntriesAcrossKinds(t *testing.T) {
	state := NewSemanticAlertState()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-time.Minute)
	contextEvent := &pb.Event{ToolCallId: "old-context"}

	state.RememberSecret(contextEvent, "/tmp/secret", stale)
	state.RememberExecutable(contextEvent, "/tmp/tool", "0755", stale)
	state.IncrementForkCount(contextEvent, stale)
	state.ObserveAgenticResourceLoop(&pb.Event{
		Type:       "native_hook",
		ToolCallId: "old-loop",
		ExtraInfo:  "prompt_digest=old-prompt",
	}, stale)
	state.ObserveMultiAgentFileContention(&pb.Event{
		Type:       "write",
		Path:       "/workspace/old.txt",
		AgentRunId: "old-agent",
	}, stale)

	before := state.Status()
	if before.Entries != 5 {
		t.Fatalf("semantic entries before GC = %d, want 5: %+v", before.Entries, before)
	}
	after := state.EvictExpired(now)
	if after.Entries != 0 || after.ExpiredEvictionsTotal != 5 {
		t.Fatalf("semantic GC did not evict all stale kinds: %+v", after)
	}
	if !after.LastSweepAt.Equal(now) {
		t.Fatalf("last sweep = %s, want %s", after.LastSweepAt, now)
	}
}

func TestExtraInfoFieldBounded(t *testing.T) {
	value, oversized := extraInfoFieldBounded("x=1,\u2003prompt_digest=abc123,foo=bar", "prompt_digest", SemanticPromptDigestMaxBytes)
	if value != "abc123" || oversized {
		t.Fatalf("unicode/comma metadata parse = %q, %v; want abc123, false", value, oversized)
	}

	value, oversized = extraInfoFieldBounded(
		"prompt_digest="+strings.Repeat("x", SemanticPromptDigestMaxBytes+1),
		"prompt_digest",
		SemanticPromptDigestMaxBytes,
	)
	if value != "" || !oversized {
		t.Fatalf("oversized value parse = %q, %v; want empty, true", value, oversized)
	}

	value, oversized = extraInfoFieldBounded(
		strings.Repeat("x", SemanticExtraInfoMaxScanBytes)+" prompt_digest=too-late",
		"prompt_digest",
		SemanticPromptDigestMaxBytes,
	)
	if value != "" || !oversized {
		t.Fatalf("scan-limited parse = %q, %v; want empty, true", value, oversized)
	}
}

func TestOversizedPromptMetadataIsCountedWithoutAllocatingWindow(t *testing.T) {
	state := NewSemanticAlertState()
	state.ObserveAgenticResourceLoop(&pb.Event{
		Type:       "native_hook",
		ToolCallId: "oversized-prompt",
		ExtraInfo:  "prompt_digest=" + strings.Repeat("x", SemanticPromptDigestMaxBytes+1),
	}, time.Now().UTC())

	status := state.Status()
	if status.AgenticLoopWindows != 0 || status.Entries != 0 {
		t.Fatalf("rejected prompt metadata allocated state: %+v", status)
	}
	if status.IgnoredOversizedMetadataTotal != 1 {
		t.Fatalf("ignored metadata count = %d, want 1", status.IgnoredOversizedMetadataTotal)
	}
}

func TestIsAPILikeNetworkEventHandlesFieldsWithoutJoining(t *testing.T) {
	if !isAPILikeNetworkEvent(&pb.Event{
		Type:    "network_connect",
		Sni:     "API.OpenAI.Example",
		DstPort: 443,
	}) {
		t.Fatal("mixed-case API SNI was not classified")
	}
	if isAPILikeNetworkEvent(&pb.Event{
		Type:        "network_connect",
		NetEndpoint: "127.0.0.1:443",
		Sni:         "api.openai.example",
		DstPort:     443,
	}) {
		t.Fatal("event containing a localhost target was classified as API egress")
	}
}

func TestSemanticAlertStateConcurrentAccess(t *testing.T) {
	state := NewSemanticAlertState()
	now := time.Now().UTC()
	const workers = 8
	const observationsPerWorker = 256

	var workersDone sync.WaitGroup
	workersDone.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func(worker int) {
			defer workersDone.Done()
			for observation := 0; observation < observationsPerWorker; observation++ {
				event := &pb.Event{ToolCallId: fmt.Sprintf("worker-%d-observation-%d", worker, observation)}
				state.RememberSecret(event, "/tmp/secret", now)
				state.RecentSecretTarget(event, now)
				state.IncrementForkCount(event, now)
				if observation%16 == 0 {
					state.Status()
				}
			}
		}(worker)
	}
	workersDone.Wait()

	status := state.Status()
	wantEntriesPerKind := workers * observationsPerWorker
	if status.RecentSecrets != wantEntriesPerKind || status.ForkWindows != wantEntriesPerKind {
		t.Fatalf("concurrent semantic state lost entries: %+v", status)
	}
	if status.Entries > status.MaxEntries {
		t.Fatalf("concurrent semantic state exceeded capacity: %+v", status)
	}
}
