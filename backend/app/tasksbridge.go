package app

// Bridge aliases for value types hoisted into core (see core/shared_types.go)
// and for the task runtime extracted to package tasks.

import (
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/core"
)

type researchCount = core.ResearchCount
type DatasetQualitySummary = core.DatasetQualitySummary
type loopDetectionFinding = core.LoopDetectionFinding

type backendTaskRuntimeEntry = tasks.Entry
type backendTaskRuntimeStats = tasks.Stats
type backendTaskRuntimeSnapshot = tasks.Snapshot
type backendTaskRuntime = tasks.Runtime

var (
	newBackendTaskRuntime      = tasks.New
	newUnstartedTaskRuntime    = tasks.NewUnstarted
	newBackendTaskRuntimeEntry = tasks.NewEntry
	newBackendTaskPanicError   = tasks.NewPanicError
	errBackendTaskCanceled     = tasks.ErrCanceled
)
