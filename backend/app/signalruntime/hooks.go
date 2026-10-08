package signalruntime

// Runtime data sources injected by the app layer at bootstrap. Defaults are
// inert so unwired callers degrade to zero-value settings and empty event
// lists instead of panicking.

import (
	"context"

	"agent-ebpf-filter/core"
)

var (
	// SnapshotSettingsHook returns the live runtime settings.
	SnapshotSettingsHook = func() core.RuntimeSettings { return core.RuntimeSettings{} }

	// SignalSettingsHook returns only the signal-processing section. The
	// per-event gates use it so they never copy or normalise the whole
	// settings struct on the hot path.
	SignalSettingsHook = func() core.SignalProcessingSettings { return SnapshotSettingsHook().SignalProcessing }

	// RecentEventsContextHook mirrors runtimeState.RecentEventsContext.
	RecentEventsContextHook = func(ctx context.Context, limit int) ([]CapturedEventRecord, string, error) {
		return nil, "", nil
	}
)
