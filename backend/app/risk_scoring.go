package app

import (
	"math"
	"strings"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/pb"
)

// computeRiskScore combines classification, anomaly, and ML into a 0-100 risk score.
// Uses app-level types (ml.Prediction, NetworkAuditResult, llmAssessment).
func computeRiskScore(classification *pb.BehaviorClassification, anomalyScore float64, mlPrediction ml.Prediction, netAudit NetworkAuditResult, llmAssessment *llmAssessment) float64 {
	score := 0.0

	if classification != nil {
		switch classification.PrimaryCategory {
		case "SENSITIVE":
			score += 35
		case "FILE_DELETE", "PROCESS_KILL":
			score += 28
		case "FILE_PERMISSION", "NETWORK":
			score += 18
		case "PROCESS_EXEC", "FILE_WRITE":
			score += 13
		case "CONTAINER", "DATABASE":
			score += 8
		case "PACKAGE_MANAGER", "COMPRESSION":
			score += 5
		}
		switch classification.Confidence {
		case "high":
			score += 10
		case "medium":
			score += 5
		}
	}

	score += anomalyScore * 30

	if mlPrediction.Confidence >= 0.60 {
		switch mlPrediction.Action {
		case 1:
			score += mlPrediction.Confidence * 25
		case 3:
			score += mlPrediction.Confidence * 15
		case 2:
			score += mlPrediction.Confidence * 8
		}
	}

	switch netAudit.RiskLevel {
	case "CRITICAL":
		score += 20
	case "HIGH":
		score += 15
	case "MEDIUM":
		score += 10
	case "LOW":
		score += 5
	}

	if llmAssessment != nil && strings.TrimSpace(llmAssessment.Error) == "" {
		score += clampFloat64(llmAssessment.RiskScore*0.18, 0, 20)
		if llmAssessment.Confidence > 0 {
			score += clampFloat64(llmAssessment.Confidence*6, 0, 6)
		}
		switch llmAssessment.RecommendedAction {
		case "BLOCK":
			score += 8
		case "ALERT":
			score += 5
		case "REWRITE":
			score += 3
		}
	}

	if score > 100 {
		score = 100
	}
	return math.Round(score)
}

// riskLevel maps a risk score to a severity level.
func riskLevel(score float64) string {
	switch {
	case score >= 80:
		return "CRITICAL"
	case score >= 60:
		return "HIGH"
	case score >= 40:
		return "MEDIUM"
	case score >= 20:
		return "LOW"
	default:
		return "SAFE"
	}
}
