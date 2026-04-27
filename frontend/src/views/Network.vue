<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import axios from 'axios';
import {
  DeleteOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  PauseOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons-vue';

import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';
import NetworkStatsCards from '../components/NetworkStatsCards.vue';
import NetworkEventModal from '../components/NetworkEventModal.vue';

interface NetworkEvent {
  key: string; pid: number; ppid: number; uid: number; type: string;
  eventType?: number; tag: string; comm: string; path: string;
  netDirection: string; netEndpoint: string; netFamily: string;
  netBytes: number; time: string;
}

const eventTypeLabelMap: Record<number, string> = {
  [pb.EventType.NETWORK_CONNECT]: 'network_connect',
  [pb.EventType.NETWORK_BIND]: 'network_bind',
  [pb.EventType.NETWORK_SENDTO]: 'network_sendto',
  [pb.EventType.NETWORK_RECVFROM]: 'network_recvfrom',
  [pb.EventType.ACCEPT]: 'accept',
  [pb.EventType.ACCEPT4]: 'accept4',
};
const eventTypeColorMap: Record<number, string> = {
  [pb.EventType.NETWORK_CONNECT]: 'orange', [pb.EventType.NETWORK_BIND]: 'volcano',
  [pb.EventType.NETWORK_SENDTO]: 'cyan', [pb.EventType.NETWORK_RECVFROM]: 'geekblue',
  [pb.EventType.ACCEPT]: 'volcano', [pb.EventType.ACCEPT4]: 'volcano',
};

const events = ref<NetworkEvent[]>([]);
const tags = ref<string[]>([]);
const selectedTags = ref<string[]>([]);
const selectedTypes = ref<number[]>([]);
const searchQuery = ref('');
const isDeduplicated = ref(false);
const isPaused = ref(false);
const isConnected = ref(false);
const showDetails = ref(false);
const selectedEvent = ref<NetworkEvent | null>(null);

let ws: WebSocket | null = null;
let reconnectTimer: any = null;
let shouldReconnect = true;
const netEventBuffer: NetworkEvent[] = [];
let netRafId: number | null = null;

const flushNetEventBuffer = () => {
  netRafId = null;
  if (netEventBuffer.length === 0) return;

  const newEvents = [...netEventBuffer.reverse(), ...events.value];
  if (newEvents.length > 2000) {
    newEvents.length = 2000;
  }
  events.value = newEvents;
  netEventBuffer.length = 0;
};

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), units.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(1)} ${units[index]}`;
};

const extractEventType = (event: pb.IEvent) =>
  Object.prototype.hasOwnProperty.call(event, 'eventType') && event.eventType !== null && event.eventType !== undefined
    ? Number(event.eventType) : undefined;

const isNetworkEvent = (eventType: number | undefined, type?: string) => {
  if (eventType !== undefined && (eventTypeLabelMap[eventType] || eventType === 20)) return true;
  return type === 'accept' || type === 'accept4' || type === 'socket' || Boolean(type?.startsWith('network_'));
};

const networkFilteredEvents = computed(() => {
  let list = events.value;
  if (isDeduplicated.value) {
    const seen = new Set<string>();
    list = list.filter(e => {
      const id = `${e.pid}-${e.netEndpoint}-${e.type}`;
      if (seen.has(id)) return false;
      seen.add(id); return true;
    });
  }
  if (selectedTags.value.length > 0) list = list.filter(e => selectedTags.value.includes(e.tag));
  if (selectedTypes.value.length > 0) list = list.filter(e => e.eventType !== undefined && selectedTypes.value.includes(e.eventType));
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase();
    list = list.filter(e => e.comm.toLowerCase().includes(q) || e.netEndpoint.toLowerCase().includes(q) || String(e.pid).includes(q));
  }
  return list;
});

const summaryStats = computed(() => {
  const fe = networkFilteredEvents.value;
  return {
    outgoing: fe.filter(e => e.netDirection === 'outgoing').length,
    incoming: fe.filter(e => e.netDirection === 'incoming').length,
    listening: fe.filter(e => e.netDirection === 'listening').length,
    uniquePids: new Set(fe.map(e => e.pid)).size,
    uniqueEndpoints: new Set(fe.map(e => e.netEndpoint).filter(Boolean)).size,
    totalBytes: fe.reduce((sum, e) => sum + (e.netBytes || 0), 0),
  };
});

const directionColor = (v: string) => (v === 'outgoing' ? 'green' : v === 'incoming' ? 'blue' : v === 'listening' ? 'gold' : 'default');
const formatDirection = (v: string) => v ? v.charAt(0).toUpperCase() + v.slice(1) : 'Unknown';
const typeColor = (et?: number, _v?: string) => (et !== undefined ? eventTypeColorMap[et] : 'default');
const familyColor = (v: string) => (v === 'ipv4' ? 'blue' : v === 'ipv6' ? 'purple' : 'default');
const formatDetailValue = (v: any) => (v === undefined || v === null || v === '' ? '—' : String(v));

const fetchRecentEvents = async () => {
  try {
    const res = await axios.get('/events/recent?limit=400');
    events.value = (res.data.events || []).map((r: any) => {
      const d = r.Event;
      return {
        key: `${d.pid}-${d.type}-${r.Timestamp}-${Math.random()}`,
        pid: d.pid || 0, ppid: d.ppid || 0, uid: d.uid || 0,
        type: d.type || '', eventType: extractEventType(d),
        tag: d.tag || '', comm: d.comm || '', path: d.path || '',
        netDirection: d.netDirection || '', netEndpoint: d.netEndpoint || '',
        netFamily: d.netFamily || '', netBytes: Number(d.netBytes || 0),
        time: new Date(r.Timestamp).toLocaleTimeString(),
      };
    }).filter((e: any) => isNetworkEvent(e.eventType, e.type));
  } catch (err) {}
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();
  const socket = new WebSocket(buildWebSocketUrl('/ws'));
  ws = socket;
  socket.binaryType = 'arraybuffer';
  socket.onopen = () => isConnected.value = true;
  socket.onmessage = (me) => {
    if (isPaused.value) return;
    const incoming = (payload: Uint8Array): pb.IEvent[] => (payload[0] === 10 ? pb.EventBatch.decode(payload).events || [] : [pb.Event.decode(payload)]);
    incoming(new Uint8Array(me.data)).forEach(d => {
      const et = extractEventType(d);
      if (!isNetworkEvent(et, d.type || '')) return;
      netEventBuffer.push({
        key: `${d.pid}-${d.type}-${Date.now()}-${Math.random()}`,
        pid: d.pid || 0, ppid: d.ppid || 0, uid: d.uid || 0,
        type: d.type || '', eventType: et, tag: d.tag || '',
        comm: d.comm || '', path: d.path || '',
        netDirection: d.netDirection || '', netEndpoint: d.netEndpoint || '',
        netFamily: d.netFamily || '', netBytes: Number(d.netBytes || 0),
        time: new Date().toLocaleTimeString(),
      });
    });
    if (netRafId === null) {
      netRafId = requestAnimationFrame(flushNetEventBuffer);
    }
  };
  socket.onclose = () => {
    isConnected.value = false;
    if (shouldReconnect) reconnectTimer = setTimeout(connectWebSocket, 3000);
  };
};

onMounted(() => {
  fetchRecentEvents();
  connectWebSocket();
  axios.get('/config/tags').then(res => tags.value = res.data);
});
const clearNetworkEvents = () => {
  events.value = [];
  netEventBuffer.length = 0;
  if (netRafId !== null) {
    cancelAnimationFrame(netRafId);
    netRafId = null;
  }
};

onUnmounted(() => {
  shouldReconnect = false;
  if (reconnectTimer) clearTimeout(reconnectTimer);
  if (netRafId !== null) {
    cancelAnimationFrame(netRafId);
    netRafId = null;
  }
  netEventBuffer.length = 0;
  if (ws) ws.close();
});
</script>

<template>
  <div class="network-page">
    <a-card :bordered="false">
      <template #title><span><GlobalOutlined /> Network Monitor</span></template>
      <template #extra>
        <a-space>
          <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Live' : 'Offline'" />
          <a-tag color="purple">{{ events.length }} events</a-tag>
        </a-space>
      </template>

      <NetworkStatsCards :totalEvents="networkFilteredEvents.length" :stats="summaryStats" :formatBytes="formatBytes" />

      <div style="display: flex; justify-content: space-between; margin-bottom: 16px; flex-wrap: wrap; gap: 8px;">
        <a-space>
          <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" danger size="small">
            <template #icon><PauseOutlined v-if="isPaused" /><PlayCircleOutlined v-else /></template>
            {{ isPaused ? 'Resume' : 'Pause' }}
          </a-button>
          <a-button danger @click="clearNetworkEvents" size="small"><template #icon><DeleteOutlined /></template>Clear</a-button>
        </a-space>
        <a-space>
          <a-input-search v-model:value="searchQuery" placeholder="Search..." size="small" style="width: 180px" />
          <a-select v-model:value="selectedTags" mode="multiple" placeholder="Tags" style="min-width: 120px" size="small" :options="tags.map(t => ({label:t, value:t}))" max-tag-count="responsive" />
        </a-space>
      </div>

      <a-table :dataSource="networkFilteredEvents" row-key="key" size="small" :pagination="{ pageSize: 20, showSizeChanger: true }">
        <a-table-column title="Time" dataIndex="time" key="time" width="100" />
        <a-table-column title="Dir" dataIndex="netDirection" key="netDirection" width="100">
          <template #default="{ text }"><a-tag :color="directionColor(text)" size="small">{{ formatDirection(text) }}</a-tag></template>
        </a-table-column>
        <a-table-column title="Type" dataIndex="type" key="type" width="140">
          <template #default="{ text, record }"><a-tag :color="typeColor(record.eventType, text)" size="small">{{ text.toUpperCase() }}</a-tag></template>
        </a-table-column>
        <a-table-column title="Command" dataIndex="comm" key="comm" ellipsis />
        <a-table-column title="Endpoint" dataIndex="netEndpoint" key="netEndpoint" ellipsis />
        <a-table-column title="Bytes" dataIndex="netBytes" key="netBytes" width="100" align="right">
          <template #default="{ text }">{{ formatBytes(text) }}</template>
        </a-table-column>
        <a-table-column title="" key="action" width="50">
          <template #default="{ record }"><a-button type="link" size="small" @click="selectedEvent = record; showDetails = true"><InfoCircleOutlined /></a-button></template>
        </a-table-column>
      </a-table>
    </a-card>

    <NetworkEventModal 
      v-model:open="showDetails" :event="selectedEvent"
      :directionColor="directionColor" :formatDirection="formatDirection"
      :typeColor="typeColor" :familyColor="familyColor"
      :formatBytes="formatBytes" :formatDetailValue="formatDetailValue"
    />
  </div>
</template>

<style scoped>
.network-page { padding: 0; }
:deep(.ant-table-thead > tr > th) { background: #f9fafb; font-weight: 600; }
:deep(.ant-tag) { margin-right: 0; }
</style>
