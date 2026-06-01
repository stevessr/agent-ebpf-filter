package app

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlers_benchmark.go ----

var (
	benchmarkEngineInst    = newBenchmarkEngine()
	benchmarkEngineInstMu  sync.Mutex
	latestBenchmarkStats   benchmarkStats
	latestBenchmarkStatsMu sync.RWMutex
)

func handleRunBenchmark(c *gin.Context) {
	benchmarkEngineInstMu.Lock()
	engine := benchmarkEngineInst
	benchmarkEngineInstMu.Unlock()

	run := engine.runAll()
	stats := computeBenchmarkStats(engine.runs)

	latestBenchmarkStatsMu.Lock()
	latestBenchmarkStats = stats
	latestBenchmarkStatsMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"run":   run,
		"stats": stats,
	})
}

func handleGetBenchmarkResults(c *gin.Context) {
	latestBenchmarkStatsMu.RLock()
	stats := latestBenchmarkStats
	latestBenchmarkStatsMu.RUnlock()

	c.JSON(http.StatusOK, stats)
}
