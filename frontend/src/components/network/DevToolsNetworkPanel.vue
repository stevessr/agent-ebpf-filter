<script setup lang="ts">
/**
 * DevToolsNetworkPanel — Chromium DevTools Network tab-style viewer for TLS captured events.
 *
 * Merges HTTP request + response events into unified transactions and displays them
 * in a split-panel layout with a Chrome-like request list on the left and a detail
 * panel with Headers / Payload / Response / Timing tabs on the right.
 */
import { computed, reactive, ref, watch, nextTick } from "vue";
import { CopyOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import type { TLSPlaintextEvent, MergedTransaction } from "../../types/tls";
import SanitizedFieldViewer from "../common/SanitizedFieldViewer.vue";

/* ──── Props & Emits ──── */
const props = defineProps<{
  events: TLSPlaintextEvent[];
  isPaused: boolean;
  isConnected: boolean;
}>();

const emit = defineEmits<{
  "toggle-pause": [];
  clear: [];
}>();

/* ──── Local state ──── */
const selected = ref<MergedTransaction | null>(null);
const activeDetailTab = ref<"headers" | "payload" | "response" | "timing">(
  "headers",
);
const filterText = ref("");
const activeTypeFilter = ref("all");
const preserveLog = ref(false);

const openSections = reactive<Record<string, boolean>>({
  general: true,
  resHeaders: true,
  reqHeaders: true,
  queryParams: true,
});

const toggleSection = (key: string) => {
  openSections[key] = !openSections[key];
};

/* ──── Type filter chips ──── */
const typeFilters = [
  { label: "All", value: "all" },
  { label: "Fetch/XHR", value: "xhr" },
  { label: "SSE", value: "sse" },
  { label: "JSON", value: "json" },
  { label: "LLM", value: "llm" },
];

/* ──── Helpers ──── */
const isRequestEvent = (e: TLSPlaintextEvent) => e.type === "http_request";
const isResponseEvent = (e: TLSPlaintextEvent) =>
  e.type === "http_response" || e.type === "sse_message";

const extractPathname = (url?: string): string => {
  if (!url) return "/";
  try {
    // Handle both absolute and relative URLs
    if (url.startsWith("http://") || url.startsWith("https://")) {
      const u = new URL(url);
      const path = u.pathname + u.search;
      return path || "/";
    }
    return url;
  } catch {
    return url;
  }
};

const buildFullUrl = (event: TLSPlaintextEvent): string => {
  if (!event.url) return "";
  if (
    event.url.startsWith("http://") ||
    event.url.startsWith("https://")
  )
    return event.url;
  if (event.host) return `https://${event.host}${event.url}`;
  return event.url;
};

const formatBytes = (bytes?: number): string => {
  const n = Number(bytes || 0);
  if (!n) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
};

const formatTime = (ms?: number): string => {
  if (ms === undefined || ms === null) return "—";
  if (ms < 1) return "<1 ms";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
};

const formatTimestamp = (ts?: string): string => {
  if (!ts) return "—";
  try {
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return ts;
    return d.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    });
  } catch {
    return ts;
  }
};

const statusClass = (status?: number): string => {
  if (!status) return "status-pending";
  if (status >= 200 && status < 300) return "status-ok";
  if (status >= 300 && status < 400) return "status-redirect";
  return "status-error";
};

const shortType = (ct?: string): string => {
  if (!ct) return "";
  const lower = ct.toLowerCase();
  if (lower.includes("json")) return "json";
  if (lower.includes("html")) return "html";
  if (lower.includes("javascript") || lower.includes("ecmascript"))
    return "js";
  if (lower.includes("css")) return "css";
  if (lower.includes("xml")) return "xml";
  if (lower.includes("text/plain")) return "text";
  if (lower.includes("form")) return "form";
  if (lower.includes("event-stream")) return "sse";
  if (lower.includes("image")) return "img";
  if (lower.includes("font")) return "font";
  return ct.split("/").pop()?.split(";")[0] || "";
};

const getMethodColor = (method?: string): string => {
  switch ((method || "").toUpperCase()) {
    case "GET":
      return "#188038";
    case "POST":
      return "#1967d2";
    case "PUT":
      return "#e37400";
    case "PATCH":
      return "#9334e6";
    case "DELETE":
      return "#d93025";
    default:
      return "#5f6368";
  }
};

const isJson = (body?: string): boolean => {
  if (!body) return false;
  const t = body.trim();
  return (
    (t.startsWith("{") && t.endsWith("}")) ||
    (t.startsWith("[") && t.endsWith("]"))
  );
};

const formatBody = (body?: string): string => {
  if (!body) return "";
  if (isJson(body)) {
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      return body;
    }
  }
  return body;
};

const truncateBody = (body?: string, maxLen = 8000): string => {
  const formatted = formatBody(body);
  if (formatted.length > maxLen) return formatted.slice(0, maxLen) + "\n\n… [truncated]";
  return formatted;
};

const copyText = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};

const buildCurl = (tx: MergedTransaction): string => {
  const req = tx.request;
  if (!req) return "";
  const target = tx.fullUrl || `https://${tx.host}${tx.name}`;
  const parts = ["curl", "-X", req.method || "GET"];
  Object.entries(req.headers || {}).forEach(([k, v]) => {
    if (v !== "***REDACTED***") parts.push("-H", `${k}: ${v}`);
  });
  if (req.body) parts.push("--data", req.body);
  parts.push(target);
  return parts.map((p) => `'${p.replaceAll("'", "'\\''")}'`).join(" ");
};

const parseQueryParams = (url?: string): [string, string][] => {
  if (!url) return [];
  try {
    const qIdx = url.indexOf("?");
    if (qIdx < 0) return [];
    const search = url.slice(qIdx);
    const params = new URLSearchParams(search);
    return Array.from(params.entries());
  } catch {
    return [];
  }
};

/* ──── Transaction merging ──── */
const mergedTransactions = computed<MergedTransaction[]>(() => {
  const list = props.events;
  const transactions: MergedTransaction[] = [];
  // Map from match key → index of the *most recent unmatched* request transaction
  const pendingMap = new Map<string, number>();

  // Process in chronological order (events are newest-first in the array)
  const chronological = [...list].reverse();

  for (const event of chronological) {
    const matchKey =
      `${event.tgid || event.pid}|${(event.host || "").toLowerCase()}|${event.url || ""}`.toLowerCase();

    if (isRequestEvent(event)) {
      const tx: MergedTransaction = {
        id: event.key || `tx-${transactions.length}`,
        request: event,
        name: extractPathname(event.url),
        fullUrl: buildFullUrl(event),
        host: event.host || "",
        method: event.method || "",
        type: shortType(event.content_type),
        timestamp: event.timestamp || "",
        comm: event.comm,
        pid: event.pid,
        lib: event.lib,
        isComplete: false,
        redactionState: event.redaction_state,
        vendor: event.vendor,
        isSse: false,
      };
      pendingMap.set(matchKey, transactions.length);
      transactions.push(tx);
    } else if (isResponseEvent(event)) {
      const idx = pendingMap.get(matchKey);
      if (idx !== undefined && !transactions[idx].isComplete) {
        // Merge with pending request
        const tx = transactions[idx];
        tx.response = event;
        tx.status = event.status;
        tx.size = event.body_size;
        tx.type = shortType(event.content_type) || tx.type;
        tx.isComplete = true;
        tx.isSse = event.type === "sse_message";
        if (event.vendor) tx.vendor = event.vendor;
        if (event.redaction_state) tx.redactionState = event.redaction_state;
        // Calculate latency
        if (tx.request?.timestamp && event.timestamp) {
          const reqT = new Date(tx.request.timestamp).getTime();
          const resT = new Date(event.timestamp).getTime();
          if (!isNaN(reqT) && !isNaN(resT) && resT >= reqT) {
            tx.timeMs = resT - reqT;
          }
        }
        if (event.latency_ms) tx.timeMs = event.latency_ms;
        pendingMap.delete(matchKey);
      } else {
        // Standalone response (no matching request found)
        transactions.push({
          id: event.key || `tx-${transactions.length}`,
          response: event,
          name: extractPathname(event.url),
          fullUrl: buildFullUrl(event),
          host: event.host || "",
          method: event.method || "",
          status: event.status,
          type: shortType(event.content_type),
          size: event.body_size,
          timeMs: event.latency_ms,
          timestamp: event.timestamp || "",
          comm: event.comm,
          pid: event.pid,
          lib: event.lib,
          isComplete: false,
          redactionState: event.redaction_state,
          vendor: event.vendor,
          isSse: event.type === "sse_message",
        });
      }
    }
  }

  return transactions.reverse(); // Newest first
});

/* ──── Filtering ──── */
const displayTransactions = computed(() => {
  let list = mergedTransactions.value;

  // Type filter
  if (activeTypeFilter.value !== "all") {
    list = list.filter((tx) => {
      switch (activeTypeFilter.value) {
        case "xhr":
          return !tx.isSse;
        case "sse":
          return tx.isSse;
        case "json":
          return tx.type === "json";
        case "llm":
          return !!(tx.vendor || tx.request?.vendor || tx.response?.vendor);
        default:
          return true;
      }
    });
  }

  // Text filter
  if (filterText.value.trim()) {
    const q = filterText.value.trim().toLowerCase();
    list = list.filter(
      (tx) =>
        tx.name.toLowerCase().includes(q) ||
        tx.fullUrl.toLowerCase().includes(q) ||
        tx.host.toLowerCase().includes(q) ||
        tx.method.toLowerCase().includes(q) ||
        (tx.comm || "").toLowerCase().includes(q) ||
        String(tx.status || "").includes(q) ||
        (tx.request?.body || "").toLowerCase().includes(q) ||
        (tx.response?.body || "").toLowerCase().includes(q) ||
        JSON.stringify(tx.request?.headers || {})
          .toLowerCase()
          .includes(q) ||
        JSON.stringify(tx.response?.headers || {})
          .toLowerCase()
          .includes(q),
    );
  }

  return list;
});

/* ──── Stats ──── */
const totalSize = computed(() => {
  const bytes = displayTransactions.value.reduce(
    (sum, tx) => sum + (tx.size || 0),
    0,
  );
  return formatBytes(bytes);
});

/* ──── Waterfall ──── */
const waterfallRange = computed(() => {
  const txs = displayTransactions.value;
  if (txs.length === 0) return { min: 0, max: 1 };
  let min = Infinity;
  let max = -Infinity;
  for (const tx of txs) {
    const t = new Date(tx.timestamp).getTime();
    if (!isNaN(t)) {
      if (t < min) min = t;
      if (t > max) max = t;
      if (tx.timeMs) {
        const end = t + tx.timeMs;
        if (end > max) max = end;
      }
    }
  }
  if (min === Infinity) return { min: 0, max: 1 };
  const range = max - min || 1;
  return { min, max: min + range };
});

const waterfallStyle = (tx: MergedTransaction) => {
  const { min, max } = waterfallRange.value;
  const range = max - min || 1;
  const t = new Date(tx.timestamp).getTime();
  if (isNaN(t)) return { left: "0%", width: "2px" };
  const left = ((t - min) / range) * 100;
  const duration = tx.timeMs || 50;
  const width = Math.max((duration / range) * 100, 0.5);
  return {
    left: `${Math.min(left, 99)}%`,
    width: `${Math.max(width, 0.5)}%`,
  };
};

const waterfallColor = (tx: MergedTransaction): string => {
  if (!tx.status) return "#80868b";
  if (tx.status >= 400) return "#d93025";
  if (tx.isSse) return "#9334e6";
  return "#1a73e8";
};

/* ──── Selection ──── */
const selectTx = (tx: MergedTransaction) => {
  if (selected.value?.id === tx.id) {
    selected.value = null;
  } else {
    selected.value = tx;
    activeDetailTab.value = "headers";
    // Open default sections
    openSections.general = true;
    openSections.resHeaders = true;
    openSections.reqHeaders = true;
    openSections.queryParams = true;
  }
};

/* ──── Detail computed ──── */
const selectedQueryParams = computed(() => {
  if (!selected.value) return [];
  return parseQueryParams(
    selected.value.request?.url || selected.value.response?.url,
  );
});

const hasPayload = computed(
  () => !!selected.value?.request?.body,
);
const hasResponse = computed(
  () => !!selected.value?.response?.body,
);

const detailTabs = computed(() => {
  const tabs: { key: string; label: string }[] = [
    { key: "headers", label: "Headers" },
  ];
  if (hasPayload.value) {
    tabs.push({ key: "payload", label: "Payload" });
  }
  tabs.push({ key: "response", label: "Response" });
  tabs.push({ key: "timing", label: "Timing" });
  return tabs;
});

/* Keep selected in sync when events change */
watch(
  () => mergedTransactions.value,
  (txs) => {
    if (selected.value) {
      const updated = txs.find((t) => t.id === selected.value!.id);
      if (updated) {
        selected.value = updated;
      }
    }
  },
);
</script>

<template>
  <div class="nw">
    <!-- ═══ Toolbar ═══ -->
    <div class="nw-toolbar">
      <div class="nw-toolbar-primary">
        <div class="nw-toolbar-actions">
          <button
            class="nw-icon-btn"
            :class="{ 'nw-recording': !isPaused }"
            :title="isPaused ? 'Resume recording' : 'Pause recording'"
            @click="emit('toggle-pause')"
          >
            <span v-if="!isPaused" class="nw-record-dot"></span>
            <span v-else class="nw-pause-icon">▶</span>
          </button>
          <button
            class="nw-icon-btn"
            title="Clear log"
            @click="emit('clear')"
          >
            🚫
          </button>
          <span class="nw-sep-v"></span>
        </div>

        <div class="nw-filter-wrap">
          <svg
            class="nw-filter-icon"
            viewBox="0 0 16 16"
            width="12"
            height="12"
          >
            <path
              fill="#5f6368"
              d="M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.868-3.834zm-5.242.656a4.5 4.5 0 1 1 0-9 4.5 4.5 0 0 1 0 9z"
            />
          </svg>
          <input
            v-model="filterText"
            class="nw-filter-input"
            placeholder="Filter"
            spellcheck="false"
          />
          <button
            v-if="filterText"
            class="nw-filter-clear"
            @click="filterText = ''"
          >
            ✕
          </button>
        </div>

        <div class="nw-toolbar-meta">
          <span class="nw-meta-badge">
            <span class="nw-meta-num">{{
              displayTransactions.length
            }}</span>
            requests
          </span>
          <span class="nw-meta-sep">│</span>
          <span class="nw-meta-badge">{{ totalSize }} transferred</span>
          <span class="nw-meta-sep">│</span>
          <span
            :class="[
              'nw-meta-status',
              isConnected ? 'nw-live' : 'nw-offline',
            ]"
          >
            <span class="nw-status-dot"></span>
            {{ isConnected ? "Live" : "Offline" }}
          </span>
        </div>
      </div>

      <div class="nw-toolbar-secondary">
        <button
          v-for="f in typeFilters"
          :key="f.value"
          :class="['nw-chip', { active: activeTypeFilter === f.value }]"
          @click="activeTypeFilter = f.value"
        >
          {{ f.label }}
        </button>
      </div>
    </div>

    <!-- ═══ Content: List + Detail ═══ -->
    <div class="nw-content" :class="{ 'nw-split': !!selected }">
      <!-- Request list -->
      <div class="nw-list">
        <!-- Table header -->
        <div class="nw-hdr">
          <div class="nw-col nw-col-name">Name</div>
          <div class="nw-col nw-col-status">Status</div>
          <div class="nw-col nw-col-method">Method</div>
          <div class="nw-col nw-col-type">Type</div>
          <div class="nw-col nw-col-size">Size</div>
          <div class="nw-col nw-col-time">Time</div>
          <div class="nw-col nw-col-waterfall">Waterfall</div>
        </div>

        <!-- Rows -->
        <div class="nw-body">
          <div
            v-for="tx in displayTransactions"
            :key="tx.id"
            :class="[
              'nw-row',
              {
                'nw-selected': selected?.id === tx.id,
                'nw-pending': !tx.status && !tx.isSse,
                'nw-error': tx.status && tx.status >= 400,
              },
            ]"
            @click="selectTx(tx)"
          >
            <div class="nw-col nw-col-name" :title="tx.fullUrl">
              <span class="nw-name-text">{{ tx.name || "/" }}</span>
              <span class="nw-name-host">{{ tx.host }}</span>
            </div>
            <div
              class="nw-col nw-col-status"
              :class="statusClass(tx.status)"
            >
              {{ tx.status || "" }}
            </div>
            <div class="nw-col nw-col-method">
              <span
                class="nw-method-badge"
                :style="{ color: getMethodColor(tx.method) }"
                >{{ tx.method }}</span
              >
            </div>
            <div class="nw-col nw-col-type">{{ tx.type }}</div>
            <div class="nw-col nw-col-size">{{ formatBytes(tx.size) }}</div>
            <div class="nw-col nw-col-time">{{ formatTime(tx.timeMs) }}</div>
            <div class="nw-col nw-col-waterfall">
              <div class="nw-wf-track">
                <div
                  class="nw-wf-bar"
                  :style="{
                    ...waterfallStyle(tx),
                    background: waterfallColor(tx),
                  }"
                ></div>
              </div>
            </div>
          </div>

          <!-- Empty state -->
          <div v-if="displayTransactions.length === 0" class="nw-empty">
            <div class="nw-empty-icon">📡</div>
            <span class="nw-empty-title">Recording network activity…</span>
            <span class="nw-empty-hint"
              >Perform a request or adjust filters to see results.</span
            >
          </div>
        </div>
      </div>

      <!-- ═══ Detail panel ═══ -->
      <div v-if="selected" class="nw-detail">
        <div class="nw-detail-header">
          <div class="nw-detail-tabs">
            <button
              v-for="tab in detailTabs"
              :key="tab.key"
              :class="[
                'nw-dtab',
                { active: activeDetailTab === tab.key },
              ]"
              @click="activeDetailTab = tab.key as any"
            >
              {{ tab.label }}
            </button>
          </div>
          <div class="nw-detail-actions">
            <button
              v-if="selected.request"
              class="nw-copy-btn"
              @click="copyText(buildCurl(selected), 'cURL')"
              title="Copy as cURL"
            >
              <CopyOutlined /> cURL
            </button>
            <button
              class="nw-close-btn"
              @click="selected = null"
              title="Close detail panel"
            >
              ✕
            </button>
          </div>
        </div>

        <div class="nw-detail-body">
          <!-- ── Headers tab ── -->
          <div v-if="activeDetailTab === 'headers'" class="nw-tab-content">
            <!-- General -->
            <div class="nw-hgroup">
              <div
                class="nw-hgroup-title"
                @click="toggleSection('general')"
              >
                <span
                  class="nw-caret"
                  :class="{ open: openSections.general }"
                  >▶</span
                >
                General
              </div>
              <div v-if="openSections.general" class="nw-hgroup-body">
                <div class="nw-kv">
                  <span class="nw-kv-k">Request URL:</span>
                  <span class="nw-kv-v nw-kv-url">{{
                    selected.fullUrl
                  }}</span>
                </div>
                <div class="nw-kv">
                  <span class="nw-kv-k">Request Method:</span>
                  <span
                    class="nw-kv-v"
                    :style="{ color: getMethodColor(selected.method) }"
                    >{{ selected.method }}</span
                  >
                </div>
                <div v-if="selected.status" class="nw-kv">
                  <span class="nw-kv-k">Status Code:</span>
                  <span class="nw-kv-v" :class="statusClass(selected.status)">
                    {{ selected.status
                    }}{{ selected.status >= 200 && selected.status < 300 ? " OK" : selected.status >= 400 ? " Error" : "" }}
                  </span>
                </div>
                <div class="nw-kv">
                  <span class="nw-kv-k">Remote Address:</span>
                  <span class="nw-kv-v">{{ selected.host }}</span>
                </div>
                <div class="nw-kv">
                  <span class="nw-kv-k">Process:</span>
                  <span class="nw-kv-v"
                    >{{ selected.comm || "—" }} (PID:
                    {{ selected.pid ?? "—" }})</span
                  >
                </div>
                <div class="nw-kv">
                  <span class="nw-kv-k">TLS Library:</span>
                  <span class="nw-kv-v">{{ selected.lib || "—" }}</span>
                </div>
                <div v-if="selected.vendor" class="nw-kv">
                  <span class="nw-kv-k">LLM Vendor:</span>
                  <span class="nw-kv-v nw-kv-vendor">{{
                    selected.vendor
                  }}</span>
                </div>
                <div v-if="selected.redactionState" class="nw-kv">
                  <span class="nw-kv-k">Redaction:</span>
                  <span
                    class="nw-kv-v"
                    :class="
                      selected.redactionState === 'sanitized'
                        ? 'nw-kv-redacted'
                        : ''
                    "
                    >{{ selected.redactionState }}</span
                  >
                </div>
              </div>
            </div>

            <!-- Response Headers -->
            <div
              v-if="
                selected.response?.headers &&
                Object.keys(selected.response.headers).length
              "
              class="nw-hgroup"
            >
              <div
                class="nw-hgroup-title"
                @click="toggleSection('resHeaders')"
              >
                <span
                  class="nw-caret"
                  :class="{ open: openSections.resHeaders }"
                  >▶</span
                >
                Response Headers
                <span class="nw-hgroup-count"
                  >({{
                    Object.keys(selected.response.headers).length
                  }})</span
                >
              </div>
              <div
                v-if="openSections.resHeaders"
                class="nw-hgroup-body"
              >
                <div
                  v-for="(val, key) in selected.response.headers"
                  :key="'rh-' + key"
                  class="nw-kv"
                >
                  <span class="nw-kv-k">{{ key }}:</span>
                  <span
                    :class="
                      val === '***REDACTED***'
                        ? 'nw-kv-v nw-kv-val-redacted'
                        : 'nw-kv-v'
                    "
                    >{{ val }}</span
                  >
                </div>
              </div>
            </div>

            <!-- Request Headers -->
            <div
              v-if="
                selected.request?.headers &&
                Object.keys(selected.request.headers).length
              "
              class="nw-hgroup"
            >
              <div
                class="nw-hgroup-title"
                @click="toggleSection('reqHeaders')"
              >
                <span
                  class="nw-caret"
                  :class="{ open: openSections.reqHeaders }"
                  >▶</span
                >
                Request Headers
                <span class="nw-hgroup-count"
                  >({{
                    Object.keys(selected.request.headers).length
                  }})</span
                >
              </div>
              <div
                v-if="openSections.reqHeaders"
                class="nw-hgroup-body"
              >
                <div
                  v-for="(val, key) in selected.request.headers"
                  :key="'qh-' + key"
                  class="nw-kv"
                >
                  <span class="nw-kv-k">{{ key }}:</span>
                  <span
                    :class="
                      val === '***REDACTED***'
                        ? 'nw-kv-v nw-kv-val-redacted'
                        : 'nw-kv-v'
                    "
                    >{{ val }}</span
                  >
                </div>
              </div>
            </div>

            <!-- Query String Parameters -->
            <div
              v-if="selectedQueryParams.length"
              class="nw-hgroup"
            >
              <div
                class="nw-hgroup-title"
                @click="toggleSection('queryParams')"
              >
                <span
                  class="nw-caret"
                  :class="{ open: openSections.queryParams }"
                  >▶</span
                >
                Query String Parameters
                <span class="nw-hgroup-count"
                  >({{ selectedQueryParams.length }})</span
                >
              </div>
              <div
                v-if="openSections.queryParams"
                class="nw-hgroup-body"
              >
                <div
                  v-for="([qk, qv], idx) in selectedQueryParams"
                  :key="'qp-' + idx"
                  class="nw-kv"
                >
                  <span class="nw-kv-k">{{ qk }}:</span>
                  <span class="nw-kv-v">{{ qv }}</span>
                </div>
              </div>
            </div>

            <!-- SSE metadata -->
            <div
              v-if="
                selected.response?.sse_event ||
                selected.response?.sse_data_digest
              "
              class="nw-hgroup"
            >
              <div class="nw-hgroup-title">
                <span class="nw-caret open">▶</span>
                Server-Sent Events
              </div>
              <div class="nw-hgroup-body">
                <div v-if="selected.response.sse_event" class="nw-kv">
                  <span class="nw-kv-k">Event:</span>
                  <span class="nw-kv-v">{{
                    selected.response.sse_event
                  }}</span>
                </div>
                <div
                  v-if="selected.response.sse_data_digest"
                  class="nw-kv"
                >
                  <span class="nw-kv-k">Data Digest:</span>
                  <span class="nw-kv-v nw-kv-mono">{{
                    selected.response.sse_data_digest
                  }}</span>
                </div>
              </div>
            </div>

            <!-- LLM Metadata -->
            <div
              v-if="
                selected.request?.vendor ||
                selected.response?.vendor ||
                selected.request?.prompt_digest ||
                selected.request?.message_role
              "
              class="nw-hgroup"
            >
              <div class="nw-hgroup-title">
                <span class="nw-caret open">▶</span>
                LLM Metadata
              </div>
              <div class="nw-hgroup-body">
                <div
                  v-if="selected.request?.vendor || selected.response?.vendor"
                  class="nw-kv"
                >
                  <span class="nw-kv-k">Vendor:</span>
                  <span class="nw-kv-v nw-kv-vendor">{{
                    selected.request?.vendor || selected.response?.vendor
                  }}</span>
                </div>
                <div v-if="selected.request?.message_role" class="nw-kv">
                  <span class="nw-kv-k">Message Role:</span>
                  <span class="nw-kv-v">{{
                    selected.request.message_role
                  }}</span>
                </div>
                <div v-if="selected.request?.prompt_digest" class="nw-kv">
                  <span class="nw-kv-k">Prompt Digest:</span>
                  <span class="nw-kv-v nw-kv-mono">{{
                    selected.request.prompt_digest
                  }}</span>
                </div>
                <div v-if="selected.request?.prompt_len" class="nw-kv">
                  <span class="nw-kv-k">Prompt Length:</span>
                  <span class="nw-kv-v"
                    >{{ selected.request.prompt_len }} chars</span
                  >
                </div>
              </div>
            </div>
          </div>

          <!-- ── Payload tab ── -->
          <div v-if="activeDetailTab === 'payload'" class="nw-tab-content">
            <template v-if="selected.request?.body">
              <div class="nw-payload-hdr">
                <span>Request Payload</span>
                <span class="nw-payload-meta">
                  {{
                    formatBytes(
                      selected.request.body_size ||
                        selected.request.body?.length,
                    )
                  }}
                  <template v-if="selected.request.content_type">
                    · {{ selected.request.content_type }}
                  </template>
                </span>
                <button
                  class="nw-copy-btn"
                  @click="
                    copyText(
                      formatBody(selected.request.body),
                      'Request payload',
                    )
                  "
                >
                  <CopyOutlined /> Copy
                </button>
              </div>
              <pre
                :class="[
                  'nw-code',
                  isJson(selected.request.body)
                    ? 'nw-code-json'
                    : 'nw-code-text',
                ]"
              >{{ truncateBody(selected.request.body) }}</pre>
            </template>
            <div v-else class="nw-empty-tab">
              This request has no payload.
            </div>
          </div>

          <!-- ── Response tab ── -->
          <div
            v-if="activeDetailTab === 'response'"
            class="nw-tab-content"
          >
            <template v-if="selected.response?.body">
              <div class="nw-payload-hdr">
                <span>Response Body</span>
                <span class="nw-payload-meta">
                  {{
                    formatBytes(
                      selected.response.body_size ||
                        selected.response.body?.length,
                    )
                  }}
                  <template v-if="selected.response.content_type">
                    · {{ selected.response.content_type }}
                  </template>
                  <template v-if="selected.response.truncated">
                    · <span class="nw-truncated-badge">truncated</span>
                  </template>
                </span>
                <button
                  class="nw-copy-btn"
                  @click="
                    copyText(
                      formatBody(selected.response.body),
                      'Response body',
                    )
                  "
                >
                  <CopyOutlined /> Copy
                </button>
              </div>
              <pre
                :class="[
                  'nw-code',
                  isJson(selected.response.body)
                    ? 'nw-code-json'
                    : 'nw-code-text',
                ]"
              >{{ truncateBody(selected.response.body) }}</pre>
            </template>
            <template
              v-else-if="
                selected.response?.raw_hex_dump
              "
            >
              <div class="nw-payload-hdr">
                <span>Raw Response (hex)</span>
              </div>
              <pre class="nw-code nw-code-hex">{{
                selected.response.raw_hex_dump?.slice(0, 2000)
              }}</pre>
            </template>
            <div v-else class="nw-empty-tab">
              {{
                selected.isComplete
                  ? "Response has no body."
                  : "Waiting for response…"
              }}
            </div>
          </div>

          <!-- ── Timing tab ── -->
          <div
            v-if="activeDetailTab === 'timing'"
            class="nw-tab-content"
          >
            <div class="nw-timing">
              <div class="nw-timing-row">
                <span class="nw-timing-label">Request sent</span>
                <span class="nw-timing-val">{{
                  formatTimestamp(
                    selected.request?.timestamp ||
                      selected.response?.timestamp,
                  )
                }}</span>
              </div>
              <div v-if="selected.response" class="nw-timing-row">
                <span class="nw-timing-label">Response received</span>
                <span class="nw-timing-val">{{
                  formatTimestamp(selected.response.timestamp)
                }}</span>
              </div>
              <div
                v-if="selected.timeMs !== undefined"
                class="nw-timing-row nw-timing-total"
              >
                <span class="nw-timing-label">Total duration</span>
                <span class="nw-timing-val nw-timing-dur">{{
                  formatTime(selected.timeMs)
                }}</span>
              </div>
              <div
                v-if="selected.timeMs !== undefined"
                class="nw-timing-bar-wrap"
              >
                <div class="nw-timing-bar-track">
                  <div
                    class="nw-timing-bar-fill"
                    :style="{
                      width: '100%',
                      background:
                        selected.status && selected.status >= 400
                          ? '#d93025'
                          : '#1a73e8',
                    }"
                  ></div>
                </div>
                <span class="nw-timing-bar-label">{{
                  formatTime(selected.timeMs)
                }}</span>
              </div>
              <div class="nw-timing-meta">
                <div class="nw-kv" v-if="selected.request?.captured_len">
                  <span class="nw-kv-k">Request captured:</span>
                  <span class="nw-kv-v"
                    >{{ formatBytes(selected.request.captured_len) }} /
                    {{
                      formatBytes(selected.request.original_len)
                    }}</span
                  >
                </div>
                <div class="nw-kv" v-if="selected.response?.captured_len">
                  <span class="nw-kv-k">Response captured:</span>
                  <span class="nw-kv-v"
                    >{{ formatBytes(selected.response.captured_len) }} /
                    {{
                      formatBytes(selected.response.original_len)
                    }}</span
                  >
                </div>
                <div class="nw-kv" v-if="selected.request?.function">
                  <span class="nw-kv-k">Hook function:</span>
                  <span class="nw-kv-v nw-kv-mono">{{
                    selected.request.function
                  }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* ═══════════════════════════════════════════
   Chrome DevTools Network Panel — Styles
   ═══════════════════════════════════════════ */
:root {
  --nw-font-ui: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    "Helvetica Neue", Arial, sans-serif;
  --nw-font-mono: "SF Mono", Monaco, Menlo, Consolas, "Ubuntu Mono",
    "Liberation Mono", "Courier New", monospace;
}

.nw {
  display: flex;
  flex-direction: column;
  border: 1px solid #dadce0;
  border-radius: 8px;
  overflow: hidden;
  background: #fff;
  font-family: var(--nw-font-ui);
  font-size: 12px;
  color: #202124;
  min-height: 420px;
}

/* ── Toolbar ── */
.nw-toolbar {
  background: #f8f9fa;
  border-bottom: 1px solid #dadce0;
  flex-shrink: 0;
}

.nw-toolbar-primary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  min-height: 32px;
}

.nw-toolbar-secondary {
  display: flex;
  align-items: center;
  gap: 0;
  padding: 0 8px 4px;
  border-top: 1px solid #e8eaed;
}

.nw-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.nw-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: none;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  color: #5f6368;
  transition: background 0.15s;
}
.nw-icon-btn:hover {
  background: #e8eaed;
}

.nw-record-dot {
  display: block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #d93025;
  box-shadow: 0 0 0 2px rgba(217, 48, 37, 0.2);
  animation: nw-pulse 1.5s ease-in-out infinite;
}

@keyframes nw-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 2px rgba(217, 48, 37, 0.2);
  }
  50% {
    box-shadow: 0 0 0 5px rgba(217, 48, 37, 0.08);
  }
}

.nw-pause-icon {
  font-size: 10px;
  color: #5f6368;
}

.nw-sep-v {
  width: 1px;
  height: 16px;
  background: #dadce0;
  margin: 0 4px;
}

/* Filter */
.nw-filter-wrap {
  display: flex;
  align-items: center;
  flex: 1;
  max-width: 380px;
  background: #fff;
  border: 1px solid #dadce0;
  border-radius: 4px;
  padding: 0 6px;
  transition: border-color 0.15s;
}
.nw-filter-wrap:focus-within {
  border-color: #1a73e8;
  box-shadow: 0 0 0 1px rgba(26, 115, 232, 0.2);
}

.nw-filter-icon {
  flex-shrink: 0;
  opacity: 0.5;
}

.nw-filter-input {
  flex: 1;
  border: none;
  outline: none;
  padding: 4px 6px;
  font-size: 12px;
  font-family: var(--nw-font-ui);
  background: transparent;
  color: #202124;
}
.nw-filter-input::placeholder {
  color: #80868b;
}

.nw-filter-clear {
  border: none;
  background: transparent;
  color: #80868b;
  cursor: pointer;
  font-size: 11px;
  padding: 2px;
  line-height: 1;
}
.nw-filter-clear:hover {
  color: #202124;
}

/* Toolbar meta */
.nw-toolbar-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  flex-shrink: 0;
  white-space: nowrap;
}

.nw-meta-badge {
  font-size: 11px;
  color: #5f6368;
}
.nw-meta-num {
  font-weight: 600;
  color: #202124;
}
.nw-meta-sep {
  color: #dadce0;
  font-size: 10px;
}

.nw-meta-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 500;
}
.nw-status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.nw-live .nw-status-dot {
  background: #188038;
}
.nw-live {
  color: #188038;
}
.nw-offline .nw-status-dot {
  background: #d93025;
}
.nw-offline {
  color: #d93025;
}

/* Type chips */
.nw-chip {
  border: none;
  background: transparent;
  padding: 4px 10px;
  font-size: 11px;
  font-family: var(--nw-font-ui);
  color: #5f6368;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition:
    color 0.15s,
    border-color 0.15s;
  margin-bottom: -1px;
}
.nw-chip:hover {
  color: #202124;
}
.nw-chip.active {
  color: #1a73e8;
  border-bottom-color: #1a73e8;
  font-weight: 500;
}

/* ── Content ── */
.nw-content {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.nw-content.nw-split .nw-list {
  flex: 0 0 50%;
  max-width: 50%;
  border-right: 1px solid #dadce0;
}

.nw-list {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}

/* Table header */
.nw-hdr {
  display: flex;
  align-items: center;
  background: #f8f9fa;
  border-bottom: 1px solid #dadce0;
  min-height: 26px;
  padding: 0;
  flex-shrink: 0;
  font-weight: 500;
  color: #5f6368;
  font-size: 11px;
  user-select: none;
}

.nw-col {
  padding: 4px 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nw-col-name {
  flex: 2.5;
  min-width: 120px;
}
.nw-col-status {
  flex: 0 0 56px;
  text-align: center;
}
.nw-col-method {
  flex: 0 0 56px;
  text-align: center;
}
.nw-col-type {
  flex: 0 0 56px;
  text-align: center;
}
.nw-col-size {
  flex: 0 0 68px;
  text-align: right;
}
.nw-col-time {
  flex: 0 0 68px;
  text-align: right;
}
.nw-col-waterfall {
  flex: 1;
  min-width: 80px;
  padding: 4px 6px;
}

/* Body */
.nw-body {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

/* Rows */
.nw-row {
  display: flex;
  align-items: center;
  min-height: 24px;
  cursor: pointer;
  border-bottom: 1px solid #f1f3f4;
  transition: background 0.1s;
  font-family: var(--nw-font-mono);
  font-size: 11px;
}
.nw-row:hover {
  background: #f5f5f5;
}
.nw-row.nw-selected {
  background: #e8f0fe;
}
.nw-row.nw-selected:hover {
  background: #d2e3fc;
}
.nw-row.nw-pending {
  color: #80868b;
  font-style: italic;
}
.nw-row.nw-error .nw-col-name .nw-name-text {
  color: #d93025;
}

.nw-name-text {
  color: #202124;
  overflow: hidden;
  text-overflow: ellipsis;
}

.nw-name-host {
  display: none;
  color: #80868b;
  font-size: 10px;
  margin-left: 6px;
}
.nw-col-name {
  display: flex;
  align-items: baseline;
  gap: 4px;
}
.nw-row:hover .nw-name-host {
  display: inline;
}

/* Status */
.status-ok {
  color: #188038;
  font-weight: 600;
}
.status-redirect {
  color: #e37400;
  font-weight: 500;
}
.status-error {
  color: #d93025;
  font-weight: 600;
}
.status-pending {
  color: #80868b;
}

/* Method */
.nw-method-badge {
  font-weight: 600;
  font-size: 10px;
  font-family: var(--nw-font-mono);
}

/* Waterfall */
.nw-wf-track {
  position: relative;
  height: 8px;
  width: 100%;
}
.nw-wf-bar {
  position: absolute;
  top: 1px;
  height: 6px;
  border-radius: 3px;
  min-width: 3px;
  opacity: 0.7;
}

/* Empty */
.nw-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 8px;
}
.nw-empty-icon {
  font-size: 32px;
  opacity: 0.4;
}
.nw-empty-title {
  font-size: 13px;
  color: #5f6368;
  font-weight: 500;
}
.nw-empty-hint {
  font-size: 11px;
  color: #80868b;
}

/* ── Detail panel ── */
.nw-detail {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  background: #fff;
}

.nw-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #f8f9fa;
  border-bottom: 1px solid #dadce0;
  min-height: 30px;
  flex-shrink: 0;
}

.nw-detail-tabs {
  display: flex;
  align-items: center;
}

.nw-dtab {
  border: none;
  background: transparent;
  padding: 6px 12px;
  font-size: 11px;
  font-family: var(--nw-font-ui);
  color: #5f6368;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition:
    color 0.15s,
    border-color 0.15s;
  margin-bottom: -1px;
}
.nw-dtab:hover {
  color: #202124;
}
.nw-dtab.active {
  color: #1a73e8;
  border-bottom-color: #1a73e8;
  font-weight: 500;
}

.nw-detail-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-right: 8px;
}

.nw-copy-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: #5f6368;
  font-size: 11px;
  font-family: var(--nw-font-ui);
  cursor: pointer;
  padding: 3px 8px;
  border-radius: 4px;
  transition: background 0.15s;
}
.nw-copy-btn:hover {
  background: #e8eaed;
  color: #202124;
}

.nw-close-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  color: #5f6368;
  cursor: pointer;
  border-radius: 4px;
  font-size: 13px;
  transition: background 0.15s;
}
.nw-close-btn:hover {
  background: #e8eaed;
  color: #202124;
}

/* Detail body */
.nw-detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 0;
}

.nw-tab-content {
  padding: 8px 12px;
}

/* Header groups (collapsible sections) */
.nw-hgroup {
  margin-bottom: 4px;
}

.nw-hgroup-title {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 0;
  cursor: pointer;
  font-weight: 600;
  font-size: 11px;
  color: #202124;
  user-select: none;
  border-bottom: 1px solid #f1f3f4;
}
.nw-hgroup-title:hover {
  color: #1a73e8;
}

.nw-hgroup-count {
  font-weight: 400;
  color: #80868b;
  font-size: 10px;
}

.nw-caret {
  display: inline-block;
  font-size: 8px;
  color: #5f6368;
  transition: transform 0.15s;
}
.nw-caret.open {
  transform: rotate(90deg);
}

.nw-hgroup-body {
  padding: 4px 0 4px 16px;
}

/* Key-value pairs */
.nw-kv {
  display: flex;
  gap: 8px;
  padding: 2px 0;
  font-size: 11px;
  line-height: 1.5;
  word-break: break-all;
}

.nw-kv-k {
  font-weight: 600;
  color: #881280;
  font-family: var(--nw-font-mono);
  white-space: nowrap;
  flex-shrink: 0;
}

.nw-kv-v {
  color: #1a1aa6;
  font-family: var(--nw-font-mono);
  word-break: break-all;
}
.nw-kv-url {
  color: #202124;
}
.nw-kv-val-redacted {
  color: #e37400;
  font-style: italic;
}
.nw-kv-vendor {
  color: #9334e6;
  font-weight: 600;
}
.nw-kv-redacted {
  color: #188038;
}
.nw-kv-mono {
  font-family: var(--nw-font-mono);
}

/* Payload / Response */
.nw-payload-hdr {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 11px;
  color: #202124;
  padding: 6px 0;
  border-bottom: 1px solid #f1f3f4;
  margin-bottom: 8px;
}

.nw-payload-meta {
  font-weight: 400;
  color: #80868b;
  font-size: 10px;
  flex: 1;
}

.nw-truncated-badge {
  color: #e37400;
  font-weight: 500;
}

.nw-code {
  margin: 0;
  padding: 12px;
  border-radius: 6px;
  font-size: 11px;
  font-family: var(--nw-font-mono);
  line-height: 1.6;
  max-height: 480px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  tab-size: 2;
}

.nw-code-json {
  background: #1e1e2e;
  color: #cdd6f4;
  border: 1px solid #313244;
}

.nw-code-text {
  background: #f8f9fa;
  color: #202124;
  border: 1px solid #e8eaed;
}

.nw-code-hex {
  background: #f8f9fa;
  color: #5f6368;
  border: 1px solid #e8eaed;
  font-size: 10px;
  max-height: 200px;
}

.nw-empty-tab {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  color: #80868b;
  font-size: 12px;
}

/* Timing */
.nw-timing {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 0;
}

.nw-timing-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 11px;
}

.nw-timing-label {
  color: #5f6368;
}

.nw-timing-val {
  font-family: var(--nw-font-mono);
  color: #202124;
}

.nw-timing-total {
  border-top: 1px solid #e8eaed;
  margin-top: 4px;
  padding-top: 8px;
  font-weight: 600;
}

.nw-timing-dur {
  color: #1a73e8;
  font-weight: 700;
}

.nw-timing-bar-wrap {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 0;
}

.nw-timing-bar-track {
  flex: 1;
  height: 8px;
  background: #f1f3f4;
  border-radius: 4px;
  overflow: hidden;
}

.nw-timing-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.nw-timing-bar-label {
  font-size: 11px;
  font-family: var(--nw-font-mono);
  color: #5f6368;
  white-space: nowrap;
}

.nw-timing-meta {
  border-top: 1px solid #e8eaed;
  padding-top: 12px;
  margin-top: 8px;
}

/* ── Responsive ── */
@media (max-width: 960px) {
  .nw-content.nw-split {
    flex-direction: column;
  }
  .nw-content.nw-split .nw-list {
    flex: 0 0 auto;
    max-width: 100%;
    max-height: 45%;
    border-right: none;
    border-bottom: 1px solid #dadce0;
  }
  .nw-col-waterfall {
    display: none;
  }
  .nw-toolbar-meta {
    display: none;
  }
}

@media (max-width: 640px) {
  .nw-col-type,
  .nw-col-size {
    display: none;
  }
}
</style>
