package app

import (
	"agent-ebpf-filter/app/platform"
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
type DomainForwardRoute = core.DomainForwardRoute
type DomainForwardProxySettings = core.DomainForwardProxySettings

// ── Constants ────────────────────────────────────────────────────────────────

const udsPath = core.UDSPATH
const ebpfPinRoot = platform.EBPFPinRoot
const ebpfPinMapsDir = platform.EBPFPinMapsDir
const ebpfPinLinksDir = platform.EBPFPinLinksDir

const HookTypeNative = core.HookTypeNative
const HookTypeWrapper = core.HookTypeWrapper
const ConfigFormatJSON = core.ConfigFormatJSON
const ConfigFormatTOML = core.ConfigFormatTOML

// ModelType constants (re-exported from core)
const (
	ModelRandomForest       = core.ModelRandomForest
	ModelKNN                = core.ModelKNN
	ModelLogisticRegression = core.ModelLogisticRegression
	ModelNaiveBayes         = core.ModelNaiveBayes
	ModelNearestCentroid    = core.ModelNearestCentroid
	ModelExtraTrees         = core.ModelExtraTrees
	ModelAdaBoost           = core.ModelAdaBoost
	ModelSVM                = core.ModelSVM
	ModelRidge              = core.ModelRidge
	ModelPerceptron         = core.ModelPerceptron
	ModelPassiveAggressive  = core.ModelPassiveAggressive
	ModelEnsemble           = core.ModelEnsemble
	ModelAdditiveAttention  = core.ModelAdditiveAttention

	// Advanced attention mechanisms
	ModelScaledDotProductAttention = core.ModelScaledDotProductAttention
	ModelMultiHeadAttention        = core.ModelMultiHeadAttention
	ModelRWKVAttention             = core.ModelRWKVAttention
	ModelMambaAttention            = core.ModelMambaAttention

	// Attention-enhanced model variants
	ModelRandomForestScaledDotProduct = core.ModelRandomForestScaledDotProduct
	ModelLogisticScaledDotProduct     = core.ModelLogisticScaledDotProduct
	ModelKNNScaledDotProduct          = core.ModelKNNScaledDotProduct
	ModelRandomForestMultiHead        = core.ModelRandomForestMultiHead
	ModelLogisticMultiHead            = core.ModelLogisticMultiHead
	ModelKNNMultiHead                 = core.ModelKNNMultiHead
	ModelRandomForestRWKV             = core.ModelRandomForestRWKV
	ModelLogisticRWKV                 = core.ModelLogisticRWKV
	ModelKNNRWKV                      = core.ModelKNNRWKV
	ModelRandomForestMamba            = core.ModelRandomForestMamba
	ModelLogisticMamba                = core.ModelLogisticMamba
	ModelKNNMamba                     = core.ModelKNNMamba

	ModelRandomForestFast    = core.ModelRandomForestFast
	ModelRandomForestShallow = core.ModelRandomForestShallow
	ModelRandomForestStable  = core.ModelRandomForestStable
	ModelRandomForestDeep    = core.ModelRandomForestDeep
	ModelRandomForestWide    = core.ModelRandomForestWide
	ModelRandomForestAttn    = core.ModelRandomForestAttn

	ModelExtraTreesFast = core.ModelExtraTreesFast
	ModelExtraTreesDeep = core.ModelExtraTreesDeep
	ModelExtraTreesWide = core.ModelExtraTreesWide

	ModelLogisticFast       = core.ModelLogisticFast
	ModelLogisticNone       = core.ModelLogisticNone
	ModelLogisticL1         = core.ModelLogisticL1
	ModelLogisticBalanced   = core.ModelLogisticBalanced
	ModelLogisticL1Balanced = core.ModelLogisticL1Balanced
	ModelLogisticAttn       = core.ModelLogisticAttn

	ModelSVMLong     = core.ModelSVMLong
	ModelSVMBalanced = core.ModelSVMBalanced

	ModelPerceptronLong     = core.ModelPerceptronLong
	ModelPerceptronBalanced = core.ModelPerceptronBalanced

	ModelPassiveAggressiveLong     = core.ModelPassiveAggressiveLong
	ModelPassiveAggressiveBalanced = core.ModelPassiveAggressiveBalanced

	ModelKNNManhattan = core.ModelKNNManhattan
	ModelKNNCosine    = core.ModelKNNCosine
	ModelKNNDistance  = core.ModelKNNDistance
	ModelKNNAttn      = core.ModelKNNAttn

	ModelNearestCentroidBalanced  = core.ModelNearestCentroidBalanced
	ModelNearestCentroidCosine    = core.ModelNearestCentroidCosine
	ModelNearestCentroidManhattan = core.ModelNearestCentroidManhattan

	ModelNaiveBayesBalanced = core.ModelNaiveBayesBalanced

	ModelEnsembleSoft    = core.ModelEnsembleSoft
	ModelEnsembleHard    = core.ModelEnsembleHard
	ModelEnsembleStacked = core.ModelEnsembleStacked

	ModelRidgeLight  = core.ModelRidgeLight
	ModelRidgeStrong = core.ModelRidgeStrong

	ModelAdaBoostFast  = core.ModelAdaBoostFast
	ModelAdaBoostLarge = core.ModelAdaBoostLarge
)
// ── Global variables ─────────────────────────────────────────────────────────

var (
	trackerMaps trackerMapSet

	tagsMu      sync.RWMutex
	tagMap             = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "System Pkg", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security", 9: "Shell", 10: "Language Pkg", 11: "Container CLI", 12: "Agent CLI"}
	tagNameToID        = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "System Pkg": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8, "Shell": 9, "Language Pkg": 10, "Container CLI": 11, "Agent CLI": 12}
	nextTagID   uint32 = 13

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)

	disabledCommsMu sync.RWMutex
	disabledComms   = make(map[string]struct{})

	disabledEventTypesMu sync.RWMutex
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
