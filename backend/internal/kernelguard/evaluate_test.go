package kernelguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVHoldoutReplayAndBadLabels(t *testing.T) {
	m := sampleModel()
	dir := t.TempDir()
	path := filepath.Join(dir, "labels.csv")
	header := "agent_run_id,destination_ip,destination_port,label\n"
	var rows strings.Builder
	rows.WriteString(header)
	for i := 0; i < 20; i++ {
		run := "run-" + string(rune('a'+i))
		rows.WriteString(run + ",8.8.8.8,4444,attack\n")
		rows.WriteString(run + ",8.8.8.8,443,benign\n")
	}
	if err := os.WriteFile(path, []byte(rows.String()), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := EvaluateCSVHoldout(path, m)
	if err != nil {
		t.Fatal(err)
	}
	if got.IndependentGroups < 1 || got.Attacks != got.TruePositives ||
		got.Benign < 1 || got.FalsePositives != 0 {
		t.Fatalf("unexpected deterministic holdout: %+v", got)
	}
	if err := os.WriteFile(path, []byte(header+"run-x,8.8.8.8,4444,untrusted\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateCSVHoldout(path, m); err == nil {
		t.Fatal("invalid holdout label accepted")
	}
}

func TestCSVRejectsForgedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.csv")
	if err := os.WriteFile(path, []byte("agent_run_id,agent_run_id,destination_port,label\nx,x,22,attack\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateCSVHoldout(path, sampleModel()); err == nil {
		t.Fatal("duplicate header was accepted")
	}
}
