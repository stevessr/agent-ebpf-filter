<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import axios from 'axios';
import { FilterOutlined, InfoCircleOutlined } from '@ant-design/icons-vue';
import { pb } from '../pb/tracker_pb.js';

interface AgentEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  tag: string;
  comm: string;
  path: string;
  time: string;
}

const events = ref<AgentEvent[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const showDetails = ref(false);
const selectedEvent = ref<AgentEvent | null>(null);
const selectedTag = ref<string | null>(null);
const tags = ref<string[]>([]);
let ws: WebSocket | null = null;

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
  } catch (err) {
    console.error('Failed to fetch tags', err);
  }
};

const filteredEvents = computed(() => {
  if (!selectedTag.value) return events.value;
  return events.value.filter(e => e.tag === selectedTag.value);
});

const openDetails = (record: AgentEvent) => {
  selectedEvent.value = record;
  showDetails.value = true;
};

const columns = [
  { title: 'Time', dataIndex: 'time', key: 'time', width: 120 },
  { title: 'Tag', dataIndex: 'tag', key: 'tag', width: 120 },
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100 },
  { title: 'Command', dataIndex: 'comm', key: 'comm', width: 150 },
  { title: 'Event Type', dataIndex: 'type', key: 'type', width: 150 },
  { title: 'Path', dataIndex: 'path', key: 'path', ellipsis: true },
  { title: 'Action', key: 'action', width: 80, fixed: 'right' as const }
];

const getTagColor = (type: string) => {
  const colors: Record<string, string> = {
    'execve': 'blue', 'openat': 'green', 'network_connect': 'orange',
    'network_bind': 'volcano', 'mkdir': 'cyan', 'unlink': 'red', 'ioctl': 'purple',
  };
  return colors[type] || 'default';
};

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta', 'Git': 'orange', 'Build Tool': 'cyan',
    'Package Manager': 'green', 'Runtime': 'blue', 'System Tool': 'geekblue', 'Network Tool': 'purple',
    'Security': 'red'
  };
  return colors[tag] || 'default';
};

const connectWebSocket = () => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  ws = new WebSocket(`${protocol}//${host}/ws`);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    isConnected.value = true;
  };

  ws.onmessage = (message) => {
    if (isPaused.value) return;
    try {
      const uint8Array = new Uint8Array(message.data);
      const data = pb.Event.decode(uint8Array);
      const now = new Date();
      events.value.unshift({
        key: `${data.pid}-${data.path}-${Date.now()}-${Math.random()}`,
        pid: data.pid,
        ppid: data.ppid,
        uid: data.uid,
        type: data.type,
        tag: data.tag,
        comm: data.comm,
        path: data.path,
        time: now.toLocaleTimeString(),
      });
      if (events.value.length > 1000) events.value.pop();
    } catch (e) {
      console.error('Failed to parse message', e);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
    setTimeout(connectWebSocket, 3000);
  };
};

const clearEvents = () => {
  events.value = [];
};

const exportEvents = () => {
  try {
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(events.value, null, 2));
    const downloadAnchorNode = document.createElement('a');
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute("download", `ebpf-events-${new Date().toISOString()}.json`);
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    downloadAnchorNode.remove();
    message.success('Events exported');
  } catch (err) {
    message.error('Failed to export events');
  }
};

onMounted(() => {
  connectWebSocket();
  fetchTags();
});

onUnmounted(() => {
  if (ws) ws.close();
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 280px">
    <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center;">
      <div style="display: flex; align-items: center; gap: 16px;">
        <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
        <span>Total Events: {{ events.length }}</span>
        <a-divider type="vertical" />
        <a-select v-model:value="selectedTag" placeholder="Filter by Tag" style="width: 160px" allowClear>
          <template #suffixIcon><FilterOutlined /></template>
          <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
        </a-select>
        
        <a-divider type="vertical" />
        <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" danger>
          {{ isPaused ? 'Resume Stream' : 'Pause Stream' }}
        </a-button>
        <a-button @click="exportEvents">Export Data</a-button>
      </div>
      <a-button type="primary" danger @click="clearEvents">Clear Events</a-button>
    </div>
    <a-table 
      :dataSource="filteredEvents" 
      :columns="columns" 
      size="small"
      :pagination="{ pageSize: 20 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'type'">
          <a-tag :color="getTagColor(record.type)">{{ record.type.toUpperCase() }}</a-tag>
        </template>
        <template v-if="column.key === 'tag'">
          <a-tag :color="getCategoryColor(record.tag)">{{ record.tag }}</a-tag>
        </template>
        <template v-if="column.key === 'path'">
          <a-typography-text code>{{ record.path }}</a-typography-text>
        </template>
        <template v-if="column.key === 'action'">
          <a-button type="link" size="small" @click="openDetails(record)">
            <template #icon><InfoCircleOutlined /></template>
          </a-button>
        </template>
      </template>
    </a-table>

    <a-modal v-model:open="showDetails" title="Event Details" :footer="null" width="600px">
      <a-descriptions bordered :column="1" size="small" v-if="selectedEvent">
        <a-descriptions-item label="Time">{{ selectedEvent.time }}</a-descriptions-item>
        <a-descriptions-item label="Event Type">
          <a-tag :color="getTagColor(selectedEvent.type)">{{ selectedEvent.type.toUpperCase() }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Tag">
          <a-tag :color="getCategoryColor(selectedEvent.tag)">{{ selectedEvent.tag }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Command"><a-typography-text strong>{{ selectedEvent.comm }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="PID"><a-typography-text code>{{ selectedEvent.pid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Parent PID (PPID)"><a-typography-text code>{{ selectedEvent.ppid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="User ID (UID)"><a-typography-text code>{{ selectedEvent.uid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Resource Path / Info"><a-typography-text code style="word-break: break-all;">{{ selectedEvent.path }}</a-typography-text></a-descriptions-item>
      </a-descriptions>
    </a-modal>
  </div>
</template>
