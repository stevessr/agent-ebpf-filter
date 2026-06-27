<script setup lang="ts">
import { computed, ref } from "vue";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  CodeOutlined,
  FileTextOutlined,
  GlobalOutlined,
  ApartmentOutlined,
  LockOutlined,
  ClearOutlined,
} from "@ant-design/icons-vue";
import type { ObserverEvent } from "../../../composables/monitor/useProcessObserver";

const props = defineProps<{
  events: ObserverEvent[];
  selectedPid: number | null;
}>();

const emit = defineEmits<{
  clear: [];
  selectPid: [pid: number];
}>();

const expanded = ref<Set<string>>(new Set());

const toggle = (key: string) => {
  const s = new Set(expanded.value);
  if (s.has(key)) s.delete(key);
  else s.add(key);
  expanded.value = s;
};

// Sort newest first
const sorted = computed(() => [...props.events].sort((a, b) => b.timestamp - a.timestamp));

// Event type classification for visual styling
type EventStyle = { color: string; border: string; bg: string; icon: any; label: string };
const eventStyle = (e: ObserverEvent): EventStyle => {
  const t = e.type?.toLowerCase() || "";
  // Process lifecycle
  if (t.includes("exec") || t.includes("fork") || t.includes("clone") || t.includes("exit"))
    return { color: "purple", border: "#8b5cf6", bg: "linear-gradient(90deg,#f5f3ff,#faf5ff)", icon: ApartmentOutlined, label: "Process" };
  // File access
  if (t.includes("open") || t.includes("read") || t.includes("write") || t.includes("mkdir") || t.includes("unlink") || t.includes("chmod") || t.includes("chown") || t.includes("rename") || t.includes("link") || t.includes("symlink") || t.includes("mknod"))
    return { color: "cyan", border: "#06b6d4", bg: "linear-gradient(90deg,#ecfeff,#eff6ff)", icon: FileTextOutlined, label: "File" };
  // Network
  if (t.includes("connect") || t.includes("bind") || t.includes("send") || t.includes("recv") || t.includes("socket") || t.includes("accept") || t.includes("dns"))
    return { color: "orange", border: "#f59e0b", bg: "linear-gradient(90deg,#fff7ed,#fffbeb)", icon: GlobalOutlined, label: "Network" };
  // TLS/SSL
  if (t.includes("tls") || t.includes("ssl") || t.includes("http"))
    return { color: "green", border: "#10b981", bg: "linear-gradient(90deg,#ecfdf5,#f0fdfa)", icon: LockOutlined, label: "TLS" };
  // Default syscall
  return { color: "geekblue", border: "#6366f1", bg: "linear-gradient(90deg,#eef2ff,#f0f9ff)", icon: CodeOutlined, label: "Syscall" };
};

// Build a short preview line
const preview = (e: ObserverEvent): string => {
  const parts: string[] = [];
  if (e.comm) parts.push(e.comm);
  if (e.path) parts.push(e.path);
  if (e.extraInfo) parts.push(e.extraInfo);
  return parts.join(" → ") || e.type;
};
</script>

<template>
  <div class="timeline-root">
    <div class="timeline-toolbar">
      <span class="toolbar-count">{{ sorted.length }} events</span>
      <a-button size="small" type="link" danger @click="emit('clear')">
        <ClearOutlined /> Clear
      </a-button>
    </div>

    <a-empty
      v-if="sorted.length === 0"
      description="No events yet — select a PID and wait for activity"
      style="margin-top: 40px"
    />

    <div v-else class="timeline-list">
      <div
        v-for="e in sorted"
        :key="e.key"
        class="event-block"
        :style="{ borderLeft: `4px solid ${eventStyle(e).border}`, background: eventStyle(e).bg }"
      >
        <div class="event-head" @click="toggle(e.key)">
          <span class="event-icon">
            <component :is="eventStyle(e).icon" />
          </span>
          <a-tag :color="eventStyle(e).color" size="small">{{ eventStyle(e).label }}</a-tag>
          <span class="event-type">{{ e.type.toUpperCase() }}</span>
          <span class="event-comm">{{ e.comm }}</span>
          <a-tag
            v-if="e.pid"
            color="processing"
            size="small"
            class="pid-tag"
            @click.stop="emit('selectPid', e.pid)"
          >
            PID {{ e.pid }}
          </a-tag>
          <span class="event-time">{{ e.time }}</span>
          <span class="expand-icon">
            <CaretDownOutlined v-if="expanded.has(e.key)" />
            <CaretRightOutlined v-else />
          </span>
        </div>

        <div class="event-preview" v-if="!expanded.has(e.key)">
          {{ preview(e) }}
        </div>

        <div v-if="expanded.has(e.key)" class="event-detail">
          <div class="detail-row" v-if="e.path"><span class="dl">Path</span><code>{{ e.path }}</code></div>
          <div class="detail-row" v-if="e.extraInfo"><span class="dl">Info</span><code>{{ e.extraInfo }}</code></div>
          <div class="detail-row" v-if="e.bytes"><span class="dl">Bytes</span><span>{{ e.bytes.toLocaleString() }}</span></div>
          <div class="detail-row" v-if="e.retval"><span class="dl">Retval</span><code>{{ e.retval }}</code></div>
          <div class="detail-row"><span class="dl">PPID</span><span>{{ e.ppid }}</span></div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.timeline-root { display: flex; flex-direction: column; }
.timeline-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 8px; padding-bottom: 6px;
  border-bottom: 1px solid #f0f0f0;
}
.toolbar-count { font-size: 12px; color: #888; }

.timeline-list { display: flex; flex-direction: column; gap: 6px; max-height: 520px; overflow-y: auto; }

.event-block {
  border-radius: 8px; padding: 6px 10px;
  box-shadow: 0 1px 3px rgba(15,23,42,.06);
  transition: box-shadow 0.15s;
}
.event-block:hover { box-shadow: 0 2px 6px rgba(15,23,42,.12); }

.event-head {
  display: flex; align-items: center; gap: 6px; cursor: pointer;
  user-select: none;
}
.event-icon { color: #64748b; font-size: 13px; display: flex; }
.event-type {
  font-family: ui-monospace, monospace; font-size: 11px;
  color: #475569; font-weight: 600; min-width: 70px;
}
.event-comm {
  font-family: ui-monospace, monospace; font-size: 12px;
  color: #1e293b; font-weight: 500; flex: 1; overflow: hidden;
  text-overflow: ellipsis; white-space: nowrap;
}
.pid-tag { cursor: pointer; }
.event-time { font-size: 11px; color: #94a3b8; white-space: nowrap; font-family: ui-monospace,monospace; }
.expand-icon { color: #94a3b8; font-size: 10px; }

.event-preview {
  margin-top: 4px; padding-left: 24px; font-size: 12px;
  color: #64748b; overflow: hidden; text-overflow: ellipsis;
  white-space: nowrap; font-family: ui-monospace,monospace;
}

.event-detail {
  margin-top: 6px; padding: 8px 12px; background: rgba(255,255,255,.6);
  border-radius: 6px; font-size: 12px; display: flex; flex-direction: column; gap: 4px;
}
.detail-row { display: flex; gap: 8px; align-items: baseline; }
.dl { color: #94a3b8; min-width: 45px; font-size: 11px; text-transform: uppercase; }
.detail-row code { font-family: ui-monospace,monospace; color: #0f172a; font-size: 12px; word-break: break-all; }
</style>
