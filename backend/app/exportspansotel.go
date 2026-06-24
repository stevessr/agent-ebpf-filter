package app

import (
	"agent-ebpf-filter/pb"
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ---- moved from backend/zz_merged_backend.go section exportspansotel.go ----

type otelSpanHierarchy struct {
	run  *activeOTelSpan
	task *activeOTelSpan
	tool *activeOTelSpan
}

func (s *otelExporterState) ensureSpanHierarchy(envelope *pb.EventEnvelope, ts time.Time) otelSpanHierarchy {
	var hierarchy otelSpanHierarchy
	if s == nil || envelope == nil {
		return hierarchy
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready || s.tracer == nil {
		return hierarchy
	}

	runKey := otelRunKey(envelope)
	taskKey := otelTaskKey(envelope, runKey)
	toolKey := otelToolKey(envelope, taskKey, runKey)

	if runKey != "" {
		if span := s.runSpans[runKey]; span != nil {
			span.lastSeen = ts
			hierarchy.run = span
		} else {
			ctx, span := s.tracer.Start(
				context.Background(),
				"agent.run",
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "run")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: runKey, runKey: runKey, lastSeen: ts}
			s.runSpans[runKey] = active
			hierarchy.run = active
		}
	}

	if taskKey != "" {
		if span := s.taskSpans[taskKey]; span != nil {
			span.lastSeen = ts
			hierarchy.task = span
		} else {
			parentCtx := context.Background()
			if hierarchy.run != nil {
				parentCtx = hierarchy.run.ctx
			}
			ctx, span := s.tracer.Start(
				parentCtx,
				"codex.task",
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "task")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: taskKey, runKey: runKey, taskKey: taskKey, lastSeen: ts}
			s.taskSpans[taskKey] = active
			hierarchy.task = active
		}
	}

	if toolKey != "" {
		if span := s.toolSpans[toolKey]; span != nil {
			span.lastSeen = ts
			hierarchy.tool = span
		} else {
			parentCtx := context.Background()
			if hierarchy.task != nil {
				parentCtx = hierarchy.task.ctx
			} else if hierarchy.run != nil {
				parentCtx = hierarchy.run.ctx
			}
			ctx, span := s.tracer.Start(
				parentCtx,
				otelToolSpanName(envelope),
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "tool")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: toolKey, runKey: runKey, taskKey: taskKey, lastSeen: ts}
			s.toolSpans[toolKey] = active
			hierarchy.tool = active
		}
	}

	if hierarchy.task == nil && hierarchy.tool != nil {
		hierarchy.task = s.taskSpans[hierarchy.tool.taskKey]
	}
	if hierarchy.run == nil {
		switch {
		case hierarchy.tool != nil:
			hierarchy.run = s.runSpans[hierarchy.tool.runKey]
		case hierarchy.task != nil:
			hierarchy.run = s.runSpans[hierarchy.task.runKey]
		}
	}
	return hierarchy
}

func (s *otelExporterState) shouldCreateChildSpan(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	switch envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent, *pb.EventEnvelope_NetworkEvent, *pb.EventEnvelope_ProcessEvent, *pb.EventEnvelope_McpEvent, *pb.EventEnvelope_WrapperEvent, *pb.EventEnvelope_HookEvent:
		return true
	case *pb.EventEnvelope_FileEvent:
		return strings.HasPrefix(otelEventName(envelope), "file.")
	case *pb.EventEnvelope_PolicyEvent:
		return strings.TrimSpace(envelope.GetPolicyDecision()) != "" || envelope.GetRiskScore() > 0
	default:
		return false
	}
}

func (s *otelExporterState) createChildSpan(parentCtx context.Context, spanName string, envelope *pb.EventEnvelope, attrs []attribute.KeyValue, ts time.Time) {
	if s == nil || envelope == nil {
		return
	}
	s.mu.RLock()
	tracer := s.tracer
	ready := s.ready
	s.mu.RUnlock()
	if !ready || tracer == nil {
		return
	}
	endTs := ts.Add(time.Nanosecond)
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetDurationNs() > 0 {
		endTs = ts.Add(time.Duration(legacy.GetDurationNs()))
	}
	_, span := tracer.Start(parentCtx, spanName, oteltrace.WithTimestamp(ts), oteltrace.WithAttributes(attrs...))
	if shouldMarkSpanError(envelope) {
		span.SetStatus(codes.Error, otelStatusMessage(envelope))
	}
	span.End(oteltrace.WithTimestamp(endTs))
}

func (s *otelExporterState) endIdleSpans(now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	toolSpans := collectIdleSpans(s.toolSpans, now, otelToolIdleTimeout)
	for _, span := range toolSpans {
		delete(s.toolSpans, span.key)
	}
	taskSpans := collectIdleTaskSpans(s.taskSpans, s.toolSpans, now, otelTaskIdleTimeout)
	for _, span := range taskSpans {
		delete(s.taskSpans, span.key)
	}
	runSpans := collectIdleRunSpans(s.runSpans, s.taskSpans, s.toolSpans, now, otelRunIdleTimeout)
	for _, span := range runSpans {
		delete(s.runSpans, span.key)
	}
	tp := s.tp
	s.mu.Unlock()

	endSpanSlice(toolSpans, now)
	endSpanSlice(taskSpans, now)
	endSpanSlice(runSpans, now)
	if tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = tp.ForceFlush(ctx)
		cancel()
	}
}

func (s *otelExporterState) endRelatedSpans(envelope *pb.EventEnvelope, ts time.Time) {
	if s == nil || envelope == nil {
		return
	}
	runKey := otelRunKey(envelope)
	taskKey := otelTaskKey(envelope, runKey)
	toolKey := otelToolKey(envelope, taskKey, runKey)

	s.mu.Lock()
	var spans []*activeOTelSpan
	if toolKey != "" {
		if span := s.toolSpans[toolKey]; span != nil {
			spans = append(spans, span)
			delete(s.toolSpans, toolKey)
		}
	}
	if taskKey != "" && !hasActiveToolForTask(s.toolSpans, taskKey) {
		if span := s.taskSpans[taskKey]; span != nil {
			spans = append(spans, span)
			delete(s.taskSpans, taskKey)
		}
	}
	if runKey != "" && !hasActiveTaskForRun(s.taskSpans, runKey) && !hasActiveToolForRun(s.toolSpans, runKey) {
		if span := s.runSpans[runKey]; span != nil {
			spans = append(spans, span)
			delete(s.runSpans, runKey)
		}
	}
	tp := s.tp
	s.mu.Unlock()

	endSpanSlice(spans, ts)
	if tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = tp.ForceFlush(ctx)
		cancel()
	}
}
