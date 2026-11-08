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

func TestDecisionExternalEvaluationRejectsOverlappingProvenanceBeforeModelLoad(t *testing.T) {
	cases := []struct {
		name, train, test, matchingID string
	}{
		{
			name: "scenario",
			train: `{"command":"ls /tmp/one","label":"ALLOW","scenario_id":"shared-case","run_id":"run-1"}` + "\n",
			test: `{"command":"rm /tmp/two","label":"BLOCK","scenario_id":"shared-case","run_id":"run-2"}` + "\n",
			matchingID: "scenario_id",
		},
		{
			name: "run",
			train: `{"command":"ls /tmp/one","label":"ALLOW","scenario_id":"scenario-1","run_id":"shared-run"}` + "\n",
			test: `{"command":"rm /tmp/two","label":"BLOCK","scenario_id":"scenario-2","run_id":"shared-run"}` + "\n",
			matchingID: "run_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trainPath, trainHash := writeDecisionTrainingFixture(t, tc.train)
			testPath, testHash := writeDecisionTrainingFixture(t, tc.test)
			_, err := EvaluateDecisionModelOnExternalHoldout(
				filepath.Join(t.TempDir(), "missing-candidate.bin"),
				DecisionTrainingDataset{Name: "train", LocalPath: trainPath, ExpectedSHA256: trainHash, License: "MIT"},
				DecisionTrainingDataset{Name: "test", LocalPath: testPath, ExpectedSHA256: testHash, License: "MIT"},
			)
			if err == nil || !strings.Contains(err.Error(), "contamination: "+tc.matchingID) {
				t.Fatalf("overlapping %s must fail before model load: %v", tc.matchingID, err)
			}
		})
	}
}

func TestDecisionExternalEvaluationRejectsMissingGroupProvenance(t *testing.T) {
	trainPath, trainHash := writeDecisionTrainingFixture(t,
		`{"command":"ls /tmp/one","label":"ALLOW"}`+"\n")
	testPath, testHash := writeDecisionTrainingFixture(t,
		`{"command":"rm /tmp/two","label":"BLOCK","scenario_id":"test-2","run_id":"test-run-2"}`+"\n")
	_, err := EvaluateDecisionModelOnExternalHoldout(
		filepath.Join(t.TempDir(), "missing-candidate.bin"),
		DecisionTrainingDataset{Name: "train", LocalPath: trainPath, ExpectedSHA256: trainHash, License: "MIT"},
		DecisionTrainingDataset{Name: "test", LocalPath: testPath, ExpectedSHA256: testHash, License: "MIT"},
	)
	if err == nil || !strings.Contains(err.Error(), "requires scenario_id and run_id") {
		t.Fatalf("a corpus without group provenance cannot support independent validation: %v", err)
	}
}

func TestDecisionWilsonConfidenceIntervals(t *testing.T) {
	low, high := wilson95(0, 10)
	if low != 0 || high < 0.27 || high > 0.29 {
		t.Fatalf("zero errors in 10 benign samples must retain uncertainty: [%v,%v]", low, high)
	}
	low, high = wilson95(10, 10)
	if low < 0.71 || low > 0.73 || high != 1 {
		t.Fatalf("perfect 10-sample recall must retain uncertainty: [%v,%v]", low, high)
	}
	low, high = wilson95(0, 0)
	if low != 0 || high != 1 {
		t.Fatalf("missing data must not imply a perfect estimate: [%v,%v]", low, high)
	}
}
