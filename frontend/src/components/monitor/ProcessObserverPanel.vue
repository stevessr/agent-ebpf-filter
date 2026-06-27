<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from "vue";
import axios from "axios";
import {
  SearchOutlined,
  ReloadOutlined,
  NodeIndexOutlined,
} from "@ant-design/icons-vue";
import {
  useProcessObserver,
  type ProcessInfo,
  type ProcessTreeNode,
} from "../../composables/monitor/useProcessObserver";
import ProcessPickerModal from "./ProcessPickerModal.vue";
import ProcessTreeNodeDisplay from "./ProcessTreeNodeDisplay.vue";

// ── Props ────────────────────────────────────────────────────────────────

const props = defineProps<{
  processes: ProcessInfo[];
  sendProcessSignal: (pid: number, signal: string) => void;
  isActive: boolean;
}>();

// ── Sub-tab state ────────────────────────────────────────────────────────

const subTabKeys = [
  "selection",
  "tree",
  "network",
  "syscalls",
  "file-access",
  "resources",
  "ssl",
] as const;
type SubTabKey = (typeof subTabKeys)[number];
const activeSubTab = ref<SubTabKey>("selection");

// ── Composable ───────────────────────────────────────────────────────────

const obs = useProcessObserver();
const {
  selectedPid,
  showPicker,
  processTree,
  selectedProcessTree,
  treeProcessList,
  treeNetworkEvents,
  treeSyscallEvents,
  treeFileAccessEvents,
  treeNetworkFlows,
  treeTCPConns,
  treeTLSEvents,
  setProcesses,
  connectAll,
  disconnectAll,
  loadAllInitial,
} = obs;

setProcesses(props.processes);
watch(
  () => props.processes,
  (p) => setProcesses(p),
);

// ── PID input ────────────────────────────────────────────────────────────

const pidInput = ref("");
const pidInvalid = ref(false);

const onPidSearch = () => {
  const val = parseInt(pidInput.value, 10);
  if (!isNaN(val) && val > 0) {
    pidInvalid.value = false;
    selectedPid.value = val;
    pidInput.value = "";
  } else if (pidInput.value.trim()) {
    pidInvalid.value = true;
  }
};

const onClearSelection = () => {
  selectedPid.value = null;
  pidInput.value = "";
  pidInvalid.value = false;
};

const onPickerSelect = (proc: ProcessInfo) => {
  selectedPid.value = proc.pid;
  pidInput.value = "";
  pidInvalid.value = false;
};

const selectedProcessLabel = (): string => {
  if (selectedPid.value === null) return "";
  const p = props.processes.find((x) => x.pid === selectedPid.value);
  return p ? `[${p.pid}] ${p.name}` : `PID ${selectedPid.value}`;
};

// ── Launch controls ──────────────────────────────────────────────────────

const launchPath = ref("");
const launchUser = ref("");
const launchArgs = ref("");
const launching = ref(false);
const launchError = ref("");

const fetchUserInfo = async () => {
  try {
    const res = await axios.get("/system/user-info");
    launchPath.value = res.data.home || "/tmp";
    launchUser.value = res.data.username || "";
  } catch {
    launchPath.value = "/tmp";
    launchUser.value = "";
  }
};

fetchUserInfo();

const doLaunch = async () => {
  if (!launchPath.value.trim()) return;
  launching.value = true;
  launchError.value = "";
  try {
    const args = launchArgs.value
      .split(/\s+/)
      .filter((a) => a.length > 0);
    const res = await axios.post("/system/run", {
      comm: launchPath.value.trim(),
      args,
      user: launchUser.value.trim() || undefined,
      cwd: launchPath.value.trim().split("/").slice(0, -1).join("/") || "/",
    });
    if (res.data.pid) {
      selectedPid.value = res.data.pid;
      pidInput.value = "";
    }
  } catch (e: any) {
    launchError.value =
      e?.response?.data?.error || e?.message || "Launch failed";
  } finally {
    launching.value = false;
  }
};

// ── Helpers ──────────────────────────────────────────────────────────────

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes === 0) return "—";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    u.length - 1,
  );
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

const collectAllPids = (nodes: ProcessTreeNode[]): number[] =>
  nodes.flatMap((n) => [n.pid, ...collectAllPids(n.children)]);

// ── Recursive tree state ─────────────────────────────────────────────────

const expandedNodes = ref<Set<number>>(new Set());

const toggleExpand = (pid: number) => {
  const s = new Set(expandedNodes.value);
  if (s.has(pid)) s.delete(pid);
  else s.add(pid);
  expandedNodes.value = s;
};

// Auto-expand ancestors of selected PID
watch(selectedPid, (pid) => {
  if (pid !== null) {
    const expandTo = (nodes: ProcessTreeNode[], target: number): boolean => {
      for (const n of nodes) {
        if (n.pid === target || expandTo(n.children, target)) {
          expandedNodes.value = new Set([...expandedNodes.value, n.pid]);
          return true;
        }
      }
      return false;
    };
    expandTo(processTree.value, pid);
  }
});

// ── Table columns ────────────────────────────────────────────────────────

const networkFlowColumns = [
  { title: "Proto", dataIndex: "protocol", key: "protocol", width: 60 },
  { title: "Src", dataIndex: "srcIp", key: "srcIp", width: 130, ellipsis: true },
  { title: "Dst", dataIndex: "dstIp", key: "dstIp", width: 130, ellipsis: true },
  { title: "Port", dataIndex: "dstPort", key: "dstPort", width: 65 },
  { title: "Svc", dataIndex: "dstService", key: "dstService", width: 90 },
  { title: "In", dataIndex: "bytesIn", key: "bytesIn", width: 75, align: "right" as const },
  { title: "Out", dataIndex: "bytesOut", key: "bytesOut", width: 75, align: "right" as const },
];

const tcpConnColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 100 },
  { title: "State", dataIndex: "state", key: "state", width: 80 },
  { title: "Local", key: "local", width: 150, ellipsis: true },
  { title: "Remote", key: "remote", width: 150, ellipsis: true },
];

const eventColumns = [
  { title: "Time", dataIndex: "time", key: "time", width: 90 },
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 100, ellipsis: true },
  { title: "Type", dataIndex: "type", key: "type", width: 95 },
  { title: "Path", dataIndex: "path", key: "path", ellipsis: true },
  { title: "Bytes", dataIndex: "bytes", key: "bytes", width: 80, align: "right" as const },
];

const tlsColumns = [
  { title: "Time", dataIndex: "timestamp", key: "timestamp", width: 90 },
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 90, ellipsis: true },
  { title: "Dir", dataIndex: "direction", key: "direction", width: 50 },
  { title: "Host", dataIndex: "host", key: "host", width: 160, ellipsis: true },
  { title: "URL", dataIndex: "url", key: "url", ellipsis: true },
  { title: "Status", dataIndex: "status", key: "status", width: 60, align: "right" as const },
];

const resourceColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 65, sorter: (a: ProcessInfo, b: ProcessInfo) => a.pid - b.pid },
  { title: "Name", dataIndex: "name", key: "name", width: 140, ellipsis: true },
  { title: "CPU %", dataIndex: "cpu", key: "cpu", width: 80, align: "right" as const, sorter: (a: ProcessInfo, b: ProcessInfo) => a.cpu - b.cpu },
  { title: "Mem %", dataIndex: "mem", key: "mem", width: 80, align: "right" as const, sorter: (a: ProcessInfo, b: ProcessInfo) => a.mem - b.mem },
  { title: "User", dataIndex: "user", key: "user", width: 95 },
  { title: "Cmdline", dataIndex: "cmdline", key: "cmdline", ellipsis: true },
];

// ── Lifecycle ────────────────────────────────────────────────────────────

onMounted(() => {
  if (props.isActive) {
    connectAll();
    loadAllInitial();
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
    } else {
      disconnectAll();
    }
  },
);
</script>

<template>
  <div class="observer-panel">
    <a-tabs
      v-model:activeKey="activeSubTab"
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
            <a-tag
              v-if="selectedPid !== null"
              color="processing"
              closable
              @close="onClearSelection"
            >
              {{ selectedProcessLabel() }}
            </a-tag>
            <span v-else style="color: #999; font-size: 12px">
              No PID selected
            </span>
          </div>

          <!-- Launch program via wrapper -->
          <a-divider style="margin: 12px 0; font-size: 12px; color: #888">
            Launch &amp; Observe
          </a-divider>
          <div class="launch-grid">
            <div class="launch-field">
              <span class="launch-label">Program</span>
              <a-input
                v-model:value="launchPath"
                placeholder="/usr/bin/..."
                size="small"
                spellcheck="false"
              />
            </div>
            <div class="launch-field">
              <span class="launch-label">User</span>
              <a-input
                v-model:value="launchUser"
                placeholder="username"
                size="small"
                style="width: 130px"
              />
            </div>
            <div class="launch-field" style="grid-column: span 2">
              <span class="launch-label">Args</span>
              <a-input
                v-model:value="launchArgs"
                placeholder="--verbose --output /tmp/out"
                size="small"
              />
            </div>
          </div>
          <div style="margin-top: 8px; display: flex; align-items: center; gap: 10px">
            <a-button
              type="primary"
              size="small"
              :loading="launching"
              @click="doLaunch"
            >
              Launch & Observe
            </a-button>
            <span v-if="launchError" style="color: #ff4d4f; font-size: 12px">
              {{ launchError }}
            </span>
          </div>
        </div>
      </a-tab-pane>

      <!-- 2. Process Tree -->
      <a-tab-pane key="tree">
        <template #tab><NodeIndexOutlined /> Tree</template>
        <a-empty
          v-if="selectedPid === null"
          description="Select a PID to view its process tree"
        />
        <div v-else class="tree-container">
          <div style="margin-bottom: 8px">
            <a-button
              size="small"
              type="link"
              @click="expandedNodes = new Set(collectAllPids(selectedProcessTree))"
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
            :highlight-pid="selectedPid ?? 0"
            :expanded-set="expandedNodes"
            @toggle="toggleExpand"
            @select="(pid: number) => (selectedPid = pid)"
          />
        </div>
      </a-tab-pane>

      <!-- 3. Network -->
      <a-tab-pane key="network">
        <template #tab><ReloadOutlined /> Network</template>
        <a-empty
          v-if="selectedPid === null"
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

      <!-- 4. Syscalls -->
      <a-tab-pane key="syscalls">
        <template #tab>Syscalls</template>
        <a-empty
          v-if="selectedPid === null"
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
              {{ text?.toUpperCase?.() || '—' }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
          </template>
        </a-table>
      </a-tab-pane>

      <!-- 5. File Access -->
      <a-tab-pane key="file-access">
        <template #tab>File Access</template>
        <a-empty
          v-if="selectedPid === null"
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
              {{ text?.toUpperCase?.() || '—' }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
          </template>
        </a-table>
      </a-tab-pane>

      <!-- 6. Resource Usage -->
      <a-tab-pane key="resources">
        <template #tab>Resources</template>
        <a-empty
          v-if="selectedPid === null"
          description="Select a PID to view resource usage"
        />
        <template v-else>
          <div class="sub-title">
            {{ treeProcessList.length }} process(es) in tree
          </div>
          <a-table
            :dataSource="treeProcessList"
            :columns="resourceColumns"
            row-key="pid"
            size="small"
            :pagination="{ pageSize: 20, size: 'small' }"
          />
        </template>
      </a-tab-pane>

      <!-- 7. SSL Decrypt -->
      <a-tab-pane key="ssl">
        <template #tab>SSL</template>
        <a-empty
          v-if="selectedPid === null"
          description="Select a PID to view decrypted TLS events"
        />
        <a-table
          v-else
          :dataSource="treeTLSEvents"
          :columns="tlsColumns"
          row-key="key"
          size="small"
          :pagination="{ pageSize: 20, size: 'small' }"
        />
      </a-tab-pane>
    </a-tabs>

    <!-- Process picker modal -->
    <ProcessPickerModal
      :open="showPicker"
      :processes="processes"
      :selected-pid="selectedPid"
      @update:open="showPicker = $event"
      @select="onPickerSelect"
    />
  </div>
</template>

<style scoped>
.observer-panel {
  background: #fff;
  padding: 0;
  border-radius: 4px;
}

.observer-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 12px;
}

.selection-container {
  display: flex;
  flex-direction: column;
}

.selection-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 0;
}

.launch-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 16px;
}

.launch-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.launch-label {
  font-size: 11px;
  color: #888;
  font-weight: 500;
}

.tree-container {
  max-height: 500px;
  overflow-y: auto;
  font-family: "JetBrains Mono", monospace;
  font-size: 13px;
}

.sub-section {
  margin-bottom: 16px;
}
.sub-title {
  font-weight: 600;
  font-size: 13px;
  color: #555;
  margin-bottom: 6px;
  padding-bottom: 4px;
  border-bottom: 1px solid #f0f0f0;
}
</style>
