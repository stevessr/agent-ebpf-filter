<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import axios from "axios";
import { message } from "ant-design-vue";
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
} from "@ant-design/icons-vue";
import ExecutionGraphCanvas from "../../components/execution-graph/ExecutionGraphCanvas.vue";
import ProcessPickerModal from "../../components/monitor/ProcessPickerModal.vue";
import AgentSightTracePanel from "../../components/agentsight/AgentSightTracePanel.vue";
import { useMonitorData } from "../../composables/monitor/useMonitorData";
import type { ProcessInfo } from "../../composables/monitor/useMonitorData";
import { useExecutionGraph } from "../../composables/execution-graph/useExecutionGraph";
import { useExecutionGraphRecording } from "../../composables/execution-graph/useExecutionGraphRecording";
import ExecutionGraphRecordingPanel from "./ExecutionGraphRecordingPanel.vue";
import type {
  ExecutionGraphEdge,
  ExecutionGraphFilterState,
  ExecutionGraphNode,
  ExecutionGraphResponse,
} from "../../types/executionGraph";
import { filtersFromRoute, useGraphFilters } from "./useGraphFilters";
import { useGraphWebSocket } from "./useGraphWebSocket";
const route = useRoute();
const router = useRouter();
const monitorData = useMonitorData();
const {
  processes,
  loading: processLoading,
  setup: setupMonitorData,
  teardown: teardownMonitorData,
} = monitorData;
const detailTabs = [
  "processes",
  "files",
  "network",
  "policy",
  "edges",
  "metadata",
] as const;
const traceTabs = ["topology", "behavior", "recording"] as const;
type DetailTab = (typeof detailTabs)[number];
type TraceTab = (typeof traceTabs)[number];
type GraphState = ExecutionGraphResponse & {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
};
type BrowserGraphSnapshot = { recordedAt: string; graph: GraphState };
// ── Standalone state (created first to break circular deps) ──────────
const filters = reactive<ExecutionGraphFilterState>(
  filtersFromRoute(route.query),
);
const selectedProcessPid = ref<number | null>(
  filters.pid ? Number(filters.pid) || null : null,
);
const graph = ref<GraphState>({
  eventCount: 0,
  source: "memory",
  nodeCounts: {},
  edgeCounts: {},
  nodes: [],
  edges: [],
});
const selectedNodeId = ref("");
const activeDetailTab = ref<DetailTab>("processes");
const lastLoadedAt = ref("");
const processPickerOpen = ref(false);
const liveListen = ref(true);
const replayPath = ref(
  String(
    (Array.isArray(route.query.replay_path)
      ? route.query.replay_path[0]
      : route.query.replay_path) ?? "",
  ).trim(),
);
const browserRecordingActive = ref(false);
const browserReplayActive = ref(false);
const browserSnapshots = ref<BrowserGraphSnapshot[]>([]);
// ── Composables (order: executionGraph -> ws -> filters) ─────────────
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
const applyGraphPayload = (
  payload: Partial<ExecutionGraphResponse> | undefined,
) => {
  graph.value = normalizeGraphResponse(payload);
  appendBrowserSnapshot(graph.value);
  const focusedId = focusedProcessNodeId.value;
  if (selectedNodeId.value && !nodeMap.value.has(selectedNodeId.value)) {
    selectedNodeId.value =
      focusedId && nodeMap.value.has(focusedId)
        ? focusedId
        : (graph.value.nodes[0]?.id ?? "");
  }
  if (!selectedNodeId.value && graph.value.nodes.length) {
    selectedNodeId.value =
      focusedId && nodeMap.value.has(focusedId)
        ? focusedId
        : graph.value.nodes[0].id;
  }
  lastLoadedAt.value = new Date().toLocaleString();
};
const { loading, connectGraphSocket, closeGraphSocket } = useGraphWebSocket({
  liveListen,
  buildParams,
  applyGraphPayload,
});
const {
  timePresetOptions,
  applyFilters,
  resetFilters,
  focusProcess: focusProcessBase,
} = useGraphFilters({
  route,
  router,
  filters,
  selectedProcessPid,
  replayPath,
  buildPresetSince,
  syncRouteQuery,
  connectGraphSocket,
});
// ── Recording composable ─────────────────────────────────────────────
const recording = useExecutionGraphRecording({
  graph,
  filters,
  replayPath,
  browserSnapshots,
  browserRecordingActive,
  browserReplayActive,
  applyGraphPayload,
  syncRouteQuery,
  connectGraphSocket,
  closeGraphSocket,
  cloneGraphState,
  appendBrowserSnapshot,
  setLastLoadedAt: (value: string) => {
    lastLoadedAt.value = value;
  },
  browserSnapshotCount,
  replayEnabled,
  browserRecordingSummary,
});
const {
  recordingPath,
  recordingActive,
  recordingCount,
  recordingStartedAt,
  recordingBusy,
  replayBusy,
  startRecording,
  stopRecording,
  playRecording,
  stopReplay,
  browserReplayIndex,
  browserSavePath,
  browserSaveBusy,
  startBrowserRecording,
  stopBrowserRecording,
  playBrowserRecording,
  clearBrowserRecording,
  exitBrowserReplay,
  exportBrowserRecording,
  saveBrowserRecordingToBackend,
  startRecordingStatusPolling,
  cleanup: cleanupRecording,
} = recording;
// ── Computed nodes ───────────────────────────────────────────────────
const nearestProcessNode = computed(() => {
  const node = selectedNode.value;
  if (!node) return null;
  if (node.kind === "process") return node;
  for (const id of collectReachableIds(node.id, 2)) {
    const c = nodeMap.value.get(id);
    if (c?.kind === "process") return c;
  }
  return null;
});
const actionableComm = computed(
  () =>
    nearestProcessNode.value?.metadata?.comm?.trim() ||
    nearestProcessNode.value?.label.trim() ||
    "",
);
const replayAvailable = computed(
  () =>
    !!(
      selectedNode.value?.metadata?.agentRunId ||
      selectedNode.value?.metadata?.toolCallId ||
      selectedNode.value?.metadata?.traceId
    ),
);
const hiddenGraphKinds = ref<Set<string>>(new Set());
const graphKindLabels: Record<string, string> = {
  agent_run: "agent",
  tool_call: "tool",
  process: "process",
  syscall: "syscall",
  wrapper_event: "wrapper",
  hook_event: "hook",
  file: "file",
  network: "network",
  policy_decision: "policy",
  policy_alert: "alert",
  exit_status: "exit",
};
const graphKindOrder = [
  "process",
  "syscall",
  "tool_call",
  "agent_run",
  "network",
  "file",
  "policy_alert",
  "policy_decision",
  "wrapper_event",
  "hook_event",
  "exit_status",
];
const graphKindFilters = computed(() => {
  const counts = new Map(Object.entries(graph.value.nodeCounts ?? {}));
  graph.value.nodes.forEach((node) => {
    if (!counts.has(node.kind)) counts.set(node.kind, 0);
  });
  return [...counts.entries()]
    .sort(([left], [right]) => {
      const leftIndex = graphKindOrder.indexOf(left);
      const rightIndex = graphKindOrder.indexOf(right);
      return (
        (leftIndex === -1 ? graphKindOrder.length : leftIndex) -
          (rightIndex === -1 ? graphKindOrder.length : rightIndex) ||
        left.localeCompare(right)
      );
    })
    .map(([kind, count]) => ({
      kind,
      count,
      label: graphKindLabels[kind] ?? kind,
      color: kindTagColorMap[kind] ?? "default",
      hidden: hiddenGraphKinds.value.has(kind),
    }));
});
const visibleGraphNodes = computed(() =>
  graph.value.nodes.filter((node) => !hiddenGraphKinds.value.has(node.kind)),
);
const visibleGraphEdges = computed(() => {
  const visibleIds = new Set(visibleGraphNodes.value.map((node) => node.id));
  return graph.value.edges.filter(
    (edge) => visibleIds.has(edge.source) && visibleIds.has(edge.target),
  );
});
const hasHiddenGraphKinds = computed(() => hiddenGraphKinds.value.size > 0);
const activeTraceTab = computed<TraceTab>({
  get() {
    const tab = String(route.params.tab || "topology");
    return traceTabs.includes(tab as TraceTab) ? (tab as TraceTab) : "topology";
  },
  set(tab) {
    void router.push({
      name: "ExecutionGraph",
      params: { tab },
      query: route.query,
    });
  },
});
const renderNodeSubtitle = (node: ExecutionGraphNode) =>
  node.subtitle?.trim() ||
  node.metadata?.path ||
  node.metadata?.endpoint ||
  "—";
// ── Event handlers ───────────────────────────────────────────────────
const handleSelectNode = (id: string) => {
  selectedNodeId.value = id;
};
const toggleGraphKind = (kind: string) => {
  const next = new Set(hiddenGraphKinds.value);
  if (next.has(kind)) {
    next.delete(kind);
  } else {
    next.add(kind);
  }
  hiddenGraphKinds.value = next;
  if (selectedNode.value && next.has(selectedNode.value.kind)) {
    selectedNodeId.value =
      graph.value.nodes.find((node) => !next.has(node.kind))?.id ?? "";
  }
};
const resetGraphKindFilters = () => {
  hiddenGraphKinds.value = new Set();
};
const handleProcessPicked = (processes: ProcessInfo[]) => {
  const p = processes[0];
  if (p) void focusProcessBase(p.pid);
};
const focusRelatedTab = (tab: DetailTab) => {
  activeDetailTab.value = tab;
};
const focusProcessFromNode = async () => {
  const pid = Number(
    nearestProcessNode.value?.metadata?.pid ??
      nearestProcessNode.value?.pid ??
      0,
  );
  if (!pid) {
    message.warning("Select a process-related node first");
    return;
  }
  await focusProcessBase(pid);
  message.success(`Listening to process tree for pid ${pid}`);
};
const addRule = async (action: "ALLOW" | "BLOCK") => {
  const comm = actionableComm.value;
  if (!comm) {
    message.warning("Select a process-related node first");
    return;
  }
  try {
    await axios.post("/config/rules", { comm, action, rewritten_cmd: [] });
    message.success(`${action} rule added for ${comm}`);
  } catch (e) {
    console.error("Failed to add rule", e);
    message.error(`Failed to add ${action} rule`);
  }
};
const exportTrainingSample = async (label: "ALLOW" | "ALERT" | "BLOCK") => {
  const comm = actionableComm.value;
  if (!comm) {
    message.warning("Select a process-related node first");
    return;
  }
  try {
    await axios.post("/config/ml/samples", {
      commandLine: comm,
      comm,
      args: [],
      label,
    });
    message.success(`${label} sample exported for ${comm}`);
  } catch (e) {
    console.error("Failed to export training sample", e);
    message.error("Failed to export training sample");
  }
};
const replaySelectedContext = async () => {
  if (!selectedNode.value) return;
  const m = selectedNode.value.metadata ?? {};
  Object.assign(filters, {
    agentRunId: m.agentRunId ?? filters.agentRunId,
    toolCallId: m.toolCallId ?? filters.toolCallId,
    traceId: m.traceId ?? filters.traceId,
    pid: m.pid ?? filters.pid,
  });
  await applyFilters();
  message.success("Replayed current graph context filters");
};
onMounted(async () => {
  setupMonitorData();
  startRecordingStatusPolling();
  connectGraphSocket();
});
onUnmounted(() => {
  teardownMonitorData();
  cleanupRecording();
});
</script>
<template>
  <div class="execution-graph-page">
    <a-card :bordered="false" class="hero-card">
      <div class="hero-header">
        <div>
          <a-typography-title :level="3" style="margin-bottom: 8px">
            <ClusterOutlined /> 追踪
          </a-typography-title>
          <a-typography-paragraph type="secondary" style="margin-bottom: 0">
            将执行拓扑、行为追踪、录制与回放集中在同一个追踪工作台中。
          </a-typography-paragraph>
        </div>
        <a-space wrap>
          <a-badge status="processing" :text="`Source: ${graph.source}`" />
          <a-tag color="purple">{{ graph.eventCount }} matched events</a-tag>
          <a-tag color="blue">{{ graph.nodes.length }} nodes</a-tag>
          <a-tag color="geekblue">{{ graph.edges.length }} edges</a-tag>
          <a-tag v-if="lastLoadedAt" color="default"
            >Updated {{ lastLoadedAt }}</a-tag
          >
        </a-space>
      </div>
    </a-card>

    <a-tabs v-model:activeKey="activeTraceTab" class="trace-tabs">
      <a-tab-pane key="topology" tab="执行拓扑">
        <a-row :gutter="16" class="summary-row">
          <a-col :xs="24" :lg="8">
            <a-card size="small" title="Top Node Kinds">
              <a-space wrap>
                <a-tag
                  v-for="[kind, count] in sortedNodeCounts.slice(0, 8)"
                  :key="kind"
                  :color="kindTagColorMap[kind] || 'default'"
                >
                  {{ kind }} · {{ count }}
                </a-tag>
              </a-space>
            </a-card>
          </a-col>
          <a-col :xs="24" :lg="8">
            <a-card size="small" title="Top Edge Kinds">
              <a-space wrap>
                <a-tag
                  v-for="[kind, count] in sortedEdgeCounts.slice(0, 8)"
                  :key="kind"
                  color="processing"
                >
                  {{ kind }} · {{ count }}
                </a-tag>
              </a-space>
            </a-card>
          </a-col>
          <a-col :xs="24" :lg="8">
            <a-card size="small" title="Time Scope">
              <a-space direction="vertical" size="small">
                <span
                  ><b>Preset:</b>
                  {{ timePresetLabels[filters.timePreset] }}</span
                >
                <span v-if="filters.timePreset === 'custom'"
                  ><b>Since:</b> {{ filters.since || "—" }}</span
                >
                <span v-if="filters.timePreset === 'custom'"
                  ><b>Until:</b> {{ filters.until || "—" }}</span
                >
                <span v-else-if="filters.timePreset !== 'all'"
                  ><b>Computed since:</b>
                  {{ buildPresetSince(filters.timePreset) }}</span
                >
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-card :bordered="false" class="process-listener-card">
          <template #title
            ><span><RadarChartOutlined /> Process Tree Listener</span></template
          >
          <a-row :gutter="12" align="middle">
            <a-col :xs="24" :lg="10">
              <a-space wrap>
                <a-button type="primary" @click="processPickerOpen = true">
                  从进程列表选择
                </a-button>
                <a-button v-if="filters.pid" @click="focusProcessBase(null)"
                  >清除 PID</a-button
                >
                <a-tag v-if="filters.pid" color="processing"
                  >PID {{ filters.pid }}</a-tag
                >
              </a-space>
            </a-col>
            <a-col :xs="24" :lg="8">
              <a-typography-text type="secondary">{{
                selectedProcessSummary
              }}</a-typography-text>
            </a-col>
            <a-col :xs="24" :lg="6">
              <a-space wrap>
                <a-switch
                  v-model:checked="liveListen"
                  checked-children="监听"
                  un-checked-children="暂停"
                />
                <a-checkbox
                  v-model:checked="filters.processTree"
                  :disabled="!filters.pid"
                  @change="applyFilters"
                  >显示子进程调用树</a-checkbox
                >
                <a-button
                  size="small"
                  :disabled="!nearestProcessNode"
                  @click="focusProcessFromNode"
                  >监听当前节点 PID</a-button
                >
              </a-space>
            </a-col>
          </a-row>
        </a-card>

        <ProcessPickerModal
          v-model:open="processPickerOpen"
          :processes="processList"
          :selected-pids="selectedProcessPid != null ? [selectedProcessPid] : []"
          :loading="processLoading"
          title="选择要监听的进程"
          @select="(ps: ProcessInfo[]) => handleProcessPicked(ps)"
        />

        <a-card :bordered="false" class="filter-card">
          <template #title
            ><span><FilterOutlined /> Graph Filters</span></template
          >
          <a-form layout="vertical">
            <div class="filter-grid">
              <a-form-item label="Agent Run ID"
                ><a-input
                  v-model:value="filters.agentRunId"
                  allow-clear
                  placeholder="run-..."
              /></a-form-item>
              <a-form-item label="Tool Call ID"
                ><a-input
                  v-model:value="filters.toolCallId"
                  allow-clear
                  placeholder="tool-..."
              /></a-form-item>
              <a-form-item label="Trace ID"
                ><a-input
                  v-model:value="filters.traceId"
                  allow-clear
                  placeholder="trace-..."
              /></a-form-item>
              <a-form-item label="PID"
                ><a-input
                  v-model:value="filters.pid"
                  allow-clear
                  placeholder="101"
              /></a-form-item>
              <a-form-item label="Command"
                ><a-input
                  v-model:value="filters.comm"
                  allow-clear
                  placeholder="bash / git / python"
              /></a-form-item>
              <a-form-item label="Tool Name"
                ><a-input
                  v-model:value="filters.toolName"
                  allow-clear
                  placeholder="read_file / bash / npm"
              /></a-form-item>
              <a-form-item label="Path"
                ><a-input
                  v-model:value="filters.path"
                  allow-clear
                  placeholder="/workspace or id_rsa"
              /></a-form-item>
              <a-form-item label="Domain / Endpoint"
                ><a-input
                  v-model:value="filters.domain"
                  allow-clear
                  placeholder="github.com or :443"
              /></a-form-item>
              <a-form-item label="Decision"
                ><a-select
                  v-model:value="filters.decision"
                  :options="decisionOptions"
              /></a-form-item>
              <a-form-item label="Minimum Risk Score"
                ><a-input-number
                  v-model:value="filters.riskMin"
                  :min="0"
                  :max="100"
                  :step="5"
                  style="width: 100%"
              /></a-form-item>
              <a-form-item label="Event Limit"
                ><a-input-number
                  v-model:value="filters.limit"
                  :min="50"
                  :max="2000"
                  :step="50"
                  style="width: 100%"
              /></a-form-item>
              <a-form-item label="Time Range Preset"
                ><a-select
                  v-model:value="filters.timePreset"
                  :options="
                    timePresetOptions.map((value) => ({
                      label: timePresetLabels[value],
                      value,
                    }))
                  "
              /></a-form-item>
              <a-form-item
                v-if="filters.timePreset === 'custom'"
                label="Since (RFC3339 / unix ms)"
                ><a-input
                  v-model:value="filters.since"
                  allow-clear
                  placeholder="2026-05-08T10:00:00Z"
              /></a-form-item>
              <a-form-item
                v-if="filters.timePreset === 'custom'"
                label="Until (RFC3339 / unix ms)"
                ><a-input
                  v-model:value="filters.until"
                  allow-clear
                  placeholder="2026-05-08T12:00:00Z"
              /></a-form-item>
            </div>
          </a-form>
          <div class="filter-actions">
            <a-space wrap>
              <a-button type="primary" :loading="loading" @click="applyFilters"
                ><ReloadOutlined /> Refresh Graph</a-button
              >
              <a-button @click="resetFilters">Reset Filters</a-button>
              <a-button
                :disabled="!replayAvailable"
                @click="replaySelectedContext"
                ><PlayCircleOutlined /> Replay This Run</a-button
              >
            </a-space>
          </div>
        </a-card>

        <div class="graph-layout">
          <a-card :bordered="false" class="graph-card">
            <template #title>
              <a-space wrap>
                <span>Execution Topology</span>
                <a-tag color="green"
                  >process tree {{ processTreeNodes.length }}</a-tag
                >
                <a-tag color="cyan"
                  >chain edges {{ processTreeEdges.length }}</a-tag
                >
              </a-space>
            </template>
            <template #extra>
              <a-space wrap>
                <a-tag
                  v-for="item in graphKindFilters"
                  :key="item.kind"
                  :color="item.hidden ? 'default' : item.color"
                  class="graph-kind-tag"
                  :class="{ 'graph-kind-tag-hidden': item.hidden }"
                  @click="toggleGraphKind(item.kind)"
                >
                  {{ item.hidden ? "显示" : "隐藏" }} {{ item.label }} ·
                  {{ item.count }}
                </a-tag>
                <a-button
                  v-if="hasHiddenGraphKinds"
                  size="small"
                  type="link"
                  @click="resetGraphKindFilters"
                >
                  全部显示
                </a-button>
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
                :nodes="visibleGraphNodes"
                :edges="visibleGraphEdges"
                :selected-node-id="selectedNodeId"
                zoom-storage-key="agent-ebpf.execution-graph.execution-topology.zoom"
                @select-node="handleSelectNode"
              />
            </a-spin>
          </a-card>
          <a-card :bordered="false" class="detail-card">
            <template #title
              ><span><InfoCircleOutlined /> Node Details</span></template
            >
            <template #extra>
              <a-space v-if="selectedNode">
                <a-tag :color="selectedNodeKindColor">{{
                  selectedNode.kind
                }}</a-tag>
                <a-tag
                  v-if="selectedNode.riskScore !== undefined"
                  color="volcano"
                  >risk {{ Number(selectedNode.riskScore).toFixed(0) }}</a-tag
                >
              </a-space>
            </template>
            <a-empty
              v-if="!selectedNode"
              description="Select a node from the graph to inspect context, resources, and actions."
            />
            <template v-else>
              <a-space direction="vertical" size="middle" style="width: 100%">
                <div>
                  <a-typography-title :level="5" style="margin-bottom: 6px">{{
                    selectedNode.label
                  }}</a-typography-title>
                  <a-typography-paragraph
                    type="secondary"
                    style="margin-bottom: 0"
                  >
                    {{ renderNodeSubtitle(selectedNode) }}
                  </a-typography-paragraph>
                </div>
                <a-descriptions :column="1" size="small" bordered>
                  <a-descriptions-item label="Node ID">{{
                    selectedNode.id
                  }}</a-descriptions-item>
                  <a-descriptions-item label="Kind">{{
                    selectedNode.kind
                  }}</a-descriptions-item>
                  <a-descriptions-item v-if="selectedNode.pid" label="PID">{{
                    selectedNode.pid
                  }}</a-descriptions-item>
                  <a-descriptions-item
                    v-if="actionableComm"
                    label="Actionable Command"
                    >{{ actionableComm }}</a-descriptions-item
                  >
                </a-descriptions>
                <div class="node-actions">
                  <a-space wrap>
                    <a-button size="small" @click="addRule('ALLOW')"
                      ><SafetyCertificateOutlined /> Add allow rule</a-button
                    >
                    <a-button size="small" danger @click="addRule('BLOCK')"
                      ><StopOutlined /> Add block rule</a-button
                    >
                    <a-button
                      size="small"
                      @click="exportTrainingSample('ALLOW')"
                      >Mark benign</a-button
                    >
                    <a-button
                      size="small"
                      type="primary"
                      ghost
                      @click="exportTrainingSample('ALERT')"
                      ><AlertOutlined /> Mark suspicious</a-button
                    >
                    <a-button
                      size="small"
                      type="dashed"
                      @click="exportTrainingSample('BLOCK')"
                      >Export BLOCK sample</a-button
                    >
                  </a-space>
                </div>
                <a-space wrap>
                  <a-button size="small" @click="focusRelatedTab('processes')"
                    >Show related process tree</a-button
                  >
                  <a-button size="small" @click="focusRelatedTab('files')"
                    >Show related files</a-button
                  >
                  <a-button size="small" @click="focusRelatedTab('network')"
                    >Show related network flows</a-button
                  >
                  <a-button size="small" @click="focusRelatedTab('policy')"
                    >Show related policy events</a-button
                  >
                </a-space>
                <a-tabs v-model:activeKey="activeDetailTab" size="small">
                  <a-tab-pane
                    key="processes"
                    :tab="`Processes (${relatedProcesses.length})`"
                  >
                    <a-list
                      size="small"
                      :data-source="
                        selectedNode?.kind === 'process'
                          ? processTreeNodes
                          : relatedProcesses
                      "
                      bordered
                    >
                      <template #renderItem="{ item }">
                        <a-list-item
                          @click="selectedNodeId = item.id"
                          class="clickable-list-item"
                        >
                          <a-space direction="vertical" size="small">
                            <span
                              ><b>{{ item.label }}</b>
                              <a-tag color="green">process</a-tag></span
                            >
                            <span class="muted-line">{{
                              renderNodeSubtitle(item)
                            }}</span>
                          </a-space>
                        </a-list-item>
                      </template>
                    </a-list>
                  </a-tab-pane>
                  <a-tab-pane
                    key="files"
                    :tab="`Files (${relatedFiles.length})`"
                  >
                    <a-list size="small" :data-source="relatedFiles" bordered>
                      <template #renderItem="{ item }">
                        <a-list-item
                          @click="selectedNodeId = item.id"
                          class="clickable-list-item"
                        >
                          <a-space direction="vertical" size="small">
                            <span
                              ><b>{{ item.label }}</b></span
                            >
                            <span class="muted-line">{{
                              item.metadata?.path || "file access"
                            }}</span>
                          </a-space>
                        </a-list-item>
                      </template>
                    </a-list>
                  </a-tab-pane>
                  <a-tab-pane
                    key="network"
                    :tab="`Network (${relatedNetwork.length})`"
                  >
                    <a-list size="small" :data-source="relatedNetwork" bordered>
                      <template #renderItem="{ item }">
                        <a-list-item
                          @click="selectedNodeId = item.id"
                          class="clickable-list-item"
                        >
                          <a-space direction="vertical" size="small">
                            <span
                              ><b>{{ item.label }}</b></span
                            >
                            <span class="muted-line">{{
                              item.subtitle ||
                              item.metadata?.domain ||
                              "network relation"
                            }}</span>
                          </a-space>
                        </a-list-item>
                      </template>
                    </a-list>
                  </a-tab-pane>
                  <a-tab-pane
                    key="policy"
                    :tab="`Policy (${relatedPolicies.length})`"
                  >
                    <a-list
                      size="small"
                      :data-source="relatedPolicies"
                      bordered
                    >
                      <template #renderItem="{ item }">
                        <a-list-item
                          @click="selectedNodeId = item.id"
                          class="clickable-list-item"
                        >
                          <a-space direction="vertical" size="small">
                            <span>
                              <b>{{ item.label }}</b>
                              <a-tag
                                :color="
                                  item.kind === 'policy_alert'
                                    ? 'error'
                                    : 'default'
                                "
                                >{{ item.kind }}</a-tag
                              >
                            </span>
                            <span class="muted-line">{{
                              renderNodeSubtitle(item)
                            }}</span>
                          </a-space>
                        </a-list-item>
                      </template>
                    </a-list>
                  </a-tab-pane>
                  <a-tab-pane
                    key="edges"
                    :tab="`Edges (${incidentEdges.length})`"
                  >
                    <a-list size="small" :data-source="incidentEdges" bordered>
                      <template #renderItem="{ item }">
                        <a-list-item>
                          <a-space direction="vertical" size="small">
                            <span
                              ><b>{{ item.kind }}</b></span
                            >
                            <span class="muted-line"
                              >{{ item.source }} → {{ item.target }}</span
                            >
                          </a-space>
                        </a-list-item>
                      </template>
                    </a-list>
                  </a-tab-pane>
                  <a-tab-pane key="metadata" tab="Metadata">
                    <a-list
                      size="small"
                      :data-source="metadataEntries"
                      bordered
                    >
                      <template #renderItem="{ item }">
                        <a-list-item>
                          <div class="metadata-row">
                            <span class="metadata-key">{{ item[0] }}</span>
                            <span class="metadata-value">{{
                              item[1] || "—"
                            }}</span>
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
      </a-tab-pane>

      <a-tab-pane key="behavior" tab="行为追踪">
        <AgentSightTracePanel :pid="filters.pid" :comm="filters.comm" />
      </a-tab-pane>

      <a-tab-pane key="recording" tab="录制 / 回放">
        <ExecutionGraphRecordingPanel
          :recording="recording"
          :browser-recording-active="browserRecordingActive"
          :browser-replay-active="browserReplayActive"
          :browser-snapshot-count="browserSnapshotCount"
          :browser-recording-summary="browserRecordingSummary"
          :replay-enabled="replayEnabled"
        />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>
<style scoped src="./execution-graph.css"></style>
