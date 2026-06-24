package attention

import (
	"math"
	"path/filepath"
	"testing"
)

func TestSelfAttentionForward(t *testing.T) {
	attn := NewSelfAttention()
	var x [FeatureDim]float64
	x[0] = 2
	x[1] = 3

	var out [FeatureDim]float64
	out = attn.Forward(x)

	if out[0] != x[0] || out[1] != x[1] {
		t.Fatalf("expected identity output, got [%v %v]", out[0], out[1])
	}

	score := (x[0]*x[0] + x[1]*x[1]) / math.Sqrt(float64(FeatureDim))
	if attn.LastAttention[0] != 1 || score <= 0 {
		t.Fatalf("expected attention weight 1, got %v", attn.LastAttention[0])
	}
}

func TestSelfAttentionBackward(t *testing.T) {
	attn := NewSelfAttention()
	var x, grad [FeatureDim]float64
	x[0] = 1
	grad[0] = 7
	grad[1] = -2

	got := attn.Backward(x, grad)
	if got[0] != grad[0] || got[1] != grad[1] {
		t.Fatalf("backward should pass gradients through in the identity setup, got [%v %v]", got[0], got[1])
	}
}

func TestSelfAttentionSerializeRoundtrip(t *testing.T) {
	attn := NewSelfAttention()
	attn.WQ[0][0] = 2.5
	attn.WK[1][1] = 3.5
	attn.WV[2][2] = 4.5

	path := filepath.Join(t.TempDir(), "attention.bin")
	if err := attn.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeSelfAttention(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.WQ[0][0] != 2.5 || loaded.WK[1][1] != 3.5 || loaded.WV[2][2] != 4.5 {
		t.Fatalf("roundtrip mismatch: %+v", loaded)
	}
}
