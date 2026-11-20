package research

import (
	"time"
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
	kind     researchProcessingWorkKind
	record   CapturedEventRecord
	records  []CapturedEventRecord
	force    bool
	queuedAt time.Time
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

type ResearchProcessingStatus struct {
	Enabled               bool                       `json:"enabled"`
	Settings              ResearchProcessingSettings `json:"settings"`
	QueueLen              int                        `json:"queueLen"`
	QueueCap              int                        `json:"queueCap"`
	EnqueuedTotal         uint64                     `json:"enqueuedTotal,omitempty"`
	ConsumedTotal         uint64                     `json:"consumedTotal"`
	DroppedTotal          uint64                     `json:"droppedTotal"`
	BufferedTotal         int                        `json:"bufferedTotal"`
	BufferEvictedTotal    uint64                     `json:"bufferEvictedTotal"`
	LastError             string                     `json:"lastError,omitempty"`
	LastDropReason        string                     `json:"lastDropReason,omitempty"`
	LastEnqueuedAt        *time.Time                 `json:"lastEnqueuedAt,omitempty"`
	LastProcessedAt       *time.Time                 `json:"lastProcessedAt,omitempty"`
	LastDroppedAt         *time.Time                 `json:"lastDroppedAt,omitempty"`
	LastSummaryAt         *time.Time                 `json:"lastSummaryAt,omitempty"`
	PendingSummary        bool                       `json:"pendingSummary"`
	SummaryRebuilds       uint64                     `json:"summaryRebuilds,omitempty"`
	LastSummaryDurationMs float64                    `json:"lastSummaryDurationMs,omitempty"`
	LastWorkLatencyMs     float64                    `json:"lastWorkLatencyMs,omitempty"`
	ThroughputPerSecond   float64                    `json:"throughputPerSecond,omitempty"`
	UpdatedAt             time.Time                  `json:"updatedAt"`
	Summary               researchProcessingSummary  `json:"summary"`
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
type researchProcessingSummarySettings struct {
	TimelineBucketSeconds int
	TopK                  int
	RecentSamples         int
}

// researchEventRing retains samples in insertion order while
// overwriting the oldest slot in O(1) once full. It is guarded by the owning
// researchProcessingWorker.mu.
type researchEventRing struct {
	items []researchEventSample
	start int
	limit int
}

func (r *researchEventRing) Len() int {
	return len(r.items)
}

func (r *researchEventRing) Reset() {
	clear(r.items)
	r.items = r.items[:0]
	r.start = 0
}

func (r *researchEventRing) Append(samples []researchEventSample, limit int) int {
	if len(samples) == 0 {
		return 0
	}
	if limit <= 0 {
		limit = researchProcessingDefaultMaxEvents
	}
	evicted := r.resize(limit)
	if overflow := len(r.items) + len(samples) - limit; overflow > 0 {
		evicted += overflow
	}
	if len(samples) >= limit {
		if cap(r.items) < limit {
			r.items = make([]researchEventSample, limit)
		} else {
			clear(r.items)
			r.items = r.items[:limit]
		}
		copy(r.items, samples[len(samples)-limit:])
		r.start = 0
		return evicted
	}
	needed := len(r.items) + len(samples)
	if needed > limit {
		needed = limit
	}
	r.grow(needed, limit)
	for _, sample := range samples {
		if len(r.items) < limit {
			r.items = append(r.items, sample)
			continue
		}
		r.items[r.start] = sample
		r.start++
		if r.start == limit {
			r.start = 0
		}
	}
	return evicted
}

func (r *researchEventRing) Snapshot() []researchEventSample {
	if len(r.items) == 0 {
		return nil
	}
	out := make([]researchEventSample, len(r.items))
	if r.start == 0 {
		copy(out, r.items)
		return out
	}
	copied := copy(out, r.items[r.start:])
	copy(out[copied:], r.items[:r.start])
	return out
}

func (r *researchEventRing) resize(limit int) int {
	if r.limit == limit {
		return 0
	}
	ordered := r.Snapshot()
	evicted := 0
	if len(ordered) > limit {
		evicted = len(ordered) - limit
		ordered = ordered[len(ordered)-limit:]
	}
	r.items = nil
	if len(ordered) > 0 {
		capacity := len(ordered)
		if capacity < 16 {
			capacity = min(16, limit)
		}
		r.items = make([]researchEventSample, len(ordered), capacity)
		copy(r.items, ordered)
	}
	r.start = 0
	r.limit = limit
	return evicted
}

func (r *researchEventRing) grow(needed, limit int) {
	if needed <= cap(r.items) {
		return
	}
	capacity := cap(r.items) * 2
	if capacity < 16 {
		capacity = 16
	}
	if capacity < needed {
		capacity = needed
	}
	if capacity > limit {
		capacity = limit
	}
	items := make([]researchEventSample, len(r.items), capacity)
	copy(items, r.items)
	r.items = items
}
