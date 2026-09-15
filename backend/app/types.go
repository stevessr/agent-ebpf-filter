package app

import (
	"agent-ebpf-filter/core"
	"log"
	"os/user"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// ---- moved from backend/zz_merged_backend.go section types.go ----

// ── Type aliases to core package ─────────────────────────────────────────────

type bpfEvent = core.BpfEvent
type WrapperRule = core.WrapperRule
type HookType = core.HookType
type ConfigFormat = core.ConfigFormat
type HookDef = core.HookDef
type gpuInfo = core.GpuInfo
type vmFaultCounters = core.VmFaultCounters
type kiroHookState = core.KiroHookState
type FilePreviewResponse = core.FilePreviewResponse
type trackerMapSet = core.TrackerMapSet
type ModelType = core.ModelType
type MLConfig = core.MLConfig
type KernelRiskFeedbackSettings = core.KernelRiskFeedbackSettings
type LoopDetectionSettings = core.LoopDetectionSettings
type ResearchProcessingSettings = core.ResearchProcessingSettings
type SignalCondition = core.SignalCondition
type SignalRule = core.SignalRule
type SelectedProgramSignalLog = core.SelectedProgramSignalLog
type SignalProcessingSettings = core.SignalProcessingSettings
type DomainForwardRoute = core.DomainForwardRoute
type DomainForwardProxySettings = core.DomainForwardProxySettings

// ── Global variables ─────────────────────────────────────────────────────────

var (
	trackerMaps trackerMapSet

	tagsMu      sync.RWMutex
	tagMap             = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "System Pkg", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security", 9: "Shell", 10: "Language Pkg", 11: "Container CLI", 12: "Agent CLI"}
	tagNameToID        = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "System Pkg": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8, "Shell": 9, "Language Pkg": 10, "Container CLI": 11, "Agent CLI": 12}
	nextTagID   uint32 = 13

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)

	disabledCommsMu filterPublishingRWMutex
	disabledComms   = make(map[string]struct{})

	disabledEventTypesMu filterPublishingRWMutex
	disabledEventTypes   = make(map[uint32]struct{})

	nvmlInitialized bool

	// For non-NVIDIA GPU tracking (Intel/AMD via fdinfo)
	fdinfoHistory   = make(map[string]uint64) // pid:fd -> last_engine_ns
	fdinfoHistoryMu sync.RWMutex
	fdinfoTime      time.Time

	sudoUser          *user.User
	sudoUserHomeCache string
)

func init() {
	if ret := nvml.Init(); ret == nvml.SUCCESS {
		nvmlInitialized = true
	} else {
		log.Printf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
}

// availableHooks is a local copy of core.AvailableHooks for path resolution.
var availableHooks = core.AvailableHooks
