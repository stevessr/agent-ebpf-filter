package app

import (
	"agent-ebpf-filter/core"
)

// ---- moved from backend/zz_merged_backend.go section statetypesruntime.go ----

// ── Type aliases to core package ─────────────────────────────────────────────

type RuntimeSettings = core.RuntimeSettings
type CapturedEventRecord = core.CapturedEventRecord
type eventArchive = core.EventArchive

// ── Constructors ─────────────────────────────────────────────────────────────

func newEventArchive(max int) *eventArchive {
	return core.NewEventArchive(max)
}

// ── Global state ─────────────────────────────────────────────────────────────

var (
	runtimeSettingsStore = newRuntimeState()
	capturedEventArchive = newEventArchive(1500)
)

// eventSchemaVersion is the canonical schema version for captured events.
// It was moved from context_event.go to app/events/ as EventSchemaVersion.
// This local const preserves backward compatibility for remaining callers.
const eventSchemaVersion = "event.v3"
