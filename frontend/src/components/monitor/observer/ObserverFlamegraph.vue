<script setup lang="ts">
import { computed, ref } from "vue";
import type { ObserverEvent } from "../../../composables/monitor/useProcessObserver";

const props = withDefaults(defineProps<{
  events?: ObserverEvent[];
}>(), {
  events: () => [],
});

const metric = ref<"count" | "bytes">("count");

// Simple flamegraph: level 0 = event category (network/file/syscall/process/ssl),
// level 1 = comm, level 2 = type
const flameNodes = computed(() => {
  // Build hierarchy: category → comm → type
  const root: Record<string, any> = {};

  const cat = (e: ObserverEvent): string => {
    const t = (e.type || "").toLowerCase();
    if (t.includes("exec")||t.includes("fork")||t.includes("clone")||t.includes("exit")) return "Process";
    if (t.includes("open")||t.includes("read")||t.includes("write")||t.includes("mkdir")||t.includes("unlink")||t.includes("chmod")||t.includes("chown")||t.includes("rename")||t.includes("link")||t.includes("symlink")||t.includes("mknod")) return "File";
    if (t.includes("connect")||t.includes("bind")||t.includes("send")||t.includes("recv")||t.includes("socket")||t.includes("accept")||t.includes("dns")) return "Network";
    if (t.includes("tls")||t.includes("ssl")||t.includes("http")) return "TLS";
    return "Syscall";
  };

  for (const e of props.events) {
    const c = cat(e);
    const comm = e.comm || "unknown";
    const typ = e.type || "unknown";
    if (!root[c]) root[c] = {};
    if (!root[c][comm]) root[c][comm] = {};
    if (!root[c][comm][typ]) root[c][comm][typ] = { count: 0, bytes: 0 };
    root[c][comm][typ].count++;
    root[c][comm][typ].bytes += e.bytes || 0;
  }

  // Flatten into rectangles
  interface Rect { x: number; y: number; w: number; h: number; label: string; color: string; value: number; depth: number }
  const rects: Rect[] = [];
  const catColors: Record<string, string> = {
    Process: "#8b5cf6", File: "#06b6d4", Network: "#f59e0b", TLS: "#10b981", Syscall: "#6366f1",
  };
  const totalW = 1000;
  const rowH = 22;
  const m = metric.value;

  // Level 0: categories
  const catTotal = Object.values(root).reduce((s: number, c: any) => {
    return s + Object.values(c).reduce((s2: number, comm: any) => {
      return s2 + Object.values(comm as any).reduce((s3: number, t: any) => s3 + (t as any)[m], 0);
    }, 0);
  }, 0);
  if (catTotal === 0) return rects;

  let cx = 0;
  for (const [catName, comms] of Object.entries(root)) {
    let catVal = 0;
    for (const comm of Object.values(comms as any)) {
      for (const t of Object.values(comm as any)) catVal += (t as any)[m];
    }
    const cw = Math.max((catVal / catTotal) * totalW, 2);
    rects.push({ x: cx, y: 0, w: cw, h: rowH, label: catName, color: catColors[catName] || "#888", value: catVal, depth: 0 });
    cx += cw;
  }

  return rects;
});

const svgW = 1000;
const svgH = computed(() => {
  const maxY = Math.max(...flameNodes.value.map(r => r.y + r.h), 30);
  return Math.min(maxY + 10, 120);
});

const formatVal = (v: number): string =>
  metric.value === "count" ? v.toLocaleString() : v < 1024 ? `${v}B` : v < 1048576 ? `${(v/1024).toFixed(1)}KB` : `${(v/1048576).toFixed(1)}MB`;
</script>

<template>
  <div class="flame-root">
    <div class="flame-toolbar">
      <span class="flame-title">Event Flamegraph</span>
      <a-radio-group v-model:value="metric" size="small" button-style="solid">
        <a-radio-button value="count">Count</a-radio-button>
        <a-radio-button value="bytes">Bytes</a-radio-button>
      </a-radio-group>
    </div>

    <a-empty v-if="flameNodes.length === 0" description="No events to visualize" />

    <div v-else class="flame-canvas">
      <svg :viewBox="`0 0 ${svgW} ${svgH}`" preserveAspectRatio="xMidYMid meet" width="100%" :height="svgH">
        <rect
          v-for="r in flameNodes"
          :key="`${r.label}-${r.x}-${r.y}`"
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
      <div class="flame-legend">
        <span v-for="(color, name) in { Process:'#8b5cf6', File:'#06b6d4', Network:'#f59e0b', TLS:'#10b981', Syscall:'#6366f1' }" :key="name" class="legend-item">
          <span class="legend-dot" :style="{ background: color }"></span>{{ name }}
        </span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.flame-root { display: flex; flex-direction: column; }
.flame-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 10px;
}
.flame-title { font-size: 13px; font-weight: 600; color: #475569; }
.flame-canvas {
  background: linear-gradient(180deg, #fff 0%, #f8fafc 100%);
  border: 1px solid #e2e8f0; border-radius: 10px; padding: 8px;
}
.flame-legend { display: flex; gap: 12px; margin-top: 8px; font-size: 11px; color: #64748b; }
.legend-item { display: flex; align-items: center; gap: 4px; }
.legend-dot { width: 8px; height: 8px; border-radius: 2px; display: inline-block; }
</style>
