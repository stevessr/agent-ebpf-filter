import type { LoopDetectionFinding } from "./loop-detection";
import type { ResearchCount, ResearchProcessingSummary } from "./research-processing";

export interface ResearchSessionSummary {
  schemaVersion?: string;
  eventCount: number;
  earliestTimestamp?: number;
  latestTimestamp?: number;
  earliestTime?: string;
  latestTime?: string;
  topSource?: string;
  topEventType?: string;
  topComm?: string;
  maxRiskScore?: number;
  riskAlerts?: number;
  loopFindings?: number;
  generatedTimestamp?: number;
  generatedTime?: string;
}
export interface ResearchSession {
  id: string;
  name: string;
  description?: string;
  tags?: string[];
  createdAt: string;
  updatedAt: string;
  status: string;
  summary: ResearchSessionSummary;
  artifactRefs?: Record<string, ResearchArtifactRef>;
  lastError?: string;
}
export interface ResearchSessionListResponse {
  sessions: ResearchSession[];
}
export interface ResearchSourceFilter {
  sources?: string[];
  eventTypes?: string[];
  comms?: string[];
  pids?: number[];
  traceId?: string;
  spanId?: string;
  query?: string;
  limit?: number;
  includeTLS?: boolean;
  includeUploaded?: boolean;
}
export interface ResearchTimeRange {
  since?: number;
  until?: number;
  sinceTime?: string;
  untilTime?: string;
}
export interface ResearchCreateSessionRequest {
  name: string;
  description?: string;
  tags?: string[];
  sourceFilter?: ResearchSourceFilter;
  timeRange?: ResearchTimeRange;
}
export interface ResearchTaskRequest {
  action:
    | "scan_recent"
    | "build_session"
    | "compare_windows"
    | "security_eval"
    | "export_bundle"
    | "reset_session";
  limit?: number;
  sourceFilter?: ResearchSourceFilter;
  timeRange?: ResearchTimeRange;
  leftWindow?: ResearchTimeRange;
  rightWindow?: ResearchTimeRange;
  formats?: string[];
  format?: string;
  targetTaskId?: string;
  evaluationMode?: "combined" | "builtin" | "session" | string;
  labelPolicy?: "decision_then_heuristic" | "decision" | "heuristic" | "unlabeled" | string;
  includeLLM?: boolean;
  params?: Record<string, unknown>;
}
export interface ResearchTask {
  taskId: string;
  sessionId?: string;
  action: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled" | string;
  progress: number;
  queuedAt: string;
  startedAt?: string;
  finishedAt?: string;
  error?: string;
  resultRef?: string;
  result?: Record<string, unknown>;
  records?: number;
  queueLen?: number;
  cancelRequested?: boolean;
}
export interface ResearchArtifactRef {
  format: string;
  name: string;
  path?: string;
  contentType: string;
  bytes: number;
  sha256: string;
  createdAt: string;
}
export interface ResearchEvent {
  id: string;
  timestamp: number;
  time: string;
  source: string;
  eventType: string;
  pid?: number;
  ppid?: number;
  comm?: string;
  traceId?: string;
  spanId?: string;
  target?: string;
  riskScore?: number;
  decision?: string;
  redactionLevel?: string;
  features?: Record<string, unknown>;
}
export interface ResearchEventsResponse {
  events: ResearchEvent[];
  total: number;
  offset: number;
  limit: number;
}
export interface ResearchRiskFinding {
  eventId: string;
  timestamp: number;
  time: string;
  source: string;
  eventType: string;
  pid?: number;
  comm?: string;
  target?: string;
  riskScore?: number;
  decision?: string;
  traceId?: string;
  associated?: string;
}
export interface ResearchSecurityEvaluationTotals {
  total: number;
  labeled: number;
  benign: number;
  risky: number;
  unlabeled: number;
  skipped: number;
  builtin: number;
  session: number;
  passed: number;
  failed: number;
}
export interface ResearchSecurityEvaluationMetrics {
  accuracy: number;
  precision: number;
  recall: number;
  allowRecall: number;
  alertRecall: number;
  blockRecall: number;
  falsePositiveRate: number;
  falseNegativeRate: number;
  balancedAccuracy: number;
}
export interface ResearchSecurityEvaluationGroup {
  key: string;
  total: number;
  passed: number;
  failed: number;
  falsePositives: number;
  falseNegatives: number;
  avgRiskScore: number;
}
export interface ResearchSecurityOutcomeEvidence {
  level: "hypothesis" | "reachable" | "reproduced" | "impact_confirmed" | string;
  kind: string;
  eventId?: string;
  source?: string;
  detail?: string;
  correlation?: string;
  validatorId?: string;
  authorizationId?: string;
  runId?: string;
  authorized: boolean;
}
export interface ResearchSecurityEvaluationSampleRow {
  id: string;
  eventId?: string;
  timestamp?: number;
  time?: string;
  source: string;
  eventType?: string;
  category?: string;
  comm: string;
  commandLine: string;
  args?: string[];
  target?: string;
  expectedAction: string;
  expectedSource: string;
  observedAction: string;
  passed: boolean;
  findingType?: string;
  riskScore: number;
  riskLevel?: string;
  confidence?: number;
  reasoning?: string;
  recommendation?: string;
  redactionLevel?: string;
  traceId?: string;
  spanId?: string;
  signals?: Record<string, unknown>;
  benchmarkCase?: string;
  benchmarkTool?: string;
  benchmarkDetail?: string;
  validationStatus?: string;
  evidenceLevel?: "hypothesis" | "reachable" | "reproduced" | "impact_confirmed" | string;
  findingKey?: string;
  reachable?: boolean;
  reproduced?: boolean;
  impactConfirmed?: boolean;
  evidenceConflict?: boolean;
  actionable?: boolean;
  validatorReason?: string;
  evidence?: ResearchSecurityOutcomeEvidence[];
}
export interface ResearchSecurityEvaluationFindings {
  falsePositives?: ResearchSecurityEvaluationSampleRow[];
  falseNegatives?: ResearchSecurityEvaluationSampleRow[];
  policyGaps?: ResearchSecurityEvaluationSampleRow[];
  highConfidenceDisagreements?: ResearchSecurityEvaluationSampleRow[];
  unlabeledHighRisk?: ResearchSecurityEvaluationSampleRow[];
}
export interface ResearchSecurityRemediationItem {
  id: string;
  priority: string;
  area: string;
  findingType?: string;
  category?: string;
  action: string;
  rationale: string;
  count: number;
  relatedCommands?: string[];
}
export interface ResearchSecurityEvaluationPosture {
  status: "pass" | "needs_review" | "critical" | string;
  riskScore: number;
  findingCounts?: ResearchCount[];
  blockingReasons?: string[];
  warnings?: string[];
  suggestedActions?: string[];
  remediationPlan?: ResearchSecurityRemediationItem[];
  topFailingCategories?: ResearchSecurityEvaluationGroup[];
}
export interface ResearchSecurityOutcomeValidationSummary {
  enabled: boolean;
  minimumEvidence: "hypothesis" | "reachable" | "reproduced" | "impact_confirmed" | string;
  adversarialReview: boolean;
  requireAuthorization: boolean;
  requireIndependentRefutation: boolean;
  dedupeActionable: boolean;
  correlationWindowSeconds: number;
  allowedValidatorSources?: string[];
  allowedAuthorizationIds?: string[];
  allowedTargets?: string[];
  candidates: number;
  notApplicable: number;
  outOfScope: number;
  unproven: number;
  reachable: number;
  reproduced: number;
  impactConfirmed: number;
  rejected: number;
  conflicted: number;
  unauthorizedEvidence: number;
  nonIndependentRefutations: number;
  actionable: number;
  uniqueActionable: number;
  duplicateActionable: number;
  findings?: ResearchSecurityEvaluationSampleRow[];
}
export interface ResearchSecurityEvaluationReport {
  schemaVersion: string;
  sessionId: string;
  generatedAt: string;
  mode: string;
  labelPolicy: string;
  includeLLM: boolean;
  validationMode?: "prediction" | "outcome" | string;
  outcomeValidation?: ResearchSecurityOutcomeValidationSummary;
  totals: ResearchSecurityEvaluationTotals;
  metrics: ResearchSecurityEvaluationMetrics;
  confusionMatrix: Record<string, Record<string, number>>;
  byCategory?: ResearchSecurityEvaluationGroup[];
  byCommand?: ResearchSecurityEvaluationGroup[];
  bySource?: ResearchSecurityEvaluationGroup[];
  riskBuckets?: ResearchCount[];
  posture?: ResearchSecurityEvaluationPosture;
  findings?: ResearchSecurityEvaluationFindings;
  samples?: ResearchSecurityEvaluationSampleRow[];
}
export interface ResearchResults {
  schemaVersion: string;
  sessionId: string;
  generatedTimestamp: number;
  generatedTime: string;
  summary: ResearchProcessingSummary;
  topTargets?: ResearchCount[];
  topDecisions?: ResearchCount[];
  loopFindings?: LoopDetectionFinding[];
  riskAlerts?: ResearchRiskFinding[];
  kernelRiskFeedback?: {
    enabled: boolean;
    policyGateEnabled: boolean;
    minRiskScore: number;
    enforceNetwork: boolean;
    enforceFileNames: boolean;
    enforceExec: boolean;
    maxActionsPerMinute: number;
  };
  compareWindows?: unknown;
  securityEvaluation?: ResearchSecurityEvaluationReport;
}
