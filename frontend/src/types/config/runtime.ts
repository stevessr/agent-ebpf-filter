import type { DomainForwardProxySettings } from "./domain-forward";
import type { KernelRiskFeedbackSettings } from "./kernel-risk";
import type { LoopDetectionSettings } from "./loop-detection";
import type { ResearchProcessingSettings } from "./research-processing";
import type { SignalProcessingSettings } from "./signals";

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
  signalProcessing: SignalProcessingSettings;
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
export interface RuntimeConfigResponse {
  runtime: RuntimeSettings;
  mcpEndpoint: string;
  authHeaderName: string;
  bearerAuthHeaderName: string;
  persistedEventLogPath: string;
  persistedEventLogAlive: boolean;
}
