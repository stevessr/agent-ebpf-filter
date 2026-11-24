package research

import (
	"time"
)

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
