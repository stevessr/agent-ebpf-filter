<script setup lang="ts">
import { computed } from 'vue';
import { CopyOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import {
  decodeStdioMessage,
  formatFullTime,
  formatStdioExpandedContent,
  isStdioSource,
  type ProcessedAgentSightEvent,
} from '../../utils/agentsight';

const props = defineProps<{
  open: boolean;
  event: ProcessedAgentSightEvent | null;
}>();

const emit = defineEmits<{
  close: [];
}>();

const decodedStdio = computed(() => (props.event && isStdioSource(props.event.source) ? decodeStdioMessage(props.event.data) : null));
const rawJson = computed(() => JSON.stringify(props.event?.raw ?? props.event?.data ?? {}, null, 2));
const dataJson = computed(() => JSON.stringify(props.event?.data ?? {}, null, 2));
const decodedStdioText = computed(() => (decodedStdio.value ? formatStdioExpandedContent(decodedStdio.value) : ''));

const copy = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};
</script>

<template>
  <a-modal :open="open" width="920px" :footer="null" title="AgentSight event details" @cancel="emit('close')">
    <div v-if="event" class="event-details">
      <a-descriptions size="small" bordered :column="2">
        <a-descriptions-item label="ID" :span="2">
          <a-typography-text code copyable>{{ event.id }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Source">
          <a-tag :color="event.sourceColorClass">{{ event.source }}</a-tag>
          <a-typography-text type="secondary">{{ event.rawSource }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Type">
          <a-tag color="geekblue">{{ event.eventType }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Process">{{ event.comm }}</a-descriptions-item>
        <a-descriptions-item label="PID">{{ event.pid || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Time">{{ formatFullTime(event.timestamp) }}</a-descriptions-item>
        <a-descriptions-item label="Timestamp">{{ event.timestamp }}</a-descriptions-item>
        <a-descriptions-item label="Trace" :span="2">
          <a-typography-text code>{{ event.traceId || '—' }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Summary" :span="2">{{ event.title }}</a-descriptions-item>
      </a-descriptions>

      <a-card v-if="decodedStdio" size="small" title="Decoded stdio / MCP payload" class="details-card">
        <template #extra>
          <a-button size="small" @click="copy(decodedStdioText, 'Decoded payload')">
            <template #icon><CopyOutlined /></template>
            Copy
          </a-button>
        </template>
        <pre>{{ decodedStdioText }}</pre>
      </a-card>

      <a-card size="small" title="Normalized data" class="details-card">
        <template #extra>
          <a-button size="small" @click="copy(dataJson, 'Normalized data')">
            <template #icon><CopyOutlined /></template>
            Copy
          </a-button>
        </template>
        <pre>{{ dataJson }}</pre>
      </a-card>

      <a-card size="small" title="Raw record" class="details-card">
        <template #extra>
          <a-button size="small" @click="copy(rawJson, 'Raw record')">
            <template #icon><CopyOutlined /></template>
            Copy
          </a-button>
        </template>
        <pre>{{ rawJson }}</pre>
      </a-card>
    </div>
  </a-modal>
</template>

<style scoped>
.event-details {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.details-card pre {
  max-height: 320px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border-radius: 8px;
  background: #0f172a;
  color: #dbeafe;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
}
</style>
