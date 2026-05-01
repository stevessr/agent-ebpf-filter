package main

import (
	"log"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

// MLConfig holds configuration for the ML behavior classifier
type MLConfig struct {
	Enabled                  bool    `json:"enabled"`
	ModelPath                string  `json:"modelPath"`
	AutoTrain                bool    `json:"autoTrain"`
	TrainInterval            string  `json:"trainInterval"`
	MinSamplesForTraining    int     `json:"minSamplesForTraining"`
	BlockConfidenceThreshold float64 `json:"blockConfidenceThreshold"`
	MlMinConfidence          float64 `json:"mlMinConfidence"`
	LowAnomalyThreshold      float64 `json:"lowAnomalyThreshold"`
	HighAnomalyThreshold     float64 `json:"highAnomalyThreshold"`
	RuleOverridePriority     int     `json:"ruleOverridePriority"`
	ActiveLearningEnabled    bool    `json:"activeLearningEnabled"`
	FeatureHistorySize       int     `json:"featureHistorySize"`
	NumTrees                 int     `json:"numTrees"`
	MaxDepth                 int     `json:"maxDepth"`
	MinSamplesLeaf           int     `json:"minSamplesLeaf"`
}

// DefaultMLConfig returns sensible defaults
func DefaultMLConfig() MLConfig {
	return MLConfig{
		Enabled:                  true,
		ModelPath:                "",
		AutoTrain:                true,
		TrainInterval:            "24h",
		MinSamplesForTraining:    1000,
		BlockConfidenceThreshold: 0.85,
		MlMinConfidence:          0.60,
		LowAnomalyThreshold:      0.30,
		HighAnomalyThreshold:     0.70,
		RuleOverridePriority:     100,
		ActiveLearningEnabled:    false,
		FeatureHistorySize:       100,
		NumTrees:                 31,
		MaxDepth:                 8,
		MinSamplesLeaf:           5,
	}
}

// Global ML state
var (
	mlEngine  *DecisionForest
	mlConfig  MLConfig
	mlEnabled bool
	mlModelLoaded bool
)

// InitMLEngine initializes the ML engine. Only active on master nodes.
func InitMLEngine(cfg MLConfig) {
	mlConfig = cfg
	if !cfg.Enabled {
		log.Printf("[ML] Behavior classifier disabled by configuration")
		return
	}

	if !clusterManagerStore.IsMaster() {
		log.Printf("[ML] Slave node detected — ML inference disabled (runs only on master)")
		return
	}

	// Initialize training store
	InitTrainingStore(100000)

	// Try loading existing model
	if cfg.ModelPath != "" {
		forest, err := DeserializeForest(cfg.ModelPath)
		if err != nil {
			log.Printf("[ML] No pre-trained model found at %s (%v) — will train once sufficient data is collected", cfg.ModelPath, err)
		} else {
			mlEngine = forest
			mlModelLoaded = true
			log.Printf("[ML] Loaded pre-trained model: %d trees, %d features", len(forest.Trees), forest.NumFeatures)
		}
	} else {
		// Try default path
		defaultPath := defaultMLModelPath()
		if forest, err := DeserializeForest(defaultPath); err == nil {
			mlEngine = forest
			mlModelLoaded = true
			log.Printf("[ML] Loaded default pre-trained model from %s", defaultPath)
		}
	}

	log.Printf("[ML] Behavior classifier initialized on master node (features=%d dims)", FeatureDim)
	mlEnabled = true
}

// StartMLEngine starts background tasks for the ML engine
func StartMLEngine() {
	if !mlEnabled {
		return
	}

	// Auto-training scheduler
	if mlConfig.AutoTrain {
		go mlAutoTrainLoop()
	}

	// Periodic data flush
	go mlFlushLoop()
}

// resolveAction fuses rule-based classification, anomaly scoring, and ML prediction
// into a final WrapperResponse action. Inspired by the LIGHT-HIDS two-layer architecture.
//
// Layer 1: Fast heuristic triage using existing regex classifier + anomaly score
// Layer 2: ML random forest for uncertain cases
func resolveAction(
	req *pb.WrapperRequest,
	ruleAction string,
	rulePriority int,
	classification *pb.BehaviorClassification,
	anomalyScore float64,
	mlPrediction Prediction,
	cfg MLConfig,
) (pb.WrapperResponse_Action, string) {

	// ── Explicit high-priority rules always win ──
	if ruleAction != "" && rulePriority >= cfg.RuleOverridePriority {
		switch ruleAction {
		case "BLOCK":
			return pb.WrapperResponse_BLOCK, "High-priority explicit rule: BLOCK"
		case "ALERT":
			return pb.WrapperResponse_ALERT, "High-priority explicit rule: ALERT"
		case "REWRITE":
			return pb.WrapperResponse_REWRITE, "High-priority explicit rule: REWRITE"
		}
	}

	// ── Layer 1: Heuristic triage ──
	if classification != nil && classification.Confidence == "high" {
		primaryCat := classification.PrimaryCategory
		if primaryCat == "SENSITIVE" || primaryCat == "FILE_DELETE" {
			if anomalyScore > cfg.HighAnomalyThreshold {
				return pb.WrapperResponse_ALERT,
					"High-confidence sensitive/file-delete category with anomalous pattern"
			}
		}
	}

	// ── Layer 1.5: Network audit escalation ──
	cmdline := strings.Join(req.Args, " ")
	netAudit := AuditNetworkBehavior(req.Comm, cmdline)
	if netAudit.RiskLevel == "CRITICAL" {
		return pb.WrapperResponse_ALERT,
			"CRITICAL network audit: " + netAudit.Findings[0].Description
	}
	if netAudit.RiskLevel == "HIGH" && anomalyScore > 0.5 {
		return pb.WrapperResponse_ALERT,
			"HIGH network risk with anomalous pattern"
	}

	// ── Layer 2: ML model ──
	if mlEnabled && mlModelLoaded && mlPrediction.Confidence >= cfg.MlMinConfidence {
		switch mlPrediction.Action {
		case 1: // BLOCK
			if mlPrediction.Confidence >= cfg.BlockConfidenceThreshold {
				return pb.WrapperResponse_BLOCK, "ML classification: BLOCK (high confidence)"
			}
			// Uncertain block → alert instead
			return pb.WrapperResponse_ALERT, "ML classification: suspicious (elevated to ALERT)"
		case 3: // ALERT
			return pb.WrapperResponse_ALERT, "ML classification: ALERT"
		case 2: // REWRITE
			if ruleAction == "REWRITE" {
				return pb.WrapperResponse_REWRITE, "ML classification: REWRITE (rule exists)"
			}
			return pb.WrapperResponse_ALERT, "ML classification: REWRITE (no rewrite rule available, alerting)"
		case 0: // ALLOW
			if anomalyScore < cfg.LowAnomalyThreshold {
				return pb.WrapperResponse_ALLOW, "ML classification: benign behavior"
			}
			return pb.WrapperResponse_ALERT, "ML classification: uncertain benign (anomaly elevated)"
		}
	}

	// ── Layer 2 anomaly-only (model not confident, but anomalous) ──
	if mlEnabled && anomalyScore > cfg.HighAnomalyThreshold {
		// High anomaly with low model confidence — still worth alerting
		return pb.WrapperResponse_ALERT, "Anomalous behavior detected (insufficient labeled data for ML classification)"
	}

	// ── Fallback: existing rule-based behavior ──
	switch ruleAction {
	case "BLOCK":
		return pb.WrapperResponse_BLOCK, "Rule-based policy: BLOCK"
	case "ALERT":
		return pb.WrapperResponse_ALERT, "Rule-based policy: ALERT"
	case "REWRITE":
		return pb.WrapperResponse_REWRITE, "Rule-based policy: REWRITE"
	default:
		return pb.WrapperResponse_ALLOW, ""
	}
}

// mlAutoTrainLoop periodically checks if enough labeled data exists and triggers training
func mlAutoTrainLoop() {
	interval := 1 * time.Hour
	if d, err := time.ParseDuration(mlConfig.TrainInterval); err == nil && d > 0 {
		interval = d
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if !mlEnabled {
			return
		}
		_, labeled := globalTrainingStore.Status()
		if labeled >= mlConfig.MinSamplesForTraining {
			log.Printf("[ML] Auto-training triggered: %d labeled samples available", labeled)
			forest, result := globalTrainer.Train(globalTrainingStore, mlConfig.NumTrees, mlConfig.MaxDepth, mlConfig.MinSamplesLeaf)
			if result.Error != "" {
				log.Printf("[ML] Auto-training failed: %s", result.Error)
				continue
			}
			mlEngine = forest
			mlModelLoaded = true
			log.Printf("[ML] Auto-training complete: accuracy=%.2f%%, trees=%d", result.Accuracy*100, result.NumTrees)

			// Persist model
			modelPath := mlConfig.ModelPath
			if modelPath == "" {
				modelPath = defaultMLModelPath()
			}
			if err := forest.Serialize(modelPath); err != nil {
				log.Printf("[ML] Failed to save model: %v", err)
			}
		}
	}
}

// mlFlushLoop periodically flushes training data to disk
func mlFlushLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !mlEnabled {
			return
		}
		if globalTrainingStore != nil {
			if err := globalTrainingStore.Flush(); err != nil {
				log.Printf("[ML] Failed to flush training data: %v", err)
			}
		}
	}
}

func defaultMLModelPath() string {
	return runtimeSettingsDir() + "/ml_model.bin"
}

// mlStatus builds the ML status protobuf for the API
func mlStatus() *pb.MLStatus {
	status := &pb.MLStatus{
		ModelLoaded:    mlModelLoaded,
		TrainingInProgress: globalTrainer.isRunning,
		TrainingProgress:   globalTrainer.progress,
	}

	if mlEngine != nil {
		status.NumTrees = int32(len(mlEngine.Trees))
	}

	if globalTrainingStore != nil {
		total, labeled := globalTrainingStore.Status()
		status.NumSamples = int32(total)
		status.NumLabeledSamples = int32(labeled)
	}

	if !globalTrainer.lastTrain.IsZero() {
		status.LastTrained = globalTrainer.lastTrain.Format(time.RFC3339)
		status.TestAccuracy = globalTrainer.accuracy
	}

	if mlConfig.ModelPath != "" {
		status.ModelPath = mlConfig.ModelPath
	} else {
		status.ModelPath = defaultMLModelPath()
	}

	return status
}
