<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import axios from 'axios';
import { SafetyCertificateOutlined, PauseOutlined, PlayCircleOutlined, ReloadOutlined, SearchOutlined, CopyOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import { buildWebSocketUrl } from '../utils/requestContext';

interface TLSPlaintextEvent {
  type?: string;
  timestamp?: string;
  pid?: number;
  tgid?: number;
  comm?: string;
  direction?: string;
  lib?: string;
  method?: string;
  url?: string;
  host?: string;
  status?: number;
  headers?: Record<string, string>;
  body?: string;
  body_size?: number;
  content_type?: string;
  raw_hex_dump?: string;
  raw_available?: boolean;
  truncated?: boolean;
}

interface TLSLibraryStatus {
  library?: number;
  name: string;
  path?: string;
  attached: boolean;
  available?: boolean;
  error?: string;
}

const events = ref<TLSPlaintextEvent[]>([]);
const libraries = ref<TLSLibraryStatus[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const searchQuery = ref('');
const commFilter = ref('');
const hostFilter = ref('');
const selectedLib = ref<string>('all');
const selectedDirection = ref<string>('all');
const showDetails = ref(false);
const selectedEvent = ref<TLSPlaintextEvent | null>(null);

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let shouldReconnect = true;

const formatBytes = (bytes?: number) => {
  const value = Number(bytes || 0);
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
};

const formatTimestamp = (timestamp?: string) => {
  if (!timestamp) return '—';
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString();
};

const eventKey = (event: TLSPlaintextEvent, index: number) => `${event.timestamp ?? 'ts'}-${event.pid ?? 0}-${index}`;
const directionLabel = (direction?: string) => (direction === 'send' ? 'Send' : direction === 'recv' ? 'Recv' : '—');
const directionColor = (direction?: string) => (direction === 'send' ? 'green' : direction === 'recv' ? 'blue' : 'default');

const filteredEvents = computed(() => {
  let list = [...events.value];

  if (selectedLib.value !== 'all') {
    list = list.filter(event => (event.lib || '').toLowerCase() === selectedLib.value.toLowerCase());
  }
  if (selectedDirection.value !== 'all') {
    list = list.filter(event => (event.direction || '').toLowerCase() === selectedDirection.value.toLowerCase());
  }
  if (commFilter.value.trim()) {
    const q = commFilter.value.trim().toLowerCase();
    list = list.filter(event => (event.comm || '').toLowerCase().includes(q));
  }
  if (hostFilter.value.trim()) {
    const q = hostFilter.value.trim().toLowerCase();
    list = list.filter(event => (event.host || '').toLowerCase().includes(q));
  }
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.trim().toLowerCase();
    list = list.filter(event =>
      [event.method, event.url, event.host, event.body, JSON.stringify(event.headers || {})]
        .some(value => (value || '').toLowerCase().includes(q))
    );
  }

  return list;
});

const summaryStats = computed(() => {
  const list = filteredEvents.value;
  return {
    total: list.length,
    sends: list.filter(event => event.direction === 'send').length,
    recvs: list.filter(event => event.direction === 'recv').length,
    withBody: list.filter(event => Number(event.body_size || 0) > 0).length,
    attachedLibs: libraries.value.filter(item => item.attached).length,
  };
});

const fetchRecentEvents = async () => {
  try {
    const response = await axios.get('/tls-capture/recent?limit=500');
    events.value = Array.isArray(response.data?.events) ? response.data.events : [];
  } catch (error: any) {
    message.error(error?.response?.data?.error || 'Failed to load TLS capture events');
  }
};

const fetchLibraries = async () => {
  try {
    const response = await axios.get('/tls-capture/libraries');
    libraries.value = Array.isArray(response.data?.libraries) ? response.data.libraries : [];
  } catch (error: any) {
    message.error(error?.response?.data?.error || 'Failed to load TLS capture libraries');
  }
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();

  const socket = new WebSocket(buildWebSocketUrl('/ws/tls-capture'));
  ws = socket;

  socket.onopen = () => {
    isConnected.value = true;
  };

  socket.onmessage = (event) => {
    if (isPaused.value) return;
    try {
      const payload = JSON.parse(String(event.data)) as TLSPlaintextEvent;
      events.value = [payload, ...events.value].slice(0, 500);
    } catch (error) {
      console.error('TLS capture websocket parse error', error);
    }
  };

  socket.onclose = () => {
    isConnected.value = false;
    if (shouldReconnect) {
      reconnectTimer = setTimeout(connectWebSocket, 3000);
    }
  };

  socket.onerror = () => {
    isConnected.value = false;
  };
};

const refreshData = async () => {
  await Promise.all([fetchRecentEvents(), fetchLibraries()]);
};

const openDetails = (event: TLSPlaintextEvent) => {
  selectedEvent.value = event;
  showDetails.value = true;
};

const clearFilters = () => {
  searchQuery.value = '';
  commFilter.value = '';
  hostFilter.value = '';
  selectedLib.value = 'all';
  selectedDirection.value = 'all';
};

const copyText = async (text: string, label: string) => {
  await navigator.clipboard.writeText(text);
  message.success(`${label} copied`);
};

const buildCurl = (event: TLSPlaintextEvent): string => {
  const target = event.host && (event.url || '').startsWith('/') ? `https://${event.host}${event.url}` : (event.url || 'https://example.invalid');
  const parts = ['curl', '-X', event.method || 'GET'];
  Object.entries(event.headers || {}).forEach(([key, value]) => {
    if (value !== '***REDACTED***') {
      parts.push('-H', `${key}: ${value}`);
    }
  });
  if (event.body) parts.push('--data', event.body);
  parts.push(target);
  return parts.map(part => `'${part.replaceAll("'", "'\\''")}'`).join(' ');
};

onMounted(() => {
  void refreshData();
  connectWebSocket();
});

onUnmounted(() => {
  shouldReconnect = false;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.close();
    ws = null;
  }
});
</script>

<template>
  <div class="tls-capture-page">
    <a-card :bordered="false" class="tls-card">
      <template #title>
        <span class="tls-title"><SafetyCertificateOutlined /> TLS Capture</span>
      </template>
      <template #extra>
        <a-space>
          <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Live' : 'Offline'" />
          <a-tag color="purple">{{ summaryStats.total }} events</a-tag>
          <a-button size="small" @click="refreshData">
            <template #icon><ReloadOutlined /></template>
            Refresh
          </a-button>
        </a-space>
      </template>

      <a-row :gutter="16" class="tls-stats">
        <a-col :xs="12" :sm="6">
          <a-statistic title="Total" :value="summaryStats.total" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Send" :value="summaryStats.sends" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Recv" :value="summaryStats.recvs" />
        </a-col>
        <a-col :xs="12" :sm="6">
          <a-statistic title="Attached Libraries" :value="summaryStats.attachedLibs" />
        </a-col>
      </a-row>

      <a-space wrap class="tls-toolbar">
        <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" danger size="small">
          <template #icon><PauseOutlined v-if="isPaused" /><PlayCircleOutlined v-else /></template>
          {{ isPaused ? 'Resume' : 'Pause' }}
        </a-button>
        <a-input v-model:value="searchQuery" size="small" placeholder="Search URL, headers, body" allow-clear style="width: 220px;">
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <a-input v-model:value="commFilter" size="small" placeholder="Command filter" allow-clear style="width: 180px;" />
        <a-input v-model:value="hostFilter" size="small" placeholder="Host filter" allow-clear style="width: 180px;" />
        <a-select v-model:value="selectedLib" size="small" style="width: 160px;" :options="[{ label: 'All libraries', value: 'all' }, ...libraries.map(item => ({ label: item.name, value: item.name }))]" />
        <a-select v-model:value="selectedDirection" size="small" style="width: 120px;" :options="[
          { label: 'All directions', value: 'all' },
          { label: 'Send', value: 'send' },
          { label: 'Recv', value: 'recv' },
        ]" />
        <a-button size="small" @click="clearFilters">Clear Filters</a-button>
      </a-space>

      <a-empty v-if="events.length === 0" description="暂无 TLS 明文事件 — 请确保后端已启动且 eBPF TLS 探针已挂载" />
      <a-empty v-else-if="filteredEvents.length === 0" description="无匹配事件，请调整过滤条件" />

      <a-table
        :data-source="filteredEvents"
        :row-key="eventKey"
        size="small"
        :pagination="{ pageSize: 20, showSizeChanger: true }"
        :scroll="{ x: 1200 }"
      >
        <a-table-column title="Time" data-index="timestamp" key="timestamp" width="180">
          <template #default="{ text }">{{ formatTimestamp(text) }}</template>
        </a-table-column>
        <a-table-column title="Direction" data-index="direction" key="direction" width="100">
          <template #default="{ text }">
            <a-tag :color="directionColor(text)">{{ directionLabel(text) }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="Library" data-index="lib" key="lib" width="120" />
        <a-table-column title="Command" data-index="comm" key="comm" width="140" ellipsis />
        <a-table-column title="Host" data-index="host" key="host" width="180" ellipsis />
        <a-table-column title="Method" data-index="method" key="method" width="90" />
        <a-table-column title="URL" data-index="url" key="url" ellipsis />
        <a-table-column title="Body Size" data-index="body_size" key="body_size" width="110" align="right">
          <template #default="{ text }">{{ formatBytes(text) }}</template>
        </a-table-column>
        <a-table-column title="" key="action" width="160" fixed="right">
          <template #default="{ record }">
            <a-space :size="4">
              <a-button type="link" size="small" @click="openDetails(record)">Detail</a-button>
              <a-button type="link" size="small" @click="copyText(record.body || record.raw_hex_dump || '', 'Body')">
                <template #icon><CopyOutlined /></template>
              </a-button>
            </a-space>
          </template>
        </a-table-column>
      </a-table>

      <a-divider />

      <div class="tls-libraries">
        <div class="tls-libraries-title">Library Status</div>
        <a-list :data-source="libraries" size="small" bordered>
          <template #renderItem="{ item }">
            <a-list-item>
              <a-list-item-meta :description="item.path || '—'">
                <template #title>
                  <a-space>
                    <span>{{ item.name }}</span>
                    <a-tag :color="item.attached ? 'green' : 'default'">{{ item.attached ? 'Attached' : 'Not attached' }}</a-tag>
                    <a-tag v-if="item.available === false" color="red">Unavailable</a-tag>
                  </a-space>
                </template>
              </a-list-item-meta>
              <template #actions>
                <span v-if="item.error" class="tls-error">{{ item.error }}</span>
              </template>
            </a-list-item>
          </template>
        </a-list>
      </div>
    </a-card>

    <a-modal v-model:open="showDetails" title="TLS Plaintext Event" :footer="null" width="760px">
      <template v-if="selectedEvent">
        <a-space style="margin-bottom: 12px;">
          <a-button size="small" @click="copyText(selectedEvent.body || selectedEvent.raw_hex_dump || '', 'Body')">
            <template #icon><CopyOutlined /></template>Copy Body
          </a-button>
          <a-button v-if="selectedEvent.direction === 'send'" size="small" @click="copyText(buildCurl(selectedEvent), 'cURL')">
            <template #icon><CopyOutlined /></template>Copy cURL
          </a-button>
        </a-space>
        <a-descriptions bordered :column="1" size="small">
        <a-descriptions-item label="Timestamp">{{ formatTimestamp(selectedEvent.timestamp) }}</a-descriptions-item>
        <a-descriptions-item label="Direction"><a-tag :color="directionColor(selectedEvent.direction)">{{ directionLabel(selectedEvent.direction) }}</a-tag></a-descriptions-item>
        <a-descriptions-item label="Library">{{ selectedEvent.lib || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Command">{{ selectedEvent.comm || '—' }}</a-descriptions-item>
        <a-descriptions-item label="PID">{{ selectedEvent.pid ?? '—' }}</a-descriptions-item>
        <a-descriptions-item label="TGID">{{ selectedEvent.tgid ?? '—' }}</a-descriptions-item>
        <a-descriptions-item label="Method">{{ selectedEvent.method || '—' }}</a-descriptions-item>
        <a-descriptions-item label="URL"><a-typography-text code style="word-break: break-all;">{{ selectedEvent.url || '—' }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Host">{{ selectedEvent.host || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Status">{{ selectedEvent.status ?? '—' }}</a-descriptions-item>
        <a-descriptions-item label="Content Type">{{ selectedEvent.content_type || '—' }}</a-descriptions-item>
        <a-descriptions-item label="Body Size">{{ formatBytes(selectedEvent.body_size) }}</a-descriptions-item>
        <a-descriptions-item label="Headers">
          <pre class="tls-pre">{{ JSON.stringify(selectedEvent.headers || {}, null, 2) }}</pre>
        </a-descriptions-item>
        <a-descriptions-item label="Body">
          <pre class="tls-pre tls-body">{{ selectedEvent.body || '—' }}</pre>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.raw_hex_dump" label="Raw Hex Dump">
          <pre class="tls-pre">{{ selectedEvent.raw_hex_dump }}</pre>
        </a-descriptions-item>
        </a-descriptions>
      </template>
    </a-modal>
  </div>
</template>

<style scoped>
.tls-capture-page {
  padding: 0;
}

.tls-card {
  min-height: 320px;
}

.tls-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.tls-stats {
  margin-bottom: 16px;
}

.tls-toolbar {
  margin-bottom: 16px;
}

.tls-libraries-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.tls-error {
  color: #cf1322;
}

.tls-pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  padding: 12px;
  max-height: 240px;
  overflow: auto;
}

.tls-body {
  max-height: 320px;
}
</style>
