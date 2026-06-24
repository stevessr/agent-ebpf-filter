package attention

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// MambaAttention implements a simplified selective state-space model (SSM) attention.
// Mamba uses data-dependent selection mechanisms to choose what information to retain or forget.
// Formula: h_t = (1 - σ(z)) ⊙ h_{t-1} + σ(z) ⊙ f(x_t)
// Where z is the selection gate computed from input.
type MambaAttention struct {
	Wx         [FeatureDim][FeatureDim]float64 `json:"-"` // Input transformation
	Wz         [FeatureDim][FeatureDim]float64 `json:"-"` // Selection gate projection
	Wh         [FeatureDim][FeatureDim]float64 `json:"-"` // Hidden state projection
	Wo         [FeatureDim][FeatureDim]float64 `json:"-"` // Output projection
	Bz         [FeatureDim]float64             `json:"-"` // Selection gate bias
	A          [FeatureDim]float64             `json:"-"` // State transition parameters
	LastH      [FeatureDim]float64             `json:"-"` // Hidden state (retained between calls)
	LastZ      [FeatureDim]float64             `json:"-"` // Selection gate
	LastX      [FeatureDim]float64             `json:"-"` // Transformed input
	LastOutput [FeatureDim]float64             `json:"-"`
}

func NewMambaAttention() *MambaAttention {
	m := &MambaAttention{}

	// Initialize projection matrices as identity
	for i := 0; i < FeatureDim; i++ {
		m.Wx[i][i] = 1.0
		m.Wz[i][i] = 1.0
		m.Wh[i][i] = 1.0
		m.Wo[i][i] = 1.0
		// Initialize state transition parameters (small negative values for stability)
		m.A[i] = -0.1
		m.Bz[i] = 0.0
	}

	return m
}

func (m *MambaAttention) Type() ModelType { return ModelMambaAttention }

func (m *MambaAttention) Forward(input [FeatureDim]float64) [FeatureDim]float64 {
	// Transform input: x' = Wx · x
	for i := 0; i < FeatureDim; i++ {
		m.LastX[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastX[i] += m.Wx[i][j] * input[j]
		}
	}

	// Compute selection gate: z = σ(Wz · x + Bz)
	for i := 0; i < FeatureDim; i++ {
		zPre := m.Bz[i]
		for j := 0; j < FeatureDim; j++ {
			zPre += m.Wz[i][j] * input[j]
		}
		m.LastZ[i] = 1.0 / (1.0 + math.Exp(-zPre)) // sigmoid
	}

	// Apply activation to transformed input: f(x') = tanh(x')
	var activatedX [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		activatedX[i] = math.Tanh(m.LastX[i])
	}

	// Update hidden state with selective mechanism:
	// h_t = (1 - z) ⊙ (A ⊙ h_{t-1}) + z ⊙ f(x')
	// This allows the model to selectively forget old state and incorporate new input
	var newH [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		// State transition with A parameter (exponential decay)
		decayedH := math.Exp(m.A[i]) * m.LastH[i]
		// Selective update
		newH[i] = (1.0-m.LastZ[i])*decayedH + m.LastZ[i]*activatedX[i]
	}
	m.LastH = newH

	// Project hidden state to output
	for i := 0; i < FeatureDim; i++ {
		m.LastOutput[i] = 0.0
		for j := 0; j < FeatureDim; j++ {
			m.LastOutput[i] += m.Wo[i][j] * m.LastH[j]
		}
	}

	return m.LastOutput
}

func (m *MambaAttention) Backward(x, gradOutput [FeatureDim]float64) [FeatureDim]float64 {
	// Simplified gradient: pass through with output projection transpose
	var gradInput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			gradInput[i] += m.Wo[j][i] * gradOutput[j]
		}
	}
	return gradInput
}

func (m *MambaAttention) Reset() {
	// Reset hidden state (useful for new sequences)
	for i := 0; i < FeatureDim; i++ {
		m.LastH[i] = 0.0
	}
}

func (m *MambaAttention) Serialize(path string) error {
	data := make([]byte, 0, 8+FeatureDim*FeatureDim*8*4+FeatureDim*8*3)
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

	data = append(data, []byte("MABA")...)
	putU32(1) // version

	// Serialize Wx, Wz, Wh, Wo
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wx[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wz[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wh[i][j])
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			putF64(m.Wo[i][j])
		}
	}

	// Serialize Bz and A
	for i := 0; i < FeatureDim; i++ {
		putF64(m.Bz[i])
	}
	for i := 0; i < FeatureDim; i++ {
		putF64(m.A[i])
	}

	// Note: We do NOT serialize LastH (hidden state) as it's transient per-sequence

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

func DeserializeMambaAttention(path string) (*MambaAttention, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) < 8 || string(raw[:4]) != "MABA" {
		return nil, fmt.Errorf("invalid Mamba attention model file")
	}

	pos := 4
	readU32 := func() uint32 { v := binary.LittleEndian.Uint32(raw[pos:]); pos += 4; return v }
	readF64 := func() float64 { v := math.Float64frombits(binary.LittleEndian.Uint64(raw[pos:])); pos += 8; return v }
	_ = readU32() // version

	m := NewMambaAttention()
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wx[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wz[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wh[i][j] = readF64()
		}
	}
	for i := 0; i < FeatureDim; i++ {
		for j := 0; j < FeatureDim; j++ {
			m.Wo[i][j] = readF64()
		}
	}

	for i := 0; i < FeatureDim; i++ {
		m.Bz[i] = readF64()
	}
	for i := 0; i < FeatureDim; i++ {
		m.A[i] = readF64()
	}

	return m, nil
}
