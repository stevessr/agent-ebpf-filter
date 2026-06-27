<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { ObserverEvent } from "../../../composables/monitor/useProcessObserver";

const props = withDefaults(defineProps<{
  events?: ObserverEvent[];
}>(), {
  events: () => [],
});

// ── Persistent view state (localStorage backed) ─────────────────────────
const readStored = <T>(key: string, fallback: T): T => {
  try {
    const v = localStorage.getItem(key);
    return v !== null ? JSON.parse(v) as T : fallback;
  } catch { return fallback; }
};

const writeStored = (key: string, value: unknown): void => {
  try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* ignore */ }
};

const FLAMEGRAPH_METRIC_KEY = "flamegraph-metric";
const FLAMEGRAPH_SCALE_KEY = "flamegraph-scale";
const FLAMEGRAPH_VIEW_KEY = "flamegraph-view";

const metric = ref<"count" | "bytes" | "duration">(
  readStored<"count" | "bytes" | "duration">(FLAMEGRAPH_METRIC_KEY, "count")
);
const scale = ref<"linear" | "log">(
  readStored<"linear" | "log">(FLAMEGRAPH_SCALE_KEY, "linear")
);
const viewMode = ref<"flamegraph" | "barchart" | "donut">(
  readStored<"flamegraph" | "barchart" | "donut">(FLAMEGRAPH_VIEW_KEY, "flamegraph")
);

watch(metric, (v) => writeStored(FLAMEGRAPH_METRIC_KEY, v));
watch(scale, (v) => writeStored(FLAMEGRAPH_SCALE_KEY, v));
watch(viewMode, (v) => writeStored(FLAMEGRAPH_VIEW_KEY, v));

// ── Constants ───────────────────────────────────────────────────────────
const catColors: Record<string, string> = {
  Process: "#8b5cf6",
  File: "#06b6d4",
  Network: "#f59e0b",
  TLS: "#10b981",
  Syscall: "#6366f1",
};

// ── Category classification (mirrors ObserverTimeline) ──────────────────
const cat = (e: ObserverEvent): string => {
  const t = (e.type || "").toLowerCase();
  if (t.includes("exec")||t.includes("fork")||t.includes("clone")||t.includes("exit")) return "Process";
  if (t.includes("open")||t.includes("read")||t.includes("write")||t.includes("mkdir")||t.includes("unlink")||t.includes("chmod")||t.includes("chown")||t.includes("rename")||t.includes("link")||t.includes("symlink")||t.includes("mknod")) return "File";
  if (t.includes("connect")||t.includes("bind")||t.includes("send")||t.includes("recv")||t.includes("socket")||t.includes("accept")||t.includes("dns")) return "Network";
  if (t.includes("tls")||t.includes("ssl")||t.includes("http")) return "TLS";
  return "Syscall";
};

// ── Utility functions ───────────────────────────────────────────────────
const scaleFn = (v: number): number =>
  scale.value === "log" ? Math.log10(v + 1) : v;

const formatBytes = (v: number): string => {
  if (v === 0) return "0 B";
  if (v < 1024) return `${v} B`;
  if (v < 1048576) return `${(v / 1024).toFixed(1)} KB`;
  return `${(v / 1048576).toFixed(1)} MB`;
};

const formatDuration = (v: number): string => {
  if (v === 0) return "0 ms";
  if (v < 1000) return `${v} ms`;
  if (v < 60000) return `${(v / 1000).toFixed(1)} s`;
  if (v < 3600000) return `${(v / 60000).toFixed(1)} min`;
  return `${(v / 3600000).toFixed(1)} h`;
};

const formatVal = (v: number): string => {
  if (metric.value === "count") return v.toLocaleString();
  if (metric.value === "bytes") return formatBytes(v);
  return formatDuration(v);
};

// ── Core aggregation: single-pass per-category stats ────────────────────
interface CategoryAgg {
  category: string;
  color: string;
  count: number;
  bytes: number;
  minTimestamp: number;
  maxTimestamp: number;
  duration: number;
  commDist: Record<string, number>;
  typeDist: Record<string, number>;
  rawValue: number;
  scaledValue: number;
}

const catAggs = computed<CategoryAgg[]>(() => {
  const map: Record<string, CategoryAgg> = {};
  for (const name of Object.keys(catColors)) {
    map[name] = {
      category: name,
      color: catColors[name],
      count: 0,
      bytes: 0,
      minTimestamp: Infinity,
      maxTimestamp: -Infinity,
      duration: 0,
      commDist: {},
      typeDist: {},
      rawValue: 0,
      scaledValue: 0,
    };
  }

  for (const e of props.events) {
    const c = cat(e);
    const agg = map[c];
    agg.count++;
    agg.bytes += e.bytes || 0;
    if (e.timestamp < agg.minTimestamp) agg.minTimestamp = e.timestamp;
    if (e.timestamp > agg.maxTimestamp) agg.maxTimestamp = e.timestamp;
    const comm = e.comm || "unknown";
    const typ = e.type || "unknown";
    agg.commDist[comm] = (agg.commDist[comm] || 0) + 1;
    agg.typeDist[typ] = (agg.typeDist[typ] || 0) + 1;
  }

  const result: CategoryAgg[] = [];
  for (const agg of Object.values(map)) {
    if (agg.count === 0) continue;
    agg.duration = agg.maxTimestamp - agg.minTimestamp;
    agg.rawValue = metric.value === "count" ? agg.count
      : metric.value === "bytes" ? agg.bytes
      : agg.duration;
    agg.scaledValue = scaleFn(agg.rawValue);
    result.push(agg);
  }
  return result;
});

const totalScaled = computed(() =>
  catAggs.value.reduce((s, a) => s + a.scaledValue, 0),
);

// ── Flamegraph nodes ────────────────────────────────────────────────────
interface Rect { x: number; y: number; w: number; h: number; label: string; color: string; value: number; depth: number }

const svgW = 1000;
const rowH = 22;

const flameNodes = computed<Rect[]>(() => {
  const rects: Rect[] = [];
  const total = totalScaled.value;
  if (total === 0) return rects;

  let cx = 0;
  for (const agg of catAggs.value) {
    const cw = Math.max((agg.scaledValue / total) * svgW, 2);
    rects.push({
      x: cx, y: 0, w: cw, h: rowH,
      label: agg.category,
      color: agg.color,
      value: agg.rawValue,
      depth: 0,
    });
    cx += cw;
  }
  return rects;
});

const svgH = computed(() => {
  const maxY = Math.max(...flameNodes.value.map(r => r.y + r.h), 30);
  return Math.min(maxY + 10, 120);
});

// ── Bar chart data ──────────────────────────────────────────────────────
interface BarItem {
  category: string;
  color: string;
  value: number;
  pct: number;
  display: string;
}

const barData = computed<BarItem[]>(() => {
  const total = totalScaled.value;
  return catAggs.value.map(agg => ({
    category: agg.category,
    color: agg.color,
    value: agg.rawValue,
    pct: total > 0 ? (agg.scaledValue / total) * 100 : 0,
    display: formatVal(agg.rawValue),
  }));
});

// ── Donut segments ──────────────────────────────────────────────────────
interface DonutSegment {
  path: string;
  color: string;
  label: string;
  value: number;
  proportion: number;
}

const donutSegments = computed<DonutSegment[]>(() => {
  const total = totalScaled.value;
  if (total === 0) return [];

  const segments: DonutSegment[] = [];
  const cx = 100, cy = 100;
  const outerR = 80, innerR = 52;
  let currentAngle = -Math.PI / 2;

  for (const agg of catAggs.value) {
    const proportion = agg.scaledValue / total;
    const angle = proportion * 2 * Math.PI;

    const startAngle = currentAngle;
    const endAngle = currentAngle + angle;

    const x1 = cx + outerR * Math.cos(startAngle);
    const y1 = cy + outerR * Math.sin(startAngle);
    const x2 = cx + outerR * Math.cos(endAngle);
    const y2 = cy + outerR * Math.sin(endAngle);
    const x3 = cx + innerR * Math.cos(endAngle);
    const y3 = cy + innerR * Math.sin(endAngle);
    const x4 = cx + innerR * Math.cos(startAngle);
    const y4 = cy + innerR * Math.sin(startAngle);

    const largeArc = angle > Math.PI ? 1 : 0;

    segments.push({
      path: `M${x1.toFixed(3)},${y1.toFixed(3)} A${outerR},${outerR} 0 ${largeArc} 1 ${x2.toFixed(3)},${y2.toFixed(3)} L${x3.toFixed(3)},${y3.toFixed(3)} A${innerR},${innerR} 0 ${largeArc} 0 ${x4.toFixed(3)},${y4.toFixed(3)} Z`,
      color: agg.color,
      label: agg.category,
      value: agg.rawValue,
      proportion,
    });

    currentAngle = endAngle;
  }
  return segments;
});

const donutCenterLabel = computed(() => {
  if (metric.value === "count") return "events";
  if (metric.value === "bytes") return "bytes";
  return "duration";
});

// ── Stats table data ────────────────────────────────────────────────────
interface CatStat {
  key: string;
  category: string;
  color: string;
  count: number;
  countDisplay: string;
  bytes: number;
  bytesDisplay: string;
  duration: number;
  durationDisplay: string;
  pct: number;
  pctDisplay: string;
  topComm: string;
  topEventType: string;
  commCount: number;
}

const catStats = computed<CatStat[]>(() => {
  const rawTotal = catAggs.value.reduce((s, a) => s + a.rawValue, 0);
  return catAggs.value.map(agg => {
    const pct = rawTotal > 0 ? (agg.rawValue / rawTotal) * 100 : 0;
    const topComm = Object.entries(agg.commDist).sort((a, b) => b[1] - a[1])[0]?.[0] || "-";
    const topType = Object.entries(agg.typeDist).sort((a, b) => b[1] - a[1])[0]?.[0] || "-";
    return {
      key: agg.category,
      category: agg.category,
      color: agg.color,
      count: agg.count,
      countDisplay: agg.count.toLocaleString(),
      bytes: agg.bytes,
      bytesDisplay: formatBytes(agg.bytes),
      duration: agg.duration,
      durationDisplay: formatDuration(agg.duration),
      pct,
      pctDisplay: pct.toFixed(1) + "%",
      topComm,
      topEventType: topType,
      commCount: Object.keys(agg.commDist).length,
    };
  });
});

const statsColumns = [
  { title: "Category", dataIndex: "category", key: "category", width: 90 },
  { title: "Count", dataIndex: "countDisplay", key: "count", width: 70, align: "right" as const, sorter: (a: CatStat, b: CatStat) => a.count - b.count },
  { title: "Bytes", dataIndex: "bytesDisplay", key: "bytes", width: 80, align: "right" as const, sorter: (a: CatStat, b: CatStat) => a.bytes - b.bytes },
  { title: "Duration", dataIndex: "durationDisplay", key: "duration", width: 85, align: "right" as const, sorter: (a: CatStat, b: CatStat) => a.duration - b.duration },
  { title: "%", dataIndex: "pctDisplay", key: "pct", width: 55, align: "right" as const, sorter: (a: CatStat, b: CatStat) => a.pct - b.pct },
  { title: "Top Comm", dataIndex: "topComm", key: "topComm", width: 110, ellipsis: true },
  { title: "Top Event", dataIndex: "topEventType", key: "topEventType", width: 110, ellipsis: true },
  { title: "Comms", dataIndex: "commCount", key: "commCount", width: 55, align: "right" as const, sorter: (a: CatStat, b: CatStat) => a.commCount - b.commCount },
];
</script>

<template>
  <div class="flame-root">
    <!-- ── Toolbar ────────────────────────────────────────────────────── -->
    <div class="flame-toolbar">
      <span class="flame-title">Event Flamegraph</span>
      <div class="flame-controls">
        <a-radio-group v-model:value="metric" size="small" button-style="solid">
          <a-radio-button value="count">Count</a-radio-button>
          <a-radio-button value="bytes">Bytes</a-radio-button>
          <a-radio-button value="duration">Duration</a-radio-button>
        </a-radio-group>
        <a-radio-group v-model:value="scale" size="small" button-style="solid">
          <a-radio-button value="linear">Linear</a-radio-button>
          <a-radio-button value="log">Log</a-radio-button>
        </a-radio-group>
        <a-radio-group v-model:value="viewMode" size="small" button-style="solid">
          <a-radio-button value="flamegraph">Flame</a-radio-button>
          <a-radio-button value="barchart">Bar</a-radio-button>
          <a-radio-button value="donut">Donut</a-radio-button>
        </a-radio-group>
      </div>
    </div>

    <!-- ── Empty state ────────────────────────────────────────────────── -->
    <a-empty v-if="catAggs.length === 0" description="No events to visualize" />

    <!-- ── Main content ───────────────────────────────────────────────── -->
    <div v-else>
      <!-- Flamegraph View -->
      <div v-if="viewMode === 'flamegraph'" class="flame-canvas">
        <div v-if="totalScaled > 0">
          <svg :viewBox="`0 0 ${svgW} ${svgH}`" preserveAspectRatio="xMidYMid meet" width="100%" :height="svgH">
            <rect
              v-for="r in flameNodes"
              :key="`${r.label}-${r.x}`"
              :x="r.x"
              :y="r.y"
              :width="Math.max(r.w - 1, 1)"
              :height="r.h - 1"
              :fill="r.color"
              :opacity="0.85"
              rx="2"
            >
              <title>{{ r.label }}: {{ formatVal(r.value) }} {{ metric }}</title>
            </rect>
            <text
              v-for="r in flameNodes"
              :key="`t-${r.label}-${r.x}`"
              :x="r.x + 4"
              :y="r.y + 15"
              fill="#fff"
              font-size="11"
              font-family="ui-monospace,monospace"
              font-weight="600"
            >{{ r.w > 60 ? r.label : r.w > 25 ? r.label[0] : '' }}</text>
          </svg>
        </div>
        <div v-else class="zero-msg">All values are zero for the current metric</div>
      </div>

      <!-- Bar Chart View -->
      <div v-else-if="viewMode === 'barchart'" class="bar-canvas">
        <div v-if="totalScaled > 0" class="bar-chart">
          <div v-for="bar in barData" :key="bar.category" class="bar-row">
            <div class="bar-label-row">
              <span class="legend-dot" :style="{ background: bar.color }"></span>
              <span class="bar-cat-name">{{ bar.category }}</span>
              <span class="bar-value">{{ bar.display }}</span>
              <span class="bar-pct">{{ bar.pct < 0.01 ? '&lt;0.1' : bar.pct.toFixed(1) }}%</span>
            </div>
            <div class="bar-track">
              <div
                class="bar-fill"
                :style="{
                  width: Math.max(bar.pct, 0.3) + '%',
                  background: bar.color,
                }"
              >
                <span v-if="bar.pct > 20" class="bar-fill-label">{{ bar.category }}</span>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="zero-msg">All values are zero for the current metric</div>
      </div>

      <!-- Donut Chart View -->
      <div v-else class="donut-canvas">
        <div v-if="donutSegments.length > 0" class="donut-container">
          <svg viewBox="0 0 200 200" width="220" height="220">
            <!-- Background ring -->
            <circle cx="100" cy="100" r="66" fill="none" stroke="#f1f5f9" stroke-width="28" />
            <!-- Segments -->
            <path
              v-for="seg in donutSegments"
              :key="seg.label"
              :d="seg.path"
              :fill="seg.color"
              :opacity="0.85"
              stroke="#fff"
              stroke-width="0.5"
            >
              <title>{{ seg.label }}: {{ formatVal(seg.value) }} {{ metric }} ({{ (seg.proportion * 100).toFixed(1) }}%)</title>
            </path>
            <!-- Center text -->
            <text x="100" y="93" text-anchor="middle" fill="#1e293b" font-size="20" font-weight="700" font-family="ui-monospace,monospace">
              {{ catAggs.reduce((s, a) => s + a.rawValue, 0).toLocaleString() }}
            </text>
            <text x="100" y="113" text-anchor="middle" fill="#94a3b8" font-size="11" font-family="ui-monospace,monospace">
              {{ donutCenterLabel }}
            </text>
          </svg>
          <!-- Donut legend -->
          <div class="donut-legend">
            <span v-for="seg in donutSegments" :key="'dl-'+seg.label" class="donut-legend-item">
              <span class="legend-dot" :style="{ background: seg.color }"></span>
              <span class="donut-legend-label">{{ seg.label }}</span>
              <span class="donut-legend-pct">{{ (seg.proportion * 100).toFixed(1) }}%</span>
            </span>
          </div>
        </div>
        <div v-else class="zero-msg">All values are zero for the current metric</div>
      </div>

      <!-- Legend (flamegraph + bar chart) -->
      <div v-if="viewMode !== 'donut'" class="flame-legend">
        <span v-for="agg in catAggs" :key="agg.category" class="legend-item">
          <span class="legend-dot" :style="{ background: agg.color }"></span>{{ agg.category }}
        </span>
      </div>

      <!-- ── Stats table ──────────────────────────────────────────────── -->
      <div class="stats-section">
        <div class="stats-title">Per-Category Statistics</div>
        <a-table
          :dataSource="catStats"
          :columns="statsColumns"
          :pagination="false"
          size="small"
          row-key="key"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'category'">
              <span class="legend-dot" :style="{ background: record.color, display: 'inline-block', marginRight: 4 }"></span>
              {{ record.category }}
            </template>
          </template>
        </a-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.flame-root { display: flex; flex-direction: column; }

.flame-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 10px; gap: 8px; flex-wrap: wrap;
}
.flame-title { font-size: 13px; font-weight: 600; color: #475569; white-space: nowrap; }
.flame-controls {
  display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
}

/* ── Canvas shared ───────────────────────────────────────────────────── */
.flame-canvas,
.bar-canvas,
.donut-canvas {
  background: linear-gradient(180deg, #fff 0%, #f8fafc 100%);
  border: 1px solid #e2e8f0; border-radius: 10px; padding: 12px;
}

/* ── Flamegraph ──────────────────────────────────────────────────────── */
.flame-legend { display: flex; gap: 12px; margin-top: 8px; font-size: 11px; color: #64748b; }
.legend-item { display: flex; align-items: center; gap: 4px; }
.legend-dot { width: 8px; height: 8px; border-radius: 2px; display: inline-block; flex-shrink: 0; }

/* ── Bar chart ───────────────────────────────────────────────────────── */
.bar-chart { display: flex; flex-direction: column; gap: 10px; }

.bar-row { display: flex; flex-direction: column; gap: 4px; }

.bar-label-row {
  display: flex; align-items: center; gap: 6px; font-size: 11px;
}
.bar-cat-name { font-weight: 600; color: #334155; min-width: 60px; }
.bar-value {
  color: #1e293b; font-family: ui-monospace, monospace;
  font-size: 11px; font-weight: 500; margin-left: auto;
}
.bar-pct {
  color: #94a3b8; font-size: 10px; min-width: 38px; text-align: right;
}

.bar-track {
  height: 22px; background: #f1f5f9; border-radius: 4px; overflow: hidden; position: relative;
}
.bar-fill {
  height: 100%; border-radius: 4px; transition: width 0.35s ease;
  display: flex; align-items: center; padding-left: 8px; min-width: 2px;
}
.bar-fill-label {
  font-size: 11px; font-weight: 600; color: #fff;
  text-shadow: 0 1px 2px rgba(0,0,0,.3); white-space: nowrap;
}

/* ── Donut chart ─────────────────────────────────────────────────────── */
.donut-canvas { display: flex; justify-content: center; }
.donut-container {
  display: flex; align-items: center; gap: 20px;
}
.donut-legend { display: flex; flex-direction: column; gap: 6px; }
.donut-legend-item { display: flex; align-items: center; gap: 6px; font-size: 11px; }
.donut-legend-label { color: #334155; min-width: 55px; }
.donut-legend-pct {
  color: #94a3b8; font-family: ui-monospace,monospace; font-size: 10px;
}

/* ── Stats table ─────────────────────────────────────────────────────── */
.stats-section { margin-top: 16px; }
.stats-title { font-size: 12px; font-weight: 600; color: #475569; margin-bottom: 8px; }
.stats-section :deep(.ant-table) { font-size: 11px; }
.stats-section :deep(.ant-table-thead > tr > th) {
  font-size: 10px; padding: 4px 8px; background: #f8fafc;
}
.stats-section :deep(.ant-table-tbody > tr > td) { padding: 4px 8px; }

/* ── Zero state ──────────────────────────────────────────────────────── */
.zero-msg {
  text-align: center; padding: 28px 0; color: #94a3b8; font-size: 13px;
}
</style>
