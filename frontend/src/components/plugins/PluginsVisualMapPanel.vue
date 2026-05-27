<script setup lang="ts">
import { computed } from "vue";
import "./PluginsVisualMapPanel.css";

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
                  <stop offset="0%" stop-color="#1677ff" />
                  <stop offset="50%" stop-color="#4096ff" />
                  <stop offset="100%" stop-color="#1677ff" />
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
                  stroke="#1677ff"
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
                  stroke="#1677ff"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Extract to Map -->
                <path
                  d="M 195 45 C 210 45, 205 135, 215 135"
                  stroke="#4096ff"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Map to Threshold -->
                <path
                  d="M 335 135 C 350 135, 340 90, 360 90"
                  stroke="#1677ff"
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
                  stroke="#1677ff"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Map Check to Decision -->
                <path
                  d="M 195 45 C 210 45, 205 135, 215 135"
                  stroke="#4096ff"
                  stroke-width="1.5"
                  fill="none"
                  filter="url(#map-wire-glow)"
                  class="flowing-dash"
                />
                <!-- Decision to Action -->
                <path
                  d="M 335 135 C 350 135, 340 90, 360 90"
                  stroke="#1677ff"
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
              <div class="node-title" style="color: #1677ff; border-bottom-color: #91caff">DECISION</div>
              <div class="node-content">Trigger Action</div>
            </div>

            <!-- NONE Mode Center Node -->
            <div v-if="localMode === 'NONE'" class="map-node node-center-bypass">
              <div class="node-title" style="color: #4096ff; border-bottom-color: #91caff">BYPASS</div>
              <div class="node-content">No State (Direct)</div>
            </div>

            <!-- COUNTER Mode Center Nodes -->
            <template v-if="localMode === 'COUNTER'">
              <!-- Key Extractor (Mid-Left, top) -->
              <div class="map-node node-mid-left">
                <div class="node-title" style="color: #1677ff; border-bottom-color: rgba(250,140,22,0.3)">EXTRACT KEY</div>
                <div class="node-content val-highlight">{{ localKeyField.toUpperCase() }}</div>
              </div>

              <!-- rate_limit_map (Mid-Right, bottom) -->
              <div class="map-node node-mid-right map-db-node">
                <div class="node-title" style="color: #4096ff; border-bottom-color: rgba(114,46,209,0.3); display:flex; justify-content:space-between; align-items:center;">
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
                <div class="node-title" style="color: #1677ff; border-bottom-color: #91caff">LOOKUP KEY</div>
                <div class="node-content val-highlight">{{ localKeyField.toUpperCase() }}</div>
              </div>

              <!-- blocklist_map (Mid-Right, bottom) -->
              <div class="map-node node-mid-right map-db-node">
                <div class="node-title" style="color: #4096ff; border-bottom-color: rgba(114,46,209,0.3); display:flex; justify-content:space-between; align-items:center;">
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
