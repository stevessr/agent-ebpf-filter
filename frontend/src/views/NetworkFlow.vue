<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { LoadingOutlined, GlobalOutlined, ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons-vue';
import VueApexCharts from 'vue3-apexcharts';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

interface IOSpeed {
  name: string;
  readSpeed: number;
  writeSpeed: number;
}

interface HistoryData {
  time: number;
  value: number;
  value2?: number;
}

const isConnected = ref(false);
const netInterfaces = ref<IOSpeed[]>([]);
const totalNetRecv = ref(0);
const totalNetSent = ref(0);
const cumRecv = ref(0);
const cumSent = ref(0);
let lastCumRecv = 0;
let lastCumSent = 0;
const historyMap = ref<Record<string, HistoryData[]>>({});
const selectedInterface = ref('__all__');
const timeRange = ref(60);
const refreshInterval = ref(2000);

let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let shouldReconnect = true;
let lastIO: { networks: Record<string, { r: number; s: number }>; time: number } | null = null;

const formatBytes = (bytes: number, decimals = 2) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(decimals)) + ' ' + sizes[i];
};

const interfaceOptions = computed(() => {
  const names = netInterfaces.value.map(n => ({ label: n.name, value: n.name }));
  return [{ label: 'All Interfaces', value: '__all__' }, ...names];
});

const activeInterfaces = computed(() => netInterfaces.value.length);

const getChartKey = (iface: string) => iface === '__all__' ? 'total_net' : `net_${iface}`;

const chartData = computed(() => {
  const key = getChartKey(selectedInterface.value);
  const data = historyMap.value[key] || [];
  const now = Date.now();
  const min = now - timeRange.value * 1000;
  const valid = data.filter(d => d.time >= min);
  return [
    { name: 'Download', data: valid.map(d => ({ x: d.time, y: d.value })) },
    { name: 'Upload', data: valid.map(d => ({ x: d.time, y: d.value2 || 0 })) },
  ];
});

const chartOptions = computed(() => {
  const now = Date.now();
  const min = now - timeRange.value * 1000;
  return {
    chart: {
      animations: { enabled: false },
      toolbar: { show: true, autoSelected: 'pan' as const },
      zoom: { type: 'x' as const, enabled: true },
      background: 'transparent',
    },
    colors: ['#1890ff', '#52c41a'],
    xaxis: {
      type: 'datetime' as const,
      min,
      max: now,
      labels: {
        datetimeUTC: false,
        style: { fontSize: '11px' },
        formatter: (value: string | number) => {
          const d = new Date(Number(value));
          return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`;
        },
      },
      tickAmount: 8,
    },
    yaxis: {
      labels: {
        style: { fontSize: '11px' },
        formatter: (v: number) => formatBytes(v, 1) + '/s',
      },
    },
    stroke: { curve: 'smooth' as const, width: 2 },
    fill: {
      type: 'gradient' as const,
      gradient: {
        shadeIntensity: 1,
        opacityFrom: 0.5,
        opacityTo: 0.1,
      },
    },
    dataLabels: { enabled: false },
    grid: { borderColor: '#f0f0f0' },
    legend: { position: 'top' as const, horizontalAlign: 'right' as const },
    tooltip: {
      x: { format: 'HH:mm:ss' },
      y: {
        formatter: (v: number) => formatBytes(v, 2) + '/s',
      },
    },
    theme: { mode: 'light' as const },
    title: {
      text: selectedInterface.value === '__all__'
        ? 'Total Network Traffic'
        : `Interface: ${selectedInterface.value}`,
      align: 'left' as const,
      style: { fontSize: '14px', fontWeight: '600' },
    },
  };
});

const connectWebSocket = () => {
  if (ws) ws.close();
  lastIO = null;
  historyMap.value = {};
  ws = new WebSocket(buildWebSocketUrl('/ws/system', { interval: refreshInterval.value }));
  ws.binaryType = 'arraybuffer';

  ws.onopen = () => { isConnected.value = true; };
  ws.onmessage = (msg) => {
    try {
      const decoded = pb.SystemStats.decode(new Uint8Array(msg.data));
      const now = Date.now();
      const newSpeeds: IOSpeed[] = [];

      const updateHistory = (key: string, val: number, val2?: number) => {
        if (!historyMap.value[key]) historyMap.value[key] = [];
        historyMap.value[key].push({ time: now, value: val, value2: val2 });
        const maxPoints = Math.max(300, Math.ceil(timeRange.value * 1000 / refreshInterval.value) * 2);
        if (historyMap.value[key].length > maxPoints) historyMap.value[key].shift();
      };

      if (lastIO && decoded.io) {
        const dt = (now - lastIO.time) / 1000;
        let totalR = 0, totalS = 0;
        (decoded.io.networks || []).forEach((n: any) => {
          const prev = lastIO?.networks[n.name];
          if (prev) {
            const rin = (Number(n.recvBytes) - prev.r) / dt;
            const rout = (Number(n.sentBytes) - prev.s) / dt;
            if (rin > 0 || rout > 0) {
              newSpeeds.push({ name: n.name, readSpeed: rin, writeSpeed: rout });
            }
            totalR += rin; totalS += rout;
            updateHistory(`net_${n.name}`, rin, rout);
          }
        });
        totalNetRecv.value = totalR;
        totalNetSent.value = totalS;
        updateHistory('total_net', totalR, totalS);
      }

      if (decoded.io) {
        const nets: Record<string, { r: number; s: number }> = {};
        (decoded.io.networks || []).forEach((n: any) => {
          nets[n.name] = { r: Number(n.recvBytes), s: Number(n.sentBytes) };
        });
        lastIO = { networks: nets, time: now };
        netInterfaces.value = newSpeeds;
      }

      // Cumulative totals
      if (decoded.io) {
        let curR = 0, curS = 0;
        (decoded.io.networks || []).forEach((n: any) => {
          curR += Number(n.recvBytes);
          curS += Number(n.sentBytes);
        });
        if (lastCumRecv > 0) {
          cumRecv.value += curR - lastCumRecv;
          cumSent.value += curS - lastCumSent;
        }
        lastCumRecv = curR;
        lastCumSent = curS;
      }
    } catch (e) { console.error(e); }
  };
  ws.onclose = () => {
    isConnected.value = false;
    if (!shouldReconnect) return;
    if (reconnectTimer !== null) clearTimeout(reconnectTimer);
    reconnectTimer = window.setTimeout(connectWebSocket, 3000);
  };
};

const disconnectWebSocket = () => {
  shouldReconnect = false;
  if (reconnectTimer !== null) { clearTimeout(reconnectTimer); reconnectTimer = null; }
  ws?.close();
  ws = null;
};

onMounted(() => { shouldReconnect = true; connectWebSocket(); });
onUnmounted(() => { disconnectWebSocket(); });
</script>

<template>
  <div style="padding: 20px; background: #f0f2f5; min-height: 100%;">
    <!-- Summary Cards -->
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
              <div style="font-size: 12px; color: #666;">Active Interfaces</div>
              <div style="font-size: 22px; font-weight: bold; color: #fa8c16;">{{ activeInterfaces }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card size="small" :bordered="false" style="background: #f9f0ff;">
          <div style="display: flex; align-items: center; gap: 12px;">
            <LoadingOutlined style="font-size: 24px; color: #722ed1;" />
            <div>
              <div style="font-size: 12px; color: #666;">Session Total</div>
              <div style="font-size: 18px; font-weight: bold; color: #722ed1;">↓{{ formatBytes(cumRecv, 1) }} ↑{{ formatBytes(cumSent, 1) }}</div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- Controls -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :span="24">
        <a-card size="small" :bordered="false">
          <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
            <span style="font-weight: 600; font-size: 13px;">Interface:</span>
            <a-select v-model:value="selectedInterface" style="width: 200px;" size="small" :options="interfaceOptions" />
            <a-divider type="vertical" />
            <span style="font-weight: 600; font-size: 13px;">Time Window:</span>
            <a-radio-group v-model:value="timeRange" size="small" button-style="solid">
              <a-radio-button :value="30">30s</a-radio-button>
              <a-radio-button :value="60">60s</a-radio-button>
              <a-radio-button :value="120">2m</a-radio-button>
              <a-radio-button :value="300">5m</a-radio-button>
            </a-radio-group>
            <a-divider type="vertical" />
            <a-badge :status="isConnected ? 'success' : 'error'" :text="isConnected ? 'Connected' : 'Disconnected'" />
          </div>
        </a-card>
      </a-col>
    </a-row>

    <!-- Chart -->
    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :span="24">
        <a-card size="small" :bordered="false">
          <VueApexCharts
            :type="'area'"
            :height="380"
            :options="chartOptions"
            :series="chartData"
          />
        </a-card>
      </a-col>
    </a-row>

    <!-- Interface Details Table -->
    <a-row :gutter="[16, 16]">
      <a-col :span="24">
        <a-card title="Interface Details" size="small" :bordered="false">
          <template #extra>
            <a-tag color="blue">{{ netInterfaces.length }} active</a-tag>
          </template>
          <a-table
            :data-source="netInterfaces"
            :columns="[
              { title: 'Interface', dataIndex: 'name', key: 'name' },
              { title: 'Download', dataIndex: 'readSpeed', key: 'readSpeed', align: 'right' as const },
              { title: 'Upload', dataIndex: 'writeSpeed', key: 'writeSpeed', align: 'right' as const },
              { title: 'Total', key: 'total', align: 'right' as const },
            ]"
            :pagination="false"
            size="small"
            row-key="name"
            :locale="{ emptyText: 'No active traffic' }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'readSpeed'">
                <span style="color: #1890ff;">{{ formatBytes(record.readSpeed, 2) }}/s</span>
              </template>
              <template v-if="column.key === 'writeSpeed'">
                <span style="color: #52c41a;">{{ formatBytes(record.writeSpeed, 2) }}/s</span>
              </template>
              <template v-if="column.key === 'total'">
                <span style="font-weight: 500;">{{ formatBytes(record.readSpeed + record.writeSpeed, 2) }}/s</span>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>
