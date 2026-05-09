<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref } from 'vue';
import {
  GlobalOutlined, ArrowDownOutlined, ArrowUpOutlined,
  DashboardOutlined, NodeIndexOutlined, WifiOutlined, AlertOutlined,
} from '@ant-design/icons-vue';
import { useNetworkEnrichment, type NetworkFlow } from '../composables/useNetworkEnrichment';
import { useNetworkInterfaces } from '../composables/useNetworkInterfaces';
import TrafficGraph from '../components/TrafficGraph.vue';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

// ── WebSocket interface monitoring (kept from original) ─────────────
interface IOSpeed { name: string; readSpeed: number; writeSpeed: number; }
interface InterfaceSample { time: number; readSpeed: number; writeSpeed: number; }
type NetworkSnapshot = Record<string, { r: number; s: number }>;

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

const maxHistorySeconds = 300;
const megabyte = 1024 * 1024;
const VueApexCharts = defineAsyncComponent(async () => (await import('vue3-apexcharts')).default as any) as any;

// ── Flow enrichment ────────────────────────────────────────────────
const {
  flows, tcpConns, loading: flowsLoading, error: flowsError,
  fetchFlows, fetchTCPState,
  totalBytesOut, totalBytesIn, suspiciousFlows, publicFlows, establishedConns,
} = useNetworkEnrichment(5000);

// ── Interface stats from REST API ──────────────────────────────────
const {
  interfaces: apiInterfaces, dnsMap,
  fetchInterfaces, fetchDNSCache,
  totalErrors, totalDrops,
} = useNetworkInterfaces(5000);

// ── Tab state ──────────────────────────────────────────────────────
const activeTab = ref('overview');
const selectedFlow = ref<NetworkFlow | null>(null);
const showFlowDetail = ref(false);

// ── Flow filter state ──────────────────────────────────────────────
const filterQuery = ref('');
const showHistoric = ref(false);
const sortKey = ref('lastSeen');
const filterError = ref('');

const filterExamples = ['process:curl', 'dport:443', 'sni:github.com', 'state:ESTABLISHED', 'risk:0.7'];

const validateFilter = (query: string) => {
  const allowed = new Set([
    'port', 'dport', 'sport', 'src', 'dst', 'process', 'comm', 'pid',
    'agent', 'task', 'tool', 'sni', 'host', 'domain', 'service', 'app',
    'state', 'proto', 'transport', 'scope', 'risk',
  ]);
  const invalid = query.trim().split(/\s+/).filter(Boolean)
    .filter((t) => t.includes(':') && !allowed.has(t.split(':', 1)[0].toLowerCase()));
  return invalid.length ? `Unknown filter: ${invalid.join(', ')}` : '';
};

const flowParams = computed(() => {
  const params: Record<string, string> = {
    showHistoric: String(showHistoric.value),
    sort: sortKey.value,
    limit: '100',
  };
  const f = filterQuery.value.trim();
  if (f) params.filter = f;
  return params;
});

const refreshFlows = async () => {
  filterError.value = validateFilter(filterQuery.value);
  if (!filterError.value) await fetchFlows(flowParams.value);
  await fetchTCPState();
};

const applyFilterExample = (example: string) => {
  const tokens = filterQuery.value.trim().split(/\s+/).filter(Boolean);
  if (!tokens.includes(example)) tokens.push(example);
  filterQuery.value = tokens.join(' ');
  void refreshFlows();
};

// ── Overview computed ──────────────────────────────────────────────
const topProcesses = computed(() => {
  const counts: Record<string, number> = {};
  for (const f of flows.value) {
    for (const c of f.processComms) {
      counts[c] = (counts[c] || 0) + 1;
    }
  }
  return Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 10);
});

const riskSummary = computed(() => ({
  high: flows.value.filter(f => f.riskScore >= 0.8).length,
  medium: flows.value.filter(f => f.riskScore >= 0.5 && f.riskScore < 0.8).length,
  low: flows.value.filter(f => f.riskScore > 0 && f.riskScore < 0.5).length,
}));

const flowProtocols = computed(() => {
  const counts: Record<string, number> = {};
  for (const f of flows.value) {
    const p = f.appProtocol || 'Unknown';
    counts[p] = (counts[p] || 0) + 1;
  }
  return Object.entries(counts).sort((a, b) => b[1] - a[1]);
});

// ── Format helpers ─────────────────────────────────────────────────
const formatBytes = (value: number | string, decimals = 2) => {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), sizes.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : decimals)} ${sizes[index]}`;
};

const formatRate = (bytesPerSecond: number) => `${formatBytes(bytesPerSecond)}/s`;

interface RateScale { divisor: number; unit: string; precision: number; }
const resolveRateScale = (maxBytesPerSecond: number): RateScale => {
  const v = Math.max(0, maxBytesPerSecond);
  if (v >= 1024**4) return { divisor: 1024**4, unit: 'TB/s', precision: 1 };
  if (v >= 1024**3) return { divisor: 1024**3, unit: 'GB/s', precision: 1 };
  if (v >= 1024**2) return { divisor: 1024**2, unit: 'MB/s', precision: 1 };
  if (v >= 1024) return { divisor: 1024, unit: 'KB/s', precision: 1 };
  return { divisor: 1, unit: 'B/s', precision: 0 };
};

const getTrafficLevelColor = (bps: number) => {
  if (bps >= 10 * megabyte) return 'red';
  if (bps >= megabyte) return 'gold';
  return 'green';
};

const getTrafficLevelLabel = (bps: number) => {
  if (bps >= 10 * megabyte) return 'hot';
  if (bps >= megabyte) return 'busy';
  return 'steady';
};

const protocolColor = (p?: string) => {
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

const stateColor = (s: string) => {
  switch (s) {
    case 'ESTABLISHED': return 'green';
    case 'SYN_SENT': case 'SYN_RECV': return 'orange';
    case 'FIN_WAIT1': case 'FIN_WAIT2': case 'CLOSING': return 'gold';
    case 'TIME_WAIT': case 'CLOSE_WAIT': case 'LAST_ACK': return 'volcano';
    default: return 'default';
  }
};

const staleColor = (level?: string) => {
  switch (level) {
    case 'active': return 'green';
    case 'warning': return 'gold';
    case 'critical': return 'red';
    case 'historic': return 'default';
    default: return 'default';
  }
};

const riskColor = (score: number) => {
  if (score >= 0.8) return 'red';
  if (score >= 0.6) return 'orange';
  if (score >= 0.3) return 'gold';
  return 'green';
};

// ── WebSocket interface monitoring ─────────────────────────────────
const pad2 = (v: number) => String(Math.floor(Math.abs(v))).padStart(2, '0');
const formatChartTime = (ts: number, rangeS: number) => {
  const d = new Date(ts);
  const hh = pad2(d.getHours()), mm = pad2(d.getMinutes()), ss = pad2(d.getSeconds());
  if (rangeS <= 120) return `${hh}:${mm}:${ss}`;
  if (rangeS <= 1800) return `${hh}:${mm}`;
  return `${pad2(d.getMonth()+1)}-${pad2(d.getDate())} ${hh}:${mm}`;
};

const pruneSamples = (samples: InterfaceSample[]) => {
  const minTime = Date.now() - maxHistorySeconds * 1000;
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

// ── Flow detail ────────────────────────────────────────────────────
const openFlowDetail = (flow: NetworkFlow) => {
  selectedFlow.value = flow;
  showFlowDetail.value = true;
};

// ── Lifecycle ──────────────────────────────────────────────────────
let flowTimer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  shouldReconnect = true;
  connectWebSocket();
  void refreshFlows();
  void fetchInterfaces();
  fetchDNSCache();
  flowTimer = setInterval(() => { void refreshFlows(); void fetchInterfaces(); }, 5000);
});

onUnmounted(() => {
  disconnectWebSocket();
  if (flowTimer !== null) { clearInterval(flowTimer); flowTimer = null; }
});

// ── Flow table columns ─────────────────────────────────────────────
const flowColumns = [
  { title: 'Destination', dataIndex: 'dstIp', key: 'dst', width: 200 },
  { title: 'Port', dataIndex: 'dstPort', key: 'port', width: 70 },
  { title: 'Protocol', dataIndex: 'appProtocol', key: 'app', width: 110 },
  { title: 'Domain/DPI', dataIndex: 'dstDomain', key: 'domain', width: 220 },
  { title: 'Scope', dataIndex: 'ipScope', key: 'scope', width: 90 },
  { title: 'Process', dataIndex: 'comm', key: 'comm', width: 120 },
  { title: 'Out', dataIndex: 'bytesOut', key: 'out', width: 90 },
  { title: 'Rate', dataIndex: 'currentBpsOut', key: 'rate', width: 90 },
  { title: 'State', dataIndex: 'staleLevel', key: 'stale', width: 90 },
  { title: 'Risk', dataIndex: 'riskScore', key: 'risk', width: 70 },
];

const flowData = computed(() =>
  flows.value.map(f => ({ ...f, comm: f.processComms[0] || '-', key: f.flowId || `${f.srcIp}:${f.srcPort}->${f.dstIp}:${f.dstPort}` })));

const tcpColumns = [
  { title: 'Source', dataIndex: 'srcIp', key: 'src', width: 150 },
  { title: 'Destination', dataIndex: 'dstIp', key: 'dst', width: 150 },
  { title: 'Port', dataIndex: 'dstPort', key: 'port', width: 70 },
  { title: 'State', dataIndex: 'state', key: 'state', width: 120 },
  { title: 'Process', dataIndex: 'comm', key: 'comm', width: 120 },
];

// ── Graph resize ───────────────────────────────────────────────────
const graphHeight = ref(420);
const isResizing = ref(false);
const startResize = (e: MouseEvent) => {
  isResizing.value = true;
  const startY = e.clientY, startH = graphHeight.value;
  const onMove = (me: MouseEvent) => { if (isResizing.value) graphHeight.value = Math.max(200, Math.min(1200, startH + me.clientY - startY)); };
  const onUp = () => { isResizing.value = false; document.removeEventListener('mousemove', onMove); document.removeEventListener('mouseup', onUp); };
  document.addEventListener('mousemove', onMove);
  document.addEventListener('mouseup', onUp);
};
</script>

<template>
  <div style="padding: 20px; background: #f0f2f5; min-height: 100%;">
    <!-- ── Stat cards (always visible) ──────────────────────────── -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #e6f7ff;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <ArrowDownOutlined style="font-size: 24px; color: #1890ff;" />
            <div>
              <div style="font-size: 12px; color: #666;">Download</div>
              <div style="font-size: 22px; font-weight: bold; color: #1890ff;">{{ formatBytes(totalNetRecv, 1) }}/s</div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #f6ffed;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <ArrowUpOutlined style="font-size: 24px; color: #52c41a;" />
            <div>
              <div style="font-size: 12px; color: #666;">Upload</div>
              <div style="font-size: 22px; font-weight: bold; color: #52c41a;">{{ formatBytes(totalNetSent, 1) }}/s</div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #fff7e6;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <GlobalOutlined style="font-size: 24px; color: #fa8c16;" />
            <div>
              <div style="font-size: 12px; color: #666;">Active Flows</div>
              <div style="font-size: 22px; font-weight: bold; color: #fa8c16;">{{ flows.length }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #f9f0ff;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <AlertOutlined style="font-size: 24px; color: #722ed1;" />
            <div>
              <div style="font-size: 12px; color: #666;">Suspicious</div>
              <div style="font-size: 22px; font-weight: bold; color: #722ed1;">{{ suspiciousFlows().length }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- ── Connectivity bar ────────────────────────────────────── -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :span="24">
        <a-card size="small" :bordered="false">
          <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap; justify-content: space-between;">
            <div style="display: flex; align-items: center; gap: 12px;">
              <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
              <a-divider type="vertical" />
              <span style="font-size: 12px; color: #475569;">
                Session: ↓{{ formatBytes(cumRecv, 1) }} ↑{{ formatBytes(cumSent, 1) }}
              </span>
              <span style="font-size: 12px; color: #475569;">
                TCP Est: {{ establishedConns().length }}
              </span>
              <span style="font-size: 12px; color: #475569;">
                DNS: {{ dnsMap.length }}
              </span>
            </div>
            <a-radio-group v-model:value="wsTimeRange" size="small" button-style="solid">
              <a-radio-button :value="30">30s</a-radio-button>
              <a-radio-button :value="60">60s</a-radio-button>
              <a-radio-button :value="120">2m</a-radio-button>
              <a-radio-button :value="300">5m</a-radio-button>
            </a-radio-group>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- ── Tabbed workspace ────────────────────────────────────── -->
    <a-card size="small" :bordered="false">
      <a-tabs v-model:activeKey="activeTab" size="small">
        <!-- ── Overview tab ──────────────────────────────────── -->
        <a-tab-pane key="overview">
          <template #tab>
            <span><DashboardOutlined /> Overview</span>
          </template>
          <a-row :gutter="[16, 16]">
            <!-- Traffic graph -->
            <a-col :span="24">
              <a-card title="Traffic Graph" size="small">
                <div style="position: relative;">
                  <TrafficGraph :interfaces="netInterfaces" :height="graphHeight" @select-interface="openInterfaceChart" />
                  <div
                    style="position: absolute; bottom: 0; left: 0; right: 0; height: 8px; cursor: ns-resize; background: rgba(0,0,0,0.02); display: flex; justify-content: center; align-items: center;"
                    @mousedown="startResize"
                  >
                    <div style="width: 32px; height: 3px; background: #d9d9d9; border-radius: 2px;" />
                  </div>
                </div>
              </a-card>
            </a-col>
            <!-- Flow summary -->
            <a-col :xs="24" :lg="12">
              <a-card title="Top Processes" size="small">
                <a-table
                  :data-source="topProcesses"
                  :columns="[{ title: 'Process', dataIndex: '0', key: 'name' }, { title: 'Flows', dataIndex: '1', key: 'count', align: 'right' }]"
                  :pagination="false" size="small" row-key="0"
                  :locale="{ emptyText: 'No flows yet' }"
                />
              </a-card>
            </a-col>
            <a-col :xs="24" :lg="12">
              <a-card title="Protocols" size="small">
                <a-table
                  :data-source="flowProtocols"
                  :columns="[{ title: 'Protocol', dataIndex: '0', key: 'name' }, { title: 'Count', dataIndex: '1', key: 'count', align: 'right' }]"
                  :pagination="false" size="small" row-key="0"
                  :locale="{ emptyText: 'No flows yet' }"
                >
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.key === 'name'">
                      <a-tag :color="protocolColor(record[0])" size="small">{{ record[0] }}</a-tag>
                    </template>
                  </template>
                </a-table>
              </a-card>
            </a-col>
            <!-- Risk summary -->
            <a-col :span="24">
              <a-card title="Risk Distribution" size="small">
                <div style="display: flex; gap: 24px; align-items: center;">
                  <div style="display: flex; align-items: center; gap: 8px;">
                    <a-tag color="red">High</a-tag>
                    <span style="font-size: 20px; font-weight: bold;">{{ riskSummary.high }}</span>
                  </div>
                  <div style="display: flex; align-items: center; gap: 8px;">
                    <a-tag color="orange">Medium</a-tag>
                    <span style="font-size: 20px; font-weight: bold;">{{ riskSummary.medium }}</span>
                  </div>
                  <div style="display: flex; align-items: center; gap: 8px;">
                    <a-tag color="gold">Low</a-tag>
                    <span style="font-size: 20px; font-weight: bold;">{{ riskSummary.low }}</span>
                  </div>
                  <a-divider type="vertical" />
                  <span style="font-size: 12px; color: #666;">
                    Public: {{ publicFlows().length }} | ↓{{ formatBytes(totalBytesIn()) }} ↑{{ formatBytes(totalBytesOut()) }}
                  </span>
                </div>
              </a-card>
            </a-col>
          </a-row>
        </a-tab-pane>

        <!-- ── Flows tab ─────────────────────────────────────── -->
        <a-tab-pane key="flows">
          <template #tab>
            <span><NodeIndexOutlined /> Flows</span>
          </template>
          <!-- Filter bar -->
          <div style="margin-bottom: 12px;">
            <a-space wrap>
              <a-input-search
                v-model:value="filterQuery"
                placeholder="process:curl dport:443 sni:github.com state:ESTABLISHED"
                allow-clear size="small" style="width: 420px"
                @search="refreshFlows" @press-enter="refreshFlows"
              />
              <a-select v-model:value="sortKey" size="small" style="width: 140px" @change="refreshFlows">
                <a-select-option value="lastSeen">Recently Updated</a-select-option>
                <a-select-option value="risk">Risk Priority</a-select-option>
                <a-select-option value="bandwidth">Bandwidth</a-select-option>
                <a-select-option value="+dst">By Destination</a-select-option>
              </a-select>
              <a-switch v-model:checked="showHistoric" size="small" checked-children="Historic" un-checked-children="Active" @change="refreshFlows" />
              <a-button size="small" @click="refreshFlows">Refresh</a-button>
            </a-space>
            <div style="margin-top: 8px; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; font-size: 12px; color: #64748b;">
              <span>Quick:</span>
              <a-tag v-for="ex in filterExamples" :key="ex" class="filter-chip" @click="applyFilterExample(ex)">{{ ex }}</a-tag>
            </div>
            <a-alert v-if="filterError" type="warning" show-icon :message="filterError" style="margin-top: 8px;" />
          </div>

          <a-tabs size="small">
            <a-tab-pane key="flows-table" tab="Aggregated Flows">
              <a-table
                :columns="flowColumns" :data-source="flowData"
                :pagination="{ pageSize: 20, size: 'small' }" size="small" row-key="key"
                :loading="flowsLoading"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'dst'">
                    <div style="display:flex;flex-direction:column;gap:2px;">
                      <span style="font-family:monospace;font-size:12px;cursor:pointer;color:#1677ff;" @click="openFlowDetail(record)">{{ record.dstIp }}</span>
                      <span style="font-family:monospace;font-size:11px;color:#94a3b8;">{{ record.srcIp }}:{{ record.srcPort }}</span>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'app'">
                    <a-space :size="4" wrap>
                      <a-tag v-if="record.appProtocol" :color="protocolColor(record.appProtocol)" size="small">{{ record.appProtocol }}</a-tag>
                      <a-tag v-if="record.dstService" color="blue" size="small">{{ record.dstService }}</a-tag>
                    </a-space>
                  </template>
                  <template v-else-if="column.key === 'domain'">
                    <div style="display:flex;flex-direction:column;gap:4px;">
                      <span v-if="record.dstDomain" style="color:#1890ff">{{ record.dstDomain }}</span>
                      <a-space :size="4" wrap>
                        <a-tag v-if="record.sni" color="geekblue" size="small">SNI {{ record.sni }}</a-tag>
                        <a-tag v-if="record.httpHost" color="cyan" size="small">{{ record.httpMethod || 'HTTP' }} {{ record.httpHost }}</a-tag>
                        <a-tag v-if="record.dnsName && record.dnsName !== record.dstDomain" color="purple" size="small">DNS {{ record.dnsName }}</a-tag>
                        <a-tag v-if="record.tlsAlpn" color="blue" size="small">ALPN {{ record.tlsAlpn }}</a-tag>
                      </a-space>
                    </div>
                  </template>
                  <template v-else-if="column.key === 'scope'">
                    <a-tag :color="record.ipScope === 'Public' ? 'orange' : record.ipScope === 'Private' ? 'green' : 'default'" size="small">{{ record.ipScope }}</a-tag>
                  </template>
                  <template v-else-if="column.key === 'out'">{{ formatBytes(record.bytesOut) }}</template>
                  <template v-else-if="column.key === 'rate'">
                    <span style="font-family:monospace;font-size:12px;">↑{{ formatRate(record.currentBpsOut || 0) }}</span>
                  </template>
                  <template v-else-if="column.key === 'stale'">
                    <a-tag :color="staleColor(record.staleLevel)" size="small">{{ record.staleLevel || 'active' }}</a-tag>
                  </template>
                  <template v-else-if="column.key === 'risk'">
                    <a-tooltip :title="(record.riskReasons || []).join('; ')">
                      <a-tag :color="riskColor(record.riskScore)" size="small">{{ record.riskLevel || 'risk' }} {{ (record.riskScore * 100).toFixed(0) }}%</a-tag>
                    </a-tooltip>
                  </template>
                </template>
              </a-table>
            </a-tab-pane>
            <a-tab-pane key="tcp" tab="TCP State">
              <a-table
                :columns="tcpColumns" :data-source="tcpConns"
                :pagination="{ pageSize: 20, size: 'small' }" size="small" row-key="key"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'state'">
                    <a-badge :color="stateColor(record.state)" :text="record.state" />
                  </template>
                </template>
              </a-table>
            </a-tab-pane>
          </a-tabs>
          <div v-if="flowsError" style="color:#ff4d4f;margin-top:8px;">{{ flowsError }}</div>
        </a-tab-pane>

        <!-- ── Interfaces tab ────────────────────────────────── -->
        <a-tab-pane key="interfaces">
          <template #tab>
            <span><WifiOutlined /> Interfaces</span>
          </template>
          <a-table
            :data-source="netInterfaces"
            :columns="[
              { title: 'Interface', dataIndex: 'name', key: 'name' },
              { title: 'Download', dataIndex: 'readSpeed', key: 'readSpeed', align: 'right' },
              { title: 'Upload', dataIndex: 'writeSpeed', key: 'writeSpeed', align: 'right' },
              { title: 'Total', key: 'total', align: 'right' },
              { title: 'Level', key: 'level', align: 'center' },
              { title: 'Errors', key: 'errors', align: 'right' },
              { title: 'Drops', key: 'drops', align: 'right' },
            ]"
            :pagination="false" size="small" row-key="name"
            :locale="{ emptyText: 'No interfaces detected' }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'name'">
                <span style="color:#1677ff;cursor:pointer;" @click="openInterfaceChart(record.name)">{{ record.name }}</span>
              </template>
              <template v-else-if="column.key === 'readSpeed'">
                <span style="color:#1890ff;">{{ formatBytes(record.readSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'writeSpeed'">
                <span style="color:#52c41a;">{{ formatBytes(record.writeSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'total'">
                <span style="font-weight:600;">{{ formatBytes(record.readSpeed + record.writeSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'level'">
                <a-tag :color="getTrafficLevelColor(record.readSpeed + record.writeSpeed)">{{ getTrafficLevelLabel(record.readSpeed + record.writeSpeed) }}</a-tag>
              </template>
              <template v-else-if="column.key === 'errors'">
                {{ (apiInterfaces.find(i => i.name === record.name)?.errin || 0) + (apiInterfaces.find(i => i.name === record.name)?.errout || 0) }}
              </template>
              <template v-else-if="column.key === 'drops'">
                {{ (apiInterfaces.find(i => i.name === record.name)?.dropin || 0) + (apiInterfaces.find(i => i.name === record.name)?.dropout || 0) }}
              </template>
            </template>
          </a-table>
          <div v-if="totalErrors() > 0 || totalDrops() > 0" style="margin-top: 12px; display: flex; gap: 12px;">
            <a-tag v-if="totalErrors() > 0" color="red">Total Errors: {{ totalErrors() }}</a-tag>
            <a-tag v-if="totalDrops() > 0" color="orange">Total Drops: {{ totalDrops() }}</a-tag>
          </div>
        </a-tab-pane>
      </a-tabs>
    </a-card>

    <!-- ── Flow detail modal ──────────────────────────────────── -->
    <a-modal v-model:open="showFlowDetail" title="Flow Detail" :footer="null" width="800px">
      <template v-if="selectedFlow">
        <a-descriptions :column="2" size="small" bordered>
          <a-descriptions-item label="Flow ID">{{ selectedFlow.flowId }}</a-descriptions-item>
          <a-descriptions-item label="Transport">{{ selectedFlow.transport || selectedFlow.protocol }}</a-descriptions-item>
          <a-descriptions-item label="Source">{{ selectedFlow.srcIp }}:{{ selectedFlow.srcPort }}</a-descriptions-item>
          <a-descriptions-item label="Destination">{{ selectedFlow.dstIp }}:{{ selectedFlow.dstPort }}</a-descriptions-item>
          <a-descriptions-item label="App Protocol">
            <a-tag v-if="selectedFlow.appProtocol" :color="protocolColor(selectedFlow.appProtocol)" size="small">{{ selectedFlow.appProtocol }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Service">{{ selectedFlow.dstService || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Domain">{{ selectedFlow.dstDomain || selectedFlow.dnsName || '-' }}</a-descriptions-item>
          <a-descriptions-item label="SNI">{{ selectedFlow.sni || '-' }}</a-descriptions-item>
          <a-descriptions-item label="HTTP Host">{{ selectedFlow.httpHost || '-' }}</a-descriptions-item>
          <a-descriptions-item label="TLS ALPN">{{ selectedFlow.tlsAlpn || '-' }}</a-descriptions-item>
          <a-descriptions-item label="IP Scope">
            <a-tag :color="selectedFlow.ipScope === 'Public' ? 'orange' : selectedFlow.ipScope === 'Private' ? 'green' : 'default'" size="small">{{ selectedFlow.ipScope }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Direction">{{ selectedFlow.direction }}</a-descriptions-item>
          <a-descriptions-item label="State">{{ selectedFlow.state || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Stale">
            <a-tag :color="staleColor(selectedFlow.staleLevel)" size="small">{{ selectedFlow.staleLevel }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Bytes In">{{ formatBytes(selectedFlow.bytesIn) }}</a-descriptions-item>
          <a-descriptions-item label="Bytes Out">{{ formatBytes(selectedFlow.bytesOut) }}</a-descriptions-item>
          <a-descriptions-item label="Current Rate">
            ↓{{ formatRate(selectedFlow.currentBpsIn || 0) }} ↑{{ formatRate(selectedFlow.currentBpsOut || 0) }}
          </a-descriptions-item>
          <a-descriptions-item label="Peak Rate">
            ↓{{ formatRate(selectedFlow.peakBpsIn || 0) }} ↑{{ formatRate(selectedFlow.peakBpsOut || 0) }}
          </a-descriptions-item>
          <a-descriptions-item label="Risk">
            <a-tag :color="riskColor(selectedFlow.riskScore)">{{ (selectedFlow.riskScore * 100).toFixed(0) }}% {{ selectedFlow.riskLevel }}</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Historic">{{ selectedFlow.historic ? 'Yes' : 'No' }}</a-descriptions-item>
          <a-descriptions-item label="Processes" :span="2">{{ (selectedFlow.processComms || []).join(', ') || '-' }}</a-descriptions-item>
          <a-descriptions-item label="PIDs" :span="2">{{ (selectedFlow.processPids || []).join(', ') }}</a-descriptions-item>
          <a-descriptions-item label="Agent Run IDs" :span="2">{{ (selectedFlow.agentRunIds || []).join(', ') || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Task IDs" :span="2">{{ (selectedFlow.taskIds || []).join(', ') || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Tool Call IDs" :span="2">{{ (selectedFlow.toolCallIds || []).join(', ') || '-' }}</a-descriptions-item>
          <a-descriptions-item label="Risk Reasons" :span="2">
            <a-space wrap>
              <a-tag v-for="r in (selectedFlow.riskReasons || [])" :key="r" color="volcano" size="small">{{ r }}</a-tag>
            </a-space>
          </a-descriptions-item>
        </a-descriptions>
      </template>
    </a-modal>

    <!-- ── Interface chart modal ──────────────────────────────── -->
    <a-modal
      v-model:open="showInterfaceChartModal"
      :title="selectedInterfaceName ? `Interface History: ${selectedInterfaceName}` : 'Interface History'"
      :footer="null" width="900px"
    >
      <div style="margin-bottom:16px;display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:12px;">
        <a-radio-group v-model:value="interfaceChartTimeRange" size="small" button-style="solid">
          <a-radio-button :value="30">30s</a-radio-button>
          <a-radio-button :value="60">60s</a-radio-button>
          <a-radio-button :value="120">2m</a-radio-button>
          <a-radio-button :value="300">5m</a-radio-button>
        </a-radio-group>
      </div>
      <div v-if="showInterfaceChartModal" style="background:#fff;padding:10px;border-radius:4px;border:1px solid #f0f0f0;">
        <VueApexCharts type="line" height="360" :options="interfaceChartOptions" :series="interfaceChartSeries" />
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.filter-chip { cursor: pointer; }
</style>
