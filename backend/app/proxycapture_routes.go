package app

import (
	"agent-ebpf-filter/internal/proxycapture"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Capture intentionally has a separate lifecycle from the persistent runtime
// feature switches: restarting the daemon never silently re-enables a proxy.
var proxyCaptureService = proxycapture.NewManager(proxycapture.DefaultCADir())

func registerProxyCaptureRoutes(r gin.IRouter) {
	r.GET("/proxy-capture/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, proxyCaptureService.Status())
	})
	r.GET("/proxy-capture/flows", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"flows": proxyCaptureService.Flows()})
	})
	r.DELETE("/proxy-capture/flows", func(c *gin.Context) {
		proxyCaptureService.Clear()
		c.Status(http.StatusNoContent)
	})
	r.PUT("/proxy-capture/config", func(c *gin.Context) {
		if !isLocalCaptureController(c) {
			c.JSON(http.StatusForbidden, gin.H{"error":"proxy capture can only be configured by a local controller"})
			return
		}
		var config proxycapture.Config
		if err := c.ShouldBindJSON(&config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
			return
		}
		if err := proxyCaptureService.Configure(config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
			return
		}
		c.JSON(http.StatusOK, proxyCaptureService.Status())
	})
	r.POST("/proxy-capture/start", func(c *gin.Context) {
		if !isLocalCaptureController(c) {
			c.JSON(http.StatusForbidden,gin.H{"error":"proxy capture can only be started locally"})
			return
		}
		var payload struct { Confirm string `json:"confirm"` }
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
			return
		}
		if err := proxyCaptureService.Start(payload.Confirm); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
			return
		}
		c.JSON(http.StatusOK, proxyCaptureService.Status())
	})
	r.POST("/proxy-capture/stop", func(c *gin.Context) {
		if !isLocalCaptureController(c) {
			c.JSON(http.StatusForbidden,gin.H{"error":"proxy capture can only be stopped locally"})
			return
		}
		proxyCaptureService.Stop()
		c.JSON(http.StatusOK, proxyCaptureService.Status())
	})
	r.GET("/proxy-capture/ca", func(c *gin.Context) {
		cert, err := proxyCaptureService.CA()
		if err != nil {
			c.JSON(http.StatusNotFound,gin.H{"error":"capture CA not generated"})
			return
		}
		c.Header("Content-Disposition","attachment; filename=capture-ca.pem")
		c.Header("Cache-Control","no-store")
		c.Data(http.StatusOK,"application/x-pem-file",cert)
	})
	r.POST("/proxy-capture/ca/generate", func(c *gin.Context) {
		if !isLocalCaptureController(c) {
			c.JSON(http.StatusForbidden,gin.H{"error":"CA generation requires a local controller"})
			return
		}
		var payload struct { Confirm string `json:"confirm"` }
		if err := c.ShouldBindJSON(&payload); err != nil || payload.Confirm!="GENERATE_CAPTURE_CA" {
			c.JSON(http.StatusBadRequest,gin.H{"error":"typed CA generation confirmation required"})
			return
		}
		if err := proxyCaptureService.GenerateCA(); err != nil {
			c.JSON(http.StatusBadRequest,gin.H{"error":err.Error()})
			return
		}
		c.JSON(http.StatusOK,proxyCaptureService.Status())
	})
}

// The backend's auth middleware is still mandatory. A bearer token from a
// remote browser is insufficient to create a root-signing identity or open
// a new proxy listener on the backend host.
func isLocalCaptureController(c *gin.Context) bool {
 // Reject browser-originated controls served from a remote WebUI, including
 // when a local reverse proxy obscures the network peer's address.
 if origin := c.GetHeader("Origin"); origin != "" {
  u, err := url.Parse(origin)
  if err != nil {return false}
  host := strings.ToLower(u.Hostname())
  if host!="localhost" {
   ip:=net.ParseIP(host)
   if ip==nil || !ip.IsLoopback() {return false}
  }
 }
 addr := strings.TrimSpace(c.Request.RemoteAddr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil { return addr=="" || addr=="@" || addr=="unix" }
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
