<script setup lang="ts">
import { computed } from "vue";
import { CopyOutlined, EyeOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";
import type { ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";

const props = defineProps<{
  open: boolean;
  event: ObserverTLSEvent | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const hasBody = computed(() => !!props.event?.body);
const hasHeaders = computed(
  () => props.event?.headers && Object.keys(props.event.headers).length > 0,
);
const hasAgentCtx = computed(
  () =>
    !!(
      props.event?.vendor ||
      props.event?.agent_run_id ||
      props.event?.task_id ||
      props.event?.message_role ||
      props.event?.tool_call_id
    ),
);
const hasSSE = computed(() => !!(props.event?.sse_event || props.event?.sse_data_count));

const isJSON = computed(() => {
  const body = props.event?.body;
  if (!body) return false;
  const t = body.trim();
  return (t.startsWith("{") || t.startsWith("[")) && t.length > 2;
});

const formattedBody = computed(() => {
  const body = props.event?.body;
  if (!body) return "";
  if (isJSON.value) {
    try {
      const parsed = JSON.parse(body);
      return JSON.stringify(parsed, null, 2);
    } catch {
      return body;
    }
  }
  return body;
});

const displayBody = computed(() => {
  const body = formattedBody.value;
  if (body.length > 5000) return body.slice(0, 5000) + "\n\n… [truncated for display]";
  return body;
});

const fullTimestamp = computed(() => {
  const ts = props.event?.timestamp;
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
});

const copy = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};

const copyHeaders = async () => {
  if (!props.event?.headers) return;
  const text = Object.entries(props.event.headers)
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");
  await copy(text, "Headers");
};
</script>

<template>
  <a-modal
    :open="open"
    width="860px"
    :footer="null"
    title="Decrypted TLS Event"
    @cancel="emit('close')"
  >
    <div v-if="event" class="tls-modal-body">
      <!-- Metadata -->
      <a-descriptions size="small" bordered :column="3" class="tls-meta">
        <a-descriptions-item label="Type">
          <a-tag v-if="event.type === 'http_request'" color="blue">HTTP Request</a-tag>
          <a-tag v-else-if="event.type === 'http_response'" color="green">HTTP Response</a-tag>
          <a-tag v-else-if="event.type === 'sse_message'" color="purple">SSE Message</a-tag>
          <a-tag v-else-if="event.type === 'tls_plaintext'" color="default">Raw TLS</a-tag>
          <a-tag v-else color="default">{{ event.type || 'unknown' }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Direction">
          <a-tag :color="event.direction === 'send' ? 'orange' : 'cyan'">
            {{ event.direction === 'send' ? '⬆ Send' : '⬇ Recv' }}
          </a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Library">
          <a-tag color="geekblue">{{ event.lib || '—' }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Process">
          <span class="tls-meta-code">{{ event.comm || '—' }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="PID">
          <a-typography-text code>{{ event.pid }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="TGID">
          <a-typography-text code>{{ event.tgid }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Time">
          <span class="tls-meta-code">{{ fullTimestamp }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="Size">
          <span>{{ event.captured_len }}B (original: {{ event.original_len }}B)</span>
          <a-tag v-if="event.truncated" color="warning" size="small" style="margin-left:4px">truncated</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Function">
          <a-typography-text code>{{ event.function || '—' }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Redaction" v-if="event.redaction_state">
          <a-tag :color="event.redaction_state === 'sanitized' ? 'green' : 'orange'">
            {{ event.redaction_state }}
          </a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Method" v-if="event.method">
          <a-tag color="blue">{{ event.method }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Status" v-if="event.status">
          <a-tag :color="event.status < 400 ? 'green' : 'red'">{{ event.status }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Host" v-if="event.host" :span="2">
          <span class="tls-meta-code">{{ event.host }}</span>
        </a-descriptions-item>
        <a-descriptions-item label="URL" v-if="event.url" :span="3">
          <a-typography-text code>{{ event.url }}</a-typography-text>
        </a-descriptions-item>
      </a-descriptions>

      <!-- Agent Context -->
      <div v-if="hasAgentCtx" class="tls-section">
        <div class="tls-section-title">Agent Context</div>
        <a-descriptions size="small" bordered :column="2">
          <a-descriptions-item label="Vendor" v-if="event.vendor">
            <a-tag color="geekblue">{{ event.vendor }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Message Role" v-if="event.message_role">
            <a-tag color="purple">{{ event.message_role }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Run ID" v-if="event.agent_run_id" :span="2">
            <a-typography-text code>{{ event.agent_run_id }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Task ID" v-if="event.task_id" :span="2">
            <a-typography-text code>{{ event.task_id }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Tool Call" v-if="event.tool_call_id">
            <a-typography-text code>{{ event.tool_call_id }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Tool Name" v-if="event.tool_name">
            <a-tag>{{ event.tool_name }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Prompt Digest" v-if="event.prompt_digest" :span="2">
            <a-typography-text code>{{ event.prompt_digest }}</a-typography-text>
          </a-descriptions-item>
        </a-descriptions>
      </div>

      <!-- SSE -->
      <div v-if="hasSSE" class="tls-section">
        <div class="tls-section-title">Server-Sent Events</div>
        <a-descriptions size="small" bordered :column="2">
          <a-descriptions-item label="Event" v-if="event.sse_event">
            <code>{{ event.sse_event }}</code>
          </a-descriptions-item>
          <a-descriptions-item label="Data Parts" v-if="event.sse_data_count">
            {{ event.sse_data_count }}
          </a-descriptions-item>
          <a-descriptions-item label="Digest" v-if="event.sse_data_digest" :span="2">
            <a-typography-text code>{{ event.sse_data_digest }}</a-typography-text>
          </a-descriptions-item>
        </a-descriptions>
      </div>

      <!-- Headers -->
      <div v-if="hasHeaders" class="tls-section">
        <div class="tls-section-title">
          Headers
          <a-button size="small" type="link" @click="copyHeaders" style="padding:0;font-size:11px">
            <template #icon><CopyOutlined /></template>
            Copy
          </a-button>
        </div>
        <div class="tls-headers-box">
          <div
            v-for="(value, key) in event.headers"
            :key="key"
            class="tls-header-row"
          >
            <span class="tls-header-key">{{ key }}</span>
            <span
              :class="value === '***REDACTED***' ? 'tls-header-val-redacted' : 'tls-header-val'"
            >{{ value }}</span>
          </div>
        </div>
      </div>

      <!-- Body -->
      <div v-if="hasBody" class="tls-section">
        <div class="tls-section-title">
          Body
          <span class="tls-body-meta">
            ({{ event.body_size || event.body?.length || 0 }} bytes
            <template v-if="event.content_type">, {{ event.content_type }}</template>
            <template v-if="event.truncated">, truncated</template>)
          </span>
          <a-button
            size="small"
            type="link"
            @click="copy(formattedBody, 'Body')"
            style="padding:0;font-size:11px"
          >
            <template #icon><CopyOutlined /></template>
            Copy
          </a-button>
        </div>
        <pre :class="isJSON ? 'tls-body-json' : 'tls-body-text'">{{ displayBody }}</pre>
        <div
          v-if="formattedBody.length > 5000"
          class="tls-body-warning"
        >
          Body is {{ formattedBody.length.toLocaleString() }} bytes — showing first 5,000. Copy to see full content.
        </div>
      </div>

      <!-- Raw hex dump (for unparseable events) -->
      <div v-if="event.raw_hex_dump && !hasBody" class="tls-section">
        <div class="tls-section-title">Raw Data (hex)</div>
        <pre class="tls-raw-hex">{{ event.raw_hex_dump.slice(0, 500) }}…</pre>
      </div>
    </div>
    <a-empty
      v-else
      description="No event data"
      style="padding: 40px 0"
    />
  </a-modal>
</template>

<style scoped>
.tls-modal-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-height: 70vh;
  overflow-y: auto;
}

.tls-meta :deep(.ant-descriptions-item-label) {
  font-size: 11px;
  min-width: 72px;
}
.tls-meta :deep(.ant-descriptions-item-content) {
  font-size: 12px;
}

.tls-meta-code {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: #1e293b;
}

.tls-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tls-section-title {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid #e2e8f0;
}

.tls-body-meta {
  font-size: 10px;
  font-weight: 400;
  color: #64748b;
  text-transform: none;
}

.tls-headers-box {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 6px 10px;
  max-height: 200px;
  overflow-y: auto;
}

.tls-header-row {
  display: flex;
  gap: 10px;
  font-size: 12px;
  line-height: 1.7;
}

.tls-header-key {
  font-weight: 600;
  color: #475569;
  font-family: ui-monospace, monospace;
  min-width: 140px;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
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
  padding: 12px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.6;
  max-height: 400px;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
}

.tls-body-text {
  background: #f1f5f9;
  color: #1e293b;
  padding: 12px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 1.5;
  max-height: 300px;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
}

.tls-body-warning {
  font-size: 11px;
  color: #f59e0b;
  padding: 4px 8px;
  background: #fffbeb;
  border-radius: 4px;
}

.tls-raw-hex {
  background: #f8fafc;
  color: #64748b;
  padding: 10px;
  border-radius: 6px;
  font-size: 10px;
  line-height: 1.4;
  max-height: 120px;
  overflow: auto;
  margin: 0;
  font-family: ui-monospace, monospace;
  white-space: pre-wrap;
}
</style>
