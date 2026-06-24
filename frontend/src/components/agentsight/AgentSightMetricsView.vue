<script setup lang="ts">
import { computed, shallowRef } from "vue";

import { formatBytes, type AgentSightEvent } from "../../utils/agentsight";

interface ResourceMetric {
  timestamp: number;
  formattedTime: string;
  pid: number;
  comm: string;
  cpuPercent: number;
  memoryBytes: number;
  threads: number;
  children: number;
  alert: boolean;
}

const props = defineProps<{
  events: AgentSightEvent[];
}>();

const selectedProcess = shallowRef("all");
const metricType = shallowRef<"cpu" | "memory">("cpu");

const metrics = computed<ResourceMetric[]>(() =>
  props.events
    .filter((event) => event.source === "system")
    .map((event) => ({
      timestamp: event.timestamp,
      formattedTime: new Date(event.timestamp).toLocaleTimeString(),
      pid: event.pid,
      comm: event.comm,
      cpuPercent: Number(
        event.data.cpu?.percent ||
          event.data.cpu_percent ||
          event.data.cpuPercent ||
          0,
      ),
      memoryBytes: Number(
        event.data.memory_bytes ||
          event.data.memoryBytes ||
          event.data.memory?.rss_bytes ||
          event.data.memory?.used_bytes ||
          event.data.memory?.rss_mb * 1024 * 1024 ||
          0,
      ),
      threads: Number(event.data.process?.threads || event.data.threads || 0),
      children: Number(
        event.data.process?.children || event.data.children || 0,
      ),
      alert: Boolean(event.data.alert),
    }))
    .sort((a, b) => a.timestamp - b.timestamp),
);

const processes = computed(() => {
  const processMap = new Map<
    string,
    { key: string; pid: number; comm: string; count: number }
  >();
  metrics.value.forEach((metric) => {
    const key = `${metric.pid}-${metric.comm}`;
    const existing = processMap.get(key);
    if (existing) existing.count += 1;
    else
      processMap.set(key, {
        key,
        pid: metric.pid,
        comm: metric.comm,
        count: 1,
      });
  });
  return Array.from(processMap.values()).sort((a, b) => b.count - a.count);
});

const processOptions = computed(() => [
  { label: `All processes (${metrics.value.length})`, value: "all" },
  ...processes.value.map((process) => ({
    label: `${process.comm} (${process.pid}) · ${process.count}`,
    value: process.key,
  })),
]);

const filteredMetrics = computed(() => {
  if (selectedProcess.value === "all") return metrics.value;
  const [pid, ...commParts] = selectedProcess.value.split("-");
  const comm = commParts.join("-");
  return metrics.value.filter(
    (metric) => metric.pid === Number(pid) && metric.comm === comm,
  );
});

const stats = computed(() => {
  if (filteredMetrics.value.length === 0)
    return {
      avgCpu: "0.00",
      maxCpu: "0.00",
      avgMemory: "0 B",
      maxMemory: "0 B",
      alertCount: 0,
    };
  const cpuValues = filteredMetrics.value.map((metric) => metric.cpuPercent);
  const memoryValues = filteredMetrics.value.map(
    (metric) => metric.memoryBytes,
  );
  const avgCpu =
    cpuValues.reduce((sum, value) => sum + value, 0) / cpuValues.length;
  const avgMemory =
    memoryValues.reduce((sum, value) => sum + value, 0) / memoryValues.length;
  return {
    avgCpu: avgCpu.toFixed(2),
    maxCpu: Math.max(...cpuValues).toFixed(2),
    avgMemory: formatBytes(avgMemory),
    maxMemory: formatBytes(Math.max(...memoryValues)),
    alertCount: filteredMetrics.value.filter((metric) => metric.alert).length,
  };
});

const chartMax = computed(() => {
  if (filteredMetrics.value.length === 0) return 1;
  if (metricType.value === "cpu") {
    const maxCpu = Math.max(
      ...filteredMetrics.value.map((metric) => metric.cpuPercent),
    );
    return Math.max(100, Math.ceil(maxCpu * 1.1));
  }
  const maxMemory = Math.max(
    1,
    ...filteredMetrics.value.map((metric) => metric.memoryBytes),
  );
  return maxMemory * 1.1;
});

const pointX = (index: number) => {
  if (filteredMetrics.value.length === 0) return 50;
  if (filteredMetrics.value.length === 1) return 50;
  return (index / (filteredMetrics.value.length - 1)) * 100;
};

const pointY = (value: number) => {
  if (chartMax.value === 0) return 100;
  return Math.max(0, Math.min(100, 100 - (value / chartMax.value) * 100));
};

const metricValue = (metric: ResourceMetric) =>
  metricType.value === "cpu" ? metric.cpuPercent : metric.memoryBytes;

const metricLabel = (value: number) =>
  metricType.value === "cpu" ? `${value.toFixed(2)}%` : formatBytes(value);
</script>

<template>
  <div class="metrics-view">
    <a-empty
      v-if="metrics.length === 0"
      description="No system metric events loaded yet. Enable AgentSight/system metrics to populate this view."
    />
    <template v-else>
      <div class="metrics-toolbar">
        <h3>Resource Metrics</h3>
        <a-space wrap>
          <a-segmented
            v-model:value="metricType"
            :options="[
              { label: 'CPU', value: 'cpu' },
              { label: 'Memory', value: 'memory' },
            ]"
          />
          <a-select
            v-model:value="selectedProcess"
            :options="processOptions"
            style="width: 260px"
          />
        </a-space>
      </div>

      <a-row :gutter="[12, 12]">
        <a-col :xs="12" :md="4"
          ><a-card size="small"
            ><a-statistic
              title="Avg CPU"
              :value="stats.avgCpu"
              suffix="%" /></a-card
        ></a-col>
        <a-col :xs="12" :md="4"
          ><a-card size="small"
            ><a-statistic
              title="Peak CPU"
              :value="stats.maxCpu"
              suffix="%" /></a-card
        ></a-col>
        <a-col :xs="12" :md="5"
          ><a-card size="small"
            ><a-statistic title="Avg Memory" :value="stats.avgMemory" /></a-card
        ></a-col>
        <a-col :xs="12" :md="5"
          ><a-card size="small"
            ><a-statistic
              title="Peak Memory"
              :value="stats.maxMemory" /></a-card
        ></a-col>
        <a-col :xs="12" :md="4"
          ><a-card size="small"
            ><a-statistic title="Alerts" :value="stats.alertCount" /></a-card
        ></a-col>
      </a-row>

      <a-card
        class="chart-card"
        :title="metricType === 'cpu' ? 'CPU over time' : 'Memory over time'"
      >
        <template #extra>{{ filteredMetrics.length }} data points</template>
        <div v-if="filteredMetrics.length === 0" class="chart-empty">
          <a-empty description="No data points available for the selected filters" />
        </div>
        <div v-else class="chart">
          <div class="y-axis">
            <span>{{ metricLabel(chartMax) }}</span>
            <span>{{ metricLabel(chartMax * 0.75) }}</span>
            <span>{{ metricLabel(chartMax * 0.5) }}</span>
            <span>{{ metricLabel(chartMax * 0.25) }}</span>
            <span>0</span>
          </div>
          <svg
            class="chart-svg"
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
          >
            <line
              v-for="grid in [0, 25, 50, 75, 100]"
              :key="grid"
              x1="0"
              x2="100"
              :y1="grid"
              :y2="grid"
              class="grid-line"
            />
            <template
              v-for="(metric, index) in filteredMetrics"
              :key="`${metric.timestamp}-${index}`"
            >
              <line
                v-if="index > 0"
                :x1="pointX(index - 1)"
                :y1="pointY(metricValue(filteredMetrics[index - 1]))"
                :x2="pointX(index)"
                :y2="pointY(metricValue(metric))"
                :class="metric.alert ? 'metric-line alert' : 'metric-line'"
              />
              <circle
                :cx="pointX(index)"
                :cy="pointY(metricValue(metric))"
                r="1.3"
                :class="metric.alert ? 'metric-point alert' : 'metric-point'"
              >
                <title>
                  {{ metric.formattedTime }} · {{ metric.comm }}#{{
                    metric.pid
                  }}
                  · {{ metricLabel(metricValue(metric)) }}
                </title>
              </circle>
            </template>
          </svg>
        </div>
      </a-card>

      <a-table
        :data-source="filteredMetrics.slice().reverse()"
        size="small"
        row-key="timestamp"
        :pagination="{ pageSize: 12 }"
        :scroll="{ x: 860 }"
      >
        <a-table-column title="Time" data-index="formattedTime" key="time" />
        <a-table-column title="Process" data-index="comm" key="comm" />
        <a-table-column title="PID" data-index="pid" key="pid" />
        <a-table-column title="CPU" key="cpu">
          <template #default="{ record }"
            >{{ record.cpuPercent.toFixed(2) }}%</template
          >
        </a-table-column>
        <a-table-column title="Memory" key="memory">
          <template #default="{ record }">{{
            formatBytes(record.memoryBytes)
          }}</template>
        </a-table-column>
        <a-table-column title="Threads" data-index="threads" key="threads" />
        <a-table-column title="Children" data-index="children" key="children" />
      </a-table>
    </template>
  </div>
</template>

<style scoped>
.metrics-view {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.metrics-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.metrics-toolbar h3 {
  margin: 0;
}

.chart-card {
  overflow: hidden;
}

.chart-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
}

.chart {
  display: grid;
  grid-template-columns: 95px 1fr;
  gap: 12px;
  height: 300px;
  align-items: stretch;
}

.y-axis {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: flex-end;
  color: #64748b;
  font-size: 12px;
  padding-right: 8px;
}

.chart-svg {
  width: 100%;
  height: 100%;
  border-left: 2px solid #cbd5e1;
  border-bottom: 2px solid #cbd5e1;
  overflow: visible;
}

.grid-line {
  stroke: #e2e8f0;
  stroke-width: 0.35;
}

.metric-line {
  stroke: #1677ff;
  stroke-width: 0.9;
  vector-effect: non-scaling-stroke;
  fill: none;
}

.metric-line.alert {
  stroke: #ef4444;
}

.metric-point {
  fill: #1677ff;
  cursor: pointer;
}

.metric-point.alert {
  fill: #ef4444;
}

.metric-point:hover {
  r: 2;
  filter: brightness(1.2);
}
</style>
