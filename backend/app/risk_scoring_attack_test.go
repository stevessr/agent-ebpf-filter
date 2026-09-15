package app

import (
	"testing"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/pb"
)

func TestAttackVectorScoresPrioritizeDestruction(t *testing.T) {
	classification := &pb.BehaviorClassification{
		PrimaryCategory: "FILE_DELETE",
		Confidence:      "high",
	}
	scores := computeAttackVectorScores(classification, 0.25, ml.Prediction{}, NetworkAuditResult{}, nil)
	if scores.Destruction <= scores.Intrusion || scores.Destruction <= scores.Exfiltration {
		t.Fatalf("FILE_DELETE should be destruction-led: %+v", scores)
	}
	if scores.Global < 80 || scores.Class != "ATTACK" {
		t.Fatalf("destructive high-confidence behavior should be attack-class: %+v", scores)
	}
}

func TestAttackVectorScoresTreatReverseShellAsStrongIntrusionEvidence(t *testing.T) {
	network := NetworkAuditResult{
		Flags: NetworkAuditFlags{ReverseShell: true},
	}
	scores := computeAttackVectorScores(nil, 0.10, ml.Prediction{}, network, nil)
	if scores.Intrusion < 95 {
		t.Fatalf("reverse shell must strongly raise intrusion score: %+v", scores)
	}
	if scores.Persistence < 60 {
		t.Fatalf("reverse shell should also carry persistence context: %+v", scores)
	}
	if scores.Class != "ATTACK" {
		t.Fatalf("reverse shell should be attack-class: %+v", scores)
	}
}

func TestAttackVectorScoresDoNotInventVectorFromAnomalyOnly(t *testing.T) {
	scores := computeAttackVectorScores(nil, 1.0, ml.Prediction{}, NetworkAuditResult{}, nil)
	if scores.Intrusion != 0 || scores.Destruction != 0 || scores.Exfiltration != 0 || scores.Persistence != 0 {
		t.Fatalf("anomaly-only evidence must remain a global prior: %+v", scores)
	}
	if scores.Global <= 0 || scores.Global >= 40 {
		t.Fatalf("anomaly-only evidence should be suspicious context, not a fabricated attack vector: %+v", scores)
	}
}

func TestAttackRiskClassBoundaries(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "CLEAN"},
		{20, "LIKELY_CLEAN"},
		{40, "SUSPICIOUS"},
		{60, "LIKELY_ATTACK"},
		{80, "ATTACK"},
	}
	for _, tc := range cases {
		if got := attackRiskClass(tc.score); got != tc.want {
			t.Fatalf("attackRiskClass(%v)=%q want %q", tc.score, got, tc.want)
		}
	}
}
