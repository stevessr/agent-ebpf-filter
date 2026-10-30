package kernelguard

import (
	"fmt"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopeEvidenceGate(t *testing.T) {
	good := ScopeEvidence{Port: 4444, MatchedAttacks: 30, MatchedAttackRun: 11}
	if err := AssessScopeEvidence(good); err != nil {
		t.Fatal(err)
	}
	for _, x := range []ScopeEvidence{
		{Port: 4444, MatchedAttacks: 30, MatchedAttackRun: 11, MatchedBenign: 1},
		{Port: 4444, MatchedAttacks: 100, MatchedAttackRun: 1},
		{Port: 4444, MatchedAttacks: 10, MatchedAttackRun: 10},
		{Port: 0, MatchedAttacks: 30, MatchedAttackRun: 11},
	} {
		if err := AssessScopeEvidence(x); err == nil {
			t.Fatalf("unsafe approval accepted: %+v", x)
		}
	}
}

func TestPortSpecificHoldoutScopeReplayed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "labeled.csv")
	var data strings.Builder
	data.WriteString("agent_run_id,destination_ip,destination_port,label\n")
	// Generate many *distinct* runs; the hash-defined split excludes training
	// runs. Every held-out Agent run contains attack data on port 4444.
	for i := 0; i < 200; i++ {
		run := fmt.Sprintf("scope-run-%04d", i)
		for k := 0; k < 3; k++ {
			fmt.Fprintf(&data, "%s,8.8.8.8,4444,attack\n", run)
			fmt.Fprintf(&data, "%s,8.8.8.8,443,benign\n", run)
		}
	}
	if err := os.WriteFile(file, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	m := sampleModel()
	if err := VerifyScopeCoverageCSV(file, m, []uint32{4444}); err != nil {
		t.Fatalf("known attack port rejected: %v", err)
	}
	for _, scope := range [][]uint32{{22}, {4444, 22}, {4444, 4444}} {
		if err := VerifyScopeCoverageCSV(file, m, scope); err == nil {
			t.Fatalf("scope without independently verified evidence accepted: %v", scope)
		}
	}
	data.WriteString("scope-run-0001,8.8.8.8,4444,benign\n")
	// Force a held-out benign collision regardless of run split.
	for i := 0; i < 200; i++ {
		run := fmt.Sprintf("scope-run-%04d", i)
		digest := sha256.Sum256([]byte(run))
		if digest[0] < 51 {
			fmt.Fprintf(&data, "%s,8.8.8.8,4444,benign\n", run)
			break
		}
	}
	if err := os.WriteFile(file, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyScopeCoverageCSV(file, m, []uint32{4444}); err == nil {
		t.Fatal("matched benign collision unexpectedly accepted")
	}
}
