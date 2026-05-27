import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue';
import { pb } from '../../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../../utils/requestContext';

export interface IOSpeed { name: string; readSpeed: number; writeSpeed: number; }
export interface InterfaceSample { time: number; readSpeed: number; writeSpeed: number; }
export type NetworkSnapshot = Record<string, { r: number; s: number }>;

export interface RateScale { divisor: number; unit: string; precision: number; }

const MAX_HISTORY_SECONDS = 300;
const MEGABYTE = 1024 * 1024;

export const VueApexCharts = defineAsyncComponent(
  async () => (await import('vue3-apexcharts')).default as any
) as any;

export const formatBytes = (value: number | string, decimals = 2) => {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), sizes.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : decimals)} ${sizes[index]}`;
};

export const formatRate = (bytesPerSecond: number) => `${formatBytes(bytesPerSecond)}/s`;

export const resolveRateScale = (maxBytesPerSecond: number): RateScale => {
  const v = Math.max(0, maxBytesPerSecond);
  if (v >= 1024**4) return { divisor: 1024**4, unit: 'TB/s', precision: 1 };
  if (v >= 1024**3) return { divisor: 1024**3, unit: 'GB/s', precision: 1 };
  if (v >= 1024**2) return { divisor: 1024**2, unit: 'MB/s', precision: 1 };
  if (v >= 1024) return { divisor: 1024, unit: 'KB/s', precision: 1 };
  return { divisor: 1, unit: 'B/s', precision: 0 };
};

export const getTrafficLevelColor = (bps: number) => {
  if (bps >= 10 * MEGABYTE) return 'red';
  if (bps >= MEGABYTE) return 'gold';
  return 'green';
};

export const getTrafficLevelLabel = (bps: number) => {
  if (bps >= 10 * MEGABYTE) return 'hot';
  if (bps >= MEGABYTE) return 'busy';
  return 'steady';
};

export const protocolColor = (p?: string) => {
  switch ((p || '').toUpperCase()) {
    case 'HTTP': return 'blue';
    case 'TLS': case 'HTTPS/TLS': return 'geekblue';
    case 'DNS': case 'MDNS': case 'LLMNR': return 'purple';
    case 'SSH': return 'volcano';
    case 'QUIC': return 'cyan';
    case 'SSDP': return 'orange';
    case 'NTP': return 'gold';
    case 'SNMP': return 'green';
    case 'NETBIOS': return 'red';
    case 'DHCP': return 'lime';
    default: return 'default';
  }
};

export const stateColor = (s: string) => {
  switch (s) {
    case 'ESTABLISHED': return 'green';
    case 'SYN_SENT': case 'SYN_RECV': return 'orange';
    case 'FIN_WAIT1': case 'FIN_WAIT2': case 'CLOSING': return 'gold';
    case 'TIME_WAIT': case 'CLOSE_WAIT': case 'LAST_ACK': return 'volcano';
    default: return 'default';
  }
};

export const staleColor = (level?: string) => {
  switch (level) {
    case 'active': return 'green';
    case 'warning': return 'gold';
    case 'critical': return 'red';
    case 'historic': return 'default';
    default: return 'default';
  }
};

export const riskColor = (score: number) => {
  if (score >= 0.8) return 'red';
  if (score >= 0.6) return 'orange';
  if (score >= 0.3) return 'gold';
  return 'green';
};

export function useInterfaceMonitor() {
  const isConnected = ref(false);
  const wsTimeRange = ref(60);
  const refreshInterval = ref(2000);
  const interfaceHistory = ref<Record<string, InterfaceSample[]>>({});
  const interfaceNames = ref<string[]>([]);
  const cumRecv = ref(0);
  const cumSent = ref(0);
  const showInterfaceChartModal = ref(false);
  const selectedInterfaceName = ref('');
  const interfaceChartTimeRange = ref(60);
  let lastIO: { networks: NetworkSnapshot; time: number } | null = null;
  let ws: WebSocket | null = null;
  let reconnectTimer: number | null = null;
  let shouldReconnect = true;

  const pad2 = (v: number) => String(Math.floor(Math.abs(v))).padStart(2, '0');
  const formatChartTime = (ts: number, rangeS: number) => {
    const d = new Date(ts);
    const hh = pad2(d.getHours()), mm = pad2(d.getMinutes()), ss = pad2(d.getSeconds());
    if (rangeS <= 120) return `${hh}:${mm}:${ss}`;
    if (rangeS <= 1800) return `${hh}:${mm}`;
    return `${pad2(d.getMonth()+1)}-${pad2(d.getDate())} ${hh}:${mm}`;
  };

  const pruneSamples = (samples: InterfaceSample[]) => {
    const minTime = Date.now() - MAX_HISTORY_SECONDS * 1000;
    return samples.filter(s => s.time >= minTime);
  };

  const rememberSample = (name: string, sample: InterfaceSample) => {
    const prev = interfaceHistory.value[name] || [];
    interfaceHistory.value[name] = pruneSamples([...prev, sample]);
  };

  const averageSpeed = (samples: InterfaceSample[], key: 'readSpeed' | 'writeSpeed') =>
    samples.length ? samples.reduce((s, x) => s + x[key], 0) / samples.length : 0;

  const netInterfaces = computed<IOSpeed[]>(() => {
    const minTime = Date.now() - wsTimeRange.value * 1000;
    return interfaceNames.value
      .map(name => {
        const samples = (interfaceHistory.value[name] || []).filter(s => s.time >= minTime);
        return { name, readSpeed: averageSpeed(samples, 'readSpeed'), writeSpeed: averageSpeed(samples, 'writeSpeed') };
      })
      .sort((a, b) => (b.readSpeed + b.writeSpeed) - (a.readSpeed + a.writeSpeed) || a.name.localeCompare(b.name, undefined, { numeric: true }));
  });

  const totalNetRecv = computed(() => netInterfaces.value.reduce((s, i) => s + i.readSpeed, 0));
  const totalNetSent = computed(() => netInterfaces.value.reduce((s, i) => s + i.writeSpeed, 0));

  const openInterfaceChart = (name: string) => {
    selectedInterfaceName.value = name;
    showInterfaceChartModal.value = true;
  };

  const selectedInterfaceHistory = computed(() =>
    selectedInterfaceName.value ? interfaceHistory.value[selectedInterfaceName.value] || [] : []);

  const interfaceChartWindow = computed(() => {
    const data = selectedInterfaceHistory.value;
    const max = data.length ? data[data.length - 1].time : Date.now();
    return { min: max - interfaceChartTimeRange.value * 1000, max };
  });

  const interfaceChartSamples = computed(() => {
    const { min } = interfaceChartWindow.value;
    return selectedInterfaceHistory.value.filter(s => s.time >= min);
  });

  const interfaceChartRateScale = computed(() => {
    const maxRate = interfaceChartSamples.value.reduce((peak, s) => Math.max(peak, s.readSpeed, s.writeSpeed), 0);
    return resolveRateScale(maxRate);
  });

  const interfaceChartOptions = computed(() => {
    const { min, max } = interfaceChartWindow.value;
    const scale = interfaceChartRateScale.value;
    return {
      chart: { animations: { enabled: false }, toolbar: { show: false }, zoom: { enabled: false }, background: 'transparent' },
      colors: ['#1890ff', '#52c41a'],
      xaxis: {
        type: 'datetime' as const, min, max,
        labels: { datetimeUTC: false, style: { fontSize: '10px' }, formatter: (v: any) => formatChartTime(Number(v), interfaceChartTimeRange.value) },
        range: interfaceChartTimeRange.value * 1000, tickAmount: 6,
      },
      yaxis: {
        min: 0, forceNiceScale: true, decimalsInFloat: scale.precision,
        labels: { style: { fontSize: '10px' }, formatter: (v: any) => formatRate(Number(v) * scale.divisor) },
      },
      tooltip: {
        x: { formatter: (v: any) => formatChartTime(Number(v), interfaceChartTimeRange.value) },
        y: { formatter: (v: number) => formatRate(Number(v) * scale.divisor) },
      },
      stroke: { curve: 'smooth' as const, width: 2 },
      grid: { borderColor: '#f1f1f1' },
      legend: { position: 'top' as const, horizontalAlign: 'right' as const },
    };
  });

  const interfaceChartSeries = computed(() => {
    const scale = interfaceChartRateScale.value;
    return [
      { name: 'Download', data: interfaceChartSamples.value.map(s => ({ x: s.time, y: s.readSpeed / scale.divisor })) },
      { name: 'Upload', data: interfaceChartSamples.value.map(s => ({ x: s.time, y: s.writeSpeed / scale.divisor })) },
    ];
  });

  const connectWebSocket = () => {
    if (ws) { ws.onopen = null; ws.onmessage = null; ws.onclose = null; ws.close(); }
    lastIO = null; interfaceHistory.value = {}; interfaceNames.value = [];
    const socket = new WebSocket(buildWebSocketUrl('/ws/system', { interval: refreshInterval.value }));
    ws = socket;
    socket.binaryType = 'arraybuffer';
    socket.onopen = () => { if (ws === socket) isConnected.value = true; };
    socket.onmessage = (msg) => {
      if (ws !== socket) return;
      try {
        const decoded = pb.SystemStats.decode(new Uint8Array(msg.data));
        const now = Date.now();
        if (decoded.io) {
          const networkList = decoded.io.networks || [];
          interfaceNames.value = networkList.map((n: any) => n.name);
          const dt = lastIO ? (now - lastIO.time) / 1000 : 0;
          let curRecv = 0, curSent = 0;
          networkList.forEach((n: any) => {
            const rs = Number(n.recvBytes), ws = Number(n.sentBytes);
            rememberSample(n.name, { time: now, readSpeed: rs, writeSpeed: ws });
            curRecv += rs; curSent += ws;
          });
          if (dt > 0) { cumRecv.value += curRecv * dt; cumSent.value += curSent * dt; }
          const nets: NetworkSnapshot = {};
          networkList.forEach((n: any) => { nets[n.name] = { r: Number(n.recvBytes), s: Number(n.sentBytes) }; });
          lastIO = { networks: nets, time: now };
        }
      } catch { /* decode error */ }
    };
    socket.onclose = () => {
      if (ws !== socket) return;
      isConnected.value = false; ws = null;
      if (!shouldReconnect) return;
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      reconnectTimer = window.setTimeout(connectWebSocket, 3000);
    };
  };

  const disconnectWebSocket = () => {
    shouldReconnect = false;
    if (reconnectTimer !== null) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    if (ws) { ws.onopen = null; ws.onmessage = null; ws.onclose = null; ws.close(); }
    ws = null;
  };

  return {
    isConnected,
    wsTimeRange,
    refreshInterval,
    interfaceHistory,
    interfaceNames,
    cumRecv,
    cumSent,
    showInterfaceChartModal,
    selectedInterfaceName,
    interfaceChartTimeRange,
    netInterfaces,
    totalNetRecv,
    totalNetSent,
    selectedInterfaceHistory,
    interfaceChartWindow,
    interfaceChartSamples,
    interfaceChartRateScale,
    interfaceChartOptions,
    interfaceChartSeries,
    openInterfaceChart,
    connectWebSocket,
    disconnectWebSocket,
  };
}
