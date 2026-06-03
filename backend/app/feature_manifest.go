package app

import (
	"net/http"

	"agent-ebpf-filter/core"

	"github.com/gin-gonic/gin"
)

type FeatureID string

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

type FeatureManifestEntry struct {
	ID                   FeatureID          `json:"id"`
	Name                 string             `json:"name"`
	CompiledIn           bool               `json:"compiledIn"`
	RuntimeEnabled       bool               `json:"runtimeEnabled"`
	RuntimeGate          string             `json:"runtimeGate,omitempty"`
	AuthRequired         bool               `json:"authRequired"`
	RoutePrefixes        []string           `json:"routePrefixes"`
	DangerLevel          FeatureDangerLevel `json:"dangerLevel"`
	BuildTag             string             `json:"buildTag"`
	CompatibilityAliases []string           `json:"compatibilityAliases,omitempty"`
}

type FeatureManifestResponse struct {
	Features []FeatureManifestEntry `json:"features"`
}

type featureDefinition struct {
	id                   FeatureID
	name                 string
	runtimeGate          string
	authRequired         bool
	routePrefixes        []string
	dangerLevel          FeatureDangerLevel
	compatibilityAliases []string
	runtimeEnabled       func(core.RuntimeSettings) bool
}

var optionalFeatureIDs = []FeatureID{
	FeatureShellSessions,
	FeatureSystemRun,
	FeatureHooks,
	FeaturePolicyManagement,
	FeatureTLSCapture,
	FeatureOTLP,
	FeatureDomainForward,
	FeatureML,
	FeaturePlugins,
	FeatureSandboxCgroup,
	FeatureSandboxLSM,
	FeatureNetworkExport,
	FeatureAgentSight,
}

var featureDefinitions = []featureDefinition{
	{
		id:            FeatureShellSessions,
		name:          "Shell sessions",
		runtimeGate:   "shell_sessions",
		authRequired:  true,
		routePrefixes: []string{"/shell-sessions", "/ws/shell", "/ws/shell-sessions"},
		dangerLevel:   FeatureDangerHigh,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.ShellSessionsEnabled
		},
	},
	{
		id:            FeatureSystemRun,
		name:          "System command runner",
		runtimeGate:   "system_run",
		authRequired:  true,
		routePrefixes: []string{"/system/run"},
		dangerLevel:   FeatureDangerCritical,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.SystemRunEnabled
		},
	},
	{
		id:            FeatureHooks,
		name:          "Native hook management",
		runtimeGate:   "hook_management",
		authRequired:  true,
		routePrefixes: []string{"/hooks/event", "/config/hooks"},
		dangerLevel:   FeatureDangerHigh,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.HookManagementEnabled
		},
	},
	{
		id:            FeaturePolicyManagement,
		name:          "Policy management",
		runtimeGate:   "policy_management",
		authRequired:  true,
		routePrefixes: []string{"/config/tags", "/config/comms", "/config/paths", "/config/prefixes", "/config/rules", "/sandbox", "/api/v1/policies"},
		dangerLevel:   FeatureDangerHigh,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.PolicyManagementEnabled
		},
	},
	{
		id:            FeatureTLSCapture,
		name:          "TLS and Codex capture",
		runtimeGate:   "tls_capture",
		authRequired:  true,
		routePrefixes: []string{"/tls-capture", "/ws/tls-capture", "/codex/capture"},
		dangerLevel:   FeatureDangerCritical,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.TlsCaptureEnabled
		},
	},
	{
		id:            FeatureOTLP,
		name:          "OpenTelemetry export",
		runtimeGate:   "otlp",
		authRequired:  true,
		routePrefixes: []string{"/system/otel-health"},
		dangerLevel:   FeatureDangerMedium,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.OtlpEnabled
		},
	},
	{
		id:            FeatureDomainForward,
		name:          "Domain forward proxy",
		runtimeGate:   "domain_forward",
		authRequired:  true,
		routePrefixes: []string{"/system/domain-forward/status"},
		dangerLevel:   FeatureDangerHigh,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.DomainForwardProxy.Enabled
		},
	},
	{
		id:            FeatureML,
		name:          "ML and LLM scoring",
		authRequired:  true,
		routePrefixes: []string{"/config/ml", "/ws/ml-status"},
		dangerLevel:   FeatureDangerMedium,
		runtimeEnabled: func(settings core.RuntimeSettings) bool {
			return settings.MLConfig.Enabled
		},
	},
	{
		id:            FeaturePlugins,
		name:          "Plugin registry and visual builder",
		authRequired:  true,
		routePrefixes: []string{"/plugins"},
		dangerLevel:   FeatureDangerHigh,
	},
	{
		id:            FeatureSandboxCgroup,
		name:          "cgroup sandbox enforcement",
		authRequired:  true,
		routePrefixes: []string{"/sandbox/cgroup", "/api/v1/sandbox/cgroup"},
		dangerLevel:   FeatureDangerHigh,
	},
	{
		id:            FeatureSandboxLSM,
		name:          "BPF LSM enforcement",
		authRequired:  true,
		routePrefixes: []string{"/sandbox/lsm", "/api/v1/sandbox/lsm"},
		dangerLevel:   FeatureDangerHigh,
	},
	{
		id:            FeatureNetworkExport,
		name:          "Network export",
		authRequired:  true,
		routePrefixes: []string{"/network/export", "/network/export-pcap", "/api/v1/network/export"},
		dangerLevel:   FeatureDangerMedium,
	},
	{
		id:            FeatureAgentSight,
		name:          "AgentSight compatibility",
		authRequired:  true,
		routePrefixes: []string{"/agentsight", "/api/agentsight", "/api/v1/agentsight"},
		dangerLevel:   FeatureDangerLow,
		compatibilityAliases: []string{
			"/api/agentsight",
			"/api/v1/agentsight",
		},
	},
}

func buildFeatureManifest(settings core.RuntimeSettings) FeatureManifestResponse {
	features := make([]FeatureManifestEntry, 0, len(featureDefinitions))
	for _, definition := range featureDefinitions {
		compiledIn := isFeatureCompiledIn(definition.id)
		runtimeEnabled := compiledIn
		if definition.runtimeEnabled != nil {
			runtimeEnabled = compiledIn && definition.runtimeEnabled(settings)
		}
		features = append(features, FeatureManifestEntry{
			ID:                   definition.id,
			Name:                 definition.name,
			CompiledIn:           compiledIn,
			RuntimeEnabled:       runtimeEnabled,
			RuntimeGate:          definition.runtimeGate,
			AuthRequired:         definition.authRequired,
			RoutePrefixes:        append([]string(nil), definition.routePrefixes...),
			DangerLevel:          definition.dangerLevel,
			BuildTag:             featureBuildTag(definition.id),
			CompatibilityAliases: append([]string(nil), definition.compatibilityAliases...),
		})
	}
	return FeatureManifestResponse{Features: features}
}

func handleSystemFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, buildFeatureManifest(runtimeSettingsStore.Snapshot()))
}

func featureBuildTag(id FeatureID) string {
	return "agentfeat_" + string(id)
}
