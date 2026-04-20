<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue';
import axios from 'axios';
import { 
  PlusOutlined, SearchOutlined, ClusterOutlined, TableOutlined, 
  FilterOutlined, DeploymentUnitOutlined,
  DashboardOutlined, PieChartOutlined, SwapOutlined, DatabaseOutlined,
  AppstoreOutlined, BarChartOutlined
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

const activeTab = ref('dashboard');
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

const memoryVisualizationData = computed(() => {
  const groups: Record<string, { name: string; mem: number; count: number; pids: number[] }> = {};
  
  processes.value.forEach(p => {
    if (p.mem <= 0.05) return; // Ignore tiny fragments
    
    if (!groups[p.name]) {
      groups[p.name] = { name: p.name, mem: 0, count: 0, pids: [] };
    }
    groups[p.name].mem += p.mem;
    groups[p.name].count += 1;
    groups[p.name].pids.push(p.pid);
  });

  return Object.values(groups)
    .sort((a, b) => b.mem - a.mem)
    .slice(0, 50); // Top 50 grouped apps
});

const getMemColor = (percent: number) => {
  if (percent > 20) return '#cf1322'; // Critical
  if (percent > 10) return '#d4380d'; // High
  if (percent > 5) return '#d46b08';  // Medium-High
  if (percent > 2) return '#1d39c4';  // Medium
  return '#389e0d'; // Low
};

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
    <a-tabs v-model:activeKey="activeTab" type="card" class="monitor-tabs">
      
      <!-- DASHBOARD TAB -->
      <a-tab-pane key="dashboard" tab="Health">
        <template #tab><span><DashboardOutlined /> Health</span></template>
        
        <!-- CPU Row -->
        <a-row style="margin-bottom: 16px;">
          <a-col :span="24">
            <a-card size="small" class="stat-card-row">
              <template #title>
                <div style="display: flex; justify-content: space-between; align-items: center;">
                  <span><DashboardOutlined /> CPU Status</span>
                  <a-radio-group v-model:value="cpuViewMode" size="small">
                    <a-radio-button value="total">Overall</a-radio-button>
                    <a-radio-button value="cores">Cores</a-radio-button>
                  </a-radio-group>
                </div>
              </template>
              <div v-if="cpuViewMode === 'total'" style="display: flex; align-items: center; justify-content: center; height: 120px; gap: 40px;">
                <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="100" />
                <div style="text-align: left;">
                  <div style="font-size: 24px; font-weight: bold; color: #1890ff;">{{ systemStats.cpuTotal.toFixed(1) }}%</div>
                  <div style="color: #888;">Total System Load</div>
                </div>
              </div>
              <div v-else class="core-grid-full">
                <div v-for="(p, i) in systemStats.cpuCores" :key="i" class="core-item-full">
                  <span class="core-label">Core {{ i }}</span>
                  <div style="flex: 1; margin: 0 10px;">
                    <a-progress :percent="Math.round(p)" size="small" :showInfo="false" stroke-color="#52c41a" />
                  </div>
                  <span class="core-val">{{ p.toFixed(1) }}%</span>
                </div>
              </div>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="16" style="margin-bottom: 16px;">
          <!-- Memory & I/O Section -->
          <a-col :span="12">
            <a-card size="small" class="stat-card-row" title="Memory & System I/O">
              <template #extra><PieChartOutlined /></template>
              <div style="display: flex; gap: 24px; padding: 10px;">
                <div style="flex: 1;">
                  <div style="display: flex; justify-content: space-between; margin-bottom: 4px; font-size: 12px;">
                    <span>RAM: {{ formatBytes(systemStats.memUsed) }} / {{ formatBytes(systemStats.memTotal) }}</span>
                    <span style="font-weight: bold;">{{ systemStats.memPercent.toFixed(1) }}%</span>
                  </div>
                  <a-progress :percent="Math.round(systemStats.memPercent)" stroke-color="#1890ff" status="active" />
                </div>
                <div style="flex: 1; display: flex; flex-direction: column; gap: 12px; border-left: 1px solid #f0f0f0; padding-left: 20px;">
                  <div class="io-stat">
                    <span class="io-label"><SwapOutlined /> Network</span>
                    <div class="io-values">
                      <span style="color: #52c41a">↓ {{ formatBytes(systemStats.netRecvSpeed) }}/s</span>
                      <span style="color: #1890ff">↑ {{ formatBytes(systemStats.netSentSpeed) }}/s</span>
                    </div>
                  </div>
                  <div class="io-stat">
                    <span class="io-label"><DatabaseOutlined /> Disk I/O</span>
                    <div class="io-values">
                      <span style="color: #faad14">R: {{ formatBytes(systemStats.readSpeed) }}/s</span>
                      <span style="color: #ff4d4f">W: {{ formatBytes(systemStats.writeSpeed) }}/s</span>
                    </div>
                  </div>
                </div>
              </div>
            </a-card>
          </a-col>

          <!-- GPU Section -->
          <a-col :span="12">
            <a-card size="small" class="stat-card-row" :title="gpus.length ? 'GPU Acceleration' : 'No GPU'">
              <template #extra><DeploymentUnitOutlined /></template>
              <div v-for="gpu in gpus" :key="gpu.index" style="display: flex; align-items: center; gap: 20px; padding: 5px 10px;">
                <div style="flex-shrink: 0; text-align: center;">
                  <a-tag color="volcano" style="margin-bottom: 4px;">{{ gpu.temp }}°C</a-tag>
                  <div style="font-size: 10px; color: #999;">GPU {{ gpu.index }}</div>
                </div>
                <div style="flex: 1;">
                  <div style="font-size: 11px; margin-bottom: 2px;">{{ gpu.name }}</div>
                  <a-row :gutter="12">
                    <a-col :span="12">
                      <div style="font-size: 10px; color: #666;">Utilization: {{ gpu.utilGpu }}%</div>
                      <a-progress :percent="gpu.utilGpu" size="small" stroke-color="#13c2c2" />
                    </a-col>
                    <a-col :span="12">
                      <div style="font-size: 10px; color: #666;">VRAM: {{ gpu.memUsed }}MB</div>
                      <a-progress :percent="Math.round((gpu.memUsed / gpu.memTotal) * 100)" size="small" stroke-color="#722ed1" />
                    </a-col>
                  </a-row>
                </div>
              </div>
              <a-empty v-if="!gpus.length" :image="false" description="NVIDIA SMI not available" />
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <!-- PROCESSES TAB -->
      <a-tab-pane key="processes" tab="Processes">
        <template #tab><span><BarChartOutlined /> Processes</span></template>
        
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
            :dataSource="displayData" :columns="[
              { title: 'PID', dataIndex: 'pid', width: 100 },
              { title: 'Name', dataIndex: 'name' },
              { title: 'CPU', dataIndex: 'cpu', width: 100 },
              { title: 'MEM', dataIndex: 'mem', width: 100 },
              { title: 'VRAM', dataIndex: 'gpuMem', width: 100 },
              { title: 'User', dataIndex: 'user', width: 100 },
              { title: '', key: 'action', width: 80, fixed: 'right' }
            ]" 
            size="small" :pagination="viewMode === 'list' ? { pageSize: 50 } : false" rowKey="pid" :scroll="{ y: 'calc(100vh - 400px)' }"
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
      </a-tab-pane>

      <!-- MEMORY MAP TAB -->
      <a-tab-pane key="memmap" tab="Memory Map">
        <template #tab><span><AppstoreOutlined /> Memory Map</span></template>
        <div class="mem-container">
          <div v-for="g in memoryVisualizationData" :key="g.name" 
               class="mem-block"
               :style="{ 
                 backgroundColor: getMemColor(g.mem),
                 flexGrow: g.mem,
                 flexBasis: Math.max(10, g.mem * 2) + '%',
                 minHeight: Math.max(60, Math.sqrt(g.mem) * 30) + 'px'
               }">
            <a-tooltip>
              <template #title>
                App: {{ g.name }}<br/>
                Total Mem: {{ g.mem.toFixed(2) }}%<br/>
                Instances: {{ g.count }}<br/>
                PIDs: {{ g.pids.slice(0, 5).join(', ') }}{{ g.pids.length > 5 ? '...' : '' }}
              </template>
              <div class="mem-block-content">
                <div class="mem-name">{{ g.name }}</div>
                <div class="mem-value">{{ g.mem.toFixed(1) }}%</div>
                <div v-if="g.count > 1" class="mem-count">x{{ g.count }}</div>
              </div>
            </a-tooltip>
          </div>
        </div>
      </a-tab-pane>

    </a-tabs>
  </div>
</template>

<style scoped>
.stat-card-row { border-radius: 8px; overflow: hidden; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
.core-grid-full { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; padding: 10px; max-height: 250px; overflow-y: auto; }
.core-item-full { display: flex; align-items: center; background: #fafafa; padding: 4px 12px; border-radius: 4px; }
.core-label { font-size: 11px; color: #666; min-width: 50px; }
.core-val { font-size: 11px; font-family: monospace; min-width: 40px; text-align: right; }
.io-stat { display: flex; flex-direction: column; }
.io-label { font-size: 11px; color: #999; margin-bottom: 2px; }
.io-values { display: flex; gap: 12px; font-size: 13px; font-weight: bold; font-family: monospace; }
.stat-row { display: flex; justify-content: space-between; font-size: 11px; margin-top: 4px; }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 12px; }

.mem-container {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  background: #fff;
  padding: 16px;
  border-radius: 8px;
  min-height: calc(100vh - 200px);
  align-content: flex-start;
}

.mem-block {
  height: 80px;
  border-radius: 4px;
  padding: 8px;
  color: white;
  transition: all 0.3s;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  overflow: hidden;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.mem-block:hover {
  transform: scale(1.05);
  box-shadow: 0 4px 8px rgba(0,0,0,0.2);
  z-index: 10;
}

.mem-block-content {
  text-align: center;
  width: 100%;
}

.mem-name {
  font-weight: bold;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mem-value {
  font-size: 14px;
  font-family: 'JetBrains Mono', monospace;
}

.mem-count {
  font-size: 11px;
  opacity: 0.8;
  margin-top: 2px;
}

:deep(.ant-progress-circle-path) { stroke-linecap: round; }
</style>
