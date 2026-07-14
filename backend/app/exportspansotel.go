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
	if !s.ready || s.tracer == nil {
		s.mu.Unlock()
		return hierarchy
	}
	var evicted otelSpanEvictions

	runKey := otelRunKey(envelope)
	taskKey := otelTaskKey(envelope, runKey)
	toolKey := otelToolKey(envelope, taskKey, runKey)

	if runKey != "" {
		if span := s.runSpans[runKey]; span != nil {
			touchActiveOTelSpan(s.runLRU, span, ts)
			hierarchy.run = span
		} else {
			for s.maxRunSpans > 0 && len(s.runSpans) >= s.maxRunSpans {
				before := len(s.runSpans)
				reclaimed := s.evictOldestRunLocked()
				s.noteCapacityEvictionsLocked(reclaimed)
				appendOTelSpanEvictions(&evicted, reclaimed)
				if len(s.runSpans) >= before {
					break
				}
			}
			ctx, span := s.tracer.Start(
				context.Background(),
				"agent.run",
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "run")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: runKey, runKey: runKey, lastSeen: ts}
			s.addRunSpanLocked(active)
			hierarchy.run = active
		}
	}

	if taskKey != "" {
		if span := s.taskSpans[taskKey]; span != nil {
			touchActiveOTelSpan(s.taskLRU, span, ts)
			hierarchy.task = span
		} else {
			for s.maxTaskSpans > 0 && len(s.taskSpans) >= s.maxTaskSpans {
				before := len(s.taskSpans)
				reclaimed := s.evictOldestTaskLocked()
				s.noteCapacityEvictionsLocked(reclaimed)
				appendOTelSpanEvictions(&evicted, reclaimed)
				if len(s.taskSpans) >= before {
					break
				}
			}
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
			s.addTaskSpanLocked(active)
			hierarchy.task = active
		}
	}

	if toolKey != "" {
		if span := s.toolSpans[toolKey]; span != nil {
			touchActiveOTelSpan(s.toolLRU, span, ts)
			hierarchy.tool = span
		} else {
			for s.maxToolSpans > 0 && len(s.toolSpans) >= s.maxToolSpans {
				before := len(s.toolSpans)
				reclaimed := s.evictOldestToolLocked()
				s.noteCapacityEvictionsLocked(reclaimed)
				appendOTelSpanEvictions(&evicted, reclaimed)
				if len(s.toolSpans) >= before {
					break
				}
			}
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
			s.addToolSpanLocked(active)
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
	s.mu.Unlock()

	// Span.End may synchronously invoke a configured span processor. Keep it
	// outside the state mutex so monitoring callbacks can safely update health.
	endOTelSpanEvictions(evicted, ts)
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
	s.processMu.Lock()
	defer s.processMu.Unlock()
	s.mu.Lock()
	toolSpans := collectIdleSpans(s.toolSpans, now, otelToolIdleTimeout)
	for _, span := range toolSpans {
		s.removeToolSpanLocked(span.key)
	}
	taskSpans := collectIdleTaskSpans(s.taskSpans, s.taskTools, now, otelTaskIdleTimeout)
	for _, span := range taskSpans {
		s.removeTaskSpanLocked(span.key)
	}
	runSpans := collectIdleRunSpans(s.runSpans, s.runTasks, s.runTools, now, otelRunIdleTimeout)
	for _, span := range runSpans {
		s.removeRunSpanLocked(span.key)
	}
	s.mu.Unlock()

	endSpanSlice(toolSpans, now)
	endSpanSlice(taskSpans, now)
	endSpanSlice(runSpans, now)
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
			s.removeToolSpanLocked(toolKey)
		}
	}
	if taskKey != "" && !hasOTelIndexedChildren(s.taskTools, taskKey) {
		if span := s.taskSpans[taskKey]; span != nil {
			spans = append(spans, span)
			s.removeTaskSpanLocked(taskKey)
		}
	}
	if runKey != "" && !hasOTelIndexedChildren(s.runTasks, runKey) && !hasOTelIndexedChildren(s.runTools, runKey) {
		if span := s.runSpans[runKey]; span != nil {
			spans = append(spans, span)
			s.removeRunSpanLocked(runKey)
		}
	}
	s.mu.Unlock()

	endSpanSlice(spans, ts)
}
