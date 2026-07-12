package handlers

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	pluginUpsertMaxBodyBytes  int64 = 320 << 10
	pluginCompileMaxBodyBytes int64 = 320 << 10
	pluginControlMaxBodyBytes int64 = 64 << 10
)

func bindPluginJSON(c *gin.Context, target any, maxBytes int64) (int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

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
	ID string `json:"id"`
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
	var req PluginUpsertRequest
	if status, err := bindPluginJSON(c, &req, pluginUpsertMaxBodyBytes); err != nil {
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
	plugin, err := Deps.PluginUpsert(&req)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"plugin": plugin})
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
	if status, err := bindPluginJSON(c, &req, pluginControlMaxBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if _, ok := Deps.PluginGet(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}
	result, err := Deps.PluginSetEnabled(c.Request.Context(), id, req.Enabled)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error(), "plugin": result})
		return
	}
	c.JSON(200, gin.H{"status": "ok", "plugin": result})
}

func HandleBPFTemplates(c *gin.Context) {
	templates := Deps.BPFTemplates()
	c.JSON(200, gin.H{"templates": templates})
}

func HandleBPFCompile(c *gin.Context) {
	var req bpfCompileRequest
	if status, err := bindPluginJSON(c, &req, pluginCompileMaxBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	if err := Deps.PluginValidateID(req.ID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	objPath, log, err := Deps.CompileUserBPF(c.Request.Context(), req.ID, req.Source)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
			"log":   string(log),
		})
		return
	}
	sourceDigest := sha256.Sum256([]byte(req.Source))
	c.JSON(200, gin.H{
		"objectPath":   objPath,
		"sourceSha256": fmt.Sprintf("%x", sourceDigest),
		"log":          string(log),
		"compiledAt":   time.Now().UTC(),
	})
}

func HandleBPFLoad(c *gin.Context) {
	var req bpfLoadRequest
	if status, err := bindPluginJSON(c, &req, pluginControlMaxBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if err := Deps.PluginValidateID(req.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, ok := Deps.PluginGet(req.ID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}
	plugin, err := Deps.PluginLoadEBPF(c.Request.Context(), req.ID)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugin": plugin})
}

func HandleBPFUnload(c *gin.Context) {
	var req bpfLoadRequest
	if status, err := bindPluginJSON(c, &req, pluginControlMaxBodyBytes); err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if err := Deps.PluginValidateID(req.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, ok := Deps.PluginGet(req.ID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
		return
	}
	Deps.PluginUnloadEBPF(req.ID)
	plugin, _ := Deps.PluginGet(req.ID)
	c.JSON(http.StatusOK, gin.H{"plugin": plugin})
}

func RegisterPluginRoutes(rg *gin.RouterGroup, policyMiddleware gin.HandlerFunc) {
	rg.GET("", HandlePluginsList)
	rg.GET("/", HandlePluginsList)
	rg.GET("/:id", HandlePluginGet)
	rg.POST("", policyMiddleware, HandlePluginUpsert)
	rg.PUT("/:id", policyMiddleware, HandlePluginUpsert)
	rg.DELETE("/:id", policyMiddleware, HandlePluginDelete)
	rg.POST("/:id/toggle", policyMiddleware, HandlePluginToggle)

	bpf := rg.Group("/bpf")
	{
		bpf.GET("/templates", HandleBPFTemplates)
		bpf.POST("/compile", policyMiddleware, HandleBPFCompile)
		bpf.POST("/load", policyMiddleware, HandleBPFLoad)
		bpf.POST("/unload", policyMiddleware, HandleBPFUnload)
	}
}
