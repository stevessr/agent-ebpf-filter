package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type resultRow struct {
	Script       string
	Model        string
	Accuracy     string
	TrainTime    string
	Inference    string
	RawOutput    string
	ExitError    error
}

func main() {
	root := mustRepoRoot()
	scripts := []string{
		"backend/cmd/train_baseline/main.go",
		"backend/cmd/train_logistic_baseline/main.go",
		"backend/cmd/train_attention/main.go",
		"backend/cmd/train_logistic_attention/main.go",
	}

	rows := make([]resultRow, 0, len(scripts))
	for _, script := range scripts {
		rows = append(rows, runScript(root, script))
	}

	report := buildReport(rows)
	reportPath := filepath.Join(root, "reports", "attention-comparison.md")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		panic(err)
	}

	fmt.Print(report)
}

func mustRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func runScript(root, rel string) resultRow {
	cmd := exec.Command("go", "run", rel)
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	out := buf.String()
	row := resultRow{
		Script:    rel,
		Model:     inferModelName(rel, out),
		Accuracy:   pickMetric(out, `(?m)^(?:validation_accuracy|baseline_validation_accuracy|attention_validation_accuracy)=([0-9.]+%)`, `(?m)^(?:validation_accuracy|baseline_validation_accuracy|attention_validation_accuracy)=([0-9.]+%)`),
		TrainTime: pickMetric(out, `(?m)^(?:training_time|baseline_training_time|attention_training_time)=([0-9a-zA-Z.µs]+)`, `(?m)^(?:training_time|baseline_training_time|attention_training_time)=([0-9a-zA-Z.µs]+)`),
		Inference: pickInference(out),
		RawOutput: out,
		ExitError: err,
	}
	if row.TrainTime == "" {
		row.TrainTime = elapsed.String()
	}
	if row.Accuracy == "" {
		row.Accuracy = "n/a"
	}
	if row.Inference == "" {
		row.Inference = "n/a"
	}
	return row
}

func inferModelName(rel, out string) string {
	switch {
	case strings.Contains(rel, "logistic_attention"):
		return "Logistic+Attention"
	case strings.Contains(rel, "logistic_baseline"):
		return "Logistic"
	case strings.Contains(rel, "attention"):
		return "RandomForest+SelfAttention"
	default:
		return "RandomForest"
	}
}

func pickMetric(out, _ string, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(out)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func pickInference(out string) string {
	re := regexp.MustCompile(`(?m)^(?:inference_time|prediction_time|eval_time)=([0-9a-zA-Z.µs]+)`)
	matches := re.FindStringSubmatch(out)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func buildReport(rows []resultRow) string {
	var b strings.Builder
	b.WriteString("# Training Results Comparison\n\n")
	b.WriteString("| Script | Model | Accuracy | Training Time | Inference Time |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: |\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", r.Script, r.Model, r.Accuracy, r.TrainTime, r.Inference))
	}
	b.WriteString("\n## Raw execution notes\n\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("- %s: %v\n", r.Script, r.ExitError))
	}
	return b.String()
}
