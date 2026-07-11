<script setup lang="ts">
import { computed, defineAsyncComponent, ref, onUnmounted, watch } from "vue";
import axios from "axios";
import type { ProcessInfo } from "../../../composables/monitor/useProcessObserver";
import { useProcessResourceHistory } from "./useProcessResourceHistory";

const VueApexCharts = defineAsyncComponent(
  async () => (await import("vue3-apexcharts")).default as any,
) as any;

const props = withDefaults(defineProps<{
  processes: ProcessInfo[];
  treePids?: Set<number>;
  memTotal?: number;
}>(), {
  treePids: () => new Set(),
  memTotal: 0,
});

const { cpuHistory, memHistory } = useProcessResourceHistory({
  processes: () => props.processes,
  treePids: () => props.treePids,
});

// ── I/O stats (auto-fetch on treePids change, with debounce) ───────────────
const ioStats = ref<Record<number, Record<string, string>>>({});
const ioLoading = ref(false);
let ioAutoFetchTimer: ReturnType<typeof setTimeout> | null = null;
let ioRequestController: AbortController | null = null;
let ioRequestGeneration = 0;
let ioUnmounted = false;

const currentIOPidKey = () => [...props.treePids].sort((a, b) => a - b).join(",");

const fetchIO = async () => {
  const pidKey = currentIOPidKey();
  const pids = pidKey ? pidKey.split(",").map(Number) : [];
  const generation = ++ioRequestGeneration;

  ioRequestController?.abort();
  const controller = new AbortController();
  ioRequestController = controller;

  if (pids.length === 0) {
    ioStats.value = {};
    ioLoading.value = false;
    if (generation === ioRequestGeneration) ioRequestController = null;
    return;
  }

  ioLoading.value = true;
  const stats: Record<number, Record<string, string>> = {};
  // Fetch in parallel with Promise.all, not sequentially
  await Promise.all(pids.map(async (pid) => {
    try {
      const res = await axios.get(`/system/process/io`, {
        params: { pid },
        signal: controller.signal,
      });
      stats[pid] = res.data;
    } catch {}
  }));

  if (
    !ioUnmounted
    && generation === ioRequestGeneration
    && !controller.signal.aborted
    && pidKey === currentIOPidKey()
  ) {
    ioStats.value = stats;
  }
  if (generation === ioRequestGeneration) {
    ioRequestController = null;
    ioLoading.value = false;
  }
};

// Auto-fetch I/O when treePids changes (debounced to avoid thundering herd)
watch(currentIOPidKey, (pidKey) => {
  if (ioAutoFetchTimer) clearTimeout(ioAutoFetchTimer);
  ioRequestController?.abort();
  ioRequestController = null;
  ioRequestGeneration++;
  ioLoading.value = false;
  if (pidKey) {
    ioAutoFetchTimer = setTimeout(() => { fetchIO(); }, 500);
  } else {
    ioStats.value = {};
  }
}, { immediate: true });

onUnmounted(() => {
  ioUnmounted = true;
  ioRequestGeneration++;
  ioRequestController?.abort();
  if (ioAutoFetchTimer) clearTimeout(ioAutoFetchTimer);
});

// ── Chart options ────────────────────────────────────────────────────────
const cpuChartOptions = computed(() => makeLineOpts("CPU %"));
const memChartOptions = computed(() => makeLineOpts("Memory", true));

const makeLineOpts = (yLabel: string, byteFormat = false) => ({
  chart: { id: yLabel, animations: { enabled: false }, toolbar: { show: false }, background: "transparent" },
  xaxis: { type: "datetime" as const, labels: { show: true, style: { fontSize: "10px" }, datetimeUTC: false } },
  yaxis: {
    title: { text: yLabel, style: { fontSize: "11px" } },
    min: 0,
    tickAmount: 4,
    ...(byteFormat
      ? {
          labels: {
            formatter: (val: number) => fmtBytes(val),
            style: { fontSize: "10px" },
          },
        }
      : {}),
  },
  stroke: { width: 2, curve: "smooth" as const },
  legend: { show: true, fontSize: "10px", position: "bottom" as const },
  grid: { borderColor: "#f0f0f0" },
  tooltip: {
    x: { format: "HH:mm:ss" },
    ...(byteFormat
      ? {
          y: {
            formatter: (val: number) => fmtBytes(val),
          },
        }
      : {}),
  },
  colors: ["#1677ff","#52c41a","#fa8c16","#722ed1","#eb2f96","#13c2c2","#f5222d","#2f54eb","#faad14","#a0d911"],
});

const cpuSeries = computed(() => cpuHistory.value.series.map((s) => ({ name: s.name, data: s.data })));
const memSeries = computed(() =>
  memHistory.value.series.map((s) => ({
    name: s.name,
    data: s.data.map(([t, v]) => [t, memPercentToBytes(v)] as [number, number]),
  })),
);

// ── I/O summary ──────────────────────────────────────────────────────────
const ioSummary = computed(() => {
  let readBytes = 0, writeBytes = 0;
  for (const s of Object.values(ioStats.value)) {
    readBytes += parseInt(s["read_bytes"] || "0", 10);
    writeBytes += parseInt(s["write_bytes"] || "0", 10);
  }
  return { readBytes, writeBytes };
});

const fmtBytes = (b: number) => {
  if (!b || b === 0) return "—";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(
    Math.floor(Math.log(Math.abs(b)) / Math.log(1024)),
    u.length - 1,
  );
  const precision = i > 2 ? 2 : 1;
  return `${(b / Math.pow(1024, i)).toFixed(precision)} ${u[i]}`;
};

// Convert mem% (0-100) to approximate RSS bytes
const memPercentToBytes = (memPercent: number): number =>
  props.memTotal > 0 ? (memPercent / 100) * props.memTotal : 0;

// Total memory bytes across all processes in the tree
const totalMemBytes = computed(() =>
  tableData.value.reduce((s, p) => s + memPercentToBytes(p.mem ?? 0), 0),
);

// ── Process table ────────────────────────────────────────────────────────
const resourceColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 65 },
  { title: "Name", dataIndex: "name", key: "name", width: 130, ellipsis: true },
  { title: "CPU %", dataIndex: "cpu", key: "cpu", width: 80, align: "right" as const },
  {
    title: "Memory",
    dataIndex: "mem",
    key: "mem",
    width: 95,
    align: "right" as const,
    customRender: ({ text }: { text: number }) => fmtBytes(memPercentToBytes(text ?? 0)),
  },
  { title: "User", dataIndex: "user", key: "user", width: 90 },
];

const tableData = computed(() => props.processes.filter((p) => props.treePids.has(p.pid)));
</script>

<template>
  <div class="resources-root">
    <!-- Summary cards -->
    <a-row :gutter="12" style="margin-bottom: 12px">
      <a-col :span="6">
        <a-card size="small"><a-statistic title="CPU" :value="tableData.reduce((s,p)=>s+(p.cpu??0),0).toFixed(1)" suffix="%" /></a-card>
      </a-col>
      <a-col :span="6">
        <a-card size="small"><a-statistic title="Memory" :value="fmtBytes(totalMemBytes)" /></a-card>
      </a-col>
      <a-col :span="6">
        <a-card size="small"><a-statistic title="Read" :value="fmtBytes(ioSummary.readBytes)" /></a-card>
      </a-col>
      <a-col :span="6">
        <a-card size="small"><a-statistic title="Write" :value="fmtBytes(ioSummary.writeBytes)" /></a-card>
      </a-col>
    </a-row>

    <!-- CPU Chart -->
    <div class="chart-box">
      <div class="chart-title">CPU Usage</div>
      <VueApexCharts type="line" height="200" :options="cpuChartOptions" :series="cpuSeries" />
    </div>

    <!-- Memory Chart -->
    <div class="chart-box">
      <div class="chart-title">Memory Usage</div>
      <VueApexCharts type="line" height="200" :options="memChartOptions" :series="memSeries" />
    </div>

    <!-- Process table -->
    <div class="chart-title" style="margin-top: 8px">Processes in Tree ({{ tableData.length }})</div>
    <a-table :dataSource="tableData" :columns="resourceColumns" row-key="pid" size="small" :pagination="false" />

    <a-button size="small" type="link" style="margin-top: 8px" :loading="ioLoading" @click="fetchIO">Refresh I/O Stats</a-button>
  </div>
</template>

<style scoped>
.resources-root { display: flex; flex-direction: column; }
.chart-box {
  background: #fafbfc; border: 1px solid #f0f0f0; border-radius: 8px;
  padding: 10px; margin-bottom: 10px;
}
.chart-title {
  font-size: 13px; font-weight: 600; color: #475569;
  margin-bottom: 6px;
}
</style>
