package app

import (
	"time"
	"unsafe"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/types"
	"agent-ebpf-filter/core"
	netcore "agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/internal/protocoldetect"
)

// ── Bridge: AppProto from internal/protocoldetect ────────────────────────────
const (
	AppProtoTLS     = protocoldetect.AppProtoTLS
	AppProtoHTTP    = protocoldetect.AppProtoHTTP
	AppProtoSSH     = protocoldetect.AppProtoSSH
	AppProtoDNS     = protocoldetect.AppProtoDNS
	AppProtoQUIC    = protocoldetect.AppProtoQUIC
	AppProtoDHCP    = protocoldetect.AppProtoDHCP
	AppProtomDNS    = protocoldetect.AppProtomDNS
	AppProtoLLMNR   = protocoldetect.AppProtoLLMNR
	AppProtoSSDP    = protocoldetect.AppProtoSSDP
	AppProtoNTP     = protocoldetect.AppProtoNTP
	AppProtoSNMP    = protocoldetect.AppProtoSNMP
	AppProtoNetBIOS = protocoldetect.AppProtoNetBIOS
	AppProtoUnknown = protocoldetect.AppProtoUnknown
)

// ── Bridge: TCPState from internal/network ───────────────────────────────────
const (
	TCPStateUnknown     = netcore.TCPStateUnknown
	TCPStateEstablished = netcore.TCPStateEstablished
	TCPStateSynSent     = netcore.TCPStateSynSent
	TCPStateSynRecv     = netcore.TCPStateSynRecv
	TCPStateFinWait1    = netcore.TCPStateFinWait1
	TCPStateFinWait2    = netcore.TCPStateFinWait2
	TCPStateTimeWait    = netcore.TCPStateTimeWait
	TCPStateClose       = netcore.TCPStateClose
	TCPStateCloseWait   = netcore.TCPStateCloseWait
	TCPStateLastAck     = netcore.TCPStateLastAck
	TCPStateListen      = netcore.TCPStateListen
	TCPStateClosing     = netcore.TCPStateClosing
	TCPStateClosed      = netcore.TCPStateClosed
)

// ── Bridge: Plugin types from app/types ──────────────────────────────────────
const (
	PluginKindEBPF    = types.PluginKindEBPF
	PluginKindWebhook = types.PluginKindWebhook
	PluginKindCommand = types.PluginKindCommand
)
const (
	PluginAttachTracepoint = types.PluginAttachTracepoint
	PluginAttachKprobe     = types.PluginAttachKprobe
	PluginAttachKretprobe  = types.PluginAttachKretprobe
	PluginAttachLSM        = types.PluginAttachLSM
	PluginAttachNone       = types.PluginAttachNone
)

// ── Bridge: Feature flags from app/types ─────────────────────────────────────
const (
	FeatureShellSessions    = types.FeatureShellSessions
	FeatureSystemRun        = types.FeatureSystemRun
	FeatureHooks            = types.FeatureHooks
	FeaturePolicyManagement = types.FeaturePolicyManagement
	FeatureTLSCapture       = types.FeatureTLSCapture
	FeatureOTLP             = types.FeatureOTLP
	FeatureDomainForward    = types.FeatureDomainForward
	FeatureML               = types.FeatureML
	FeaturePlugins          = types.FeaturePlugins
	FeatureSandboxCgroup    = types.FeatureSandboxCgroup
	FeatureSandboxLSM       = types.FeatureSandboxLSM
	FeatureNetworkExport    = types.FeatureNetworkExport
	FeatureAgentSight       = types.FeatureAgentSight
)
const (
	FeatureDangerLow      = types.FeatureDangerLow
	FeatureDangerMedium   = types.FeatureDangerMedium
	FeatureDangerHigh     = types.FeatureDangerHigh
	FeatureDangerCritical = types.FeatureDangerCritical
)

// ── Bridge: Semantic alert / kernel risk from app/events ─────────────────────
const (
	semanticPromptLoopThreshold = events.SemanticPromptLoopThreshold
	semanticAPILoopThreshold    = events.SemanticAPILoopThreshold
	semanticFileIOLoopThreshold = events.SemanticFileIOLoopThreshold
	semanticStateGCInterval     = events.SemanticStateGCInterval
)
const (
	kernelRiskFeedbackKindNetworkIP   = events.KernelRiskFeedbackKindNetworkIP
	kernelRiskFeedbackKindNetworkPort = events.KernelRiskFeedbackKindNetworkPort
	kernelRiskFeedbackKindLSMFileName = events.KernelRiskFeedbackKindLSMFileName
	kernelRiskFeedbackKindLSMExecPath = events.KernelRiskFeedbackKindLSMExecPath
	kernelRiskFeedbackKindLSMExecName = events.KernelRiskFeedbackKindLSMExecName
)

// ── Bridge: Core types ───────────────────────────────────────────────────────
const (
	HookTypeNative   = core.HookTypeNative
	HookTypeWrapper  = core.HookTypeWrapper
	ConfigFormatJSON = core.ConfigFormatJSON
	ConfigFormatTOML = core.ConfigFormatTOML
)

// FeatureDim is re-exported from core; kept here for array-size references.
const FeatureDim = core.FeatureDim

// ── Bridge: ModelType re-exports ─────────────────────────────────────────────
const (
	ModelRandomForest                 = core.ModelRandomForest
	ModelKNN                          = core.ModelKNN
	ModelLogisticRegression           = core.ModelLogisticRegression
	ModelNaiveBayes                   = core.ModelNaiveBayes
	ModelNearestCentroid              = core.ModelNearestCentroid
	ModelExtraTrees                   = core.ModelExtraTrees
	ModelAdaBoost                     = core.ModelAdaBoost
	ModelSVM                          = core.ModelSVM
	ModelRidge                        = core.ModelRidge
	ModelPerceptron                   = core.ModelPerceptron
	ModelPassiveAggressive            = core.ModelPassiveAggressive
	ModelEnsemble                     = core.ModelEnsemble
	ModelAdditiveAttention            = core.ModelAdditiveAttention
	ModelGANTransformer               = core.ModelGANTransformer
	ModelScaledDotProductAttention    = core.ModelScaledDotProductAttention
	ModelMultiHeadAttention           = core.ModelMultiHeadAttention
	ModelRWKVAttention                = core.ModelRWKVAttention
	ModelMambaAttention               = core.ModelMambaAttention
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
	ModelNGramRandomForest            = core.ModelNGramRandomForest
	ModelNGramLogistic                = core.ModelNGramLogistic
	ModelNGramKNN                     = core.ModelNGramKNN
	ModelRandomForestAttn             = core.ModelRandomForestAttn
	ModelLogisticAttn                 = core.ModelLogisticAttn
	ModelKNNAttn                      = core.ModelKNNAttn
	ModelRandomForestFast             = core.ModelRandomForestFast
	ModelRandomForestShallow          = core.ModelRandomForestShallow
	ModelRandomForestStable           = core.ModelRandomForestStable
	ModelRandomForestDeep             = core.ModelRandomForestDeep
	ModelRandomForestWide             = core.ModelRandomForestWide
	ModelExtraTreesFast               = core.ModelExtraTreesFast
	ModelExtraTreesDeep               = core.ModelExtraTreesDeep
	ModelExtraTreesWide               = core.ModelExtraTreesWide
	ModelLogisticFast                 = core.ModelLogisticFast
	ModelLogisticNone                 = core.ModelLogisticNone
	ModelLogisticL1                   = core.ModelLogisticL1
	ModelLogisticBalanced             = core.ModelLogisticBalanced
	ModelLogisticL1Balanced           = core.ModelLogisticL1Balanced
	ModelSVMLong                      = core.ModelSVMLong
	ModelSVMBalanced                  = core.ModelSVMBalanced
	ModelPerceptronLong               = core.ModelPerceptronLong
	ModelPerceptronBalanced           = core.ModelPerceptronBalanced
	ModelPassiveAggressiveLong        = core.ModelPassiveAggressiveLong
	ModelPassiveAggressiveBalanced    = core.ModelPassiveAggressiveBalanced
	ModelKNNManhattan                 = core.ModelKNNManhattan
	ModelKNNCosine                    = core.ModelKNNCosine
	ModelKNNDistance                  = core.ModelKNNDistance
	ModelNearestCentroidBalanced      = core.ModelNearestCentroidBalanced
	ModelNearestCentroidCosine        = core.ModelNearestCentroidCosine
	ModelNearestCentroidManhattan     = core.ModelNearestCentroidManhattan
	ModelNaiveBayesBalanced           = core.ModelNaiveBayesBalanced
	ModelRidgeLong                    = core.ModelRidgeLong
	ModelRidgeBalanced                = core.ModelRidgeBalanced
	ModelRidgeLight                   = core.ModelRidgeLight
	ModelRidgeStrong                  = core.ModelRidgeStrong
	ModelAdaBoostLong                 = core.ModelAdaBoostLong
	ModelAdaBoostBalanced             = core.ModelAdaBoostBalanced
	ModelAdaBoostFast                 = core.ModelAdaBoostFast
	ModelAdaBoostLarge                = core.ModelAdaBoostLarge
	ModelEnsembleSoft                 = core.ModelEnsembleSoft
	ModelEnsembleHard                 = core.ModelEnsembleHard
	ModelEnsembleStacked              = core.ModelEnsembleStacked
)

// ── Standalone config ────────────────────────────────────────────────────────
const (
	bpfEventSampleSize  = int(unsafe.Sizeof(bpfEvent{}))
	bpfEventSampleAlign = uintptr(unsafe.Alignof(bpfEvent{}))
)
const (
	otelExporterQueueSize  = 2048
	otelMaxActiveRunSpans  = 1024
	otelMaxActiveTaskSpans = 4096
	otelMaxActiveToolSpans = 8192
	otelMaxAttributeLength = 4096
	otelMaxNameLength      = 256
	otelMaxSpanAttributes  = 128
	otelMaxSpanEvents      = 128
	otelMaxEventAttributes = 128
	otelMaxSpanLinks       = 32
	otelMaxLinkAttributes  = 32
	otelToolIdleTimeout    = 20 * time.Second
	otelTaskIdleTimeout    = 45 * time.Second
	otelRunIdleTimeout     = 90 * time.Second
)
const (
	agentSightDefaultLimit = 1000
	agentSightMaxLimit     = 10000
)
const (
	hookMarker              = "agent-ebpf-hook-active"
	kiroManagedAgent        = "agent-ebpf-hook"
	textPreviewLimitBytes   = 64 * 1024
	binaryPreviewLimitBytes = 4 * 1024
	imagePreviewLimitBytes  = 2 * 1024 * 1024
)
const udsPath = core.UDSPATH
const ebpfPinRoot = platform.EBPFPinRoot
const ebpfPinMapsDir = platform.EBPFPinMapsDir
const ebpfPinLinksDir = platform.EBPFPinLinksDir
