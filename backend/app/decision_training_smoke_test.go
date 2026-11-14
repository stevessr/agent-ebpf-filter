package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecisionCandidateLocalCorpusTrainingAndExternalHoldout(t *testing.T) {
	// Commands are parsed only, never executed. This is a pipeline smoke test,
	// not a claim about generalization to genuine attacker behavior.
	var train, test strings.Builder
	for i := 0; i < 48; i++ {
		fmt.Fprintf(&train, "{\"command\":\"echo benign-%d\",\"label\":\"ALLOW\",\"scenario_id\":\"train-allow-%d\",\"run_id\":\"train-allow-run-%d\"}\n", i, i, i)
		fmt.Fprintf(&train, "{\"command\":\"rm -rf ./isolated-risk-%d\",\"label\":\"BLOCK\",\"scenario_id\":\"train-block-%d\",\"run_id\":\"train-block-run-%d\"}\n", i, i, i)
	}
	for i := 100; i < 112; i++ {
		fmt.Fprintf(&test, "{\"command\":\"echo benign-%d\",\"label\":\"ALLOW\",\"scenario_id\":\"test-allow-%d\",\"run_id\":\"test-allow-run-%d\"}\n", i, i, i)
		fmt.Fprintf(&test, "{\"command\":\"rm -rf ./isolated-risk-%d\",\"label\":\"BLOCK\",\"scenario_id\":\"test-block-%d\",\"run_id\":\"test-block-run-%d\"}\n", i, i, i)
	}
	trainPath, trainDigest := writeDecisionTrainingFixture(t, train.String())
	testPath, testDigest := writeDecisionTrainingFixture(t, test.String())
	modelPath := filepath.Join(t.TempDir(), "candidate.bin")
	source := DecisionTrainingDataset{
		Name: "controlled-train", LocalPath: trainPath,
		ExpectedSHA256: trainDigest, License: "INTERNAL-TEST",
	}
	report, err := TrainDecisionModelFromPublicDatasets(
		[]DecisionTrainingDataset{source}, modelPath,
		DecisionTrainingHyperparameters{NumTrees: 9, MaxDepth: 6, MinSamplesLeaf: 2},
	)
	if err != nil {
		t.Fatalf("train verified local candidate: %v", err)
	}
	if !report.CandidateOnly || !report.ArtifactReloadVerified || report.Samples != 96 ||
		report.TrainingSamples < 1 || report.ValidationSamples < 1 {
		t.Fatalf("candidate training report invalid: %+v", report)
	}
	evaluation, err := EvaluateDecisionModelOnExternalHoldout(modelPath, source,
		DecisionTrainingDataset{Name: "controlled-test", LocalPath: testPath,
			ExpectedSHA256: testDigest, License: "INTERNAL-TEST"})
	if err != nil {
		t.Fatalf("evaluate disjoint test: %v", err)
	}
	if evaluation.Samples != 24 || evaluation.AllowSamples != 12 || evaluation.BlockSamples != 12 ||
		evaluation.Accuracy < 0 || evaluation.Accuracy > 1 ||
		evaluation.BenignFPR < 0 || evaluation.BenignFPR > 1 {
		t.Fatalf("invalid independent evaluation: %+v", evaluation)
	}
}
