package app

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section sweeprenderreport.go ----

func writeCSV(path string, results []sweepResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(sweepResultCSVHeader()); err != nil {
		return err
	}
	for _, r := range results {
		if err := w.Write(sweepResultCSVRow(r)); err != nil {
			return err
		}
	}
	return w.Error()
}

func appendSweepResultsCSV(path string, results []sweepResult) error {
	if len(results) == 0 {
		return nil
	}
	needsHeader := true
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		needsHeader = false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if needsHeader {
		if err := w.Write(sweepResultCSVHeader()); err != nil {
			return err
		}
	}
	for _, result := range results {
		if err := w.Write(sweepResultCSVRow(result)); err != nil {
			return err
		}
	}
	return w.Error()
}

func sweepResultCSVHeader() []string {
	return []string{
		"profile", "dataset", "baseProfile", "modelType",
		"parameterName", "parameterKind", "requiredDiscretePoints", "configuredDiscretePoints",
		"xValue", "yValue", "configSummary",
		"trainAccuracy", "validationAccuracy", "allowPassRate", "durationSeconds",
		"inferenceDurationSeconds", "inferenceSamples", "inferenceLatencyMs", "inferenceThroughput",
		"memoryBytes",
		"numSamples", "trainSamples", "validationSamples", "error",
	}
}

func sweepResultCSVRow(r sweepResult) []string {
	return []string{
		r.Profile,
		r.Dataset,
		r.BaseProfile,
		string(r.ModelType),
		r.ParameterName,
		r.ParameterKind,
		strconv.Itoa(r.RequiredPoints),
		strconv.Itoa(r.ConfiguredPoints),
		strconv.Itoa(r.XValue),
		strconv.Itoa(r.YValue),
		r.ConfigSummary,
		fmt.Sprintf("%.6f", r.TrainAccuracy),
		fmt.Sprintf("%.6f", r.ValidationAccuracy),
		fmt.Sprintf("%.6f", r.AllowPassRate),
		fmt.Sprintf("%.6f", r.Duration),
		fmt.Sprintf("%.6f", r.InferenceDuration),
		strconv.Itoa(r.InferenceSamples),
		fmt.Sprintf("%.6f", r.InferenceLatencyMs),
		fmt.Sprintf("%.6f", r.InferenceThroughput),
		strconv.Itoa(int(r.MemoryBytes)),
		strconv.Itoa(r.NumSamples),
		strconv.Itoa(r.TrainSamples),
		strconv.Itoa(r.ValidationSamples),
		r.Error,
	}
}

func readSweepResultsCSV(path string) ([]sweepResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("no cached sweep rows in %s", path)
	}
	header := make(map[string]int, len(rows[0]))
	for i, name := range rows[0] {
		header[name] = i
	}
	out := make([]sweepResult, 0, len(rows)-1)
	for _, row := range rows[1:] {
		out = append(out, sweepResult{
			Profile:             csvString(row, header, "profile"),
			Dataset:             csvString(row, header, "dataset"),
			BaseProfile:         csvString(row, header, "baseProfile"),
			ModelType:           ModelType(csvString(row, header, "modelType")),
			ParameterName:       csvString(row, header, "parameterName"),
			ParameterKind:       csvString(row, header, "parameterKind"),
			RequiredPoints:      csvInt(row, header, "requiredDiscretePoints"),
			ConfiguredPoints:    csvInt(row, header, "configuredDiscretePoints"),
			XValue:              csvInt(row, header, "xValue"),
			YValue:              csvInt(row, header, "yValue"),
			ConfigSummary:       csvString(row, header, "configSummary"),
			TrainAccuracy:       csvFloat(row, header, "trainAccuracy"),
			ValidationAccuracy:  csvFloat(row, header, "validationAccuracy"),
			AllowPassRate:       csvFloat(row, header, "allowPassRate"),
			Duration:            csvFloat(row, header, "durationSeconds"),
			InferenceDuration:   csvFloat(row, header, "inferenceDurationSeconds"),
			InferenceSamples:    csvInt(row, header, "inferenceSamples"),
			InferenceLatencyMs:  csvFloat(row, header, "inferenceLatencyMs"),
			InferenceThroughput: csvFloat(row, header, "inferenceThroughput"),
			MemoryBytes:         int64(csvInt(row, header, "memoryBytes")),
			NumSamples:          csvInt(row, header, "numSamples"),
			TrainSamples:        csvInt(row, header, "trainSamples"),
			ValidationSamples:   csvInt(row, header, "validationSamples"),
			Error:               csvString(row, header, "error"),
		})
	}
	return out, nil
}

func csvString(row []string, header map[string]int, name string) string {
	idx, ok := header[name]
	if !ok || idx < 0 || idx >= len(row) {
		return ""
	}
	return row[idx]
}

func csvInt(row []string, header map[string]int, name string) int {
	value, _ := strconv.Atoi(csvString(row, header, name))
	return value
}

func csvFloat(row []string, header map[string]int, name string) float64 {
	value, _ := strconv.ParseFloat(csvString(row, header, name), 64)
	return value
}

func writeRepeatCSV(path string, runs []repeatRunResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"profile", "modelType", "xValue", "yValue", "runIndex", "configSummary",
		"trainAccuracy", "validationAccuracy", "allowPassRate", "durationSeconds",
		"inferenceDurationSeconds", "inferenceSamples", "inferenceLatencyMs", "inferenceThroughput",
		"memoryBytes",
		"numSamples", "trainSamples", "validationSamples", "error",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range runs {
		row := []string{
			r.Profile,
			string(r.ModelType),
			strconv.Itoa(r.XValue),
			strconv.Itoa(r.YValue),
			strconv.Itoa(r.RunIndex),
			r.ConfigSummary,
			fmt.Sprintf("%.6f", r.TrainAccuracy),
			fmt.Sprintf("%.6f", r.ValidationAccuracy),
			fmt.Sprintf("%.6f", r.AllowPassRate),
			fmt.Sprintf("%.6f", r.Duration),
			fmt.Sprintf("%.6f", r.InferenceDuration),
			strconv.Itoa(r.InferenceSamples),
			fmt.Sprintf("%.6f", r.InferenceLatencyMs),
			fmt.Sprintf("%.6f", r.InferenceThroughput),
			strconv.Itoa(int(r.MemoryBytes)),
			strconv.Itoa(r.NumSamples),
			strconv.Itoa(r.TrainSamples),
			strconv.Itoa(r.ValidationSamples),
			r.Error,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeRepeatSummaryCSV(path string, summaries []repeatSummary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"profile", "modelType", "comparable", "xValue", "yValue", "configSummary",
		"runs", "successRuns", "failureRuns", "successRate",
		"trainMean", "trainStd", "validationMean", "validationStd",
		"validationMin", "validationMax", "allowMean", "allowStd", "allowMin", "allowMax",
		"durationMean", "durationStd",
		"inferenceMean", "inferenceStd", "inferenceMin", "inferenceMax", "inferenceLatencyMean", "inferenceLatencyStd",
		"memoryMean", "memoryStd", "memoryMin", "memoryMax",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, s := range summaries {
		row := []string{
			s.Profile,
			string(s.ModelType),
			strconv.FormatBool(s.Comparable),
			strconv.Itoa(s.XValue),
			strconv.Itoa(s.YValue),
			s.ConfigSummary,
			strconv.Itoa(s.Runs),
			strconv.Itoa(s.SuccessRuns),
			strconv.Itoa(s.FailureRuns),
			fmt.Sprintf("%.6f", s.SuccessRate),
			fmt.Sprintf("%.6f", s.TrainMean),
			fmt.Sprintf("%.6f", s.TrainStd),
			fmt.Sprintf("%.6f", s.ValidationMean),
			fmt.Sprintf("%.6f", s.ValidationStd),
			fmt.Sprintf("%.6f", s.ValidationMin),
			fmt.Sprintf("%.6f", s.ValidationMax),
			fmt.Sprintf("%.6f", s.AllowMean),
			fmt.Sprintf("%.6f", s.AllowStd),
			fmt.Sprintf("%.6f", s.AllowMin),
			fmt.Sprintf("%.6f", s.AllowMax),
			fmt.Sprintf("%.6f", s.DurationMean),
			fmt.Sprintf("%.6f", s.DurationStd),
			fmt.Sprintf("%.6f", s.InferenceMean),
			fmt.Sprintf("%.6f", s.InferenceStd),
			fmt.Sprintf("%.6f", s.InferenceMin),
			fmt.Sprintf("%.6f", s.InferenceMax),
			fmt.Sprintf("%.6f", s.InferenceLatencyMean),
			fmt.Sprintf("%.6f", s.InferenceLatencyStd),
			fmt.Sprintf("%.0f", s.MemoryMean),
			fmt.Sprintf("%.0f", s.MemoryStd),
			fmt.Sprintf("%.0f", s.MemoryMin),
			fmt.Sprintf("%.0f", s.MemoryMax),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func renderStabilityChart(summaries []repeatSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no stability summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortProfileLabel(s.Profile),
			Value: s.ValidationMean,
			Title: fmt.Sprintf("%s | %s | mean=%.2f%% ± %.2f%% | success=%.0f%%",
				s.Profile, s.ConfigSummary, s.ValidationMean*100, s.ValidationStd*100, s.SuccessRate*100),
		})
	}
	return renderBarChart("100-run mean validation accuracy", "higher is better", items, 0.0, 1.0)
}

func renderOverallSpeedChart(summaries []profileSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no sweep summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortProfileLabel(s.Profile.Name),
			Value: s.Best.InferenceThroughput,
			Title: fmt.Sprintf("%s | %s | infer=%.0f/s (%.2fms) | val=%.2f%%",
				s.Profile.Name, s.Best.ConfigSummary, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, s.Best.ValidationAccuracy*100),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	values := make([]float64, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}
	minV, maxV := minMax(values)
	return renderBarChart("Best inference throughput by model", "higher is better", items, minV, maxV)
}

func renderStabilitySpeedChart(summaries []repeatSummary) (string, error) {
	if len(summaries) == 0 {
		return "", fmt.Errorf("no stability summaries")
	}
	items := make([]barItem, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, barItem{
			Label: shortModelLabel(s.ModelType),
			Value: s.InferenceMean,
			Title: fmt.Sprintf("%s | %s | infer=%.0f/s ± %.0f/s | mean val=%.2f%%",
				s.Profile, s.ConfigSummary, s.InferenceMean, s.InferenceStd, s.ValidationMean*100),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	values := make([]float64, 0, len(items))
	for _, item := range items {
		values = append(values, item.Value)
	}
	minV, maxV := minMax(values)
	return renderBarChart("100-run mean inference throughput", "higher is better", items, minV, maxV)
}

func indexOf(xs []int, target int) int {
	for i, v := range xs {
		if v == target {
			return i
		}
	}
	return -1
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		" ", "-",
		"_", "-",
		"/", "-",
		"(", "",
		")", "",
	)
	return repl.Replace(s)
}

func shortModelLabel(mt ModelType) string {
	switch mt {
	case ModelRandomForest:
		return "RF"
	case ModelExtraTrees:
		return "ET"
	case ModelKNN:
		return "KNN"
	case ModelNaiveBayes:
		return "NB"
	case ModelAdaBoost:
		return "Ada"
	case ModelLogisticRegression:
		return "LR"
	case ModelSVM:
		return "SVM"
	case ModelRidge:
		return "Ridge"
	case ModelPerceptron:
		return "Perc"
	case ModelPassiveAggressive:
		return "PA"
	default:
		return string(mt)
	}
}

func shortProfileLabel(profile string) string {
	label := strings.ReplaceAll(strings.TrimSpace(profile), "_", " ")
	repl := strings.NewReplacer(
		"random forest", "RF",
		"extra trees", "ET",
		"nearest centroid cosine", "NC cos",
		"nearest centroid balanced", "NC bal",
		"nearest centroid", "NC",
		"logistic regression", "LR",
		"logistic balanced", "LR bal",
		"logistic", "LR",
		"passive aggressive", "PA",
		"passive aggressive balanced", "PA bal",
		"perceptron", "Perc",
		"perceptron balanced", "Perc bal",
		"knn", "KNN",
		"knn cosine", "KNN cos",
		"adaboost", "Ada",
		"naive bayes", "NB",
		"naive bayes balanced", "NB bal",
		"ensemble", "Ens",
	)
	return repl.Replace(label)
}

func colorForScore(v, minV, maxV float64) string {
	if maxV <= minV {
		return "#1890ff"
	}
	t := (v - minV) / (maxV - minV)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	red := [3]float64{245, 34, 45}
	yellow := [3]float64{250, 173, 20}
	green := [3]float64{82, 196, 26}
	var c [3]float64
	if t < 0.5 {
		u := t * 2
		for i := 0; i < 3; i++ {
			c[i] = red[i] + (yellow[i]-red[i])*u
		}
	} else {
		u := (t - 0.5) * 2
		for i := 0; i < 3; i++ {
			c[i] = yellow[i] + (green[i]-yellow[i])*u
		}
	}
	return fmt.Sprintf("#%02x%02x%02x", int(c[0]+0.5), int(c[1]+0.5), int(c[2]+0.5))
}

func contrastColor(fill string) string {
	if len(fill) != 7 || !strings.HasPrefix(fill, "#") {
		return "#111"
	}
	r, _ := strconv.ParseInt(fill[1:3], 16, 64)
	g, _ := strconv.ParseInt(fill[3:5], 16, 64)
	b, _ := strconv.ParseInt(fill[5:7], 16, 64)
	luma := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luma < 150 {
		return "#fff"
	}
	return "#111"
}

func ternary(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
