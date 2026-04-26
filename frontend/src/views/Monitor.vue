<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, defineAsyncComponent } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { 
  SearchOutlined, DeploymentUnitOutlined,
  DashboardOutlined,
  AppstoreOutlined,
  ApiOutlined, AudioOutlined, VideoCameraOutlined,
  SoundOutlined, AudioMutedOutlined, WarningOutlined,
  DatabaseOutlined
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
  user: string; gpuMem: number; gpuId: number; gpuUtil: number;
  cmdline: string; createTime: number;
  minorFaults: number; majorFaults: number;
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

const processSearch = ref('');
const processViewMode = ref<'flat' | 'tree' | 'merged'>('flat');
const showProcessMapsModal = ref(false);
const selectedProcessMaps = ref('');
const selectedProcessDetails = ref<any>(null);
const processMapsLoading = ref(false);

const processedProcesses = computed(() => {
  let list = processes.value.map(p => ({ ...p, key: p.pid }));
  
  if (processSearch.value) {
    const q = processSearch.value.toLowerCase();
    list = list.filter(p => p.name.toLowerCase().includes(q) || p.pid.toString().includes(q) || (p.cmdline && p.cmdline.toLowerCase().includes(q)));
  }

  if (processViewMode.value === 'merged') {
    const merged: Record<string, any> = {};
    list.forEach(p => {
      if (!merged[p.name]) {
        merged[p.name] = { ...p, key: `group-${p.name}`, children: [], instances: 0, totalCpu: 0, totalMem: 0, totalGpuMem: 0, totalGpuUtil: 0 };
      }
      merged[p.name].instances++;
      merged[p.name].totalCpu += p.cpu;
      merged[p.name].totalMem += p.mem;
      merged[p.name].totalGpuMem += p.gpuMem;
      merged[p.name].totalGpuUtil += p.gpuUtil;
      merged[p.name].children.push({ ...p, key: p.pid });
    });
    return Object.values(merged).map(m => ({
      ...m,
      cpu: m.totalCpu,
      mem: m.totalMem,
      gpuMem: m.totalGpuMem,
      gpuUtil: m.totalGpuUtil,
      name: `${m.name} (${m.instances})`
    })).sort((a, b) => b.cpu - a.cpu);
  }

  if (processViewMode.value === 'tree') {
    const map: Record<number, any> = {};
    list.forEach(p => map[p.pid] = { ...p, key: p.pid, children: [] });
    const roots: any[] = [];
    list.forEach(p => {
      if (map[p.ppid] && p.ppid !== p.pid) {
        map[p.ppid].children.push(map[p.pid]);
      } else {
        roots.push(map[p.pid]);
      }
    });
    // If filtering is active, tree view might look weird as parents might be filtered out.
    // Usually tree view is best without filtering or with a way to show matches and their parents.
    return roots;
  }

  return list;
});

const showProcessDetails = async (record: any) => {
  if (record.key && typeof record.key === 'string' && record.key.startsWith('group-')) return;
  selectedProcessDetails.value = record;
  showProcessMapsModal.value = true;
  processMapsLoading.value = true;
  selectedProcessMaps.value = '';
  try {
    const res = await axios.get(`/system/process/maps?pid=${record.pid}`);
    selectedProcessMaps.value = res.data.maps;
  } catch (err) {
    selectedProcessMaps.value = 'Failed to fetch maps. Process might have exited.';
  } finally {
    processMapsLoading.value = false;
  }
};

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
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 90, align: 'right', sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'Mem %', dataIndex: 'mem', key: 'mem', width: 90, align: 'right', sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'GPU Util', dataIndex: 'gpuUtil', key: 'gpuUtil', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuUtil - b.gpuUtil },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 90, align: 'right', sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
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
  while (bytes >= divisor * 1024 && i < units.length - 1) { divisor *= 1024; i++; }
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
const faultTopN = ref(5);
const mergeFaultProcesses = ref(true);

const statsHistory = ref<{
  cpu: { time: number; value: number }[];
  cores: Record<number, { time: number; value: number }[]>;
  mem: { time: number; value: number }[];
  memUsed: { time: number; value: number }[];
  memCached: { time: number; value: number }[];
  memBuffers: { time: number; value: number }[];
  swapUsage: { time: number; value: number }[];
  zramUsage: { time: number; value: number }[];
  netRecv: { time: number; value: number }[];
  netSent: { time: number; value: number }[];
  diskRead: { time: number; value: number }[];
  diskWrite: { time: number; value: number }[];
  faults: { time: number; value: number }[];
  swapIn: { time: number; value: number }[];
  swapOut: { time: number; value: number }[];
  netDevices: Record<string, { recv: { time: number; value: number }[]; sent: { time: number; value: number }[] }>;
  diskDevices: Record<string, { read: { time: number; value: number }[]; write: { time: number; value: number }[] }>;
}>({
  cpu: [], cores: {}, mem: [], memUsed: [], memCached: [], memBuffers: [], swapUsage: [], zramUsage: [],
  netRecv: [], netSent: [], diskRead: [], diskWrite: [], faults: [], swapIn: [], swapOut: [],
  netDevices: {}, diskDevices: {}
});

const showHistoryModal = ref(false);
const historyModalTitle = ref('');
const historySeries = ref<any[]>([]);
const historyChartOptions = computed(() => ({
  chart: { id: 'history-chart', animations: { enabled: true }, toolbar: { show: true }, background: 'transparent' },
  xaxis: { type: 'datetime' as const, labels: { datetimeUTC: false, style: { fontSize: '10px' } } },
  yaxis: {
    labels: {
      style: { fontSize: '10px' },
      formatter: (val: number) => {
        const t = historyModalTitle.value.toLowerCase();
        if (t.includes('speed') || t.includes('recv') || t.includes('sent') || t.includes('read') || t.includes('write') || t.includes('activity') || t.includes('memory usage') || t.includes('swap usage') || t.includes('zram usage')) {
           if (!t.includes('%')) return formatBytesWithUnit(val) + (t.includes('usage') ? '' : '/s');
        }
        if (t.includes('mem %') || t.includes('cpu') || t.includes('core') || (t.includes('usage') && t.includes('%'))) return val.toFixed(1) + '%';
        return val.toFixed(1);
      }
    }
  },
  stroke: { width: 2, curve: 'smooth' as const },
  tooltip: { x: { format: 'HH:mm:ss' } },
  legend: { position: 'top' as const, horizontalAlign: 'right' as const }
}));

const openHistoryChart = (title: string, datasets: { name: string; data: { time: number; value: number }[]; color?: string }[]) => {
  historyModalTitle.value = title;
  historySeries.value = datasets.map(ds => ({
    name: ds.name,
    data: ds.data.map(d => ({ x: d.time, y: d.value })),
    color: ds.color
  }));
  showHistoryModal.value = true;
};

const groupedNetInterfaces = computed(() => {
  const groups: Record<string, IOSpeed[]> = { 'Loopback': [], 'Wi-Fi': [], 'Ethernet': [], 'Virtual': [], 'Zerotier': [], 'Other': [] };
  systemStats.value.netInterfaces.forEach(iface => {
    const name = iface.name.toLowerCase();
    if (name === 'lo') groups['Loopback'].push(iface);
    else if (name.startsWith('wlan') || name.startsWith('wlp') || name.startsWith('wifi')) groups['Wi-Fi'].push(iface);
    else if (name.startsWith('eth') || name.startsWith('enp') || name.startsWith('eno') || name.startsWith('ens')) groups['Ethernet'].push(iface);
    else if (name.startsWith('veth') || name.startsWith('docker') || name.startsWith('br-') || name.startsWith('virbr') || name.startsWith('tun') || name.startsWith('tap') || name.startsWith('wg')) groups['Virtual'].push(iface);
    else if (name.startsWith('zt')) groups['Zerotier'].push(iface);
    else groups['Other'].push(iface);
  });
  return groups;
});

const groupedDiskDevices = computed(() => {
  const disks: Record<string, { main: IOSpeed, partitions: IOSpeed[] }> = {};
  systemStats.value.diskDevices.forEach(d => {
    const isPartition = /p\d+$/.test(d.name) || (/[a-z]\d+$/.test(d.name) && !d.name.startsWith('nvme'));
    if (!isPartition) disks[d.name] = { main: d, partitions: [] };
  });
  systemStats.value.diskDevices.forEach(d => {
    const isPartition = /p\d+$/.test(d.name) || (/[a-z]\d+$/.test(d.name) && !d.name.startsWith('nvme'));
    if (isPartition) {
      let parent = d.name.startsWith('nvme') ? d.name.split('p')[0] : d.name.replace(/\d+$/, '');
      if (disks[parent]) disks[parent].partitions.push(d);
      else disks[d.name] = { main: d, partitions: [] };
    }
  });
  return disks;
});

const gpus = ref<GPUStatus[]>([]);
const systemStats = ref<GlobalStats>({
  cpuTotal: 0, cpuCores: [], cpuCoresDetailed: [], memTotal: 0, memUsed: 0, memPercent: 0,
  memCached: 0, memBuffers: 0, memShared: 0, zramUsed: 0, zramTotal: 0,
  swapUsed: 0, swapTotal: 0,
  netInterfaces: [], diskDevices: [],
  totalNetRecv: 0, totalNetSent: 0, totalDiskRead: 0, totalDiskWrite: 0,
  faults: { pageFaults: 0, majorFaults: 0, minorFaults: 0, pageFaultRate: 0, majorFaultRate: 0, minorFaultRate: 0, swapIn: 0, swapOut: 0, swapInRate: 0, swapOutRate: 0 }
});

const loading = ref(false);
const tags = ref<string[]>([]);
let ws: WebSocket | null = null;
let reconnectTimer: any = null;
let shouldReconnect = true;

const topFaultProcesses = computed(() => {
  if (mergeFaultProcesses.value) {
    const merged: Record<string, { pid: string; name: string; minorFaults: number; majorFaults: number; count: number }> = {};
    processes.value.forEach(p => {
      if (!merged[p.name]) merged[p.name] = { pid: 'Multiple', name: p.name, minorFaults: 0, majorFaults: 0, count: 0 };
      merged[p.name].minorFaults += p.minorFaults;
      merged[p.name].majorFaults += p.majorFaults;
      merged[p.name].count++;
    });
    return Object.values(merged)
      .sort((a, b) => (b.majorFaults + b.minorFaults) - (a.majorFaults + a.minorFaults))
      .slice(0, faultTopN.value);
  }
  return [...processes.value]
    .sort((a, b) => (b.majorFaults + b.minorFaults) - (a.majorFaults + a.minorFaults))
    .slice(0, faultTopN.value);
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
    
    // Update core history
    systemStats.value.cpuCoresDetailed.forEach(core => {
      if (!statsHistory.value.cores[core.index]) statsHistory.value.cores[core.index] = [];
      statsHistory.value.cores[core.index].push({ time: now, value: core.usage });
      if (statsHistory.value.cores[core.index].length > 60) statsHistory.value.cores[core.index].shift();
    });

    // Update net device history
    systemStats.value.netInterfaces.forEach(iface => {
      if (!statsHistory.value.netDevices[iface.name]) statsHistory.value.netDevices[iface.name] = { recv: [], sent: [] };
      const dev = statsHistory.value.netDevices[iface.name];
      dev.recv.push({ time: now, value: iface.readSpeed });
      dev.sent.push({ time: now, value: iface.writeSpeed });
      if (dev.recv.length > 60) dev.recv.shift();
      if (dev.sent.length > 60) dev.sent.shift();
    });

    // Update disk device history
    systemStats.value.diskDevices.forEach(disk => {
      if (!statsHistory.value.diskDevices[disk.name]) statsHistory.value.diskDevices[disk.name] = { read: [], write: [] };
      const dev = statsHistory.value.diskDevices[disk.name];
      dev.read.push({ time: now, value: disk.readSpeed });
      dev.write.push({ time: now, value: disk.writeSpeed });
      if (dev.read.length > 60) dev.read.shift();
      if (dev.write.length > 60) dev.write.shift();
    });

    statsHistory.value.mem.push({ time: now, value: systemStats.value.memPercent });
    statsHistory.value.memUsed.push({ time: now, value: systemStats.value.memUsed });
    statsHistory.value.memCached.push({ time: now, value: systemStats.value.memCached });
    statsHistory.value.memBuffers.push({ time: now, value: systemStats.value.memBuffers });
    
    if (systemStats.value.swapTotal > 0) statsHistory.value.swapUsage.push({ time: now, value: systemStats.value.swapUsed });
    if (systemStats.value.zramTotal > 0) statsHistory.value.zramUsage.push({ time: now, value: systemStats.value.zramUsed });

    statsHistory.value.netRecv.push({ time: now, value: systemStats.value.totalNetRecv });
    statsHistory.value.netSent.push({ time: now, value: systemStats.value.totalNetSent });
    statsHistory.value.diskRead.push({ time: now, value: systemStats.value.totalDiskRead });
    statsHistory.value.diskWrite.push({ time: now, value: systemStats.value.totalDiskWrite });
    statsHistory.value.faults.push({ time: now, value: systemStats.value.faults.pageFaultRate });
    statsHistory.value.swapIn.push({ time: now, value: systemStats.value.faults.swapInRate });
    statsHistory.value.swapOut.push({ time: now, value: systemStats.value.faults.swapOutRate });

    // Clean up arrays
    Object.keys(statsHistory.value).forEach((key) => {
      const val = (statsHistory.value as any)[key];
      if (Array.isArray(val)) {
        if (val.length > 60) val.shift();
      }
    });
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
                  <a-button type="link" size="small" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])">History Chart</a-button>
                </div>
                
                <div v-if="cpuView === 'overall'" style="background: #fafafa; padding: 24px; border-radius: 8px; text-align: center; border: 1px solid #f0f0f0;">
                   <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="180" :stroke-color="systemStats.cpuTotal > 80 ? '#ff4d4f' : '#1890ff'" @click="openHistoryChart('CPU Usage History', [{ name: 'Total CPU', data: statsHistory.cpu, color: '#1890ff' }])" style="cursor: pointer;" />
                   <div style="margin-top: 16px; font-size: 18px; font-weight: bold;">System CPU Usage: {{ systemStats.cpuTotal.toFixed(1) }}%</div>
                </div>
                
                <div v-else style="display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px;">
                  <div v-for="core in systemStats.cpuCoresDetailed" :key="core.index" 
                       style="padding: 12px; border: 1px solid #f0f0f0; border-radius: 8px; text-align: center; background: #fafafa; cursor: pointer; transition: all 0.2s;"
                       class="core-card"
                       @click="openHistoryChart(`Core #${core.index} Usage History`, [{ name: `Core #${core.index}`, data: statsHistory.cores[core.index] || [], color: getCoreTypeColor(core.type) }])">
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
                          <a-button type="link" size="small" @click="openHistoryChart('Memory Usage History', [
                            { name: 'Used', data: statsHistory.memUsed, color: '#1890ff' },
                            { name: 'Cached', data: statsHistory.memCached, color: '#52c41a' },
                            { name: 'Buffers', data: statsHistory.memBuffers, color: '#faad14' }
                          ])">History (All)</a-button>
                       </template>
                       <a-statistic title="Overall Usage" :value="systemStats.memPercent" suffix="%" :precision="1" @click="openHistoryChart('Memory Usage History (%)', [{ name: 'Mem %', data: statsHistory.mem, color: '#52c41a' }])" style="cursor: pointer;" />
                       <div style="margin-top: 16px; display: grid; gap: 8px;">
                          <div style="display: flex; justify-content: space-between;"><span>Total:</span><b>{{ formatBytesWithUnit(systemStats.memTotal) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #1890ff; cursor: pointer;" @click="openHistoryChart('Used Memory Usage History', [{ name: 'Used', data: statsHistory.memUsed, color: '#1890ff' }])"><span>Used:</span><b>{{ formatBytesWithUnit(systemStats.memUsed) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #52c41a; cursor: pointer;" @click="openHistoryChart('Cached Memory Usage History', [{ name: 'Cached', data: statsHistory.memCached, color: '#52c41a' }])"><span>Cached:</span><b>{{ formatBytesWithUnit(systemStats.memCached) }}</b></div>
                          <div style="display: flex; justify-content: space-between; color: #faad14; cursor: pointer;" @click="openHistoryChart('Buffers Memory Usage History', [{ name: 'Buffers', data: statsHistory.memBuffers, color: '#faad14' }])"><span>Buffers:</span><b>{{ formatBytesWithUnit(systemStats.memBuffers) }}</b></div>
                       </div>
                    </a-card>
                  </a-col>
                  <a-col :span="12">
                    <a-card title="Swap / ZRAM" size="small" :bordered="false" style="background: #fafafa;">
                       <template #extra>
                          <a-button type="link" size="small" @click="openHistoryChart('Swap/ZRAM Usage History', [
                            { name: 'Swap Used', data: statsHistory.swapUsage, color: '#722ed1' },
                            { name: 'ZRAM Used', data: statsHistory.zramUsage, color: '#13c2c2' }
                          ])">History</a-button>
                       </template>
                       <div style="display: grid; gap: 16px;">
                          <div v-if="systemStats.swapTotal > 0" style="cursor: pointer;" @click="openHistoryChart('Swap Usage History', [{ name: 'Swap Used', data: statsHistory.swapUsage, color: '#722ed1' }])">
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
                          <div v-if="systemStats.zramTotal > 0" style="cursor: pointer;" @click="openHistoryChart('ZRAM Usage History', [{ name: 'ZRAM Used', data: statsHistory.zramUsage, color: '#13c2c2' }])">
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
                    <a-card size="small" :bordered="false" style="background: #fafafa;">
                       <template #title>
                          <div style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
                             <span>Network Activity</span>
                             <span style="font-size: 11px; color: #888; font-weight: normal;">(Total: ↓{{ formatBytesWithUnit(systemStats.totalNetRecv) }}/s ↑{{ formatBytesWithUnit(systemStats.totalNetSent) }}/s)</span>
                          </div>
                       </template>
                       <template #extra>
                          <a-space>
                            <a-button type="link" size="small" @click="openHistoryChart('Global Network Activity', [
                              { name: 'Recv', data: statsHistory.netRecv, color: '#52c41a' },
                              { name: 'Sent', data: statsHistory.netSent, color: '#1890ff' }
                            ])">All</a-button>
                            <a-button type="link" size="small" @click="openHistoryChart('Split Network Activity', Object.entries(statsHistory.netDevices).flatMap(([name, d]) => [
                               { name: `${name} Recv`, data: d.recv, color: undefined },
                               { name: `${name} Sent`, data: d.sent, color: undefined }
                            ]))">Split</a-button>
                          </a-space>
                       </template>
                       <div style="display: flex; flex-direction: column; gap: 8px;">
                          <div v-for="(group, label) in groupedNetInterfaces" :key="label">
                             <div v-if="group.length > 0">
                                <div style="font-size: 11px; font-weight: bold; color: #888; margin: 8px 0 4px;">{{ label }}</div>
                                <div v-for="iface in group" :key="iface.name" style="margin-bottom: 8px; padding: 10px; border-radius: 6px; background: #fff; border: 1px solid #f0f0f0; cursor: pointer; transition: all 0.2s;" class="io-card" @click="openHistoryChart(`${iface.name} Activity`, [
                                   { name: 'Recv', data: statsHistory.netDevices[iface.name]?.recv || [], color: '#52c41a' },
                                   { name: 'Sent', data: statsHistory.netDevices[iface.name]?.sent || [], color: '#1890ff' }
                                ])">
                                   <div style="font-family: monospace; font-weight: bold; margin-bottom: 6px; display: flex; justify-content: space-between;">
                                      <span>{{ iface.name }}</span>
                                   </div>
                                   <div style="display: flex; gap: 16px; font-size: 12px;">
                                      <span style="color: #52c41a; flex: 1;">↓ {{ formatBytesWithUnit(iface.readSpeed) }}/s</span>
                                      <span style="color: #1890ff; flex: 1;">↑ {{ formatBytesWithUnit(iface.writeSpeed) }}/s</span>
                                   </div>
                                </div>
                             </div>
                          </div>
                       </div>
                    </a-card>
                  </a-col>
                  <a-col :span="12">
                    <a-card size="small" :bordered="false" style="background: #fafafa;">
                       <template #title>
                          <div style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
                             <span>Storage Activity</span>
                             <span style="font-size: 11px; color: #888; font-weight: normal;">(Total: R:{{ formatBytesWithUnit(systemStats.totalDiskRead) }}/s W:{{ formatBytesWithUnit(systemStats.totalDiskWrite) }}/s)</span>
                          </div>
                       </template>
                       <template #extra>
                          <a-space>
                            <a-button type="link" size="small" @click="openHistoryChart('Global Disk Activity', [
                              { name: 'Read', data: statsHistory.diskRead, color: '#faad14' },
                              { name: 'Write', data: statsHistory.diskWrite, color: '#722ed1' }
                            ])">All</a-button>
                            <a-button type="link" size="small" @click="openHistoryChart('Split Disk Activity', Object.entries(statsHistory.diskDevices).flatMap(([name, d]) => [
                               { name: `${name} Read`, data: d.read, color: undefined },
                               { name: `${name} Write`, data: d.write, color: undefined }
                            ]))">Split</a-button>
                          </a-space>
                       </template>
                       <div style="display: flex; flex-direction: column; gap: 12px;">
                          <div v-for="(disk, name) in groupedDiskDevices" :key="name">
                             <div style="padding: 10px; border-radius: 6px; background: #fff; border: 1px solid #e8e8e8; cursor: pointer;" class="io-card" @click="openHistoryChart(`${name} Activity`, [
                                { name: 'Read', data: statsHistory.diskDevices[name]?.read || [], color: '#faad14' },
                                { name: 'Write', data: statsHistory.diskDevices[name]?.write || [], color: '#722ed1' }
                             ])">
                                <div style="font-family: monospace; font-weight: bold; margin-bottom: 6px; display: flex; align-items: center; gap: 8px;">
                                   <DatabaseOutlined style="color: #1890ff;" />
                                   <span>{{ name }}</span>
                                </div>
                                <div style="display: flex; gap: 16px; font-size: 12px; border-bottom: 1px solid #f5f5f5; padding-bottom: 8px; margin-bottom: 8px;">
                                   <span style="color: #faad14; flex: 1; font-weight: 500;">Read: {{ formatBytesWithUnit(disk.main.readSpeed) }}/s</span>
                                   <span style="color: #722ed1; flex: 1; font-weight: 500;">Write: {{ formatBytesWithUnit(disk.main.writeSpeed) }}/s</span>
                                </div>
                                <div v-if="disk.partitions.length > 0">
                                   <div v-for="part in disk.partitions" :key="part.name" style="display: flex; gap: 12px; font-size: 10px; padding: 2px 0 2px 20px; color: #666; border-left: 2px solid #f0f0f0; margin-left: 6px;">
                                      <span style="width: 80px; font-family: monospace;">└─ {{ part.name }}</span>
                                      <span style="color: #faad14; opacity: 0.8;">R: {{ formatBytesWithUnit(part.readSpeed) }}/s</span>
                                      <span style="color: #722ed1; opacity: 0.8;">W: {{ formatBytesWithUnit(part.writeSpeed) }}/s</span>
                                   </div>
                                </div>
                             </div>
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
                        <a-button type="link" size="small" @click="openHistoryChart('Page Fault Rate History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#ff4d4f' }])">History Chart</a-button>
                      </template>
                      <a-row :gutter="16">
                         <a-col :span="6"><a-statistic title="Soft Faults (Minor)" :value="systemStats.faults.minorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Minor Fault History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#52c41a' }])" style="cursor: pointer;" /></a-col>
                         <a-col :span="6"><a-statistic title="Hard Faults (Major)" :value="systemStats.faults.majorFaultRate" :precision="1" suffix="/s" @click="openHistoryChart('Major Fault History', [{ name: 'Faults/s', data: statsHistory.faults, color: '#ff4d4f' }])" style="cursor: pointer;" /></a-col>
                         <a-col :span="6"><a-statistic title="Swap-Out Rate" :value="systemStats.faults.swapOutRate" :precision="1" suffix="/s" @click="openHistoryChart('Swap-Out Rate History', [{ name: 'Faults/s', data: statsHistory.swapOut, color: '#722ed1' }])" style="cursor: pointer;" /></a-col>
                         <a-col :span="6"><a-statistic title="Swap-In Rate" :value="systemStats.faults.swapInRate" :precision="1" suffix="/s" @click="openHistoryChart('Swap-In Rate History', [{ name: 'Faults/s', data: statsHistory.swapIn, color: '#13c2c2' }])" style="cursor: pointer;" /></a-col>
                      </a-row>
                    </a-card>
                  </a-col>
                  <a-col :span="24" style="margin-top: 16px;">
                    <a-card title="Top Processes by Faults" size="small" :bordered="false" style="background: #fafafa;">
                      <template #extra>
                        <a-space>
                          <span style="font-size: 12px; color: #888;">Top:</span>
                          <a-select v-model:value="faultTopN" size="small" style="width: 70px;">
                            <a-select-option :value="5">5</a-select-option>
                            <a-select-option :value="10">10</a-select-option>
                            <a-select-option :value="20">20</a-select-option>
                          </a-select>
                          <a-checkbox v-model:checked="mergeFaultProcesses" style="font-size: 12px; color: #888;">Merge by Name</a-checkbox>
                        </a-space>
                      </template>
                      <a-table :dataSource="topFaultProcesses" :columns="[
                        { title: mergeFaultProcesses ? 'Group' : 'PID', dataIndex: mergeFaultProcesses ? 'name' : 'pid', key: 'id', width: mergeFaultProcesses ? 200 : 80 },
                        { title: mergeFaultProcesses ? 'Instances' : 'Command', dataIndex: mergeFaultProcesses ? 'count' : 'name', key: 'name' },
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

      <a-tab-pane key="processes" tab="Processes">
        <template #tab><span><AppstoreOutlined /> Processes</span></template>
        <div style="background: #fff; padding: 20px; border-radius: 4px;">
          <div style="margin-bottom: 16px; display: flex; justify-content: space-between; align-items: center; gap: 16px;">
            <a-space>
              <a-radio-group v-model:value="processViewMode" button-style="solid" size="small">
                <a-radio-button value="flat">Flat</a-radio-button>
                <a-radio-button value="tree">Tree</a-radio-button>
                <a-radio-button value="merged">Merged</a-radio-button>
              </a-radio-group>
              <a-input-search v-model:value="processSearch" placeholder="Search processes (name/PID/cmd)..." style="width: 300px" size="small" allow-clear />
            </a-space>
            <span style="font-size: 12px; color: #888;">Total: {{ processes.length }} processes</span>
          </div>
          <a-table :dataSource="processedProcesses" :columns="trackedColumns" size="small" rowKey="key" :scroll="{ y: 'calc(100vh - 420px)' }" @change="(p, f, s) => {}" :pagination="false">
             <template #bodyCell="{ column, record, text }">
                <template v-if="column.key === 'pid'">
                   <span v-if="record.key && typeof record.key === 'string' && record.key.startsWith('group-')" style="color: #888;">Multiple</span>
                   <span v-else style="font-family: monospace;">{{ text }}</span>
                </template>
                <template v-if="column.key === 'name'">
                   <span style="font-weight: 500; cursor: pointer; color: #1890ff;" @click="showProcessDetails(record)">{{ text }}</span>
                </template>
                <template v-if="column.key === 'cpu'">
                   <span :style="{ color: text > 50 ? '#ff4d4f' : 'inherit', fontWeight: text > 20 ? 'bold' : 'normal' }">{{ text.toFixed(1) }}%</span>
                </template>
                <template v-if="column.key === 'mem'">
                   <span>{{ text.toFixed(1) }}%</span>
                </template>
                <template v-if="column.key === 'gpuUtil'">
                   <span v-if="text > 0">{{ text }}%</span>
                   <span v-else color="#ccc">-</span>
                </template>
                <template v-if="column.key === 'gpuMem'">
                   <span v-if="text > 0">{{ text }} MB</span>
                   <span v-else color="#ccc">-</span>
                </template>
                <template v-if="column.key === 'action'">
                   <a-space v-if="!(record.key && typeof record.key === 'string' && record.key.startsWith('group-'))">
                      <a-button type="link" size="small" @click="showProcessDetails(record)">Details</a-button>
                      <a-button type="link" size="small" danger @click="sendProcessSignal(record.pid, 'kill')">Kill</a-button>
                   </a-space>
                </template>
             </template>
          </a-table>
        </div>
      </a-tab-pane>

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

    <a-modal v-model:open="showProcessMapsModal" :title="`Process Details: ${selectedProcessDetails?.name} (PID: ${selectedProcessDetails?.pid})`" width="1000px" :footer="null">
       <div style="display: flex; flex-direction: column; gap: 16px;">
          <div v-if="selectedProcessDetails" style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; background: #fafafa; padding: 12px; border-radius: 4px;">
             <div><span style="color:#888">User:</span> <b>{{ selectedProcessDetails.user }}</b></div>
             <div><span style="color:#888">CPU:</span> <b>{{ selectedProcessDetails.cpu.toFixed(1) }}%</b></div>
             <div><span style="color:#888">Mem:</span> <b>{{ selectedProcessDetails.mem.toFixed(1) }}%</b></div>
             <div style="grid-column: span 3;"><span style="color:#888">Command:</span> <code style="font-size: 11px;">{{ selectedProcessDetails.cmdline }}</code></div>
          </div>
          <div style="height: 500px; overflow-y: auto; background: #1e1e1e; color: #d4d4d4; padding: 12px; border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 12px;">
             <a-spin :spinning="processMapsLoading">
                <pre v-if="selectedProcessMaps" style="margin: 0; white-space: pre-wrap; word-break: break-all;">{{ selectedProcessMaps }}</pre>
                <a-empty v-else-if="!processMapsLoading" description="No map data available" />
             </a-spin>
          </div>
       </div>
    </a-modal>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 0; }
</style>
