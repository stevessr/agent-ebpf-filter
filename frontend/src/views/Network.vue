<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import axios from 'axios';
import {
  DeleteOutlined,
  DownloadOutlined,
  FilterOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  PauseOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import { pb } from '../pb/tracker_pb.js';

interface NetworkEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  tag: string;
  comm: string;
  path: string;
  netDirection: string;
  netEndpoint: string;
  netFamily: string;
  netBytes: number;
  time: string;
}

const eventTypes = ['network_connect', 'network_bind', 'network_sendto', 'network_recvfrom'];
const directionOptions = [
  { label: 'Outgoing', value: 'outgoing' },
  { label: 'Incoming', value: 'incoming' },
  { label: 'Listening', value: 'listening' },
];
const pageSizeOptions = ['20', '50', '100', '200'];
const columns = [
  { title: 'Time', dataIndex: 'time', key: 'time', width: 120 },
  { title: 'Direction', dataIndex: 'netDirection', key: 'netDirection', width: 110 },
  { title: 'Type', dataIndex: 'type', key: 'type', width: 150 },
  { title: 'Tag', dataIndex: 'tag', key: 'tag', width: 120 },
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100 },
  { title: 'Command', dataIndex: 'comm', key: 'comm', width: 150 },
  { title: 'Endpoint', dataIndex: 'netEndpoint', key: 'netEndpoint', width: 240, ellipsis: true },
  { title: 'Bytes', dataIndex: 'netBytes', key: 'netBytes', width: 120 },
  { title: 'Family', dataIndex: 'netFamily', key: 'netFamily', width: 100 },
  { title: 'Summary', dataIndex: 'path', key: 'path', ellipsis: true },
  { title: 'Action', key: 'action', width: 80, fixed: 'right' as const },
];

const events = ref<NetworkEvent[]>([]);
const tags = ref<string[]>([]);
const selectedTags = ref<string[]>([]);
const selectedTypes = ref<string[]>([]);
const selectedDirections = ref<string[]>([]);
const searchQuery = ref('');
const isDeduplicated = ref(false);
const isPaused = ref(false);
const isConnected = ref(false);
const currentPage = ref(1);
const pageSize = ref(20);
const showDetails = ref(false);
const selectedEvent = ref<NetworkEvent | null>(null);

let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;

const tagOptions = computed(() =>
  tags.value.map((tag) => ({
    label: tag,
    value: tag,
  })),
);

const eventTypeOptions = computed(() =>
  eventTypes.map((type) => ({
    label: type.toUpperCase(),
    value: type,
  })),
);

const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(value) / Math.log(base)), units.length - 1);
  const precision = index === 0 ? 0 : 2;
  return `${(value / Math.pow(base, index)).toFixed(precision)} ${units[index]}`;
};

const networkFilteredEvents = computed(() => {
  let result = events.value;

  if (selectedTags.value.length) {
    const activeTags = new Set(selectedTags.value);
    result = result.filter((event) => activeTags.has(event.tag));
  }

  if (selectedTypes.value.length) {
    const activeTypes = new Set(selectedTypes.value);
    result = result.filter((event) => activeTypes.has(event.type));
  }

  if (selectedDirections.value.length) {
    const activeDirections = new Set(selectedDirections.value);
    result = result.filter((event) => activeDirections.has(event.netDirection));
  }

  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase();
    result = result.filter((event) =>
      event.comm.toLowerCase().includes(q) ||
      event.path.toLowerCase().includes(q) ||
      event.netEndpoint.toLowerCase().includes(q) ||
      event.netDirection.toLowerCase().includes(q) ||
      event.type.toLowerCase().includes(q) ||
      String(event.pid).includes(q) ||
      event.netFamily.toLowerCase().includes(q),
    );
  }

  if (isDeduplicated.value) {
    const seen = new Set<string>();
    result = result.filter((event) => {
      const id = `${event.type}-${event.pid}-${event.netDirection}-${event.netEndpoint}-${event.netBytes}`;
      if (seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }

  return result;
});

const tablePagination = computed(() => ({
  current: currentPage.value,
  pageSize: pageSize.value,
  total: networkFilteredEvents.value.length,
  showSizeChanger: true,
  pageSizeOptions,
  showTotal: (total: number, range: [number, number]) => `${range[0]}-${range[1]} / ${total}`,
}));

const summaryStats = computed(() => {
  const outgoing = networkFilteredEvents.value.filter((event) => event.netDirection === 'outgoing').length;
  const incoming = networkFilteredEvents.value.filter((event) => event.netDirection === 'incoming').length;
  const listening = networkFilteredEvents.value.filter((event) => event.netDirection === 'listening').length;
  const uniquePids = new Set(networkFilteredEvents.value.map((event) => event.pid)).size;
  const uniqueEndpoints = new Set(
    networkFilteredEvents.value.map((event) => event.netEndpoint).filter((endpoint) => Boolean(endpoint)),
  ).size;
  const totalBytes = networkFilteredEvents.value.reduce((sum, event) => sum + (event.netBytes || 0), 0);

  return {
    outgoing,
    incoming,
    listening,
    uniquePids,
    uniqueEndpoints,
    totalBytes,
  };
});

const formatDirection = (value: string) => {
  switch (value) {
    case 'outgoing':
      return 'Outgoing';
    case 'incoming':
      return 'Incoming';
    case 'listening':
      return 'Listening';
    default:
      return 'Unknown';
  }
};

const directionColor = (value: string) => {
  switch (value) {
    case 'outgoing':
      return 'green';
    case 'incoming':
      return 'blue';
    case 'listening':
      return 'gold';
    default:
      return 'default';
  }
};

const typeColor = (value: string) => {
  switch (value) {
    case 'network_connect':
      return 'orange';
    case 'network_bind':
      return 'volcano';
    case 'network_sendto':
      return 'cyan';
    case 'network_recvfrom':
      return 'geekblue';
    default:
      return 'default';
  }
};

const familyColor = (value: string) => {
  switch (value) {
    case 'ipv4':
      return 'blue';
    case 'ipv6':
      return 'purple';
    default:
      return 'default';
  }
};

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
  } catch (err) {
    console.error('Failed to fetch tags', err);
  }
};

const handleTableChange = (pagination: { current?: number; pageSize?: number }) => {
  currentPage.value = pagination.current ?? 1;
  pageSize.value = pagination.pageSize ?? pageSize.value;
};

watch([selectedTags, selectedTypes, selectedDirections, searchQuery, isDeduplicated], () => {
  currentPage.value = 1;
});

watch([() => networkFilteredEvents.value.length, pageSize], ([total]) => {
  const maxPage = Math.max(1, Math.ceil(total / pageSize.value));
  if (currentPage.value > maxPage) {
    currentPage.value = maxPage;
  }
});

const openDetails = (record: NetworkEvent) => {
  selectedEvent.value = record;
  showDetails.value = true;
};

const clearEvents = () => {
  events.value = [];
  currentPage.value = 1;
};

const exportEvents = () => {
  try {
    const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(networkFilteredEvents.value, null, 2));
    const downloadAnchorNode = document.createElement('a');
    downloadAnchorNode.setAttribute('href', dataStr);
    downloadAnchorNode.setAttribute('download', `network-events-${new Date().toISOString()}.json`);
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    downloadAnchorNode.remove();
    message.success('Network events exported as JSON');
  } catch (err) {
    message.error('Failed to export network events');
  }
};

const exportEventsCSV = () => {
  try {
    const headers = ['Time', 'Direction', 'Type', 'Tag', 'PID', 'PPID', 'UID', 'Command', 'Endpoint', 'Family', 'Bytes', 'Summary'];
    const rows = networkFilteredEvents.value.map((event) => [
      event.time,
      event.netDirection,
      event.type,
      event.tag,
      event.pid,
      event.ppid,
      event.uid,
      event.comm,
      event.netEndpoint,
      event.netFamily,
      event.netBytes,
      event.path,
    ]);
    const csvContent = [headers, ...rows]
      .map((row) => row.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(','))
      .join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute('href', url);
    link.setAttribute('download', `network-events-${new Date().toISOString()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    message.success('Network events exported as CSV');
  } catch (err) {
    message.error('Failed to export CSV');
  }
};

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) {
    ws.close();
    ws = null;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  ws = new WebSocket(`${protocol}//${host}/ws`);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    isConnected.value = true;
  };

  ws.onmessage = (messageEvent) => {
    if (isPaused.value) return;
    try {
      const data = pb.Event.decode(new Uint8Array(messageEvent.data));
      if (!data.type?.startsWith('network_')) {
        return;
      }
      const now = new Date();
      events.value.unshift({
        key: `${data.pid}-${data.type}-${data.path}-${Date.now()}-${Math.random()}`,
        pid: data.pid,
        ppid: data.ppid,
        uid: data.uid,
        type: data.type,
        tag: data.tag,
        comm: data.comm,
        path: data.path,
        netDirection: data.netDirection || '',
        netEndpoint: data.netEndpoint || '',
        netFamily: data.netFamily || '',
        netBytes: Number(data.netBytes || 0),
        time: now.toLocaleTimeString(),
      });
      if (events.value.length > 1000) {
        events.value.pop();
      }
    } catch (err) {
      console.error('Failed to parse network message', err);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
    ws = null;
    if (!shouldReconnect) return;
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer);
    }
    reconnectTimer = window.setTimeout(() => {
      connectWebSocket();
    }, 3000);
  };
};

onMounted(() => {
  connectWebSocket();
  fetchTags();
});

onUnmounted(() => {
  shouldReconnect = false;
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  ws?.close();
  ws = null;
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 100%;">
    <a-card :bordered="false">
      <template #title>
        <span><GlobalOutlined /> Network Packet / Flow Monitor</span>
      </template>
      <template #extra>
        <a-space :size="8" wrap>
          <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
          <a-tag color="blue">eBPF</a-tag>
          <a-tag color="purple">{{ events.length }} buffered</a-tag>
        </a-space>
      </template>

      <a-alert
        type="info"
        show-icon
        style="margin-bottom: 16px;"
        message="Syscall-derived network events"
        description="This tab shows connect / bind / sendto / recvfrom flow records captured via eBPF tracepoints. It is best-effort flow telemetry, not raw packet payload capture."
      />

      <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Packets" :value="networkFilteredEvents.length" />
          </a-card>
        </a-col>
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Outgoing" :value="summaryStats.outgoing" />
          </a-card>
        </a-col>
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Incoming" :value="summaryStats.incoming" />
          </a-card>
        </a-col>
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Listening" :value="summaryStats.listening" />
          </a-card>
        </a-col>
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Endpoints" :value="summaryStats.uniqueEndpoints" />
          </a-card>
        </a-col>
        <a-col :xs="12" :lg="4">
          <a-card size="small" :bordered="false">
            <a-statistic title="Traffic" :value="formatBytes(summaryStats.totalBytes)" />
          </a-card>
        </a-col>
      </a-row>

      <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 16px;">
        <a-space :size="8" wrap>
          <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" danger>
            <template #icon>
              <PauseOutlined v-if="isPaused" />
              <PlayCircleOutlined v-else />
            </template>
            {{ isPaused ? 'Resume Stream' : 'Pause Stream' }}
          </a-button>
          <a-button type="primary" danger @click="clearEvents">
            <template #icon><DeleteOutlined /></template>
            Clear
          </a-button>
        </a-space>

        <a-dropdown>
          <template #overlay>
            <a-menu>
              <a-menu-item key="json" @click="exportEvents">
                <template #icon><DownloadOutlined /></template>
                JSON Format
              </a-menu-item>
              <a-menu-item key="csv" @click="exportEventsCSV">
                <template #icon><DownloadOutlined /></template>
                CSV Format
              </a-menu-item>
            </a-menu>
          </template>
          <a-button>
            <template #icon><DownloadOutlined /></template>
            Export Data
          </a-button>
        </a-dropdown>
      </div>

      <div style="background: #fafafa; padding: 12px; border-radius: 8px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap; border: 1px solid #f0f0f0; margin-bottom: 16px;">
        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
          <span style="font-size: 12px; color: #888;">Filter:</span>
          <a-select
            v-model:value="selectedTags"
            mode="multiple"
            placeholder="All Tags"
            style="width: 200px"
            size="small"
            allow-clear
            show-search
            max-tag-count="responsive"
            :options="tagOptions"
            option-filter-prop="label"
          >
            <template #suffixIcon><FilterOutlined /></template>
          </a-select>
          <a-select
            v-model:value="selectedTypes"
            mode="multiple"
            placeholder="All Types"
            style="width: 240px"
            size="small"
            allow-clear
            show-search
            max-tag-count="responsive"
            :options="eventTypeOptions"
            option-filter-prop="label"
          />
          <a-select
            v-model:value="selectedDirections"
            mode="multiple"
            placeholder="All Directions"
            style="width: 200px"
            size="small"
            allow-clear
            :options="directionOptions"
            option-filter-prop="label"
          />
        </div>

        <a-input-search
          v-model:value="searchQuery"
          placeholder="Search comm, endpoint, family or pid..."
          size="small"
          style="width: 260px"
          allow-clear
        />

        <a-divider type="vertical" />

        <a-checkbox v-model:checked="isDeduplicated" size="small">
          <span style="font-size: 12px;">Clean Duplicates</span>
        </a-checkbox>
      </div>

      <a-table
        :dataSource="networkFilteredEvents"
        :columns="columns"
        row-key="key"
        size="small"
        :pagination="tablePagination"
        @change="handleTableChange"
        :scroll="{ x: 1400 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'netDirection'">
            <a-tag :color="directionColor(record.netDirection)">{{ formatDirection(record.netDirection) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'type'">
            <a-tag :color="typeColor(record.type)">{{ record.type.toUpperCase() }}</a-tag>
          </template>
          <template v-else-if="column.key === 'tag'">
            <a-tag color="purple">{{ record.tag }}</a-tag>
          </template>
          <template v-else-if="column.key === 'netEndpoint'">
            <a-typography-text code style="word-break: break-all;">{{ record.netEndpoint || '—' }}</a-typography-text>
          </template>
          <template v-else-if="column.key === 'netBytes'">
            <span>{{ formatBytes(record.netBytes) }}</span>
          </template>
          <template v-else-if="column.key === 'netFamily'">
            <a-tag :color="familyColor(record.netFamily)">{{ record.netFamily || 'unknown' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'path'">
            <a-typography-text code style="word-break: break-all;">{{ record.path }}</a-typography-text>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" size="small" @click="openDetails(record)">
              <template #icon><InfoCircleOutlined /></template>
            </a-button>
          </template>
        </template>

        <template #emptyText>
          <a-empty description="No network events yet" />
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:open="showDetails" title="Network Event Details" :footer="null" width="700px">
      <a-descriptions bordered :column="1" size="small" v-if="selectedEvent">
        <a-descriptions-item label="Time">{{ selectedEvent.time }}</a-descriptions-item>
        <a-descriptions-item label="Direction">
          <a-tag :color="directionColor(selectedEvent.netDirection)">{{ formatDirection(selectedEvent.netDirection) }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Event Type">
          <a-tag :color="typeColor(selectedEvent.type)">{{ selectedEvent.type.toUpperCase() }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Tag">
          <a-tag color="purple">{{ selectedEvent.tag }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Command"><a-typography-text strong>{{ selectedEvent.comm }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="PID"><a-typography-text code>{{ selectedEvent.pid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Parent PID (PPID)"><a-typography-text code>{{ selectedEvent.ppid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="User ID (UID)"><a-typography-text code>{{ selectedEvent.uid }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Endpoint"><a-typography-text code style="word-break: break-all;">{{ selectedEvent.netEndpoint || '—' }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="Family">
          <a-tag :color="familyColor(selectedEvent.netFamily)">{{ selectedEvent.netFamily || 'unknown' }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Bytes">{{ formatBytes(selectedEvent.netBytes) }}</a-descriptions-item>
        <a-descriptions-item label="Summary"><a-typography-text code style="word-break: break-all;">{{ selectedEvent.path }}</a-typography-text></a-descriptions-item>
      </a-descriptions>
    </a-modal>
  </div>
</template>
