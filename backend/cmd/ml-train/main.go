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
	datasetFile := flag.String("dataset-file", "", "reviewed local JSONL corpus (candidate-only training)")
	datasetSHA := flag.String("dataset-sha256", "", "required expected SHA-256 for local JSONL")
	datasetLicense := flag.String("dataset-license", "", "required original license identifier for local JSONL")
	maxDepth := flag.Int("max-depth", defaults.MaxDepth, "random-forest max depth (sweep-tuned default)")
	minSamplesLeaf := flag.Int("min-samples-leaf", defaults.MinSamplesLeaf, "random-forest minimum samples per leaf (sweep-tuned default)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flag.Args())
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
	})
	if err != nil {
		return err
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
	return nil
}
