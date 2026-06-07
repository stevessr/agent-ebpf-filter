package app

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMultiHeadAttentionForwardMatchesMath(t *testing.T) {
	m := NewMultiHeadAttentionLayer(2)
	for h := 0; h < 2; h++ {
		for i := 0; i < FeatureDim; i++ {
			for j := 0; j < FeatureDim; j++ {
				m.WQ[h][i][j] = 0
				m.WK[h][i][j] = 0
				m.WV[h][i][j] = 0
			}
		}
	}
	for i := 0; i < FeatureDim; i++ {
		m.WO[i][i] = 0
	}
	m.WQ[0][0][0], m.WK[0][0][0], m.WV[0][0][0] = 2, 3, 4
	m.WQ[0][1][1], m.WK[0][1][1], m.WV[0][1][1] = 5, 6, 7
	m.WQ[1][0][0], m.WK[1][0][0], m.WV[1][0][0] = 8, 9, 10
	m.WQ[1][1][1], m.WK[1][1][1], m.WV[1][1][1] = 11, 12, 13
	m.WO[0][0], m.WO[1][1] = 1, 2

	var fq, fk, fv [FeatureDim]float64
	fq[0], fq[1] = 1, 1
	fk[0], fk[1] = 1, 2
	fv[0], fv[1] = 10, 20

	got := m.Output(fq, fk, fv)
	scale := math.Sqrt(float64(FeatureDim))

	// head 1
	q10, q11 := 2.0, 5.0
	k10, k11 := 3.0, 12.0
	v10, v11 := 40.0, 140.0
	s10 := q10 * k10 / scale
	s11 := q11 * k11 / scale
	m1 := math.Max(s10, s11)
	e10 := math.Exp(s10 - m1)
	e11 := math.Exp(s11 - m1)
	a10 := e10 / (e10 + e11)
	a11 := e11 / (e10 + e11)
	h10 := a10 * v10
	h11 := a11 * v11

	// head 2
	q20, q21 := 8.0, 11.0
	k20, k21 := 9.0, 24.0
	v20, v21 := 100.0, 260.0
	s20 := q20 * k20 / scale
	s21 := q21 * k21 / scale
	m2 := math.Max(s20, s21)
	e20 := math.Exp(s20 - m2)
	e21 := math.Exp(s21 - m2)
	a20 := e20 / (e20 + e21)
	a21 := e21 / (e20 + e21)
	h20 := a20 * v20
	h21 := a21 * v21

	want0 := ((h10 + h20) / 2.0) * m.WO[0][0]
	want1 := ((h11 + h21) / 2.0) * m.WO[1][1]
	if math.Abs(got[0]-want0) > 1e-9 {
		t.Fatalf("got[0]=%v want %v", got[0], want0)
	}
	if math.Abs(got[1]-want1) > 1e-9 {
		t.Fatalf("got[1]=%v want %v", got[1], want1)
	}
}

func TestMultiHeadAttentionSerializeDeserialize(t *testing.T) {
	m := NewMultiHeadAttentionLayer(3)
	m.LearningRate = 0.123
	m.WQ[0][0][1] = 1.5
	m.WK[1][2][3] = -2.25
	m.WV[2][4][5] = 3.75
	m.WO[6][7] = -4.5

	path := filepath.Join(t.TempDir(), "mha.bin")
	if err := m.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	loaded, err := DeserializeMultiHeadAttention(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.NumHeads != 3 || loaded.LearningRate != m.LearningRate {
		t.Fatalf("metadata mismatch: %+v", loaded)
	}
	if loaded.WQ[0][0][1] != 1.5 || loaded.WK[1][2][3] != -2.25 || loaded.WV[2][4][5] != 3.75 || loaded.WO[6][7] != -4.5 {
		t.Fatalf("weights did not round-trip")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("serialized file missing: %v", err)
	}
}

func TestMultiHeadAttentionBackwardUpdatesWeights(t *testing.T) {
	m := NewMultiHeadAttentionLayer(2)
	var x [FeatureDim]float64
	x[0], x[1] = 1, 2
	_ = m.Output(x, x, x)
	before := m.WO[0][0]
	m.Backward([FeatureDim]float64{1, 0})
	if m.WO[0][0] == before {
		t.Fatalf("expected backward pass to update output weights")
	}
}
