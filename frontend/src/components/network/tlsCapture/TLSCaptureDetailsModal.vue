<script setup lang="ts">
import { CopyOutlined } from "@ant-design/icons-vue";
import { message } from "ant-design-vue";

import SanitizedFieldViewer from "../../common/SanitizedFieldViewer.vue";
import type { TLSPlaintextEvent } from "../../../types/tls";
import {
  buildTLSCurlCommand,
  formatTLSBytes,
  formatTLSTimestamp,
  isTLSRequestEvent,
  tlsDirectionColor,
  tlsDirectionLabel,
  tlsPacketTypeLabel,
} from "../../../views/network/tlsCapture/utils";

defineProps<{
  selectedEvent: TLSPlaintextEvent | null;
}>();

const open = defineModel<boolean>("open", { required: true });

const formatBytes = formatTLSBytes;
const formatTimestamp = formatTLSTimestamp;
const isRequestEvent = isTLSRequestEvent;
const directionColor = tlsDirectionColor;
const directionLabel = tlsDirectionLabel;
const packetTypeLabel = tlsPacketTypeLabel;
const buildCurl = buildTLSCurlCommand;

const copyText = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};
</script>

<template>
  <a-modal
      v-model:open="open"
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
          <a-descriptions-item label="Timestamp">
            <SanitizedFieldViewer
              :value="formatTimestamp(selectedEvent.timestamp)"
              :isSanitized="false"
              field-name="timestamp"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Packet"
            ><a-tag :color="directionColor(selectedEvent.direction)">{{
              directionLabel(selectedEvent.direction)
            }}</a-tag></a-descriptions-item
          >
          <a-descriptions-item label="Library">
            <SanitizedFieldViewer
              :value="selectedEvent.lib || '—'"
              :isSanitized="false"
              field-name="library"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Function">
            <SanitizedFieldViewer
              :value="selectedEvent.function || '—'"
              :isSanitized="false"
              field-name="function"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Command">
            <SanitizedFieldViewer
              :value="selectedEvent.comm || '—'"
              :isSanitized="selectedEvent.redaction_state === 'sanitized'"
              field-name="command"
            />
          </a-descriptions-item>
          <a-descriptions-item label="PID">
            <SanitizedFieldViewer
              :value="String(selectedEvent.pid ?? '—')"
              :isSanitized="false"
              field-name="pid"
            />
          </a-descriptions-item>
          <a-descriptions-item label="TGID">
            <SanitizedFieldViewer
              :value="String(selectedEvent.tgid ?? '—')"
              :isSanitized="false"
              field-name="tgid"
            />
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedEvent.uid" label="UID">
            <SanitizedFieldViewer
              :value="String(selectedEvent.uid)"
              :isSanitized="false"
              field-name="uid"
            />
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedEvent.tid" label="TID">
            <SanitizedFieldViewer
              :value="String(selectedEvent.tid)"
              :isSanitized="false"
              field-name="tid"
            />
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedEvent.data_type" label="Data Type">
            <a-tag :color="selectedEvent.data_type === 'http_request' || selectedEvent.data_type === 'http_response' ? 'blue' : selectedEvent.data_type === 'json' ? 'green' : selectedEvent.data_type === 'sse' ? 'purple' : 'default'">
              {{ selectedEvent.data_type }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedEvent.is_handshake" label="Handshake">
            <a-tag color="orange">TLS Handshake</a-tag>
          </a-descriptions-item>
          <a-descriptions-item v-if="selectedEvent.latency_ms" label="Latency">
            {{ selectedEvent.latency_ms.toFixed(1) }} ms
          </a-descriptions-item>
          <a-descriptions-item label="Method">
            <SanitizedFieldViewer
              :value="selectedEvent.method || '—'"
              :isSanitized="false"
              field-name="method"
            />
          </a-descriptions-item>
          <a-descriptions-item label="URL">
            <SanitizedFieldViewer
              :value="selectedEvent.url || '—'"
              :isSanitized="selectedEvent.redaction_state === 'sanitized'"
              field-name="url"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Host">
            <SanitizedFieldViewer
              :value="selectedEvent.host || '—'"
              :isSanitized="selectedEvent.redaction_state === 'sanitized'"
              field-name="host"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Status">
            <SanitizedFieldViewer
              :value="String(selectedEvent.status ?? '—')"
              :isSanitized="false"
              field-name="status"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Content Type">
            <SanitizedFieldViewer
              :value="selectedEvent.content_type || '—'"
              :isSanitized="false"
              field-name="content type"
            />
          </a-descriptions-item>
          <a-descriptions-item label="Body Size">
            <SanitizedFieldViewer
              :value="formatBytes(selectedEvent.body_size)"
              :isSanitized="false"
              field-name="body size"
            />
          </a-descriptions-item>
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
            <pre class="tls-pre">
              {{ JSON.stringify(selectedEvent.headers || {}, null, 2) }}
            </pre>
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
</template>
