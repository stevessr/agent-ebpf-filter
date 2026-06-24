package attention

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossAttentionForwardMatchesMath(t *testing.T) {
	m := NewCrossAttentionLayer()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wq[i][j] = 0
			m.Wk[i][j] = 0
			m.Wv[i][j] = 0
		}
	}
	m.Wq[0][0], m.Wq[1][1] = 2, 3
	m.Wk[0][0], m.Wk[1][1] = 4, 5
	m.Wv[0][0], m.Wv[1][1] = 6, 7

	var fq, fk, fv [FeatureDim]float64
	fq[0], fq[1] = 1, 1
	fk[0], fk[1] = 1, 2
	fv[0], fv[1] = 10, 20

	y := m.Output(fq, fk, fv)
	q := [FeatureDim]float64{2, 3}
	k := [FeatureDim]float64{4, 10}
	v := [FeatureDim]float64{60, 140}
	scale := math.Sqrt(float64(FeatureDim))
	s0 := q[0] * k[0] / scale
	s1 := q[1] * k[1] / scale
	m0 := math.Max(s0, s1)
	e0 := math.Exp(s0 - m0)
	e1 := math.Exp(s1 - m0)
	eRest := math.Exp(-m0) * float64(FeatureDim-2)
	sum := e0 + e1 + eRest
	a0 := e0 / sum
	a1 := e1 / sum

	if got, want := y[0], a0*v[0]; math.Abs(got-want) > 1e-9 {
		t.Fatalf("y[0]=%v want %v", got, want)
	}
	if got, want := y[1], a1*v[1]; math.Abs(got-want) > 1e-9 {
		t.Fatalf("y[1]=%v want %v", got, want)
	}
}

func TestCrossAttentionSerializeDeserialize(t *testing.T) {
	m := NewCrossAttentionLayer()
	m.LearningRate = 0.123
	m.Wq[0][1] = 1.5
	m.Wk[2][3] = -2.25
	m.Wv[4][5] = 3.75

	dir := t.TempDir()
	path := filepath.Join(dir, "catn.bin")
	if err := m.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	loaded, err := DeserializeCrossAttention(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.LearningRate != m.LearningRate {
		t.Fatalf("learning rate mismatch: got %v want %v", loaded.LearningRate, m.LearningRate)
	}
	if loaded.Wq[0][1] != m.Wq[0][1] || loaded.Wk[2][3] != m.Wk[2][3] || loaded.Wv[4][5] != m.Wv[4][5] {
		t.Fatalf("weights did not round-trip")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("serialized file missing: %v", err)
	}
}

func TestCrossAttentionBackwardUpdatesWeights(t *testing.T) {
	m := NewCrossAttentionLayer()
	var fq, fk, fv [FeatureDim]float64
	fq[0], fk[0], fv[0] = 1, 1, 1
	_ = m.Output(fq, fk, fv)
	before := m.Wq[0][0]
	m.Backward([FeatureDim]float64{1})
	if m.Wq[0][0] == before {
		t.Fatalf("expected backward pass to update weights")
	}
}
