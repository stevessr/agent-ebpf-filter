package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section engine_benchmark.go ----

// ── Statistics ────────────────────────────────────────────────────────

func computeBenchmarkStats(runs []benchmarkRun) benchmarkStats {
	stats := benchmarkStats{
		TotalRuns:     len(runs),
		CoverageBy:    make(map[string]float64),
		CategoryStats: make(map[string]categoryStat),
	}

	if len(runs) == 0 {
		return stats
	}

	var totalPassed, totalCases int
	var totalFP, totalFN int
	var allLatencies []int64
	totalRiskDiff := 0.0

	for _, run := range runs {
		totalPassed += run.Passed
		totalCases += run.TotalCases
		totalFP += run.FalsePos
		totalFN += run.FalseNeg

		for _, r := range run.Results {
			allLatencies = append(allLatencies, r.LatencyUs)
			totalRiskDiff += r.RiskScore

			cat := r.Case.Category
			cs := stats.CategoryStats[cat]
			cs.Total++
			if r.Passed {
				cs.Passed++
			}
			stats.CategoryStats[cat] = cs
		}
	}

	if totalCases > 0 {
		stats.OverallPass = float64(totalPassed) / float64(totalCases) * 100
		stats.FalsePosRate = float64(totalFP) / float64(totalCases) * 100
		stats.FalseNegRate = float64(totalFN) / float64(totalCases) * 100
	}

	if len(allLatencies) > 0 {
		sort.Slice(allLatencies, func(i, j int) bool {
			return allLatencies[i] < allLatencies[j]
		})
		stats.P50LatencyUs = float64(allLatencies[len(allLatencies)*50/100])
		stats.P95LatencyUs = float64(allLatencies[len(allLatencies)*95/100])
		stats.P99LatencyUs = float64(allLatencies[len(allLatencies)*99/100])
		stats.AvgRiskDiff = totalRiskDiff / float64(len(allLatencies))
	}

	for cat, cs := range stats.CategoryStats {
		if cs.Total > 0 {
			cs.PassRate = float64(cs.Passed) / float64(cs.Total) * 100
		}
		stats.CategoryStats[cat] = cs
		stats.CoverageBy[cat] = float64(cs.Total) / float64(len(benchmarkCases)) * 100
	}

	return stats
}

// ── CLI and export ─────────────────────────────────────────────────────

func runBenchmarkSuite() error {
	fmt.Println("Agent eBPF Filter - Benchmark Suite")
	fmt.Println(strings.Repeat("=", 60))

	engine := newBenchmarkEngine()
	run := engine.runAll()

	fmt.Printf("\nResults: %d/%d passed (%.1f%%)\n",
		run.Passed, run.TotalCases,
		float64(run.Passed)/float64(run.TotalCases)*100)
	fmt.Printf("False positives: %d, False negatives: %d\n",
		run.FalsePos, run.FalseNeg)

	fmt.Println("\nBy category:")
	stats := computeBenchmarkStats(engine.runs)
	for cat, cs := range stats.CategoryStats {
		fmt.Printf("  %s: %d/%d (%.1f%%)\n", cat, cs.Passed, cs.Total, cs.PassRate)
	}

	fmt.Printf("\nLatency: p50=%.0fus p95=%.0fus p99=%.0fus\n",
		stats.P50LatencyUs, stats.P95LatencyUs, stats.P99LatencyUs)

	// Export results to JSON
	exportData := map[string]interface{}{
		"run":   run,
		"stats": stats,
	}
	exportPath := defaultExportPath()
	if data, err := json.MarshalIndent(exportData, "", "  "); err == nil {
		os.WriteFile(exportPath, data, 0644)
		fmt.Printf("\nResults exported to %s\n", exportPath)
	}

	return nil
}

func defaultExportPath() string {
	path := runtimeSettingsDir()
	return path + "/benchmark-results.json"
}

// ── Continuous benchmark runner ───────────────────────────────────────

func startContinuousBenchmark(interval time.Duration) chan benchmarkStats {
	statsChan := make(chan benchmarkStats, 16)
	engine := newBenchmarkEngine()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run initial benchmark
		engine.runAll()
		stats := computeBenchmarkStats(engine.runs)
		select {
		case statsChan <- stats:
		default:
		}

		for range ticker.C {
			engine.runAll()
			stats := computeBenchmarkStats(engine.runs)
			select {
			case statsChan <- stats:
			default:
			}
		}
	}()

	return statsChan
}
