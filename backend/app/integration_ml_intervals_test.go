package app

import (
	"math"
	"testing"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/pb"
)

func mlActionIntervalConfig() MLConfig {
	cfg := DefaultMLConfig()
	cfg.Enabled = true
	cfg.MlMinConfidence = 0.6
	cfg.AllowRiskScoreThreshold = 0.3
	cfg.BlockConfidenceThreshold = 0.8
	cfg.LowAnomalyThreshold = 0.3
	cfg.HighAnomalyThreshold = 0.7
	return cfg
}

func TestMLActionRiskScoreMapsConfidenceToRisk(t *testing.T) {
	cases := []struct {
		name       string
		action     int32
		confidence float64
		want       float64
	}{
		{"allow full confidence", 0, 1, 0},
		{"allow moderate confidence", 0, 0.8, 0.2},
		{"allow weak confidence", 0, 0.5, 0.5},
		{"block confidence is risk", 1, 0.9, 0.9},
		{"block moderate confidence", 1, 0.65, 0.65},
		{"alert is neutral", 3, 0.95, 0.5},
		{"rewrite is neutral", 2, 0.7, 0.5},
		{"unknown action is neutral", 9, 0.9, 0.5},
		{"negative confidence clamps to zero risk", 1, -0.4, 0},
		{"negative confidence clamps for allow", 0, -0.4, 1},
		{"confidence above one clamps", 1, 1.7, 1},
		{"nan confidence is neutral", 1, math.NaN(), 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mlActionRiskScore(tc.action, tc.confidence)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("mlActionRiskScore(%d, %v) = %v, want %v", tc.action, tc.confidence, got, tc.want)
			}
		})
	}
}

func TestResolveActionIntervalBoundaries(t *testing.T) {
	cfg := mlActionIntervalConfig()
	cfg.MlMinConfidence = 0.3
	// ALLOW interval: risk = 1 - confidence, so confidence 0.75 → risk 0.25.
	if action, reason := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 0, Confidence: 0.75}, cfg, true, true); action != pb.WrapperResponse_ALLOW {
		t.Fatalf("low-risk ALLOW got %s (%s), want ALLOW", ml.ActionLabel[int32(action)], reason)
	}
	// confidence 0.60 → risk 0.40 > allow threshold → ALERT.
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 0, Confidence: 0.60}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("mid-risk ALLOW got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
	// BLOCK interval: confidence 0.85 → risk 0.85 ≥ 0.8 → BLOCK.
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 1, Confidence: 0.85}, cfg, true, true); action != pb.WrapperResponse_BLOCK {
		t.Fatalf("high-risk BLOCK got %s, want BLOCK", ml.ActionLabel[int32(action)])
	}
	// confidence 0.70 → risk 0.70 < 0.8 → ALERT, never ALLOW.
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 1, Confidence: 0.70}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("weak BLOCK got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
}

func TestResolveActionNeverDowngradesBlockToAllow(t *testing.T) {
	cfg := mlActionIntervalConfig()
	// Keep the decision above the min-confidence gate so the interval logic,
	// not the fallback, decides the outcome.
	cfg.MlMinConfidence = 0.3
	// Even with a wide allow interval, a BLOCK prediction cannot become ALLOW:
	// risk 0.5 sits below the block boundary but above any allow boundary.
	cfg.AllowRiskScoreThreshold = 0.49
	if action, reason := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 1, Confidence: 0.5}, cfg, true, true); action == pb.WrapperResponse_ALLOW {
		t.Fatalf("weak BLOCK downgraded to ALLOW (%s)", reason)
	}
	// Malformed model action must stay on ALERT, not fall through to rule ALLOW.
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 42, Confidence: 0.9}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("unknown action got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
	// Non-finite confidence must not bypass the intervals.
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 0, Confidence: math.NaN()}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("non-finite ALLOW prediction got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 1, Confidence: math.Inf(1)}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("infinite confidence got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
	if action, _ := resolveAction(&pb.WrapperRequest{}, "", 0, nil, 0,
		ml.Prediction{Action: 0, Confidence: 0.2}, cfg, true, true); action != pb.WrapperResponse_ALERT {
		t.Fatalf("low-confidence model prediction got %s, want ALERT", ml.ActionLabel[int32(action)])
	}
}
