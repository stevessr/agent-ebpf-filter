package research

import (
	"fmt"
	"strings"
	"time"
)

const researchSecurityOutcomeCorrelationWindow = 30 * time.Second

func normalizeResearchSecurityValidationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "outcome", "result", "results", "proof", "poc", "glasswing":
		return researchSecurityValidationModeOutcome
	default:
		return researchSecurityValidationModePrediction
	}
}

func normalizeResearchSecurityMinimumEvidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "reachable", "reachability":
		return researchSecurityEvidenceReachable
	case "impact", "impact_confirmed", "impact-confirmed", "confirmed":
		return researchSecurityEvidenceImpactConfirmed
	case "hypothesis", "candidate":
		return researchSecurityEvidenceHypothesis
	default:
		return researchSecurityEvidenceReproduced
	}
}

func researchSecurityEvidenceRank(level string) int {
	switch normalizeResearchSecurityMinimumEvidence(level) {
	case researchSecurityEvidenceImpactConfirmed:
		return 3
	case researchSecurityEvidenceReproduced:
		return 2
	case researchSecurityEvidenceReachable:
		return 1
	default:
		return 0
	}
}

func applyResearchSecurityOutcomeValidation(report *ResearchSecurityEvaluationReport, events []ResearchEvent, req ResearchSecurityEvaluationRequest) {
	if report == nil || normalizeResearchSecurityValidationMode(req.ValidationMode) != researchSecurityValidationModeOutcome {
		return
	}

	minimumEvidence := normalizeResearchSecurityMinimumEvidence(req.MinimumEvidence)
	summary := &ResearchSecurityOutcomeValidationSummary{
		Enabled:           true,
		MinimumEvidence:   minimumEvidence,
		AdversarialReview: req.AdversarialReview,
		Findings:          make([]ResearchSecurityEvaluationSampleRow, 0),
	}
	report.ValidationMode = researchSecurityValidationModeOutcome

	for i := range report.Samples {
		row := &report.Samples[i]
		if row.Source == "builtin" {
			row.ValidationStatus = "not_applicable"
			row.EvidenceLevel = researchSecurityEvidenceHypothesis
			row.ValidatorReason = "Built-in benchmark samples are classifier fixtures; outcome validation requires runtime evidence from a research session."
			summary.NotApplicable++
			continue
		}

		summary.Candidates++
		matches := correlatedResearchSecurityEvents(*row, events)
		if len(matches) > 0 {
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceReachable
			row.ValidationStatus = researchSecurityEvidenceReachable
			summary.Reachable++
			first := matches[0]
			row.Evidence = append(row.Evidence, ResearchSecurityOutcomeEvidence{
				Level:   researchSecurityEvidenceReachable,
				Kind:    "runtime_correlation",
				EventID: first.ID,
				Source:  first.Source,
				Detail:  "A captured runtime event reached the same trace/span or command-target scope.",
			})
		}

		reproduced, reproducedEvent := researchSecurityEvidenceMarker(matches,
			"validation.reproduced", "validation.proof", "outcome.reproduced", "outcome.success",
			"poc.success", "proof.success", "exploit.reproduced")
		impact, impactEvent := researchSecurityEvidenceMarker(matches,
			"validation.impactConfirmed", "outcome.impactConfirmed", "impact.confirmed", "proof.impactConfirmed")
		refuted, refutedEvent := researchSecurityEvidenceMarker(matches,
			"validation.rejected", "validation.refuted", "outcome.rejected", "proof.rejected")

		if reproduced {
			row.Reproduced = true
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceReproduced
			row.ValidationStatus = researchSecurityEvidenceReproduced
			summary.Reproduced++
			row.Evidence = append(row.Evidence, ResearchSecurityOutcomeEvidence{
				Level:   researchSecurityEvidenceReproduced,
				Kind:    "reproduction_proof",
				EventID: reproducedEvent.ID,
				Source:  reproducedEvent.Source,
				Detail:  "An authorized validation producer recorded a successful reproduction marker.",
			})
		}

		if impact {
			row.ImpactConfirmed = true
			row.Reproduced = true
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceImpactConfirmed
			row.ValidationStatus = researchSecurityEvidenceImpactConfirmed
			summary.ImpactConfirmed++
			row.Evidence = append(row.Evidence, ResearchSecurityOutcomeEvidence{
				Level:   researchSecurityEvidenceImpactConfirmed,
				Kind:    "impact_proof",
				EventID: impactEvent.ID,
				Source:  impactEvent.Source,
				Detail:  "An authorized validation producer recorded confirmed security impact.",
			})
		}

		if req.AdversarialReview && refuted && !row.ImpactConfirmed {
			row.ValidationStatus = "rejected"
			row.Actionable = false
			row.ValidatorReason = "Independent validation evidence explicitly refuted the candidate."
			summary.Rejected++
			row.Evidence = append(row.Evidence, ResearchSecurityOutcomeEvidence{
				Level:   researchSecurityEvidenceHypothesis,
				Kind:    "adversarial_refutation",
				EventID: refutedEvent.ID,
				Source:  refutedEvent.Source,
				Detail:  "A validator recorded an explicit refutation marker.",
			})
			continue
		}

		if row.EvidenceLevel == "" {
			row.EvidenceLevel = researchSecurityEvidenceHypothesis
			row.ValidationStatus = "unproven"
			row.ValidatorReason = "No correlated runtime proof was found; keep as a hypothesis instead of an actionable vulnerability."
			summary.Unproven++
		} else if row.ValidatorReason == "" {
			row.ValidatorReason = fmt.Sprintf("Outcome evidence reached %s; minimum actionable evidence is %s.", row.EvidenceLevel, minimumEvidence)
		}

		row.Actionable = row.ValidationStatus != "rejected" && researchSecurityEvidenceRank(row.EvidenceLevel) >= researchSecurityEvidenceRank(minimumEvidence)
		if row.Actionable {
			summary.Actionable++
			if len(summary.Findings) < researchSecurityEvaluationMaxFindings {
				summary.Findings = append(summary.Findings, *row)
			}
		}
	}

	report.OutcomeValidation = summary
}

func correlatedResearchSecurityEvents(row ResearchSecurityEvaluationSampleRow, events []ResearchEvent) []ResearchEvent {
	matches := make([]ResearchEvent, 0, 4)
	for _, event := range events {
		if row.EventID != "" && event.ID == row.EventID {
			matches = append(matches, event)
			continue
		}
		if row.TraceID != "" && event.TraceID == row.TraceID {
			matches = append(matches, event)
			continue
		}
		if row.SpanID != "" && event.SpanID == row.SpanID {
			matches = append(matches, event)
			continue
		}
		if row.Comm == "" || event.Comm != row.Comm || row.Target == "" || event.Target != row.Target {
			continue
		}
		if row.Timestamp <= 0 || event.Timestamp <= 0 {
			matches = append(matches, event)
			continue
		}
		delta := time.Duration(event.Timestamp-row.Timestamp) * time.Millisecond
		if delta < 0 {
			delta = -delta
		}
		if delta <= researchSecurityOutcomeCorrelationWindow {
			matches = append(matches, event)
		}
	}
	return matches
}

func researchSecurityEvidenceMarker(events []ResearchEvent, keys ...string) (bool, ResearchEvent) {
	for _, event := range events {
		for _, key := range keys {
			if researchSecurityFeatureTruthy(event.Features, key) {
				return true, event
			}
		}
	}
	return false, ResearchEvent{}
}

func researchSecurityFeatureTruthy(features map[string]any, path string) bool {
	if len(features) == 0 {
		return false
	}
	if value, ok := features[path]; ok {
		return researchSecurityTruthy(value)
	}
	parts := strings.Split(path, ".")
	var current any = features
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = m[part]
		if !ok {
			return false
		}
	}
	return researchSecurityTruthy(current)
}

func researchSecurityTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "success", "succeeded", "confirmed", "reproduced", "pass", "passed":
			return true
		}
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}
