package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func handleConfigHooksList(c *gin.Context) {
	res := []gin.H{}
	for _, h := range availableHooks {
		res = append(res, gin.H{
			"id": h.ID, "name": h.Name, "description": h.Description,
			"target_cmd": h.TargetCmd, "hook_type": h.HookType,
			"installed": isHookInstalled(h),
		})
	}
	c.JSON(200, res)
}

func handleConfigHooksInstall(c *gin.Context) {
	var req struct {
		ID         string `json:"id"`
		Install    bool   `json:"install"`
		UseWrapper bool   `json:"use_wrapper"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == req.ID {
			target = h
			found = true
			break
		}
	}
	if !found {
		c.JSON(404, gin.H{"error": "hook not found"})
		return
	}

	effectiveType := target.HookType
	if req.UseWrapper {
		effectiveType = HookTypeWrapper
	}

	if req.Install {
		if effectiveType == HookTypeNative {
			if err := installNativeHook(target); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		} else {
			p := getShellConfigPath()
			b, _ := os.ReadFile(p)
			content := string(b)
			aliasLine := fmt.Sprintf("\nalias %s='agent-wrapper %s' # agent-ebpf-hook\n", target.TargetCmd, target.TargetCmd)
			if !strings.Contains(content, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				newContent := content + aliasLine
				if err := writeFileAsRealUser(p, []byte(newContent), 0644); err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
			}
		}
	} else {
		if target.HookType == HookTypeNative {
			_ = uninstallNativeHook(target)
		}
		p := getShellConfigPath()
		b, _ := os.ReadFile(p)
		lines := strings.Split(string(b), "\n")
		newLines := []string{}
		for _, l := range lines {
			if !strings.Contains(l, fmt.Sprintf("alias %s=", target.TargetCmd)) {
				newLines = append(newLines, l)
			}
		}
		_ = writeFileAsRealUser(p, []byte(strings.Join(newLines, "\n")), 0644)
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleConfigHooksRawGet(c *gin.Context) {
	id := c.Param("id")
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	if target.ID == "kiro" {
		if err := ensureKiroManagedAgentExists(); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	b, err := os.ReadFile(target.NativeConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(200, gin.H{"content": "{}", "path": target.NativeConfigPath, "format": target.ConfigFormat})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"content": string(b), "path": target.NativeConfigPath, "format": target.ConfigFormat})
}

func handleConfigHooksRawPost(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	var target HookDef
	found := false
	for _, h := range availableHooks {
		if h.ID == id {
			target = h
			found = true
			break
		}
	}
	if !found || target.HookType != HookTypeNative {
		c.JSON(404, gin.H{"error": "native hook not found"})
		return
	}
	var js map[string]interface{}
	if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
		c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}

	if err := mkdirAllAsRealUser(filepath.Dir(target.NativeConfigPath), 0755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := writeFileAsRealUser(target.NativeConfigPath, []byte(req.Content), 0644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func registerConfigRoutes(rg *gin.RouterGroup) {
	rg.GET("/tags", handleConfigTagsGet)
	rg.POST("/tags", policyManagementEnabledMiddleware(), handleConfigTagsPost)
	rg.GET("/comms", handleConfigCommsGet)
	rg.POST("/comms", policyManagementEnabledMiddleware(), handleConfigCommsPost)
	rg.DELETE("/comms/:comm", policyManagementEnabledMiddleware(), handleConfigCommsDelete)
	rg.POST("/comms/:comm/disable", policyManagementEnabledMiddleware(), handleConfigCommsDisable)
	rg.DELETE("/comms/:comm/disable", policyManagementEnabledMiddleware(), handleConfigCommsEnable)
	rg.GET("/event-types", handleConfigEventTypesGet)
	rg.POST("/event-types/:type/disable", policyManagementEnabledMiddleware(), handleConfigEventTypeDisable)
	rg.DELETE("/event-types/:type/disable", policyManagementEnabledMiddleware(), handleConfigEventTypeEnable)
	rg.GET("/paths", handleConfigPathsGet)
	rg.POST("/paths", policyManagementEnabledMiddleware(), handleConfigPathsPost)
	rg.DELETE("/paths/*path", policyManagementEnabledMiddleware(), handleConfigPathsDelete)
	rg.GET("/prefixes", handleConfigPrefixesGet)
	rg.POST("/prefixes", policyManagementEnabledMiddleware(), handleConfigPrefixesPost)
	rg.DELETE("/prefixes", policyManagementEnabledMiddleware(), handleConfigPrefixesDelete)
	rg.GET("/rules", handleConfigRulesGet)
	rg.POST("/rules", policyManagementEnabledMiddleware(), handleConfigRulesPost)
	rg.DELETE("/rules/:comm", policyManagementEnabledMiddleware(), handleConfigRulesDelete)
	rg.GET("/runtime", handleConfigRuntimeGet)
	rg.PUT("/runtime", handleConfigRuntimePut)
	rg.POST("/access-token", handleConfigAccessTokenPost)
	rg.GET("/export", handleConfigExportGet)
	rg.POST("/import", policyManagementEnabledMiddleware(), handleConfigImportPost)

	// ML classification endpoints
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
		ml.POST("/datasets/pull", handleMLDatasetPullPost)
		ml.POST("/datasets/import", handleMLDatasetImportPost)
		ml.GET("/datasets/export", handleMLDatasetExportGet)
		ml.DELETE("/datasets", handleMLDatasetClearDelete)
		ml.POST("/backtest", handleMLBacktestPost)
	}

	hooks := rg.Group("/hooks")
	{
		hooks.GET("", handleConfigHooksList)
		hooks.POST("", hookManagementEnabledMiddleware(), handleConfigHooksInstall)
		hooks.GET("/:id/raw", handleConfigHooksRawGet)
		hooks.POST("/:id/raw", hookManagementEnabledMiddleware(), handleConfigHooksRawPost)
	}
}
