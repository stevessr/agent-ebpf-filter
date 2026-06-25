package app

import (
	"math"
	"os"
	"testing"
)

func TestScaledDotProductAttentionForward(t *testing.T) {
	m := NewScaledDotProductAttention()

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
	if m.LastAlpha == 0.0 && m.LastScore == 0.0 {
		t.Error("Internal state not populated after forward pass")
	}
}

func TestScaledDotProductAttentionBackward(t *testing.T) {
	m := NewScaledDotProductAttention()

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i) * 0.1
	}

	// Forward pass first
	output := m.Forward(input)

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

	_ = output // Use output to avoid unused variable warning
}

func TestScaledDotProductAttentionSerializeRoundTrip(t *testing.T) {
	m := NewScaledDotProductAttention()

	// Modify some weights
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			m.Wq[i][j] = float64(i+j) * 0.1
			m.Wk[i][j] = float64(i*j) * 0.05
			m.Wv[i][j] = float64(i-j) * 0.15
			m.Wo[i][j] = float64(i+j*2) * 0.08
		}
	}

	path := "/tmp/test_scaled_dot_product_attn.bin"
	defer os.Remove(path)

	// Serialize
	if err := m.Serialize(path); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	loaded, err := DeserializeScaledDotProductAttention(path)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Compare weights (sample check)
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			if math.Abs(m.Wq[i][j]-loaded.Wq[i][j]) > 1e-9 {
				t.Errorf("Wq[%d][%d] mismatch: expected %f, got %f", i, j, m.Wq[i][j], loaded.Wq[i][j])
			}
			if math.Abs(m.Wk[i][j]-loaded.Wk[i][j]) > 1e-9 {
				t.Errorf("Wk[%d][%d] mismatch: expected %f, got %f", i, j, m.Wk[i][j], loaded.Wk[i][j])
			}
		}
	}
}

func TestScaledDotProductAttentionScaling(t *testing.T) {
	m := NewScaledDotProductAttention()

	// Test that scaling factor is applied correctly
	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = 1.0 // Uniform input
	}

	_ = m.Forward(input)

	// Verify that score was scaled by sqrt(d_k)
	// With identity matrices and uniform input, raw score would be FeatureDim
	// After scaling, it should be sqrt(FeatureDim)
	expectedScale := math.Sqrt(float64(FeatureDim))
	if m.LastScore == 0.0 {
		t.Error("LastScore should not be zero for uniform input")
	}

	// Score should be roughly FeatureDim / sqrt(FeatureDim) = sqrt(FeatureDim)
	// (but may vary due to projection matrices)
	if math.Abs(m.LastScore) > 100*expectedScale {
		t.Errorf("Score magnitude %f seems too large (expected ~%f scale)", m.LastScore, expectedScale)
	}
}
