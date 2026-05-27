package core

import (
	"sync"
	"time"

	"agent-ebpf-filter/pb"
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
	DomainForwardProxy      DomainForwardProxySettings `json:"domainForwardProxy"`
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
	records []CapturedEventRecord
	max     int
}

// NewEventArchive creates a new archive with the given capacity.
func NewEventArchive(max int) *EventArchive {
	if max <= 0 {
		max = 1000
	}
	return &EventArchive{max: max}
}

// Add appends a record, evicting the oldest if at capacity.
func (a *EventArchive) Add(record CapturedEventRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.records = append(a.records, record)
	if len(a.records) > a.max {
		copy(a.records, a.records[len(a.records)-a.max:])
		a.records = a.records[:a.max]
	}
}

// Snapshot returns up to limit recent records (newest last).
func (a *EventArchive) Snapshot(limit int) []CapturedEventRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 || limit > len(a.records) {
		limit = len(a.records)
	}
	if limit == 0 {
		return nil
	}

	out := make([]CapturedEventRecord, limit)
	copy(out, a.records[len(a.records)-limit:])
	return out
}

// Clear drops all records.
func (a *EventArchive) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = nil
}

// SetMax updates the maximum capacity, evicting if necessary.
func (a *EventArchive) SetMax(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 {
		n = 1000
	}
	a.max = n
	if len(a.records) > a.max {
		copy(a.records, a.records[len(a.records)-a.max:])
		a.records = a.records[:a.max]
	}
}

// EvictOlderThan removes records older than the given threshold.
func (a *EventArchive) EvictOlderThan(threshold time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	keep := 0
	for _, r := range a.records {
		if !r.ReceivedAt.Before(threshold) {
			a.records[keep] = r
			keep++
		}
	}
	a.records = a.records[:keep]
}

// Count returns the current number of records.
func (a *EventArchive) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.records)
}
