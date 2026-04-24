<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue';
import { useRouter } from 'vue-router';
import axios from 'axios';
import { EyeOutlined, FilterOutlined, FolderOpenOutlined, InfoCircleOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import FilePreviewDrawer from '../components/FilePreviewDrawer.vue';
import { pb } from '../pb/tracker_pb.js';
import { canPreviewEventPath, type FilePreviewResponse } from '../types/filePreview';

interface AgentEvent {
  key: string;
  pid: number;
  ppid: number;
  uid: number;
  type: string;
  tag: string;
  comm: string;
  path: string;
  netDirection?: string;
  netEndpoint?: string;
  netFamily?: string;
  netBytes?: number;
  time: string;
}

type ResizableColumnKey = 'time' | 'tag' | 'pid' | 'comm' | 'type' | 'path' | 'action';

const events = ref<AgentEvent[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const showDetails = ref(false);
const selectedEvent = ref<AgentEvent | null>(null);
const showPreview = ref(false);
const previewLoading = ref(false);
const previewData = ref<FilePreviewResponse | null>(null);
const selectedTags = ref<string[]>([]);
const selectedTypes = ref<string[]>([]);
const timeFilter = ref('');
const pidFilter = ref('');
const commandFilter = ref('');
const pathFilter = ref('');
const isDeduplicated = ref(false);
const activeHeaderFilter = ref<string | null>(null);
const tags = ref<string[]>([]);
const currentPage = ref(1);
const pageSize = ref(20);
const tableWrapperRef = ref<HTMLElement | null>(null);
const tableContentWidth = ref(0);
const router = useRouter();
let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;
let resizeObserver: ResizeObserver | null = null;
let cleanupColumnResize: (() => void) | null = null;
let recentRowTimer: number | null = null;

const STREAM_DIRECTION_STORAGE_KEY = 'dashboard.streamDirection';
const SHOW_ALL_ROWS_STORAGE_KEY = 'dashboard.showAllRows';
const streamDirection = ref<'top' | 'bottom'>(getStoredStreamDirection());
const showAllRows = ref(getStoredShowAllRows());

function getStoredStreamDirection(): 'top' | 'bottom' {
  if (typeof window === 'undefined') return 'top';
  return window.localStorage.getItem(STREAM_DIRECTION_STORAGE_KEY) === 'bottom' ? 'bottom' : 'top';
}

function getStoredShowAllRows(): boolean {
  if (typeof window === 'undefined') return false;
  return window.localStorage.getItem(SHOW_ALL_ROWS_STORAGE_KEY) === 'true';
}

const columnWidths = ref<Record<ResizableColumnKey, number>>({
  time: 120,
  tag: 120,
  pid: 96,
  comm: 150,
  type: 140,
  path: 180,
  action: 80,
});

const minColumnWidths: Record<ResizableColumnKey, number> = {
  time: 100,
  tag: 100,
  pid: 88,
  comm: 120,
  type: 120,
  path: 160,
  action: 72,
};

const eventTypes = [
  'execve',
  'openat',
  'network_connect',
  'network_bind',
  'network_sendto',
  'network_recvfrom',
  'mkdir',
  'unlink',
  'ioctl',
  'wrapper_intercept',
  'native_hook',
];
const pageSizeOptions = ['20', '50', '100', '200'];

const baseColumns = [
  { title: 'Time', dataIndex: 'time', key: 'time' },
  { title: 'Tag', dataIndex: 'tag', key: 'tag' },
  { title: 'PID', dataIndex: 'pid', key: 'pid' },
  { title: 'Command', dataIndex: 'comm', key: 'comm' },
  { title: 'Event Type', dataIndex: 'type', key: 'type' },
  { title: 'Path', dataIndex: 'path', key: 'path', ellipsis: true },
  { title: 'Action', key: 'action', fixed: 'right' as const },
] as const;

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

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
  } catch (err) {
    console.error('Failed to fetch tags', err);
  }
};

const filteredEvents = computed(() => {
  let result = events.value;
  if (selectedTags.value.length) {
    const activeTags = new Set(selectedTags.value);
    result = result.filter(e => activeTags.has(e.tag));
  }
  if (selectedTypes.value.length) {
    const activeTypes = new Set(selectedTypes.value);
    result = result.filter(e => activeTypes.has(e.type));
  }
  const timeQuery = timeFilter.value.trim().toLowerCase();
  if (timeQuery) {
    result = result.filter(e => e.time.toLowerCase().includes(timeQuery));
  }
  const pidQuery = pidFilter.value.trim();
  const commQuery = commandFilter.value.trim().toLowerCase();
  const pathQuery = pathFilter.value.trim().toLowerCase();
  if (pidQuery) {
    result = result.filter(e => String(e.pid).includes(pidQuery));
  }
  if (commQuery) {
    result = result.filter(e => e.comm.toLowerCase().includes(commQuery));
  }
  if (pathQuery) {
    result = result.filter(e => e.path.toLowerCase().includes(pathQuery));
  }
  if (isDeduplicated.value) {
    const seen = new Set();
    result = result.filter(e => {
      const id = `${e.type}-${e.comm}-${e.path}`;
      if (seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }
  return streamDirection.value === 'bottom' ? [...result].reverse() : result;
});

const tablePagination = computed(() => {
  if (showAllRows.value) {
    return false;
  }
  return {
    current: currentPage.value,
    pageSize: pageSize.value,
    total: filteredEvents.value.length,
    showSizeChanger: true,
    pageSizeOptions,
    showTotal: (total: number, range: [number, number]) => `${range[0]}-${range[1]} / ${total}`,
  };
});

const handleTableChange = (pagination: { current?: number; pageSize?: number }) => {
  if (showAllRows.value) return;
  currentPage.value = pagination.current ?? 1;
  pageSize.value = pagination.pageSize ?? pageSize.value;
};

const recentRowKey = ref<string | null>(null);

const markRecentRow = (key: string) => {
  recentRowKey.value = key;
  if (recentRowTimer !== null) {
    window.clearTimeout(recentRowTimer);
  }
  recentRowTimer = window.setTimeout(() => {
    if (recentRowKey.value === key) {
      recentRowKey.value = null;
    }
    recentRowTimer = null;
  }, 320);
};

const getRowClassName = (record: AgentEvent, index: number) => {
  const classes = [index % 2 === 0 ? 'excel-row-even' : 'excel-row-odd'];
  if (recentRowKey.value === record.key) {
    classes.push(streamDirection.value === 'bottom' ? 'excel-row-enter-bottom' : 'excel-row-enter-top');
  }
  return classes.join(' ');
};

const hasHeaderFilter = (key: string | number | symbol) => ['time', 'tag', 'pid', 'comm', 'type', 'path'].includes(String(key));

const isResizableColumn = (key: string | number | symbol) => (['time', 'tag', 'pid', 'comm', 'type', 'path', 'action'] as const).includes(String(key) as ResizableColumnKey);

const getFilterPopupContainer = (triggerNode: HTMLElement) =>
  (triggerNode.closest('.excel-filter-popover') as HTMLElement | null) ?? document.body;

const computePathWidth = () => {
  const fixedWidth = (['time', 'tag', 'pid', 'comm', 'type', 'action'] as const)
    .reduce((total, key) => total + columnWidths.value[key], 0);
  const availableWidth = tableContentWidth.value > 0 ? tableContentWidth.value : 0;
  const remainingWidth = availableWidth > 0 ? Math.max(minColumnWidths.path, availableWidth - fixedWidth - 12) : columnWidths.value.path;
  return Math.max(minColumnWidths.path, columnWidths.value.path, remainingWidth);
};

const tableColumns = computed(() => baseColumns.map((column) => {
  if (column.key === 'path') {
    return { ...column, width: computePathWidth() };
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

const startColumnResize = (key: string, event: MouseEvent) => {
  if (!isResizableColumn(key)) return;
  event.preventDefault();

  const resizeKey = key as ResizableColumnKey;

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

const toggleHeaderFilter = (key: string | number | symbol) => {
  const filterKey = String(key);
  activeHeaderFilter.value = activeHeaderFilter.value === filterKey ? null : filterKey;
};

const closeHeaderFilter = () => {
  activeHeaderFilter.value = null;
};

const handleDocumentClick = (event: MouseEvent) => {
  if (!activeHeaderFilter.value) return;
  const target = event.target;
  if (!(target instanceof Element)) return;
  if (target.closest('.excel-filter-popover') || target.closest('.excel-header-filter-trigger')) {
    return;
  }
  closeHeaderFilter();
};

const isHeaderFilterActive = (key: string | number | symbol) => {
  switch (String(key)) {
    case 'time':
      return Boolean(timeFilter.value.trim());
    case 'tag':
      return selectedTags.value.length > 0;
    case 'pid':
      return Boolean(pidFilter.value.trim());
    case 'comm':
      return Boolean(commandFilter.value.trim());
    case 'type':
      return selectedTypes.value.length > 0;
    case 'path':
      return Boolean(pathFilter.value.trim());
    default:
      return false;
  }
};

const clearHeaderFilter = (key: string | number | symbol) => {
  switch (String(key)) {
    case 'time':
      timeFilter.value = '';
      break;
    case 'tag':
      selectedTags.value = [];
      break;
    case 'pid':
      pidFilter.value = '';
      break;
    case 'comm':
      commandFilter.value = '';
      break;
    case 'type':
      selectedTypes.value = [];
      break;
    case 'path':
      pathFilter.value = '';
      break;
  }
};

watch([selectedTags, selectedTypes, timeFilter, pidFilter, commandFilter, pathFilter, isDeduplicated], () => {
  if (showAllRows.value) return;
  currentPage.value = 1;
});

watch(streamDirection, (direction) => {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STREAM_DIRECTION_STORAGE_KEY, direction);
  }
  if (showAllRows.value) return;
  currentPage.value = direction === 'bottom'
    ? Math.max(1, Math.ceil(filteredEvents.value.length / pageSize.value))
    : 1;
});

watch(showAllRows, (enabled) => {
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(SHOW_ALL_ROWS_STORAGE_KEY, enabled ? 'true' : 'false');
  }
  if (enabled) {
    currentPage.value = 1;
    return;
  }
  const maxPage = Math.max(1, Math.ceil(filteredEvents.value.length / pageSize.value));
  currentPage.value = streamDirection.value === 'bottom' ? maxPage : 1;
});

watch([() => filteredEvents.value.length, pageSize, streamDirection], ([total]) => {
  if (showAllRows.value) return;
  const maxPage = Math.max(1, Math.ceil(total / pageSize.value));
  if (streamDirection.value === 'bottom') {
    currentPage.value = maxPage;
    return;
  }
  if (currentPage.value > maxPage) {
    currentPage.value = maxPage;
  }
});

const openDetails = (record: AgentEvent) => {
  selectedEvent.value = { ...record };
  showDetails.value = true;
};

const formatDetailValue = (value: number | string | undefined | null) => {
  if (value === undefined || value === null || value === '') {
    return '—';
  }
  return String(value);
};

const canInteractWithPath = (record: AgentEvent) => canPreviewEventPath(record);

const previewPath = async (path: string) => {
  previewLoading.value = true;
  try {
    const res = await axios.get(`/system/file-preview?path=${encodeURIComponent(path)}`);
    previewData.value = res.data as FilePreviewResponse;
    showPreview.value = true;
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to preview file');
  } finally {
    previewLoading.value = false;
  }
};

const previewRecordPath = (record: AgentEvent) => {
  if (!canInteractWithPath(record)) return;
  void previewPath(record.path);
};

const openInExplorer = (record: AgentEvent) => {
  if (!canInteractWithPath(record)) return;
  void router.push({
    path: '/explorer',
    query: {
      path: record.path,
      preview: '1',
    },
  });
};

const getTagColor = (type: string) => {
  const colors: Record<string, string> = {
    'execve': 'blue',
    'openat': 'green',
    'network_connect': 'orange',
    'network_bind': 'volcano',
    'network_sendto': 'cyan',
    'network_recvfrom': 'geekblue',
    'mkdir': 'cyan',
    'unlink': 'red',
    'ioctl': 'purple',
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

  ws.onmessage = (message) => {
    if (isPaused.value) return;
    try {
      const uint8Array = new Uint8Array(message.data);
      const data = pb.Event.decode(uint8Array);
      const now = new Date();
      const nextEvent: AgentEvent = {
        key: `${data.pid}-${data.path}-${Date.now()}-${Math.random()}`,
        pid: data.pid,
        ppid: data.ppid,
        uid: data.uid,
        type: data.type,
        tag: data.tag,
        comm: data.comm,
        path: data.path,
        netDirection: data.type?.startsWith('network_') ? (data.netDirection || '') : undefined,
        netEndpoint: data.type?.startsWith('network_') ? (data.netEndpoint || '') : undefined,
        netFamily: data.type?.startsWith('network_') ? (data.netFamily || '') : undefined,
        netBytes: data.type?.startsWith('network_') ? Number(data.netBytes || 0) : undefined,
        time: now.toLocaleTimeString(),
      };
      events.value.unshift(nextEvent);
      if (events.value.length > 1000) events.value.pop();
      markRecentRow(nextEvent.key);
    } catch (e) {
      console.error('Failed to parse message', e);
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

const clearEvents = () => {
  events.value = [];
  recentRowKey.value = null;
  currentPage.value = 1;
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
    message.success('Events exported as JSON');
  } catch (err) {
    message.error('Failed to export events');
  }
};

const exportEventsCSV = () => {
  try {
    const headers = ['Time', 'Tag', 'PID', 'PPID', 'UID', 'Command', 'Event Type', 'Path', 'Net Direction', 'Net Endpoint', 'Net Bytes'];
    const rows = filteredEvents.value.map(e => [
      e.time,
      e.tag,
      e.pid,
      e.ppid,
      e.uid,
      e.comm,
      e.type,
      e.path,
      e.netDirection || '',
      e.netEndpoint || '',
      e.netBytes || 0,
    ]);
    const csvContent = [headers, ...rows].map(r => r.map(c => `"${String(c).replace(/"/g, '""')}"`).join(',')).join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute("href", url);
    link.setAttribute("download", `ebpf-events-${new Date().toISOString()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    message.success('Events exported as CSV');
  } catch (err) {
    message.error('Failed to export CSV');
  }
};

onMounted(() => {
  connectWebSocket();
  fetchTags();
  document.addEventListener('click', handleDocumentClick);
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(handleTableResize);
    resizeObserver.observe(tableWrapperRef.value);
  }
});

onUnmounted(() => {
  shouldReconnect = false;
  document.removeEventListener('click', handleDocumentClick);
  resizeObserver?.disconnect();
  resizeObserver = null;
  cleanupColumnResize?.();
  cleanupColumnResize = null;
  if (recentRowTimer !== null) {
    window.clearTimeout(recentRowTimer);
    recentRowTimer = null;
  }
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) ws.close();
  ws = null;
});
</script>

<template>
  <div class="dashboard-page">
    <div class="dashboard-toolbar">
      <div style="display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; width: 100%;">
        <div style="display: flex; align-items: center; gap: 16px;">
          <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
          <span style="font-weight: 500;">Total Events: {{ events.length }}</span>
          <a-divider type="vertical" />
          <a-button @click="isPaused = !isPaused" :type="isPaused ? 'primary' : 'default'" size="small" danger>
            {{ isPaused ? 'Resume Stream' : 'Pause Stream' }}
          </a-button>
          <a-button type="primary" danger size="small" @click="clearEvents">Clear Events</a-button>
          <a-select
            v-model:value="streamDirection"
            size="small"
            style="width: 150px;"
          >
            <a-select-option value="top">Newest First</a-select-option>
            <a-select-option value="bottom">Log Flow ↓</a-select-option>
          </a-select>
          <a-checkbox v-model:checked="showAllRows">
            <span style="font-size: 12px;">No Page Limit</span>
          </a-checkbox>
          <a-checkbox v-model:checked="isDeduplicated" size="small">
            <span style="font-size: 12px;">Clean Duplicates</span>
          </a-checkbox>
        </div>
        <div style="display: flex; gap: 8px;">
          <a-dropdown>
            <template #overlay>
              <a-menu>
                <a-menu-item key="json" @click="exportEvents">JSON Format</a-menu-item>
                <a-menu-item key="csv" @click="exportEventsCSV">CSV Format</a-menu-item>
              </a-menu>
            </template>
            <a-button size="small">Export Data</a-button>
          </a-dropdown>
        </div>
      </div>
    </div>

    <div ref="tableWrapperRef" class="dashboard-table-wrap">
      <a-table
        class="excel-table"
        :dataSource="filteredEvents"
        :columns="tableColumns"
        size="small"
        :pagination="tablePagination"
        :rowClassName="getRowClassName"
        :tableLayout="'fixed'"
        @change="handleTableChange"
      >
      <template #headerCell="{ column }">
        <div class="excel-header-cell">
          <span class="excel-header-title">{{ column.title }}</span>
          <div class="excel-header-actions">
            <a-popover
              v-if="hasHeaderFilter(column.key)"
              trigger="click"
              placement="bottomRight"
              :arrow="false"
              :open="activeHeaderFilter === column.key"
              overlay-class-name="excel-filter-popover"
            >
              <template #content>
                <div
                  class="excel-filter-dropdown"
                  :class="{ 'excel-filter-dropdown--wide': column.key === 'tag' || column.key === 'type' }"
                  @mousedown.stop
                  @click.stop
                >
                  <div class="excel-filter-dropdown-title">
                    {{ column.title }} Filter
                  </div>
                  <template v-if="column.key === 'time'">
                    <a-input
                      v-model:value="timeFilter"
                      placeholder="Search time..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'tag'">
                    <a-select
                      v-model:value="selectedTags"
                      mode="multiple"
                      placeholder="All Tags"
                      size="small"
                      allow-clear
                      show-search
                      :options="tagOptions"
                      option-filter-prop="label"
                      :get-popup-container="getFilterPopupContainer"
                      style="width: 100%;"
                    />
                  </template>
                  <template v-else-if="column.key === 'pid'">
                    <a-input
                      v-model:value="pidFilter"
                      placeholder="PID contains..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'comm'">
                    <a-input
                      v-model:value="commandFilter"
                      placeholder="Command contains..."
                      size="small"
                      allow-clear
                    />
                  </template>
                  <template v-else-if="column.key === 'type'">
                    <a-select
                      v-model:value="selectedTypes"
                      mode="multiple"
                      placeholder="All Types"
                      size="small"
                      allow-clear
                      show-search
                      :options="eventTypeOptions"
                      option-filter-prop="label"
                      :get-popup-container="getFilterPopupContainer"
                      style="width: 100%;"
                    />
                  </template>
                  <template v-else-if="column.key === 'path'">
                    <a-input
                      v-model:value="pathFilter"
                      placeholder="Path contains..."
                      size="small"
                      allow-clear
                    />
                  </template>

                  <div class="excel-filter-dropdown-actions">
                    <a-button size="small" :disabled="!isHeaderFilterActive(column.key)" @click="clearHeaderFilter(column.key)">
                      Clear
                    </a-button>
                  </div>
                </div>
              </template>
              <a-button
                type="text"
                size="small"
                class="excel-header-filter-trigger"
                :class="{ active: isHeaderFilterActive(column.key) }"
                @click.stop="toggleHeaderFilter(column.key)"
              >
                <template #icon><FilterOutlined /></template>
              </a-button>
            </a-popover>
            <span
              v-if="isResizableColumn(column.key)"
              class="excel-column-resizer"
              title="Drag to resize"
              @mousedown.stop.prevent="startColumnResize(column.key, $event)"
            />
          </div>
        </div>
      </template>
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'type'">
          <a-tag :color="getTagColor(record.type)">{{ record.type.toUpperCase() }}</a-tag>
        </template>
        <template v-if="column.key === 'tag'">
          <a-tag :color="getCategoryColor(record.tag)">{{ record.tag }}</a-tag>
        </template>
        <template v-if="column.key === 'path'">
          <div class="excel-path-cell">
            <a-typography-text
              class="excel-path-text"
              :style="{ cursor: canInteractWithPath(record) ? 'pointer' : 'default' }"
              @click="previewRecordPath(record)"
            >
              {{ formatDetailValue(record.path) }}
            </a-typography-text>
            <a-tooltip v-if="canInteractWithPath(record)" title="Preview file">
              <a-button type="link" size="small" @click.stop="previewRecordPath(record)">
                <template #icon><EyeOutlined /></template>
              </a-button>
            </a-tooltip>
            <a-tooltip v-if="canInteractWithPath(record)" title="Open in Explorer">
              <a-button type="link" size="small" @click.stop="openInExplorer(record)">
                <template #icon><FolderOpenOutlined /></template>
              </a-button>
            </a-tooltip>
          </div>
        </template>
        <template v-if="column.key === 'action'">
          <a-button type="link" size="small" @click="openDetails(record)">
            <template #icon><InfoCircleOutlined /></template>
          </a-button>
        </template>
      </template>
      </a-table>
    </div>

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
        <a-descriptions-item label="PID"><code>{{ formatDetailValue(selectedEvent.pid) }}</code></a-descriptions-item>
        <a-descriptions-item label="Parent PID (PPID)"><code>{{ formatDetailValue(selectedEvent.ppid) }}</code></a-descriptions-item>
        <a-descriptions-item label="User ID (UID)"><code>{{ formatDetailValue(selectedEvent.uid) }}</code></a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netDirection" label="Network Direction">
          <a-tag color="blue">{{ selectedEvent.netDirection }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netEndpoint" label="Network Endpoint">
          <a-typography-text code style="word-break: break-all;">{{ selectedEvent.netEndpoint }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netFamily" label="Network Family">
          <a-tag color="purple">{{ selectedEvent.netFamily }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item v-if="selectedEvent.netBytes !== undefined" label="Network Bytes">
          <a-typography-text code>{{ selectedEvent.netBytes }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Resource Path / Info">
          <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
            <code style="word-break: break-all;">{{ formatDetailValue(selectedEvent.path) }}</code>
            <a-button
              v-if="canInteractWithPath(selectedEvent)"
              type="link"
              size="small"
              @click="previewRecordPath(selectedEvent)"
            >
              <template #icon><EyeOutlined /></template>
              Preview
            </a-button>
            <a-button
              v-if="canInteractWithPath(selectedEvent)"
              type="link"
              size="small"
              @click="openInExplorer(selectedEvent)"
            >
              <template #icon><FolderOpenOutlined /></template>
              Open in Explorer
            </a-button>
          </div>
        </a-descriptions-item>
      </a-descriptions>
    </a-modal>

    <FilePreviewDrawer
      v-model:open="showPreview"
      :loading="previewLoading"
      :preview="previewData"
      title="Log File Preview"
    />
  </div>
</template>

<style scoped>
.dashboard-page {
  min-height: 280px;
  padding: 0;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbf5 100%);
  font-family: Calibri, 'Segoe UI', Arial, sans-serif;
  color: #1f2937;
  width: 100%;
  box-sizing: border-box;
}

.dashboard-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  padding: 12px 14px;
  background: #f8fcf6;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.75);
}

.dashboard-toolbar {
  justify-content: space-between;
  margin-bottom: 10px;
}

.excel-table {
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
  width: 100%;
  min-width: 100%;
}

.dashboard-table-wrap {
  width: 100%;
  min-width: 0;
  overflow-x: auto;
}

.excel-table :deep(.ant-table) {
  font-family: inherit;
  background: #fff;
  width: 100%;
}

.excel-table :deep(.ant-table-container) {
  border-top-left-radius: 6px;
  border-top-right-radius: 6px;
  width: 100%;
}

.excel-table :deep(.ant-table-thead > tr > th) {
  background: linear-gradient(180deg, #f7fbf4 0%, #edf4e8 100%);
  color: #1f3a1f;
  font-weight: 700;
  border-right: 1px solid #d9e4d1;
  border-bottom: 1px solid #c7d7bf;
  padding: 10px 12px;
  white-space: nowrap;
}

.excel-header-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  width: 100%;
  min-width: 0;
}

.excel-header-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1 1 auto;
}

.excel-header-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: 0 0 auto;
}

.excel-header-filter-trigger {
  flex: 0 0 auto;
  width: 20px !important;
  height: 20px !important;
  padding: 0 !important;
  border-radius: 2px !important;
  color: #5f7a52 !important;
}

.excel-header-filter-trigger.active {
  color: #2f7d32 !important;
  background: rgba(72, 143, 81, 0.12) !important;
}

.excel-column-resizer {
  width: 10px;
  align-self: stretch;
  flex: 0 0 auto;
  cursor: col-resize;
  position: relative;
  margin-left: 2px;
}

.excel-column-resizer::before {
  content: '';
  position: absolute;
  top: 18%;
  bottom: 18%;
  left: 4px;
  width: 1px;
  border-radius: 1px;
  background: rgba(95, 122, 82, 0.5);
}

.excel-column-resizer:hover::before {
  background: #2f7d32;
}

.excel-filter-dropdown {
  width: 240px;
  padding: 12px;
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 6px 18px rgba(34, 54, 24, 0.12);
}

.excel-filter-dropdown--wide {
  width: 420px;
}

.excel-filter-dropdown-title {
  font-size: 12px;
  font-weight: 700;
  color: #355238;
  margin-bottom: 10px;
  letter-spacing: 0.2px;
}

.excel-filter-dropdown-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
}

.excel-filter-dropdown :deep(.ant-select-selector) {
  min-height: 32px !important;
  height: auto !important;
  align-items: flex-start !important;
}

.excel-filter-dropdown :deep(.ant-select-selection-overflow) {
  flex-wrap: wrap;
  align-items: flex-start;
}

.excel-filter-dropdown :deep(.ant-select-selection-overflow-item) {
  margin-bottom: 2px;
}

.excel-path-cell {
  display: flex;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 6px;
  min-width: 0;
}

.excel-path-text {
  flex: 1 1 auto;
  min-width: 0;
  display: block;
  color: #28402a;
  white-space: normal;
  word-break: break-word;
  overflow-wrap: anywhere;
}

.excel-table :deep(.ant-table-thead > tr > th:last-child),
.excel-table :deep(.ant-table-tbody > tr > td:last-child) {
  border-right: none;
}

.excel-table :deep(.ant-table-tbody > tr > td) {
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

.excel-table :deep(.ant-table-tbody > tr.excel-row-even > td) {
  background: #ffffff;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-odd > td) {
  background: #fbfdf8;
}

.excel-table :deep(.ant-table-tbody > tr:hover > td) {
  background: #eef6e8 !important;
}

.excel-table :deep(.ant-table-row) {
  transition: background-color 0.15s ease;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-enter-top > td) {
  animation: excel-row-enter-top 320ms ease-out;
}

.excel-table :deep(.ant-table-tbody > tr.excel-row-enter-bottom > td) {
  animation: excel-row-enter-bottom 320ms ease-out;
}

.excel-table :deep(.ant-tag) {
  border-radius: 2px;
  font-weight: 600;
  letter-spacing: 0.1px;
}

.excel-table :deep(.ant-input),
.excel-table :deep(.ant-select-selector),
.excel-table :deep(.ant-btn),
.excel-table :deep(.ant-checkbox-inner) {
  border-radius: 2px !important;
}

.excel-table :deep(.ant-table-pagination) {
  margin: 12px 0 0;
}

.excel-table :deep(.ant-pagination-item),
.excel-table :deep(.ant-pagination-prev),
.excel-table :deep(.ant-pagination-next),
.excel-table :deep(.ant-select-selector) {
  box-shadow: none;
}

.excel-table :deep(.ant-dropdown) {
  z-index: 1200;
}

:global(html.excel-resizing),
:global(html.excel-resizing body),
:global(html.excel-resizing *) {
  cursor: col-resize !important;
  user-select: none !important;
}

@keyframes excel-row-enter-top {
  0% {
    opacity: 0;
    transform: translateY(-10px);
    background-color: #edf8e9;
  }
  70% {
    opacity: 1;
    transform: translateY(0);
    background-color: #f3fbef;
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    background-color: inherit;
  }
}

@keyframes excel-row-enter-bottom {
  0% {
    opacity: 0;
    transform: translateY(10px);
    background-color: #edf8e9;
  }
  70% {
    opacity: 1;
    transform: translateY(0);
    background-color: #f3fbef;
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    background-color: inherit;
  }
}
</style>
