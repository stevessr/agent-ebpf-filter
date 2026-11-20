package research

import (
	"sync"
	"time"
)

const (
	researchSchemaVersion     = "research.v2"
	researchManifestVersion   = "research-manifest.v1"
	researchTaskQueued        = "queued"
	researchTaskRunning       = "running"
	researchTaskSucceeded     = "succeeded"
	researchTaskFailed        = "failed"
	researchTaskCanceled      = "canceled"
	researchSessionActive     = "active"
	researchSessionBuilding   = "building"
	researchSessionReady      = "ready"
	researchSessionEmpty      = "empty"
	researchSessionError      = "error"
	researchDefaultPageLimit  = 500
	researchMaxPageLimit      = 5000
	researchDefaultTaskLimit  = 5000
	researchMaxTaskLimit      = 50000
	researchTaskStoreMaxItems = 2048
)

// ResearchSession is the persisted top-level research workspace. It stores
// normalized, already-redacted views and references to export artifacts rather
// than long-lived raw payloads.
type ResearchSession struct {
	ID           string                         `json:"id"`
	Name         string                         `json:"name"`
	Description  string                         `json:"description,omitempty"`
	Tags         []string                       `json:"tags,omitempty"`
	CreatedAt    time.Time                      `json:"createdAt"`
	UpdatedAt    time.Time                      `json:"updatedAt"`
	SourceFilter ResearchSourceFilter           `json:"sourceFilter,omitempty"`
	TimeRange    ResearchTimeRange              `json:"timeRange,omitempty"`
	Status       string                         `json:"status"`
	Summary      ResearchSessionSummary         `json:"summary"`
	ArtifactRefs map[string]ResearchArtifactRef `json:"artifactRefs,omitempty"`
	LastError    string                         `json:"lastError,omitempty"`
}

type ResearchSessionSummary struct {
	SchemaVersion      string  `json:"schemaVersion"`
	EventCount         int     `json:"eventCount"`
	EarliestTimestamp  int64   `json:"earliestTimestamp,omitempty"`
	LatestTimestamp    int64   `json:"latestTimestamp,omitempty"`
	EarliestTime       string  `json:"earliestTime,omitempty"`
	LatestTime         string  `json:"latestTime,omitempty"`
	TopSource          string  `json:"topSource,omitempty"`
	TopEventType       string  `json:"topEventType,omitempty"`
	TopComm            string  `json:"topComm,omitempty"`
	MaxRiskScore       float64 `json:"maxRiskScore,omitempty"`
	RiskAlerts         int     `json:"riskAlerts,omitempty"`
	LoopFindings       int     `json:"loopFindings,omitempty"`
	GeneratedTimestamp int64   `json:"generatedTimestamp,omitempty"`
	GeneratedTime      string  `json:"generatedTime,omitempty"`
}

type ResearchSourceFilter struct {
	Sources         []string `json:"sources,omitempty"`
	EventTypes      []string `json:"eventTypes,omitempty"`
	Comms           []string `json:"comms,omitempty"`
	PIDs            []uint32 `json:"pids,omitempty"`
	TraceID         string   `json:"traceId,omitempty"`
	SpanID          string   `json:"spanId,omitempty"`
	Query           string   `json:"query,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	IncludeTLS      *bool    `json:"includeTLS,omitempty"`
	IncludeUploaded *bool    `json:"includeUploaded,omitempty"`
}

type ResearchTimeRange struct {
	Since     int64  `json:"since,omitempty"`
	Until     int64  `json:"until,omitempty"`
	SinceTime string `json:"sinceTime,omitempty"`
	UntilTime string `json:"untilTime,omitempty"`
}

type ResearchArtifactRef struct {
	Format      string    `json:"format"`
	Name        string    `json:"name"`
	Path        string    `json:"path,omitempty"`
	ContentType string    `json:"contentType"`
	Bytes       int64     `json:"bytes"`
	SHA256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ResearchEvent struct {
	ID             string         `json:"id"`
	Timestamp      int64          `json:"timestamp"`
	Time           string         `json:"time"`
	Source         string         `json:"source"`
	EventType      string         `json:"eventType"`
	PID            uint32         `json:"pid,omitempty"`
	PPID           uint32         `json:"ppid,omitempty"`
	Comm           string         `json:"comm,omitempty"`
	TraceID        string         `json:"traceId,omitempty"`
	SpanID         string         `json:"spanId,omitempty"`
	Target         string         `json:"target,omitempty"`
	RiskScore      float64        `json:"riskScore,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	RedactionLevel string         `json:"redactionLevel,omitempty"`
	Features       map[string]any `json:"features,omitempty"`
}

type ResearchResults struct {
	SchemaVersion      string                            `json:"schemaVersion"`
	SessionID          string                            `json:"sessionId"`
	GeneratedTimestamp int64                             `json:"generatedTimestamp"`
	GeneratedTime      string                            `json:"generatedTime"`
	Summary            researchProcessingSummary         `json:"summary"`
	TopTargets         []researchCount                   `json:"topTargets"`
	TopDecisions       []researchCount                   `json:"topDecisions"`
	LoopFindings       []loopDetectionFinding            `json:"loopFindings,omitempty"`
	RiskAlerts         []ResearchRiskFinding             `json:"riskAlerts,omitempty"`
	KernelRiskFeedback ResearchKernelRiskFeedbackInfo    `json:"kernelRiskFeedback"`
	CompareWindows     *ResearchWindowCompare            `json:"compareWindows,omitempty"`
	SecurityEvaluation *ResearchSecurityEvaluationReport `json:"securityEvaluation,omitempty"`
}

type ResearchRiskFinding struct {
	EventID    string  `json:"eventId"`
	Timestamp  int64   `json:"timestamp"`
	Time       string  `json:"time"`
	Source     string  `json:"source"`
	EventType  string  `json:"eventType"`
	PID        uint32  `json:"pid,omitempty"`
	Comm       string  `json:"comm,omitempty"`
	Target     string  `json:"target,omitempty"`
	RiskScore  float64 `json:"riskScore,omitempty"`
	Decision   string  `json:"decision,omitempty"`
	TraceID    string  `json:"traceId,omitempty"`
	Associated string  `json:"associated,omitempty"`
}

type ResearchKernelRiskFeedbackInfo struct {
	Enabled             bool    `json:"enabled"`
	PolicyGateEnabled   bool    `json:"policyGateEnabled"`
	MinRiskScore        float64 `json:"minRiskScore"`
	EnforceNetwork      bool    `json:"enforceNetwork"`
	EnforceFileNames    bool    `json:"enforceFileNames"`
	EnforceExec         bool    `json:"enforceExec"`
	MaxActionsPerMinute int     `json:"maxActionsPerMinute"`
}

type ResearchWindowCompare struct {
	Left   ResearchWindowSummary `json:"left"`
	Right  ResearchWindowSummary `json:"right"`
	Deltas []ResearchCountDelta  `json:"deltas"`
}

type ResearchWindowSummary struct {
	Name      string                    `json:"name"`
	TimeRange ResearchTimeRange         `json:"timeRange"`
	Summary   researchProcessingSummary `json:"summary"`
}

type ResearchCountDelta struct {
	Category string `json:"category"`
	Key      string `json:"key"`
	Left     int    `json:"left"`
	Right    int    `json:"right"`
	Delta    int    `json:"delta"`
}

type ResearchManifest struct {
	SchemaVersion     string                         `json:"schemaVersion"`
	GeneratedAt       time.Time                      `json:"generatedAt"`
	SessionID         string                         `json:"sessionId"`
	SessionName       string                         `json:"sessionName"`
	SourceFilter      ResearchSourceFilter           `json:"sourceFilter,omitempty"`
	TimeRange         ResearchTimeRange              `json:"timeRange,omitempty"`
	RedactionLevel    string                         `json:"redactionLevel"`
	EventCount        int                            `json:"eventCount"`
	Artifacts         map[string]ResearchArtifactRef `json:"artifacts"`
	Hashes            map[string]string              `json:"hashes"`
	ResearchSchema    string                         `json:"researchSchema"`
	ExportedBy        string                         `json:"exportedBy"`
	RetentionDays     int                            `json:"retentionDays"`
	MaxSessionEvents  int                            `json:"maxSessionEvents"`
	ConfiguredFormats []string                       `json:"configuredFormats"`
}

type researchCreateSessionRequest struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Tags         []string             `json:"tags"`
	SourceFilter ResearchSourceFilter `json:"sourceFilter"`
	TimeRange    ResearchTimeRange    `json:"timeRange"`
}

type researchTaskRequest struct {
	Action         string               `json:"action"`
	Limit          int                  `json:"limit"`
	SourceFilter   ResearchSourceFilter `json:"sourceFilter"`
	TimeRange      ResearchTimeRange    `json:"timeRange"`
	LeftWindow     ResearchTimeRange    `json:"leftWindow"`
	RightWindow    ResearchTimeRange    `json:"rightWindow"`
	Formats        []string             `json:"formats"`
	Format         string               `json:"format"`
	TargetTaskID   string               `json:"targetTaskId"`
	EvaluationMode string               `json:"evaluationMode"`
	LabelPolicy    string               `json:"labelPolicy"`
	IncludeLLM     bool                 `json:"includeLLM"`
	Params         map[string]any       `json:"params,omitempty"`
}

type ResearchTask struct {
	TaskID      string         `json:"taskId"`
	SessionID   string         `json:"sessionId,omitempty"`
	Action      string         `json:"action"`
	Status      string         `json:"status"`
	Progress    float64        `json:"progress"`
	QueuedAt    time.Time      `json:"queuedAt"`
	StartedAt   *time.Time     `json:"startedAt,omitempty"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	Error       string         `json:"error,omitempty"`
	ResultRef   string         `json:"resultRef,omitempty"`
	Result      map[string]any `json:"result,omitempty"`
	Records     int            `json:"records,omitempty"`
	QueueLen    int            `json:"queueLen,omitempty"`
	CancelAsked bool           `json:"cancelRequested,omitempty"`
}

type researchTaskManagerStatus struct {
	Runtime      backendTaskRuntimeStats `json:"runtime"`
	TrackedTotal int                     `json:"trackedTotal"`
	ByStatus     map[string]int          `json:"byStatus"`
	UpdatedAt    time.Time               `json:"updatedAt"`
}

type researchTaskEntry struct {
	mu       sync.RWMutex
	task     ResearchTask
	request  researchTaskRequest
	tlsStore *TLSCaptureStore
	runtime  *backendTaskRuntimeEntry
	cancel   <-chan struct{}

	// retention fields are owned by researchTaskManager.mu.
	retentionTracked bool
	retentionNext    *researchTaskEntry
}

type researchSessionStore struct {
	mu       sync.RWMutex
	fsMu     sync.Mutex
	baseDir  string
	loaded   bool
	sessions map[string]*ResearchSession
}

type researchTaskManager struct {
	mu          sync.RWMutex
	runtime     *backendTaskRuntime
	tasks       map[string]*researchTaskEntry
	store       *researchSessionStore
	maxItems    int
	taskHandler func(*researchTaskEntry) error

	terminalHead *researchTaskEntry
	terminalTail *researchTaskEntry
}

var (
	researchSessionsStore = newResearchSessionStore("")
	researchTaskStore     = newResearchTaskManager(researchSessionsStore)
)
