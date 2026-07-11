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
