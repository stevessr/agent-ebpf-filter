// Package signalruntime implements the single-consumer signal processing
// worker, its HTTP handlers, and program-log persistence. The app layer
// bridges legacy identifiers onto these exports (see app/signalbridge.go).
package signalruntime

import "agent-ebpf-filter/core"

type (
	CapturedEventRecord      = core.CapturedEventRecord
	RuntimeSettings          = core.RuntimeSettings
	SignalProcessingSettings = core.SignalProcessingSettings
	SignalRule               = core.SignalRule
	SignalCondition          = core.SignalCondition
	SelectedProgramSignalLog = core.SelectedProgramSignalLog
)
