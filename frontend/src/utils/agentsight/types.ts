export interface AgentSightEventRecord {
  Event?: Record<string, any>;
  Timestamp?: number | string;
  Envelope?: Record<string, any>;
  event?: Record<string, any>;
  timestamp?: number | string;
  envelope?: Record<string, any>;
  [key: string]: any;
}

export interface AgentSightEvent {
  id: string;
  timestamp: number;
  source: string;
  rawSource: string;
  pid: number;
  ppid?: number;
  comm: string;
  eventType: string;
  traceId: string;
  spanId: string;
  redactionState: string;
  title: string;
  data: Record<string, any>;
  raw: any;
  envelope?: Record<string, any>;
  legacyEvent?: Record<string, any>;
}

export interface ProcessedAgentSightEvent extends AgentSightEvent {
  datetime: Date;
  formattedTime: string;
  sourceColor: string;
  sourceColorClass: string;
}

export type ParsedAgentSightEventType =
  | "prompt"
  | "response"
  | "ssl"
  | "file"
  | "process"
  | "stdio"
  | "system"
  | "policy"
  | "agent";

export interface ParsedAgentSightEvent {
  id: string;
  timestamp: number;
  type: ParsedAgentSightEventType;
  title: string;
  content: string;
  metadata: Record<string, any>;
  promptDiff?: {
    diff: string;
    summary: string;
    hasChanges: boolean;
    previousPromptId?: string;
  };
}

export interface AgentSightTimelineItem {
  type: "event" | "process";
  timestamp: number;
  event?: ParsedAgentSightEvent;
  process?: AgentSightProcessNode;
}

export interface AgentSightProcessNode {
  pid: number;
  ppid?: number;
  comm: string;
  children: AgentSightProcessNode[];
  events: ParsedAgentSightEvent[];
  timeline: AgentSightTimelineItem[];
}

export interface AgentSightProcessFilters {
  eventTypes: string[];
  models: string[];
  sources: string[];
  commands: string[];
  searchText: string;
  timeRange: {
    start?: number;
    end?: number;
  };
}

export interface AgentSightFilterOptions {
  eventTypes: string[];
  models: string[];
  sources: string[];
  commands: string[];
}

export type AgentSightStdioProtocol =
  | "lsp"
  | "mcp"
  | "jsonrpc"
  | "text"
  | "unknown";

export interface DecodedStdioMessage {
  direction: string;
  fdRole: string;
  fdTarget: string;
  fd: number | null;
  length: number;
  truncated: boolean;
  rawPayload: string;
  parsedPayload: any | null;
  parsedMessages: any[];
  protocol: AgentSightStdioProtocol;
  framed: boolean;
  frameCount: number;
  incompleteFrame: boolean;
  framingError?: string;
  kind: "request" | "notification" | "response" | "error" | "text" | "unknown";
  method?: string;
  id?: string;
  toolName?: string;
  preview?: string;
  title: string;
  summary: string;

  // Incremental Content-Length framing metadata. These fields are optional so
  // one-off/stateless decoding remains API-compatible.
  streamKey?: string;
  reassembled?: boolean;
  reassembledBytes?: number;
  pendingBytes?: number;
  reassemblyReset?: string;
}
