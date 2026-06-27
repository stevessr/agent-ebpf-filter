<script setup lang="ts">
import { computed, defineAsyncComponent, ref, onUnmounted, watch } from "vue";
import axios from "axios";
import type { ProcessInfo } from "../../../composables/monitor/useProcessObserver";

const VueApexCharts = defineAsyncComponent(
  async () => (await import("vue3-apexcharts")).default as any,
) as any;

const props = defineProps<{
  processes: ProcessInfo[];
  treePids: Set<number>;
}>();

// ── History for time-series ──────────────────────────────────────────────
const MAX_HISTORY = 60;
const cpuHistory = ref<{ time: number; series: { name: string; data: [number, number][] }[] }>({ time: 0, series: [] });
const memHistory = ref<{ time: number; series: { name: string; data: [number, number][] }[] }>({ time: 0, series: [] });

// Sample CPU/Mem every 2 seconds
let sampleTimer: ReturnType<typeof setInterval> | null = null;
const doSample = () => {
  const now = Date.now();
  const procs = props.processes.filter((p) => props.treePids.has(p.pid));
  const update = (history: typeof cpuHistory.value, field: "cpu" | "mem") => {
    const series = [...history.series];
    for (const p of procs) {
      const name = `[${p.pid}] ${p.name}`;
      let s = series.find((x) => x.name === name);
      if (!s) { s = { name, data: [] }; series.push(s); }
      s.data.push([now, p[field] ?? 0]);
      if (s.data.length > MAX_HISTORY) s.data.shift();
    }
    history.series = series;
    history.time = now;
  };
  update(cpuHistory, "cpu");
  update(memHistory, "mem");
};

watch(() => props.treePids, (pids) => {
  if (sampleTimer) clearInterval(sampleTimer);
  if (pids.size > 0) { doSample(); sampleTimer = setInterval(doSample, 2000); }
}, { immediate: true });

onUnmounted(() => { if (sampleTimer) clearInterval(sampleTimer); });

// ── I/O stats ────────────────────────────────────────────────────────────
const ioStats = ref<Record<number, Record<string, string>>>({});
const ioLoading = ref(false);

const fetchIO = async () => {
  ioLoading.value = true;
  const stats: Record<number, Record<string, string>> = {};
  for (const pid of props.treePids) {
    try {
      const res = await axios.get(`/system/process/io`, { params: { pid } });
      stats[pid] = res.data;
    } catch {}
  }
  ioStats.value = stats;
  ioLoading.value = false;
};

watch(() => props.treePids, (pids) => { if (pids.size > 0) fetchIO(); });

// ── Chart options ────────────────────────────────────────────────────────
const cpuChartOptions = computed(() => makeLineOpts("CPU %"));
const memChartOptions = computed(() => makeLineOpts("Memory %"));

const makeLineOpts = (yLabel: string) => ({
  chart: { id: yLabel, animations: { enabled: false }, toolbar: { show: false }, background: "transparent" },
  xaxis: { type: "datetime" as const, labels: { show: true, style: { fontSize: "10px" }, datetimeUTC: false } },
  yaxis: { title: { text: yLabel, style: { fontSize: "11px" } }, min: 0, tickAmount: 4 },
  stroke: { width: 2, curve: "smooth" as const },
  legend: { show: true, fontSize: "10px", position: "bottom" as const },
  grid: { borderColor: "#f0f0f0" },
  tooltip: { x: { format: "HH:mm:ss" } },
  colors: ["#1677ff","#52c41a","#fa8c16","#722ed1","#eb2f96","#13c2c2","#f5222d","#2f54eb","#faad14","#a0d911"],
});

const cpuSeries = computed(() => cpuHistory.value.series.map((s) => ({ name: s.name, data: s.data })));
const memSeries = computed(() => memHistory.value.series.map((s) => ({ name: s.name, data: s.data })));

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
  if (b < 1024) return `${b} B`;
  if (b < 1048576) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / 1048576).toFixed(1)} MB`;
};

// ── Process table ────────────────────────────────────────────────────────
const resourceColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 65 },
  { title: "Name", dataIndex: "name", key: "name", width: 130, ellipsis: true },
  { title: "CPU %", dataIndex: "cpu", key: "cpu", width: 80, align: "right" as const },
  { title: "Mem %", dataIndex: "mem", key: "mem", width: 80, align: "right" as const },
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
        <a-card size="small"><a-statistic title="Memory" :value="tableData.reduce((s,p)=>s+(p.mem??0),0).toFixed(1)" suffix="%" /></a-card>
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
