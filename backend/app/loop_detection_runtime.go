package app

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

const (
	loopDetectionDefaultWindowSeconds   = 30
	loopDetectionDefaultRepeatThreshold = 5
	loopDetectionDefaultMaxContexts     = 512
	loopDetectionDefaultQueueSize       = 2048
	loopDetectionFindingLimit           = 50
)

type loopDetectionFinding struct {
	ID              string    `json:"id"`
	ObservedAt      time.Time `json:"observedAt"`
	FirstSeen       time.Time `json:"firstSeen"`
	LastSeen        time.Time `json:"lastSeen"`
	ContextType     string    `json:"contextType"`
	ContextKey      string    `json:"contextKey"`
	RepeatCount     int       `json:"repeatCount"`
	WindowSeconds   int       `json:"windowSeconds"`
	Fingerprint     string    `json:"fingerprint"`
	Target          string    `json:"target"`
	EventTypes      []string  `json:"eventTypes"`
	Pids            []uint32  `json:"pids"`
	Comms           []string  `json:"comms"`
	Paths           []string  `json:"paths"`
	ToolNames       []string  `json:"toolNames"`
	AgentRunID      string    `json:"agentRunId,omitempty"`
	TaskID          string    `json:"taskId,omitempty"`
	ToolCallID      string    `json:"toolCallId,omitempty"`
	TraceID         string    `json:"traceId,omitempty"`
	RootAgentPID    uint32    `json:"rootAgentPid,omitempty"`
	PID             uint32    `json:"pid,omitempty"`
	Comm            string    `json:"comm,omitempty"`
	Reason          string    `json:"reason"`
	SuggestedAction string    `json:"suggestedAction"`
}

type loopDetectionStatus struct {
	Enabled        bool                   `json:"enabled"`
	Settings       LoopDetectionSettings  `json:"settings"`
	QueueLen       int                    `json:"queueLen"`
	QueueCap       int                    `json:"queueCap"`
	ConsumedTotal  uint64                 `json:"consumedTotal"`
	FindingsTotal  uint64                 `json:"findingsTotal"`
	DroppedTotal   uint64                 `json:"droppedTotal"`
	WindowCount    int                    `json:"windowCount"`
	RecentFindings []loopDetectionFinding `json:"recentFindings"`
	LastError      string                 `json:"lastError,omitempty"`
	UpdatedAt      time.Time              `json:"updatedAt"`
}

type loopDetectionTaskRequest struct {
	Action string `json:"action"`
	Limit  int    `json:"limit"`
}

type loopDetectionTaskResponse struct {
	Status   string `json:"status"`
	Action   string `json:"action"`
	Records  int    `json:"records,omitempty"`
	QueueLen int    `json:"queueLen"`
}

type loopDetectionWorkKind string

const (
	loopDetectionWorkEvent loopDetectionWorkKind = "event"
	loopDetectionWorkScan  loopDetectionWorkKind = "scan"
	loopDetectionWorkReset loopDetectionWorkKind = "reset"
)

type loopDetectionWorkItem struct {
	kind    loopDetectionWorkKind
	record  CapturedEventRecord
	records []CapturedEventRecord
	force   bool
}

type loopDetectionWindow struct {
	FirstSeen     time.Time
	LastSeen      time.Time
	Count         int
	Alerted       bool
	ContextType   string
	ContextKey    string
	Fingerprint   string
	Target        string
	EventTypes    map[string]struct{}
	Pids          map[uint32]struct{}
	Comms         map[string]struct{}
	Paths         map[string]struct{}
	ToolNames     map[string]struct{}
	AgentRunID    string
	TaskID        string
	ToolCallID    string
	TraceID       string
	RootAgentPID  uint32
	PID           uint32
	Comm          string
	SampleEvent   *pb.Event
	WindowSeconds int
}

type loopDetectionWorker struct {
	lifecycleMu   sync.Mutex
	mu            sync.RWMutex
	queue         chan loopDetectionWorkItem
	cancel        context.CancelFunc
	done          chan struct{}
	started       bool
	windows       map[string]*loopDetectionWindow
	findings      []loopDetectionFinding
	consumedTotal uint64
	findingsTotal uint64
	droppedTotal  uint64
	lastError     string
	updatedAt     time.Time
}

var loopDetectionWorkerStore = newLoopDetectionWorker()

func newLoopDetectionWorker() *loopDetectionWorker {
	return &loopDetectionWorker{
		windows:   make(map[string]*loopDetectionWindow),
		findings:  make([]loopDetectionFinding, 0, loopDetectionFindingLimit),
		updatedAt: time.Now().UTC(),
	}
}

func startLoopDetectionWorker(ctx context.Context) {
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	loopDetectionWorkerStore.Start(ctx, settings.QueueSize)
}

func (w *loopDetectionWorker) Start(ctx context.Context, queueSize int) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if queueSize <= 0 {
		queueSize = loopDetectionDefaultQueueSize
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.queue = make(chan loopDetectionWorkItem, queueSize)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.started = true
	w.updatedAt = time.Now().UTC()
	queue := w.queue
	done := w.done
	w.mu.Unlock()

	go func() {
		w.run(workerCtx, queue)
		w.mu.Lock()
		if w.done == done {
			w.queue = nil
			w.cancel = nil
			w.done = nil
			w.started = false
			w.updatedAt = time.Now().UTC()
		}
		w.mu.Unlock()
		close(done)
	}()
}

func (w *loopDetectionWorker) run(ctx context.Context, queue <-chan loopDetectionWorkItem) {
	for {
		var item loopDetectionWorkItem
		select {
		case <-ctx.Done():
			return
		case item = <-queue:
		}
		switch item.kind {
		case loopDetectionWorkReset:
			w.resetNow()
		case loopDetectionWorkScan:
			for _, record := range item.records {
				w.processRecord(record, item.force)
			}
		case loopDetectionWorkEvent:
			w.processRecord(item.record, item.force)
		}
	}
}

func (w *loopDetectionWorker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.queue = nil
	w.cancel = nil
	w.done = nil
	w.started = false
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func queueLoopDetectionRecord(record CapturedEventRecord) {
	if record.Event == nil || shouldIgnoreLoopDetectionEvent(record.Event) {
		return
	}
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	if !settings.Enabled {
		return
	}
	loopDetectionWorkerStore.EnqueueEvent(record)
}

func (w *loopDetectionWorker) EnqueueEvent(record CapturedEventRecord) bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkEvent, record: record})
}

func (w *loopDetectionWorker) EnqueueScan(records []CapturedEventRecord) bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkScan, records: records, force: true})
}

func (w *loopDetectionWorker) EnqueueReset() bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkReset})
}

func (w *loopDetectionWorker) enqueue(item loopDetectionWorkItem) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	queue := w.queue
	w.mu.RUnlock()
	if queue == nil {
		w.noteDrop("loop detection worker is not started")
		return false
	}
	select {
	case queue <- item:
		return true
	default:
		w.noteDrop("loop detection queue is full")
		return false
	}
}

func (w *loopDetectionWorker) resetNow() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.windows = make(map[string]*loopDetectionWindow)
	w.findings = w.findings[:0]
	w.lastError = ""
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *loopDetectionWorker) noteDrop(message string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.droppedTotal++
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *loopDetectionWorker) processRecord(record CapturedEventRecord, force bool) {
	if w == nil || record.Event == nil || shouldIgnoreLoopDetectionEvent(record.Event) {
		return
	}
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	if !force && !settings.Enabled {
		return
	}

	event := record.Event
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	contextType, contextKey := loopDetectionContext(event)
	fingerprint, target := loopDetectionFingerprint(event)
	if contextKey == "" || fingerprint == "" {
		return
	}
	windowSeconds := settings.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = loopDetectionDefaultWindowSeconds
	}
	windowDuration := time.Duration(windowSeconds) * time.Second
	threshold := settings.RepeatThreshold
	if threshold <= 0 {
		threshold = loopDetectionDefaultRepeatThreshold
	}
	windowKey := contextKey + "\x00" + fingerprint

	var finding *loopDetectionFinding
	w.mu.Lock()
	w.consumedTotal++
	win := w.windows[windowKey]
	if win == nil || win.FirstSeen.IsZero() || observedAt.Sub(win.FirstSeen) > windowDuration {
		win = newLoopDetectionWindow(contextType, contextKey, fingerprint, target, event, observedAt, windowSeconds)
		w.windows[windowKey] = win
	} else {
		win.Count++
		win.LastSeen = observedAt
		win.WindowSeconds = windowSeconds
		mergeLoopDetectionWindow(win, event, target)
	}
	w.evictLoopDetectionWindowsLocked(observedAt, windowDuration, settings.MaxContexts)
	if win.Count >= threshold && !win.Alerted {
		built := buildLoopDetectionFinding(win, observedAt)
		w.appendFindingLocked(built)
		win.Alerted = true
		finding = &built
	}
	w.updatedAt = observedAt
	w.mu.Unlock()

	if finding != nil && settings.EmitSemanticAlerts {
		w.emitSemanticAlert(*finding)
	}
}

func (w *loopDetectionWorker) evictLoopDetectionWindowsLocked(now time.Time, windowDuration time.Duration, maxContexts int) {
	if maxContexts <= 0 {
		maxContexts = loopDetectionDefaultMaxContexts
	}
	for key, win := range w.windows {
		if win == nil || (!win.LastSeen.IsZero() && now.Sub(win.LastSeen) > windowDuration*2) {
			delete(w.windows, key)
		}
	}
	if len(w.windows) <= maxContexts {
		return
	}
	type candidate struct {
		key string
		ts  time.Time
	}
	candidates := make([]candidate, 0, len(w.windows))
	for key, win := range w.windows {
		candidates = append(candidates, candidate{key: key, ts: win.LastSeen})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ts.Before(candidates[j].ts) })
	for len(w.windows) > maxContexts && len(candidates) > 0 {
		delete(w.windows, candidates[0].key)
		candidates = candidates[1:]
	}
}

func (w *loopDetectionWorker) appendFindingLocked(finding loopDetectionFinding) {
	w.findingsTotal++
	w.findings = append(w.findings, finding)
	if len(w.findings) > loopDetectionFindingLimit {
		copy(w.findings, w.findings[len(w.findings)-loopDetectionFindingLimit:])
		w.findings = w.findings[:loopDetectionFindingLimit]
	}
}

func (w *loopDetectionWorker) emitSemanticAlert(finding loopDetectionFinding) {
	alert := loopDetectionSemanticAlert(finding)
	if !enqueueBroadcastEvent(broadcast, alert, "loop_detection_alert") {
		w.noteDrop("broadcast queue is full while emitting loop detection alert")
	}
}

func (w *loopDetectionWorker) Status() loopDetectionStatus {
	if w == nil {
		settings := runtimeSettingsStore.Snapshot().LoopDetection
		normalizeLoopDetectionSettings(&settings)
		return loopDetectionStatus{Enabled: settings.Enabled, Settings: settings, UpdatedAt: time.Now().UTC()}
	}
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	w.mu.RLock()
	queueLen := 0
	queueCap := 0
	if w.queue != nil {
		queueLen = len(w.queue)
		queueCap = cap(w.queue)
	}
	findings := make([]loopDetectionFinding, len(w.findings))
	copy(findings, w.findings)
	status := loopDetectionStatus{
		Enabled:        settings.Enabled,
		Settings:       settings,
		QueueLen:       queueLen,
		QueueCap:       queueCap,
		ConsumedTotal:  w.consumedTotal,
		FindingsTotal:  w.findingsTotal,
		DroppedTotal:   w.droppedTotal,
		WindowCount:    len(w.windows),
		RecentFindings: findings,
		LastError:      w.lastError,
		UpdatedAt:      w.updatedAt,
	}
	w.mu.RUnlock()
	return status
}

func newLoopDetectionWindow(contextType, contextKey, fingerprint, target string, event *pb.Event, observedAt time.Time, windowSeconds int) *loopDetectionWindow {
	win := &loopDetectionWindow{
		FirstSeen:     observedAt,
		LastSeen:      observedAt,
		Count:         1,
		ContextType:   contextType,
		ContextKey:    contextKey,
		Fingerprint:   fingerprint,
		Target:        target,
		EventTypes:    make(map[string]struct{}),
		Pids:          make(map[uint32]struct{}),
		Comms:         make(map[string]struct{}),
		Paths:         make(map[string]struct{}),
		ToolNames:     make(map[string]struct{}),
		AgentRunID:    strings.TrimSpace(event.GetAgentRunId()),
		TaskID:        strings.TrimSpace(event.GetTaskId()),
		ToolCallID:    strings.TrimSpace(event.GetToolCallId()),
		TraceID:       strings.TrimSpace(event.GetTraceId()),
		RootAgentPID:  event.GetRootAgentPid(),
		PID:           event.GetPid(),
		Comm:          strings.TrimSpace(event.GetComm()),
		SampleEvent:   cloneProtoEvent(event),
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
		win.EventTypes[eventType] = struct{}{}
	}
	if pid := event.GetPid(); pid != 0 {
		win.Pids[pid] = struct{}{}
		if win.PID == 0 {
			win.PID = pid
		}
	}
	if comm := strings.TrimSpace(event.GetComm()); comm != "" {
		win.Comms[comm] = struct{}{}
		if win.Comm == "" {
			win.Comm = comm
		}
	}
	if path := strings.TrimSpace(event.GetPath()); path != "" {
		win.Paths[path] = struct{}{}
	}
	if extraPath := strings.TrimSpace(event.GetExtraPath()); extraPath != "" {
		win.Paths[extraPath] = struct{}{}
	}
	if toolName := strings.TrimSpace(event.GetToolName()); toolName != "" {
		win.ToolNames[toolName] = struct{}{}
	}
	if win.Target == "" {
		win.Target = target
	}
	if win.AgentRunID == "" {
		win.AgentRunID = strings.TrimSpace(event.GetAgentRunId())
	}
	if win.TaskID == "" {
		win.TaskID = strings.TrimSpace(event.GetTaskId())
	}
	if win.ToolCallID == "" {
		win.ToolCallID = strings.TrimSpace(event.GetToolCallId())
	}
	if win.TraceID == "" {
		win.TraceID = strings.TrimSpace(event.GetTraceId())
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

func shouldIgnoreLoopDetectionEvent(event *pb.Event) bool {
	if event == nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(event.GetType()), "semantic_alert") {
		return true
	}
	if event.GetEventType() == pb.EventType_SEMANTIC_ALERT {
		return true
	}
	return false
}

func loopDetectionContext(event *pb.Event) (string, string) {
	if event == nil {
		return "", ""
	}
	runID := strings.TrimSpace(event.GetAgentRunId())
	taskID := strings.TrimSpace(event.GetTaskId())
	toolCallID := strings.TrimSpace(event.GetToolCallId())
	traceID := strings.TrimSpace(event.GetTraceId())
	if toolCallID != "" {
		return "tool_call", "tool_call:" + strings.Join(nonEmptyLoopParts(runID, taskID, toolCallID), "/")
	}
	if taskID != "" {
		return "task", "task:" + strings.Join(nonEmptyLoopParts(runID, taskID), "/")
	}
	if runID != "" {
		return "agent_run", "agent_run:" + runID
	}
	if traceID != "" {
		return "trace", "trace:" + traceID
	}
	if root := event.GetRootAgentPid(); root != 0 {
		return "root_agent_pid", fmt.Sprintf("root_agent_pid:%d", root)
	}
	if tgid := event.GetTgid(); tgid != 0 {
		return "tgid", fmt.Sprintf("tgid:%d", tgid)
	}
	if pid := event.GetPid(); pid != 0 {
		return "pid", fmt.Sprintf("pid:%d", pid)
	}
	if comm := strings.TrimSpace(event.GetComm()); comm != "" {
		return "comm", "comm:" + comm
	}
	return "", ""
}

func nonEmptyLoopParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func loopDetectionFingerprint(event *pb.Event) (fingerprint string, target string) {
	if event == nil {
		return "", ""
	}
	eventType := normalizedLoopEventType(event)
	target = loopDetectionStableTarget(event)
	parts := []string{eventType}
	if toolName := strings.TrimSpace(event.GetToolName()); toolName != "" {
		parts = append(parts, "tool="+strings.ToLower(toolName))
	}
	if target != "" {
		parts = append(parts, "target="+target)
	}
	if eventType == "" && target == "" {
		return "", ""
	}
	return strings.Join(parts, "|"), target
}

func normalizedLoopEventType(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if value := strings.TrimSpace(event.GetType()); value != "" {
		return strings.ToLower(value)
	}
	if value := strings.TrimSpace(event.GetEventType().String()); value != "" {
		return strings.ToLower(value)
	}
	return ""
}

func loopDetectionStableTarget(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if digest := extractLoopDetectionDigest(event.GetExtraInfo()); digest != "" {
		return "digest:" + digest
	}
	candidates := []string{
		event.GetPath(),
		event.GetExtraPath(),
		event.GetNetEndpoint(),
		event.GetDnsName(),
		event.GetSni(),
		event.GetDomain(),
		event.GetServiceName(),
		event.GetArgvDigest(),
		event.GetToolName(),
		event.GetComm(),
	}
	for _, candidate := range candidates {
		if normalized := normalizeLoopDetectionTarget(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func extractLoopDetectionDigest(extraInfo string) string {
	extraInfo = strings.TrimSpace(extraInfo)
	if extraInfo == "" {
		return ""
	}
	lower := strings.ToLower(extraInfo)
	keys := []string{"prompt_digest", "promptdigest", "context_digest", "request_digest", "response_digest", "argv_digest", "digest"}
	for _, key := range keys {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		rest := lower[idx+len(key):]
		rest = strings.TrimLeft(rest, " :=\t\n\r\"'")
		if rest == "" {
			continue
		}
		end := len(rest)
		for i, r := range rest {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				end = i
				break
			}
		}
		value := strings.Trim(rest[:end], " _-")
		if len(value) >= 6 {
			return value
		}
	}
	return ""
}

func normalizeLoopDetectionTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if strings.HasPrefix(value, "/") {
		value = filepath.Clean(value)
	}
	if strings.Contains(value, "://") || strings.Contains(value, ".") || strings.Contains(value, ":") {
		value = strings.ToLower(value)
	}
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func sortedStringSet(set map[string]struct{}, limit int) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func sortedUint32Set(set map[uint32]struct{}, limit int) []uint32 {
	values := make([]uint32, 0, len(set))
	for value := range set {
		if value != 0 {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func handleLoopDetectionStatus(c *gin.Context) {
	c.JSON(200, loopDetectionWorkerStore.Status())
}

func handleLoopDetectionTask(c *gin.Context) {
	var req loopDetectionTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid loop detection task"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "scan_recent"
	}
	switch action {
	case "scan_recent", "scan":
		limit := req.Limit
		if limit <= 0 {
			limit = 500
		}
		if limit > 10000 {
			limit = 10000
		}
		records, _, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !loopDetectionWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "loop detection worker queue is full or not started"})
			return
		}
		status := loopDetectionWorkerStore.Status()
		c.JSON(202, loopDetectionTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "reset":
		if !loopDetectionWorkerStore.EnqueueReset() {
			loopDetectionWorkerStore.resetNow()
		}
		status := loopDetectionWorkerStore.Status()
		c.JSON(202, loopDetectionTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported loop detection task", "supported": []string{"scan_recent", "reset"}})
	}
}

// loopDetectionEnvSummary is used by the UI copy and tests to document the
// environment variables that override persisted loop settings on startup.
func loopDetectionEnvSummary() []string {
	return []string{
		"AGENT_RUNTIME_LOOP_DETECTION_ENABLED",
		"AGENT_RUNTIME_LOOP_DETECTION_WINDOW_SECONDS",
		"AGENT_RUNTIME_LOOP_DETECTION_REPEAT_THRESHOLD",
		"AGENT_RUNTIME_LOOP_DETECTION_MAX_CONTEXTS",
		"AGENT_RUNTIME_LOOP_DETECTION_QUEUE_SIZE",
		"AGENT_RUNTIME_LOOP_DETECTION_EMIT_ALERTS",
		platform.RuntimeSettingsPath(),
	}
}
