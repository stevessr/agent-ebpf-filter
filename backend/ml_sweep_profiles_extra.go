package main

import (
	"fmt"
	"strconv"
	"strings"
)

func categoricalAxisProfile(modelType ModelType, paramName, xName string, values []int, apply func(*MLConfig, int), label func(int) string) sweepProfile {
	return axisProfile(modelType, paramName, "categorical", xName, values, len(values), apply, label)
}

func fixedAxisProfile(modelType ModelType, paramName, summary string) sweepProfile {
	return sweepProfile{
		Name:                   string(modelType) + "/" + paramName,
		ModelType:              modelType,
		Comparable:             profileComparable(string(modelType)),
		Kind:                   "bar",
		XName:                  paramName,
		XValues:                []int{0},
		XLabel:                 func(int) string { return "default" },
		ParameterName:          paramName,
		ParameterKind:          "fixed",
		RequiredDiscretePoints: 1,
		Build: func(_, _ int) MLConfig {
			cfg := defaultSweepConfigForModel(modelType)
			return cfg
		},
		Summary: func(MLConfig) string { return summary },
	}
}

func axisProfile(modelType ModelType, paramName, paramKind, xName string, values []int, required int, apply func(*MLConfig, int), label func(int) string) sweepProfile {
	if label == nil {
		label = strconv.Itoa
	}
	return sweepProfile{
		Name:                   string(modelType) + "/" + paramName,
		ModelType:              modelType,
		Comparable:             profileComparable(string(modelType)),
		Kind:                   "bar",
		XName:                  xName,
		XValues:                values,
		XLabel:                 label,
		ParameterName:          paramName,
		ParameterKind:          paramKind,
		RequiredDiscretePoints: required,
		Build: func(x, _ int) MLConfig {
			cfg := defaultSweepConfigForModel(modelType)
			if apply != nil {
				apply(&cfg, x)
			}
			return cfg
		},
		Summary: summarizeSweepConfig,
	}
}

func defaultSweepConfigForModel(modelType ModelType) MLConfig {
	cfg := DefaultMLConfig()
	cfg.ModelType = modelType
	cfg.ValidationSplitRatio = 0.2
	cfg.LlmEnabled = false
	cfg.LlmBaseURL = ""
	cfg.LlmModel = ""
	cfg.LlmAPIKey = ""
	for _, profile := range builtinModelProfiles {
		if profile.Type != modelType {
			continue
		}
		if v := profile.Defaults["numTrees"]; v > 0 {
			cfg.NumTrees = v
		}
		if v := profile.Defaults["maxDepth"]; v > 0 {
			cfg.MaxDepth = v
		}
		if v := profile.Defaults["minSamplesLeaf"]; v > 0 {
			cfg.MinSamplesLeaf = v
		}
		if profile.Apply != nil {
			cfg = profile.Apply(cfg)
		}
		break
	}
	return cfg
}

func summarizeSweepConfig(cfg MLConfig) string {
	switch baseModelType(cfg.ModelType) {
	case ModelRandomForest, ModelExtraTrees:
		return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
	case ModelLogisticRegression:
		reg := "l2"
		switch cfg.MaxDepth {
		case 4:
			reg = "none"
		case 12:
			reg = "l1"
		}
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("lr=%.3f reg=%s%s iter=%d", float64(cfg.NumTrees)/1000.0, reg, balanced, cfg.MinSamplesLeaf)
	case ModelSVM, ModelPerceptron:
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("lr=%.3f%s iter=%d", float64(cfg.NumTrees)/1000.0, balanced, cfg.MinSamplesLeaf)
	case ModelPassiveAggressive:
		balanced := ""
		if cfg.BalanceClasses {
			balanced = " balanced"
		}
		return fmt.Sprintf("C=%.2f%s iter=%d", float64(cfg.NumTrees)/10.0, balanced, cfg.MinSamplesLeaf)
	case ModelKNN:
		distance := "euclidean"
		if cfg.MaxDepth >= 16 {
			distance = "cosine"
		} else if cfg.MaxDepth >= 12 {
			distance = "manhattan"
		}
		weight := "uniform"
		if cfg.MinSamplesLeaf >= 8 {
			weight = "distance"
		}
		return fmt.Sprintf("k=%d distance=%s weight=%s", cfg.NumTrees, distance, weight)
	case ModelRidge:
		return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
	case ModelAdaBoost:
		return fmt.Sprintf("estimators=%d", cfg.NumTrees)
	case ModelNearestCentroid:
		metric := "euclidean"
		switch cfg.MaxDepth {
		case 4:
			metric = "cosine"
		case 12:
			metric = "manhattan"
		}
		prior := "empirical"
		if cfg.BalanceClasses {
			prior = "uniform"
		}
		return fmt.Sprintf("metric=%s prior=%s", metric, prior)
	case ModelNaiveBayes:
		if cfg.BalanceClasses {
			return "balanced-prior"
		}
		return "empirical-prior"
	case ModelEnsemble:
		return "soft-vote ensemble"
	default:
		return string(cfg.ModelType)
	}
}

func intRange(minVal, count int) []int {
	if count <= 0 {
		return nil
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		out[i] = minVal + i
	}
	return out
}

func linspaceIntGlobal(minVal, maxVal, count int) []int {
	if count <= 0 {
		return nil
	}
	if count == 1 {
		return []int{(minVal + maxVal) / 2}
	}
	out := make([]int, count)
	seen := make(map[int]bool, count)
	for i := 0; i < count; i++ {
		v := minVal + (maxVal-minVal)*i/(count-1)
		for seen[v] {
			v++
		}
		out[i] = v
		seen[v] = true
	}
	return out
}

func parseModelFilter(raw string) map[ModelType]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := make(map[ModelType]bool)
	for _, part := range strings.Split(raw, ",") {
		mt := ModelType(strings.TrimSpace(part))
		if mt != "" {
			out[mt] = true
		}
	}
	return out
}
