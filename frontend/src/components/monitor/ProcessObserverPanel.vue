<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, watch, computed, h } from "vue";
import axios from "axios";
import {
  SearchOutlined,
  ReloadOutlined,
  NodeIndexOutlined,
  CaretDownOutlined,
  EyeOutlined,
  MergeCellsOutlined,
} from "@ant-design/icons-vue";
import {
  useProcessObserver,
  type ProcessInfo,
  type ProcessTreeNode,
  type ObserverTLSEvent,
} from "../../composables/monitor/useProcessObserver";
import ProcessPickerModal from "./ProcessPickerModal.vue";
import ProcessTreeNodeDisplay from "./ProcessTreeNodeDisplay.vue";
import ObserverTimeline from "./observer/ObserverTimeline.vue";
import ObserverFlamegraph from "./observer/ObserverFlamegraph.vue";
import ObserverResources from "./observer/ObserverResources.vue";
import AgentContextPanel from "./observer/AgentContextPanel.vue";
import FilePathBrowserModal from "./FilePathBrowserModal.vue";
import SSLDecryptedEventModal from "./observer/SSLDecryptedEventModal.vue";

// ── Props ────────────────────────────────────────────────────────────────

const props = defineProps<{
  processes: ProcessInfo[];
  sendProcessSignal: (pid: number, signal: string) => void;
  isActive: boolean;
  memTotal?: number;
}>();

// ── Sub-tab state ────────────────────────────────────────────────────────

const subTabKeys = [
  "selection",
  "timeline",
  "flamegraph",
  "tree",
  "network",
  "syscalls",
  "file-access",
  "resources",
  "ssl",
  "agent-context",
] as const;
type SubTabKey = (typeof subTabKeys)[number];
const TAB_STORAGE_KEY = "observe-active-tab";
const activeSubTab = ref<SubTabKey>(
  (localStorage.getItem(TAB_STORAGE_KEY) as SubTabKey) || "selection",
);

watch(activeSubTab, (tab) => {
  try { localStorage.setItem(TAB_STORAGE_KEY, tab); } catch { /* ignore */ }
});

// ── Composable ───────────────────────────────────────────────────────────

const obs = useProcessObserver();
const {
  selectedPid,
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
  // Clear + SSL
  clearEvents,
  clearTLSEvents,
  clearNetworkFlows,
  clearTCPConns,
  attachedPIDs,
  fetchAttachedPIDs,
} = obs;

setProcesses(props.processes);
watch(
  () => props.processes,
  (p) => setProcesses(p),
);

// ── PID input ────────────────────────────────────────────────────────────

const pidInput = ref("");
const pidInvalid = ref(false);

// Persist selected PID across page refreshes
const PID_STORAGE_KEY = "observe-selected-pid";

const restorePidFromStorage = () => {
  try {
    const raw = localStorage.getItem(PID_STORAGE_KEY);
    if (raw !== null) {
      const parsed = parseInt(raw, 10);
      if (!isNaN(parsed) && parsed > 0) {
        selectedPid.value = parsed;
        return;
      }
    }
  } catch { /* ignore */ }
};
restorePidFromStorage();

watch(selectedPid, (pid) => {
  try {
    if (pid !== null) {
      localStorage.setItem(PID_STORAGE_KEY, String(pid));
    } else {
      localStorage.removeItem(PID_STORAGE_KEY);
    }
  } catch { /* ignore */ }
});

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
const launchCwd = ref("");
const launchArgs = ref("");
const launching = ref(false);
const launchError = ref("");

// File/dir browser state
type BrowseTarget = "program" | "cwd";
const browserTarget = ref<BrowseTarget>("program");
const browserOpen = ref(false);
const browserStartPath = ref("/");

// User list state
interface SysUser {
  username: string;
  uid: number;
  home: string;
  shell: string;
}
const sysUsers = ref<SysUser[]>([]);
const usersLoading = ref(false);

// Recent launches (localStorage persistence)
interface RecentLaunch {
  program: string;
  user: string;
  cwd: string;
  args: string;
}
const STORAGE_KEY = "observe-recent-launches";
const MAX_RECENT = 10;

const loadRecentLaunches = (): RecentLaunch[] => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
};
const recentLaunches = ref<RecentLaunch[]>(loadRecentLaunches());

const saveRecentLaunch = (rl: RecentLaunch) => {
  const existing = loadRecentLaunches().filter(
    (r) => r.program !== rl.program || r.args !== rl.args,
  );
  existing.unshift(rl);
  if (existing.length > MAX_RECENT) existing.length = MAX_RECENT;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(existing));
  recentLaunches.value = existing;
};

const applyRecent = (rl: RecentLaunch) => {
  launchPath.value = rl.program;
  launchUser.value = rl.user;
  launchCwd.value = rl.cwd;
  launchArgs.value = rl.args;
};

const fetchUserInfo = async () => {
  try {
    const res = await axios.get("/system/user-info");
    launchCwd.value = res.data.home || "/tmp";
    launchUser.value = res.data.username || "";
  } catch {
    launchCwd.value = "/tmp";
    launchUser.value = "";
  }
};

const fetchUsers = async () => {
  usersLoading.value = true;
  try {
    const res = await axios.get("/system/users");
    sysUsers.value = Array.isArray(res.data) ? res.data : [];
  } catch {
    sysUsers.value = [];
  } finally {
    usersLoading.value = false;
  }
};

fetchUserInfo();
fetchUsers();

const openBrowser = (target: BrowseTarget) => {
  browserTarget.value = target;
  browserStartPath.value =
    target === "program"
      ? launchPath.value
        ? launchPath.value.split("/").slice(0, -1).join("/") || "/"
        : "/usr/bin"
      : launchCwd.value || "/";
  browserOpen.value = true;
};

const onBrowserSelect = (path: string) => {
  if (browserTarget.value === "program") {
    launchPath.value = path;
    // Auto-fill cwd from program directory if cwd is empty
    if (!launchCwd.value.trim()) {
      launchCwd.value = path.split("/").slice(0, -1).join("/") || "/";
    }
  } else {
    launchCwd.value = path;
  }
};

const doLaunch = async () => {
  if (!launchPath.value.trim()) return;
  launching.value = true;
  launchError.value = "";
  try {
    const args = launchArgs.value.split(/\s+/).filter((a) => a.length > 0);
    const res = await axios.post("/system/run", {
      comm: launchPath.value.trim(),
      args,
      user: launchUser.value.trim() || undefined,
      cwd: launchCwd.value.trim() || undefined,
    });
    if (res.data.pid) {
      selectedPid.value = res.data.pid;
      pidInput.value = "";
      // Persist to localStorage
      saveRecentLaunch({
        program: launchPath.value.trim(),
        user: launchUser.value.trim(),
        cwd: launchCwd.value.trim(),
        args: launchArgs.value.trim(),
      });
    }
  } catch (e: any) {
    launchError.value =
      e?.response?.data?.error || e?.message || "Launch failed";
  } finally {
    launching.value = false;
  }
};

// ── Persistent SSL display / auto-attach toggles ────────────────────────

const readStoredBool = (key: string, fallback: boolean): boolean => {
  try {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    return v === "true";
  } catch { return fallback; }
};

const SSL_SKIP_KEY = "observe-skip-ssl";
const SSL_AUTO_ATTACH_KEY = "observe-auto-attach";

const skipSSL = ref(readStoredBool(SSL_SKIP_KEY, false));
const autoAttach = ref(readStoredBool(SSL_AUTO_ATTACH_KEY, false));

watch(skipSSL, (v) => { try { localStorage.setItem(SSL_SKIP_KEY, String(v)); } catch {} });
watch(autoAttach, (v) => { try { localStorage.setItem(SSL_AUTO_ATTACH_KEY, String(v)); } catch {} });

// Filtered TLS events (respects skipSSL toggle)
const visibleTLSEvents = computed(() =>
  skipSSL.value ? [] : treeTLSEvents.value
);

// Auto-attach tracked PIDs that were already attempted (avoid infinite retry)
const autoAttachSeen = new Set<number>();

watch(treeSSLPending, (pending) => {
  if (!autoAttach.value) return;
  for (const p of pending) {
    if (autoAttachSeen.has(p.pid)) continue;
    autoAttachSeen.add(p.pid);
    // Small stagger delay so we don't flood the backend
    setTimeout(() => doAttachBuiltins(p.pid), 100 * autoAttachSeen.size);
  }
}, { deep: false });

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

const formatBytes = (bytes: number): string => {
  if (!bytes || bytes === 0) return "—";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    u.length - 1,
  );
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

// Format body preview for table display
const formatTLSBodyPreview = (body: string | undefined, maxLen: number = 120): string => {
  if (!body) return "";
  const trimmed = body.replace(/\s+/g, " ").trim();
  return trimmed.length > maxLen ? trimmed.slice(0, maxLen) + "…" : trimmed;
};

// Hex to text decoder for raw TLS events
const hexToText = (hex: string, maxLen: number = 100): string => {
  try {
    const bytes: number[] = [];
    for (let i = 0; i < hex.length - 1; i += 2) {
      const byte = parseInt(hex.slice(i, i + 2), 16);
      if (isNaN(byte)) break;
      bytes.push(byte);
    }
    const text = bytes
      .map((b) => (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d ? String.fromCharCode(b) : ".")
      .join("");
    return text.length > maxLen ? text.slice(0, maxLen) + "…" : text;
  } catch { return ""; }
};

// Best available preview from any TLS event (body first, then hex decode)
const bestTLSPreview = (ev: ObserverTLSEvent, maxLen: number = 100): string => {
  if (ev.body) return formatTLSBodyPreview(ev.body, maxLen);
  if (ev.raw_hex_dump) return hexToText(ev.raw_hex_dump, maxLen);
  return "";
};

// Check if body looks like JSON
const looksLikeJSON = (body: string | undefined): boolean => {
  if (!body) return false;
  const t = body.trim();
  return (t.startsWith("{") || t.startsWith("[")) && t.length > 2;
};

// TLS event detail modal
const tlsDetailOpen = ref(false);
const tlsDetailEvent = ref<ObserverTLSEvent | null>(null);

const openTLSDetail = (event: ObserverTLSEvent) => {
  tlsDetailEvent.value = event;
  tlsDetailOpen.value = true;
};

// Expanded row render for TLS events — shows decoded body + headers
const expandedTLSRowRender = (record: ObserverTLSEvent) => {
  const items: any[] = [];

  // Headers section
  if (record.headers && Object.keys(record.headers).length > 0) {
    const headerTags = Object.entries(record.headers).map(([k, v]) => {
      const isRedacted = v === "***REDACTED***";
      return h("div", { class: "tls-header-row" }, [
        h("span", { class: "tls-header-key" }, k),
        h("span", { class: isRedacted ? "tls-header-val-redacted" : "tls-header-val" }, v),
      ]);
    });
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, `Headers (${Object.keys(record.headers).length})`),
        h("div", { class: "tls-headers-box" }, headerTags),
      ]),
    );
  }

  // Agent context section
  if (record.vendor || record.agent_run_id || record.task_id || record.message_role) {
    const ctxItems: any[] = [];
    if (record.vendor) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Vendor:"), h("a-tag", { color: "geekblue", size: "small" }, () => record.vendor)]));
    if (record.message_role) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Role:"), h("code", {}, record.message_role)]));
    if (record.agent_run_id) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Run:"), h("code", {}, record.agent_run_id)]));
    if (record.task_id) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Task:"), h("code", {}, record.task_id)]));
    if (record.prompt_digest) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Prompt:"), h("code", {}, record.prompt_digest.slice(0, 32) + "…")]));
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, "Agent Context"),
        ...ctxItems,
      ]),
    );
  }

  // Body section
  if (record.body) {
    const bodyContent = record.body.length > 3000
      ? record.body.slice(0, 3000) + "\n… [truncated]"
      : record.body;
    const isJSON = looksLikeJSON(record.body);
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, `Body (${record.body_size || record.body.length} bytes)${record.truncated ? ' — truncated' : ''}`),
        h("pre", { class: isJSON ? "tls-body-json" : "tls-body-text" }, bodyContent),
        h("a-button", {
          size: "small", type: "link",
          onClick: () => openTLSDetail(record),
        }, { default: () => "View Full Detail" }),
      ]),
    );
  }

  // SSE info
  if (record.sse_event || record.sse_data_count) {
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, "SSE"),
        h("div", { class: "tls-sse-info" }, [
          record.sse_event ? h("span", {}, `Event: ${record.sse_event}`) : null,
          record.sse_data_count ? h("span", { style: { marginLeft: "8px" } }, `Data parts: ${record.sse_data_count}`) : null,
        ].filter(Boolean)),
      ]),
    );
  }

  if (items.length === 0) return null;
  return h("div", { class: "tls-expand-content" }, items);
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

// SSL attachment status per PID
const sslAttachedSet = computed<Set<number>>(
  () => new Set(attachedPIDs.value.map((a: any) => a.pid)),
);
const sslLibForPid = (pid: number): string => {
  const a = attachedPIDs.value.find((x: any) => x.pid === pid);
  return a ? a.library_name || "attached" : "";
};

// Classify SSL/TLS library by name
const classifySSLLib = (libName: string): { type: string; color: string; tagColor: string } => {
  const name = libName.toLowerCase();
  if (name.includes("go-crypto") || name.includes("crypto/tls")) return { type: "Go crypto/tls", color: "#00ADD8", tagColor: "cyan" };
  if (name.includes("openssl") || name.includes("libssl") || name.includes("libcrypto")) return { type: "OpenSSL", color: "#1677ff", tagColor: "blue" };
  if (name.includes("gnutls")) return { type: "GnuTLS", color: "#10b981", tagColor: "green" };
  if (name.includes("nss3") || name.includes("libnss")) return { type: "NSS", color: "#f59e0b", tagColor: "orange" };
  if (name.includes("mbedtls")) return { type: "Mbed TLS", color: "#8b5cf6", tagColor: "purple" };
  if (name.includes("wolfssl")) return { type: "WolfSSL", color: "#ef4444", tagColor: "red" };
  if (name.includes("boringssl")) return { type: "BoringSSL", color: "#6366f1", tagColor: "geekblue" };
  if (name) return { type: "SSL Library", color: "#64748b", tagColor: "default" };
  return { type: "Detected", color: "#94a3b8", tagColor: "default" };
};

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

const sslAttachmentColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 65 },
  { title: "Comm", key: "comm", width: 110, ellipsis: true },
  { title: "Library Path", dataIndex: "library_name", key: "lib", ellipsis: true },
  { title: "Type", key: "libType", width: 100 },
  { title: "Binary", dataIndex: "binary_path", key: "bin", ellipsis: true },
  { title: "Status", key: "status", width: 80 },
];

// ── Table columns ────────────────────────────────────────────────────────

const networkFlowColumns = [
  { title: "Proto", dataIndex: "protocol", key: "protocol", width: 60 },
  {
    title: "Src",
    dataIndex: "srcIp",
    key: "srcIp",
    width: 130,
    ellipsis: true,
  },
  {
    title: "Dst",
    dataIndex: "dstIp",
    key: "dstIp",
    width: 130,
    ellipsis: true,
  },
  { title: "Port", dataIndex: "dstPort", key: "dstPort", width: 65 },
  { title: "Svc", dataIndex: "dstService", key: "dstService", width: 90 },
  {
    title: "In",
    dataIndex: "bytesIn",
    key: "bytesIn",
    width: 75,
    align: "right" as const,
  },
  {
    title: "Out",
    dataIndex: "bytesOut",
    key: "bytesOut",
    width: 75,
    align: "right" as const,
  },
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
  {
    title: "Bytes",
    dataIndex: "bytes",
    key: "bytes",
    width: 80,
    align: "right" as const,
  },
];

const tlsColumns = [
  { title: "Time", dataIndex: "timestamp", key: "timestamp", width: 90 },
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 90, ellipsis: true },
  { title: "Dir", dataIndex: "direction", key: "direction", width: 50 },
  { title: "Type", key: "evType", width: 95 },
  { title: "Host", dataIndex: "host", key: "host", width: 140, ellipsis: true },
  { title: "URL", dataIndex: "url", key: "url", ellipsis: true },
  { title: "Body Preview", key: "bodyPreview", ellipsis: true },
  {
    title: "Size",
    dataIndex: "captured_len",
    key: "size",
    width: 70,
    align: "right" as const,
  },
  { title: "", key: "actions", width: 36 },
];

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
            <span v-else style="color: #6b7280; font-size: 12px">
              No PID selected
            </span>
          </div>

          <!-- Launch program via wrapper -->
          <a-divider style="margin: 12px 0; font-size: 12px; color: #4b5563">
            Launch &amp; Observe
          </a-divider>
          <div class="launch-grid">
            <!-- Program -->
            <div class="launch-field" style="grid-column: span 2">
              <span class="launch-label">Program</span>
              <div class="launch-row">
                <a-input
                  v-model:value="launchPath"
                  placeholder="/usr/bin/python3"
                  size="small"
                  spellcheck="false"
                  style="flex: 1"
                />
                <a-button size="small" @click="openBrowser('program')">
                  Browse
                </a-button>
              </div>
            </div>
            <!-- User -->
            <div class="launch-field">
              <span class="launch-label">User</span>
              <a-select
                v-model:value="launchUser"
                size="small"
                show-search
                :filter-option="
                  (input: string, option: any) =>
                    option.value.toLowerCase().includes(input.toLowerCase())
                "
                :loading="usersLoading"
                placeholder="Select user..."
                style="width: 100%"
              >
                <a-select-option
                  v-for="u in sysUsers"
                  :key="u.username"
                  :value="u.username"
                >
                  {{ u.username }}
                  <span class="user-uid">({{ u.uid }})</span>
                </a-select-option>
              </a-select>
            </div>
            <!-- Working Directory -->
            <div class="launch-field">
              <span class="launch-label">Working Directory</span>
              <div class="launch-row">
                <a-input
                  v-model:value="launchCwd"
                  placeholder="/home/..."
                  size="small"
                  spellcheck="false"
                  style="flex: 1"
                />
                <a-button size="small" @click="openBrowser('cwd')">
                  Browse
                </a-button>
              </div>
            </div>
            <!-- Args -->
            <div class="launch-field" style="grid-column: span 2">
              <span class="launch-label">Arguments</span>
              <a-input
                v-model:value="launchArgs"
                placeholder="--verbose --output /tmp/out"
                size="small"
              />
            </div>
          </div>
          <div
            style="
              margin-top: 8px;
              display: flex;
              align-items: center;
              gap: 10px;
            "
          >
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

          <!-- Recent launches -->
          <div v-if="recentLaunches.length > 0" class="recent-section">
            <div class="recent-title">Recent</div>
            <div class="recent-chips">
              <a-tag
                v-for="(rl, i) in recentLaunches"
                :key="i"
                class="recent-chip"
                color="default"
                @click="applyRecent(rl)"
              >
                <span class="recent-prog">{{ rl.program.split('/').pop() || rl.program }}</span>
                <span class="recent-args" v-if="rl.args">{{ rl.args.slice(0, 30) }}{{ rl.args.length > 30 ? '…' : '' }}</span>
              </a-tag>
            </div>
          </div>
        </div>
      </a-tab-pane>

      <!-- 2. Timeline (NEW) -->
      <a-tab-pane key="timeline">
        <template #tab>Timeline</template>
        <ObserverTimeline
          :events="allEvents"
          :tlsEvents="treeTLSEvents"
          :selectedPid="selectedPid"
          @clear="clearEvents()"
          @selectPid="(pid: number) => (selectedPid = pid)"
          @viewTLSEvent="(e: ObserverTLSEvent) => openTLSDetail(e)"
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
          v-if="selectedPid === null"
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
            :highlight-pid="selectedPid ?? 0"
            :expanded-set="expandedNodes"
            :ssl-attached-set="sslAttachedSet"
            :ssl-lib-for-pid="sslLibForPid"
            @toggle="toggleExpand"
            @select="(pid: number) => (selectedPid = pid)"
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
              {{ text?.toUpperCase?.() || "—" }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
            <span v-else-if="column.key === 'path'">
              <span v-if="text" :title="text">{{ text }}</span>
              <span v-else-if="record.extraInfo" class="path-fallback" :title="record.extraInfo">{{ record.extraInfo }}</span>
              <span v-else style="color: #9ca3af">—</span>
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
              {{ text?.toUpperCase?.() || "—" }}
            </a-tag>
            <span v-else-if="column.key === 'bytes'">
              {{ formatBytes(record.bytes) }}
            </span>
            <span v-else-if="column.key === 'path'">
              <span v-if="text" :title="text">{{ text }}</span>
              <span v-else-if="record.extraInfo" class="path-fallback" :title="record.extraInfo">{{ record.extraInfo }}</span>
              <span v-else style="color: #9ca3af">—</span>
            </span>
          </template>
        </a-table>
      </a-tab-pane>

      <!-- 8. Resources -->
      <a-tab-pane key="resources">
        <template #tab>Resources</template>
        <a-empty
          v-if="selectedPid === null"
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
          v-if="selectedPid === null"
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
                  <span v-else style="color: #9ca3af; font-size: 11px">—</span>
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
          v-if="selectedPid === null"
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
      :selected-pid="selectedPid"
      @update:open="showPicker = $event"
      @select="onPickerSelect"
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
  color: #4b5563;
  font-weight: 500;
}

.launch-row {
  display: flex;
  gap: 6px;
  align-items: center;
}

.user-uid {
  font-size: 11px;
  color: #6b7280;
  margin-left: 4px;
}

.recent-section {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed #e8e8e8;
}

.recent-title {
  font-size: 11px;
  color: #6b7280;
  font-weight: 500;
  margin-bottom: 6px;
}

.recent-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.recent-chip {
  cursor: pointer;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: all 0.15s;
}

.recent-chip:hover {
  border-color: #1677ff;
  color: #1677ff;
}

.recent-prog {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  font-size: 12px;
}

.recent-args {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #6b7280;
  margin-left: 4px;
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
  display: flex;
  align-items: center;
  gap: 6px;
}
.sub-count {
  font-size: 11px;
  color: #6b7280;
  font-weight: 400;
}
.ssl-attach-comm {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  font-weight: 500;
}
.ssl-pending-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ssl-pending-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  transition: background 0.15s;
}
.ssl-pending-row:hover {
  background: #f5f5f5;
}
.ssl-pending-pid {
  font-family: ui-monospace, monospace;
  font-weight: 700;
  color: #1677ff;
  font-size: 12px;
  min-width: 50px;
}
.ssl-pending-name {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  font-weight: 500;
  color: #1f2937;
}
.ssl-pending-cmd {
  font-size: 11px;
  color: #6b7280;
  font-family: ui-monospace, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 180px;
}
.attach-menu-item {
  font-size: 12px;
}
.attach-errors {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 8px;
}
.attach-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  background: #fff2f0;
  border: 1px solid #ffccc7;
  border-radius: 4px;
  font-size: 11px;
  color: #ff4d4f;
}
.attach-error code {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #cf1322;
  background: #fff1f0;
  padding: 0 3px;
  border-radius: 2px;
}

/* TLS expanded row styles */
.tls-expand-content {
  padding: 8px 16px;
  max-width: 700px;
}
.tls-expand-section {
  margin-bottom: 10px;
}
.tls-expand-label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  margin-bottom: 4px;
  padding-bottom: 2px;
  border-bottom: 1px solid #f0f0f0;
}
.tls-headers-box {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  padding: 4px 8px;
  max-height: 160px;
  overflow-y: auto;
}
.tls-header-row {
  display: flex;
  gap: 8px;
  font-size: 11px;
  line-height: 1.6;
}
.tls-header-key {
  font-weight: 600;
  color: #475569;
  font-family: ui-monospace, monospace;
  min-width: 120px;
}
.tls-header-val {
  color: #334155;
  font-family: ui-monospace, monospace;
  word-break: break-all;
}
.tls-header-val-redacted {
  color: #f59e0b;
  font-family: ui-monospace, monospace;
  font-style: italic;
}
.tls-body-json {
  background: #0f172a;
  color: #dbeafe;
  padding: 10px;
  border-radius: 6px;
  font-size: 11px;
  line-height: 1.55;
  max-height: 260px;
  overflow: auto;
  margin: 4px 0;
  white-space: pre-wrap;
}
.tls-body-text {
  background: #f1f5f9;
  color: #1e293b;
  padding: 10px;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.5;
  max-height: 200px;
  overflow: auto;
  margin: 4px 0;
  white-space: pre-wrap;
}
.tls-ctx-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  line-height: 1.8;
}
.tls-ctx-key {
  color: #64748b;
  font-weight: 500;
  min-width: 55px;
}
.tls-ctx-row code {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #334155;
  background: #f1f5f9;
  padding: 1px 4px;
  border-radius: 3px;
}
.tls-sse-info {
  font-size: 11px;
  color: #64748b;
  font-family: ui-monospace, monospace;
}
.tls-body-preview {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #475569;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 180px;
  display: inline-block;
}
.tls-body-preview.tls-body-json {
  color: #2563eb;
}

/* Path fallback */
.path-fallback {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #94a3b8;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: inline-block;
}
</style>
