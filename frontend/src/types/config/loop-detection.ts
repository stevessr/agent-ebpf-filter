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
  windowGCRunsTotal: number;
  windowEvictedTotal: number;
  recentFindings: LoopDetectionFinding[];
  lastError?: string;
  updatedAt: string;
}
