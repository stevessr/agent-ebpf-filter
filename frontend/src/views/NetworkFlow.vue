<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref } from 'vue';
import { LoadingOutlined, GlobalOutlined, ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons-vue';
import TrafficGraph from '../components/TrafficGraph.vue';
import { pb } from '../pb/tracker_pb.js';
import { buildWebSocketUrl } from '../utils/requestContext';

interface IOSpeed {
  name: string;
  readSpeed: number;
  writeSpeed: number;
}

interface InterfaceSample {
  time: number;
  readSpeed: number;
  writeSpeed: number;
}

type NetworkSnapshot = Record<string, { r: number; s: number }>;

const isConnected = ref(false);
const timeRange = ref(60);
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
const VueApexCharts = defineAsyncComponent(() => import('vue3-apexcharts'));

const formatBytes = (value: number | string, decimals = 2) => {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), sizes.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : decimals)} ${sizes[index]}`;
};

interface RateScale {
  divisor: number;
  unit: string;
  precision: number;
}

const resolveRateScale = (maxBytesPerSecond: number): RateScale => {
  const value = Math.max(0, maxBytesPerSecond);
  if (value >= 1024 ** 4) return { divisor: 1024 ** 4, unit: 'TB/s', precision: 1 };
  if (value >= 1024 ** 3) return { divisor: 1024 ** 3, unit: 'GB/s', precision: 1 };
  if (value >= 1024 ** 2) return { divisor: 1024 ** 2, unit: 'MB/s', precision: 1 };
  if (value >= 1024) return { divisor: 1024, unit: 'KB/s', precision: 1 };
  return { divisor: 1, unit: 'B/s', precision: 0 };
};

const formatRate = (bytesPerSecond: number, scale: RateScale) => {
  const value = bytesPerSecond / scale.divisor;
  if (!Number.isFinite(value)) return `0 ${scale.unit}`;
  return `${value.toFixed(scale.precision)} ${scale.unit}`;
};

const getTrafficLevelColor = (bytesPerSecond: number) => {
  if (bytesPerSecond >= 10 * megabyte) return 'red';
  if (bytesPerSecond >= megabyte) return 'gold';
  return 'green';
};

const getTrafficLevelLabel = (bytesPerSecond: number) => {
  if (bytesPerSecond >= 10 * megabyte) return 'hot';
  if (bytesPerSecond >= megabyte) return 'busy';
  return 'steady';
};

const pad2 = (value: number) => String(Math.floor(Math.abs(value))).padStart(2, '0');

const formatChartTime = (timestamp: number, rangeSeconds: number) => {
  const date = new Date(timestamp);
  const hh = pad2(date.getHours());
  const mm = pad2(date.getMinutes());
  const ss = pad2(date.getSeconds());

  if (rangeSeconds <= 120) {
    return `${hh}:${mm}:${ss}`;
  }

  if (rangeSeconds <= 1800) {
    return `${hh}:${mm}`;
  }

  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${hh}:${mm}`;
};

const pruneSamples = (samples: InterfaceSample[]) => {
  const minTime = Date.now() - maxHistorySeconds * 1000;
  return samples.filter((sample) => sample.time >= minTime);
};

const rememberSample = (name: string, sample: InterfaceSample) => {
  const existing = interfaceHistory.value[name] || [];
  interfaceHistory.value[name] = pruneSamples([...existing, sample]);
};

const averageSpeed = (samples: InterfaceSample[], key: 'readSpeed' | 'writeSpeed') => {
  if (!samples.length) return 0;
  return samples.reduce((sum, sample) => sum + sample[key], 0) / samples.length;
};

const selectedInterfaceHistory = computed(() => {
  if (!selectedInterfaceName.value) return [];
  return interfaceHistory.value[selectedInterfaceName.value] || [];
});

const selectedInterface = computed(() => netInterfaces.value.find((item) => item.name === selectedInterfaceName.value) || null);

const interfaceChartWindow = computed(() => {
  const data = selectedInterfaceHistory.value;
  const max = data.length ? data[data.length - 1].time : Date.now();
  const min = max - (interfaceChartTimeRange.value * 1000);
  return { min, max };
});

const interfaceChartSamples = computed(() => {
  const { min } = interfaceChartWindow.value;
  return selectedInterfaceHistory.value.filter((sample) => sample.time >= min);
});

const interfaceChartRateScale = computed(() => {
  const maxRate = interfaceChartSamples.value.reduce((peak, sample) => (
    Math.max(peak, sample.readSpeed, sample.writeSpeed)
  ), 0);
  return resolveRateScale(maxRate);
});

const interfaceChartOptions = computed(() => {
  const { min, max } = interfaceChartWindow.value;
  const scale = interfaceChartRateScale.value;
  return {
    chart: {
      animations: { enabled: false },
      toolbar: { show: false },
      zoom: { enabled: false },
      background: 'transparent',
    },
    colors: ['#1890ff', '#52c41a'],
    xaxis: {
      type: 'datetime' as const,
      min,
      max,
      labels: {
        datetimeUTC: false,
        style: { fontSize: '10px' },
        formatter: (value: string | number) => formatChartTime(Number(value), interfaceChartTimeRange.value),
      },
      tooltip: {
        enabled: true,
        formatter: (value: string | number) => formatChartTime(Number(value), interfaceChartTimeRange.value),
      },
      range: interfaceChartTimeRange.value * 1000,
      tickAmount: 6,
    },
    tooltip: {
      x: {
        formatter: (value: string | number) => formatChartTime(Number(value), interfaceChartTimeRange.value),
      },
      y: {
        formatter: (value: number) => formatRate(Number(value) * scale.divisor, scale),
      },
    },
    yaxis: {
      min: 0,
      forceNiceScale: true,
      decimalsInFloat: scale.precision,
      labels: {
        style: { fontSize: '10px' },
        formatter: (value: number | string) => formatRate(Number(value) * scale.divisor, scale),
      },
    },
    stroke: { curve: 'smooth' as const, width: 2 },
    grid: { borderColor: '#f1f1f1' },
    legend: { position: 'top' as const, horizontalAlign: 'right' as const },
    theme: { mode: 'light' as const },
  };
});

const interfaceChartSeries = computed(() => {
  const scale = interfaceChartRateScale.value;
  return [
    {
      name: 'Download',
      data: interfaceChartSamples.value.map((sample) => ({ x: sample.time, y: sample.readSpeed / scale.divisor })),
    },
    {
      name: 'Upload',
      data: interfaceChartSamples.value.map((sample) => ({ x: sample.time, y: sample.writeSpeed / scale.divisor })),
    },
  ];
});

const openInterfaceChart = (name: string) => {
  selectedInterfaceName.value = name;
  showInterfaceChartModal.value = true;
};

const netInterfaces = computed<IOSpeed[]>(() => {
  const minTime = Date.now() - timeRange.value * 1000;
  return interfaceNames.value
    .map((name) => {
      const samples = (interfaceHistory.value[name] || []).filter((sample) => sample.time >= minTime);
      const readSpeed = averageSpeed(samples, 'readSpeed');
      const writeSpeed = averageSpeed(samples, 'writeSpeed');
      return { name, readSpeed, writeSpeed };
    })
    .sort((a, b) => (
      (b.readSpeed + b.writeSpeed) - (a.readSpeed + a.writeSpeed)
      || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' })
    ));
});

const totalNetRecv = computed(() => netInterfaces.value.reduce((sum, item) => sum + item.readSpeed, 0));
const totalNetSent = computed(() => netInterfaces.value.reduce((sum, item) => sum + item.writeSpeed, 0));
const activeInterfaces = computed(() => netInterfaces.value.length);
const hottestInterface = computed(() => netInterfaces.value[0] || null);

const graphHeight = ref(420);
const isResizing = ref(false);

const startResize = (e: MouseEvent) => {
  isResizing.value = true;
  const startY = e.clientY;
  const startHeight = graphHeight.value;

  const onMouseMove = (moveEvent: MouseEvent) => {
    if (!isResizing.value) return;
    const delta = moveEvent.clientY - startY;
    graphHeight.value = Math.max(200, Math.min(1200, startHeight + delta));
  };

  const onMouseUp = () => {
    isResizing.value = false;
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
  };

  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
};

const connectWebSocket = () => {
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }

  lastIO = null;
  interfaceHistory.value = {};
  interfaceNames.value = [];
  const socket = new WebSocket(buildWebSocketUrl('/ws/system', { interval: refreshInterval.value }));
  ws = socket;
  socket.binaryType = 'arraybuffer';

  socket.onopen = () => {
    if (ws !== socket) return;
    isConnected.value = true;
  };

  socket.onmessage = (msg) => {
    if (ws !== socket) return;
    try {
      const decoded = pb.SystemStats.decode(new Uint8Array(msg.data));
      const now = Date.now();

      if (decoded.io) {
        const networkList = decoded.io.networks || [];
        interfaceNames.value = networkList.map((network: any) => network.name);

        const dt = lastIO ? (now - lastIO.time) / 1000 : 0;
        let curRecv = 0;
        let curSent = 0;

        // Backend sends rates (bytes/sec), use them directly — do NOT
        // compute another delta/dt on top (that would be a double division).
        networkList.forEach((network: any) => {
          const readSpeed = Number(network.recvBytes);
          const writeSpeed = Number(network.sentBytes);
          rememberSample(network.name, { time: now, readSpeed, writeSpeed });
          curRecv += readSpeed;
          curSent += writeSpeed;
        });

        // Accumulate session total: rate × interval = bytes transferred
        if (dt > 0) {
          cumRecv.value += curRecv * dt;
          cumSent.value += curSent * dt;
        }

        const networks: NetworkSnapshot = {};
        networkList.forEach((network: any) => {
          networks[network.name] = {
            r: Number(network.recvBytes),
            s: Number(network.sentBytes),
          };
        });
        lastIO = { networks, time: now };
      }
    } catch (error) {
      console.error(error);
    }
  };

  socket.onclose = () => {
    if (ws !== socket) return;
    isConnected.value = false;
    ws = null;
    if (!shouldReconnect) return;
    if (reconnectTimer !== null) clearTimeout(reconnectTimer);
    reconnectTimer = window.setTimeout(connectWebSocket, 3000);
  };
};

const disconnectWebSocket = () => {
  shouldReconnect = false;
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.close();
  }
  ws = null;
};

onMounted(() => {
  shouldReconnect = true;
  connectWebSocket();
});

onUnmounted(() => {
  disconnectWebSocket();
});
</script>

<template>
  <div style="padding: 20px; background: #f0f2f5; min-height: 100%;">
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
              <div style="font-size: 12px; color: #666;">Detected Interfaces</div>
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

    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :span="24">
        <a-card size="small" :bordered="false">
          <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap; justify-content: space-between;">
            <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
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
            <div v-if="hottestInterface" style="font-size: 12px; color: #475569;">
              Top interface:
              <strong>{{ hottestInterface.name }}</strong>
              · {{ formatBytes(hottestInterface.readSpeed + hottestInterface.writeSpeed, 1) }}/s
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="[16, 16]" style="margin-bottom: 16px;">
      <a-col :span="24">
        <a-card title="Directed Traffic Graph" size="small" :bordered="false">
          <template #extra>
            <a-space :size="8" wrap>
              <a-tag color="default">&lt; 100kbps</a-tag>
              <a-tag color="green">100kbps-1Mbps</a-tag>
              <a-tag color="cyan">1-10Mbps</a-tag>
              <a-tag color="gold">10-100Mbps</a-tag>
              <a-tag color="red">&gt; 100Mbps</a-tag>
            </a-space>
          </template>
          <a-alert
            type="info"
            show-icon
            style="margin-bottom: 16px;"
            message="Internet is the hub node"
            description="Interface → Internet represents TX traffic, and Internet → Interface represents RX traffic. Node size and edge width both scale with traffic rate over the selected time window. Click an interface node to open its history chart."
          />
          <div style="position: relative;">
            <TrafficGraph 
              :interfaces="netInterfaces" 
              :height="graphHeight"
              @select-interface="openInterfaceChart" 
            />
            <div 
              style="
                position: absolute; 
                bottom: 0; 
                left: 0; 
                right: 0; 
                height: 8px; 
                cursor: ns-resize; 
                background: rgba(0,0,0,0.02);
                border-bottom-left-radius: 12px;
                border-bottom-right-radius: 12px;
                display: flex;
                justify-content: center;
                align-items: center;
                transition: background 0.2s;
              "
              @mousedown="startResize"
              class="graph-resize-handle"
            >
              <div style="width: 32px; height: 3px; background: #d9d9d9; border-radius: 2px;"></div>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="[16, 16]">
      <a-col :span="24">
        <a-card title="Interface Details" size="small" :bordered="false">
          <template #extra>
            <a-tag color="blue">{{ netInterfaces.length }} interfaces</a-tag>
          </template>
          <a-table
            :data-source="netInterfaces"
            :columns="[
              { title: 'Interface', dataIndex: 'name', key: 'name' },
              { title: 'Download', dataIndex: 'readSpeed', key: 'readSpeed', align: 'right' as const },
              { title: 'Upload', dataIndex: 'writeSpeed', key: 'writeSpeed', align: 'right' as const },
              { title: 'Total', key: 'total', align: 'right' as const },
              { title: 'Level', key: 'level', align: 'center' as const },
            ]"
            :pagination="false"
            size="small"
            row-key="name"
            :locale="{ emptyText: 'No network interfaces detected' }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'name'">
                <span style="color: #1677ff; cursor: pointer;" @click.stop="openInterfaceChart(record.name)">
                  {{ record.name }}
                </span>
              </template>
              <template v-if="column.key === 'readSpeed'">
                <span style="color: #1890ff;">{{ formatBytes(record.readSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'writeSpeed'">
                <span style="color: #52c41a;">{{ formatBytes(record.writeSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'total'">
                <span style="font-weight: 600;">{{ formatBytes(record.readSpeed + record.writeSpeed, 2) }}/s</span>
              </template>
              <template v-else-if="column.key === 'level'">
                <a-tag :color="getTrafficLevelColor(record.readSpeed + record.writeSpeed)">
                  {{ getTrafficLevelLabel(record.readSpeed + record.writeSpeed) }}
                </a-tag>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>

    <a-modal
      v-model:open="showInterfaceChartModal"
      :title="selectedInterfaceName ? `Interface History: ${selectedInterfaceName}` : 'Interface History'"
      :footer="null"
      width="900px"
    >
      <div style="margin-bottom: 16px; display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px;">
        <div v-if="selectedInterface" style="display: flex; flex-wrap: wrap; gap: 8px;">
          <a-tag color="blue">Download {{ formatBytes(selectedInterface.readSpeed, 1) }}/s</a-tag>
          <a-tag color="green">Upload {{ formatBytes(selectedInterface.writeSpeed, 1) }}/s</a-tag>
        </div>
        <a-radio-group v-model:value="interfaceChartTimeRange" size="small" button-style="solid">
          <a-radio-button :value="30">30s</a-radio-button>
          <a-radio-button :value="60">60s</a-radio-button>
          <a-radio-button :value="120">2m</a-radio-button>
          <a-radio-button :value="300">5m</a-radio-button>
        </a-radio-group>
      </div>
      <div v-if="showInterfaceChartModal" style="background: #fff; padding: 10px; border-radius: 4px; border: 1px solid #f0f0f0;">
        <VueApexCharts type="line" height="360" :options="interfaceChartOptions" :series="interfaceChartSeries" />
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.graph-resize-handle:hover {
  background: rgba(24, 144, 255, 0.1) !important;
}
.graph-resize-handle:hover > div {
  background: #1890ff !important;
}
</style>
