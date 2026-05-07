export interface RuntimeSettings {
  logPersistenceEnabled: boolean;
  logFilePath: string;
  accessToken: string;
  maxEventCount: number;
  maxEventAge: string;
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
export type MLAutoTuneMetric = 'validationAccuracy' | 'inferenceThroughput';
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
  inferenceThroughput: number;
  inferenceMsPerSample: number;
  trainDuration: number;
  evalDuration: number;
  score: number;
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
  cells: MLAutoTuneCell[];
  best: MLAutoTuneCell | null;
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
}

export interface LLMProductionDatasetMessage {
  role: 'system' | 'user' | 'assistant';
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
  format?: 'auto' | 'json' | 'jsonl' | 'csv' | 'tsv' | 'text';
  labelMode?: 'preserve' | 'unlabeled' | 'heuristic' | 'block';
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

export type SecurityRuleAction = 'BLOCK' | 'ALERT';

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
  format: 'json' | 'yaml' | 'markdown';
  sourceAttribution: string;
  category: 'agent-security' | 'community' | 'owasp';
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
