package app

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// RWKVAttention implements the RWKV (Receptance Weighted Key Value) attention mechanism.
// This is a linear attention variant that replaces softmax with element-wise operations,
// achieving O(N) complexity instead of O(N²).
// Formula: output = sigmoid(R) ⊙ (exp(W+K) ⊙ V) / (exp(W+K) ⊙ 1)
type RWKVAttention struct {
	Wr         [FeatureDim][FeatureDim]float64 `json:"-"` // Receptance projection
	Wk         [FeatureDim][FeatureDim]float64 `json:"-"` // Key projection
	Wv         [FeatureDim][FeatureDim]float64 `json:"-"` // Value projection
	Wo         [FeatureDim][FeatureDim]float64 `json:"-"` // Output projection
	W          [FeatureDim]float64             `json:"-"` // Time-mixing weights
	LastR      [FeatureDim]float64             `json:"-"` // Receptance
	LastK      [FeatureDim]float64             `json:"-"` // Key
	LastV      [FeatureDim]float64             `json:"-"` // Value
	LastWK     [FeatureDim]float64             `json:"-"` // W + K
	LastOutput [FeatureDim]float64             `json:"-"`
}

func NewRWKVAttention() *RWKVAttention {
	m := &RWKVAttention{}

	// Initialize projection matrices as identity
	for i := 0; i < FeatureDim; i++ {
		m.Wr[i][i] = 1.0
		m.Wk[i][i] = 1.0
		m.Wv[i][i] = 1.0
		m.Wo[i][i] = 1.0
		// Initialize time-mixing weights
		m.W[i] = 0.0 // Start with neutral time mixing
	}

	return m
}

func (m *RWKVAttention) Type() ModelType { return ModelRWKVAttention }

func (m *RWKVAttention) Forward(input [FeatureDim]float64) [FeatureDim]float64 {
	// Project to R (receptance), K (key), V (value)
	for i := 0; i < FeatureDim; i++ {
		m.LastR[i] = 0.0
		m.LastK[i] = 0.0
		m.LastV[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastR[i] += m.Wr[i][j] * input[j]
			m.LastK[i] += m.Wk[i][j] * input[j]
			m.LastV[i] += m.Wv[i][j] * input[j]
		}
	}

	// Apply sigmoid to receptance: σ(R)
	var sigmoidR [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		sigmoidR[i] = 1.0 / (1.0 + math.Exp(-m.LastR[i]))
	}

	// Compute W + K (time-mixing)
	for i := 0; i < FeatureDim; i++ {
		m.LastWK[i] = m.W[i] + m.LastK[i]
	}

	// Compute exp(W+K)
	var expWK [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		expWK[i] = math.Exp(m.LastWK[i])
	}

	// Numerator: exp(W+K) ⊙ V
	var numerator [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		numerator[i] = expWK[i] * m.LastV[i]
	}

	// Denominator: sum(exp(W+K)) for normalization
	denominator := 0.0
	for i := 0; i < FeatureDim; i++ {
		denominator += expWK[i]
	}
	if denominator < 1e-8 {
		denominator = 1.0 // Avoid division by zero
	}

	// Normalized attention: (exp(W+K) ⊙ V) / sum(exp(W+K))
	var attnOut [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		attnOut[i] = numerator[i] / denominator
	}

	// Apply receptance gating: σ(R) ⊙ normalized_attention
	var gatedOut [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		gatedOut[i] = sigmoidR[i] * attnOut[i]
	}

	// Output projection
	for i := 0; i < FeatureDim; i++ {
		m.LastOutput[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastOutput[i] += m.Wo[i][j] * gatedOut[j]
		}
	}

	return m.LastOutput
}

func (m *RWKVAttention) Backward(x, gradOutput [FeatureDim]float64) [FeatureDim]float64 {
	// Simplified gradient: pass through with output projection transpose
	var gradInput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			gradInput[i] += m.Wo[j][i] * gradOutput[j]
		}
	}
	return gradInput
}

func (m *RWKVAttention) Serialize(path string) error {
	data := make([]byte, 0, 8+FeatureDim*FeatureDim*8*4+FeatureDim*8)
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

	data = append(data, []byte("RWKV")...)
	putU32(1) // version

	// Serialize Wr, Wk, Wv, Wo
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wr[i][j])
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

	// Serialize time-mixing weights W
	for i := 0; i < FeatureDim; i++ {
		putF64(m.W[i])
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

func DeserializeRWKVAttention(path string) (*RWKVAttention, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 || string(raw[:4]) != "RWKV" {
		return nil, fmt.Errorf("invalid RWKV attention model file")
	}

	pos := 4
	readU32 := func() uint32 { v := binary.LittleEndian.Uint32(raw[pos:]); pos += 4; return v }
	readF64 := func() float64 { v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:])); pos += 8; return v }
	_ = readU32() // version

	m := NewRWKVAttention()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wr[i][j] = readF64()
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

	// Deserialize time-mixing weights W
	for i := 0; i < FeatureDim; i++ {
		m.W[i] = readF64()
	}

	return m, nil
}
