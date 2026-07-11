export type SignalKind =
  | "path_access"
  | "child_process"
  | "repeated_read"
  | "custom"
  | string;
export interface SignalCondition {
  field: string;
  operator:
    | "equals"
    | "contains"
    | "prefix"
    | "suffix"
    | "regex"
    | "exists"
    | "any"
    | "not_contains"
    | "not_equals"
    | string;
  value: string;
}
export interface SignalRule {
  id: string;
  name: string;
  enabled: boolean;
  kind: SignalKind;
  ttlSeconds: number;
  weight: number;
  conditions: SignalCondition[];
}
export interface SelectedProgramSignalLog {
  program: string;
  enabled: boolean;
  path?: string;
}
export interface SignalProcessingSettings {
  enabled: boolean;
  queueSize: number;
  cronIntervalSeconds: number;
  defaultTTLSeconds: number;
  maxStates: number;
  protoLogCompression: "gzip" | string;
  selectedPrograms: SelectedProgramSignalLog[];
  rules: SignalRule[];
}
export interface SignalKindInfo {
  kind: SignalKind;
  label: string;
  description: string;
}
export interface SignalState {
  id: string;
  ruleId: string;
  ruleName: string;
  kind: SignalKind;
  key: string;
  target: string;
  pid?: number;
  tgid?: number;
  comm?: string;
  count: number;
  score: number;
  ttlSeconds: number;
  firstSeen: string;
  lastMatchedAt: string;
  expiresAt: string;
  updatedAt: string;
  lastEventType?: string;
  lastPath?: string;
  lastExtraPath?: string;
  lastEventId?: string;
}
export interface SignalProcessingStatus {
  enabled: boolean;
  settings: SignalProcessingSettings;
  queueLen: number;
  queueCap: number;
  consumedTotal: number;
  updatedTotal: number;
  droppedTotal: number;
  expiredTotal: number;
  activeStates: number;
  recentStates: SignalState[];
  availableKinds: SignalKindInfo[];
  lastError?: string;
  updatedAt: string;
}
export interface SignalRuleTestMatch {
  timestamp: string;
  pid?: number;
  tgid?: number;
  comm?: string;
  eventType?: string;
  target?: string;
  path?: string;
  extraPath?: string;
  extraInfo?: string;
  eventId?: string;
  signalKey?: string;
  wouldScore: number;
}
export interface SignalRuleTestResponse {
  rule: SignalRule;
  scannedTotal: number;
  matchedTotal: number;
  matches: SignalRuleTestMatch[];
}
export interface SignalProgramLogStatus {
  program: string;
  enabled: boolean;
  path: string;
  exists: boolean;
  sizeBytes: number;
  frameCount: number;
  modifiedAt?: string;
  error?: string;
}
export interface SignalProgramLogsResponse {
  compression: string;
  logs: SignalProgramLogStatus[];
}
