<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import axios from 'axios';
import { 
  PlusOutlined, SearchOutlined, ClusterOutlined, TableOutlined, 
  FilterOutlined, DeploymentUnitOutlined,
  DashboardOutlined, PieChartOutlined, SwapOutlined, DatabaseOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';

interface GPUStatus {
  index: number; name: string; utilGpu: number; utilMem: number;
  memTotal: number; memUsed: number; temp: number;
}

interface ProcessInfo {
  pid: number; ppid: number; name: string; cpu: number; mem: number;
  user: string; gpuMem: number; gpuId: number; children?: ProcessInfo[];
}

interface GlobalStats {
  cpuTotal: number; cpuCores: number[];
  memTotal: number; memUsed: number; memPercent: number;
  readSpeed: number; writeSpeed: number;
  netRecvSpeed: number; netSentSpeed: number;
}

const processes = ref<ProcessInfo[]>([]);
const gpus = ref<GPUStatus[]>([]);
const systemStats = ref<GlobalStats>({
  cpuTotal: 0, cpuCores: [], memTotal: 0, memUsed: 0, memPercent: 0,
  readSpeed: 0, writeSpeed: 0, netRecvSpeed: 0, netSentSpeed: 0
});

const isConnected = ref(false);
const loading = ref(false);
const searchText = ref('');
const viewMode = ref<'list' | 'tree'>('tree');
const cpuViewMode = ref<'total' | 'cores'>('total');
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
let lastIO: { diskR: number; diskW: number; netR: number; netS: number; time: number } | null = null;

const formatBytes = (bytes: number, decimals = 2) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
};

const connectWebSocket = () => {
  if (ws) ws.close();
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${protocol}//${window.location.host}/ws/system?interval=${refreshInterval.value}`);
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => { isConnected.value = true; loading.value = false; };
  ws.onmessage = (msg) => {
    try {
      const decoded = pb.SystemStats.decode(new Uint8Array(msg.data));
      const now = Date.now();
      
      if (lastIO && decoded.io) {
        const dt = (now - lastIO.time) / 1000;
        systemStats.value.readSpeed = (Number(decoded.io.readBytes) - lastIO.diskR) / dt;
        systemStats.value.writeSpeed = (Number(decoded.io.writeBytes) - lastIO.diskW) / dt;
        systemStats.value.netRecvSpeed = (Number(decoded.io.netRecvBytes) - lastIO.netR) / dt;
        systemStats.value.netSentSpeed = (Number(decoded.io.netSentBytes) - lastIO.netS) / dt;
      }
      if (decoded.io) {
        lastIO = { 
          diskR: Number(decoded.io.readBytes), diskW: Number(decoded.io.writeBytes),
          netR: Number(decoded.io.netRecvBytes), netS: Number(decoded.io.netSentBytes),
          time: now 
        };
      }

      if (decoded.cpu) {
        systemStats.value.cpuTotal = decoded.cpu.total || 0;
        systemStats.value.cpuCores = (decoded.cpu.cores as number[]) || [];
      }
      
      if (decoded.memory) {
        systemStats.value.memTotal = Number(decoded.memory.total);
        systemStats.value.memUsed = Number(decoded.memory.used);
        systemStats.value.memPercent = decoded.memory.percent || 0;
      }

      processes.value = (decoded.processes || []).map((p: any) => ({
        pid: p.pid, ppid: p.ppid, name: p.name, cpu: p.cpu, mem: p.mem, user: p.user, gpuMem: p.gpuMem, gpuId: p.gpuId
      }));

      gpus.value = (decoded.gpus || []).map((g: any) => ({
        index: g.index, name: g.name, utilGpu: g.utilGpu, utilMem: g.utilMem,
        memTotal: g.memTotal, memUsed: g.memUsed, temp: g.temp
      }));
    } catch (e) { console.error(e); }
  };
  ws.onclose = () => { isConnected.value = false; };
};

const buildTree = (list: ProcessInfo[]) => {
  const map: Record<number, ProcessInfo> = {};
  const roots: ProcessInfo[] = [];
  list.forEach(p => map[p.pid] = { ...p, children: [] });
  list.forEach(p => {
    if (p.ppid !== 0 && map[p.ppid]) map[p.ppid].children?.push(map[p.pid]);
    else roots.push(map[p.pid]);
  });
  const clean = (nodes: ProcessInfo[]) => {
    nodes.forEach(n => { if (n.children?.length === 0) delete n.children; else if (n.children) clean(n.children); });
  };
  clean(roots);
  return roots;
};

const uniqueUsers = computed(() => Array.from(new Set(processes.value.map(p => p.user))).sort());

const displayData = computed(() => {
  let filtered = processes.value;
  if (searchText.value) {
    const s = searchText.value.toLowerCase();
    filtered = filtered.filter(p => p.name.toLowerCase().includes(s) || p.pid.toString().includes(s));
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
    await axios.post('/config/comms', { comm: proc.name, tag: selectedTag.value });
    message.success(`Tracking ${proc.name}`);
  } catch (err) { message.error('Failed to add rule'); }
};

onMounted(() => {
  loading.value = true;
  axios.get('/config/tags').then(res => tags.value = res.data);
  connectWebSocket();
});
onUnmounted(() => ws?.close());
watch(refreshInterval, connectWebSocket);
</script>

<template>
  <div style="background: #f0f2f5; padding: 20px; min-height: 100%;">
    <!-- Top System Overview (btop style) -->
    <a-row :gutter="12" style="margin-bottom: 16px;">
      <!-- CPU Section -->
      <a-col :span="8">
        <a-card size="small" class="stat-card">
          <template #title>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span><DashboardOutlined /> CPU</span>
              <a-radio-group v-model:value="cpuViewMode" size="small">
                <a-radio-button value="total">Overall</a-radio-button>
                <a-radio-button value="cores">Cores</a-radio-button>
              </a-radio-group>
            </div>
          </template>
          <div v-if="cpuViewMode === 'total'" style="text-align: center; padding: 10px;">
            <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="100" />
            <div style="margin-top: 4px; font-weight: bold;">{{ systemStats.cpuTotal.toFixed(1) }}% Usage</div>
          </div>
          <div v-else class="core-grid">
            <div v-for="(p, i) in systemStats.cpuCores" :key="i" class="core-item">
              <span class="core-label">#{{ i }}</span>
              <a-progress :percent="Math.round(p)" size="small" :showInfo="false" stroke-color="#52c41a" />
            </div>
          </div>
        </a-card>
      </a-col>

      <!-- RAM Section -->
      <a-col :span="8">
        <a-card size="small" class="stat-card" title="Memory">
          <template #extra><PieChartOutlined /></template>
          <div style="padding: 10px;">
            <div style="display: flex; justify-content: space-between; margin-bottom: 8px;">
              <span>Used: {{ formatBytes(systemStats.memUsed) }}</span>
              <span>Total: {{ formatBytes(systemStats.memTotal) }}</span>
            </div>
            <a-progress :percent="Math.round(systemStats.memPercent)" stroke-color="#1890ff" status="active" />
            <div style="margin-top: 15px;">
              <div class="stat-row">
                <span><SwapOutlined /> Network</span>
                <span style="color: #52c41a">↓ {{ formatBytes(systemStats.netRecvSpeed) }}/s</span>
                <span style="color: #1890ff">↑ {{ formatBytes(systemStats.netSentSpeed) }}/s</span>
              </div>
              <div class="stat-row">
                <span><DatabaseOutlined /> Disk I/O</span>
                <span style="color: #faad14">R: {{ formatBytes(systemStats.readSpeed) }}/s</span>
                <span style="color: #ff4d4f">W: {{ formatBytes(systemStats.writeSpeed) }}/s</span>
              </div>
            </div>
          </div>
        </a-card>
      </a-col>

      <!-- GPU Section -->
      <a-col :span="8">
        <a-card size="small" class="stat-card" :title="gpus.length ? 'NVIDIA GPU' : 'No GPU Detected'">
          <template #extra><DeploymentUnitOutlined /></template>
          <div v-for="gpu in gpus" :key="gpu.index" style="margin-bottom: 12px;">
            <div style="display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 4px;">
              <span>GPU {{ gpu.index }}: {{ gpu.name }}</span>
              <a-tag color="volcano" size="small">{{ gpu.temp }}°C</a-tag>
            </div>
            <a-row :gutter="8">
              <a-col :span="12">
                <a-progress :percent="gpu.utilGpu" size="small" stroke-color="#13c2c2" />
                <div style="font-size: 10px; color: #999;">Util: {{ gpu.utilGpu }}%</div>
              </a-col>
              <a-col :span="12">
                <a-progress :percent="Math.round((gpu.memUsed / gpu.memTotal) * 100)" size="small" stroke-color="#722ed1" />
                <div style="font-size: 10px; color: #999;">VRAM: {{ gpu.memUsed }}MB</div>
              </a-col>
            </a-row>
          </div>
          <a-empty v-if="!gpus.length" :image="false" description="None" />
        </a-card>
      </a-col>
    </a-row>

    <!-- Advanced Filter Bar -->
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

    <!-- Processes Section -->
    <div style="background: #fff; padding: 16px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03);">
      <div style="display: flex; justify-content: space-between; margin-bottom: 12px; align-items: center;">
        <div style="display: flex; align-items: center; gap: 8px;">
          <a-input v-model:value="searchText" placeholder="Search..." style="width: 180px"><template #prefix><SearchOutlined /></template></a-input>
          <a-radio-group v-model:value="viewMode" button-style="solid" size="small">
            <a-radio-button value="tree"><ClusterOutlined /></a-radio-button>
            <a-radio-button value="list"><TableOutlined /></a-radio-button>
          </a-radio-group>
          <a-button size="small" @click="showAdvancedFilters = !showAdvancedFilters"><FilterOutlined /></a-button>
          <a-select v-model:value="refreshInterval" size="small" style="width: 80px">
            <a-select-option :value="1000">1s</a-select-option>
            <a-select-option :value="2000">2s</a-select-option>
            <a-select-option :value="5000">5s</a-select-option>
          </a-select>
          <a-badge :status="isConnected ? 'success' : 'processing'" />
        </div>
        <div style="display: flex; align-items: center; gap: 4px;">
          <span style="font-size: 12px;">Track as:</span>
          <a-select v-model:value="selectedTag" size="small" style="width: 120px">
            <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
          </a-select>
        </div>
      </div>

      <a-table 
        :dataSource="displayData" :columns="[
          { title: 'PID', dataIndex: 'pid', width: 100 },
          { title: 'Name', dataIndex: 'name' },
          { title: 'CPU', dataIndex: 'cpu', width: 100 },
          { title: 'MEM', dataIndex: 'mem', width: 100 },
          { title: 'VRAM', dataIndex: 'gpuMem', width: 100 },
          { title: 'User', dataIndex: 'user', width: 100 },
          { title: '', key: 'action', width: 80, fixed: 'right' }
        ]" 
        size="small" :pagination="viewMode === 'list' ? { pageSize: 50 } : false" rowKey="pid" :scroll="{ y: 'calc(100vh - 550px)' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'name'"><span class="mono">{{ record.name }}</span></template>
          <template v-if="column.dataIndex === 'cpu'"><span :style="{color: record.cpu > 10 ? 'red' : 'inherit'}">{{ record.cpu.toFixed(1) }}%</span></template>
          <template v-if="column.dataIndex === 'mem'">{{ record.mem.toFixed(1) }}%</template>
          <template v-if="column.dataIndex === 'gpuMem'">{{ record.gpuMem > 0 ? record.gpuMem + 'MB' : '-' }}</template>
          <template v-if="column.key === 'action'"><a-button type="link" size="small" @click="addToRules(record)"><PlusOutlined /></a-button></template>
        </template>
      </a-table>
    </div>
  </div>
</template>

<style scoped>
.stat-card { height: 200px; border-radius: 8px; overflow: hidden; }
.core-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 4px; padding: 4px; overflow: auto; height: 150px; }
.core-item { font-size: 10px; display: flex; flex-direction: column; align-items: center; }
.core-label { margin-bottom: 2px; color: #999; }
.stat-row { display: flex; justify-content: space-between; font-size: 11px; margin-top: 4px; }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 12px; }
:deep(.ant-progress-circle-path) { stroke-linecap: round; }
</style>
