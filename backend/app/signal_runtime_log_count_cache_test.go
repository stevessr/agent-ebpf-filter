package app

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestSignalProgramLogFrameCountCacheAdvancesAndEvictsLRU(t *testing.T) {
	cache := newSignalProgramLogFrameCountCache(2)
	one := signalProgramLogFileSignature{size: 10, modifiedNs: 1}
	two := signalProgramLogFileSignature{size: 20, modifiedNs: 2}
	three := signalProgramLogFileSignature{size: 30, modifiedNs: 3}

	cache.Remember("one", one, 2, "")
	cache.Remember("two", two, 4, "")
	if frames, _, ok := cache.Lookup("one", one); !ok || frames != 2 {
		t.Fatalf("Lookup(one) = %d, %v; want 2, true", frames, ok)
	}
	cache.Remember("three", three, 6, "")
	if _, _, ok := cache.Lookup("two", two); ok {
		t.Fatal("least-recently-used cache entry was not evicted")
	}

	after := signalProgramLogFileSignature{size: 15, modifiedNs: 4}
	cache.Advance("one", one, after)
	if frames, errorText, ok := cache.Lookup("one", after); !ok || frames != 3 || errorText != "" {
		t.Fatalf("advanced cache entry = %d, %q, %v; want 3, empty, true", frames, errorText, ok)
	}

	cache.Remember("broken", two, 1, "corrupt frame")
	brokenAfter := signalProgramLogFileSignature{size: 25, modifiedNs: 5}
	cache.Advance("broken", two, brokenAfter)
	if frames, errorText, ok := cache.Lookup("broken", brokenAfter); !ok || frames != 1 || errorText != "corrupt frame" {
		t.Fatalf("advanced broken cache entry = %d, %q, %v", frames, errorText, ok)
	}
}

func TestSignalProgramLogAppendMaintainsCachedFrameCount(t *testing.T) {
	tempDir := t.TempDir()
	oldRoot := signalProgramLogsRootPath
	oldCache := signalProgramLogFrameCountCacheStore
	signalProgramLogsRootPath = func() string { return tempDir }
	signalProgramLogFrameCountCacheStore = newSignalProgramLogFrameCountCache(4)
	t.Cleanup(func() {
		signalProgramLogsRootPath = oldRoot
		signalProgramLogFrameCountCacheStore = oldCache
	})

	selected := SelectedProgramSignalLog{Program: "codex", Enabled: true, Path: "codex.pb.gzlog"}
	for index := 0; index < 2; index++ {
		if err := appendCompressedProtoRecordForSelected(selected, wrapperspb.String("event")); err != nil {
			t.Fatalf("append frame %d: %v", index, err)
		}
	}
	path := filepath.Join(tempDir, selected.Path)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat signal program log: %v", err)
	}
	frames, errorText, ok := signalProgramLogFrameCountCacheStore.Lookup(path, signalProgramLogSignature(info))
	if !ok || frames != 2 || errorText != "" {
		t.Fatalf("cached append count = %d, %q, %v; want 2, empty, true", frames, errorText, ok)
	}

	statuses := selectedProgramLogStatuses(SignalProcessingSettings{SelectedPrograms: []SelectedProgramSignalLog{selected}})
	if len(statuses) != 1 || statuses[0].FrameCount != 2 || statuses[0].Error != "" {
		t.Fatalf("cached signal program log status = %+v", statuses)
	}
}
