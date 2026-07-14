package core

import (
	"sync"
	"time"

	"agent-ebpf-filter/internal/boundedring"
	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/redaction"
)

// ── Runtime settings ─────────────────────────────────────────────────────────

// RuntimeSettings holds all runtime-configurable options.
type RuntimeSettings struct {
	LogPersistenceEnabled   bool                       `json:"logPersistenceEnabled"`
	LogFilePath             string                     `json:"logFilePath"`
	AccessToken             string                     `json:"accessToken"`
	MaxEventCount           int                        `json:"maxEventCount"`
	MaxEventAge             string                     `json:"maxEventAge"`
	ShellSessionsEnabled    bool                       `json:"shellSessionsEnabled"`
	SystemRunEnabled        bool                       `json:"systemRunEnabled"`
	HookManagementEnabled   bool                       `json:"hookManagementEnabled"`
	PolicyManagementEnabled bool                       `json:"policyManagementEnabled"`
	OtlpEnabled             bool                       `json:"otlpEnabled"`
	OtlpEndpoint            string                     `json:"otlpEndpoint"`
	OtlpServiceName         string                     `json:"otlpServiceName"`
	OtlpHeaders             map[string]string          `json:"otlpHeaders,omitempty"`
	HookSecrets             map[string]string          `json:"hookSecrets,omitempty"`
	MLConfig                MLConfig                   `json:"mlConfig,omitempty"`
	TlsCaptureEnabled       bool                       `json:"tlsCaptureEnabled"`
	KernelRiskFeedback      KernelRiskFeedbackSettings `json:"kernelRiskFeedback,omitempty"`
	LoopDetection           LoopDetectionSettings      `json:"loopDetection,omitempty"`
	ResearchProcessing      ResearchProcessingSettings `json:"researchProcessing,omitempty"`
	SignalProcessing        SignalProcessingSettings   `json:"signalProcessing,omitempty"`
	DomainForwardProxy      DomainForwardProxySettings `json:"domainForwardProxy"`
	RedactionPolicy         redaction.RedactionPolicy  `json:"redaction,omitempty"`
}

// KernelRiskFeedbackSettings controls the optional closed loop from user-space
// kernel-event risk scoring back into kernel-enforced cgroup/LSM policy maps.
type KernelRiskFeedbackSettings struct {
	Enabled             bool    `json:"enabled"`
	MinRiskScore        float64 `json:"minRiskScore"`
	EnforceNetwork      bool    `json:"enforceNetwork"`
	EnforceFileNames    bool    `json:"enforceFileNames"`
	EnforceExec         bool    `json:"enforceExec"`
	MaxActionsPerMinute int     `json:"maxActionsPerMinute"`
}

// LoopDetectionSettings controls the single-consumer repeated-context detector.
// It is intentionally separated from hard enforcement: findings are surfaced to
// the UI and, optionally, mirrored as RESOURCE_WASTING_LOOP semantic alerts.
type LoopDetectionSettings struct {
	Enabled            bool `json:"enabled"`
	WindowSeconds      int  `json:"windowSeconds"`
	RepeatThreshold    int  `json:"repeatThreshold"`
	MaxContexts        int  `json:"maxContexts"`
	QueueSize          int  `json:"queueSize"`
	EmitSemanticAlerts bool `json:"emitSemanticAlerts"`
}

// ResearchProcessingSettings controls the backend mirror of frontend
// AgentSight/research data transforms. The worker keeps normalized event
// summaries, process context, and timeline buckets ready for UI/research APIs.
type ResearchProcessingSettings struct {
	Enabled               bool   `json:"enabled"`
	MaxEvents             int    `json:"maxEvents"`
	QueueSize             int    `json:"queueSize"`
	TimelineBucketSeconds int    `json:"timelineBucketSeconds"`
	TopK                  int    `json:"topK"`
	RecentSamples         int    `json:"recentSamples"`
	ArtifactRetentionDays int    `json:"artifactRetentionDays"`
	MaxSessionEvents      int    `json:"maxSessionEvents"`
	ExportFormats         string `json:"exportFormats"`
}

// SignalCondition describes one predicate used by a signal rule. Conditions are
// ANDed inside a rule and evaluate against normalized captured-event fields.
type SignalCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// SignalRule defines a configurable runtime signal. Kind is intentionally open
// ended so the UI can add new signal classes without changing the persistence
// schema; the backend ships built-in semantics for path_access, child_process,
// repeated_read, and custom.
type SignalRule struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Enabled    bool              `json:"enabled"`
	Kind       string            `json:"kind"`
	TTLSeconds int               `json:"ttlSeconds"`
	Weight     float64           `json:"weight"`
	Conditions []SignalCondition `json:"conditions,omitempty"`
}

// SelectedProgramSignalLog configures compressed protobuf binary persistence for
// events whose selected frontend program matches comm/path/basename fields.
type SelectedProgramSignalLog struct {
	Program string `json:"program"`
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

// SignalProcessingSettings controls the single-consumer signal worker. Event
// matching is lazy (only touched keys are recomputed on updates) while a cron
// pass evicts expired signal state from the bounded in-memory map.
type SignalProcessingSettings struct {
	Enabled             bool                       `json:"enabled"`
	QueueSize           int                        `json:"queueSize"`
	CronIntervalSeconds int                        `json:"cronIntervalSeconds"`
	DefaultTTLSeconds   int                        `json:"defaultTTLSeconds"`
	MaxStates           int                        `json:"maxStates"`
	ProtoLogCompression string                     `json:"protoLogCompression"`
	SelectedPrograms    []SelectedProgramSignalLog `json:"selectedPrograms,omitempty"`
	Rules               []SignalRule               `json:"rules,omitempty"`
}

// ExportConfig is the JSON shape returned by GET /config/export.
type ExportConfig struct {
	Tags    []string               `json:"tags"`
	Comms   map[string]string      `json:"comms"`
	Paths   map[string]string      `json:"paths"`
	Rules   map[string]WrapperRule `json:"rules"`
	Runtime *RuntimeSettings       `json:"runtime,omitempty"`
}

// RuntimeConfigResponse is returned by GET /config/runtime.
type RuntimeConfigResponse struct {
	Runtime                RuntimeSettings `json:"runtime"`
	MCPEndpoint            string          `json:"mcpEndpoint"`
	AuthHeaderName         string          `json:"authHeaderName"`
	BearerAuthHeaderName   string          `json:"bearerAuthHeaderName"`
	PersistedEventLogPath  string          `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool            `json:"persistedEventLogAlive"`
}

// ── Event archive ────────────────────────────────────────────────────────────

// CapturedEventRecord wraps a decoded protobuf event with its receive timestamp.
type CapturedEventRecord struct {
	ReceivedAt time.Time         `json:"receivedAt"`
	Event      *pb.Event         `json:"event"`
	Envelope   *pb.EventEnvelope `json:"-"`
}

// EventArchive is a bounded, thread-safe ring of recent events.
type EventArchive struct {
	mu      sync.RWMutex
	records *boundedring.Ring[CapturedEventRecord]
}

// NewEventArchive creates a new archive with the given capacity.
func NewEventArchive(max int) *EventArchive {
	if max <= 0 {
		max = 1000
	}
	return &EventArchive{records: boundedring.New[CapturedEventRecord](max)}
}

// Add appends a record, evicting the oldest if at capacity.
func (a *EventArchive) Add(record CapturedEventRecord) {
	a.mu.Lock()
	if a.records == nil {
		a.records = boundedring.New[CapturedEventRecord](1000)
	}
	a.records.Add(record)
	a.mu.Unlock()
}

// Snapshot returns up to limit recent records (newest last).
func (a *EventArchive) Snapshot(limit int) []CapturedEventRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.records == nil {
		return nil
	}
	return a.records.Recent(limit)
}

// Clear drops all records.
func (a *EventArchive) Clear() {
	a.mu.Lock()
	if a.records != nil {
		a.records.Clear()
	}
	a.mu.Unlock()
}

// SetMax updates the maximum capacity, evicting if necessary.
func (a *EventArchive) SetMax(n int) {
	if n <= 0 {
		n = 1000
	}
	a.mu.Lock()
	if a.records == nil {
		a.records = boundedring.New[CapturedEventRecord](n)
	} else if a.records.Limit() != n {
		retained := a.records.Recent(n)
		a.records = boundedring.New[CapturedEventRecord](n)
		a.records.AddBatch(retained)
	}
	a.mu.Unlock()
}

// EvictOlderThan removes records older than the given threshold.
func (a *EventArchive) EvictOlderThan(threshold time.Time) {
	a.mu.Lock()
	if a.records != nil {
		a.records.Retain(func(record CapturedEventRecord) bool {
			return !record.ReceivedAt.Before(threshold)
		})
	}
	a.mu.Unlock()
}

// Count returns the current number of records.
func (a *EventArchive) Count() int {
	a.mu.RLock()
	count := 0
	if a.records != nil {
		count = a.records.Len()
	}
	a.mu.RUnlock()
	return count
}
