<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import axios from "axios";
import {
  SafetyCertificateOutlined,
  PauseOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  CopyOutlined,
} from "@ant-design/icons-vue";
import { message } from "ant-design-vue";

import FileBrowserPanel from "../../components/explorer/FileBrowserPanel.vue";
import { buildWebSocketUrl } from "../../utils/requestContext";

interface TLSPlaintextEvent {
  key?: string;
  type?: string;
  timestamp?: string;
  pid?: number;
  tgid?: number;
  comm?: string;
  direction?: string;
  lib?: string;
  function?: string;
  captured_len?: number;
  original_len?: number;
  method?: string;
  url?: string;
  host?: string;
  status?: number;
  headers?: Record<string, string>;
  body?: string;
  body_size?: number;
  content_type?: string;
  raw_hex_dump?: string;
  raw_available?: boolean;
  truncated?: boolean;
  redaction_state?: string;
  sse_event?: string;
  sse_data_digest?: string;
  message_role?: string;
  prompt_digest?: string;
  prompt_len?: number;
  vendor?: string;
}

interface TLSLibraryStatus {
  library?: number;
  name: string;
  path?: string;
  attached: boolean;
  available?: boolean;
  error?: string;
}

interface TLSCaptureRule {
  id: string;
  name: string;
  enabled: boolean;
  scope: string;
  comms?: string[];
  hosts?: string[];
  methods?: string[];
  libraries?: string[];
  directions?: string[];
  description?: string;
}

interface TLSCaptureStatus {
  enabled?: boolean;
  available?: boolean;
  readStarted?: boolean;
  error?: string;
  libraries?: TLSLibraryStatus[];
}

interface TLSBuiltinExecutableTarget {
  name: string;
  command: string;
  library: string;
  description?: string;
}

interface TLSBuiltinExecutableAttachStatus {
  target: TLSBuiltinExecutableTarget;
  available?: boolean;
  attached?: boolean;
  result?: any;
  error?: string;
}

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
}

const events = ref<TLSPlaintextEvent[]>([]);
const libraries = ref<TLSLibraryStatus[]>([]);
const rules = ref<TLSCaptureRule[]>([]);
const captureStatus = ref<TLSCaptureStatus>({});
const isConnected = ref(false);
const isPaused = ref(false);
const searchQuery = ref("");
const commFilter = ref("");
const hostFilter = ref("");
const selectedLib = ref<string>("all");
const selectedDirection = ref<string>("all");
const showDetails = ref(false);
const selectedEvent = ref<TLSPlaintextEvent | null>(null);
const rulesLoading = ref(false);
const attachLoading = ref(false);
const builtinAttachLoading = ref(false);
const manualHookLoading = ref(false);
const hookManagementTab = ref("rules");
const manualHookType = ref<"executable" | "go" | "openssl" | "gnutls" | "nss">(
  "executable",
);
const manualHookPid = ref<number | null>(null);
const builtinAttachStatuses = ref<TLSBuiltinExecutableAttachStatus[]>([]);
const executablePathInput = ref("");
const executableLibraryHint = ref<"auto" | "openssl" | "gnutls" | "nss">(
  "auto",
);
const executableAttachResult = ref<any | null>(null);

const manualHookOptions = [
  { label: "Executable / CLI bin", value: "executable" },
  { label: "Go TLS binary", value: "go" },
  { label: "OpenSSL libssl", value: "openssl" },
  { label: "GnuTLS library", value: "gnutls" },
  { label: "NSS / NSPR library", value: "nss" },
];

const executableLibraryOptions = [
  { label: "Auto detect", value: "auto" },
  { label: "OpenSSL", value: "openssl" },
  { label: "GnuTLS", value: "gnutls" },
  { label: "NSS / NSPR", value: "nss" },
];

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let shouldReconnect = true;
let eventKeySequence = 0;

const formatBytes = (bytes?: number) => {
  const value = Number(bytes || 0);
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
};

const formatTimestamp = (timestamp?: string) => {
  if (!timestamp) return "—";
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString();
};

const withEventKey = (event: TLSPlaintextEvent): TLSPlaintextEvent => {
  if (event.key) return event;
  eventKeySequence += 1;
  return {
    ...event,
    key: `${event.timestamp ?? "ts"}-${event.pid ?? 0}-${event.direction ?? "dir"}-${eventKeySequence}`,
  };
};
const isRequestEvent = (event: TLSPlaintextEvent) =>
  event.type === "http_request";
const isResponseEvent = (event: TLSPlaintextEvent) =>
  event.type === "http_response" || event.type === "sse_message";
const isDisplayEvent = (event: TLSPlaintextEvent) =>
  isRequestEvent(event) || isResponseEvent(event);
const directionLabel = (direction?: string) =>
  direction === "send" ? "Request" : direction === "recv" ? "Response" : "—";
const directionColor = (direction?: string) =>
  direction === "send" ? "green" : direction === "recv" ? "blue" : "default";
const packetTypeLabel = (event: TLSPlaintextEvent) => {
  if (event.type === "http_request") return "HTTP Request";
  if (event.type === "http_response") return "HTTP Response";
  if (event.type === "sse_message") return "SSE Response";
  return "—";
};
const packetTypeColor = (event: TLSPlaintextEvent) =>
  event.type === "sse_message"
    ? "cyan"
    : isRequestEvent(event)
      ? "green"
      : isResponseEvent(event)
        ? "blue"
        : "default";

const filteredEvents = computed(() => {
  let list = events.value.filter(isDisplayEvent);

  if (selectedLib.value !== "all") {
    list = list.filter(
      (event) =>
        (event.lib || "").toLowerCase() === selectedLib.value.toLowerCase(),
    );
  }
  if (selectedDirection.value !== "all") {
    list = list.filter(
      (event) =>
        (event.direction || "").toLowerCase() ===
        selectedDirection.value.toLowerCase(),
    );
  }
  if (commFilter.value.trim()) {
    const q = commFilter.value.trim().toLowerCase();
    list = list.filter((event) => (event.comm || "").toLowerCase().includes(q));
  }
  if (hostFilter.value.trim()) {
    const q = hostFilter.value.trim().toLowerCase();
    list = list.filter((event) => (event.host || "").toLowerCase().includes(q));
  }
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.trim().toLowerCase();
    list = list.filter((event) =>
      [
        event.method,
        event.url,
        event.host,
        String(event.status || ""),
        event.body,
        JSON.stringify(event.headers || {}),
      ].some((value) => (value || "").toLowerCase().includes(q)),
    );
  }

  return list;
});

const summaryStats = computed(() => {
  const list = filteredEvents.value;
  return {
    total: list.length,
    sends: list.filter(isRequestEvent).length,
    recvs: list.filter(isResponseEvent).length,
    withBody: list.filter((event) => Number(event.body_size || 0) > 0).length,
    http: list.filter(
      (event) =>
        event.type === "http_request" || event.type === "http_response",
    ).length,
    sse: list.filter((event) => event.type === "sse_message").length,
    llm: list.filter((event) => event.prompt_digest || event.vendor).length,
    redacted: list.filter((event) => event.redaction_state === "sanitized")
      .length,
    attachedLibs: libraries.value.filter((item) => item.attached).length,
  };
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
    const response = await axios.get("/tls-capture/recent?limit=500");
    const recentEvents = Array.isArray(response.data?.events)
      ? (response.data.events as TLSPlaintextEvent[])
      : [];
    events.value = recentEvents.filter(isDisplayEvent).map(withEventKey);
  } catch (error: any) {
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
    message.error(
      error?.response?.data?.error || "Failed to load TLS capture libraries",
    );
  }
};

const fetchStatus = async () => {
  try {
    const response = await axios.get("/tls-capture/status");
    captureStatus.value = response.data || {};
    if (Array.isArray(response.data?.libraries))
      libraries.value = response.data.libraries;
  } catch (error: any) {
    message.error(
      error?.response?.data?.error || "Failed to load Hook SSL status",
    );
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
    if (attached > 0) message.success(`Attached ${attached} built-in TLS target(s)`);
    else message.warning(response.data?.error || "No built-in TLS targets attached");
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

const splitRuleValues = (value: string) =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
const joinRuleValues = (values?: string[]) => (values || []).join(", ");
type TLSRuleListField =
  | "comms"
  | "hosts"
  | "methods"
  | "libraries"
  | "directions";
const updateRuleValues = (
  rule: TLSCaptureRule,
  field: TLSRuleListField,
  value: string,
) => {
  rule[field] = splitRuleValues(value);
};
const onRuleValuesChange = (
  rule: TLSCaptureRule,
  field: TLSRuleListField,
  event: Event,
) => {
  updateRuleValues(rule, field, (event.target as HTMLInputElement).value);
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();

  const socket = new WebSocket(buildWebSocketUrl("/ws/tls-capture"));
  ws = socket;

  socket.onopen = () => {
    isConnected.value = true;
  };

  socket.onmessage = (event) => {
    if (isPaused.value) return;
    try {
      const payload = JSON.parse(String(event.data)) as TLSPlaintextEvent;
      if (isDisplayEvent(payload)) {
        events.value = [withEventKey(payload), ...events.value].slice(0, 500);
      }
    } catch (error) {
      console.error("TLS capture websocket parse error", error);
    }
  };

  socket.onclose = () => {
    isConnected.value = false;
    if (shouldReconnect) {
      reconnectTimer = setTimeout(connectWebSocket, 3000);
    }
  };

  socket.onerror = () => {
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
};

const copyText = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};

const buildCurl = (event: TLSPlaintextEvent): string => {
  const target =
    event.host && (event.url || "").startsWith("/")
      ? `https://${event.host}${event.url}`
      : event.url || "https://example.invalid";
  const parts = ["curl", "-X", event.method || "GET"];
  Object.entries(event.headers || {}).forEach(([key, value]) => {
    if (value !== "***REDACTED***") {
      parts.push("-H", `${key}: ${value}`);
    }
  });
  if (event.body) parts.push("--data", event.body);
  parts.push(target);
  return parts.map((part) => `'${part.replaceAll("'", "'\\''")}'`).join(" ");
};

onMounted(async () => {
  await refreshData();
  if (!captureStatus.value.enabled || summaryStats.value.attachedLibs === 0) {
    await attachDefaultLibraries(true);
  }
  connectWebSocket();
});

onUnmounted(() => {
  shouldReconnect = false;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.close();
    ws = null;
  }
});
</script>

<template>
  <div class="tls-capture-page">
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

      <a-card size="small" class="tls-runtime-card">
        <a-space wrap>
          <a-tag :color="captureStatusColor">{{ captureStatusText }}</a-tag>
          <a-tag :color="isConnected ? 'green' : 'red'"
            >WebSocket {{ isConnected ? "live" : "offline" }}</a-tag
          >
          <a-tag color="blue"
            >{{ summaryStats.attachedLibs }} attached libraries</a-tag
          >
          <a-button
            type="primary"
            size="small"
            :loading="attachLoading"
            @click="() => attachDefaultLibraries()"
          >
            Start / Attach SSL hooks
          </a-button>
          <a-button size="small" :loading="attachLoading" @click="refreshData"
            >Refresh status</a-button
          >
        </a-space>
        <a-alert
          v-if="captureStatus.error"
          type="warning"
          show-icon
          class="tls-runtime-error"
          :message="captureStatus.error"
        />
      </a-card>

      <a-card size="small" title="Hook SSL Management" class="tls-rules-card">
        <a-tabs v-model:activeKey="hookManagementTab" size="small">
          <a-tab-pane key="rules" tab="Rules">
            <div class="tls-tab-actions">
              <a-space>
                <a-button size="small" @click="addRule">Add Rule</a-button>
                <a-button
                  size="small"
                  type="primary"
                  :loading="rulesLoading"
                  @click="saveRules"
                  >Save Rules</a-button
                >
              </a-space>
            </div>
            <a-list :data-source="rules" size="small" class="tls-rule-list">
              <template #renderItem="{ item }">
                <a-list-item class="tls-rule-item">
                  <div class="tls-rule-card">
                    <div class="tls-rule-header">
                      <a-space wrap>
                        <a-switch
                          v-model:checked="item.enabled"
                          checked-children="on"
                          un-checked-children="off"
                        />
                        <a-input
                          v-model:value="item.name"
                          size="small"
                          class="tls-rule-name"
                          placeholder="Rule name"
                        />
                        <a-select
                          v-model:value="item.scope"
                          size="small"
                          class="tls-rule-scope"
                          :options="[
                            { label: 'Agent CLI tag', value: 'agent_cli_tag' },
                            { label: 'Custom', value: 'custom' },
                          ]"
                        />
                        <a-tag v-if="item.id === 'agent-cli-tag'" color="green"
                          >default</a-tag
                        >
                        <a-tag
                          v-else-if="item.scope === 'agent_cli_tag'"
                          color="cyan"
                          >agent context</a-tag
                        >
                      </a-space>
                      <a-button
                        v-if="item.id !== 'agent-cli-tag'"
                        size="small"
                        danger
                        ghost
                        @click="removeRule(item.id)"
                        >Remove</a-button
                      >
                    </div>

                    <div class="tls-rule-fields">
                      <label class="tls-rule-field">
                        <span>Commands</span>
                        <a-input
                          size="small"
                          placeholder="claude, cursor, node"
                          :value="joinRuleValues(item.comms)"
                          @change="onRuleValuesChange(item, 'comms', $event)"
                        />
                      </label>
                      <label class="tls-rule-field">
                        <span>Hosts</span>
                        <a-input
                          size="small"
                          placeholder="api.anthropic.com, github.com"
                          :value="joinRuleValues(item.hosts)"
                          @change="onRuleValuesChange(item, 'hosts', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Methods</span>
                        <a-input
                          size="small"
                          placeholder="POST, GET"
                          :value="joinRuleValues(item.methods)"
                          @change="onRuleValuesChange(item, 'methods', $event)"
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Libraries</span>
                        <a-input
                          size="small"
                          placeholder="openssl, gnutls"
                          :value="joinRuleValues(item.libraries)"
                          @change="
                            onRuleValuesChange(item, 'libraries', $event)
                          "
                        />
                      </label>
                      <label class="tls-rule-field compact">
                        <span>Directions</span>
                        <a-input
                          size="small"
                          placeholder="send, recv"
                          :value="joinRuleValues(item.directions)"
                          @change="
                            onRuleValuesChange(item, 'directions', $event)
                          "
                        />
                      </label>
                    </div>

                    <a-typography-text type="secondary" class="tls-rule-help">
                      {{
                        item.description ||
                        "All filled fields must match. Empty fields match any value."
                      }}
                    </a-typography-text>
                  </div>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>

          <a-tab-pane key="libraries" tab="Libraries">
            <a-list :data-source="libraries" size="small" bordered>
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-list-item-meta :description="item.path || '—'">
                    <template #title>
                      <a-space>
                        <span>{{ item.name }}</span>
                        <a-tag :color="item.attached ? 'green' : 'default'">{{
                          item.attached ? "Attached" : "Not attached"
                        }}</a-tag>
                        <a-tag v-if="item.available === false" color="red"
                          >Unavailable</a-tag
                        >
                      </a-space>
                    </template>
                  </a-list-item-meta>
                  <template #actions>
                    <span v-if="item.error" class="tls-error">{{
                      item.error
                    }}</span>
                  </template>
                </a-list-item>
              </template>
            </a-list>
          </a-tab-pane>

          <a-tab-pane key="manual" tab="Manual Hook">
            <a-alert
              type="info"
              show-icon
              class="tls-manual-hint"
              message="Select a local TLS library, Go binary, or executable hook target"
              description="Executable mode accepts a command name or binary path such as claude, /usr/local/bin/claude, node, deno, bun, codex, or a symlink/shebang CLI wrapper. The backend resolves symlinks and #! interpreters before attaching TLS uprobes."
            />
            <a-card size="small" class="tls-builtin-card">
              <template #title>Built-in SSL client binaries</template>
              <template #extra>
                <a-button
                  size="small"
                  type="primary"
                  :loading="builtinAttachLoading"
                  @click="attachBuiltinExecutables"
                  >Attach built-ins</a-button
                >
              </template>
              <a-space wrap>
                <a-button size="small" @click="attachBuiltinCommand('node')">node</a-button>
                <a-button size="small" @click="attachBuiltinCommand('deno')">deno</a-button>
                <a-button size="small" @click="attachBuiltinCommand('bun')">bun</a-button>
                <a-button size="small" @click="attachBuiltinCommand('codex')">codex</a-button>
                <a-button size="small" @click="attachBuiltinCommand('claude')">claude</a-button>
                <a-button size="small" @click="attachBuiltinCommand('gemini')">gemini</a-button>
              </a-space>
              <a-list
                v-if="builtinAttachStatuses.length"
                :data-source="builtinAttachStatuses"
                size="small"
                class="tls-builtin-status-list"
              >
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-list-item-meta
                      :title="`${item.target?.name || item.target?.command} (${item.target?.command})`"
                      :description="
                        item.result?.attachPath ||
                        item.result?.resolved?.realPath ||
                        item.error ||
                        item.target?.description
                      "
                    />
                    <template #actions>
                      <a-tag :color="item.attached ? 'green' : item.available ? 'orange' : 'red'">
                        {{ item.attached ? 'Attached' : item.available ? 'Available' : 'Missing' }}
                      </a-tag>
                    </template>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>
            <a-space wrap class="tls-manual-controls">
              <span class="tls-manual-label">Target type</span>
              <a-select
                v-model:value="manualHookType"
                size="small"
                style="width: 190px"
                :options="manualHookOptions"
              />
              <template v-if="manualHookType === 'executable'">
                <span class="tls-manual-label">TLS symbols</span>
                <a-select
                  v-model:value="executableLibraryHint"
                  size="small"
                  style="width: 150px"
                  :options="executableLibraryOptions"
                />
              </template>
              <template
                v-if="
                  manualHookType === 'executable' || manualHookType === 'go'
                "
              >
                <span class="tls-manual-label">PID</span>
                <a-input-number
                  v-model:value="manualHookPid"
                  size="small"
                  :min="0"
                  placeholder="0 = all"
                  style="width: 120px"
                />
              </template>
              <a-tag v-if="manualHookLoading" color="blue">Attaching…</a-tag>
            </a-space>

            <a-input-search
              v-if="manualHookType === 'executable'"
              v-model:value="executablePathInput"
              class="tls-executable-input"
              placeholder="claude, /usr/local/bin/claude, /usr/bin/node, or /proc/<pid>/exe"
              enter-button="Hook executable"
              :loading="manualHookLoading"
              @search="attachExecutableInput"
            />

            <a-alert
              v-if="executableAttachResult"
              :type="executableAttachResult.error ? 'warning' : 'success'"
              show-icon
              class="tls-manual-hint"
              :message="
                executableAttachResult.error
                  ? 'Executable hook attach failed'
                  : 'Executable hook target resolved'
              "
            >
              <template #description>
                <a-descriptions size="small" :column="1" bordered>
                  <a-descriptions-item label="Input">{{
                    executableAttachResult.resolved?.input ||
                    executablePathInput ||
                    "—"
                  }}</a-descriptions-item>
                  <a-descriptions-item label="Resolved path">{{
                    executableAttachResult.resolved?.realPath ||
                    executableAttachResult.resolved?.path ||
                    "—"
                  }}</a-descriptions-item>
                  <a-descriptions-item
                    v-if="executableAttachResult.resolved?.shebang"
                    label="Shebang"
                    >{{
                      executableAttachResult.resolved.shebang
                    }}</a-descriptions-item
                  >
                  <a-descriptions-item label="Attach path">{{
                    executableAttachResult.attachPath ||
                    executableAttachResult.resolved?.realPath ||
                    "—"
                  }}</a-descriptions-item>
                  <a-descriptions-item label="Mode">{{
                    executableAttachResult.targetKind ||
                    executableAttachResult.library ||
                    "resolved"
                  }}</a-descriptions-item>
                </a-descriptions>
              </template>
            </a-alert>

            <FileBrowserPanel
              action-type="emit"
              action-label="Hook"
              :show-tracking-controls="false"
              :show-upload="false"
              :file-action-only="true"
              alert-message=""
              alert-description=""
              preview-title="TLS Hook File Preview"
              @action="attachManualHook"
            />
          </a-tab-pane>
        </a-tabs>
      </a-card>

      <a-row :gutter="16" class="tls-stats">
        <a-col :xs="12" :sm="6">
          <a-statistic title="Total" :value="summaryStats.total" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Requests" :value="summaryStats.sends" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Responses" :value="summaryStats.recvs" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic
            title="Attached Libraries"
            :value="summaryStats.attachedLibs"
          />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="HTTP" :value="summaryStats.http" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="SSE" :value="summaryStats.sse" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="LLM Metadata" :value="summaryStats.llm" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Sanitized" :value="summaryStats.redacted" />
        </a-col>
      </a-row>

      <a-space wrap class="tls-toolbar">
        <a-button
          @click="isPaused = !isPaused"
          :type="isPaused ? 'primary' : 'default'"
          danger
          size="small"
        >
          <template #icon
            ><PauseOutlined v-if="isPaused" /><PlayCircleOutlined v-else
          /></template>
          {{ isPaused ? "Resume" : "Pause" }}
        </a-button>
        <a-input
          v-model:value="searchQuery"
          size="small"
          placeholder="Search URL, headers, body"
          allow-clear
          style="width: 220px"
        >
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <a-input
          v-model:value="commFilter"
          size="small"
          placeholder="Command filter"
          allow-clear
          style="width: 180px"
        />
        <a-input
          v-model:value="hostFilter"
          size="small"
          placeholder="Host filter"
          allow-clear
          style="width: 180px"
        />
        <a-select
          v-model:value="selectedLib"
          size="small"
          style="width: 160px"
          :options="[
            { label: 'All libraries', value: 'all' },
            ...libraries.map((item) => ({
              label: item.name,
              value: item.name,
            })),
          ]"
        />
        <a-select
          v-model:value="selectedDirection"
          size="small"
          style="width: 120px"
          :options="[
            { label: 'All directions', value: 'all' },
            { label: 'Send', value: 'send' },
            { label: 'Recv', value: 'recv' },
          ]"
        />
        <a-button size="small" @click="clearFilters">Clear Filters</a-button>
      </a-space>

      <a-empty
        v-if="events.length === 0"
        description="暂无完整 HTTP 请求/返回包 — 请确保后端已启动且 eBPF TLS 探针已挂载"
      />
      <a-empty
        v-else-if="filteredEvents.length === 0"
        description="无匹配请求/返回包，请调整过滤条件"
      />

      <a-table
        :data-source="filteredEvents"
        row-key="key"
        size="small"
        :pagination="{ pageSize: 20, showSizeChanger: true }"
        :scroll="{ x: 1200 }"
      >
        <a-table-column
          title="Time"
          data-index="timestamp"
          key="timestamp"
          width="180"
        >
          <template #default="{ text }">{{ formatTimestamp(text) }}</template>
        </a-table-column>
        <a-table-column
          title="Packet"
          data-index="direction"
          key="direction"
          width="110"
        >
          <template #default="{ text }">
            <a-tag :color="directionColor(text)">{{
              directionLabel(text)
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Library"
          data-index="lib"
          key="lib"
          width="120"
        />
        <a-table-column
          title="Command"
          data-index="comm"
          key="comm"
          width="140"
          ellipsis
        />
        <a-table-column
          title="Host"
          data-index="host"
          key="host"
          width="180"
          ellipsis
        />
        <a-table-column title="Type" key="type" width="140">
          <template #default="{ record }">
            <a-tag :color="packetTypeColor(record)">{{
              packetTypeLabel(record)
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Method"
          data-index="method"
          key="method"
          width="90"
        />
        <a-table-column
          title="Status"
          data-index="status"
          key="status"
          width="90"
        />
        <a-table-column title="URL" data-index="url" key="url" ellipsis />
        <a-table-column
          title="Redaction"
          data-index="redaction_state"
          key="redaction_state"
          width="110"
        >
          <template #default="{ text }">
            <a-tag :color="text === 'sanitized' ? 'green' : 'default'">{{
              text || "raw"
            }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column
          title="Body Size"
          data-index="body_size"
          key="body_size"
          width="110"
          align="right"
        >
          <template #default="{ text }">{{ formatBytes(text) }}</template>
        </a-table-column>
        <a-table-column title="" key="action" width="160" fixed="right">
          <template #default="{ record }">
            <a-space :size="4">
              <a-button type="link" size="small" @click="openDetails(record)"
                >Detail</a-button
              >
              <a-button
                type="link"
                size="small"
                @click="
                  copyText(record.body || record.raw_hex_dump || '', 'Body')
                "
              >
                <template #icon><CopyOutlined /></template>
              </a-button>
            </a-space>
          </template>
        </a-table-column>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="showDetails"
      :title="
        selectedEvent ? packetTypeLabel(selectedEvent) : 'TLS HTTP Packet'
      "
      :footer="null"
      width="820px"
    >
      <template v-if="selectedEvent">
        <a-space style="margin-bottom: 12px">
          <a-button
            size="small"
            @click="
              copyText(
                selectedEvent.body || selectedEvent.raw_hex_dump || '',
                'Body',
              )
            "
          >
            <template #icon><CopyOutlined /></template>Copy Body
          </a-button>
          <a-button
            v-if="isRequestEvent(selectedEvent)"
            size="small"
            @click="copyText(buildCurl(selectedEvent), 'cURL')"
          >
            <template #icon><CopyOutlined /></template>Copy cURL
          </a-button>
        </a-space>
        <a-descriptions bordered :column="1" size="small">
          <a-descriptions-item label="Timestamp">{{
            formatTimestamp(selectedEvent.timestamp)
          }}</a-descriptions-item>
          <a-descriptions-item label="Packet"
            ><a-tag :color="directionColor(selectedEvent.direction)">{{
              directionLabel(selectedEvent.direction)
            }}</a-tag></a-descriptions-item
          >
          <a-descriptions-item label="Library">{{
            selectedEvent.lib || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Function">{{
            selectedEvent.function || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Command">{{
            selectedEvent.comm || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="PID">{{
            selectedEvent.pid ?? "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="TGID">{{
            selectedEvent.tgid ?? "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Method">{{
            selectedEvent.method || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="URL"
            ><a-typography-text code style="word-break: break-all">{{
              selectedEvent.url || "—"
            }}</a-typography-text></a-descriptions-item
          >
          <a-descriptions-item label="Host">{{
            selectedEvent.host || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Status">{{
            selectedEvent.status ?? "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Content Type">{{
            selectedEvent.content_type || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Body Size">{{
            formatBytes(selectedEvent.body_size)
          }}</a-descriptions-item>
          <a-descriptions-item label="TLS Capture"
            >{{ formatBytes(selectedEvent.captured_len) }} /
            {{ formatBytes(selectedEvent.original_len) }}</a-descriptions-item
          >
          <a-descriptions-item label="Redaction"
            ><a-tag
              :color="
                selectedEvent.redaction_state === 'sanitized'
                  ? 'green'
                  : 'default'
              "
              >{{ selectedEvent.redaction_state || "raw" }}</a-tag
            ></a-descriptions-item
          >
          <a-descriptions-item
            v-if="selectedEvent.sse_event || selectedEvent.sse_data_digest"
            label="SSE"
          >
            <a-space wrap>
              <a-tag v-if="selectedEvent.sse_event" color="cyan">{{
                selectedEvent.sse_event
              }}</a-tag>
              <a-typography-text v-if="selectedEvent.sse_data_digest" code>{{
                selectedEvent.sse_data_digest
              }}</a-typography-text>
            </a-space>
          </a-descriptions-item>
          <a-descriptions-item
            v-if="selectedEvent.vendor || selectedEvent.prompt_digest"
            label="LLM Metadata"
          >
            <a-space wrap>
              <a-tag v-if="selectedEvent.vendor" color="purple">{{
                selectedEvent.vendor
              }}</a-tag>
              <a-tag v-if="selectedEvent.message_role" color="blue">{{
                selectedEvent.message_role
              }}</a-tag>
              <a-typography-text v-if="selectedEvent.prompt_digest" code>{{
                selectedEvent.prompt_digest
              }}</a-typography-text>
              <span v-if="selectedEvent.prompt_len"
                >{{ selectedEvent.prompt_len }} chars</span
              >
            </a-space>
          </a-descriptions-item>
          <a-descriptions-item label="Headers">
            <pre class="tls-pre">{{
              JSON.stringify(selectedEvent.headers || {}, null, 2)
            }}</pre>
          </a-descriptions-item>
          <a-descriptions-item label="Body">
            <pre class="tls-pre tls-body">{{ selectedEvent.body || "—" }}</pre>
          </a-descriptions-item>
          <a-descriptions-item
            v-if="selectedEvent.raw_hex_dump"
            label="Raw Hex Dump"
          >
            <pre class="tls-pre">{{ selectedEvent.raw_hex_dump }}</pre>
          </a-descriptions-item>
        </a-descriptions>
      </template>
    </a-modal>
  </div>
</template>

<style scoped>
.tls-capture-page {
  padding: 0;
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

.tls-rules-hint,
.tls-runtime-card,
.tls-rules-card,
.tls-stats {
  margin-bottom: 16px;
}

.tls-runtime-error {
  margin-top: 10px;
}

.tls-tab-actions {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}

.tls-manual-hint,
.tls-builtin-card,
.tls-manual-controls,
.tls-executable-input {
  margin-bottom: 12px;
}

.tls-builtin-status-list {
  margin-top: 10px;
}

.tls-manual-label {
  color: #64748b;
  font-size: 12px;
}

.tls-rule-list :deep(.ant-list-items) {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tls-rule-item {
  padding: 0 !important;
  border: 0 !important;
}

.tls-rule-card {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #fff;
}

.tls-rule-header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
  margin-bottom: 12px;
}

.tls-rule-name {
  width: 260px;
}

.tls-rule-scope {
  width: 170px;
}

.tls-rule-fields {
  display: grid;
  grid-template-columns: minmax(220px, 1.4fr) minmax(260px, 1.6fr) repeat(
      3,
      minmax(140px, 1fr)
    );
  gap: 10px;
  align-items: end;
}

.tls-rule-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  color: #64748b;
  font-size: 12px;
}

.tls-rule-field span {
  line-height: 18px;
}

.tls-rule-help {
  display: block;
  margin-top: 10px;
}

.tls-toolbar {
  margin-bottom: 16px;
}

.tls-libraries-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.tls-error {
  color: #cf1322;
}

.tls-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  padding: 12px;
  max-height: 240px;
  overflow: auto;
}

.tls-body {
  max-height: 320px;
}

@media (max-width: 1200px) {
  .tls-rule-fields {
    grid-template-columns: repeat(2, minmax(220px, 1fr));
  }
}

@media (max-width: 720px) {
  .tls-rule-header {
    flex-direction: column;
  }

  .tls-rule-name,
  .tls-rule-scope {
    width: 100%;
  }

  .tls-rule-fields {
    grid-template-columns: 1fr;
  }
}
</style>
