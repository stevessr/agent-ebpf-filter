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
import { buildWebSocketUrl } from '../utils/requestContext';

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

type ResizableColumnKey =
  | 'time'
  | 'netDirection'
  | 'type'
  | 'tag'
  | 'pid'
  | 'comm'
  | 'netEndpoint'
  | 'netBytes'
  | 'netFamily'
  | 'path'
  | 'action';

const eventTypeLabelMap: Record<number, string> = {
  [pb.EventType.NETWORK_CONNECT]: 'network_connect',
  [pb.EventType.NETWORK_BIND]: 'network_bind',
  [pb.EventType.NETWORK_SENDTO]: 'network_sendto',
  [pb.EventType.NETWORK_RECVFROM]: 'network_recvfrom',
  [pb.EventType.ACCEPT]: 'accept',
};
const eventTypes = Object.entries(eventTypeLabelMap).map(([value, label]) => ({ value: Number(value), label }));
const eventTypeColorMap: Record<number, string> = {
  [pb.EventType.NETWORK_CONNECT]: 'orange',
  [pb.EventType.NETWORK_BIND]: 'volcano',
  [pb.EventType.NETWORK_SENDTO]: 'cyan',
  [pb.EventType.NETWORK_RECVFROM]: 'geekblue',
  [pb.EventType.ACCEPT]: 'volcano',
};
const networkEventTypes = new Set<number>(eventTypes.map((item) => item.value));
const decodeIncomingEvents = (payload: Uint8Array): pb.IEvent[] => {
  if (payload[0] === 10) {
    return pb.EventBatch.decode(payload).events || [];
  }
  return [pb.Event.decode(payload)];
};
const extractEventType = (event: pb.IEvent) =>
  Object.prototype.hasOwnProperty.call(event, 'eventType') && event.eventType !== null && event.eventType !== undefined
    ? Number(event.eventType)
    : undefined;
const isNetworkEvent = (eventType: number | undefined, type?: string) => {
  if (eventType !== undefined && networkEventTypes.has(eventType)) {
    return true;
  }
  return type === 'accept' || Boolean(type?.startsWith('network_'));
};
const directionOptions = [
  { label: 'Outgoing', value: 'outgoing' },
  { label: 'Incoming', value: 'incoming' },
  { label: 'Listening', value: 'listening' },
];
const pageSizeOptions = ['20', '50', '100', '200'];
const baseColumns = [
  { title: 'Time', dataIndex: 'time', key: 'time' },
  { title: 'Direction', dataIndex: 'netDirection', key: 'netDirection' },
  { title: 'Type', dataIndex: 'type', key: 'type' },
  { title: 'Tag', dataIndex: 'tag', key: 'tag' },
  { title: 'PID', dataIndex: 'pid', key: 'pid' },
  { title: 'Command', dataIndex: 'comm', key: 'comm' },
  { title: 'Endpoint', dataIndex: 'netEndpoint', key: 'netEndpoint' },
  { title: 'Bytes', dataIndex: 'netBytes', key: 'netBytes' },
  { title: 'Family', dataIndex: 'netFamily', key: 'netFamily' },
  { title: 'Summary', dataIndex: 'path', key: 'path' },
  { title: 'Action', key: 'action', fixed: 'right' as const },
] as const;

const events = ref<NetworkEvent[]>([]);
const tags = ref<string[]>([]);
const selectedTags = ref<string[]>([]);
const selectedTypes = ref<number[]>([]);
const selectedDirections = ref<string[]>([]);
const searchQuery = ref('');
const isDeduplicated = ref(false);
const isPaused = ref(false);
const isConnected = ref(false);
const currentPage = ref(1);
const pageSize = ref(20);
const showDetails = ref(false);
const selectedEvent = ref<NetworkEvent | null>(null);
const tableWrapperRef = ref<HTMLElement | null>(null);
const tableContentWidth = ref(0);

let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;
let resizeObserver: ResizeObserver | null = null;
let cleanupColumnResize: (() => void) | null = null;

const columnWidths = ref<Record<ResizableColumnKey, number>>({
  time: 120,
  netDirection: 110,
  type: 150,
  tag: 120,
  pid: 96,
  comm: 150,
  netEndpoint: 260,
  netBytes: 120,
  netFamily: 100,
  path: 220,
  action: 80,
});

const minColumnWidths: Record<ResizableColumnKey, number> = {
  time: 100,
  netDirection: 96,
  type: 120,
  tag: 100,
  pid: 88,
  comm: 120,
  netEndpoint: 180,
  netBytes: 96,
  netFamily: 90,
  path: 180,
  action: 72,
};

const tagOptions = computed(() =>
  tags.value.map((tag) => ({
    label: tag,
    value: tag,
  })),
);

const eventTypeOptions = computed(() =>
  eventTypes.map((type) => ({
    label: type.label.toUpperCase(),
    value: type.value,
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

const formatDetailValue = (value: number | string | undefined | null) => {
  if (value === undefined || value === null || value === '') {
    return '—';
  }
  return String(value);
};

const networkFilteredEvents = computed(() => {
  let result = events.value;

  if (selectedTags.value.length) {
    const activeTags = new Set(selectedTags.value);
    result = result.filter((event) => activeTags.has(event.tag));
  }

  if (selectedTypes.value.length) {
    const activeTypes = new Set(selectedTypes.value);
    result = result.filter((event) => event.eventType !== undefined && activeTypes.has(event.eventType));
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

const getRowClassName = (_record: NetworkEvent, index: number) =>
  (index % 2 === 0 ? 'excel-row-even' : 'excel-row-odd');

const isResizableColumn = (key: string | number | symbol) => ([
  'time',
  'netDirection',
  'type',
  'tag',
  'pid',
  'comm',
  'netEndpoint',
  'netBytes',
  'netFamily',
  'path',
  'action',
] as const).includes(String(key) as ResizableColumnKey);

const tableColumns = computed(() => baseColumns.map((column) => {
  if (column.key === 'path') {
    const fixedWidth = (['time', 'netDirection', 'type', 'tag', 'pid', 'comm', 'netEndpoint', 'netBytes', 'netFamily', 'action'] as const)
      .reduce((total, key) => total + columnWidths.value[key], 0);
    const availableWidth = tableContentWidth.value > 0 ? tableContentWidth.value : 0;
    const dynamicWidth = availableWidth > 0
      ? Math.max(minColumnWidths.path, availableWidth - fixedWidth - 12)
      : columnWidths.value.path;
    return { ...column, width: Math.max(minColumnWidths.path, columnWidths.value.path, dynamicWidth) };
  }
  if (column.key in columnWidths.value) {
    return { ...column, width: columnWidths.value[column.key as ResizableColumnKey] };
  }
  return column;
}));

const handleTableResize = (entries: ResizeObserverEntry[]) => {
  const entry = entries[0];
  if (!entry) return;
  tableContentWidth.value = entry.contentRect.width;
};

const startColumnResize = (key: string | number | symbol, event: MouseEvent) => {
  if (!isResizableColumn(key)) return;
  event.preventDefault();

  const resizeKey = String(key) as ResizableColumnKey;
  const startX = event.clientX;
  const startWidth = columnWidths.value[resizeKey];
  const minWidth = minColumnWidths[resizeKey];

  const onMouseMove = (moveEvent: MouseEvent) => {
    const nextWidth = Math.max(minWidth, startWidth + moveEvent.clientX - startX);
    columnWidths.value[resizeKey] = nextWidth;
  };

  const stopResize = () => {
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', stopResize);
    document.documentElement.classList.remove('excel-resizing');
    cleanupColumnResize = null;
  };

  cleanupColumnResize?.();
  document.documentElement.classList.add('excel-resizing');
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', stopResize);
  cleanupColumnResize = stopResize;
};

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

const typeColor = (eventType?: number, value?: string) => {
  if (eventType !== undefined && eventTypeColorMap[eventType]) {
    return eventTypeColorMap[eventType];
  }
  const fallback = Object.entries(eventTypeLabelMap)
    .find(([, label]) => label === value)
    ?.at(0);
  if (fallback) {
    return eventTypeColorMap[Number(fallback)] || 'default';
  }
  return 'default';
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
  selectedEvent.value = { ...record };
  showDetails.value = true;
};

const clearEvents = async () => {
  try {
    await axios.post('/data/clear-events-memory');
    events.value = [];
    currentPage.value = 1;
    message.success('Event buffer cleared on backend');
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to clear events');
    events.value = [];
    currentPage.value = 1;
  }
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
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }

  const socket = new WebSocket(buildWebSocketUrl('/ws'));
  ws = socket;
  socket.binaryType = 'arraybuffer';

  socket.onopen = () => {
    if (ws !== socket) return;
    isConnected.value = true;
  };

  socket.onmessage = (messageEvent) => {
    if (ws !== socket) return;
    if (isPaused.value) return;
    try {
      const incomingEvents = decodeIncomingEvents(new Uint8Array(messageEvent.data));
      incomingEvents.forEach((data) => {
        const type = data.type ?? '';
        const path = data.path ?? '';
        const pid = data.pid ?? 0;
        const ppid = data.ppid ?? 0;
        const uid = data.uid ?? 0;
        const tag = data.tag ?? '';
        const comm = data.comm ?? '';
        const eventType = extractEventType(data);
        if (!isNetworkEvent(eventType, type)) {
          return;
        }
        const now = new Date();
        events.value.unshift({
          key: `${pid}-${type}-${path}-${Date.now()}-${Math.random()}`,
          pid,
          ppid,
          uid,
          type,
          eventType,
          tag,
          comm,
          path,
          netDirection: data.netDirection || '',
          netEndpoint: data.netEndpoint || '',
          netFamily: data.netFamily || '',
          netBytes: Number(data.netBytes || 0),
          time: now.toLocaleTimeString(),
        });
      });
      while (events.value.length > 1000) {
        events.value.pop();
      }
    } catch (err) {
      console.error('Failed to parse network message', err);
    }
  };

  socket.onclose = () => {
    if (ws !== socket) return;
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
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(handleTableResize);
    resizeObserver.observe(tableWrapperRef.value);
  }
});

onUnmounted(() => {
  shouldReconnect = false;
  resizeObserver?.disconnect();
  resizeObserver = null;
  cleanupColumnResize?.();
  cleanupColumnResize = null;
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }
  ws = null;
});
</script>

<template>
  <div class="network-page">
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

      <div ref="tableWrapperRef" class="network-table-wrap">
        <a-table
          class="network-table"
          :dataSource="networkFilteredEvents"
          :columns="tableColumns"
          row-key="key"
          size="small"
          :pagination="tablePagination"
          :rowClassName="getRowClassName"
          :tableLayout="'fixed'"
          @change="handleTableChange"
        >
        <template #headerCell="{ column }">
          <div class="network-header-cell">
            <span class="network-header-title">{{ column.title }}</span>
            <span
              v-if="isResizableColumn(column.key)"
              class="network-column-resizer"
              title="Drag to resize"
              @mousedown.stop.prevent="startColumnResize(column.key, $event)"
            />
          </div>
        </template>
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'netDirection'">
            <a-tag :color="directionColor(record.netDirection)">{{ formatDirection(record.netDirection) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'type'">
            <a-tag :color="typeColor(record.eventType, record.type)">{{ record.type.toUpperCase() }}</a-tag>
          </template>
          <template v-else-if="column.key === 'tag'">
            <a-tag color="purple">{{ record.tag }}</a-tag>
          </template>
          <template v-else-if="column.key === 'netEndpoint'">
            <a-typography-text class="network-cell-text">{{ formatDetailValue(record.netEndpoint) }}</a-typography-text>
          </template>
          <template v-else-if="column.key === 'netBytes'">
            <span>{{ formatBytes(record.netBytes) }}</span>
          </template>
          <template v-else-if="column.key === 'netFamily'">
            <a-tag :color="familyColor(record.netFamily)">{{ record.netFamily || 'unknown' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'path'">
            <a-typography-text class="network-summary-text">{{ formatDetailValue(record.path) }}</a-typography-text>
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
      </div>
    </a-card>

    <a-modal v-model:open="showDetails" title="Network Event Details" :footer="null" width="700px">
      <a-descriptions bordered :column="1" size="small" v-if="selectedEvent">
        <a-descriptions-item label="Time">{{ selectedEvent.time }}</a-descriptions-item>
        <a-descriptions-item label="Direction">
          <a-tag :color="directionColor(selectedEvent.netDirection)">{{ formatDirection(selectedEvent.netDirection) }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Event Type">
          <a-tag :color="typeColor(selectedEvent.eventType, selectedEvent.type)">{{ selectedEvent.type.toUpperCase() }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Tag">
          <a-tag color="purple">{{ selectedEvent.tag }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Command"><a-typography-text strong>{{ selectedEvent.comm }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="PID"><code>{{ formatDetailValue(selectedEvent.pid) }}</code></a-descriptions-item>
        <a-descriptions-item label="Parent PID (PPID)"><code>{{ formatDetailValue(selectedEvent.ppid) }}</code></a-descriptions-item>
        <a-descriptions-item label="User ID (UID)"><code>{{ formatDetailValue(selectedEvent.uid) }}</code></a-descriptions-item>
        <a-descriptions-item label="Endpoint"><code style="word-break: break-all;">{{ formatDetailValue(selectedEvent.netEndpoint) }}</code></a-descriptions-item>
        <a-descriptions-item label="Family">
          <a-tag :color="familyColor(selectedEvent.netFamily)">{{ selectedEvent.netFamily || 'unknown' }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="Bytes">{{ formatBytes(selectedEvent.netBytes) }}</a-descriptions-item>
        <a-descriptions-item label="Summary"><code style="word-break: break-all;">{{ formatDetailValue(selectedEvent.path) }}</code></a-descriptions-item>
      </a-descriptions>
    </a-modal>
  </div>
</template>

<style scoped>
.network-page {
  width: 100%;
  min-height: 100%;
  box-sizing: border-box;
  color: #1f2937;
}

.network-table-wrap {
  width: 100%;
  min-width: 0;
  overflow-x: auto;
}

.network-table {
  width: 100%;
  min-width: 100%;
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
}

.network-table :deep(.ant-table) {
  font-family: Calibri, 'Segoe UI', Arial, sans-serif;
  background: #fff;
  width: 100%;
}

.network-table :deep(.ant-table-container) {
  width: 100%;
}

.network-table :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, #f7fbf4 0%, #edf4e8 100%);
  color: #1f3a1f;
  font-weight: 700;
  border-right: 1px solid #d9e4d1;
  border-bottom: 1px solid #c7d7bf;
  padding: 10px 12px;
  white-space: nowrap;
}

.network-header-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  width: 100%;
  min-width: 0;
}

.network-header-title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.network-column-resizer {
  width: 10px;
  align-self: stretch;
  flex: 0 0 auto;
  cursor: col-resize;
  position: relative;
  margin-left: 2px;
}

.network-column-resizer::before {
  content: '';
  position: absolute;
  top: 18%;
  bottom: 18%;
  left: 4px;
  width: 1px;
  border-radius: 1px;
  background: rgba(95, 122, 82, 0.5);
}

.network-column-resizer:hover::before {
  background: #2f7d32;
}

.network-table :deep(.ant-table-tbody > tr > td) {
  border-right: 1px solid #e6ece0;
  border-bottom: 1px solid #e6ece0;
  padding: 8px 12px;
  background: #fff;
  vertical-align: top;
  min-width: 0;
  white-space: normal;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.network-table :deep(.ant-table-tbody > tr.excel-row-even > td) {
  background: #ffffff;
}

.network-table :deep(.ant-table-tbody > tr.excel-row-odd > td) {
  background: #fbfdf8;
}

.network-table :deep(.ant-table-tbody > tr:hover > td) {
  background: #eef6e8 !important;
}

.network-table :deep(.ant-table-row) {
  transition: background-color 0.15s ease;
}

.network-table :deep(.ant-tag) {
  border-radius: 2px;
  font-weight: 600;
  letter-spacing: 0.1px;
}

.network-table :deep(.ant-input),
.network-table :deep(.ant-select-selector),
.network-table :deep(.ant-btn),
.network-table :deep(.ant-checkbox-inner) {
  border-radius: 2px !important;
}

.network-cell-text,
.network-summary-text {
  display: block;
  min-width: 0;
  color: #28402a;
  white-space: normal;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.network-table :deep(.ant-table-pagination) {
  margin: 12px 0 0;
}

.network-table :deep(.ant-pagination-item),
.network-table :deep(.ant-pagination-prev),
.network-table :deep(.ant-pagination-next),
.network-table :deep(.ant-select-selector) {
  box-shadow: none;
}

:global(html.excel-resizing),
:global(html.excel-resizing body),
:global(html.excel-resizing *) {
  cursor: col-resize !important;
  user-select: none !important;
}
</style>
