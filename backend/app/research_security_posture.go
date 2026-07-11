package app

import (
	"fmt"
	"sort"
	"strings"
)

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
	posture.RemediationPlan = buildResearchSecurityRemediationPlan(report, posture)
	return posture
}

func buildResearchSecurityRemediationPlan(report ResearchSecurityEvaluationReport, posture ResearchSecurityEvaluationPosture) []ResearchSecurityRemediationItem {
	items := []ResearchSecurityRemediationItem{}
	appendItem := func(priority, area, findingType, category, action, rationale string, count int, rows []ResearchSecurityEvaluationSampleRow) {
		if count <= 0 {
			return
		}
		items = append(items, ResearchSecurityRemediationItem{
			ID:              researchStableID("security-remediation", priority, area, findingType, category, action),
			Priority:        priority,
			Area:            area,
			FindingType:     findingType,
			Category:        category,
			Action:          action,
			Rationale:       rationale,
			Count:           count,
			RelatedCommands: researchSecurityTopCommands(rows, 5),
		})
	}

	appendItem(
		"critical",
		"detection_coverage",
		"false_negative",
		"",
		"tighten_detection_for_missed_risky_agent_behaviors",
		"Risky Agent behaviors were expected to be blocked or alerted but were observed as ALLOW; prioritize detection and policy coverage before trusting the model.",
		len(report.Findings.FalseNegatives),
		report.Findings.FalseNegatives,
	)
	appendItem(
		"critical",
		"manual_audit",
		"high_confidence_disagreement",
		"",
		"prioritize_manual_audit_for_high_confidence_disagreements",
		"The evaluator produced high-confidence disagreements against the expected action; require human review before changing runtime policy.",
		len(report.Findings.HighConfidenceDisagreements),
		report.Findings.HighConfidenceDisagreements,
	)
	appendItem(
		"high",
		"policy_explainability",
		"policy_gap",
		"",
		"inspect_policy_gap_signal_weights_and_explanations",
		"Alert-like decisions had weak risk evidence; review signal weights, explanations and threshold boundaries.",
		len(report.Findings.PolicyGaps),
		report.Findings.PolicyGaps,
	)
	appendItem(
		"medium",
		"threshold_tuning",
		"false_positive",
		"",
		"review_over_sensitive_rules_or_ml_thresholds",
		"Benign Agent workflows were flagged; tune thresholds or add known-safe samples before enabling stricter blocking.",
		len(report.Findings.FalsePositives),
		report.Findings.FalsePositives,
	)
	appendItem(
		"high",
		"labeling",
		"unlabeled_high_risk",
		"",
		"route_unlabeled_high_risk_events_to_research_labeling",
		"High-risk unlabeled events should be labeled or reviewed so future sessions can distinguish true attacks from noisy heuristics.",
		len(report.Findings.UnlabeledHighRisk),
		report.Findings.UnlabeledHighRisk,
	)

	for _, group := range posture.TopFailingCategories {
		if group.Failed <= 0 {
			continue
		}
		priority := "medium"
		if group.FalseNegatives > 0 || group.AvgRiskScore >= 70 {
			priority = "high"
		}
		items = append(items, ResearchSecurityRemediationItem{
			ID:        researchStableID("security-remediation-category", group.Key),
			Priority:  priority,
			Area:      "category_drilldown",
			Category:  group.Key,
			Action:    "drill_down_top_failing_category",
			Rationale: fmt.Sprintf("Category %s has %d failed evaluations with %.1f average risk; inspect representative events and update labels, thresholds or rules.", group.Key, group.Failed, group.AvgRiskScore),
			Count:     group.Failed,
		})
	}

	if len(items) == 0 && posture.Status == "pass" {
		items = append(items, ResearchSecurityRemediationItem{
			ID:        researchStableID("security-remediation", "pass", report.SessionID, report.GeneratedAt),
			Priority:  "low",
			Area:      "reproducibility",
			Action:    "security_posture_passed_export_report_for_reproducibility",
			Rationale: "No blocking security findings were detected; export the report and bundle so the passing baseline can be reproduced later.",
			Count:     report.Totals.Total,
		})
	}
	return items
}

func researchSecurityTopCommands(rows []ResearchSecurityEvaluationSampleRow, limit int) []string {
	if len(rows) == 0 || limit == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, row := range rows {
		cmd := strings.TrimSpace(row.CommandLine)
		if cmd == "" {
			cmd = strings.TrimSpace(row.Comm)
		}
		if cmd == "" {
			continue
		}
		counts[cmd]++
	}
	items := make([]researchCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, researchCount{Key: key, Count: count})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Key)
	}
	return out
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
