package attention

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func init() {
	RegisterModel(ModelType("cross-attention"), func() Model { return NewCrossAttentionLayer() })
}

// AttentionLayer defines a single-head attention layer over fixed-size feature vectors.

// CrossAttentionLayer implements Y = softmax((QK^T)/sqrt(d_k))V with linear projections.
// For the fixed [FeatureDim]float64 setting, Q, K, and V are treated as length-FeatureDim vectors.
type CrossAttentionLayer struct {
	Wq [FeatureDim][FeatureDim]float64 `json:"-"`
	Wk [FeatureDim][FeatureDim]float64 `json:"-"`
	Wv [FeatureDim][FeatureDim]float64 `json:"-"`

	LastQ [FeatureDim]float64 `json:"-"`
	LastK [FeatureDim]float64 `json:"-"`
	LastV [FeatureDim]float64 `json:"-"`
	LastA [FeatureDim]float64 `json:"-"`
	LastY [FeatureDim]float64 `json:"-"`

	LearningRate float64 `json:"learningRate"`
}

func NewCrossAttentionLayer() *CrossAttentionLayer {
	m := &CrossAttentionLayer{LearningRate: 0.01}
	for i := 0; i < FeatureDim; i++ {
		m.Wq[i][i] = 1
		m.Wk[i][i] = 1
		m.Wv[i][i] = 1
	}
	return m
}

func (m *CrossAttentionLayer) Type() ModelType { return ModelType("cross-attention") }

func (m *CrossAttentionLayer) Predict(features [FeatureDim]float64) Prediction {
	// Keep the Model interface satisfied; use self-attention on the same input.
	y := m.Output(features, features, features)
	score := 0.0
	for i := 0; i < FeatureDim; i++ {
		score += y[i]
	}
	confidence := 1.0 / (1.0 + math.Exp(-score/float64(FeatureDim)))
	return Prediction{Action: 0, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *CrossAttentionLayer) softmax1D(x [FeatureDim]float64) [FeatureDim]float64 {
	maxV := x[0]
	for i := 1; i < FeatureDim; i++ {
		if x[i] > maxV {
			maxV = x[i]
		}
	}
	sum := 0.0
	var out [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		out[i] = math.Exp(x[i] - maxV)
		sum += out[i]
	}
	if sum == 0 {
		return out
	}
	for i := 0; i < FeatureDim; i++ {
		out[i] /= sum
	}
	return out
}

func (m *CrossAttentionLayer) matVec(W [FeatureDim][FeatureDim]float64, x [FeatureDim]float64) [FeatureDim]float64 {
	var y [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		sum := 0.0
		for j := 0; j < FeatureDim; j++ {
			sum += W[i][j] * x[j]
		}
		y[i] = sum
	}
	return y
}

func dotProduct(a, b [FeatureDim]float64) float64 {
	s := 0.0
	for i := 0; i < FeatureDim; i++ {
		s += a[i] * b[i]
	}
	return s
}

func (m *CrossAttentionLayer) Output(featuresQ, featuresK, featuresV [FeatureDim]float64) [FeatureDim]float64 {
	q := m.matVec(m.Wq, featuresQ)
	k := m.matVec(m.Wk, featuresK)
	v := m.matVec(m.Wv, featuresV)

	scale := math.Sqrt(float64(FeatureDim))
	var scores [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		scores[i] = (q[i] * k[i]) / scale
	}
	a := m.softmax1D(scores)

	var y [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		y[i] = a[i] * v[i]
	}

	m.LastQ, m.LastK, m.LastV, m.LastA, m.LastY = q, k, v, a, y
	return y
}

func (m *CrossAttentionLayer) Backward(dY [FeatureDim]float64) {
	// Minimal, deterministic SGD update using the cached forward pass.
	if m.LearningRate <= 0 {
		m.LearningRate = 0.01
	}

	scale := math.Sqrt(float64(FeatureDim))
	var dA, dV [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		dA[i] = dY[i] * m.LastV[i]
		dV[i] = dY[i] * m.LastA[i]
	}

	var sumAD float64
	for i := 0; i < FeatureDim; i++ {
		sumAD += dA[i] * m.LastA[i]
	}
	var dScores [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		dScores[i] = m.LastA[i] * (dA[i] - sumAD)
	}

	var dQ, dK [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		dQ[i] = dScores[i] * m.LastK[i] / scale
		dK[i] = dScores[i] * m.LastQ[i] / scale
	}

	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wq[i][j] -= m.LearningRate * dQ[i] * m.LastQ[j]
			m.Wk[i][j] -= m.LearningRate * dK[i] * m.LastK[j]
			m.Wv[i][j] -= m.LearningRate * dV[i] * m.LastV[j]
		}
	}
}

func (m *CrossAttentionLayer) Serialize(path string) error {
	data := make([]byte, 0, 4+4+8+3*FeatureDim*FeatureDim*8)
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

	data = append(data, []byte("CATN")...)
	putU32(1)
	putF64(m.LearningRate)
	for _, W := range [3][FeatureDim][FeatureDim]float64{m.Wq, m.Wk, m.Wv} {
		for i := 0; i < FeatureDim; i++ {
			for j := 0; j < FeatureDim; j++ {
				putF64(W[i][j])
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DeserializeCrossAttention(path string) (*CrossAttentionLayer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	need := 4 + 4 + 8 + 3*FeatureDim*FeatureDim*8
	if len(raw) < need || string(raw[:4]) != "CATN" {
		return nil, fmt.Errorf("invalid cross-attention model file")
	}
	pos := 4
	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(raw[pos:])
		pos += 4
		return v
	}
	readF64 := func() float64 {
		v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:]))
		pos += 8
		return v
	}
	_ = readU32()
	m := NewCrossAttentionLayer()
	m.LearningRate = readF64()
	for _, W := range []*[FeatureDim][FeatureDim]float64{&m.Wq, &m.Wk, &m.Wv} {
		for i := 0; i < FeatureDim; i++ {
			for j := 0; j < FeatureDim; j++ {
				W[i][j] = readF64()
			}
		}
	}
	return m, nil
}
