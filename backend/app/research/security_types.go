package research

import "time"

const (
	researchSecurityEvaluationSchemaVersion = "research-security-evaluation.v1"
	researchSecurityEvaluationDefaultMode   = "combined"
	researchSecurityEvaluationModeBuiltin   = "builtin"
	researchSecurityEvaluationModeSession   = "session"
	researchSecurityEvaluationModeCombined  = "combined"
	researchSecurityEvaluationLabelDecision = "decision_then_heuristic"
	researchSecurityEvaluationMaxFindings   = 200

	researchSecurityValidationModePrediction = "prediction"
	researchSecurityValidationModeOutcome    = "outcome"
	researchSecurityEvidenceHypothesis       = "hypothesis"
	researchSecurityEvidenceReachable        = "reachable"
	researchSecurityEvidenceReproduced       = "reproduced"
	researchSecurityEvidenceImpactConfirmed  = "impact_confirmed"

	researchSecurityOutcomeDefaultCorrelationWindowSeconds = 30
	researchSecurityOutcomeMaxCorrelationWindowSeconds     = 300
)

type ResearchSecurityEvaluationRequest struct {
	Mode                         string               `json:"mode,omitempty"`
	LabelPolicy                  string               `json:"labelPolicy,omitempty"`
	Limit                        int                  `json:"limit,omitempty"`
	IncludeLLM                   bool                 `json:"includeLLM,omitempty"`
	ValidationMode               string               `json:"validationMode,omitempty"`
	MinimumEvidence              string               `json:"minimumEvidence,omitempty"`
	AdversarialReview            bool                 `json:"adversarialReview,omitempty"`
	RequireAuthorization         bool                 `json:"requireAuthorization,omitempty"`
	RequireIndependentRefutation bool                 `json:"requireIndependentRefutation,omitempty"`
	DedupeActionable             bool                 `json:"dedupeActionable,omitempty"`
	CorrelationWindowSeconds     int                  `json:"correlationWindowSeconds,omitempty"`
	AllowedValidatorSources      []string             `json:"allowedValidatorSources,omitempty"`
	AllowedAuthorizationIDs      []string             `json:"allowedAuthorizationIds,omitempty"`
	AllowedTargets               []string             `json:"allowedTargets,omitempty"`
	SourceFilter                 ResearchSourceFilter `json:"sourceFilter,omitempty"`
	TimeRange                    ResearchTimeRange    `json:"timeRange,omitempty"`
}

type ResearchSecurityEvaluationReport struct {
	SchemaVersion     string                                    `json:"schemaVersion"`
	SessionID         string                                    `json:"sessionId"`
	GeneratedAt       time.Time                                 `json:"generatedAt"`
	Mode              string                                    `json:"mode"`
	LabelPolicy       string                                    `json:"labelPolicy"`
	IncludeLLM        bool                                      `json:"includeLLM"`
	ValidationMode    string                                    `json:"validationMode,omitempty"`
	OutcomeValidation *ResearchSecurityOutcomeValidationSummary `json:"outcomeValidation,omitempty"`
	Totals            ResearchSecurityEvaluationTotals          `json:"totals"`
	Metrics           ResearchSecurityEvaluationMetrics         `json:"metrics"`
	ConfusionMatrix   map[string]map[string]int                 `json:"confusionMatrix"`
	ByCategory        []ResearchSecurityEvaluationGroup         `json:"byCategory"`
	ByCommand         []ResearchSecurityEvaluationGroup         `json:"byCommand"`
	BySource          []ResearchSecurityEvaluationGroup         `json:"bySource"`
	RiskBuckets       []researchCount                           `json:"riskBuckets"`
	Posture           ResearchSecurityEvaluationPosture         `json:"posture"`
	Findings          ResearchSecurityEvaluationFindings        `json:"findings"`
	Samples           []ResearchSecurityEvaluationSampleRow     `json:"samples,omitempty"`
}

type ResearchSecurityOutcomeValidationSummary struct {
	Enabled                      bool                                  `json:"enabled"`
	MinimumEvidence              string                                `json:"minimumEvidence"`
	AdversarialReview            bool                                  `json:"adversarialReview"`
	RequireAuthorization         bool                                  `json:"requireAuthorization"`
	RequireIndependentRefutation bool                                  `json:"requireIndependentRefutation"`
	DedupeActionable             bool                                  `json:"dedupeActionable"`
	CorrelationWindowSeconds     int                                   `json:"correlationWindowSeconds"`
	AllowedValidatorSources      []string                              `json:"allowedValidatorSources,omitempty"`
	AllowedAuthorizationIDs      []string                              `json:"allowedAuthorizationIds,omitempty"`
	AllowedTargets               []string                              `json:"allowedTargets,omitempty"`
	Candidates                   int                                   `json:"candidates"`
	NotApplicable                int                                   `json:"notApplicable"`
	OutOfScope                   int                                   `json:"outOfScope"`
	Unproven                     int                                   `json:"unproven"`
	Reachable                    int                                   `json:"reachable"`
	Reproduced                   int                                   `json:"reproduced"`
	ImpactConfirmed              int                                   `json:"impactConfirmed"`
	Rejected                     int                                   `json:"rejected"`
	Conflicted                   int                                   `json:"conflicted"`
	UnauthorizedEvidence         int                                   `json:"unauthorizedEvidence"`
	NonIndependentRefutations    int                                   `json:"nonIndependentRefutations"`
	Actionable                   int                                   `json:"actionable"`
	UniqueActionable             int                                   `json:"uniqueActionable"`
	DuplicateActionable          int                                   `json:"duplicateActionable"`
	Findings                     []ResearchSecurityEvaluationSampleRow `json:"findings,omitempty"`
}

type ResearchSecurityOutcomeEvidence struct {
	Level           string `json:"level"`
	Kind            string `json:"kind"`
	EventID         string `json:"eventId,omitempty"`
	Source          string `json:"source,omitempty"`
	Detail          string `json:"detail,omitempty"`
	Correlation     string `json:"correlation,omitempty"`
	ValidatorID     string `json:"validatorId,omitempty"`
	AuthorizationID string `json:"authorizationId,omitempty"`
	RunID           string `json:"runId,omitempty"`
	Authorized      bool   `json:"authorized"`
}

type ResearchSecurityEvaluationTotals struct {
	Total     int `json:"total"`
	Labeled   int `json:"labeled"`
	Benign    int `json:"benign"`
	Risky     int `json:"risky"`
	Unlabeled int `json:"unlabeled"`
	Skipped   int `json:"skipped"`
	Builtin   int `json:"builtin"`
	Session   int `json:"session"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
}

type ResearchSecurityEvaluationMetrics struct {
	Accuracy          float64 `json:"accuracy"`
	Precision         float64 `json:"precision"`
	Recall            float64 `json:"recall"`
	AllowRecall       float64 `json:"allowRecall"`
	AlertRecall       float64 `json:"alertRecall"`
	BlockRecall       float64 `json:"blockRecall"`
	FalsePositiveRate float64 `json:"falsePositiveRate"`
	FalseNegativeRate float64 `json:"falseNegativeRate"`
	BalancedAccuracy  float64 `json:"balancedAccuracy"`
}

type ResearchSecurityEvaluationGroup struct {
	Key            string  `json:"key"`
	Total          int     `json:"total"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	FalsePositives int     `json:"falsePositives"`
	FalseNegatives int     `json:"falseNegatives"`
	AvgRiskScore   float64 `json:"avgRiskScore"`
}

type ResearchSecurityEvaluationPosture struct {
	Status               string                            `json:"status"`
	RiskScore            float64                           `json:"riskScore"`
	FindingCounts        []researchCount                   `json:"findingCounts,omitempty"`
	BlockingReasons      []string                          `json:"blockingReasons,omitempty"`
	Warnings             []string                          `json:"warnings,omitempty"`
	SuggestedActions     []string                          `json:"suggestedActions,omitempty"`
	RemediationPlan      []ResearchSecurityRemediationItem `json:"remediationPlan,omitempty"`
	TopFailingCategories []ResearchSecurityEvaluationGroup `json:"topFailingCategories,omitempty"`
}

type ResearchSecurityRemediationItem struct {
	ID              string   `json:"id"`
	Priority        string   `json:"priority"`
	Area            string   `json:"area"`
	FindingType     string   `json:"findingType,omitempty"`
	Category        string   `json:"category,omitempty"`
	Action          string   `json:"action"`
	Rationale       string   `json:"rationale"`
	Count           int      `json:"count"`
	RelatedCommands []string `json:"relatedCommands,omitempty"`
}

type ResearchSecurityEvaluationFindings struct {
	FalsePositives              []ResearchSecurityEvaluationSampleRow `json:"falsePositives,omitempty"`
	FalseNegatives              []ResearchSecurityEvaluationSampleRow `json:"falseNegatives,omitempty"`
	PolicyGaps                  []ResearchSecurityEvaluationSampleRow `json:"policyGaps,omitempty"`
	HighConfidenceDisagreements []ResearchSecurityEvaluationSampleRow `json:"highConfidenceDisagreements,omitempty"`
	UnlabeledHighRisk           []ResearchSecurityEvaluationSampleRow `json:"unlabeledHighRisk,omitempty"`
}

type ResearchSecurityEvaluationSampleRow struct {
	ID               string                            `json:"id"`
	EventID          string                            `json:"eventId,omitempty"`
	Timestamp        int64                             `json:"timestamp,omitempty"`
	Time             string                            `json:"time,omitempty"`
	Source           string                            `json:"source"`
	EventType        string                            `json:"eventType,omitempty"`
	Category         string                            `json:"category,omitempty"`
	Comm             string                            `json:"comm"`
	CommandLine      string                            `json:"commandLine"`
	Args             []string                          `json:"args,omitempty"`
	Target           string                            `json:"target,omitempty"`
	ExpectedAction   string                            `json:"expectedAction"`
	ExpectedSource   string                            `json:"expectedSource"`
	ObservedAction   string                            `json:"observedAction"`
	Passed           bool                              `json:"passed"`
	FindingType      string                            `json:"findingType,omitempty"`
	RiskScore        float64                           `json:"riskScore"`
	RiskLevel        string                            `json:"riskLevel,omitempty"`
	Confidence       float64                           `json:"confidence,omitempty"`
	Reasoning        string                            `json:"reasoning,omitempty"`
	Recommendation   string                            `json:"recommendation,omitempty"`
	RedactionLevel   string                            `json:"redactionLevel,omitempty"`
	TraceID          string                            `json:"traceId,omitempty"`
	SpanID           string                            `json:"spanId,omitempty"`
	Signals          map[string]any                    `json:"signals,omitempty"`
	BenchmarkCase    string                            `json:"benchmarkCase,omitempty"`
	BenchmarkTool    string                            `json:"benchmarkTool,omitempty"`
	BenchmarkDetail  string                            `json:"benchmarkDetail,omitempty"`
	ValidationStatus string                            `json:"validationStatus,omitempty"`
	EvidenceLevel    string                            `json:"evidenceLevel,omitempty"`
	FindingKey       string                            `json:"findingKey,omitempty"`
	Reachable        bool                              `json:"reachable,omitempty"`
	Reproduced       bool                              `json:"reproduced,omitempty"`
	ImpactConfirmed  bool                              `json:"impactConfirmed,omitempty"`
	EvidenceConflict bool                              `json:"evidenceConflict,omitempty"`
	Actionable       bool                              `json:"actionable,omitempty"`
	ValidatorReason  string                            `json:"validatorReason,omitempty"`
	Evidence         []ResearchSecurityOutcomeEvidence `json:"evidence,omitempty"`
}

type researchSecurityEvaluationCandidate struct {
	ID              string
	EventID         string
	Timestamp       int64
	Time            string
	Source          string
	EventType       string
	Category        string
	Comm            string
	CommandLine     string
	Args            []string
	Target          string
	ExpectedAction  string
	ExpectedSource  string
	RedactionLevel  string
	TraceID         string
	SpanID          string
	BenchmarkCase   string
	BenchmarkTool   string
	BenchmarkDetail string
}

type researchSecurityEvaluationAccumulator struct {
	total          int
	passed         int
	falsePositives int
	falseNegatives int
	riskSum        float64
}

type researchSecurityEvaluationCounters struct {
	tp               int
	tn               int
	fp               int
	fn               int
	expectedAllow    int
	expectedRisky    int
	expectedAlert    int
	alertMatched     int
	expectedBlock    int
	blockMatched     int
	strictCorrect    int
	labeledEvaluated int
}
