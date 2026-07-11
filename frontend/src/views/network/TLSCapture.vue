<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import axios from "axios";
import {
  ReloadOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";

import RedactionBadge from "../../components/common/RedactionBadge.vue";
import TLSCaptureDetailsModal from "../../components/network/tlsCapture/TLSCaptureDetailsModal.vue";
import TLSCaptureEventsPanel from "../../components/network/tlsCapture/TLSCaptureEventsPanel.vue";
import TLSCaptureRuntimeStatus from "../../components/network/tlsCapture/TLSCaptureRuntimeStatus.vue";
import TLSHookManagement from "../../components/network/tlsCapture/TLSHookManagement.vue";
import { buildWebSocketUrl } from "../../utils/requestContext";
import {
  TLS_CAPTURE_EVENT_LIMIT,
  TLS_CAPTURE_RECONNECT_DELAY_MS,
  TLS_CAPTURE_STATUS_REFRESH_MS,
  TLS_CAPTURE_STATUS_TIMEOUT_MS,
  type TLSExecutableLibraryHint,
  type TLSManualHookType,
} from "./tlsCapture/constants";
import type { TLSExecutableAttachResult } from "./tlsCapture/types";
import { useTLSCaptureFilters } from "./tlsCapture/useTLSCaptureFilters";
import {
  isTLSDisplayEvent,
  loadTLSIgnoreRules,
  saveTLSIgnoreRulesToStorage,
} from "./tlsCapture/utils";

import type {
  TLSPlaintextEvent,
  TLSLibraryStatus,
  TLSCaptureRule,
  TLSIgnoreRule,
  TLSCaptureStatus,
  TLSBuiltinExecutableAttachStatus,
} from "../../types/tls";

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
}

const events = ref<TLSPlaintextEvent[]>([]);
const libraries = ref<TLSLibraryStatus[]>([]);
const rules = ref<TLSCaptureRule[]>([]);
const ignoreRules = ref<TLSIgnoreRule[]>(loadTLSIgnoreRules());
const ignoreRulesLoading = ref(false);
const captureStatus = ref<TLSCaptureStatus>({});
const isConnected = ref(false);
const isPaused = ref(false);
const searchQuery = ref("");
const commFilter = ref("");
const hostFilter = ref("");
const selectedLib = ref<string>("all");
const selectedDirection = ref<string>("all");
const sslFilterExpr = ref("");
const ignoreFilter = ref("");
const showDetails = ref(false);
const selectedEvent = ref<TLSPlaintextEvent | null>(null);
const rulesLoading = ref(false);
const attachLoading = ref(false);
const builtinAttachLoading = ref(false);
const manualHookLoading = ref(false);
const hookManagementTab = ref("rules");
const manualHookType = ref<TLSManualHookType>("executable");
const manualHookPid = ref<number | null>(null);
const builtinAttachStatuses = ref<TLSBuiltinExecutableAttachStatus[]>([]);
const executablePathInput = ref("");
const executableLibraryHint = ref<TLSExecutableLibraryHint>("auto");
const executableAttachResult = ref<TLSExecutableAttachResult | null>(null);

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let statusRefreshTimer: ReturnType<typeof setInterval> | null = null;
let statusFetchInFlight = false;
let componentMounted = false;
let runtimeGateDisabled = false;
let shouldReconnect = true;
let eventKeySequence = 0;

const markRuntimeGateDisabled = () => {
  runtimeGateDisabled = true;
  isConnected.value = false;
  captureStatus.value = {
    enabled: false,
    available: false,
    error: "TLS capture is disabled by runtime settings",
  };
};

const withEventKey = (event: TLSPlaintextEvent): TLSPlaintextEvent => {
  if (event.key) return event;
  eventKeySequence += 1;
  return {
    ...event,
    key: [
      event.timestamp ?? "ts",
      event.pid ?? 0,
      event.direction ?? "dir",
      eventKeySequence,
    ].join("-"),
  };
};

const { filteredEvents, summaryStats } = useTLSCaptureFilters({
  events,
  libraries,
  ignoreRules,
  searchQuery,
  commFilter,
  hostFilter,
  selectedLib,
  selectedDirection,
  sslFilterExpr,
  ignoreFilter,
});

const captureStatusText = computed(() => {
  if (captureStatus.value.enabled)
    return summaryStats.value.attachedLibs > 0
      ? "Running"
      : "Running, no libraries attached";
  return "Not started";
});

const captureStatusColor = computed(() => {
  if (!captureStatus.value.enabled) return "default";
  return summaryStats.value.attachedLibs > 0 ? "green" : "orange";
});

const fetchRecentEvents = async () => {
  try {
    const response = await axios.get(
      "/tls-capture/recent?limit=" + TLS_CAPTURE_EVENT_LIMIT,
    );
    const recentEvents = Array.isArray(response.data?.events)
      ? (response.data.events as TLSPlaintextEvent[])
      : [];
    events.value = recentEvents.filter(isTLSDisplayEvent).map(withEventKey);
  } catch (error: any) {
    if (error?.response?.status === 403) {
      markRuntimeGateDisabled();
      events.value = [];
      return;
    }
    message.error(
      error?.response?.data?.error || "Failed to load TLS capture events",
    );
  }
};

const fetchLibraries = async () => {
  try {
    const response = await axios.get("/tls-capture/libraries");
    libraries.value = Array.isArray(response.data?.libraries)
      ? response.data.libraries
      : [];
  } catch (error: any) {
    if (error?.response?.status === 403) {
      markRuntimeGateDisabled();
      libraries.value = [];
      return;
    }
    message.error(
      error?.response?.data?.error || "Failed to load TLS capture libraries",
    );
  }
};

const fetchStatus = async (silent = false) => {
  if (statusFetchInFlight) return;
  statusFetchInFlight = true;
  try {
    const response = await axios.get("/tls-capture/status", {
      timeout: TLS_CAPTURE_STATUS_TIMEOUT_MS,
    });
    if (!componentMounted) return;
    const wasDisabled = runtimeGateDisabled;
    runtimeGateDisabled = false;
    captureStatus.value = response.data || {};
    if (Array.isArray(response.data?.libraries))
      libraries.value = response.data.libraries;
    if (wasDisabled && !ws) connectWebSocket();
  } catch (error: any) {
    if (error?.response?.status === 403 && componentMounted) {
      markRuntimeGateDisabled();
      return;
    }
    if (!silent && componentMounted) {
      message.error(
        error?.response?.data?.error || "Failed to load Hook SSL status",
      );
    }
  } finally {
    statusFetchInFlight = false;
  }
};

const attachDefaultLibraries = async (silent = false) => {
  attachLoading.value = true;
  try {
    const response = await axios.post("/tls-capture/attach-defaults");
    captureStatus.value = response.data || {};
    if (!silent) {
      if (response.data?.error) message.warning(response.data.error);
      else message.success("Hook SSL probes attached");
    }
    await Promise.all([fetchLibraries(), fetchRecentEvents()]);
  } catch (error: any) {
    if (error?.response?.status === 403) {
      markRuntimeGateDisabled();
      if (!silent)
        message.warning("TLS capture is disabled in runtime settings");
      return;
    }
    const status = error?.response?.data?.status;
    if (status) captureStatus.value = status;
    if (!silent)
      message.error(
        error?.response?.data?.error || "Failed to attach Hook SSL probes",
      );
    await fetchLibraries();
  } finally {
    attachLoading.value = false;
  }
};

const attachBuiltinExecutables = async () => {
  builtinAttachLoading.value = true;
  try {
    const response = await axios.post("/tls-capture/attach-builtins", {
      pid: manualHookPid.value || 0,
    });
    captureStatus.value = response.data?.status || captureStatus.value;
    builtinAttachStatuses.value = Array.isArray(response.data?.statuses)
      ? response.data.statuses
      : [];
    const attached = builtinAttachStatuses.value.filter((item) => item.attached)
      .length;
    if (attached > 0)
      message.success(`Attached ${attached} built-in TLS target(s)`);
    else
      message.warning(
        response.data?.error || "No built-in TLS targets attached",
      );
    await Promise.all([fetchStatus(), fetchLibraries()]);
  } catch (error: any) {
    const payload = error?.response?.data || {};
    if (payload.status) captureStatus.value = payload.status;
    builtinAttachStatuses.value = Array.isArray(payload.statuses)
      ? payload.statuses
      : [];
    message.error(payload.error || "Failed to attach built-in TLS targets");
    await fetchLibraries();
  } finally {
    builtinAttachLoading.value = false;
  }
};

const attachBuiltinCommand = async (command: string) => {
  manualHookType.value = "executable";
  executableLibraryHint.value = "auto";
  executablePathInput.value = command;
  await attachHookPath(command, command);
};

const attachHookPath = async (path: string, label: string) => {
  manualHookLoading.value = true;
  executableAttachResult.value = null;
  try {
    if (manualHookType.value === "executable") {
      const response = await axios.post("/tls-capture/executable", {
        path,
        pid: manualHookPid.value || 0,
        library: executableLibraryHint.value,
      });
      executableAttachResult.value = response.data?.result || null;
    } else if (manualHookType.value === "go") {
      const response = await axios.post("/tls-capture/go-binary", {
        path,
        pid: manualHookPid.value || 0,
      });
      executableAttachResult.value = response.data?.resolved
        ? { resolved: response.data.resolved }
        : null;
    } else {
      await axios.post("/tls-capture/library", {
        path,
        library: manualHookType.value,
      });
    }
    message.success(`Hook attached for ${label}`);
    await Promise.all([fetchStatus(), fetchLibraries()]);
  } catch (error: any) {
    executableAttachResult.value = error?.response?.data?.result || null;
    message.error(
      error?.response?.data?.error || "Failed to attach manual hook",
    );
  } finally {
    manualHookLoading.value = false;
  }
};

const attachManualHook = async (entry: FileEntry) => {
  if (entry.isDir) {
    message.warning("Select a TLS library, Go binary, or executable file");
    return;
  }
  executablePathInput.value = entry.path;
  await attachHookPath(entry.path, entry.name);
};

const attachExecutableInput = async () => {
  const path = executablePathInput.value.trim();
  if (!path) {
    message.warning("Enter a binary name or absolute executable path");
    return;
  }
  await attachHookPath(path, path);
};

const fetchRules = async () => {
  try {
    const response = await axios.get("/tls-capture/rules");
    rules.value = Array.isArray(response.data?.rules)
      ? response.data.rules
      : [];
  } catch (error: any) {
    if (error?.response?.status === 403) {
      markRuntimeGateDisabled();
      rules.value = [];
      return;
    }
    message.error(
      error?.response?.data?.error || "Failed to load Hook SSL rules",
    );
  }
};

const saveRules = async () => {
  rulesLoading.value = true;
  try {
    const response = await axios.put("/tls-capture/rules", {
      rules: rules.value,
    });
    rules.value = Array.isArray(response.data?.rules)
      ? response.data.rules
      : rules.value;
    message.success("Hook SSL rules saved");
  } catch (error: any) {
    message.error(
      error?.response?.data?.error || "Failed to save Hook SSL rules",
    );
  } finally {
    rulesLoading.value = false;
  }
};

const addRule = () => {
  rules.value = [
    ...rules.value,
    {
      id: `custom-${Date.now()}`,
      name: "Custom Hook SSL rule",
      enabled: true,
      scope: "custom",
      comms: [],
      hosts: [],
      methods: [],
      libraries: [],
      directions: [],
    },
  ];
};

const removeRule = (id: string) => {
  rules.value = rules.value.filter((rule) => rule.id !== id);
};

// ── Ignore rules CRUD ──
const addIgnoreRule = () => {
  const newRules = [
    ...ignoreRules.value,
    {
      id: `ignore-${Date.now()}`,
      name: "New ignore rule",
      enabled: true,
      comms: [] as string[],
      hosts: [] as string[],
      urls: [] as string[],
      methods: [] as string[],
      libraries: [] as string[],
      directions: [] as string[],
      statusCodes: [] as string[],
    },
  ];
  ignoreRules.value = newRules;
  saveTLSIgnoreRulesToStorage(newRules);
};

const removeIgnoreRule = (id: string) => {
  const newRules = ignoreRules.value.filter((r) => r.id !== id);
  ignoreRules.value = newRules;
  saveTLSIgnoreRulesToStorage(newRules);
};

const saveIgnoreRules = () => {
  ignoreRulesLoading.value = true;
  try {
    saveTLSIgnoreRulesToStorage(ignoreRules.value);
    message.success("Ignore rules saved (local storage)");
  } catch (err: any) {
    message.error("Failed to save ignore rules");
  } finally {
    ignoreRulesLoading.value = false;
  }
};

const connectWebSocket = () => {
  if (!shouldReconnect || runtimeGateDisabled) return;
  if (
    ws?.readyState === WebSocket.CONNECTING ||
    ws?.readyState === WebSocket.OPEN
  )
    return;
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.onerror = null;
    ws.close();
    ws = null;
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  const socket = new WebSocket(buildWebSocketUrl("/ws/tls-capture"));
  ws = socket;

  socket.onopen = () => {
    if (ws !== socket) return;
    isConnected.value = true;
    void fetchStatus(true);
  };

  socket.onmessage = (event) => {
    if (ws !== socket) return;
    if (isPaused.value) return;
    try {
      const payload = JSON.parse(String(event.data)) as TLSPlaintextEvent;
      if (isTLSDisplayEvent(payload)) {
        events.value = [withEventKey(payload), ...events.value].slice(
          0,
          TLS_CAPTURE_EVENT_LIMIT,
        );
      }
    } catch (error) {
      console.error("TLS capture websocket parse error", error);
    }
  };

  socket.onclose = () => {
    if (ws !== socket) return;
    ws = null;
    isConnected.value = false;
    if (shouldReconnect && !runtimeGateDisabled) {
      void fetchStatus(true);
      if (reconnectTimer) clearTimeout(reconnectTimer);
      reconnectTimer = setTimeout(
        connectWebSocket,
        TLS_CAPTURE_RECONNECT_DELAY_MS,
      );
    }
  };

  socket.onerror = () => {
    if (ws !== socket) return;
    isConnected.value = false;
  };
};

const refreshData = async () => {
  await Promise.all([fetchRecentEvents(), fetchStatus(), fetchRules()]);
};

const openDetails = (event: TLSPlaintextEvent) => {
  selectedEvent.value = event;
  showDetails.value = true;
};

const clearFilters = () => {
  searchQuery.value = "";
  commFilter.value = "";
  hostFilter.value = "";
  selectedLib.value = "all";
  selectedDirection.value = "all";
  sslFilterExpr.value = "";
  ignoreFilter.value = "";
};

const copyEventBody = async (event: TLSPlaintextEvent) => {
  await navigator.clipboard.writeText(event.body || event.raw_hex_dump || "");
  message.success("Body copied");
};

onMounted(async () => {
  componentMounted = true;
  await refreshData();
  if (!componentMounted) return;
  if (
    !runtimeGateDisabled &&
    (!captureStatus.value.enabled || summaryStats.value.attachedLibs === 0)
  ) {
    await attachDefaultLibraries(true);
    if (!componentMounted) return;
  }
  if (!runtimeGateDisabled) connectWebSocket();
  statusRefreshTimer = setInterval(() => {
    void fetchStatus(true);
  }, TLS_CAPTURE_STATUS_REFRESH_MS);
});

onUnmounted(() => {
  componentMounted = false;
  shouldReconnect = false;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (statusRefreshTimer) {
    clearInterval(statusRefreshTimer);
    statusRefreshTimer = null;
  }
  if (ws) {
    const socket = ws;
    ws = null;
    socket.onopen = null;
    socket.onmessage = null;
    socket.onclose = null;
    socket.onerror = null;
    socket.close();
  }
});
</script>

<template>
  <div class="tls-capture-page">
    <div class="tls-redaction-bar">
      <RedactionBadge level="standard" />
    </div>
    <a-card :bordered="false" class="tls-card">
      <template #title>
        <span class="tls-title"><SafetyCertificateOutlined /> TLS Capture</span>
      </template>
      <template #extra>
        <a-space>
          <a-badge
            :status="isConnected ? 'success' : 'error'"
            :text="isConnected ? 'Live' : 'Offline'"
          />
          <a-tag color="purple">{{ summaryStats.total }} events</a-tag>
          <a-button size="small" @click="refreshData">
            <template #icon><ReloadOutlined /></template>
            Refresh
          </a-button>
        </a-space>
      </template>

      <a-alert
        type="info"
        show-icon
        class="tls-rules-hint"
        message="Hook SSL uses eBPF uprobes on common TLS libraries"
        description="OpenSSL/libssl, GnuTLS, NSS/NSPR, and Go crypto/tls symbols are attached when TLS capture is enabled. Independent Hook SSL rules decide which plaintext events are retained; by default only agent CLI tagged processes are shown."
      />

      <TLSCaptureRuntimeStatus
        :capture-status="captureStatus"
        :capture-status-text="captureStatusText"
        :capture-status-color="captureStatusColor"
        :is-connected="isConnected"
        :attached-libraries="summaryStats.attachedLibs"
        :attach-loading="attachLoading"
        @attach="attachDefaultLibraries()"
        @refresh="refreshData"
      />

      <TLSHookManagement
        v-model:active-tab="hookManagementTab"
        v-model:rules="rules"
        v-model:ignore-rules="ignoreRules"
        v-model:manual-hook-type="manualHookType"
        v-model:manual-hook-pid="manualHookPid"
        v-model:executable-library-hint="executableLibraryHint"
        v-model:executable-path-input="executablePathInput"
        :libraries="libraries"
        :rules-loading="rulesLoading"
        :ignore-rules-loading="ignoreRulesLoading"
        :builtin-attach-loading="builtinAttachLoading"
        :manual-hook-loading="manualHookLoading"
        :builtin-attach-statuses="builtinAttachStatuses"
        :executable-attach-result="executableAttachResult"
        @add-rule="addRule"
        @save-rules="saveRules"
        @remove-rule="removeRule"
        @attach-builtins="attachBuiltinExecutables"
        @attach-builtin-command="attachBuiltinCommand"
        @attach-executable="attachExecutableInput"
        @attach-manual-hook="attachManualHook"
        @add-ignore-rule="addIgnoreRule"
        @save-ignore-rules="saveIgnoreRules"
        @remove-ignore-rule="removeIgnoreRule"
        @persist-ignore-rules="saveTLSIgnoreRulesToStorage(ignoreRules)"
      />

      <TLSCaptureEventsPanel
        v-model:is-paused="isPaused"
        v-model:search-query="searchQuery"
        v-model:comm-filter="commFilter"
        v-model:host-filter="hostFilter"
        v-model:selected-lib="selectedLib"
        v-model:selected-direction="selectedDirection"
        v-model:ssl-filter-expr="sslFilterExpr"
        v-model:ignore-filter="ignoreFilter"
        :summary-stats="summaryStats"
        :events-count="events.length"
        :filtered-events="filteredEvents"
        :libraries="libraries"
        @clear-filters="clearFilters"
        @open-details="openDetails"
        @copy-body="copyEventBody"
      />
    </a-card>

    <TLSCaptureDetailsModal
      v-model:open="showDetails"
      :selected-event="selectedEvent"
    />
  </div>
</template>

<style scoped>
.tls-capture-page {
  padding: 0;
}

.tls-redaction-bar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
}

.tls-card {
  min-height: 320px;
}

.tls-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.tls-rules-hint {
  margin-bottom: 16px;
}
</style>
