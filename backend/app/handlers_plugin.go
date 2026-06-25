package app

import (
	"agent-ebpf-filter/app/platform"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlers_plugin.go ----

// ─── Plugin CRUD ──────────────────────────────────────────────────────────────

func handlePluginsList(c *gin.Context) {
	plugins := pluginRegistry.List()
	c.JSON(200, gin.H{"plugins": plugins})
}

func handlePluginGet(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := validatePluginID(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	plugin, ok := pluginRegistry.Get(id)
	if !ok {
		c.JSON(404, gin.H{"error": "plugin not found"})
		return
	}
	source, _ := PluginSource(id)
	c.JSON(200, gin.H{"plugin": plugin, "source": source})
}

type pluginUpsertRequest struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Author         string           `json:"author"`
	Version        string           `json:"version"`
	Kind           PluginKind       `json:"kind"`
	Enabled        bool             `json:"enabled"`
	Source         string           `json:"source"`
	AttachKind     PluginAttachKind `json:"attachKind"`
	AttachTarget   string           `json:"attachTarget"`
	ProgramName    string           `json:"programName"`
	WebhookURL     string           `json:"webhookUrl"`
	WebhookEvents  []string         `json:"webhookEvents"`
	CommandComm    string           `json:"commandComm"`
	CommandArgs    []string         `json:"commandArgs"`
	CommandRule    string           `json:"commandRule"`
	CommandRewrite []string         `json:"commandRewrite"`
}

func handlePluginUpsert(c *gin.Context) {
	var req pluginUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := validatePluginID(req.ID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Kind == "" {
		req.Kind = PluginKindEBPF
	}
	manifest := &PluginManifest{
		ID:             req.ID,
		Name:           sanitizePluginName(req.Name),
		Description:    strings.TrimSpace(req.Description),
		Author:         strings.TrimSpace(req.Author),
		Version:        strings.TrimSpace(req.Version),
		Kind:           req.Kind,
		Enabled:        req.Enabled,
		AttachKind:     req.AttachKind,
		AttachTarget:   strings.TrimSpace(req.AttachTarget),
		ProgramName:    strings.TrimSpace(req.ProgramName),
		WebhookURL:     strings.TrimSpace(req.WebhookURL),
		WebhookEvents:  req.WebhookEvents,
		CommandComm:    strings.TrimSpace(req.CommandComm),
		CommandArgs:    req.CommandArgs,
		CommandRule:    strings.TrimSpace(req.CommandRule),
		CommandRewrite: req.CommandRewrite,
	}

	if req.Kind == PluginKindEBPF && strings.TrimSpace(req.Source) != "" {
		manifest.SourceSHA256 = sha256Hex([]byte(req.Source))
		if err := platform.WritePluginSource(req.ID, req.Source); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}

	if err := pluginRegistry.Upsert(manifest); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"plugin": manifest})
}

func handlePluginDelete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := validatePluginID(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	UnloadEBPFPlugin(id)
	if err := pluginRegistry.Delete(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handlePluginToggle(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := validatePluginID(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	manifest, err := pluginRegistry.SetEnabled(id, req.Enabled)
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	if manifest.Kind == PluginKindEBPF {
		if req.Enabled {
			if loadErr := LoadEBPFPlugin(&manifest); loadErr != nil {
				c.JSON(500, gin.H{"error": loadErr.Error(), "plugin": manifest})
				return
			}
		} else {
			UnloadEBPFPlugin(id)
		}
	}
	plugin, _ := pluginRegistry.Get(id)
	c.JSON(200, gin.H{"plugin": plugin})
}

// ─── eBPF online builder ──────────────────────────────────────────────────────

func handleBPFTemplates(c *gin.Context) {
	c.JSON(200, gin.H{"templates": bpfTemplates()})
}

type bpfCompileRequest struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

func handleBPFCompile(c *gin.Context) {
	var req bpfCompileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := validatePluginID(req.ID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	objPath, log, err := CompileUserBPF(req.ID, req.Source)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
			"log":   string(log),
		})
		return
	}
	c.JSON(200, gin.H{
		"objectPath":   objPath,
		"sourceSha256": sha256Hex([]byte(req.Source)),
		"log":          string(log),
		"compiledAt":   time.Now().UTC(),
	})
}

type bpfLoadRequest struct {
	ID string `json:"id"`
}

func handleBPFLoad(c *gin.Context) {
	var req bpfLoadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	plugin, ok := pluginRegistry.Get(req.ID)
	if !ok {
		c.JSON(404, gin.H{"error": "plugin not found"})
		return
	}
	if plugin.Kind != PluginKindEBPF {
		c.JSON(400, gin.H{"error": "not an eBPF plugin"})
		return
	}
	if err := LoadEBPFPlugin(&plugin); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	plugin, _ = pluginRegistry.Get(req.ID)
	c.JSON(200, gin.H{"plugin": plugin})
}

func handleBPFUnload(c *gin.Context) {
	var req bpfLoadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	UnloadEBPFPlugin(req.ID)
	plugin, _ := pluginRegistry.Get(req.ID)
	c.JSON(200, gin.H{"plugin": plugin})
}

// ─── Registration ────────────────────────────────────────────────────────────

func registerPluginRoutes(rg *gin.RouterGroup) {
	rg.GET("", handlePluginsList)
	rg.GET("/", handlePluginsList)
	rg.GET("/:id", handlePluginGet)
	rg.POST("", policyManagementEnabledMiddleware(), handlePluginUpsert)
	rg.PUT("/:id", policyManagementEnabledMiddleware(), handlePluginUpsert)
	rg.DELETE("/:id", policyManagementEnabledMiddleware(), handlePluginDelete)
	rg.POST("/:id/toggle", policyManagementEnabledMiddleware(), handlePluginToggle)
	rg.POST("/visual/llm-compile", handlePluginVisualLLMCompile)

	bpf := rg.Group("/bpf")
	{
		bpf.GET("/templates", handleBPFTemplates)
		bpf.POST("/compile", policyManagementEnabledMiddleware(), handleBPFCompile)
		bpf.POST("/load", policyManagementEnabledMiddleware(), handleBPFLoad)
		bpf.POST("/unload", policyManagementEnabledMiddleware(), handleBPFUnload)
	}
}
