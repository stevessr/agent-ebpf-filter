<script setup lang="ts">
/**
 * DevToolsNetworkPanel — Chromium DevTools Network tab-style viewer for TLS captured events.
 *
 * Merges HTTP request + response events into unified transactions and displays them
 * in a split-panel layout with a Chrome-like request list on the left and a detail
 * panel with Headers / Payload / Response / Timing tabs on the right.
 */
import { computed, reactive, ref, watch } from "vue";
import { CopyOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import {
  activateOnKeyboard,
  buildFullUrl,
  createTransactionSearchIndex,
  extractPathname,
  formatBody,
  formatBytes,
  formatTime,
  formatTimestamp,
  getMethodColor,
  isJson,
  isRequestEvent,
  isResponseEvent,
  shortType,
  statusClass,
  truncateBody,
  typeFilters,
} from "./devToolsNetworkHelpers";
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

const openSections = reactive<Record<string, boolean>>({
  general: true,
  resHeaders: true,
  reqHeaders: true,
  queryParams: true,
});

const toggleSection = (key: string) => {
  openSections[key] = !openSections[key];
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
const transactionSearchText = computed(() =>
  createTransactionSearchIndex(mergedTransactions.value),
);

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
    list = list.filter((tx) =>
      transactionSearchText.value.get(tx)?.includes(q),
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
  if (!tx.status) return "#5f6368";
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
            aria-label="Filter network requests"
            placeholder="Filter"
            spellcheck="false"
          />
          <button
            v-if="filterText"
            class="nw-filter-clear"
            aria-label="Clear network request filter"
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
          :aria-pressed="activeTypeFilter === f.value"
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
            role="button" tabindex="0" :aria-pressed="selected?.id === tx.id"
            @click="selectTx(tx)"
            @keydown="activateOnKeyboard($event, () => selectTx(tx))"
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

<style scoped src="./DevToolsNetworkPanel.css"></style>
