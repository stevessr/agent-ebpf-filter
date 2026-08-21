<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { CopyOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import type { MergedTransaction } from "../../types/tls";
import {
  buildCurl,
  formatBody,
  formatBytes,
  formatTime,
  formatTimestamp,
  getMethodColor,
  isJson,
  statusClass,
  truncateBody,
} from "./devToolsNetworkHelpers";

const props = defineProps<{
  transaction: MergedTransaction | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

type DetailTabKey = "headers" | "payload" | "response" | "timing";

/* ──── Detail-local state ──── */
const activeDetailTab = ref<DetailTabKey>("headers");

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

/* ──── Detail computed ──── */
const selectedQueryParams = computed(() => {
  if (!props.transaction) return [];
  return parseQueryParams(
    props.transaction.request?.url || props.transaction.response?.url,
  );
});

const hasPayload = computed(() => !!props.transaction?.request?.body);

const hasResponse = computed(() => !!props.transaction?.response?.body);

const detailTabs = computed<{ key: DetailTabKey; label: string }[]>(() => {
  const tabs: { key: DetailTabKey; label: string }[] = [
    { key: "headers", label: "Headers" },
  ];
  if (hasPayload.value) {
    tabs.push({ key: "payload", label: "Payload" });
  }
  tabs.push({ key: "response", label: "Response" });
  tabs.push({ key: "timing", label: "Timing" });
  return tabs;
});

/* Reset tab and default sections whenever a different transaction is picked */
watch(
  () => props.transaction?.id,
  () => {
    activeDetailTab.value = "headers";
    openSections.general = true;
    openSections.resHeaders = true;
    openSections.reqHeaders = true;
    openSections.queryParams = true;
  },
);
</script>

<template>
  <div v-if="transaction" class="nw-detail">
    <div class="nw-detail-header">
      <div class="nw-detail-tabs">
        <button
          v-for="tab in detailTabs"
          :key="tab.key"
          :class="['nw-dtab', { active: activeDetailTab === tab.key }]"
          @click="activeDetailTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
      <div class="nw-detail-actions">
        <button
          v-if="transaction.request"
          class="nw-copy-btn"
          @click="copyText(buildCurl(transaction), 'cURL')"
          title="Copy as cURL"
        >
          <CopyOutlined /> cURL
        </button>
        <button
          class="nw-close-btn"
          @click="emit('close')"
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
          <div class="nw-hgroup-title" @click="toggleSection('general')">
            <span class="nw-caret" :class="{ open: openSections.general }"
              >▶</span
            >
            General
          </div>
          <div v-if="openSections.general" class="nw-hgroup-body">
            <div class="nw-kv">
              <span class="nw-kv-k">Request URL:</span>
              <span class="nw-kv-v nw-kv-url">{{ transaction.fullUrl }}</span>
            </div>
            <div class="nw-kv">
              <span class="nw-kv-k">Request Method:</span>
              <span
                class="nw-kv-v"
                :style="{ color: getMethodColor(transaction.method) }"
                >{{ transaction.method }}</span
              >
            </div>
            <div v-if="transaction.status" class="nw-kv">
              <span class="nw-kv-k">Status Code:</span>
              <span class="nw-kv-v" :class="statusClass(transaction.status)">
                {{ transaction.status
                }}{{
                  transaction.status >= 200 && transaction.status < 300
                    ? " OK"
                    : transaction.status >= 400
                      ? " Error"
                      : ""
                }}
              </span>
            </div>
            <div class="nw-kv">
              <span class="nw-kv-k">Remote Address:</span>
              <span class="nw-kv-v">{{ transaction.host }}</span>
            </div>
            <div class="nw-kv">
              <span class="nw-kv-k">Process:</span>
              <span class="nw-kv-v"
                >{{ transaction.comm || "—" }} (PID:
                {{ transaction.pid ?? "—" }})</span
              >
            </div>
            <div class="nw-kv">
              <span class="nw-kv-k">TLS Library:</span>
              <span class="nw-kv-v">{{ transaction.lib || "—" }}</span>
            </div>
            <div v-if="transaction.vendor" class="nw-kv">
              <span class="nw-kv-k">LLM Vendor:</span>
              <span class="nw-kv-v nw-kv-vendor">{{ transaction.vendor }}</span>
            </div>
            <div v-if="transaction.redactionState" class="nw-kv">
              <span class="nw-kv-k">Redaction:</span>
              <span
                class="nw-kv-v"
                :class="
                  transaction.redactionState === 'sanitized'
                    ? 'nw-kv-redacted'
                    : ''
                "
                >{{ transaction.redactionState }}</span
              >
            </div>
          </div>
        </div>

        <!-- Response Headers -->
        <div
          v-if="
            transaction.response?.headers &&
            Object.keys(transaction.response.headers).length
          "
          class="nw-hgroup"
        >
          <div class="nw-hgroup-title" @click="toggleSection('resHeaders')">
            <span class="nw-caret" :class="{ open: openSections.resHeaders }"
              >▶</span
            >
            Response Headers
            <span class="nw-hgroup-count"
              >({{ Object.keys(transaction.response.headers).length }})</span
            >
          </div>
          <div v-if="openSections.resHeaders" class="nw-hgroup-body">
            <div
              v-for="(val, key) in transaction.response.headers"
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
            transaction.request?.headers &&
            Object.keys(transaction.request.headers).length
          "
          class="nw-hgroup"
        >
          <div class="nw-hgroup-title" @click="toggleSection('reqHeaders')">
            <span class="nw-caret" :class="{ open: openSections.reqHeaders }"
              >▶</span
            >
            Request Headers
            <span class="nw-hgroup-count"
              >({{ Object.keys(transaction.request.headers).length }})</span
            >
          </div>
          <div v-if="openSections.reqHeaders" class="nw-hgroup-body">
            <div
              v-for="(val, key) in transaction.request.headers"
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
        <div v-if="selectedQueryParams.length" class="nw-hgroup">
          <div class="nw-hgroup-title" @click="toggleSection('queryParams')">
            <span class="nw-caret" :class="{ open: openSections.queryParams }"
              >▶</span
            >
            Query String Parameters
            <span class="nw-hgroup-count"
              >({{ selectedQueryParams.length }})</span
            >
          </div>
          <div v-if="openSections.queryParams" class="nw-hgroup-body">
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
            transaction.response?.sse_event ||
            transaction.response?.sse_data_digest
          "
          class="nw-hgroup"
        >
          <div class="nw-hgroup-title">
            <span class="nw-caret open">▶</span>
            Server-Sent Events
          </div>
          <div class="nw-hgroup-body">
            <div v-if="transaction.response.sse_event" class="nw-kv">
              <span class="nw-kv-k">Event:</span>
              <span class="nw-kv-v">{{ transaction.response.sse_event }}</span>
            </div>
            <div v-if="transaction.response.sse_data_digest" class="nw-kv">
              <span class="nw-kv-k">Data Digest:</span>
              <span class="nw-kv-v nw-kv-mono">{{
                transaction.response.sse_data_digest
              }}</span>
            </div>
          </div>
        </div>

        <!-- LLM Metadata -->
        <div
          v-if="
            transaction.request?.vendor ||
            transaction.response?.vendor ||
            transaction.request?.prompt_digest ||
            transaction.request?.message_role
          "
          class="nw-hgroup"
        >
          <div class="nw-hgroup-title">
            <span class="nw-caret open">▶</span>
            LLM Metadata
          </div>
          <div class="nw-hgroup-body">
            <div
              v-if="transaction.request?.vendor || transaction.response?.vendor"
              class="nw-kv"
            >
              <span class="nw-kv-k">Vendor:</span>
              <span class="nw-kv-v nw-kv-vendor">{{
                transaction.request?.vendor || transaction.response?.vendor
              }}</span>
            </div>
            <div v-if="transaction.request?.message_role" class="nw-kv">
              <span class="nw-kv-k">Message Role:</span>
              <span class="nw-kv-v">{{
                transaction.request.message_role
              }}</span>
            </div>
            <div v-if="transaction.request?.prompt_digest" class="nw-kv">
              <span class="nw-kv-k">Prompt Digest:</span>
              <span class="nw-kv-v nw-kv-mono">{{
                transaction.request.prompt_digest
              }}</span>
            </div>
            <div v-if="transaction.request?.prompt_len" class="nw-kv">
              <span class="nw-kv-k">Prompt Length:</span>
              <span class="nw-kv-v"
                >{{ transaction.request.prompt_len }} chars</span
              >
            </div>
          </div>
        </div>
      </div>

      <!-- ── Payload tab ── -->
      <div v-if="activeDetailTab === 'payload'" class="nw-tab-content">
        <template v-if="transaction.request?.body">
          <div class="nw-payload-hdr">
            <span>Request Payload</span>
            <span class="nw-payload-meta">
              {{
                formatBytes(
                  transaction.request.body_size ||
                    transaction.request.body?.length,
                )
              }}
              <template v-if="transaction.request.content_type">
                · {{ transaction.request.content_type }}
              </template>
            </span>
            <button
              class="nw-copy-btn"
              @click="
                copyText(
                  formatBody(transaction.request.body),
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
              isJson(transaction.request.body)
                ? 'nw-code-json'
                : 'nw-code-text',
            ]"
            >{{ truncateBody(transaction.request.body) }}</pre>
        </template>
        <div v-else class="nw-empty-tab">This request has no payload.</div>
      </div>

      <!-- ── Response tab ── -->
      <div v-if="activeDetailTab === 'response'" class="nw-tab-content">
        <template v-if="transaction.response?.body">
          <div class="nw-payload-hdr">
            <span>Response Body</span>
            <span class="nw-payload-meta">
              {{
                formatBytes(
                  transaction.response.body_size ||
                    transaction.response.body?.length,
                )
              }}
              <template v-if="transaction.response.content_type">
                · {{ transaction.response.content_type }}
              </template>
              <template v-if="transaction.response.truncated">
                · <span class="nw-truncated-badge">truncated</span>
              </template>
            </span>
            <button
              class="nw-copy-btn"
              @click="
                copyText(formatBody(transaction.response.body), 'Response body')
              "
            >
              <CopyOutlined /> Copy
            </button>
          </div>
          <pre
            :class="[
              'nw-code',
              isJson(transaction.response.body)
                ? 'nw-code-json'
                : 'nw-code-text',
            ]"
            >{{ truncateBody(transaction.response.body) }}</pre>
        </template>
        <template v-else-if="transaction.response?.raw_hex_dump">
          <div class="nw-payload-hdr">
            <span>Raw Response (hex)</span>
          </div>
          <pre class="nw-code nw-code-hex">{{
            transaction.response.raw_hex_dump?.slice(0, 2000)
          }}</pre>
        </template>
        <div v-else class="nw-empty-tab">
          {{
            transaction.isComplete
              ? "Response has no body."
              : "Waiting for response…"
          }}
        </div>
      </div>

      <!-- ── Timing tab ── -->
      <div v-if="activeDetailTab === 'timing'" class="nw-tab-content">
        <div class="nw-timing">
          <div class="nw-timing-row">
            <span class="nw-timing-label">Request sent</span>
            <span class="nw-timing-val">{{
              formatTimestamp(
                transaction.request?.timestamp ||
                  transaction.response?.timestamp,
              )
            }}</span>
          </div>
          <div v-if="transaction.response" class="nw-timing-row">
            <span class="nw-timing-label">Response received</span>
            <span class="nw-timing-val">{{
              formatTimestamp(transaction.response.timestamp)
            }}</span>
          </div>
          <div
            v-if="transaction.timeMs !== undefined"
            class="nw-timing-row nw-timing-total"
          >
            <span class="nw-timing-label">Total duration</span>
            <span class="nw-timing-val nw-timing-dur">{{
              formatTime(transaction.timeMs)
            }}</span>
          </div>
          <div
            v-if="transaction.timeMs !== undefined"
            class="nw-timing-bar-wrap"
          >
            <div class="nw-timing-bar-track">
              <div
                class="nw-timing-bar-fill"
                :style="{
                  width: '100%',
                  background:
                    transaction.status && transaction.status >= 400
                      ? '#d93025'
                      : '#1a73e8',
                }"
              ></div>
            </div>
            <span class="nw-timing-bar-label">{{
              formatTime(transaction.timeMs)
            }}</span>
          </div>
          <div class="nw-timing-meta">
            <div class="nw-kv" v-if="transaction.request?.captured_len">
              <span class="nw-kv-k">Request captured:</span>
              <span class="nw-kv-v"
                >{{ formatBytes(transaction.request.captured_len) }} /
                {{ formatBytes(transaction.request.original_len) }}</span
              >
            </div>
            <div class="nw-kv" v-if="transaction.response?.captured_len">
              <span class="nw-kv-k">Response captured:</span>
              <span class="nw-kv-v"
                >{{ formatBytes(transaction.response.captured_len) }} /
                {{ formatBytes(transaction.response.original_len) }}</span
              >
            </div>
            <div class="nw-kv" v-if="transaction.request?.function">
              <span class="nw-kv-k">Hook function:</span>
              <span class="nw-kv-v nw-kv-mono">{{
                transaction.request.function
              }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
