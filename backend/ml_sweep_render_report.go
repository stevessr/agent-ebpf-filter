package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

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

type sweepCoverage struct {
	Summary  map[string]any         `json:"summary"`
	Datasets []map[string]any       `json:"datasets"`
	Profiles []sweepCoverageProfile `json:"profiles"`
}

type sweepCoverageProfile struct {
	Dataset                  string `json:"dataset"`
	Profile                  string `json:"profile"`
	ModelType                string `json:"modelType"`
	Parameter                string `json:"parameter"`
	ParameterKind            string `json:"parameterKind"`
	RequiredDiscretePoints   int    `json:"requiredDiscretePoints"`
	ConfiguredDiscretePoints int    `json:"configuredDiscretePoints"`
	TestedRows               int    `json:"testedRows"`
	Passed                   bool   `json:"passed"`
}

func buildSweepCoverage(datasets []sweepDataset, profiles []sweepProfile, results []sweepResult, pointsPerParam int) sweepCoverage {
	rowCounts := make(map[string]int)
	for _, result := range results {
		rowCounts[result.Profile]++
	}
	entries := make([]sweepCoverageProfile, 0, len(datasets)*len(profiles))
	passed := 0
	required := 0
	for _, dataset := range datasets {
		for _, profile := range profiles {
			scoped := profileForDataset(profile, dataset)
			configured := configuredProfilePointCount(profile)
			req := profile.RequiredDiscretePoints
			if req < 1 {
				req = configured
			}
			ok := rowCounts[scoped.Name] >= configured && configured >= req
			if profile.ParameterKind == "categorical" || profile.ParameterKind == "fixed" {
				ok = rowCounts[scoped.Name] >= configured && configured == req
			}
			required++
			if ok {
				passed++
			}
			entries = append(entries, sweepCoverageProfile{
				Dataset:                  dataset.Name,
				Profile:                  scoped.Name,
				ModelType:                string(profile.ModelType),
				Parameter:                profile.ParameterName,
				ParameterKind:            profile.ParameterKind,
				RequiredDiscretePoints:   req,
				ConfiguredDiscretePoints: configured,
				TestedRows:               rowCounts[scoped.Name],
				Passed:                   ok,
			})
		}
	}
	datasetRows := make([]map[string]any, 0, len(datasets))
	for _, dataset := range datasets {
		datasetRows = append(datasetRows, map[string]any{
			"name":        dataset.Name,
			"description": dataset.Description,
			"samples":     len(dataset.Samples),
		})
	}
	return sweepCoverage{
		Summary: map[string]any{
			"datasets":               len(datasets),
			"profiles":               len(profiles),
			"coverageEntries":        required,
			"passedEntries":          passed,
			"pointsPerParam":         pointsPerParam,
			"numericRequirementNote": "numeric comprehensive axis profiles require at least pointsPerParam unique tested values per tunable parameter; categorical/fixed profiles enumerate all meaningful values",
		},
		Datasets: datasetRows,
		Profiles: entries,
	}
}

func writeCoverageJSON(path string, coverage sweepCoverage) error {
	data, err := json.MarshalIndent(coverage, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func uniqueIntCount(values []int) int {
	if len(values) == 0 {
		return 0
	}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	return len(seen)
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

func writeReportHTML(path string, summaries []profileSummary, repeats []repeatSummary, repeatCount, stabilityTop int) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>ML Sweep Report</title>`)
	b.WriteString(`<style>
		body { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 24px; color: #222; }
		h1, h2, h3 { margin: 0.2em 0 0.4em; }
		p, li { line-height: 1.5; }
		table { border-collapse: collapse; width: 100%; margin: 16px 0 28px; }
		th, td { border: 1px solid #ddd; padding: 8px 10px; vertical-align: top; }
		th { background: #fafafa; text-align: left; position: sticky; top: 0; }
		.small { color: #666; font-size: 12px; }
		.card { border: 1px solid #e8e8e8; border-radius: 10px; padding: 16px; margin: 20px 0; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
		.chart { max-width: 100%; overflow-x: auto; }
		.chart-row { display: flex; gap: 16px; flex-wrap: wrap; }
		.chart-row .chart { flex: 1 1 440px; }
		code { background: #f6f8fa; padding: 2px 4px; border-radius: 4px; }
	</style></head><body>`)

	best := bestScreenSummary(summaries)
	if best == nil {
		return fmt.Errorf("no sweep summaries")
	}
	stabilityBest := bestComparableSummary(repeats)

	fmt.Fprintf(&b, `<h1>ML Sweep Report</h1>`)
	fmt.Fprintf(&b, `<p class="small">Generated at %s. Results are based on the persisted local training store used by the running backend.</p>`, html.EscapeString(time.Now().Format(time.RFC3339)))
	fmt.Fprintf(&b, `<div class="card"><h2>Grid best</h2><p><b>%s</b> — %s — validation <b>%.2f%%</b>, ALLOW pass <b>%.2f%%</b>, train <b>%.2f%%</b>, infer <b>%.0f/s</b> (%.2fms)</p><p class="small">Charts: <code>overall_best.svg</code> and <code>overall_speed.svg</code>; raw CSV: <code>results.csv</code>; JSON summary: <code>best.json</code></p><div class="chart-row"><div class="chart"><img src="overall_best.svg" alt="Overall best chart" style="max-width:100%%;height:auto"></div><div class="chart"><img src="overall_speed.svg" alt="Overall speed chart" style="max-width:100%%;height:auto"></div></div></div>`,
		html.EscapeString(best.Profile.Name), html.EscapeString(best.Best.ConfigSummary), best.Best.ValidationAccuracy*100, best.Best.AllowPassRate*100, best.Best.TrainAccuracy*100, best.Best.InferenceThroughput, best.Best.InferenceLatencyMs)

	if stabilityBest != nil {
		fmt.Fprintf(&b, `<div class="card"><h2>100-run stability best</h2><p><b>%s</b> — %s — mean validation <b>%.2f%%</b> ± <b>%.2f%%</b>, mean ALLOW pass <b>%.2f%%</b> ± <b>%.2f%%</b>; mean speed <b>%.0f/s</b> ± <b>%.0f/s</b> across %d runs</p><p class="small">Charts: <code>stability_best.svg</code> and <code>stability_speed.svg</code>; raw runs: <code>stability-runs.csv</code>; summary CSV: <code>stability-summary.csv</code></p><div class="chart-row"><div class="chart"><img src="stability_best.svg" alt="Stability chart" style="max-width:100%%;height:auto"></div><div class="chart"><img src="stability_speed.svg" alt="Stability speed chart" style="max-width:100%%;height:auto"></div></div></div>`,
			html.EscapeString(stabilityBest.Profile), html.EscapeString(stabilityBest.ConfigSummary), stabilityBest.ValidationMean*100, stabilityBest.ValidationStd*100, stabilityBest.AllowMean*100, stabilityBest.AllowStd*100, stabilityBest.InferenceMean, stabilityBest.InferenceStd, repeatCount)
	}

	if best != nil {
		bf := slug(best.Profile.Name)
		paramRows := append([]sweepResult(nil), best.Results...)
		sort.Slice(paramRows, func(i, j int) bool {
			if paramRows[i].ValidationAccuracy != paramRows[j].ValidationAccuracy {
				return paramRows[i].ValidationAccuracy > paramRows[j].ValidationAccuracy
			}
			if paramRows[i].InferenceThroughput != paramRows[j].InferenceThroughput {
				return paramRows[i].InferenceThroughput > paramRows[j].InferenceThroughput
			}
			if paramRows[i].Duration != paramRows[j].Duration {
				return paramRows[i].Duration < paramRows[j].Duration
			}
			if paramRows[i].XValue != paramRows[j].XValue {
				return paramRows[i].XValue < paramRows[j].XValue
			}
			return paramRows[i].YValue < paramRows[j].YValue
		})
		fmt.Fprintf(&b, `<div class="card"><h2>Best model parameter sweep</h2><p><b>%s</b> — grid best <b>%s</b>. The charts below show <b>validation accuracy</b>, <b>training duration</b>, <b>inference throughput</b>, and <b>ALLOW pass rate</b> for every tested parameter point.</p><p class="small">Artifacts: <code>%s.svg</code>, <code>%s-duration.svg</code>, <code>%s-inference.svg</code>, <code>%s-grid.csv</code></p><div class="chart-row"><div class="chart"><img src="%s.svg" alt="%s validation heatmap" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-duration.svg" alt="%s duration heatmap" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-inference.svg" alt="%s inference heatmap" style="max-width:100%%;height:auto"></div></div>`,
			html.EscapeString(best.Profile.Name), html.EscapeString(best.Best.ConfigSummary), bf, bf, bf, bf, bf, html.EscapeString(best.Profile.Name), bf, html.EscapeString(best.Profile.Name), bf, html.EscapeString(best.Profile.Name))
		fmt.Fprintf(&b, `<table><thead><tr><th>Config</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Duration</th><th>Infer/s</th><th>Latency</th><th>X</th><th>Y</th></tr></thead><tbody>`)
		for _, r := range paramRows {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2fs</td><td>%.0f/s</td><td>%.2fms</td><td>%d</td><td>%d</td></tr>`,
				html.EscapeString(r.ConfigSummary), r.TrainAccuracy*100, r.ValidationAccuracy*100, r.AllowPassRate*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs, r.XValue, r.YValue)
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<h2>Profile details</h2>`)
	for _, s := range summaries {
		fmt.Fprintf(&b, `<div class="card"><h3>%s</h3>`, html.EscapeString(s.Profile.Name))
		fmt.Fprintf(&b, `<p class="small">Best grid point: <b>%s</b> — validation <b>%.2f%%</b> / ALLOW pass <b>%.2f%%</b> / train <b>%.2f%%</b> / infer <b>%.0f/s</b> (%.2fms) (%s)</p>`,
			html.EscapeString(s.Best.ConfigSummary), s.Best.ValidationAccuracy*100, s.Best.AllowPassRate*100, s.Best.TrainAccuracy*100, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, ternary(s.Profile.Comparable, "holdout-comparable", "train-set / optimistic"))
		fmt.Fprintf(&b, `<div class="chart-row"><div class="chart"><img src="%s.svg" alt="%s" style="max-width:100%%;height:auto"></div><div class="chart"><img src="%s-inference.svg" alt="%s inference" style="max-width:100%%;height:auto"></div></div>`, slug(s.Profile.Name), html.EscapeString(s.Profile.Name), slug(s.Profile.Name), html.EscapeString(s.Profile.Name))
		topRows := append([]sweepResult(nil), s.Results...)
		sort.Slice(topRows, func(i, j int) bool {
			if topRows[i].ValidationAccuracy != topRows[j].ValidationAccuracy {
				return topRows[i].ValidationAccuracy > topRows[j].ValidationAccuracy
			}
			if topRows[i].AllowPassRate != topRows[j].AllowPassRate {
				return topRows[i].AllowPassRate > topRows[j].AllowPassRate
			}
			if topRows[i].InferenceThroughput != topRows[j].InferenceThroughput {
				return topRows[i].InferenceThroughput > topRows[j].InferenceThroughput
			}
			return topRows[i].Duration < topRows[j].Duration
		})
		if len(topRows) > 5 {
			topRows = topRows[:5]
		}
		fmt.Fprintf(&b, `<table><thead><tr><th>Config</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Duration</th><th>Infer/s</th><th>Latency</th><th>Error</th></tr></thead><tbody>`)
		for _, r := range topRows {
			fmt.Fprintf(&b, `<tr><td><code>%s</code></td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2fs</td><td>%.0f/s</td><td>%.2fms</td><td>%s</td></tr>`,
				html.EscapeString(r.ConfigSummary), r.TrainAccuracy*100, r.ValidationAccuracy*100, r.AllowPassRate*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs, html.EscapeString(r.Error))
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<div class="card"><h2>Grid summary</h2><table><thead><tr><th>Model</th><th>Best config</th><th>Comparable</th><th>Train</th><th>Validation</th><th>ALLOW pass</th><th>Infer/s</th><th>Latency</th><th>Runs</th></tr></thead><tbody>`)
	for _, s := range summaries {
		fmt.Fprintf(&b, `<tr><td>%s</td><td><code>%s</code></td><td>%s</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.0f/s</td><td>%.2fms</td><td>%d</td></tr>`,
			html.EscapeString(s.Profile.Name), html.EscapeString(s.Best.ConfigSummary), ternary(s.Profile.Comparable, "yes", "no"), s.Best.TrainAccuracy*100, s.Best.ValidationAccuracy*100, s.Best.AllowPassRate*100, s.Best.InferenceThroughput, s.Best.InferenceLatencyMs, len(s.Results))
	}
	fmt.Fprintf(&b, `</tbody></table></div>`)

	if len(repeats) > 0 {
		fmt.Fprintf(&b, `<div class="card"><h2>100-run stability summary</h2><table><thead><tr><th>Model</th><th>Config</th><th>Comparable</th><th>Mean val</th><th>Std val</th><th>Mean ALLOW</th><th>Std ALLOW</th><th>Mean speed</th><th>Std speed</th><th>Success</th><th>Runs</th></tr></thead><tbody>`)
		for _, s := range repeats {
			fmt.Fprintf(&b, `<tr><td>%s</td><td><code>%s</code></td><td>%s</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.2f%%</td><td>%.0f/s</td><td>%.0f/s</td><td>%.0f%%</td><td>%d</td></tr>`,
				html.EscapeString(s.Profile), html.EscapeString(s.ConfigSummary), ternary(s.Comparable, "yes", "no"), s.ValidationMean*100, s.ValidationStd*100, s.AllowMean*100, s.AllowStd*100, s.InferenceMean, s.InferenceStd, s.SuccessRate*100, s.Runs)
		}
		fmt.Fprintf(&b, `</tbody></table></div>`)
	}

	fmt.Fprintf(&b, `<div class="card"><h2>Notes</h2><ul>`)
	fmt.Fprintf(&b, `<li><code>random_forest</code> / <code>extra_trees</code> sweep trees × depth with leaf fixed at 3.</li>`)
	fmt.Fprintf(&b, `<li><code>logistic</code> uses <code>numTrees</code> as learning-rate × 1000 and <code>maxDepth</code> as regularization selector.</li>`)
	fmt.Fprintf(&b, `<li><code>svm</code>, <code>perceptron</code>, and <code>passive_aggressive</code> use <code>numTrees</code> as learning-rate × 1000 and <code>minSamplesLeaf</code> as iterations.</li>`)
	fmt.Fprintf(&b, `<li>Phase 1 runs a horizontal grid sweep; phase 2 repeats each profile's top <code>%d</code> grid point(s) <code>%d</code> times for stability.</li>`, stabilityTop, repeatCount)
	fmt.Fprintf(&b, `<li>Inference speed is benchmarked on a fixed cached sample slice from the persisted dataset, so throughput and latency are comparable across all families.</li>`)
	fmt.Fprintf(&b, `<li><code>random_forest</code>, <code>extra_trees</code>, <code>logistic</code>, <code>svm</code>, <code>perceptron</code>, <code>passive_aggressive</code>, and <code>nearest_centroid</code> are holdout-comparable in this repo; <code>knn</code>, <code>ridge</code>, <code>adaboost</code>, and <code>naive_bayes</code> currently report training-set-based scores in their trainers.</li>`)
	fmt.Fprintf(&b, `<li>We now track <strong>ALLOW pass rate</strong> alongside overall accuracy so the sweep does not over-optimize on catching bad commands while accidentally blocking good ones.</li>`)
	fmt.Fprintf(&b, `<li>The sweep runs offline against the persisted dataset, so it does not require the live backend to be free.</li>`)
	fmt.Fprintf(&b, `</ul></div>`)

	fmt.Fprintf(&b, `</body></html>`)
	return os.WriteFile(path, []byte(b.String()), 0o644)
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
