package main

import (
	"strconv"

	"github.com/stevessr/agent-ebpf-filter/desktop/renew/design"
)

// overviewMetricSpecs is a pure projection of the already-loaded bounded
// event summary. No new backend calls or security conclusions are made here.
func overviewMetricSpecs(events, attention, danger int) []design.Metric {
	riskTone, attentionTone := design.Neutral, design.Neutral
	if danger > 0 {
		riskTone = design.Danger
	}
	if attention > 0 {
		attentionTone = design.Warning
	}
	return []design.Metric{
		{
			Label: "摘要活动", Value: strconv.Itoa(events),
			Detail: "已加载窗口 · 非全部历史", Tone: design.Accent,
		},
		{
			Label: "高风险", Value: strconv.Itoa(danger),
			Detail: "当前摘要 · 请核查处置证据", Tone: riskTone,
		},
		{
			Label: "需关注", Value: strconv.Itoa(attention),
			Detail: "当前摘要 · 需人工研判", Tone: attentionTone,
		},
	}
}
