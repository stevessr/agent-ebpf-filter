package kernelguard

import (
	"net/netip"
	"testing"
)

func sampleModel() Model {
	return Model{SchemaVersion: SchemaVersion, Generation: 1, Bias: -200,
		Threshold: 300, Weights: [6]int32{40, 600, 700, 100, 30, 0}}
}

func TestModelValidationAndSafeDefault(t *testing.T) {
	m := sampleModel()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if !Predict(m, netip.MustParseAddr("::ffff:8.8.8.8"), 4444) {
		t.Fatal("IPv4-mapped IPv6 must share the IPv4 scorer")
	}
	if !ExternalIPv4Candidate(netip.MustParseAddr("::ffff:8.8.8.8")) {
		t.Fatal("mapped IPv4 should remain in scope")
	}
	if !Predict(m, netip.MustParseAddr("8.8.8.8"), 4444) {
		t.Fatal("scoped high-risk port should match")
	}
	for _, test := range []struct {
		ip   string
		port uint16
	}{
		{"127.0.0.1", 4444}, {"10.1.2.3", 4444},
		{"192.168.1.1", 4444}, {"172.20.1.1", 22},
		{"198.51.100.1", 4444}, {"2001:4860:4860::8888", 4444},
		{"8.8.8.8", 443}, {"8.8.8.8", 80}, {"8.8.8.8", 0},
	} {
		if Predict(m, netip.MustParseAddr(test.ip), test.port) {
			t.Errorf("unexpected match: %s:%d", test.ip, test.port)
		}
	}
	m.Weights = [6]int32{1000}
	if err := m.Validate(); err == nil {
		t.Fatal("generic-only model accepted")
	}
	m = sampleModel()
	m.Weights[2] = 4000
	if err := m.Validate(); err == nil {
		t.Fatal("unbounded weight accepted")
	}
}

func TestMapConfigFixedWidthABI(t *testing.T) {
	m := sampleModel()
	cfg := m.Config(ModeEnforce)
	if cfg.Version != 1 || cfg.Generation != m.Generation || cfg.Mode != 2 || cfg.Weights != m.Weights {
		t.Fatalf("broken map ABI: %+v", cfg)
	}
}

func TestHoldoutReleaseGate(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h := Holdout{SchemaVersion: "kernel-guard-holdout-v2", ModelSHA256: hash,
		Benign: 10000, Attacks: 500, TruePositives: 499, FalsePositives: 0,
		IndependentGroups: 25, BenignGroups: 20, AttackGroups: 20}
	if err := AssessHoldout(h, hash); err != nil {
		t.Fatal(err)
	}
	h.FalsePositives = 30
	if err := AssessHoldout(h, hash); err == nil {
		t.Fatal("high false positive rate accepted")
	}
	h.FalsePositives = 0
	h.TruePositives = 440
	if err := AssessHoldout(h, hash); err == nil {
		t.Fatal("low recall accepted")
	}
	h.TruePositives = 499
	h.IndependentGroups = 1
	if err := AssessHoldout(h, hash); err == nil {
		t.Fatal("insufficient run groups accepted")
	}
}

func TestABISize(t *testing.T) {
	// The kernel guard_config size is 44 bytes (five 32-bit header fields and
	// six 32-bit weights); C/Go struct ordering is intentionally identical.
	const want = 44
	if got := 4*5 + 4*FeatureCount; got != want {
		t.Fatalf("ABI changed: %d", got)
	}
}
func TestWilsonBoundsRejectLuckySmallHoldout(t *testing.T) {
	_, smallFPRUpper := WilsonBounds95(0, 1000)
	if smallFPRUpper <= 0.001 {
		t.Fatalf("1000 benign with zero FP insufficient: %.6f", smallFPRUpper)
	}
	_, largeFPRUpper := WilsonBounds95(0, 10000)
	if largeFPRUpper >= 0.001 {
		t.Fatalf("10000 benign with zero FP should pass: %.6f", largeFPRUpper)
	}
	smallRecallLower, _ := WilsonBounds95(95, 100)
	if smallRecallLower >= 0.95 {
		t.Fatalf("95/100 recall should fail lower confidence bound: %.6f", smallRecallLower)
	}
}
