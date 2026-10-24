package ml

import (
	"math"
	"testing"
)

func TestNaiveBayesNeverPredictsUnobservedAction(t *testing.T) {
	m := NewNaiveBayes()
	m.Means = make([][FeatureDim]float64, 4)
	m.Vars = make([][FeatureDim]float64, 4)
	m.Priors = []float64{1, 0, 0, 0}
	for d := 0; d < FeatureDim; d++ {
		m.Vars[0][d] = 1
	}
	// With a fake prior of 1e-10 and 128 zero-variance dimensions, the
	// old classifier strongly preferred the never-seen BLOCK class here.
	pred := m.Predict([FeatureDim]float64{})
	if pred.Action != 0 || pred.Confidence < 0.99 {
		t.Fatalf("unseen action dominated Naive Bayes: %+v", pred)
	}
}

func TestNaiveBayesEmptyPriorsNeverEmitConfidentDecision(t *testing.T) {
	m := NewNaiveBayes()
	m.Means = make([][FeatureDim]float64, 4)
	m.Vars = make([][FeatureDim]float64, 4)
	m.Priors = []float64{0, 0, 0, 0}
	pred := m.Predict([FeatureDim]float64{})
	if pred.Confidence != 0 || math.IsNaN(pred.AnomalyScore) {
		t.Fatalf("zero training evidence must not produce a confident action: %+v", pred)
	}
}

func TestNaiveBayesObservedMinorityClassStillPredictable(t *testing.T) {
	m := NewNaiveBayes()
	m.Means = make([][FeatureDim]float64, 4)
	m.Vars = make([][FeatureDim]float64, 4)
	m.Priors = []float64{0.99, 0.01, 0, 0}
	m.Means[1][0] = 5
	m.Vars[0][0], m.Vars[1][0] = 0.1, 0.1
	var features [FeatureDim]float64
	features[0] = 5
	pred := m.Predict(features)
	if pred.Action != 1 || pred.Confidence < 0.9 {
		t.Fatalf("observed rare BLOCK evidence lost: %+v", pred)
	}
}
