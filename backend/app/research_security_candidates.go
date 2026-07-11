package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"agent-ebpf-filter/internal/behavior"
)

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
