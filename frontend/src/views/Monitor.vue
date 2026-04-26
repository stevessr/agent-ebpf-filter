<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch, defineAsyncComponent } from 'vue';
import axios from 'axios';
import { 
  PlusOutlined, SearchOutlined, ClusterOutlined, TableOutlined, 
  FilterOutlined, DeploymentUnitOutlined,
  DashboardOutlined, PieChartOutlined,
  AppstoreOutlined, BarChartOutlined, LineChartOutlined, InfoCircleOutlined,
  WarningOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

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
  faultRate?: number;
  majorFaultRate?: number;
  minorFaultRate?: number;
  children?: ProcessInfo[];
}

interface FaultProcessInfo extends ProcessInfo {
  faultRate: number;
  majorFaultRate: number;
  minorFaultRate: number;
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

interface FaultSnapshot {
  createTime: number;
  minorFaults: number;
  majorFaults: number;
  time: number;
}

interface HistoryData {
  time: number;
  value: number;
  value2?: number; // for in/out or R/W
}

type ChartUnit = 'percent' | 'bytes' | 'fault' | 'pages' | 'vram' | 'raw';

interface ByteScale {
  divisor: number;
  unit: 'B' | 'KB' | 'MB' | 'GB' | 'TB';
  precision: number;
}

const activeTab = ref('dashboard');
const healthTab = ref('cpu');
const processes = ref<ProcessInfo[]>([]);
const faultProcesses = ref<FaultProcessInfo[]>([]);
const gpus = ref<GPUStatus[]>([]);
const systemStats = ref<GlobalStats>({
  cpuTotal: 0, cpuCores: [], cpuCoresDetailed: [], memTotal: 0, memUsed: 0, memPercent: 0,
  memCached: 0, memBuffers: 0, memShared: 0, zramUsed: 0, zramTotal: 0,
  netInterfaces: [], diskDevices: [],
  totalNetRecv: 0, totalNetSent: 0, totalDiskRead: 0, totalDiskWrite: 0,
  faults: {
    pageFaults: 0,
    majorFaults: 0,
    minorFaults: 0,
    pageFaultRate: 0,
    majorFaultRate: 0,
    minorFaultRate: 0,
    swapIn: 0,
    swapOut: 0,
    swapInRate: 0,
    swapOutRate: 0,
  },
});

// Chart State
const showChartModal = ref(false);
const chartTitle = ref('');
const chartType = ref<'single' | 'double'>('single');
const chartSeriesName = ref(['Value']);
const historyMap = ref<Record<string, HistoryData[]>>({});
const activeChartKey = ref('');
const chartTimeRange = ref(60); // seconds
const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

// Process Detail State
const showProcModal = ref(false);
const selectedProc = ref<ProcessInfo | null>(null);

const isConnected = ref(false);
const loading = ref(false);
const searchText = ref('');
const viewMode = ref<'list' | 'tree'>('tree');
const cpuViewMode = ref<'total' | 'cores'>('total');
const cpuDisplayMode = ref<'bar' | 'circle'>('bar');
const refreshInterval = ref(2000);
const tags = ref<string[]>([]);
const selectedTag = ref('AI Agent');

// Advanced Filters
const cpuThreshold = ref(0);
const cpuMax = ref(100);
const memThreshold = ref(0);
const memMax = ref(100);
const gpuThreshold = ref(0);
const gpuMax = ref(8192);
const filterUser = ref<string | null>(null);
const showAdvancedFilters = ref(false);

const cpuRange = computed({
  get: () => [cpuThreshold.value, cpuMax.value],
  set: (val: number[]) => { cpuThreshold.value = val[0]; cpuMax.value = val[1]; }
});
const memRange = computed({
  get: () => [memThreshold.value, memMax.value],
  set: (val: number[]) => { memThreshold.value = val[0]; memMax.value = val[1]; }
});
const gpuRange = computed({
  get: () => [gpuThreshold.value, gpuMax.value],
  set: (val: number[]) => { gpuThreshold.value = val[0]; gpuMax.value = val[1]; }
});

let ws: WebSocket | null = null;
const faultSnapshots = ref<Record<number, FaultSnapshot>>({});
let lastIO: { 
  networks: Record<string, {r: number, s: number}>;
  disks: Record<string, {r: number, w: number}>;
  time: number 
} | null = null;

const formatBytes = (bytes: number, decimals = 2) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(decimals)) + ' ' + sizes[i];
};

const formatRateBytes = (bytes: number, decimals = 1) => `${formatBytes(bytes, decimals)}/s`;
const formatCount = (value: number) => new Intl.NumberFormat('en-US').format(Math.max(0, Math.round(value || 0)));
const formatRate = (value: number) => `${(value || 0).toFixed(1)}/s`;
const formatPercent = (value?: number | string | null) => `${Math.round(Number(value ?? 0))}%`;
const formatDetailValue = (value: number | string | undefined | null) => {
  if (value === null || value === undefined || value === '') return '—';
  return typeof value === 'number' ? String(Math.trunc(value)) : String(value);
};
const pad2 = (value: number) => String(Math.floor(Math.abs(value))).padStart(2, '0');

const resolveByteScale = (values: number[]): ByteScale => {
  const max = Math.max(
    1,
    ...values
      .map((value) => Math.abs(Number(value || 0)))
      .filter((value) => Number.isFinite(value)),
  );

  if (max >= 1024 ** 4) return { divisor: 1024 ** 4, unit: 'TB', precision: 2 };
  if (max >= 1024 ** 3) return { divisor: 1024 ** 3, unit: 'GB', precision: 2 };
  if (max >= 1024 ** 2) return { divisor: 1024 ** 2, unit: 'MB', precision: 2 };
  if (max >= 1024) return { divisor: 1024, unit: 'KB', precision: 1 };
  return { divisor: 1, unit: 'B', precision: 0 };
};

const chartUnitForKey = (key: string): ChartUnit => {
  const normalized = key.toLowerCase();
  if (normalized.includes('cpu') || normalized.includes('mem_usage') || normalized.includes('util') || normalized.includes('percent')) {
    return 'percent';
  }
  if (normalized.includes('swap')) return 'pages';
  if (normalized.includes('fault')) return 'fault';
  if (normalized.includes('vram')) return 'vram';
  if (normalized.startsWith('net_') || normalized.startsWith('disk_') || normalized.includes('io') || normalized.includes('total_net') || normalized.includes('total_disk')) {
    return 'bytes';
  }
  return 'raw';
};

const formatChartTime = (timestamp: number, rangeSeconds: number) => {
  const date = new Date(timestamp);
  const hh = pad2(date.getHours());
  const mm = pad2(date.getMinutes());
  const ss = pad2(date.getSeconds());

  // Short spans: show seconds.
  if (rangeSeconds <= 120) {
    return `${hh}:${mm}:${ss}`;
  }

  // Medium spans: show HH:mm to keep the axis readable.
  if (rangeSeconds <= 1800) {
    return `${hh}:${mm}`;
  }

  // Longer spans: include date.
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${hh}:${mm}`;
};

const getChartWindow = () => {
  const data = historyMap.value[activeChartKey.value] || [];
  const max = data.length ? data[data.length - 1].time : Date.now();
  const min = max - (chartTimeRange.value * 1000);
  return { min, max };
};

const buildFaultProcesses = (list: ProcessInfo[], now: number) => {
  const nextSnapshots: Record<number, FaultSnapshot> = {};
  const rows = list.map((proc) => {
    const minorFaults = Number(proc.minorFaults || 0);
    const majorFaults = Number(proc.majorFaults || 0);
    const prev = faultSnapshots.value[proc.pid];
    let minorFaultRate = 0;
    let majorFaultRate = 0;

    if (prev && prev.createTime === proc.createTime && now > prev.time) {
      const dt = (now - prev.time) / 1000;
      if (dt > 0) {
        minorFaultRate = Math.max(0, (minorFaults - prev.minorFaults) / dt);
        majorFaultRate = Math.max(0, (majorFaults - prev.majorFaults) / dt);
      }
    }

    nextSnapshots[proc.pid] = {
      createTime: proc.createTime,
      minorFaults,
      majorFaults,
      time: now,
    };

    return {
      ...proc,
      minorFaults,
      majorFaults,
      minorFaultRate,
      majorFaultRate,
      faultRate: minorFaultRate + majorFaultRate,
    } satisfies FaultProcessInfo;
  });

  faultSnapshots.value = nextSnapshots;
  faultProcesses.value = rows
    .sort((a, b) => b.majorFaultRate - a.majorFaultRate || b.faultRate - a.faultRate || b.majorFaults - a.majorFaults || b.minorFaults - a.minorFaults)
    .slice(0, 20);
};

const connectWebSocket = () => {
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }
  lastIO = null;
  faultSnapshots.value = {};
  const socket = new WebSocket(buildWebSocketUrl('/ws/system', { interval: refreshInterval.value }));
  ws = socket;
  socket.binaryType = 'arraybuffer';

  socket.onopen = () => {
    if (ws !== socket) return;
    isConnected.value = true;
    loading.value = false;
  };
  socket.onmessage = (msg) => {
    if (ws !== socket) return;
    try {
      const decoded = pb.SystemStats.decode(new Uint8Array(msg.data));
      const now = Date.now();

      const newNetSpeeds: IOSpeed[] = (decoded.io?.networks || []).map((n: any) => ({
        name: n.name,
        readSpeed: 0,
        writeSpeed: 0,
      }));
      const newDiskSpeeds: IOSpeed[] = (decoded.io?.disks || []).map((d: any) => ({
        name: d.name,
        readSpeed: 0,
        writeSpeed: 0,
      }));
      const netSpeedMap = new Map(newNetSpeeds.map((item) => [item.name, item]));
      const diskSpeedMap = new Map(newDiskSpeeds.map((item) => [item.name, item]));
      
      const updateHistory = (key: string, val: number, val2?: number) => {
        if (!historyMap.value[key]) historyMap.value[key] = [];
        historyMap.value[key].push({ time: now, value: val, value2: val2 });
        if (historyMap.value[key].length > 1200) historyMap.value[key].shift();
      };

      if (lastIO && decoded.io) {
        const dt = (now - lastIO.time) / 1000;
        (decoded.io.networks || []).forEach((n: any) => {
          const prev = lastIO?.networks[n.name];
          if (prev) {
            const rin = (Number(n.recvBytes) - prev.r) / dt;
            const rout = (Number(n.sentBytes) - prev.s) / dt;
            const entry = netSpeedMap.get(n.name);
            if (entry) {
              entry.readSpeed = rin;
              entry.writeSpeed = rout;
            }
          }
        });
        (decoded.io.disks || []).forEach((d: any) => {
          const prev = lastIO?.disks[d.name];
          if (prev) {
            const rin = (Number(d.readBytes) - prev.r) / dt;
            const win = (Number(d.writeBytes) - prev.w) / dt;
            const entry = diskSpeedMap.get(d.name);
            if (entry) {
              entry.readSpeed = rin;
              entry.writeSpeed = win;
            }
          }
        });
      }

      if (decoded.io) {
        const nets: Record<string, {r: number, s: number}> = {};
        (decoded.io.networks || []).forEach((n: any) => nets[n.name] = {r: Number(n.recvBytes), s: Number(n.sentBytes)});
        const dsks: Record<string, {r: number, w: number}> = {};
        (decoded.io.disks || []).forEach((d: any) => dsks[d.name] = {r: Number(d.readBytes), w: Number(d.writeBytes)});
        lastIO = { networks: nets, disks: dsks, time: now };
        systemStats.value.netInterfaces = [...newNetSpeeds].sort(
          (a, b) => (b.readSpeed + b.writeSpeed) - (a.readSpeed + a.writeSpeed)
            || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }),
        );
        systemStats.value.diskDevices = [...newDiskSpeeds].sort(
          (a, b) => (b.readSpeed + b.writeSpeed) - (a.readSpeed + a.writeSpeed)
            || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }),
        );
        let totalNetR = 0, totalNetS = 0, totalDiskR = 0, totalDiskW = 0;
        newNetSpeeds.forEach(s => { totalNetR += s.readSpeed; totalNetS += s.writeSpeed; });
        newDiskSpeeds.forEach(s => { totalDiskR += s.readSpeed; totalDiskW += s.writeSpeed; });
        systemStats.value.totalNetRecv = totalNetR;
        systemStats.value.totalNetSent = totalNetS;
        systemStats.value.totalDiskRead = totalDiskR;
        systemStats.value.totalDiskWrite = totalDiskW;
        newNetSpeeds.forEach((s) => updateHistory(`net_${s.name}`, s.readSpeed, s.writeSpeed));
        newDiskSpeeds.forEach((s) => updateHistory(`disk_${s.name}`, s.readSpeed, s.writeSpeed));
        updateHistory('total_net', totalNetR, totalNetS);
        updateHistory('total_disk', totalDiskR, totalDiskW);
      }

      if (decoded.cpu) {
        systemStats.value.cpuTotal = decoded.cpu.total || 0;
        updateHistory('cpu_total', systemStats.value.cpuTotal);
        systemStats.value.cpuCores = (decoded.cpu.cores as number[]) || [];
        systemStats.value.cpuCoresDetailed = (decoded.cpu.coreDetails || []).map((c: any) => {
          updateHistory(`cpu_core_${c.index}`, c.usage || 0);
          return { index: c.index, usage: c.usage || 0, type: c.type };
        });
      }
      
      if (decoded.memory) {
        systemStats.value.memTotal = Number(decoded.memory.total);
        systemStats.value.memUsed = Number(decoded.memory.used);
        systemStats.value.memPercent = decoded.memory.percent || 0;
        systemStats.value.memCached = Number(decoded.memory.cached);
        systemStats.value.memBuffers = Number(decoded.memory.buffers);
        systemStats.value.memShared = Number(decoded.memory.shared);
        systemStats.value.zramUsed = Number(decoded.memory.zramUsed);
        systemStats.value.zramTotal = Number(decoded.memory.zramTotal);
        updateHistory('mem_usage', systemStats.value.memPercent);
      }

      if (decoded.faults) {
        systemStats.value.faults.pageFaults = Number(decoded.faults.pageFaults || 0);
        systemStats.value.faults.majorFaults = Number(decoded.faults.majorFaults || 0);
        systemStats.value.faults.minorFaults = Number(decoded.faults.minorFaults || 0);
        systemStats.value.faults.pageFaultRate = Number(decoded.faults.pageFaultRate || 0);
        systemStats.value.faults.majorFaultRate = Number(decoded.faults.majorFaultRate || 0);
        systemStats.value.faults.minorFaultRate = Number(decoded.faults.minorFaultRate || 0);
        systemStats.value.faults.swapIn = Number(decoded.faults.swapIn || 0);
        systemStats.value.faults.swapOut = Number(decoded.faults.swapOut || 0);
        systemStats.value.faults.swapInRate = Number(decoded.faults.swapInRate || 0);
        systemStats.value.faults.swapOutRate = Number(decoded.faults.swapOutRate || 0);
        updateHistory('fault_page_rate', systemStats.value.faults.pageFaultRate);
        updateHistory('fault_major_rate', systemStats.value.faults.majorFaultRate);
        updateHistory('fault_minor_rate', systemStats.value.faults.minorFaultRate);
        updateHistory('fault_swap_rate', systemStats.value.faults.swapInRate, systemStats.value.faults.swapOutRate);
      }

      processes.value = (decoded.processes || []).map((p: any) => ({
        pid: p.pid,
        ppid: p.ppid,
        name: p.name,
        cpu: p.cpu,
        mem: p.mem,
        user: p.user,
        gpuMem: p.gpuMem,
        gpuId: p.gpuId,
        cmdline: p.cmdline,
        createTime: Number(p.createTime),
        minorFaults: Number(p.minorFaults || 0),
        majorFaults: Number(p.majorFaults || 0),
      }));
      buildFaultProcesses(processes.value, now);

      gpus.value = (decoded.gpus || []).map((g: any) => {
        updateHistory(`gpu_${g.index}_util`, g.utilGpu);
        updateHistory(`gpu_${g.index}_vram`, g.memUsed);
        return { index: g.index, name: g.name, utilGpu: g.utilGpu, utilMem: g.utilMem, memTotal: g.memTotal, memUsed: g.memUsed, temp: g.temp };
      });
    } catch (e) { console.error(e); }
  };
  socket.onclose = () => {
    if (ws !== socket) return;
    isConnected.value = false;
  };
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

const pCores = computed(() => systemStats.value.cpuCoresDetailed.filter(c => c.type === 0));
const eCores = computed(() => systemStats.value.cpuCoresDetailed.filter(c => c.type === 1));
const uniqueUsers = computed(() => Array.from(new Set(processes.value.map(p => p.user))).sort());

const displayData = computed(() => {
  let filtered = processes.value;
  if (searchText.value) {
    const s = searchText.value.toLowerCase();
    filtered = filtered.filter(p => p.name.toLowerCase().includes(s) || p.pid.toString().includes(s));
  }
  
  // Apply Advanced Filters
  if (cpuThreshold.value > 0) filtered = filtered.filter(p => p.cpu >= cpuThreshold.value);
  if (cpuMax.value < 100) filtered = filtered.filter(p => p.cpu <= cpuMax.value);
  
  if (memThreshold.value > 0) filtered = filtered.filter(p => p.mem >= memThreshold.value);
  if (memMax.value < 100) filtered = filtered.filter(p => p.mem <= memMax.value);
  
  if (gpuThreshold.value > 0) filtered = filtered.filter(p => p.gpuMem >= gpuThreshold.value);
  if (gpuMax.value < 8192) filtered = filtered.filter(p => p.gpuMem <= gpuMax.value);
  
  if (filterUser.value) filtered = filtered.filter(p => p.user === filterUser.value);

  const anyFilterActive = searchText.value || cpuThreshold.value > 0 || cpuMax.value < 100 || 
                         memThreshold.value > 0 || memMax.value < 100 || 
                         gpuThreshold.value > 0 || gpuMax.value < 8192 || filterUser.value;

  if (viewMode.value === 'tree' && !anyFilterActive) {
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

const openChart = (key: string, title: string, type: 'single' | 'double', seriesNames: string[]) => {
  activeChartKey.value = key;
  chartTitle.value = title;
  chartType.value = type;
  chartSeriesName.value = seriesNames;
  showChartModal.value = true;
};

const chartVisibleData = computed(() => {
  const data = historyMap.value[activeChartKey.value] || [];
  const { min } = getChartWindow();
  return data.filter((entry) => entry.time >= min);
});

const chartUnit = computed<ChartUnit>(() => chartUnitForKey(activeChartKey.value));

const chartByteScale = computed(() => {
  if (chartUnit.value !== 'bytes') {
    return { divisor: 1, unit: 'B', precision: 0 } as ByteScale;
  }
  return resolveByteScale(chartVisibleData.value.flatMap((entry) => [entry.value, entry.value2 ?? 0]));
});

const formatChartValue = (value: number) => {
  const unit = chartUnit.value;
  if (unit === 'percent') return `${Number(value || 0).toFixed(1)}%`;
  if (unit === 'pages') return `${Number(value || 0).toFixed(1)} pages/s`;
  if (unit === 'fault') return `${Number(value || 0).toFixed(1)} faults/s`;
  if (unit === 'vram') return `${Number(value || 0).toFixed(1)} MiB`;
  if (unit === 'bytes') return `${Number(value || 0).toFixed(chartByteScale.value.precision)} ${chartByteScale.value.unit}/s`;
  return Number(value || 0).toFixed(1);
};

const chartOptions = computed(() => {
  const { min, max } = getChartWindow();
  const unit = chartUnit.value;
  const byteScale = chartByteScale.value;
  return {
    chart: { animations: { enabled: false }, toolbar: { show: false }, zoom: { enabled: false }, background: 'transparent' },
    xaxis: {
      type: 'datetime' as const,
      min,
      max,
      labels: {
        datetimeUTC: false,
        style: { fontSize: '10px' },
        formatter: (value: string | number) => formatChartTime(Number(value), chartTimeRange.value),
      },
      tooltip: {
        enabled: true,
        formatter: (value: string | number) => formatChartTime(Number(value), chartTimeRange.value),
      },
      range: chartTimeRange.value * 1000,
      tickAmount: 6
    },
    tooltip: {
      x: {
        formatter: (value: string | number) => formatChartTime(Number(value), chartTimeRange.value),
      },
      y: {
        formatter: (value: number) => formatChartValue(value),
      },
    },
    yaxis: {
      min: unit === 'percent' ? 0 : undefined,
      max: unit === 'percent' ? 100 : undefined,
      decimalsInFloat: unit === 'bytes' ? byteScale.precision : 1,
      labels: {
        style: { fontSize: '10px' },
        formatter: (v: number) => formatChartValue(v),
      },
    },
    stroke: { curve: 'smooth' as const, width: 2 },
    grid: { borderColor: '#f1f1f1' },
    legend: { position: 'top' as const, horizontalAlign: 'right' as const },
    theme: { mode: 'light' as const }
  };
});

const chartSeries = computed(() => {
  const filtered = chartVisibleData.value;
  const unit = chartUnit.value;
  const scale = unit === 'bytes' ? chartByteScale.value.divisor : 1;
  const mapValue = (value: number | undefined) => {
    const normalized = Number(value || 0);
    return unit === 'bytes' ? normalized / scale : normalized;
  };
  if (chartType.value === 'single') {
    return [{ name: chartSeriesName.value[0], data: filtered.map((entry) => ({ x: entry.time, y: mapValue(entry.value) })) }];
  }
  return [
    { name: chartSeriesName.value[0], data: filtered.map((entry) => ({ x: entry.time, y: mapValue(entry.value) })) },
    { name: chartSeriesName.value[1], data: filtered.map((entry) => ({ x: entry.time, y: mapValue(entry.value2) })) }
  ];
});

const showGroupDetails = ref(false);
const selectedGroup = ref<{ name: string; pids: number[] } | null>(null);

const openGroupDetails = (name: string, pids: number[]) => {
  selectedGroup.value = { name, pids };
  showGroupDetails.value = true;
};

const openProcDetails = (proc: ProcessInfo) => {
  selectedProc.value = proc;
  showProcModal.value = true;
};

const selectedGroupProcesses = computed(() => {
  if (!selectedGroup.value) return [];
  return processes.value.filter(p => selectedGroup.value?.pids.includes(p.pid));
});

const columns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name', sorter: (a: any, b: any) => a.name.localeCompare(b.name) },
  { title: 'CPU', dataIndex: 'cpu', key: 'cpu', width: 100, sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'MEM', dataIndex: 'mem', key: 'mem', width: 100, sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 100, sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
  { title: 'User', dataIndex: 'user', width: 100 },
  { title: '', key: 'action', width: 100, fixed: 'right' as const }
];

const faultColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'Name', dataIndex: 'name', key: 'name' },
  { title: 'Hard/s', dataIndex: 'majorFaultRate', key: 'majorFaultRate', width: 120, sorter: (a: any, b: any) => a.majorFaultRate - b.majorFaultRate },
  { title: 'Page/s', dataIndex: 'minorFaultRate', key: 'minorFaultRate', width: 120, sorter: (a: any, b: any) => a.minorFaultRate - b.minorFaultRate },
  { title: 'Total/s', dataIndex: 'faultRate', key: 'faultRate', width: 120, sorter: (a: any, b: any) => a.faultRate - b.faultRate },
  { title: 'Hard Total', dataIndex: 'majorFaults', key: 'majorFaults', width: 120, sorter: (a: any, b: any) => a.majorFaults - b.majorFaults },
  { title: 'Page Total', dataIndex: 'minorFaults', key: 'minorFaults', width: 120, sorter: (a: any, b: any) => a.minorFaults - b.minorFaults },
  { title: 'Action', key: 'action', width: 80, fixed: 'right' as const }
];

const groupColumns = [
  { title: 'PID', dataIndex: 'pid', key: 'pid', width: 100, sorter: (a: any, b: any) => a.pid - b.pid },
  { title: 'CPU %', dataIndex: 'cpu', key: 'cpu', width: 100, sorter: (a: any, b: any) => a.cpu - b.cpu },
  { title: 'MEM %', dataIndex: 'mem', key: 'mem', width: 100, sorter: (a: any, b: any) => a.mem - b.mem },
  { title: 'VRAM', dataIndex: 'gpuMem', key: 'gpuMem', width: 100, sorter: (a: any, b: any) => a.gpuMem - b.gpuMem },
  { title: 'User', dataIndex: 'user', key: 'user' },
  { title: '', key: 'action', width: 100 }
];

const resetFilters = () => {
  cpuThreshold.value = 0; cpuMax.value = 100;
  memThreshold.value = 0; memMax.value = 100;
  gpuThreshold.value = 0; gpuMax.value = 8192;
  filterUser.value = null;
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
onUnmounted(() => {
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }
  ws = null;
});
watch(refreshInterval, connectWebSocket);
</script>

<template>
  <div style="background: #f0f2f5; padding: 20px; min-height: 100%;">
    <a-tabs v-model:activeKey="activeTab" type="card" class="monitor-tabs">
      
      <!-- HEALTH TAB -->
      <a-tab-pane key="dashboard" tab="Health">
        <template #tab><span><DashboardOutlined /> Health</span></template>
        <a-tabs v-model:activeKey="healthTab" type="card" size="small" class="health-subtabs">
          <a-tab-pane key="cpu">
            <template #tab><span><DashboardOutlined /> CPU</span></template>
            <a-row style="margin-bottom: 16px;">
              <a-col :span="24">
                <a-card size="small" class="stat-card-row">
                  <template #title>
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                      <span><DashboardOutlined /> CPU Status</span>
                      <div style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
                        <a-radio-group v-model:value="cpuViewMode" size="small">
                          <a-radio-button value="total">Overall</a-radio-button>
                          <a-radio-button value="cores">Cores</a-radio-button>
                        </a-radio-group>
                        <a-radio-group v-model:value="cpuDisplayMode" size="small">
                          <a-radio-button value="bar">Bar</a-radio-button>
                          <a-radio-button value="circle">Circle</a-radio-button>
                        </a-radio-group>
                      </div>
                    </div>
                  </template>
                  <div v-if="cpuViewMode === 'total'" @click="openChart('cpu_total', 'Global CPU Usage', 'single', ['Usage'])" style="display: flex; align-items: center; justify-content: center; min-height: 120px; gap: 32px; cursor: pointer;">
                    <template v-if="cpuDisplayMode === 'circle'">
                      <div class="cpu-total-circle-shell">
                        <a-progress type="dashboard" :percent="Math.round(systemStats.cpuTotal)" :width="110" :stroke-width="10" :showInfo="false" stroke-color="#1890ff" />
                        <span class="cpu-total-circle-value">{{ systemStats.cpuTotal.toFixed(1) }}%</span>
                      </div>
                      <div style="text-align: left;">
                        <div style="font-size: 24px; font-weight: bold; color: #1890ff;">{{ systemStats.cpuTotal.toFixed(1) }}% <LineChartOutlined style="font-size: 14px; color: #ccc" /></div>
                        <div style="color: #888;">Total System Load</div>
                      </div>
                    </template>
                    <template v-else>
                      <div style="display: flex; flex-direction: column; width: min(420px, 100%); gap: 10px;">
                        <div style="display: flex; justify-content: space-between; align-items: baseline;">
                          <div style="font-size: 24px; font-weight: bold; color: #1890ff;">{{ systemStats.cpuTotal.toFixed(1) }}%</div>
                          <div style="color: #888;">Total System Load</div>
                        </div>
                        <a-progress :percent="Math.round(systemStats.cpuTotal)" :showInfo="false" stroke-color="#1890ff" />
                        <div style="display: flex; justify-content: space-between; font-size: 11px; color: #aaa;">
                          <span>0%</span>
                          <span>{{ systemStats.cpuTotal.toFixed(1) }}%</span>
                          <span>100%</span>
                        </div>
                      </div>
                    </template>
                  </div>
                  <div v-else style="padding: 10px;">
                    <div v-if="pCores.length > 0">
                      <div style="font-size: 11px; color: #999; margin-bottom: 8px; font-weight: bold; border-left: 3px solid #1890ff; padding-left: 8px;">PERFORMANCE CORES (P-CORES)</div>
                      <div class="core-grid-full">
                        <div v-for="core in pCores" :key="core.index" @click="openChart('cpu_core_' + core.index, 'Core #' + core.index + ' Usage', 'single', ['Usage'])" :class="['core-item-full', { 'core-item-full--circle': cpuDisplayMode === 'circle' }]" style="cursor: pointer">
                          <span class="core-label">#{{ core.index }}</span>
                          <template v-if="cpuDisplayMode === 'circle'">
                            <div class="core-circle-shell">
                              <a-progress
                                type="circle"
                                :percent="Math.round(core.usage)"
                                :width="96"
                                :stroke-width="8"
                                :showInfo="false"
                                stroke-color="#1890ff"
                              />
                              <span class="core-circle-value">{{ core.usage.toFixed(1) }}%</span>
                            </div>
                          </template>
                          <template v-else>
                            <div style="flex: 1; margin: 0 10px; display: flex; align-items: center; justify-content: center;">
                              <a-progress :percent="Math.round(core.usage)" size="small" :showInfo="false" stroke-color="#1890ff" />
                            </div>
                            <span class="core-val">{{ core.usage.toFixed(1) }}%</span>
                          </template>
                        </div>
                      </div>
                    </div>
                    <div v-if="eCores.length > 0" style="margin-top: 16px;">
                      <div style="font-size: 11px; color: #999; margin-bottom: 8px; font-weight: bold; border-left: 3px solid #52c41a; padding-left: 8px;">EFFICIENCY CORES (E-CORES)</div>
                      <div class="core-grid-full">
                        <div v-for="core in eCores" :key="core.index" @click="openChart('cpu_core_' + core.index, 'Core #' + core.index + ' Usage', 'single', ['Usage'])" :class="['core-item-full', { 'core-item-full--circle': cpuDisplayMode === 'circle' }]" style="cursor: pointer">
                          <span class="core-label">#{{ core.index }}</span>
                          <template v-if="cpuDisplayMode === 'circle'">
                            <div class="core-circle-shell">
                              <a-progress
                                type="circle"
                                :percent="Math.round(core.usage)"
                                :width="96"
                                :stroke-width="8"
                                :showInfo="false"
                                stroke-color="#52c41a"
                              />
                              <span class="core-circle-value core-circle-value--green">{{ core.usage.toFixed(1) }}%</span>
                            </div>
                          </template>
                          <template v-else>
                            <div style="flex: 1; margin: 0 10px; display: flex; align-items: center; justify-content: center;">
                              <a-progress :percent="Math.round(core.usage)" size="small" :showInfo="false" stroke-color="#52c41a" />
                            </div>
                            <span class="core-val" style="color: #52c41a">{{ core.usage.toFixed(1) }}%</span>
                          </template>
                        </div>
                      </div>
                    </div>
                  </div>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <a-tab-pane key="memory">
            <template #tab><span><PieChartOutlined /> RAM / I/O</span></template>
            <a-row style="margin-bottom: 16px;">
              <a-col :span="24">
                <a-card size="small" class="stat-card-row" title="Memory & Interface I/O">
                  <template #extra><PieChartOutlined /></template>
                  <div class="monitor-io-shell">
                    <!-- RAM Breakdown -->
                    <div class="monitor-io-summary" @click="openChart('mem_usage', 'RAM Usage', 'single', ['Usage %'])">
                      <div style="margin-bottom: 10px;">
                        <div style="display: flex; justify-content: space-between; margin-bottom: 4px; font-size: 13px;">
                          <span>RAM Usage <LineChartOutlined style="font-size: 12px; color: #ccc" /></span>
                          <span style="font-weight: bold;">{{ systemStats.memPercent.toFixed(1) }}%</span>
                        </div>
                        <a-progress
                          :percent="[
                            ((systemStats.memUsed - systemStats.memCached - systemStats.memBuffers) / systemStats.memTotal) * 100,
                            (systemStats.memCached / systemStats.memTotal) * 100,
                            (systemStats.memBuffers / systemStats.memTotal) * 100,
                          ]"
                          :stroke-color="['#1890ff', '#52c41a', '#faad14']"
                          status="active"
                          :showInfo="false"
                        />
                        <div class="mem-legend">
                          <span><span class="dot" style="background: #1890ff"></span> Apps: {{ formatBytes(systemStats.memUsed - systemStats.memCached - systemStats.memBuffers) }}</span>
                          <span><span class="dot" style="background: #52c41a"></span> Cached: {{ formatBytes(systemStats.memCached) }}</span>
                          <span><span class="dot" style="background: #faad14"></span> Buffers: {{ formatBytes(systemStats.memBuffers) }}</span>
                        </div>
                        <div v-if="systemStats.zramTotal > 0" style="margin-top: 8px; font-size: 12px;">
                          ZRAM: {{ formatBytes(systemStats.zramUsed) }} / {{ formatBytes(systemStats.zramTotal) }}
                          <a-progress :percent="(systemStats.zramUsed / systemStats.zramTotal) * 100" size="small" />
                        </div>
                      </div>
                    </div>
                    <!-- I/O Details -->
                    <div class="monitor-io-panels">
                      <!-- Net Detail -->
                      <div class="monitor-io-panel">
                        <div class="monitor-io-panel__title">NETWORK INTERFACES</div>
                        <div class="monitor-io-panel__list">
                          <div v-for="s in systemStats.netInterfaces" :key="s.name" class="io-row" style="cursor: pointer"
                               @click="openChart('net_' + s.name, 'Interface: ' + s.name, 'double', ['Download', 'Upload'])">
                            <span class="io-name">{{ s.name }}</span>
                            <span class="io-val-in">↓{{ formatRateBytes(s.readSpeed) }}</span>
                            <span class="io-val-out">↑{{ formatRateBytes(s.writeSpeed) }}</span>
                          </div>
                          <div v-if="!systemStats.netInterfaces.length" style="font-size: 11px; color: #ccc;">No network interfaces detected</div>
                        </div>
                      </div>
                      <!-- Disk Detail -->
                      <div class="monitor-io-panel">
                        <div class="monitor-io-panel__title">DISK DEVICES</div>
                        <div class="monitor-io-panel__list">
                          <div v-for="s in systemStats.diskDevices" :key="s.name" class="io-row" style="cursor: pointer"
                               @click="openChart('disk_' + s.name, 'Disk: ' + s.name, 'double', ['Read', 'Write'])">
                            <span class="io-name">{{ s.name }}</span>
                            <span class="io-val-read">R:{{ formatRateBytes(s.readSpeed) }}</span>
                            <span class="io-val-write">W:{{ formatRateBytes(s.writeSpeed) }}</span>
                          </div>
                          <div v-if="!systemStats.diskDevices.length" style="font-size: 11px; color: #ccc;">No disk devices detected</div>
                        </div>
                      </div>
                    </div>
                  </div>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <a-tab-pane key="gpu">
            <template #tab><span><DeploymentUnitOutlined /> GPU</span></template>
            <!-- GPU Row -->
            <a-row>
              <a-col :span="24">
                <a-card size="small" class="stat-card-row" :title="gpus.length ? 'GPU Acceleration Status' : 'No GPU Detected'">
                  <template #extra><DeploymentUnitOutlined /></template>
                  <div style="display: flex; flex-wrap: wrap; gap: 16px; padding: 10px;">
                    <div v-for="gpu in gpus" :key="gpu.index" @click="openChart('gpu_' + gpu.index + '_util', 'GPU ' + gpu.index + ' Load', 'single', ['Load %'])" class="gpu-row-item" style="cursor: pointer; flex: 1; min-width: 500px;">
                      <div style="display: flex; align-items: center; gap: 24px; width: 100%;">
                        <div style="text-align: center; flex-shrink: 0;">
                          <a-tag v-if="gpu.temp > 0" color="volcano" style="margin-bottom: 4px;">{{ gpu.temp }}°C</a-tag>
                          <div style="font-size: 11px; font-weight: bold; color: #666;">GPU {{ gpu.index }}</div>
                        </div>
                        <div style="flex: 1;">
                          <div style="font-size: 13px; font-weight: bold; margin-bottom: 8px; color: #333;">{{ gpu.name }}</div>
                          <div style="display: flex; gap: 40px; align-items: center;">
                            <div style="text-align: center;">
                              <div style="font-size: 10px; color: #999; margin-bottom: 4px; text-transform: uppercase;">Core Util</div>
                              <a-progress type="circle" :percent="gpu.utilGpu" :width="65" :stroke-width="10" stroke-color="#13c2c2" :format="formatPercent" />
                            </div>
                            <div v-if="gpu.memTotal > 0" style="text-align: center;" @click.stop="openChart('gpu_' + gpu.index + '_vram', 'GPU ' + gpu.index + ' VRAM', 'single', ['Used MB'])">
                              <div style="font-size: 10px; color: #999; margin-bottom: 4px; text-transform: uppercase;">VRAM Usage</div>
                              <a-progress type="circle" :percent="Math.round((gpu.memUsed / gpu.memTotal) * 100)" :width="65" :stroke-width="10" stroke-color="#722ed1" :format="formatPercent" />
                            </div>
                            <div style="flex: 1; background: #f5f5f5; padding: 8px 12px; border-radius: 6px;">
                              <div style="display: flex; justify-content: space-between; font-size: 12px; margin-bottom: 4px;"><span style="color: #666;">Used:</span><span style="font-weight: bold; font-family: monospace;">{{ gpu.memUsed }} MiB</span></div>
                              <div v-if="gpu.memTotal > 0" style="display: flex; justify-content: space-between; font-size: 12px;"><span style="color: #666;">Total:</span><span style="font-family: monospace;">{{ gpu.memTotal }} MiB</span></div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                    <a-empty v-if="!gpus.length" :image="false" description="No GPU hardware detected (NVML or DRM)" style="width: 100%" />
                  </div>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <a-tab-pane key="faults">
            <template #tab><span><WarningOutlined /> Errors</span></template>
            <a-row style="margin-bottom: 16px;">
              <a-col :span="24">
                <a-card size="small" class="stat-card-row" title="Page Fault & Hard Error Overview">
                  <template #extra><WarningOutlined /></template>
                  <a-row :gutter="[16, 16]">
                    <a-col :xs="24" :sm="12" :lg="6">
                      <div style="cursor: pointer;" @click="openChart('fault_page_rate', 'Page Fault Rate', 'single', ['Faults/s'])">
                        <a-card size="small" :bordered="false" style="background: #f8fbff;">
                          <div style="font-size: 12px; color: #666; margin-bottom: 8px;">Page Faults</div>
                          <div style="font-size: 26px; font-weight: bold; color: #1890ff;">{{ formatRate(systemStats.faults.pageFaultRate) }}</div>
                          <div style="font-size: 12px; color: #888; margin-top: 6px;">Total {{ formatCount(systemStats.faults.pageFaults) }}</div>
                        </a-card>
                      </div>
                    </a-col>
                    <a-col :xs="24" :sm="12" :lg="6">
                      <div style="cursor: pointer;" @click="openChart('fault_major_rate', 'Hard Fault Rate', 'single', ['Faults/s'])">
                        <a-card size="small" :bordered="false" style="background: #fff7f7;">
                          <div style="font-size: 12px; color: #666; margin-bottom: 8px;">Hard Faults</div>
                          <div style="font-size: 26px; font-weight: bold; color: #cf1322;">{{ formatRate(systemStats.faults.majorFaultRate) }}</div>
                          <div style="font-size: 12px; color: #888; margin-top: 6px;">Total {{ formatCount(systemStats.faults.majorFaults) }}</div>
                        </a-card>
                      </div>
                    </a-col>
                    <a-col :xs="24" :sm="12" :lg="6">
                      <div style="cursor: pointer;" @click="openChart('fault_minor_rate', 'Soft Fault Rate', 'single', ['Faults/s'])">
                        <a-card size="small" :bordered="false" style="background: #fffaf0;">
                          <div style="font-size: 12px; color: #666; margin-bottom: 8px;">Soft / Minor Faults</div>
                          <div style="font-size: 26px; font-weight: bold; color: #d46b08;">{{ formatRate(systemStats.faults.minorFaultRate) }}</div>
                          <div style="font-size: 12px; color: #888; margin-top: 6px;">Total {{ formatCount(systemStats.faults.minorFaults) }}</div>
                        </a-card>
                      </div>
                    </a-col>
                    <a-col :xs="24" :sm="12" :lg="6">
                      <div style="cursor: pointer;" @click="openChart('fault_swap_rate', 'Swap Activity', 'double', ['Swap In', 'Swap Out'])">
                        <a-card size="small" :bordered="false" style="background: #fbf8ff;">
                          <div style="font-size: 12px; color: #666; margin-bottom: 8px;">Swap Activity</div>
                          <div style="font-size: 18px; font-weight: bold; color: #722ed1;">In {{ formatRate(systemStats.faults.swapInRate) }}</div>
                          <div style="font-size: 18px; font-weight: bold; color: #722ed1;">Out {{ formatRate(systemStats.faults.swapOutRate) }}</div>
                          <div style="font-size: 12px; color: #888; margin-top: 6px;">Total {{ formatCount(systemStats.faults.swapIn) }} / {{ formatCount(systemStats.faults.swapOut) }}</div>
                        </a-card>
                      </div>
                    </a-col>
                  </a-row>
                </a-card>
              </a-col>
            </a-row>
            <a-row>
              <a-col :span="24">
                <a-card size="small" class="stat-card-row" title="Top Faulting Processes">
                  <template #extra><LineChartOutlined /></template>
                  <a-table
                    :dataSource="faultProcesses"
                    :columns="faultColumns"
                    size="small"
                    :pagination="{ pageSize: 10 }"
                    rowKey="pid"
                    :scroll="{ x: 900 }"
                  >
                    <template #bodyCell="{ column, record }">
                      <template v-if="column.key === 'name'"><span class="mono">{{ record.name }}</span></template>
                      <template v-if="column.key === 'majorFaultRate'">
                        <a-tag color="red">{{ formatRate(record.majorFaultRate) }}</a-tag>
                      </template>
                      <template v-if="column.key === 'minorFaultRate'">
                        <a-tag color="orange">{{ formatRate(record.minorFaultRate) }}</a-tag>
                      </template>
                      <template v-if="column.key === 'faultRate'">
                        <a-tag color="blue">{{ formatRate(record.faultRate) }}</a-tag>
                      </template>
                      <template v-if="column.key === 'majorFaults'">{{ formatCount(record.majorFaults) }}</template>
                      <template v-if="column.key === 'minorFaults'">{{ formatCount(record.minorFaults) }}</template>
                      <template v-if="column.key === 'action'">
                        <a-button type="link" size="small" @click="openProcDetails(record)">
                          <template #icon><InfoCircleOutlined /></template>
                        </a-button>
                      </template>
                    </template>
                  </a-table>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>
        </a-tabs>
      </a-tab-pane>

      <!-- PROCESSES TAB -->
      <a-tab-pane key="processes" tab="Processes">
        <template #tab><span><BarChartOutlined /> Processes</span></template>
        <div style="background: #fff; padding: 16px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03);">
          <div style="display: flex; justify-content: space-between; margin-bottom: 12px; align-items: center;"><div style="display: flex; align-items: center; gap: 8px;"><a-input v-model:value="searchText" placeholder="Search..." style="width: 180px"><template #prefix><SearchOutlined /></template></a-input><a-radio-group v-model:value="viewMode" button-style="solid" size="small"><a-radio-button value="tree"><ClusterOutlined /></a-radio-button><a-radio-button value="list"><TableOutlined /></a-radio-button></a-radio-group><a-button size="small" @click="showAdvancedFilters = !showAdvancedFilters"><FilterOutlined /></a-button><a-select v-model:value="refreshInterval" size="small" style="width: 80px"><a-select-option :value="1000">1s</a-select-option><a-select-option :value="2000">2s</a-select-option><a-select-option :value="5000">5s</a-select-option></a-select><a-badge :status="isConnected ? 'success' : 'processing'" /></div><div style="display: flex; align-items: center; gap: 4px;"><span style="font-size: 12px;">Track as:</span><a-select v-model:value="selectedTag" size="small" style="width: 120px"><a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option></a-select></div></div>
          <a-card v-if="showAdvancedFilters" size="small" style="margin-bottom: 16px; background: #fafafa;">
            <a-row :gutter="[24, 12]" align="middle">
              <a-col :span="6">
                <div style="font-size: 11px; color: #888; display: flex; justify-content: space-between;">
                  <span>CPU Range %</span>
                  <span>{{ cpuThreshold }}% - {{ cpuMax }}%</span>
                </div>
                <a-slider v-model:value="cpuRange" :min="0" :max="100" range />
              </a-col>
              <a-col :span="6">
                <div style="font-size: 11px; color: #888; display: flex; justify-content: space-between;">
                  <span>Memory Range %</span>
                  <span>{{ memThreshold }}% - {{ memMax }}%</span>
                </div>
                <a-slider v-model:value="memRange" :min="0" :max="100" range />
              </a-col>
              <a-col :span="6">
                <div style="font-size: 11px; color: #888; display: flex; justify-content: space-between;">
                  <span>VRAM Range (MiB)</span>
                  <span>{{ gpuThreshold }} - {{ gpuMax }}</span>
                </div>
                <a-slider v-model:value="gpuRange" :min="0" :max="8192" range />
              </a-col>
              <a-col :span="4">
                <a-select v-model:value="filterUser" style="width: 100%" placeholder="Filter User" allowClear size="small">
                  <a-select-option v-for="user in uniqueUsers" :key="user" :value="user">{{ user }}</a-select-option>
                </a-select>
              </a-col>
              <a-col :span="2" style="text-align: right;">
                <a-button size="small" type="link" @click="resetFilters">Reset</a-button>
              </a-col>
            </a-row>
          </a-card>
          <a-table 
            :dataSource="displayData" :columns="columns" 
            size="small" :pagination="viewMode === 'list' ? { pageSize: 50 } : false" rowKey="pid" :scroll="{ y: 'calc(100vh - 400px)' }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'"><span class="mono">{{ record.name }}</span></template>
              <template v-if="column.dataIndex === 'cpu'"><span :style="{color: record.cpu > 10 ? 'red' : 'inherit'}">{{ record.cpu.toFixed(1) }}%</span></template>
              <template v-if="column.dataIndex === 'mem'">{{ record.mem.toFixed(1) }}%</template>
              <template v-if="column.dataIndex === 'gpuMem'">{{ record.gpuMem > 0 ? record.gpuMem + 'MB' : '-' }}</template>
              <template v-if="column.key === 'action'">
                <div style="display: flex; gap: 8px;">
                  <a-button type="link" size="small" @click="openProcDetails(record)"><InfoCircleOutlined /></a-button>
                  <a-button type="link" size="small" @click="addToRules(record)"><PlusOutlined /></a-button>
                </div>
              </template>
            </template>
          </a-table>
        </div>
      </a-tab-pane>

      <!-- MEMORY MAP TAB -->
      <a-tab-pane key="memmap" tab="Memory Map">
        <template #tab><span><AppstoreOutlined /> Memory Map</span></template>
        <div class="mem-container"><div v-for="g in memoryVisualizationData" :key="g.name" class="mem-block" @click="openGroupDetails(g.name, g.pids)" :style="{ backgroundColor: getMemColor(g.mem), flexGrow: g.mem, flexBasis: Math.max(10, g.mem * 2) + '%', minHeight: Math.max(60, Math.sqrt(g.mem) * 30) + 'px' }"><a-tooltip><template #title>App: {{ g.name }}<br/>Total Mem: {{ g.mem.toFixed(2) }}%<br/>Instances: {{ g.count }} (Click for details)</template><div class="mem-block-content"><div class="mem-name">{{ g.name }}</div><div class="mem-value">{{ g.mem.toFixed(1) }}%</div><div v-if="g.count > 1" class="mem-count">x{{ g.count }}</div></div></a-tooltip></div></div>
        <a-modal v-model:open="showGroupDetails" :title="'Instances: ' + selectedGroup?.name" :footer="null" width="800px">
          <a-table :dataSource="selectedGroupProcesses" :columns="groupColumns" size="small" :pagination="{ pageSize: 10 }">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'cpu'">{{ record.cpu.toFixed(1) }}%</template>
              <template v-if="column.key === 'mem'">{{ record.mem.toFixed(1) }}%</template>
              <template v-if="column.key === 'gpuMem'">{{ record.gpuMem > 0 ? record.gpuMem + 'MB' : '-' }}</template>
              <template v-if="column.key === 'action'"><div style="display: flex; gap: 8px;"><a-button type="link" size="small" @click="openProcDetails(record)"><InfoCircleOutlined /></a-button><a-button type="link" size="small" @click="addToRules(record)">Track</a-button></div></template>
            </template>
          </a-table>
        </a-modal>
      </a-tab-pane>
    </a-tabs>

    <!-- Chart Modal -->
    <a-modal v-model:open="showChartModal" :title="chartTitle" :footer="null" width="800px">
      <div style="margin-bottom: 16px; display: flex; justify-content: flex-end; align-items: center; gap: 10px;"><span style="font-size: 12px; color: #666;">Horizontal Time Axis:</span><a-select v-model:value="chartTimeRange" size="small" style="width: 150px"><a-select-option :value="60">Last 1 Minute</a-select-option><a-select-option :value="300">Last 5 Minutes</a-select-option><a-select-option :value="600">Last 10 Minutes</a-select-option><a-select-option :value="1800">Last 30 Minutes</a-select-option></a-select></div>
      <div v-if="showChartModal" style="background: #fff; padding: 10px; border-radius: 4px; border: 1px solid #f0f0f0;"><VueApexCharts type="line" height="350" :options="chartOptions" :series="chartSeries" /></div>
    </a-modal>

    <!-- Process Detail Modal -->
    <a-modal v-model:open="showProcModal" title="Process Details" :footer="null" width="600px">
      <a-descriptions bordered :column="1" size="small" v-if="selectedProc">
        <a-descriptions-item label="Name"><a-typography-text strong>{{ selectedProc.name }}</a-typography-text></a-descriptions-item>
        <a-descriptions-item label="PID"><span class="proc-detail-value">{{ formatDetailValue(selectedProc.pid) }}</span></a-descriptions-item>
        <a-descriptions-item label="PPID"><span class="proc-detail-value">{{ formatDetailValue(selectedProc.ppid) }}</span></a-descriptions-item>
        <a-descriptions-item label="User">{{ selectedProc.user }}</a-descriptions-item>
        <a-descriptions-item label="CPU Load">{{ selectedProc.cpu.toFixed(1) }}%</a-descriptions-item>
        <a-descriptions-item label="Memory Usage">{{ selectedProc.mem.toFixed(1) }}%</a-descriptions-item>
        <a-descriptions-item label="GPU VRAM">{{ selectedProc.gpuMem > 0 ? selectedProc.gpuMem + ' MiB' : 'None' }}</a-descriptions-item>
        <a-descriptions-item label="Minor Faults">{{ formatCount(selectedProc.minorFaults || 0) }}</a-descriptions-item>
        <a-descriptions-item label="Major Faults">{{ formatCount(selectedProc.majorFaults || 0) }}</a-descriptions-item>
        <a-descriptions-item v-if="selectedProc.faultRate !== undefined" label="Total Fault Rate">{{ formatRate(selectedProc.faultRate) }}</a-descriptions-item>
        <a-descriptions-item v-if="selectedProc.majorFaultRate !== undefined" label="Hard Fault Rate">{{ formatRate(selectedProc.majorFaultRate) }}</a-descriptions-item>
        <a-descriptions-item v-if="selectedProc.minorFaultRate !== undefined" label="Page Fault Rate">{{ formatRate(selectedProc.minorFaultRate) }}</a-descriptions-item>
        <a-descriptions-item label="Full Command"><div class="proc-command-block">{{ formatDetailValue(selectedProc.cmdline) }}</div></a-descriptions-item>
        <a-descriptions-item label="Started At">{{ new Date(selectedProc.createTime).toLocaleString() }}</a-descriptions-item>
      </a-descriptions>
    </a-modal>
  </div>
</template>

<style scoped>
.monitor-tabs :deep(.ant-tabs-nav) { margin-bottom: 12px; }
.health-subtabs :deep(.ant-tabs-nav) { margin-bottom: 12px; }
.stat-card-row { border-radius: 8px; overflow: hidden; box-shadow: 0 1px 2px rgba(0,0,0,0.03); }
.monitor-io-shell {
  display: flex;
  gap: 24px;
  align-items: stretch;
  min-height: clamp(380px, 46vh, 560px);
  padding: 12px 10px 10px;
}
.monitor-io-summary {
  flex: 0 0 clamp(300px, 28vw, 420px);
  min-width: 300px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  justify-content: center;
  border-right: 1px solid #f0f0f0;
  padding-right: 18px;
}
.monitor-io-panels {
  flex: 1;
  min-width: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.monitor-io-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}
.monitor-io-panel__title {
  font-size: 11px;
  color: #999;
  margin-bottom: 8px;
  font-weight: bold;
  letter-spacing: 0.04em;
}
.monitor-io-panel__list {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding-right: 4px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.cpu-total-circle-shell {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 110px;
  height: 110px;
  flex: 0 0 auto;
}
.cpu-total-circle-value {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: 'JetBrains Mono', monospace;
  font-size: 18px;
  font-weight: 700;
  color: #1890ff;
  pointer-events: none;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.85);
}
.core-grid-full {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  padding: 10px;
  overflow: visible;
}
.core-item-full {
  display: flex;
  align-items: center;
  min-height: 132px;
  background: #fafafa;
  padding: 8px 12px;
  border-radius: 4px;
  border: 1px solid #f0f0f0;
}
.core-item-full--circle {
  align-items: center;
  justify-content: flex-start;
  min-height: 176px;
  gap: 14px;
}
.core-label { font-size: 11px; color: #999; min-width: 35px; }
.core-val { font-size: 11px; font-family: monospace; min-width: 40px; text-align: right; color: #1890ff; font-weight: bold; }
.core-circle-shell {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  margin: 0 auto;
  flex: 1;
}
.core-circle-value {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: 'JetBrains Mono', monospace;
  font-size: 16px;
  font-weight: 700;
  color: #1890ff;
  pointer-events: none;
  text-shadow: 0 1px 2px rgba(255, 255, 255, 0.85);
}
.core-circle-value--green {
  color: #52c41a;
}
.mem-legend {
  display: flex;
  justify-content: space-around;
  gap: 12px;
  flex-wrap: wrap;
  font-size: 11px;
  color: #666;
  margin-top: 8px;
}
.mem-legend .dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; }
.io-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 12px; padding: 6px 8px; background: #f9f9f9; border-radius: 3px; font-family: monospace; }
.io-name { font-weight: bold; color: #555; overflow: hidden; text-overflow: ellipsis; max-width: 120px; flex: 1; min-width: 0; }
.io-val-in { color: #52c41a; } .io-val-out { color: #1890ff; }
.io-val-read { color: #faad14; } .io-val-write { color: #ff4d4f; }
.gpu-row-item { flex: 1; min-width: 400px; display: flex; align-items: center; gap: 16px; background: #fafafa; padding: 12px; border-radius: 6px; border: 1px solid #f0f0f0; }
.mono { font-family: 'JetBrains Mono', monospace; font-size: 12px; }
.proc-detail-value {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 6px;
  background: #f6f8fa;
  border: 1px solid #d9d9d9;
  color: #1f1f1f;
  font-family: 'JetBrains Mono', monospace;
  font-size: 13px;
  font-weight: 600;
  line-height: 1.4;
}

.proc-command-block {
  max-height: 140px;
  overflow-y: auto;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  padding: 10px 12px;
  color: #1f1f1f;
  font-family: 'JetBrains Mono', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.mem-container { display: flex; flex-wrap: wrap; gap: 8px; background: #fff; padding: 16px; border-radius: 8px; min-height: calc(100vh - 200px); align-content: flex-start; }
.mem-block { height: 80px; border-radius: 4px; padding: 8px; color: white; transition: all 0.3s; cursor: pointer; display: flex; flex-direction: column; justify-content: center; align-items: center; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
.mem-block:hover { transform: scale(1.02); box-shadow: 0 4px 8px rgba(0,0,0,0.2); z-index: 10; }
.mem-name { font-weight: bold; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; width: 100%; text-align: center; }
.mem-value { font-size: 14px; font-family: 'JetBrains Mono', monospace; }
.mem-count { font-size: 11px; opacity: 0.8; }

@supports not selector(:has(*)) {
  .core-item-full {
    min-height: 176px;
    align-items: center;
  }
}

@media (max-width: 1200px) {
  .monitor-io-shell {
    flex-direction: column;
  }

  .monitor-io-summary {
    flex: none;
    width: 100%;
    min-width: 0;
    border-right: 0;
    border-bottom: 1px solid #f0f0f0;
    padding-right: 0;
    padding-bottom: 16px;
  }

  .monitor-io-panels {
    grid-template-columns: 1fr;
  }
}
</style>
