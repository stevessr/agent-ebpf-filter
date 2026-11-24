package research

import (
	"strings"
)

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
