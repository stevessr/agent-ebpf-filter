package app

import (
	"agent-ebpf-filter/pb"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlersruntimeconfig.go ----

func handleConfigRuntimeGet(c *gin.Context) {
	rc := buildRuntimeConfigResponse()
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
	writeProtoOrJSON(c, http.StatusOK, protoResp, rc)
}

func handleConfigRuntimePut(c *gin.Context) {
	var req runtimeSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings"})
		return
	}

	settings := runtimeSettingsStore.Snapshot()
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
	if req.KernelRiskFeedback != nil {
		settings.KernelRiskFeedback = *req.KernelRiskFeedback
		normalizeKernelRiskFeedbackSettings(&settings.KernelRiskFeedback)
	}
	applyMLConfigPatch(&settings.MLConfig, req.MLConfigPatch)
	if req.DomainForwardProxy != nil {
		settings.DomainForwardProxy = *req.DomainForwardProxy
	}

	settings, err := runtimeSettingsStore.Replace(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	applyRetentionConfig(settings)
	applyRuntimeDomainForwardProxy(settings)
	c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
}

type runtimeSettingsPatch struct {
	LogPersistenceEnabled   *bool                       `json:"logPersistenceEnabled,omitempty"`
	LogFilePath             *string                     `json:"logFilePath,omitempty"`
	AccessToken             *string                     `json:"accessToken,omitempty"`
	MaxEventCount           *int                        `json:"maxEventCount,omitempty"`
	MaxEventAge             *string                     `json:"maxEventAge,omitempty"`
	ShellSessionsEnabled    *bool                       `json:"shellSessionsEnabled,omitempty"`
	SystemRunEnabled        *bool                       `json:"systemRunEnabled,omitempty"`
	HookManagementEnabled   *bool                       `json:"hookManagementEnabled,omitempty"`
	PolicyManagementEnabled *bool                       `json:"policyManagementEnabled,omitempty"`
	OtlpEnabled             *bool                       `json:"otlpEnabled,omitempty"`
	OtlpEndpoint            *string                     `json:"otlpEndpoint,omitempty"`
	OtlpServiceName         *string                     `json:"otlpServiceName,omitempty"`
	OtlpHeaders             map[string]string           `json:"otlpHeaders,omitempty"`
	TlsCaptureEnabled       *bool                       `json:"tlsCaptureEnabled,omitempty"`
	KernelRiskFeedback      *KernelRiskFeedbackSettings `json:"kernelRiskFeedback,omitempty"`
	DomainForwardProxy      *DomainForwardProxySettings `json:"domainForwardProxy,omitempty"`
	MLConfigPatch
}

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

func applyMLConfigPatch(dst *MLConfig, patch MLConfigPatch) {
	if patch.Enabled != nil {
		dst.Enabled = *patch.Enabled
	}
	if patch.BlockConfidenceThreshold != nil {
		dst.BlockConfidenceThreshold = *patch.BlockConfidenceThreshold
	}
	if patch.MlMinConfidence != nil {
		dst.MlMinConfidence = *patch.MlMinConfidence
	}
	if patch.LowAnomalyThreshold != nil {
		dst.LowAnomalyThreshold = *patch.LowAnomalyThreshold
	}
	if patch.HighAnomalyThreshold != nil {
		dst.HighAnomalyThreshold = *patch.HighAnomalyThreshold
	}
	if patch.RuleOverridePriority != nil {
		dst.RuleOverridePriority = *patch.RuleOverridePriority
	}
	if patch.ModelType != nil {
		t := ModelType(strings.TrimSpace(*patch.ModelType))
		if _, ok := modelRegistry[t]; ok {
			dst.ModelType = t
			currentModelType = t
		}
	}
	if patch.ModelPath != nil {
		dst.ModelPath = strings.TrimSpace(*patch.ModelPath)
	}
	if patch.AutoTrain != nil {
		dst.AutoTrain = *patch.AutoTrain
	}
	if patch.TrainInterval != nil {
		dst.TrainInterval = strings.TrimSpace(*patch.TrainInterval)
	}
	if patch.MinSamplesForTraining != nil {
		dst.MinSamplesForTraining = *patch.MinSamplesForTraining
	}
	if patch.ActiveLearningEnabled != nil {
		dst.ActiveLearningEnabled = *patch.ActiveLearningEnabled
	}
	if patch.FeatureHistorySize != nil {
		dst.FeatureHistorySize = *patch.FeatureHistorySize
	}
	if patch.NumTrees != nil {
		dst.NumTrees = *patch.NumTrees
	}
	if patch.MaxDepth != nil {
		dst.MaxDepth = *patch.MaxDepth
	}
	if patch.MinSamplesLeaf != nil {
		dst.MinSamplesLeaf = *patch.MinSamplesLeaf
	}
	if patch.ValidationSplitRatio != nil {
		dst.ValidationSplitRatio = *patch.ValidationSplitRatio
	}
	if patch.BalanceClasses != nil {
		dst.BalanceClasses = *patch.BalanceClasses
	}
	if patch.EnsembleVoting != nil {
		dst.EnsembleVoting = normalizeEnsembleVoting(*patch.EnsembleVoting)
	}
	if patch.LlmEnabled != nil {
		dst.LlmEnabled = *patch.LlmEnabled
	}
	if patch.LlmBaseURL != nil {
		dst.LlmBaseURL = strings.TrimSpace(*patch.LlmBaseURL)
	}
	if patch.LlmAPIKey != nil {
		if key := strings.TrimSpace(*patch.LlmAPIKey); key != "" {
			dst.LlmAPIKey = key
		}
	}
	if patch.LlmModel != nil {
		dst.LlmModel = strings.TrimSpace(*patch.LlmModel)
	}
	if patch.LlmTimeoutSeconds != nil {
		dst.LlmTimeoutSeconds = *patch.LlmTimeoutSeconds
	}
	if patch.LlmTemperature != nil {
		dst.LlmTemperature = *patch.LlmTemperature
	}
	if patch.LlmMaxTokens != nil {
		dst.LlmMaxTokens = *patch.LlmMaxTokens
	}
	if patch.LlmSystemPrompt != nil {
		dst.LlmSystemPrompt = strings.TrimSpace(*patch.LlmSystemPrompt)
	}
}

func handleConfigAccessTokenPost(c *gin.Context) {
	settings, err := runtimeSettingsStore.RotateAccessToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
}
