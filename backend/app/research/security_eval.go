package research

import (
	"strings"
	"time"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/core"
)

func researchSecurityEvaluationRequestFromTask(req researchTaskRequest) ResearchSecurityEvaluationRequest {
	out := ResearchSecurityEvaluationRequest{
		Mode:                     req.EvaluationMode,
		LabelPolicy:              req.LabelPolicy,
		Limit:                    req.Limit,
		IncludeLLM:               req.IncludeLLM,
		CorrelationWindowSeconds: researchSecurityOutcomeDefaultCorrelationWindowSeconds,
		SourceFilter:             req.SourceFilter,
		TimeRange:                req.TimeRange,
	}

	// UI/API convenience aliases: keep the existing corpus selector while making
	// the Glasswing-inspired result-oriented path an explicit opt-in choice.
	switch strings.ToLower(strings.TrimSpace(out.Mode)) {
	case "session_outcome", "session-outcome", "session_glasswing", "session-glasswing":
		out.Mode = researchSecurityEvaluationModeSession
		out.ValidationMode = researchSecurityValidationModeOutcome
	case "combined_outcome", "combined-outcome", "combined_glasswing", "combined-glasswing", "glasswing":
		out.Mode = researchSecurityEvaluationModeCombined
		out.ValidationMode = researchSecurityValidationModeOutcome
	}

	if len(req.Params) > 0 {
		out.Mode = firstNonEmptyResearchSecurityParam(req.Params, out.Mode, "mode", "evaluationMode", "corpus", "source")
		out.LabelPolicy = firstNonEmptyResearchSecurityParam(req.Params, out.LabelPolicy, "labelPolicy", "label_policy")
		out.ValidationMode = firstNonEmptyResearchSecurityParam(req.Params, out.ValidationMode, "validationMode", "validation_mode", "validation", "proofMode")
		out.MinimumEvidence = firstNonEmptyResearchSecurityParam(req.Params, out.MinimumEvidence, "minimumEvidence", "minimum_evidence", "evidence")
		if limit, ok := researchSecurityParamInt(req.Params, "limit", "maxSamples", "max_samples"); ok {
			out.Limit = limit
		}
		if includeLLM, ok := researchSecurityParamBool(req.Params, "includeLLM", "include_llm", "llm"); ok {
			out.IncludeLLM = includeLLM
		}
		if adversarialReview, ok := researchSecurityParamBool(req.Params, "adversarialReview", "adversarial_review", "independentReview"); ok {
			out.AdversarialReview = adversarialReview
		}
		if requireAuthorization, ok := researchSecurityParamBool(req.Params, "requireAuthorization", "require_authorization", "authorizedEvidenceOnly"); ok {
			out.RequireAuthorization = requireAuthorization
		}
		if independentRefutation, ok := researchSecurityParamBool(req.Params, "requireIndependentRefutation", "require_independent_refutation", "independentRefutation"); ok {
			out.RequireIndependentRefutation = independentRefutation
		}
		if dedupeActionable, ok := researchSecurityParamBool(req.Params, "dedupeActionable", "dedupe_actionable", "dedupe"); ok {
			out.DedupeActionable = dedupeActionable
		}
		if correlationWindow, ok := researchSecurityParamInt(req.Params, "correlationWindowSeconds", "correlation_window_seconds", "correlationWindow"); ok {
			out.CorrelationWindowSeconds = correlationWindow
		}
		out.AllowedValidatorSources = researchSecurityParamStrings(req.Params, "allowedValidatorSources", "allowed_validator_sources", "validatorSources")
		out.AllowedAuthorizationIDs = researchSecurityParamStrings(req.Params, "allowedAuthorizationIds", "allowed_authorization_ids", "authorizationIds")
		out.AllowedTargets = researchSecurityParamStrings(req.Params, "allowedTargets", "allowed_targets", "validationTargets")
	}
	out.Mode = normalizeResearchSecurityEvaluationMode(out.Mode)
	out.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(out.LabelPolicy)
	out.ValidationMode = normalizeResearchSecurityValidationMode(out.ValidationMode)
	out.MinimumEvidence = normalizeResearchSecurityMinimumEvidence(out.MinimumEvidence)
	out.CorrelationWindowSeconds = normalizeResearchSecurityOutcomeCorrelationWindow(out.CorrelationWindowSeconds)
	out.AllowedValidatorSources = normalizeResearchSecurityStringList(out.AllowedValidatorSources)
	out.AllowedAuthorizationIDs = normalizeResearchSecurityStringList(out.AllowedAuthorizationIDs)
	out.AllowedTargets = normalizeResearchSecurityStringList(out.AllowedTargets)
	if out.ValidationMode == researchSecurityValidationModeOutcome {
		if !researchSecurityParamPresent(req.Params, "adversarialReview", "adversarial_review", "independentReview") {
			out.AdversarialReview = true
		}
		if !researchSecurityParamPresent(req.Params, "requireAuthorization", "require_authorization", "authorizedEvidenceOnly") {
			out.RequireAuthorization = true
		}
		if !researchSecurityParamPresent(req.Params, "dedupeActionable", "dedupe_actionable", "dedupe") {
			out.DedupeActionable = true
		}
		if out.AdversarialReview && !researchSecurityParamPresent(req.Params, "requireIndependentRefutation", "require_independent_refutation", "independentRefutation") {
			out.RequireIndependentRefutation = true
		}
	}
	out.SourceFilter = normalizeResearchSourceFilter(out.SourceFilter)
	out.TimeRange = normalizeResearchTimeRange(out.TimeRange)
	return out
}

func researchSecurityParamPresent(params map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

func researchSecurityParamStrings(params map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return append([]string(nil), typed...)
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := researchSecurityString(item); text != "" {
					out = append(out, text)
				}
			}
			return out
		case string:
			parts := strings.Split(typed, ",")
			out := make([]string, 0, len(parts))
			for _, item := range parts {
				if text := strings.TrimSpace(item); text != "" {
					out = append(out, text)
				}
			}
			return out
		default:
			if text := researchSecurityString(typed); text != "" {
				return []string{text}
			}
		}
	}
	return nil
}

func normalizeResearchSecurityStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeResearchSecurityOutcomeCorrelationWindow(seconds int) int {
	if seconds <= 0 {
		return researchSecurityOutcomeDefaultCorrelationWindowSeconds
	}
	if seconds > researchSecurityOutcomeMaxCorrelationWindowSeconds {
		return researchSecurityOutcomeMaxCorrelationWindowSeconds
	}
	return seconds
}

func buildResearchSecurityEvaluationReport(sessionID string, events []ResearchEvent, req ResearchSecurityEvaluationRequest, entry *researchTaskEntry) (ResearchSecurityEvaluationReport, error) {
	req.Mode = normalizeResearchSecurityEvaluationMode(req.Mode)
	req.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(req.LabelPolicy)
	req.ValidationMode = normalizeResearchSecurityValidationMode(req.ValidationMode)
	req.MinimumEvidence = normalizeResearchSecurityMinimumEvidence(req.MinimumEvidence)
	req.CorrelationWindowSeconds = normalizeResearchSecurityOutcomeCorrelationWindow(req.CorrelationWindowSeconds)
	req.AllowedValidatorSources = normalizeResearchSecurityStringList(req.AllowedValidatorSources)
	req.AllowedAuthorizationIDs = normalizeResearchSecurityStringList(req.AllowedAuthorizationIDs)
	req.AllowedTargets = normalizeResearchSecurityStringList(req.AllowedTargets)
	req.SourceFilter = normalizeResearchSourceFilter(req.SourceFilter)
	req.TimeRange = normalizeResearchTimeRange(req.TimeRange)
	req.Limit = normalizeResearchSecurityEvaluationLimit(req.Limit)

	candidates := make([]researchSecurityEvaluationCandidate, 0, len(benchmarkCases())+len(events))
	skipped := 0
	if req.Mode == researchSecurityEvaluationModeBuiltin || req.Mode == researchSecurityEvaluationModeCombined {
		for _, bc := range benchmarkCases() {
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
		ValidationMode:  req.ValidationMode,
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
			entry.setProgress(0.05 + 0.82*(float64(index+1)/float64(maxInt(1, len(candidates)))))
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
		ml.IncrementResearchCount(riskBuckets, researchSecurityRiskBucket(row.RiskScore))

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
	report.RiskBuckets = ml.TopResearchCounts(riskBuckets, 0)
	if req.ValidationMode == researchSecurityValidationModeOutcome {
		if entry != nil {
			entry.setProgress(0.90)
		}
		applyResearchSecurityOutcomeValidation(&report, events, req)
		report.Posture = buildResearchSecurityOutcomePosture(report)
	} else {
		report.Posture = buildResearchSecurityEvaluationPosture(report)
	}
	return report, nil
}

// benchmarkCases exposes the built-in security benchmark fixtures.
func benchmarkCases() []benchmarkCase { return core.DefaultBenchmarkCases() }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
