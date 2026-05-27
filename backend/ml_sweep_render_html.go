package main

import (
	"fmt"
	"html"
	"os"
	"sort"
	"strings"
	"time"
)

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
