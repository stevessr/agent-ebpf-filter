package tls

import (
	"fmt"
	"strings"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"github.com/gorilla/websocket"
)

// ── Deps: external dependencies injected by app/main.go via Init() ─────────

type ProcessContextStore interface {
	Get(pid uint32) (ProcessContext, bool)
}

type ProcessContext struct {
	RootAgentPid   uint32
	AgentRunID     string
	TaskID         string
	ConversationID string
	TurnID         string
	ToolCallID     string
	ToolName       string
	TraceID        string
	SpanID         string
	Decision       string
	ContainerID    string
	ArgvDigest     string
	Cwd            string
	RiskScore      float64
}

type CollectorMetricsStore interface {
	RecordAgentSightCounter(name string)
}

type RuntimeSettings = core.RuntimeSettings

type Deps struct {
	Broadcast            chan<- *pb.Event
	TrackedProcessContexts ProcessContextStore
	CollectorMetrics     CollectorMetricsStore
	Upgrader             *websocket.Upgrader
}

var deps Deps

func Init(d Deps) { deps = d }

// ── Dependency accessors ─────────────────────────────────────────────────────

func GetBroadcast() chan<- *pb.Event          { return deps.Broadcast }
func GetUpgrader() *websocket.Upgrader        { return deps.Upgrader }
func GetCollectorMetrics() CollectorMetricsStore { return deps.CollectorMetrics }
func GetTrackedProcessContexts() ProcessContextStore { return deps.TrackedProcessContexts }

// ── In-package constants ─────────────────────────────────────────────────────

const eventSchemaVersion = "event.v3"

// ── Utility functions migrated inline ────────────────────────────────────────

func sanitizeUTF8(b []byte) string {
	return strings.ToValidUTF8(strings.TrimRight(string(b), "\x00"), "�")
}

func parseEventLimitQuery(raw string, defaultLimit int) int {
	n := 0
	if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n > 0 && n <= 10000 {
		return n
	}
	return defaultLimit
}
