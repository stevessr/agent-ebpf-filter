package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"
)

// DecisionExternalEvaluation records a completely separate corpus evaluation.
// It is not a training-time tuning score; the test rows never fit the model.
type DecisionExternalEvaluation struct {
	Source         string                    `json:"source"`
	SHA256         string                    `json:"sha256"`
	License        string                    `json:"license"`
	Samples        int                       `json:"samples"`
	AllowSamples   int                       `json:"allowSamples"`
	BlockSamples   int                       `json:"blockSamples"`
	Accuracy       float64                   `json:"accuracy"`
	Balanced       float64                   `json:"balancedAccuracy"`
	MacroF1        float64                   `json:"macroF1"`
	BlockRecall    float64                   `json:"blockRecall"`
	BlockPrecision float64                   `json:"blockPrecision"`
	BenignFPR      float64                   `json:"benignFalsePositiveRate"`
	BenignFPRUpper95   float64              `json:"benignFPRUpper95"`
	BlockRecallLower95 float64              `json:"blockRecallLower95"`
	Confusion      map[string]map[string]int `json:"confusion"`
}

func loadReviewedCandidateCorpus(source DecisionTrainingDataset) ([]remoteDatasetRow, string, error) {
	path := strings.TrimSpace(source.LocalPath)
	if path == "" || strings.TrimSpace(source.License) == "" {
		return nil, "", fmt.Errorf("reviewed external corpus requires file and license")
	}
	expected := strings.ToLower(strings.TrimSpace(source.ExpectedSHA256))
	if len(expected) != 64 {
		return nil, "", fmt.Errorf("reviewed external corpus requires SHA-256")
	}
	for _, char := range expected {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return nil, "", fmt.Errorf("invalid reviewed corpus digest")
		}
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if !stat.Mode().IsRegular() || stat.Size() > remoteDatasetUploadLimitBytes {
		return nil, "", fmt.Errorf("reviewed corpus must be a regular file <= %d bytes", remoteDatasetUploadLimitBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(raw) > int(remoteDatasetUploadLimitBytes) {
		return nil, "", fmt.Errorf("reviewed corpus too large")
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(raw))
	if actual != expected {
		return nil, "", fmt.Errorf("reviewed corpus SHA-256 mismatch: %q", source.Name)
	}
	parsed, err := pullRemoteDataset(remoteDatasetRequest{
		Content: string(raw), SourceName: source.Name, Format: "jsonl",
		Limit: remoteDatasetAbsoluteRecordLimit, LabelMode: "preserve",
		ImportAll: true, CleanSensitive: true,
	})
	if err != nil {
		return nil, "", err
	}
	if parsed.Total == 0 || parsed.Truncated || parsed.TotalIsLowerBound || len(parsed.Rows) != parsed.Total {
		return nil, "", fmt.Errorf("reviewed corpus is empty or truncated")
	}
	for _, row := range parsed.Rows {
		if row.LabelSource != "dataset" {
			return nil, "", fmt.Errorf("reviewed corpus row %d lacks explicit action label", row.Row)
		}
		if action := normalizeActionLabel(row.Label); action != "ALLOW" && action != "BLOCK" {
			return nil, "", fmt.Errorf("reviewed corpus row %d has unsupported action label %q", row.Row, row.Label)
		}
	}
	return parsed.Rows, actual, nil
}

// reviewedCorpusIdentities reads the independently declared provenance IDs
// from raw reviewed JSONL. The generic dataset parser intentionally discards
// these fields, so their isolation must be checked before loading any model.
// IDs are global across sources, not prefixed by export name.
func reviewedCorpusIdentities(path string) (map[string]struct{}, map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	scenarios, runs := make(map[string]struct{}), make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(strings.TrimSpace(scanner.Text())) == 0 {
			continue
		}
		var item struct {
			ScenarioID string `json:"scenario_id"`
			RunID      string `json:"run_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, nil, fmt.Errorf("row %d: invalid JSON: %w", line, err)
		}
		scenario, run := strings.TrimSpace(item.ScenarioID), strings.TrimSpace(item.RunID)
		if scenario == "" || run == "" {
			return nil, nil, fmt.Errorf("row %d: independent evaluation requires scenario_id and run_id", line)
		}
		scenarios[scenario], runs[run] = struct{}{}, struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return scenarios, runs, nil
}

// wilson95 gives a two-sided 95%% interval for a Bernoulli rate. An observed
// zero false-positive count is not evidence of a zero true false-positive rate.
func wilson95(successes, total int) (float64, float64) {
	if total <= 0 {
		return 0, 1
	}
	const z = 1.959963984540054
	n := float64(total)
	p := float64(successes) / n
	z2 := z * z
	denominator := 1 + z2/n
	center := (p + z2/(2*n)) / denominator
	margin := z * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denominator
	return math.Max(0, center-margin), math.Min(1, center+margin)
}

// EvaluateDecisionModelOnExternalHoldout reads disjoint local training/test
// corpora, rejects duplicate commands across them, and evaluates the serialized
// candidate with the *runtime* 128-dimensional feature extractor. This function
// never updates a deployed model or security policy.
func EvaluateDecisionModelOnExternalHoldout(modelPath string, training, test DecisionTrainingDataset) (DecisionExternalEvaluation, error) {
	trainRows, trainDigest, err := loadReviewedCandidateCorpus(training)
	if err != nil {
		return DecisionExternalEvaluation{}, fmt.Errorf("training corpus: %w", err)
	}
	testRows, testDigest, err := loadReviewedCandidateCorpus(test)
	if err != nil {
		return DecisionExternalEvaluation{}, fmt.Errorf("test corpus: %w", err)
	}
	if trainDigest == testDigest {
		return DecisionExternalEvaluation{}, fmt.Errorf("external test corpus is byte-identical to training corpus")
	}
	trainingCommands := make(map[string]struct{}, len(trainRows))
	for _, row := range trainRows {
		comm, args := behavior.NormalizeCommandInput(row.CommandLine, row.Comm, row.Args)
		trainingCommands[behavior.CommandKey(comm, args)] = struct{}{}
	}
	seenTest := make(map[string]struct{}, len(testRows))
	for _, row := range testRows {
		comm, args := behavior.NormalizeCommandInput(row.CommandLine, row.Comm, row.Args)
		key := behavior.CommandKey(comm, args)
		if _, exists := trainingCommands[key]; exists {
			return DecisionExternalEvaluation{}, fmt.Errorf("external holdout contamination: test row %d duplicates training command", row.Row)
		}
		if _, exists := seenTest[key]; exists {
			return DecisionExternalEvaluation{}, fmt.Errorf("external holdout contains duplicate command at row %d", row.Row)
		}
		seenTest[key] = struct{}{}
	}
	trainScenarios, trainRuns, err := reviewedCorpusIdentities(training.LocalPath)
	if err != nil {
		return DecisionExternalEvaluation{}, fmt.Errorf("training group provenance: %w", err)
	}
	testScenarios, testRuns, err := reviewedCorpusIdentities(test.LocalPath)
	if err != nil {
		return DecisionExternalEvaluation{}, fmt.Errorf("test group provenance: %w", err)
	}
	for scenario := range testScenarios {
		if _, exists := trainScenarios[scenario]; exists {
			return DecisionExternalEvaluation{}, fmt.Errorf("external holdout contamination: scenario_id %q appears in both corpora", scenario)
		}
	}
	for run := range testRuns {
		if _, exists := trainRuns[run]; exists {
			return DecisionExternalEvaluation{}, fmt.Errorf("external holdout contamination: run_id %q appears in both corpora", run)
		}
	}
	model := tryLoadModel(modelPath, ml.ModelRandomForest)
	if model == nil {
		return DecisionExternalEvaluation{}, fmt.Errorf("unable to reload candidate model for external evaluation")
	}
	result := DecisionExternalEvaluation{
		Source: test.Name, SHA256: testDigest, License: test.License,
		Confusion: map[string]map[string]int{
			"ALLOW": {"ALLOW": 0, "BLOCK": 0, "REWRITE": 0, "ALERT": 0},
			"BLOCK": {"ALLOW": 0, "BLOCK": 0, "REWRITE": 0, "ALERT": 0},
		},
	}
	extractor := &FeatureExtractor{history: newRecentHistoryBuffer(1)}
	var correct, allowCorrect, blockCorrect, blockPredicted, benignFalsePositives int
	for _, row := range testRows {
		sample := buildRemoteDatasetSampleWithExtractor(row, "preserve", true, extractor)
		neutralizeOfflineDecisionClockFeatures(&sample)
		actual := ml.ActionLabel[sample.Label]
		prediction := model.Predict(sample.Features).Action
		predName := ml.ActionLabel[prediction]
		if predName == "" {
			return DecisionExternalEvaluation{}, fmt.Errorf("invalid model action %d", prediction)
		}
		result.Confusion[actual][predName]++
		result.Samples++
		if prediction == sample.Label {
			correct++
		}
		if prediction == 1 {
			blockPredicted++
		}
		if sample.Label == 0 {
			result.AllowSamples++
			if prediction == 0 {
				allowCorrect++
			} else {
				benignFalsePositives++
			}
		} else if sample.Label == 1 {
			result.BlockSamples++
			if prediction == 1 {
				blockCorrect++
			}
		}
	}
	if result.AllowSamples == 0 || result.BlockSamples == 0 {
		return DecisionExternalEvaluation{}, fmt.Errorf("external test requires both ALLOW and BLOCK reference actions")
	}
	result.Accuracy = float64(correct) / float64(result.Samples)
	result.BlockRecall = float64(blockCorrect) / float64(result.BlockSamples)
	result.BenignFPR = float64(benignFalsePositives) / float64(result.AllowSamples)
	result.Balanced = (float64(allowCorrect)/float64(result.AllowSamples) + result.BlockRecall) / 2
	if blockPredicted > 0 {
		result.BlockPrecision = float64(blockCorrect) / float64(blockPredicted)
	}
	allowPredicted := 0
	for _, counts := range result.Confusion {
		allowPredicted += counts["ALLOW"]
	}
	allowF1, blockF1 := 0.0, 0.0
	if denom := result.AllowSamples + allowPredicted; denom > 0 {
		allowF1 = 2 * float64(allowCorrect) / float64(denom)
	}
	if denom := result.BlockSamples + blockPredicted; denom > 0 {
		blockF1 = 2 * float64(blockCorrect) / float64(denom)
	}
	result.MacroF1 = (allowF1 + blockF1) / 2
	_, result.BenignFPRUpper95 = wilson95(benignFalsePositives, result.AllowSamples)
	result.BlockRecallLower95, _ = wilson95(blockCorrect, result.BlockSamples)
	return result, nil
}
