package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func researchSecurityActionsMatch(expected, observed string) bool {
	expected = normalizeResearchSecurityAction(expected)
	observed = normalizeResearchSecurityAction(observed)
	if !researchSecurityActionIsLabeled(expected) {
		return true
	}
	if expected == observed {
		return true
	}
	return expected != "ALLOW" && observed != "ALLOW"
}

func normalizeResearchSecurityAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ALLOW", "ALLOWED", "PASS":
		return "ALLOW"
	case "ALERT", "WARN", "WARNING":
		return "ALERT"
	case "BLOCK", "DENY", "DENIED":
		return "BLOCK"
	case "REWRITE":
		return "REWRITE"
	case "UNLABELED", "UNKNOWN", "", "-", "NONE":
		return "UNLABELED"
	default:
		return strings.ToUpper(strings.TrimSpace(action))
	}
}

func researchSecurityActionIsLabeled(action string) bool {
	switch normalizeResearchSecurityAction(action) {
	case "ALLOW", "ALERT", "BLOCK", "REWRITE":
		return true
	default:
		return false
	}
}

func researchSecurityObservedActionFromRisk(score float64) string {
	switch {
	case score >= 90:
		return "BLOCK"
	case score >= 60:
		return "ALERT"
	default:
		return "ALLOW"
	}
}

func normalizeResearchSecurityEvaluationMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "builtin", "benchmark", "baseline":
		return researchSecurityEvaluationModeBuiltin
	case "session", "events", "research":
		return researchSecurityEvaluationModeSession
	case "", "combined", "all", "both":
		return researchSecurityEvaluationModeCombined
	default:
		return researchSecurityEvaluationDefaultMode
	}
}

func normalizeResearchSecurityEvaluationLabelPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "decision", "decision_only":
		return "decision"
	case "unlabeled", "manual", "none":
		return "unlabeled"
	case "", "decision_then_heuristic", "decision_then_heuristics", "auto":
		return researchSecurityEvaluationLabelDecision
	default:
		return researchTrainingPolicyHeuristic
	}
}

func normalizeResearchSecurityEvaluationLimit(limit int) int {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if limit <= 0 {
		limit = researchDefaultTaskLimit
	}
	if settings.MaxSessionEvents > 0 && limit > settings.MaxSessionEvents {
		limit = settings.MaxSessionEvents
	}
	if limit > researchMaxTaskLimit {
		limit = researchMaxTaskLimit
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}

func normalizeResearchSecurityBenchmarkArgs(comm string, args []string) []string {
	out := append([]string(nil), args...)
	if len(out) > 0 && strings.EqualFold(strings.TrimSpace(out[0]), strings.TrimSpace(comm)) {
		out = out[1:]
	}
	return out
}

func researchSecurityAssessmentSignals(assessment map[string]any) map[string]any {
	signals := map[string]any{
		"riskLevel":    researchSecurityString(assessment["riskLevel"]),
		"anomalyScore": researchSecurityFloat(assessment["anomalyScore"]),
		"modelLoaded":  assessment["modelLoaded"],
		"mlEnabled":    assessment["mlEnabled"],
	}
	ml := researchSecurityMap(assessment["mlPrediction"])
	if len(ml) > 0 {
		signals["mlAction"] = researchSecurityString(ml["action"])
		signals["mlConfidence"] = researchSecurityFloat(ml["confidence"])
	}
	network := researchSecurityMap(assessment["networkAudit"])
	if len(network) > 0 {
		signals["networkRisk"] = firstNonEmptyResearchSecurityString(researchSecurityString(network["riskLevel"]), researchSecurityString(network["risk"]))
		signals["networkScore"] = researchSecurityFloat(network["riskScore"])
	}
	classification := researchSecurityMap(assessment["classification"])
	if len(classification) > 0 {
		signals["classificationCategory"] = firstNonEmptyResearchSecurityString(researchSecurityString(classification["primaryCategory"]), researchSecurityString(classification["PrimaryCategory"]))
		signals["classificationConfidence"] = firstNonEmptyResearchSecurityString(researchSecurityString(classification["confidence"]), researchSecurityString(classification["Confidence"]))
	}
	return signals
}

func researchSecurityAssessmentConfidence(assessment map[string]any) float64 {
	ml := researchSecurityMap(assessment["mlPrediction"])
	confidence := researchSecurityFloat(ml["confidence"])
	evidence := researchSecurityMap(assessment["sampleEvidence"])
	if sampleConfidence := researchSecurityFloat(evidence["confidence"]); sampleConfidence > confidence {
		confidence = sampleConfidence
	}
	if confidence > 1 && confidence <= 100 {
		confidence = confidence / 100
	}
	return researchSecurityRoundFloat(confidence, 3)
}

func researchSecurityMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

func researchSecurityString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func researchSecurityFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func researchSecurityRiskBucket(score float64) string {
	switch {
	case score >= 80:
		return "80-100"
	case score >= 60:
		return "60-79"
	case score >= 40:
		return "40-59"
	case score >= 20:
		return "20-39"
	default:
		return "0-19"
	}
}

func firstNonEmptyResearchSecurityString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyResearchSecurityParam(params map[string]any, fallback string, keys ...string) string {
	for _, key := range keys {
		if value := researchSecurityString(params[key]); value != "" {
			return value
		}
	}
	return fallback
}

func researchSecurityParamInt(params map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return int(researchSecurityFloat(value)), true
		}
	}
	return 0, false
}

func researchSecurityParamBool(params map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "on":
				return true, true
			case "false", "0", "no", "off":
				return false, true
			}
		default:
			return researchSecurityFloat(typed) != 0, true
		}
	}
	return false, false
}

func percentFloat(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return researchSecurityRoundFloat(float64(numerator)/float64(denominator)*100, 2)
}

func researchSecurityRoundFloat(value float64, places int) float64 {
	if places <= 0 {
		return math.Round(value)
	}
	factor := math.Pow10(places)
	return math.Round(value*factor) / factor
}

func researchSecurityEvaluationJSONBytes(report *ResearchSecurityEvaluationReport) ([]byte, error) {
	if report == nil {
		return nil, errors.New("security evaluation report is unavailable")
	}
	return json.MarshalIndent(report, "", "  ")
}

func researchSecurityEvaluationJSONLBytes(report *ResearchSecurityEvaluationReport) []byte {
	var buf bytes.Buffer
	if report == nil {
		return nil
	}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, sample := range report.Samples {
		_ = encoder.Encode(sample)
	}
	return buf.Bytes()
}

func researchSecurityEvaluationCSVBytes(report *ResearchSecurityEvaluationReport) ([]byte, error) {
	if report == nil {
		return nil, errors.New("security evaluation report is unavailable")
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"id", "event_id", "timestamp", "time", "source", "event_type", "category", "comm", "command_line", "expected_action", "expected_source", "observed_action", "passed", "finding_type", "risk_score", "risk_level", "confidence", "target", "trace_id", "span_id", "benchmark_case", "reasoning", "recommendation"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range report.Samples {
		record := []string{
			row.ID,
			row.EventID,
			strconv.FormatInt(row.Timestamp, 10),
			row.Time,
			row.Source,
			row.EventType,
			row.Category,
			row.Comm,
			row.CommandLine,
			row.ExpectedAction,
			row.ExpectedSource,
			row.ObservedAction,
			strconv.FormatBool(row.Passed),
			row.FindingType,
			researchFloatString(row.RiskScore),
			row.RiskLevel,
			strconv.FormatFloat(row.Confidence, 'f', -1, 64),
			row.Target,
			row.TraceID,
			row.SpanID,
			row.BenchmarkCase,
			row.Reasoning,
			row.Recommendation,
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}
