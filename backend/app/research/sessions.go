package research

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
)

const (
	researchSchemaVersion     = "research.v2"
	researchManifestVersion   = "research-manifest.v1"
	researchTaskQueued        = "queued"
	researchTaskRunning       = "running"
	researchTaskSucceeded     = "succeeded"
	researchTaskFailed        = "failed"
	researchTaskCanceled      = "canceled"
	researchSessionActive     = "active"
	researchSessionBuilding   = "building"
	researchSessionReady      = "ready"
	researchSessionEmpty      = "empty"
	researchSessionError      = "error"
	researchDefaultPageLimit  = 500
	researchMaxPageLimit      = 5000
	researchDefaultTaskLimit  = 5000
	researchMaxTaskLimit      = 50000
	researchTaskStoreMaxItems = 2048
)

// ResearchSession is the persisted top-level research workspace. It stores
// normalized, already-redacted views and references to export artifacts rather
// than long-lived raw payloads.
type ResearchSession struct {
	ID           string                         `json:"id"`
	Name         string                         `json:"name"`
	Description  string                         `json:"description,omitempty"`
	Tags         []string                       `json:"tags,omitempty"`
	CreatedAt    time.Time                      `json:"createdAt"`
	UpdatedAt    time.Time                      `json:"updatedAt"`
	SourceFilter ResearchSourceFilter           `json:"sourceFilter,omitempty"`
	TimeRange    ResearchTimeRange              `json:"timeRange,omitempty"`
	Status       string                         `json:"status"`
	Summary      ResearchSessionSummary         `json:"summary"`
	ArtifactRefs map[string]ResearchArtifactRef `json:"artifactRefs,omitempty"`
	LastError    string                         `json:"lastError,omitempty"`
}

type ResearchSessionSummary struct {
	SchemaVersion      string  `json:"schemaVersion"`
	EventCount         int     `json:"eventCount"`
	EarliestTimestamp  int64   `json:"earliestTimestamp,omitempty"`
	LatestTimestamp    int64   `json:"latestTimestamp,omitempty"`
	EarliestTime       string  `json:"earliestTime,omitempty"`
	LatestTime         string  `json:"latestTime,omitempty"`
	TopSource          string  `json:"topSource,omitempty"`
	TopEventType       string  `json:"topEventType,omitempty"`
	TopComm            string  `json:"topComm,omitempty"`
	MaxRiskScore       float64 `json:"maxRiskScore,omitempty"`
	RiskAlerts         int     `json:"riskAlerts,omitempty"`
	LoopFindings       int     `json:"loopFindings,omitempty"`
	GeneratedTimestamp int64   `json:"generatedTimestamp,omitempty"`
	GeneratedTime      string  `json:"generatedTime,omitempty"`
}

type ResearchSourceFilter struct {
	Sources         []string `json:"sources,omitempty"`
	EventTypes      []string `json:"eventTypes,omitempty"`
	Comms           []string `json:"comms,omitempty"`
	PIDs            []uint32 `json:"pids,omitempty"`
	TraceID         string   `json:"traceId,omitempty"`
	SpanID          string   `json:"spanId,omitempty"`
	Query           string   `json:"query,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	IncludeTLS      *bool    `json:"includeTLS,omitempty"`
	IncludeUploaded *bool    `json:"includeUploaded,omitempty"`
}

type ResearchTimeRange struct {
	Since     int64  `json:"since,omitempty"`
	Until     int64  `json:"until,omitempty"`
	SinceTime string `json:"sinceTime,omitempty"`
	UntilTime string `json:"untilTime,omitempty"`
}

type ResearchArtifactRef struct {
	Format      string    `json:"format"`
	Name        string    `json:"name"`
	Path        string    `json:"path,omitempty"`
	ContentType string    `json:"contentType"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ResearchEvent struct {
	ID             string         `json:"id"`
	Timestamp      int64          `json:"timestamp"`
	Time           string         `json:"time"`
	Source         string         `json:"source"`
	EventType      string         `json:"eventType"`
	PID            uint32         `json:"pid,omitempty"`
	PPID           uint32         `json:"ppid,omitempty"`
	Comm           string         `json:"comm,omitempty"`
	TraceID        string         `json:"traceId,omitempty"`
	SpanID         string         `json:"spanId,omitempty"`
	Target         string         `json:"target,omitempty"`
	RiskScore      float64        `json:"riskScore,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	RedactionLevel string         `json:"redactionLevel,omitempty"`
	Features       map[string]any `json:"features,omitempty"`
}

type ResearchResults struct {
	SchemaVersion      string                            `json:"schemaVersion"`
	SessionID          string                            `json:"sessionId"`
	GeneratedTimestamp int64                             `json:"generatedTimestamp"`
	GeneratedTime      string                            `json:"generatedTime"`
	Summary            researchProcessingSummary         `json:"summary"`
	TopTargets         []researchCount                   `json:"topTargets"`
	TopDecisions       []researchCount                   `json:"topDecisions"`
	LoopFindings       []loopDetectionFinding            `json:"loopFindings,omitempty"`
	RiskAlerts         []ResearchRiskFinding             `json:"riskAlerts,omitempty"`
	KernelRiskFeedback ResearchKernelRiskFeedbackInfo    `json:"kernelRiskFeedback"`
	CompareWindows     *ResearchWindowCompare            `json:"compareWindows,omitempty"`
	SecurityEvaluation *ResearchSecurityEvaluationReport `json:"securityEvaluation,omitempty"`
}

type ResearchRiskFinding struct {
	EventID    string  `json:"eventId"`
	Timestamp  int64   `json:"timestamp"`
	Time       string  `json:"time"`
	Source     string  `json:"source"`
	EventType  string  `json:"eventType"`
	PID        uint32  `json:"pid,omitempty"`
	Comm       string  `json:"comm,omitempty"`
	Target     string  `json:"target,omitempty"`
	RiskScore  float64 `json:"riskScore,omitempty"`
	Decision   string  `json:"decision,omitempty"`
	TraceID    string  `json:"traceId,omitempty"`
	Associated string  `json:"associated,omitempty"`
}

type ResearchKernelRiskFeedbackInfo struct {
	Enabled             bool    `json:"enabled"`
	PolicyGateEnabled   bool    `json:"policyGateEnabled"`
	MinRiskScore        float64 `json:"minRiskScore"`
	EnforceNetwork      bool    `json:"enforceNetwork"`
	EnforceFileNames    bool    `json:"enforceFileNames"`
	EnforceExec         bool    `json:"enforceExec"`
	MaxActionsPerMinute int     `json:"maxActionsPerMinute"`
}

type ResearchWindowCompare struct {
	Left   ResearchWindowSummary `json:"left"`
	Right  ResearchWindowSummary `json:"right"`
	Deltas []ResearchCountDelta  `json:"deltas"`
}

type ResearchWindowSummary struct {
	Name      string                    `json:"name"`
	TimeRange ResearchTimeRange         `json:"timeRange"`
	Summary   researchProcessingSummary `json:"summary"`
}

type ResearchCountDelta struct {
	Category string `json:"category"`
	Key      string `json:"key"`
	Left     int    `json:"left"`
	Right    int    `json:"right"`
	Delta    int    `json:"delta"`
}

type ResearchManifest struct {
	SchemaVersion     string                         `json:"schemaVersion"`
	GeneratedAt       time.Time                      `json:"generatedAt"`
	SessionID         string                         `json:"sessionId"`
	SessionName       string                         `json:"sessionName"`
	SourceFilter      ResearchSourceFilter           `json:"sourceFilter,omitempty"`
	TimeRange         ResearchTimeRange              `json:"timeRange,omitempty"`
	RedactionLevel    string                         `json:"redactionLevel"`
	EventCount        int                            `json:"eventCount"`
	Artifacts         map[string]ResearchArtifactRef `json:"artifacts"`
	Hashes            map[string]string              `json:"hashes"`
	ResearchSchema    string                         `json:"researchSchema"`
	ExportedBy        string                         `json:"exportedBy"`
	RetentionDays     int                            `json:"retentionDays"`
	MaxSessionEvents  int                            `json:"maxSessionEvents"`
	ConfiguredFormats []string                       `json:"configuredFormats"`
}

type researchCreateSessionRequest struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Tags         []string             `json:"tags"`
	SourceFilter ResearchSourceFilter `json:"sourceFilter"`
	TimeRange    ResearchTimeRange    `json:"timeRange"`
}

type researchTaskRequest struct {
	Action         string               `json:"action"`
	Limit          int                  `json:"limit"`
	SourceFilter   ResearchSourceFilter `json:"sourceFilter"`
	TimeRange      ResearchTimeRange    `json:"timeRange"`
	LeftWindow     ResearchTimeRange    `json:"leftWindow"`
	RightWindow    ResearchTimeRange    `json:"rightWindow"`
	Formats        []string             `json:"formats"`
	Format         string               `json:"format"`
	TargetTaskID   string               `json:"targetTaskId"`
	EvaluationMode string               `json:"evaluationMode"`
	LabelPolicy    string               `json:"labelPolicy"`
	IncludeLLM     bool                 `json:"includeLLM"`
	Params         map[string]any       `json:"params,omitempty"`
}

type ResearchTask struct {
	TaskID      string         `json:"taskId"`
	SessionID   string         `json:"sessionId,omitempty"`
	Action      string         `json:"action"`
	Status      string         `json:"status"`
	Progress    float64        `json:"progress"`
	QueuedAt    time.Time      `json:"queuedAt"`
	StartedAt   *time.Time     `json:"startedAt,omitempty"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	Error       string         `json:"error,omitempty"`
	ResultRef   string         `json:"resultRef,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Records     int            `json:"records,omitempty"`
	QueueLen    int            `json:"queueLen,omitempty"`
	CancelAsked bool           `json:"cancelRequested,omitempty"`
}

type researchTaskManagerStatus struct {
	Runtime      backendTaskRuntimeStats `json:"runtime"`
	TrackedTotal int                     `json:"trackedTotal"`
	ByStatus     map[string]int          `json:"byStatus"`
	UpdatedAt    time.Time               `json:"updatedAt"`
}

type researchTaskEntry struct {
	mu       sync.RWMutex
	task     ResearchTask
	request  researchTaskRequest
	tlsStore *TLSCaptureStore
	runtime  *backendTaskRuntimeEntry
	cancel   <-chan struct{}

	// retention fields are owned by researchTaskManager.mu.
	retentionTracked bool
	retentionNext    *researchTaskEntry
}

type researchSessionStore struct {
	mu       sync.RWMutex
	fsMu     sync.Mutex
	baseDir  string
	loaded   bool
	sessions map[string]*ResearchSession
}

type researchTaskManager struct {
	mu          sync.RWMutex
	runtime     *backendTaskRuntime
	tasks       map[string]*researchTaskEntry
	store       *researchSessionStore
	maxItems    int
	taskHandler func(*researchTaskEntry) error

	terminalHead *researchTaskEntry
	terminalTail *researchTaskEntry
}

var (
	researchSessionsStore = newResearchSessionStore("")
	researchTaskStore     = newResearchTaskManager(researchSessionsStore)
)

func newResearchSessionStore(baseDir string) *researchSessionStore {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join(platform.RuntimeSettingsDir(), "research")
	}
	return &researchSessionStore{baseDir: baseDir, sessions: make(map[string]*ResearchSession)}
}

func newResearchTaskManager(store *researchSessionStore) *researchTaskManager {
	if store == nil {
		store = newResearchSessionStore("")
	}
	manager := &researchTaskManager{
		store:    store,
		tasks:    make(map[string]*researchTaskEntry),
		maxItems: researchTaskStoreMaxItems,
	}
	manager.taskHandler = manager.runTask
	manager.runtime = newBackendTaskRuntime("research", researchTaskStoreMaxItems, manager.runRuntimeTask)
	return manager
}

func startResearchTaskWorker() {
	settings := snapshotRuntimeSettings().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	researchTaskStore.Start(settings.QueueSize)
	_ = researchSessionsStore.CleanupRetention(settings.ArtifactRetentionDays)
}

func (m *researchTaskManager) Start(queueSize int) {
	if m == nil {
		return
	}
	m.ensureRuntime().Start(queueSize)
}

func (m *researchTaskManager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	runtime := m.runtime
	m.mu.RUnlock()
	if runtime == nil {
		return nil
	}
	return runtime.Shutdown(ctx)
}

func (m *researchTaskManager) ensureRuntime() *backendTaskRuntime {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtime == nil {
		m.runtime = newBackendTaskRuntime("research", researchTaskStoreMaxItems, m.runRuntimeTask)
	}
	return m.runtime
}

func (m *researchTaskManager) runRuntimeTask(runtimeEntry *backendTaskRuntimeEntry) (err error) {
	var entry *researchTaskEntry
	defer func() {
		if recovered := recover(); recovered != nil {
			err = newBackendTaskPanicError(recovered)
			if entry != nil {
				entry.finish(researchTaskFailed, entry.progress(), err.Error(), nil)
			}
		}
		m.noteTaskFinished(entry)
	}()
	if runtimeEntry == nil {
		return errors.New("research task runtime entry is nil")
	}
	var ok bool
	entry, ok = runtimeEntry.Payload().(*researchTaskEntry)
	if !ok || entry == nil {
		return errors.New("research task payload is invalid")
	}
	if entry.isCanceled() {
		entry.finish(researchTaskCanceled, 1, "", nil)
		return errBackendTaskCanceled
	}
	if !entry.markRunning() {
		entry.finish(researchTaskCanceled, 1, "", nil)
		return errBackendTaskCanceled
	}
	handler := m.taskHandler
	if handler == nil {
		handler = m.runTask
	}
	err = handler(entry)
	if err != nil {
		if errors.Is(err, errResearchTaskCanceled) || entry.isCanceled() {
			entry.finish(researchTaskCanceled, 1, "", nil)
			return errBackendTaskCanceled
		}
		entry.finish(researchTaskFailed, entry.progress(), err.Error(), nil)
		return err
	}
	entry.finish(researchTaskSucceeded, 1, "", nil)
	return nil
}

var errResearchTaskCanceled = errors.New("research task canceled")

func (m *researchTaskManager) Submit(sessionID string, req researchTaskRequest, tlsStore *TLSCaptureStore) (ResearchTask, error) {
	if m == nil {
		return ResearchTask{}, errors.New("research task manager is unavailable")
	}
	m.Start(snapshotRuntimeSettings().ResearchProcessing.QueueSize)
	action := normalizeResearchAction(req.Action)
	if action == "cancel" {
		if strings.TrimSpace(req.TargetTaskID) == "" {
			return ResearchTask{}, errors.New("targetTaskId is required for cancel action")
		}
		return m.Cancel(req.TargetTaskID), nil
	}
	if _, err := m.store.Get(sessionID); err != nil {
		return ResearchTask{}, err
	}
	if req.Action == "" {
		req.Action = action
	}
	entry := &researchTaskEntry{
		task: ResearchTask{
			TaskID:    researchGenerateID("rtask"),
			SessionID: sessionID,
			Action:    action,
			Status:    researchTaskQueued,
			Progress:  0,
			QueuedAt:  time.Now().UTC(),
		},
		request:  req,
		tlsStore: tlsStore,
	}
	runtimeEntry := newBackendTaskRuntimeEntry(entry.task.TaskID, action, entry)
	entry.runtime = runtimeEntry
	entry.cancel = runtimeEntry.CancelChan()
	m.mu.Lock()
	m.tasks[entry.task.TaskID] = entry
	m.mu.Unlock()

	if err := m.ensureRuntime().Submit(runtimeEntry); err != nil {
		m.mu.Lock()
		delete(m.tasks, entry.task.TaskID)
		m.mu.Unlock()
		if errors.Is(err, errBackendTaskQueueFull) {
			return ResearchTask{}, errResearchQueueFull
		}
		return ResearchTask{}, err
	}
	m.pruneTrackedTasks()
	entry.setQueueLen(runtimeEntry.Snapshot().QueueLen)
	return entry.snapshot(), nil
}

var errResearchQueueFull = errors.New("research task queue is full")

func (m *researchTaskManager) Get(taskID string) (ResearchTask, bool) {
	if m == nil {
		return ResearchTask{}, false
	}
	m.mu.RLock()
	entry := m.tasks[strings.TrimSpace(taskID)]
	m.mu.RUnlock()
	if entry == nil {
		return ResearchTask{}, false
	}
	return entry.snapshot(), true
}

func (m *researchTaskManager) Status() researchTaskManagerStatus {
	if m == nil {
		return researchTaskManagerStatus{ByStatus: map[string]int{}, UpdatedAt: time.Now().UTC()}
	}
	runtimeStats := m.ensureRuntime().Stats()
	byStatus := map[string]int{}
	m.mu.RLock()
	for _, entry := range m.tasks {
		task := entry.snapshot()
		byStatus[task.Status]++
	}
	trackedTotal := len(m.tasks)
	m.mu.RUnlock()
	return researchTaskManagerStatus{
		Runtime:      runtimeStats,
		TrackedTotal: trackedTotal,
		ByStatus:     byStatus,
		UpdatedAt:    time.Now().UTC(),
	}
}

func (m *researchTaskManager) Cancel(taskID string) ResearchTask {
	if m == nil {
		return ResearchTask{TaskID: taskID, Status: researchTaskCanceled, FinishedAt: ptrTime(time.Now().UTC())}
	}
	m.mu.RLock()
	entry := m.tasks[strings.TrimSpace(taskID)]
	m.mu.RUnlock()
	if entry == nil {
		return ResearchTask{TaskID: taskID, Status: researchTaskCanceled, FinishedAt: ptrTime(time.Now().UTC()), Error: "task not found"}
	}
	entry.requestCancel()
	return entry.snapshot()
}

func (m *researchTaskManager) pruneTrackedTasks() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.pruneLocked()
	m.mu.Unlock()
}

func (m *researchTaskManager) pruneLocked() {
	if m == nil {
		return
	}
	limit := m.maxItems
	if limit <= 0 {
		limit = researchTaskStoreMaxItems
	}
	for len(m.tasks) > limit {
		entry := m.terminalHead
		if entry == nil {
			return
		}
		m.terminalHead = entry.retentionNext
		entry.retentionNext = nil
		if m.terminalHead == nil {
			m.terminalTail = nil
		}
		if current := m.tasks[entry.task.TaskID]; current == entry {
			delete(m.tasks, entry.task.TaskID)
		}
	}
}

func (m *researchTaskManager) noteTaskFinished(entry *researchTaskEntry) {
	if m == nil || entry == nil {
		return
	}
	// Only the worker drain path calls this method. A queued task that was
	// canceled by the API therefore stays addressable until its queue slot has
	// actually been consumed.
	m.mu.Lock()
	defer m.mu.Unlock()
	if current := m.tasks[entry.task.TaskID]; current != entry || entry.retentionTracked {
		return
	}
	entry.retentionTracked = true
	if m.terminalTail == nil {
		m.terminalHead = entry
	} else {
		m.terminalTail.retentionNext = entry
	}
	m.terminalTail = entry
	m.pruneLocked()
}

func (m *researchTaskManager) runTask(entry *researchTaskEntry) error {
	if m == nil || entry == nil {
		return errors.New("invalid research task")
	}
	action := normalizeResearchAction(entry.request.Action)
	sessionID := entry.task.SessionID
	switch action {
	case "scan_recent", "build_session":
		entry.setProgress(0.05)
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		limit := entry.request.Limit
		settings := snapshotRuntimeSettings().ResearchProcessing
		normalizeResearchProcessingSettings(&settings)
		if limit <= 0 {
			limit = entry.request.SourceFilter.Limit
		}
		if limit <= 0 {
			limit = researchDefaultTaskLimit
		}
		if limit > settings.MaxSessionEvents {
			limit = settings.MaxSessionEvents
		}
		filter, timerange := m.mergedSessionFilter(sessionID, entry.request.SourceFilter, entry.request.TimeRange)
		filter.Limit = limit
		events, err := collectResearchEvents(filter, timerange, limit, entry.tlsStore, entry)
		if err != nil {
			return err
		}
		entry.setProgress(0.65)
		results, err := buildResearchResultsWithCancel(sessionID, events, nil, entry)
		if err != nil {
			return err
		}
		entry.setProgress(0.85)
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		if err := m.store.ReplaceSessionEvents(sessionID, events, results, filter, timerange); err != nil {
			return err
		}
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		entry.setRecords(len(events))
		entry.setResultRef("results")
		entry.setResult(map[string]any{"events": len(events), "results": "available"})
		return nil
	case "compare_windows":
		events, err := m.store.LoadEvents(sessionID)
		if err != nil {
			return err
		}
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		compare := buildResearchWindowCompare(events, entry.request.LeftWindow, entry.request.RightWindow)
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		results, err := buildResearchResultsWithCancel(sessionID, events, compare, entry)
		if err != nil {
			return err
		}
		if previous, err := m.store.LoadResults(sessionID); err == nil {
			results.SecurityEvaluation = previous.SecurityEvaluation
		}
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		if err := m.store.SaveResults(sessionID, results); err != nil {
			return err
		}
		entry.setRecords(len(events))
		entry.setResultRef("results.compareWindows")
		entry.setResult(map[string]any{"compareWindows": compare})
		return nil
	case "security_eval":
		entry.setProgress(0.03)
		events, err := m.store.LoadEvents(sessionID)
		if err != nil {
			return err
		}
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		reportReq := researchSecurityEvaluationRequestFromTask(entry.request)
		report, err := buildResearchSecurityEvaluationReport(sessionID, events, reportReq, entry)
		if err != nil {
			return err
		}
		entry.setProgress(0.93)
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		if err := m.store.SaveSecurityEvaluation(sessionID, report); err != nil {
			return err
		}
		entry.setRecords(report.Totals.Total)
		entry.setResultRef("results.securityEvaluation")
		entry.setResult(map[string]any{
			"securityEvaluation": map[string]any{
				"total":             report.Totals.Total,
				"labeled":           report.Totals.Labeled,
				"falsePositiveRate": report.Metrics.FalsePositiveRate,
				"falseNegativeRate": report.Metrics.FalseNegativeRate,
				"accuracy":          report.Metrics.Accuracy,
				"mode":              report.Mode,
			},
		})
		return nil
	case "export_bundle":
		formats := researchFormatsFromRequest(entry.request)
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		refs, err := m.store.GenerateExportsWithCancel(sessionID, formats, entry)
		if err != nil {
			return err
		}
		result := map[string]any{"artifacts": refs}
		entry.setResultRef("artifactRefs")
		entry.setResult(result)
		return nil
	case "reset_session", "reset":
		if err := entry.checkCanceled(); err != nil {
			return err
		}
		if err := m.store.ResetSession(sessionID); err != nil {
			return err
		}
		entry.setResultRef("session")
		entry.setResult(map[string]any{"reset": true})
		return nil
	default:
		return fmt.Errorf("unsupported research task action %q", action)
	}
}

func (m *researchTaskManager) mergedSessionFilter(sessionID string, override ResearchSourceFilter, overrideRange ResearchTimeRange) (ResearchSourceFilter, ResearchTimeRange) {
	session, err := m.store.Get(sessionID)
	if err != nil {
		return normalizeResearchSourceFilter(override), normalizeResearchTimeRange(overrideRange)
	}
	filter := mergeResearchSourceFilter(session.SourceFilter, override)
	timerange := mergeResearchTimeRange(session.TimeRange, overrideRange)
	return filter, timerange
}

func (entry *researchTaskEntry) snapshot() ResearchTask {
	if entry == nil {
		return ResearchTask{}
	}
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	out := entry.task
	if out.Result != nil {
		out.Result = cloneStringAnyMap(out.Result)
	}
	return out
}

func (entry *researchTaskEntry) markRunning() bool {
	now := time.Now().UTC()
	entry.mu.Lock()
	if entry.task.Status == researchTaskCanceled {
		entry.mu.Unlock()
		return false
	}
	entry.task.Status = researchTaskRunning
	entry.task.StartedAt = &now
	entry.task.Progress = 0.01
	entry.mu.Unlock()
	return true
}

func (entry *researchTaskEntry) requestCancel() {
	if entry == nil {
		return
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	if researchTaskStatusTerminal(entry.task.Status) {
		entry.mu.Unlock()
		return
	}
	entry.task.CancelAsked = true
	if entry.task.Status == researchTaskQueued {
		entry.task.Status = researchTaskCanceled
		entry.task.Progress = 1
		entry.task.FinishedAt = &now
	}
	// Commit the shared runtime cancellation before releasing the domain lock.
	// finish() uses the same lock, so both task views linearize in one order.
	if entry.runtime != nil {
		entry.runtime.Cancel()
	}
	entry.mu.Unlock()
}

func (entry *researchTaskEntry) isCanceled() bool {
	if entry == nil {
		return false
	}
	if entry.runtime != nil {
		return entry.runtime.IsCanceled()
	}
	// Runtime-less entries (tests, detached tasks): honor an explicit cancel
	// request or a closed cancel channel.
	entry.mu.RLock()
	asked := entry.task.CancelAsked
	entry.mu.RUnlock()
	if asked {
		return true
	}
	if entry.cancel == nil {
		return false
	}
	select {
	case <-entry.cancel:
		return true
	default:
		return false
	}
}

func (entry *researchTaskEntry) checkCanceled() error {
	if entry != nil && entry.isCanceled() {
		return errResearchTaskCanceled
	}
	return nil
}

func (entry *researchTaskEntry) setProgress(progress float64) {
	if entry == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	entry.mu.Lock()
	if entry.task.Status == researchTaskRunning && progress > entry.task.Progress {
		entry.task.Progress = progress
	}
	entry.mu.Unlock()
	if entry.runtime != nil {
		entry.runtime.SetProgress(progress)
	}
}

func (entry *researchTaskEntry) progress() float64 {
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	return entry.task.Progress
}

func (entry *researchTaskEntry) setRecords(records int) {
	entry.mu.Lock()
	entry.task.Records = records
	entry.mu.Unlock()
}

func (entry *researchTaskEntry) setQueueLen(queueLen int) {
	entry.mu.Lock()
	entry.task.QueueLen = queueLen
	entry.mu.Unlock()
}

func (entry *researchTaskEntry) setResultRef(ref string) {
	entry.mu.Lock()
	entry.task.ResultRef = ref
	entry.mu.Unlock()
}

func (entry *researchTaskEntry) setResult(result map[string]any) {
	entry.mu.Lock()
	entry.task.Result = result
	entry.mu.Unlock()
}

func (entry *researchTaskEntry) finish(status string, progress float64, message string, result map[string]any) {
	if entry == nil {
		return
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	if researchTaskStatusTerminal(entry.task.Status) && entry.task.FinishedAt != nil {
		entry.mu.Unlock()
		return
	}
	if status != researchTaskCanceled && (entry.task.CancelAsked || entry.isCanceled()) {
		status = researchTaskCanceled
		progress = 1
		message = ""
	}
	entry.task.Status = status
	entry.task.Progress = progress
	entry.task.FinishedAt = &now
	entry.task.Error = message
	if result != nil {
		entry.task.Result = result
	}
	entry.mu.Unlock()
}

func researchTaskStatusTerminal(status string) bool {
	switch status {
	case researchTaskSucceeded, researchTaskFailed, researchTaskCanceled:
		return true
	default:
		return false
	}
}

// StartTaskWorker launches the research task manager worker loop.
func StartTaskWorker() { startResearchTaskWorker() }

// TaskStoreShutdown gracefully stops the shared research task manager.
func TaskStoreShutdown(ctx context.Context) error {
	return researchTaskStore.Shutdown(ctx)
}
