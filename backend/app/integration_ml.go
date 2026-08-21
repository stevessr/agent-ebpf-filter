package app

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
)

// ---- moved from backend/zz_merged_backend.go section integration_ml.go ----

// DefaultMLConfig re-exports the core default configuration.
// The canonical MLConfig type lives in the core package.
var DefaultMLConfig = core.DefaultMLConfig

const (
	mlAutoTrainFallbackInterval = time.Hour
	mlFlushInterval             = time.Minute
)

func currentMLConfig() MLConfig {
	return runtimeSettingsStore.Snapshot().MLConfig
}

// InitMLEngine initializes the ML engine. Only active on master nodes.
func InitMLEngine(cfg MLConfig) {
	if !cfg.Enabled {
		ml.ReplaceMLRuntime(ml.MLRuntimeSnapshot{Config: cfg, ModelType: cfg.ModelType})
		log.Printf("[ML] Behavior classifier disabled by configuration")
		return
	}

	if !clusterManagerStore.IsMaster() {
		ml.ReplaceMLRuntime(ml.MLRuntimeSnapshot{Config: cfg, ModelType: cfg.ModelType})
		log.Printf("[ML] Slave node detected — ML inference disabled (runs only on master)")
		return
	}

	// Initialize training store
	ml.InitTrainingStore(100000)

	if cfg.ModelType == "" {
		cfg.ModelType = ModelRandomForest
	}
	if _, ok := ml.ModelRegistry[cfg.ModelType]; !ok {
		log.Printf("[ML] Unknown model type %q; falling back to %s", cfg.ModelType, ModelRandomForest)
		cfg.ModelType = ModelRandomForest
	}
	// Try loading existing model
	modelPath := cfg.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}

	model := tryLoadModel(modelPath, cfg.ModelType)
	if model != nil {
		log.Printf("[ML] Loaded pre-trained %s model from %s", ml.ModelName(cfg.ModelType), modelPath)
	} else {
		log.Printf("[ML] No pre-trained %s model found at %s — will train once sufficient data is collected", ml.ModelName(cfg.ModelType), modelPath)
	}

	ml.ReplaceMLRuntime(ml.MLRuntimeSnapshot{
		Engine:      model,
		Config:      cfg,
		Enabled:     true,
		ModelLoaded: model != nil,
		ModelType:   cfg.ModelType,
	})
	log.Printf("[ML] Behavior classifier initialized on master node (type=%s, features=%d dims)", cfg.ModelType, FeatureDim)
}

func tryLoadModel(path string, t ModelType) ml.Model {
	requested := t
	base := ml.BaseModelType(t)
	var loaded ml.Model
	switch base {
	case ModelRandomForest:
		if m, err := ml.DeserializeForest(path); err == nil {
			loaded = m
		}
	case ModelExtraTrees:
		if m, err := ml.DeserializeForest(path); err == nil {
			loaded = &ml.ExtraTreesModel{Forest: m, MaxDepth: m.MaxDepth, NumTrees: len(m.Trees)}
		}
	case ModelKNN:
		if m, err := ml.DeserializeKNN(path); err == nil {
			loaded = m
		}
	case ModelLogisticRegression:
		if m, err := ml.DeserializeLogistic(path); err == nil {
			loaded = m
		}
	case ModelNaiveBayes:
		if m, err := ml.DeserializeNaiveBayes(path); err == nil {
			loaded = m
		}
	case ModelNearestCentroid:
		if m, err := ml.DeserializeNearestCentroid(path); err == nil {
			loaded = m
		}
	case ModelAdaBoost:
		if m, err := ml.DeserializeAdaBoost(path); err == nil {
			loaded = m
		}
	case ModelSVM:
		if m, err := ml.DeserializeSVM(path); err == nil {
			loaded = m
		}
	case ModelRidge:
		if m, err := ml.DeserializeRidge(path); err == nil {
			loaded = m
		}
	case ModelPerceptron:
		if m, err := ml.DeserializePerceptron(path); err == nil {
			loaded = m
		}
	case ModelPassiveAggressive:
		if m, err := ml.DeserializePA(path); err == nil {
			loaded = m
		}
	case ModelEnsemble:
		if m, err := ml.DeserializeEnsemble(path); err == nil {
			loaded = m
		}
	case core.ModelGraphLearning:
		if m, err := ml.DeserializeGraphLearning(path); err == nil {
			loaded = m
		}
	case ModelGANTransformer:
		if m, err := ml.DeserializeGANTransformer(path); err == nil {
			loaded = m
		}
	}
	return ml.WrapModelType(loaded, requested)
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

	// Keep the scheduler alive even when AutoTrain is toggled off temporarily.
	// It re-reads runtime settings after every interval and becomes active again
	// without requiring a process restart.
	workers.Add(1)
	go func() {
		defer workers.Done()
		mlAutoTrainLoop(ctx)
	}()

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
	mlPrediction ml.Prediction,
	cfg MLConfig,
	mlEnabled bool,
	mlModelLoaded bool,
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
	mlAutoTrainLoopWithWait(ctx, waitForMLInterval)
}

type mlIntervalWaitFunc func(context.Context, time.Duration) bool

func mlAutoTrainLoopWithWait(ctx context.Context, wait mlIntervalWaitFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if wait == nil {
		wait = waitForMLInterval
	}
	for {
		cfg := currentMLConfig()
		if !wait(ctx, mlAutoTrainInterval(cfg)) {
			return
		}
		cfg = currentMLConfig()
		if !cfg.Enabled || !cfg.AutoTrain || !clusterManagerStore.IsMaster() || ml.GlobalTrainingStore == nil {
			continue
		}
		_, labeled := ml.GlobalTrainingStore.Status()
		if labeled >= cfg.MinSamplesForTraining {
			log.Printf("[ML] Auto-training triggered: %d labeled samples available", labeled)
			model, result := ml.GlobalTrainer.TrainWithConfig(ml.GlobalTrainingStore, cfg)
			if result.Error != "" {
				log.Printf("[ML] Auto-training failed: %s", result.Error)
				continue
			}
			ml.PublishMLRuntimeModel(model, model.Type())
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

func mlAutoTrainInterval(cfg MLConfig) time.Duration {
	if interval, err := time.ParseDuration(strings.TrimSpace(cfg.TrainInterval)); err == nil && interval > 0 {
		return interval
	}
	return mlAutoTrainFallbackInterval
}

func waitForMLInterval(ctx context.Context, interval time.Duration) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = mlAutoTrainFallbackInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// mlFlushLoop periodically flushes training data to disk
func mlFlushLoop(ctx context.Context) {
	mlFlushLoopWithInterval(ctx, mlFlushInterval)
}

func mlFlushLoopWithInterval(ctx context.Context, interval time.Duration) {
	mlFlushLoopWithWait(ctx, interval, waitForMLInterval, flushMLTrainingStore)
}

func mlFlushLoopWithWait(ctx context.Context, interval time.Duration, wait mlIntervalWaitFunc, flush func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = mlFlushInterval
	}
	if wait == nil {
		wait = waitForMLInterval
	}
	if flush == nil {
		flush = flushMLTrainingStore
	}

	for {
		if !wait(ctx, interval) {
			flush()
			return
		}
		// A disabled runtime can be enabled again. Keep the flush worker alive so
		// it resumes automatically and any pre-disable data remains durable.
		flush()
	}
}

func flushMLTrainingStore() {
	if ml.GlobalTrainingStore == nil {
		return
	}
	if err := ml.GlobalTrainingStore.Flush(); err != nil {
		log.Printf("[ML] Failed to flush training data: %v", err)
	}
}

func defaultMLModelPath() string {
	return platform.RuntimeSettingsDir() + "/ml_model.bin"
}

// mlStatus builds the ML status protobuf for the API
func mlStatus() *pb.MLStatus {
	return mlStatusFromRuntime(ml.SnapshotMLRuntime())
}

func mlStatusFromRuntime(runtime ml.MLRuntimeSnapshot) *pb.MLStatus {
	cfg := runtime.Config
	trainerState := ml.GlobalTrainer.StateSnapshot()
	status := &pb.MLStatus{
		ModelLoaded:        runtime.ModelLoaded,
		TrainingInProgress: trainerState.IsRunning,
		TrainingProgress:   trainerState.Progress,
	}

	if runtime.Engine != nil {
		switch model := ml.UnwrapModelType(runtime.Engine).(type) {
		case *ml.DecisionForest:
			status.NumTrees = int32(len(model.Trees))
		case *ml.ExtraTreesModel:
			if model.Forest != nil {
				status.NumTrees = int32(len(model.Forest.Trees))
			} else {
				status.NumTrees = int32(model.NumTrees)
			}
		case *ml.AdaBoostModel:
			status.NumTrees = int32(len(model.Stumps))
		case *ml.EnsembleModel:
			status.NumTrees = int32(len(model.Models))
		case *ml.GraphLearningModel:
			if model.Classifier != nil {
				status.NumTrees = int32(model.Classifier.Config.HiddenDim)
			}
		}
	}

	if ml.GlobalTrainingStore != nil {
		total, labeled := ml.GlobalTrainingStore.Status()
		status.NumSamples = int32(total)
		status.NumLabeledSamples = int32(labeled)
	}

	if !trainerState.LastTrain.IsZero() {
		status.LastTrained = trainerState.LastTrain.Format(time.RFC3339)
		status.TestAccuracy = trainerState.Accuracy
	}

	if cfg.ModelPath != "" {
		status.ModelPath = cfg.ModelPath
	} else {
		status.ModelPath = defaultMLModelPath()
	}

	return status
}
