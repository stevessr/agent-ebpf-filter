package app

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

func init() {
	RegisterModel(ModelType("multi-head-attention"), func() Model { return NewMultiHeadAttentionLayer(4) })
}

// MultiHeadAttentionLayer implements scaled dot-product multi-head attention over
// fixed-size [FeatureDim]float64 feature vectors.
type MultiHeadAttentionLayer struct {
	NumHeads     int                                  `json:"numHeads"`
	LearningRate float64                               `json:"learningRate"`
	WQ           [][FeatureDim][FeatureDim]float64     `json:"-"`
	WK           [][FeatureDim][FeatureDim]float64     `json:"-"`
	WV           [][FeatureDim][FeatureDim]float64     `json:"-"`
	WO           [FeatureDim][FeatureDim]float64       `json:"-"`
	LastQ        [FeatureDim]float64                   `json:"-"`
	LastK        [FeatureDim]float64                   `json:"-"`
	LastV        [FeatureDim]float64                   `json:"-"`
	LastHeads    [][FeatureDim]float64                 `json:"-"`
	LastConcat   [FeatureDim]float64                   `json:"-"`
	LastY        [FeatureDim]float64                   `json:"-"`
}

func NewMultiHeadAttentionLayer(numHeads int) *MultiHeadAttentionLayer {
	if numHeads <= 0 {
		numHeads = 1
	}
	m := &MultiHeadAttentionLayer{
		NumHeads:     numHeads,
		LearningRate:  0.01,
		WQ:           make([][FeatureDim][FeatureDim]float64, numHeads),
		WK:           make([][FeatureDim][FeatureDim]float64, numHeads),
		WV:           make([][FeatureDim][FeatureDim]float64, numHeads),
		LastHeads:    make([][FeatureDim]float64, numHeads),
	}
	for h := 0; h < numHeads; h++ {
		for i := 0; i < FeatureDim; i++ {
			m.WQ[h][i][i] = 1
			m.WK[h][i][i] = 1
			m.WV[h][i][i] = 1
		}
	}
	for i := 0; i < FeatureDim; i++ {
		m.WO[i][i] = 1
	}
	return m
}

func (m *MultiHeadAttentionLayer) Type() ModelType { return ModelType("multi-head-attention") }

func (m *MultiHeadAttentionLayer) Predict(features [FeatureDim]float64) Prediction {
	y := m.Output(features, features, features)
	sum := 0.0
	for i := 0; i < FeatureDim; i++ {
		sum += y[i]
	}
	confidence := 1.0 / (1.0 + math.Exp(-sum/float64(FeatureDim)))
	return Prediction{Action: 0, Confidence: confidence, AnomalyScore: 1 - confidence}
}

func (m *MultiHeadAttentionLayer) headCount() int {
	if m.NumHeads <= 0 {
		return 1
	}
	return m.NumHeads
}

func (m *MultiHeadAttentionLayer) matVec(W [FeatureDim][FeatureDim]float64, x [FeatureDim]float64) [FeatureDim]float64 {
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

func (m *MultiHeadAttentionLayer) softmax1D(x [FeatureDim]float64) [FeatureDim]float64 {
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

func (m *MultiHeadAttentionLayer) Output(featuresQ, featuresK, featuresV [FeatureDim]float64) [FeatureDim]float64 {
	headCount := m.headCount()
	m.LastQ, m.LastK, m.LastV = featuresQ, featuresK, featuresV
	if len(m.WQ) != headCount {
		m.WQ = make([][FeatureDim][FeatureDim]float64, headCount)
		m.WK = make([][FeatureDim][FeatureDim]float64, headCount)
		m.WV = make([][FeatureDim][FeatureDim]float64, headCount)
		m.LastHeads = make([][FeatureDim]float64, headCount)
		for h := 0; h < headCount; h++ {
			for i := 0; i < FeatureDim; i++ {
				m.WQ[h][i][i] = 1
				m.WK[h][i][i] = 1
				m.WV[h][i][i] = 1
			}
		}
	}
	if len(m.LastHeads) != headCount {
		m.LastHeads = make([][FeatureDim]float64, headCount)
	}

	var concat [FeatureDim]float64
	for h := 0; h < headCount; h++ {
		q := m.matVec(m.WQ[h], featuresQ)
		k := m.matVec(m.WK[h], featuresK)
		v := m.matVec(m.WV[h], featuresV)

		var scores [FeatureDim]float64
		scale := math.Sqrt(float64(FeatureDim))
		for i := 0; i < FeatureDim; i++ {
			scores[i] = (q[i] * k[i]) / scale
		}
		a := m.softmax1D(scores)

		var head [FeatureDim]float64
		for i := 0; i < FeatureDim; i++ {
			head[i] = a[i] * v[i]
		}
		m.LastHeads[h] = head
		for i := 0; i < FeatureDim; i++ {
			concat[i] += head[i]
		}
	}
	for i := 0; i < FeatureDim; i++ {
		concat[i] /= float64(headCount)
	}
	m.LastConcat = concat
	m.LastY = m.matVec(m.WO, concat)
	return m.LastY
}

func (m *MultiHeadAttentionLayer) Backward(dY [FeatureDim]float64) {
	if m.LearningRate <= 0 {
		m.LearningRate = 0.01
	}
	var dConcat [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			dConcat[j] += m.WO[i][j] * dY[i]
			m.WO[i][j] -= m.LearningRate * dY[i] * m.LastConcat[j]
		}
	}
	headCount := m.headCount()
	for h := 0; h < headCount; h++ {
		var dHead [FeatureDim]float64
		for i := 0; i < FeatureDim; i++ {
			dHead[i] = dConcat[i] / float64(headCount)
		}
		var dV [FeatureDim]float64
		for i := 0; i < FeatureDim; i++ {
			dV[i] = dHead[i]
		}
		for i := 0; i < FeatureDim; i++ {
			for j := 0; j < FeatureDim; j++ {
				m.WV[h][i][j] -= m.LearningRate * dV[i] * m.LastV[j]
			}
		}
	}
}

func (m *MultiHeadAttentionLayer) Serialize(path string) error {
	data := make([]byte, 0, 4+4+8+3*FeatureDim*FeatureDim*8*maxIntValue(1, m.headCount()))
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
	data = append(data, []byte("MHA1")...)
	putU32(1)
	putU32(uint32(m.headCount()))
	putF64(m.LearningRate)
	for h := 0; h < m.headCount(); h++ {
		for _, W := range [3][FeatureDim][FeatureDim]float64{m.WQ[h], m.WK[h], m.WV[h]} {
			for i := 0; i < FeatureDim; i++ {
				for j := 0; j < FeatureDim; j++ {
					putF64(W[i][j])
				}
			}
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.WO[i][j])
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

func DeserializeMultiHeadAttention(path string) (*MultiHeadAttentionLayer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 16 || string(raw[:4]) != "MHA1" {
		return nil, fmt.Errorf("invalid multi-head attention model file")
	}
	pos := 4
	readU32 := func() uint32 { v := binary.LittleEndian.Uint32(raw[pos:]); pos += 4; return v }
	readF64 := func() float64 { v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:])); pos += 8; return v }
	_ = readU32()
	headCount := int(readU32())
	m := NewMultiHeadAttentionLayer(headCount)
	m.LearningRate = readF64()
	for h := 0; h < headCount; h++ {
		for _, W := range []*[FeatureDim][FeatureDim]float64{&m.WQ[h], &m.WK[h], &m.WV[h]} {
			for i := 0; i < FeatureDim; i++ {
				for j := 0; j < FeatureDim; j++ {
					W[i][j] = readF64()
				}
			}
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.WO[i][j] = readF64()
		}
	}
	return m, nil
}

func maxIntValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}
