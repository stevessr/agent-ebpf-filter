package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"agent-ebpf-filter/app"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	defaults := app.DefaultDecisionTrainingHyperparameters()
	output := flag.String("output", "artifacts/decision-model.bin", "candidate model output path")
	numTrees := flag.Int("num-trees", defaults.NumTrees, "random-forest tree count (sweep-tuned default)")
	seed := flag.Int64("seed", defaults.Seed, "deterministic forest training seed (recorded in candidate manifest)")
	datasetFile := flag.String("dataset-file", "", "reviewed local JSONL corpus (candidate-only training)")
	datasetSHA := flag.String("dataset-sha256", "", "required expected SHA-256 for local JSONL")
	datasetLicense := flag.String("dataset-license", "", "required original license identifier for local JSONL")
	testFile := flag.String("external-test-file", "", "independent reviewed JSONL test corpus (not used for training)")
	testSHA := flag.String("external-test-sha256", "", "required SHA-256 of the external test corpus")
	maxDepth := flag.Int("max-depth", defaults.MaxDepth, "random-forest max depth (sweep-tuned default)")
	minSamplesLeaf := flag.Int("min-samples-leaf", defaults.MinSamplesLeaf, "random-forest minimum samples per leaf (sweep-tuned default)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flag.Args())
	}
	if *testFile != "" && (*datasetFile == "" || *testSHA == "") {
		return fmt.Errorf("--external-test-file requires --dataset-file and --external-test-sha256")
	}
	if *testFile == "" && *testSHA != "" {
		return fmt.Errorf("--external-test-sha256 requires --external-test-file")
	}

	sources := app.DefaultDecisionTrainingDatasets()
	if *datasetFile != "" {
		if *datasetSHA == "" || *datasetLicense == "" {
			return fmt.Errorf("--dataset-file requires both --dataset-sha256 and --dataset-license")
		}
		sources = []app.DecisionTrainingDataset{{
			Name: filepath.Base(*datasetFile), LocalPath: *datasetFile,
			ExpectedSHA256: *datasetSHA, License: *datasetLicense,
		}}
	} else if *datasetSHA != "" || *datasetLicense != "" {
		return fmt.Errorf("--dataset-sha256 and --dataset-license require --dataset-file")
	}

	report, err := app.TrainDecisionModelFromPublicDatasets(sources, *output, app.DecisionTrainingHyperparameters{
		NumTrees:       *numTrees,
		MaxDepth:       *maxDepth,
		MinSamplesLeaf: *minSamplesLeaf,
		Seed:           *seed,
	})
	if err != nil {
		return err
	}
	if *testFile != "" {
		evaluation, evalErr := app.EvaluateDecisionModelOnExternalHoldout(
			*output,
			app.DecisionTrainingDataset{Name: filepath.Base(*datasetFile), LocalPath: *datasetFile, ExpectedSHA256: *datasetSHA, License: *datasetLicense},
			app.DecisionTrainingDataset{Name: filepath.Base(*testFile), LocalPath: *testFile, ExpectedSHA256: *testSHA, License: *datasetLicense},
		)
		if evalErr != nil {
			return fmt.Errorf("external holdout: %w", evalErr)
		}
		report.ExternalTest = &evaluation
	}

	manifestPath := *output + ".manifest.json"
	if dir := filepath.Dir(manifestPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create manifest directory: %w", err)
		}
	}
	manifest, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode training manifest: %w", err)
	}
	manifest = append(manifest, '\n')
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		return fmt.Errorf("write training manifest: %w", err)
	}

	fmt.Printf("Candidate model saved: %s\n", report.ModelPath)
	fmt.Printf("Training manifest saved: %s\n", manifestPath)
	fmt.Printf("Samples: %d (ALLOW=%d, BLOCK=%d); validation accuracy=%.3f balanced=%.3f block-recall=%.3f; runtime reload=%t\n",
		report.Samples,
		report.LabelCounts["ALLOW"],
		report.LabelCounts["BLOCK"],
		report.ValidationAccuracy,
		report.ValidationBalancedAccuracy,
		report.ValidationBlockRecall,
		report.ArtifactReloadVerified,
	)
	fmt.Printf("Validation: BLOCK precision=%.3f; benign FPR=%.3f; macro-F1=%.3f (internal grouped holdout only)\n",
		report.ValidationBlockPrecision, report.ValidationBenignFPR, report.ValidationMacroF1)
	if report.ExternalTest != nil {
		fmt.Printf("External test: n=%d accuracy=%.3f balanced=%.3f BLOCK recall=%.3f precision=%.3f benign FPR=%.3f macro-F1=%.3f\n",
			report.ExternalTest.Samples, report.ExternalTest.Accuracy,
			report.ExternalTest.Balanced, report.ExternalTest.BlockRecall,
			report.ExternalTest.BlockPrecision, report.ExternalTest.BenignFPR,
			report.ExternalTest.MacroF1)
		fmt.Printf("External 95%% Wilson row-level bounds: BLOCK recall >= %.3f, benign FPR <= %.3f (correlated scenarios reduce effective support)\n",
			report.ExternalTest.BlockRecallLower95, report.ExternalTest.BenignFPRUpper95)
	}
	return nil
}
