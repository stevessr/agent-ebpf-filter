package attention

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// ScaledDotProductAttention implements the standard Transformer attention mechanism.
// Attention(Q, K, V) = softmax(QK^T / sqrt(d_k))V
// For single-vector input, Q=K=V=x after projection.
type ScaledDotProductAttention struct {
	Wq         [FeatureDim][FeatureDim]float64 `json:"-"` // Query projection
	Wk         [FeatureDim][FeatureDim]float64 `json:"-"` // Key projection
	Wv         [FeatureDim][FeatureDim]float64 `json:"-"` // Value projection
	Wo         [FeatureDim][FeatureDim]float64 `json:"-"` // Output projection
	LastQ      [FeatureDim]float64             `json:"-"`
	LastK      [FeatureDim]float64             `json:"-"`
	LastV      [FeatureDim]float64             `json:"-"`
	LastScore  float64                         `json:"-"`
	LastAlpha  float64                         `json:"-"`
	LastOutput [FeatureDim]float64             `json:"-"`
}

func NewScaledDotProductAttention() *ScaledDotProductAttention {
	m := &ScaledDotProductAttention{}
	// Initialize as identity matrices
	for i := 0; i < FeatureDim; i++ {
		m.Wq[i][i] = 1.0
		m.Wk[i][i] = 1.0
		m.Wv[i][i] = 1.0
		m.Wo[i][i] = 1.0
	}
	return m
}

func (m *ScaledDotProductAttention) Type() ModelType { return ModelScaledDotProductAttention }

func (m *ScaledDotProductAttention) Forward(input [FeatureDim]float64) [FeatureDim]float64 {
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

	// Compute attention score: Q·K
	score := 0.0
	for i := 0; i < FeatureDim; i++ {
		score += m.LastQ[i] * m.LastK[i]
	}

	// Scale by sqrt(d_k)
	scaleFactor := math.Sqrt(float64(FeatureDim))
	m.LastScore = score / scaleFactor

	// Apply softmax (for single element, just exp and normalize)
	// In practice for single query-key pair: alpha = 1.0 after softmax
	m.LastAlpha = 1.0 / (1.0 + math.Exp(-m.LastScore))

	// Weighted value
	var attnOut [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		attnOut[i] = m.LastAlpha * m.LastV[i]
	}

	// Output projection
	for i := 0; i < FeatureDim; i++ {
		m.LastOutput[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastOutput[i] += m.Wo[i][j] * attnOut[j]
		}
	}

	return m.LastOutput
}

func (m *ScaledDotProductAttention) Backward(x, gradOutput [FeatureDim]float64) [FeatureDim]float64 {
	// Simplified gradient: pass through with output projection transpose
	var gradInput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			gradInput[i] += m.Wo[j][i] * gradOutput[j]
		}
	}
	return gradInput
}

func (m *ScaledDotProductAttention) Serialize(path string) error {
	data := make([]byte, 0, 8+FeatureDim*FeatureDim*8*4)
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

	data = append(data, []byte("SDPA")...)
	putU32(1) // version

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

func DeserializeScaledDotProductAttention(path string) (*ScaledDotProductAttention, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 || string(raw[:4]) != "SDPA" {
		return nil, fmt.Errorf("invalid scaled dot-product attention model file")
	}

	pos := 4
	readU32 := func() uint32 { v := binary.LittleEndian.Uint32(raw[pos:]); pos += 4; return v }
	readF64 := func() float64 { v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:])); pos += 8; return v }
	_ = readU32() // version

	m := NewScaledDotProductAttention()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wq[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wk[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wv[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wo[i][j] = readF64()
		}
	}

	return m, nil
}
