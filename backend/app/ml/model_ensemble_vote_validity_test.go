package ml

import (
	"math"
	"testing"
)

type ensembleVoteStub struct{ prediction Prediction }

func (m *ensembleVoteStub) Predict([FeatureDim]float64) Prediction { return m.prediction }
func (m *ensembleVoteStub) Serialize(string) error                 { return nil }
func (m *ensembleVoteStub) Type() ModelType                        { return ModelType("ensemble-vote-stub") }

func TestEnsembleAbstainingMembersDoNotInventAllowOrRiskVotes(t *testing.T) {
	unknown := &ensembleVoteStub{prediction: Prediction{Action: 0, Confidence: 0}}
	risk := &ensembleVoteStub{prediction: Prediction{Action: 1, Confidence: 0.8}}
	for _, mode := range []string{"hard", "soft", "stacked"} {
		ensemble := NewEnsembleModel([]Model{unknown, risk}, mode, []float64{0.8, 0.2})
		prediction := ensemble.Predict([FeatureDim]float64{})
		if prediction.Action != 1 {
			t.Fatalf("%s: unknown member overruled BLOCK: %+v", mode, prediction)
		}
		if math.IsNaN(prediction.Confidence) || prediction.Confidence <= 0 {
			t.Fatalf("%s: invalid confidence %+v", mode, prediction)
		}
	}
	allUnknown := NewEnsembleModel([]Model{unknown, unknown}, "soft", nil)
	if pred := allUnknown.Predict([FeatureDim]float64{}); pred.Confidence != 0 {
		t.Fatalf("untrained ensemble must abstain, got %+v", pred)
	}
}

func TestEnsembleSkipsInvalidModelWeightsAndConfidences(t *testing.T) {
	good := &ensembleVoteStub{prediction: Prediction{Action: 0, Confidence: 0.9}}
	bad := &ensembleVoteStub{prediction: Prediction{Action: 1, Confidence: math.NaN()}}
	weights := []float64{1, math.NaN()}
	m := NewEnsembleModel([]Model{good, bad}, "soft", weights)
	if !math.IsNaN(weights[1]) {
		t.Fatal("model constructor mutated caller-owned weights")
	}
	prediction := m.Predict([FeatureDim]float64{})
	if prediction.Action != 0 || math.IsNaN(prediction.Confidence) || prediction.Confidence < 0.5 {
		t.Fatalf("invalid member biased soft vote: %+v", prediction)
	}
}

func TestStackedEnsembleDoesNotInflateBlockConfidenceUsingAllowVotes(t *testing.T) {
	allow := &ensembleVoteStub{prediction: Prediction{Action: 0, Confidence: 0.95}}
	risk := &ensembleVoteStub{prediction: Prediction{Action: 1, Confidence: 0.10}}
	m := NewEnsembleModel([]Model{allow, allow, risk}, "stacked", []float64{0.4, 0.4, 0.2})
	pred := m.Predict([FeatureDim]float64{})
	if pred.Action != 1 || math.Abs(pred.Confidence-0.10) > 1e-12 {
		t.Fatalf("stacked ensemble borrowed ALLOW certainty to BLOCK: %+v", pred)
	}
}
