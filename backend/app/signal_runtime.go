package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
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
}

type signalProcessingStatus struct {
	Enabled        bool                     `json:"enabled"`
	Settings       SignalProcessingSettings `json:"settings"`
	QueueLen       int                      `json:"queueLen"`
	QueueCap       int                      `json:"queueCap"`
	ConsumedTotal  uint64                   `json:"consumedTotal"`
	UpdatedTotal   uint64                   `json:"updatedTotal"`
	DroppedTotal   uint64                   `json:"droppedTotal"`
	ExpiredTotal   uint64                   `json:"expiredTotal"`
	ActiveStates   int                      `json:"activeStates"`
	RecentStates   []signalState            `json:"recentStates"`
	AvailableKinds []signalKindInfo         `json:"availableKinds"`
	LastError      string                   `json:"lastError,omitempty"`
	UpdatedAt      time.Time                `json:"updatedAt"`
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
	Compression string                   `json:"compression"`
	Logs        []signalProgramLogStatus `json:"logs"`
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
	lifecycleMu   sync.Mutex
	mu            sync.RWMutex
	queue         chan signalProcessingWorkItem
	cancel        context.CancelFunc
	done          chan struct{}
	started       bool
	states        map[string]*signalState
	consumedTotal uint64
	updatedTotal  uint64
	droppedTotal  uint64
	expiredTotal  uint64
	lastError     string
	updatedAt     time.Time
}

var (
	signalProcessingWorkerStore = newSignalProcessingWorker()
	signalProgramLogMu          sync.Mutex
)

func defaultSignalProcessingSettings() SignalProcessingSettings {
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

func normalizeSignalProcessingSettings(settings *SignalProcessingSettings) {
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
		defaults := defaultSignalProcessingSettings()
		settings.Rules = defaults.Rules
	}
	for index := range settings.Rules {
		normalizeSignalRule(&settings.Rules[index], settings.DefaultTTLSeconds, index)
	}
	dedupeSignalRuleIDs(settings.Rules)
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
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	signalProcessingWorkerStore.Start(ctx, settings.QueueSize)
}

func (w *signalProcessingWorker) Start(ctx context.Context, queueSize int) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if queueSize <= 0 {
		queueSize = signalDefaultQueueSize
	}
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

func (w *signalProcessingWorker) run(ctx context.Context, queue <-chan signalProcessingWorkItem) {
	for {
		var item signalProcessingWorkItem
		select {
		case <-ctx.Done():
			return
		case item = <-queue:
		}
		switch item.kind {
		case signalProcessingWorkReset:
			w.resetNow()
		case signalProcessingWorkScan:
			for _, record := range item.records {
				w.processRecord(record, item.force)
			}
		case signalProcessingWorkExpire:
			w.expireNow(time.Now().UTC())
		case signalProcessingWorkEvent:
			w.processRecord(item.record, item.force)
		}
	}
}

func (w *signalProcessingWorker) runCron(ctx context.Context) {
	for {
		settings := runtimeSettingsStore.Snapshot().SignalProcessing
		normalizeSignalProcessingSettings(&settings)
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

func queueSignalProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
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
	w.mu.RUnlock()
	if queue == nil {
		w.noteDrop("signal processing worker is not started")
		return false
	}
	select {
	case queue <- item:
		return true
	default:
		w.noteDrop("signal processing queue is full")
		return false
	}
}

func (w *signalProcessingWorker) resetNow() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.states = make(map[string]*signalState)
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
	w.evictExpiredLocked(now)
	w.updatedAt = now
	w.mu.Unlock()
}

func (w *signalProcessingWorker) processRecord(record CapturedEventRecord, force bool) {
	if w == nil || record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	if !force && !settings.Enabled {
		return
	}
	if len(settings.Rules) == 0 {
		return
	}
	record = normalizeCapturedEventRecord(record)
	event := record.Event
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	updates := make([]*signalState, 0, 4)
	for _, rule := range settings.Rules {
		ruleCopy := rule
		normalizeSignalRule(&ruleCopy, settings.DefaultTTLSeconds, 0)
		if !ruleCopy.Enabled || !signalRuleMatches(ruleCopy, event) {
			continue
		}
		stateKey, target := signalStateKey(ruleCopy, event)
		if stateKey == "" {
			continue
		}
		updates = append(updates, &signalState{
			ID:            signalStateID(stateKey),
			RuleID:        ruleCopy.ID,
			RuleName:      ruleCopy.Name,
			Kind:          ruleCopy.Kind,
			Key:           stateKey,
			Target:        target,
			PID:           event.GetPid(),
			TGID:          event.GetTgid(),
			Comm:          strings.TrimSpace(event.GetComm()),
			TTLSeconds:    ruleCopy.TTLSeconds,
			LastMatchedAt: observedAt,
			UpdatedAt:     observedAt,
			ExpiresAt:     observedAt.Add(time.Duration(ruleCopy.TTLSeconds) * time.Second),
			LastEventType: signalEventType(event),
			LastPath:      strings.TrimSpace(event.GetPath()),
			LastExtraPath: strings.TrimSpace(event.GetExtraPath()),
			LastEventID:   recordEnvelopeID(record),
			Score:         ruleCopy.Weight,
			Count:         1,
		})
	}
	if len(updates) == 0 {
		w.mu.Lock()
		w.consumedTotal++
		w.evictExpiredLocked(observedAt)
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	w.consumedTotal++
	w.evictExpiredLocked(observedAt)
	for _, update := range updates {
		existing := w.states[update.Key]
		if existing == nil {
			update.FirstSeen = observedAt
			w.states[update.Key] = update
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
		}
		w.updatedTotal++
	}
	w.enforceMaxStatesLocked(settings.MaxStates)
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
			delete(w.states, key)
			w.expiredTotal++
		}
	}
}

func (w *signalProcessingWorker) enforceMaxStatesLocked(maxStates int) {
	if maxStates <= 0 || len(w.states) <= maxStates {
		return
	}
	type candidate struct {
		key string
		ts  time.Time
	}
	candidates := make([]candidate, 0, len(w.states))
	for key, state := range w.states {
		ts := state.UpdatedAt
		if ts.IsZero() {
			ts = state.ExpiresAt
		}
		candidates = append(candidates, candidate{key: key, ts: ts})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ts.Before(candidates[j].ts) })
	for len(w.states) > maxStates && len(candidates) > 0 {
		delete(w.states, candidates[0].key)
		w.expiredTotal++
		candidates = candidates[1:]
	}
}

func (w *signalProcessingWorker) Status() signalProcessingStatus {
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
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
		cloned.Score = decaySignalScore(cloned.Score, cloned.LastMatchedAt, now, cloned.TTLSeconds)
		states = append(states, cloned)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].UpdatedAt.After(states[j].UpdatedAt) })
	if len(states) > signalRecentStateLimit {
		states = states[:signalRecentStateLimit]
	}
	status := signalProcessingStatus{
		Enabled:        settings.Enabled,
		Settings:       settings,
		QueueLen:       queueLen,
		QueueCap:       queueCap,
		ConsumedTotal:  w.consumedTotal,
		UpdatedTotal:   w.updatedTotal,
		DroppedTotal:   w.droppedTotal,
		ExpiredTotal:   w.expiredTotal,
		ActiveStates:   len(states),
		RecentStates:   states,
		AvailableKinds: availableSignalKinds(),
		LastError:      w.lastError,
		UpdatedAt:      w.updatedAt,
	}
	w.mu.RUnlock()
	return status
}

func shouldIgnoreSignalProcessingEvent(event *pb.Event) bool {
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

func signalRuleMatches(rule SignalRule, event *pb.Event) bool {
	if event == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(rule.Kind))
	if kind == "" {
		kind = signalKindCustom
	}
	if kind != signalKindCustom && !signalKindDefaultMatches(kind, event) {
		return false
	}
	if len(rule.Conditions) == 0 {
		return kind != signalKindCustom
	}
	for _, condition := range rule.Conditions {
		if !signalConditionMatches(condition, event) {
			return false
		}
	}
	return true
}

func signalKindDefaultMatches(kind string, event *pb.Event) bool {
	if event == nil {
		return false
	}
	eventType := strings.ToLower(signalEventType(event))
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case signalKindPathAccess:
		return strings.TrimSpace(event.GetPath()) != "" ||
			strings.TrimSpace(event.GetExtraPath()) != "" ||
			strings.Contains(eventType, "open") ||
			strings.Contains(eventType, "read") ||
			strings.Contains(eventType, "write")
	case signalKindChildProcess:
		return event.GetEventType() == pb.EventType_EXECVE ||
			event.GetEventType() == pb.EventType_SCHED_PROCESS_EXEC ||
			event.GetEventType() == pb.EventType_SCHED_PROCESS_FORK ||
			event.GetEventType() == pb.EventType_CLONE ||
			strings.Contains(eventType, "exec") ||
			strings.Contains(eventType, "fork") ||
			strings.Contains(eventType, "clone")
	case signalKindRepeatedRead:
		return (event.GetEventType() == pb.EventType_READ ||
			event.GetEventType() == pb.EventType_OPEN ||
			event.GetEventType() == pb.EventType_OPENAT ||
			strings.Contains(eventType, "read") ||
			strings.Contains(eventType, "open")) &&
			(strings.TrimSpace(event.GetPath()) != "" || strings.TrimSpace(event.GetExtraPath()) != "")
	default:
		return false
	}
}

func signalConditionMatches(condition SignalCondition, event *pb.Event) bool {
	values := signalFieldValues(condition.Field, event)
	operator := normalizeSignalConditionOperator(condition.Operator, condition.Value)
	needle := strings.TrimSpace(condition.Value)
	switch operator {
	case "exists", "any":
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
		return false
	case "regex":
		if needle == "" {
			return false
		}
		re, err := regexp.Compile(needle)
		if err != nil {
			return false
		}
		for _, value := range values {
			if re.MatchString(value) {
				return true
			}
		}
		return false
	}
	if needle == "" {
		return false
	}
	needleLower := strings.ToLower(needle)
	for _, value := range values {
		value = strings.TrimSpace(value)
		valueLower := strings.ToLower(value)
		switch operator {
		case "equals":
			if valueLower == needleLower {
				return true
			}
		case "not_equals":
			if valueLower != "" && valueLower != needleLower {
				return true
			}
		case "prefix":
			if strings.HasPrefix(valueLower, needleLower) {
				return true
			}
		case "suffix":
			if strings.HasSuffix(valueLower, needleLower) {
				return true
			}
		case "not_contains":
			if valueLower != "" && !strings.Contains(valueLower, needleLower) {
				return true
			}
		default:
			if strings.Contains(valueLower, needleLower) {
				return true
			}
		}
	}
	return false
}

func signalFieldValues(field string, event *pb.Event) []string {
	if event == nil {
		return nil
	}
	switch normalizeSignalFieldName(field) {
	case "path":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath())
	case "extrapath":
		return nonEmptySignalValues(event.GetExtraPath())
	case "comm", "program":
		return nonEmptySignalValues(event.GetComm())
	case "type", "eventtype":
		return nonEmptySignalValues(event.GetType(), event.GetEventType().String(), strconv.Itoa(int(event.GetEventType())))
	case "childcommand", "command", "cmdline":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraInfo(), event.GetArgvDigest(), event.GetComm())
	case "readkey":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath(), event.GetArgvDigest())
	case "target":
		return nonEmptySignalValues(signalStableTarget(event))
	case "extrainfo":
		return nonEmptySignalValues(event.GetExtraInfo())
	case "cwd":
		return nonEmptySignalValues(event.GetCwd())
	case "tool", "toolname":
		return nonEmptySignalValues(event.GetToolName())
	case "netendpoint", "endpoint":
		return nonEmptySignalValues(event.GetNetEndpoint(), event.GetDstIp(), event.GetDnsName(), event.GetSni(), event.GetHttpHost(), event.GetDomain())
	case "decision":
		return nonEmptySignalValues(event.GetDecision())
	default:
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath(), event.GetExtraInfo(), event.GetComm(), event.GetToolName(), signalEventType(event))
	}
}

func normalizeSignalFieldName(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	replacer := strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
	return replacer.Replace(field)
}

func nonEmptySignalValues(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func signalStateKey(rule SignalRule, event *pb.Event) (string, string) {
	target := signalStableTarget(event)
	switch strings.ToLower(strings.TrimSpace(rule.Kind)) {
	case signalKindRepeatedRead:
		target = firstNonEmptySignalValue(event.GetPath(), event.GetExtraPath(), event.GetArgvDigest(), target)
	case signalKindChildProcess:
		target = firstNonEmptySignalValue(event.GetPath(), extractSignalCommand(event.GetExtraInfo()), event.GetArgvDigest(), event.GetComm(), target)
	case signalKindPathAccess:
		target = firstNonEmptySignalValue(event.GetPath(), event.GetExtraPath(), target)
	default:
		target = firstNonEmptySignalValue(target, event.GetPath(), event.GetExtraPath(), event.GetComm(), signalEventType(event))
	}
	if target == "" {
		return "", ""
	}
	context := signalContextKey(event)
	if context == "" {
		context = "global"
	}
	return strings.Join([]string{rule.ID, strings.ToLower(strings.TrimSpace(rule.Kind)), context, target}, "\x00"), target
}

func signalContextKey(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if toolCallID := strings.TrimSpace(event.GetToolCallId()); toolCallID != "" {
		return "tool_call:" + strings.Join(nonEmptySignalValues(event.GetAgentRunId(), event.GetTaskId(), toolCallID), "/")
	}
	if taskID := strings.TrimSpace(event.GetTaskId()); taskID != "" {
		return "task:" + strings.Join(nonEmptySignalValues(event.GetAgentRunId(), taskID), "/")
	}
	if runID := strings.TrimSpace(event.GetAgentRunId()); runID != "" {
		return "agent_run:" + runID
	}
	if root := event.GetRootAgentPid(); root != 0 {
		return fmt.Sprintf("root:%d", root)
	}
	if tgid := event.GetTgid(); tgid != 0 {
		return fmt.Sprintf("tgid:%d", tgid)
	}
	if pid := event.GetPid(); pid != 0 {
		return fmt.Sprintf("pid:%d", pid)
	}
	if comm := strings.TrimSpace(event.GetComm()); comm != "" {
		return "comm:" + comm
	}
	return ""
}

func signalStableTarget(event *pb.Event) string {
	if event == nil {
		return ""
	}
	target := firstNonEmptySignalValue(
		event.GetPath(),
		event.GetExtraPath(),
		event.GetNetEndpoint(),
		event.GetDnsName(),
		event.GetSni(),
		event.GetHttpHost(),
		event.GetDomain(),
		event.GetArgvDigest(),
		event.GetToolName(),
		event.GetComm(),
	)
	if strings.HasPrefix(target, "/") {
		target = filepath.Clean(target)
	}
	if len(target) > 240 {
		target = target[:240]
	}
	return target
}

func firstNonEmptySignalValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func signalEventType(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if value := strings.TrimSpace(event.GetType()); value != "" {
		return value
	}
	return strings.TrimSpace(event.GetEventType().String())
}

func extractSignalCommand(extraInfo string) string {
	extraInfo = strings.TrimSpace(extraInfo)
	if extraInfo == "" {
		return ""
	}
	lower := strings.ToLower(extraInfo)
	for _, key := range []string{"cmdline", "command", "argv", "exec"} {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(extraInfo[idx+len(key):], " :=\t\n\r\"'")
		if rest == "" {
			continue
		}
		if len(rest) > 240 {
			rest = rest[:240]
		}
		return strings.TrimSpace(rest)
	}
	return extraInfo
}

func signalStateID(key string) string {
	sum := sha1.Sum([]byte(key))
	return "sig_" + hex.EncodeToString(sum[:8])
}

func recordEnvelopeID(record CapturedEventRecord) string {
	if record.Envelope != nil {
		return strings.TrimSpace(record.Envelope.GetEventId())
	}
	return ""
}

func persistSignalProgramLog(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	if len(settings.SelectedPrograms) == 0 {
		return
	}
	for _, selected := range settings.SelectedPrograms {
		if !selected.Enabled || strings.TrimSpace(selected.Program) == "" {
			continue
		}
		matched, reason := selectedProgramMatches(record.Event, selected.Program)
		if !matched {
			continue
		}
		path := selectedProgramLogPath(selected)
		logRecord := &pb.ProgramSignalLogRecord{
			SchemaVersion: eventSchemaVersion,
			Program:       strings.TrimSpace(selected.Program),
			Reason:        reason,
			PersistedAt:   time.Now().UTC().UnixMilli(),
			CapturedEvent: recordToProtoCapturedEvent(record),
			SignalKind:    "selected_program",
		}
		if err := appendCompressedProtoRecord(path, logRecord); err != nil {
			message := fmt.Sprintf("failed to persist selected program signal log %s: %v", path, err)
			log.Printf("[WARN] %s", message)
			signalProcessingWorkerStore.noteError(message)
		}
	}
}

func recordToProtoCapturedEvent(record CapturedEventRecord) *pb.CapturedEventRecord {
	record = normalizeCapturedEventRecord(record)
	return &pb.CapturedEventRecord{
		Event:     record.Event,
		Timestamp: record.ReceivedAt.UnixMilli(),
		Envelope:  record.Envelope,
	}
}

func selectedProgramMatches(event *pb.Event, program string) (bool, string) {
	if event == nil {
		return false, ""
	}
	program = strings.TrimSpace(program)
	if program == "" {
		return false, ""
	}
	candidates := []struct {
		label string
		value string
	}{
		{label: "comm", value: event.GetComm()},
		{label: "path", value: event.GetPath()},
		{label: "path_basename", value: filepath.Base(event.GetPath())},
		{label: "extra_path", value: event.GetExtraPath()},
		{label: "extra_path_basename", value: filepath.Base(event.GetExtraPath())},
		{label: "tool_name", value: event.GetToolName()},
	}
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate.value)
		if value == "." || value == "" {
			continue
		}
		if signalProgramPatternMatches(program, value) {
			return true, fmt.Sprintf("selected program %q matched %s=%q", program, candidate.label, value)
		}
	}
	return false, ""
}

func signalProgramPatternMatches(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" || value == "" {
		return false
	}
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		if ok, err := filepath.Match(pattern, value); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(pattern, filepath.Base(value)); err == nil && ok {
			return true
		}
	}
	return pattern == value || pattern == filepath.Base(value)
}

func defaultSignalProgramLogPath(program string) string {
	return filepath.Join(platform.RuntimeSettingsDir(), "signals", "program-logs", sanitizeSignalFilename(program)+".pb.gzlog")
}

func selectedProgramLogPath(selected SelectedProgramSignalLog) string {
	path := strings.TrimSpace(selected.Path)
	if path == "" {
		path = defaultSignalProgramLogPath(selected.Program)
	}
	return expandSignalPath(path)
}

func selectedProgramLogStatuses(settings SignalProcessingSettings) []signalProgramLogStatus {
	normalizeSignalProcessingSettings(&settings)
	statuses := make([]signalProgramLogStatus, 0, len(settings.SelectedPrograms))
	for _, selected := range settings.SelectedPrograms {
		program := strings.TrimSpace(selected.Program)
		if program == "" {
			continue
		}
		path := selectedProgramLogPath(selected)
		status := signalProgramLogStatus{
			Program: program,
			Enabled: selected.Enabled,
			Path:    path,
		}
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				status.Error = err.Error()
			}
			statuses = append(statuses, status)
			continue
		}
		status.Exists = true
		status.SizeBytes = info.Size()
		status.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		if frames, err := countCompressedProtoFrames(path); err == nil {
			status.FrameCount = frames
		} else {
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func resolveSelectedProgramLogPath(settings SignalProcessingSettings, program string) (string, bool) {
	normalizeSignalProcessingSettings(&settings)
	program = strings.TrimSpace(program)
	if program == "" {
		return "", false
	}
	for _, selected := range settings.SelectedPrograms {
		if strings.EqualFold(strings.TrimSpace(selected.Program), program) || strings.EqualFold(sanitizeSignalFilename(selected.Program), program) {
			return selectedProgramLogPath(selected), true
		}
	}
	return "", false
}

func sanitizeSignalFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "program"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "program"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func expandSignalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		return platform.GetRealHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(platform.GetRealHomeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}

func appendCompressedProtoRecord(path string, message proto.Message) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("signal proto log path is empty")
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(payload); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	frame := make([]byte, 4)
	binary.BigEndian.PutUint32(frame, uint32(compressed.Len()))

	signalProgramLogMu.Lock()
	defer signalProgramLogMu.Unlock()

	if err := platform.MkdirAllAsRealUser(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(frame); err != nil {
		return err
	}
	if _, err := file.Write(compressed.Bytes()); err != nil {
		return err
	}
	if os.Getuid() == 0 {
		if uid, gid, ok := platform.OriginalInvokerIDs(); ok {
			_ = os.Chown(path, int(uid), int(gid))
		}
	}
	return nil
}

func readCompressedProtoFrames(path string, newMessage func() proto.Message) ([]proto.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []proto.Message
	for {
		var frameLen uint32
		if err := binary.Read(file, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return messages, nil
			}
			return nil, err
		}
		if frameLen == 0 || frameLen > 64*1024*1024 {
			return nil, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		compressed := make([]byte, frameLen)
		if _, err := io.ReadFull(file, compressed); err != nil {
			return nil, err
		}
		gz, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(gz)
		closeErr := gz.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		msg := newMessage()
		if err := proto.Unmarshal(payload, msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
}

func countCompressedProtoFrames(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	for {
		var frameLen uint32
		if err := binary.Read(file, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return count, err
		}
		if frameLen == 0 || frameLen > 64*1024*1024 {
			return count, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		if _, err := io.CopyN(io.Discard, file, int64(frameLen)); err != nil {
			return count, err
		}
		count++
	}
}

func handleSignalProcessingStatus(c *gin.Context) {
	c.JSON(200, signalProcessingWorkerStore.Status())
}

func handleSignalProcessingTask(c *gin.Context) {
	var req signalProcessingTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid signal processing task"})
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
		if !signalProcessingWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "signal processing worker queue is full or not started"})
			return
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "expire", "cron":
		if !signalProcessingWorkerStore.EnqueueExpire() {
			signalProcessingWorkerStore.expireNow(time.Now().UTC())
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "expire", QueueLen: status.QueueLen})
	case "reset":
		if !signalProcessingWorkerStore.EnqueueReset() {
			signalProcessingWorkerStore.resetNow()
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported signal processing action"})
	}
}

func handleSignalRuleTest(c *gin.Context) {
	var req signalRuleTestRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Rule == nil {
		c.JSON(400, gin.H{"error": "invalid signal rule test request"})
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	rule := *req.Rule
	normalizeSignalRule(&rule, settings.DefaultTTLSeconds, 0)

	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 50000 {
		limit = 50000
	}
	records, _, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	response := signalRuleTestResponse{
		Rule:         rule,
		ScannedTotal: len(records),
		Matches:      make([]signalRuleTestMatch, 0, 25),
	}
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
			continue
		}
		if !signalRuleMatches(rule, record.Event) {
			continue
		}
		response.MatchedTotal++
		if len(response.Matches) >= 25 {
			continue
		}
		key, target := signalStateKey(rule, record.Event)
		response.Matches = append(response.Matches, signalRuleTestMatch{
			Timestamp:  record.ReceivedAt.UTC(),
			PID:        record.Event.GetPid(),
			TGID:       record.Event.GetTgid(),
			Comm:       strings.TrimSpace(record.Event.GetComm()),
			EventType:  signalEventType(record.Event),
			Target:     target,
			Path:       strings.TrimSpace(record.Event.GetPath()),
			ExtraPath:  strings.TrimSpace(record.Event.GetExtraPath()),
			ExtraInfo:  strings.TrimSpace(record.Event.GetExtraInfo()),
			EventID:    recordEnvelopeID(record),
			SignalKey:  key,
			WouldScore: rule.Weight,
		})
	}
	c.JSON(200, response)
}

func handleSignalProgramLogs(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	c.JSON(200, signalProgramLogsResponse{
		Compression: settings.ProtoLogCompression,
		Logs:        selectedProgramLogStatuses(settings),
	})
}

func handleSignalProgramLogDownload(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	program := c.Query("program")
	path, ok := resolveSelectedProgramLogPath(settings, program)
	if !ok {
		c.JSON(404, gin.H{"error": "selected program log is not configured"})
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"error": "selected program log file does not exist"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if info.IsDir() {
		c.JSON(400, gin.H{"error": "selected program log path is a directory"})
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}
