<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import axios from 'axios';
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

interface ProcessInfo {
  pid: number;
  ppid: number;
  name: string;
  cpu: number;
  mem: number;
  user: string;
}

const processes = ref<ProcessInfo[]>([]);
const loading = ref(false);
const searchText = ref('');
const autoRefresh = ref(true);
const tags = ref<string[]>([]);
const selectedTag = ref('AI Agent');
let refreshInterval: any = null;

const fetchProcesses = async () => {
  loading.value = true;
  try {
    const res = await axios.get('/system/processes');
    processes.value = res.data.sort((a: any, b: any) => b.cpu - a.cpu);
  } catch (err) {
    message.error('Failed to fetch system processes');
  } finally {
    loading.value = false;
  }
};

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
    if (tags.value.length > 0 && !selectedTag.value) {
      selectedTag.value = tags.value[0];
    }
  } catch (err) {
    console.error('Failed to fetch tags', err);
  }
};

const addToRules = async (proc: ProcessInfo) => {
  try {
    await axios.post('/config/comms', { 
      comm: proc.name,
      tag: selectedTag.value
    });
    message.success(`Added ${proc.name} to ${selectedTag.value} tracking`);
  } catch (err) {
    message.error('Failed to add process to rules');
  }
};

const columns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 100, sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'MEM %', dataIndex: 'mem', key: 'mem', width: 100, sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'User', dataIndex: 'user', key: 'user', width: 120 },
  { title: 'Action', key: 'action', width: 120, fixed: 'right' as const }
];

const startRefresh = () => {
  if (refreshInterval) clearInterval(refreshInterval);
  refreshInterval = setInterval(() => {
    if (autoRefresh.value) fetchProcesses();
  }, 3000);
};

onMounted(() => {
  fetchProcesses();
  fetchTags();
  startRefresh();
});

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval);
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 100%;">
    <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center;">
      <div style="display: flex; align-items: center; gap: 16px;">
        <a-input v-model:value="searchText" placeholder="Search process..." style="width: 200px">
          <template #prefix><SearchOutlined /></template>
        </a-input>
        <a-checkbox v-model:checked="autoRefresh">Auto-refresh (3s)</a-checkbox>
        <a-divider type="vertical" />
        <span>Tag for new rules:</span>
        <a-select v-model:value="selectedTag" style="width: 150px">
          <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
        </a-select>
      </div>
      <a-button @click="fetchProcesses" :loading="loading">
        <template #icon><ReloadOutlined /></template>
        Refresh Now
      </a-button>
    </div>

    <a-table 
      :dataSource="processes.filter(p => p.name.toLowerCase().includes(searchText.toLowerCase()))" 
      :columns="columns" 
      size="small"
      :pagination="{ pageSize: 50 }"
      :loading="loading"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'cpu'">
          <a-progress :percent="Math.min(100, Math.round(record.cpu))" size="small" :status="record.cpu > 50 ? 'exception' : 'normal'" />
        </template>
        <template v-if="column.key === 'mem'">
          <span :style="{ color: record.mem > 10 ? 'red' : 'inherit' }">{{ record.mem.toFixed(1) }}%</span>
        </template>
        <template v-if="column.key === 'action'">
          <a-button type="primary" size="small" @click="addToRules(record)">
            <template #icon><PlusOutlined /></template>
            Track
          </a-button>
        </template>
      </template>
    </a-table>
  </div>
</template>
