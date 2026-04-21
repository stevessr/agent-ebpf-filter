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

interface IOSpeed {
  name: string;
  readSpeed: number;
  writeSpeed: number;
}

interface GlobalStats {
  cpuTotal: number; cpuCores: number[];
  cpuCoresDetailed: { index: number; usage: number; type: number }[];
  memTotal: number; memUsed: number; memPercent: number;
  netInterfaces: IOSpeed[];
  diskDevices: IOSpeed[];
  totalNetRecv: number; totalNetSent: number;
  totalDiskRead: number; totalDiskWrite: number;
}

const activeTab = ref('dashboard');
const processes = ref<ProcessInfo[]>([]);
const gpus = ref<GPUStatus[]>([]);
const systemStats = ref<GlobalStats>({
  cpuTotal: 0, cpuCores: [], cpuCoresDetailed: [], memTotal: 0, memUsed: 0, memPercent: 0,
  netInterfaces: [], diskDevices: [],
  totalNetRecv: 0, totalNetSent: 0, totalDiskRead: 0, totalDiskWrite: 0
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
let lastIO: { 
  networks: Record<string, {r: number, s: number}>;
  disks: Record<string, {r: number, w: number}>;
  time: number 
} | null = null;

const formatBytes = (bytes: number, decimals = 2) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(decimals)) + ' ' + sizes[i];
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
      
      const newNetSpeeds: IOSpeed[] = [];
      const newDiskSpeeds: IOSpeed[] = [];
      
      if (lastIO && decoded.io) {
        const dt = (now - lastIO.time) / 1000;
        
        (decoded.io.networks || []).forEach((n: any) => {
          const prev = lastIO?.networks[n.name];
          if (prev) {
            newNetSpeeds.push({
              name: n.name,
              readSpeed: (Number(n.recvBytes) - prev.r) / dt,
              writeSpeed: (Number(n.sentBytes) - prev.s) / dt
            });
          }
        });

        (decoded.io.disks || []).forEach((d: any) => {
          const prev = lastIO?.disks[d.name];
          if (prev) {
            newDiskSpeeds.push({
              name: d.name,
              readSpeed: (Number(d.readBytes) - prev.r) / dt,
              writeSpeed: (Number(d.writeBytes) - prev.w) / dt
            });
          }
        });
      }

      if (decoded.io) {
        const nets: Record<string, {r: number, s: number}> = {};
        (decoded.io.networks || []).forEach((n: any) => nets[n.name] = {r: Number(n.recvBytes), s: Number(n.sentBytes)});
        const dsks: Record<string, {r: number, w: number}> = {};
        (decoded.io.disks || []).forEach((d: any) => dsks[d.name] = {r: Number(d.readBytes), w: Number(d.writeBytes)});
        
        lastIO = { networks: nets, disks: dsks, time: now };
        
        systemStats.value.netInterfaces = newNetSpeeds.filter(s => s.readSpeed > 0 || s.writeSpeed > 0);
        systemStats.value.diskDevices = newDiskSpeeds.filter(s => s.readSpeed > 0 || s.writeSpeed > 0);
        
        // Use totals from proto if available, or sum up
        let totalNetR = 0, totalNetS = 0, totalDiskR = 0, totalDiskW = 0;
        newNetSpeeds.forEach(s => { totalNetR += s.readSpeed; totalNetS += s.writeSpeed; });
        newDiskSpeeds.forEach(s => { totalDiskR += s.readSpeed; totalDiskW += s.writeSpeed; });
        
        systemStats.value.totalNetRecv = totalNetR;
        systemStats.value.totalNetSent = totalNetS;
        systemStats.value.totalDiskRead = totalDiskR;
        systemStats.value.totalDiskWrite = totalDiskW;
      }

      if (decoded.cpu) {
        systemStats.value.cpuTotal = decoded.cpu.total || 0;
        systemStats.value.cpuCores = (decoded.cpu.cores as number[]) || [];
        systemStats.value.cpuCoresDetailed = (decoded.cpu.coreDetails || []).map((c: any) => ({
          index: c.index,
          usage: c.usage || 0,
          type: c.type
        }));
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
    if (p.mem <= 0.05) return;
    if (!groups[p.name]) groups[p.name] = { name: p.name, mem: 0, count: 0, pids: [] };
    groups[p.name].mem += p.mem;
    groups[p.name].count += 1;
    groups[p.name].pids.push(p.pid);
  });
  return Object.values(groups).sort((a, b) => b.mem - a.mem).slice(0, 50);
});

const getMemColor = (percent: number) => {
  if (percent > 20) return '#cf1322';
  if (percent > 10) return '#d4380d';
  if (percent > 5) return '#d46b08';
  if (percent > 2) return '#1d39c4';
  return '#389e0d';
};

const showGroupDetails = ref(false);
const selectedGroup = ref<{ name: string; pids: number[] } | null>(null);

const openGroupDetails = (name: string, pids: number[]) => {
  selectedGroup.value = { name, pids };
  showGroupDetails.value = true;
};

const selectedGroupProcesses = computed(() => {
  if (!selectedGroup.value) return [];
  return processes.value.filter(p => selectedGroup.value?.pids.includes(p.pid));
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
    <a-tabs v-model:activeKey="activeTab" type="card" class="monitor-tabs">
      
      <!-- HEALTH TAB -->
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
                <div v-for="core in systemStats.cpuCoresDetailed" :key="core.index" class="core-item-full">
                  <span class="core-label">
                    <a-tag :color="core.type === 0 ? 'blue' : 'green'" size="small" style="font-size: 9px; line-height: 16px; height: 16px; padding: 0 4px; margin-right: 4px;">
                      {{ core.type === 0 ? 'P' : 'E' }}
                    </a-tag>
                    #{{ core.index }}
                  </span>
                  <div style="flex: 1; margin: 0 10px;">
                    <a-progress :percent="Math.round(core.usage)" size="small" :showInfo="false" :stroke-color="core.type === 0 ? '#1890ff' : '#52c41a'" />
                  </div>
                  <span class="core-val">{{ core.usage.toFixed(1) }}%</span>
                </div>
              </div>
            </a-card>
          </a-col>
        </a-row>

        <!-- Memory & I/O Row -->
        <a-row style="margin-bottom: 16px;">
          <a-col :span="24">
            <a-card size="small" class="stat-card-row" title="Memory & Interface I/O">
              <template #extra><PieChartOutlined /></template>
              <div style="display: flex; gap: 24px; padding: 10px;">
                <div style="flex: 0 0 300px;">
                  <div style="margin-bottom: 15px;">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 4px; font-size: 13px;">
                      <span>RAM Usage</span>
                      <span style="font-weight: bold;">{{ systemStats.memPercent.toFixed(1) }}%</span>
                    </div>
                    <a-progress :percent="Math.round(systemStats.memPercent)" stroke-color="#1890ff" status="active" />
                    <div style="font-size: 12px; color: #999; margin-top: 4px;">
                      {{ formatBytes(systemStats.memUsed) }} / {{ formatBytes(systemStats.memTotal) }}
                    </div>
                  </div>
                  <div style="border-top: 1px solid #f0f0f0; padding-top: 15px;">
                    <div style="display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 8px;">
                      <span>Total Network:</span>
                      <span style="color: #52c41a">↓ {{ formatBytes(systemStats.totalNetRecv) }}/s</span>
                      <span style="color: #1890ff">↑ {{ formatBytes(systemStats.totalNetSent) }}/s</span>
                    </div>
                    <div style="display: flex; justify-content: space-between; font-size: 12px;">
                      <span>Total Disk:</span>
                      <span style="color: #faad14">R: {{ formatBytes(systemStats.totalDiskRead) }}/s</span>
                      <span style="color: #ff4d4f">W: {{ formatBytes(systemStats.totalDiskWrite) }}/s</span>
                    </div>
                  </div>
                </div>
                
                <div style="flex: 1; display: grid; grid-template-columns: 1fr 1fr; gap: 16px; border-left: 1px solid #f0f0f0; padding-left: 24px;">
                  <!-- Net Detail -->
                  <div>
                    <div style="font-size: 11px; color: #999; margin-bottom: 8px; font-weight: bold;">NETWORK INTERFACES</div>
                    <div style="max-height: 120px; overflow-y: auto;">
                      <div v-for="s in systemStats.netInterfaces" :key="s.name" class="io-row">
                        <span class="io-name">{{ s.name }}</span>
                        <span class="io-val-in">↓{{ formatBytes(s.readSpeed, 0) }}</span>
                        <span class="io-val-out">↑{{ formatBytes(s.writeSpeed, 0) }}</span>
                      </div>
                      <div v-if="!systemStats.netInterfaces.length" style="font-size: 11px; color: #ccc;">No active traffic</div>
                    </div>
                  </div>
                  <!-- Disk Detail -->
                  <div>
                    <div style="font-size: 11px; color: #999; margin-bottom: 8px; font-weight: bold;">DISK DEVICES</div>
                    <div style="max-height: 120px; overflow-y: auto;">
                      <div v-for="s in systemStats.diskDevices" :key="s.name" class="io-row">
                        <span class="io-name">{{ s.name }}</span>
                        <span class="io-val-read">R:{{ formatBytes(s.readSpeed, 0) }}</span>
                        <span class="io-val-write">W:{{ formatBytes(s.writeSpeed, 0) }}</span>
                      </div>
                      <div v-if="!systemStats.diskDevices.length" style="font-size: 11px; color: #ccc;">No active I/O</div>
                    </div>
                  </div>
                </div>
              </div>
            </a-card>
          </a-col>
        </a-row>

        <!-- GPU Row -->
        <a-row>
          <a-col :span="24">
            <a-card size="small" class="stat-card-row" :title="gpus.length ? 'GPU Acceleration Status' : 'No GPU'">
              <template #extra><DeploymentUnitOutlined /></template>
              <div style="display: flex; flex-wrap: wrap; gap: 16px; padding: 10px;">
                <div v-for="gpu in gpus" :key="gpu.index" class="gpu-row-item">
                  <div style="flex-shrink: 0; min-width: 80px;">
                    <a-tag color="volcano" style="display: block; text-align: center;">{{ gpu.temp }}°C</a-tag>
                    <div style="font-size: 10px; color: #999; text-align: center; margin-top: 4px;">GPU {{ gpu.index }}</div>
                  </div>
                  <div style="flex: 1;">
                    <div style="font-size: 12px; font-weight: bold; margin-bottom: 6px;">{{ gpu.name }}</div>
                    <div style="display: flex; gap: 20px;">
                      <div style="flex: 1;">
                        <div style="font-size: 10px; color: #666; display: flex; justify-content: space-between;">
                          <span>Utilization</span><span>{{ gpu.utilGpu }}%</span>
                        </div>
                        <a-progress :percent="gpu.utilGpu" size="small" stroke-color="#13c2c2" :showInfo="false" />
                      </div>
                      <div style="flex: 1;">
                        <div style="font-size: 10px; color: #666; display: flex; justify-content: space-between;">
                          <span>VRAM</span><span>{{ gpu.memUsed }} / {{ gpu.memTotal }} MB</span>
                        </div>
                        <a-progress :percent="Math.round((gpu.memUsed / gpu.memTotal) * 100)" size="small" stroke-color="#722ed1" :showInfo="false" />
                      </div>
                    </div>
                  </div>
                </div>
                <a-empty v-if="!gpus.length" :image="false" description="No NVIDIA hardware detected via nvidia-smi" style="width: 100%" />
              </div>
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
               @click="openGroupDetails(g.name, g.pids)"
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
                Instances: {{ g.count }} (Click for details)
              </template>
              <div class="mem-block-content">
                <div class="mem-name">{{ g.name }}</div>
                <div class="mem-value">{{ g.mem.toFixed(1) }}%</div>
                <div v-if="g.count > 1" class="mem-count">x{{ g.count }}</div>
              </div>
            </a-tooltip>
          </div>
        </div>

        <a-modal v-model:open="showGroupDetails" :title="'Instances: ' + selectedGroup?.name" :footer="null" width="800px">
          <a-table :dataSource="selectedGroupProcesses" :columns="[{ title: 'PID', dataIndex: 'pid', width: 100 }, { title: 'CPU %', dataIndex: 'cpu', width: 100 }, { title: 'MEM %', dataIndex: 'mem', width: 100 }, { title: 'VRAM', dataIndex: 'gpuMem', width: 100 }, { title: 'User', dataIndex: 'user' }, { title: '', key: 'action', width: 80 }]" size="small" :pagination="{ pageSize: 10 }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'cpu'">{{ record.cpu.toFixed(1) }}%</template>
              <template v-if="column.key === 'mem'">{{ record.mem.toFixed(1) }}%</template>
              <template v-if="column.key === 'gpuMem'">{{ record.gpuMem > 0 ? record.gpuMem + 'MB' : '-' }}</template>
              <template v-if="column.key === 'action'"><a-button type="link" size="small" @click="addToRules(record)">Track</a-button></template>
            </template>
          </a-table>
        </a-modal>
      </a-tab-pane>

    </a-tabs>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 12px; }
.stat-card-row { border-radius: 8px; overflow: hidden; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
.core-grid-full { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; padding: 10px; max-height: 250px; overflow-y: auto; }
.core-item-full { display: flex; align-items: center; background: #fafafa; padding: 4px 12px; border-radius: 4px; border: 1px solid #f0f0f0; }
.core-label { font-size: 11px; color: #999; min-width: 45px; }
.core-val { font-size: 11px; font-family: monospace; min-width: 40px; text-align: right; color: #52c41a; font-weight: bold; }
.io-row { display: flex; justify-content: space-between; font-size: 12px; padding: 4px 8px; background: #f9f9f9; margin-bottom: 4px; border-radius: 3px; font-family: monospace; }
.io-name { font-weight: bold; color: #555; overflow: hidden; text-overflow: ellipsis; max-width: 80px; }
.io-val-in { color: #52c41a; } .io-val-out { color: #1890ff; }
.io-val-read { color: #faad14; } .io-val-write { color: #ff4d4f; }
.gpu-row-item { flex: 1; min-width: 400px; display: flex; align-items: center; gap: 16px; background: #fafafa; padding: 12px; border-radius: 6px; border: 1px solid #f0f0f0; }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 12px; }

.mem-container { display: flex; flex-wrap: wrap; gap: 8px; background: #fff; padding: 16px; border-radius: 8px; min-height: calc(100vh - 200px); align-content: flex-start; }
.mem-block { height: 80px; border-radius: 4px; padding: 8px; color: white; transition: all 0.3s; cursor: pointer; display: flex; flex-direction: column; justify-content: center; align-items: center; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.mem-block:hover { transform: scale(1.02); box-shadow: 0 4px 8px rgba(0,0,0,0.2); z-index: 10; }
.mem-name { font-weight: bold; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; width: 100%; text-align: center; }
.mem-value { font-size: 14px; font-family: 'JetBrains Mono', monospace; }
.mem-count { font-size: 11px; opacity: 0.8; }
</style>
