package app

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// ---- moved from backend/zz_merged_backend.go section ml__attention_additive attention (bahdanau).go ----

// AdditiveAttention implements Bahdanau attention over a single feature vector input.
// The layer projects the input with W_f, applies tanh, scores with v^T, softmaxes
// the score, and returns the weighted context. For a single input vector this
// reduces to a gated residual projection over [FeatureDim]float64.
type AdditiveAttention struct {
	W          [FeatureDim][FeatureDim]float64 `json:"-"`
	B          [FeatureDim]float64             `json:"-"`
	V          [FeatureDim]float64             `json:"-"`
	LastInput  [FeatureDim]float64             `json:"-"`
	LastHidden [FeatureDim]float64             `json:"-"`
	LastScore  float64                         `json:"-"`
	LastAlpha  float64                         `json:"-"`
}

func NewAdditiveAttention() *AdditiveAttention {
	m := &AdditiveAttention{}
	for i := 0; i < FeatureDim; i++ {
		m.V[i] = 1.0
		for j := 0; j < FeatureDim; j++ {
			if i == j {
				m.W[i][j] = 1.0
			}
		}
	}
	return m
}

func (m *AdditiveAttention) Type() ModelType { return ModelAdditiveAttention }

func (m *AdditiveAttention) Forward(input [FeatureDim]float64) [FeatureDim]float64 {
	m.LastInput = input
	for i := 0; i < FeatureDim; i++ {
		x := m.B[i]
		for j := 0; j < FeatureDim; j++ {
			x += m.W[i][j] * input[j]
		}
		m.LastHidden[i] = math.Tanh(x)
	}
	score := 0.0
	for i := 0; i < FeatureDim; i++ {
		score += m.V[i] * m.LastHidden[i]
	}
	m.LastScore = score
	m.LastAlpha = 1.0
	var out [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		out[i] = m.LastAlpha * input[i]
	}
	return out
}

func (m *AdditiveAttention) Backward(x, gradOutput [FeatureDim]float64) [FeatureDim]float64 {
	_ = x
	var gradInput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		gradInput[i] = m.LastAlpha * gradOutput[i]
	}
	return gradInput
}

func (m *AdditiveAttention) Serialize(path string) error {
	data := make([]byte, 0, 4+4+FeatureDim*FeatureDim*8+FeatureDim*8*2)
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

	data = append(data, []byte("ATAD")...)
	putU32(1)
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.W[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		putF64(m.B[i])
	}
	for i := 0; i < FeatureDim; i++ {
		putF64(m.V[i])
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

func DeserializeAdditiveAttention(path string) (*AdditiveAttention, error) {
	r, err := newMLBinaryModelReader(path, "ATAD")
	if err != nil {
		return nil, err
	}
	r.readVersion()
	r.requireItems("additive attention", FeatureDim*FeatureDim+2*FeatureDim, mlBinaryFloatBytes, 0)
	if err := r.doneIfInvalid(); err != nil {
		return nil, err
	}
	m := NewAdditiveAttention()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.W[i][j] = r.readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		m.B[i] = r.readF64()
	}
	for i := 0; i < FeatureDim; i++ {
		m.V[i] = r.readF64()
	}
	if err := r.done(); err != nil {
		return nil, err
	}
	return m, nil
}
