package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

type runtimeReplayCatalog struct {
	Version   string                  `json:"version"`
	Scenarios []runtimeReplayScenario `json:"scenarios"`
}

type runtimeReplayScenario struct {
	ID                       string                    `json:"id"`
	Class                    string                    `json:"class"`
	Description              string                    `json:"description"`
	Seed                     *runtimeReplaySeed        `json:"seed,omitempty"`
	Events                   []runtimeReplayEventInput `json:"events"`
	ExpectedAlertCodes       []string                  `json:"expectedAlertCodes,omitempty"`
	ExpectedWrapperAction    string                    `json:"expectedWrapperAction,omitempty"`
	ExpectContextInheritance bool                      `json:"expectContextInheritance,omitempty"`
}

type runtimeReplaySeed struct {
	WrapperRequest *runtimeReplayWrapperRequest `json:"wrapperRequest,omitempty"`
}

type runtimeReplayWrapperRequest struct {
	PID          uint32   `json:"pid"`
	Comm         string   `json:"comm"`
	Args         []string `json:"args"`
	User         string   `json:"user"`
	ToolName     string   `json:"toolName"`
	AgentRunID   string   `json:"agentRunId"`
	TaskID       string   `json:"taskId"`
	ToolCallID   string   `json:"toolCallId"`
	TraceID      string   `json:"traceId"`
	RootAgentPID uint32   `json:"rootAgentPid"`
	Cwd          string   `json:"cwd"`
}

type runtimeReplayEventInput struct {
	PID          uint32  `json:"pid"`
	PPID         uint32  `json:"ppid"`
	Type         string  `json:"type"`
	Comm         string  `json:"comm"`
	Path         string  `json:"path,omitempty"`
	ExtraPath    string  `json:"extraPath,omitempty"`
	NetEndpoint  string  `json:"netEndpoint,omitempty"`
	NetDirection string  `json:"netDirection,omitempty"`
	ExtraInfo    string  `json:"extraInfo,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	Cwd          string  `json:"cwd,omitempty"`
	Decision     string  `json:"decision,omitempty"`
	RiskScore    float64 `json:"riskScore,omitempty"`
}

type runtimeReplayScenarioResult struct {
	ID                    string   `json:"id"`
	Class                 string   `json:"class"`
	Description           string   `json:"description"`
	ExpectedAlertCodes    []string `json:"expectedAlertCodes,omitempty"`
	ObservedAlertCodes    []string `json:"observedAlertCodes,omitempty"`
	MissingAlertCodes     []string `json:"missingAlertCodes,omitempty"`
	UnexpectedAlertCodes  []string `json:"unexpectedAlertCodes,omitempty"`
	ExpectedWrapperAction string   `json:"expectedWrapperAction,omitempty"`
	ObservedWrapperAction string   `json:"observedWrapperAction,omitempty"`
	ContextChecks         int      `json:"contextChecks"`
	ContextMatches        int      `json:"contextMatches"`
	EventCount            int      `json:"eventCount"`
}

type runtimeReplaySummary struct {
	Version                     string                        `json:"version"`
	GeneratedAt                 string                        `json:"generatedAt"`
	ScenarioCount               int                           `json:"scenarioCount"`
	ClassCounts                 map[string]int                `json:"classCounts"`
	PassedScenarios             int                           `json:"passedScenarios"`
	FailedScenarios             int                           `json:"failedScenarios"`
	ExpectedAlertCount          int                           `json:"expectedAlertCount"`
	ObservedAlertCount          int                           `json:"observedAlertCount"`
	FalsePositiveCount          int                           `json:"falsePositiveCount"`
	FalseNegativeCount          int                           `json:"falseNegativeCount"`
	TraceCorrelationAccuracy    float64                       `json:"traceCorrelationAccuracy"`
	EventLatencyP50Ns           int64                         `json:"eventLatencyP50Ns"`
	EventLatencyP95Ns           int64                         `json:"eventLatencyP95Ns"`
	EventLatencyP99Ns           int64                         `json:"eventLatencyP99Ns"`
	WrapperDecisionLatencyP50Ns int64                         `json:"wrapperDecisionLatencyP50Ns"`
	WrapperDecisionLatencyP95Ns int64                         `json:"wrapperDecisionLatencyP95Ns"`
	WrapperDecisionLatencyP99Ns int64                         `json:"wrapperDecisionLatencyP99Ns"`
	BlockLatencyP50Ns           int64                         `json:"blockLatencyP50Ns"`
	BlockLatencyP95Ns           int64                         `json:"blockLatencyP95Ns"`
	BlockLatencyP99Ns           int64                         `json:"blockLatencyP99Ns"`
	WallDurationNs              int64                         `json:"wallDurationNs"`
	MemoryAllocDeltaBytes       uint64                        `json:"memoryAllocDeltaBytes"`
	RingbufDropRate             float64                       `json:"ringbufDropRate"`
	Notes                       []string                      `json:"notes"`
	Scenarios                   []runtimeReplayScenarioResult `json:"scenarios"`
}

func TestRuntimeReplaySuite(t *testing.T) {
	catalog := loadRuntimeReplayCatalog(t)
	origTracked := trackedProcessContexts
	origSemanticState := semanticAlertsState
	origMLEnabled := mlEnabled
	origMLLoaded := mlModelLoaded
	origMLConfig := mlConfig
	defer func() {
		trackedProcessContexts = origTracked
		semanticAlertsState = origSemanticState
		mlEnabled = origMLEnabled
		mlModelLoaded = origMLLoaded
		mlConfig = origMLConfig
	}()

	mlEnabled = false
	mlModelLoaded = false
	mlConfig = DefaultMLConfig()

	var beforeMem, afterMem runtimeMemStats
	readRuntimeMemStats(&beforeMem)
	start := time.Now()

	results := make([]runtimeReplayScenarioResult, 0, len(catalog.Scenarios))
	classCounts := make(map[string]int)
	eventLatencies := make([]int64, 0, 256)
	wrapperLatencies := make([]int64, 0, len(catalog.Scenarios))
	blockLatencies := make([]int64, 0, len(catalog.Scenarios))

	expectedAlerts := 0
	observedAlerts := 0
	falsePositives := 0
	falseNegatives := 0
	contextChecks := 0
	contextMatches := 0
	failedScenarios := 0

	for _, scenario := range catalog.Scenarios {
		classCounts[scenario.Class]++
		result, metrics := runRuntimeReplayScenario(t, scenario)
		results = append(results, result)

		expectedAlerts += len(result.ExpectedAlertCodes)
		observedAlerts += len(result.ObservedAlertCodes)
		eventLatencies = append(eventLatencies, metrics.eventLatencies...)
		if metrics.wrapperLatency > 0 {
			wrapperLatencies = append(wrapperLatencies, metrics.wrapperLatency)
		}
		if metrics.blockLatency > 0 {
			blockLatencies = append(blockLatencies, metrics.blockLatency)
		}
		contextChecks += result.ContextChecks
		contextMatches += result.ContextMatches

		if result.Class == "benign" {
			falsePositives += len(result.ObservedAlertCodes)
		}
		falseNegatives += len(result.MissingAlertCodes)

		passed := len(result.MissingAlertCodes) == 0
		if result.Class == "benign" && len(result.ObservedAlertCodes) > 0 {
			passed = false
		}
		if strings.TrimSpace(result.ExpectedWrapperAction) != "" && result.ExpectedWrapperAction != result.ObservedWrapperAction {
			passed = false
		}
		if !passed {
			failedScenarios++
		}
	}

	readRuntimeMemStats(&afterMem)
	summary := runtimeReplaySummary{
		Version:                     catalog.Version,
		GeneratedAt:                 time.Now().UTC().Format(time.RFC3339Nano),
		ScenarioCount:               len(results),
		ClassCounts:                 classCounts,
		PassedScenarios:             len(results) - failedScenarios,
		FailedScenarios:             failedScenarios,
		ExpectedAlertCount:          expectedAlerts,
		ObservedAlertCount:          observedAlerts,
		FalsePositiveCount:          falsePositives,
		FalseNegativeCount:          falseNegatives,
		TraceCorrelationAccuracy:    ratio(contextMatches, contextChecks),
		EventLatencyP50Ns:           percentileNs(eventLatencies, 50),
		EventLatencyP95Ns:           percentileNs(eventLatencies, 95),
		EventLatencyP99Ns:           percentileNs(eventLatencies, 99),
		WrapperDecisionLatencyP50Ns: percentileNs(wrapperLatencies, 50),
		WrapperDecisionLatencyP95Ns: percentileNs(wrapperLatencies, 95),
		WrapperDecisionLatencyP99Ns: percentileNs(wrapperLatencies, 99),
		BlockLatencyP50Ns:           percentileNs(blockLatencies, 50),
		BlockLatencyP95Ns:           percentileNs(blockLatencies, 95),
		BlockLatencyP99Ns:           percentileNs(blockLatencies, 99),
		WallDurationNs:              time.Since(start).Nanoseconds(),
		MemoryAllocDeltaBytes:       deltaUint64(afterMem.Alloc, beforeMem.Alloc),
		RingbufDropRate:             0,
		Notes: []string{
			"Offline replay bypasses the live kernel ringbuf, so ringbufDropRate is 0 by construction for this suite.",
			"Wrapper decision latency is measured through resolveAction() with deterministic non-ML inputs.",
			"Context correlation accuracy checks that child events inherit agent_run_id/task_id/tool_call_id/trace_id/root_agent_pid via enrichEventContext().",
		},
		Scenarios: results,
	}

	writeRuntimeReplaySummary(t, summary)

	if failedScenarios > 0 || falsePositives > 0 || falseNegatives > 0 {
		t.Fatalf("runtime replay suite found regressions: failedScenarios=%d falsePositives=%d falseNegatives=%d", failedScenarios, falsePositives, falseNegatives)
	}
}

type runtimeReplayScenarioMetrics struct {
	eventLatencies []int64
	wrapperLatency int64
	blockLatency   int64
}

func runRuntimeReplayScenario(t *testing.T, scenario runtimeReplayScenario) (runtimeReplayScenarioResult, runtimeReplayScenarioMetrics) {
	t.Helper()
	trackedProcessContexts = newProcessContextStore()
	resetSemanticAlertState()

	result := runtimeReplayScenarioResult{
		ID:                 scenario.ID,
		Class:              scenario.Class,
		Description:        scenario.Description,
		ExpectedAlertCodes: append([]string(nil), scenario.ExpectedAlertCodes...),
		EventCount:         len(scenario.Events),
	}
	metrics := runtimeReplayScenarioMetrics{eventLatencies: make([]int64, 0, len(scenario.Events))}

	var seedReq *pb.WrapperRequest
	if scenario.Seed != nil && scenario.Seed.WrapperRequest != nil {
		seedReq = scenario.Seed.WrapperRequest.toProto()
		start := time.Now()
		action := simulateWrapperDecision(seedReq)
		metrics.wrapperLatency = time.Since(start).Nanoseconds()
		result.ObservedWrapperAction = action
		result.ExpectedWrapperAction = strings.TrimSpace(scenario.ExpectedWrapperAction)
		seedCtx := buildProcessContextFromWrapperRequest(seedReq, action, 0.1)
		trackedProcessContexts.Set(seedReq.Pid, seedCtx)
	}

	observedCodes := make(map[string]struct{})
	firstAlertLatency := int64(0)
	for index, raw := range scenario.Events {
		event := enrichEventContext(raw.toProto())
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: time.Now().UTC(),
			Event:      event,
		})
		eventStart := time.Now()
		alerts := buildSemanticAlerts(record.Event)
		latency := time.Since(eventStart).Nanoseconds()
		metrics.eventLatencies = append(metrics.eventLatencies, latency)
		if len(alerts) > 0 && firstAlertLatency == 0 {
			firstAlertLatency = latency
		}

		for _, alert := range alerts {
			code := strings.TrimSpace(alert.GetComm())
			if code != "" {
				observedCodes[code] = struct{}{}
			}
		}

		if scenario.ExpectContextInheritance && seedReq != nil && raw.PID != seedReq.Pid {
			result.ContextChecks++
			if contextMatchesSeed(record.Envelope, seedReq) {
				result.ContextMatches++
			} else {
				t.Logf("context mismatch in scenario %s event %d: envelope=%+v seed=%+v", scenario.ID, index, record.Envelope, seedReq)
			}
		}
	}

	result.ObservedAlertCodes = sortedSetKeys(observedCodes)
	result.MissingAlertCodes = missingStrings(scenario.ExpectedAlertCodes, result.ObservedAlertCodes)
	if scenario.Class == "benign" {
		result.UnexpectedAlertCodes = append(result.UnexpectedAlertCodes, result.ObservedAlertCodes...)
	}
	if firstAlertLatency > 0 {
		metrics.blockLatency = firstAlertLatency
	} else if metrics.wrapperLatency > 0 && result.ObservedWrapperAction != "" && result.ObservedWrapperAction != "ALLOW" {
		metrics.blockLatency = metrics.wrapperLatency
	}
	return result, metrics
}

func loadRuntimeReplayCatalog(t *testing.T) runtimeReplayCatalog {
	t.Helper()
	path := filepath.Join("..", "benchmarks", "runtime-replay", "scenarios.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime replay catalog %s: %v", path, err)
	}
	var catalog runtimeReplayCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse runtime replay catalog %s: %v", path, err)
	}
	if len(catalog.Scenarios) == 0 {
		t.Fatalf("runtime replay catalog %s is empty", path)
	}
	return catalog
}

func (r *runtimeReplayWrapperRequest) toProto() *pb.WrapperRequest {
	if r == nil {
		return nil
	}
	return &pb.WrapperRequest{
		Pid:          r.PID,
		Comm:         r.Comm,
		Args:         append([]string(nil), r.Args...),
		User:         r.User,
		ToolName:     r.ToolName,
		AgentRunId:   r.AgentRunID,
		TaskId:       r.TaskID,
		ToolCallId:   r.ToolCallID,
		TraceId:      r.TraceID,
		RootAgentPid: r.RootAgentPID,
		Cwd:          r.Cwd,
	}
}

func (e runtimeReplayEventInput) toProto() *pb.Event {
	return &pb.Event{
		Pid:          e.PID,
		Ppid:         e.PPID,
		Type:         e.Type,
		Comm:         e.Comm,
		Path:         e.Path,
		ExtraPath:    e.ExtraPath,
		NetEndpoint:  e.NetEndpoint,
		NetDirection: e.NetDirection,
		ExtraInfo:    e.ExtraInfo,
		Mode:         e.Mode,
		Cwd:          e.Cwd,
		Decision:     e.Decision,
		RiskScore:    e.RiskScore,
	}
}

func simulateWrapperDecision(req *pb.WrapperRequest) string {
	if req == nil {
		return ""
	}
	classification := ClassifyBehavior(req.GetComm(), req.GetArgs())
	action, _ := resolveAction(req, "", 0, classification, 0, Prediction{}, DefaultMLConfig())
	return actionLabel[int32(action)]
}

func contextMatchesSeed(envelope *pb.EventEnvelope, seed *pb.WrapperRequest) bool {
	if envelope == nil || seed == nil {
		return false
	}
	rootAgentPID := uint32(0)
	if legacy := envelope.GetLegacyEvent(); legacy != nil {
		rootAgentPID = legacy.GetRootAgentPid()
	}
	return rootAgentPID == seed.GetRootAgentPid() &&
		envelope.GetAgentRunId() == seed.GetAgentRunId() &&
		envelope.GetTaskId() == seed.GetTaskId() &&
		envelope.GetToolCallId() == seed.GetToolCallId() &&
		envelope.GetTraceId() == seed.GetTraceId() &&
		envelope.GetToolName() == firstNonEmpty(seed.GetToolName(), seed.GetComm()) &&
		envelope.GetCwd() == seed.GetCwd()
}

func missingStrings(expected, observed []string) []string {
	seen := make(map[string]struct{}, len(observed))
	for _, item := range observed {
		seen[item] = struct{}{}
	}
	missing := make([]string, 0)
	for _, item := range expected {
		if _, ok := seen[item]; !ok {
			missing = append(missing, item)
		}
	}
	return missing
}

func sortedSetKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func percentileNs(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted) - 1) * percentile / 100
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

type runtimeMemStats struct {
	Alloc uint64
}

func readRuntimeMemStats(out *runtimeMemStats) {
	if out == nil {
		return
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	out.Alloc = stats.Alloc
}

func writeRuntimeReplaySummary(t *testing.T, summary runtimeReplaySummary) {
	t.Helper()
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime replay summary: %v", err)
	}
	target := strings.TrimSpace(os.Getenv("RUNTIME_REPLAY_OUT"))
	if target == "" {
		t.Logf("runtime replay summary:\n%s", string(data))
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatalf("mkdir runtime replay report dir: %v", err)
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatalf("write runtime replay summary: %v", err)
	}
	t.Logf("runtime replay summary written to %s", target)
}
