package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func registerDshInspectorRoutes(router gin.IRouter, ac *AppContext, store *TLSCaptureStore) {
	group := router.Group("/dsh/inspector")
	group.GET("/status", func(c *gin.Context) {
		if ac == nil || ac.DshInspector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dsh inspector integration is unavailable"})
			return
		}
		c.JSON(http.StatusOK, ac.DshInspector.Status())
	})
	group.GET("/events", func(c *gin.Context) {
		limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
		if err != nil || limit < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		if limit > 500 {
			limit = 500
		}
		if store == nil {
			c.JSON(http.StatusOK, gin.H{"events": []any{}})
			return
		}
		// Pull a wider tail because the shared userspace capture store may also
		// contain Codex/eBPF records; return only the dsh Inspector source.
		candidates := store.Recent(limit * 4)
		events := make([]any, 0, limit)
		for _, event := range candidates {
			if event.CaptureSource != "dsh_inspector" {
				continue
			}
			events = append(events, event)
			if len(events) >= limit {
				break
			}
		}
		c.JSON(http.StatusOK, gin.H{"events": events})
	})
	group.POST("/connect", func(c *gin.Context) {
		if ac == nil || ac.DshInspector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dsh inspector integration is unavailable"})
			return
		}
		var req struct {
			Port int `json:"port"`
		}
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Port < 0 || req.Port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "port must be 0 (auto-discover) or 1..65535"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		if err := ac.DshInspector.DiscoverAndConnect(ctx, req.Port); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "status": ac.DshInspector.Status()})
			return
		}
		c.JSON(http.StatusOK, ac.DshInspector.Status())
	})
	group.POST("/disconnect", func(c *gin.Context) {
		if ac == nil || ac.DshInspector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dsh inspector integration is unavailable"})
			return
		}
		ac.DshInspector.Close()
		c.JSON(http.StatusOK, ac.DshInspector.Status())
	})
}
