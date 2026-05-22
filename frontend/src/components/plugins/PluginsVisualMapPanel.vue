<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  mode: "NONE" | "COUNTER" | "BLOCKLIST";
  keyField: "uid" | "pid" | "comm";
  limit: number;
}>();

const emit = defineEmits<{
  (e: "update:mode", val: "NONE" | "COUNTER" | "BLOCKLIST"): void;
  (e: "update:keyField", val: "uid" | "pid" | "comm"): void;
  (e: "update:limit", val: number): void;
}>();

const localMode = computed({
  get: () => props.mode,
  set: (val) => emit("update:mode", val),
});

const localKeyField = computed({
  get: () => props.keyField,
  set: (val) => emit("update:keyField", val),
});

const localLimit = computed({
  get: () => props.limit,
  set: (val) => emit("update:limit", val),
});
</script>

<template>
  <div class="block-card block-map">
    <!-- Node ports for visual wire connections in parents -->
    <div class="node-port port-input map-port-in"></div>
    <div class="node-port port-output map-port-out"></div>

    <div class="block-header">
      <span class="block-badge">Block 2.5</span>
      <strong style="color: #fff">低代码 Map 状态化存储积木 (Map Stateful Operations)</strong>
    </div>

    <div class="block-body">
      <div class="desc-line">
        选择是否启用 BPF 内核高性能 Map Stateful 数据流运算进行状态化追踪判定：
      </div>

      <a-row :gutter="16">
        <!-- Configuration Columns -->
        <a-col :span="11">
          <div class="control-group">
            <label>Map 运行模式 (Map Mode)</label>
            <a-select v-model:value="localMode" style="width: 100%">
              <a-select-option value="NONE">无状态 (直接决策)</a-select-option>
              <a-select-option value="COUNTER">计数器限频 (COUNTER)</a-select-option>
              <a-select-option value="BLOCKLIST">外部 Hash 黑名单判定 (BLOCKLIST)</a-select-option>
            </a-select>
          </div>

          <a-row :gutter="12" style="margin-top: 12px;">
            <a-col :span="12" v-if="localMode !== 'NONE'">
              <label>操作追踪主键 (Map Key)</label>
              <a-select v-model:value="localKeyField" style="width: 100%">
                <a-select-option value="pid">当前进程 PID</a-select-option>
                <a-select-option value="uid">当前用户 UID</a-select-option>
                <a-select-option value="comm">当前进程名 (Comm)</a-select-option>
              </a-select>
            </a-col>
            <a-col :span="12" v-if="localMode === 'COUNTER'">
              <label>阈值限制 (Max Hits)</label>
              <a-input-number v-model:value="localLimit" :min="1" :max="10000" style="width: 100%" />
            </a-col>
          </a-row>

          <div v-if="localMode !== 'NONE'" class="helper-text-note">
            * 状态机制将自动在内核声明 eBPF HASH 映射表。满足以上累计命中过滤规则的阈值条件后，才下发执行 Block 3 终极动作。
          </div>
        </a-col>

        <!-- Live Visual Blueprint Map Diagram -->
        <a-col :span="13" style="border-left: 1px dashed rgba(255, 255, 255, 0.1); padding-left: 16px;">
          <div class="canvas-title">
            <span>数据流向与状态映射拓扑 (Map Topology)</span>
            <a-tag size="small" color="blue" style="font-size: 10px; margin: 0; transform: scale(0.9);">
              {{ localMode === 'NONE' ? 'Bypass' : localMode }}
            </a-tag>
          </div>

          <div class="map-blueprint-canvas">
            <!-- Background Grid -->
            <div class="map-blueprint-grid"></div>

            <!-- SVG Wires for Map topology -->
            <svg class="map-blueprint-wires" viewBox="0 0 400 180" width="100%" height="100%" preserveAspectRatio="none">
              <defs>
                <linearGradient id="map-wire-gradient" x1="0%" y1="0%" x2="100%" y2="0%">
                  <stop offset="0%" stop-color="#fa8c16" />
                  <stop offset="50%" stop-color="#722ed1" />
                  <stop offset="100%" stop-color="#52c41a" />
                </linearGradient>
                <filter id="map-wire-glow">
                  <feGaussianBlur stdDeviation="1.5" result="coloredBlur"/>
                  <feMerge>
                    <feMergeNode in="coloredBlur"/>
                    <feMergeNode in="SourceGraphic"/>
                  </feMerge>
                </filter>
              </defs>

              <!-- NONE mode wires -->
              <g v-if="localMode === 'NONE'">
                <path
                  d="M 60 90 L 340 90"
                  stroke="#52c41a"
                  stroke-width="2"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
              </g>

              <!-- COUNTER mode wires -->
              <g v-else-if="localMode === 'COUNTER'">
                <!-- Event to Extract -->
                <path
                  d="M 60 90 C 80 90, 100 45, 115 45"
                  stroke="#fa8c16"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Extract to Map -->
                <path
                  d="M 195 45 C 210 45, 205 135, 215 135"
                  stroke="#722ed1"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Map to Threshold -->
                <path
                  d="M 335 135 C 350 135, 340 90, 360 90"
                  stroke="#52c41a"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
              </g>

              <!-- BLOCKLIST mode wires -->
              <g v-else-if="localMode === 'BLOCKLIST'">
                <!-- Event to Map Check -->
                <path
                  d="M 60 90 C 80 90, 100 45, 115 45"
                  stroke="#1890ff"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Map Check to Decision -->
                <path
                  d="M 195 45 C 210 45, 205 135, 215 135"
                  stroke="#722ed1"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Decision to Action -->
                <path
                  d="M 335 135 C 350 135, 340 90, 360 90"
                  stroke="#52c41a"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
              </g>
            </svg>

            <!-- HTML Badges overlayed on fixed positions matching the SVG -->
            <!-- ALL MODES: Event Input (Left) -->
            <div class="map-node node-left">
              <div class="node-title">EVENT IN</div>
              <div class="node-content">Incoming</div>
            </div>

            <!-- ALL MODES: Target Action (Right) -->
            <div class="map-node node-right">
              <div class="node-title" style="color: #52c41a; border-bottom-color: rgba(82,196,26,0.3)">DECISION</div>
              <div class="node-content">Trigger Action</div>
            </div>

            <!-- NONE Mode Center Node -->
            <div v-if="localMode === 'NONE'" class="map-node node-center-bypass">
              <div class="node-title" style="color: #13c2c2; border-bottom-color: rgba(19,194,194,0.3)">BYPASS</div>
              <div class="node-content">No State (Direct)</div>
            </div>

            <!-- COUNTER Mode Center Nodes -->
            <template v-if="localMode === 'COUNTER'">
              <!-- Key Extractor (Mid-Left, top) -->
              <div class="map-node node-mid-left">
                <div class="node-title" style="color: #fa8c16; border-bottom-color: rgba(250,140,22,0.3)">EXTRACT KEY</div>
                <div class="node-content val-highlight">{{ localKeyField.toUpperCase() }}</div>
              </div>

              <!-- rate_limit_map (Mid-Right, bottom) -->
              <div class="map-node node-mid-right map-db-node">
                <div class="node-title" style="color: #722ed1; border-bottom-color: rgba(114,46,209,0.3); display:flex; justify-content:space-between; align-items:center;">
                  <span>rate_limit_map</span>
                  <span class="db-type">HASH</span>
                </div>
                <div class="db-grid">
                  <div class="db-row font-mono">
                    <span class="db-k">key({{ localKeyField }})</span>
                    <span class="db-v">hits</span>
                  </div>
                  <div class="db-row font-mono">
                    <span class="db-k">#{{ localKeyField === 'comm' ? 'nc' : '1001' }}</span>
                    <span class="db-v val-warn">{{ localLimit > 2 ? localLimit - 1 : 1 }} / {{ localLimit }}</span>
                  </div>
                  <div class="db-row font-mono trigger-row">
                    <span class="db-k">#{{ localKeyField === 'comm' ? 'curl' : '1024' }}</span>
                    <span class="db-v val-danger">&gt;{{ localLimit }} 💥</span>
                  </div>
                </div>
              </div>
            </template>

            <!-- BLOCKLIST Mode Center Nodes -->
            <template v-if="localMode === 'BLOCKLIST'">
              <!-- Key Lookup (Mid-Left, top) -->
              <div class="map-node node-mid-left">
                <div class="node-title" style="color: #1890ff; border-bottom-color: rgba(24,144,255,0.3)">LOOKUP KEY</div>
                <div class="node-content val-highlight">{{ localKeyField.toUpperCase() }}</div>
              </div>

              <!-- blocklist_map (Mid-Right, bottom) -->
              <div class="map-node node-mid-right map-db-node">
                <div class="node-title" style="color: #722ed1; border-bottom-color: rgba(114,46,209,0.3); display:flex; justify-content:space-between; align-items:center;">
                  <span>blocklist_map</span>
                  <span class="db-type">HASH</span>
                </div>
                <div class="db-grid">
                  <div class="db-row font-mono">
                    <span class="db-k">key({{ localKeyField }})</span>
                    <span class="db-v">blocked</span>
                  </div>
                  <div class="db-row font-mono trigger-row">
                    <span class="db-k">#{{ localKeyField === 'comm' ? 'nc' : '0' }}</span>
                    <span class="db-v val-danger">TRUE 🚫</span>
                  </div>
                  <div class="db-row font-mono">
                    <span class="db-k">#{{ localKeyField === 'comm' ? 'python' : '1000' }}</span>
                    <span class="db-v">FALSE</span>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </a-col>
      </a-row>
    </div>
  </div>
</template>

<style scoped>
/* Blueprint nodes styling */
.block-card {
  border-radius: 8px;
  overflow: visible; /* to show ports */
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  background: rgba(13, 19, 33, 0.85);
  backdrop-filter: blur(8px);
  transition: all 0.3s ease;
  border: 1px solid rgba(255, 255, 255, 0.08);
}
.block-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.7);
}

.block-map {
  border-color: rgba(47, 84, 235, 0.35) !important;
}
.block-map:hover {
  border-color: rgba(47, 84, 235, 0.7) !important;
  box-shadow: 0 0 15px rgba(47, 84, 235, 0.2);
}

.block-header {
  padding: 10px 14px;
  display: flex;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  background: linear-gradient(135deg, #2f54eb, #12005e);
}
.block-badge {
  background: rgba(0, 0, 0, 0.35);
  color: white;
  padding: 2px 8px;
  font-size: 11px;
  border-radius: 4px;
  margin-right: 12px;
  font-weight: bold;
}
.block-body {
  background: #0f172a;
  padding: 18px;
  color: #cbd5e1;
}
.desc-line {
  font-size: 13px;
  color: #94a3b8;
  margin-bottom: 12px;
}

.control-group label {
  display: block;
  font-size: 11px;
  color: #94a3b8;
  margin-bottom: 4px;
}

.helper-text-note {
  color: #5c7cfa;
  margin-top: 10px;
  font-size: 11px;
}

.canvas-title {
  font-size: 12px;
  font-weight: 600;
  color: #2f54eb;
  margin-bottom: 8px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.map-blueprint-canvas {
  height: 180px;
  position: relative;
  border: 1px solid rgba(47, 84, 235, 0.25);
  background: rgba(13, 19, 33, 0.5);
  border-radius: 6px;
  overflow: hidden;
  box-shadow: inset 0 0 15px rgba(0, 0, 0, 0.5);
}

.map-blueprint-grid {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-image: 
    linear-gradient(to right, rgba(47, 84, 235, 0.05) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(47, 84, 235, 0.05) 1px, transparent 1px);
  background-size: 15px 15px;
  pointer-events: none;
}

.map-blueprint-wires {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 1;
}

/* Glowing Flowing Dash Animation */
.flowing-dash {
  stroke-dasharray: 6, 8;
  animation: map-dash 1.5s linear infinite;
}

@keyframes map-dash {
  to {
    stroke-dashoffset: -28;
  }
}

/* Map node cards */
.map-node {
  position: absolute;
  background: rgba(15, 23, 42, 0.9);
  border: 1px solid rgba(47, 84, 235, 0.4);
  border-radius: 5px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.5);
  font-family: monospace;
  font-size: 10px;
  color: #e2e8f0;
  z-index: 2;
  box-sizing: border-box;
  transition: all 0.2s ease;
}

.map-node:hover {
  border-color: #2f54eb;
  box-shadow: 0 0 8px rgba(47, 84, 235, 0.4);
}

.node-title {
  padding: 3px 6px;
  background: rgba(47, 84, 235, 0.1);
  border-bottom: 1px solid rgba(47, 84, 235, 0.2);
  font-weight: bold;
  color: #89a5df;
}

.node-content {
  padding: 6px;
  text-align: center;
}

.val-highlight {
  color: #38bdf8;
  font-weight: bold;
}

/* Staggered node placements (percentages match SVG coordinates of viewBox 0 0 400 180) */
/* Node left: x = 60, y = 90 */
.node-left {
  left: 15px;
  top: 50%;
  transform: translateY(-50%);
  width: 80px;
}

/* Node right: x = 360, y = 90 */
.node-right {
  right: 15px;
  top: 50%;
  transform: translateY(-50%);
  width: 90px;
}

/* Node center bypass (NONE): x = 200, y = 90 */
.node-center-bypass {
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  width: 100px;
}

/* Node mid-left (COUNTER/BLOCKLIST): x = 115..195, y = 45 */
.node-mid-left {
  left: 27%;
  top: 15px;
  width: 90px;
}

/* Node mid-right (COUNTER/BLOCKLIST): x = 215..295, y = 135 */
.node-mid-right {
  left: 50%;
  bottom: 10px;
  width: 135px;
}

/* Database map styles inside the node */
.map-db-node {
  border-color: rgba(114, 46, 209, 0.4);
}
.map-db-node:hover {
  border-color: #722ed1;
  box-shadow: 0 0 8px rgba(114, 46, 209, 0.4);
}
.db-type {
  font-size: 8px;
  background: rgba(114, 46, 209, 0.3);
  padding: 0px 3px;
  border-radius: 2px;
  color: #d3adf7;
}
.db-grid {
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.db-row {
  display: flex;
  justify-content: space-between;
  border-bottom: 1px solid rgba(255, 255, 255, 0.03);
  padding: 1px 2px;
  font-size: 8px;
}
.db-row:last-child {
  border-bottom: none;
}
.db-k {
  color: #94a3b8;
}
.db-v {
  color: #10b981;
}
.val-warn {
  color: #f59e0b;
}
.val-danger {
  color: #ef4444;
  font-weight: bold;
}
.trigger-row {
  background: rgba(239, 68, 68, 0.1);
  border-radius: 2px;
}

/* Node ports */
.node-port {
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  left: 50%;
  transform: translateX(-50%);
  z-index: 10;
  border: 1px solid rgba(255, 255, 255, 0.6);
}
.port-input {
  top: -5px;
}
.port-output {
  bottom: -5px;
}
.map-port-in {
  background: #fa8c16;
  border-color: #fa8c16;
  box-shadow: 0 0 8px #fa8c16;
}
.map-port-out {
  background: #722ed1;
  border-color: #722ed1;
  box-shadow: 0 0 8px #722ed1;
}

/* Dark theme form styling */
:deep(.ant-select-selector),
:deep(.ant-input-number) {
  background-color: #1e293b !important;
  border-color: #334155 !important;
  color: #f1f5f9 !important;
}
:deep(.ant-select-arrow) {
  color: #94a3b8 !important;
}
</style>
