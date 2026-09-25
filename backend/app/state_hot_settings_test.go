package app

import "testing"

func TestRuntimeHotSettingsSnapshotRefresh(t *testing.T) {
	state := &runtimeState{
		settings: RuntimeSettings{
			LoopDetection:      LoopDetectionSettings{Enabled: true, QueueSize: 17},
			ResearchProcessing: ResearchProcessingSettings{Enabled: true, QueueSize: 23},
			SignalProcessing:   SignalProcessingSettings{Enabled: true, QueueSize: 31},
		},
	}
	state.publishHotSettingsLocked()

	if got := state.LoopDetectionSettings(); !got.Enabled || got.QueueSize != 17 {
		t.Fatalf("unexpected loop detection hot settings: %+v", got)
	}
	if got := state.ResearchProcessingSettings(); !got.Enabled || got.QueueSize != 23 {
		t.Fatalf("unexpected research hot settings: %+v", got)
	}
	if got := state.SignalProcessingSettings(); !got.Enabled || got.QueueSize != 31 {
		t.Fatalf("unexpected signal hot settings: %+v", got)
	}

	// The published object is immutable: mutating the locked backing settings
	// does not change readers until the next explicit publication.
	state.settings.LoopDetection.Enabled = false
	state.settings.ResearchProcessing.Enabled = false
	state.settings.SignalProcessing.Enabled = false
	if got := state.LoopDetectionSettings(); !got.Enabled {
		t.Fatalf("published loop settings changed without refresh: %+v", got)
	}
	if got := state.ResearchProcessingSettings(); !got.Enabled {
		t.Fatalf("published research settings changed without refresh: %+v", got)
	}
	if got := state.SignalProcessingSettings(); !got.Enabled {
		t.Fatalf("published signal settings changed without refresh: %+v", got)
	}

	state.publishHotSettingsLocked()
	if state.LoopDetectionSettings().Enabled ||
		state.ResearchProcessingSettings().Enabled ||
		state.SignalProcessingSettings().Enabled {
		t.Fatal("hot settings were not refreshed")
	}
}

func TestRuntimeHotSettingsFallbackForLiteralState(t *testing.T) {
	state := &runtimeState{
		settings: RuntimeSettings{
		LoopDetection: LoopDetectionSettings{Enabled: true, QueueSize: 11},
		},
	}
	got := state.LoopDetectionSettings()
	if !got.Enabled || got.QueueSize != 11 {
		t.Fatalf("literal-state fallback mismatch: %+v", got)
	}
}
