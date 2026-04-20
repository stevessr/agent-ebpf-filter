<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import axios from 'axios';
import { PlusOutlined, SearchOutlined, ClusterOutlined, TableOutlined, HistoryOutlined, FilterOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';

interface ProcessInfo {
  pid: number;
  ppid: number;
  name: string;
  cpu: number;
  mem: number;
  user: string;
  gpu_mem: number;
  children?: ProcessInfo[];
}

const processes = ref<ProcessInfo[]>([]);
const isConnected = ref(false);
const loading = ref(false);
const searchText = ref('');
const viewMode = ref<'list' | 'tree'>('tree');
const refreshInterval = ref(2000);
const tags = ref<string[]>([]);
const selectedTag = ref('AI Agent');

// Advanced Filters
const cpuThreshold = ref(0);
const memThreshold = ref(0);
const gpuThreshold = ref(0);
const filterUser = ref<string | null>(null);
const showAdvancedFilters = ref(false);

let ws: WebSocket | null = null;

const uniqueUsers = computed(() => {
  const users = new Set(processes.value.map(p => p.user));
  return Array.from(users).sort();
});

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

const connectWebSocket = () => {
  if (ws) ws.close();
  
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const host = window.location.host;
  ws = new WebSocket(`${protocol}//${host}/ws/system?interval=${refreshInterval.value}`);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => {
    isConnected.value = true;
    loading.value = false;
  };

  ws.onmessage = (msg) => {
    try {
      const uint8Array = new Uint8Array(msg.data);
      const decoded = pb.ProcessList.decode(uint8Array);
      processes.value = decoded.processes.map((p: any) => ({
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: p.cpu,
        mem: p.mem,
        user: p.user,
        gpu_mem: p.gpuMem
      }));
    } catch (e) {
      console.error('Failed to decode process list', e);
    }
  };

  ws.onclose = () => {
    isConnected.value = false;
  };
};

watch(refreshInterval, () => {
  connectWebSocket();
});

const buildTree = (list: ProcessInfo[]) => {
  const map: Record<number, ProcessInfo> = {};
  const roots: ProcessInfo[] = [];

  list.forEach(p => {
    map[p.pid] = { ...p, children: [] };
  });

  list.forEach(p => {
    if (p.ppid !== 0 && map[p.ppid]) {
      map[p.ppid].children?.push(map[p.pid]);
    } else {
      roots.push(map[p.pid]);
    }
  });

  const clean = (nodes: ProcessInfo[]) => {
    nodes.forEach(n => {
      if (n.children && n.children.length === 0) {
        delete n.children;
      } else if (n.children) {
        clean(n.children);
      }
    });
  };
  clean(roots);

  return roots;
};

const displayData = computed(() => {
  let filtered = processes.value;

  // Apply Search
  if (searchText.value) {
    const search = searchText.value.toLowerCase();
    filtered = filtered.filter(p => 
      p.name.toLowerCase().includes(search) || 
      p.pid.toString().includes(search)
    );
  }

  // Apply Advanced Filters
  if (cpuThreshold.value > 0) {
    filtered = filtered.filter(p => p.cpu >= cpuThreshold.value);
  }
  if (memThreshold.value > 0) {
    filtered = filtered.filter(p => p.mem >= memThreshold.value);
  }
  if (gpuThreshold.value > 0) {
    filtered = filtered.filter(p => p.gpu_mem >= gpuThreshold.value);
  }
  if (filterUser.value) {
    filtered = filtered.filter(p => p.user === filterUser.value);
  }

  if (viewMode.value === 'tree' && !searchText.value && cpuThreshold.value === 0 && memThreshold.value === 0 && gpuThreshold.value === 0 && !filterUser.value) {
    return buildTree(filtered);
  }
  
  return [...filtered].sort((a, b) => b.cpu - a.cpu);
});

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
  { 
    title: 'PID', 
    dataIndex: 'pid', 
    key: 'pid', 
    width: 120,
    sorter: (a: ProcessInfo, b: ProcessInfo) => a.pid - b.pid 
  },
  { 
    title: 'Name', 
    dataIndex: 'name', 
    key: 'name',
    sorter: (a: ProcessInfo, b: ProcessInfo) => a.name.localeCompare(b.name)
  },
  { 
    title: 'CPU %', 
    dataIndex: 'cpu', 
    key: 'cpu', 
    width: 140,
    sorter: (a: ProcessInfo, b: ProcessInfo) => a.cpu - b.cpu 
  },
  { 
    title: 'MEM %', 
    dataIndex: 'mem', 
    key: 'mem', 
    width: 120,
    sorter: (a: ProcessInfo, b: ProcessInfo) => a.mem - b.mem 
  },
  { 
    title: 'VRAM (MiB)', 
    dataIndex: 'gpu_mem', 
    key: 'gpu_mem', 
    width: 130,
    sorter: (a: ProcessInfo, b: ProcessInfo) => a.gpu_mem - b.gpu_mem 
  },
  { title: 'User', dataIndex: 'user', key: 'user', width: 120 },
  { title: 'Action', key: 'action', width: 120, fixed: 'right' as const }
];

onMounted(() => {
  loading.value = true;
  fetchTags();
  connectWebSocket();
});

onUnmounted(() => {
  if (ws) ws.close();
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 100%;">
    <!-- Top Toolbar -->
    <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center; flex-wrap: wrap; gap: 16px;">
      <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
        <a-input v-model:value="searchText" placeholder="Search PID or Name..." style="width: 220px">
          <template #prefix><SearchOutlined /></template>
        </a-input>
        
        <a-radio-group v-model:value="viewMode" button-style="solid">
          <a-radio-button value="tree"><ClusterOutlined /> Tree</a-radio-button>
          <a-radio-button value="list"><TableOutlined /> List</a-radio-button>
        </a-radio-group>

        <a-button @click="showAdvancedFilters = !showAdvancedFilters" :type="showAdvancedFilters ? 'primary' : 'default'">
          <template #icon><FilterOutlined /></template>
          Filters
        </a-button>

        <div style="display: flex; align-items: center; gap: 8px; background: #f5f5f5; padding: 4px 12px; border-radius: 4px;">
          <HistoryOutlined />
          <a-select v-model:value="refreshInterval" size="small" style="width: 100px" :bordered="false">
            <a-select-option :value="1000">1s Refresh</a-select-option>
            <a-select-option :value="2000">2s Refresh</a-select-option>
            <a-select-option :value="5000">5s Refresh</a-select-option>
            <a-select-option :value="10000">10s Refresh</a-select-option>
          </a-select>
        </div>

        <a-badge :status="isConnected ? 'success' : 'processing'" :text="isConnected ? 'Live' : 'Connecting...'" />
      </div>

      <div style="display: flex; align-items: center; gap: 8px;">
        <span style="font-size: 13px; color: #666;">Track as:</span>
        <a-select v-model:value="selectedTag" style="width: 150px">
          <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
        </a-select>
      </div>
    </div>

    <!-- Advanced Filter Bar -->
    <a-card v-if="showAdvancedFilters" size="small" style="margin-bottom: 16px; background: #fafafa;">
      <a-row :gutter="24" align="middle">
        <a-col :span="6">
          <span style="font-size: 12px; color: #888;">Min CPU %</span>
          <a-slider v-model:value="cpuThreshold" :min="0" :max="100" />
        </a-col>
        <a-col :span="6">
          <span style="font-size: 12px; color: #888;">Min Memory %</span>
          <a-slider v-model:value="memThreshold" :min="0" :max="20" :step="0.1" />
        </a-col>
        <a-col :span="6">
          <span style="font-size: 12px; color: #888;">Min VRAM (MiB)</span>
          <a-slider v-model:value="gpuThreshold" :min="0" :max="4096" :step="1" />
        </a-col>
        <a-col :span="6">
          <span style="font-size: 12px; color: #888;">User</span>
          <a-select v-model:value="filterUser" style="width: 100%" placeholder="All Users" allowClear>
            <a-select-option v-for="user in uniqueUsers" :key="user" :value="user">{{ user }}</a-select-option>
          </a-select>
        </a-col>
        <a-col :span="6" style="text-align: right;">
          <a-button size="small" @click="cpuThreshold = 0; memThreshold = 0; gpuThreshold = 0; filterUser = null;">Reset</a-button>
        </a-col>
      </a-row>
    </a-card>

    <a-table 
      :dataSource="displayData" 
      :columns="columns" 
      size="small"
      :pagination="viewMode === 'list' ? { pageSize: 50 } : false"
      :loading="loading"
      rowKey="pid"
      :scroll="{ y: 'calc(100vh - 350px)' }"
      :indentSize="20"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'name'">
          <span style="font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 13px;">{{ record.name }}</span>
        </template>
        <template v-if="column.key === 'cpu'">
          <div style="display: flex; align-items: center; gap: 8px;">
            <a-progress 
              :percent="Math.min(100, Math.round(record.cpu))" 
              size="small" 
              :status="record.cpu > 50 ? 'exception' : 'normal'" 
              :showInfo="false"
              style="width: 60px"
            />
            <span style="font-size: 11px; font-weight: 500; width: 45px; text-align: right;">{{ record.cpu.toFixed(1) }}%</span>
          </div>
        </template>
        <template v-if="column.key === 'mem'">
          <span :style="{ color: record.mem > 10 ? '#ff4d4f' : '#8c8c8c', fontSize: '12px' }">{{ record.mem.toFixed(1) }}%</span>
        </template>
        <template v-if="column.key === 'gpu_mem'">
          <a-tag v-if="record.gpu_mem > 0" color="purple" style="font-family: monospace;">{{ record.gpu_mem }} MiB</a-tag>
          <span v-else style="color: #bfbfbf;">-</span>
        </template>
        <template v-if="column.key === 'action'">
          <a-button type="primary" size="small" @click="addToRules(record)" ghost>
            <template #icon><PlusOutlined /></template>
            Track
          </a-button>
        </template>
      </template>
    </a-table>
  </div>
</template>

<style scoped>
:deep(.ant-table-wrapper) {
  background: #fff;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
}
:deep(.ant-table-thead > tr > th) {
  background: #fafafa;
}
</style>
