package core

import (
	"errors"
	"math"
)

// ── ML model types ───────────────────────────────────────────────────────────

// MLConfig holds all ML-related configuration.
type MLConfig struct {
	Enabled                  bool      `json:"enabled"`
	ModelType                ModelType `json:"modelType"`
	ModelPath                string    `json:"modelPath"`
	AutoTrain                bool      `json:"autoTrain"`
	TrainInterval            string    `json:"trainInterval"`
	MinSamplesForTraining    int       `json:"minSamplesForTraining"`
	BlockConfidenceThreshold float64   `json:"blockConfidenceThreshold"`
	AllowRiskScoreThreshold  float64   `json:"allowRiskScoreThreshold"`
	MlMinConfidence          float64   `json:"mlMinConfidence"`
	LowAnomalyThreshold      float64   `json:"lowAnomalyThreshold"`
	HighAnomalyThreshold     float64   `json:"highAnomalyThreshold"`
	RuleOverridePriority     int       `json:"ruleOverridePriority"`
	ActiveLearningEnabled    bool      `json:"activeLearningEnabled"`
	FeatureHistorySize       int       `json:"featureHistorySize"`
	NumTrees                 int       `json:"numTrees"`
	MaxDepth                 int       `json:"maxDepth"`
	MinSamplesLeaf           int       `json:"minSamplesLeaf"`
	ValidationSplitRatio     float64   `json:"validationSplitRatio"`
	// TrainingSeed=0 preserves nondeterministic online training; a nonzero
	// seed makes offline model comparisons and candidate artifacts reproducible.
	TrainingSeed             int64     `json:"trainingSeed,omitempty"`
	BalanceClasses           bool      `json:"balanceClasses"`
	EnsembleVoting           string    `json:"ensembleVoting,omitempty"`
	LlmEnabled               bool      `json:"llmEnabled"`
	LlmBaseURL               string    `json:"llmBaseUrl"`
	LlmAPIKey                string    `json:"llmApiKey,omitempty"`
	LlmModel                 string    `json:"llmModel"`
	LlmTimeoutSeconds        int       `json:"llmTimeoutSeconds"`
	LlmTemperature           float64   `json:"llmTemperature"`
	LlmMaxTokens             int       `json:"llmMaxTokens"`
	LlmSystemPrompt          string    `json:"llmSystemPrompt"`
}

// DefaultMLConfig returns sensible defaults.
func DefaultMLConfig() MLConfig {
	return MLConfig{
		Enabled:                  false,
		ModelType:                ModelRandomForest,
		AutoTrain:                true,
		TrainInterval:            "10m",
		MinSamplesForTraining:    50,
		BlockConfidenceThreshold: 0.85,
		AllowRiskScoreThreshold:  0.30,
		MlMinConfidence:          0.60,
		LowAnomalyThreshold:      0.3,
		HighAnomalyThreshold:     0.7,
		RuleOverridePriority:     10,
		ActiveLearningEnabled:    true,
		FeatureHistorySize:       1000,
		NumTrees:                 31,
		MaxDepth:                 8,
		MinSamplesLeaf:           4,
		ValidationSplitRatio:     0.2,
		BalanceClasses:           true,
		EnsembleVoting:           "soft",
		LlmEnabled:               false,
		LlmTimeoutSeconds:        30,
		LlmTemperature:           0.1,
		LlmMaxTokens:             512,
	}
}

// ValidateMLActionRiskThresholds ensures the action score intervals are ordered
// and keep the neutral ALERT score (0.5) between ALLOW and BLOCK.
func ValidateMLActionRiskThresholds(allowRiskScore, blockRiskScore float64) error {
	if math.IsNaN(allowRiskScore) || math.IsInf(allowRiskScore, 0) ||
		math.IsNaN(blockRiskScore) || math.IsInf(blockRiskScore, 0) ||
		allowRiskScore < 0 || allowRiskScore >= 0.5 ||
		blockRiskScore < 0.5 || blockRiskScore > 1 {
		return errors.New("ML action risk thresholds must satisfy 0 <= allow < 0.5 <= block <= 1")
	}
	return nil
}
