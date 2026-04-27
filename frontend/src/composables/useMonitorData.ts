import { ref, computed } from 'vue';
import axios from 'axios';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

export interface GPUStatus {
  index: number; name: string; utilGpu: number; utilMem: number;
  memTotal: number; memUsed: number; temp: number;
}

export interface ProcessInfo {
  pid: number; ppid: number; name: string; cpu: number; mem: number;
  user: string; gpuMem: number; gpuId: number; gpuUtil: number;
  cmdline: string; createTime: number;
  minorFaults: number; majorFaults: number;
}

export interface IOSpeed {
  name: string;
  readSpeed: number;
  writeSpeed: number;
}

export interface GlobalStats {
  cpuTotal: number; cpuCores: number[];
  cpuCoresDetailed: { index: number; usage: number; type: number }[];
  memTotal: number; memUsed: number; memPercent: number;
  memCached: number; memBuffers: number; memShared: number;
  zramUsed: number; zramTotal: number;
  swapUsed: number; swapTotal: number;
  netInterfaces: IOSpeed[];
  diskDevices: IOSpeed[];
  totalNetRecv: number; totalNetSent: number;
  totalDiskRead: number; totalDiskWrite: number;
  faults: FaultInfo;
}

export interface FaultInfo {
  pageFaults: number; majorFaults: number; minorFaults: number;
  pageFaultRate: number; majorFaultRate: number; minorFaultRate: number;
  swapIn: number; swapOut: number; swapInRate: number; swapOutRate: number;
}

export interface ByteScale {
  divisor: number;
  unit: 'B' | 'KB' | 'MB' | 'GB' | 'TB';
  precision: number;
}

export interface StatsHistory {
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
}

function createEmptyHistory(): StatsHistory {
  return {
    cpu: [], cores: {}, mem: [], memUsed: [], memCached: [], memBuffers: [],
    swapUsage: [], zramUsage: [], netRecv: [], netSent: [], diskRead: [], diskWrite: [],
    faults: [], swapIn: [], swapOut: [], netDevices: {}, diskDevices: {}
  };
}

export function useMonitorData() {
  const processes = ref<ProcessInfo[]>([]);
  const gpus = ref<GPUStatus[]>([]);
  const loading = ref(false);
  const tags = ref<string[]>([]);
  const trackedCommsNames = ref<string[]>([]);
  const trackedLoading = ref(false);

  const systemStats = ref<GlobalStats>({
    cpuTotal: 0, cpuCores: [], cpuCoresDetailed: [], memTotal: 0, memUsed: 0, memPercent: 0,
    memCached: 0, memBuffers: 0, memShared: 0, zramUsed: 0, zramTotal: 0,
    swapUsed: 0, swapTotal: 0, netInterfaces: [], diskDevices: [],
    totalNetRecv: 0, totalNetSent: 0, totalDiskRead: 0, totalDiskWrite: 0,
    faults: { pageFaults: 0, majorFaults: 0, minorFaults: 0, pageFaultRate: 0, majorFaultRate: 0, minorFaultRate: 0, swapIn: 0, swapOut: 0, swapInRate: 0, swapOutRate: 0 }
  });

  const statsHistory = ref<StatsHistory>(createEmptyHistory());

  const faultTopN = ref(5);
  const mergeFaultProcesses = ref(true);

  let ws: WebSocket | null = null;
  let reconnectTimer: any = null;
  let shouldReconnect = true;

  const cpuView = ref<'overall' | 'cores'>('cores');

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

  const trackedProcesses = computed(() => {
    if (trackedCommsNames.value.length === 0) return [];
    return processes.value.filter(p => trackedCommsNames.value.includes(p.name));
  });

  const getCoreTypeColor = (type: number) => type === pb.CPUInfo.Core.Type.PERFORMANCE ? '#1890ff' : '#52c41a';
  const getCoreTypeName = (type: number) => type === pb.CPUInfo.Core.Type.PERFORMANCE ? 'P-Core' : 'E-Core';

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

      systemStats.value.cpuCoresDetailed.forEach(core => {
        if (!statsHistory.value.cores[core.index]) statsHistory.value.cores[core.index] = [];
        statsHistory.value.cores[core.index].push({ time: now, value: core.usage });
        if (statsHistory.value.cores[core.index].length > 60) statsHistory.value.cores[core.index].shift();
      });

      systemStats.value.netInterfaces.forEach(iface => {
        if (!statsHistory.value.netDevices[iface.name]) statsHistory.value.netDevices[iface.name] = { recv: [], sent: [] };
        const dev = statsHistory.value.netDevices[iface.name];
        dev.recv.push({ time: now, value: iface.readSpeed });
        dev.sent.push({ time: now, value: iface.writeSpeed });
        if (dev.recv.length > 60) dev.recv.shift();
        if (dev.sent.length > 60) dev.sent.shift();
      });

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

      Object.keys(statsHistory.value).forEach((key) => {
        const val = (statsHistory.value as any)[key];
        if (Array.isArray(val)) {
          if (val.length > 60) val.shift();
        }
      });
    };
    socket.onclose = () => { if (shouldReconnect) reconnectTimer = setTimeout(connectWebSocket, 3000); };
  };

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
      return true;
    } catch (err: any) {
      return false;
    }
  };

  // History chart modal
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

  // Process maps modal
  const showProcessMapsModal = ref(false);
  const selectedProcessMaps = ref('');
  const selectedProcessDetails = ref<any>(null);
  const processMapsLoading = ref(false);

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

  // Lifecycle
  const setup = () => {
    loading.value = true;
    axios.get('/config/tags').then(res => tags.value = res.data);
    connectWebSocket();
  };

  const teardown = () => {
    shouldReconnect = false;
    if (ws) ws.close();
    if (reconnectTimer) clearTimeout(reconnectTimer);
  };

  return {
    // state
    processes, gpus, systemStats, statsHistory, loading, tags,
    trackedCommsNames, trackedLoading,
    faultTopN, mergeFaultProcesses, cpuView,
    // computed
    groupedNetInterfaces, groupedDiskDevices, topFaultProcesses, trackedProcesses,
    // methods
    byteScale, formatBytesWithUnit, getCoreTypeColor, getCoreTypeName,
    connectWebSocket, fetchTrackedComms, sendProcessSignal,
    openHistoryChart,
    showProcessDetails,
    // history modal
    showHistoryModal, historyModalTitle, historySeries, historyChartOptions,
    // process maps modal
    showProcessMapsModal, selectedProcessMaps, selectedProcessDetails, processMapsLoading,
    // lifecycle
    setup, teardown
  };
}
