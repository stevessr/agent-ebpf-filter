package app

import (
	"math"
	"os"
	"testing"
)

func TestRWKVAttentionForward(t *testing.T) {
	m := NewRWKVAttention()

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

	// Verify internal state is populated
	hasNonZeroR := false
	hasNonZeroK := false
	hasNonZeroV := false
	for i := 0; i < FeatureDim; i++ {
		if m.LastR[i] != 0.0 {
			hasNonZeroR = true
		}
		if m.LastK[i] != 0.0 {
			hasNonZeroK = true
		}
		if m.LastV[i] != 0.0 {
			hasNonZeroV = true
		}
	}
	if !hasNonZeroR || !hasNonZeroK || !hasNonZeroV {
		t.Error("Internal projections (R/K/V) are not properly computed")
	}
}

func TestRWKVAttentionLinearComplexity(t *testing.T) {
	// RWKV should use element-wise operations, not quadratic attention
	m := NewRWKVAttention()

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = math.Sin(float64(i) * 0.5)
	}

	// Forward pass
	output := m.Forward(input)

	// Verify W + K computation (time-mixing)
	for i := 0; i < FeatureDim; i++ {
		expected := m.W[i] + m.LastK[i]
		if math.Abs(m.LastWK[i]-expected) > 1e-9 {
			t.Errorf("W+K[%d] mismatch: expected %f, got %f", i, expected, m.LastWK[i])
		}
	}

	_ = output
}

func TestRWKVAttentionReceptanceGating(t *testing.T) {
	m := NewRWKVAttention()

	// Test with different inputs to verify receptance gating
	var input1 [FeatureDim]float64
	var input2 [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input1[i] = 1.0
		input2[i] = -1.0
	}

	output1 := m.Forward(input1)
	output2 := m.Forward(input2)

	// Outputs should be different due to different receptance gating
	same := true
	for i := 0; i < FeatureDim; i++ {
		if math.Abs(output1[i]-output2[i]) > 1e-6 {
			same = false
			break
		}
	}
	if same {
		t.Error("Different inputs produced identical outputs")
	}
}

func TestRWKVAttentionBackward(t *testing.T) {
	m := NewRWKVAttention()

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

func TestRWKVAttentionSerializeRoundTrip(t *testing.T) {
	m := NewRWKVAttention()

	// Modify some weights and time-mixing parameters
	for i := 0; i < 3 && i < FeatureDim; i++ {
		m.W[i] = float64(i) * 0.1
		for j := 0; j < 3 && j < FeatureDim; j++ {
			m.Wr[i][j] = float64(i+j) * 0.1
			m.Wk[i][j] = float64(i*j) * 0.05
			m.Wv[i][j] = float64(i-j) * 0.15
			m.Wo[i][j] = float64(i+j*2) * 0.08
		}
	}

	path := "/tmp/test_rwkv_attn.bin"
	defer os.Remove(path)

	// Serialize
	if err := m.Serialize(path); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	loaded, err := DeserializeRWKVAttention(path)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Compare time-mixing weights
	for i := 0; i < 3 && i < FeatureDim; i++ {
		if math.Abs(m.W[i]-loaded.W[i]) > 1e-9 {
			t.Errorf("W[%d] mismatch: expected %f, got %f", i, m.W[i], loaded.W[i])
		}
	}

	// Compare projection matrices (sample check)
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			if math.Abs(m.Wr[i][j]-loaded.Wr[i][j]) > 1e-9 {
				t.Errorf("Wr[%d][%d] mismatch: expected %f, got %f", i, j, m.Wr[i][j], loaded.Wr[i][j])
			}
		}
	}
}

func TestRWKVAttentionNumericalStability(t *testing.T) {
	m := NewRWKVAttention()

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
}

func TestRWKVAttentionTimeMixing(t *testing.T) {
	m := NewRWKVAttention()

	// Set non-zero time-mixing weights
	for i := 0; i < FeatureDim; i++ {
		m.W[i] = float64(i) * 0.01
	}

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}

	_ = m.Forward(input)

	// Verify that W+K includes the time-mixing weights
	for i := 0; i < FeatureDim; i++ {
		if math.Abs(m.LastWK[i]-(m.W[i]+m.LastK[i])) > 1e-9 {
			t.Errorf("Time-mixing not applied correctly at index %d", i)
		}
	}
}
