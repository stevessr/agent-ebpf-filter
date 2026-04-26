<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, defineAsyncComponent } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { 
  SearchOutlined, DeploymentUnitOutlined,
  DashboardOutlined,
  AppstoreOutlined,
  ApiOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

interface GPUStatus {
  index: number; name: string; utilGpu: number; utilMem: number;
  memTotal: number; memUsed: number; temp: number;
}

interface ProcessInfo {
  pid: number; ppid: number; name: string; cpu: number; mem: number;
  user: string; gpuMem: number; gpuId: number; 
  cmdline: string; createTime: number;
  minorFaults: number;
  majorFaults: number;
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
  memCached: number; memBuffers: number; memShared: number;
  zramUsed: number; zramTotal: number;
  netInterfaces: IOSpeed[];
  diskDevices: IOSpeed[];
  totalNetRecv: number; totalNetSent: number;
  totalDiskRead: number; totalDiskWrite: number;
  faults: FaultInfo;
}

interface FaultInfo {
  pageFaults: number;
  majorFaults: number;
  minorFaults: number;
  pageFaultRate: number;
  majorFaultRate: number;
  minorFaultRate: number;
  swapIn: number;
  swapOut: number;
  swapInRate: number;
  swapOutRate: number;
}

const route = useRoute();
const router = useRouter();
const activeTab = ref((route.params.tab as string) || 'dashboard');

interface SystemdService {
  unit: string;
  load: string;
  active: string;
  sub: string;
  description: string;
}

const systemdServices = ref<SystemdService[]>([]);
const systemdLoading = ref(false);
const systemdSearch = ref('');
const systemdScope = ref<'system' | 'user'>('system');

// SENSORS STATE
const sensorData = ref<any[]>([]);
const sensorsLoading = ref(false);
const cameras = ref<string[]>([]);
const selectedCamera = ref<string | null>(null);
const cameraSnapshotUrl = ref('');
const cameraLiveMode = ref(false);
const cameraLoading = ref(false);
const cameraFrameUrl = ref('');
const sensorInterval = ref(2000);
const sensorHistory = ref<Record<string, {time: number, value: number}[]>>({});
const sensorVisibility = ref<Record<string, boolean>>({});

let cameraWs: WebSocket | null = null;
let sensorWs: WebSocket | null = null;

const connectSensorsWS = () => {
  if (sensorWs) sensorWs.close();
  const wsUrl = buildWebSocketUrl(`/ws/sensors?interval=${sensorInterval.value}`);
  sensorWs = new WebSocket(wsUrl);
  sensorWs.onmessage = (e) => {
    const data = JSON.parse(e.data);
    sensorData.value = (data || []).map((s: any) => ({
      ...s,
      sensorKey: s.sensorKey || s.label
    }));
    
    const now = Date.now();
    sensorData.value.forEach(s => {
      const key = s.sensorKey;
      if (!sensorHistory.value[key]) {
        sensorHistory.value[key] = [];
        sensorVisibility.value[key] = true;
      }
      sensorHistory.value[key].push({ time: now, value: s.temperature });
      if (sensorHistory.value[key].length > 60) sensorHistory.value[key].shift();
    });
  };
};

const sensorChartOptions = computed(() => ({
  chart: { 
    id: 'sensor-chart', 
    animations: { enabled: false }, 
    toolbar: { show: false },
    background: 'transparent'
  },
  xaxis: { 
    type: 'datetime' as const,
    labels: { show: true, style: { fontSize: '10px' }, datetimeUTC: false },
    axisBorder: { show: false }
  },
  yaxis: { 
    title: { text: 'Temp (°C)', style: { fontSize: '12px' } },
    min: 0,
    max: 110,
    tickAmount: 5
  },
  stroke: { width: 2, curve: 'smooth' as const },
  colors: ['#1890ff', '#52c41a', '#faad14', '#ff4d4f', '#722ed1', '#13c2c2', '#eb2f96'],
  legend: { show: false },
  grid: { borderColor: '#f0f0f0' },
  tooltip: { x: { format: 'HH:mm:ss' } }
}));

const sensorChartSeries = computed(() => {
  return Object.keys(sensorHistory.value)
    .filter(key => sensorVisibility.value[key])
    .map(key => ({
      name: key,
      data: sensorHistory.value[key].map(d => ({ x: d.time, y: d.value }))
    }));
});

const toggleAllSensors = (visible: boolean) => {
  Object.keys(sensorVisibility.value).forEach(k => sensorVisibility.value[k] = visible);
};

const showLogsModal = ref(false);
const activeLogUnit = ref('');
const serviceLogs = ref('');
const logsLoading = ref(false);

const fetchSystemdLogs = async (unit: string) => {
  activeLogUnit.value = unit;
  showLogsModal.value = true;
  logsLoading.value = true;
  serviceLogs.value = '';
  try {
    const res = await axios.get(`/system/systemd/logs?unit=${unit}&lines=200&scope=${systemdScope.value}`);
    serviceLogs.value = res.data.logs;
  } catch (err) {
    message.error('Failed to fetch logs');
  } finally {
    logsLoading.value = false;
  }
};

const fetchSystemdServices = async () => {
  systemdLoading.value = true;
  try {
    const res = await axios.get(`/system/systemd?scope=${systemdScope.value}`);
    systemdServices.value = res.data;
  } catch (err) {
    message.error(`Failed to fetch ${systemdScope.value} systemd services`);
  } finally {
    systemdLoading.value = false;
  }
};

const controlSystemdService = async (unit: string, action: string) => {
  try {
    await axios.post('/system/systemd/control', { unit, action, scope: systemdScope.value });
    message.success(`${systemdScope.value.toUpperCase()} service ${unit} ${action} command sent`);
    void fetchSystemdServices();
  } catch (err: any) {
    message.error(err?.response?.data?.error || `Failed to ${action} service`);
  }
};

const filteredSystemdServices = computed(() => {
  if (!systemdSearch.value.trim()) return systemdServices.value;
  const q = systemdSearch.value.toLowerCase();
  return systemdServices.value.filter(s => 
    s.unit.toLowerCase().includes(q) || 
    s.description.toLowerCase().includes(q)
  );
});

const systemdColumns = [
  { title: 'Unit', dataIndex: 'unit', key: 'unit', sorter: (a: any, b: any) => a.unit.localeCompare(b.unit) },
  { 
    title: 'Active', 
    dataIndex: 'active', 
    key: 'active', 
    width: 120,
    filters: [
      { text: 'active', value: 'active' },
      { text: 'inactive', value: 'inactive' },
      { text: 'failed', value: 'failed' },
      { text: 'activating', value: 'activating' },
      { text: 'deactivating', value: 'deactivating' },
    ],
    onFilter: (value: string, record: any) => record.active === value,
  },
  { 
    title: 'Sub', 
    dataIndex: 'sub', 
    key: 'sub', 
    width: 140,
    filters: [
      { text: 'running', value: 'running' },
      { text: 'exited', value: 'exited' },
      { text: 'dead', value: 'dead' },
      { text: 'waiting', value: 'waiting' },
    ],
    onFilter: (value: string, record: any) => record.sub === value,
  },
  { title: 'Description', dataIndex: 'description', key: 'description', ellipsis: true },
  { title: 'Action', key: 'action', width: 220, align: 'right' },
];

const handleTabChange = (key: any) => {
  activeTab.value = key;
  void router.replace({ name: 'Monitor', params: { tab: key } });
};

watch(() => route.params.tab, (newTab) => {
  if (newTab && newTab !== activeTab.value) {
    activeTab.value = newTab as string;
  } else if (!newTab && activeTab.value !== 'dashboard') {
    activeTab.value = 'dashboard';
  }
});

// TRACING STATE
const trackedCommsNames = ref<string[]>([]);
const trackedLoading = ref(false);

const fetchTrackedComms = async () => {
  trackedLoading.value = true;
  try {
    const res = await axios.get('/system/tracked-comms');
    trackedCommsNames.value = res.data;
  } catch (err) {} finally {
    trackedLoading.value = false;
  }
};

const sendProcessSignal = async (pid: number, signal: string) => {
  try {
    await axios.post('/system/process/signal', { pid, signal });
    message.success(`Signal ${signal.toUpperCase()} sent to PID ${pid}`);
  } catch (err: any) {
    message.error(err?.response?.data?.error || `Failed to send ${signal}`);
  }
};

const trackedProcesses = computed(() => {
  if (trackedCommsNames.value.length === 0) return [];
  return processes.value.filter(p => trackedCommsNames.value.includes(p.name));
});

const trackedColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 80, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right', sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right', sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'User', dataIndex: 'user', key: 'user', width: 100 },
  { title: 'Action', key: 'action', width: 260, align: 'right' },
];

const fetchSensors = async () => {
  sensorsLoading.value = true;
  try {
    const res = await axios.get('/system/sensors');
    sensorData.value = (res.data || []).map((s: any) => ({
      ...s,
      sensorKey: s.sensorKey || s.label
    }));
  } catch (err) {} finally {
    sensorsLoading.value = false;
  }
};

const fetchCameras = async () => {
  try {
    const res = await axios.get('/system/cameras');
    cameras.value = res.data;
    if (res.data.length > 0 && !selectedCamera.value) {
      selectedCamera.value = res.data[0];
    }
  } catch (err) {}
};

const connectCameraWS = () => {
  if (!selectedCamera.value) return;
  if (cameraWs) cameraWs.close();
  cameraLoading.value = true;
  const wsUrl = buildWebSocketUrl(`/ws/camera?device=${encodeURIComponent(selectedCamera.value)}`);
  cameraWs = new WebSocket(wsUrl);
  cameraWs.binaryType = 'arraybuffer';
  cameraWs.onopen = () => cameraLoading.value = false;
  cameraWs.onmessage = (e) => {
    if (typeof e.data !== 'string') {
      const blob = new Blob([e.data], { type: 'image/jpeg' });
      const url = URL.createObjectURL(blob);
      if (cameraFrameUrl.value) URL.revokeObjectURL(cameraFrameUrl.value);
      cameraFrameUrl.value = url;
    }
  };
  cameraWs.onclose = () => { if (cameraLiveMode.value) setTimeout(connectCameraWS, 2000); };
};

const stopCameraWS = () => {
  if (cameraWs) { cameraWs.onclose = null; cameraWs.close(); cameraWs = null; }
  if (cameraFrameUrl.value) { URL.revokeObjectURL(cameraFrameUrl.value); cameraFrameUrl.value = ''; }
};

const refreshCamera = async () => {
  if (!selectedCamera.value) return;
  if (cameraLiveMode.value) { connectCameraWS(); return; }
  cameraLoading.value = true;
  try { cameraSnapshotUrl.value = `/system/camera/snapshot?device=${encodeURIComponent(selectedCamera.value)}&t=${Date.now()}`; } catch (err) {} finally { cameraLoading.value = false; }
};

const cameraStreamUrl = computed(() => {
  if (!selectedCamera.value) return '';
  return cameraLiveMode.value ? cameraFrameUrl.value : cameraSnapshotUrl.value;
});

const sensorColumns = [
  { title: 'Label', dataIndex: 'label', key: 'label' },
  { title: 'Temperature', dataIndex: 'temperature', key: 'temperature', align: 'right' },
  { title: 'Key', dataIndex: 'sensorKey', key: 'sensorKey', width: 100 },
];

watch(activeTab, (newTab) => {
  if (newTab === 'systemd' && systemdServices.value.length === 0) {
    void fetchSystemdServices();
  } else if (newTab === 'sensors') {
    void fetchSensors();
    void fetchCameras();
    connectSensorsWS();
  } else if (newTab === 'tracing') {
    void fetchTrackedComms();
  } else {
    if (sensorWs) { sensorWs.close(); sensorWs = null; }
    cameraLiveMode.value = false;
  }
});

watch(sensorInterval, () => {
  if (activeTab.value === 'sensors') connectSensorsWS();
});

watch(cameraLiveMode, (val) => {
  if (val) connectCameraWS(); else stopCameraWS();
});

const processes = ref<ProcessInfo[]>([]);
const gpus = ref<GPUStatus[]>([]);
const systemStats = ref<GlobalStats>({
  cpuTotal: 0, cpuCores: [], cpuCoresDetailed: [], memTotal: 0, memUsed: 0, memPercent: 0,
  memCached: 0, memBuffers: 0, memShared: 0, zramUsed: 0, zramTotal: 0,
  netInterfaces: [], diskDevices: [],
  totalNetRecv: 0, totalNetSent: 0, totalDiskRead: 0, totalDiskWrite: 0,
  faults: { pageFaults: 0, majorFaults: 0, minorFaults: 0, pageFaultRate: 0, majorFaultRate: 0, minorFaultRate: 0, swapIn: 0, swapOut: 0, swapInRate: 0, swapOutRate: 0 }
});
const loading = ref(false);
const tags = ref<string[]>([]);

let ws: WebSocket | null = null;
let reconnectTimer: any = null;
let shouldReconnect = true;

const refreshInterval = ref(2000);

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();
  const socket = new WebSocket(buildWebSocketUrl(`/ws/system?interval=${refreshInterval.value}`));
  ws = socket;
  socket.binaryType = 'arraybuffer';
  socket.onopen = () => { loading.value = false; };
  socket.onmessage = (me) => {
    const s = pb.SystemStats.decode(new Uint8Array(me.data));
    processes.value = (s.processes || []) as any;
    gpus.value = (s.gpus || []) as any;
    systemStats.value = {
      cpuTotal: s.cpu?.total || 0,
      cpuCores: s.cpu?.cores || [],
      cpuCoresDetailed: (s.cpu?.coreDetails || []) as any,
      memTotal: Number(s.memory?.total || 0),
      memUsed: Number(s.memory?.used || 0),
      memPercent: s.memory?.percent || 0,
      memCached: Number(s.memory?.cached || 0),
      memBuffers: Number(s.memory?.buffers || 0),
      memShared: Number(s.memory?.shared || 0),
      zramUsed: Number(s.memory?.zramUsed || 0),
      zramTotal: Number(s.memory?.zramTotal || 0),
      netInterfaces: (s.io?.networks || []).map(n => ({ name: n.name || '', readSpeed: Number(n.recvBytes || 0), writeSpeed: Number(n.sentBytes || 0) })),
      diskDevices: (s.io?.disks || []).map(d => ({ name: d.name || '', readSpeed: Number(d.readBytes || 0), writeSpeed: Number(d.writeBytes || 0) })),
      totalNetRecv: Number(s.io?.totalNetRecvBytes || 0),
      totalNetSent: Number(s.io?.totalNetSentBytes || 0),
      totalDiskRead: Number(s.io?.totalReadBytes || 0),
      totalDiskWrite: Number(s.io?.totalWriteBytes || 0),
      faults: (s.faults || {}) as any
    };
  };
  socket.onclose = () => { if (shouldReconnect) reconnectTimer = setTimeout(connectWebSocket, 3000); };
};

onMounted(() => {
  loading.value = true;
  axios.get('/config/tags').then(res => tags.value = res.data);
  connectWebSocket();
  if (activeTab.value === 'systemd') void fetchSystemdServices();
  else if (activeTab.value === 'sensors') { void fetchSensors(); void fetchCameras(); connectSensorsWS(); }
  else if (activeTab.value === 'tracing') void fetchTrackedComms();
});

onUnmounted(() => {
  shouldReconnect = false;
  stopCameraWS();
  if (sensorWs) sensorWs.close();
  if (ws) ws.close();
  if (reconnectTimer) clearTimeout(reconnectTimer);
});
</script>

<template>
  <div style="background: #f0f2f5; padding: 20px; min-height: 100%;">
    <a-tabs :activeKey="activeTab" @change="handleTabChange" type="card" class="monitor-tabs">
      <a-tab-pane key="dashboard" tab="Health">
        <template #tab><span><DashboardOutlined /> Health</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">Health Dashboard Content</div>
      </a-tab-pane>

      <a-tab-pane key="processes" tab="Processes">
        <template #tab><span><AppstoreOutlined /> Processes</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <a-table :dataSource="processes" :columns="trackedColumns" size="small" rowKey="pid" :scroll="{ y: 'calc(100vh - 400px)' }" />
        </div>
      </a-tab-pane>

      <a-tab-pane key="systemd" tab="Systemd">
        <template #tab><span><DeploymentUnitOutlined /> Systemd</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap;">
            <a-space>
              <a-radio-group v-model:value="systemdScope" button-style="solid" size="small">
                <a-radio-button value="system">System</a-radio-button>
                <a-radio-button value="user">User</a-radio-button>
              </a-radio-group>
              <a-input-search v-model:value="systemdSearch" placeholder="Filter services..." style="width: 260px" size="small" allow-clear />
            </a-space>
            <a-button type="primary" size="small" :loading="systemdLoading" @click="fetchSystemdServices">Refresh</a-button>
          </div>
          <a-table :dataSource="filteredSystemdServices" :columns="systemdColumns" row-key="unit" size="small" :pagination="{ pageSize: 50, showSizeChanger: true }" :loading="systemdLoading" :scroll="{ x: 800 }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'active'"><a-tag :color="record.active === 'active' ? 'success' : 'default'">{{ record.active }}</a-tag></template>
              <template v-else-if="column.key === 'sub'"><a-tag :color="record.sub === 'running' ? 'blue' : 'default'">{{ record.sub }}</a-tag></template>
              <template v-if="column.key === 'action'">
                <a-space>
                  <a-button type="link" size="small" @click="fetchSystemdLogs(record.unit)">Logs</a-button>
                  <a-button v-if="record.active !== 'active'" type="link" size="small" @click="controlSystemdService(record.unit, 'start')">Start</a-button>
                  <a-button v-if="record.active === 'active'" type="link" size="small" danger @click="controlSystemdService(record.unit, 'stop')">Stop</a-button>
                  <a-button type="link" size="small" @click="controlSystemdService(record.unit, 'restart')">Restart</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
        </div>
      </a-tab-pane>

      <a-tab-pane key="sensors" tab="Sensors">
        <template #tab><span><ApiOutlined /> Sensors</span></template>
        <div style="display: flex; flex-direction: column; gap: 16px;">
          <a-row :gutter="16">
            <a-col :span="16">
              <a-card title="Real-time Temperature Chart" size="small">
                <template #extra>
                  <a-space>
                    <span style="font-size: 12px; color: #888;">Interval:</span>
                    <a-select v-model:value="sensorInterval" size="small" style="width: 80px">
                      <a-select-option :value="500">500ms</a-select-option>
                      <a-select-option :value="1000">1s</a-select-option>
                      <a-select-option :value="2000">2s</a-select-option>
                      <a-select-option :value="5000">5s</a-select-option>
                      <a-select-option :value="10000">10s</a-select-option>
                    </a-select>
                    <a-button-group size="small">
                      <a-button @click="toggleAllSensors(true)">All</a-button>
                      <a-button @click="toggleAllSensors(false)">None</a-button>
                    </a-button-group>
                  </a-space>
                </template>
                <div style="height: 350px; padding: 10px;">
                  <VueApexCharts type="line" height="330" :options="sensorChartOptions" :series="sensorChartSeries" />
                </div>
              </a-card>
            </a-col>
            <a-col :span="8">
              <a-card title="Hardware Sensors" size="small" :bodyStyle="{ padding: '0px' }">
                <a-table :dataSource="sensorData" :columns="sensorColumns" size="small" :pagination="false" rowKey="sensorKey" :scroll="{ y: 310 }">
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'label'">
                      <a-checkbox v-model:checked="sensorVisibility[record.sensorKey]">
                        <span style="font-size: 12px;">{{ record.label }}</span>
                      </a-checkbox>
                    </template>
                    <template v-if="column.key === 'temperature'">
                      <span :style="{ color: record.temperature > 75 ? '#ff4d4f' : record.temperature > 60 ? '#faad14' : '#52c41a', fontWeight: 'bold' }">
                        {{ record.temperature.toFixed(1) }}°C
                      </span>
                    </template>
                  </template>
                </a-table>
              </a-card>
            </a-col>
          </a-row>

          <a-card title="Camera Live Feed" size="small">
            <template #extra>
              <a-space>
                <a-tag v-if="cameraLiveMode" color="red">LIVE</a-tag>
                <span style="font-size: 12px; color: #888;">Live Stream:</span>
                <a-switch v-model:checked="cameraLiveMode" size="small" />
              </a-space>
            </template>
            <div style="display: flex; gap: 16px;">
              <div style="flex: 1;">
                <div style="margin-bottom: 12px;">
                  <a-select v-model:value="selectedCamera" style="width: 100%" placeholder="Select Camera" @change="refreshCamera">
                    <a-select-option v-for="cam in cameras" :key="cam" :value="cam">{{ cam }}</a-select-option>
                  </a-select>
                </div>
                <div style="background: #000; border-radius: 4px; overflow: hidden; position: relative; aspect-ratio: 16/9; display: flex; align-items: center; justify-content: center;">
                  <img v-if="cameraStreamUrl" :src="cameraStreamUrl" style="width: 100%; height: 100%; object-fit: contain;" />
                  <a-empty v-else description="No camera stream" :image-style="{ height: '60px' }" />
                  <div v-if="cameraLoading && !cameraLiveMode" style="position: absolute; inset: 0; background: rgba(0,0,0,0.3); display: flex; align-items: center; justify-content: center;"><a-spin /></div>
                </div>
              </div>
              
            </div>
            <div v-if="!cameraLiveMode" style="width: 200px; display: flex; flex-direction: column; gap: 12px; justify-content: center;">
                <a-card size="small" title="Snapshot"><a-button block size="small" @click="refreshCamera" :disabled="!selectedCamera">Manual Capture</a-button></a-card>
              </div>
          </a-card>
        </div>
      </a-tab-pane>

      <a-tab-pane key="tracing" tab="Tracing">
        <template #tab><span><SearchOutlined /> Tracing</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
            <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
               <span style="font-size: 14px; font-weight: bold;">Tracked:</span>
               <a-tag v-for="name in trackedCommsNames" :key="name" color="blue">{{ name }}</a-tag>
            </div>
            <a-button size="small" @click="fetchTrackedComms">Refresh Config</a-button>
          </div>
          <a-table :dataSource="trackedProcesses" :columns="trackedColumns" row-key="pid" size="small" :pagination="{ pageSize: 20 }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'cpu'"><span :style="{ color: record.cpu > 50 ? '#ff4d4f' : 'inherit' }">{{ record.cpu.toFixed(1) }}%</span></template>
              <template v-else-if="column.key === 'mem'"><span>{{ record.mem.toFixed(1) }}%</span></template>
              <template v-if="column.key === 'action'">
                <a-space>
                  <a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'stop')">Suspend</a-button>
                  <a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'cont')">Resume</a-button>
                  <a-button type="link" size="small" danger @click="sendProcessSignal(record.pid, 'kill')">Kill</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
        </div>
      </a-tab-pane>
    </a-tabs>

    <a-modal v-model:open="showLogsModal" :title="`Logs: ${activeLogUnit}`" width="1000px" :footer="null">
      <div style="background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 13px; max-height: 600px; overflow-y: auto; white-space: pre-wrap; word-break: break-all;">
        <a-spin :spinning="logsLoading"><div v-if="serviceLogs">{{ serviceLogs }}</div><a-empty v-else-if="!logsLoading" description="No logs found" /></a-spin>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 0; }
.mono { font-family: monospace; }
</style>
