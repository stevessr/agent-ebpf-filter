package app

import (
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section suite_benchmark.go ----

// ── Benchmark types ───────────────────────────────────────────────────

type benchmarkCase = core.BenchmarkCase

var benchmarkCases = core.DefaultBenchmarkCases()

type benchmarkResult struct {
	Case      benchmarkCase `json:"case"`
	Passed    bool          `json:"passed"`
	Actual    string        `json:"actual"`
	RiskScore float64       `json:"riskScore"`
	Alerts    []string      `json:"alerts,omitempty"`
	LatencyUs int64         `json:"latencyUs"`
	MatchedAt string        `json:"matchedAt"`
	MatchedBy string        `json:"matchedBy"`
}

type benchmarkRun struct {
	Name        string            `json:"name"`
	StartedAt   time.Time         `json:"startedAt"`
	CompletedAt time.Time         `json:"completedAt"`
	TotalCases  int               `json:"totalCases"`
	Passed      int               `json:"passed"`
	Failed      int               `json:"failed"`
	FalsePos    int               `json:"falsePos"`
	FalseNeg    int               `json:"falseNeg"`
	Results     []benchmarkResult `json:"results"`
}

type benchmarkStats struct {
	TotalRuns     int                     `json:"totalRuns"`
	OverallPass   float64                 `json:"overallPassRate"`
	FalsePosRate  float64                 `json:"falsePositiveRate"`
	FalseNegRate  float64                 `json:"falseNegativeRate"`
	P50LatencyUs  float64                 `json:"p50LatencyUs"`
	P95LatencyUs  float64                 `json:"p95LatencyUs"`
	P99LatencyUs  float64                 `json:"p99LatencyUs"`
	AvgRiskDiff   float64                 `json:"avgRiskDiff"`
	CoverageBy    map[string]float64      `json:"coverageByCategory"`
	CategoryStats map[string]categoryStat `json:"categoryStats"`
}

type categoryStat struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	PassRate float64 `json:"passRate"`
}

// ── Benchmark cases ───────────────────────────────────────────────────

// ── Benchmark engine ──────────────────────────────────────────────────

type benchmarkEngine struct {
	runMu  sync.Mutex
	mu     sync.Mutex
	runner atomic.Int32
	runs   []benchmarkRun
}

const benchmarkRunHistoryLimit = 100

var benchmarkEngineStore = newBenchmarkEngine()

func newBenchmarkEngine() *benchmarkEngine {
	return &benchmarkEngine{}
}

func (e *benchmarkEngine) runAll() benchmarkRun {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	e.runner.Add(1)
	run := benchmarkRun{
		Name:       fmt.Sprintf("benchmark-%d", e.runner.Load()),
		StartedAt:  time.Now().UTC(),
		TotalCases: len(benchmarkCases),
		Results:    make([]benchmarkResult, 0, len(benchmarkCases)),
	}

	results := make([]benchmarkResult, len(benchmarkCases))
	workers := benchmarkWorkerCount(len(benchmarkCases), runtime.GOMAXPROCS(0))
	jobs := make(chan int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				results[index] = e.evaluateCase(benchmarkCases[index])
			}
		}()
	}
	for index := range benchmarkCases {
		jobs <- index
	}
	close(jobs)
	wg.Wait()

	for _, r := range results {
		run.Results = append(run.Results, r)
		if r.Passed {
			run.Passed++
		} else {
			run.Failed++
			if r.Case.Expected == "ALLOW" && r.Actual != "ALLOW" {
				run.FalsePos++
			}
			if r.Case.Expected != "ALLOW" && r.Actual == "ALLOW" {
				run.FalseNeg++
			}
		}
	}

	run.CompletedAt = time.Now().UTC()
	sort.Slice(run.Results, func(i, j int) bool {
		return run.Results[i].Case.Category < run.Results[j].Case.Category
	})

	e.storeRun(run)

	return run
}

func benchmarkWorkerCount(caseCount, cpuCount int) int {
	if caseCount <= 0 {
		return 0
	}
	if cpuCount < 1 {
		cpuCount = 1
	}
	if cpuCount > caseCount {
		cpuCount = caseCount
	}
	return cpuCount
}

func (e *benchmarkEngine) storeRun(run benchmarkRun) {
	e.mu.Lock()
	e.runs = append(e.runs, run)
	if overflow := len(e.runs) - benchmarkRunHistoryLimit; overflow > 0 {
		copy(e.runs, e.runs[overflow:])
		e.runs = e.runs[:benchmarkRunHistoryLimit]
	}
	e.mu.Unlock()
}

func (e *benchmarkEngine) runsSnapshot() []benchmarkRun {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]benchmarkRun(nil), e.runs...)
}

func (e *benchmarkEngine) evaluateCase(bc benchmarkCase) benchmarkResult {
	start := time.Now()

	// Build a synthetic event from the benchmark case
	event := buildBenchmarkEvent(bc)

	// Classify behavior
	classification := behavior.ClassifyBehavior(bc.Comm, bc.Args)

	// Enrich with event context
	event.Behavior = classification
	event = enrichEventContext(event)

	// Build semantic alerts
	alerts := buildSemanticAlerts(event)

	// Determine actual decision
	actual := "ALLOW"
	alertCodes := make([]string, 0)
	maxRisk := 0.0
	for _, alert := range alerts {
		alertCodes = append(alertCodes, alert.GetComm())
		if alert.GetRiskScore() > maxRisk {
			maxRisk = alert.GetRiskScore()
		}
	}
	if maxRisk >= 0.90 {
		actual = "ALERT"
	} else if maxRisk >= 0.70 {
		actual = "ALERT"
	}

	latency := time.Since(start).Microseconds()

	passed := actual == bc.Expected
	if !passed && actual == "ALERT" && bc.Expected == "ALERT" {
		passed = true // Conservative: alert for expected-alert is always OK
	}

	matchedBy := "rule"
	if classification != nil && classification.GetPrimaryCategory() != "" && classification.GetPrimaryCategory() != "UNKNOWN" {
		matchedBy = "behavior_classifier"
	}
	if len(alerts) > 0 {
		matchedBy = "semantic_alerts"
	}

	return benchmarkResult{
		Case:      bc,
		Passed:    passed,
		Actual:    actual,
		RiskScore: maxRisk,
		Alerts:    alertCodes,
		LatencyUs: latency,
		MatchedAt: time.Now().UTC().Format(time.RFC3339Nano),
		MatchedBy: matchedBy,
	}
}

func buildBenchmarkEvent(bc benchmarkCase) *pb.Event {
	event := &pb.Event{
		Type:          bc.EventType,
		EventType:     pb.EventType_EXECVE,
		Comm:          bc.Comm,
		Path:          bc.Path,
		ToolName:      bc.ToolName,
		NetEndpoint:   bc.NetEndpoint,
		SchemaVersion: eventSchemaVersion,
		Pid:           1000 + uint32(hashString(bc.Name)%10000),
		Ppid:          100,
		Uid:           1000,
		Gid:           1000,
		Cwd:           "/home/user/project",
	}

	// Map event type to proto EventType
	for name, et := range map[string]pb.EventType{
		"execve":          pb.EventType_EXECVE,
		"openat":          pb.EventType_OPENAT,
		"network_connect": pb.EventType_NETWORK_CONNECT,
		"network_sendto":  pb.EventType_NETWORK_SENDTO,
		"chmod":           pb.EventType_CHMOD,
		"write":           pb.EventType_WRITE,
		"process_fork":    pb.EventType_SCHED_PROCESS_FORK,
	} {
		if bc.EventType == name {
			event.EventType = et
			break
		}
	}

	// Set net direction
	if bc.NetEndpoint != "" {
		event.NetDirection = "outgoing"
	}

	// Add tool context for agentic cases
	if bc.Category == "agentic" {
		event.AgentRunId = "benchmark-run-001"
		event.TaskId = "benchmark-task-001"
		event.ToolCallId = "benchmark-tool-" + bc.Name
		event.ToolName = bc.ToolName
	}

	return event
}

func hashString(s string) uint32 {
	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}
