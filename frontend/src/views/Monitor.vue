<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, defineAsyncComponent } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { 
  SearchOutlined, DeploymentUnitOutlined,
  DashboardOutlined,
  AppstoreOutlined,
  ApiOutlined, AudioOutlined, VideoCameraOutlined,
  SoundOutlined, AudioMutedOutlined, WarningOutlined
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
  pageFaults: number; majorFaults: number; minorFaults: number;
  pageFaultRate: number; majorFaultRate: number; minorFaultRate: number;
  swapIn: number; swapOut: number; swapInRate: number; swapOutRate: number;
}

interface ByteScale {
  divisor: number;
  unit: 'B' | 'KB' | 'MB' | 'GB' | 'TB';
  precision: number;
}

const route = useRoute();
const router = useRouter();
const activeTab = ref((route.params.tab as string) || 'dashboard');
const sensorSubTab = ref((route.params.subtab as string) || 'hardware');
const healthTab = ref('cpu');

interface SystemdService {
  unit: string; load: string; active: string; sub: string; description: string;
}

const systemdServices = ref<SystemdService[]>([]);
const systemdLoading = ref(false);
const systemdSearch = ref('');
const systemdScope = ref<'system' | 'user'>('system');

// SENSORS STATE
const sensorData = ref<any[]>([]);
const fanData = ref<any[]>([]);
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

const getSensorCategory = (key: string, label: string) => {
  const s = (key + label).toLowerCase();
  if (s.includes('nvme')) return 'Storage (NVMe)';
  if (s.includes('acpi')) return 'System (ACPI)';
  if (s.includes('coretemp') || s.includes('package_id') || s.includes('cpu') || s.includes('k10temp')) return 'Processor (CPU)';
  if (s.includes('gpu') || s.includes('amdgpu') || s.includes('nvidia')) return 'Graphics (GPU)';
  if (s.includes('bat')) return 'Power (Battery)';
  if (s.includes('wifi') || s.includes('iwl') || s.includes('ath')) return 'Network (Wi-Fi)';
  if (s.includes('fan')) return 'Cooling (Fan)';
  if (s.includes('sata') || s.includes('sda') || s.includes('sdb')) return 'Storage (SATA)';
  return 'Other Sensors';
};

const connectSensorsWS = () => {
  if (sensorWs) sensorWs.close();
  const wsUrl = buildWebSocketUrl(`/ws/sensors?interval=${sensorInterval.value}`);
  sensorWs = new WebSocket(wsUrl);
  sensorWs.onmessage = (e) => {
    const res = JSON.parse(e.data);
    const temps = res.temperatures || [];
    fanData.value = res.fans || [];
    sensorData.value = temps.map((s: any) => ({ ...s, sensorKey: s.sensorKey || s.label, category: getSensorCategory(s.sensorKey || '', s.label || '') }));
    const now = Date.now();
    sensorData.value.forEach(s => {
      const key = s.sensorKey;
      if (!sensorHistory.value[key]) {
        sensorHistory.value[key] = [];
        if (sensorVisibility.value[key] === undefined) sensorVisibility.value[key] = true;
      }
      sensorHistory.value[key].push({ time: now, value: s.temperature });
      if (sensorHistory.value[key].length > 60) sensorHistory.value[key].shift();
    });
  };
};

const sensorChartOptions = computed(() => ({
  chart: { id: 'sensor-chart', animations: { enabled: false }, toolbar: { show: false }, background: 'transparent' },
  xaxis: { type: 'datetime' as const, labels: { show: true, style: { fontSize: '10px' }, datetimeUTC: false }, axisBorder: { show: false } },
  yaxis: { title: { text: 'Temp (°C)', style: { fontSize: '12px' } }, min: 0, max: (maxVal: number) => Math.max(70, maxVal * 1.1), tickAmount: 5 },
  stroke: { width: 2, curve: 'smooth' as const },
  colors: ['#1890ff', '#52c41a', '#faad14', '#ff4d4f', '#722ed1', '#13c2c2', '#eb2f96'],
  legend: { show: false },
  grid: { borderColor: '#f0f0f0' },
  tooltip: { x: { format: 'HH:mm:ss' } }
}));

const groupedSensors = computed(() => {
  const groups: Record<string, any[]> = {};
  sensorData.value.forEach(s => {
    if (!groups[s.category]) groups[s.category] = [];
    groups[s.category].push(s);
  });
  return groups;
});

const toggleAllSensors = (visible: boolean) => {
  Object.keys(sensorVisibility.value).forEach(k => sensorVisibility.value[k] = visible);
};

// MICROPHONE STATE
const micDevices = ref<{id: string, name: string}[]>([]);
const selectedMic = ref('default');
const micLiveMode = ref(false);
const micListenBrowser = ref(false);
const micVolume = ref(0);
const micCanvasRef = ref<HTMLCanvasElement | null>(null);
let micWs: WebSocket | null = null;
let micCanvasCtx: CanvasRenderingContext2D | null = null;
let micAnimationId: number | null = null;
const micDataBuffer = new Int16Array(1024);
let audioCtx: AudioContext | null = null;
let nextStartTime = 0;

const connectMicWS = () => {
  if (micWs) micWs.close();
  const wsUrl = buildWebSocketUrl(`/ws/microphone?device=${encodeURIComponent(selectedMic.value)}`);
  micWs = new WebSocket(wsUrl);
  micWs.binaryType = 'arraybuffer';
  micWs.onmessage = async (e) => {
    const samples = new Int16Array(e.data);
    let sum = 0;
    for (let i = 0; i < samples.length; i++) {
      sum += Math.abs(samples[i]);
      if (i < micDataBuffer.length) micDataBuffer[i] = samples[i];
    }
    micVolume.value = Math.min(100, (sum / samples.length) / 327.68 * 2.5);
    if (micListenBrowser.value) {
      if (!audioCtx) audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)({ sampleRate: 16000 });
      if (audioCtx.state === 'suspended') await audioCtx.resume();
      const floatSamples = new Float32Array(samples.length);
      for (let i = 0; i < samples.length; i++) floatSamples[i] = samples[i] / 32768;
      const buffer = audioCtx.createBuffer(1, floatSamples.length, 16000);
      buffer.copyToChannel(floatSamples, 0);
      const source = audioCtx.createBufferSource();
      source.buffer = buffer;
      source.connect(audioCtx.destination);
      const currentTime = audioCtx.currentTime;
      if (nextStartTime < currentTime) nextStartTime = currentTime;
      source.start(nextStartTime);
      nextStartTime += buffer.duration;
    }
  };
};

const drawMicWaveform = () => {
  if (!micCanvasRef.value) return;
  const canvas = micCanvasRef.value;
  if (!micCanvasCtx) micCanvasCtx = canvas.getContext('2d');
  const ctx = micCanvasCtx!;
  const w = canvas.width; const h = canvas.height;
  ctx.clearRect(0, 0, w, h); ctx.beginPath(); ctx.strokeStyle = '#1890ff'; ctx.lineWidth = 2;
  const step = w / micDataBuffer.length;
  ctx.moveTo(0, h / 2);
  for (let i = 0; i < micDataBuffer.length; i++) {
    const val = (micDataBuffer[i] / 32768) * (h / 2) * 2;
    ctx.lineTo(i * step, h / 2 + val);
  }
  ctx.stroke();
  micAnimationId = requestAnimationFrame(drawMicWaveform);
};

const stopMicWS = () => {
  if (micWs) { micWs.close(); micWs = null; }
  if (micAnimationId) { cancelAnimationFrame(micAnimationId); micAnimationId = null; }
  if (audioCtx) { audioCtx.close(); audioCtx = null; }
  micVolume.value = 0; nextStartTime = 0;
};

const fetchMicrophones = async () => {
  try {
    const res = await axios.get('/system/microphones');
    micDevices.value = res.data;
    if (res.data.length > 0 && selectedMic.value === 'default') selectedMic.value = res.data[0].id;
  } catch (err) {}
};

watch(micLiveMode, (val) => { if (val) { connectMicWS(); drawMicWaveform(); } else stopMicWS(); });
watch(selectedMic, () => { if (micLiveMode.value) connectMicWS(); });

const showLogsModal = ref(false);
const activeLogUnit = ref('');
const serviceLogs = ref('');
const logsLoading = ref(false);

const fetchSystemdLogs = async (unit: string) => {
  activeLogUnit.value = unit; showLogsModal.value = true; logsLoading.value = true; serviceLogs.value = '';
  try {
    const res = await axios.get(`/system/systemd/logs?unit=${unit}&lines=200&scope=${systemdScope.value}`);
    serviceLogs.value = res.data.logs;
  } catch (err) { message.error('Failed to fetch logs'); } finally { logsLoading.value = false; }
};

const fetchSystemdServices = async () => {
  systemdLoading.value = true;
  try {
    const res = await axios.get(`/system/systemd?scope=${systemdScope.value}`);
    systemdServices.value = res.data;
  } catch (err) { message.error(`Failed to fetch ${systemdScope.value} systemd services`); } finally { systemdLoading.value = false; }
};

const controlSystemdService = async (unit: string, action: string) => {
  try {
    await axios.post('/system/systemd/control', { unit, action, scope: systemdScope.value });
    message.success(`${systemdScope.value.toUpperCase()} service ${unit} ${action} command sent`);
    void fetchSystemdServices();
  } catch (err: any) { message.error(err?.response?.data?.error || `Failed to ${action} service`); }
};

const filteredSystemdServices = computed(() => {
  if (!systemdSearch.value.trim()) return systemdServices.value;
  const q = systemdSearch.value.toLowerCase();
  return systemdServices.value.filter(s => s.unit.toLowerCase().includes(q) || s.description.toLowerCase().includes(q));
});

const systemdColumns = [
  { title: 'Unit', dataIndex: 'unit', key: 'unit', sorter: (a: any, b: any) => a.unit.localeCompare(b.unit) },
  { title: 'Active', dataIndex: 'active', key: 'active', width: 120, filters: [ { text: 'active', value: 'active' }, { text: 'inactive', value: 'inactive' } ], onFilter: (v: string, r: any) => r.active === v },
  { title: 'Sub', dataIndex: 'sub', key: 'sub', width: 140 },
  { title: 'Description', dataIndex: 'description', key: 'description', ellipsis: true },
  { title: 'Action', key: 'action', width: 220, align: 'right' },
];

const handleTabChange = (key: any) => {
  activeTab.value = key;
  void router.replace({ name: 'Monitor', params: { tab: key, subtab: key === 'sensors' ? sensorSubTab.value : undefined } });
};

const handleSubTabChange = (key: any) => {
  sensorSubTab.value = key;
  void router.replace({ name: 'Monitor', params: { tab: 'sensors', subtab: key } });
};

// TRACING STATE
const trackedCommsNames = ref<string[]>([]);
const trackedLoading = ref(false);

const fetchTrackedComms = async () => {
  trackedLoading.value = true;
  try {
    const res = await axios.get('/system/tracked-comms');
    trackedCommsNames.value = res.data;
  } catch (err) {} finally { trackedLoading.value = false; }
};

const sendProcessSignal = async (pid: number, signal: string) => {
  try {
    await axios.post('/system/process/signal', { pid, signal });
    message.success(`Signal ${signal.toUpperCase()} sent to PID ${pid}`);
  } catch (err: any) { message.error(err?.response?.data?.error || `Failed to send ${signal}`); }
};

const processes = ref<ProcessInfo[]>([]);
const trackedProcesses = computed(() => {
  if (trackedCommsNames.value.length === 0) return [];
  return processes.value.filter(p => trackedCommsNames.value.includes(p.name));
});

const trackedColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 80, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right' },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right' },
  { title: 'User', dataIndex: 'user', key: 'user', width: 100 },
  { title: 'Action', key: 'action', width: 260, align: 'right' },
];

const fetchSensors = async () => {
  sensorsLoading.value = true;
  try {
    const res = await axios.get('/system/sensors');
    sensorData.value = (res.data.temperatures || []).map((s: any) => ({ ...s, sensorKey: s.sensorKey || s.label, category: getSensorCategory(s.sensorKey || '', s.label || '') }));
    fanData.value = res.data.fans || [];
  } catch (err) {} finally { sensorsLoading.value = false; }
};

const fetchCameras = async () => {
  try {
    const res = await axios.get('/system/cameras');
    cameras.value = res.data;
    if (res.data.length > 0 && !selectedCamera.value) selectedCamera.value = res.data[0];
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

const getCoreTypeColor = (type: number) => type === pb.CPUInfo.Core.Type.PERFORMANCE ? '#1890ff' : '#52c41a';
const getCoreTypeName = (type: number) => type === pb.CPUInfo.Core.Type.PERFORMANCE ? 'P-Core' : 'E-Core';

const byteScale = (bytes: number): ByteScale => {
  const units: ('B' | 'KB' | 'MB' | 'GB' | 'TB')[] = ['B', 'KB', 'MB', 'GB', 'TB'];
  let divisor = 1; let i = 0;
  while (bytes >= 1024 && i < units.length - 1) { divisor *= 1024; i++; }
  return { divisor, unit: units[i], precision: i > 2 ? 2 : 1 };
};

const formatBytesWithUnit = (bytes: number) => {
  const { divisor, unit, precision } = byteScale(bytes);
  return `${(bytes / divisor).toFixed(precision)} ${unit}`;
};

watch(activeTab, (newTab) => {
  if (newTab === 'systemd' && systemdServices.value.length === 0) void fetchSystemdServices();
  else if (newTab === 'sensors') { 
    if (sensorSubTab.value === 'hardware') { void fetchSensors(); connectSensorsWS(); }
    else if (sensorSubTab.value === 'camera') void fetchCameras();
    else if (sensorSubTab.value === 'mic') void fetchMicrophones();
  } else if (newTab === 'tracing') void fetchTrackedComms();
  else {
    if (sensorWs) { sensorWs.close(); sensorWs = null; }
    cameraLiveMode.value = false; micLiveMode.value = false;
  }
});

watch(sensorSubTab, (newSub) => {
  if (activeTab.value !== 'sensors') return;
  if (newSub === 'hardware') { void fetchSensors(); connectSensorsWS(); }
  else { 
    if (sensorWs) { sensorWs.close(); sensorWs = null; }
    if (newSub === 'camera') void fetchCameras();
    else if (newSub === 'mic') void fetchMicrophones();
  }
});

watch(sensorInterval, () => { if (activeTab.value === 'sensors' && sensorSubTab.value === 'hardware') connectSensorsWS(); });
watch(cameraLiveMode, (val) => { if (val) connectCameraWS(); else stopCameraWS(); });

const cpuView = ref<'overall' | 'cores'>('cores');
const statsHistory = ref<{
  cpu: { time: number; value: number }[];
  mem: { time: number; value: number }[];
  netRecv: { time: number; value: number }[];
  netSent: { time: number; value: number }[];
  diskRead: { time: number; value: number }[];
  diskWrite: { time: number; value: number }[];
  faults: { time: number; value: number }[];
}>({
  cpu: [], mem: [], netRecv: [], netSent: [], diskRead: [], diskWrite: [], faults: []
});

const showHistoryModal = ref(false);
const historyModalTitle = ref('');
const historySeries = ref<any[]>([]);
const historyChartOptions = computed(() => ({
  chart: { id: 'history-chart', animations: { enabled: true }, toolbar: { show: true } },
  xaxis: { type: 'datetime' as const, labels: { datetimeUTC: false } },
  stroke: { width: 2, curve: 'smooth' as const },
  tooltip: { x: { format: 'HH:mm:ss' } }
}));

const openHistoryChart = (title: string, data: { time: number; value: number }[], name: string, color?: string) => {
  historyModalTitle.value = title;
  historySeries.value = [{ name, data: data.map(d => ({ x: d.time, y: d.value })) }];
  if (color) historySeries.value[0].color = color;
  showHistoryModal.value = true;
};

const topFaultProcesses = computed(() => {
  return [...processes.value].sort((a, b) => (b.majorFaults + b.minorFaults) - (a.majorFaults + a.minorFaults)).slice(0, 5);
});

const connectWebSocket = () => {
  if (!shouldReconnect) return;
  if (ws) ws.close();
  const socket = new WebSocket(buildWebSocketUrl(`/ws/system?interval=2000`));
  ws = socket; socket.binaryType = 'arraybuffer';
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
      swapUsed: Number(s.memory?.swapUsed || 0),
      swapTotal: Number(s.memory?.swapTotal || 0),
      netInterfaces: (s.io?.networks || []).map(n => ({ name: n.name || '', readSpeed: Number(n.recvBytes || 0), writeSpeed: Number(n.sentBytes || 0) })),
      diskDevices: (s.io?.disks || []).map(d => ({ name: d.name || '', readSpeed: Number(d.readBytes || 0), writeSpeed: Number(d.writeBytes || 0) })),
      totalNetRecv: Number(s.io?.totalNetRecvBytes || 0),
      totalNetSent: Number(s.io?.totalNetSentBytes || 0),
      totalDiskRead: Number(s.io?.totalReadBytes || 0),
      totalDiskWrite: Number(s.io?.totalWriteBytes || 0),
      faults: (s.faults || {}) as any
    };

    const now = Date.now();
    statsHistory.value.cpu.push({ time: now, value: systemStats.value.cpuTotal });
    statsHistory.value.mem.push({ time: now, value: systemStats.value.memPercent });
    statsHistory.value.netRecv.push({ time: now, value: systemStats.value.totalNetRecv });
    statsHistory.value.netSent.push({ time: now, value: systemStats.value.totalNetSent });
    statsHistory.value.diskRead.push({ time: now, value: systemStats.value.totalDiskRead });
    statsHistory.value.diskWrite.push({ time: now, value: systemStats.value.totalDiskWrite });
    statsHistory.value.faults.push({ time: now, value: systemStats.value.faults.pageFaultRate });

    Object.values(statsHistory.value).forEach(h => { if (h.length > 60) h.shift(); });
  };
  socket.onclose = () => { if (shouldReconnect) reconnectTimer = setTimeout(connectWebSocket, 3000); };
};

onMounted(() => {
  loading.value = true;
  axios.get('/config/tags').then(res => tags.value = res.data);
  connectWebSocket();
  if (activeTab.value === 'systemd') void fetchSystemdServices();
  else if (activeTab.value === 'sensors') { 
    if (sensorSubTab.value === 'hardware') { void fetchSensors(); connectSensorsWS(); }
    else if (sensorSubTab.value === 'camera') void fetchCameras();
    else if (sensorSubTab.value === 'mic') void fetchMicrophones();
  }
  else if (activeTab.value === 'tracing') void fetchTrackedComms();
});

onUnmounted(() => {
  shouldReconnect = false; stopCameraWS(); stopMicWS();
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
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <a-tabs v-model:activeKey="healthTab" size="small" type="line" style="margin-top: -12px;">
            <a-tab-pane key="cpu" tab="CPU">
              <div style="padding-top: 16px;">
                <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center;">
                  <a-radio-group v-model:value="cpuView" button-style="solid" size="small">
                    <a-radio-button value="overall">Overall</a-radio-button>
                    <a-radio-button value="cores">Per Core</a-radio-button>
                  </a-radio-group>
                  <a-button type="link" size="small" @click="openHistoryChart('CPU Usage History', statsHistory.cpu, 'Total CPU', '#1890ff')">History Chart</a-button>
                </div>
                
                <div v-if="cpuView === 'overall'" style="background: #fafafa; padding: 24px; border-radius: 8px; text-align: center; border: 1px solid #f0f0f0;">
                   <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="180" :stroke-color="systemStats.cpuTotal > 80 ? '#ff4d4f' : '#1890ff'" />
                   <div style="margin-top: 16px; font-size: 18px; font-weight: bold;">System CPU Usage: {{ systemStats.cpuTotal.toFixed(1) }}%</div>
                </div>
                
                <div v-else style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px;">
                  <div v-for="core in systemStats.cpuCoresDetailed" :key="core.index" style="padding: 12px; border: 1px solid #f0f0f0; border-radius: 8px; text-align: center; background: #fafafa;">
                     <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                        <span style="font-family: monospace; font-size: 11px; font-weight: bold;">#{{ core.index }}</span>
                        <a-tag :color="getCoreTypeColor(core.type)" style="font-size: 9px; padding: 0 4px; line-height: 16px;">{{ getCoreTypeName(core.type) }}</a-tag>
                     </div>
                     <a-progress type="dashboard" :percent="Math.round(core.usage)" :width="70" :stroke-color="core.usage > 80 ? '#ff4d4f' : getCoreTypeColor(core.type)" />
                  </div>
                </div>
              </div>
            </a-tab-pane>
            <a-tab-pane key="mem" tab="Memory">
               <a-row :gutter="16" style="padding-top: 16px;">
                  <a-col :span="12">
                    <a-card title="Physical Memory" size="small" :bordered="false" style="background: #fafafa;">
                       <template #extra>
                          <a-button type="link" size="small" @click="openHistoryChart('Memory Usage History', statsHistory.mem, 'Mem %', '#52c41a')">History</a-button>
                       </template>
                       <a-statistic title="Overall Usage" :value="systemStats.memPercent" suffix="%" :precision="1" @click="openHistoryChart('Memory Usage History', statsHistory.mem, 'Mem %', '#52c41a')" style="cursor: pointer;" />
                       <div style="margin-top: 16px; display: grid; gap: 8px;">
                          <div style="display: flex; justify-content: space-between;"><span>Total:</span><b>{{ formatBytesWithUnit(systemStats.memTotal) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #1890ff;"><span>Used:</span><b>{{ formatBytesWithUnit(systemStats.memUsed) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #52c41a;"><span>Cached:</span><b>{{ formatBytesWithUnit(systemStats.memCached) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #faad14;"><span>Buffers:</span><b>{{ formatBytesWithUnit(systemStats.memBuffers) }}</b></div>
                       </div>
                    </a-card>
                  </a-col>
                  <a-col :span="12">
                    <a-card title="Swap / ZRAM" size="small" :bordered="false" style="background: #fafafa;">
                       <div style="display: grid; gap: 16px;">
                          <div v-if="systemStats.swapTotal > 0">
                             <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                                <span style="font-size: 12px; color: #888;">System Swap</span>
                                <span style="font-weight: bold;">{{ systemStats.swapTotal > 0 ? ((systemStats.swapUsed / systemStats.swapTotal) * 100).toFixed(1) : 0 }}%</span>
                             </div>
                             <a-progress :percent="systemStats.swapTotal > 0 ? Math.round((systemStats.swapUsed / systemStats.swapTotal) * 100) : 0" size="small" stroke-color="#722ed1" />
                             <div style="display: flex; justify-content: space-between; font-size: 11px; margin-top: 2px;">
                                <span>Used: {{ formatBytesWithUnit(systemStats.swapUsed) }}</span>
                                <span>Total: {{ formatBytesWithUnit(systemStats.swapTotal) }}</span>
                             </div>
                          </div>
                          <div v-if="systemStats.zramTotal > 0">
                             <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                                <span style="font-size: 12px; color: #888;">ZRAM (Compressed)</span>
                                <span style="font-weight: bold;">{{ systemStats.zramTotal > 0 ? ((systemStats.zramUsed / systemStats.zramTotal) * 100).toFixed(1) : 0 }}%</span>
                             </div>
                             <a-progress :percent="systemStats.zramTotal > 0 ? Math.round((systemStats.zramUsed / systemStats.zramTotal) * 100) : 0" size="small" stroke-color="#13c2c2" />
                             <div style="display: flex; justify-content: space-between; font-size: 11px; margin-top: 2px;">
                                <span>Compressed: {{ formatBytesWithUnit(systemStats.zramUsed) }}</span>
                                <span>Original: {{ formatBytesWithUnit(systemStats.zramTotal) }}</span>
                             </div>
                          </div>
                          <a-empty v-if="systemStats.swapTotal === 0 && systemStats.zramTotal === 0" description="No Swap/ZRAM detected" />
                       </div>
                    </a-card>
                  </a-col>
               </a-row>
            </a-tab-pane>
            <a-tab-pane key="io" tab="I/O">
               <a-row :gutter="16" style="padding-top: 16px;">
                  <a-col :span="12">
                    <a-card title="Network Activity" size="small" :bordered="false" style="background: #fafafa;">
                       <template #extra>
                          <a-space>
                            <a-button type="link" size="small" @click="openHistoryChart('Network Recv History', statsHistory.netRecv, 'Recv', '#52c41a')">Recv</a-button>
                            <a-button type="link" size="small" @click="openHistoryChart('Network Sent History', statsHistory.netSent, 'Sent', '#1890ff')">Sent</a-button>
                          </a-space>
                       </template>
                       <div v-for="iface in systemStats.netInterfaces" :key="iface.name" style="margin-bottom: 12px; padding: 8px; border-radius: 4px; background: #fff; border: 1px solid #f0f0f0;">
                          <div style="font-family: monospace; font-weight: bold; margin-bottom: 4px; display: flex; justify-content: space-between;">
                             <span>{{ iface.name }}</span>
                             <span style="font-size: 10px; color: #888;">(Live Speed)</span>
                          </div>
                          <div style="display: flex; gap: 16px; font-size: 12px;">
                             <span style="color: #52c41a; flex: 1; cursor: pointer;" @click="openHistoryChart(`${iface.name} Recv Speed`, statsHistory.netRecv, 'Bytes/s', '#52c41a')">↓ {{ formatBytesWithUnit(iface.readSpeed) }}/s</span>
                             <span style="color: #1890ff; flex: 1; cursor: pointer;" @click="openHistoryChart(`${iface.name} Sent Speed`, statsHistory.netSent, 'Bytes/s', '#1890ff')">↑ {{ formatBytesWithUnit(iface.writeSpeed) }}/s</span>
                          </div>
                       </div>
                    </a-card>
                  </a-col>
                  <a-col :span="12">
                    <a-card title="Storage Activity" size="small" :bordered="false" style="background: #fafafa;">
                       <template #extra>
                          <a-space>
                            <a-button type="link" size="small" @click="openHistoryChart('Disk Read History', statsHistory.diskRead, 'Read', '#faad14')">Read</a-button>
                            <a-button type="link" size="small" @click="openHistoryChart('Disk Write History', statsHistory.diskWrite, 'Write', '#722ed1')">Write</a-button>
                          </a-space>
                       </template>
                       <div v-for="disk in systemStats.diskDevices" :key="disk.name" style="margin-bottom: 12px; padding: 8px; border-radius: 4px; background: #fff; border: 1px solid #f0f0f0;">
                          <div style="font-family: monospace; font-weight: bold; margin-bottom: 4px; display: flex; justify-content: space-between;">
                             <span>{{ disk.name }}</span>
                             <span style="font-size: 10px; color: #888;">(I/O Throughput)</span>
                          </div>
                          <div style="display: flex; gap: 16px; font-size: 12px;">
                             <span style="color: #faad14; flex: 1; cursor: pointer;" @click="openHistoryChart(`${disk.name} Read Speed`, statsHistory.diskRead, 'Bytes/s', '#faad14')">Read: {{ formatBytesWithUnit(disk.readSpeed) }}/s</span>
                             <span style="color: #722ed1; flex: 1; cursor: pointer;" @click="openHistoryChart(`${disk.name} Write Speed`, statsHistory.diskWrite, 'Bytes/s', '#722ed1')">Write: {{ formatBytesWithUnit(disk.writeSpeed) }}/s</span>
                          </div>
                       </div>
                    </a-card>
                  </a-col>
               </a-row>
            </a-tab-pane>
            <a-tab-pane key="faults" tab="Faults">
               <a-row :gutter="16" style="padding-top: 16px;">
                  <a-col :span="24">
                    <a-card title="System Page Faults" size="small" :bordered="false" style="background: #fafafa;">
                      <template #extra>
                        <a-button type="link" size="small" @click="openHistoryChart('Page Fault Rate History', statsHistory.faults, 'Faults/s', '#ff4d4f')">History Chart</a-button>
                      </template>
                      <a-row :gutter="16">
                         <a-col :span="6"><a-statistic title="Soft Faults (Minor)" :value="systemStats.faults.minorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Minor Fault History', statsHistory.faults, 'Faults/s', '#52c41a')" style="cursor: pointer;" /></a-col>
                         <a-col :span="6"><a-statistic title="Hard Faults (Major)" :value="systemStats.faults.majorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Major Fault History', statsHistory.faults, 'Faults/s', '#ff4d4f')" style="cursor: pointer;" /></a-col>
                         <a-col :span="6"><a-statistic title="Swap-Out Rate" :value="systemStats.faults.swapOutRate" :precision="1" suffix="/s" /></a-col>
                         <a-col :span="6"><a-statistic title="Swap-In Rate" :value="systemStats.faults.swapInRate" :precision="1" suffix="/s" /></a-col>
                      </a-row>
                    </a-card>
                  </a-col>
                  <a-col :span="24" style="margin-top: 16px;">
                    <a-card title="Top Processes by Faults" size="small" :bordered="false" style="background: #fafafa;">
                      <a-table :dataSource="topFaultProcesses" :columns="[
                        { title: 'PID', dataIndex: 'pid', key: 'pid', width: 80 },
                        { title: 'Command', dataIndex: 'name', key: 'name' },
                        { title: 'Minor Faults', dataIndex: 'minorFaults', key: 'minorFaults', align: 'right' },
                        { title: 'Major Faults', dataIndex: 'majorFaults', key: 'majorFaults', align: 'right', customCell: (r) => ({ style: { color: r.majorFaults > 0 ? 'red' : 'inherit', fontWeight: r.majorFaults > 0 ? 'bold' : 'normal' } }) }
                      ]" size="small" :pagination="false" rowKey="pid" />
                    </a-card>
                  </a-col>
               </a-row>
            </a-tab-pane>
          </a-tabs>
        </div>
      </a-tab-pane>

      <a-tab-pane key="processes" tab="Processes"><template #tab><span><AppstoreOutlined /> Processes</span></template><div style="background: #fff; padding: 20px; border-radius: 4px;"><a-table :dataSource="processes" :columns="trackedColumns" size="small" rowKey="pid" :scroll="{ y: 'calc(100vh - 400px)' }" /></div></a-tab-pane>

      <a-tab-pane key="systemd" tab="Systemd">
        <template #tab><span><DeploymentUnitOutlined /> Systemd</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px; flex-wrap: wrap;">
            <a-space><a-radio-group v-model:value="systemdScope" button-style="solid" size="small"><a-radio-button value="system">System</a-radio-button><a-radio-button value="user">User</a-radio-button></a-radio-group><a-input-search v-model:value="systemdSearch" placeholder="Filter services..." style="width: 260px" size="small" allow-clear /></a-space>
            <a-button type="primary" size="small" :loading="systemdLoading" @click="fetchSystemdServices">Refresh</a-button>
          </div>
          <a-table :dataSource="filteredSystemdServices" :columns="systemdColumns" row-key="unit" size="small" :pagination="{ pageSize: 50, showSizeChanger: true }" :loading="systemdLoading" :scroll="{ x: 800 }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'active'"><a-tag :color="record.active === 'active' ? 'success' : 'default'">{{ record.active }}</a-tag></template>
              <template v-else-if="column.key === 'action'"><a-space><a-button type="link" size="small" @click="fetchSystemdLogs(record.unit)">Logs</a-button><a-button v-if="record.active !== 'active'" type="link" size="small" @click="controlSystemdService(record.unit, 'start')">Start</a-button><a-button v-if="record.active === 'active'" type="link" size="small" danger @click="controlSystemdService(record.unit, 'stop')">Stop</a-button><a-button type="link" size="small" @click="controlSystemdService(record.unit, 'restart')">Restart</a-button></a-space></template>
            </template>
          </a-table>
        </div>
      </a-tab-pane>

      <a-tab-pane key="sensors" tab="Sensors">
        <template #tab><span><ApiOutlined /> Sensors</span></template>
        <div style="background: #fff; padding: 16px; border-radius: 4px; border: 1px solid #f0f0f0;">
          <a-tabs :activeKey="sensorSubTab" @change="handleSubTabChange" size="small">
             <a-tab-pane key="hardware" tab="Hardware">
               <template #tab><span><ApiOutlined /> Hardware</span></template>
               <div style="display: flex; flex-direction: column; gap: 16px;">
                  <div style="display: flex; justify-content: flex-end; margin-bottom: 8px;">
                    <a-space><span style="font-size:12px;color:#888;">Interval:</span><a-select v-model:value="sensorInterval" size="small" style="width:70px"><a-select-option :value="1000">1s</a-select-option><a-select-option :value="2000">2s</a-select-option><a-select-option :value="5000">5s</a-select-option></a-select><a-button-group size="small"><a-button @click="toggleAllSensors(true)">All</a-button><a-button @click="toggleAllSensors(false)">None</a-button></a-button-group></a-space>
                  </div>
                  <div v-for="(sensors, category) in groupedSensors" :key="category" style="margin-bottom: 24px;">
                    <div style="font-weight: bold; font-size: 14px; color: #1890ff; border-bottom: 2px solid #e6f7ff; padding-bottom: 8px; margin-bottom: 12px;"><span>{{ category }}</span></div>
                    <a-row :gutter="16">
                      <a-col :span="16"><div style="height:260px; background: #fafafa; border-radius: 8px; padding: 8px;"><VueApexCharts type="line" height="240" :options="{ ...sensorChartOptions, chart: { ...sensorChartOptions.chart, id: `chart-${category.replace(/\s+/g, '-')}` } }" :series="sensors.filter(s => sensorVisibility[s.sensorKey]).map(s => ({ name: s.label || s.sensorKey, data: (sensorHistory[s.sensorKey] || []).map(d => ({ x: d.time, y: d.value })) }))" /></div></a-col>
                      <a-col :span="8">
                        <div style="max-height: 260px; overflow-y: auto; padding-right: 4px;">
                           <div v-for="s in sensors" :key="s.sensorKey" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; padding: 6px 10px; background: #fff; border: 1px solid #f0f0f0; border-radius: 4px;">
                              <a-checkbox v-model:checked="sensorVisibility[s.sensorKey]" style="display: flex; align-items: center; flex: 1; overflow: hidden;"><span style="font-size: 12px; margin-left: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 150px;" :title="s.label || s.sensorKey">{{ s.label || s.sensorKey }}</span></a-checkbox>
                              <span :style="{ color: s.temperature > 75 ? 'red' : s.temperature > 60 ? 'orange' : 'green', fontWeight:'bold', fontSize: '12px', marginLeft: '8px' }">{{ s.temperature.toFixed(1) }}°C</span>
                           </div>
                        </div>
                      </a-col>
                    </a-row>
                  </div>
                  <div v-if="fanData.length > 0">
                    <div style="font-weight: bold; font-size: 14px; color: #52c41a; border-bottom: 2px solid #f6ffed; padding-bottom: 8px; margin-bottom: 12px;">Cooling (Fans)</div>
                    <a-row :gutter="16"><a-col v-for="f in fanData" :key="f.label" :span="6"><a-card size="small" style="margin-bottom: 8px;"><a-statistic :title="f.label" :value="f.speed" suffix="RPM" /></a-card></a-col></a-row>
                  </div>
               </div>
             </a-tab-pane>
             <a-tab-pane key="camera" tab="Camera">
               <template #tab><span><VideoCameraOutlined /> Camera</span></template>
               <a-card title="Live Feed" size="small">
                 <template #extra><a-space><a-tag v-if="cameraLiveMode" color="red">LIVE</a-tag><span>Live:</span><a-switch v-model:checked="cameraLiveMode" size="small" /></a-space></template>
                 <div style="display:flex;gap:16px;">
                    <div style="flex:1;">
                      <a-select v-model:value="selectedCamera" style="width:100%;margin-bottom:12px;" @change="refreshCamera"><a-select-option v-for="cam in cameras" :key="cam" :value="cam">{{ cam }}</a-select-option></a-select>
                      <div style="background:#000;border-radius:4px;overflow:hidden;aspect-ratio:16/9;display:flex;align-items:center;justify-content:center;"><img v-if="cameraStreamUrl" :src="cameraStreamUrl" style="width:100%;height:100%;object-fit:contain;" /><a-empty v-else description="No stream" /></div>
                    </div>
                    <div v-if="!cameraLiveMode" style="width:200px;"><a-card size="small" title="Snapshot"><a-button block size="small" @click="refreshCamera">Capture</a-button></a-card></div>
                 </div>
               </a-card>
             </a-tab-pane>
             <a-tab-pane key="mic" tab="Microphone">
               <template #tab><span><AudioOutlined /> Microphone</span></template>
               <a-card title="Input Monitor" size="small">
                  <template #extra><a-space><a-tag v-if="micLiveMode" color="green">ON</a-tag><a-switch v-model:checked="micLiveMode" size="small" /></a-space></template>
                  <div style="display:flex;gap:24px;align-items:center;">
                     <div style="flex:1;"><div style="margin-bottom:8px;font-size:12px;color:#888;">Waveform</div><canvas ref="micCanvasRef" width="600" height="120" style="width:100%;height:120px;background:#fafafa;border-radius:4px;border:1px solid #f0f0f0;"></canvas></div>
                     <div style="width:280px;">
                        <div style="margin-bottom:16px;">
                           <div style="margin-bottom:8px;font-size:12px;color:#888;display:flex;justify-content:space-between;">
                             <span>Input Level</span>
                             <a-button type="link" size="small" style="padding:0;height:auto;" @click="micListenBrowser = !micListenBrowser">
                                <template #icon><SoundOutlined v-if="micListenBrowser" /><AudioMutedOutlined v-else /></template>
                                {{ micListenBrowser ? 'Listening' : 'Muted' }}
                             </a-button>
                           </div>
                           <a-progress :percent="micVolume" :show-info="false" :stroke-color="micVolume > 80 ? '#ff4d4f' : '#52c41a'" />
                        </div>
                        <div style="font-size: 12px; color: #888; margin-bottom: 4px;">Device</div>
                        <a-select v-model:value="selectedMic" style="width:100%" size="small" placeholder="Select Microphone">
                           <a-select-option v-for="dev in micDevices" :key="dev.id" :value="dev.id">{{ dev.name }}</a-select-option>
                        </a-select>
                     </div>
                  </div>
               </a-card>
             </a-tab-pane>
          </a-tabs>
        </div>
      </a-tab-pane>

      <a-tab-pane key="tracing" tab="Tracing"><template #tab><span><SearchOutlined /> Tracing</span></template><div style="background:#fff;padding:20px;border-radius:4px;"><div style="margin-bottom:16px;display:flex;justify-content:space-between;align-items:center;"><div style="display:flex;gap:8px;"><span style="font-weight:bold;">Tracked:</span><a-tag v-for="name in trackedCommsNames" :key="name" color="blue">{{ name }}</a-tag></div><a-button size="small" @click="fetchTrackedComms">Refresh</a-button></div><a-table :dataSource="trackedProcesses" :columns="trackedColumns" row-key="pid" size="small" :pagination="{pageSize:20}"><template #bodyCell="{ column, record }"><template v-if="column.key === 'cpu'"><span :style="{color: record.cpu > 50 ? 'red' : 'inherit'}">{{ record.cpu.toFixed(1) }}%</span></template><template v-if="column.key === 'action'"><a-space><a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'stop')">Suspend</a-button><a-button type="link" size="small" @click="sendProcessSignal(record.pid, 'cont')">Resume</a-button><a-button type="link" size="small" danger @click="sendProcessSignal(record.pid, 'kill')">Kill</a-button></a-space></template></template></a-table></div></a-tab-pane>
    </a-tabs>

    <a-modal v-model:open="showLogsModal" :title="`Logs: ${activeLogUnit}`" width="1000px" :footer="null"><div style="background:#1e1e1e;color:#d4d4d4;padding:12px;border-radius:4px;font-family:'JetBrains Mono',monospace;font-size:13px;max-height:600px;overflow-y:auto;white-space:pre-wrap;word-break:break-all;"><a-spin :spinning="logsLoading"><div v-if="serviceLogs">{{ serviceLogs }}</div><a-empty v-else-if="!logsLoading" description="No logs" /></a-spin></div></a-modal>

    <a-modal v-model:open="showHistoryModal" :title="historyModalTitle" width="800px" :footer="null">
       <div style="height: 400px; padding: 12px;">
          <VueApexCharts type="line" height="380" :options="historyChartOptions" :series="historySeries" />
       </div>
    </a-modal>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 0; }
</style>
