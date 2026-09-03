package research

import (
	"strings"
	"time"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/core"
)

func researchSecurityEvaluationRequestFromTask(req researchTaskRequest) ResearchSecurityEvaluationRequest {
	out := ResearchSecurityEvaluationRequest{
		Mode:         req.EvaluationMode,
		LabelPolicy:  req.LabelPolicy,
		Limit:        req.Limit,
		IncludeLLM:   req.IncludeLLM,
		SourceFilter: req.SourceFilter,
		TimeRange:    req.TimeRange,
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
	}
	out.Mode = normalizeResearchSecurityEvaluationMode(out.Mode)
	out.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(out.LabelPolicy)
	out.ValidationMode = normalizeResearchSecurityValidationMode(out.ValidationMode)
	out.MinimumEvidence = normalizeResearchSecurityMinimumEvidence(out.MinimumEvidence)
	if out.ValidationMode == researchSecurityValidationModeOutcome {
		if _, ok := req.Params["adversarialReview"]; !ok {
			if _, ok := req.Params["adversarial_review"]; !ok {
				if _, ok := req.Params["independentReview"]; !ok {
					out.AdversarialReview = true
				}
			}
		}
	}
	out.SourceFilter = normalizeResearchSourceFilter(out.SourceFilter)
	out.TimeRange = normalizeResearchTimeRange(out.TimeRange)
	return out
}

func buildResearchSecurityEvaluationReport(sessionID string, events []ResearchEvent, req ResearchSecurityEvaluationRequest, entry *researchTaskEntry) (ResearchSecurityEvaluationReport, error) {
	req.Mode = normalizeResearchSecurityEvaluationMode(req.Mode)
	req.LabelPolicy = normalizeResearchSecurityEvaluationLabelPolicy(req.LabelPolicy)
	req.ValidationMode = normalizeResearchSecurityValidationMode(req.ValidationMode)
	req.MinimumEvidence = normalizeResearchSecurityMinimumEvidence(req.MinimumEvidence)
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
