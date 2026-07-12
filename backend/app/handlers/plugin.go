package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const pluginUpsertMaxBodyBytes int64 = 320 << 10

// ---- moved from app/handlers_plugin.go ----

// pluginUpsertRequest mirrors the request body for plugin upsert operations.
type PluginUpsertRequest struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Author         string   `json:"author"`
	Version        string   `json:"version"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	Source         string   `json:"source"`
	AttachKind     string   `json:"attachKind"`
	AttachTarget   string   `json:"attachTarget"`
	ProgramName    string   `json:"programName"`
	WebhookURL     string   `json:"webhookUrl"`
	WebhookEvents  []string `json:"webhookEvents"`
	CommandComm    string   `json:"commandComm"`
	CommandArgs    []string `json:"commandArgs"`
	CommandRule    string   `json:"commandRule"`
	CommandRewrite []string `json:"commandRewrite"`
}

type bpfCompileRequest struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

type bpfLoadRequest struct {
	ObjPath     string `json:"objPath"`
	Attach      string `json:"attach"`
	Target      string `json:"target"`
	ProgramName string `json:"programName"`
}

func HandlePluginsList(c *gin.Context) {
	plugins := Deps.PluginList()
	c.JSON(200, gin.H{"plugins": plugins})
}

func HandlePluginGet(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := Deps.PluginValidateID(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	plugin, ok := Deps.PluginGet(id)
	if !ok {
		c.JSON(404, gin.H{"error": "plugin not found"})
		return
	}
	source, _ := Deps.PluginSource(id)
	c.JSON(200, gin.H{"plugin": plugin, "source": source})
}

func HandlePluginUpsert(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, pluginUpsertMaxBodyBytes)
	var req PluginUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	pathID := strings.TrimSpace(c.Param("id"))
	if pathID != "" {
		if err := Deps.PluginValidateID(pathID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			req.ID = pathID
		} else if strings.TrimSpace(req.ID) != pathID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plugin id does not match request path"})
			return
		}
	}
	if err := Deps.PluginValidateID(strings.TrimSpace(req.ID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.PluginUpsert(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandlePluginDelete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := Deps.PluginValidateID(id); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	Deps.PluginUnloadEBPF(id)
	if err := Deps.PluginDelete(id); err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandlePluginToggle(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if err := Deps.PluginValidateID(id); err != nil {
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
	result, err := Deps.PluginSetEnabled(id, req.Enabled)
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	if !req.Enabled {
		Deps.PluginUnloadEBPF(id)
	}
	c.JSON(200, gin.H{"status": "ok", "plugin": result})
}

func HandleBPFTemplates(c *gin.Context) {
	templates := Deps.BPFTemplates()
	c.JSON(200, gin.H{"templates": templates})
}

func HandleBPFCompile(c *gin.Context) {
	var req bpfCompileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.PluginValidateID(req.ID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	objPath, log, err := Deps.CompileUserBPF(req.ID, req.Source)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
			"log":   string(log),
		})
		return
	}
	c.JSON(200, gin.H{"objPath": objPath, "log": string(log)})
}

func HandleBPFLoad(c *gin.Context) {
	var req bpfLoadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not yet implemented"})
}

func HandleBPFUnload(c *gin.Context) {
	var req bpfLoadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not yet implemented"})
}

func RegisterPluginRoutes(rg *gin.RouterGroup) {
	rg.GET("", HandlePluginsList)
	rg.GET("/", HandlePluginsList)
	rg.GET("/:id", HandlePluginGet)
	rg.POST("", HandlePluginUpsert)
	rg.PUT("/:id", HandlePluginUpsert)
	rg.DELETE("/:id", HandlePluginDelete)
	rg.POST("/:id/toggle", HandlePluginToggle)

	bpf := rg.Group("/bpf")
	{
		bpf.GET("/templates", HandleBPFTemplates)
		bpf.POST("/compile", HandleBPFCompile)
		bpf.POST("/load", HandleBPFLoad)
		bpf.POST("/unload", HandleBPFUnload)
	}
}
