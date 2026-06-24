package attention

import (
	"math"
	"os"
	"testing"
)

func TestMambaAttentionForward(t *testing.T) {
	m := NewMambaAttention()

	// Create test input
	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}

	// Forward pass
	output := m.Forward(input)

	// Verify output shape
	if len(output) != FeatureDim {
		t.Errorf("Expected output dimension %d, got %d", FeatureDim, len(output))
	}

	// Verify output is not all zeros
	allZero := true
	for i := 0; i < FeatureDim; i++ {
		if output[i] != 0.0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Output is all zeros")
	}

	// Verify hidden state is updated
	hasNonZeroH := false
	for i := 0; i < FeatureDim; i++ {
		if m.LastH[i] != 0.0 {
			hasNonZeroH = true
			break
		}
	}
	if !hasNonZeroH {
		t.Error("Hidden state was not updated")
	}
}

func TestMambaAttentionSelectiveGating(t *testing.T) {
	m := NewMambaAttention()

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}

	// Forward pass
	_ = m.Forward(input)

	// Verify selection gates are in [0, 1] range (sigmoid output)
	for i := 0; i < FeatureDim; i++ {
		if m.LastZ[i] < 0.0 || m.LastZ[i] > 1.0 {
			t.Errorf("Selection gate Z[%d] = %f is out of [0,1] range", i, m.LastZ[i])
		}
	}
}

func TestMambaAttentionStateRetention(t *testing.T) {
	m := NewMambaAttention()

	// First input
	var input1 [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input1[i] = 1.0
	}
	_ = m.Forward(input1)

	// Save hidden state after first forward
	var savedH [FeatureDim]float64
	copy(savedH[:], m.LastH[:])

	// Second input - hidden state should influence output
	var input2 [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input2[i] = 0.5
	}
	_ = m.Forward(input2)

	// Verify hidden state was used (should be different from zero initialization)
	stateUsed := false
	for i := 0; i < FeatureDim; i++ {
		if math.Abs(savedH[i]) > 1e-6 {
			stateUsed = true
			break
		}
	}
	if !stateUsed {
		t.Error("Hidden state does not appear to retain information across forward passes")
	}
}

func TestMambaAttentionReset(t *testing.T) {
	m := NewMambaAttention()

	// Create some state
	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}
	_ = m.Forward(input)

	// Verify state is non-zero
	hasState := false
	for i := 0; i < FeatureDim; i++ {
		if m.LastH[i] != 0.0 {
			hasState = true
			break
		}
	}
	if !hasState {
		t.Error("Expected non-zero state before reset")
	}

	// Reset
	m.Reset()

	// Verify state is zero after reset
	for i := 0; i < FeatureDim; i++ {
		if m.LastH[i] != 0.0 {
			t.Errorf("Hidden state H[%d] = %f is not zero after reset", i, m.LastH[i])
		}
	}
}

func TestMambaAttentionBackward(t *testing.T) {
	m := NewMambaAttention()

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}

	// Forward pass first
	_ = m.Forward(input)

	// Create gradient
	var gradOutput [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		gradOutput[i] = 0.01
	}

	// Backward pass
	gradInput := m.Backward(input, gradOutput)

	// Verify gradient shape
	if len(gradInput) != FeatureDim {
		t.Errorf("Expected gradient dimension %d, got %d", FeatureDim, len(gradInput))
	}

	// Verify gradient is not all zeros
	allZero := true
	for i := 0; i < FeatureDim; i++ {
		if gradInput[i] != 0.0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Gradient is all zeros")
	}
}

func TestMambaAttentionSerializeRoundTrip(t *testing.T) {
	m := NewMambaAttention()

	// Modify some weights
	for i := 0; i < 3 && i < FeatureDim; i++ {
		m.A[i] = -float64(i) * 0.05
		m.Bz[i] = float64(i) * 0.1
		for j := 0; j < 3 && j < FeatureDim; j++ {
			m.Wx[i][j] = float64(i+j) * 0.1
			m.Wz[i][j] = float64(i*j) * 0.05
			m.Wh[i][j] = float64(i-j) * 0.15
			m.Wo[i][j] = float64(i+j*2) * 0.08
		}
	}

	path := "/tmp/test_mamba_attn.bin"
	defer os.Remove(path)

	// Serialize
	if err := m.Serialize(path); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	loaded, err := DeserializeMambaAttention(path)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Compare state transition parameters
	for i := 0; i < 3 && i < FeatureDim; i++ {
		if math.Abs(m.A[i]-loaded.A[i]) > 1e-9 {
			t.Errorf("A[%d] mismatch: expected %f, got %f", i, m.A[i], loaded.A[i])
		}
		if math.Abs(m.Bz[i]-loaded.Bz[i]) > 1e-9 {
			t.Errorf("Bz[%d] mismatch: expected %f, got %f", i, m.Bz[i], loaded.Bz[i])
		}
	}

	// Compare projection matrices (sample check)
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			if math.Abs(m.Wx[i][j]-loaded.Wx[i][j]) > 1e-9 {
				t.Errorf("Wx[%d][%d] mismatch: expected %f, got %f", i, j, m.Wx[i][j], loaded.Wx[i][j])
			}
		}
	}

	// Note: LastH (hidden state) should NOT be serialized, so loaded should have zero state
	for i := 0; i < FeatureDim; i++ {
		if loaded.LastH[i] != 0.0 {
			t.Errorf("Loaded model should have zero hidden state, but H[%d] = %f", i, loaded.LastH[i])
		}
	}
}

func TestMambaAttentionStateTransition(t *testing.T) {
	m := NewMambaAttention()

	// Set specific A values for testing decay
	for i := 0; i < FeatureDim; i++ {
		m.A[i] = -0.1 // Small negative value for decay
	}

	// Initialize with some state
	for i := 0; i < FeatureDim; i++ {
		m.LastH[i] = 1.0
	}

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = 0.0 // Zero input to observe state decay
	}

	_ = m.Forward(input)

	// With A = -0.1, exp(A) ≈ 0.905, so state should decay but not to zero
	for i := 0; i < FeatureDim; i++ {
		// State should be less than 1.0 but greater than 0
		if m.LastH[i] >= 1.0 {
			t.Errorf("State H[%d] = %f did not decay (should be < 1.0)", i, m.LastH[i])
		}
	}
}

func TestMambaAttentionNumericalStability(t *testing.T) {
	m := NewMambaAttention()

	// Test with extreme values
	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = 100.0 // Large values
	}

	output := m.Forward(input)

	// Check for NaN or Inf
	for i := 0; i < FeatureDim; i++ {
		if math.IsNaN(output[i]) {
			t.Errorf("Output[%d] is NaN", i)
		}
		if math.IsInf(output[i], 0) {
			t.Errorf("Output[%d] is Inf", i)
		}
	}

	// Check hidden state too
	for i := 0; i < FeatureDim; i++ {
		if math.IsNaN(m.LastH[i]) {
			t.Errorf("Hidden state H[%d] is NaN", i)
		}
		if math.IsInf(m.LastH[i], 0) {
			t.Errorf("Hidden state H[%d] is Inf", i)
		}
	}
}
