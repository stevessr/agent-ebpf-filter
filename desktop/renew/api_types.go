package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type eventSummary struct {
	EventID         string  `json:"eventId"`
	PID             int     `json:"pid"`
	PPID            int     `json:"ppid"`
	RootAgentPID    int     `json:"rootAgentPid"`
	EventType       int     `json:"eventType"`
	Type            string  `json:"type"`
	Tag             string  `json:"tag"`
	Comm            string  `json:"comm"`
	Target          string  `json:"target"`
	Domain          string  `json:"domain,omitempty"`
	NetEndpoint     string  `json:"netEndpoint,omitempty"`
	Network         bool    `json:"network"`
	NetBytes        int64   `json:"netBytes"`
	Decision        string  `json:"decision"`
	RiskScore       float64 `json:"riskScore"`
	AgentRunID      string  `json:"agentRunId"`
	ConversationID  string  `json:"conversationId"`
	ToolName        string  `json:"toolName"`
	HasAgentContext bool    `json:"hasAgentContext"`
	ReceivedAtMS    int64   `json:"receivedAtMs"`
	SearchText      string  `json:"-"`
}

func (e *eventSummary) UnmarshalJSON(data []byte) error {
	type compactEventSummary eventSummary
	var wire struct {
		compactEventSummary
		Path       string `json:"path"`
		ExtraPath  string `json:"extraPath"`
		ToolCallID string `json:"toolCallId"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*e = eventSummary(wire.compactEventSummary)
	if strings.TrimSpace(e.Target) == "" {
		for _, value := range []string{wire.Path, e.NetEndpoint, e.Domain, wire.ExtraPath, e.ToolName} {
			if strings.TrimSpace(value) != "" {
				e.Target = value
				break
			}
		}
	}
	if !e.Network {
		e.Network = strings.TrimSpace(e.NetEndpoint) != "" || strings.TrimSpace(e.Domain) != "" || isNetworkEvent(*e)
	}
	if !e.HasAgentContext {
		e.HasAgentContext = e.AgentRunID != "" ||
			e.ConversationID != "" ||
			wire.ToolCallID != "" ||
			e.RootAgentPID > 0 ||
			(strings.TrimSpace(e.Tag) != "" && !strings.EqualFold(e.Tag, "Unknown"))
	}
	return nil
}

type eventSummaryResponse struct {
	Events     []eventSummary `json:"events"`
	NextCursor string         `json:"nextCursor"`
}

type collectorHealth struct {
	CaptureHealthy      bool  `json:"captureHealthy"`
	RingbufDroppedTotal int64 `json:"ringbufDroppedTotal"`
	BackendQueueLen     int64 `json:"backendQueueLen"`
	PersistQueueLen     int64 `json:"persistQueueLen"`
	PersistQueueCap     int64 `json:"persistQueueCap"`
	PersistPending      int64 `json:"persistPending"`
}

type eventTypeConfig struct {
	DisabledEventTypes []int `json:"disabled_event_types"`
}

type runtimeToggle struct {
	Enabled bool `json:"enabled"`
}

type desktopPrivacySettings struct {
	ContextMode    string `json:"contextMode"`
	JSONFieldRules []struct {
		Path      string `json:"path"`
		Action    string `json:"action"`
		Recipient string `json:"recipient"`
	} `json:"jsonFieldRules"`
	Enabled         bool   `json:"enabled"`
	Action          string `json:"action"`
	RestoreResponse bool   `json:"restoreResponse"`
	MaxBodyBytes    int64  `json:"maxBodyBytes"`
}
type desktopDomainForwardSettings struct {
	Enabled bool                   `json:"enabled"`
	Privacy desktopPrivacySettings `json:"privacy"`
}

type runtimeSettings struct {
	LogPersistenceEnabled   bool                         `json:"logPersistenceEnabled"`
	LogFilePath             string                       `json:"logFilePath"`
	DisabledEventTypes      []int                        `json:"disabledEventTypes"`
	PolicyManagementEnabled bool                         `json:"policyManagementEnabled"`
	TLSCaptureEnabled       bool                         `json:"tlsCaptureEnabled"`
	DomainForwardProxy      desktopDomainForwardSettings `json:"domainForwardProxy"`
	LoopDetection           runtimeToggle                `json:"loopDetection"`
	SignalProcessing        runtimeToggle                `json:"signalProcessing"`
	ResearchProcessing      runtimeToggle                `json:"researchProcessing"`
}

type runtimeConfigResponse struct {
	Runtime                runtimeSettings `json:"runtime"`
	PersistedEventLogPath  string          `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool            `json:"persistedEventLogAlive"`
}

type wrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

type trackedComm struct {
	Comm     string `json:"comm"`
	Tag      string `json:"tag"`
	Disabled bool   `json:"disabled"`
}

type trackedPath struct {
	Path string `json:"path"`
	Tag  string `json:"tag"`
}

type trackedPrefix struct {
	Prefix string `json:"prefix"`
	Tag    string `json:"tag"`
}

type registrySnapshot struct {
	Tags     []string
	Comms    []trackedComm
	Paths    []trackedPath
	Prefixes []trackedPrefix
}

type cgroupSandboxStatus struct {
	Available    bool     `json:"available"`
	Attached     bool     `json:"attached"`
	BlockedIPs   []string `json:"blockedIPs"`
	BlockedPorts []int    `json:"blockedPorts"`
	Error        string   `json:"error"`
}

type fileAccessRule struct {
	Path      string `json:"path"`
	DenyRead  bool   `json:"denyRead"`
	DenyWrite bool   `json:"denyWrite"`
}

type lsmSandboxStatus struct {
	PathAccessRules     []fileAccessRule `json:"pathAccessRules"`
	PathAccessSupported bool             `json:"pathAccessSupported"`
	Available           bool             `json:"available"`
	Attached            bool             `json:"attached"`
	BlockedExecPaths    []string         `json:"blockedExecPaths"`
	Error               string           `json:"error"`
}

type enforcementSnapshot struct {
	Cgroup cgroupSandboxStatus
	LSM    lsmSandboxStatus
}

type apiSnapshot struct {
	Events       []eventSummary
	NextCursor   string
	Health       collectorHealth
	TrackedComms []string
	FetchedAt    time.Time
}

type apiClient struct {
	origin     string
	token      string
	http       *http.Client
	unixSocket string
}
