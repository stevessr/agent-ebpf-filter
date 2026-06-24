package attention

import (
	"math"
	"os"
	"testing"
)

func TestMultiHeadAttentionForward(t *testing.T) {
	m := NewMultiHeadAttention(4)

	// Verify head configuration
	if m.NumHeads != 4 {
		t.Errorf("Expected 4 heads, got %d", m.NumHeads)
	}
	if m.HeadDim != FeatureDim/4 {
		t.Errorf("Expected head dimension %d, got %d", FeatureDim/4, m.HeadDim)
	}

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

	// Verify per-head scores and alphas are populated
	if len(m.LastScores) != m.NumHeads {
		t.Errorf("Expected %d scores, got %d", m.NumHeads, len(m.LastScores))
	}
	if len(m.LastAlphas) != m.NumHeads {
		t.Errorf("Expected %d alphas, got %d", m.NumHeads, len(m.LastAlphas))
	}

	// Verify at least one head has non-zero attention
	hasNonZero := false
	for h := 0; h < m.NumHeads; h++ {
		if m.LastAlphas[h] != 0.0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("All head attentions are zero")
	}
}

func TestMultiHeadAttentionMultipleHeads(t *testing.T) {
	// Test different head counts
	headCounts := []int{2, 4, 8}

	for _, numHeads := range headCounts {
		if FeatureDim%numHeads != 0 {
			continue // Skip if not divisible
		}

		m := NewMultiHeadAttention(numHeads)
		if m.NumHeads != numHeads {
			t.Errorf("Expected %d heads, got %d", numHeads, m.NumHeads)
		}

		expectedHeadDim := FeatureDim / numHeads
		if m.HeadDim != expectedHeadDim {
			t.Errorf("For %d heads: expected head dim %d, got %d", numHeads, expectedHeadDim, m.HeadDim)
		}

		var input [FeatureDim]float64
		for i := 0; i < FeatureDim; i++ {
			input[i] = float64(i) * 0.05
		}

		output := m.Forward(input)
		if len(output) != FeatureDim {
			t.Errorf("For %d heads: expected output dim %d, got %d", numHeads, FeatureDim, len(output))
		}
	}
}

func TestMultiHeadAttentionBackward(t *testing.T) {
	m := NewMultiHeadAttention(4)

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

func TestMultiHeadAttentionSerializeRoundTrip(t *testing.T) {
	m := NewMultiHeadAttention(4)

	// Modify some weights
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			m.Wq[i][j] = float64(i+j) * 0.1
			m.Wk[i][j] = float64(i*j) * 0.05
		}
	}

	path := "/tmp/test_multi_head_attn.bin"
	defer os.Remove(path)

	// Serialize
	if err := m.Serialize(path); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize
	loaded, err := DeserializeMultiHeadAttention(path)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Verify configuration
	if loaded.NumHeads != m.NumHeads {
		t.Errorf("NumHeads mismatch: expected %d, got %d", m.NumHeads, loaded.NumHeads)
	}
	if loaded.HeadDim != m.HeadDim {
		t.Errorf("HeadDim mismatch: expected %d, got %d", m.HeadDim, loaded.HeadDim)
	}

	// Compare weights (sample check)
	for i := 0; i < 3 && i < FeatureDim; i++ {
		for j := 0; j < 3 && j < FeatureDim; j++ {
			if math.Abs(m.Wq[i][j]-loaded.Wq[i][j]) > 1e-9 {
				t.Errorf("Wq[%d][%d] mismatch: expected %f, got %f", i, j, m.Wq[i][j], loaded.Wq[i][j])
			}
		}
	}
}

func TestMultiHeadAttentionInvalidHeadCount(t *testing.T) {
	// Test that invalid head count falls back to default
	m := NewMultiHeadAttention(0)
	if m.NumHeads != 4 {
		t.Errorf("Expected fallback to 4 heads for invalid input, got %d", m.NumHeads)
	}

	// Test non-divisible head count
	m = NewMultiHeadAttention(7) // Assuming FeatureDim is not divisible by 7
	if FeatureDim%7 != 0 {
		if m.NumHeads != 4 {
			t.Errorf("Expected fallback to 4 heads for non-divisible input, got %d", m.NumHeads)
		}
	}
}

func TestMultiHeadAttentionPerHeadIndependence(t *testing.T) {
	m := NewMultiHeadAttention(4)

	var input [FeatureDim]float64
	for i := 0; i < FeatureDim; i++ {
		input[i] = float64(i%m.HeadDim) * 0.1 // Create pattern per head
	}

	_ = m.Forward(input)

	// Verify that each head computed a score
	for h := 0; h < m.NumHeads; h++ {
		if m.LastScores[h] == 0.0 && m.LastAlphas[h] == 0.0 {
			t.Errorf("Head %d has zero score and alpha", h)
		}
	}
}
