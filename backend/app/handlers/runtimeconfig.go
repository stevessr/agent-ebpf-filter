package handlers

import (
	"agent-ebpf-filter/pb"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlersruntimeconfig.go ----

// MLConfigPatch holds optional ML configuration fields for PATCH semantics.
type MLConfigPatch struct {
	Enabled                  *bool    `json:"enabled,omitempty"`
	BlockConfidenceThreshold *float64 `json:"blockConfidenceThreshold,omitempty"`
	MlMinConfidence          *float64 `json:"mlMinConfidence,omitempty"`
	LowAnomalyThreshold      *float64 `json:"lowAnomalyThreshold,omitempty"`
	HighAnomalyThreshold     *float64 `json:"highAnomalyThreshold,omitempty"`
	RuleOverridePriority     *int     `json:"ruleOverridePriority,omitempty"`
	ModelType                *string  `json:"modelType,omitempty"`
	ModelPath                *string  `json:"modelPath,omitempty"`
	AutoTrain                *bool    `json:"autoTrain,omitempty"`
	TrainInterval            *string  `json:"trainInterval,omitempty"`
	MinSamplesForTraining    *int     `json:"minSamplesForTraining,omitempty"`
	ActiveLearningEnabled    *bool    `json:"activeLearningEnabled,omitempty"`
	FeatureHistorySize       *int     `json:"featureHistorySize,omitempty"`
	NumTrees                 *int     `json:"numTrees,omitempty"`
	MaxDepth                 *int     `json:"maxDepth,omitempty"`
	MinSamplesLeaf           *int     `json:"minSamplesLeaf,omitempty"`
	ValidationSplitRatio     *float64 `json:"validationSplitRatio,omitempty"`
	BalanceClasses           *bool    `json:"balanceClasses,omitempty"`
	EnsembleVoting           *string  `json:"ensembleVoting,omitempty"`
	LlmEnabled               *bool    `json:"llmEnabled,omitempty"`
	LlmBaseURL               *string  `json:"llmBaseUrl,omitempty"`
	LlmAPIKey                *string  `json:"llmApiKey,omitempty"`
	LlmModel                 *string  `json:"llmModel,omitempty"`
	LlmTimeoutSeconds        *int     `json:"llmTimeoutSeconds,omitempty"`
	LlmTemperature           *float64 `json:"llmTemperature,omitempty"`
	LlmMaxTokens             *int     `json:"llmMaxTokens,omitempty"`
	LlmSystemPrompt          *string  `json:"llmSystemPrompt,omitempty"`
}

// RuntimeSettingsPatch holds optional runtime setting fields for PATCH semantics.
type RuntimeSettingsPatch struct {
	LogPersistenceEnabled   *bool         `json:"logPersistenceEnabled,omitempty"`
	LogFilePath             *string       `json:"logFilePath,omitempty"`
	AccessToken             *string       `json:"accessToken,omitempty"`
	MaxEventCount           *int          `json:"maxEventCount,omitempty"`
	MaxEventAge             *string       `json:"maxEventAge,omitempty"`
	ShellSessionsEnabled    *bool         `json:"shellSessionsEnabled,omitempty"`
	SystemRunEnabled        *bool         `json:"systemRunEnabled,omitempty"`
	HookManagementEnabled   *bool         `json:"hookManagementEnabled,omitempty"`
	PolicyManagementEnabled *bool         `json:"policyManagementEnabled,omitempty"`
	OtlpEnabled             *bool         `json:"otlpEnabled,omitempty"`
	OtlpEndpoint            *string       `json:"otlpEndpoint,omitempty"`
	OtlpServiceName         *string       `json:"otlpServiceName,omitempty"`
	OtlpHeaders             map[string]string `json:"otlpHeaders,omitempty"`
	TlsCaptureEnabled       *bool         `json:"tlsCaptureEnabled,omitempty"`
	KernelRiskFeedback      *interface{}  `json:"kernelRiskFeedback,omitempty"`
	DomainForwardProxy      *interface{}  `json:"domainForwardProxy,omitempty"`
	MLConfigPatch
}

func HandleConfigRuntimeGet(c *gin.Context) {
	rc := Deps.BuildRuntimeConfigResponse()
	protoResp := &pb.RuntimeConfigResponse{
		Runtime: &pb.RuntimeSettings{
			LogPersistenceEnabled:   rc.Runtime.LogPersistenceEnabled,
			LogFilePath:             rc.Runtime.LogFilePath,
			AccessToken:             rc.Runtime.AccessToken,
			MaxEventCount:           int32(rc.Runtime.MaxEventCount),
			MaxEventAge:             rc.Runtime.MaxEventAge,
			ShellSessionsEnabled:    rc.Runtime.ShellSessionsEnabled,
			SystemRunEnabled:        rc.Runtime.SystemRunEnabled,
			HookManagementEnabled:   rc.Runtime.HookManagementEnabled,
			PolicyManagementEnabled: rc.Runtime.PolicyManagementEnabled,
			OtlpEnabled:             rc.Runtime.OtlpEnabled,
			OtlpEndpoint:            rc.Runtime.OtlpEndpoint,
			OtlpServiceName:         rc.Runtime.OtlpServiceName,
			OtlpHeaders:             rc.Runtime.OtlpHeaders,
		},
		McpEndpoint:            rc.MCPEndpoint,
		AuthHeaderName:         rc.AuthHeaderName,
		BearerAuthHeaderName:   rc.BearerAuthHeaderName,
		PersistedEventLogPath:  rc.PersistedEventLogPath,
		PersistedEventLogAlive: rc.PersistedEventLogAlive,
	}
	Deps.WriteProtoOrJSON(c, http.StatusOK, protoResp, rc)
}

func HandleConfigRuntimePut(c *gin.Context) {
	var req RuntimeSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings"})
		return
	}

	settings := Deps.RuntimeSettings.Snapshot()
	if req.LogPersistenceEnabled != nil {
		settings.LogPersistenceEnabled = *req.LogPersistenceEnabled
	}
	if req.LogFilePath != nil {
		settings.LogFilePath = strings.TrimSpace(*req.LogFilePath)
	}
	if req.AccessToken != nil {
		settings.AccessToken = strings.TrimSpace(*req.AccessToken)
	}
	if req.MaxEventCount != nil {
		settings.MaxEventCount = *req.MaxEventCount
	}
	if req.MaxEventAge != nil {
		settings.MaxEventAge = strings.TrimSpace(*req.MaxEventAge)
	}
	if req.ShellSessionsEnabled != nil {
		settings.ShellSessionsEnabled = *req.ShellSessionsEnabled
	}
	if req.SystemRunEnabled != nil {
		settings.SystemRunEnabled = *req.SystemRunEnabled
	}
	if req.HookManagementEnabled != nil {
		settings.HookManagementEnabled = *req.HookManagementEnabled
	}
	if req.PolicyManagementEnabled != nil {
		settings.PolicyManagementEnabled = *req.PolicyManagementEnabled
	}
	if req.OtlpEnabled != nil {
		settings.OtlpEnabled = *req.OtlpEnabled
	}
	if req.OtlpEndpoint != nil {
		settings.OtlpEndpoint = strings.TrimSpace(*req.OtlpEndpoint)
	}
	if req.OtlpServiceName != nil {
		settings.OtlpServiceName = strings.TrimSpace(*req.OtlpServiceName)
	}
	if req.OtlpHeaders != nil {
		settings.OtlpHeaders = make(map[string]string, len(req.OtlpHeaders))
		for key, value := range req.OtlpHeaders {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			settings.OtlpHeaders[trimmedKey] = strings.TrimSpace(value)
		}
	}
	if req.TlsCaptureEnabled != nil {
		settings.TlsCaptureEnabled = *req.TlsCaptureEnabled
	}
	// KernelRiskFeedback and DomainForwardProxy use interface{} bridge;
	// actual application is handled by Deps closures if set.
	_ = req.KernelRiskFeedback
	_ = req.DomainForwardProxy

	settings, err := Deps.RuntimeSettingsReplace(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	Deps.ApplyRetentionConfig(settings)
	Deps.ApplyRuntimeDomainForwardProxy(settings)
	c.JSON(http.StatusOK, Deps.BuildRuntimeConfigResponseFromSettings(settings))
}

// HandleConfigAccessTokenPost rotates the access token.
func HandleConfigAccessTokenPost(c *gin.Context) {
	settings, err := Deps.RuntimeSettingsReplace(Deps.RuntimeSettings.Snapshot())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Rotate token via the app-level bridge
	settings = Deps.RotateAccessToken(settings)
	c.JSON(http.StatusOK, Deps.BuildRuntimeConfigResponseFromSettings(settings))
}