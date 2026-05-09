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
import ProcessPickerModal from '../components/ProcessPickerModal.vue';
import { useMonitorData } from '../composables/useMonitorData';
import type { ProcessInfo } from '../composables/useMonitorData';
import { buildWebSocketUrl } from '../utils/requestContext';
import { useExecutionGraph } from '../composables/useExecutionGraph';
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
type BrowserGraphSnapshot = { recordedAt: string; graph: GraphState };
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
const selectedProcessPid = ref<number | null>(filters.pid ? Number(filters.pid) || null : null);
const processPickerOpen = ref(false);
const liveListen = ref(true);
const graphSocketStatus = ref<'connecting' | 'connected' | 'paused' | 'closed' | 'error'>('closed');
const recordingPath = ref('');
const replayPath = ref(String(singleQuery(route.query.replay_path)).trim());
const recordingActive = ref(false);
const recordingCount = ref(0);
const recordingStartedAt = ref('');
const recordingBusy = ref(false);
const replayBusy = ref(false);
const browserRecordingActive = ref(false);
const browserReplayActive = ref(false);
const browserReplayIndex = ref(0);
const browserSnapshots = ref<BrowserGraphSnapshot[]>([]);
const browserSavePath = ref('');
const browserSaveBusy = ref(false);
let graphWs: WebSocket | null = null;
let graphReconnectTimer: ReturnType<typeof setTimeout> | null = null;
let recordingStatusTimer: ReturnType<typeof setInterval> | null = null;
let browserReplayTimer: ReturnType<typeof setInterval> | null = null;
const {
  kindTagColorMap,
  decisionOptions,
  timePresetLabels,
  nodeMap,
  selectedNode,
  selectedNodeKindColor,
  incidentEdges,
  collectReachableIds,
  relatedProcesses,
  relatedFiles,
  relatedNetwork,
  relatedPolicies,
  sortedNodeCounts,
  sortedEdgeCounts,
  metadataEntries,
  processList,
  focusedProcessNodeId,
  selectedProcessSummary,
  replayEnabled,
  browserSnapshotCount,
  browserRecordingSummary,
  processTreeNodes,
  processTreeEdges,
  buildPresetSince,
  buildParams,
  syncRouteQuery,
  normalizeGraphResponse,
  cloneGraphState,
  appendBrowserSnapshot,
} = useExecutionGraph({
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
});
const applyGraphPayload = (payload: Partial<ExecutionGraphResponse> | undefined) => {
  graph.value = normalizeGraphResponse(payload);
  appendBrowserSnapshot(graph.value);
  const focusedId = focusedProcessNodeId.value;
  if (selectedNodeId.value && !nodeMap.value.has(selectedNodeId.value)) {
    selectedNodeId.value = focusedId && nodeMap.value.has(focusedId) ? focusedId : graph.value.nodes[0]?.id ?? '';
  }
  if (!selectedNodeId.value && graph.value.nodes.length) {
    selectedNodeId.value = focusedId && nodeMap.value.has(focusedId) ? focusedId : graph.value.nodes[0].id;
  }
  lastLoadedAt.value = new Date().toLocaleString();
};
const closeGraphSocket = (status: typeof graphSocketStatus.value = 'closed') => {
  if (graphReconnectTimer) {
    clearTimeout(graphReconnectTimer);
    graphReconnectTimer = null;
  }
  if (graphWs) {
    const socket = graphWs;
    graphWs = null;
    socket.onopen = null;
    socket.onmessage = null;
    socket.onerror = null;
    socket.onclose = null;
    socket.close();
  }
  loading.value = false;
  graphSocketStatus.value = status;
};
const connectGraphSocket = () => {
  if (!liveListen.value) {
    closeGraphSocket('paused');
    return;
  }
  if (graphReconnectTimer) {
    clearTimeout(graphReconnectTimer);
    graphReconnectTimer = null;
  }
  if (graphWs) {
    graphWs.onclose = null;
    graphWs.close();
    graphWs = null;
  }
  loading.value = true;
  graphSocketStatus.value = 'connecting';
  const socket = new WebSocket(buildWebSocketUrl('/ws/events/graph', { ...buildParams(), interval: 1500 }));
  graphWs = socket;
  socket.onopen = () => {
    graphSocketStatus.value = 'connected';
  };
  socket.onmessage = (event) => {
    try {
      const payload = JSON.parse(String(event.data));
      if (payload?.error) {
        throw new Error(String(payload.error));
      }
      applyGraphPayload(payload);
      loading.value = false;
    } catch (error) {
      console.error('Failed to parse execution graph websocket payload', error);
      graphSocketStatus.value = 'error';
      loading.value = false;
    }
  };
  socket.onerror = () => {
    graphSocketStatus.value = 'error';
    loading.value = false;
  };
  socket.onclose = () => {
    if (graphWs !== socket) return;
    graphWs = null;
    if (!liveListen.value) {
      graphSocketStatus.value = 'paused';
      loading.value = false;
      return;
    }
    graphSocketStatus.value = 'closed';
    loading.value = false;
    graphReconnectTimer = setTimeout(() => connectGraphSocket(), 2000);
  };
};
const applyFilters = async () => {
  await syncRouteQuery();
  connectGraphSocket();
};
const loadRecordingStatus = async () => {
  try {
    const { data } = await axios.get('/events/recording');
    recordingActive.value = Boolean(data?.active);
    recordingCount.value = Number(data?.count ?? 0);
    recordingStartedAt.value = String(data?.startedAt ?? '');
    if (!recordingPath.value) {
      recordingPath.value = String(data?.path || data?.defaultPath || '');
    }
  } catch (error) {
    console.error('Failed to load event recording status', error);
  }
};
const startRecording = async () => {
  recordingBusy.value = true;
  try {
    const { data } = await axios.post('/events/recording/start', { path: recordingPath.value, truncate: false });
    recordingActive.value = Boolean(data?.active);
    recordingCount.value = Number(data?.count ?? 0);
    recordingStartedAt.value = String(data?.startedAt ?? '');
    recordingPath.value = String(data?.path || recordingPath.value);
    message.success('已开始录制事件到文件');
  } catch (error) {
    console.error('Failed to start event recording', error);
    message.error('开始录制失败');
  } finally {
    recordingBusy.value = false;
  }
};
const stopRecording = async () => {
  recordingBusy.value = true;
  try {
    const { data } = await axios.post('/events/recording/stop');
    recordingActive.value = Boolean(data?.active);
    recordingCount.value = Number(data?.count ?? recordingCount.value);
    message.success('已停止录制');
  } catch (error) {
    console.error('Failed to stop event recording', error);
    message.error('停止录制失败');
  } finally {
    recordingBusy.value = false;
  }
};
const playRecording = async () => {
  const path = recordingPath.value.trim();
  if (!path) {
    message.warning('请先填写录制文件路径');
    return;
  }
  replayBusy.value = true;
  try {
    const { data } = await axios.post('/events/recording/replay', { path, limit: filters.limit });
    replayPath.value = String(data?.path || path);
    applyGraphPayload(data?.graph);
    await syncRouteQuery();
    connectGraphSocket();
    message.success(`已回放 ${Number(data?.events ?? 0)} 条事件`);
  } catch (error) {
    console.error('Failed to replay event recording', error);
    message.error('回放录制文件失败');
  } finally {
    replayBusy.value = false;
  }
};
const stopReplay = async () => {
  replayPath.value = '';
  await applyFilters();
};
const stopBrowserReplay = () => {
  if (browserReplayTimer) {
    clearInterval(browserReplayTimer);
    browserReplayTimer = null;
  }
  browserReplayActive.value = false;
  browserReplayIndex.value = 0;
};
const startBrowserRecording = () => {
  stopBrowserReplay();
  browserSnapshots.value = [];
  browserRecordingActive.value = true;
  appendBrowserSnapshot(graph.value);
  message.success('已开始录制到浏览器内存');
};
const stopBrowserRecording = () => {
  browserRecordingActive.value = false;
  message.success(`已停止内存录制，共 ${browserSnapshotCount.value} 个快照`);
};
const playBrowserRecording = () => {
  if (!browserSnapshots.value.length) {
    message.warning('浏览器内存中没有可回放的快照');
    return;
  }
  closeGraphSocket('paused');
  browserRecordingActive.value = false;
  browserReplayActive.value = true;
  browserReplayIndex.value = 0;
  const snapshots = browserSnapshots.value;
  const playNext = () => {
    const snapshot = snapshots[browserReplayIndex.value];
    if (!snapshot) {
      stopBrowserReplay();
      return;
    }
    applyGraphPayload({ ...cloneGraphState(snapshot.graph), source: 'browser_memory' });
    lastLoadedAt.value = snapshot.recordedAt;
    browserReplayIndex.value += 1;
    if (browserReplayIndex.value >= snapshots.length) {
      stopBrowserReplay();
    }
  };
  playNext();
  browserReplayTimer = setInterval(playNext, 900);
};
const clearBrowserRecording = () => {
  stopBrowserReplay();
  browserRecordingActive.value = false;
  browserSnapshots.value = [];
  message.success('已清空浏览器内存录制');
};
const exitBrowserReplay = () => {
  stopBrowserReplay();
  if (liveListen.value) {
    connectGraphSocket();
  }
};
const buildBrowserRecordingExport = () => ({
  version: 1,
  kind: 'agent-ebpf-filter.execution-graph.browser-memory',
  exportedAt: new Date().toISOString(),
  snapshotCount: browserSnapshots.value.length,
  snapshots: browserSnapshots.value.map((snapshot) => ({
    recordedAt: snapshot.recordedAt,
    graph: cloneGraphState(snapshot.graph),
  })),
});
const browserRecordingFilename = () => {
  const stamp = new Date().toISOString().replace(/[:.]/g, '-');
  return `execution-graph-browser-memory-${stamp}.json`;
};
const exportBrowserRecording = () => {
  if (!browserSnapshots.value.length) {
    message.warning('浏览器内存中没有可导出的快照');
    return;
  }
  const payload = JSON.stringify(buildBrowserRecordingExport(), null, 2);
  const blob = new Blob([payload, '\n'], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = browserRecordingFilename();
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
  message.success('已导出浏览器内存录制');
};
const saveBrowserRecordingToBackend = async () => {
  if (!browserSnapshots.value.length) {
    message.warning('浏览器内存中没有可保存的快照');
    return;
  }
  browserSaveBusy.value = true;
  try {
    const { data } = await axios.post('/events/recording/browser/save', {
      path: browserSavePath.value.trim(),
      export: buildBrowserRecordingExport(),
    });
    browserSavePath.value = String(data?.path || browserSavePath.value);
    message.success(`已保存到后端：${browserSavePath.value}`);
  } catch (error) {
    console.error('Failed to save browser recording to backend', error);
    message.error('保存浏览器内存录制到后端失败');
  } finally {
    browserSaveBusy.value = false;
  }
};
const resetFilters = async () => {
  Object.assign(filters, defaultFilters());
  selectedProcessPid.value = null;
  replayPath.value = '';
  await applyFilters();
};
const handleSelectNode = (nodeId: string) => {
  selectedNodeId.value = nodeId;
};
const focusProcess = async (pid: number | null) => {
  selectedProcessPid.value = pid;
  filters.pid = pid ? String(pid) : '';
  filters.processTree = Boolean(pid);
  selectedNodeId.value = pid ? `proc:${pid}` : '';
  if (pid && filters.timePreset === 'all') {
    filters.timePreset = '24h';
  }
  await applyFilters();
};
const handleProcessPicked = (process: ProcessInfo) => {
  void focusProcess(process.pid);
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
  if (enabled) {
    connectGraphSocket();
  } else {
    closeGraphSocket('paused');
  }
});
onMounted(async () => {
  setupMonitorData();
  void loadRecordingStatus();
  recordingStatusTimer = setInterval(() => {
    void loadRecordingStatus();
  }, 2500);
  connectGraphSocket();
});
onUnmounted(() => {
  teardownMonitorData();
  if (recordingStatusTimer) {
    clearInterval(recordingStatusTimer);
    recordingStatusTimer = null;
  }
  stopBrowserReplay();
  closeGraphSocket('closed');
});
watch(liveListen, (enabled) => {
  if (enabled) {
    connectGraphSocket();
  } else {
    closeGraphSocket('paused');
  }
});
onMounted(async () => {
  setupMonitorData();
  void loadRecordingStatus();
  recordingStatusTimer = setInterval(() => {
    void loadRecordingStatus();
  }, 2500);
  connectGraphSocket();
});
onUnmounted(() => {
  teardownMonitorData();
  if (recordingStatusTimer) {
    clearInterval(recordingStatusTimer);
    recordingStatusTimer = null;
  }
  stopBrowserReplay();
  closeGraphSocket('closed');
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
          <a-space wrap>
            <a-button type="primary" @click="processPickerOpen = true">
              从进程列表选择
            </a-button>
            <a-button v-if="filters.pid" @click="focusProcess(null)">清除 PID</a-button>
            <a-tag v-if="filters.pid" color="processing">PID {{ filters.pid }}</a-tag>
          </a-space>
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
    <a-card :bordered="false" class="recording-card">
      <template #title><span><PlayCircleOutlined /> 录制 / 回放</span></template>
      <a-row :gutter="12" align="middle">
        <a-col :xs="24" :lg="12">
          <a-input v-model:value="recordingPath" allow-clear placeholder="~/.config/agent-ebpf-filter/recordings/events.jsonl" />
        </a-col>
        <a-col :xs="24" :lg="12">
          <a-space wrap>
            <a-button type="primary" :loading="recordingBusy" :disabled="recordingActive" @click="startRecording">开始录制到文件</a-button>
            <a-button danger :loading="recordingBusy" :disabled="!recordingActive" @click="stopRecording">停止录制</a-button>
            <a-button :loading="replayBusy" @click="playRecording">回放文件</a-button>
            <a-button v-if="replayEnabled" @click="stopReplay">退出回放</a-button>
            <a-tag v-if="recordingActive" color="red">录制中 · {{ recordingCount }}</a-tag>
            <a-tag v-if="replayEnabled" color="purple">回放中</a-tag>
          </a-space>
        </a-col>
      </a-row>
      <a-typography-text v-if="recordingStartedAt" type="secondary" class="recording-meta">
        started {{ recordingStartedAt }}
      </a-typography-text>
      <div class="browser-recording-row">
        <a-space wrap>
          <a-button type="primary" ghost :disabled="browserRecordingActive" @click="startBrowserRecording">开始录制到浏览器内存</a-button>
          <a-button :disabled="!browserRecordingActive" @click="stopBrowserRecording">停止内存录制</a-button>
          <a-button :disabled="!browserSnapshotCount" @click="playBrowserRecording">回放内存</a-button>
          <a-button v-if="browserReplayActive" @click="exitBrowserReplay">退出内存回放</a-button>
          <a-button :disabled="!browserSnapshotCount" danger ghost @click="clearBrowserRecording">清空内存</a-button>
          <a-button :disabled="!browserSnapshotCount" @click="exportBrowserRecording">导出内存 JSON</a-button>
          <a-button type="primary" :loading="browserSaveBusy" :disabled="!browserSnapshotCount" @click="saveBrowserRecordingToBackend">保存到后端</a-button>
          <a-tag v-if="browserRecordingActive" color="blue">内存录制中 · {{ browserSnapshotCount }}</a-tag>
          <a-tag v-if="browserReplayActive" color="purple">内存回放 {{ browserReplayIndex }}/{{ browserSnapshotCount }}</a-tag>
        </a-space>
        <a-input
          v-model:value="browserSavePath"
          allow-clear
          class="browser-save-path"
          placeholder="后端保存路径，可空；默认保存到 ~/.config/agent-ebpf-filter/recordings/browser-memory-*.json"
        />
        <a-typography-text type="secondary" class="recording-meta">
          {{ browserRecordingSummary }}
        </a-typography-text>
      </div>
    </a-card>
    <ProcessPickerModal
      v-model:open="processPickerOpen"
      :processes="processList"
      :selected-pid="selectedProcessPid"
      :loading="processLoading"
      title="选择要监听的进程"
      @select="handleProcessPicked"
    />
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
        <template #title>
          <a-space wrap>
            <span>Execution Topology</span>
            <a-tag color="green">process tree {{ processTreeNodes.length }}</a-tag>
            <a-tag color="cyan">chain edges {{ processTreeEdges.length }}</a-tag>
          </a-space>
        </template>
        <template #extra>
          <a-space wrap>
            <a-tag color="green">process</a-tag>
            <a-tag color="orange">syscall</a-tag>
            <a-tag color="blue">tool</a-tag>
            <a-tag color="red">network</a-tag>
            <a-tag color="default">file</a-tag>
          </a-space>
        </template>
        <a-alert
          v-if="replayEnabled"
          type="warning"
          show-icon
          class="graph-hint"
          :message="`正在回放文件：${replayPath}`"
        />
        <a-alert
          v-if="filters.pid"
          type="info"
          show-icon
          class="graph-hint"
          :message="`正在实时监听 PID ${filters.pid}${filters.processTree ? ' 的进程树和调用链' : ''}`"
        />
        <a-spin :spinning="loading">
          <ExecutionGraphCanvas
            :nodes="graph.nodes"
            :edges="graph.edges"
            :selected-node-id="selectedNodeId"
            zoom-storage-key="agent-ebpf.execution-graph.execution-topology.zoom"
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
                <a-list size="small" :data-source="selectedNode?.kind === 'process' ? processTreeNodes : relatedProcesses" bordered>
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
.recording-card,
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
.graph-hint {
  margin-bottom: 12px;
}
.recording-meta {
  display: block;
  margin-top: 8px;
}
.browser-recording-row {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
}
.browser-save-path {
  margin-top: 10px;
  max-width: 780px;
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
