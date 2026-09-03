package research

import "fmt"

func buildResearchSecurityOutcomePosture(report ResearchSecurityEvaluationReport) ResearchSecurityEvaluationPosture {
	summary := report.OutcomeValidation
	if summary == nil || !summary.Enabled {
		return buildResearchSecurityEvaluationPosture(report)
	}

	base := buildResearchSecurityEvaluationPosture(report)
	findingCounts := map[string]int{}
	if summary.Actionable > 0 {
		findingCounts["outcome_actionable"] = summary.Actionable
	}
	if summary.ImpactConfirmed > 0 {
		findingCounts["impact_confirmed"] = summary.ImpactConfirmed
	}
	if summary.Reproduced > 0 {
		findingCounts["reproduced"] = summary.Reproduced
	}
	if summary.Reachable > 0 {
		findingCounts["reachable"] = summary.Reachable
	}
	if summary.Unproven > 0 {
		findingCounts["unproven"] = summary.Unproven
	}
	if summary.Rejected > 0 {
		findingCounts["rejected"] = summary.Rejected
	}

	maxActionableRisk := 0.0
	for _, row := range summary.Findings {
		if row.RiskScore > maxActionableRisk {
			maxActionableRisk = row.RiskScore
		}
	}

	posture := ResearchSecurityEvaluationPosture{
		Status:               "pass",
		RiskScore:            researchSecurityRoundFloat(maxActionableRisk, 2),
		FindingCounts:        topResearchOutcomeCounts(findingCounts),
		TopFailingCategories: base.TopFailingCategories,
	}

	if summary.ImpactConfirmed > 0 {
		posture.Status = "critical"
		if posture.RiskScore < 90 {
			posture.RiskScore = 90
		}
		posture.BlockingReasons = append(posture.BlockingReasons, fmt.Sprintf("impact_confirmed:%d", summary.ImpactConfirmed))
	} else if summary.Actionable > 0 {
		if posture.RiskScore < 60 {
			posture.RiskScore = 60
		}
		if posture.RiskScore >= 70 {
			posture.Status = "critical"
		} else {
			posture.Status = "needs_review"
		}
		posture.BlockingReasons = append(posture.BlockingReasons, fmt.Sprintf("reproduced_actionable_findings:%d", summary.Actionable))
	} else if summary.Reachable > 0 || summary.Unproven > 0 {
		posture.Status = "needs_review"
		if posture.RiskScore < 20 {
			posture.RiskScore = 20
		}
	}

	if summary.Reachable > summary.Reproduced {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("reachable_without_reproduction:%d", summary.Reachable-summary.Reproduced))
	}
	if summary.Unproven > 0 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("unproven_candidates:%d", summary.Unproven))
	}
	if summary.Rejected > 0 {
		posture.Warnings = append(posture.Warnings, fmt.Sprintf("adversarially_rejected:%d", summary.Rejected))
	}
	if base.Status != "pass" {
		posture.Warnings = append(posture.Warnings, "detector_quality_"+base.Status)
	}

	posture.SuggestedActions = researchSecurityOutcomeSuggestedActions(summary)
	posture.RemediationPlan = buildResearchSecurityOutcomeRemediationPlan(report, *summary)
	return posture
}

func topResearchOutcomeCounts(counts map[string]int) []researchCount {
	items := make([]researchCount, 0, len(counts))
	order := []string{"impact_confirmed", "outcome_actionable", "reproduced", "reachable", "unproven", "rejected"}
	for _, key := range order {
		if count := counts[key]; count > 0 {
			items = append(items, researchCount{Key: key, Count: count})
		}
	}
	return items
}

func researchSecurityOutcomeSuggestedActions(summary *ResearchSecurityOutcomeValidationSummary) []string {
	if summary == nil {
		return nil
	}
	actions := []string{}
	if summary.ImpactConfirmed > 0 {
		actions = append(actions, "prioritize_confirmed_impact_remediation")
	} else if summary.Actionable > 0 {
		actions = append(actions, "prioritize_reproduced_findings_for_remediation")
	}
	if summary.Reachable > summary.Reproduced {
		actions = append(actions, "collect_reproduction_proof_for_reachable_candidates")
	}
	if summary.Unproven > 0 {
		actions = append(actions, "collect_explicit_reachability_or_reproduction_evidence")
	}
	if summary.Rejected > 0 {
		actions = append(actions, "retain_refutation_evidence_to_reduce_repeat_noise")
	}
	if len(actions) == 0 {
		actions = append(actions, "outcome_validation_passed_export_report_for_reproducibility")
	}
	return actions
}

func buildResearchSecurityOutcomeRemediationPlan(report ResearchSecurityEvaluationReport, summary ResearchSecurityOutcomeValidationSummary) []ResearchSecurityRemediationItem {
	items := []ResearchSecurityRemediationItem{}
	if summary.Actionable > 0 {
		priority := "high"
		area := "reproduced_outcome"
		action := "prioritize_reproduced_findings_for_remediation"
		rationale := "These findings crossed the configured evidence threshold and should be prioritized over unproven hypotheses."
		if summary.ImpactConfirmed > 0 {
			priority = "critical"
			area = "confirmed_impact"
			action = "prioritize_confirmed_impact_remediation"
			rationale = "At least one authorized validation run recorded confirmed security impact."
		}
		items = append(items, ResearchSecurityRemediationItem{
			ID:              researchStableID("security-outcome-remediation", report.SessionID, action),
			Priority:        priority,
			Area:            area,
			FindingType:     "outcome_actionable",
			Action:          action,
			Rationale:       rationale,
			Count:           summary.Actionable,
			RelatedCommands: researchSecurityTopCommands(summary.Findings, 5),
		})
	}
	if summary.Reachable > summary.Reproduced {
		items = append(items, ResearchSecurityRemediationItem{
			ID:          researchStableID("security-outcome-remediation", report.SessionID, "collect-reproduction"),
			Priority:    "medium",
			Area:        "evidence_gap",
			FindingType: "reachable_only",
			Action:      "collect_reproduction_proof_for_reachable_candidates",
			Rationale:   "Reachability is proven, but reproduction has not crossed the default actionable evidence threshold.",
			Count:       summary.Reachable - summary.Reproduced,
		})
	}
	if summary.Unproven > 0 {
		items = append(items, ResearchSecurityRemediationItem{
			ID:          researchStableID("security-outcome-remediation", report.SessionID, "collect-evidence"),
			Priority:    "low",
			Area:        "evidence_gap",
			FindingType: "unproven",
			Action:      "collect_explicit_reachability_or_reproduction_evidence",
			Rationale:   "Candidates without explicit validation evidence remain hypotheses and should not enter the remediation queue yet.",
			Count:       summary.Unproven,
		})
	}
	if len(items) == 0 {
		items = append(items, ResearchSecurityRemediationItem{
			ID:        researchStableID("security-outcome-remediation", report.SessionID, "pass"),
			Priority:  "low",
			Area:      "reproducibility",
			Action:    "outcome_validation_passed_export_report_for_reproducibility",
			Rationale: "No candidate crossed the configured outcome-evidence threshold.",
			Count:     summary.Candidates,
		})
	}
	return items
}
