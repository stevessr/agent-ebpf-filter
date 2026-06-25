package app

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestAdditiveAttentionForwardMatchesFormula(t *testing.T) {
	m := NewAdditiveAttention()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.W[i][j] = 0
		}
		m.B[i] = 0
		m.V[i] = 0
	}
	m.W[0][0] = 2
	m.W[1][1] = -1
	m.B[0] = 0.5
	m.B[1] = -0.25
	m.V[0] = 1.2
	m.V[1] = -0.7

	in := [FeatureDim]float64{1.0, -2.0}
	out := m.Forward(in)

	h0 := math.Tanh(2*in[0] + 0.5)
	h1 := math.Tanh(-1*in[1] - 0.25)
	e := 1.2*h0 + (-0.7)*h1
	alpha := 1.0
	if math.Abs(m.LastScore-e) > 1e-12 {
		t.Fatalf("score mismatch: got %v want %v", m.LastScore, e)
	}
	if math.Abs(m.LastAlpha-alpha) > 1e-12 {
		t.Fatalf("alpha mismatch: got %v want %v", m.LastAlpha, alpha)
	}
	if out != in {
		t.Fatalf("output mismatch: got %#v want %#v", out, in)
	}
}

func TestAdditiveAttentionBackwardReturnsStableShapes(t *testing.T) {
	m := NewAdditiveAttention()
	in := [FeatureDim]float64{0.3, 0.7}
	m.Forward(in)
	gradOut := [FeatureDim]float64{1.5, -0.5}
	gradIn := m.Backward(in, gradOut)

	if gradIn[0] != gradOut[0] || gradIn[1] != gradOut[1] {
		t.Fatalf("grad input mismatch: got %#v want %#v", gradIn, gradOut)
	}
}

func TestAdditiveAttentionSerializeRoundTrip(t *testing.T) {
	m := NewAdditiveAttention()
	m.W[0][0] = 3.5
	m.B[0] = -1.25
	m.V[0] = 0.125

	path := filepath.Join(t.TempDir(), "attn.bin")
	if err := m.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	got, err := DeserializeAdditiveAttention(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if got.W[0][0] != 3.5 || got.B[0] != -1.25 || got.V[0] != 0.125 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("serialized file missing: %v", err)
	}
}
