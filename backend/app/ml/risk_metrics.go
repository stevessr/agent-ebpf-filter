package ml

import "math"

const confidenceCalibrationBins = 10

// riskEvaluation captures security-oriented quality metrics for a classifier.
// ConfidenceBrier and ECE measure whether Prediction.Confidence behaves like a
// trustworthy probability of the predicted class being correct. HighRiskRecall
// focuses on BLOCK/ALERT labels, while BenignFalsePositiveRate protects normal
// workloads from over-eager enforcement.
type riskEvaluation struct {
	Samples                 int
	Accuracy                float64
	BalancedAccuracy        float64
	HighRiskSamples         int
	HighRiskRecall          float64
	BenignSamples           int
	BenignFalsePositiveRate float64
	ConfidenceBrier         float64
	ECE                     float64
}

type confidenceBin struct {
	count      int
	confidence float64
	correct    int
}

func evaluateRiskModel(model Model, samples []TrainingSample) riskEvaluation {
	var result riskEvaluation
	if model == nil || len(samples) == 0 {
		return result
	}

	var classTotal [4]int
	var classCorrect [4]int
	var bins [confidenceCalibrationBins]confidenceBin
	correctTotal := 0
	highRiskDetected := 0
	benignFalsePositives := 0
	brierTotal := 0.0

	for _, sample := range samples {
		label := sample.Label
		if label < 0 || label >= 4 {
			continue
		}

		pred := model.Predict(sample.Features)
		confidence := finiteUnitInterval(pred.Confidence)
		correct := pred.Action == label

		result.Samples++
		classTotal[label]++
		if correct {
			correctTotal++
			classCorrect[label]++
		}

		if label == 1 || label == 3 {
			result.HighRiskSamples++
			if pred.Action == 1 || pred.Action == 3 {
				highRiskDetected++
			}
		}
		if label == 0 {
			result.BenignSamples++
			if pred.Action != 0 {
				benignFalsePositives++
			}
		}

		target := 0.0
		if correct {
			target = 1.0
		}
		delta := confidence - target
		brierTotal += delta * delta

		binIndex := int(confidence * confidenceCalibrationBins)
		if binIndex >= confidenceCalibrationBins {
			binIndex = confidenceCalibrationBins - 1
		}
		bins[binIndex].count++
		bins[binIndex].confidence += confidence
		if correct {
			bins[binIndex].correct++
		}
	}

	if result.Samples == 0 {
		return result
	}

	result.Accuracy = float64(correctTotal) / float64(result.Samples)
	result.ConfidenceBrier = brierTotal / float64(result.Samples)

	observedClasses := 0
	for class := 0; class < len(classTotal); class++ {
		if classTotal[class] == 0 {
			continue
		}
		observedClasses++
		result.BalancedAccuracy += float64(classCorrect[class]) / float64(classTotal[class])
	}
	if observedClasses > 0 {
		result.BalancedAccuracy /= float64(observedClasses)
	}

	if result.HighRiskSamples > 0 {
		result.HighRiskRecall = float64(highRiskDetected) / float64(result.HighRiskSamples)
	}
	if result.BenignSamples > 0 {
		result.BenignFalsePositiveRate = float64(benignFalsePositives) / float64(result.BenignSamples)
	}

	for _, bin := range bins {
		if bin.count == 0 {
			continue
		}
		avgConfidence := bin.confidence / float64(bin.count)
		binAccuracy := float64(bin.correct) / float64(bin.count)
		result.ECE += float64(bin.count) / float64(result.Samples) * math.Abs(avgConfidence-binAccuracy)
	}

	return result
}

// calibratedEnsembleWeight turns hold-out quality into an ensemble weight.
// It deliberately shrinks small validation sets toward a neutral 0.5 score so
// a handful of lucky predictions cannot dominate the online classifier.
func calibratedEnsembleWeight(metrics riskEvaluation) float64 {
	if metrics.Samples == 0 {
		return 1.0
	}

	riskRecall := metrics.BalancedAccuracy
	if metrics.HighRiskSamples > 0 {
		riskRecall = metrics.HighRiskRecall
	}
	benignSpecificity := metrics.BalancedAccuracy
	if metrics.BenignSamples > 0 {
		benignSpecificity = 1 - metrics.BenignFalsePositiveRate
	}
	calibration := 1 - 0.5*(metrics.ConfidenceBrier+metrics.ECE)
	calibration = finiteUnitInterval(calibration)

	quality := 0.35*metrics.BalancedAccuracy +
		0.30*riskRecall +
		0.20*benignSpecificity +
		0.15*calibration

	// Bayesian-style shrinkage: about 20 validation observations are needed
	// before measured quality strongly overrides the neutral prior.
	shrink := float64(metrics.Samples) / float64(metrics.Samples+20)
	weight := 0.5 + shrink*(quality-0.5)
	if weight < 0.10 {
		return 0.10
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func finiteUnitInterval(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
