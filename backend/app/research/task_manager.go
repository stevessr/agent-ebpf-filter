package research

import (
	"agent-ebpf-filter/app/platform"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
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
	m.ensureRuntime().Start(queueSize)
}

func (m *researchTaskManager) Shutdown(ctx context.Context) error {
	m.mu.RLock()
	runtime := m.runtime
	m.mu.RUnlock()
	if runtime == nil {
		return nil
	}
	return runtime.Shutdown(ctx)
}

func (m *researchTaskManager) ensureRuntime() *backendTaskRuntime {
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
	m.mu.RLock()
	entry := m.tasks[strings.TrimSpace(taskID)]
	m.mu.RUnlock()
	if entry == nil {
		return ResearchTask{}, false
	}
	return entry.snapshot(), true
}

func (m *researchTaskManager) Status() researchTaskManagerStatus {
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
	m.mu.Lock()
	m.pruneLocked()
	m.mu.Unlock()
}

func (m *researchTaskManager) pruneLocked() {
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
	if entry == nil {
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
	if entry == nil {
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

// StartTaskWorker launches the research task manager worker loop.
func StartTaskWorker() { startResearchTaskWorker() }

// TaskStoreShutdown gracefully stops the shared research task manager.
func TaskStoreShutdown(ctx context.Context) error {
	return researchTaskStore.Shutdown(ctx)
}
