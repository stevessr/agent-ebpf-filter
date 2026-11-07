package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionExternalEvaluationRejectsOverlappingCommandsBeforeModelLoad(t *testing.T) {
	trainPath, trainHash := writeDecisionTrainingFixture(t,
		"{\"command\":\"stat ./secrets\",\"label\":\"ALLOW\"}\n")
	// Same command but different content/digest must still be rejected.
	const external = "{\"command\":\"stat ./secrets\",\"label\":\"BLOCK\"}\n"
	testPath, testHash := writeDecisionTrainingFixture(t, external)
	_, err := EvaluateDecisionModelOnExternalHoldout(
		filepath.Join(t.TempDir(), "missing-candidate.bin"),
		DecisionTrainingDataset{Name: "train", LocalPath: trainPath, ExpectedSHA256: trainHash, License: "MIT"},
		DecisionTrainingDataset{Name: "test", LocalPath: testPath, ExpectedSHA256: testHash, License: "MIT"},
	)
	if err == nil || !strings.Contains(err.Error(), "contamination") {
		t.Fatalf("identical command must not cross independent holdout: %v", err)
	}
}

func TestDecisionExternalEvaluationRequiresVerifiedTestDigest(t *testing.T) {
	trainPath, trainHash := writeDecisionTrainingFixture(t,
		"{\"command\":\"ls\",\"label\":\"ALLOW\"}\n")
	testPath, _ := writeDecisionTrainingFixture(t,
		"{\"command\":\"pwd\",\"label\":\"BLOCK\"}\n")
	_, err := EvaluateDecisionModelOnExternalHoldout(
		filepath.Join(t.TempDir(), "missing-candidate.bin"),
		DecisionTrainingDataset{Name: "train", LocalPath: trainPath, ExpectedSHA256: trainHash, License: "MIT"},
		DecisionTrainingDataset{Name: "test", LocalPath: testPath, ExpectedSHA256: strings.Repeat("f", 64), License: "MIT"},
	)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("unverified external test must fail: %v", err)
	}
}
