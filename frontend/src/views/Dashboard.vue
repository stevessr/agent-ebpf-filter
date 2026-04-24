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
const pidFilter = ref('');
const commandFilter = ref('');
const pathFilter = ref('');
const isDeduplicated = ref(false);
const tags = ref<string[]>([]);
const currentPage = ref(1);
const pageSize = ref(20);
const router = useRouter();
let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;

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
  return result;
});

const tablePagination = computed(() => ({
  current: currentPage.value,
  pageSize: pageSize.value,
  total: filteredEvents.value.length,
  showSizeChanger: true,
  pageSizeOptions,
  showTotal: (total: number, range: [number, number]) => `${range[0]}-${range[1]} / ${total}`,
}));

const handleTableChange = (pagination: { current?: number; pageSize?: number }) => {
  currentPage.value = pagination.current ?? 1;
  pageSize.value = pagination.pageSize ?? pageSize.value;
};

const getRowClassName = (_record: AgentEvent, index: number) =>
  (index % 2 === 0 ? 'excel-row-even' : 'excel-row-odd');

watch([selectedTags, selectedTypes, pidFilter, commandFilter, pathFilter, isDeduplicated], () => {
  currentPage.value = 1;
});

watch([() => filteredEvents.value.length, pageSize], ([total]) => {
  const maxPage = Math.max(1, Math.ceil(total / pageSize.value));
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
      events.value.unshift({
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
      });
      if (events.value.length > 1000) events.value.pop();
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
});

onUnmounted(() => {
  shouldReconnect = false;
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

    <div class="dashboard-filter-bar">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 12px; color: #888;">Filter:</span>
          <a-select
            v-model:value="selectedTags"
            mode="multiple"
            placeholder="All Tags"
            style="width: 220px"
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
            style="width: 220px"
            size="small"
            allow-clear
            show-search
            max-tag-count="responsive"
            :options="eventTypeOptions"
            option-filter-prop="label"
          />
        </div>

        <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
          <a-input
            v-model:value="pidFilter"
            placeholder="PID"
            size="small"
            allow-clear
            style="width: 120px"
          />
          <a-input
            v-model:value="commandFilter"
            placeholder="Command"
            size="small"
            allow-clear
            style="width: 160px"
          />
          <a-input
            v-model:value="pathFilter"
            placeholder="Path"
            size="small"
            allow-clear
            style="width: 240px"
          />
        </div>

        <a-divider type="vertical" />

        <a-checkbox v-model:checked="isDeduplicated" size="small">
          <span style="font-size: 12px;">Clean Duplicates</span>
        </a-checkbox>
      </div>
    </div>

    <a-table 
      class="excel-table"
      :dataSource="filteredEvents" 
      :columns="columns" 
      size="small"
      :pagination="tablePagination"
      :rowClassName="getRowClassName"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'type'">
          <a-tag :color="getTagColor(record.type)">{{ record.type.toUpperCase() }}</a-tag>
        </template>
        <template v-if="column.key === 'tag'">
          <a-tag :color="getCategoryColor(record.tag)">{{ record.tag }}</a-tag>
        </template>
        <template v-if="column.key === 'path'">
          <div style="display: flex; align-items: center; gap: 6px;">
            <a-typography-text
              code
              style="word-break: break-all;"
              :style="{ cursor: canInteractWithPath(record) ? 'pointer' : 'default' }"
              @click="previewRecordPath(record)"
            >
              {{ record.path }}
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
  padding: 24px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fbf5 100%);
  font-family: Calibri, 'Segoe UI', Arial, sans-serif;
  color: #1f2937;
}

.dashboard-toolbar,
.dashboard-filter-bar {
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

.dashboard-filter-bar {
  margin-bottom: 12px;
  background: #fbfdf8;
}

.excel-table {
  border: 1px solid #d9e4d1;
  border-radius: 6px;
  overflow: hidden;
  background: #fff;
}

.excel-table :deep(.ant-table) {
  font-family: inherit;
  background: #fff;
}

.excel-table :deep(.ant-table-container) {
  border-top-left-radius: 6px;
  border-top-right-radius: 6px;
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

.excel-table :deep(.ant-table-thead > tr > th:last-child),
.excel-table :deep(.ant-table-tbody > tr > td:last-child) {
  border-right: none;
}

.excel-table :deep(.ant-table-tbody > tr > td) {
  border-right: 1px solid #e6ece0;
  border-bottom: 1px solid #e6ece0;
  padding: 8px 12px;
  background: #fff;
  vertical-align: middle;
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
</style>
