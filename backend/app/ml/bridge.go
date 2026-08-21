// Package ml — behavior-ML algorithms: attention mechanisms, model zoo,
// trainers, datasets and the online runtime state holder.
//
// Bridge file: type aliases and constant re-exports from core so moved
// algorithm files keep their original identifiers.

package ml

import "agent-ebpf-filter/core"

type ModelType = core.ModelType
type MLConfig = core.MLConfig

var DefaultMLConfig = core.DefaultMLConfig

// FeatureDim is re-exported from core for array-size references.
const FeatureDim = core.FeatureDim

// ── Bridge: ModelType re-exports ─────────────────────────────────────────────
const (
	ModelRandomForest                 = core.ModelRandomForest
	ModelKNN                          = core.ModelKNN
	ModelLogisticRegression           = core.ModelLogisticRegression
	ModelNaiveBayes                   = core.ModelNaiveBayes
	ModelNearestCentroid              = core.ModelNearestCentroid
	ModelExtraTrees                   = core.ModelExtraTrees
	ModelAdaBoost                     = core.ModelAdaBoost
	ModelSVM                          = core.ModelSVM
	ModelRidge                        = core.ModelRidge
	ModelPerceptron                   = core.ModelPerceptron
	ModelPassiveAggressive            = core.ModelPassiveAggressive
	ModelEnsemble                     = core.ModelEnsemble
	ModelAdditiveAttention            = core.ModelAdditiveAttention
	ModelGANTransformer               = core.ModelGANTransformer
	ModelScaledDotProductAttention    = core.ModelScaledDotProductAttention
	ModelMultiHeadAttention           = core.ModelMultiHeadAttention
	ModelRWKVAttention                = core.ModelRWKVAttention
	ModelMambaAttention               = core.ModelMambaAttention
	ModelRandomForestScaledDotProduct = core.ModelRandomForestScaledDotProduct
	ModelLogisticScaledDotProduct     = core.ModelLogisticScaledDotProduct
	ModelKNNScaledDotProduct          = core.ModelKNNScaledDotProduct
	ModelRandomForestMultiHead        = core.ModelRandomForestMultiHead
	ModelLogisticMultiHead            = core.ModelLogisticMultiHead
	ModelKNNMultiHead                 = core.ModelKNNMultiHead
	ModelRandomForestRWKV             = core.ModelRandomForestRWKV
	ModelLogisticRWKV                 = core.ModelLogisticRWKV
	ModelKNNRWKV                      = core.ModelKNNRWKV
	ModelRandomForestMamba            = core.ModelRandomForestMamba
	ModelLogisticMamba                = core.ModelLogisticMamba
	ModelKNNMamba                     = core.ModelKNNMamba
	ModelNGramRandomForest            = core.ModelNGramRandomForest
	ModelNGramLogistic                = core.ModelNGramLogistic
	ModelNGramKNN                     = core.ModelNGramKNN
	ModelRandomForestAttn             = core.ModelRandomForestAttn
	ModelLogisticAttn                 = core.ModelLogisticAttn
	ModelKNNAttn                      = core.ModelKNNAttn
	ModelRandomForestFast             = core.ModelRandomForestFast
	ModelRandomForestShallow          = core.ModelRandomForestShallow
	ModelRandomForestStable           = core.ModelRandomForestStable
	ModelRandomForestDeep             = core.ModelRandomForestDeep
	ModelRandomForestWide             = core.ModelRandomForestWide
	ModelExtraTreesFast               = core.ModelExtraTreesFast
	ModelExtraTreesDeep               = core.ModelExtraTreesDeep
	ModelExtraTreesWide               = core.ModelExtraTreesWide
	ModelLogisticFast                 = core.ModelLogisticFast
	ModelLogisticNone                 = core.ModelLogisticNone
	ModelLogisticL1                   = core.ModelLogisticL1
	ModelLogisticBalanced             = core.ModelLogisticBalanced
	ModelLogisticL1Balanced           = core.ModelLogisticL1Balanced
	ModelSVMLong                      = core.ModelSVMLong
	ModelSVMBalanced                  = core.ModelSVMBalanced
	ModelPerceptronLong               = core.ModelPerceptronLong
	ModelPerceptronBalanced           = core.ModelPerceptronBalanced
	ModelPassiveAggressiveLong        = core.ModelPassiveAggressiveLong
	ModelPassiveAggressiveBalanced    = core.ModelPassiveAggressiveBalanced
	ModelKNNManhattan                 = core.ModelKNNManhattan
	ModelKNNCosine                    = core.ModelKNNCosine
	ModelKNNDistance                  = core.ModelKNNDistance
	ModelNearestCentroidBalanced      = core.ModelNearestCentroidBalanced
	ModelNearestCentroidCosine        = core.ModelNearestCentroidCosine
	ModelNearestCentroidManhattan     = core.ModelNearestCentroidManhattan
	ModelNaiveBayesBalanced           = core.ModelNaiveBayesBalanced
	ModelRidgeLong                    = core.ModelRidgeLong
	ModelRidgeBalanced                = core.ModelRidgeBalanced
	ModelRidgeLight                   = core.ModelRidgeLight
	ModelRidgeStrong                  = core.ModelRidgeStrong
	ModelAdaBoostLong                 = core.ModelAdaBoostLong
	ModelAdaBoostBalanced             = core.ModelAdaBoostBalanced
	ModelAdaBoostFast                 = core.ModelAdaBoostFast
	ModelAdaBoostLarge                = core.ModelAdaBoostLarge
	ModelEnsembleSoft                 = core.ModelEnsembleSoft
	ModelEnsembleHard                 = core.ModelEnsembleHard
	ModelEnsembleStacked              = core.ModelEnsembleStacked
)
