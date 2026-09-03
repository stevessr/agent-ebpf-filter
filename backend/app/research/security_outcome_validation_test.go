package research

import "testing"

func authorizedValidationFeatures(extra map[string]any) map[string]any {
	validation := map[string]any{
		"authorized":      true,
		"validatorId":     "validator-a",
		"authorizationId": "auth-test",
		"runId":           "run-test",
	}
	for key, value := range extra {
		validation[key] = value
	}
	return map[string]any{"validation": validation}
}

func TestOutcomeValidationPromotesAuthorizedReproducedFinding(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{
				ID:        "finding-1",
				EventID:   "event-1",
				Source:    "session",
				TraceID:   "trace-1",
				Comm:      "curl",
				Target:    "https://example.invalid/api",
				RiskScore: 91,
			},
		},
	}
	events := []ResearchEvent{
		{
			ID:       "proof-1",
			Source:   "validator",
			TraceID:  "trace-1",
			Features: authorizedValidationFeatures(map[string]any{"reproduced": true}),
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:       "outcome",
		MinimumEvidence:      "reproduced",
		AdversarialReview:    true,
		RequireAuthorization: true,
		DedupeActionable:     true,
	})

	if report.OutcomeValidation == nil {
		t.Fatal("expected outcome validation summary")
	}
	if got := report.OutcomeValidation.Actionable; got != 1 {
		t.Fatalf("expected one actionable finding, got %d", got)
	}
	if got := report.OutcomeValidation.UniqueActionable; got != 1 {
		t.Fatalf("expected one unique actionable finding, got %d", got)
	}
	row := report.Samples[0]
	if !row.Reachable || !row.Reproduced || row.ImpactConfirmed {
		t.Fatalf("unexpected evidence state: %+v", row)
	}
	if row.ValidationStatus != researchSecurityEvidenceReproduced || !row.Actionable {
		t.Fatalf("expected reproduced actionable status, got status=%q actionable=%v", row.ValidationStatus, row.Actionable)
	}
	if len(row.Evidence) != 1 || !row.Evidence[0].Authorized || row.Evidence[0].ValidatorID != "validator-a" {
		t.Fatalf("expected structured authorized evidence, got %+v", row.Evidence)
	}
}

func TestOutcomeValidationDoesNotInferReachabilityFromCandidateEvent(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", TraceID: "trace-1"},
		},
	}
	events := []ResearchEvent{{ID: "event-1", Source: "runtime", TraceID: "trace-1"}}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:       "outcome",
		MinimumEvidence:      "reachable",
		RequireAuthorization: true,
	})

	row := report.Samples[0]
	if row.Reachable || row.Reproduced || row.Actionable {
		t.Fatalf("candidate correlation alone must stay unproven: %+v", row)
	}
	if row.ValidationStatus != "unproven" || row.EvidenceLevel != researchSecurityEvidenceHypothesis {
		t.Fatalf("expected hypothesis/unproven state, got %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.Unproven != 1 || report.OutcomeValidation.Reachable != 0 {
		t.Fatalf("unexpected summary: %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationExplicitAuthorizedReachabilityHonorsThreshold(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session"},
		},
	}
	events := []ResearchEvent{
		{
			ID:     "proof-1",
			Source: "trace-validator",
			Features: authorizedValidationFeatures(map[string]any{
				"candidateEventId": "event-1",
				"reachable":        true,
			}),
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:       "outcome",
		MinimumEvidence:      "reachable",
		RequireAuthorization: true,
		DedupeActionable:     true,
	})

	row := report.Samples[0]
	if !row.Reachable || row.Reproduced || !row.Actionable {
		t.Fatalf("explicit reachability should satisfy reachable threshold: %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.Reachable != 1 || report.OutcomeValidation.Actionable != 1 {
		t.Fatalf("unexpected summary: %+v", report.OutcomeValidation)
	}
	if len(row.Evidence) != 1 || row.Evidence[0].Correlation != "candidate_event_id" {
		t.Fatalf("expected explicit candidate event correlation, got %+v", row.Evidence)
	}
}

func TestOutcomeValidationIgnoresUnauthorizedEvidence(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", TraceID: "trace-1"},
		},
	}
	events := []ResearchEvent{
		{
			ID:      "proof-1",
			Source:  "validator",
			TraceID: "trace-1",
			Features: map[string]any{
				"validation.reproduced": true,
			},
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:       "outcome",
		MinimumEvidence:      "reproduced",
		RequireAuthorization: true,
	})

	row := report.Samples[0]
	if row.Actionable || row.Reproduced || row.ValidationStatus != "unproven" {
		t.Fatalf("unauthorized evidence must not promote a candidate: %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.UnauthorizedEvidence != 1 {
		t.Fatalf("expected one ignored unauthorized evidence event, got %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationEnforcesAuthorizationAndValidatorAllowlists(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", TraceID: "trace-1"},
		},
	}
	events := []ResearchEvent{
		{
			ID:       "proof-1",
			Source:   "validator-dev",
			TraceID:  "trace-1",
			Features: authorizedValidationFeatures(map[string]any{"reproduced": true}),
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:          "outcome",
		MinimumEvidence:         "reproduced",
		RequireAuthorization:    true,
		AllowedValidatorSources: []string{"validator-prod*"},
		AllowedAuthorizationIDs: []string{"auth-prod"},
	})

	if report.Samples[0].Actionable {
		t.Fatalf("disallowed validator source/authorization must not be actionable: %+v", report.Samples[0])
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.UnauthorizedEvidence != 1 {
		t.Fatalf("expected disallowed proof to be counted as unauthorized: %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationTargetScope(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", Source: "session", Target: "https://outside.invalid/api"},
		},
	}

	applyResearchSecurityOutcomeValidation(&report, nil, ResearchSecurityEvaluationRequest{
		ValidationMode:   "outcome",
		AllowedTargets:   []string{"https://internal.invalid/*"},
		DedupeActionable: true,
	})

	row := report.Samples[0]
	if row.ValidationStatus != "out_of_scope" || row.Actionable {
		t.Fatalf("expected candidate to be excluded by target scope: %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.OutOfScope != 1 {
		t.Fatalf("unexpected summary: %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationIndependentAdversarialRefutationWinsWithoutImpact(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", TraceID: "trace-1"},
		},
	}
	proofFeatures := authorizedValidationFeatures(map[string]any{"reproduced": true})
	refuteFeatures := authorizedValidationFeatures(map[string]any{"refuted": true})
	refuteFeatures["validation"].(map[string]any)["validatorId"] = "validator-b"
	events := []ResearchEvent{
		{ID: "proof-1", Source: "validator", TraceID: "trace-1", Features: proofFeatures},
		{ID: "refute-1", Source: "validator", TraceID: "trace-1", Features: refuteFeatures},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:               "outcome",
		MinimumEvidence:              "reachable",
		AdversarialReview:            true,
		RequireAuthorization:         true,
		RequireIndependentRefutation: true,
	})

	row := report.Samples[0]
	if row.ValidationStatus != "rejected" || row.Actionable {
		t.Fatalf("expected independent adversarial rejection, got %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.Rejected != 1 {
		t.Fatalf("expected one rejected candidate, got %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationIgnoresSameValidatorRefutation(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", TraceID: "trace-1"},
		},
	}
	events := []ResearchEvent{
		{ID: "proof-1", Source: "validator", TraceID: "trace-1", Features: authorizedValidationFeatures(map[string]any{"reproduced": true})},
		{ID: "refute-1", Source: "validator", TraceID: "trace-1", Features: authorizedValidationFeatures(map[string]any{"refuted": true})},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:               "outcome",
		MinimumEvidence:              "reproduced",
		AdversarialReview:            true,
		RequireAuthorization:         true,
		RequireIndependentRefutation: true,
		DedupeActionable:             true,
	})

	row := report.Samples[0]
	if !row.Actionable || row.ValidationStatus != researchSecurityEvidenceReproduced {
		t.Fatalf("same-validator refutation should be ignored: %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.NonIndependentRefutations != 1 {
		t.Fatalf("expected one ignored non-independent refutation: %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationDedupesActionableVariants(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session", Category: "ssrf", Comm: "curl", Target: "https://service.invalid/api"},
			{ID: "finding-2", EventID: "event-2", Source: "session", Category: "ssrf", Comm: "curl", Target: "https://service.invalid/api"},
		},
	}
	proof1 := authorizedValidationFeatures(map[string]any{"candidateId": "finding-1", "reproduced": true})
	proof2 := authorizedValidationFeatures(map[string]any{"candidateId": "finding-2", "reproduced": true})
	events := []ResearchEvent{
		{ID: "proof-1", Source: "validator", Features: proof1},
		{ID: "proof-2", Source: "validator", Features: proof2},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:       "outcome",
		MinimumEvidence:      "reproduced",
		RequireAuthorization: true,
		DedupeActionable:     true,
	})

	summary := report.OutcomeValidation
	if summary == nil || summary.Actionable != 2 || summary.UniqueActionable != 1 || summary.DuplicateActionable != 1 {
		t.Fatalf("unexpected dedupe summary: %+v", summary)
	}
	if len(summary.Findings) != 1 {
		t.Fatalf("expected one deduped finding in remediation queue, got %d", len(summary.Findings))
	}
	if report.Samples[0].FindingKey == "" || report.Samples[0].FindingKey != report.Samples[1].FindingKey {
		t.Fatalf("expected variants to share a stable finding key: %q vs %q", report.Samples[0].FindingKey, report.Samples[1].FindingKey)
	}
}

func TestOutcomeValidationBuiltinIsNotApplicable(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{{ID: "fixture", Source: "builtin"}},
	}

	applyResearchSecurityOutcomeValidation(&report, nil, ResearchSecurityEvaluationRequest{ValidationMode: "outcome"})

	if report.OutcomeValidation == nil || report.OutcomeValidation.NotApplicable != 1 {
		t.Fatalf("unexpected summary: %+v", report.OutcomeValidation)
	}
	if report.Samples[0].ValidationStatus != "not_applicable" {
		t.Fatalf("unexpected status: %+v", report.Samples[0])
	}
}

func TestOutcomeCorpusAliasEnablesSafeDefaults(t *testing.T) {
	req := researchSecurityEvaluationRequestFromTask(researchTaskRequest{
		EvaluationMode: "session_outcome",
	})

	if req.Mode != researchSecurityEvaluationModeSession {
		t.Fatalf("expected session corpus, got %q", req.Mode)
	}
	if req.ValidationMode != researchSecurityValidationModeOutcome {
		t.Fatalf("expected outcome validation, got %q", req.ValidationMode)
	}
	if req.MinimumEvidence != researchSecurityEvidenceReproduced {
		t.Fatalf("expected reproduced threshold, got %q", req.MinimumEvidence)
	}
	if !req.AdversarialReview || !req.RequireAuthorization || !req.RequireIndependentRefutation || !req.DedupeActionable {
		t.Fatalf("expected conservative outcome defaults, got %+v", req)
	}
	if req.CorrelationWindowSeconds != researchSecurityOutcomeDefaultCorrelationWindowSeconds {
		t.Fatalf("expected default correlation window, got %d", req.CorrelationWindowSeconds)
	}
}

func TestOutcomeTaskParsesScopeAndCapsCorrelationWindow(t *testing.T) {
	req := researchSecurityEvaluationRequestFromTask(researchTaskRequest{
		EvaluationMode: "session_outcome",
		Params: map[string]any{
			"allowedValidatorSources":  []any{"validator-prod*", "trace-validator"},
			"allowedAuthorizationIds":  "auth-a, auth-b",
			"allowedTargets":           []string{"https://internal.invalid/*"},
			"correlationWindowSeconds": 9999,
		},
	})

	if req.CorrelationWindowSeconds != researchSecurityOutcomeMaxCorrelationWindowSeconds {
		t.Fatalf("expected bounded correlation window, got %d", req.CorrelationWindowSeconds)
	}
	if len(req.AllowedValidatorSources) != 2 || len(req.AllowedAuthorizationIDs) != 2 || len(req.AllowedTargets) != 1 {
		t.Fatalf("expected parsed scope lists, got %+v", req)
	}
}
