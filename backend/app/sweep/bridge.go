// Package sweep — ML hyperparameter sweep harness: profile grids,
// incremental-count evaluation, comprehensive verifiers and HTML report
// rendering.
//
// Bridge file: type aliases re-exported from core/app/ml so moved files
// keep their original identifiers.

package sweep

import "agent-ebpf-filter/app/ml"

type ModelType = ml.ModelType
type MLConfig = ml.MLConfig

var DefaultMLConfig = ml.DefaultMLConfig

// ── Bridge: ModelType constant re-exports ─────────────────────────────────────
const (
	ModelAdaBoost           = ml.ModelAdaBoost
	ModelEnsemble           = ml.ModelEnsemble
	ModelExtraTrees         = ml.ModelExtraTrees
	ModelKNN                = ml.ModelKNN
	ModelLogisticRegression = ml.ModelLogisticRegression
	ModelNaiveBayes         = ml.ModelNaiveBayes
	ModelNearestCentroid    = ml.ModelNearestCentroid
	ModelPassiveAggressive  = ml.ModelPassiveAggressive
	ModelPerceptron         = ml.ModelPerceptron
	ModelRandomForest       = ml.ModelRandomForest
	ModelRidge              = ml.ModelRidge
	ModelSVM                = ml.ModelSVM
)

// FeatureDim is re-exported for array-size references.
const FeatureDim = ml.FeatureDim

var selectBenchmarkSamples = ml.SelectBenchmarkSamples
