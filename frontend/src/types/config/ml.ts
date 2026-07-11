import type { ResearchCount } from "./research-processing";

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
  training_readiness: MLTrainingReadiness | null;
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
export interface DatasetQualitySummary {
  importableCount: number;
  labeledCount: number;
  unlabeledCount: number;
  duplicateCount: number;
  dominantLabel?: string;
  dominantLabelRatio?: number;
  classImbalance: boolean;
  featureOutOfRange: number;
  normalizationStatus: string;
  warnings?: string[];
}
export interface MLTrainingReadiness {
  ready: boolean;
  sampleCount: number;
  labeledCount: number;
  unlabeledCount: number;
  minSamples: number;
  minClasses: number;
  classCount: number;
  featureDim: number;
  byLabel?: ResearchCount[];
  byCategory?: ResearchCount[];
  normalization: FeatureNormalizationReport;
  quality: DatasetQualitySummary;
  blockingReasons?: string[];
  warnings?: string[];
  suggestedActions?: string[];
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
  totalIsLowerBound?: boolean;
  limit: number;
  recordLimit?: number;
  truncated: boolean;
  imported?: number;
  skipped?: number;
  totalSamples?: number;
  labeledSamples?: number;
  rows?: RemoteDatasetRow[];
  families?: Record<string, number>;
  byLabel?: ResearchCount[];
  byCategory?: ResearchCount[];
  bySource?: ResearchCount[];
  skipReasons?: ResearchCount[];
  parseWarnings?: Array<{
    source?: string;
    row?: number;
    reason: string;
    count?: number;
  }>;
  normalization?: FeatureNormalizationReport;
  quality?: DatasetQualitySummary;
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
  byCategory?: ResearchCount[];
  bySource?: ResearchCount[];
  normalization: FeatureNormalizationReport;
  quality?: DatasetQualitySummary;
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
  skippedByReason?: ResearchCount[];
  normalization: FeatureNormalizationReport;
  quality?: DatasetQualitySummary;
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
  bundledAsset?: "safety-net" | "balanced-training";
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
