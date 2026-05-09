export interface ExecutionGraphNode {
  id: string;
  kind: string;
  label: string;
  subtitle?: string;
  pid?: number;
  riskScore?: number;
  metadata?: Record<string, string>;
}

export interface ExecutionGraphEdge {
  id: string;
  source: string;
  target: string;
  kind: string;
  label?: string;
}

export interface ExecutionGraphResponse {
  eventCount: number;
  source: string;
  nodeCounts?: Record<string, number>;
  edgeCounts?: Record<string, number>;
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
}

export interface ExecutionGraphFilterState {
  limit: number;
  agentRunId: string;
  toolCallId: string;
  traceId: string;
  pid: string;
  comm: string;
  toolName: string;
  path: string;
  domain: string;
  decision: string;
  riskMin: number;
  timePreset: 'all' | '15m' | '1h' | '6h' | '24h' | '7d' | 'custom';
  since: string;
  until: string;
}
