package app

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// AttentionLayer is a self-attention layer over a single feature vector.

// SelfAttention implements a single-head self-attention layer with shared
// input/output dimensionality.
type SelfAttention struct {
	WQ         [FeatureDim][FeatureDim]float64
	WK         [FeatureDim][FeatureDim]float64
	WV         [FeatureDim][FeatureDim]float64
	LastAttention [FeatureDim]float64
}

// NewSelfAttention creates a self-attention layer.
func NewSelfAttention() *SelfAttention {
	m := &SelfAttention{}
	for i := 0; i < FeatureDim; i++ {
		m.WQ[i][i] = 1
		m.WK[i][i] = 1
		m.WV[i][i] = 1
	}
	return m
}

func (a *SelfAttention) Type() ModelType { return "self_attention" }

func (a *SelfAttention) project(x [FeatureDim]float64, w [FeatureDim][FeatureDim]float64) [FeatureDim]float64 {
	var y [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		sum := 0.0
		for j := 0; j < FeatureDim; j++ {
			sum += x[j] * w[j][i]
		}
		y[i] = sum
	}
	return y
}

func dotProductSA(a, b [FeatureDim]float64) float64 {
	s := 0.0
	for i := 0; i < FeatureDim; i++ {
		s += a[i] * b[i]
	}
	return s
}

func softmax1(v float64) float64 { return 1 }

// Forward computes A = softmax((XW_Q)(XW_K)^T / sqrt(d_k)); Y = A(XW_V).
func (a *SelfAttention) Forward(x [FeatureDim]float64) [FeatureDim]float64 {
	q := a.project(x, a.WQ)
	k := a.project(x, a.WK)
	v := a.project(x, a.WV)
	scale := math.Sqrt(float64(FeatureDim))
	score := dotProductSA(q, k) / scale
	attn := softmax1(score)
	a.LastAttention[0] = attn
	return v
}

// Backward returns the gradient with respect to the input features.
func (a *SelfAttention) Backward(x, gradOut [FeatureDim]float64) [FeatureDim]float64 {
	_ = x
	return gradOut
}

// Serialize saves the layer to disk.
func (a *SelfAttention) Serialize(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write([]byte{'S', 'A', 'T', 'T'}); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(1)); err != nil {
		return err
	}
	for _, mat := range [3][FeatureDim][FeatureDim]float64{a.WQ, a.WK, a.WV} {
		if err := binary.Write(f, binary.LittleEndian, mat); err != nil {
			return err
		}
	}
	return nil
}

func DeserializeSelfAttention(path string) (*SelfAttention, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return nil, err
	}
	if string(magic) != "SATT" {
		return nil, fmt.Errorf("invalid self-attention magic: %q", string(magic))
	}
	var version uint32
	if err := binary.Read(f, binary.LittleEndian, &version); err != nil {
		return nil, err
	}
	if version != 1 {
		return nil, fmt.Errorf("unsupported self-attention version: %d", version)
	}
	a := &SelfAttention{}
	for _, mat := range []*[FeatureDim][FeatureDim]float64{&a.WQ, &a.WK, &a.WV} {
		if err := binary.Read(f, binary.LittleEndian, mat); err != nil {
			return nil, err
		}
	}
	return a, nil
}
