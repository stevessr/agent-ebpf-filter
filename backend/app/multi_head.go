package app

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// MultiHeadAttention implements multi-head attention mechanism.
// Splits input into multiple heads, applies scaled dot-product attention per head,
// then concatenates and projects the results.
type MultiHeadAttention struct {
	NumHeads   int                             `json:"numHeads"`
	HeadDim    int                             `json:"headDim"`
	Wq         [FeatureDim][FeatureDim]float64 `json:"-"` // Combined Q projection for all heads
	Wk         [FeatureDim][FeatureDim]float64 `json:"-"` // Combined K projection for all heads
	Wv         [FeatureDim][FeatureDim]float64 `json:"-"` // Combined V projection for all heads
	Wo         [FeatureDim][FeatureDim]float64 `json:"-"` // Output projection
	LastQ      [FeatureDim]float64             `json:"-"`
	LastK      [FeatureDim]float64             `json:"-"`
	LastV      [FeatureDim]float64             `json:"-"`
	LastScores []float64                       `json:"-"` // Per-head scores
	LastAlphas []float64                       `json:"-"` // Per-head attention weights
	LastOutput [FeatureDim]float64             `json:"-"`
}

func NewMultiHeadAttention(numHeads int) *MultiHeadAttention {
	if numHeads <= 0 || FeatureDim%numHeads != 0 {
		numHeads = 4 // Default: 4 heads
	}
	headDim := FeatureDim / numHeads

	m := &MultiHeadAttention{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		LastScores: make([]float64, numHeads),
		LastAlphas: make([]float64, numHeads),
	}

	// Initialize as identity matrices
	for i := 0; i < FeatureDim; i++ {
		m.Wq[i][i] = 1.0
		m.Wk[i][i] = 1.0
		m.Wv[i][i] = 1.0
		m.Wo[i][i] = 1.0
	}

	return m
}

func (m *MultiHeadAttention) Type() ModelType { return ModelMultiHeadAttention }

func (m *MultiHeadAttention) Forward(input [FeatureDim]float64) [FeatureDim]float64 {
	// Project to Q, K, V
	for i := 0; i < FeatureDim; i++ {
		m.LastQ[i] = 0.0
		m.LastK[i] = 0.0
		m.LastV[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastQ[i] += m.Wq[i][j] * input[j]
			m.LastK[i] += m.Wk[i][j] * input[j]
			m.LastV[i] += m.Wv[i][j] * input[j]
		}
	}

	// Multi-head attention
	var concatHeads [FeatureDim]float64
	scaleFactor := math.Sqrt(float64(m.HeadDim))

	for h := 0; h < m.NumHeads; h++ {
		startIdx := h * m.HeadDim
		endIdx := startIdx + m.HeadDim

		// Compute attention score for this head: Q_h · K_h
		score := 0.0
		for i := startIdx; i < endIdx; i++ {
			score += m.LastQ[i] * m.LastK[i]
		}
		score /= scaleFactor
		m.LastScores[h] = score

		// Apply softmax (for single query-key pair, use sigmoid approximation)
		m.LastAlphas[h] = 1.0 / (1.0 + math.Exp(-score))

		// Weighted value for this head
		for i := startIdx; i < endIdx; i++ {
			concatHeads[i] = m.LastAlphas[h] * m.LastV[i]
		}
	}

	// Output projection
	for i := 0; i < FeatureDim; i++ {
		m.LastOutput[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastOutput[i] += m.Wo[i][j] * concatHeads[j]
		}
	}

	return m.LastOutput
}

func (m *MultiHeadAttention) Backward(x, gradOutput [FeatureDim]float64) [FeatureDim]float64 {
	// Simplified gradient: pass through with output projection transpose
	var gradInput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			gradInput[i] += m.Wo[j][i] * gradOutput[j]
		}
	}
	return gradInput
}

func (m *MultiHeadAttention) Serialize(path string) error {
	data := make([]byte, 0, 16+FeatureDim*FeatureDim*8*4)
	putU32 := func(v uint32) {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, v)
		data = append(data, b...)
	}
	putF64 := func(v float64) {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		data = append(data, b...)
	}

	data = append(data, []byte("MHAT")...)
	putU32(1) // version
	putU32(uint32(m.NumHeads))
	putU32(uint32(m.HeadDim))

	// Serialize Wq, Wk, Wv, Wo
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wq[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wk[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wv[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wo[i][j])
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DeserializeMultiHeadAttention(path string) (*MultiHeadAttention, error) {
	r, err := newMLBinaryModelReader(path, "MHAT")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	numHeads := r.readBoundedCount("multi-head attention head count", 1, FeatureDim)
	headDim := r.readBoundedCount("multi-head attention head dimension", 1, FeatureDim)
	if numHeads*headDim != FeatureDim || FeatureDim%numHeads != 0 {
		return nil, fmt.Errorf("invalid multi-head attention shape %dx%d", numHeads, headDim)
	}
	r.requireItems("multi-head attention", 4*FeatureDim*FeatureDim, mlBinaryFloatBytes, 0)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}

	m := &MultiHeadAttention{
		NumHeads:   numHeads,
		HeadDim:    headDim,
		LastScores: make([]float64, numHeads),
		LastAlphas: make([]float64, numHeads),
	}

	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wq[i][j] = r.readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wk[i][j] = r.readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wv[i][j] = r.readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wo[i][j] = r.readF64()
		}
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}
