package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"

	"github.com/gin-gonic/gin"
)

const (
	researchProcessingDefaultMaxEvents             = 5000
	researchProcessingDefaultQueueSize             = 2048
	researchProcessingDefaultTimelineBucketSeconds = 60
	researchProcessingDefaultTopK                  = 20
	researchProcessingDefaultRecentSamples         = 25
	researchProcessingDefaultArtifactRetentionDays = 14
	researchProcessingDefaultMaxSessionEvents      = 50000
	researchProcessingDefaultExportFormats         = "jsonl,csv,bundle"
)

type researchProcessingWorkKind string

const (
	researchProcessingWorkEvent researchProcessingWorkKind = "event"
	researchProcessingWorkScan  researchProcessingWorkKind = "scan"
	researchProcessingWorkReset researchProcessingWorkKind = "reset"
)

type researchProcessingWorkItem struct {
	kind    researchProcessingWorkKind
	record  CapturedEventRecord
	records []CapturedEventRecord
	force   bool
}

type researchEventSample struct {
	ID        string  `json:"id"`
	Timestamp int64   `json:"timestamp"`
	Time      string  `json:"time"`
	Source    string  `json:"source"`
	EventType string  `json:"eventType"`
	PID       uint32  `json:"pid,omitempty"`
	PPID      uint32  `json:"ppid,omitempty"`
	Comm      string  `json:"comm,omitempty"`
	TraceID   string  `json:"traceId,omitempty"`
	SpanID    string  `json:"spanId,omitempty"`
	Title     string  `json:"title"`
	Target    string  `json:"target,omitempty"`
	RiskScore float64 `json:"riskScore,omitempty"`
	Decision  string  `json:"decision,omitempty"`
}

type researchCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type researchTimelineBucket struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Time  string `json:"time"`
	Count int    `json:"count"`
}

type researchProcessSummary struct {
	PID        uint32   `json:"pid"`
	PPID       uint32   `json:"ppid,omitempty"`
	Comm       string   `json:"comm"`
	EventCount int      `json:"eventCount"`
	FirstSeen  int64    `json:"firstSeen,omitempty"`
	LastSeen   int64    `json:"lastSeen,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	EventTypes []string `json:"eventTypes,omitempty"`
	ChildPids  []uint32 `json:"childPids,omitempty"`
}

type researchTraceSummary struct {
	TraceID    string   `json:"traceId"`
	EventCount int      `json:"eventCount"`
	FirstSeen  int64    `json:"firstSeen,omitempty"`
	LastSeen   int64    `json:"lastSeen,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	Comms      []string `json:"comms,omitempty"`
}

type researchProcessingSummary struct {
	Total              int                      `json:"total"`
	EarliestTimestamp  int64                    `json:"earliestTimestamp,omitempty"`
	LatestTimestamp    int64                    `json:"latestTimestamp,omitempty"`
	EarliestTime       string                   `json:"earliestTime,omitempty"`
	LatestTime         string                   `json:"latestTime,omitempty"`
	BySource           []researchCount          `json:"bySource"`
	ByType             []researchCount          `json:"byType"`
	ByComm             []researchCount          `json:"byComm"`
	ByPID              []researchCount          `json:"byPid"`
	ByTrace            []researchCount          `json:"byTrace"`
	Timeline           []researchTimelineBucket `json:"timeline"`
	TopProcesses       []researchProcessSummary `json:"topProcesses"`
	TopTraces          []researchTraceSummary   `json:"topTraces"`
	RecentSamples      []researchEventSample    `json:"recentSamples"`
	GeneratedTimestamp int64                    `json:"generatedTimestamp"`
	GeneratedTime      string                   `json:"generatedTime"`
}

type researchProcessingStatus struct {
	Enabled       bool                       `json:"enabled"`
	Settings      ResearchProcessingSettings `json:"settings"`
	QueueLen      int                        `json:"queueLen"`
	QueueCap      int                        `json:"queueCap"`
	ConsumedTotal uint64                     `json:"consumedTotal"`
	DroppedTotal  uint64                     `json:"droppedTotal"`
	BufferedTotal int                        `json:"bufferedTotal"`
	LastError     string                     `json:"lastError,omitempty"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
	Summary       researchProcessingSummary  `json:"summary"`
}

type researchProcessingTaskRequest struct {
	Action string `json:"action"`
	Limit  int    `json:"limit"`
}

type researchProcessingTaskResponse struct {
	Status   string `json:"status"`
	Action   string `json:"action"`
	Records  int    `json:"records,omitempty"`
	QueueLen int    `json:"queueLen"`
}

type researchProcessingWorker struct {
	mu            sync.RWMutex
	queue         chan researchProcessingWorkItem
	started       bool
	events        []researchEventSample
	summary       researchProcessingSummary
	consumedTotal uint64
	droppedTotal  uint64
	lastError     string
	updatedAt     time.Time
}

var researchProcessingWorkerStore = newResearchProcessingWorker()

func newResearchProcessingWorker() *researchProcessingWorker {
	return &researchProcessingWorker{
		events:    make([]researchEventSample, 0, researchProcessingDefaultMaxEvents),
		summary:   researchProcessingSummary{},
		updatedAt: time.Now().UTC(),
	}
}

func startResearchProcessingWorker() {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	researchProcessingWorkerStore.Start(settings.QueueSize)
}

func (w *researchProcessingWorker) Start(queueSize int) {
	if w == nil {
		return
	}
	if queueSize <= 0 {
		queueSize = researchProcessingDefaultQueueSize
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.queue = make(chan researchProcessingWorkItem, queueSize)
	w.started = true
	w.updatedAt = time.Now().UTC()
	queue := w.queue
	w.mu.Unlock()
	go w.run(queue)
}

func (w *researchProcessingWorker) run(queue <-chan researchProcessingWorkItem) {
	for item := range queue {
		switch item.kind {
		case researchProcessingWorkReset:
			w.resetNow()
		case researchProcessingWorkScan:
			w.resetNow()
			for _, record := range item.records {
				w.processRecord(record, item.force)
			}
		case researchProcessingWorkEvent:
			w.processRecord(item.record, item.force)
		}
	}
}

func queueResearchProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if !settings.Enabled {
		return
	}
	researchProcessingWorkerStore.EnqueueEvent(record)
}

func (w *researchProcessingWorker) EnqueueEvent(record CapturedEventRecord) bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkEvent, record: record})
}

func (w *researchProcessingWorker) EnqueueScan(records []CapturedEventRecord) bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkScan, records: records, force: true})
}

func (w *researchProcessingWorker) EnqueueReset() bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkReset})
}

func (w *researchProcessingWorker) enqueue(item researchProcessingWorkItem) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	queue := w.queue
	w.mu.RUnlock()
	if queue == nil {
		w.noteDrop("research processing worker is not started")
		return false
	}
	select {
	case queue <- item:
		return true
	default:
		w.noteDrop("research processing queue is full")
		return false
	}
}

func (w *researchProcessingWorker) noteDrop(message string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.droppedTotal++
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *researchProcessingWorker) resetNow() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.events = w.events[:0]
	w.summary = researchProcessingSummary{}
	w.lastError = ""
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *researchProcessingWorker) processRecord(record CapturedEventRecord, force bool) {
	if w == nil || record.Event == nil {
		return
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if !force && !settings.Enabled {
		return
	}
	sample, ok := researchEventSampleFromRecord(record)
	if !ok {
		return
	}
	w.mu.Lock()
	w.consumedTotal++
	w.events = append(w.events, sample)
	maxEvents := settings.MaxEvents
	if maxEvents <= 0 {
		maxEvents = researchProcessingDefaultMaxEvents
	}
	if len(w.events) > maxEvents {
		copy(w.events, w.events[len(w.events)-maxEvents:])
		w.events = w.events[:maxEvents]
	}
	w.summary = buildResearchProcessingSummary(w.events, settings)
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *researchProcessingWorker) Status() researchProcessingStatus {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if w == nil {
		return researchProcessingStatus{Enabled: settings.Enabled, Settings: settings, UpdatedAt: time.Now().UTC()}
	}
	w.mu.RLock()
	queueLen := 0
	queueCap := 0
	if w.queue != nil {
		queueLen = len(w.queue)
		queueCap = cap(w.queue)
	}
	status := researchProcessingStatus{
		Enabled:       settings.Enabled,
		Settings:      settings,
		QueueLen:      queueLen,
		QueueCap:      queueCap,
		ConsumedTotal: w.consumedTotal,
		DroppedTotal:  w.droppedTotal,
		BufferedTotal: len(w.events),
		LastError:     w.lastError,
		UpdatedAt:     w.updatedAt,
		Summary:       cloneResearchProcessingSummary(w.summary),
	}
	w.mu.RUnlock()
	return status
}

func researchEventSampleFromRecord(record CapturedEventRecord) (researchEventSample, bool) {
	record = normalizeCapturedEventRecord(record)
	event := record.Event
	if event == nil {
		return researchEventSample{}, false
	}
	envelope := record.Envelope
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() && envelope != nil && envelope.GetTimestampNs() > 0 {
		observedAt = time.Unix(0, int64(envelope.GetTimestampNs())).UTC()
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	source := determineEventEnvelopeSource(event)
	if envelope != nil && strings.TrimSpace(envelope.GetSource()) != "" {
		source = envelope.GetSource()
	}
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	eventType := envelopeEventTypeName(envelope, event)
	if eventType == "" {
		eventType = platform.FirstNonEmpty(event.GetType(), event.GetEventType().String())
	}
	pid := event.GetPid()
	ppid := event.GetPpid()
	comm := strings.TrimSpace(event.GetComm())
	traceID := strings.TrimSpace(event.GetTraceId())
	spanID := strings.TrimSpace(event.GetSpanId())
	if envelope != nil {
		if pid == 0 {
			pid = envelope.GetPid()
		}
		if ppid == 0 {
			ppid = envelope.GetPpid()
		}
		comm = platform.FirstNonEmpty(comm, envelope.GetComm())
		traceID = platform.FirstNonEmpty(traceID, envelope.GetTraceId())
		spanID = platform.FirstNonEmpty(spanID, envelope.GetSpanId())
	}
	target := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetNetEndpoint(), event.GetDnsName(), event.GetSni(), event.GetDomain(), event.GetServiceName())
	titleParts := []string{source, eventType}
	if comm != "" {
		titleParts = append(titleParts, comm)
	}
	if target != "" {
		titleParts = append(titleParts, target)
	}
	id := ""
	if envelope != nil {
		id = strings.TrimSpace(envelope.GetEventId())
	}
	if id == "" {
		id = fmt.Sprintf("research-%d-%s-%d-%s", observedAt.UnixMilli(), sanitizeResearchIDPart(source), pid, sanitizeResearchIDPart(eventType))
	}
	return researchEventSample{
		ID:        id,
		Timestamp: observedAt.UnixMilli(),
		Time:      observedAt.Format(time.RFC3339Nano),
		Source:    strings.ToLower(strings.TrimSpace(source)),
		EventType: strings.TrimSpace(eventType),
		PID:       pid,
		PPID:      ppid,
		Comm:      comm,
		TraceID:   traceID,
		SpanID:    spanID,
		Title:     strings.Join(nonEmptyResearchParts(titleParts...), " · "),
		Target:    target,
		RiskScore: event.GetRiskScore(),
		Decision:  strings.TrimSpace(event.GetDecision()),
	}, true
}

func buildResearchProcessingSummary(events []researchEventSample, settings ResearchProcessingSettings) researchProcessingSummary {
	normalizeResearchProcessingSettings(&settings)
	now := time.Now().UTC()
	summary := researchProcessingSummary{
		Total:              len(events),
		BySource:           []researchCount{},
		ByType:             []researchCount{},
		ByComm:             []researchCount{},
		ByPID:              []researchCount{},
		ByTrace:            []researchCount{},
		Timeline:           []researchTimelineBucket{},
		TopProcesses:       []researchProcessSummary{},
		TopTraces:          []researchTraceSummary{},
		RecentSamples:      []researchEventSample{},
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
	}
	if len(events) == 0 {
		return summary
	}
	bySource := map[string]int{}
	byType := map[string]int{}
	byComm := map[string]int{}
	byPID := map[string]int{}
	byTrace := map[string]int{}
	processes := map[uint32]*mutableResearchProcess{}
	traces := map[string]*mutableResearchTrace{}
	timeline := map[int64]int{}
	bucketMs := int64(settings.TimelineBucketSeconds) * 1000
	if bucketMs <= 0 {
		bucketMs = int64(researchProcessingDefaultTimelineBucketSeconds) * 1000
	}
	for _, event := range events {
		if summary.EarliestTimestamp == 0 || event.Timestamp < summary.EarliestTimestamp {
			summary.EarliestTimestamp = event.Timestamp
		}
		if event.Timestamp > summary.LatestTimestamp {
			summary.LatestTimestamp = event.Timestamp
		}
		incrementResearchCount(bySource, event.Source)
		incrementResearchCount(byType, event.EventType)
		incrementResearchCount(byComm, event.Comm)
		if event.PID != 0 {
			incrementResearchCount(byPID, strconv.FormatUint(uint64(event.PID), 10)+":"+event.Comm)
			proc := processes[event.PID]
			if proc == nil {
				proc = &mutableResearchProcess{summary: researchProcessSummary{PID: event.PID, PPID: event.PPID, Comm: platform.FirstNonEmpty(event.Comm, "unknown")}, sources: map[string]struct{}{}, eventTypes: map[string]struct{}{}, childPids: map[uint32]struct{}{}}
				processes[event.PID] = proc
			}
			if proc.summary.PPID == 0 {
				proc.summary.PPID = event.PPID
			}
			proc.summary.EventCount++
			if proc.summary.FirstSeen == 0 || event.Timestamp < proc.summary.FirstSeen {
				proc.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > proc.summary.LastSeen {
				proc.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				proc.sources[event.Source] = struct{}{}
			}
			if event.EventType != "" {
				proc.eventTypes[event.EventType] = struct{}{}
			}
		}
		if event.TraceID != "" {
			incrementResearchCount(byTrace, event.TraceID)
			trace := traces[event.TraceID]
			if trace == nil {
				trace = &mutableResearchTrace{summary: researchTraceSummary{TraceID: event.TraceID}, sources: map[string]struct{}{}, comms: map[string]struct{}{}}
				traces[event.TraceID] = trace
			}
			trace.summary.EventCount++
			if trace.summary.FirstSeen == 0 || event.Timestamp < trace.summary.FirstSeen {
				trace.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > trace.summary.LastSeen {
				trace.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				trace.sources[event.Source] = struct{}{}
			}
			if event.Comm != "" {
				trace.comms[event.Comm] = struct{}{}
			}
		}
		bucket := event.Timestamp / bucketMs * bucketMs
		timeline[bucket]++
	}
	for _, proc := range processes {
		if proc.summary.PPID != 0 {
			if parent := processes[proc.summary.PPID]; parent != nil {
				parent.childPids[proc.summary.PID] = struct{}{}
			}
		}
	}
	summary.BySource = topResearchCounts(bySource, settings.TopK)
	summary.ByType = topResearchCounts(byType, settings.TopK)
	summary.ByComm = topResearchCounts(byComm, settings.TopK)
	summary.ByPID = topResearchCounts(byPID, settings.TopK)
	summary.ByTrace = topResearchCounts(byTrace, settings.TopK)
	summary.TopProcesses = topResearchProcesses(processes, settings.TopK)
	summary.TopTraces = topResearchTraces(traces, settings.TopK)
	summary.Timeline = researchTimelineBuckets(timeline, bucketMs)
	summary.RecentSamples = recentResearchSamples(events, settings.RecentSamples)
	if summary.EarliestTimestamp > 0 {
		summary.EarliestTime = time.UnixMilli(summary.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if summary.LatestTimestamp > 0 {
		summary.LatestTime = time.UnixMilli(summary.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	return summary
}

type mutableResearchProcess struct {
	summary    researchProcessSummary
	sources    map[string]struct{}
	eventTypes map[string]struct{}
	childPids  map[uint32]struct{}
}

type mutableResearchTrace struct {
	summary researchTraceSummary
	sources map[string]struct{}
	comms   map[string]struct{}
}

func incrementResearchCount(counts map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	counts[key]++
}

func topResearchCounts(counts map[string]int, limit int) []researchCount {
	items := make([]researchCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, researchCount{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func topResearchProcesses(processes map[uint32]*mutableResearchProcess, limit int) []researchProcessSummary {
	items := make([]researchProcessSummary, 0, len(processes))
	for _, proc := range processes {
		item := proc.summary
		item.Sources = sortedResearchStrings(proc.sources, 12)
		item.EventTypes = sortedResearchStrings(proc.eventTypes, 12)
		item.ChildPids = sortedResearchUint32s(proc.childPids, 24)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].PID < items[j].PID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func topResearchTraces(traces map[string]*mutableResearchTrace, limit int) []researchTraceSummary {
	items := make([]researchTraceSummary, 0, len(traces))
	for _, trace := range traces {
		item := trace.summary
		item.Sources = sortedResearchStrings(trace.sources, 12)
		item.Comms = sortedResearchStrings(trace.comms, 12)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].TraceID < items[j].TraceID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func researchTimelineBuckets(counts map[int64]int, bucketMs int64) []researchTimelineBucket {
	starts := make([]int64, 0, len(counts))
	for start := range counts {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	items := make([]researchTimelineBucket, 0, len(starts))
	for _, start := range starts {
		items = append(items, researchTimelineBucket{Start: start, End: start + bucketMs, Time: time.UnixMilli(start).UTC().Format(time.RFC3339Nano), Count: counts[start]})
	}
	return items
}

func recentResearchSamples(events []researchEventSample, limit int) []researchEventSample {
	if limit <= 0 {
		limit = researchProcessingDefaultRecentSamples
	}
	if len(events) <= limit {
		out := make([]researchEventSample, len(events))
		copy(out, events)
		return out
	}
	out := make([]researchEventSample, limit)
	copy(out, events[len(events)-limit:])
	return out
}

func sortedResearchStrings(set map[string]struct{}, limit int) []string {
	items := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func sortedResearchUint32s(set map[uint32]struct{}, limit int) []uint32 {
	items := make([]uint32, 0, len(set))
	for value := range set {
		if value != 0 {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func cloneResearchProcessingSummary(summary researchProcessingSummary) researchProcessingSummary {
	cloned := summary
	cloned.BySource = append([]researchCount(nil), summary.BySource...)
	cloned.ByType = append([]researchCount(nil), summary.ByType...)
	cloned.ByComm = append([]researchCount(nil), summary.ByComm...)
	cloned.ByPID = append([]researchCount(nil), summary.ByPID...)
	cloned.ByTrace = append([]researchCount(nil), summary.ByTrace...)
	cloned.Timeline = append([]researchTimelineBucket(nil), summary.Timeline...)
	cloned.TopProcesses = append([]researchProcessSummary(nil), summary.TopProcesses...)
	cloned.TopTraces = append([]researchTraceSummary(nil), summary.TopTraces...)
	cloned.RecentSamples = append([]researchEventSample(nil), summary.RecentSamples...)
	return cloned
}

func sanitizeResearchIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func nonEmptyResearchParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func handleResearchProcessingStatus(c *gin.Context) {
	c.JSON(200, researchProcessingWorkerStore.Status())
}

func handleResearchProcessingTask(c *gin.Context) {
	var req researchProcessingTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid research processing task"})
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
			limit = 1000
		}
		if limit > 50000 {
			limit = 50000
		}
		records, _, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !researchProcessingWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "research processing worker queue is full or not started"})
			return
		}
		status := researchProcessingWorkerStore.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "reset":
		if !researchProcessingWorkerStore.EnqueueReset() {
			researchProcessingWorkerStore.resetNow()
		}
		status := researchProcessingWorkerStore.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported research processing task", "supported": []string{"scan_recent", "reset"}})
	}
}
