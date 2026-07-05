package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/behavior"
)

const (
	researchSecurityEvaluationSchemaVersion = "research-security-evaluation.v1"
	researchSecurityEvaluationDefaultMode   = "combined"
	researchSecurityEvaluationModeBuiltin   = "builtin"
	researchSecurityEvaluationModeSession   = "session"
	researchSecurityEvaluationModeCombined  = "combined"
	researchSecurityEvaluationLabelDecision = "decision_then_heuristic"
	researchSecurityEvaluationMaxFindings   = 200
)

type ResearchSecurityEvaluationRequest struct {
	Mode         string               `json:"mode,omitempty"`
	LabelPolicy  string               `json:"labelPolicy,omitempty"`
	Limit        int                  `json:"limit,omitempty"`
	IncludeLLM   bool                 `json:"includeLLM,omitempty"`
	SourceFilter ResearchSourceFilter `json:"sourceFilter,omitempty"`
	TimeRange    ResearchTimeRange    `json:"timeRange,omitempty"`
}

type ResearchSecurityEvaluationReport struct {
	SchemaVersion   string                                `json:"schemaVersion"`
	SessionID       string                                `json:"sessionId"`
	GeneratedAt     time.Time                             `json:"generatedAt"`
	Mode            string                                `json:"mode"`
	LabelPolicy     string                                `json:"labelPolicy"`
	IncludeLLM      bool                                  `json:"includeLLM"`
	Totals          ResearchSecurityEvaluationTotals      `json:"totals"`
	Metrics         ResearchSecurityEvaluationMetrics     `json:"metrics"`
	ConfusionMatrix map[string]map[string]int             `json:"confusionMatrix"`
	ByCategory      []ResearchSecurityEvaluationGroup     `json:"byCategory"`
	ByCommand       []ResearchSecurityEvaluationGroup     `json:"byCommand"`
	BySource        []ResearchSecurityEvaluationGroup     `json:"bySource"`
	RiskBuckets     []researchCount                       `json:"riskBuckets"`
	Posture         ResearchSecurityEvaluationPosture     `json:"posture"`
	Findings        ResearchSecurityEvaluationFindings    `json:"findings"`
	Samples         []ResearchSecurityEvaluationSampleRow `json:"samples,omitempty"`
}

type ResearchSecurityEvaluationTotals struct {
	Total     int `json:"total"`
	Labeled   int `json:"labeled"`
	Benign    int `json:"benign"`
	Risky     int `json:"risky"`
	Unlabeled int `json:"unlabeled"`
	Skipped   int `json:"skipped"`
	Builtin   int `json:"builtin"`
	Session   int `json:"session"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
}

type ResearchSecurityEvaluationMetrics struct {
	Accuracy          float64 `json:"accuracy"`
	Precision         float64 `json:"precision"`
	Recall            float64 `json:"recall"`
	AllowRecall       float64 `json:"allowRecall"`
	AlertRecall       float64 `json:"alertRecall"`
	BlockRecall       float64 `json:"blockRecall"`
	FalsePositiveRate float64 `json:"falsePositiveRate"`
	FalseNegativeRate float64 `json:"falseNegativeRate"`
	BalancedAccuracy  float64 `json:"balancedAccuracy"`
}

type ResearchSecurityEvaluationGroup struct {
	Key            string  `json:"key"`
	Total          int     `json:"total"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	FalsePositives int     `json:"falsePositives"`
	FalseNegatives int     `json:"falseNegatives"`
	AvgRiskScore   float64 `json:"avgRiskScore"`
}

type ResearchSecurityEvaluationPosture struct {
	Status               string                            `json:"status"`
	RiskScore            float64                           `json:"riskScore"`
	FindingCounts        []researchCount                   `json:"findingCounts,omitempty"`
	BlockingReasons      []string                          `json:"blockingReasons,omitempty"`
	Warnings             []string                          `json:"warnings,omitempty"`
	SuggestedActions     []string                          `json:"suggestedActions,omitempty"`
	TopFailingCategories []ResearchSecurityEvaluationGroup `json:"topFailingCategories,omitempty"`
}

type ResearchSecurityEvaluationFindings struct {
	FalsePositives              []ResearchSecurityEvaluationSampleRow `json:"falsePositives,omitempty"`
	FalseNegatives              []ResearchSecurityEvaluationSampleRow `json:"falseNegatives,omitempty"`
	PolicyGaps                  []ResearchSecurityEvaluationSampleRow `json:"policyGaps,omitempty"`
	HighConfidenceDisagreements []ResearchSecurityEvaluationSampleRow `json:"highConfidenceDisagreements,omitempty"`
	UnlabeledHighRisk           []ResearchSecurityEvaluationSampleRow `json:"unlabeledHighRisk,omitempty"`
}

type ResearchSecurityEvaluationSampleRow struct {
	ID              string         `json:"id"`
	EventID         string         `json:"eventId,omitempty"`
	Timestamp       int64          `json:"timestamp,omitempty"`
	Time            string         `json:"time,omitempty"`
	Source          string         `json:"source"`
	EventType       string         `json:"eventType,omitempty"`
	Category        string         `json:"category,omitempty"`
	Comm            string         `json:"comm"`
	CommandLine     string         `json:"commandLine"`
	Args            []string       `json:"args,omitempty"`
	Target          string         `json:"target,omitempty"`
	ExpectedAction  string         `json:"expectedAction"`
	ExpectedSource  string         `json:"expectedSource"`
	ObservedAction  string         `json:"observedAction"`
	Passed          bool           `json:"passed"`
	FindingType     string         `json:"findingType,omitempty"`
	RiskScore       float64        `json:"riskScore"`
	RiskLevel       string         `json:"riskLevel,omitempty"`
	Confidence      float64        `json:"confidence,omitempty"`
	Reasoning       string         `json:"reasoning,omitempty"`
	Recommendation  string         `json:"recommendation,omitempty"`
	RedactionLevel  string         `json:"redactionLevel,omitempty"`
	TraceID         string         `json:"traceId,omitempty"`
	SpanID          string         `json:"spanId,omitempty"`
	Signals         map[string]any `json:"signals,omitempty"`
	BenchmarkCase   string         `json:"benchmarkCase,omitempty"`
	BenchmarkTool   string         `json:"benchmarkTool,omitempty"`
	BenchmarkDetail string         `json:"benchmarkDetail,omitempty"`
}

type researchSecurityEvaluationCandidate struct {
	ID              string
	EventID         string
	Timestamp       int64
	Time            string
	Source          string
	EventType       string
	Category        string
	Comm            string
	CommandLine     string
	Args            []string
	Target          string
	ExpectedAction  string
	ExpectedSource  string
	RedactionLevel  string
	TraceID         string
	SpanID          string
	BenchmarkCase   string
	BenchmarkTool   string
	BenchmarkDetail string
}

type researchSecurityEvaluationAccumulator struct {
	total          int
	passed         int
	falsePositives int
	falseNegatives int
	riskSum        float64
}

type researchSecurityEvaluationCounters struct {
	tp               int
	tn               int
	fp               int
	fn               int
	expectedAllow    int
	expectedRisky    int
	expectedAlert    int
	alertMatched     int
	expectedBlock    int
	blockMatched     int
	strictCorrect    int
	labeledEvaluated int
}

func researchSecurityEvaluationRequestFromTask(req researchTaskRequest) ResearchSecurityEvaluationRequest {
	out := ResearchSecurityEvaluationRequest{
		Mode:         req.EvaluationMode,
		LabelPolicy:  req.LabelPolicy,
		Limit:        req.Limit,
		IncludeLLM:   req.IncludeLLM,
		SourceFilter: req.SourceFilter,
		TimeRange:    req.TimeRange,
	}
	if len(req.Params) > 0 {
		out.Mode = firstNonEmptyResearchSecurityParam(req.Params, out.Mode, "mode", "evaluationMode", "corpus", "source")
		out.LabelPolicy = firstNonEmptyResearchSecurityParam(req.Params, out.LabelPolicy, "labelPolicy", "label_policy")
		if limit, ok := researchSecurityParamInt(req.Params, "limit", "maxSamples", "max_samples"); ok {
			out.Limit = limit
		}
		if includeLLM, ok := researchSecurityParamBool(req.Params, "includeLLM", "include_llm", "llm"); ok {
			out.IncludeLLM = includeLLM
		}
	}
	out.Mode = normalizeResearchSecurityEvaluationMode(out.Mode)
	out.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(out.LabelPolicy)
	out.SourceFilter = normalizeResearchSourceFilter(out.SourceFilter)
	out.TimeRange = normalizeResearchTimeRange(out.TimeRange)
	return out
}

func buildResearchSecurityEvaluationReport(sessionID string, events []ResearchEvent, req ResearchSecurityEvaluationRequest, entry *researchTaskEntry) (ResearchSecurityEvaluationReport, error) {
	req.Mode = normalizeResearchSecurityEvaluationMode(req.Mode)
	req.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(req.LabelPolicy)
	req.SourceFilter = normalizeResearchSourceFilter(req.SourceFilter)
	req.TimeRange = normalizeResearchTimeRange(req.TimeRange)
	req.Limit = normalizeResearchSecurityEvaluationLimit(req.Limit)

	candidates := make([]researchSecurityEvaluationCandidate, 0, len(benchmarkCases)+len(events))
	skipped := 0
	if req.Mode == researchSecurityEvaluationModeBuiltin || req.Mode == researchSecurityEvaluationModeCombined {
		for _, bc := range benchmarkCases {
			candidates = append(candidates, researchSecurityCandidateFromBenchmarkCase(bc))
		}
	}
	if req.Mode == researchSecurityEvaluationModeSession || req.Mode == researchSecurityEvaluationModeCombined {
		sessionEvents := filterResearchEventsForSecurityEvaluation(events, req.SourceFilter, req.TimeRange, req.Limit)
		for _, event := range sessionEvents {
			if entry != nil && entry.isCanceled() {
				return ResearchSecurityEvaluationReport{}, errResearchTaskCanceled
			}
			candidate, ok := researchSecurityCandidateFromEvent(event, req.LabelPolicy)
			if !ok {
				skipped++
				continue
			}
			candidates = append(candidates, candidate)
		}
	}

	now := time.Now().UTC()
	report := ResearchSecurityEvaluationReport{
		SchemaVersion:   researchSecurityEvaluationSchemaVersion,
		SessionID:       sessionID,
		GeneratedAt:     now,
		Mode:            req.Mode,
		LabelPolicy:     req.LabelPolicy,
		IncludeLLM:      req.IncludeLLM,
		ConfusionMatrix: make(map[string]map[string]int),
		Samples:         make([]ResearchSecurityEvaluationSampleRow, 0, len(candidates)),
	}
	report.Totals.Skipped = skipped

	byCategory := map[string]*researchSecurityEvaluationAccumulator{}
	byCommand := map[string]*researchSecurityEvaluationAccumulator{}
	bySource := map[string]*researchSecurityEvaluationAccumulator{}
	riskBuckets := map[string]int{}
	counters := researchSecurityEvaluationCounters{}

	for index, candidate := range candidates {
		if entry != nil {
			if index%16 == 0 && entry.isCanceled() {
				return ResearchSecurityEvaluationReport{}, errResearchTaskCanceled
			}
			entry.setProgress(0.05 + 0.85*(float64(index+1)/float64(maxInt(1, len(candidates)))))
		}
		row := evaluateResearchSecurityCandidate(candidate, req.IncludeLLM)
		report.Samples = append(report.Samples, row)
		report.Totals.Total++
		if candidate.Source == "builtin" {
			report.Totals.Builtin++
		} else {
			report.Totals.Session++
		}

		expected := normalizeResearchSecurityAction(row.ExpectedAction)
		observed := normalizeResearchSecurityAction(row.ObservedAction)
		if report.ConfusionMatrix[expected] == nil {
			report.ConfusionMatrix[expected] = map[string]int{}
		}
		report.ConfusionMatrix[expected][observed]++
		incrementResearchCount(riskBuckets, researchSecurityRiskBucket(row.RiskScore))

		labeled := researchSecurityActionIsLabeled(expected)
		if labeled {
			report.Totals.Labeled++
			if expected == "ALLOW" {
				report.Totals.Benign++
			} else {
				report.Totals.Risky++
			}
			updateResearchSecurityCounters(&counters, expected, observed)
			if row.Passed {
				report.Totals.Passed++
			} else {
				report.Totals.Failed++
			}
		} else {
			report.Totals.Unlabeled++
		}
		updateResearchSecurityGroup(byCategory, firstNonEmptyResearchSecurityString(row.Category, "uncategorized"), row)
		updateResearchSecurityGroup(byCommand, firstNonEmptyResearchSecurityString(row.Comm, "unknown"), row)
		updateResearchSecurityGroup(bySource, firstNonEmptyResearchSecurityString(row.Source, "unknown"), row)
		appendResearchSecurityFindings(&report.Findings, row)
	}

	report.Metrics = computeResearchSecurityEvaluationMetrics(counters)
	report.ByCategory = finishResearchSecurityGroups(byCategory)
	report.ByCommand = finishResearchSecurityGroups(byCommand)
	report.BySource = finishResearchSecurityGroups(bySource)
	report.RiskBuckets = topResearchCounts(riskBuckets, 0)
	report.Posture = buildResearchSecurityEvaluationPosture(report)
	return report, nil
}

func researchSecurityCandidateFromBenchmarkCase(bc benchmarkCase) researchSecurityEvaluationCandidate {
	args := normalizeResearchSecurityBenchmarkArgs(bc.Comm, bc.Args)
	commandLine := joinCommandLine(bc.Comm, args)
	now := time.Now().UTC()
	return researchSecurityEvaluationCandidate{
		ID:              researchStableID("security-benchmark", bc.Name, bc.Comm, commandLine, bc.Expected),
		Timestamp:       now.UnixMilli(),
		Time:            now.Format(time.RFC3339Nano),
		Source:          "builtin",
		EventType:       bc.EventType,
		Category:        firstNonEmptyResearchSecurityString(bc.Category, "benchmark"),
		Comm:            bc.Comm,
		CommandLine:     commandLine,
		Args:            args,
		Target:          firstNonEmptyResearchSecurityString(bc.Path, bc.NetEndpoint),
		ExpectedAction:  normalizeResearchSecurityAction(bc.Expected),
		ExpectedSource:  "builtin-benchmark",
		RedactionLevel:  "synthetic",
		BenchmarkCase:   bc.Name,
		BenchmarkTool:   bc.ToolName,
		BenchmarkDetail: bc.Description,
	}
}

func researchSecurityCandidateFromEvent(event ResearchEvent, labelPolicy string) (researchSecurityEvaluationCandidate, bool) {
	comm, args, commandLine := researchCommandPartsFromEvent(event)
	if strings.TrimSpace(comm) == "" {
		return researchSecurityEvaluationCandidate{}, false
	}
	expected, expectedSource := researchSecurityExpectedActionForEvent(event, labelPolicy)
	category := ""
	if classification := behavior.ClassifyBehavior(comm, args); classification != nil {
		category = classification.PrimaryCategory
	}
	if category == "" {
		category = strings.TrimSpace(researchStringFromMap(event.Features, "category"))
	}
	timestamp := event.Timestamp
	if timestamp <= 0 {
		timestamp = time.Now().UTC().UnixMilli()
	}
	timeText := event.Time
	if strings.TrimSpace(timeText) == "" {
		timeText = time.UnixMilli(timestamp).UTC().Format(time.RFC3339Nano)
	}
	return researchSecurityEvaluationCandidate{
		ID:             researchStableID("security-session", event.ID, commandLine, expected),
		EventID:        event.ID,
		Timestamp:      timestamp,
		Time:           timeText,
		Source:         firstNonEmptyResearchSecurityString(event.Source, "session"),
		EventType:      event.EventType,
		Category:       category,
		Comm:           comm,
		CommandLine:    commandLine,
		Args:           append([]string(nil), args...),
		Target:         event.Target,
		ExpectedAction: normalizeResearchSecurityAction(expected),
		ExpectedSource: expectedSource,
		RedactionLevel: event.RedactionLevel,
		TraceID:        event.TraceID,
		SpanID:         event.SpanID,
	}, true
}

func evaluateResearchSecurityCandidate(candidate researchSecurityEvaluationCandidate, includeLLM bool) ResearchSecurityEvaluationSampleRow {
	assessment := assessCommandSafetyWithOptions(context.Background(), candidate.Comm, candidate.Args, "", 0, commandSafetyAssessmentOptions{IncludeLLM: includeLLM})
	observed := normalizeResearchSecurityAction(researchSecurityString(assessment["recommendedAction"]))
	riskScore := researchSecurityFloat(assessment["riskScore"])
	if !researchSecurityActionIsLabeled(observed) {
		observed = researchSecurityObservedActionFromRisk(riskScore)
	}
	expected := normalizeResearchSecurityAction(candidate.ExpectedAction)
	passed := researchSecurityActionsMatch(expected, observed)
	confidence := researchSecurityAssessmentConfidence(assessment)
	reasoning := researchSecurityString(assessment["reasoning"])
	findingType := researchSecurityFindingType(expected, observed, riskScore, confidence)
	recommendation := researchSecurityRecommendation(findingType, expected, observed)
	signals := researchSecurityAssessmentSignals(assessment)
	category := candidate.Category
	if category == "" {
		category = researchSecurityString(signals["classificationCategory"])
	}
	return ResearchSecurityEvaluationSampleRow{
		ID:              candidate.ID,
		EventID:         candidate.EventID,
		Timestamp:       candidate.Timestamp,
		Time:            candidate.Time,
		Source:          candidate.Source,
		EventType:       candidate.EventType,
		Category:        category,
		Comm:            candidate.Comm,
		CommandLine:     candidate.CommandLine,
		Args:            append([]string(nil), candidate.Args...),
		Target:          candidate.Target,
		ExpectedAction:  expected,
		ExpectedSource:  candidate.ExpectedSource,
		ObservedAction:  observed,
		Passed:          passed,
		FindingType:     findingType,
		RiskScore:       riskScore,
		RiskLevel:       researchSecurityString(assessment["riskLevel"]),
		Confidence:      confidence,
		Reasoning:       reasoning,
		Recommendation:  recommendation,
		RedactionLevel:  candidate.RedactionLevel,
		TraceID:         candidate.TraceID,
		SpanID:          candidate.SpanID,
		Signals:         signals,
		BenchmarkCase:   candidate.BenchmarkCase,
		BenchmarkTool:   candidate.BenchmarkTool,
		BenchmarkDetail: candidate.BenchmarkDetail,
	}
}

func filterResearchEventsForSecurityEvaluation(events []ResearchEvent, filter ResearchSourceFilter, timerange ResearchTimeRange, limit int) []ResearchEvent {
	filter = normalizeResearchSourceFilter(filter)
	timerange = normalizeResearchTimeRange(timerange)
	out := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, filter, timerange) {
			out = append(out, event)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func researchSecurityExpectedActionForEvent(event ResearchEvent, labelPolicy string) (string, string) {
	labelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(labelPolicy)
	decision := normalizeResearchSecurityAction(event.Decision)
	if researchSecurityActionIsLabeled(decision) {
		return decision, "research-decision"
	}
	switch labelPolicy {
	case "decision":
		return "UNLABELED", "research-decision-missing"
	case "unlabeled":
		return "UNLABELED", "research-unlabeled"
	default:
		if event.RiskScore >= 80 || strings.Contains(strings.ToLower(event.EventType), "alert") {
			return "ALERT", "research-risk-heuristic"
		}
		if event.RiskScore <= 30 && event.RiskScore >= 0 {
			return "ALLOW", "research-low-risk-heuristic"
		}
		return "UNLABELED", "research-ambiguous"
	}
}

func updateResearchSecurityCounters(counters *researchSecurityEvaluationCounters, expected, observed string) {
	if counters == nil {
		return
	}
	expectedRisky := expected != "ALLOW"
	observedRisky := observed != "ALLOW"
	counters.labeledEvaluated++
	if expected == observed {
		counters.strictCorrect++
	}
	if expected == "ALLOW" {
		counters.expectedAllow++
	} else {
		counters.expectedRisky++
	}
	if expected == "ALERT" {
		counters.expectedAlert++
		if observed == "ALERT" || observedRisky {
			counters.alertMatched++
		}
	}
	if expected == "BLOCK" {
		counters.expectedBlock++
		if observed == "BLOCK" {
			counters.blockMatched++
		}
	}
	switch {
	case expectedRisky && observedRisky:
		counters.tp++
	case !expectedRisky && !observedRisky:
		counters.tn++
	case !expectedRisky && observedRisky:
		counters.fp++
	case expectedRisky && !observedRisky:
		counters.fn++
	}
}

func computeResearchSecurityEvaluationMetrics(counters researchSecurityEvaluationCounters) ResearchSecurityEvaluationMetrics {
	return ResearchSecurityEvaluationMetrics{
		Accuracy:          percentFloat(counters.tp+counters.tn, counters.labeledEvaluated),
		Precision:         percentFloat(counters.tp, counters.tp+counters.fp),
		Recall:            percentFloat(counters.tp, counters.tp+counters.fn),
		AllowRecall:       percentFloat(counters.tn, counters.expectedAllow),
		AlertRecall:       percentFloat(counters.alertMatched, counters.expectedAlert),
		BlockRecall:       percentFloat(counters.blockMatched, counters.expectedBlock),
		FalsePositiveRate: percentFloat(counters.fp, counters.expectedAllow),
		FalseNegativeRate: percentFloat(counters.fn, counters.expectedRisky),
		BalancedAccuracy:  (percentFloat(counters.tn, counters.expectedAllow) + percentFloat(counters.tp, counters.expectedRisky)) / 2,
	}
}

func updateResearchSecurityGroup(groups map[string]*researchSecurityEvaluationAccumulator, key string, row ResearchSecurityEvaluationSampleRow) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	group := groups[key]
	if group == nil {
		group = &researchSecurityEvaluationAccumulator{}
		groups[key] = group
	}
	group.total++
	if row.Passed {
		group.passed++
	}
	switch row.FindingType {
	case "false_positive":
		group.falsePositives++
	case "false_negative":
		group.falseNegatives++
	}
	group.riskSum += row.RiskScore
}

func finishResearchSecurityGroups(groups map[string]*researchSecurityEvaluationAccumulator) []ResearchSecurityEvaluationGroup {
	out := make([]ResearchSecurityEvaluationGroup, 0, len(groups))
	for key, group := range groups {
		avgRisk := 0.0
		if group.total > 0 {
			avgRisk = researchSecurityRoundFloat(group.riskSum/float64(group.total), 2)
		}
		out = append(out, ResearchSecurityEvaluationGroup{
			Key:            key,
			Total:          group.total,
			Passed:         group.passed,
			Failed:         group.total - group.passed,
			FalsePositives: group.falsePositives,
			FalseNegatives: group.falseNegatives,
			AvgRiskScore:   avgRisk,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Failed == out[j].Failed {
			if out[i].Total == out[j].Total {
				return out[i].Key < out[j].Key
			}
			return out[i].Total > out[j].Total
		}
		return out[i].Failed > out[j].Failed
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func appendResearchSecurityFindings(findings *ResearchSecurityEvaluationFindings, row ResearchSecurityEvaluationSampleRow) {
	if findings == nil {
		return
	}
	appendCapped := func(dst *[]ResearchSecurityEvaluationSampleRow) {
		if len(*dst) < researchSecurityEvaluationMaxFindings {
			*dst = append(*dst, row)
		}
	}
	switch row.FindingType {
	case "false_positive":
		appendCapped(&findings.FalsePositives)
	case "false_negative":
		appendCapped(&findings.FalseNegatives)
	case "policy_gap":
		appendCapped(&findings.PolicyGaps)
	case "high_confidence_disagreement":
		appendCapped(&findings.HighConfidenceDisagreements)
	case "unlabeled_high_risk":
		appendCapped(&findings.UnlabeledHighRisk)
	}
}

func researchSecurityFindingType(expected, observed string, riskScore, confidence float64) string {
	expected = normalizeResearchSecurityAction(expected)
	observed = normalizeResearchSecurityAction(observed)
	if !researchSecurityActionIsLabeled(expected) {
		if observed != "ALLOW" || riskScore >= 70 {
			return "unlabeled_high_risk"
		}
		return ""
	}
	if expected == "ALLOW" && observed != "ALLOW" {
		if confidence >= 0.8 {
			return "high_confidence_disagreement"
		}
		return "false_positive"
	}
	if expected != "ALLOW" && observed == "ALLOW" {
		if confidence >= 0.8 {
			return "high_confidence_disagreement"
		}
		return "false_negative"
	}
	if expected != observed && confidence >= 0.8 {
		return "high_confidence_disagreement"
	}
	if expected != "ALLOW" && observed != "ALLOW" && riskScore < 60 {
		return "policy_gap"
	}
	return ""
}

func buildResearchSecurityEvaluationPosture(report ResearchSecurityEvaluationReport) ResearchSecurityEvaluationPosture {
	findingCounts := map[string]int{}
	falsePositives := len(report.Findings.FalsePositives)
	falseNegatives := len(report.Findings.FalseNegatives)
	policyGaps := len(report.Findings.PolicyGaps)
	highConfidence := len(report.Findings.HighConfidenceDisagreements)
	unlabeledHighRisk := len(report.Findings.UnlabeledHighRisk)
	if falsePositives > 0 {
		findingCounts["false_positive"] = falsePositives
	}
	if falseNegatives > 0 {
		findingCounts["false_negative"] = falseNegatives
	}
	if policyGaps > 0 {
		findingCounts["policy_gap"] = policyGaps
	}
	if highConfidence > 0 {
		findingCounts["high_confidence_disagreement"] = highConfidence
	}
	if unlabeledHighRisk > 0 {
		findingCounts["unlabeled_high_risk"] = unlabeledHighRisk
	}

	failureRate := percentFloat(report.Totals.Failed, report.Totals.Labeled)
	riskScore := failureRate
	if report.Metrics.FalseNegativeRate > riskScore {
		riskScore = report.Metrics.FalseNegativeRate
	}
	if report.Metrics.BalancedAccuracy > 0 {
		coverageRisk := 100 - report.Metrics.BalancedAccuracy
		if coverageRisk > riskScore {
			riskScore = coverageRisk
		}
	}
	riskScore += float64(highConfidence*10 + unlabeledHighRisk*4 + policyGaps*3)
	if riskScore > 100 {
		riskScore = 100
	}
	posture := ResearchSecurityEvaluationPosture{
		Status:               "pass",
		RiskScore:            researchSecurityRoundFloat(riskScore, 2),
		FindingCounts:        topResearchCounts(findingCounts, 0),
		TopFailingCategories: researchSecurityTopFailingGroups(report.ByCategory, 5),
	}

	if report.Totals.Labeled == 0 {
		posture.BlockingReasons = append(posture.BlockingReasons, "no_labeled_evaluation_samples")
	}
	if falseNegatives > 0 {
		posture.BlockingReasons = append(posture.BlockingReasons, fmt.Sprintf("false_negatives:%d", falseNegatives))
	}
	if highConfidence > 0 {
		posture.BlockingReasons = append(posture.BlockingReasons, fmt.Sprintf("high_confidence_disagreements:%d", highConfidence))
	}
	if report.Metrics.FalseNegativeRate >= 10 {
		posture.BlockingReasons = append(posture.BlockingReasons, fmt.Sprintf("false_negative_rate:%.1f%%", report.Metrics.FalseNegativeRate))
	}
	if falsePositives > 0 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("false_positives:%d", falsePositives))
	}
	if policyGaps > 0 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("policy_gaps:%d", policyGaps))
	}
	if unlabeledHighRisk > 0 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("unlabeled_high_risk:%d", unlabeledHighRisk))
	}
	if report.Metrics.AllowRecall > 0 && report.Metrics.AllowRecall < 90 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("allow_recall_low:%.1f%%", report.Metrics.AllowRecall))
	}
	if report.Metrics.BlockRecall > 0 && report.Metrics.BlockRecall < 90 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("block_recall_low:%.1f%%", report.Metrics.BlockRecall))
	}
	if report.Metrics.AlertRecall > 0 && report.Metrics.AlertRecall < 90 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("alert_recall_low:%.1f%%", report.Metrics.AlertRecall))
	}

	if len(posture.BlockingReasons) > 0 || posture.RiskScore >= 60 {
		posture.Status = "critical"
	} else if len(posture.Warnings) > 0 || report.Totals.Failed > 0 || posture.RiskScore >= 20 {
		posture.Status = "needs_review"
	}
	posture.SuggestedActions = researchSecurityPostureSuggestedActions(posture)
	return posture
}

func researchSecurityTopFailingGroups(groups []ResearchSecurityEvaluationGroup, limit int) []ResearchSecurityEvaluationGroup {
	capacity := len(groups)
	if limit > 0 && limit < capacity {
		capacity = limit
	}
	out := make([]ResearchSecurityEvaluationGroup, 0, capacity)
	for _, group := range groups {
		if group.Failed <= 0 {
			continue
		}
		out = append(out, group)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func researchSecurityPostureSuggestedActions(posture ResearchSecurityEvaluationPosture) []string {
	joinedBlockers := strings.Join(posture.BlockingReasons, "|")
	joinedWarnings := strings.Join(posture.Warnings, "|")
	actions := []string{}
	if strings.Contains(joinedBlockers, "false_negatives") || strings.Contains(joinedBlockers, "false_negative_rate") {
		actions = append(actions, "tighten_detection_for_missed_risky_agent_behaviors", "add_false_negative_cases_to_training_dataset")
	}
	if strings.Contains(joinedBlockers, "high_confidence_disagreements") {
		actions = append(actions, "prioritize_manual_audit_for_high_confidence_disagreements")
	}
	if strings.Contains(joinedWarnings, "false_positives") || strings.Contains(joinedWarnings, "allow_recall_low") {
		actions = append(actions, "review_over_sensitive_rules_or_ml_thresholds")
	}
	if strings.Contains(joinedWarnings, "policy_gaps") {
		actions = append(actions, "inspect_policy_gap_signal_weights_and_explanations")
	}
	if strings.Contains(joinedWarnings, "unlabeled_high_risk") {
		actions = append(actions, "route_unlabeled_high_risk_events_to_research_labeling")
	}
	if len(posture.TopFailingCategories) > 0 {
		actions = append(actions, "drill_down_top_failing_categories_before_policy_changes")
	}
	if len(actions) == 0 {
		actions = append(actions, "security_posture_passed_export_report_for_reproducibility")
	}
	return uniqueStringsPreserveOrder(actions)
}

func researchSecurityRecommendation(findingType, expected, observed string) string {
	switch findingType {
	case "false_positive":
		return "Review over-sensitive rule/ML threshold for benign Agent workflow."
	case "false_negative":
		return "Add or tighten detection coverage for risky Agent behavior."
	case "policy_gap":
		return "Alert fired with weak score; inspect signal weights and policy explanation."
	case "high_confidence_disagreement":
		return "High-confidence disagreement; prioritize manual audit before model/policy changes."
	case "unlabeled_high_risk":
		return "Unlabeled high-risk event; route to offline labeling or security review."
	default:
		if researchSecurityActionsMatch(expected, observed) {
			return "No action needed."
		}
		return "Review expected vs observed action."
	}
}

func researchSecurityActionsMatch(expected, observed string) bool {
	expected = normalizeResearchSecurityAction(expected)
	observed = normalizeResearchSecurityAction(observed)
	if !researchSecurityActionIsLabeled(expected) {
		return true
	}
	if expected == observed {
		return true
	}
	return expected != "ALLOW" && observed != "ALLOW"
}

func normalizeResearchSecurityAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ALLOW", "ALLOWED", "PASS":
		return "ALLOW"
	case "ALERT", "WARN", "WARNING":
		return "ALERT"
	case "BLOCK", "DENY", "DENIED":
		return "BLOCK"
	case "REWRITE":
		return "REWRITE"
	case "UNLABELED", "UNKNOWN", "", "-", "NONE":
		return "UNLABELED"
	default:
		return strings.ToUpper(strings.TrimSpace(action))
	}
}

func researchSecurityActionIsLabeled(action string) bool {
	switch normalizeResearchSecurityAction(action) {
	case "ALLOW", "ALERT", "BLOCK", "REWRITE":
		return true
	default:
		return false
	}
}

func researchSecurityObservedActionFromRisk(score float64) string {
	switch {
	case score >= 90:
		return "BLOCK"
	case score >= 60:
		return "ALERT"
	default:
		return "ALLOW"
	}
}

func normalizeResearchSecurityEvaluationMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "builtin", "benchmark", "baseline":
		return researchSecurityEvaluationModeBuiltin
	case "session", "events", "research":
		return researchSecurityEvaluationModeSession
	case "", "combined", "all", "both":
		return researchSecurityEvaluationModeCombined
	default:
		return researchSecurityEvaluationDefaultMode
	}
}

func normalizeResearchSecurityEvaluationLabelPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "decision", "decision_only":
		return "decision"
	case "unlabeled", "manual", "none":
		return "unlabeled"
	case "", "decision_then_heuristic", "decision_then_heuristics", "auto":
		return researchSecurityEvaluationLabelDecision
	default:
		return researchTrainingPolicyHeuristic
	}
}

func normalizeResearchSecurityEvaluationLimit(limit int) int {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if limit <= 0 {
		limit = researchDefaultTaskLimit
	}
	if settings.MaxSessionEvents > 0 && limit > settings.MaxSessionEvents {
		limit = settings.MaxSessionEvents
	}
	if limit > researchMaxTaskLimit {
		limit = researchMaxTaskLimit
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}

func normalizeResearchSecurityBenchmarkArgs(comm string, args []string) []string {
	out := append([]string(nil), args...)
	if len(out) > 0 && strings.EqualFold(strings.TrimSpace(out[0]), strings.TrimSpace(comm)) {
		out = out[1:]
	}
	return out
}

func researchSecurityAssessmentSignals(assessment map[string]any) map[string]any {
	signals := map[string]any{
		"riskLevel":    researchSecurityString(assessment["riskLevel"]),
		"anomalyScore": researchSecurityFloat(assessment["anomalyScore"]),
		"modelLoaded":  assessment["modelLoaded"],
		"mlEnabled":    assessment["mlEnabled"],
	}
	ml := researchSecurityMap(assessment["mlPrediction"])
	if len(ml) > 0 {
		signals["mlAction"] = researchSecurityString(ml["action"])
		signals["mlConfidence"] = researchSecurityFloat(ml["confidence"])
	}
	network := researchSecurityMap(assessment["networkAudit"])
	if len(network) > 0 {
		signals["networkRisk"] = firstNonEmptyResearchSecurityString(researchSecurityString(network["riskLevel"]), researchSecurityString(network["risk"]))
		signals["networkScore"] = researchSecurityFloat(network["riskScore"])
	}
	classification := researchSecurityMap(assessment["classification"])
	if len(classification) > 0 {
		signals["classificationCategory"] = firstNonEmptyResearchSecurityString(researchSecurityString(classification["primaryCategory"]), researchSecurityString(classification["PrimaryCategory"]))
		signals["classificationConfidence"] = firstNonEmptyResearchSecurityString(researchSecurityString(classification["confidence"]), researchSecurityString(classification["Confidence"]))
	}
	return signals
}

func researchSecurityAssessmentConfidence(assessment map[string]any) float64 {
	ml := researchSecurityMap(assessment["mlPrediction"])
	confidence := researchSecurityFloat(ml["confidence"])
	evidence := researchSecurityMap(assessment["sampleEvidence"])
	if sampleConfidence := researchSecurityFloat(evidence["confidence"]); sampleConfidence > confidence {
		confidence = sampleConfidence
	}
	if confidence > 1 && confidence <= 100 {
		confidence = confidence / 100
	}
	return researchSecurityRoundFloat(confidence, 3)
}

func researchSecurityMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

func researchSecurityString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func researchSecurityFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func researchSecurityRiskBucket(score float64) string {
	switch {
	case score >= 80:
		return "80-100"
	case score >= 60:
		return "60-79"
	case score >= 40:
		return "40-59"
	case score >= 20:
		return "20-39"
	default:
		return "0-19"
	}
}

func firstNonEmptyResearchSecurityString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyResearchSecurityParam(params map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		if value := researchSecurityString(params[key]); value != "" {
			return value
		}
	}
	return fallback
}

func researchSecurityParamInt(params map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return int(researchSecurityFloat(value)), true
		}
	}
	return 0, false
}

func researchSecurityParamBool(params map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "on":
				return true, true
			case "false", "0", "no", "off":
				return false, true
			}
		default:
			return researchSecurityFloat(typed) != 0, true
		}
	}
	return false, false
}

func percentFloat(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return researchSecurityRoundFloat(float64(numerator)/float64(denominator)*100, 2)
}

func researchSecurityRoundFloat(value float64, places int) float64 {
	if places <= 0 {
		return math.Round(value)
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func researchSecurityEvaluationJSONBytes(report *ResearchSecurityEvaluationReport) ([]byte, error) {
	if report == nil {
		return nil, errors.New("security evaluation report is unavailable")
	}
	return json.MarshalIndent(report, "", "  ")
}

func researchSecurityEvaluationJSONLBytes(report *ResearchSecurityEvaluationReport) []byte {
	var buf bytes.Buffer
	if report == nil {
		return nil
	}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, sample := range report.Samples {
		_ = encoder.Encode(sample)
	}
	return buf.Bytes()
}

func researchSecurityEvaluationCSVBytes(report *ResearchSecurityEvaluationReport) ([]byte, error) {
	if report == nil {
		return nil, errors.New("security evaluation report is unavailable")
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"id", "event_id", "timestamp", "time", "source", "event_type", "category", "comm", "command_line", "expected_action", "expected_source", "observed_action", "passed", "finding_type", "risk_score", "risk_level", "confidence", "target", "trace_id", "span_id", "benchmark_case", "reasoning", "recommendation"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range report.Samples {
		record := []string{
			row.ID,
			row.EventID,
			strconv.FormatInt(row.Timestamp, 10),
			row.Time,
			row.Source,
			row.EventType,
			row.Category,
			row.Comm,
			row.CommandLine,
			row.ExpectedAction,
			row.ExpectedSource,
			row.ObservedAction,
			strconv.FormatBool(row.Passed),
			row.FindingType,
			researchFloatString(row.RiskScore),
			row.RiskLevel,
			strconv.FormatFloat(row.Confidence, 'f', -1, 64),
			row.Target,
			row.TraceID,
			row.SpanID,
			row.BenchmarkCase,
			row.Reasoning,
			row.Recommendation,
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}
