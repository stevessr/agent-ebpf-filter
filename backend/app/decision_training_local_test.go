package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDecisionTrainingFixture(t *testing.T, content string) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reviewed.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

func TestDecisionTrainingLocalDatasetRejectsMissingChecksum(t *testing.T) {
	path, _ := writeDecisionTrainingFixture(t, "{\"command\":\"ls\",\"label\":\"ALLOW\"}\n")
	_, err := TrainDecisionModelFromPublicDatasets([]DecisionTrainingDataset{{
		Name: "reviewed", LocalPath: path, License: "CC-BY-4.0",
	}}, filepath.Join(t.TempDir(), "candidate.bin"), DecisionTrainingHyperparameters{})
	if err == nil || !strings.Contains(err.Error(), "requires a 64-character SHA-256") {
		t.Fatalf("missing checksum must reject dataset before training: %v", err)
	}
}

func TestDecisionTrainingLocalDatasetRejectsChecksumMismatch(t *testing.T) {
	path, _ := writeDecisionTrainingFixture(t, "{\"command\":\"ls\",\"label\":\"ALLOW\"}\n")
	_, err := TrainDecisionModelFromPublicDatasets([]DecisionTrainingDataset{{
		Name: "reviewed", LocalPath: path, License: "CC-BY-4.0",
		ExpectedSHA256: strings.Repeat("0", 64),
	}}, filepath.Join(t.TempDir(), "candidate.bin"), DecisionTrainingHyperparameters{})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("incorrect checksum must reject dataset before training: %v", err)
	}
}

func TestDecisionTrainingLocalDatasetRejectsMissingExplicitLabel(t *testing.T) {
	path, checksum := writeDecisionTrainingFixture(t, "{\"command\":\"ls -la\"}\n")
	_, err := TrainDecisionModelFromPublicDatasets([]DecisionTrainingDataset{{
		Name: "reviewed", LocalPath: path, License: "CC-BY-4.0",
		ExpectedSHA256: checksum,
	}}, filepath.Join(t.TempDir(), "candidate.bin"), DecisionTrainingHyperparameters{})
	if err == nil || !strings.Contains(err.Error(), "lacks explicit action label") {
		t.Fatalf("unlabeled local pretraining must fail closed: %v", err)
	}
}

func TestDecisionTrainingLocalDatasetRequiresSourceLicense(t *testing.T) {
	path, checksum := writeDecisionTrainingFixture(t, "{\"command\":\"ls\",\"label\":\"ALLOW\"}\n")
	_, err := TrainDecisionModelFromPublicDatasets([]DecisionTrainingDataset{{
		Name: "reviewed", LocalPath: path, ExpectedSHA256: checksum,
	}}, filepath.Join(t.TempDir(), "candidate.bin"), DecisionTrainingHyperparameters{})
	if err == nil || !strings.Contains(err.Error(), "requires an explicit license") {
		t.Fatalf("unknown local source license must reject dataset: %v", err)
	}
}
