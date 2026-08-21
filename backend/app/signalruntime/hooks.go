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

	// RecentEventsContextHook mirrors runtimeState.RecentEventsContext.
	RecentEventsContextHook = func(ctx context.Context, limit int) ([]CapturedEventRecord, string, error) {
		return nil, "", nil
	}
)
