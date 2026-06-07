package core

// ── ML model types ───────────────────────────────────────────────────────────

// ModelType identifies the ML model algorithm or a local built-in profile.
type ModelType string

const (
	ModelRandomForest          ModelType = "random_forest"
	ModelKNN                   ModelType = "knn"
	ModelLogisticRegression    ModelType = "logistic"
	ModelNaiveBayes            ModelType = "naive_bayes"
	ModelNearestCentroid       ModelType = "nearest_centroid"
	ModelExtraTrees            ModelType = "extra_trees"
	ModelAdaBoost              ModelType = "adaboost"
	ModelSVM                   ModelType = "svm"
	ModelRidge                 ModelType = "ridge"
	ModelPerceptron            ModelType = "perceptron"
	ModelPassiveAggressive     ModelType = "passive_aggressive"
	ModelEnsemble              ModelType = "ensemble"
	ModelAdditiveAttention     ModelType = "additive_attention"
	ModelRandomForestAttention ModelType = "random_forest_attention"
	ModelLogisticAttention     ModelType = "logistic_attention"
	ModelKNNAttention          ModelType = "knn_attention"

	// N-gram style sequence models
	ModelNGramRandomForest ModelType = "ngram_random_forest"
	ModelNGramLogistic     ModelType = "ngram_logistic"
	ModelNGramKNN          ModelType = "ngram_knn"

	// Aliases for attention-enhanced models
	ModelRandomForestAttn = ModelRandomForestAttention
	ModelLogisticAttn     = ModelLogisticAttention
	ModelKNNAttn          = ModelKNNAttention

	// Local built-in profiles
	ModelRandomForestFast    ModelType = "random_forest_fast"
	ModelRandomForestShallow ModelType = "random_forest_shallow"
	ModelRandomForestStable  ModelType = "random_forest_stable"
	ModelRandomForestDeep    ModelType = "random_forest_deep"
	ModelRandomForestWide    ModelType = "random_forest_wide"

	ModelExtraTreesFast ModelType = "extra_trees_fast"
	ModelExtraTreesDeep ModelType = "extra_trees_deep"
	ModelExtraTreesWide ModelType = "extra_trees_wide"

	ModelLogisticFast       ModelType = "logistic_fast"
	ModelLogisticNone       ModelType = "logistic_none"
	ModelLogisticL1         ModelType = "logistic_l1"
	ModelLogisticBalanced   ModelType = "logistic_balanced"
	ModelLogisticL1Balanced ModelType = "logistic_l1_balanced"

	ModelSVMLong     ModelType = "svm_long"
	ModelSVMBalanced ModelType = "svm_balanced"

	ModelPerceptronLong     ModelType = "perceptron_long"
	ModelPerceptronBalanced ModelType = "perceptron_balanced"

	ModelPassiveAggressiveLong     ModelType = "passive_aggressive_long"
	ModelPassiveAggressiveBalanced ModelType = "passive_aggressive_balanced"

	ModelKNNManhattan ModelType = "knn_manhattan"
	ModelKNNCosine    ModelType = "knn_cosine"
	ModelKNNDistance  ModelType = "knn_distance"

	ModelNearestCentroidBalanced  ModelType = "nearest_centroid_balanced"
	ModelNearestCentroidCosine    ModelType = "nearest_centroid_cosine"
	ModelNearestCentroidManhattan ModelType = "nearest_centroid_manhattan"

	ModelNaiveBayesBalanced ModelType = "naive_bayes_balanced"

	ModelRidgeLong     ModelType = "ridge_long"
	ModelRidgeBalanced ModelType = "ridge_balanced"
	ModelRidgeLight    ModelType = "ridge_light"
	ModelRidgeStrong   ModelType = "ridge_strong"

	ModelAdaBoostLong     ModelType = "adaboost_long"
	ModelAdaBoostBalanced ModelType = "adaboost_balanced"
	ModelAdaBoostFast     ModelType = "adaboost_fast"
	ModelAdaBoostLarge    ModelType = "adaboost_large"

	ModelEnsembleSoft    ModelType = "ensemble_soft"
	ModelEnsembleHard    ModelType = "ensemble_hard"
	ModelEnsembleStacked ModelType = "ensemble_stacked"
)

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
