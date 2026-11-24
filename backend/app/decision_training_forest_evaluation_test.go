package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"agent-ebpf-filter/app/ml"
)

// TestDecisionCandidateReviewedForestEvaluation is a repeatable candidate-only
// evaluation, not a production acceptance gate. All seeds share the same
// grouped holdout: 32 runs do not constitute 32 independent test corpora.
func TestDecisionCandidateReviewedForestEvaluation(t *testing.T) {
	const path = "../../research/curated/ai-reviewed-bash-actions.jsonl"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := loadReviewedCandidateCorpus(DecisionTrainingDataset{Name: "ai-reviewed", LocalPath: path, ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), License: "CC-BY-SA-4.0"})
	if err != nil {
		t.Fatal(err)
	}
	extractor := &FeatureExtractor{history: newRecentHistoryBuffer(1)}
	var samples []ml.TrainingSample
	for _, row := range rows {
		samples = append(samples, buildRemoteDatasetSampleWithExtractor(row, "preserve", true, extractor))
	}
	train, _, _, validation, err := ml.PrepareAutoTuneSplit(samples, 0.2)
	if err != nil {
		t.Fatal(err)
	}
	if len(train) == 0 || len(validation) == 0 {
		t.Fatal("empty train or validation split")
	}
	hp := DefaultDecisionTrainingHyperparameters()
	totalCorrect, total, tp, tn, fp, fn := 0, 0, 0, 0, 0, 0
	for seed := int64(0); seed < 32; seed++ {
		model := ml.BuildAutoTuneForest(train, hp.NumTrees, hp.MaxDepth, hp.MinSamplesLeaf, seed)
		model.Prune(train)
		for _, row := range validation {
			pred := model.Predict(row.Features).Action
			if pred != 0 && pred != 1 {
				t.Fatalf("binary corpus produced action %d", pred)
			}
			total++
			if pred == row.Label {
				totalCorrect++
			}
			if row.Label == 1 {
				if pred == 1 {
					tp++
				} else {
					fn++
				}
			} else {
				if pred == 0 {
					tn++
				} else {
					fp++
				}
			}
		}
	}
	if tp+fn == 0 || tn+fp == 0 || tp+fp == 0 {
		t.Fatal("evaluation lacks class or prediction coverage")
	}
	t.Logf("corpus=%x train=%d validation=%d seeds=0..31 trees=%d depth=%d leaf=%d correct=%d/%d accuracy=%.6f recall=%.6f FPR=%.6f precision=%.6f balanced=%.6f TP=%d TN=%d FP=%d FN=%d", sha256.Sum256(raw), len(train), len(validation), hp.NumTrees, hp.MaxDepth, hp.MinSamplesLeaf, totalCorrect, total, float64(totalCorrect)/float64(total), float64(tp)/float64(tp+fn), float64(fp)/float64(fp+tn), float64(tp)/float64(tp+fp), (float64(tp)/float64(tp+fn)+float64(tn)/float64(tn+fp))/2, tp, tn, fp, fn)
}
