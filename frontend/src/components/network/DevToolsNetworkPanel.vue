<script setup lang="ts">
/**
 * DevToolsNetworkPanel — Chromium DevTools Network tab-style viewer for TLS captured events.
 *
 * Merges HTTP request + response events into unified transactions and displays them
 * in a split-panel layout with a Chrome-like request list on the left and a detail
 * panel with Headers / Payload / Response / Timing tabs on the right.
 */
import { computed, reactive, ref, watch } from "vue";
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
  typeFilters,
} from "./devToolsNetworkHelpers";
import type { TLSPlaintextEvent, MergedTransaction } from "../../types/tls";
import NetworkDetailPanel from "./NetworkDetailPanel.vue";

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
const filterText = ref("");
const activeTypeFilter = ref("all");

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
  selected.value = selected.value?.id === tx.id ? null : tx;
};

const hasPayload = computed(() => !!selected.value?.request?.body);
const hasResponse = computed(() => !!selected.value?.response?.body);

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
          <button class="nw-icon-btn" title="Clear log" @click="emit('clear')">
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
            <span class="nw-meta-num">{{ displayTransactions.length }}</span>
            requests
          </span>
          <span class="nw-meta-sep">│</span>
          <span class="nw-meta-badge">{{ totalSize }} transferred</span>
          <span class="nw-meta-sep">│</span>
          <span
            :class="['nw-meta-status', isConnected ? 'nw-live' : 'nw-offline']"
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
            role="button"
            tabindex="0"
            :aria-pressed="selected?.id === tx.id"
            @click="selectTx(tx)"
            @keydown="activateOnKeyboard($event, () => selectTx(tx))"
          >
            <div class="nw-col nw-col-name" :title="tx.fullUrl">
              <span class="nw-name-text">{{ tx.name || "/" }}</span>
              <span class="nw-name-host">{{ tx.host }}</span>
            </div>
            <div class="nw-col nw-col-status" :class="statusClass(tx.status)">
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
      <NetworkDetailPanel
        v-if="selected"
        :transaction="selected"
        @close="selected = null"
      />
    </div>
  </div>
</template>

<style scoped src="./DevToolsNetworkPanel.css"></style>
