package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"

	"github.com/gin-gonic/gin"
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
	mu         sync.RWMutex
	task       ResearchTask
	request    researchTaskRequest
	tlsStore   *TLSCaptureStore
	runtime    *backendTaskRuntimeEntry
	cancel     chan struct{}
	cancelOnce sync.Once
}

type researchSessionStore struct {
	mu       sync.RWMutex
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
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
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
		m.pruneTrackedTasks()
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
	m.Start(runtimeSettingsStore.Snapshot().ResearchProcessing.QueueSize)
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
	entry.cancel = runtimeEntry.cancel
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
	if len(m.tasks) <= limit {
		return
	}
	type item struct {
		id string
		ts time.Time
	}
	items := make([]item, 0, len(m.tasks))
	for id, entry := range m.tasks {
		task := entry.snapshot()
		if !researchTaskStatusTerminal(task.Status) {
			continue
		}
		ts := task.QueuedAt
		if task.FinishedAt != nil {
			ts = *task.FinishedAt
		}
		items = append(items, item{id: id, ts: ts})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ts.Equal(items[j].ts) {
			return items[i].id < items[j].id
		}
		return items[i].ts.Before(items[j].ts)
	})
	for len(m.tasks) > limit && len(items) > 0 {
		delete(m.tasks, items[0].id)
		items = items[1:]
	}
}

func researchTaskStatusTerminal(status string) bool {
	switch status {
	case researchTaskSucceeded, researchTaskFailed, researchTaskCanceled:
		return true
	default:
		return false
	}
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
		settings := runtimeSettingsStore.Snapshot().ResearchProcessing
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
	if entry.runtime != nil {
		entry.runtime.Cancel()
	} else {
		entry.cancelOnce.Do(func() { close(entry.cancel) })
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	entry.task.CancelAsked = true
	if entry.task.Status == researchTaskQueued {
		entry.task.Status = researchTaskCanceled
		entry.task.Progress = 1
		entry.task.FinishedAt = &now
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
	if entry.task.Status == researchTaskCanceled && status == researchTaskSucceeded {
		status = researchTaskCanceled
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

func (s *researchSessionStore) ensureLoaded() error {
	if s == nil {
		return errors.New("research session store is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(s.baseDir, entry.Name(), "session.json")
		payload, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var session ResearchSession
		if err := json.Unmarshal(payload, &session); err != nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		normalizeResearchSession(&session)
		s.sessions[session.ID] = &session
	}
	s.loaded = true
	return nil
}

func (s *researchSessionStore) List() ([]ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	out := make([]ResearchSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, cloneResearchSession(session))
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *researchSessionStore) Create(req researchCreateSessionRequest) (ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchSession{}, err
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Research Session " + now.Format("2006-01-02 15:04:05")
	}
	session := ResearchSession{
		ID:           researchGenerateID("rs"),
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		Tags:         normalizeResearchTags(req.Tags),
		CreatedAt:    now,
		UpdatedAt:    now,
		SourceFilter: normalizeResearchSourceFilter(req.SourceFilter),
		TimeRange:    normalizeResearchTimeRange(req.TimeRange),
		Status:       researchSessionEmpty,
		Summary:      ResearchSessionSummary{SchemaVersion: researchSchemaVersion},
		ArtifactRefs: map[string]ResearchArtifactRef{},
	}
	if err := s.saveSessionLocked(&session); err != nil {
		return ResearchSession{}, err
	}
	s.mu.Lock()
	s.sessions[session.ID] = &session
	s.mu.Unlock()
	return cloneResearchSession(&session), nil
}

func (s *researchSessionStore) Get(id string) (ResearchSession, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchSession{}, err
	}
	id = strings.TrimSpace(id)
	s.mu.RLock()
	session := s.sessions[id]
	s.mu.RUnlock()
	if session == nil {
		return ResearchSession{}, os.ErrNotExist
	}
	return cloneResearchSession(session), nil
}

func (s *researchSessionStore) Delete(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	s.mu.Lock()
	if s.sessions[id] == nil {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	return os.RemoveAll(filepath.Join(s.baseDir, id))
}

func (s *researchSessionStore) ReplaceSessionEvents(id string, events []ResearchEvent, results ResearchResults, filter ResearchSourceFilter, timerange ResearchTimeRange) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	session := s.sessions[id]
	if session == nil {
		s.mu.Unlock()
		return os.ErrNotExist
	}
	session.Status = researchSessionBuilding
	session.UpdatedAt = now
	session.SourceFilter = normalizeResearchSourceFilter(filter)
	session.TimeRange = normalizeResearchTimeRange(timerange)
	session.LastError = ""
	if err := s.saveSessionLocked(session); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	if err := s.writeEvents(id, events); err != nil {
		s.markSessionError(id, err)
		return err
	}
	if err := s.writeResults(id, results); err != nil {
		s.markSessionError(id, err)
		return err
	}

	s.mu.Lock()
	session = s.sessions[id]
	if session != nil {
		session.Status = researchSessionReady
		if len(events) == 0 {
			session.Status = researchSessionEmpty
		}
		session.UpdatedAt = time.Now().UTC()
		session.Summary = buildResearchSessionSummary(events, results)
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
		if err := s.saveSessionLocked(session); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *researchSessionStore) SaveResults(id string, results ResearchResults) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if err := s.writeResults(id, results); err != nil {
		return err
	}
	events, _ := s.LoadEvents(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	session.Summary = buildResearchSessionSummary(events, results)
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionReady
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) SaveSecurityEvaluation(id string, report ResearchSecurityEvaluationReport) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if _, err := s.Get(id); err != nil {
		return err
	}
	results, err := s.LoadResults(id)
	if err != nil {
		return err
	}
	report.SessionID = id
	if strings.TrimSpace(report.SchemaVersion) == "" {
		report.SchemaVersion = researchSecurityEvaluationSchemaVersion
	}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = time.Now().UTC()
	}
	results.SecurityEvaluation = &report
	if err := s.writeResults(id, results); err != nil {
		return err
	}
	payload, err := researchSecurityEvaluationJSONBytes(&report)
	if err != nil {
		return err
	}
	artifactDir := filepath.Join(s.baseDir, id, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(artifactDir, "security-evaluation.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return err
	}
	ref := ResearchArtifactRef{Format: "security_json", Name: "security-evaluation.json", Path: path, ContentType: "application/json; charset=utf-8", Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
	events, _ := s.LoadEvents(id)
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	if session.ArtifactRefs == nil {
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
	}
	session.ArtifactRefs["security_json"] = ref
	session.Summary = buildResearchSessionSummary(events, results)
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionReady
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) ResetSession(id string) error {
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	dir := filepath.Join(s.baseDir, id)
	_ = os.Remove(filepath.Join(dir, "events.jsonl"))
	_ = os.Remove(filepath.Join(dir, "results.json"))
	_ = os.RemoveAll(filepath.Join(dir, "artifacts"))
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[id]
	if session == nil {
		return os.ErrNotExist
	}
	session.UpdatedAt = time.Now().UTC()
	session.Status = researchSessionEmpty
	session.Summary = ResearchSessionSummary{SchemaVersion: researchSchemaVersion}
	session.ArtifactRefs = map[string]ResearchArtifactRef{}
	session.LastError = ""
	return s.saveSessionLocked(session)
}

func (s *researchSessionStore) LoadEvents(id string) ([]ResearchEvent, error) {
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	path := filepath.Join(s.baseDir, id, "events.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []ResearchEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	events := make([]ResearchEvent, 0)
	for {
		var event ResearchEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *researchSessionStore) LoadResults(id string) (ResearchResults, error) {
	if err := s.ensureLoaded(); err != nil {
		return ResearchResults{}, err
	}
	if _, err := s.Get(id); err != nil {
		return ResearchResults{}, err
	}
	path := filepath.Join(s.baseDir, id, "results.json")
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		events, _ := s.LoadEvents(id)
		return buildResearchResults(id, events, nil), nil
	}
	if err != nil {
		return ResearchResults{}, err
	}
	var results ResearchResults
	if err := json.Unmarshal(payload, &results); err != nil {
		return ResearchResults{}, err
	}
	return results, nil
}

func (s *researchSessionStore) GenerateExports(id string, formats []string) ([]ResearchArtifactRef, error) {
	return s.GenerateExportsWithCancel(id, formats, nil)
}

func (s *researchSessionStore) GenerateExportsWithCancel(id string, formats []string, entry *researchTaskEntry) ([]ResearchArtifactRef, error) {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	formats = normalizeResearchFormats(formats)
	if len(formats) == 0 {
		formats = splitResearchFormats(settings.ExportFormats)
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	session, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	events, err := s.LoadEvents(id)
	if err != nil {
		return nil, err
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	results, err := s.LoadResults(id)
	if err != nil {
		return nil, err
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	artifactDir := filepath.Join(s.baseDir, id, "artifacts")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		return nil, err
	}
	refs := make([]ResearchArtifactRef, 0, len(formats))
	artifactMap := cloneArtifactRefs(session.ArtifactRefs)
	payloadsByName := map[string][]byte{}
	for _, format := range formats {
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		var payload []byte
		var contentType string
		var name string
		switch format {
		case "jsonl":
			payload = researchEventsJSONLBytes(events)
			contentType = "application/x-ndjson; charset=utf-8"
			name = "events.jsonl"
		case "csv":
			payload, err = researchEventsCSVBytes(events)
			if err != nil {
				return nil, err
			}
			contentType = "text/csv; charset=utf-8"
			name = "events.csv"
		case "json":
			payload, err = json.MarshalIndent(gin.H{"session": session, "results": results, "events": events}, "", "  ")
			if err != nil {
				return nil, err
			}
			contentType = "application/json; charset=utf-8"
			name = "research.json"
		case "security_json":
			payload, err = researchSecurityEvaluationJSONBytes(results.SecurityEvaluation)
			if err != nil {
				return nil, err
			}
			contentType = "application/json; charset=utf-8"
			name = "security-evaluation.json"
		case "security_jsonl":
			if results.SecurityEvaluation == nil {
				return nil, errors.New("security evaluation report is unavailable")
			}
			payload = researchSecurityEvaluationJSONLBytes(results.SecurityEvaluation)
			contentType = "application/x-ndjson; charset=utf-8"
			name = "security-evaluation.jsonl"
		case "security_csv":
			payload, err = researchSecurityEvaluationCSVBytes(results.SecurityEvaluation)
			if err != nil {
				return nil, err
			}
			contentType = "text/csv; charset=utf-8"
			name = "security-evaluation.csv"
		case "bundle":
			payload, err = researchBundleZipBytes(session, events, results, settings)
			if err != nil {
				return nil, err
			}
			contentType = "application/zip"
			name = "research-bundle.zip"
		default:
			return nil, fmt.Errorf("unsupported export format %q", format)
		}
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		path := filepath.Join(artifactDir, name)
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return nil, err
		}
		payloadsByName[name] = payload
		ref := ResearchArtifactRef{Format: format, Name: name, Path: path, ContentType: contentType, Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
		artifactMap[format] = ref
		refs = append(refs, ref)
	}
	if len(payloadsByName) > 0 {
		if err := entry.checkCanceled(); err != nil {
			return nil, err
		}
		manifest := researchBuildManifest(session, events, settings, payloadsByName, artifactMap)
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(artifactDir, "manifest.json")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return nil, err
		}
		ref := ResearchArtifactRef{Format: "manifest", Name: "manifest.json", Path: path, ContentType: "application/json; charset=utf-8", Bytes: int64(len(payload)), SHA256: researchSHA256Hex(payload), CreatedAt: time.Now().UTC()}
		artifactMap["manifest"] = ref
		refs = append(refs, ref)
	}
	if err := entry.checkCanceled(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if current := s.sessions[id]; current != nil {
		current.ArtifactRefs = artifactMap
		current.UpdatedAt = time.Now().UTC()
		if err := s.saveSessionLocked(current); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	return refs, nil
}

func (s *researchSessionStore) ExportArtifact(id, format string) (ResearchArtifactRef, []byte, error) {
	format = normalizeResearchFormat(format)
	if format == "" {
		format = "bundle"
	}
	if _, err := s.Get(id); err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	session, _ := s.Get(id)
	ref, ok := session.ArtifactRefs[format]
	if !ok || strings.TrimSpace(ref.Path) == "" {
		refs, err := s.GenerateExports(id, []string{format})
		if err != nil {
			return ResearchArtifactRef{}, nil, err
		}
		if len(refs) == 0 {
			return ResearchArtifactRef{}, nil, os.ErrNotExist
		}
		ref = refs[0]
	}
	payload, err := os.ReadFile(ref.Path)
	if errors.Is(err, os.ErrNotExist) {
		refs, genErr := s.GenerateExports(id, []string{format})
		if genErr != nil {
			return ResearchArtifactRef{}, nil, genErr
		}
		ref = refs[0]
		payload, err = os.ReadFile(ref.Path)
	}
	if err != nil {
		return ResearchArtifactRef{}, nil, err
	}
	return ref, payload, nil
}

func (s *researchSessionStore) CleanupRetention(days int) error {
	if s == nil {
		return nil
	}
	if days <= 0 {
		days = researchProcessingDefaultArtifactRetentionDays
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	sessions, err := s.List()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		changed := false
		refs := cloneArtifactRefs(session.ArtifactRefs)
		for format, ref := range refs {
			if ref.CreatedAt.IsZero() || ref.CreatedAt.After(cutoff) {
				continue
			}
			if strings.TrimSpace(ref.Path) != "" {
				_ = os.Remove(ref.Path)
			}
			delete(refs, format)
			changed = true
		}
		if changed {
			s.mu.Lock()
			if current := s.sessions[session.ID]; current != nil {
				current.ArtifactRefs = refs
				current.UpdatedAt = time.Now().UTC()
				_ = s.saveSessionLocked(current)
			}
			s.mu.Unlock()
		}
	}
	return nil
}

func (s *researchSessionStore) writeEvents(id string, events []ResearchEvent) error {
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "events.jsonl")
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func (s *researchSessionStore) writeResults(id string, results ResearchResults) error {
	payload, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.baseDir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "results.json"), payload, 0o600)
}

func (s *researchSessionStore) saveSessionLocked(session *ResearchSession) error {
	if s == nil || session == nil {
		return errors.New("research session is nil")
	}
	normalizeResearchSession(session)
	dir := filepath.Join(s.baseDir, session.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), payload, 0o600)
}

func (s *researchSessionStore) markSessionError(id string, err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if session := s.sessions[id]; session != nil {
		session.Status = researchSessionError
		session.LastError = err.Error()
		session.UpdatedAt = time.Now().UTC()
		_ = s.saveSessionLocked(session)
	}
}

func collectResearchEvents(filter ResearchSourceFilter, timerange ResearchTimeRange, limit int, tlsStore *TLSCaptureStore, entry *researchTaskEntry) ([]ResearchEvent, error) {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	filter = normalizeResearchSourceFilter(filter)
	timerange = normalizeResearchTimeRange(timerange)
	if limit <= 0 {
		limit = filter.Limit
	}
	if limit <= 0 {
		limit = settings.MaxSessionEvents
	}
	if limit > settings.MaxSessionEvents {
		limit = settings.MaxSessionEvents
	}
	if limit > researchMaxTaskLimit && settings.MaxSessionEvents <= researchMaxTaskLimit {
		limit = researchMaxTaskLimit
	}
	records, _, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		return nil, err
	}
	events := make([]ResearchEvent, 0, len(records))
	for index, record := range records {
		if entry != nil && index%256 == 0 && entry.isCanceled() {
			return nil, errResearchTaskCanceled
		}
		if event, ok := researchEventFromCapturedRecord(record); ok && researchEventMatches(event, filter, timerange) {
			events = append(events, event)
		}
	}
	if researchIncludeTLS(filter) && tlsStore != nil {
		for _, tlsEvent := range tlsStore.Recent(limit) {
			if entry != nil && entry.isCanceled() {
				return nil, errResearchTaskCanceled
			}
			if event, ok := researchEventFromTLS(tlsEvent); ok && researchEventMatches(event, filter, timerange) {
				events = append(events, event)
			}
		}
	}
	if researchIncludeUploaded(filter) {
		for _, uploaded := range agentSightUploadedEvents.Recent(limit) {
			if entry != nil && entry.isCanceled() {
				return nil, errResearchTaskCanceled
			}
			if event, ok := researchEventFromAgentSight(uploaded); ok && researchEventMatches(event, filter, timerange) {
				events = append(events, event)
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func researchEventFromCapturedRecord(record CapturedEventRecord) (ResearchEvent, bool) {
	sample, ok := researchEventSampleFromRecord(record)
	if !ok {
		return ResearchEvent{}, false
	}
	record = normalizeCapturedEventRecord(record)
	event := record.Event
	features := researchFeaturesFromEvent(event, record.Envelope)
	redaction := ""
	if event != nil {
		redaction = strings.TrimSpace(event.GetRedactionLevel())
	}
	if redaction == "" {
		redaction = envelopeRedactionState(record.Envelope)
	}
	if redaction == "" {
		redaction = "metadata_only"
	}
	return ResearchEvent{
		ID:             sample.ID,
		Timestamp:      sample.Timestamp,
		Time:           sample.Time,
		Source:         sample.Source,
		EventType:      sample.EventType,
		PID:            sample.PID,
		PPID:           sample.PPID,
		Comm:           sample.Comm,
		TraceID:        sample.TraceID,
		SpanID:         sample.SpanID,
		Target:         sample.Target,
		RiskScore:      sample.RiskScore,
		Decision:       sample.Decision,
		RedactionLevel: redaction,
		Features:       features,
	}, true
}

func researchEventFromTLS(event TLSPlaintextEvent) (ResearchEvent, bool) {
	timestamp := event.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	eventType := "TLS_PLAINTEXT"
	source := "ssl"
	switch event.Type {
	case "http_request", "http_response":
		eventType = "HTTP_MESSAGE"
		source = "http_parser"
	case "sse_message":
		eventType = "SSE_MESSAGE"
		source = "sse_processor"
	}
	target := platform.FirstNonEmpty(event.URL, event.Host)
	features := researchSafeFeatureMap(map[string]any{
		"runner":          "tls",
		"type":            event.Type,
		"direction":       event.Direction,
		"library":         event.Lib,
		"method":          event.Method,
		"url":             event.URL,
		"host":            event.Host,
		"status":          event.StatusCode,
		"bodySize":        event.BodySize,
		"capturedLen":     event.CapturedLen,
		"originalLen":     event.OriginalLen,
		"contentType":     event.ContentType,
		"truncated":       event.Truncated,
		"rootAgentPid":    event.RootAgentPID,
		"agentRunId":      event.AgentRunID,
		"taskId":          event.TaskID,
		"toolCallId":      event.ToolCallID,
		"toolName":        event.ToolName,
		"promptDigest":    event.PromptDigest,
		"promptLen":       event.PromptLen,
		"vendor":          event.Vendor,
		"loopAlert":       event.LoopAlert,
		"redaction_state": event.RedactionState,
	})
	return ResearchEvent{
		ID:             researchStableID("tls", timestamp.UnixMilli(), source, event.PID, event.Comm, event.Type, target),
		Timestamp:      timestamp.UnixMilli(),
		Time:           timestamp.Format(time.RFC3339Nano),
		Source:         source,
		EventType:      eventType,
		PID:            event.PID,
		Comm:           platform.FirstNonEmpty(event.Comm, "tls"),
		TraceID:        event.TraceID,
		SpanID:         event.SpanID,
		Target:         target,
		RedactionLevel: platform.FirstNonEmpty(event.RedactionState, "sanitized"),
		Features:       features,
	}, true
}

func researchEventFromAgentSight(uploaded agentSightExportEvent) (ResearchEvent, bool) {
	if uploaded.Timestamp <= 0 {
		uploaded.Timestamp = time.Now().UTC().UnixMilli()
	}
	data := researchSafeFeatureMap(uploaded.Data)
	eventType := platform.FirstNonEmpty(researchStringFromMap(data, "event_type"), researchStringFromMap(data, "eventType"), researchStringFromMap(data, "type"), "agentsight_event")
	target := platform.FirstNonEmpty(researchStringFromMap(data, "target"), researchStringFromMap(data, "path"), researchStringFromMap(data, "url"), researchStringFromMap(data, "host"), researchStringFromMap(data, "domain"))
	redaction := platform.FirstNonEmpty(researchStringFromMap(data, "redaction_state"), researchStringFromMap(data, "redactionLevel"), "sanitized")
	id := strings.TrimSpace(uploaded.ID)
	if id == "" {
		id = researchStableID("agentsight", uploaded.Timestamp, uploaded.Source, uploaded.PID, uploaded.Comm, data)
	}
	return ResearchEvent{
		ID:             id,
		Timestamp:      uploaded.Timestamp,
		Time:           time.UnixMilli(uploaded.Timestamp).UTC().Format(time.RFC3339Nano),
		Source:         platform.FirstNonEmpty(uploaded.Source, "uploaded"),
		EventType:      eventType,
		PID:            uploaded.PID,
		PPID:           uploaded.PPID,
		Comm:           uploaded.Comm,
		TraceID:        uploaded.TraceID,
		SpanID:         uploaded.SpanID,
		Target:         target,
		RiskScore:      researchFloatFromAny(data["riskScore"]),
		Decision:       researchStringFromMap(data, "decision"),
		RedactionLevel: redaction,
		Features:       data,
	}, true
}

func researchFeaturesFromEvent(event any, envelope any) map[string]any {
	pbEvent, _ := event.(interface {
		GetTag() string
		GetPath() string
		GetExtraPath() string
		GetNetDirection() string
		GetNetEndpoint() string
		GetNetBytes() uint32
		GetNetFamily() string
		GetRetval() int64
		GetExtraInfo() string
		GetBytes() uint64
		GetMode() string
		GetDomain() string
		GetSockType() string
		GetProtocol() uint32
		GetUid() uint32
		GetGid() uint32
		GetUidArg() uint32
		GetGidArg() uint32
		GetDurationNs() uint64
		GetSchemaVersion() string
		GetCgroupId() uint64
		GetRootAgentPid() uint32
		GetAgentRunId() string
		GetConversationId() string
		GetTurnId() string
		GetToolCallId() string
		GetToolName() string
		GetTaskId() string
		GetTgid() uint32
		GetFlowId() string
		GetSrcIp() string
		GetSrcPort() uint32
		GetDstIp() string
		GetDstPort() uint32
		GetTransport() string
		GetAppProtocol() string
		GetServiceName() string
		GetDnsName() string
		GetSni() string
		GetHttpHost() string
		GetTlsAlpn() string
		GetInterfaceName() string
		GetBytesIn() uint64
		GetBytesOut() uint64
		GetPacketsIn() uint64
		GetPacketsOut() uint64
		GetIpScope() string
		GetSanitizedFields() []string
	})
	if pbEvent == nil {
		return map[string]any{}
	}
	features := map[string]any{
		"tag":             pbEvent.GetTag(),
		"path":            pbEvent.GetPath(),
		"extraPath":       pbEvent.GetExtraPath(),
		"netDirection":    pbEvent.GetNetDirection(),
		"netEndpoint":     pbEvent.GetNetEndpoint(),
		"netBytes":        pbEvent.GetNetBytes(),
		"netFamily":       pbEvent.GetNetFamily(),
		"retval":          pbEvent.GetRetval(),
		"extraInfo":       pbEvent.GetExtraInfo(),
		"bytes":           pbEvent.GetBytes(),
		"mode":            pbEvent.GetMode(),
		"domain":          pbEvent.GetDomain(),
		"sockType":        pbEvent.GetSockType(),
		"protocol":        pbEvent.GetProtocol(),
		"uid":             pbEvent.GetUid(),
		"gid":             pbEvent.GetGid(),
		"uidArg":          pbEvent.GetUidArg(),
		"gidArg":          pbEvent.GetGidArg(),
		"durationNs":      pbEvent.GetDurationNs(),
		"schemaVersion":   pbEvent.GetSchemaVersion(),
		"cgroupId":        pbEvent.GetCgroupId(),
		"rootAgentPid":    pbEvent.GetRootAgentPid(),
		"agentRunId":      pbEvent.GetAgentRunId(),
		"conversationId":  pbEvent.GetConversationId(),
		"turnId":          pbEvent.GetTurnId(),
		"toolCallId":      pbEvent.GetToolCallId(),
		"toolName":        pbEvent.GetToolName(),
		"taskId":          pbEvent.GetTaskId(),
		"tgid":            pbEvent.GetTgid(),
		"flowId":          pbEvent.GetFlowId(),
		"srcIp":           pbEvent.GetSrcIp(),
		"srcPort":         pbEvent.GetSrcPort(),
		"dstIp":           pbEvent.GetDstIp(),
		"dstPort":         pbEvent.GetDstPort(),
		"transport":       pbEvent.GetTransport(),
		"appProtocol":     pbEvent.GetAppProtocol(),
		"serviceName":     pbEvent.GetServiceName(),
		"dnsName":         pbEvent.GetDnsName(),
		"sni":             pbEvent.GetSni(),
		"httpHost":        pbEvent.GetHttpHost(),
		"tlsAlpn":         pbEvent.GetTlsAlpn(),
		"interfaceName":   pbEvent.GetInterfaceName(),
		"bytesIn":         pbEvent.GetBytesIn(),
		"bytesOut":        pbEvent.GetBytesOut(),
		"packetsIn":       pbEvent.GetPacketsIn(),
		"packetsOut":      pbEvent.GetPacketsOut(),
		"ipScope":         pbEvent.GetIpScope(),
		"sanitizedFields": pbEvent.GetSanitizedFields(),
	}
	return researchSafeFeatureMap(features)
}

func buildResearchResults(sessionID string, events []ResearchEvent, compare *ResearchWindowCompare) ResearchResults {
	results, _ := buildResearchResultsWithCancel(sessionID, events, compare, nil)
	return results
}

func buildResearchResultsWithCancel(sessionID string, events []ResearchEvent, compare *ResearchWindowCompare, entry *researchTaskEntry) (ResearchResults, error) {
	now := time.Now().UTC()
	samples := make([]researchEventSample, 0, len(events))
	byTarget := map[string]int{}
	byDecision := map[string]int{}
	riskAlerts := make([]ResearchRiskFinding, 0)
	for index, event := range events {
		if index%256 == 0 {
			if err := entry.checkCanceled(); err != nil {
				return ResearchResults{}, err
			}
		}
		samples = append(samples, researchSampleFromResearchEvent(event))
		incrementResearchCount(byTarget, event.Target)
		incrementResearchCount(byDecision, event.Decision)
		if event.RiskScore >= 80 || strings.EqualFold(event.Decision, "ALERT") || strings.EqualFold(event.Decision, "BLOCK") || strings.Contains(strings.ToLower(event.EventType), "alert") {
			riskAlerts = append(riskAlerts, ResearchRiskFinding{EventID: event.ID, Timestamp: event.Timestamp, Time: event.Time, Source: event.Source, EventType: event.EventType, PID: event.PID, Comm: event.Comm, Target: event.Target, RiskScore: event.RiskScore, Decision: event.Decision, TraceID: event.TraceID, Associated: researchRiskAssociation(event)})
		}
	}
	if err := entry.checkCanceled(); err != nil {
		return ResearchResults{}, err
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	loopStatus := loopDetectionWorkerStore.Status()
	results := ResearchResults{
		SchemaVersion:      researchSchemaVersion,
		SessionID:          sessionID,
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
		Summary:            buildResearchProcessingSummary(samples, settings),
		TopTargets:         topResearchCounts(byTarget, settings.TopK),
		TopDecisions:       topResearchCounts(byDecision, settings.TopK),
		LoopFindings:       matchResearchLoopFindings(events, loopStatus.RecentFindings),
		RiskAlerts:         riskAlerts,
		KernelRiskFeedback: currentResearchKernelRiskFeedbackInfo(),
		CompareWindows:     compare,
	}
	return results, nil
}

func buildResearchSessionSummary(events []ResearchEvent, results ResearchResults) ResearchSessionSummary {
	now := time.Now().UTC()
	summary := ResearchSessionSummary{SchemaVersion: researchSchemaVersion, EventCount: len(events), GeneratedTimestamp: now.UnixMilli(), GeneratedTime: now.Format(time.RFC3339Nano), LoopFindings: len(results.LoopFindings), RiskAlerts: len(results.RiskAlerts)}
	if len(events) == 0 {
		return summary
	}
	bySource := map[string]int{}
	byType := map[string]int{}
	byComm := map[string]int{}
	for _, event := range events {
		if summary.EarliestTimestamp == 0 || event.Timestamp < summary.EarliestTimestamp {
			summary.EarliestTimestamp = event.Timestamp
		}
		if event.Timestamp > summary.LatestTimestamp {
			summary.LatestTimestamp = event.Timestamp
		}
		if event.RiskScore > summary.MaxRiskScore {
			summary.MaxRiskScore = event.RiskScore
		}
		incrementResearchCount(bySource, event.Source)
		incrementResearchCount(byType, event.EventType)
		incrementResearchCount(byComm, event.Comm)
	}
	if summary.EarliestTimestamp > 0 {
		summary.EarliestTime = time.UnixMilli(summary.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if summary.LatestTimestamp > 0 {
		summary.LatestTime = time.UnixMilli(summary.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if top := topResearchCounts(bySource, 1); len(top) > 0 {
		summary.TopSource = top[0].Key
	}
	if top := topResearchCounts(byType, 1); len(top) > 0 {
		summary.TopEventType = top[0].Key
	}
	if top := topResearchCounts(byComm, 1); len(top) > 0 {
		summary.TopComm = top[0].Key
	}
	return summary
}

func buildResearchWindowCompare(events []ResearchEvent, leftRange, rightRange ResearchTimeRange) *ResearchWindowCompare {
	leftRange = normalizeResearchTimeRange(leftRange)
	rightRange = normalizeResearchTimeRange(rightRange)
	leftEvents := filterResearchEventsByRange(events, leftRange)
	rightEvents := filterResearchEventsByRange(events, rightRange)
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	leftSummary := buildResearchProcessingSummary(researchSamplesFromResearchEvents(leftEvents), settings)
	rightSummary := buildResearchProcessingSummary(researchSamplesFromResearchEvents(rightEvents), settings)
	return &ResearchWindowCompare{
		Left:   ResearchWindowSummary{Name: "left", TimeRange: leftRange, Summary: leftSummary},
		Right:  ResearchWindowSummary{Name: "right", TimeRange: rightRange, Summary: rightSummary},
		Deltas: researchSummaryDeltas(leftSummary, rightSummary),
	}
}

func researchSummaryDeltas(left, right researchProcessingSummary) []ResearchCountDelta {
	var deltas []ResearchCountDelta
	appendDeltas := func(category string, a, b []researchCount) {
		counts := map[string][2]int{}
		for _, item := range a {
			counts[item.Key] = [2]int{item.Count, counts[item.Key][1]}
		}
		for _, item := range b {
			counts[item.Key] = [2]int{counts[item.Key][0], item.Count}
		}
		keys := make([]string, 0, len(counts))
		for key := range counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			pair := counts[key]
			deltas = append(deltas, ResearchCountDelta{Category: category, Key: key, Left: pair[0], Right: pair[1], Delta: pair[1] - pair[0]})
		}
	}
	appendDeltas("source", left.BySource, right.BySource)
	appendDeltas("eventType", left.ByType, right.ByType)
	appendDeltas("comm", left.ByComm, right.ByComm)
	sort.SliceStable(deltas, func(i, j int) bool {
		ai := deltas[i].Delta
		if ai < 0 {
			ai = -ai
		}
		aj := deltas[j].Delta
		if aj < 0 {
			aj = -aj
		}
		if ai == aj {
			if deltas[i].Category == deltas[j].Category {
				return deltas[i].Key < deltas[j].Key
			}
			return deltas[i].Category < deltas[j].Category
		}
		return ai > aj
	})
	if len(deltas) > 100 {
		deltas = deltas[:100]
	}
	return deltas
}

func researchSampleFromResearchEvent(event ResearchEvent) researchEventSample {
	return researchEventSample{ID: event.ID, Timestamp: event.Timestamp, Time: event.Time, Source: event.Source, EventType: event.EventType, PID: event.PID, PPID: event.PPID, Comm: event.Comm, TraceID: event.TraceID, SpanID: event.SpanID, Title: strings.Join(nonEmptyResearchParts(event.Source, event.EventType, event.Comm, event.Target), " · "), Target: event.Target, RiskScore: event.RiskScore, Decision: event.Decision}
}

func researchSamplesFromResearchEvents(events []ResearchEvent) []researchEventSample {
	samples := make([]researchEventSample, 0, len(events))
	for _, event := range events {
		samples = append(samples, researchSampleFromResearchEvent(event))
	}
	return samples
}

func matchResearchLoopFindings(events []ResearchEvent, findings []loopDetectionFinding) []loopDetectionFinding {
	if len(events) == 0 || len(findings) == 0 {
		return nil
	}
	pids := map[uint32]struct{}{}
	traces := map[string]struct{}{}
	comms := map[string]struct{}{}
	for _, event := range events {
		if event.PID != 0 {
			pids[event.PID] = struct{}{}
		}
		if event.TraceID != "" {
			traces[event.TraceID] = struct{}{}
		}
		if event.Comm != "" {
			comms[strings.ToLower(event.Comm)] = struct{}{}
		}
	}
	out := make([]loopDetectionFinding, 0)
	for _, finding := range findings {
		matched := false
		if finding.PID != 0 {
			_, matched = pids[finding.PID]
		}
		if !matched && finding.TraceID != "" {
			_, matched = traces[finding.TraceID]
		}
		if !matched && finding.Comm != "" {
			_, matched = comms[strings.ToLower(finding.Comm)]
		}
		if matched {
			out = append(out, finding)
		}
	}
	return out
}

func currentResearchKernelRiskFeedbackInfo() ResearchKernelRiskFeedbackInfo {
	settings := runtimeSettingsStore.Snapshot()
	normalizeKernelRiskFeedbackSettings(&settings.KernelRiskFeedback)
	return ResearchKernelRiskFeedbackInfo{Enabled: settings.KernelRiskFeedback.Enabled, PolicyGateEnabled: settings.PolicyManagementEnabled, MinRiskScore: settings.KernelRiskFeedback.MinRiskScore, EnforceNetwork: settings.KernelRiskFeedback.EnforceNetwork, EnforceFileNames: settings.KernelRiskFeedback.EnforceFileNames, EnforceExec: settings.KernelRiskFeedback.EnforceExec, MaxActionsPerMinute: settings.KernelRiskFeedback.MaxActionsPerMinute}
}

func handleResearchSessionsList(c *gin.Context) {
	sessions, err := researchSessionsStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func handleResearchSessionsCreate(c *gin.Context) {
	var req researchCreateSessionRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid research session payload"})
			return
		}
	}
	session, err := researchSessionsStore.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, session)
}

func handleResearchSessionGet(c *gin.Context) {
	session, err := researchSessionsStore.Get(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func handleResearchSessionDelete(c *gin.Context) {
	if err := researchSessionsStore.Delete(c.Param("id")); err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func handleResearchSessionTask(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req researchTaskRequest
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid research task payload"})
				return
			}
		}
		task, err := researchTaskStore.Submit(c.Param("id"), req, tlsStore)
		if err != nil {
			if errors.Is(err, errResearchQueueFull) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleResearchTaskGet(c *gin.Context) {
	task, ok := researchTaskStore.Get(c.Param("taskId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "research task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func handleResearchTasksStatus(c *gin.Context) {
	c.JSON(http.StatusOK, researchTaskStore.Status())
}

func handleResearchTaskCancel(c *gin.Context) {
	task := researchTaskStore.Cancel(c.Param("taskId"))
	status := http.StatusAccepted
	if task.Error == "task not found" {
		status = http.StatusNotFound
	}
	c.JSON(status, task)
}

func handleResearchSessionEvents(c *gin.Context) {
	events, err := researchSessionsStore.LoadEvents(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	filter := researchSourceFilterFromQuery(c)
	timerange := researchTimeRangeFromQuery(c)
	filtered := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, filter, timerange) {
			filtered = append(filtered, event)
		}
	}
	offset := parseResearchIntQuery(c, "offset", 0, 0, len(filtered))
	limit := parseResearchIntQuery(c, "limit", researchDefaultPageLimit, 1, researchMaxPageLimit)
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := []ResearchEvent{}
	if offset < len(filtered) {
		page = filtered[offset:end]
	}
	c.JSON(http.StatusOK, gin.H{"events": page, "total": len(filtered), "offset": offset, "limit": limit})
}

func handleResearchSessionResults(c *gin.Context) {
	results, err := researchSessionsStore.LoadResults(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, results)
}

func handleResearchSessionExport(c *gin.Context) {
	format := normalizeResearchFormat(c.Query("format"))
	if format == "" {
		format = "bundle"
	}
	if format != "jsonl" && format != "csv" && format != "bundle" && format != "json" && format != "security_json" && format != "security_jsonl" && format != "security_csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported export format", "supported": []string{"jsonl", "csv", "json", "bundle", "security-json", "security-jsonl", "security-csv"}})
		return
	}
	ref, payload, err := researchSessionsStore.ExportArtifact(c.Param("id"), format)
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", ref.Name))
	c.Header("X-Research-Artifact-SHA256", ref.SHA256)
	c.Data(http.StatusOK, ref.ContentType, payload)
}

func registerResearchRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	researchTaskStore.Start(runtimeSettingsStore.Snapshot().ResearchProcessing.QueueSize)
	router.GET("/sessions", handleResearchSessionsList)
	router.POST("/sessions", handleResearchSessionsCreate)
	router.GET("/sessions/:id", handleResearchSessionGet)
	router.DELETE("/sessions/:id", handleResearchSessionDelete)
	router.POST("/sessions/:id/tasks", handleResearchSessionTask(tlsStore))
	router.GET("/tasks/status", handleResearchTasksStatus)
	router.GET("/tasks/:taskId", handleResearchTaskGet)
	router.POST("/tasks/:taskId/cancel", handleResearchTaskCancel)
	router.GET("/sessions/:id/events", handleResearchSessionEvents)
	router.GET("/sessions/:id/results", handleResearchSessionResults)
	router.GET("/sessions/:id/training", handleResearchSessionTraining)
	router.POST("/sessions/:id/training/import", handleResearchSessionTrainingImport)
	router.GET("/sessions/:id/export", handleResearchSessionExport)
}

func researchWriteStoreError(c *gin.Context, err error) {
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func normalizeResearchAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "scan", "scan_recent":
		return "scan_recent"
	case "build", "build_session":
		return "build_session"
	case "compare", "compare_windows":
		return "compare_windows"
	case "export", "export_bundle":
		return "export_bundle"
	case "reset", "reset_session":
		return "reset_session"
	case "security", "security_eval", "security_evaluation":
		return "security_eval"
	case "cancel":
		return "cancel"
	default:
		return strings.ToLower(strings.TrimSpace(action))
	}
}

func researchFormatsFromRequest(req researchTaskRequest) []string {
	formats := append([]string(nil), req.Formats...)
	if strings.TrimSpace(req.Format) != "" {
		formats = append(formats, req.Format)
	}
	return normalizeResearchFormats(formats)
}

func normalizeResearchExportFormats(raw string) string {
	formats := splitResearchFormats(raw)
	if len(formats) == 0 {
		formats = splitResearchFormats(researchProcessingDefaultExportFormats)
	}
	return strings.Join(formats, ",")
}

func splitResearchFormats(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeResearchFormats(strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' }))
}

func normalizeResearchFormats(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		format := normalizeResearchFormat(value)
		if format == "" {
			continue
		}
		if _, ok := seen[format]; ok {
			continue
		}
		seen[format] = struct{}{}
		out = append(out, format)
	}
	return out
}

func normalizeResearchFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ndjson", "jsonl":
		return "jsonl"
	case "csv":
		return "csv"
	case "zip", "bundle":
		return "bundle"
	case "json":
		return "json"
	case "security_json", "security-json", "security", "security_eval", "security-eval", "security_evaluation", "security-evaluation":
		return "security_json"
	case "security_jsonl", "security-jsonl", "security_ndjson", "security-ndjson":
		return "security_jsonl"
	case "security_csv", "security-csv":
		return "security_csv"
	default:
		return ""
	}
}

func normalizeResearchSession(session *ResearchSession) {
	if session == nil {
		return
	}
	session.ID = sanitizeResearchIDPart(session.ID)
	if strings.TrimSpace(session.ID) == "" || session.ID == "unknown" {
		session.ID = researchGenerateID("rs")
	}
	session.Name = strings.TrimSpace(session.Name)
	if session.Name == "" {
		session.Name = session.ID
	}
	session.Tags = normalizeResearchTags(session.Tags)
	session.SourceFilter = normalizeResearchSourceFilter(session.SourceFilter)
	session.TimeRange = normalizeResearchTimeRange(session.TimeRange)
	if session.Status == "" {
		session.Status = researchSessionEmpty
	}
	if session.Summary.SchemaVersion == "" {
		session.Summary.SchemaVersion = researchSchemaVersion
	}
	if session.ArtifactRefs == nil {
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
}

func normalizeResearchTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func normalizeResearchSourceFilter(filter ResearchSourceFilter) ResearchSourceFilter {
	filter.Sources = normalizeResearchTerms(filter.Sources)
	filter.EventTypes = normalizeResearchTerms(filter.EventTypes)
	filter.Comms = normalizeResearchTerms(filter.Comms)
	filter.TraceID = strings.TrimSpace(filter.TraceID)
	filter.SpanID = strings.TrimSpace(filter.SpanID)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	if filter.Limit > researchMaxTaskLimit {
		filter.Limit = researchMaxTaskLimit
	}
	if len(filter.PIDs) > 0 {
		seen := map[uint32]struct{}{}
		out := make([]uint32, 0, len(filter.PIDs))
		for _, pid := range filter.PIDs {
			if pid == 0 {
				continue
			}
			if _, ok := seen[pid]; ok {
				continue
			}
			seen[pid] = struct{}{}
			out = append(out, pid)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		filter.PIDs = out
	}
	return filter
}

func normalizeResearchTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeResearchTimeRange(timerange ResearchTimeRange) ResearchTimeRange {
	if timerange.Since <= 0 && strings.TrimSpace(timerange.SinceTime) != "" {
		if parsed := parseRecentEventTime(timerange.SinceTime); !parsed.IsZero() {
			timerange.Since = parsed.UnixMilli()
		}
	}
	if timerange.Until <= 0 && strings.TrimSpace(timerange.UntilTime) != "" {
		if parsed := parseRecentEventTime(timerange.UntilTime); !parsed.IsZero() {
			timerange.Until = parsed.UnixMilli()
		}
	}
	if timerange.Since > 0 {
		timerange.SinceTime = time.UnixMilli(timerange.Since).UTC().Format(time.RFC3339Nano)
	}
	if timerange.Until > 0 {
		timerange.UntilTime = time.UnixMilli(timerange.Until).UTC().Format(time.RFC3339Nano)
	}
	return timerange
}

func mergeResearchSourceFilter(base, override ResearchSourceFilter) ResearchSourceFilter {
	if len(override.Sources) > 0 {
		base.Sources = override.Sources
	}
	if len(override.EventTypes) > 0 {
		base.EventTypes = override.EventTypes
	}
	if len(override.Comms) > 0 {
		base.Comms = override.Comms
	}
	if len(override.PIDs) > 0 {
		base.PIDs = override.PIDs
	}
	if strings.TrimSpace(override.TraceID) != "" {
		base.TraceID = override.TraceID
	}
	if strings.TrimSpace(override.SpanID) != "" {
		base.SpanID = override.SpanID
	}
	if strings.TrimSpace(override.Query) != "" {
		base.Query = override.Query
	}
	if override.Limit > 0 {
		base.Limit = override.Limit
	}
	if override.IncludeTLS != nil {
		base.IncludeTLS = override.IncludeTLS
	}
	if override.IncludeUploaded != nil {
		base.IncludeUploaded = override.IncludeUploaded
	}
	return normalizeResearchSourceFilter(base)
}

func mergeResearchTimeRange(base, override ResearchTimeRange) ResearchTimeRange {
	if override.Since > 0 || strings.TrimSpace(override.SinceTime) != "" {
		base.Since = override.Since
		base.SinceTime = override.SinceTime
	}
	if override.Until > 0 || strings.TrimSpace(override.UntilTime) != "" {
		base.Until = override.Until
		base.UntilTime = override.UntilTime
	}
	return normalizeResearchTimeRange(base)
}

func researchIncludeTLS(filter ResearchSourceFilter) bool {
	if filter.IncludeTLS != nil {
		return *filter.IncludeTLS
	}
	return true
}

func researchIncludeUploaded(filter ResearchSourceFilter) bool {
	if filter.IncludeUploaded != nil {
		return *filter.IncludeUploaded
	}
	return true
}

func researchEventMatches(event ResearchEvent, filter ResearchSourceFilter, timerange ResearchTimeRange) bool {
	filter = normalizeResearchSourceFilter(filter)
	timerange = normalizeResearchTimeRange(timerange)
	if len(filter.Sources) > 0 && !researchStringInList(event.Source, filter.Sources) {
		return false
	}
	if len(filter.EventTypes) > 0 && !researchStringInList(event.EventType, filter.EventTypes) {
		return false
	}
	if len(filter.Comms) > 0 && !researchStringInList(event.Comm, filter.Comms) {
		return false
	}
	if len(filter.PIDs) > 0 && !researchUint32InList(event.PID, filter.PIDs) {
		return false
	}
	if filter.TraceID != "" && event.TraceID != filter.TraceID {
		return false
	}
	if filter.SpanID != "" && event.SpanID != filter.SpanID {
		return false
	}
	if timerange.Since > 0 && event.Timestamp < timerange.Since {
		return false
	}
	if timerange.Until > 0 && event.Timestamp > timerange.Until {
		return false
	}
	if filter.Query != "" {
		haystack, _ := json.Marshal(event)
		if !strings.Contains(strings.ToLower(string(haystack)), strings.ToLower(filter.Query)) {
			return false
		}
	}
	return true
}

func filterResearchEventsByRange(events []ResearchEvent, timerange ResearchTimeRange) []ResearchEvent {
	out := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, ResearchSourceFilter{}, timerange) {
			out = append(out, event)
		}
	}
	return out
}

func researchSourceFilterFromQuery(c *gin.Context) ResearchSourceFilter {
	filter := ResearchSourceFilter{
		Sources:    splitResearchQueryList(platform.FirstNonEmpty(c.Query("source"), c.Query("sources"))),
		EventTypes: splitResearchQueryList(platform.FirstNonEmpty(c.Query("eventType"), c.Query("event_type"), c.Query("type"))),
		Comms:      splitResearchQueryList(platform.FirstNonEmpty(c.Query("comm"), c.Query("comms"))),
		TraceID:    strings.TrimSpace(platform.FirstNonEmpty(c.Query("traceId"), c.Query("trace_id"))),
		SpanID:     strings.TrimSpace(platform.FirstNonEmpty(c.Query("spanId"), c.Query("span_id"))),
		Query:      strings.TrimSpace(platform.FirstNonEmpty(c.Query("q"), c.Query("query"), c.Query("search"))),
	}
	if raw := strings.TrimSpace(c.Query("pid")); raw != "" {
		filter.PIDs = parseResearchPIDs(raw)
	}
	return normalizeResearchSourceFilter(filter)
}

func researchTimeRangeFromQuery(c *gin.Context) ResearchTimeRange {
	return normalizeResearchTimeRange(ResearchTimeRange{SinceTime: c.Query("since"), UntilTime: c.Query("until")})
}

func splitResearchQueryList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\t' })
}

func parseResearchPIDs(raw string) []uint32 {
	parts := splitResearchQueryList(raw)
	out := make([]uint32, 0, len(parts))
	for _, part := range parts {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil && parsed > 0 {
			out = append(out, uint32(parsed))
		}
	}
	return out
}

func parseResearchIntQuery(c *gin.Context, key string, fallback, min, max int) int {
	value := fallback
	if raw := strings.TrimSpace(c.Query(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			value = parsed
		}
	}
	if value < min {
		value = min
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func researchStringInList(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func researchUint32InList(value uint32, candidates []uint32) bool {
	for _, candidate := range candidates {
		if candidate == value {
			return true
		}
	}
	return false
}

func researchEventsJSONLBytes(events []ResearchEvent) []byte {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		_ = encoder.Encode(event)
	}
	return buf.Bytes()
}

func researchEventsCSVBytes(events []ResearchEvent) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	headers := []string{"id", "timestamp", "time", "source", "event_type", "pid", "ppid", "comm", "trace_id", "span_id", "target", "risk_score", "decision", "redaction_level"}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, event := range events {
		row := []string{event.ID, strconv.FormatInt(event.Timestamp, 10), event.Time, event.Source, event.EventType, researchUint32String(event.PID), researchUint32String(event.PPID), event.Comm, event.TraceID, event.SpanID, event.Target, researchFloatString(event.RiskScore), event.Decision, event.RedactionLevel}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func researchBundleZipBytes(session ResearchSession, events []ResearchEvent, results ResearchResults, settings ResearchProcessingSettings) ([]byte, error) {
	var buf bytes.Buffer
	zipw := zip.NewWriter(&buf)
	jsonl := researchEventsJSONLBytes(events)
	csvBytes, err := researchEventsCSVBytes(events)
	if err != nil {
		return nil, err
	}
	training := buildResearchTrainingDataset(session.ID, events, researchTrainingPolicyHeuristic, true)
	trainingJSONL := researchTrainingDatasetJSONLBytes(training)
	trainingCSV, err := researchTrainingDatasetCSVBytes(training)
	if err != nil {
		return nil, err
	}
	trainingManifestJSON, err := json.MarshalIndent(struct {
		SchemaVersion   string                     `json:"schemaVersion"`
		LabelPolicy     string                     `json:"labelPolicy"`
		FeatureSpace    string                     `json:"featureSpace"`
		FeatureVersion  string                     `json:"featureVersion"`
		FeatureDim      int                        `json:"featureDim"`
		FeatureNames    []string                   `json:"featureNames"`
		RedactionLevels []researchCount            `json:"redactionLevels"`
		SampleCount     int                        `json:"sampleCount"`
		LabeledCount    int                        `json:"labeledCount"`
		ByLabel         []researchCount            `json:"byLabel"`
		ByCategory      []researchCount            `json:"byCategory"`
		BySource        []researchCount            `json:"bySource"`
		Normalization   FeatureNormalizationReport `json:"normalization"`
		Quality         DatasetQualitySummary      `json:"quality"`
	}{
		SchemaVersion:   training.SchemaVersion,
		LabelPolicy:     training.LabelPolicy,
		FeatureSpace:    "agent-command-128-bounded-0-1",
		FeatureVersion:  "feature-extractor.v1",
		FeatureDim:      training.FeatureDim,
		FeatureNames:    training.FeatureNames,
		RedactionLevels: researchRedactionLevelCounts(events),
		SampleCount:     training.SampleCount,
		LabeledCount:    training.LabeledCount,
		ByLabel:         training.ByLabel,
		ByCategory:      training.ByCategory,
		BySource:        training.BySource,
		Normalization:   training.Normalization,
		Quality:         training.Quality,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	resultsJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return nil, err
	}
	sessionJSON, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return nil, err
	}
	payloads := map[string][]byte{"events.jsonl": jsonl, "events.csv": csvBytes, "training.jsonl": trainingJSONL, "training.csv": trainingCSV, "training-manifest.json": trainingManifestJSON, "results.json": resultsJSON, "session.json": sessionJSON}
	if results.SecurityEvaluation != nil {
		securityJSON, err := researchSecurityEvaluationJSONBytes(results.SecurityEvaluation)
		if err != nil {
			return nil, err
		}
		securityJSONL := researchSecurityEvaluationJSONLBytes(results.SecurityEvaluation)
		securityCSV, err := researchSecurityEvaluationCSVBytes(results.SecurityEvaluation)
		if err != nil {
			return nil, err
		}
		payloads["security-evaluation.json"] = securityJSON
		payloads["security-evaluation.jsonl"] = securityJSONL
		payloads["security-evaluation.csv"] = securityCSV
	}
	manifest := researchBuildManifest(session, events, settings, payloads, nil)
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files := payloads
	files["manifest.json"] = manifestJSON
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zipw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := zipw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func researchBuildManifest(session ResearchSession, events []ResearchEvent, settings ResearchProcessingSettings, payloads map[string][]byte, artifacts map[string]ResearchArtifactRef) ResearchManifest {
	hashes := map[string]string{}
	for name, payload := range payloads {
		hashes[name] = researchSHA256Hex(payload)
	}
	return ResearchManifest{SchemaVersion: researchManifestVersion, GeneratedAt: time.Now().UTC(), SessionID: session.ID, SessionName: session.Name, SourceFilter: session.SourceFilter, TimeRange: session.TimeRange, RedactionLevel: researchSessionRedactionLevel(events), EventCount: len(events), Artifacts: artifacts, Hashes: hashes, ResearchSchema: researchSchemaVersion, ExportedBy: "agent-ebpf-filter", RetentionDays: settings.ArtifactRetentionDays, MaxSessionEvents: settings.MaxSessionEvents, ConfiguredFormats: splitResearchFormats(settings.ExportFormats)}
}

func researchSessionRedactionLevel(events []ResearchEvent) string {
	if len(events) == 0 {
		return "metadata_only"
	}
	levels := map[string]struct{}{}
	for _, event := range events {
		if event.RedactionLevel != "" {
			levels[event.RedactionLevel] = struct{}{}
		}
	}
	if len(levels) == 0 {
		return "metadata_only"
	}
	items := make([]string, 0, len(levels))
	for level := range levels {
		items = append(items, level)
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func researchSHA256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func researchUint32String(value uint32) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(value), 10)
}

func researchFloatString(value float64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func researchRiskAssociation(event ResearchEvent) string {
	if event.TraceID != "" {
		return "trace:" + event.TraceID
	}
	if event.PID != 0 {
		return fmt.Sprintf("pid:%d", event.PID)
	}
	if event.Comm != "" {
		return "comm:" + event.Comm
	}
	return ""
}

func researchSafeFeatureMap(values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" || researchZeroValue(value) {
			continue
		}
		if researchSensitiveFeatureKey(trimmed) {
			out[trimmed] = "[redacted]"
			continue
		}
		out[trimmed] = researchSafeFeatureValue(value)
	}
	return out
}

func researchSafeFeatureValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return researchSafeFeatureMap(typed)
	case map[string]string:
		m := make(map[string]any, len(typed))
		for key, value := range typed {
			m[key] = value
		}
		return researchSafeFeatureMap(m)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, researchSafeFeatureValue(item))
		}
		return out
	default:
		return typed
	}
}

func researchSensitiveFeatureKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	sensitive := []string{"authorization", "cookie", "token", "secret", "password", "credential", "api_key", "apikey", "x-api-key", "body", "raw", "rawhexdump", "raw_hex_dump", "payload", "headers", "cmdline", "argv", "args"}
	for _, marker := range sensitive {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func researchZeroValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case uint32:
		return typed == 0
	case uint64:
		return typed == 0
	case float64:
		return typed == 0
	case bool:
		return !typed
	case []string:
		return len(typed) == 0
	}
	return false
}

func cloneResearchSession(session *ResearchSession) ResearchSession {
	if session == nil {
		return ResearchSession{}
	}
	out := *session
	out.Tags = append([]string(nil), session.Tags...)
	out.ArtifactRefs = cloneArtifactRefs(session.ArtifactRefs)
	return out
}

func cloneArtifactRefs(in map[string]ResearchArtifactRef) map[string]ResearchArtifactRef {
	out := map[string]ResearchArtifactRef{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func researchStringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func researchFloatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func researchGenerateID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}

func researchStableID(prefix string, parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		payload, err := json.Marshal(part)
		if err != nil {
			payload = []byte(fmt.Sprint(part))
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))[:20]
}

func ptrTime(t time.Time) *time.Time { return &t }
