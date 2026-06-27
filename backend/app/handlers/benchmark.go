package handlers

import (
	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlers_benchmark.go ----

func HandleRunBenchmark(c *gin.Context) {
	run, stats := Deps.RunBenchmark()
	c.JSON(200, gin.H{
		"run":   run,
		"stats": stats,
	})
}

func HandleGetBenchmarkResults(c *gin.Context) {
	c.JSON(200, Deps.GetBenchmarkResults())
}