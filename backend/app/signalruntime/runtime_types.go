package signalruntime

import (
	"agent-ebpf-filter/internal/workerqueue"
	"fmt"
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
	queue                workerqueue.Queue[signalProcessingWorkItem]
	mu                   sync.RWMutex
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
