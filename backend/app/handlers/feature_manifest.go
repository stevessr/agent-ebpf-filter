package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandleSystemFeatures returns the feature manifest.
func HandleSystemFeatures(c *gin.Context) {
	settings := Deps.RuntimeSettings.Snapshot()
	c.JSON(http.StatusOK, Deps.BuildFeatureManifest(settings))
}
