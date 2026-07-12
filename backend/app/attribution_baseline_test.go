package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestToolBaselineStoreBoundsToolsAndSamples(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()

	for index := 0; index < toolBaselineMaxTools+20; index++ {
		store.Record(fmt.Sprintf("tool-%04d", index), "codex", "execve", "/tmp/tool")
	}
	store.mu.RLock()
	toolCount := len(store.samples)
	_, newestToolPresent := store.samples[fmt.Sprintf("tool-%04d", toolBaselineMaxTools+19)]
	store.mu.RUnlock()
	if toolCount != toolBaselineMaxTools {
		t.Fatalf("tool count = %d, want %d", toolCount, toolBaselineMaxTools)
	}
	if !newestToolPresent {
		t.Fatal("newest tool baseline was evicted")
	}

	perTool := newToolBaselineStore()
	for index := 0; index < toolBaselineMaxSamplesPerTool+20; index++ {
		perTool.Record("codex", fmt.Sprintf("comm-%04d", index), "execve", "/tmp/tool")
	}
	perTool.mu.RLock()
	samples := perTool.samples["codex"]
	sampleCount := len(samples)
	_, newestSamplePresent := samples[fmt.Sprintf("comm-%04d:execve", toolBaselineMaxSamplesPerTool+19)]
	perTool.mu.RUnlock()
	if sampleCount != toolBaselineMaxSamplesPerTool {
		t.Fatalf("sample count = %d, want %d", sampleCount, toolBaselineMaxSamplesPerTool)
	}
	if !newestSamplePresent {
		t.Fatal("newest tool sample was evicted")
	}
}

func TestToolBaselineStoreEvictsExpiredAndBoundsKeys(t *testing.T) {
	t.Parallel()
	store := newToolBaselineStore()
	longName := strings.Repeat("工", toolBaselineMaxNameRunes+20)
	store.Record(longName, longName, strings.Repeat("e", toolBaselineMaxEventRunes+20), strings.Repeat("/x", toolBaselineMaxPathRunes))

	store.mu.Lock()
	for _, samples := range store.samples {
		for _, sample := range samples {
			if len([]rune(sample.ToolName)) > toolBaselineMaxNameRunes || len([]rune(sample.Comm)) > toolBaselineMaxNameRunes || len([]rune(sample.EventType)) > toolBaselineMaxEventRunes || len([]rune(sample.Path)) > toolBaselineMaxPathRunes {
				store.mu.Unlock()
				t.Fatal("stored baseline values exceeded configured bounds")
			}
			sample.LastSeen = time.Now().UTC().Add(-toolBaselineTTL - time.Minute)
		}
	}
	store.mu.Unlock()

	if _, drift := store.detectDrift(longName, "new-comm", "openat"); drift {
		t.Fatal("expired baseline contributed to drift detection")
	}
	store.mu.RLock()
	remaining := len(store.samples)
	store.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("expired baseline tools remaining = %d, want 0", remaining)
	}
}
