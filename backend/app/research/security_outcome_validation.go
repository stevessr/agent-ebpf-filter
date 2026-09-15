package research

import (
	"fmt"
	"strings"
	"time"
)

var (
	researchSecurityReachabilityMarkers = []string{
		"validation.reachable", "outcome.reachable", "trace.reachable", "proof.reachable",
	}
	researchSecurityReproductionMarkers = []string{
		"validation.reproduced", "validation.proof", "outcome.reproduced", "outcome.success",
		"poc.success", "proof.success", "exploit.reproduced",
	}
	researchSecurityImpactMarkers = []string{
		"validation.impactConfirmed", "outcome.impactConfirmed", "impact.confirmed", "proof.impactConfirmed",
	}
	researchSecurityRefutationMarkers = []string{
		"validation.rejected", "validation.refuted", "outcome.rejected", "proof.rejected",
	}
)

type researchSecurityCorrelatedEvent struct {
	Event       ResearchEvent
	Correlation string
}

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
	correlationWindowSeconds := normalizeResearchSecurityOutcomeCorrelationWindow(req.CorrelationWindowSeconds)
	summary := &ResearchSecurityOutcomeValidationSummary{
		Enabled:                      true,
		MinimumEvidence:              minimumEvidence,
		AdversarialReview:            req.AdversarialReview,
		RequireAuthorization:         req.RequireAuthorization,
		RequireIndependentRefutation: req.RequireIndependentRefutation,
		DedupeActionable:             req.DedupeActionable,
		CorrelationWindowSeconds:     correlationWindowSeconds,
		AllowedValidatorSources:      append([]string(nil), req.AllowedValidatorSources...),
		AllowedAuthorizationIDs:      append([]string(nil), req.AllowedAuthorizationIDs...),
		AllowedTargets:               append([]string(nil), req.AllowedTargets...),
		Findings:                     make([]ResearchSecurityEvaluationSampleRow, 0),
	}
	report.ValidationMode = researchSecurityValidationModeOutcome
	actionableRows := make([]ResearchSecurityEvaluationSampleRow, 0)

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
		if !researchSecurityTargetAllowed(row.Target, req.AllowedTargets) {
			row.ValidationStatus = "out_of_scope"
			row.EvidenceLevel = researchSecurityEvidenceHypothesis
			row.ValidatorReason = "Candidate target is outside the configured outcome-validation scope."
			summary.OutOfScope++
			continue
		}

		matches := correlatedResearchSecurityEvents(*row, events, time.Duration(correlationWindowSeconds)*time.Second)
		authorizedMatches, unauthorizedCount := researchSecurityAuthorizedEvidenceMatches(matches, req)
		summary.UnauthorizedEvidence += unauthorizedCount

		reachable, reachableMatch := researchSecurityEvidenceMarker(authorizedMatches, researchSecurityReachabilityMarkers...)
		reproduced, reproducedMatch := researchSecurityEvidenceMarker(authorizedMatches, researchSecurityReproductionMarkers...)
		impact, impactMatch := researchSecurityEvidenceMarker(authorizedMatches, researchSecurityImpactMarkers...)
		refuted, refutedMatch := researchSecurityEvidenceMarker(authorizedMatches, researchSecurityRefutationMarkers...)

		if reachable {
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceReachable
			row.ValidationStatus = researchSecurityEvidenceReachable
			row.Evidence = append(row.Evidence, researchSecurityOutcomeEvidence(
				researchSecurityEvidenceReachable,
				"reachability_proof",
				reachableMatch,
				"An authorized trace/validator explicitly marked the candidate path as reachable.",
			))
		}

		if reproduced {
			row.Reproduced = true
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceReproduced
			row.ValidationStatus = researchSecurityEvidenceReproduced
			row.Evidence = append(row.Evidence, researchSecurityOutcomeEvidence(
				researchSecurityEvidenceReproduced,
				"reproduction_proof",
				reproducedMatch,
				"An authorized validation producer recorded a successful reproduction marker.",
			))
		}

		if impact {
			row.ImpactConfirmed = true
			row.Reproduced = true
			row.Reachable = true
			row.EvidenceLevel = researchSecurityEvidenceImpactConfirmed
			row.ValidationStatus = researchSecurityEvidenceImpactConfirmed
			row.Evidence = append(row.Evidence, researchSecurityOutcomeEvidence(
				researchSecurityEvidenceImpactConfirmed,
				"impact_proof",
				impactMatch,
				"An authorized validation producer recorded confirmed security impact.",
			))
		}

		if row.Reachable {
			summary.Reachable++
		}
		if row.Reproduced {
			summary.Reproduced++
		}
		if row.ImpactConfirmed {
			summary.ImpactConfirmed++
		}

		row.FindingKey = researchSecurityOutcomeFindingKey(*row, authorizedMatches)
		if req.AdversarialReview && refuted {
			if req.RequireIndependentRefutation && !researchSecurityRefutationIndependent(*row, reachableMatch, reproducedMatch, impactMatch, refutedMatch) {
				summary.NonIndependentRefutations++
				row.Evidence = append(row.Evidence, researchSecurityOutcomeEvidence(
					researchSecurityEvidenceHypothesis,
					"non_independent_refutation_ignored",
					refutedMatch,
					"A refutation marker was ignored because it was not produced by an independent validator identity.",
				))
				refuted = false
			}
		}

		if req.AdversarialReview && refuted {
			row.Evidence = append(row.Evidence, researchSecurityOutcomeEvidence(
				researchSecurityEvidenceHypothesis,
				"adversarial_refutation",
				refutedMatch,
				"An authorized independent validator recorded an explicit refutation marker.",
			))
			if row.ImpactConfirmed {
				row.EvidenceConflict = true
				row.ValidationStatus = "conflicted"
				row.ValidatorReason = "Confirmed impact and an independent refutation both exist; retain the finding as actionable and require human review of the conflicting evidence."
				summary.Conflicted++
			} else {
				row.ValidationStatus = "rejected"
				row.Actionable = false
				row.ValidatorReason = "Independent validation evidence explicitly refuted the candidate."
				summary.Rejected++
				continue
			}
		}

		if row.EvidenceLevel == "" {
			row.EvidenceLevel = researchSecurityEvidenceHypothesis
			row.ValidationStatus = "unproven"
			if unauthorizedCount > 0 {
				row.ValidatorReason = "Only unauthorized or disallowed proof markers were correlated; keep this candidate as a hypothesis."
			} else {
				row.ValidatorReason = "No explicit reachability, reproduction, or impact proof was found; keep this candidate as a hypothesis."
			}
			summary.Unproven++
		} else if row.ValidatorReason == "" {
			row.ValidatorReason = fmt.Sprintf("Outcome evidence reached %s; minimum actionable evidence is %s.", row.EvidenceLevel, minimumEvidence)
		}

		row.Actionable = row.ValidationStatus != "rejected" && researchSecurityEvidenceRank(row.EvidenceLevel) >= researchSecurityEvidenceRank(minimumEvidence)
		if row.Actionable {
			summary.Actionable++
			actionableRows = append(actionableRows, *row)
		}
	}

	researchSecurityFinalizeOutcomeFindings(summary, actionableRows)
	report.OutcomeValidation = summary
}

func correlatedResearchSecurityEvents(row ResearchSecurityEvaluationSampleRow, events []ResearchEvent, correlationWindow time.Duration) []researchSecurityCorrelatedEvent {
	matches := make([]researchSecurityCorrelatedEvent, 0, 4)
	for _, event := range events {
		if candidateID := researchSecurityFeatureString(event.Features, "validation.candidateId", "outcome.candidateId", "proof.candidateId"); candidateID != "" && candidateID == row.ID {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "candidate_id"})
			continue
		}
		if candidateEventID := researchSecurityFeatureString(event.Features, "validation.candidateEventId", "outcome.candidateEventId", "proof.candidateEventId"); candidateEventID != "" && row.EventID != "" && candidateEventID == row.EventID {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "candidate_event_id"})
			continue
		}
		if row.EventID != "" && event.ID == row.EventID {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "event_id"})
			continue
		}
		if row.TraceID != "" && event.TraceID == row.TraceID {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "trace_id"})
			continue
		}
		if row.SpanID != "" && event.SpanID == row.SpanID {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "span_id"})
			continue
		}
		if row.Comm == "" || event.Comm != row.Comm || row.Target == "" || event.Target != row.Target {
			continue
		}
		// The weakest correlation method must remain time-bounded. Missing
		// timestamps are not treated as a match because that would silently turn
		// comm+target into an unbounded association.
		if row.Timestamp <= 0 || event.Timestamp <= 0 {
			continue
		}
		delta := time.Duration(event.Timestamp-row.Timestamp) * time.Millisecond
		if delta < 0 {
			delta = -delta
		}
		if delta <= correlationWindow {
			matches = append(matches, researchSecurityCorrelatedEvent{Event: event, Correlation: "command_target_window"})
		}
	}
	return matches
}

func researchSecurityAuthorizedEvidenceMatches(matches []researchSecurityCorrelatedEvent, req ResearchSecurityEvaluationRequest) ([]researchSecurityCorrelatedEvent, int) {
	if len(matches) == 0 {
		return nil, 0
	}
	out := make([]researchSecurityCorrelatedEvent, 0, len(matches))
	unauthorized := 0
	for _, match := range matches {
		if !researchSecurityHasOutcomeMarker(match.Event.Features) {
			continue
		}
		if !researchSecurityEvidenceAuthorized(match.Event, req) {
			unauthorized++
			continue
		}
		out = append(out, match)
	}
	return out, unauthorized
}

func researchSecurityEvidenceAuthorized(event ResearchEvent, req ResearchSecurityEvaluationRequest) bool {
	if !researchSecuritySourceAllowed(event.Source, req.AllowedValidatorSources) {
		return false
	}
	authorizationID := researchSecurityAuthorizationID(event)
	if len(req.AllowedAuthorizationIDs) > 0 && !researchSecurityStringAllowedFold(authorizationID, req.AllowedAuthorizationIDs) {
		return false
	}
	if !req.RequireAuthorization {
		return true
	}
	return researchSecurityFeatureTruthy(event.Features, "validation.authorized") ||
		researchSecurityFeatureTruthy(event.Features, "outcome.authorized") ||
		researchSecurityFeatureTruthy(event.Features, "proof.authorized")
}

func researchSecuritySourceAllowed(source string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	source = strings.TrimSpace(source)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(strings.ToLower(source), strings.ToLower(strings.TrimSuffix(pattern, "*"))) {
				return true
			}
			continue
		}
		if strings.EqualFold(source, pattern) {
			return true
		}
	}
	return false
}

func researchSecurityTargetAllowed(target string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" {
			return true
		}
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(target, strings.TrimSuffix(pattern, "*")) {
				return true
			}
			continue
		}
		if target == pattern {
			return true
		}
	}
	return false
}

func researchSecurityStringAllowedFold(value string, allowed []string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, item := range allowed {
		if strings.EqualFold(value, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func researchSecurityEvidenceMarker(events []researchSecurityCorrelatedEvent, keys ...string) (bool, researchSecurityCorrelatedEvent) {
	for _, match := range events {
		for _, key := range keys {
			if researchSecurityFeatureTruthy(match.Event.Features, key) {
				return true, match
			}
		}
	}
	return false, researchSecurityCorrelatedEvent{}
}

func researchSecurityHasOutcomeMarker(features map[string]any) bool {
	for _, keys := range [][]string{
		researchSecurityReachabilityMarkers,
		researchSecurityReproductionMarkers,
		researchSecurityImpactMarkers,
		researchSecurityRefutationMarkers,
	} {
		for _, key := range keys {
			if researchSecurityFeatureTruthy(features, key) {
				return true
			}
		}
	}
	return false
}

func researchSecurityOutcomeEvidence(level, kind string, match researchSecurityCorrelatedEvent, detail string) ResearchSecurityOutcomeEvidence {
	return ResearchSecurityOutcomeEvidence{
		Level:           level,
		Kind:            kind,
		EventID:         match.Event.ID,
		Source:          match.Event.Source,
		Detail:          detail,
		Correlation:     match.Correlation,
		ValidatorID:     researchSecurityValidatorIdentity(match.Event),
		AuthorizationID: researchSecurityAuthorizationID(match.Event),
		RunID:           researchSecurityFeatureString(match.Event.Features, "validation.runId", "outcome.runId", "proof.runId", "validator.runId"),
		Authorized:      true,
	}
}

func researchSecurityValidatorIdentity(event ResearchEvent) string {
	if value := researchSecurityFeatureString(event.Features, "validation.validatorId", "outcome.validatorId", "proof.validatorId", "validator.id", "validatorId"); value != "" {
		return value
	}
	return strings.TrimSpace(event.Source)
}

func researchSecurityAuthorizationID(event ResearchEvent) string {
	return researchSecurityFeatureString(event.Features, "validation.authorizationId", "outcome.authorizationId", "proof.authorizationId", "authorization.id", "authorizationId")
}

func researchSecurityRefutationIndependent(row ResearchSecurityEvaluationSampleRow, reachable, reproduced, impact, refuted researchSecurityCorrelatedEvent) bool {
	refuter := researchSecurityValidatorIdentity(refuted.Event)
	if refuter == "" {
		return false
	}
	baseline := ""
	for _, match := range []researchSecurityCorrelatedEvent{impact, reproduced, reachable} {
		if match.Event.ID == "" && match.Event.Source == "" && len(match.Event.Features) == 0 {
			continue
		}
		baseline = researchSecurityValidatorIdentity(match.Event)
		if baseline != "" {
			break
		}
	}
	if baseline == "" {
		baseline = strings.TrimSpace(row.Source)
	}
	return baseline == "" || !strings.EqualFold(refuter, baseline)
}

func researchSecurityOutcomeFindingKey(row ResearchSecurityEvaluationSampleRow, matches []researchSecurityCorrelatedEvent) string {
	for _, match := range matches {
		if value := researchSecurityFeatureString(match.Event.Features, "validation.findingKey", "outcome.findingKey", "proof.findingKey", "dedupe.key", "finding.key"); value != "" {
			return value
		}
	}
	category := strings.ToLower(strings.TrimSpace(row.Category))
	comm := strings.ToLower(strings.TrimSpace(row.Comm))
	target := strings.TrimSpace(row.Target)
	if target != "" {
		return researchStableID("security-outcome-finding", category, comm, target)
	}
	return researchStableID("security-outcome-finding", category, comm, strings.TrimSpace(row.TraceID), strings.TrimSpace(row.CommandLine))
}

func researchSecurityFinalizeOutcomeFindings(summary *ResearchSecurityOutcomeValidationSummary, rows []ResearchSecurityEvaluationSampleRow) {
	if summary == nil {
		return
	}
	if !summary.DedupeActionable {
		summary.UniqueActionable = len(rows)
		if len(rows) > researchSecurityEvaluationMaxFindings {
			rows = rows[:researchSecurityEvaluationMaxFindings]
		}
		summary.Findings = append(summary.Findings, rows...)
		return
	}
	seen := map[string]struct{}{}
	for _, row := range rows {
		key := strings.TrimSpace(row.FindingKey)
		if key == "" {
			key = row.ID
		}
		if _, ok := seen[key]; ok {
			summary.DuplicateActionable++
			continue
		}
		seen[key] = struct{}{}
		summary.UniqueActionable++
		if len(summary.Findings) < researchSecurityEvaluationMaxFindings {
			summary.Findings = append(summary.Findings, row)
		}
	}
}

func researchSecurityFeatureString(features map[string]any, paths ...string) string {
	for _, path := range paths {
		if len(features) == 0 {
			return ""
		}
		if value, ok := features[path]; ok {
			if text := researchSecurityString(value); text != "" {
				return text
			}
		}
		parts := strings.Split(path, ".")
		var current any = features
		found := true
		for _, part := range parts {
			m, ok := current.(map[string]any)
			if !ok {
				found = false
				break
			}
			current, ok = m[part]
			if !ok {
				found = false
				break
			}
		}
		if found {
			if text := researchSecurityString(current); text != "" {
				return text
			}
		}
	}
	return ""
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
