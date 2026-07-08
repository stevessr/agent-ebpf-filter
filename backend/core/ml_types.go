package core

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
		BlockConfidenceThreshold: 0.9,
		MlMinConfidence:          0.5,
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
