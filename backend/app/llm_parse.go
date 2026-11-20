package app

import (
	"agent-ebpf-filter/pb"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func buildLLMBehaviorContext(req llmScoreRequest) llmScoreContext {
	return llmScoreContext{
		CommandLine:    strings.TrimSpace(req.CommandLine),
		Comm:           strings.TrimSpace(req.Comm),
		Args:           append([]string(nil), req.Args...),
		Category:       strings.TrimSpace(req.Category),
		AnomalyScore:   req.AnomalyScore,
		Classification: req.Classification,
		MlAction:       strings.TrimSpace(req.MlAction),
		MlConfidence:   req.MlConfidence,
		NetworkRisk:    strings.TrimSpace(req.NetworkRisk),
		NetworkScore:   req.NetworkScore,
		SampleEvidence: strings.TrimSpace(req.SampleEvidence),
		CurrentLabel:   strings.TrimSpace(req.CurrentLabel),
		Source:         strings.TrimSpace(req.Source),
	}
}

type llmScoreContext struct {
	CommandLine    string                     `json:"commandLine"`
	Comm           string                     `json:"comm"`
	Args           []string                   `json:"args"`
	Category       string                     `json:"category,omitempty"`
	AnomalyScore   float64                    `json:"anomalyScore,omitempty"`
	Classification *pb.BehaviorClassification `json:"classification,omitempty"`
	MlAction       string                     `json:"mlAction,omitempty"`
	MlConfidence   float64                    `json:"mlConfidence,omitempty"`
	NetworkRisk    string                     `json:"networkRisk,omitempty"`
	NetworkScore   float64                    `json:"networkScore,omitempty"`
	SampleEvidence string                     `json:"sampleEvidence,omitempty"`
	CurrentLabel   string                     `json:"currentLabel,omitempty"`
	Source         string                     `json:"source,omitempty"`
}

func buildLLMScoringPrompt(contextJSON string) string {
	return fmt.Sprintf(`Score the following behavior for security risk.

Return STRICT JSON only with these keys:
- riskScore: number from 0 to 100
- recommendedAction: one of ALLOW, ALERT, BLOCK, REWRITE
- confidence: number from 0 to 1
- reasoning: short explanation
- signals: array of concise strings

Behavior context:
%s
`, contextJSON)
}

func parseLLMScoreContent(content string) (*llmScoringResult, error) {
	jsonPayload := extractJSONObject(content)
	if jsonPayload == "" {
		return nil, errors.New("LLM response did not contain JSON")
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return nil, err
	}

	result := &llmScoringResult{
		RiskScore:         min(max(extractLLMFloat(raw, "riskScore", "risk_score", "score", "risk"), 0), 100),
		Confidence:        min(max(extractLLMFloat(raw, "confidence", "conf"), 0), 1),
		RecommendedAction: normalizeLLMAction(extractLLMString(raw, "recommendedAction", "recommended_action", "action", "label")),
		Reasoning:         truncateLLMText(strings.TrimSpace(extractLLMString(raw, "reasoning", "explanation", "analysis")), maxLLMReasoningBytes),
		Signals:           extractLLMStrings(raw, "signals", "evidence", "reasons"),
	}
	if result.RecommendedAction == "" {
		result.RecommendedAction = llmActionFromRiskScore(result.RiskScore)
	}
	if result.Confidence == 0 && result.RiskScore > 0 {
		result.Confidence = min(max(result.RiskScore/100.0, 0.1), 0.99)
	}
	if result.Reasoning == "" {
		result.Reasoning = "LLM returned a risk score without reasoning"
	}
	return result, nil
}

func extractJSONObject(content string) string {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func extractLLMString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func extractLLMFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			switch v := val.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			case int:
				return float64(v)
			case int64:
				return float64(v)
			case json.Number:
				if f, err := v.Float64(); err == nil {
					return f
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func extractLLMStrings(raw map[string]any, keys ...string) []string {
	for _, key := range keys {
		val, ok := raw[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case []any:
			out := make([]string, 0, min(len(v), maxLLMSignalCount))
			for _, item := range v {
				if s, ok := item.(string); ok {
					if trimmed := strings.TrimSpace(s); trimmed != "" {
						out = append(out, truncateLLMText(trimmed, maxLLMSignalBytes))
						if len(out) >= maxLLMSignalCount {
							break
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case []string:
			out := make([]string, 0, min(len(v), maxLLMSignalCount))
			for _, s := range v {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					out = append(out, truncateLLMText(trimmed, maxLLMSignalBytes))
					if len(out) >= maxLLMSignalCount {
						break
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return []string{truncateLLMText(trimmed, maxLLMSignalBytes)}
			}
		}
	}
	return nil
}

func truncateLLMText(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return strings.ToValidUTF8(value[:maxBytes], "")
}

func normalizeLLMAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ALLOW", "BLOCK", "ALERT", "REWRITE":
		return strings.ToUpper(strings.TrimSpace(action))
	default:
		return ""
	}
}

func llmActionFromRiskScore(score float64) string {
	switch {
	case score >= 80:
		return "BLOCK"
	case score >= 60:
		return "ALERT"
	case score >= 40:
		return "REWRITE"
	default:
		return "ALLOW"
	}
}
