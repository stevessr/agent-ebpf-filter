package app

import (
	"agent-ebpf-filter/app/ml"
	"strconv"
)

// ---- moved from backend/zz_merged_backend.go section sweepprofilescomprehensive.go ----

func comprehensiveAxisSweepProfiles(pointsPerParam int) []sweepProfile {
	if pointsPerParam < 1 {
		pointsPerParam = 1000
	}
	profiles := make([]sweepProfile, 0, len(ml.AllModelTypes())*3)
	for _, mt := range ml.AllModelTypes() {
		base := ml.BaseModelType(mt)
		switch base {
		case ModelRandomForest, ModelExtraTrees:
			profiles = append(profiles,
				numericAxisProfile(mt, "numTrees", "numTrees", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "maxDepth", "maxDepth", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }),
				numericAxisProfile(mt, "minSamplesLeaf", "minSamplesLeaf", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelLogisticRegression:
			profiles = append(profiles,
				numericAxisProfile(mt, "learningRate", "learningRate×1000", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "maxIter", "maxIter", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
				categoricalAxisProfile(mt, "regularization", "regularization", []int{4, 8, 12}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch v {
					case 4:
						return "none"
					case 12:
						return "l1"
					default:
						return "l2"
					}
				}),
			)
		case ModelSVM, ModelPerceptron:
			profiles = append(profiles,
				numericAxisProfile(mt, "learningRate", "learningRate×1000", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "iterations", "iterations", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelPassiveAggressive:
			profiles = append(profiles,
				numericAxisProfile(mt, "aggressivenessC", "C×10", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				numericAxisProfile(mt, "iterations", "iterations", intRange(100, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }),
			)
		case ModelKNN:
			profiles = append(profiles,
				numericAxisProfile(mt, "k", "k", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
				categoricalAxisProfile(mt, "distance", "distance selector", []int{8, 12, 16}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch {
					case v >= 16:
						return "cosine"
					case v >= 12:
						return "manhattan"
					default:
						return "euclidean"
					}
				}),
				categoricalAxisProfile(mt, "weight", "weight selector", []int{5, 8}, func(cfg *MLConfig, v int) { cfg.MinSamplesLeaf = v }, func(v int) string {
					if v >= 8 {
						return "distance"
					}
					return "uniform"
				}),
			)
		case ModelRidge:
			profiles = append(profiles,
				numericAxisProfile(mt, "alpha", "alpha×100", intRange(1, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
			)
		case ModelAdaBoost:
			profiles = append(profiles,
				numericAxisProfile(mt, "estimators", "estimators", intRange(10, pointsPerParam), pointsPerParam, func(cfg *MLConfig, v int) { cfg.NumTrees = v }),
			)
		case ModelNearestCentroid:
			profiles = append(profiles,
				categoricalAxisProfile(mt, "metric", "metric selector", []int{4, 8, 12}, func(cfg *MLConfig, v int) { cfg.MaxDepth = v }, func(v int) string {
					switch v {
					case 4:
						return "cosine"
					case 12:
						return "manhattan"
					default:
						return "euclidean"
					}
				}),
				categoricalAxisProfile(mt, "classPrior", "class prior", []int{0, 1}, func(cfg *MLConfig, v int) { cfg.BalanceClasses = v == 1 }, func(v int) string {
					if v == 1 {
						return "uniform"
					}
					return "empirical"
				}),
			)
		case ModelNaiveBayes:
			profiles = append(profiles,
				categoricalAxisProfile(mt, "classPrior", "class prior", []int{0, 1}, func(cfg *MLConfig, v int) { cfg.BalanceClasses = v == 1 }, func(v int) string {
					if v == 1 {
						return "uniform"
					}
					return "empirical"
				}),
			)
		case ModelEnsemble:
			profiles = append(profiles, fixedAxisProfile(mt, "voting", "soft-vote ensemble"))
		}
	}
	return profiles
}

func numericAxisProfile(modelType ModelType, paramName, xName string, values []int, required int, apply func(*MLConfig, int)) sweepProfile {
	return axisProfile(modelType, paramName, "numeric", xName, values, required, apply, strconv.Itoa)
}
