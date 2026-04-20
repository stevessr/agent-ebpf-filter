<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import axios from 'axios';
import { PlusOutlined, SearchOutlined, ClusterOutlined, TableOutlined, HistoryOutlined, FilterOutlined, DeploymentUnitOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';

interface GPUStatus {
  index: number;
  name: string;
  utilGpu: number;
  utilMem: number;
  memTotal: number;
  memUsed: number;
  temp: number;
}

interface ProcessInfo {
  pid: number;
  ppid: number;
  name: string;
  cpu: number;
  mem: number;
  user: string;
  gpuMem: number;
  gpuId: number;
  children?: ProcessInfo[];
}

const processes = ref<ProcessInfo[]>([]);
const gpus = ref<GPUStatus[]>([]);
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
      const decoded = pb.SystemStats.decode(uint8Array);
      
      processes.value = decoded.processes.map((p: any) => ({
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: p.cpu,
        mem: p.mem,
        user: p.user,
        gpuMem: p.gpuMem,
        gpuId: p.gpuId
      }));

      gpus.value = decoded.gpus.map((g: any) => ({
        index: g.index,
        name: g.name,
        utilGpu: g.utilGpu,
        utilMem: g.utilMem,
        memTotal: g.memTotal,
        memUsed: g.memUsed,
        temp: g.temp
      }));
    } catch (e) {
      console.error('Failed to decode system stats', e);
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

  if (searchText.value) {
    const search = searchText.value.toLowerCase();
    filtered = filtered.filter(p => 
      p.name.toLowerCase().includes(search) || 
      p.pid.toString().includes(search)
    );
  }

  if (cpuThreshold.value > 0) filtered = filtered.filter(p => p.cpu >= cpuThreshold.value);
  if (memThreshold.value > 0) filtered = filtered.filter(p => p.mem >= memThreshold.value);
  if (gpuThreshold.value > 0) filtered = filtered.filter(p => p.gpuMem >= gpuThreshold.value);
  if (filterUser.value) filtered = filtered.filter(p => p.user === filterUser.value);

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
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 120, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 140, sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'MEM %', dataIndex: 'mem', key: 'mem', width: 120, sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 130, sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
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
  <div style="background: #f0f2f5; padding: 24px; min-height: 100%;">
    <!-- GPU Stats Section (nvtop style) -->
    <a-row :gutter="16" style="margin-bottom: 16px;">
      <a-col :span="24" v-if="gpus.length === 0 && isConnected">
        <a-empty description="No NVIDIA GPUs detected" />
      </a-col>
      <a-col v-for="gpu in gpus" :key="gpu.index" :span="Math.max(8, 24 / gpus.length)">
        <a-card size="small" class="gpu-card">
          <div style="display: flex; justify-content: space-between; align-items: center;">
            <span style="font-weight: bold;"><DeploymentUnitOutlined /> GPU {{ gpu.index }}: {{ gpu.name }}</span>
            <a-tag color="volcano">{{ gpu.temp }}°C</a-tag>
          </div>
          <a-divider style="margin: 8px 0" />
          <a-row :gutter="16">
            <a-col :span="12">
              <div class="stat-label">GPU Core</div>
              <a-progress type="dashboard" :percent="gpu.utilGpu" :width="70" :stroke-color="{ '0%': '#108ee9', '100%': '#87d068' }" />
            </a-col>
            <a-col :span="12">
              <div class="stat-label">VRAM Usage</div>
              <div style="margin-top: 10px;">
                <div style="font-size: 12px; margin-bottom: 4px;">{{ gpu.memUsed }} / {{ gpu.memTotal }} MiB</div>
                <a-progress :percent="Math.round((gpu.memUsed / gpu.memTotal) * 100)" size="small" stroke-color="#722ed1" />
              </div>
            </a-col>
          </a-row>
        </a-card>
      </a-col>
    </a-row>

    <!-- Main Monitor Section -->
    <div style="background: #fff; padding: 24px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03);">
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

      <a-card v-if="showAdvancedFilters" size="small" style="margin-bottom: 16px; background: #fafafa;">
        <a-row :gutter="24" align="middle">
          <a-col :span="6">
            <span style="font-size: 11px; color: #888;">Min CPU %</span>
            <a-slider v-model:value="cpuThreshold" :min="0" :max="100" />
          </a-col>
          <a-col :span="6">
            <span style="font-size: 11px; color: #888;">Min Memory %</span>
            <a-slider v-model:value="memThreshold" :min="0" :max="20" :step="0.1" />
          </a-col>
          <a-col :span="6">
            <span style="font-size: 11px; color: #888;">Min VRAM (MiB)</span>
            <a-slider v-model:value="gpuThreshold" :min="0" :max="4096" />
          </a-col>
          <a-col :span="6">
            <a-select v-model:value="filterUser" style="width: 100%" placeholder="Filter User" allowClear>
              <a-select-option v-for="user in uniqueUsers" :key="user" :value="user">{{ user }}</a-select-option>
            </a-select>
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
        :scroll="{ y: 'calc(100vh - 480px)' }"
        :indentSize="20"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <span style="font-family: monospace;">{{ record.name }}</span>
          </template>
          <template v-if="column.key === 'cpu'">
            <div style="display: flex; align-items: center; gap: 8px;">
              <a-progress :percent="Math.min(100, Math.round(record.cpu))" size="small" :status="record.cpu > 50 ? 'exception' : 'normal'" :showInfo="false" style="width: 50px" />
              <span style="font-size: 11px; width: 40px;">{{ record.cpu.toFixed(1) }}%</span>
            </div>
          </template>
          <template v-if="column.key === 'mem'">
            <span :style="{ color: record.mem > 10 ? '#ff4d4f' : '#8c8c8c', fontSize: '12px' }">{{ record.mem.toFixed(1) }}%</span>
          </template>
          <template v-if="column.key === 'gpuMem'">
            <a-tag v-if="record.gpuMem > 0" color="purple" style="font-family: monospace;">{{ record.gpuMem }} MiB</a-tag>
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
  </div>
</template>

<style scoped>
.gpu-card {
  border-radius: 8px;
  background: #fff;
  transition: all 0.3s;
}
.gpu-card:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
}
.stat-label {
  font-size: 11px;
  color: #999;
  margin-bottom: 4px;
  text-transform: uppercase;
  letter-spacing: 1px;
}
:deep(.ant-table-wrapper) {
  background: #fff;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
}
</style>
