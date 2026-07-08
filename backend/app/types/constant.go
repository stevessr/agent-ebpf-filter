package types

// FeatureID identifies an optional compiled-in capability.
type FeatureID string

// FeatureDangerLevel categorises the risk of enabling a feature.
type FeatureDangerLevel string

const (
	FeatureShellSessions    FeatureID = "shell_sessions"
	FeatureSystemRun        FeatureID = "system_run"
	FeatureHooks            FeatureID = "hooks"
	FeaturePolicyManagement FeatureID = "policy_management"
	FeatureTLSCapture       FeatureID = "tls_capture"
	FeatureOTLP             FeatureID = "otlp"
	FeatureDomainForward    FeatureID = "domain_forward"
	FeatureML               FeatureID = "ml"
	FeaturePlugins          FeatureID = "plugins"
	FeatureSandboxCgroup    FeatureID = "sandbox_cgroup"
	FeatureSandboxLSM       FeatureID = "sandbox_lsm"
	FeatureNetworkExport    FeatureID = "network_export"
	FeatureAgentSight       FeatureID = "agentsight"
)

const (
	FeatureDangerLow      FeatureDangerLevel = "low"
	FeatureDangerMedium   FeatureDangerLevel = "medium"
	FeatureDangerHigh     FeatureDangerLevel = "high"
	FeatureDangerCritical FeatureDangerLevel = "critical"
)

// PluginKind enumerates the plugin types supported by the registry.
type PluginKind string

const (
	PluginKindEBPF    PluginKind = "ebpf"
	PluginKindWebhook PluginKind = "webhook"
	PluginKindCommand PluginKind = "command"
)

// PluginAttachKind describes how an eBPF plugin attaches to the kernel.
type PluginAttachKind string

const (
	PluginAttachTracepoint PluginAttachKind = "tracepoint"
	PluginAttachKprobe     PluginAttachKind = "kprobe"
	PluginAttachKretprobe  PluginAttachKind = "kretprobe"
	PluginAttachLSM        PluginAttachKind = "lsm"
	PluginAttachNone       PluginAttachKind = "none"
)