package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func registerDshInspectorRoutes(router gin.IRouter, ac *AppContext) {
	group := router.Group("/dsh/inspector")
	group.GET("/status", func(c *gin.Context) {
		if ac == nil || ac.DshInspector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dsh inspector integration is unavailable"})
			return
		}
		c.JSON(http.StatusOK, ac.DshInspector.Status())
	})
	group.POST("/connect", func(c *gin.Context) {
		if ac == nil || ac.DshInspector == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dsh inspector integration is unavailable"})
			return
		}
		var req struct {
			Port int `json:"port"`
		}
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
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
