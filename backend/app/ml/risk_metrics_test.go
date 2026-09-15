package ml

import (
	"math"
	"testing"
	"time"
)

type riskMetricTestModel struct {
	predictions []Prediction
}

func (m *riskMetricTestModel) Predict(features [FeatureDim]float64) Prediction {
	index := int(features[0])
	if index < 0 || index >= len(m.predictions) {
		return Prediction{Action: 0, Confidence: 0}
	}
	return m.predictions[index]
}

func (m *riskMetricTestModel) Serialize(string) error { return nil }
func (m *riskMetricTestModel) Type() ModelType         { return ModelType("risk-metric-test") }

func TestEvaluateRiskModelTracksSecurityAndCalibrationMetrics(t *testing.T) {
	model := &riskMetricTestModel{predictions: []Prediction{
		{Action: 0, Confidence: 0.90}, // benign correct
		{Action: 1, Confidence: 0.80}, // block correct
		{Action: 0, Confidence: 0.60}, // alert missed
		{Action: 1, Confidence: 0.70}, // benign false positive
	}}

	samples := make([]TrainingSample, 4)
	labels := []int32{0, 1, 3, 0}
	for i := range samples {
		samples[i].Features[0] = float64(i)
		samples[i].Label = labels[i]
	}

	metrics := evaluateRiskModel(model, samples)
	assertNear(t, "accuracy", metrics.Accuracy, 0.5)
	assertNear(t, "balanced accuracy", metrics.BalancedAccuracy, 0.5)
	assertNear(t, "high-risk recall", metrics.HighRiskRecall, 0.5)
	assertNear(t, "benign false-positive rate", metrics.BenignFalsePositiveRate, 0.5)
	assertNear(t, "confidence brier", metrics.ConfidenceBrier, 0.225)
	if metrics.ECE <= 0 || metrics.ECE > 1 {
		t.Fatalf("ECE must be in (0,1], got %f", metrics.ECE)
	}
	if metrics.HighRiskSamples != 2 || metrics.BenignSamples != 2 || metrics.Samples != 4 {
		t.Fatalf("unexpected sample counts: %+v", metrics)
	}
}

func TestEvaluateRiskModelClampsInvalidConfidence(t *testing.T) {
	model := &riskMetricTestModel{predictions: []Prediction{
		{Action: 0, Confidence: math.NaN()},
		{Action: 1, Confidence: 4.0},
	}}
	samples := make([]TrainingSample, 2)
	samples[0].Features[0] = 0
	samples[0].Label = 0
	samples[1].Features[0] = 1
	samples[1].Label = 1

	metrics := evaluateRiskModel(model, samples)
	if math.IsNaN(metrics.ConfidenceBrier) || math.IsInf(metrics.ConfidenceBrier, 0) {
		t.Fatalf("confidence metrics must remain finite: %+v", metrics)
	}
	if metrics.ConfidenceBrier < 0 || metrics.ConfidenceBrier > 1 || metrics.ECE < 0 || metrics.ECE > 1 {
		t.Fatalf("confidence metrics must remain bounded: %+v", metrics)
	}
}

func TestCalibratedEnsembleWeightShrinksSmallValidationSets(t *testing.T) {
	base := riskEvaluation{
		Accuracy:                0.95,
		BalancedAccuracy:        0.90,
		HighRiskSamples:         2,
		HighRiskRecall:          1.0,
		BenignSamples:           2,
		BenignFalsePositiveRate: 0.0,
		ConfidenceBrier:         0.05,
		ECE:                     0.05,
	}

	small := base
	small.Samples = 4
	large := base
	large.Samples = 200
	large.HighRiskSamples = 100
	large.BenignSamples = 100

	smallWeight := calibratedEnsembleWeight(small)
	largeWeight := calibratedEnsembleWeight(large)
	if !(largeWeight > smallWeight && smallWeight > 0.5) {
		t.Fatalf("expected evidence-sensitive shrinkage, small=%f large=%f", smallWeight, largeWeight)
	}
}

func TestSplitEnsembleCalibrationHoldoutSortsRingBufferChronologically(t *testing.T) {
	samples := make([]TrainingSample, 30)
	base := time.Unix(1_700_000_000, 0)
	for i := range samples {
		// Simulate a wrapped ring: physical slots are the reverse of event time.
		age := len(samples) - 1 - i
		samples[i].Features[0] = float64(age)
		samples[i].Timestamp = base.Add(time.Duration(age) * time.Minute)
	}

	trainSet, validationSet := splitEnsembleCalibrationHoldout(samples)
	if len(trainSet) != 24 || len(validationSet) != 6 {
		t.Fatalf("unexpected split sizes train=%d validation=%d", len(trainSet), len(validationSet))
	}
	if trainSet[0].Features[0] != 0 || trainSet[len(trainSet)-1].Features[0] != 23 || validationSet[0].Features[0] != 24 || validationSet[len(validationSet)-1].Features[0] != 29 {
		t.Fatalf("holdout must be chronological: train=%v..%v validation=%v..%v", trainSet[0].Features[0], trainSet[len(trainSet)-1].Features[0], validationSet[0].Features[0], validationSet[len(validationSet)-1].Features[0])
	}

	smallTrain, smallValidation := splitEnsembleCalibrationHoldout(samples[:29])
	if len(smallTrain) != 29 || len(smallValidation) != 0 {
		t.Fatalf("small datasets should keep all samples for training")
	}
}

func TestChronologicalTrainingSamplesKeepsUnknownTimestampsOutOfNewestWindow(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	samples := []TrainingSample{
		{Timestamp: base.Add(time.Minute)},
		{},
		{Timestamp: base},
	}
	ordered := chronologicalTrainingSamples(samples)
	if !ordered[0].Timestamp.IsZero() || !ordered[1].Timestamp.Equal(base) || !ordered[2].Timestamp.Equal(base.Add(time.Minute)) {
		t.Fatalf("unexpected chronological order: %v %v %v", ordered[0].Timestamp, ordered[1].Timestamp, ordered[2].Timestamp)
	}
}

func assertNear(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s: got %f want %f", name, got, want)
	}
}
