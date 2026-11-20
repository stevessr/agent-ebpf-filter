package app

import (
	"agent-ebpf-filter/pb"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (w *loopDetectionWorker) maybeEvictExpiredLoopDetectionWindowsLocked(now time.Time, windowDuration time.Duration) {
	if now.IsZero() || windowDuration <= 0 {
		return
	}
	interval := windowDuration
	if interval > loopDetectionWindowGCMaxInterval {
		interval = loopDetectionWindowGCMaxInterval
	}
	if !w.lastWindowGC.IsZero() && now.Sub(w.lastWindowGC) < interval {
		return
	}
	w.lastWindowGC = now
	w.windowGCRuns++
	for key, win := range w.windows {
		if win == nil {
			delete(w.windows, key)
			w.windowEvicted++
			continue
		}
		if !win.LastSeen.IsZero() && now.Sub(win.LastSeen) > windowDuration*2 {
			w.removeLoopDetectionWindowLocked(win)
		}
	}
}

func (w *loopDetectionWorker) enforceLoopDetectionWindowCapacityLocked(maxContexts int) {
	if maxContexts <= 0 {
		maxContexts = loopDetectionDefaultMaxContexts
	}
	for len(w.windows) > maxContexts && w.windowLRUHead != nil {
		w.removeLoopDetectionWindowLocked(w.windowLRUHead)
	}
}

func (w *loopDetectionWorker) appendLoopDetectionWindowLocked(win *loopDetectionWindow) {
	if win == nil {
		return
	}
	win.lruPrev = w.windowLRUTail
	win.lruNext = nil
	if w.windowLRUTail == nil {
		w.windowLRUHead = win
	} else {
		w.windowLRUTail.lruNext = win
	}
	w.windowLRUTail = win
}

func (w *loopDetectionWorker) touchLoopDetectionWindowLocked(win *loopDetectionWindow) {
	if win == nil || w.windowLRUTail == win {
		return
	}
	w.detachLoopDetectionWindowLocked(win)
	w.appendLoopDetectionWindowLocked(win)
}

func (w *loopDetectionWorker) removeLoopDetectionWindowLocked(win *loopDetectionWindow) {
	if win == nil {
		return
	}
	if current := w.windows[win.key]; current == win {
		delete(w.windows, win.key)
	}
	w.detachLoopDetectionWindowLocked(win)
	w.windowEvicted++
}

func (w *loopDetectionWorker) detachLoopDetectionWindowLocked(win *loopDetectionWindow) {
	if win == nil {
		return
	}
	if win.lruPrev == nil {
		if w.windowLRUHead == win {
			w.windowLRUHead = win.lruNext
		}
	} else {
		win.lruPrev.lruNext = win.lruNext
	}
	if win.lruNext == nil {
		if w.windowLRUTail == win {
			w.windowLRUTail = win.lruPrev
		}
	} else {
		win.lruNext.lruPrev = win.lruPrev
	}
	win.lruPrev = nil
	win.lruNext = nil
}

func (w *loopDetectionWorker) appendFindingLocked(finding loopDetectionFinding) {
	w.findingsTotal++
	w.findings = append(w.findings, finding)
	if len(w.findings) > loopDetectionFindingLimit {
		copy(w.findings, w.findings[len(w.findings)-loopDetectionFindingLimit:])
		w.findings = w.findings[:loopDetectionFindingLimit]
	}
}

func newLoopDetectionWindow(contextType, contextKey, fingerprint, target string, event *pb.Event, observedAt time.Time, windowSeconds int) *loopDetectionWindow {
	win := &loopDetectionWindow{
		FirstSeen:     observedAt,
		LastSeen:      observedAt,
		Count:         1,
		ContextType:   contextType,
		ContextKey:    contextKey,
		Fingerprint:   fingerprint,
		Target:        strings.Clone(target),
		AgentRunID:    cloneBoundedLoopDetectionString(event.GetAgentRunId(), loopDetectionContextComponentBytes),
		TaskID:        cloneBoundedLoopDetectionString(event.GetTaskId(), loopDetectionContextComponentBytes),
		ToolCallID:    cloneBoundedLoopDetectionString(event.GetToolCallId(), loopDetectionContextComponentBytes),
		TraceID:       cloneBoundedLoopDetectionString(event.GetTraceId(), loopDetectionContextComponentBytes),
		RootAgentPID:  event.GetRootAgentPid(),
		PID:           event.GetPid(),
		Comm:          cloneBoundedLoopDetectionString(event.GetComm(), loopDetectionMetadataValueBytes),
		WindowSeconds: windowSeconds,
	}
	mergeLoopDetectionWindow(win, event, target)
	return win
}

func mergeLoopDetectionWindow(win *loopDetectionWindow, event *pb.Event, target string) {
	if win == nil || event == nil {
		return
	}
	if eventType := normalizedLoopEventType(event); eventType != "" {
		win.EventTypes = addLoopDetectionString(win.EventTypes, eventType)
	}
	if pid := event.GetPid(); pid != 0 {
		win.Pids = addBoundedLoopPID(win.Pids, pid)
		if win.PID == 0 {
			win.PID = pid
		}
	}
	if comm := strings.TrimSpace(event.GetComm()); comm != "" {
		comm = boundLoopDetectionString(comm, loopDetectionMetadataValueBytes)
		win.Comms = addLoopDetectionString(win.Comms, comm)
		if win.Comm == "" {
			win.Comm = strings.Clone(comm)
		}
	}
	if path := strings.TrimSpace(event.GetPath()); path != "" {
		win.Paths = addLoopDetectionString(win.Paths, normalizeLoopDetectionTarget(path))
	}
	if extraPath := strings.TrimSpace(event.GetExtraPath()); extraPath != "" {
		win.Paths = addLoopDetectionString(win.Paths, normalizeLoopDetectionTarget(extraPath))
	}
	if toolName := strings.TrimSpace(event.GetToolName()); toolName != "" {
		win.ToolNames = addLoopDetectionString(win.ToolNames, boundLoopDetectionString(toolName, loopDetectionMetadataValueBytes))
	}
	if win.Target == "" {
		win.Target = strings.Clone(target)
	}
	if win.AgentRunID == "" {
		win.AgentRunID = cloneBoundedLoopDetectionString(event.GetAgentRunId(), loopDetectionContextComponentBytes)
	}
	if win.TaskID == "" {
		win.TaskID = cloneBoundedLoopDetectionString(event.GetTaskId(), loopDetectionContextComponentBytes)
	}
	if win.ToolCallID == "" {
		win.ToolCallID = cloneBoundedLoopDetectionString(event.GetToolCallId(), loopDetectionContextComponentBytes)
	}
	if win.TraceID == "" {
		win.TraceID = cloneBoundedLoopDetectionString(event.GetTraceId(), loopDetectionContextComponentBytes)
	}
	if win.RootAgentPID == 0 {
		win.RootAgentPID = event.GetRootAgentPid()
	}
}

func buildLoopDetectionFinding(win *loopDetectionWindow, observedAt time.Time) loopDetectionFinding {
	finding := loopDetectionFinding{
		ObservedAt:      observedAt,
		FirstSeen:       win.FirstSeen,
		LastSeen:        win.LastSeen,
		ContextType:     win.ContextType,
		ContextKey:      win.ContextKey,
		RepeatCount:     win.Count,
		WindowSeconds:   win.WindowSeconds,
		Fingerprint:     win.Fingerprint,
		Target:          win.Target,
		EventTypes:      sortedStringSet(win.EventTypes, 12),
		Pids:            sortedUint32Set(win.Pids, 12),
		Comms:           sortedStringSet(win.Comms, 12),
		Paths:           sortedStringSet(win.Paths, 12),
		ToolNames:       sortedStringSet(win.ToolNames, 12),
		AgentRunID:      win.AgentRunID,
		TaskID:          win.TaskID,
		ToolCallID:      win.ToolCallID,
		TraceID:         win.TraceID,
		RootAgentPID:    win.RootAgentPID,
		PID:             win.PID,
		Comm:            win.Comm,
		Reason:          fmt.Sprintf("same event fingerprint repeated %d times in one runtime context", win.Count),
		SuggestedAction: "Inspect the localized run/task/tool context, then stop or rewrite the loop guard before enabling stronger policy enforcement.",
	}
	finding.ID = loopDetectionFindingID(finding)
	return finding
}

func loopDetectionFindingID(finding loopDetectionFinding) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d|%s", finding.ContextKey, finding.Fingerprint, finding.RepeatCount, finding.LastSeen.Format(time.RFC3339Nano))))
	return "loop_" + hex.EncodeToString(sum[:8])
}

func loopDetectionSemanticAlert(finding loopDetectionFinding) *pb.Event {
	extra := strings.Join([]string{
		"source=loop_detection_worker",
		"code=RESOURCE_WASTING_LOOP",
		"context_type=" + sanitizeLoopExtraToken(finding.ContextType),
		"context_key=" + sanitizeLoopExtraToken(finding.ContextKey),
		"repeat_count=" + strconv.Itoa(finding.RepeatCount),
		"window_seconds=" + strconv.Itoa(finding.WindowSeconds),
		"fingerprint=" + sanitizeLoopExtraToken(finding.Fingerprint),
		"target=" + sanitizeLoopExtraToken(finding.Target),
	}, " ")
	return &pb.Event{
		Pid:           finding.PID,
		Type:          "semantic_alert",
		EventType:     pb.EventType_SEMANTIC_ALERT,
		Tag:           "Loop Detection",
		Comm:          "RESOURCE_WASTING_LOOP",
		Path:          finding.Target,
		ExtraInfo:     extra,
		SchemaVersion: eventSchemaVersion,
		RootAgentPid:  finding.RootAgentPID,
		AgentRunId:    finding.AgentRunID,
		TaskId:        finding.TaskID,
		ToolCallId:    finding.ToolCallID,
		TraceId:       finding.TraceID,
		Decision:      "ALERT",
		RiskScore:     92,
	}
}

func sanitizeLoopExtraToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, "\n", "_")
	value = strings.ReplaceAll(value, "\r", "_")
	value = strings.ReplaceAll(value, "\t", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}
