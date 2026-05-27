package main

import (
	"agent-ebpf-filter/core"
)

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
