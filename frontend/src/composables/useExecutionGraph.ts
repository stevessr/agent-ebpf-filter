import { computed, type Ref } from "vue";
import type { Router } from "vue-router";

import type { ProcessInfo } from "../composables/useMonitorData";
import type {
  ExecutionGraphEdge,
  ExecutionGraphFilterState,
  ExecutionGraphNode,
  ExecutionGraphResponse,
} from "../types/executionGraph";

export type GraphState = ExecutionGraphResponse & { nodes: ExecutionGraphNode[]; edges: ExecutionGraphEdge[] };
export type BrowserGraphSnapshot = { recordedAt: string; graph: GraphState };

export interface ExecutionGraphComposableOptions {
  router: Router;
  graph: Ref<GraphState>;
  selectedNodeId: Ref<string>;
  filters: ExecutionGraphFilterState;
  replayPath: Ref<string>;
  browserSnapshots: Ref<BrowserGraphSnapshot[]>;
  browserRecordingActive: Ref<boolean>;
  browserReplayActive: Ref<boolean>;
  processes: Ref<ProcessInfo[]>;
  selectedProcessPid: Ref<number | null>;
}

export function useExecutionGraph(options: ExecutionGraphComposableOptions) {
  const {
    router,
    graph,
    selectedNodeId,
    filters,
    replayPath,
    browserSnapshots,
    browserRecordingActive,
    browserReplayActive,
    processes,
    selectedProcessPid,
  } = options;

const kindTagColorMap: Record<string, string> = {
  agent_run: 'purple',
  tool_call: 'blue',
  process: 'green',
  syscall: 'orange',
  wrapper_event: 'cyan',
  hook_event: 'geekblue',
  file: 'default',
  network: 'red',
  policy_decision: 'black',
  policy_alert: 'error',
  exit_status: 'default',
};

const decisionOptions = [
  { label: 'Any decision', value: '' },
  { label: 'ALLOW', value: 'ALLOW' },
  { label: 'ALERT', value: 'ALERT' },
  { label: 'BLOCK', value: 'BLOCK' },
  { label: 'REWRITE', value: 'REWRITE' },
];

const timePresetLabels: Record<ExecutionGraphFilterState['timePreset'], string> = {
  all: 'All retained events',
  '15m': 'Last 15 minutes',
  '1h': 'Last 1 hour',
  '6h': 'Last 6 hours',
  '24h': 'Last 24 hours',
  '7d': 'Last 7 days',
  custom: 'Custom since / until',
};

const nodeMap = computed(() => new Map(graph.value.nodes.map((node) => [node.id, node])));
const selectedNode = computed(() => nodeMap.value.get(selectedNodeId.value) ?? null);
const selectedNodeKindColor = computed(() => kindTagColorMap[selectedNode.value?.kind ?? ''] ?? 'default');

const incidentEdges = computed(() => {
  if (!selectedNodeId.value) return [] as ExecutionGraphEdge[];
  return graph.value.edges.filter((edge) => edge.source === selectedNodeId.value || edge.target === selectedNodeId.value);
});

const collectReachableIds = (startId: string, maxDepth: number) => {
  const visited = new Set<string>([startId]);
  const queue: Array<{ id: string; depth: number }> = [{ id: startId, depth: 0 }];
  while (queue.length) {
    const current = queue.shift()!;
    if (current.depth >= maxDepth) continue;
    for (const edge of graph.value.edges) {
      if (edge.source !== current.id && edge.target !== current.id) continue;
      const nextId = edge.source === current.id ? edge.target : edge.source;
      if (!visited.has(nextId)) {
        visited.add(nextId);
        queue.push({ id: nextId, depth: current.depth + 1 });
      }
    }
  }
  return visited;
};

const relatedNodes = computed(() => {
  if (!selectedNode.value) return [] as ExecutionGraphNode[];
  const maxDepth = selectedNode.value.kind === 'agent_run' || selectedNode.value.kind === 'tool_call' ? 3 : 2;
  const relatedIds = collectReachableIds(selectedNode.value.id, maxDepth);
  return graph.value.nodes.filter((node) => relatedIds.has(node.id) && node.id !== selectedNode.value?.id);
});

const relatedProcesses = computed(() => relatedNodes.value.filter((node) => node.kind === 'process'));
const relatedFiles = computed(() => relatedNodes.value.filter((node) => node.kind === 'file'));
const relatedNetwork = computed(() => relatedNodes.value.filter((node) => node.kind === 'network'));
const relatedPolicies = computed(() => relatedNodes.value.filter((node) => node.kind === 'policy_alert' || node.kind === 'policy_decision'));

const sortedNodeCounts = computed(() => Object.entries(graph.value.nodeCounts ?? {}).sort((a, b) => b[1] - a[1]));
const sortedEdgeCounts = computed(() => Object.entries(graph.value.edgeCounts ?? {}).sort((a, b) => b[1] - a[1]));
const metadataEntries = computed(() => Object.entries(selectedNode.value?.metadata ?? {}).filter(([, value]) => value !== ''));
const processList = computed(() => [...processes.value].sort((a, b) => (b.cpu ?? 0) - (a.cpu ?? 0) || a.pid - b.pid));
const processTreeEdgeKinds = new Set(['child_process', 'parent_process', 'exec_chain', 'spawned']);
const selectedProcess = computed(() => {
  if (!selectedProcessPid.value) return null;
  return processList.value.find((process) => process.pid === selectedProcessPid.value) ?? null;
});
const focusedProcessNodeId = computed(() => {
  const pid = Number(filters.pid);
  return Number.isFinite(pid) && pid > 0 ? `proc:${pid}` : '';
});
const selectedProcessSummary = computed(() => {
  const process = selectedProcess.value;
  if (!process) {
    return filters.pid ? `Listening to pid ${filters.pid}; it may have exited from the live process list.` : 'Pick a live process to focus its current and descendant execution graph.';
  }
  const cmdline = process.cmdline?.trim();
  return `${process.name} pid=${process.pid} ppid=${process.ppid} cpu=${(process.cpu ?? 0).toFixed(1)}% mem=${(process.mem ?? 0).toFixed(1)}%${cmdline ? ` · ${cmdline}` : ''}`;
});
const replayEnabled = computed(() => replayPath.value.trim().length > 0);
const browserSnapshotCount = computed(() => browserSnapshots.value.length);
const browserRecordingSummary = computed(() => {
  if (!browserSnapshotCount.value) return '浏览器内存尚无录制快照，刷新页面后会丢失。';
  const first = browserSnapshots.value[0]?.recordedAt ?? '';
  const last = browserSnapshots.value[browserSnapshots.value.length - 1]?.recordedAt ?? '';
  return `${browserSnapshotCount.value} snapshots${first && last ? ` · ${first} → ${last}` : ''}`;
});
const processTreeNodeIds = computed(() => {
  const ids = new Set<string>();
  if (focusedProcessNodeId.value) {
    ids.add(focusedProcessNodeId.value);
  }
  graph.value.edges.forEach((edge) => {
    const source = nodeMap.value.get(edge.source);
    const target = nodeMap.value.get(edge.target);
    if (processTreeEdgeKinds.has(edge.kind) && source?.kind === 'process' && target?.kind === 'process') {
      ids.add(edge.source);
      ids.add(edge.target);
    }
  });
  return ids;
});
const processTreeNodes = computed(() => graph.value.nodes.filter((node) => node.kind === 'process' && processTreeNodeIds.value.has(node.id)));
const processTreeEdges = computed(() => graph.value.edges.filter((edge) => {
  const source = nodeMap.value.get(edge.source);
  const target = nodeMap.value.get(edge.target);
  return processTreeEdgeKinds.has(edge.kind) && source?.kind === 'process' && target?.kind === 'process';
}));

const buildPresetSince = (preset: ExecutionGraphFilterState['timePreset']) => {
  const now = Date.now();
  switch (preset) {
    case '15m':
      return new Date(now - 15 * 60 * 1000).toISOString();
    case '1h':
      return new Date(now - 60 * 60 * 1000).toISOString();
    case '6h':
      return new Date(now - 6 * 60 * 60 * 1000).toISOString();
    case '24h':
      return new Date(now - 24 * 60 * 60 * 1000).toISOString();
    case '7d':
      return new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
    default:
      return '';
  }
};

const buildParams = () => {
  const params: Record<string, string | number> = { limit: filters.limit };
  const textMappings: Array<[string, string]> = [
    ['agent_run_id', filters.agentRunId],
    ['tool_call_id', filters.toolCallId],
    ['trace_id', filters.traceId],
    ['pid', filters.pid],
    ['comm', filters.comm],
    ['tool_name', filters.toolName],
    ['path', filters.path],
    ['domain', filters.domain],
    ['decision', filters.decision],
  ];
  textMappings.forEach(([key, value]) => {
    if (value.trim()) params[key] = value.trim();
  });
  if (filters.riskMin > 0) {
    params.risk_min = filters.riskMin;
  }
  if (replayEnabled.value) {
    params.replay_path = replayPath.value.trim();
  }
  if (filters.pid.trim() && filters.processTree) {
    params.process_tree = 'true';
  }
  if (filters.timePreset === 'custom') {
    if (filters.since.trim()) params.since = filters.since.trim();
    if (filters.until.trim()) params.until = filters.until.trim();
  } else if (filters.timePreset !== 'all') {
    const since = buildPresetSince(filters.timePreset);
    if (since) params.since = since;
  }
  return params;
};

const syncRouteQuery = async () => {
  const params = buildParams();
  const query: Record<string, string> = {};
  Object.entries(params).forEach(([key, value]) => {
    query[key] = String(value);
  });
  query.timePreset = filters.timePreset;
  if (filters.pid.trim() && filters.processTree) {
    query.process_tree = 'true';
  }
  if (filters.timePreset === 'custom') {
    if (filters.since.trim()) query.since = filters.since.trim();
    if (filters.until.trim()) query.until = filters.until.trim();
  }
  if (replayEnabled.value) {
    query.replay_path = replayPath.value.trim();
  }
  await router.replace({ query });
};

const normalizeGraphResponse = (payload: Partial<ExecutionGraphResponse> | undefined): GraphState => ({
  eventCount: Number(payload?.eventCount ?? 0),
  source: String(payload?.source ?? 'memory'),
  nodeCounts: payload?.nodeCounts ?? {},
  edgeCounts: payload?.edgeCounts ?? {},
  nodes: Array.isArray(payload?.nodes) ? payload!.nodes as ExecutionGraphNode[] : [],
  edges: Array.isArray(payload?.edges) ? payload!.edges as ExecutionGraphEdge[] : [],
});

const cloneGraphState = (state: GraphState): GraphState => ({
  eventCount: state.eventCount,
  source: state.source,
  nodeCounts: { ...(state.nodeCounts ?? {}) },
  edgeCounts: { ...(state.edgeCounts ?? {}) },
  nodes: state.nodes.map((node) => ({ ...node, metadata: node.metadata ? { ...node.metadata } : undefined })),
  edges: state.edges.map((edge) => ({ ...edge })),
});

const appendBrowserSnapshot = (state: GraphState) => {
  if (!browserRecordingActive.value || browserReplayActive.value) return;
  const snapshots = browserSnapshots.value;
  const recordedAt = new Date().toLocaleString();
  snapshots.push({ recordedAt, graph: cloneGraphState(state) });
  if (snapshots.length > 1000) {
    snapshots.splice(0, snapshots.length - 1000);
  }
};


  return {
    kindTagColorMap,
    decisionOptions,
    timePresetLabels,
    nodeMap,
    selectedNode,
    selectedNodeKindColor,
    incidentEdges,
    collectReachableIds,
    relatedNodes,
    relatedProcesses,
    relatedFiles,
    relatedNetwork,
    relatedPolicies,
    sortedNodeCounts,
    sortedEdgeCounts,
    metadataEntries,
    processList,
    processTreeEdgeKinds,
    selectedProcess,
    focusedProcessNodeId,
    selectedProcessSummary,
    replayEnabled,
    browserSnapshotCount,
    browserRecordingSummary,
    processTreeNodeIds,
    processTreeNodes,
    processTreeEdges,
    buildPresetSince,
    buildParams,
    syncRouteQuery,
    normalizeGraphResponse,
    cloneGraphState,
    appendBrowserSnapshot,
  };
}
