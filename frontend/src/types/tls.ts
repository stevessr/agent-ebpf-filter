/**
 * Shared type definitions for TLS capture events and merged HTTP transactions.
 * Used by TLSCapture.vue and DevToolsNetworkPanel.vue.
 */

export interface TLSPlaintextEvent {
  key?: string;
  type?: string;
  timestamp?: string;
  pid?: number;
  tgid?: number;
  comm?: string;
  direction?: string;
  lib?: string;
  function?: string;
  captured_len?: number;
  original_len?: number;
  method?: string;
  url?: string;
  host?: string;
  status?: number;
  headers?: Record<string, string>;
  body?: string;
  body_size?: number;
  content_type?: string;
  raw_hex_dump?: string;
  raw_available?: boolean;
  truncated?: boolean;
  redaction_state?: string;
  sse_event?: string;
  sse_data_digest?: string;
  message_role?: string;
  prompt_digest?: string;
  prompt_len?: number;
  vendor?: string;
  uid?: number;
  tid?: number;
  is_handshake?: boolean;
  latency_ms?: number;
  data_type?: string;
  delta_ns?: number;
}

export interface TLSLibraryStatus {
  library?: number;
  name: string;
  path?: string;
  attached: boolean;
  available?: boolean;
  error?: string;
}

export interface TLSCaptureRule {
  id: string;
  name: string;
  enabled: boolean;
  scope: string;
  comms?: string[];
  hosts?: string[];
  methods?: string[];
  libraries?: string[];
  directions?: string[];
  description?: string;
}

export interface TLSIgnoreRule {
  id: string;
  name: string;
  enabled: boolean;
  comms?: string[];
  hosts?: string[];
  urls?: string[];
  methods?: string[];
  libraries?: string[];
  directions?: string[];
  statusCodes?: string[];
  description?: string;
}

export interface TLSCaptureStatus {
  enabled?: boolean;
  available?: boolean;
  readStarted?: boolean;
  error?: string;
  libraries?: TLSLibraryStatus[];
  broadcast?: TLSBroadcastStatus;
}

export interface TLSBroadcastStatus {
  activeClients: number;
  queuedEvents: number;
  queueCapacity: number;
  queueFullDropsTotal: number;
  writeFailuresTotal: number;
  writeDeadlineFailuresTotal: number;
}

export interface TLSBuiltinExecutableTarget {
  name: string;
  command: string;
  library: string;
  description?: string;
}

export interface TLSBuiltinExecutableAttachStatus {
  target: TLSBuiltinExecutableTarget;
  available?: boolean;
  attached?: boolean;
  result?: any;
  error?: string;
}

/** A merged HTTP transaction pairing a request with its response. */
export interface MergedTransaction {
  id: string;
  request?: TLSPlaintextEvent;
  response?: TLSPlaintextEvent;
  /** URL pathname for display (the "Name" column) */
  name: string;
  /** Full URL including host */
  fullUrl: string;
  host: string;
  method: string;
  status?: number;
  /** Content-Type shorthand */
  type: string;
  /** Response body size in bytes */
  size?: number;
  /** Round-trip latency in ms */
  timeMs?: number;
  timestamp: string;
  comm?: string;
  pid?: number;
  lib?: string;
  isComplete: boolean;
  redactionState?: string;
  vendor?: string;
  isSse?: boolean;
}
