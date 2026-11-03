package handlers

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

// Runtime settings updates are PATCH-style read/modify/write operations. Keep
// the persistence step and all runtime side effects in one critical section so
// concurrent requests cannot apply stale feature-gate state out of order.
var runtimeSettingsMutationMu sync.Mutex

const runtimeSettingsMaxBodyBytes int64 = 1 << 20

// MLConfigPatch holds optional ML configuration fields for PATCH semantics.
type MLConfigPatch struct {
	Enabled                  *bool    `json:"enabled,omitempty"`
	BlockConfidenceThreshold *float64 `json:"blockConfidenceThreshold,omitempty"`
	AllowRiskScoreThreshold  *float64 `json:"allowRiskScoreThreshold,omitempty"`
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
	LogPersistenceEnabled   *bool                            `json:"logPersistenceEnabled,omitempty"`
	LogFilePath             *string                          `json:"logFilePath,omitempty"`
	EventStoreMaxRecords    *int                             `json:"eventStoreMaxRecords,omitempty"`
	EventStoreMaxAge        *string                          `json:"eventStoreMaxAge,omitempty"`
	DisabledEventTypes      *[]uint32                        `json:"disabledEventTypes,omitempty"`
	IgnoredPaths            *[]string                        `json:"ignoredPaths,omitempty"`
	AccessToken             *string                          `json:"accessToken,omitempty"`
	MaxEventCount           *int                             `json:"maxEventCount,omitempty"`
	MaxEventAge             *string                          `json:"maxEventAge,omitempty"`
	ShellSessionsEnabled    *bool                            `json:"shellSessionsEnabled,omitempty"`
	SystemRunEnabled        *bool                            `json:"systemRunEnabled,omitempty"`
	HookManagementEnabled   *bool                            `json:"hookManagementEnabled,omitempty"`
	PolicyManagementEnabled *bool                            `json:"policyManagementEnabled,omitempty"`
	OtlpEnabled             *bool                            `json:"otlpEnabled,omitempty"`
	OtlpEndpoint            *string                          `json:"otlpEndpoint,omitempty"`
	OtlpServiceName         *string                          `json:"otlpServiceName,omitempty"`
	OtlpHeaders             map[string]string                `json:"otlpHeaders,omitempty"`
	TlsCaptureEnabled       *bool                            `json:"tlsCaptureEnabled,omitempty"`
	KernelRiskFeedback      *core.KernelRiskFeedbackSettings `json:"kernelRiskFeedback,omitempty"`
	LoopDetection           *core.LoopDetectionSettings      `json:"loopDetection,omitempty"`
	ResearchProcessing      *core.ResearchProcessingSettings `json:"researchProcessing,omitempty"`
	SignalProcessing        *core.SignalProcessingSettings   `json:"signalProcessing,omitempty"`
	DomainForwardProxy      *core.DomainForwardProxySettings `json:"domainForwardProxy,omitempty"`
	MLConfigPatch
}

func applyMLConfigPatch(dst *core.MLConfig, patch MLConfigPatch) {
	if dst == nil {
		return
	}
	if patch.Enabled != nil {
		dst.Enabled = *patch.Enabled
	}
	if patch.BlockConfidenceThreshold != nil {
		dst.BlockConfidenceThreshold = *patch.BlockConfidenceThreshold
	}
	if patch.AllowRiskScoreThreshold != nil {
		dst.AllowRiskScoreThreshold = *patch.AllowRiskScoreThreshold
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
		dst.ModelType = core.ModelType(strings.TrimSpace(*patch.ModelType))
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
		dst.EnsembleVoting = strings.TrimSpace(*patch.EnsembleVoting)
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, runtimeSettingsMaxBodyBytes)
	var req RuntimeSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": "invalid runtime settings"})
		return
	}

	runtimeSettingsMutationMu.Lock()
	defer runtimeSettingsMutationMu.Unlock()

	settings := Deps.RuntimeSettings.Snapshot()
	if req.LogPersistenceEnabled != nil {
		settings.LogPersistenceEnabled = *req.LogPersistenceEnabled
	}
	if req.LogFilePath != nil {
		settings.LogFilePath = strings.TrimSpace(*req.LogFilePath)
	}
	if req.EventStoreMaxRecords != nil {
		settings.EventStoreMaxRecords = *req.EventStoreMaxRecords
	}
	if req.EventStoreMaxAge != nil {
		settings.EventStoreMaxAge = strings.TrimSpace(*req.EventStoreMaxAge)
	}
	if req.DisabledEventTypes != nil {
		settings.DisabledEventTypes = append([]uint32(nil), (*req.DisabledEventTypes)...)
	}
	if req.IgnoredPaths != nil {
		settings.IgnoredPaths = append([]string(nil), (*req.IgnoredPaths)...)
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
	}
	if req.LoopDetection != nil {
		settings.LoopDetection = *req.LoopDetection
	}
	if req.ResearchProcessing != nil {
		settings.ResearchProcessing = *req.ResearchProcessing
	}
	if req.SignalProcessing != nil {
		settings.SignalProcessing = *req.SignalProcessing
	}
	if req.DomainForwardProxy != nil {
		settings.DomainForwardProxy = *req.DomainForwardProxy
	}
	applyMLConfigPatch(&settings.MLConfig, req.MLConfigPatch)
	if req.AllowRiskScoreThreshold != nil || req.BlockConfidenceThreshold != nil {
		if err := core.ValidateMLActionRiskThresholds(
			settings.MLConfig.AllowRiskScoreThreshold,
			settings.MLConfig.BlockConfidenceThreshold,
		); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	settings, err := Deps.RuntimeSettingsReplace(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	Deps.ApplyRetentionConfig(settings)
	Deps.ApplyRuntimeDomainForwardProxy(settings)
	Deps.ApplyRuntimeTLSCapture(settings)
	c.JSON(http.StatusOK, Deps.BuildRuntimeConfigResponseFromSettings(settings))
}

// HandleConfigAccessTokenPost rotates the access token.
func HandleConfigAccessTokenPost(c *gin.Context) {
	runtimeSettingsMutationMu.Lock()
	defer runtimeSettingsMutationMu.Unlock()

	settings, err := Deps.RuntimeSettingsReplace(Deps.RuntimeSettings.Snapshot())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Rotate token via the app-level bridge
	settings = Deps.RotateAccessToken(settings)
	c.JSON(http.StatusOK, Deps.BuildRuntimeConfigResponseFromSettings(settings))
}
