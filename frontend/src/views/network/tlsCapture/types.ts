export interface TLSCaptureSummaryStats {
  total: number;
  sends: number;
  recvs: number;
  withBody: number;
  http: number;
  sse: number;
  llm: number;
  redacted: number;
  attachedLibs: number;
  handshakes: number;
  httpRequests: number;
  jsonData: number;
  sseData: number;
}

export interface TLSExecutableResolvedTarget {
  input?: string;
  path?: string;
  realPath?: string;
  shebang?: string;
}

export interface TLSExecutableAttachResult {
  error?: string;
  attachPath?: string;
  targetKind?: string;
  library?: string;
  resolved?: TLSExecutableResolvedTarget;
}
