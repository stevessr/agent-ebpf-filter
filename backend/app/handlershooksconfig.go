package app

import (
	"github.com/gin-gonic/gin"
)

// ---- route registration (stays in app/ due to FeatureRegistry dependency) ----

func registerConfigRoutes(rg *gin.RouterGroup, features *FeatureRegistry) {
	policyMiddleware := policyManagementEnabledMiddleware()
	if !features.CompiledIn(FeaturePolicyManagement) {
		policyMiddleware = compiledOutFeatureMiddleware(FeaturePolicyManagement)
	}

	rg.GET("/tags", handleConfigTagsGet)
	rg.POST("/tags", policyMiddleware, handleConfigTagsPost)
	rg.GET("/comms", handleConfigCommsGet)
	rg.POST("/comms", policyMiddleware, handleConfigCommsPost)
	rg.DELETE("/comms/:comm", policyMiddleware, handleConfigCommsDelete)
	rg.POST("/comms/:comm/disable", policyMiddleware, handleConfigCommsDisable)
	rg.DELETE("/comms/:comm/disable", policyMiddleware, handleConfigCommsEnable)
	rg.GET("/event-types", handleConfigEventTypesGet)
	rg.POST("/event-types/:type/disable", policyMiddleware, handleConfigEventTypeDisable)
	rg.DELETE("/event-types/:type/disable", policyMiddleware, handleConfigEventTypeEnable)
	rg.GET("/paths", handleConfigPathsGet)
	rg.POST("/paths", policyMiddleware, handleConfigPathsPost)
	rg.DELETE("/paths/*path", policyMiddleware, handleConfigPathsDelete)
	rg.GET("/prefixes", handleConfigPrefixesGet)
	rg.POST("/prefixes", policyMiddleware, handleConfigPrefixesPost)
	rg.DELETE("/prefixes", policyMiddleware, handleConfigPrefixesDelete)
	rg.GET("/rules", handleConfigRulesGet)
	rg.POST("/rules", policyMiddleware, handleConfigRulesPost)
	rg.DELETE("/rules/:comm", policyMiddleware, handleConfigRulesDelete)
	rg.GET("/runtime", handleConfigRuntimeGet)
	rg.PUT("/runtime", handleConfigRuntimePut)
	rg.POST("/access-token", handleConfigAccessTokenPost)
	rg.GET("/export", handleConfigExportGet)
	rg.POST("/import", policyMiddleware, handleConfigImportPost)

	if features.CompiledIn(FeatureML) {
		ml := rg.Group("/ml")
		{
			ml.GET("/status", handleMLStatusGet)
			ml.GET("/logs", handleMLLogsGet)
			ml.GET("/history", handleMLHistoryGet)
			ml.POST("/train", handleMLTrainPost)
			ml.POST("/train/cancel", handleMLTrainCancelPost)
			ml.POST("/tune", handleMLTunePost)
			ml.POST("/tune-models", handleMLTuneModelsPost)
			ml.POST("/feedback", handleMLFeedbackPost)
			ml.GET("/samples", handleMLSamplesGet)
			ml.POST("/samples", handleMLSamplesPost)
			ml.PUT("/samples/label", handleMLSampleLabelPut)
			ml.PUT("/samples/anomaly", handleMLSampleAnomalyPut)
			ml.DELETE("/samples/:index", handleMLSampleDelete)
			ml.GET("/existing-commands", handleMLExistingCommandsGet)
			ml.POST("/import-existing", handleMLImportExistingPost)
			ml.POST("/assess", handleMLAssessPost)
			ml.POST("/llm/score", handleMLLLMScorePost)
			ml.POST("/llm/batch-score", handleMLLLMBatchScorePost)
			ml.POST("/llm/production-dataset/pull", handleMLLLMProductionDatasetPullPost)
			ml.GET("/datasets/classic", handleClassicDatasetsListGet)
			ml.GET("/datasets/classic/:name", handleClassicDatasetGet)
			ml.POST("/datasets/classic/:name/preview", handleClassicDatasetPreviewPost)
			ml.POST("/datasets/pull", handleMLDatasetPullPost)
			ml.POST("/datasets/import", handleMLDatasetImportPost)
			ml.POST("/datasets/agent-legal", handleMLAgentLegalDatasetPost)
			ml.GET("/datasets/export", handleMLDatasetExportGet)
			ml.DELETE("/datasets", handleMLDatasetClearDelete)
			ml.POST("/backtest", handleMLBacktestPost)
			ml.GET("/health/processes", handleMLHealthProcessesGet)
			ml.GET("/health/generators", handleMLHealthGeneratorsGet)
			ml.POST("/health/register", handleMLHealthRegisterPost)
			ml.POST("/health/unregister", handleMLHealthUnregisterPost)
			ml.POST("/health/run", handleMLHealthRunPost)
		}
	}

	if features.CompiledIn(FeatureHooks) {
		hooks := rg.Group("/hooks")
		{
			hooks.GET("", handleConfigHooksList)
			hooks.POST("", hookManagementEnabledMiddleware(), handleConfigHooksInstall)
			hooks.GET("/:id/raw", handleConfigHooksRawGet)
			hooks.POST("/:id/raw", hookManagementEnabledMiddleware(), handleConfigHooksRawPost)
		}
	}
}
