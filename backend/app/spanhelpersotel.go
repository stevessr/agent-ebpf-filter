package app

import (
	"container/list"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// ---- moved from backend/zz_merged_backend.go section spanhelpersotel.go ----

type otelSpanEvictions struct {
	tools []*activeOTelSpan
	tasks []*activeOTelSpan
	runs  []*activeOTelSpan
}

func (s *otelExporterState) resetActiveSpansLocked() {
	s.runSpans = make(map[string]*activeOTelSpan)
	s.taskSpans = make(map[string]*activeOTelSpan)
	s.toolSpans = make(map[string]*activeOTelSpan)
	s.runLRU = list.New()
	s.taskLRU = list.New()
	s.toolLRU = list.New()
	s.runTasks = make(map[string]map[string]struct{})
	s.runTools = make(map[string]map[string]struct{})
	s.taskTools = make(map[string]map[string]struct{})
}

func touchActiveOTelSpan(order *list.List, span *activeOTelSpan, ts time.Time) {
	if span == nil {
		return
	}
	if span.lastSeen.IsZero() || ts.After(span.lastSeen) {
		span.lastSeen = ts
	}
	if order == nil {
		return
	}
	if span.lruEntry == nil {
		span.lruEntry = order.PushBack(span)
		return
	}
	order.MoveToBack(span.lruEntry)
}

func addActiveOTelSpan(spans map[string]*activeOTelSpan, order *list.List, span *activeOTelSpan) {
	if spans == nil || span == nil || span.key == "" {
		return
	}
	spans[span.key] = span
	touchActiveOTelSpan(order, span, span.lastSeen)
}

func removeActiveOTelSpan(spans map[string]*activeOTelSpan, order *list.List, key string) *activeOTelSpan {
	span := spans[key]
	if span == nil {
		return nil
	}
	delete(spans, key)
	if order != nil && span.lruEntry != nil {
		order.Remove(span.lruEntry)
	}
	span.lruEntry = nil
	return span
}

func addOTelChildIndex(index map[string]map[string]struct{}, parentKey, childKey string) {
	if index == nil || parentKey == "" || childKey == "" {
		return
	}
	children := index[parentKey]
	if children == nil {
		children = make(map[string]struct{})
		index[parentKey] = children
	}
	children[childKey] = struct{}{}
}

func removeOTelChildIndex(index map[string]map[string]struct{}, parentKey, childKey string) {
	if index == nil || parentKey == "" || childKey == "" {
		return
	}
	children := index[parentKey]
	delete(children, childKey)
	if len(children) == 0 {
		delete(index, parentKey)
	}
}

func hasOTelIndexedChildren(index map[string]map[string]struct{}, parentKey string) bool {
	return len(index[parentKey]) > 0
}

func (s *otelExporterState) addRunSpanLocked(span *activeOTelSpan) {
	addActiveOTelSpan(s.runSpans, s.runLRU, span)
}

func (s *otelExporterState) addTaskSpanLocked(span *activeOTelSpan) {
	addActiveOTelSpan(s.taskSpans, s.taskLRU, span)
	if span != nil {
		addOTelChildIndex(s.runTasks, span.runKey, span.key)
	}
}

func (s *otelExporterState) addToolSpanLocked(span *activeOTelSpan) {
	addActiveOTelSpan(s.toolSpans, s.toolLRU, span)
	if span != nil {
		addOTelChildIndex(s.runTools, span.runKey, span.key)
		addOTelChildIndex(s.taskTools, span.taskKey, span.key)
	}
}

func (s *otelExporterState) removeRunSpanLocked(key string) *activeOTelSpan {
	span := removeActiveOTelSpan(s.runSpans, s.runLRU, key)
	if span != nil {
		delete(s.runTasks, span.key)
		delete(s.runTools, span.key)
	}
	return span
}

func (s *otelExporterState) removeTaskSpanLocked(key string) *activeOTelSpan {
	span := removeActiveOTelSpan(s.taskSpans, s.taskLRU, key)
	if span != nil {
		removeOTelChildIndex(s.runTasks, span.runKey, span.key)
		delete(s.taskTools, span.key)
	}
	return span
}

func (s *otelExporterState) removeToolSpanLocked(key string) *activeOTelSpan {
	span := removeActiveOTelSpan(s.toolSpans, s.toolLRU, key)
	if span != nil {
		removeOTelChildIndex(s.runTools, span.runKey, span.key)
		removeOTelChildIndex(s.taskTools, span.taskKey, span.key)
	}
	return span
}

func oldestActiveOTelSpan(order *list.List, spans map[string]*activeOTelSpan) *activeOTelSpan {
	if order != nil && order.Front() != nil {
		span, _ := order.Front().Value.(*activeOTelSpan)
		if span != nil {
			return span
		}
	}
	// The map scan is a defensive fallback for zero-value or test-constructed
	// state. Normal runtime state always has the O(1) LRU index populated.
	var oldest *activeOTelSpan
	for _, span := range spans {
		if span == nil {
			continue
		}
		if oldest == nil || span.lastSeen.Before(oldest.lastSeen) ||
			(span.lastSeen.Equal(oldest.lastSeen) && span.key < oldest.key) {
			oldest = span
		}
	}
	return oldest
}

func (s *otelExporterState) evictOldestToolLocked() otelSpanEvictions {
	var evicted otelSpanEvictions
	if oldest := oldestActiveOTelSpan(s.toolLRU, s.toolSpans); oldest != nil {
		if span := s.removeToolSpanLocked(oldest.key); span != nil {
			evicted.tools = append(evicted.tools, span)
		}
	}
	return evicted
}

func (s *otelExporterState) evictOldestTaskLocked() otelSpanEvictions {
	var evicted otelSpanEvictions
	oldest := oldestActiveOTelSpan(s.taskLRU, s.taskSpans)
	if oldest == nil {
		return evicted
	}
	for toolKey := range s.taskTools[oldest.key] {
		if removed := s.removeToolSpanLocked(toolKey); removed != nil {
			evicted.tools = append(evicted.tools, removed)
		}
	}
	if span := s.removeTaskSpanLocked(oldest.key); span != nil {
		evicted.tasks = append(evicted.tasks, span)
	}
	return evicted
}

func (s *otelExporterState) evictOldestRunLocked() otelSpanEvictions {
	var evicted otelSpanEvictions
	oldest := oldestActiveOTelSpan(s.runLRU, s.runSpans)
	if oldest == nil {
		return evicted
	}
	for toolKey := range s.runTools[oldest.key] {
		if removed := s.removeToolSpanLocked(toolKey); removed != nil {
			evicted.tools = append(evicted.tools, removed)
		}
	}
	for taskKey := range s.runTasks[oldest.key] {
		if removed := s.removeTaskSpanLocked(taskKey); removed != nil {
			evicted.tasks = append(evicted.tasks, removed)
		}
	}
	if span := s.removeRunSpanLocked(oldest.key); span != nil {
		evicted.runs = append(evicted.runs, span)
	}
	return evicted
}

func (s *otelExporterState) noteCapacityEvictionsLocked(evicted otelSpanEvictions) {
	s.evictedToolSpans += uint64(len(evicted.tools))
	s.evictedTaskSpans += uint64(len(evicted.tasks))
	s.evictedRunSpans += uint64(len(evicted.runs))
}

func appendOTelSpanEvictions(dst *otelSpanEvictions, src otelSpanEvictions) {
	if dst == nil {
		return
	}
	dst.tools = append(dst.tools, src.tools...)
	dst.tasks = append(dst.tasks, src.tasks...)
	dst.runs = append(dst.runs, src.runs...)
}

func endOTelSpanEvictions(evicted otelSpanEvictions, ts time.Time) {
	endSpanSlice(evicted.tools, ts)
	endSpanSlice(evicted.tasks, ts)
	endSpanSlice(evicted.runs, ts)
}

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

func collectIdleTaskSpans(tasks map[string]*activeOTelSpan, taskTools map[string]map[string]struct{}, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range tasks {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasOTelIndexedChildren(taskTools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleRunSpans(runs map[string]*activeOTelSpan, runTasks, runTools map[string]map[string]struct{}, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range runs {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasOTelIndexedChildren(runTasks, span.key) || hasOTelIndexedChildren(runTools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
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
		endAt := ts
		if span.lastSeen.After(endAt) {
			endAt = span.lastSeen
		}
		span.span.End(oteltrace.WithTimestamp(endAt))
	}
}
