package main

import (
	"fmt"
	"html"
	"math"
	"strings"
)

type barItem struct {
	Label string
	Value float64
	Title string
}

func renderProfileChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.ValidationAccuracy,
				Title: fmt.Sprintf("%s | %s | val=%.2f%%", profile.Name, r.ConfigSummary, r.ValidationAccuracy*100),
			})
		}
		maxV := 1.0
		return renderBarChart(profile.Name+" validation accuracy", profile.XName, items, 0, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.ValidationAccuracy
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\ntrain=%.2f%%\ninfer=%.0f/s (%.2fms)",
			r.ConfigSummary, r.ValidationAccuracy*100, r.TrainAccuracy*100, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderHeatmap(profile.Name+" validation accuracy", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderProfileDurationChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.Duration,
				Title: fmt.Sprintf("%s | %s | duration=%.2fs", profile.Name, r.ConfigSummary, r.Duration),
			})
		}
		minV, maxV := minMax(func() []float64 {
			values := make([]float64, 0, len(results))
			for _, r := range results {
				values = append(values, r.Duration)
			}
			return values
		}())
		return renderBarChart(profile.Name+" training duration", profile.XName, items, minV, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.Duration
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\nduration=%.2fs\ninfer=%.0f/s (%.2fms)",
			r.ConfigSummary, r.ValidationAccuracy*100, r.Duration, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderDurationHeatmap(profile.Name+" training duration", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderProfileInferenceChart(profile sweepProfile, results []sweepResult) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("no results for profile %s", profile.Name)
	}

	if profile.Kind == "bar" {
		items := make([]barItem, 0, len(results))
		for _, r := range results {
			items = append(items, barItem{
				Label: profile.XLabel(r.XValue),
				Value: r.InferenceThroughput,
				Title: fmt.Sprintf("%s | %s | infer=%.0f/s (%.2fms)", profile.Name, r.ConfigSummary, r.InferenceThroughput, r.InferenceLatencyMs),
			})
		}
		maxV := 0.0
		values := make([]float64, 0, len(results))
		for _, r := range results {
			values = append(values, r.InferenceThroughput)
		}
		_, maxV = minMax(values)
		return renderBarChart(profile.Name+" inference throughput", profile.XName, items, 0, maxV)
	}

	xLabels := make([]string, 0, len(profile.XValues))
	for _, x := range profile.XValues {
		xLabels = append(xLabels, profile.XLabel(x))
	}
	yLabels := make([]string, 0, len(profile.YValues))
	for _, y := range profile.YValues {
		yLabels = append(yLabels, profile.YLabel(y))
	}

	grid := make([][]float64, len(profile.YValues))
	notes := make([][]string, len(profile.YValues))
	for yi := range profile.YValues {
		grid[yi] = make([]float64, len(profile.XValues))
		notes[yi] = make([]string, len(profile.XValues))
	}
	for _, r := range results {
		xi := indexOf(profile.XValues, r.XValue)
		yi := indexOf(profile.YValues, r.YValue)
		if xi < 0 || yi < 0 {
			continue
		}
		grid[yi][xi] = r.InferenceThroughput
		notes[yi][xi] = fmt.Sprintf("%s\nval=%.2f%%\ntrain=%.2f%%\ninfer=%.0f/s\nlatency=%.2fms",
			r.ConfigSummary, r.ValidationAccuracy*100, r.TrainAccuracy*100, r.InferenceThroughput, r.InferenceLatencyMs)
	}
	return renderThroughputHeatmap(profile.Name+" inference throughput", profile.XName, profile.YName, xLabels, yLabels, grid, notes)
}

func renderBarChart(title, subtitle string, items []barItem, minV, maxV float64) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("empty bar chart")
	}
	width, height := 960, 420
	left, right, top, bottom := 80, 30, 60, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	maxVal := maxV
	if maxVal <= minV {
		maxVal = minV + 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.grid { stroke: #eee; stroke-width: 1; }
		.label { font-size: 12px; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.bar { fill: #1890ff; }
		.bar-best { fill: #52c41a; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	if subtitle != "" {
		fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">%s</text>`, left, html.EscapeString(subtitle))
	}
	for i := 0; i <= 5; i++ {
		v := minV + (maxVal-minV)*float64(i)/5.0
		y := float64(top) + plotH - (v-minV)/(maxVal-minV)*plotH
		fmt.Fprintf(&b, `<line class="grid" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, width-right, y, y)
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-8, y+4, fmt.Sprintf("%.0f%%", v*100))
	}
	fmt.Fprintf(&b, `<line class="axis" x1="%d" x2="%d" y1="%d" y2="%d"/>`, left, width-right, top+int(plotH), top+int(plotH))
	fmt.Fprintf(&b, `<line class="axis" x1="%d" x2="%d" y1="%d" y2="%d"/>`, left, left, top, top+int(plotH))

	barGap := 0.2
	barW := plotW / float64(len(items))
	bestIdx := 0
	bestVal := items[0].Value
	for i, item := range items {
		if item.Value > bestVal {
			bestVal = item.Value
			bestIdx = i
		}
	}
	for i, item := range items {
		x := float64(left) + float64(i)*barW + barW*barGap/2
		w := barW * (1 - barGap)
		h := 0.0
		if maxVal > minV {
			h = (item.Value - minV) / (maxVal - minV) * plotH
		}
		y := float64(top) + plotH - h
		fill := colorForScore(item.Value, minV, maxVal)
		if i == bestIdx {
			fill = "#52c41a"
		}
		fmt.Fprintf(&b, `<g><title>%s: %.2f%%</title><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" class="bar" fill="%s"/></g>`,
			html.EscapeString(item.Title), item.Value*100, x, y, w, h, fill)
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`,
			x+w/2, top+int(plotH)+22, html.EscapeString(item.Label))
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+w/2, y-6, fmt.Sprintf("%.1f%%", item.Value*100))
	}
	fmt.Fprintf(&b, `<text class="text label" x="%d" y="%d">%s</text>`, left, height-22, html.EscapeString(subtitle))
	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			fill := colorForScore(val, minV, maxV)
			highlight := ""
			if val >= maxV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.1f%%", val*100))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderDurationHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			score := maxV - val
			fill := colorForScore(score, 0, maxV-minV)
			highlight := ""
			if val <= minV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.2fs", val))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}

func renderThroughputHeatmap(title, xName, yName string, xLabels, yLabels []string, grid [][]float64, notes [][]string) (string, error) {
	if len(xLabels) == 0 || len(yLabels) == 0 {
		return "", fmt.Errorf("empty heatmap")
	}
	width, height := 980, 540
	left, right, top, bottom := 120, 30, 70, 90
	plotW := float64(width - left - right)
	plotH := float64(height - top - bottom)
	cellW := plotW / float64(len(xLabels))
	cellH := plotH / float64(len(yLabels))

	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, row := range grid {
		for _, v := range row {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	if math.IsNaN(minV) || math.IsNaN(maxV) || math.IsInf(minV, 0) || math.IsInf(maxV, 0) || maxV <= minV {
		minV = 0
		maxV = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`)
	fmt.Fprintf(&b, `<style>
		.text { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; fill: #222; }
		.axis { stroke: #999; stroke-width: 1; }
		.gridline { stroke: #eee; stroke-width: 1; }
		.cell { stroke: rgba(255,255,255,0.9); stroke-width: 1; }
		.title { font-size: 20px; font-weight: 700; }
		.subtitle { font-size: 12px; fill: #666; }
		.label { font-size: 12px; }
		.celltext { font-size: 11px; font-weight: 600; }
	</style>`)
	fmt.Fprintf(&b, `<text class="text title" x="%d" y="30">%s</text>`, left, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="text subtitle" x="%d" y="48">x=%s, y=%s</text>`, left, html.EscapeString(xName), html.EscapeString(yName))

	for xi, label := range xLabels {
		x := float64(left) + (float64(xi)+0.5)*cellW
		fmt.Fprintf(&b, `<text class="text label" x="%.1f" y="%d" text-anchor="middle">%s</text>`, x, top+int(plotH)+24, html.EscapeString(label))
	}
	for yi, label := range yLabels {
		y := float64(top) + (float64(yi)+0.5)*cellH
		fmt.Fprintf(&b, `<text class="text label" x="%d" y="%.1f" text-anchor="end">%s</text>`, left-10, y+4, html.EscapeString(label))
	}

	for xi := range xLabels {
		x := float64(left) + float64(xi)*cellW
		fmt.Fprintf(&b, `<line class="gridline" x1="%.1f" x2="%.1f" y1="%d" y2="%d"/>`, x, x, top, top+int(plotH))
	}
	for yi := range yLabels {
		y := float64(top) + float64(yi)*cellH
		fmt.Fprintf(&b, `<line class="gridline" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, left, left+int(plotW), y, y)
	}

	for yi, row := range grid {
		for xi, val := range row {
			x := float64(left) + float64(xi)*cellW
			y := float64(top) + float64(yi)*cellH
			fill := colorForScore(val, minV, maxV)
			highlight := ""
			if val >= maxV {
				highlight = ` stroke="#111" stroke-width="3"`
			}
			fmt.Fprintf(&b, `<g><title>%s</title><rect class="cell" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"%s/>`,
				html.EscapeString(notes[yi][xi]), x, y, cellW, cellH, fill, highlight)
			fmt.Fprintf(&b, `<text class="text celltext" x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text></g>`,
				x+cellW/2, y+cellH/2+4, contrastColor(fill), fmt.Sprintf("%.0f/s", val))
		}
	}

	fmt.Fprintf(&b, `</svg>`)
	return b.String(), nil
}
