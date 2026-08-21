package signalruntime

import (
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/app/events"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	signalKindPathAccess   = "path_access"
	signalKindChildProcess = "child_process"
	signalKindRepeatedRead = "repeated_read"
	signalKindCustom       = "custom"

	signalDefaultQueueSize           = 2048
	signalDefaultCronIntervalSeconds = 30
	signalDefaultTTLSeconds          = 300
	signalDefaultMaxStates           = 4096
	signalRecentStateLimit           = 50
	signalProtoLogCompressionGzip    = "gzip"
	signalMaxSelectedPrograms        = 128
)

type signalKindInfo struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type signalState struct {
	ID            string    `json:"id"`
	RuleID        string    `json:"ruleId"`
	RuleName      string    `json:"ruleName"`
	Kind          string    `json:"kind"`
	Key           string    `json:"key"`
	Target        string    `json:"target"`
	PID           uint32    `json:"pid,omitempty"`
	TGID          uint32    `json:"tgid,omitempty"`
	Comm          string    `json:"comm,omitempty"`
	Count         int       `json:"count"`
	Score         float64   `json:"score"`
	TTLSeconds    int       `json:"ttlSeconds"`
	FirstSeen     time.Time `json:"firstSeen"`
	LastMatchedAt time.Time `json:"lastMatchedAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	LastEventType string    `json:"lastEventType,omitempty"`
	LastPath      string    `json:"lastPath,omitempty"`
	LastExtraPath string    `json:"lastExtraPath,omitempty"`
	LastEventID   string    `json:"lastEventId,omitempty"`

	lruPrev *signalState
	lruNext *signalState
}

type signalProcessingStatus struct {
	Enabled              bool                     `json:"enabled"`
	Settings             SignalProcessingSettings `json:"settings"`
	QueueLen             int                      `json:"queueLen"`
	QueueCap             int                      `json:"queueCap"`
	ConsumedTotal        uint64                   `json:"consumedTotal"`
	UpdatedTotal         uint64                   `json:"updatedTotal"`
	DroppedTotal         uint64                   `json:"droppedTotal"`
	ExpiredTotal         uint64                   `json:"expiredTotal"`
	CapacityEvictedTotal uint64                   `json:"capacityEvictedTotal"`
	ExpiryRunsTotal      uint64                   `json:"expiryRunsTotal"`
	ActiveStates         int                      `json:"activeStates"`
	RecentStates         []signalState            `json:"recentStates"`
	AvailableKinds       []signalKindInfo         `json:"availableKinds"`
	LastError            string                   `json:"lastError,omitempty"`
	UpdatedAt            time.Time                `json:"updatedAt"`
}

type signalProcessingTaskRequest struct {
	Action string `json:"action"`
	Limit  int    `json:"limit"`
}

type signalProcessingTaskResponse struct {
	Status   string `json:"status"`
	Action   string `json:"action"`
	Records  int    `json:"records,omitempty"`
	QueueLen int    `json:"queueLen"`
}

type signalRuleTestRequest struct {
	Rule  *SignalRule `json:"rule"`
	Limit int         `json:"limit"`
}

type signalRuleTestMatch struct {
	Timestamp  time.Time `json:"timestamp"`
	PID        uint32    `json:"pid,omitempty"`
	TGID       uint32    `json:"tgid,omitempty"`
	Comm       string    `json:"comm,omitempty"`
	EventType  string    `json:"eventType,omitempty"`
	Target     string    `json:"target,omitempty"`
	Path       string    `json:"path,omitempty"`
	ExtraPath  string    `json:"extraPath,omitempty"`
	ExtraInfo  string    `json:"extraInfo,omitempty"`
	EventID    string    `json:"eventId,omitempty"`
	SignalKey  string    `json:"signalKey,omitempty"`
	WouldScore float64   `json:"wouldScore"`
}

type signalRuleTestResponse struct {
	Rule         SignalRule            `json:"rule"`
	ScannedTotal int                   `json:"scannedTotal"`
	MatchedTotal int                   `json:"matchedTotal"`
	Matches      []signalRuleTestMatch `json:"matches"`
}

type signalProgramLogStatus struct {
	Program    string `json:"program"`
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	SizeBytes  int64  `json:"sizeBytes"`
	FrameCount int    `json:"frameCount"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
	Error      string `json:"error,omitempty"`
}

type signalProgramLogsResponse struct {
	Compression string                       `json:"compression"`
	Logs        []signalProgramLogStatus     `json:"logs"`
	Writer      signalProgramLogWriterStatus `json:"writer"`
}

type signalProcessingWorkKind string

const (
	signalProcessingWorkEvent  signalProcessingWorkKind = "event"
	signalProcessingWorkScan   signalProcessingWorkKind = "scan"
	signalProcessingWorkReset  signalProcessingWorkKind = "reset"
	signalProcessingWorkExpire signalProcessingWorkKind = "expire"
)

type signalProcessingWorkItem struct {
	kind    signalProcessingWorkKind
	record  CapturedEventRecord
	records []CapturedEventRecord
	force   bool
}

type signalProcessingWorker struct {
	lifecycleMu          sync.Mutex
	mu                   sync.RWMutex
	queue                chan signalProcessingWorkItem
	cancel               context.CancelFunc
	done                 chan struct{}
	started              bool
	states               map[string]*signalState
	stateLRUHead         *signalState
	stateLRUTail         *signalState
	consumedTotal        uint64
	updatedTotal         uint64
	droppedTotal         uint64
	expiredTotal         uint64
	capacityEvictedTotal uint64
	expiryRunsTotal      uint64
	lastError            string
	updatedAt            time.Time
}

var (
	signalProcessingWorkerStore = newSignalProcessingWorker()
	signalProgramLogMu          sync.Mutex
)

func DefaultSettings() SignalProcessingSettings {
	return SignalProcessingSettings{
		Enabled:             false,
		QueueSize:           signalDefaultQueueSize,
		CronIntervalSeconds: signalDefaultCronIntervalSeconds,
		DefaultTTLSeconds:   signalDefaultTTLSeconds,
		MaxStates:           signalDefaultMaxStates,
		ProtoLogCompression: signalProtoLogCompressionGzip,
		Rules: []SignalRule{
			{
				ID:         "path_access",
				Name:       "Path / file access",
				Enabled:    true,
				Kind:       signalKindPathAccess,
				TTLSeconds: signalDefaultTTLSeconds,
				Weight:     1,
				Conditions: []SignalCondition{{Field: "path", Operator: "exists"}},
			},
			{
				ID:         "child_process",
				Name:       "Child process command",
				Enabled:    true,
				Kind:       signalKindChildProcess,
				TTLSeconds: signalDefaultTTLSeconds,
				Weight:     2,
				Conditions: []SignalCondition{{Field: "eventType", Operator: "regex", Value: "(EXECVE|SCHED_PROCESS_EXEC|SCHED_PROCESS_FORK|CLONE|exec|fork|clone)"}},
			},
			{
				ID:         "repeated_read",
				Name:       "Repeated read",
				Enabled:    true,
				Kind:       signalKindRepeatedRead,
				TTLSeconds: signalDefaultTTLSeconds,
				Weight:     1.5,
				Conditions: []SignalCondition{{Field: "eventType", Operator: "regex", Value: "(READ|OPEN|OPENAT|read|open)"}},
			},
		},
	}
}

func NormalizeSettings(settings *SignalProcessingSettings) {
	if settings == nil {
		return
	}
	if settings.QueueSize <= 0 {
		settings.QueueSize = signalDefaultQueueSize
	}
	if settings.QueueSize < 128 {
		settings.QueueSize = 128
	}
	if settings.QueueSize > 65536 {
		settings.QueueSize = 65536
	}
	if settings.CronIntervalSeconds <= 0 {
		settings.CronIntervalSeconds = signalDefaultCronIntervalSeconds
	}
	if settings.CronIntervalSeconds < 1 {
		settings.CronIntervalSeconds = 1
	}
	if settings.CronIntervalSeconds > 86400 {
		settings.CronIntervalSeconds = 86400
	}
	if settings.DefaultTTLSeconds <= 0 {
		settings.DefaultTTLSeconds = signalDefaultTTLSeconds
	}
	if settings.DefaultTTLSeconds < 1 {
		settings.DefaultTTLSeconds = 1
	}
	if settings.DefaultTTLSeconds > 2592000 {
		settings.DefaultTTLSeconds = 2592000
	}
	if settings.MaxStates <= 0 {
		settings.MaxStates = signalDefaultMaxStates
	}
	if settings.MaxStates < 128 {
		settings.MaxStates = 128
	}
	if settings.MaxStates > 100000 {
		settings.MaxStates = 100000
	}
	settings.ProtoLogCompression = strings.ToLower(strings.TrimSpace(settings.ProtoLogCompression))
	if settings.ProtoLogCompression == "" {
		settings.ProtoLogCompression = signalProtoLogCompressionGzip
	}
	if settings.ProtoLogCompression != signalProtoLogCompressionGzip {
		settings.ProtoLogCompression = signalProtoLogCompressionGzip
	}
	if settings.Rules == nil {
		defaults := DefaultSettings()
		settings.Rules = defaults.Rules
	}
	for index := range settings.Rules {
		normalizeSignalRule(&settings.Rules[index], settings.DefaultTTLSeconds, index)
	}
	dedupeSignalRuleIDs(settings.Rules)
	if len(settings.SelectedPrograms) > signalMaxSelectedPrograms {
		settings.SelectedPrograms = settings.SelectedPrograms[:signalMaxSelectedPrograms]
	}
	for index := range settings.SelectedPrograms {
		settings.SelectedPrograms[index].Program = strings.TrimSpace(settings.SelectedPrograms[index].Program)
		settings.SelectedPrograms[index].Path = strings.TrimSpace(settings.SelectedPrograms[index].Path)
	}
}

func normalizeSignalRule(rule *SignalRule, defaultTTLSeconds int, index int) {
	if rule == nil {
		return
	}
	rule.ID = sanitizeSignalID(rule.ID)
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("signal_rule_%d", index+1)
	}
	rule.Kind = strings.ToLower(strings.TrimSpace(rule.Kind))
	if rule.Kind == "" {
		rule.Kind = signalKindCustom
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		rule.Name = signalRuleKindLabel(rule.Kind)
	}
	if rule.TTLSeconds <= 0 {
		rule.TTLSeconds = defaultTTLSeconds
	}
	if rule.TTLSeconds <= 0 {
		rule.TTLSeconds = signalDefaultTTLSeconds
	}
	if rule.TTLSeconds > 2592000 {
		rule.TTLSeconds = 2592000
	}
	if rule.Weight <= 0 {
		rule.Weight = 1
	}
	if rule.Weight > 1000 {
		rule.Weight = 1000
	}
	normalized := rule.Conditions[:0]
	for _, condition := range rule.Conditions {
		condition.Field = strings.TrimSpace(condition.Field)
		condition.Operator = normalizeSignalConditionOperator(condition.Operator, condition.Value)
		condition.Value = strings.TrimSpace(condition.Value)
		if condition.Field == "" {
			continue
		}
		normalized = append(normalized, condition)
	}
	rule.Conditions = normalized
}

func dedupeSignalRuleIDs(rules []SignalRule) {
	seen := make(map[string]int, len(rules))
	for index := range rules {
		id := sanitizeSignalID(rules[index].ID)
		if id == "" {
			id = fmt.Sprintf("signal_rule_%d", index+1)
		}
		seen[id]++
		if seen[id] > 1 {
			id = fmt.Sprintf("%s_%d", id, seen[id])
		}
		rules[index].ID = id
	}
}

func normalizeSignalConditionOperator(operator, value string) string {
	operator = strings.ToLower(strings.TrimSpace(operator))
	switch operator {
	case "eq":
		operator = "equals"
	case "starts_with", "startswith":
		operator = "prefix"
	case "ends_with", "endswith":
		operator = "suffix"
	}
	switch operator {
	case "equals", "contains", "prefix", "suffix", "regex", "exists", "any", "not_contains", "not_equals":
		return operator
	}
	if strings.TrimSpace(value) == "" {
		return "exists"
	}
	return "contains"
}

func sanitizeSignalID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('_')
		default:
			b.WriteByte('_')
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if len(normalized) > 80 {
		normalized = normalized[:80]
	}
	return normalized
}

func signalRuleKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case signalKindPathAccess:
		return "Path / file access"
	case signalKindChildProcess:
		return "Child process command"
	case signalKindRepeatedRead:
		return "Repeated read"
	default:
		return "Custom signal"
	}
}

func availableSignalKinds() []signalKindInfo {
	return []signalKindInfo{
		{
			Kind:        signalKindPathAccess,
			Label:       "Path / file access",
			Description: "Triggers when captured file/path fields match custom predicates.",
		},
		{
			Kind:        signalKindChildProcess,
			Label:       "Child process command",
			Description: "Triggers on exec/fork/clone style events and command/path predicates.",
		},
		{
			Kind:        signalKindRepeatedRead,
			Label:       "Repeated read",
			Description: "Tracks repeated READ/OPEN/OPENAT access to the same stable target.",
		},
		{
			Kind:        signalKindCustom,
			Label:       "Custom",
			Description: "Uses only user-defined conditions and a TTL-weighted state key.",
		},
	}
}

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
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queueSize = tasks.NormalizeQueueSize(queueSize, signalDefaultQueueSize)
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.queue = make(chan signalProcessingWorkItem, queueSize)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.started = true
	w.updatedAt = time.Now().UTC()
	queue := w.queue
	done := w.done
	w.mu.Unlock()

	go func() {
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			w.run(workerCtx, queue)
		}()
		go func() {
			defer workers.Done()
			w.runCron(workerCtx)
		}()
		workers.Wait()
		w.lifecycleMu.Lock()
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
		w.lifecycleMu.Unlock()
	}()
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
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		w.lifecycleMu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.queue = nil
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
	w.lifecycleMu.Unlock()
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

func queueSignalProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
		return
	}
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	if !settings.Enabled {
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
	if w == nil {
		return false
	}
	w.mu.RLock()
	queue := w.queue
	if queue == nil {
		w.mu.RUnlock()
		w.noteDrop("signal processing worker is not started")
		return false
	}
	accepted := false
	select {
	case queue <- item:
		accepted = true
	default:
	}
	w.mu.RUnlock()
	if accepted {
		return true
	}
	w.noteDrop("signal processing queue is full")
	return false
}

func (w *signalProcessingWorker) resetNow() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.states = make(map[string]*signalState)
	w.stateLRUHead = nil
	w.stateLRUTail = nil
	w.lastError = ""
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) noteDrop(message string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.droppedTotal++
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) noteError(message string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProcessingWorker) expireNow(now time.Time) {
	if w == nil {
		return
	}
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
	if w == nil {
		return
	}
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	w.processRecordWithSettings(record, force, settings)
}

func (w *signalProcessingWorker) processScan(ctx context.Context, records []CapturedEventRecord, force bool) {
	if w == nil || len(records) == 0 {
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
	if w == nil || record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
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

func decaySignalScore(score float64, lastMatchedAt, now time.Time, ttlSeconds int) float64 {
	if score <= 0 {
		return 0
	}
	if ttlSeconds <= 0 {
		ttlSeconds = signalDefaultTTLSeconds
	}
	if lastMatchedAt.IsZero() || now.IsZero() || !now.After(lastMatchedAt) {
		return score
	}
	elapsed := now.Sub(lastMatchedAt).Seconds()
	ttl := float64(ttlSeconds)
	if elapsed >= ttl {
		return 0
	}
	remaining := 1 - elapsed/ttl
	if remaining < 0 {
		return 0
	}
	return score * remaining
}

func (w *signalProcessingWorker) evictExpiredLocked(now time.Time) {
	for key, state := range w.states {
		if state == nil || (!state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now)) {
			if state == nil {
				delete(w.states, key)
				w.expiredTotal++
				continue
			}
			w.removeSignalStateLocked(state, false)
		}
	}
}

func (w *signalProcessingWorker) enforceMaxStatesLocked(maxStates int, now time.Time) {
	if maxStates <= 0 {
		maxStates = signalDefaultMaxStates
	}
	for len(w.states) > maxStates && w.stateLRUHead != nil {
		state := w.stateLRUHead
		capacityEviction := state.ExpiresAt.IsZero() || state.ExpiresAt.After(now)
		w.removeSignalStateLocked(state, capacityEviction)
	}
	// All production inserts are linked into the LRU. Keep a bounded defensive
	// fallback for malformed test or migrated state rather than leaving the map
	// above its configured hard limit.
	for len(w.states) > maxStates {
		for key, state := range w.states {
			if state == nil {
				delete(w.states, key)
				w.expiredTotal++
			} else {
				capacityEviction := state.ExpiresAt.IsZero() || state.ExpiresAt.After(now)
				w.removeSignalStateLocked(state, capacityEviction)
			}
			break
		}
	}
}

func (w *signalProcessingWorker) appendSignalStateLocked(state *signalState) {
	if w == nil || state == nil {
		return
	}
	state.lruPrev = w.stateLRUTail
	state.lruNext = nil
	if w.stateLRUTail == nil {
		w.stateLRUHead = state
	} else {
		w.stateLRUTail.lruNext = state
	}
	w.stateLRUTail = state
}

func (w *signalProcessingWorker) touchSignalStateLocked(state *signalState) {
	if w == nil || state == nil || w.stateLRUTail == state {
		return
	}
	w.detachSignalStateLocked(state)
	w.appendSignalStateLocked(state)
}

func (w *signalProcessingWorker) removeSignalStateLocked(state *signalState, capacityEviction bool) {
	if w == nil || state == nil || w.states[state.Key] != state {
		return
	}
	delete(w.states, state.Key)
	w.detachSignalStateLocked(state)
	w.expiredTotal++
	if capacityEviction {
		w.capacityEvictedTotal++
	}
}

func (w *signalProcessingWorker) detachSignalStateLocked(state *signalState) {
	if w == nil || state == nil {
		return
	}
	if state.lruPrev == nil {
		if w.stateLRUHead == state {
			w.stateLRUHead = state.lruNext
		}
	} else {
		state.lruPrev.lruNext = state.lruNext
	}
	if state.lruNext == nil {
		if w.stateLRUTail == state {
			w.stateLRUTail = state.lruPrev
		}
	} else {
		state.lruNext.lruPrev = state.lruPrev
	}
	state.lruPrev = nil
	state.lruNext = nil
}

func (w *signalProcessingWorker) Status() signalProcessingStatus {
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	if w == nil {
		return signalProcessingStatus{
			Enabled:        settings.Enabled,
			Settings:       settings,
			AvailableKinds: availableSignalKinds(),
			UpdatedAt:      time.Now().UTC(),
		}
	}
	now := time.Now().UTC()
	w.mu.RLock()
	queueLen := 0
	queueCap := 0
	if w.queue != nil {
		queueLen = len(w.queue)
		queueCap = cap(w.queue)
	}
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
