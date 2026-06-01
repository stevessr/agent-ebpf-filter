package main

import (
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func collectIdleSpans(spans map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range spans {
		if span == nil || now.Sub(span.lastSeen) < idleFor {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleTaskSpans(tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range tasks {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveToolForTask(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleRunSpans(runs, tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range runs {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveTaskForRun(tasks, span.key) || hasActiveToolForRun(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func hasActiveToolForTask(tools map[string]*activeOTelSpan, taskKey string) bool {
	for _, span := range tools {
		if span != nil && span.taskKey == taskKey {
			return true
		}
	}
	return false
}

func hasActiveTaskForRun(tasks map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tasks {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func hasActiveToolForRun(tools map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tools {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func endSpanMap(spans map[string]*activeOTelSpan, ts time.Time) {
	if len(spans) == 0 {
		return
	}
	list := make([]*activeOTelSpan, 0, len(spans))
	for _, span := range spans {
		if span != nil {
			list = append(list, span)
		}
	}
	endSpanSlice(list, ts)
}

func endSpanSlice(spans []*activeOTelSpan, ts time.Time) {
	for _, span := range spans {
		if span == nil || span.span == nil {
			continue
		}
		span.span.End(oteltrace.WithTimestamp(ts))
	}
}
