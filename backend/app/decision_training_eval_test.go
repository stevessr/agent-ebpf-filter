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

// A shared run connects distinct scenarios and a shared scenario connects runs;
// neither pair may be counted as independent validation evidence.
func TestReviewedCorpusIdentitiesUseTransitiveScenarioRunComponents(t *testing.T) {
	path, _ := writeDecisionTrainingFixture(t,
		`{"command":"ls /tmp/one","label":"ALLOW","scenario_id":"scenario-1","run_id":"run-1"}`+"\n"+
		`{"command":"rm /tmp/one","label":"BLOCK","scenario_id":"scenario-2","run_id":"run-1"}`+"\n"+
		`{"command":"cat /tmp/two","label":"ALLOW","scenario_id":"scenario-2","run_id":"run-2"}`+"\n"+
		`{"command":"rm /tmp/three","label":"BLOCK","scenario_id":"scenario-3","run_id":"run-3"}`+"\n")
	provenance, err := reviewedCorpusIdentities(path)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.groupCount != 2 || len(provenance.rowGroups) != 4 ||
		provenance.rowGroups[0] != provenance.rowGroups[1] ||
		provenance.rowGroups[1] != provenance.rowGroups[2] ||
		provenance.rowGroups[2] == provenance.rowGroups[3] {
		t.Fatalf("unexpected independent group mapping: %+v", provenance)
	}
}

func TestDecisionGroupSecurityCountsCorrelatedCommandErrorsOnce(t *testing.T) {
	outcomes := []decisionGroupOutcome{
		{group: 0, actual: 0, predicted: 0},
		{group: 0, actual: 0, predicted: 0},
		{group: 0, actual: 1, predicted: 1},
		{group: 0, actual: 1, predicted: 0}, // one missed BLOCK fails group 0
		{group: 1, actual: 0, predicted: 1}, // one benign false block fails group 1
		{group: 2, actual: 1, predicted: 1}, // group 2 contains only BLOCK
	}
	metrics := summarizeDecisionGroupSecurity(outcomes)
	if metrics.independentGroups != 3 || metrics.blockGroups != 2 || metrics.benignGroups != 2 ||
		metrics.blockRecall != 0.5 || metrics.benignFPR != 0.5 ||
		metrics.blockRecallLower95 <= 0 || metrics.blockRecallLower95 >= 0.5 ||
		metrics.benignFPRUpper95 <= 0.5 || metrics.benignFPRUpper95 >= 1 {
		t.Fatalf("incorrect strict group-level security metrics: %+v", metrics)
	}
}

func TestDecisionGroupSecurityDoesNotTreatEmptyGroupsAsPerfect(t *testing.T) {
	got := summarizeDecisionGroupSecurity(nil)
	if got.independentGroups != 0 || got.blockRecall != 0 || got.benignFPR != 0 ||
		got.blockRecallLower95 != 0 || got.benignFPRUpper95 != 0 {
		t.Fatalf("zero evidence must not be reported as success: %+v", got)
	}
}
