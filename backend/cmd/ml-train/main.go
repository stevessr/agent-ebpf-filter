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
	modelType := flag.String("model-type", "", "built-in model catalog type to train (default: "+defaults.ModelType+")")
	numTrees := flag.Int("num-trees", 0, "first family knob, e.g. tree count for forests (0 = family default)")
	maxDepth := flag.Int("max-depth", 0, "second family knob, e.g. max depth for forests (0 = family default)")
	minSamplesLeaf := flag.Int("min-samples-leaf", 0, "third family knob, e.g. min samples per leaf for forests (0 = family default)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flag.Args())
	}

	report, err := app.TrainDecisionModelFromPublicDatasets(app.DefaultDecisionTrainingDatasets(), *output, app.DecisionTrainingHyperparameters{
		ModelType:      *modelType,
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
	fmt.Printf("Model: %s numTrees=%d maxDepth=%d minSamplesLeaf=%d\n",
		report.Hyperparameters.ModelType,
		report.Hyperparameters.NumTrees,
		report.Hyperparameters.MaxDepth,
		report.Hyperparameters.MinSamplesLeaf,
	)
	fmt.Printf("Samples: %d (ALLOW=%d, BLOCK=%d); validation accuracy=%.3f balanced=%.3f block-recall=%.3f; runtime reload=%t\n",
		report.Samples,
		report.LabelCounts["ALLOW"],
		report.LabelCounts["BLOCK"],
		report.ValidationAccuracy,
		report.ValidationBalancedAccuracy,
		report.ValidationBlockRecall,
		report.ArtifactReloadVerified,
	)
	return nil
}
