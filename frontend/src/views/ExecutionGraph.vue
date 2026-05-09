<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  ClusterOutlined,
  FilterOutlined,
  InfoCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
  AlertOutlined,
  RadarChartOutlined,
} from '@ant-design/icons-vue';

import ExecutionGraphCanvas from '../components/execution-graph/ExecutionGraphCanvas.vue';
import { useMonitorData } from '../composables/useMonitorData';
import type {
  ExecutionGraphEdge,
  ExecutionGraphFilterState,
  ExecutionGraphNode,
  ExecutionGraphResponse,
} from '../types/executionGraph';

const route = useRoute();
const router = useRouter();
const monitorData = useMonitorData();
const { processes, loading: processLoading, setup: setupMonitorData, teardown: teardownMonitorData } = monitorData;

const timePresetOptions: ExecutionGraphFilterState['timePreset'][] = ['all', '15m', '1h', '6h', '24h', '7d', 'custom'];
const detailTabs = ['processes', 'files', 'network', 'policy', 'edges', 'metadata'] as const;
type DetailTab = typeof detailTabs[number];

type GraphState = ExecutionGraphResponse & { nodes: ExecutionGraphNode[]; edges: ExecutionGraphEdge[] };

const defaultFilters = (): ExecutionGraphFilterState => ({
  limit: 600,
  agentRunId: '',
  toolCallId: '',
  traceId: '',
  pid: '',
  processTree: true,
  comm: '',
  toolName: '',
  path: '',
  domain: '',
  decision: '',
  riskMin: 0,
  timePreset: '24h',
  since: '',
  until: '',
});

const singleQuery = (value: unknown) => Array.isArray(value) ? (value[0] ?? '') : (value ?? '');

const filtersFromRoute = (): ExecutionGraphFilterState => {
  const defaults = defaultFilters();
  const query = route.query;
  const parsedLimit = Number(singleQuery(query.limit));
  const parsedRisk = Number(singleQuery(query.risk_min));
  const timePreset = String(singleQuery(query.timePreset || query.time_preset || defaults.timePreset)).trim() as ExecutionGraphFilterState['timePreset'];
  const processTreeRaw = String(singleQuery(query.process_tree)).trim().toLowerCase();
  return {
    ...defaults,
    limit: Number.isFinite(parsedLimit) && parsedLimit > 0 ? parsedLimit : defaults.limit,
    agentRunId: String(singleQuery(query.agent_run_id)).trim(),
    toolCallId: String(singleQuery(query.tool_call_id)).trim(),
    traceId: String(singleQuery(query.trace_id)).trim(),
    pid: String(singleQuery(query.pid)).trim(),
    processTree: processTreeRaw === '' ? defaults.processTree : ['1', 'true', 'yes', 'on'].includes(processTreeRaw),
    comm: String(singleQuery(query.comm)).trim(),
    toolName: String(singleQuery(query.tool_name)).trim(),
    path: String(singleQuery(query.path)).trim(),
    domain: String(singleQuery(query.domain)).trim(),
    decision: String(singleQuery(query.decision)).trim(),
    riskMin: Number.isFinite(parsedRisk) && parsedRisk > 0 ? parsedRisk : defaults.riskMin,
    timePreset: timePresetOptions.includes(timePreset) ? timePreset : defaults.timePreset,
    since: String(singleQuery(query.since)).trim(),
    until: String(singleQuery(query.until)).trim(),
  };
};

const filters = reactive<ExecutionGraphFilterState>(filtersFromRoute());
const loading = ref(false);
const graph = ref<GraphState>({ eventCount: 0, source: 'memory', nodeCounts: {}, edgeCounts: {}, nodes: [], edges: [] });
const selectedNodeId = ref('');
const activeDetailTab = ref<DetailTab>('processes');
const lastLoadedAt = ref('');
const processSearch = ref('');
const selectedProcessPid = ref<number | null>(filters.pid ? Number(filters.pid) || null : null);
const liveListen = ref(true);
let liveRefreshTimer: ReturnType<typeof setInterval> | null = null;

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
const selectedProcess = computed(() => {
  if (!selectedProcessPid.value) return null;
  return processList.value.find((process) => process.pid === selectedProcessPid.value) ?? null;
});
const filteredProcessList = computed(() => {
  const query = processSearch.value.trim().toLowerCase();
  const list = query
    ? processList.value.filter((process) => (
      process.name.toLowerCase().includes(query) ||
      String(process.pid).includes(query) ||
      String(process.ppid).includes(query) ||
      (process.cmdline ?? '').toLowerCase().includes(query) ||
      (process.user ?? '').toLowerCase().includes(query)
    ))
    : processList.value;
  return list.slice(0, 200);
});
const processSelectOptions = computed(() => filteredProcessList.value.map((process) => ({
  value: process.pid,
  label: `${process.name || 'process'} · pid ${process.pid} · ppid ${process.ppid}`,
  process,
})));
const selectedProcessSummary = computed(() => {
  const process = selectedProcess.value;
  if (!process) {
    return filters.pid ? `Listening to pid ${filters.pid}; it may have exited from the live process list.` : 'Pick a live process to focus its current and descendant execution graph.';
  }
  const cmdline = process.cmdline?.trim();
  return `${process.name} pid=${process.pid} ppid=${process.ppid} cpu=${(process.cpu ?? 0).toFixed(1)}% mem=${(process.mem ?? 0).toFixed(1)}%${cmdline ? ` · ${cmdline}` : ''}`;
});

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

const loadGraph = async () => {
  loading.value = true;
  try {
    const res = await axios.get('/events/graph', { params: buildParams() });
    graph.value = normalizeGraphResponse(res.data);
    if (selectedNodeId.value && !nodeMap.value.has(selectedNodeId.value)) {
      selectedNodeId.value = graph.value.nodes[0]?.id ?? '';
    }
    if (!selectedNodeId.value && graph.value.nodes.length) {
      selectedNodeId.value = graph.value.nodes[0].id;
    }
    lastLoadedAt.value = new Date().toLocaleString();
  } catch (error) {
    console.error('Failed to load execution graph', error);
    message.error('Failed to load execution graph');
  } finally {
    loading.value = false;
  }
};

const applyFilters = async () => {
  await syncRouteQuery();
  await loadGraph();
};

const resetFilters = async () => {
  Object.assign(filters, defaultFilters());
  selectedProcessPid.value = null;
  await applyFilters();
};

const handleSelectNode = (nodeId: string) => {
  selectedNodeId.value = nodeId;
};

const focusProcess = async (pid: number | null) => {
  selectedProcessPid.value = pid;
  filters.pid = pid ? String(pid) : '';
  filters.processTree = Boolean(pid);
  if (pid && filters.timePreset === 'all') {
    filters.timePreset = '24h';
  }
  await applyFilters();
};

const handleProcessSelectChange = (value: number | string | null | undefined) => {
  void focusProcess(value ? Number(value) : null);
};

const focusProcessFromNode = async () => {
  const processNode = nearestProcessNode.value;
  const pid = Number(processNode?.metadata?.pid ?? processNode?.pid ?? 0);
  if (!pid) {
    message.warning('Select a process-related node first');
    return;
  }
  await focusProcess(pid);
  message.success(`Listening to process tree for pid ${pid}`);
};

const nearestProcessNode = computed(() => {
  const node = selectedNode.value;
  if (!node) return null;
  if (node.kind === 'process') return node;
  const related = collectReachableIds(node.id, 2);
  for (const candidateId of related) {
    const candidate = nodeMap.value.get(candidateId);
    if (candidate?.kind === 'process') return candidate;
  }
  return null;
});

const actionableComm = computed(() => {
  const processNode = nearestProcessNode.value;
  if (!processNode) return '';
  return processNode.metadata?.comm?.trim() || processNode.label.trim();
});

const replayAvailable = computed(() => Boolean(
  selectedNode.value?.metadata?.agentRunId ||
  selectedNode.value?.metadata?.toolCallId ||
  selectedNode.value?.metadata?.traceId,
));

const addRule = async (action: 'ALLOW' | 'BLOCK') => {
  const comm = actionableComm.value;
  if (!comm) {
    message.warning('Select a process-related node first');
    return;
  }
  try {
    await axios.post('/config/rules', { comm, action, rewritten_cmd: [] });
    message.success(`${action} rule added for ${comm}`);
  } catch (error) {
    console.error('Failed to add rule', error);
    message.error(`Failed to add ${action} rule`);
  }
};

const exportTrainingSample = async (label: 'ALLOW' | 'ALERT' | 'BLOCK') => {
  const comm = actionableComm.value;
  if (!comm) {
    message.warning('Select a process-related node first');
    return;
  }
  try {
    await axios.post('/config/ml/samples', {
      commandLine: comm,
      comm,
      args: [],
      label,
    });
    message.success(`${label} sample exported for ${comm}`);
  } catch (error) {
    console.error('Failed to export training sample', error);
    message.error('Failed to export training sample');
  }
};

const replaySelectedContext = async () => {
  if (!selectedNode.value) return;
  const metadata = selectedNode.value.metadata ?? {};
  filters.agentRunId = metadata.agentRunId ?? filters.agentRunId;
  filters.toolCallId = metadata.toolCallId ?? filters.toolCallId;
  filters.traceId = metadata.traceId ?? filters.traceId;
  filters.pid = metadata.pid ?? filters.pid;
  await applyFilters();
  message.success('Replayed current graph context filters');
};

const focusRelatedTab = (tab: DetailTab) => {
  activeDetailTab.value = tab;
};

const renderNodeSubtitle = (node: ExecutionGraphNode) => node.subtitle?.trim() || node.metadata?.path || node.metadata?.endpoint || '—';

watch(liveListen, (enabled) => {
  if (enabled && filters.pid.trim()) {
    void loadGraph();
  }
});

onMounted(async () => {
  setupMonitorData();
  liveRefreshTimer = setInterval(() => {
    if (liveListen.value && filters.pid.trim() && !loading.value) {
      void loadGraph();
    }
  }, 3000);
  await loadGraph();
});

onUnmounted(() => {
  teardownMonitorData();
  if (liveRefreshTimer) {
    clearInterval(liveRefreshTimer);
    liveRefreshTimer = null;
  }
});
</script>

<template>
  <div class="execution-graph-page">
    <a-card :bordered="false" class="hero-card">
      <div class="hero-header">
        <div>
          <a-typography-title :level="3" style="margin-bottom: 8px;">
            <ClusterOutlined /> Agent Execution Graph
          </a-typography-title>
          <a-typography-paragraph type="secondary" style="margin-bottom: 0;">
            Correlate agent runs, tool calls, processes, syscalls, files, network endpoints, policy decisions, and audit alerts in one execution graph.
          </a-typography-paragraph>
        </div>
        <a-space wrap>
          <a-badge status="processing" :text="`Source: ${graph.source}`" />
          <a-tag color="purple">{{ graph.eventCount }} matched events</a-tag>
          <a-tag color="blue">{{ graph.nodes.length }} nodes</a-tag>
          <a-tag color="geekblue">{{ graph.edges.length }} edges</a-tag>
          <a-tag v-if="lastLoadedAt" color="default">Updated {{ lastLoadedAt }}</a-tag>
        </a-space>
      </div>
    </a-card>

    <a-row :gutter="16" class="summary-row">
      <a-col :xs="24" :lg="8">
        <a-card size="small" title="Top Node Kinds">
          <a-space wrap>
            <a-tag v-for="[kind, count] in sortedNodeCounts.slice(0, 8)" :key="kind" :color="kindTagColorMap[kind] || 'default'">
              {{ kind }} · {{ count }}
            </a-tag>
          </a-space>
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="8">
        <a-card size="small" title="Top Edge Kinds">
          <a-space wrap>
            <a-tag v-for="[kind, count] in sortedEdgeCounts.slice(0, 8)" :key="kind" color="processing">
              {{ kind }} · {{ count }}
            </a-tag>
          </a-space>
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="8">
        <a-card size="small" title="Time Scope">
          <a-space direction="vertical" size="small">
            <span><b>Preset:</b> {{ timePresetLabels[filters.timePreset] }}</span>
            <span v-if="filters.timePreset === 'custom'"><b>Since:</b> {{ filters.since || '—' }}</span>
            <span v-if="filters.timePreset === 'custom'"><b>Until:</b> {{ filters.until || '—' }}</span>
            <span v-else-if="filters.timePreset !== 'all'"><b>Computed since:</b> {{ buildPresetSince(filters.timePreset) }}</span>
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-card :bordered="false" class="process-listener-card">
      <template #title><span><RadarChartOutlined /> Process Tree Listener</span></template>
      <a-row :gutter="12" align="middle">
        <a-col :xs="24" :lg="10">
          <a-select
            v-model:value="selectedProcessPid"
            show-search
            allow-clear
            placeholder="从实时进程列表中选择 PID"
            style="width: 100%;"
            :filter-option="false"
            :options="processSelectOptions"
            :loading="processLoading"
            @search="processSearch = $event"
            @change="handleProcessSelectChange"
          >
            <template #option="{ label, process }">
              <div class="process-option">
                <span>{{ label }}</span>
                <span class="muted-line">{{ process.user || 'unknown user' }} · {{ process.cmdline || 'no cmdline' }}</span>
              </div>
            </template>
          </a-select>
        </a-col>
        <a-col :xs="24" :lg="8">
          <a-typography-text type="secondary">{{ selectedProcessSummary }}</a-typography-text>
        </a-col>
        <a-col :xs="24" :lg="6">
          <a-space wrap>
            <a-switch v-model:checked="liveListen" checked-children="监听" un-checked-children="暂停" />
            <a-checkbox v-model:checked="filters.processTree" :disabled="!filters.pid" @change="applyFilters">显示子进程调用树</a-checkbox>
            <a-button size="small" :disabled="!nearestProcessNode" @click="focusProcessFromNode">监听当前节点 PID</a-button>
          </a-space>
        </a-col>
      </a-row>
    </a-card>

    <a-card :bordered="false" class="filter-card">
      <template #title><span><FilterOutlined /> Graph Filters</span></template>
      <a-form layout="vertical">
        <div class="filter-grid">
          <a-form-item label="Agent Run ID"><a-input v-model:value="filters.agentRunId" allow-clear placeholder="run-..." /></a-form-item>
          <a-form-item label="Tool Call ID"><a-input v-model:value="filters.toolCallId" allow-clear placeholder="tool-..." /></a-form-item>
          <a-form-item label="Trace ID"><a-input v-model:value="filters.traceId" allow-clear placeholder="trace-..." /></a-form-item>
          <a-form-item label="PID"><a-input v-model:value="filters.pid" allow-clear placeholder="101" /></a-form-item>
          <a-form-item label="Command"><a-input v-model:value="filters.comm" allow-clear placeholder="bash / git / python" /></a-form-item>
          <a-form-item label="Tool Name"><a-input v-model:value="filters.toolName" allow-clear placeholder="read_file / bash / npm" /></a-form-item>
          <a-form-item label="Path"><a-input v-model:value="filters.path" allow-clear placeholder="/workspace or id_rsa" /></a-form-item>
          <a-form-item label="Domain / Endpoint"><a-input v-model:value="filters.domain" allow-clear placeholder="github.com or :443" /></a-form-item>
          <a-form-item label="Decision"><a-select v-model:value="filters.decision" :options="decisionOptions" /></a-form-item>
          <a-form-item label="Minimum Risk Score"><a-input-number v-model:value="filters.riskMin" :min="0" :max="100" :step="5" style="width: 100%;" /></a-form-item>
          <a-form-item label="Event Limit"><a-input-number v-model:value="filters.limit" :min="50" :max="2000" :step="50" style="width: 100%;" /></a-form-item>
          <a-form-item label="Time Range Preset"><a-select v-model:value="filters.timePreset" :options="timePresetOptions.map(value => ({ label: timePresetLabels[value], value }))" /></a-form-item>
          <a-form-item v-if="filters.timePreset === 'custom'" label="Since (RFC3339 / unix ms)"><a-input v-model:value="filters.since" allow-clear placeholder="2026-05-08T10:00:00Z" /></a-form-item>
          <a-form-item v-if="filters.timePreset === 'custom'" label="Until (RFC3339 / unix ms)"><a-input v-model:value="filters.until" allow-clear placeholder="2026-05-08T12:00:00Z" /></a-form-item>
        </div>
      </a-form>
      <div class="filter-actions">
        <a-space wrap>
          <a-button type="primary" :loading="loading" @click="applyFilters"><ReloadOutlined /> Refresh Graph</a-button>
          <a-button @click="resetFilters">Reset Filters</a-button>
          <a-button :disabled="!replayAvailable" @click="replaySelectedContext"><PlayCircleOutlined /> Replay This Run</a-button>
        </a-space>
      </div>
    </a-card>

    <div class="graph-layout">
      <a-card :bordered="false" class="graph-card">
        <template #title>Execution Topology</template>
        <template #extra>
          <a-space wrap>
            <a-tag v-for="(color, kind) in kindTagColorMap" :key="kind" :color="color">{{ kind }}</a-tag>
          </a-space>
        </template>
        <a-spin :spinning="loading">
          <ExecutionGraphCanvas
            :nodes="graph.nodes"
            :edges="graph.edges"
            :selected-node-id="selectedNodeId"
            @select-node="handleSelectNode"
          />
        </a-spin>
      </a-card>

      <a-card :bordered="false" class="detail-card">
        <template #title><span><InfoCircleOutlined /> Node Details</span></template>
        <template #extra>
          <a-space v-if="selectedNode">
            <a-tag :color="selectedNodeKindColor">{{ selectedNode.kind }}</a-tag>
            <a-tag v-if="selectedNode.riskScore !== undefined" color="volcano">risk {{ Number(selectedNode.riskScore).toFixed(0) }}</a-tag>
          </a-space>
        </template>

        <a-empty v-if="!selectedNode" description="Select a node from the graph to inspect context, resources, and actions." />
        <template v-else>
          <a-space direction="vertical" size="middle" style="width: 100%;">
            <div>
              <a-typography-title :level="5" style="margin-bottom: 6px;">{{ selectedNode.label }}</a-typography-title>
              <a-typography-paragraph type="secondary" style="margin-bottom: 0;">
                {{ renderNodeSubtitle(selectedNode) }}
              </a-typography-paragraph>
            </div>

            <a-descriptions :column="1" size="small" bordered>
              <a-descriptions-item label="Node ID">{{ selectedNode.id }}</a-descriptions-item>
              <a-descriptions-item label="Kind">{{ selectedNode.kind }}</a-descriptions-item>
              <a-descriptions-item v-if="selectedNode.pid" label="PID">{{ selectedNode.pid }}</a-descriptions-item>
              <a-descriptions-item v-if="actionableComm" label="Actionable Command">{{ actionableComm }}</a-descriptions-item>
            </a-descriptions>

            <div class="node-actions">
              <a-space wrap>
                <a-button size="small" @click="addRule('ALLOW')"><SafetyCertificateOutlined /> Add allow rule</a-button>
                <a-button size="small" danger @click="addRule('BLOCK')"><StopOutlined /> Add block rule</a-button>
                <a-button size="small" @click="exportTrainingSample('ALLOW')">Mark benign</a-button>
                <a-button size="small" type="primary" ghost @click="exportTrainingSample('ALERT')"><AlertOutlined /> Mark suspicious</a-button>
                <a-button size="small" type="dashed" @click="exportTrainingSample('BLOCK')">Export BLOCK sample</a-button>
              </a-space>
            </div>

            <a-space wrap>
              <a-button size="small" @click="focusRelatedTab('processes')">Show related process tree</a-button>
              <a-button size="small" @click="focusRelatedTab('files')">Show related files</a-button>
              <a-button size="small" @click="focusRelatedTab('network')">Show related network flows</a-button>
              <a-button size="small" @click="focusRelatedTab('policy')">Show related policy events</a-button>
            </a-space>

            <a-tabs v-model:activeKey="activeDetailTab" size="small">
              <a-tab-pane key="processes" :tab="`Processes (${relatedProcesses.length})`">
                <a-list size="small" :data-source="relatedProcesses" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item @click="selectedNodeId = item.id" class="clickable-list-item">
                      <a-space direction="vertical" size="small">
                        <span><b>{{ item.label }}</b> <a-tag color="green">process</a-tag></span>
                        <span class="muted-line">{{ renderNodeSubtitle(item) }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
              <a-tab-pane key="files" :tab="`Files (${relatedFiles.length})`">
                <a-list size="small" :data-source="relatedFiles" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item @click="selectedNodeId = item.id" class="clickable-list-item">
                      <a-space direction="vertical" size="small">
                        <span><b>{{ item.label }}</b></span>
                        <span class="muted-line">{{ item.metadata?.path || 'file access' }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
              <a-tab-pane key="network" :tab="`Network (${relatedNetwork.length})`">
                <a-list size="small" :data-source="relatedNetwork" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item @click="selectedNodeId = item.id" class="clickable-list-item">
                      <a-space direction="vertical" size="small">
                        <span><b>{{ item.label }}</b></span>
                        <span class="muted-line">{{ item.subtitle || item.metadata?.domain || 'network relation' }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
              <a-tab-pane key="policy" :tab="`Policy (${relatedPolicies.length})`">
                <a-list size="small" :data-source="relatedPolicies" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item @click="selectedNodeId = item.id" class="clickable-list-item">
                      <a-space direction="vertical" size="small">
                        <span>
                          <b>{{ item.label }}</b>
                          <a-tag :color="item.kind === 'policy_alert' ? 'error' : 'default'">{{ item.kind }}</a-tag>
                        </span>
                        <span class="muted-line">{{ renderNodeSubtitle(item) }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
              <a-tab-pane key="edges" :tab="`Edges (${incidentEdges.length})`">
                <a-list size="small" :data-source="incidentEdges" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item>
                      <a-space direction="vertical" size="small">
                        <span><b>{{ item.kind }}</b></span>
                        <span class="muted-line">{{ item.source }} → {{ item.target }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
              <a-tab-pane key="metadata" tab="Metadata">
                <a-list size="small" :data-source="metadataEntries" bordered>
                  <template #renderItem="{ item }">
                    <a-list-item>
                      <div class="metadata-row">
                        <span class="metadata-key">{{ item[0] }}</span>
                        <span class="metadata-value">{{ item[1] || '—' }}</span>
                      </div>
                    </a-list-item>
                  </template>
                </a-list>
              </a-tab-pane>
            </a-tabs>
          </a-space>
        </template>
      </a-card>
    </div>
  </div>
</template>

<style scoped>
.execution-graph-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.hero-card,
.process-listener-card,
.filter-card,
.graph-card,
.detail-card {
  border-radius: 14px;
}

.hero-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  align-items: flex-start;
}

.summary-row {
  margin-top: -4px;
}

.filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 12px;
}

.filter-actions {
  display: flex;
  justify-content: flex-end;
}

.process-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.graph-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.7fr) minmax(320px, 420px);
  gap: 16px;
  align-items: start;
}

.graph-card :deep(.ant-card-body),
.detail-card :deep(.ant-card-body) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.node-actions {
  padding: 8px 0;
  border-top: 1px solid #f1f5f9;
  border-bottom: 1px solid #f1f5f9;
}

.clickable-list-item {
  cursor: pointer;
}

.clickable-list-item:hover {
  background: rgba(59, 130, 246, 0.06);
}

.muted-line {
  color: #64748b;
  font-size: 12px;
}

.metadata-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.metadata-key {
  font-weight: 600;
  color: #111827;
}

.metadata-value {
  color: #475569;
  word-break: break-all;
}

@media (max-width: 1200px) {
  .graph-layout {
    grid-template-columns: 1fr;
  }
}
</style>
