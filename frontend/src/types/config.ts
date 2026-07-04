export interface DomainForwardRoute {
  host: string;
  upstream?: string;
  certFile?: string;
  keyFile?: string;
}

export interface DomainForwardProxySettings {
  enabled: boolean;
  httpPort: number;
  httpsPort: number;
  defaultScheme: "http" | "https";
  allowAnyHost: boolean;
  dnsResolver?: string;
  dialTimeoutSeconds: number;
  certFile?: string;
  keyFile?: string;
  routes: DomainForwardRoute[];
}

export interface KernelRiskFeedbackSettings {
  enabled: boolean;
  minRiskScore: number;
  enforceNetwork: boolean;
  enforceFileNames: boolean;
  enforceExec: boolean;
  maxActionsPerMinute: number;
}

export interface LoopDetectionSettings {
  enabled: boolean;
  windowSeconds: number;
  repeatThreshold: number;
  maxContexts: number;
  queueSize: number;
  emitSemanticAlerts: boolean;
}

export interface LoopDetectionFinding {
  id: string;
  observedAt: string;
  firstSeen: string;
  lastSeen: string;
  contextType: string;
  contextKey: string;
  repeatCount: number;
  windowSeconds: number;
  fingerprint: string;
  target: string;
  eventTypes: string[];
  pids: number[];
  comms: string[];
  paths: string[];
  toolNames: string[];
  agentRunId?: string;
  taskId?: string;
  toolCallId?: string;
  traceId?: string;
  rootAgentPid?: number;
  pid?: number;
  comm?: string;
  reason: string;
  suggestedAction: string;
}

export interface LoopDetectionStatus {
  enabled: boolean;
  settings: LoopDetectionSettings;
  queueLen: number;
  queueCap: number;
  consumedTotal: number;
  findingsTotal: number;
  droppedTotal: number;
  windowCount: number;
  recentFindings: LoopDetectionFinding[];
  lastError?: string;
  updatedAt: string;
}

export interface ResearchProcessingSettings {
  enabled: boolean;
  maxEvents: number;
  queueSize: number;
  timelineBucketSeconds: number;
  topK: number;
  recentSamples: number;
  artifactRetentionDays: number;
  maxSessionEvents: number;
  exportFormats: string;
}

export interface ResearchCount {
  key: string;
  count: number;
}

export interface ResearchTimelineBucket {
  start: number;
  end: number;
  time: string;
  count: number;
}

export interface ResearchProcessSummary {
  pid: number;
  ppid?: number;
  comm: string;
  eventCount: number;
  firstSeen?: number;
  lastSeen?: number;
  sources?: string[];
  eventTypes?: string[];
  childPids?: number[];
}

export interface ResearchTraceSummary {
  traceId: string;
  eventCount: number;
  firstSeen?: number;
  lastSeen?: number;
  sources?: string[];
  comms?: string[];
}

export interface ResearchEventSample {
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
  title: string;
  target?: string;
  riskScore?: number;
  decision?: string;
}

export interface ResearchProcessingSummary {
  total: number;
  earliestTimestamp?: number;
  latestTimestamp?: number;
  earliestTime?: string;
  latestTime?: string;
  bySource: ResearchCount[];
  byType: ResearchCount[];
  byComm: ResearchCount[];
  byPid: ResearchCount[];
  byTrace: ResearchCount[];
  timeline: ResearchTimelineBucket[];
  topProcesses: ResearchProcessSummary[];
  topTraces: ResearchTraceSummary[];
  recentSamples: ResearchEventSample[];
  generatedTimestamp: number;
  generatedTime: string;
}

export interface ResearchProcessingStatus {
  enabled: boolean;
  settings: ResearchProcessingSettings;
  queueLen: number;
  queueCap: number;
  consumedTotal: number;
  droppedTotal: number;
  bufferedTotal: number;
  lastError?: string;
  updatedAt: string;
  summary: ResearchProcessingSummary;
}

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
}

export interface ResearchSecurityEvaluationFindings {
  falsePositives?: ResearchSecurityEvaluationSampleRow[];
  falseNegatives?: ResearchSecurityEvaluationSampleRow[];
  policyGaps?: ResearchSecurityEvaluationSampleRow[];
  highConfidenceDisagreements?: ResearchSecurityEvaluationSampleRow[];
  unlabeledHighRisk?: ResearchSecurityEvaluationSampleRow[];
}

export interface ResearchSecurityEvaluationReport {
  schemaVersion: string;
  sessionId: string;
  generatedAt: string;
  mode: string;
  labelPolicy: string;
  includeLLM: boolean;
  totals: ResearchSecurityEvaluationTotals;
  metrics: ResearchSecurityEvaluationMetrics;
  confusionMatrix: Record<string, Record<string, number>>;
  byCategory?: ResearchSecurityEvaluationGroup[];
  byCommand?: ResearchSecurityEvaluationGroup[];
  bySource?: ResearchSecurityEvaluationGroup[];
  riskBuckets?: ResearchCount[];
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

export interface RuntimeSettings {
  logPersistenceEnabled: boolean;
  logFilePath: string;
  accessToken: string;
  maxEventCount: number;
  maxEventAge: string;
  shellSessionsEnabled: boolean;
  systemRunEnabled: boolean;
  hookManagementEnabled: boolean;
  policyManagementEnabled: boolean;
  otlpEnabled: boolean;
  otlpEndpoint: string;
  otlpServiceName: string;
  otlpHeaders: Record<string, string>;
  tlsCaptureEnabled: boolean;
  kernelRiskFeedback: KernelRiskFeedbackSettings;
  loopDetection: LoopDetectionSettings;
  researchProcessing: ResearchProcessingSettings;
  domainForwardProxy: DomainForwardProxySettings;
  mlConfig?: {
    enabled?: boolean;
    blockConfidenceThreshold?: number;
    mlMinConfidence?: number;
    ruleOverridePriority?: number;
    lowAnomalyThreshold?: number;
    highAnomalyThreshold?: number;
    modelPath?: string;
    autoTrain?: boolean;
    trainInterval?: string;
    minSamplesForTraining?: number;
    activeLearningEnabled?: boolean;
    featureHistorySize?: number;
    numTrees?: number;
    maxDepth?: number;
    minSamplesLeaf?: number;
    validationSplitRatio?: number;
    balanceClasses?: boolean;
    llmEnabled?: boolean;
    llmBaseUrl?: string;
    llmApiKeyConfigured?: boolean;
    llmModel?: string;
    llmTimeoutSeconds?: number;
    llmTemperature?: number;
    llmMaxTokens?: number;
    modelType?: string;
    llmSystemPrompt?: string;
  };
}

export interface TrackedItem {
  comm?: string;
  path?: string;
  prefix?: string;
  tag: string;
  disabled?: boolean;
}

export interface WrapperRule {
  comm: string;
  action: string;
  rewritten_cmd: string[];
  regex?: string;
  replacement?: string;
  priority?: number;
}

export interface ClusterNodeInfo {
  id: string;
  name: string;
  url: string;
  role: "master" | "slave";
  status: string;
  lastSeen: string;
  isLocal: boolean;
  version?: string;
}

export interface ClusterStateResponse {
  role: "master" | "slave";
  masterUrl: string;
  nodeUrl: string;
  nodeId: string;
  nodeName: string;
  accountConfigured: boolean;
  passwordConfigured: boolean;
  localNode: ClusterNodeInfo;
}

export interface RuntimeConfigResponse {
  runtime: RuntimeSettings;
  mcpEndpoint: string;
  authHeaderName: string;
  bearerAuthHeaderName: string;
  persistedEventLogPath: string;
  persistedEventLogAlive: boolean;
}

export interface CollectorHealthResponse {
  collectorMapAvailable: boolean;
  ringbufEventsTotal: number;
  ringbufDroppedTotal: number;
  ringbufReserveFailedTotal: number;
  ringbufZeroCopyDecodeTotal: number;
  ringbufCopyDecodeTotal: number;
  eventsByTypeTotal: Record<string, number>;
  eventsByPidTotal?: Record<string, number>;
  agentSightCountersTotal?: Record<string, number>;
  backendQueueLen: number;
  wsClients: number;
  persistAppendLatencyNs: number;
  kernelRiskEvaluationsTotal: number;
  kernelRiskAlertsTotal: number;
  kernelRiskBlocksTotal: number;
  kernelRiskLastEvalLatencyNs: number;
  kernelRiskFeedbackApplied: number;
  kernelRiskFeedbackDropped: number;
  kernelRiskFeedbackLastError?: string;
  captureHealthy: boolean;
}

export interface TracepointBootstrapStatus {
  kernelRelease: string;
  compiledCount: number;
  attachedCount: number;
  skippedCount: number;
  skippedTracepoints: string[];
  status: "unknown" | "ready" | "partial" | "error";
  message: string;
  observedAt?: string;
}

export interface OTelHealthResponse {
  enabled: boolean;
  ready: boolean;
  endpoint: string;
  serviceName: string;
  queueLen: number;
  activeRunSpans: number;
  activeTaskSpans: number;
  activeToolSpans: number;
  exportedSpans: number;
  droppedEvents: number;
  lastExportedAt?: string;
  lastError?: string;
}

export interface DomainForwardProxyStatus {
  enabled: boolean;
  httpRunning: boolean;
  httpsRunning: boolean;
  httpAddress?: string;
  httpsAddress?: string;
  httpPort: number;
  httpsPort: number;
  routeCount: number;
  allowAnyHost: boolean;
  dnsResolver?: string;
  errors?: string[];
  updatedAt: string;
}

export interface MLReviewSummary {
  source: string;
  model: string;
  scoredSamples: number;
  averageRiskScore: number;
  agreement: number;
  validationSplitRatio?: number;
  reviewedAt: string;
}

export interface MLBuiltinModelDefaults {
  numTrees?: number;
  maxDepth?: number;
  minSamplesLeaf?: number;
}

export interface MLBuiltinModelCatalogItem {
  value: string;
  label: string;
  base: string;
  category: string;
  description: string;
  recommended?: boolean;
  defaults?: MLBuiltinModelDefaults;
  tags?: string[];
}

export interface MLCRuntimeBackend {
  id: string;
  label: string;
  available: boolean;
  accelerated: boolean;
  detail?: string;
}

export interface MLCRuntimeStatus {
  available: boolean;
  activeBackend: string;
  benchmarkBackend: string;
  backends: MLCRuntimeBackend[];
  modelType?: string;
  cSupported: boolean;
  sampleCount: number;
  goMsPerSample?: number;
  cMsPerSample?: number;
  speedup?: number;
  updatedAt?: string;
  note?: string;
}

export interface MLStatusState {
  model_type?: string;
  model_loaded: boolean;
  num_trees: number;
  num_samples: number;
  num_labeled_samples: number;
  last_trained: string;
  test_accuracy: number;
  model_path: string;
  training_in_progress: boolean;
  training_progress: number;
  train_accuracy: number;
  validation_accuracy: number;
  train_samples: number;
  validation_samples: number;
  validation_split_ratio: number;
  llm_review: MLReviewSummary | null;
}

export interface MLLlmConfig {
  enabled: boolean;
  baseUrl: string;
  apiKey: string;
  apiKeyConfigured: boolean;
  model: string;
  timeoutSeconds: number;
  temperature: number;
  maxTokens: number;
  systemPrompt: string;
}

export interface MLLlmAssessment {
  enabled: boolean;
  model?: string;
  riskScore: number;
  confidence: number;
  recommendedAction: string;
  reasoning: string;
  signals?: string[];
  error?: string;
  rawContent?: string;
}

export interface MLLlmBatchEntry {
  index?: number;
  commandLine: string;
  comm: string;
  args: string[];
  currentLabel?: string;
  riskScore: number;
  confidence: number;
  recommendedAction: string;
  reasoning: string;
  applied?: boolean;
  error?: string;
}

export interface MLLlmBatchResponse {
  source: string;
  model: string;
  total: number;
  scored: number;
  applied: number;
  skipped: number;
  averageRiskScore: number;
  agreement: number;
  validationSplitRatio?: number;
  review?: MLReviewSummary | null;
  entries: MLLlmBatchEntry[];
}

export interface MLTrainingHistoryEntry {
  timestamp: string;
  accuracy: number;
  trainAccuracy?: number;
  validationAccuracy?: number;
  numTrees: number;
  numSamples: number;
  trainSamples?: number;
  validationSamples?: number;
  validationSplitRatio?: number;
  llmScoredSamples?: number;
  llmAverageRiskScore?: number;
  llmAgreement?: number;
  duration?: number;
}

export type MLAutoTuneAxis = string;
export type MLAutoTuneMetric =
  | "validationAccuracy"
  | "balancedAccuracy"
  | "allowRecall"
  | "inferenceThroughput";
export type MLAutoTuneGranularity = 1 | 2 | 4;

export interface MLAutoTuneCell {
  xIndex: number;
  yIndex: number;
  xValue: number;
  yValue: number;
  numTrees: number;
  maxDepth: number;
  minSamplesLeaf: number;
  trainAccuracy: number;
  validationAccuracy: number;
  allowRecall?: number;
  balancedAccuracy?: number;
  inferenceThroughput: number;
  inferenceMsPerSample: number;
  trainDuration: number;
  evalDuration: number;
  score: number;
}

export interface FeatureNormalizationReport {
  mode: string;
  sampleCount: number;
  featureDim: number;
  minObserved: number;
  maxObserved: number;
  nonFiniteValues: number;
  belowZeroValues: number;
  aboveOneValues: number;
  zeroVarianceFeatures: number;
  normalizedFeatureHint?: string;
}

export interface MLAutoTuneResponse {
  xAxis: MLAutoTuneAxis;
  yAxis: MLAutoTuneAxis;
  metric: MLAutoTuneMetric;
  granularity: MLAutoTuneGranularity;
  gridSize: number;
  xValues: number[];
  yValues: number[];
  sampleCount: number;
  validationCount: number;
  totalDuration: number;
  normalization?: FeatureNormalizationReport;
  cells: MLAutoTuneCell[];
  best: MLAutoTuneCell | null;
}

export type MLAutoTuneMode = "params" | "models";

export interface MLModelTuneCandidate {
  modelType: string;
  label: string;
  base: string;
  recommended?: boolean;
  hyperParams: {
    numTrees?: number;
    maxDepth?: number;
    minSamplesLeaf?: number;
  };
  trainAccuracy: number;
  validationAccuracy: number;
  allowRecall?: number;
  balancedAccuracy?: number;
  inferenceThroughput: number;
  inferenceMsPerSample: number;
  trainDuration: number;
  evalDuration: number;
  score: number;
  sampleCount: number;
  validationCount: number;
  paramTune?: MLAutoTuneResponse | null;
  applied?: boolean;
  error?: string;
}

export interface MLModelTuneResponse {
  metric: MLAutoTuneMetric;
  sampleCount: number;
  validationCount: number;
  totalDuration: number;
  candidates: MLModelTuneCandidate[];
  best: MLModelTuneCandidate | null;
}

export interface SampleEntry {
  index: number;
  commandLine?: string;
  comm: string;
  args: string[];
  label: string;
  category: string;
  anomalyScore: number;
  timestamp: string;
  userLabel: string;
}

export interface ExistingCommandCandidate {
  commandLine: string;
  comm: string;
  args: string[];
  eventType: string;
  source: string;
  category: string;
  timestamp: string;
  duplicate: boolean;
}

export interface RemoteDatasetRow {
  row: number;
  source?: string;
  commandLine: string;
  comm: string;
  args: string[];
  label: string;
  labelSource: string;
  category: string;
  anomalyScore: number;
  timestamp: string;
  userLabel: string;
  duplicate: boolean;
}

export interface RemoteDatasetResponse {
  source: string;
  format: string;
  contentType: string;
  total: number;
  limit: number;
  truncated: boolean;
  imported?: number;
  skipped?: number;
  totalSamples?: number;
  labeledSamples?: number;
  rows?: RemoteDatasetRow[];
  families?: Record<string, number>;
  normalization?: FeatureNormalizationReport;
}

export type ResearchTrainingLabelPolicy =
  | "decision"
  | "heuristic"
  | "unlabeled";

export interface ResearchTrainingSample {
  sampleId: string;
  eventId: string;
  timestamp: number;
  time: string;
  source: string;
  eventType: string;
  pid?: number;
  comm: string;
  commandLine: string;
  args: string[];
  category: string;
  target?: string;
  traceId?: string;
  spanId?: string;
  decision?: string;
  riskScore?: number;
  label: number;
  labelName: string;
  labelSource: string;
  anomalyScore: number;
  featureVector: number[];
  featureSpace: string;
  featureVersion: string;
  metadata?: Record<string, unknown>;
}

export interface ResearchTrainingDataset {
  schemaVersion: string;
  sessionId: string;
  generatedAt: string;
  labelPolicy: ResearchTrainingLabelPolicy;
  featureDim: number;
  featureNames: string[];
  sampleCount: number;
  labeledCount: number;
  byLabel: ResearchCount[];
  normalization: FeatureNormalizationReport;
  samples?: ResearchTrainingSample[];
}

export interface ResearchTrainingImportResponse {
  sessionId: string;
  labelPolicy: ResearchTrainingLabelPolicy;
  total: number;
  imported: number;
  skipped: number;
  totalSamples: number;
  labeledSamples: number;
  normalization: FeatureNormalizationReport;
  importedSamples?: ResearchTrainingSample[];
}

export interface LLMProductionDatasetMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

export interface LLMProductionDatasetRow {
  index: number;
  commandLine: string;
  comm: string;
  args: string[];
  label: string;
  category: string;
  anomalyScore: number;
  timestamp: string;
  userLabel: string;
  targetRiskScore: number;
  targetConfidence: number;
  reasoning: string;
  signals: string[];
  prompt: string;
  completion: string;
  messages: LLMProductionDatasetMessage[];
}

export interface LLMProductionDatasetResponse {
  source: string;
  format: string;
  contentType: string;
  total: number;
  limit: number;
  truncated: boolean;
  included: number;
  skippedUnlabeled: number;
  skippedHeuristic: number;
  skippedDuplicates: number;
  systemPrompt: string;
  rows?: LLMProductionDatasetRow[];
}

export interface ClassicSecurityDatasetPreset {
  name: string;
  family: string;
  platform: string;
  pageUrl: string;
  downloadUrl?: string;
  format?: "auto" | "json" | "jsonl" | "csv" | "tsv" | "text";
  labelMode?: "preserve" | "unlabeled" | "heuristic" | "block";
  note: string;
}

export interface MLCommandSafetyResult {
  riskScore?: number;
  riskLevel?: string;
  commandLine?: string;
  comm?: string;
  args?: string[];
  recommendedAction?: string;
  classification?: any;
  anomalyScore?: number;
  mlPrediction?: { action?: string; confidence?: number };
  reasoning?: string;
  sampleEvidence?: any;
  sampleMatches?: any[];
  networkAudit?: any;
  llmAssessment?: MLLlmAssessment;
}

export type SecurityRuleAction = "BLOCK" | "ALERT";

export interface SecurityRulePreset {
  comm: string;
  action: SecurityRuleAction;
  priority: number;
  source: string;
  summary: string;
}

export interface ExternalRuleSource {
  id: string;
  name: string;
  description: string;
  url: string;
  format: "json" | "yaml" | "markdown";
  sourceAttribution: string;
  category: "agent-security" | "community" | "owasp";
}

export interface SyscallDef {
  type: number;
  name: string;
  desc: string;
}

export interface SyscallGroup {
  key: string;
  title: string;
  icon: string;
  color: string;
  syscalls: SyscallDef[];
}
