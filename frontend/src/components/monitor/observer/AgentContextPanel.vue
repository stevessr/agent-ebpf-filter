<script setup lang="ts">
import { computed, ref } from "vue";
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  MergeCellsOutlined,
  EyeOutlined,
  CaretDownOutlined,
  CaretRightOutlined,
} from "@ant-design/icons-vue";
import type { ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";

const props = defineProps<{
  events: ObserverTLSEvent[];
}>();

const emit = defineEmits<{
  viewEvent: [event: ObserverTLSEvent];
}>();

// ── Hex decoder ──────────────────────────────────────────────────────────
const hexToText = (hex: string, maxLen: number = 500): string => {
  try {
    const bytes: number[] = [];
    for (let i = 0; i < hex.length - 1; i += 2) {
      const byte = parseInt(hex.slice(i, i + 2), 16);
      if (isNaN(byte)) break;
      bytes.push(byte);
    }
    const text = bytes
      .map((b) => (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d ? String.fromCharCode(b) : ".")
      .join("");
    return text.length > maxLen ? text.slice(0, maxLen) + "…" : text;
  } catch { return ""; }
};

const getBodyText = (ev: ObserverTLSEvent): string => {
  if (ev.body) return ev.body;
  if (ev.raw_hex_dump) return hexToText(ev.raw_hex_dump, 300);
  return "";
};

// ── Merged event group ───────────────────────────────────────────────────
interface MergedGroup {
  id: string;
  type: "request" | "response" | "sse_stream" | "raw";
  events: ObserverTLSEvent[];
  method?: string;
  url?: string;
  host?: string;
  status?: number;
  startTime: string;
  endTime: string;
  mergedBody?: string;
  totalSize: number;
}

// ── Group & merge events ─────────────────────────────────────────────────

const streamGroups = computed(() => {
  const list = [...props.events].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );

  // Group by direction first
  const sendEvents = list.filter((e) => e.direction === "send");
  const recvEvents = list.filter((e) => e.direction !== "send");

  return {
    send: mergeConsecutive(sendEvents, "send"),
    recv: mergeConsecutive(recvEvents, "recv"),
  };
});

const mergeConsecutive = (events: ObserverTLSEvent[], _direction: string): MergedGroup[] => {
  if (events.length === 0) return [];

  const groups: MergedGroup[] = [];
  let i = 0;
  let groupId = 0;

  while (i < events.length) {
    const ev = events[i];

    // Check if this starts an SSE stream (consecutive sse_message events)
    if (ev.type === "sse_message") {
      const sseGroup: ObserverTLSEvent[] = [ev];
      let j = i + 1;
      while (j < events.length && events[j].type === "sse_message") {
        sseGroup.push(events[j]);
        j++;
      }
      // Merge bodies
      const mergedBody = sseGroup
        .map((e) => {
          const body = getBodyText(e);
          // Extract just the "data:" content from SSE lines
          const lines = body.split("\n");
          const dataLines = lines
            .filter((l) => l.startsWith("data:"))
            .map((l) => l.slice(5).trim());
          return dataLines.join("\n") || body;
        })
        .filter(Boolean)
        .join("\n");

      groups.push({
        id: `sse-${groupId++}`,
        type: "sse_stream",
        events: sseGroup,
        host: ev.host || undefined,
        startTime: sseGroup[0].timestamp,
        endTime: sseGroup[sseGroup.length - 1].timestamp,
        mergedBody,
        totalSize: sseGroup.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
      });
      i = j;
      continue;
    }

    // HTTP request
    if (ev.type === "http_request") {
      groups.push({
        id: `req-${groupId++}`,
        type: "request",
        events: [ev],
        method: ev.method || undefined,
        url: ev.url || undefined,
        host: ev.host || undefined,
        startTime: ev.timestamp,
        endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
      });
      i++;
      continue;
    }

    // HTTP response
    if (ev.type === "http_response") {
      groups.push({
        id: `resp-${groupId++}`,
        type: "response",
        events: [ev],
        status: ev.status || undefined,
        host: ev.host || undefined,
        startTime: ev.timestamp,
        endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
      });
      i++;
      continue;
    }

    // Raw event — group consecutive raw events together
    const rawGroup: ObserverTLSEvent[] = [ev];
    let k = i + 1;
    while (k < events.length && events[k].type === "tls_plaintext") {
      rawGroup.push(events[k]);
      k++;
    }
    groups.push({
      id: `raw-${groupId++}`,
      type: "raw",
      events: rawGroup,
      startTime: rawGroup[0].timestamp,
      endTime: rawGroup[rawGroup.length - 1].timestamp,
      totalSize: rawGroup.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
    });
    i = k;
  }

  return groups;
};

// ── Expanded state ───────────────────────────────────────────────────────
const expanded = ref<Set<string>>(new Set());
const toggle = (id: string) => {
  const s = new Set(expanded.value);
  if (s.has(id)) s.delete(id);
  else s.add(id);
  expanded.value = s;
};

// ── Stats ────────────────────────────────────────────────────────────────
const stats = computed(() => ({
  sendCount: streamGroups.value.send.length,
  recvCount: streamGroups.value.recv.length,
  sendBytes: streamGroups.value.send.reduce((s, g) => s + g.totalSize, 0),
  recvBytes: streamGroups.value.recv.reduce((s, g) => s + g.totalSize, 0),
}));

const formatBytes = (bytes: number): string => {
  if (!bytes) return "0 B";
  const u = ["B", "KB", "MB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), u.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

const formatTime = (ts: string): string => {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString();
  } catch { return ts?.slice(11, 19) || ""; }
};
</script>

<template>
  <div class="ac-root">
    <!-- Stats bar -->
    <div class="ac-stats">
      <div class="ac-stat-item send">
        <ArrowUpOutlined />
        <span class="ac-stat-label">Upstream</span>
        <span class="ac-stat-val">{{ stats.sendCount }} groups</span>
        <span class="ac-stat-size">{{ formatBytes(stats.sendBytes) }}</span>
      </div>
      <div class="ac-stat-item recv">
        <ArrowDownOutlined />
        <span class="ac-stat-label">Downstream</span>
        <span class="ac-stat-val">{{ stats.recvCount }} groups</span>
        <span class="ac-stat-size">{{ formatBytes(stats.recvBytes) }}</span>
      </div>
    </div>

    <!-- Two-column layout -->
    <div class="ac-columns">
      <!-- Upstream (send) -->
      <div class="ac-col ac-send-col">
        <div class="ac-col-header">
          <ArrowUpOutlined style="color: #f59e0b" />
          <span>Upstream (Agent → Server)</span>
        </div>
        <a-empty
          v-if="streamGroups.send.length === 0"
          description="No upstream data"
          style="padding: 24px"
        />
        <div v-else class="ac-event-list">
          <div
            v-for="group in streamGroups.send"
            :key="group.id"
            class="ac-event-card"
            :class="{ 'ac-sse': group.type === 'sse_stream' }"
            @click="toggle(group.id)"
          >
            <div class="ac-card-head">
              <span class="ac-expand-icon">
                <CaretDownOutlined v-if="expanded.has(group.id)" />
                <CaretRightOutlined v-else />
              </span>
              <a-tag v-if="group.type === 'request'" color="blue" size="small">REQ</a-tag>
              <a-tag v-else-if="group.type === 'response'" color="green" size="small">RESP</a-tag>
              <a-tag v-else-if="group.type === 'sse_stream'" color="purple" size="small">SSE</a-tag>
              <a-tag v-else color="default" size="small">RAW</a-tag>
              <span class="ac-card-method">{{ group.method || group.type }}</span>
              <span class="ac-card-host" v-if="group.host">{{ group.host }}</span>
              <span class="ac-card-size">{{ formatBytes(group.totalSize) }}</span>
              <span class="ac-card-time">{{ formatTime(group.startTime) }}</span>
              <a-button
                v-if="group.events.length === 1"
                size="small"
                type="link"
                class="ac-view-btn"
                @click.stop="emit('viewEvent', group.events[0])"
              >
                <EyeOutlined />
              </a-button>
            </div>
            <!-- Merged SSE indicator -->
            <div v-if="group.type === 'sse_stream'" class="ac-merged-badge">
              <MergeCellsOutlined /> {{ group.events.length }} SSE events merged
            </div>
            <!-- Expanded content -->
            <div v-if="expanded.has(group.id)" class="ac-card-body">
              <div v-if="group.url" class="ac-row"><span class="ac-k">URL</span><code>{{ group.url }}</code></div>
              <div v-if="group.mergedBody" class="ac-body-box">
                <pre>{{ group.mergedBody.slice(0, 2000) }}</pre>
              </div>
              <div v-else v-for="ev in group.events" :key="ev.key" class="ac-body-box">
                <pre>{{ getBodyText(ev).slice(0, 1000) || '(empty)' }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Downstream (recv) -->
      <div class="ac-col ac-recv-col">
        <div class="ac-col-header">
          <ArrowDownOutlined style="color: #06b6d4" />
          <span>Downstream (Server → Agent)</span>
        </div>
        <a-empty
          v-if="streamGroups.recv.length === 0"
          description="No downstream data"
          style="padding: 24px"
        />
        <div v-else class="ac-event-list">
          <div
            v-for="group in streamGroups.recv"
            :key="group.id"
            class="ac-event-card"
            :class="{ 'ac-sse': group.type === 'sse_stream' }"
            @click="toggle(group.id)"
          >
            <div class="ac-card-head">
              <span class="ac-expand-icon">
                <CaretDownOutlined v-if="expanded.has(group.id)" />
                <CaretRightOutlined v-else />
              </span>
              <a-tag v-if="group.type === 'request'" color="blue" size="small">REQ</a-tag>
              <a-tag v-else-if="group.type === 'response'" color="green" size="small">RESP</a-tag>
              <a-tag v-else-if="group.type === 'sse_stream'" color="purple" size="small">SSE</a-tag>
              <a-tag v-else color="default" size="small">RAW</a-tag>
              <span class="ac-card-method">
                {{ group.status ? `HTTP ${group.status}` : group.type }}
              </span>
              <span class="ac-card-host" v-if="group.host">{{ group.host }}</span>
              <span class="ac-card-size">{{ formatBytes(group.totalSize) }}</span>
              <span class="ac-card-time">{{ formatTime(group.startTime) }}</span>
              <a-button
                v-if="group.events.length === 1"
                size="small"
                type="link"
                class="ac-view-btn"
                @click.stop="emit('viewEvent', group.events[0])"
              >
                <EyeOutlined />
              </a-button>
            </div>
            <div v-if="group.type === 'sse_stream'" class="ac-merged-badge">
              <MergeCellsOutlined /> {{ group.events.length }} SSE events merged
            </div>
            <div v-if="expanded.has(group.id)" class="ac-card-body">
              <div v-if="group.status" class="ac-row"><span class="ac-k">Status</span><span>{{ group.status }}</span></div>
              <div v-if="group.mergedBody" class="ac-body-box">
                <pre>{{ group.mergedBody.slice(0, 2000) }}</pre>
              </div>
              <div v-else v-for="ev in group.events" :key="ev.key" class="ac-body-box">
                <pre>{{ getBodyText(ev).slice(0, 1000) || '(empty)' }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ac-root {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ac-stats {
  display: flex;
  gap: 16px;
  padding: 8px 12px;
  background: #f8fafc;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
}

.ac-stat-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.ac-stat-item.send { color: #d97706; }
.ac-stat-item.recv { color: #059669; }

.ac-stat-label {
  font-weight: 600;
  color: #475569;
}

.ac-stat-val {
  color: #64748b;
}

.ac-stat-size {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  color: #334155;
}

.ac-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  max-height: 560px;
}

.ac-col {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fafafa;
}

.ac-send-col { border-left: 3px solid #f59e0b; }
.ac-recv-col { border-left: 3px solid #06b6d4; }

.ac-col-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 600;
  color: #334155;
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
  position: sticky;
  top: 0;
  z-index: 1;
}

.ac-event-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px;
}

.ac-event-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  cursor: pointer;
  transition: box-shadow 0.15s;
}

.ac-event-card:hover {
  box-shadow: 0 1px 4px rgba(0,0,0,.08);
}

.ac-event-card.ac-sse {
  border-color: #c084fc;
  background: #faf5ff;
}

.ac-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
}

.ac-expand-icon {
  font-size: 9px;
  color: #94a3b8;
}

.ac-card-method {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  font-weight: 600;
  color: #1e293b;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ac-card-host {
  font-size: 10px;
  color: #64748b;
  font-family: ui-monospace, monospace;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ac-card-size {
  font-size: 10px;
  color: #94a3b8;
  font-family: ui-monospace, monospace;
}

.ac-card-time {
  font-size: 10px;
  color: #94a3b8;
  font-family: ui-monospace, monospace;
}

.ac-view-btn {
  padding: 0;
  font-size: 13px;
}

.ac-merged-badge {
  padding: 2px 10px 6px;
  font-size: 10px;
  color: #7c3aed;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 4px;
}

.ac-card-body {
  padding: 6px 10px 10px;
  border-top: 1px solid #f0f0f0;
}

.ac-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 11px;
  margin-bottom: 4px;
}

.ac-k {
  color: #94a3b8;
  text-transform: uppercase;
  min-width: 40px;
}

.ac-row code {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: #0f172a;
  word-break: break-all;
}

.ac-body-box {
  margin-top: 6px;
}

.ac-body-box pre {
  background: #0f172a;
  color: #dbeafe;
  padding: 8px;
  border-radius: 4px;
  font-size: 10px;
  line-height: 1.5;
  max-height: 200px;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
}
</style>
