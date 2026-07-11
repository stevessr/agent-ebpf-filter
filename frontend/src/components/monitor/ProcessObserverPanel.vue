<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, watch, computed } from "vue";
import axios from "axios";
import {
  SearchOutlined,
  ReloadOutlined,
  NodeIndexOutlined,
  CaretDownOutlined,
  EyeOutlined,
  MergeCellsOutlined,
  PlayCircleOutlined,
} from "@ant-design/icons-vue";
import {
  useProcessObserver,
  type ProcessInfo,
  type ProcessTreeNode,
  type ObserverTLSEvent,
  type EventIgnoreRule,
} from "../../composables/monitor/useProcessObserver";
import ProcessPickerModal from "./ProcessPickerModal.vue";
import ProcessTreeNodeDisplay from "./ProcessTreeNodeDisplay.vue";
import ProcessDetailModal from "./ProcessDetailModal.vue";
import ObserverTimeline from "./observer/ObserverTimeline.vue";
import ObserverFlamegraph from "./observer/ObserverFlamegraph.vue";
import ObserverResources from "./observer/ObserverResources.vue";
import AgentContextPanel from "./observer/AgentContextPanel.vue";
import FilePathBrowserModal from "./FilePathBrowserModal.vue";
import SSLDecryptedEventModal from "./observer/SSLDecryptedEventModal.vue";
import LaunchProgramModal from "./observer/LaunchProgramModal.vue";
import {
  TAB_STORAGE_KEY,
  subTabKeys,
  bestTLSPreview,
  classifySSLLib,
  collectAllPids,
  createExpandedTLSRowRender,
  eventColumns,
  formatBytes,
  formatTLSBodyPreview,
  getRouteParam,
  looksLikeJSON,
  networkFlowColumns,
  normalizeObserveTab,
  sslAttachmentColumns,
  tcpConnColumns,
  tlsColumns,
  type SubTabKey,
} from "./processObserverViewHelpers";
import { useObserverTLSPreferences } from "./useObserverTLSPreferences";
import { useProgramLauncher } from "./useProgramLauncher";
import { useRoute, useRouter } from "vue-router";

// ── Props ────────────────────────────────────────────────────────────────
const props = defineProps<{
  processes: ProcessInfo[];
  sendProcessSignal: (pid: number, signal: string) => void;
  isActive: boolean;
  memTotal?: number;
}>();

// ── Sub-tab state with URL routing ────────────────────────────────────────
const route = useRoute();
const router = useRouter();

// Initialize from URL param, falling back to localStorage → "selection"
const urlTab = getRouteParam(route.params.tab);
const initialTab = normalizeObserveTab(urlTab);
const activeSubTab = ref<SubTabKey>(initialTab);

// If URL is missing or invalid, rewrite it with the resolved tab
if (!urlTab || normalizeObserveTab(urlTab) !== initialTab) {
  router.replace({ name: "Observe", params: { tab: initialTab } });
}

// Tab change handler – syncs to URL + localStorage
const handleTabChange = (key: string) => {
  const tab = normalizeObserveTab(key);
  activeSubTab.value = tab;
  router.replace({ name: "Observe", params: { tab } });
  try { localStorage.setItem(TAB_STORAGE_KEY, tab); } catch { /* ignore */ }
};

// Back/forward navigation – sync tab from URL
watch(() => route.params.tab, (param) => {
  const tab = normalizeObserveTab(getRouteParam(param));
  if (tab !== activeSubTab.value) {
    activeSubTab.value = tab;
  }
});

// ── Composable ───────────────────────────────────────────────────────────
const obs = useProcessObserver();
const {
  selectedPids,
  showPicker,
  processTree,
  selectedProcessTree,
  treePids,
  treeProcessList,
  treeNetworkEvents,
  treeSyscallEvents,
  treeFileAccessEvents,
  treeNetworkFlows,
  treeTCPConns,
  treeTLSEvents,
  allEvents,
  tlsEvents,
  setProcesses,
  connectAll,
  disconnectAll,
  loadAllInitial,
  // Multi-select helpers
  addPid,
  removePid,
  togglePid,
  clearPids,
  hasPid,
  // Clear + SSL
  clearEvents,
  clearTLSEvents,
  clearNetworkFlows,
  clearTCPConns,
  attachedPIDs,
  fetchAttachedPIDs,
  // Ignore rules for timeline
  ignoreRules,
  toggleTimelineIgnoreRule,
  removeTimelineIgnoreRule,
  resetTimelineIgnoreRules,
} = obs;

setProcesses(props.processes);
watch(
  () => props.processes,
  (p) => setProcesses(p),
);

// ── PID input ────────────────────────────────────────────────────────────
const pidInput = ref("");
const pidInvalid = ref(false);

// Persist selected PIDs across page refreshes
const PIDS_STORAGE_KEY = "observe-selected-pids";

const restorePidsFromStorage = () => {
  try {
    const raw = localStorage.getItem(PIDS_STORAGE_KEY);
    if (raw) {
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) {
        const restored = arr.filter((n: any) => typeof n === "number" && n > 0);
        if (restored.length > 0) {
          selectedPids.value = new Set(restored);
          return;
        }
      }
    }
  } catch { /* ignore */ }
  // Migration: old single-PID storage
  try {
    const raw = localStorage.getItem("observe-selected-pid");
    if (raw) {
      const parsed = parseInt(raw, 10);
      if (!isNaN(parsed) && parsed > 0) {
        selectedPids.value = new Set([parsed]);
        localStorage.removeItem("observe-selected-pid");
      }
    }
  } catch { /* ignore */ }
};
restorePidsFromStorage();

watch(selectedPids, (pids) => {
  try {
    if (pids.size > 0) {
      localStorage.setItem(PIDS_STORAGE_KEY, JSON.stringify([...pids]));
    } else {
      localStorage.removeItem(PIDS_STORAGE_KEY);
    }
  } catch { /* ignore */ }
}, { deep: true });

const onPidSearch = () => {
  const val = parseInt(pidInput.value, 10);
  if (!isNaN(val) && val > 0) {
    pidInvalid.value = false;
    addPid(val);
    pidInput.value = "";
  } else if (pidInput.value.trim()) {
    pidInvalid.value = true;
  }
};

const onClearSelection = () => {
  clearPids();
  pidInput.value = "";
  pidInvalid.value = false;
};

const selectedLabels = computed(() => {
  const labels: string[] = [];
  for (const pid of selectedPids.value) {
    const p = props.processes.find((x) => x.pid === pid);
    labels.push(p ? `[${p.pid}] ${p.name}` : `PID ${pid}`);
  }
  return labels;
});

// ── Launch controls ──────────────────────────────────────────────────────

const {
  launchModalOpen,
  launchPath,
  launchUser,
  launchCwd,
  launchArgs,
  launching,
  launchError,
  browserTarget,
  browserOpen,
  browserStartPath,
  sysUsers,
  usersLoading,
  recentLaunches,
  applyRecent,
  openBrowser,
  onBrowserSelect,
  doLaunch,
} = useProgramLauncher({ addPid, pidInput });

// ── Persistent SSL display / auto-attach toggles ────────────────────────

const { skipSSL, autoAttach } = useObserverTLSPreferences();

// Filtered TLS events (respects skipSSL toggle)
const visibleTLSEvents = computed(() =>
  skipSSL.value ? [] : treeTLSEvents.value
);

// Auto-attach tracked PIDs that were already attempted (avoid infinite retry)
const autoAttachSeen = new Set<number>();

// ── Manual SSL attach ────────────────────────────────────────────────────

const attachingPids = reactive(new Set<number>());
const attachErrors = reactive<Record<number, string>>({});

const getBinaryPath = async (pid: number): Promise<string> => {
  // Always resolve via /proc/PID/exe for the real binary, not cmdline
  try {
    const res = await axios.get("/system/process/exe", { params: { pid } });
    return res.data.path || "";
  } catch {
    // Fallback: try cmdline
    const p = treeProcessList.value.find((x) => x.pid === pid);
    if (p?.cmdline) {
      const parts = p.cmdline.split(/\s+/);
      if (parts[0]) return parts[0];
    }
    return "";
  }
};

const doAttachBuiltins = async (pid: number) => {
  if (attachingPids.has(pid)) return;
  attachingPids.add(pid);
  delete attachErrors[pid];
  try {
    const exePath = await getBinaryPath(pid);
    if (!exePath) { attachErrors[pid] = "Cannot resolve binary path for PID " + pid; return; }
    // Use executable API which auto-detects: Go uprobes → static SSL → library (openssl/gnutls)
    const res = await axios.post("/tls-capture/executable", { path: exePath, pid, library: "" });
    if (res.data.result?.error) {
      attachErrors[pid] = res.data.result.error;
    } else {
      await fetchAttachedPIDs();
    }
  } catch (e: any) {
    attachErrors[pid] = e?.response?.data?.error || e?.message || "Auto-attach failed";
  } finally {
    attachingPids.delete(pid);
  }
};

const doAttachGo = async (pid: number) => {
  if (attachingPids.has(pid)) return;
  attachingPids.add(pid);
  delete attachErrors[pid];
  try {
    const path = await getBinaryPath(pid);
    if (!path) { attachErrors[pid] = "Cannot determine binary path for PID " + pid; return; }
    await axios.post("/tls-capture/go-binary", { path, pid });
    await fetchAttachedPIDs();
  } catch (e: any) {
    attachErrors[pid] = e?.response?.data?.error || e?.message || "Go attach failed";
  } finally {
    attachingPids.delete(pid);
  }
};

const doAttachLibrary = async (pid: number, library: string) => {
  if (attachingPids.has(pid)) return;
  attachingPids.add(pid);
  delete attachErrors[pid];
  try {
    const path = await getBinaryPath(pid);
    if (!path) { attachErrors[pid] = "Cannot determine binary path for PID " + pid; return; }
    await axios.post("/tls-capture/executable", { path, pid, library });
    await fetchAttachedPIDs();
  } catch (e: any) {
    attachErrors[pid] = e?.response?.data?.error || e?.message || "Library attach failed";
  } finally {
    attachingPids.delete(pid);
  }
};

const doAttachAllBuiltins = async () => {
  const pids = treeSSLPending.value.map((p) => p.pid);
  await Promise.all(pids.map((pid) => doAttachBuiltins(pid)));
};

// ── Helpers ──────────────────────────────────────────────────────────────

const tlsDetailOpen = ref(false);
const tlsDetailEvent = ref<ObserverTLSEvent | null>(null);

const openTLSDetail = (event: ObserverTLSEvent) => {
  tlsDetailEvent.value = event;
  tlsDetailOpen.value = true;
};

// Expanded row render for TLS events — shows decoded body + headers
const expandedTLSRowRender = createExpandedTLSRowRender(openTLSDetail);

const detailModalOpen = ref(false);
const detailModalProcess = ref<ProcessInfo | null>(null);

const openProcessDetail = (node: ProcessTreeNode) => {
  // Look up full ProcessInfo from the processes list
  const info = props.processes.find((p) => p.pid === node.pid)
    || treeProcessList.value.find((p) => p.pid === node.pid)
    || null;
  detailModalProcess.value = info;
  detailModalOpen.value = true;
};

// ── Recursive tree state ─────────────────────────────────────────────────

const expandedNodes = ref<Set<number>>(new Set());

const toggleExpand = (pid: number) => {
  const s = new Set(expandedNodes.value);
  if (s.has(pid)) s.delete(pid);
  else s.add(pid);
  expandedNodes.value = s;
};

// Auto-expand ancestors of all selected PIDs
watch(selectedPids, (pids) => {
  if (pids.size > 0) {
    const expandTo = (nodes: ProcessTreeNode[], target: number): boolean => {
      for (const n of nodes) {
        if (n.pid === target || expandTo(n.children, target)) {
          expandedNodes.value = new Set([...expandedNodes.value, n.pid]);
          return true;
        }
      }
      return false;
    };
    for (const pid of pids) {
      expandTo(processTree.value, pid);
    }
  }
}, { deep: true });

// SSL attachment status per PID
const sslAttachedSet = computed<Set<number>>(
  () => new Set(attachedPIDs.value.map((a: any) => a.pid)),
);
const sslLibForPid = (pid: number): string => {
  const a = attachedPIDs.value.find((x: any) => x.pid === pid);
  return a ? a.library_name || "attached" : "";
};

// Classify SSL/TLS library by name
// SSL attachments filtered to current tree, enriched with process name
const treeSSLAttachments = computed(() =>
  attachedPIDs.value
    .filter((a: any) => treePids.value.has(a.pid))
    .map((a: any) => ({
      ...a,
      comm: treeProcessList.value.find((p) => p.pid === a.pid)?.name || "",
    })),
);

// Tree processes missing SSL attachment
const treeSSLPending = computed(() =>
  treeProcessList.value.filter(
    (p) => !sslAttachedSet.value.has(p.pid),
  ),
);

// Auto-attach tracked PIDs that are pending SSL
watch(treeSSLPending, (pending) => {
  if (!autoAttach.value) return;
  for (const p of pending) {
    if (autoAttachSeen.has(p.pid)) continue;
    autoAttachSeen.add(p.pid);
    // Small stagger delay so we don't flood the backend
    setTimeout(() => doAttachBuiltins(p.pid), 100 * autoAttachSeen.size);
  }
}, { deep: false });

// ── Lifecycle ────────────────────────────────────────────────────────────

onMounted(() => {
  if (props.isActive) {
    connectAll();
    loadAllInitial();
    fetchAttachedPIDs();
  }
});

onUnmounted(() => {
  disconnectAll();
});

watch(
  () => props.isActive,
  (active) => {
    if (active) {
      connectAll();
      loadAllInitial();
      fetchAttachedPIDs();
    } else {
      disconnectAll();
    }
  },
);
</script>

<template>
  <div class="observer-panel">
    <a-tabs
      :activeKey="activeSubTab"
      @change="handleTabChange"
      size="small"
      type="card"
      class="observer-tabs"
    >
      <!-- 1. Selection -->
      <a-tab-pane key="selection">
        <template #tab><SearchOutlined /> Selection</template>
        <div class="selection-container">
          <!-- Existing PID / picker row -->
          <div class="selection-bar">
            <a-input-search
              v-model:value="pidInput"
              placeholder="Enter PID..."
              :status="pidInvalid ? 'error' : undefined"
              style="width: 160px"
              size="small"
              @search="onPidSearch"
            />
            <a-button size="small" @click="showPicker = true">
              Pick from list...
            </a-button>
            <a-button
              v-if="selectedPids.size > 0"
              size="small"
              danger
              type="link"
              @click="onClearSelection"
            >
              Clear All
            </a-button>
            <span v-if="selectedPids.size === 0" style="color: #6b7280; font-size: 12px">
              No PID selected
            </span>
          </div>

          <!-- Selected PIDs as tags -->
          <div v-if="selectedPids.size > 0" class="multi-pid-tags">
            <a-tag
              v-for="label in selectedLabels"
              :key="label"
              color="processing"
              closable
              @close="removePid(parseInt(label.match(/\[?(\d+)\]?/)?.[1] || '0'))"
            >
              {{ label }}
            </a-tag>
          </div>

          <!-- Launch program via wrapper — button opens modal -->
          <a-divider style="margin: 12px 0; font-size: 12px; color: #4b5563">
            Launch &amp; Observe
          </a-divider>
          <div style="display: flex; align-items: center; gap: 8px">
            <a-button
              type="primary"
              size="small"
              @click="launchModalOpen = true"
            >
              <template #icon><PlayCircleOutlined /></template>
              Launch Program
            </a-button>
            <span style="font-size: 12px; color: #6b7280">
              Launch a new process and observe it
            </span>
          </div>
        </div>
      </a-tab-pane>

      <!-- 2. Timeline (NEW) -->
      <a-tab-pane key="timeline">
        <template #tab>Timeline</template>
        <ObserverTimeline
          :events="allEvents"
          :tlsEvents="treeTLSEvents"
          :selectedPid="selectedPids.size > 0 ? [...selectedPids][0] : null"
          :ignoreRules="ignoreRules"
          @clear="clearEvents()"
          @selectPid="(pid: number) => togglePid(pid)"
          @viewTLSEvent="(e: ObserverTLSEvent) => openTLSDetail(e)"
          @toggleIgnoreRule="toggleTimelineIgnoreRule"
          @removeIgnoreRule="removeTimelineIgnoreRule"
          @resetIgnoreRules="resetTimelineIgnoreRules"
        />
      </a-tab-pane>

      <!-- 3. Flamegraph -->
      <a-tab-pane key="flamegraph">
        <template #tab>Flamegraph</template>
        <ObserverFlamegraph :events="allEvents" />
      </a-tab-pane>

      <!-- 4. Process Tree -->
      <a-tab-pane key="tree">
        <template #tab><NodeIndexOutlined /> Tree</template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view its process tree"
        />
        <div v-else class="tree-container">
          <div style="margin-bottom: 8px">
            <a-button
              size="small"
              type="link"
              @click="
                expandedNodes = new Set(collectAllPids(selectedProcessTree))
              "
            >
              Expand All
            </a-button>
            <a-button
              size="small"
              type="link"
              @click="expandedNodes = new Set()"
            >
              Collapse All
            </a-button>
          </div>
          <ProcessTreeNodeDisplay
            v-for="node in selectedProcessTree"
            :key="node.pid"
            :node="node"
            :depth="0"
            :highlight-pids="selectedPids"
            :expanded-set="expandedNodes"
            :ssl-attached-set="sslAttachedSet"
            :ssl-lib-for-pid="sslLibForPid"
            @toggle="toggleExpand"
            @select="(pid: number) => togglePid(pid)"
            @show-detail="openProcessDetail"
          />
        </div>
      </a-tab-pane>

      <!-- 5. Network -->
      <a-tab-pane key="network">
        <template #tab><ReloadOutlined /> Network</template>
        <template #tabBarExtraContent>
          <a-button
            size="small"
            type="link"
            danger
            @click="
              clearNetworkFlows();
              clearTCPConns();
            "
            style="padding: 0 4px; font-size: 11px"
            >Clear</a-button
          >
        </template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view network connections"
        />
        <template v-else>
          <div class="sub-section">
            <div class="sub-title">Flows</div>
            <a-table
              :dataSource="treeNetworkFlows"
              :columns="networkFlowColumns"
              row-key="flowId"
              size="small"
              :pagination="{ pageSize: 10, size: 'small' }"
            />
          </div>
          <div class="sub-section">
            <div class="sub-title">TCP Connections</div>
            <a-table
              :dataSource="treeTCPConns"
              :columns="tcpConnColumns"
              row-key="pid"
              size="small"
              :pagination="{ pageSize: 10, size: 'small' }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'local'">
                  {{ record.srcIp }}:{{ record.srcPort }}
                </template>
                <template v-else-if="column.key === 'remote'">
                  {{ record.dstIp }}:{{ record.dstPort }}
                </template>
              </template>
            </a-table>
          </div>
        </template>
      </a-tab-pane>

      <!-- 6. Syscalls -->
      <a-tab-pane key="syscalls">
        <template #tab>Syscalls</template>
        <template #tabBarExtraContent>
          <a-button
            size="small"
            type="link"
            danger
            @click="clearEvents()"
            style="padding: 0 4px; font-size: 11px"
            >Clear</a-button
          >
        </template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view syscalls"
        />
        <a-table
          v-else
          :dataSource="treeSyscallEvents"
          :columns="eventColumns"
          row-key="key"
          size="small"
          :pagination="{ pageSize: 20, size: 'small' }"
        >
          <template #bodyCell="{ column, text, record }">
            <a-tag v-if="column.key === 'type'" color="purple" size="small">
              {{ text?.toUpperCase?.() || "—" }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
            <span v-else-if="column.key === 'path'">
              <span v-if="text" :title="text">{{ text }}</span>
              <span v-else-if="record.extraInfo" class="path-fallback" :title="record.extraInfo">{{ record.extraInfo }}</span>
              <span v-else style="color: #6b7280">—</span>
            </span>
          </template>
        </a-table>
      </a-tab-pane>

      <!-- 7. File Access -->
      <a-tab-pane key="file-access">
        <template #tab>File Access</template>
        <template #tabBarExtraContent>
          <a-button
            size="small"
            type="link"
            danger
            @click="clearEvents()"
            style="padding: 0 4px; font-size: 11px"
            >Clear</a-button
          >
        </template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view file access events"
        />
        <a-table
          v-else
          :dataSource="treeFileAccessEvents"
          :columns="eventColumns"
          row-key="key"
          size="small"
          :pagination="{ pageSize: 20, size: 'small' }"
        >
          <template #bodyCell="{ column, text, record }">
            <a-tag v-if="column.key === 'type'" color="blue" size="small">
              {{ text?.toUpperCase?.() || "—" }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
            <span v-else-if="column.key === 'path'">
              <span v-if="text" :title="text">{{ text }}</span>
              <span v-else-if="record.extraInfo" class="path-fallback" :title="record.extraInfo">{{ record.extraInfo }}</span>
              <span v-else style="color: #6b7280">—</span>
            </span>
          </template>
        </a-table>
      </a-tab-pane>

      <!-- 8. Resources -->
      <a-tab-pane key="resources">
        <template #tab>Resources</template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view resource usage"
        />
        <ObserverResources v-else :processes="processes" :treePids="treePids" :mem-total="memTotal" />
      </a-tab-pane>

      <!-- 9. SSL -->
      <a-tab-pane key="ssl">
        <template #tab>SSL</template>
        <template #tabBarExtraContent>
          <a-switch
            v-model:checked="autoAttach"
            size="small"
            style="margin-right: 6px"
          >
            <template #checkedChildren>Auto</template>
            <template #unCheckedChildren>Manual</template>
          </a-switch>
          <a-switch
            v-model:checked="skipSSL"
            size="small"
            style="margin-right: 8px"
          >
            <template #checkedChildren>Skip</template>
            <template #unCheckedChildren>Show</template>
          </a-switch>
          <a-button
            size="small"
            type="link"
            @click="fetchAttachedPIDs()"
            style="padding: 0 4px; font-size: 11px"
            >Refresh</a-button
          >
          <a-button
            size="small"
            type="link"
            danger
            @click="clearTLSEvents()"
            style="padding: 0 4px; font-size: 11px"
            >Clear</a-button
          >
        </template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view SSL/TLS data"
        />
        <template v-else>
          <!-- Section 1: Uprobe captured TLS events -->
          <div class="sub-section">
            <div class="sub-title">
              Decrypted TLS Events
              <span class="sub-count">{{ visibleTLSEvents.length }}</span>
            </div>
            <a-table
              :dataSource="visibleTLSEvents"
              :columns="tlsColumns"
              row-key="key"
              size="small"
              :pagination="{ pageSize: 20, size: 'small' }"
              :expandable="{ expandedRowRender: (r: ObserverTLSEvent) => expandedTLSRowRender(r), rowExpandable: (r: ObserverTLSEvent) => !!(r.body || (r.headers && Object.keys(r.headers).length > 0)) }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'evType'">
                  <a-tag v-if="record.type === 'http_request'" color="blue" size="small">REQ</a-tag>
                  <a-tag v-else-if="record.type === 'http_response'" color="green" size="small">RESP</a-tag>
                  <a-tag v-else-if="record.type === 'sse_message'" color="purple" size="small">SSE</a-tag>
                  <a-tag v-else color="default" size="small">{{ record.type || 'raw' }}</a-tag>
                </template>
                <template v-else-if="column.key === 'url' && record.type === 'tls_plaintext'">
                  <a-tooltip :title="record.raw_hex_dump?.slice(0, 200)">
                    <span class="tls-hex-preview">{{ record.raw_hex_dump?.slice(0, 40) }}…</span>
                  </a-tooltip>
                </template>
                <template v-else-if="column.key === 'bodyPreview'">
                  <span
                    v-if="record.body || record.raw_hex_dump"
                    class="tls-body-preview"
                  >{{ bestTLSPreview(record, 80) }}</span>
                  <span v-else style="color: #6b7280; font-size: 11px">—</span>
                </template>
                <template v-else-if="column.key === 'size'">
                  <span>{{ formatBytes(record.captured_len) }}</span>
                </template>
                <template v-else-if="column.key === 'actions'">
                  <a-button
                    v-if="record.body || (record.headers && Object.keys(record.headers).length > 0)"
                    size="small"
                    type="link"
                    style="padding: 0"
                    @click="openTLSDetail(record)"
                  >
                    <EyeOutlined />
                  </a-button>
                </template>
              </template>
            </a-table>
          </div>

          <a-divider style="margin: 16px 0 12px; font-size: 12px; color: #4b5563">
            SSL Probe Attachment
          </a-divider>

          <!-- Section 2: Attached probes -->
          <div class="sub-section">
            <div class="sub-title">
              Active Probes
              <a-tag color="green" size="small">{{ treeSSLAttachments.length }}</a-tag>
            </div>
            <a-empty
              v-if="treeSSLAttachments.length === 0"
              description="No SSL probes attached to tree processes"
              style="padding: 12px"
            />
            <a-table
              v-else
              :dataSource="treeSSLAttachments"
              :columns="sslAttachmentColumns"
              row-key="pid"
              size="small"
              :pagination="false"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'comm'">
                  <span class="ssl-attach-comm">{{ record.comm || '—' }}</span>
                </template>
                <template v-else-if="column.key === 'libType'">
                  <a-tag :color="classifySSLLib(record.library_name || '').tagColor" size="small">
                    {{ classifySSLLib(record.library_name || '').type }}
                  </a-tag>
                </template>
                <template v-else-if="column.key === 'status'">
                  <a-badge status="processing" color="green" text="Active" />
                </template>
              </template>
            </a-table>
          </div>

          <!-- Section 3: Pending (not attached) processes -->
          <div v-if="treeSSLPending.length > 0" class="sub-section">
            <div class="sub-title">
              Not Attached
              <a-tag color="default" size="small">{{ treeSSLPending.length }}</a-tag>
            </div>
            <div v-if="Object.keys(attachErrors).length > 0" class="attach-errors">
              <div v-for="(err, pid) in attachErrors" :key="pid" class="attach-error">
                <code>[{{ pid }}]</code> {{ err }}
                <a-button size="small" type="link" @click="delete attachErrors[pid]">Dismiss</a-button>
              </div>
            </div>
            <div style="margin-bottom: 8px">
                <a-button size="small" type="primary" ghost
                  :loading="attachingPids.size > 0"
                  @click="doAttachAllBuiltins()"
                >Attach All ({{ treeSSLPending.length }})</a-button>
              </div>
            <div class="ssl-pending-list">
              <div
                v-for="p in treeSSLPending"
                :key="p.pid"
                class="ssl-pending-row"
              >
                <code class="ssl-pending-pid">{{ p.pid }}</code>
                <span class="ssl-pending-name">{{ p.name }}</span>
                <span class="ssl-pending-cmd" v-if="p.cmdline" :title="p.cmdline">
                  {{ p.cmdline.split(/\s+/)[0]?.split('/').pop() || p.cmdline.slice(0, 40) }}
                </span>
                <a-dropdown :trigger="['click']" placement="bottomRight">
                  <a-button
                    size="small"
                    type="dashed"
                    :loading="attachingPids.has(p.pid)"
                    style="margin-left: auto; font-size: 11px"
                  >
                    Attach <CaretDownOutlined />
                  </a-button>
                  <template #overlay>
                    <a-menu @click="({ key }: { key: string }) => {
                      if (key === 'builtins') doAttachBuiltins(p.pid);
                      else if (key === 'go') doAttachGo(p.pid);
                      else if (key.startsWith('lib:')) doAttachLibrary(p.pid, key.slice(4));
                    }">
                      <a-menu-item key="builtins">
                        <span class="attach-menu-item">🔍 Auto-detect (builtins)</span>
                      </a-menu-item>
                      <a-menu-divider />
                      <a-menu-item key="go">
                        <span class="attach-menu-item">🔷 Go crypto/tls</span>
                      </a-menu-item>
                      <a-menu-item key="lib:openssl">
                        <span class="attach-menu-item">🔒 OpenSSL</span>
                      </a-menu-item>
                      <a-menu-item key="lib:gnutls">
                        <span class="attach-menu-item">🛡️ GnuTLS</span>
                      </a-menu-item>
                    </a-menu>
                  </template>
                </a-dropdown>
              </div>
            </div>
          </div>
        </template>
      </a-tab-pane>

      <!-- 10. Agent Context (merged send/recv with SSE grouping) -->
      <a-tab-pane key="agent-context">
        <template #tab><MergeCellsOutlined /> Agent Context</template>
        <a-empty
          v-if="selectedPids.size === 0"
          description="Select a PID to view agent TLS context"
        />
        <AgentContextPanel
          v-else
          :events="visibleTLSEvents"
          @viewEvent="(e: ObserverTLSEvent) => openTLSDetail(e)"
        />
      </a-tab-pane>
    </a-tabs>

    <!-- Process picker modal -->
    <ProcessPickerModal
      :open="showPicker"
      :processes="processes"
      :selected-pids="[...selectedPids]"
      @update:open="showPicker = $event"
      @select="(procs: any) => { procs.forEach((p: ProcessInfo) => addPid(p.pid)) }"
    />

    <!-- File/directory browser modal -->
    <FilePathBrowserModal
      :open="browserOpen"
      :start-path="browserStartPath"
      :directory-only="browserTarget === 'cwd'"
      @update:open="browserOpen = $event"
      @select="onBrowserSelect"
    />

    <!-- TLS event detail modal -->
    <SSLDecryptedEventModal
      :open="tlsDetailOpen"
      :event="tlsDetailEvent"
      @close="tlsDetailOpen = false"
    />

    <!-- Process detail modal -->
    <ProcessDetailModal
      :open="detailModalOpen"
      :process="detailModalProcess"
      :process-list="treeProcessList"
      @update:open="detailModalOpen = $event"
      @select-pid="(pid: number) => togglePid(pid)"
      @signal="(pid: number, signal: string) => props.sendProcessSignal(pid, signal)"
    />

    <!-- Launch program modal -->
    <LaunchProgramModal
      :open="launchModalOpen"
      :launch-path="launchPath"
      :launch-user="launchUser"
      :launch-cwd="launchCwd"
      :launch-args="launchArgs"
      :launching="launching"
      :launch-error="launchError"
      :sys-users="sysUsers"
      :users-loading="usersLoading"
      :recent-launches="recentLaunches"
      @update:open="launchModalOpen = $event"
      @update:launch-path="launchPath = $event"
      @update:launch-user="launchUser = $event"
      @update:launch-cwd="launchCwd = $event"
      @update:launch-args="launchArgs = $event"
      @launch="doLaunch"
      @browse="openBrowser"
      @apply-recent="applyRecent"
    />
  </div>
</template>

<style scoped src="./ProcessObserverPanel.css"></style>
