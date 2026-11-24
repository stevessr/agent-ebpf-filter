package signalruntime

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/internal/workerqueue"
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

func newSignalProcessingWorker() *signalProcessingWorker {
	return &signalProcessingWorker{
		states:    make(map[string]*signalState),
		updatedAt: time.Now().UTC(),
	}
}

func startSignalProcessingWorker(ctx context.Context) {
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	signalProcessingWorkerStore.Start(ctx, settings.QueueSize)
}

func (w *signalProcessingWorker) Start(ctx context.Context, queueSize int) {
	if ctx == nil {
		ctx = context.Background()
	}
	queueSize = tasks.NormalizeQueueSize(queueSize, signalDefaultQueueSize)
	if w.queue.Start(ctx, queueSize, func(ctx context.Context, items <-chan signalProcessingWorkItem) {
		// The event consumer and the cron sweeper share one generation so a
		// shutdown waits for both.
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			w.run(ctx, items)
		}()
		go func() {
			defer workers.Done()
			w.runCron(ctx)
		}()
		workers.Wait()
		w.touch()
	}) {
		w.touch()
	}
}

func (w *signalProcessingWorker) touch() {
	w.mu.Lock()
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) run(ctx context.Context, queue <-chan signalProcessingWorkItem) {
	for {
		var item signalProcessingWorkItem
		select {
		case <-ctx.Done():
			return
		case item = <-queue:
		}
		if ctx.Err() != nil {
			return
		}
		switch item.kind {
		case signalProcessingWorkReset:
			w.resetNow()
		case signalProcessingWorkScan:
			w.processScan(ctx, item.records, item.force)
		case signalProcessingWorkExpire:
			w.expireNow(time.Now().UTC())
		case signalProcessingWorkEvent:
			w.processRecord(item.record, item.force)
		}
	}
}

func (w *signalProcessingWorker) runCron(ctx context.Context) {
	for {
		settings := SnapshotSettingsHook().SignalProcessing
		NormalizeSettings(&settings)
		interval := time.Duration(settings.CronIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = signalDefaultCronIntervalSeconds * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		if !w.EnqueueExpire() {
			w.expireNow(time.Now().UTC())
		}
	}
}

func (w *signalProcessingWorker) Shutdown(ctx context.Context) error {
	if w.queue.Stats().Started {
		w.touch()
	}
	return w.queue.Shutdown(ctx)
}

func queueSignalProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
		return
	}
	if !SignalSettingsHook().Enabled {
		return
	}
	signalProcessingWorkerStore.EnqueueEvent(record)
}

func (w *signalProcessingWorker) EnqueueEvent(record CapturedEventRecord) bool {
	return w.enqueue(signalProcessingWorkItem{kind: signalProcessingWorkEvent, record: record})
}

func (w *signalProcessingWorker) EnqueueScan(records []CapturedEventRecord) bool {
	return w.enqueue(signalProcessingWorkItem{kind: signalProcessingWorkScan, records: records, force: true})
}

func (w *signalProcessingWorker) EnqueueReset() bool {
	return w.enqueue(signalProcessingWorkItem{kind: signalProcessingWorkReset})
}

func (w *signalProcessingWorker) EnqueueExpire() bool {
	return w.enqueue(signalProcessingWorkItem{kind: signalProcessingWorkExpire})
}

func (w *signalProcessingWorker) enqueue(item signalProcessingWorkItem) bool {
	switch w.queue.TryEnqueue(item) {
	case workerqueue.Accepted:
		return true
	case workerqueue.NotStarted:
		w.noteDrop("signal processing worker is not started")
	default:
		w.noteDrop("signal processing queue is full")
	}
	return false
}

func (w *signalProcessingWorker) resetNow() {
	w.mu.Lock()
	w.states = make(map[string]*signalState)
	w.stateLRUHead = nil
	w.stateLRUTail = nil
	w.lastError = ""
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) noteDrop(message string) {
	w.mu.Lock()
	w.droppedTotal++
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) noteError(message string) {
	w.mu.Lock()
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) expireNow(now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	w.mu.Lock()
	w.expiryRunsTotal++
	w.evictExpiredLocked(now)
	w.updatedAt = now
	w.mu.Unlock()
}

func (w *signalProcessingWorker) processRecord(record CapturedEventRecord, force bool) {
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	w.processRecordWithSettings(record, force, settings)
}

func (w *signalProcessingWorker) processScan(ctx context.Context, records []CapturedEventRecord, force bool) {
	if len(records) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	if !force && !settings.Enabled {
		return
	}
	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		w.processRecordWithSettings(record, force, settings)
	}
}

func (w *signalProcessingWorker) processRecordWithSettings(record CapturedEventRecord, force bool, settings SignalProcessingSettings) {
	if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
		return
	}
	if !force && !settings.Enabled {
		return
	}
	if len(settings.Rules) == 0 {
		return
	}
	record = events.NormalizeCapturedEventRecord(record)
	event := record.Event
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	updates := make([]*signalState, 0, 4)
	for _, rule := range settings.Rules {
		if !rule.Enabled || !signalRuleMatches(rule, event) {
			continue
		}
		stateKey, target := signalStateKey(rule, event)
		if stateKey == "" {
			continue
		}
		updates = append(updates, &signalState{
			ID:            signalStateID(stateKey),
			RuleID:        rule.ID,
			RuleName:      rule.Name,
			Kind:          rule.Kind,
			Key:           stateKey,
			Target:        target,
			PID:           event.GetPid(),
			TGID:          event.GetTgid(),
			Comm:          strings.TrimSpace(event.GetComm()),
			TTLSeconds:    rule.TTLSeconds,
			LastMatchedAt: observedAt,
			UpdatedAt:     observedAt,
			ExpiresAt:     observedAt.Add(time.Duration(rule.TTLSeconds) * time.Second),
			LastEventType: signalEventType(event),
			LastPath:      strings.TrimSpace(event.GetPath()),
			LastExtraPath: strings.TrimSpace(event.GetExtraPath()),
			LastEventID:   recordEnvelopeID(record),
			Score:         rule.Weight,
			Count:         1,
		})
	}
	if len(updates) == 0 {
		w.mu.Lock()
		w.consumedTotal++
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	w.consumedTotal++
	for _, update := range updates {
		existing := w.states[update.Key]
		if existing == nil {
			update.FirstSeen = observedAt
			w.states[update.Key] = update
			w.appendSignalStateLocked(update)
		} else {
			decayed := decaySignalScore(existing.Score, existing.LastMatchedAt, observedAt, update.TTLSeconds)
			existing.RuleID = update.RuleID
			existing.RuleName = update.RuleName
			existing.Kind = update.Kind
			existing.Target = update.Target
			existing.PID = update.PID
			existing.TGID = update.TGID
			existing.Comm = update.Comm
			existing.Count++
			existing.Score = decayed + update.Score
			existing.TTLSeconds = update.TTLSeconds
			existing.LastMatchedAt = observedAt
			existing.UpdatedAt = observedAt
			existing.ExpiresAt = update.ExpiresAt
			existing.LastEventType = update.LastEventType
			existing.LastPath = update.LastPath
			existing.LastExtraPath = update.LastExtraPath
			existing.LastEventID = update.LastEventID
			w.touchSignalStateLocked(existing)
		}
		w.updatedTotal++
	}
	w.enforceMaxStatesLocked(settings.MaxStates, observedAt)
	w.updatedAt = observedAt
	w.mu.Unlock()
}

func (w *signalProcessingWorker) Status() signalProcessingStatus {
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	now := time.Now().UTC()
	queueStats := w.queue.Stats()
	queueLen, queueCap := queueStats.Len, queueStats.Cap
	w.mu.RLock()
	states := make([]signalState, 0, len(w.states))
	for _, state := range w.states {
		if state == nil {
			continue
		}
		if !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now) {
			continue
		}
		cloned := *state
		cloned.lruPrev = nil
		cloned.lruNext = nil
		states = append(states, cloned)
	}
	consumedTotal := w.consumedTotal
	updatedTotal := w.updatedTotal
	droppedTotal := w.droppedTotal
	expiredTotal := w.expiredTotal
	capacityEvictedTotal := w.capacityEvictedTotal
	expiryRunsTotal := w.expiryRunsTotal
	lastError := w.lastError
	updatedAt := w.updatedAt
	w.mu.RUnlock()

	for index := range states {
		states[index].Score = decaySignalScore(states[index].Score, states[index].LastMatchedAt, now, states[index].TTLSeconds)
	}
	activeStates := len(states)
	sort.Slice(states, func(i, j int) bool { return states[i].UpdatedAt.After(states[j].UpdatedAt) })
	if len(states) > signalRecentStateLimit {
		states = states[:signalRecentStateLimit]
	}
	status := signalProcessingStatus{
		Enabled:              settings.Enabled,
		Settings:             settings,
		QueueLen:             queueLen,
		QueueCap:             queueCap,
		ConsumedTotal:        consumedTotal,
		UpdatedTotal:         updatedTotal,
		DroppedTotal:         droppedTotal,
		ExpiredTotal:         expiredTotal,
		CapacityEvictedTotal: capacityEvictedTotal,
		ExpiryRunsTotal:      expiryRunsTotal,
		ActiveStates:         activeStates,
		RecentStates:         states,
		AvailableKinds:       availableSignalKinds(),
		LastError:            lastError,
		UpdatedAt:            updatedAt,
	}
	return status
}

// StartProcessingWorker launches the shared signal processing worker.
func StartProcessingWorker(ctx context.Context) { startSignalProcessingWorker(ctx) }

// QueueProcessingRecord enqueues a captured event into the signal worker.
func QueueProcessingRecord(record CapturedEventRecord) { queueSignalProcessingRecord(record) }

// Worker returns the shared signal processing worker.
func Worker() *signalProcessingWorker { return signalProcessingWorkerStore }
