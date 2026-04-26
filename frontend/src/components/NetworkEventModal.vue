<script setup lang="ts">
import { computed } from 'vue';

interface NetworkEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  eventType?: number;
  tag: string;
  comm: string;
  path: string;
  netDirection: string;
  netEndpoint: string;
  netFamily: string;
  netBytes: number;
  time: string;
}

const props = defineProps<{
  open: boolean;
  event: NetworkEvent | null;
  directionColor: (v: string) => string;
  formatDirection: (v: string) => string;
  typeColor: (et?: number, v?: string) => string;
  familyColor: (v: string) => string;
  formatBytes: (v: number) => string;
  formatDetailValue: (v: any) => string;
}>();

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void;
}>();

const modelOpen = computed({
  get: () => props.open,
  set: (v: boolean) => emit('update:open', v),
});
</script>

<template>
  <a-modal v-model:open="modelOpen" title="Network Event Details" :footer="null" width="700px">
    <a-descriptions bordered :column="1" size="small" v-if="event">
      <a-descriptions-item label="Time">{{ event.time }}</a-descriptions-item>
      <a-descriptions-item label="Direction">
        <a-tag :color="directionColor(event.netDirection)">{{ formatDirection(event.netDirection) }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item label="Event Type">
        <a-tag :color="typeColor(event.eventType, event.type)">{{ event.type.toUpperCase() }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item label="Tag">
        <a-tag color="purple">{{ event.tag }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item label="Command"><a-typography-text strong>{{ event.comm }}</a-typography-text></a-descriptions-item>
      <a-descriptions-item label="PID"><code>{{ formatDetailValue(event.pid) }}</code></a-descriptions-item>
      <a-descriptions-item label="Parent PID (PPID)"><code>{{ formatDetailValue(event.ppid) }}</code></a-descriptions-item>
      <a-descriptions-item label="User ID (UID)"><code>{{ formatDetailValue(event.uid) }}</code></a-descriptions-item>
      <a-descriptions-item label="Endpoint"><code style="word-break: break-all;">{{ formatDetailValue(event.netEndpoint) }}</code></a-descriptions-item>
      <a-descriptions-item label="Family">
        <a-tag :color="familyColor(event.netFamily)">{{ event.netFamily || 'unknown' }}</a-tag>
      </a-descriptions-item>
      <a-descriptions-item label="Bytes">{{ formatBytes(event.netBytes) }}</a-descriptions-item>
      <a-descriptions-item label="Summary"><code style="word-break: break-all;">{{ formatDetailValue(event.path) }}</code></a-descriptions-item>
    </a-descriptions>
  </a-modal>
</template>
