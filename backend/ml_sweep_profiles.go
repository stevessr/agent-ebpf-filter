package main

import (
	"fmt"
	"strconv"
)

func profileForDataset(profile sweepProfile, dataset sweepDataset) sweepProfile {
	scoped := profile
	scoped.Name = dataset.Name + "/" + profile.Name
	scoped.DatasetName = dataset.Name
	scoped.DatasetDescription = dataset.Description
	return scoped
}

func profilesForMode(mode string) []sweepProfile {
	return profilesForModeWithPoints(mode, 1000)
}

func profilesForModeWithPoints(mode string, pointsPerParam int) []sweepProfile {
	if pointsPerParam < 1 {
		pointsPerParam = 1000
	}
	if mode == "comprehensive" {
		return comprehensiveAxisSweepProfiles(pointsPerParam)
	}
	quick := map[string]bool{
		string(ModelRandomForest):       true,
		string(ModelExtraTrees):         true,
		string(ModelKNN):                true,
		string(ModelNaiveBayes):         true,
		string(ModelAdaBoost):           true,
		string(ModelLogisticRegression): true,
		string(ModelSVM):                true,
		string(ModelRidge):              true,
		string(ModelPerceptron):         true,
		string(ModelPassiveAggressive):  true,
	}
	_ = quick

	// ── Parameter grids ──────────────────────────────────────────
	// quick: ~9-50 points per model
	// full: ~50-200 points per model
	// comprehensive: 1000+ points per model

	// Helper: linspaceInt generates count evenly-spaced integers from min to max
	linspaceInt := func(minVal, maxVal, count int) []int {
		if count <= 0 {
			return nil
		}
		if count == 1 {
			return []int{(minVal + maxVal) / 2}
		}
		out := make([]int, count)
		for i := 0; i < count; i++ {
			out[i] = minVal + (maxVal-minVal)*i/(count-1)
		}
		return out
	}

	rfX, rfY := []int{15, 31, 51}, []int{6, 8, 10}
	etX, etY := []int{15, 31, 51}, []int{6, 8, 10}
	logX, logY := []int{5, 10, 20, 50}, []int{4, 8, 12}
	linearX, linearY := []int{5, 10, 20, 50}, []int{500, 1000, 2000}
	knnX := []int{1, 3, 5, 7, 9}
	ridgeX := []int{5, 10, 25, 50, 100}
	adaX := []int{10, 25, 50, 100}
	nbX := []int{0}

	switch mode {
	case "full":
		rfX, rfY = linspaceInt(3, 100, 15), linspaceInt(2, 16, 10)
		etX, etY = linspaceInt(3, 100, 15), linspaceInt(2, 16, 10)
		logX, logY = linspaceInt(1, 200, 20), []int{4, 8, 12}
		linearX, linearY = linspaceInt(1, 200, 20), linspaceInt(100, 5000, 10)
		knnX = linspaceInt(1, 31, 15)
		ridgeX = linspaceInt(1, 500, 20)
		adaX = linspaceInt(5, 300, 20)
	}

	return []sweepProfile{
		{
			Name:       "random_forest",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    rfX,
			YValues:    rfY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "extra_trees",
			ModelType:  ModelExtraTrees,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    etX,
			YValues:    etY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelExtraTrees
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    logX,
			YValues:    logY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				reg := "l2"
				switch cfg.MaxDepth {
				case 4:
					reg = "none"
				case 12:
					reg = "l1"
				}
				return fmt.Sprintf("lr=%.3f reg=%s iter=%d", float64(cfg.NumTrees)/1000.0, reg, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    linearX,
			YValues:    linearY,
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "knn",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    knnX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 8
				cfg.MinSamplesLeaf = 5
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "ridge",
			ModelType:  ModelRidge,
			Comparable: false,
			Kind:       "bar",
			XName:      "alpha×100",
			XValues:    ridgeX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRidge
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.2f", float64(v)/100.0) },
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
			},
		},
		{
			Name:       "adaboost",
			ModelType:  ModelAdaBoost,
			Comparable: false,
			Kind:       "bar",
			XName:      "estimators",
			XValues:    adaX,
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelAdaBoost
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("estimators=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "naive_bayes",
			ModelType:  ModelNaiveBayes,
			Comparable: false,
			Kind:       "bar",
			XName:      "preset",
			XValues:    nbX,
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNaiveBayes
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "default" },
			Summary: func(cfg MLConfig) string {
				return "default"
			},
		},
		{
			Name:       "naive_bayes_balanced",
			ModelType:  ModelNaiveBayes,
			Comparable: false,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNaiveBayes
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "balanced" },
			Summary: func(cfg MLConfig) string {
				return "balanced-prior"
			},
		},
		{
			Name:       "logistic_balanced",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{3, 5, 10, 20, 50, 100},
			YValues:    []int{1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 12
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=l1 balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm_balanced",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron_balanced",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive_balanced",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f balanced iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "nearest_centroid",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "euclidean" },
			Summary: func(cfg MLConfig) string {
				return "metric=euclidean prior=empirical"
			},
		},
		{
			Name:       "nearest_centroid_balanced",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 8
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "euclidean + uniform prior" },
			Summary: func(cfg MLConfig) string {
				return "metric=euclidean prior=uniform"
			},
		},
		{
			Name:       "nearest_centroid_cosine",
			ModelType:  ModelNearestCentroid,
			Comparable: true,
			Kind:       "bar",
			XName:      "preset",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelNearestCentroid
				cfg.MaxDepth = 4
				cfg.BalanceClasses = true
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "cosine + uniform prior" },
			Summary: func(cfg MLConfig) string {
				return "metric=cosine prior=uniform"
			},
		},
		{
			Name:       "knn_cosine",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    []int{1, 3, 5, 7, 9, 11, 15, 21, 31},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 16
				cfg.MinSamplesLeaf = 10
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d distance=cosine weight=distance", cfg.NumTrees)
			},
		},
		{
			Name:       "random_forest_fast",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{5, 9, 13, 17, 21},
			YValues:    []int{3, 4, 5, 6, 7},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 2
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "random_forest_deep",
			ModelType:  ModelRandomForest,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{31, 41, 51, 61, 71},
			YValues:    []int{8, 10, 12, 14},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRandomForest
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "extra_trees_deep",
			ModelType:  ModelExtraTrees,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "numTrees",
			YName:      "maxDepth",
			XValues:    []int{31, 41, 51, 61, 71},
			YValues:    []int{8, 10, 12, 14},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelExtraTrees
				cfg.NumTrees = x
				cfg.MaxDepth = y
				cfg.MinSamplesLeaf = 3
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("trees=%d depth=%d leaf=%d", cfg.NumTrees, cfg.MaxDepth, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic_none",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{1, 3, 5, 10, 20, 50},
			YValues:    []int{500, 1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 4
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=none iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "logistic_l1",
			ModelType:  ModelLogisticRegression,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "maxIter",
			XValues:    []int{3, 5, 10, 20, 50, 100},
			YValues:    []int{1000, 2000, 4000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelLogisticRegression
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 12
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f reg=l1 iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "svm_long",
			ModelType:  ModelSVM,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelSVM
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "perceptron_long",
			ModelType:  ModelPerceptron,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPerceptron
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "passive_aggressive_long",
			ModelType:  ModelPassiveAggressive,
			Comparable: true,
			Kind:       "heatmap",
			XName:      "learningRate×1000",
			YName:      "iterations",
			XValues:    []int{1, 5, 10, 20, 50, 100, 150},
			YValues:    []int{1000, 2000, 4000, 8000},
			Build: func(x, y int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelPassiveAggressive
				cfg.NumTrees = x
				cfg.MinSamplesLeaf = y
				cfg.MaxDepth = 8
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.3f", float64(v)/1000.0) },
			YLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("lr=%.3f iter=%d", float64(cfg.NumTrees)/1000.0, cfg.MinSamplesLeaf)
			},
		},
		{
			Name:       "knn_distance",
			ModelType:  ModelKNN,
			Comparable: false,
			Kind:       "bar",
			XName:      "k",
			XValues:    []int{1, 3, 5, 7, 9, 11, 15, 21, 31},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelKNN
				cfg.NumTrees = x
				cfg.MaxDepth = 12
				cfg.MinSamplesLeaf = 10
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("k=%d distance=manhattan weight=distance", cfg.NumTrees)
			},
		},
		{
			Name:       "ridge_strong",
			ModelType:  ModelRidge,
			Comparable: false,
			Kind:       "bar",
			XName:      "alpha×100",
			XValues:    []int{100, 150, 200, 250, 300, 400, 500},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelRidge
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(v int) string { return fmt.Sprintf("%.2f", float64(v)/100.0) },
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("alpha=%.2f", float64(cfg.NumTrees)/100.0)
			},
		},
		{
			Name:       "adaboost_large",
			ModelType:  ModelAdaBoost,
			Comparable: false,
			Kind:       "bar",
			XName:      "estimators",
			XValues:    []int{50, 100, 150, 200, 250, 300, 400},
			Build: func(x, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelAdaBoost
				cfg.NumTrees = x
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: strconv.Itoa,
			Summary: func(cfg MLConfig) string {
				return fmt.Sprintf("estimators=%d", cfg.NumTrees)
			},
		},
		{
			Name:       "ensemble",
			ModelType:  ModelEnsemble,
			Comparable: true,
			Kind:       "bar",
			XName:      "voting",
			XValues:    []int{0},
			Build: func(_, _ int) MLConfig {
				cfg := DefaultMLConfig()
				cfg.ModelType = ModelEnsemble
				cfg.ValidationSplitRatio = 0.2
				cfg.LlmEnabled = false
				return cfg
			},
			XLabel: func(int) string { return "soft" },
			Summary: func(_ MLConfig) string {
				return "soft-vote ensemble"
			},
		},
	}
}

func comprehensiveAxisSweepProfiles(pointsPerParam int) []sweepProfile {
	if pointsPerParam < 1 {
		pointsPerParam = 1000
	}
	profiles := make([]sweepProfile, 0, len(AllModelTypes())*3)
	for _, mt := range AllModelTypes() {
		base := baseModelType(mt)
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

func numericSweepParametersForModel(modelType ModelType) []string {
	switch baseModelType(modelType) {
	case ModelRandomForest, ModelExtraTrees:
		return []string{"numTrees", "maxDepth", "minSamplesLeaf"}
	case ModelLogisticRegression:
		return []string{"learningRate", "maxIter"}
	case ModelSVM, ModelPerceptron:
		return []string{"learningRate", "iterations"}
	case ModelPassiveAggressive:
		return []string{"aggressivenessC", "iterations"}
	case ModelKNN:
		return []string{"k"}
	case ModelRidge:
		return []string{"alpha"}
	case ModelAdaBoost:
		return []string{"estimators"}
	default:
		return nil
	}
}

func numericAxisProfile(modelType ModelType, paramName, xName string, values []int, required int, apply func(*MLConfig, int)) sweepProfile {
	return axisProfile(modelType, paramName, "numeric", xName, values, required, apply, strconv.Itoa)
}
