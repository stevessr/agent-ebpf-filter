package app

import "math"

// FeatureNormalizationReport summarizes the bounded feature-space contract used
// by the ML pipeline. The extractor emits a compact 128-dim vector where every
// dimension must be finite and normalized into [0, 1] before samples are stored,
// tuned, or sent to a model.
type FeatureNormalizationReport struct {
	Mode                  string  `json:"mode"`
	SampleCount           int     `json:"sampleCount"`
	FeatureDim            int     `json:"featureDim"`
	MinObserved           float64 `json:"minObserved"`
	MaxObserved           float64 `json:"maxObserved"`
	NonFiniteValues       int     `json:"nonFiniteValues"`
	BelowZeroValues       int     `json:"belowZeroValues"`
	AboveOneValues        int     `json:"aboveOneValues"`
	ZeroVarianceFeatures  int     `json:"zeroVarianceFeatures"`
	NormalizedFeatureHint string  `json:"normalizedFeatureHint"`
}

func normalizeFeatureVector(f [FeatureDim]float64) [FeatureDim]float64 {
	for i, v := range f {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			f[i] = 0
			continue
		}

		switch i {
		case 116, 117:
			// Cyclic hour components are naturally in [-1, 1]. Remap them so
			// all downstream models see a homogeneous [0, 1] feature space.
			v = (v + 1.0) / 2.0
		}

		f[i] = clampFeature01(v)
	}
	return f
}

func clampFeature01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func summarizeFeatureNormalization(samples []TrainingSample) FeatureNormalizationReport {
	report := FeatureNormalizationReport{
		Mode:                  "bounded-0-1",
		SampleCount:           len(samples),
		FeatureDim:            FeatureDim,
		MinObserved:           0,
		MaxObserved:           0,
		NormalizedFeatureHint: "FeatureExtractor repairs non-finite values, maps cyclic hour sin/cos into [0,1], and clamps every dimension to [0,1].",
	}
	if len(samples) == 0 {
		report.ZeroVarianceFeatures = FeatureDim
		return report
	}

	var mins, maxs [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		mins[i] = math.Inf(1)
		maxs[i] = math.Inf(-1)
	}
	globalMin := math.Inf(1)
	globalMax := math.Inf(-1)

	for _, sample := range samples {
		for i, v := range sample.Features {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				report.NonFiniteValues++
				continue
			}
			if v < 0 {
				report.BelowZeroValues++
			}
			if v > 1 {
				report.AboveOneValues++
			}
			if v < mins[i] {
				mins[i] = v
			}
			if v > maxs[i] {
				maxs[i] = v
			}
			if v < globalMin {
				globalMin = v
			}
			if v > globalMax {
				globalMax = v
			}
		}
	}

	zeroVariance := 0
	for i := 0; i < FeatureDim; i++ {
		if math.IsInf(mins[i], 0) || math.IsInf(maxs[i], 0) || mins[i] == maxs[i] {
			zeroVariance++
		}
	}
	report.ZeroVarianceFeatures = zeroVariance
	if !math.IsInf(globalMin, 0) {
		report.MinObserved = globalMin
	}
	if !math.IsInf(globalMax, 0) {
		report.MaxObserved = globalMax
	}
	return report
}
