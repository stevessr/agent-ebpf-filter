package research

import "testing"

func TestOutcomeValidationPromotesReproducedFinding(t *testing.T) {
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
			ID:      "event-1",
			Source:  "runtime",
			TraceID: "trace-1",
			Features: map[string]any{
				"validation": map[string]any{
					"reproduced": true,
				},
			},
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:    "outcome",
		MinimumEvidence:   "reproduced",
		AdversarialReview: true,
	})

	if report.OutcomeValidation == nil {
		t.Fatal("expected outcome validation summary")
	}
	if got := report.OutcomeValidation.Actionable; got != 1 {
		t.Fatalf("expected one actionable finding, got %d", got)
	}
	row := report.Samples[0]
	if !row.Reachable || !row.Reproduced || row.ImpactConfirmed {
		t.Fatalf("unexpected evidence state: %+v", row)
	}
	if row.ValidationStatus != researchSecurityEvidenceReproduced || !row.Actionable {
		t.Fatalf("expected reproduced actionable status, got status=%q actionable=%v", row.ValidationStatus, row.Actionable)
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
		ValidationMode:  "outcome",
		MinimumEvidence: "reachable",
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

func TestOutcomeValidationExplicitReachabilityHonorsThreshold(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session"},
		},
	}
	events := []ResearchEvent{
		{
			ID:     "event-1",
			Source: "trace-validator",
			Features: map[string]any{
				"validation.reachable": true,
			},
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:  "outcome",
		MinimumEvidence: "reachable",
	})

	row := report.Samples[0]
	if !row.Reachable || row.Reproduced || !row.Actionable {
		t.Fatalf("explicit reachability should satisfy reachable threshold: %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.Reachable != 1 || report.OutcomeValidation.Actionable != 1 {
		t.Fatalf("unexpected summary: %+v", report.OutcomeValidation)
	}
}

func TestOutcomeValidationAdversarialRefutationWinsWithoutImpact(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Samples: []ResearchSecurityEvaluationSampleRow{
			{ID: "finding-1", EventID: "event-1", Source: "session"},
		},
	}
	events := []ResearchEvent{
		{
			ID:     "event-1",
			Source: "validator",
			Features: map[string]any{
				"validation.reproduced": true,
				"validation.refuted":   true,
			},
		},
	}

	applyResearchSecurityOutcomeValidation(&report, events, ResearchSecurityEvaluationRequest{
		ValidationMode:    "outcome",
		MinimumEvidence:   "reachable",
		AdversarialReview: true,
	})

	row := report.Samples[0]
	if row.ValidationStatus != "rejected" || row.Actionable {
		t.Fatalf("expected adversarial rejection, got %+v", row)
	}
	if report.OutcomeValidation == nil || report.OutcomeValidation.Rejected != 1 {
		t.Fatalf("expected one rejected candidate, got %+v", report.OutcomeValidation)
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
	if !req.AdversarialReview {
		t.Fatal("expected adversarial review to default on for outcome mode")
	}
}
