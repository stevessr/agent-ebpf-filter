package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section integration_ml.go ----

// DefaultMLConfig re-exports the core default configuration.
// The canonical MLConfig type lives in the core package.
var DefaultMLConfig = core.DefaultMLConfig

// Global ML state
var (
	mlEngine         Model
	mlConfig         MLConfig
	mlEnabled        bool
	mlModelLoaded    bool
	currentModelType ModelType
)

func currentMLConfig() MLConfig {
	return runtimeSettingsStore.Snapshot().MLConfig
}

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

	if cfg.ModelType == "" {
		cfg.ModelType = ModelRandomForest
	}
	if _, ok := modelRegistry[cfg.ModelType]; !ok {
		log.Printf("[ML] Unknown model type %q; falling back to %s", cfg.ModelType, ModelRandomForest)
		cfg.ModelType = ModelRandomForest
	}
	mlConfig = cfg
	currentModelType = cfg.ModelType

	// Try loading existing model
	modelPath := cfg.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}

	if m := tryLoadModel(modelPath, cfg.ModelType); m != nil {
		mlEngine = m
		mlModelLoaded = true
		log.Printf("[ML] Loaded pre-trained %s model from %s", modelName(cfg.ModelType), modelPath)
	} else {
		log.Printf("[ML] No pre-trained %s model found at %s — will train once sufficient data is collected", modelName(cfg.ModelType), modelPath)
	}

	log.Printf("[ML] Behavior classifier initialized on master node (type=%s, features=%d dims)", cfg.ModelType, FeatureDim)
	mlEnabled = true
}

func tryLoadModel(path string, t ModelType) Model {
	requested := t
	base := baseModelType(t)
	var loaded Model
	switch base {
	case ModelRandomForest:
		if m, err := DeserializeForest(path); err == nil {
			loaded = m
		}
	case ModelExtraTrees:
		if m, err := DeserializeForest(path); err == nil {
			loaded = &ExtraTreesModel{Forest: m, MaxDepth: m.MaxDepth, NumTrees: len(m.Trees)}
		}
	case ModelKNN:
		if m, err := DeserializeKNN(path); err == nil {
			loaded = m
		}
	case ModelLogisticRegression:
		if m, err := DeserializeLogistic(path); err == nil {
			loaded = m
		}
	case ModelNaiveBayes:
		if m, err := DeserializeNaiveBayes(path); err == nil {
			loaded = m
		}
	case ModelNearestCentroid:
		if m, err := DeserializeNearestCentroid(path); err == nil {
			loaded = m
		}
	case ModelAdaBoost:
		if m, err := DeserializeAdaBoost(path); err == nil {
			loaded = m
		}
	case ModelSVM:
		if m, err := DeserializeSVM(path); err == nil {
			loaded = m
		}
	case ModelRidge:
		if m, err := DeserializeRidge(path); err == nil {
			loaded = m
		}
	case ModelPerceptron:
		if m, err := DeserializePerceptron(path); err == nil {
			loaded = m
		}
	case ModelPassiveAggressive:
		if m, err := DeserializePA(path); err == nil {
			loaded = m
		}
	case ModelEnsemble:
		if m, err := DeserializeEnsemble(path); err == nil {
			loaded = m
		}
	case core.ModelGraphLearning:
		if m, err := DeserializeGraphLearning(path); err == nil {
			loaded = m
		}
	case ModelGANTransformer:
		if m, err := DeserializeGANTransformer(path); err == nil {
			loaded = m
		}
	}
	return wrapModelType(loaded, requested)
}

// StartMLEngine runs the ML background tasks until ctx is cancelled. The call
// intentionally blocks so the owning runtime task can join both loops during
// graceful shutdown.
func StartMLEngine(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg := currentMLConfig()
	if !cfg.Enabled || !clusterManagerStore.IsMaster() {
		return
	}

	var workers sync.WaitGroup

	// Auto-training scheduler
	if cfg.AutoTrain {
		workers.Add(1)
		go func() {
			defer workers.Done()
			mlAutoTrainLoop(ctx)
		}()
	}

	// Periodic data flush
	workers.Add(1)
	go func() {
		defer workers.Done()
		mlFlushLoop(ctx)
	}()
	workers.Wait()
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
func mlAutoTrainLoop(ctx context.Context) {
	interval := 1 * time.Hour
	if d, err := time.ParseDuration(currentMLConfig().TrainInterval); err == nil && d > 0 {
		interval = d
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cfg := currentMLConfig()
		if !cfg.Enabled || !cfg.AutoTrain || !clusterManagerStore.IsMaster() || globalTrainingStore == nil {
			return
		}
		_, labeled := globalTrainingStore.Status()
		if labeled >= cfg.MinSamplesForTraining {
			log.Printf("[ML] Auto-training triggered: %d labeled samples available", labeled)
			model, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
			if result.Error != "" {
				log.Printf("[ML] Auto-training failed: %s", result.Error)
				continue
			}
			mlEngine = model
			mlModelLoaded = true
			log.Printf("[ML] Auto-training complete: accuracy=%.2f%%, type=%s", result.Accuracy*100, model.Type())

			// Persist model
			modelPath := cfg.ModelPath
			if modelPath == "" {
				modelPath = defaultMLModelPath()
			}
			if err := model.Serialize(modelPath); err != nil {
				log.Printf("[ML] Failed to save model: %v", err)
			}
		}
	}
}

// mlFlushLoop periodically flushes training data to disk
func mlFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			flushMLTrainingStore()
			return
		case <-ticker.C:
		}
		if !currentMLConfig().Enabled {
			flushMLTrainingStore()
			return
		}
		flushMLTrainingStore()
	}
}

func flushMLTrainingStore() {
	if globalTrainingStore == nil {
		return
	}
	if err := globalTrainingStore.Flush(); err != nil {
		log.Printf("[ML] Failed to flush training data: %v", err)
	}
}

func defaultMLModelPath() string {
	return platform.RuntimeSettingsDir() + "/ml_model.bin"
}

// mlStatus builds the ML status protobuf for the API
func mlStatus() *pb.MLStatus {
	cfg := currentMLConfig()
	status := &pb.MLStatus{
		ModelLoaded:        mlModelLoaded,
		TrainingInProgress: globalTrainer.isRunning,
		TrainingProgress:   globalTrainer.progress,
	}

	if mlEngine != nil {
		switch model := unwrapModelType(mlEngine).(type) {
		case *DecisionForest:
			status.NumTrees = int32(len(model.Trees))
		case *ExtraTreesModel:
			if model.Forest != nil {
				status.NumTrees = int32(len(model.Forest.Trees))
			} else {
				status.NumTrees = int32(model.NumTrees)
			}
		case *AdaBoostModel:
			status.NumTrees = int32(len(model.Stumps))
		case *EnsembleModel:
			status.NumTrees = int32(len(model.Models))
		case *GraphLearningModel:
			if model.Classifier != nil {
				status.NumTrees = int32(model.Classifier.Config.HiddenDim)
			}
		}
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

	if cfg.ModelPath != "" {
		status.ModelPath = cfg.ModelPath
	} else {
		status.ModelPath = defaultMLModelPath()
	}

	return status
}
