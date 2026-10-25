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
	maxDepth := flag.Int("max-depth", defaults.MaxDepth, "random-forest max depth (sweep-tuned default)")
	minSamplesLeaf := flag.Int("min-samples-leaf", defaults.MinSamplesLeaf, "random-forest minimum samples per leaf (sweep-tuned default)")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flag.Args())
	}

	report, err := app.TrainDecisionModelFromPublicDatasets(app.DefaultDecisionTrainingDatasets(), *output, app.DecisionTrainingHyperparameters{
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
	return nil
}
