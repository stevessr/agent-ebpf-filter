<script setup lang="ts">
import { computed, ref, h } from "vue";
import {
  CaretDownOutlined,
  CaretRightOutlined,
  CodeOutlined,
  FileTextOutlined,
  GlobalOutlined,
  ApartmentOutlined,
  LockOutlined,
  ClearOutlined,
  EyeOutlined,
} from "@ant-design/icons-vue";
import type { ObserverEvent, ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";

const props = withDefaults(defineProps<{
  events?: ObserverEvent[];
  tlsEvents?: ObserverTLSEvent[];
  selectedPid: number | null;
}>(), {
  events: () => [],
  tlsEvents: () => [],
});

const emit = defineEmits<{
  clear: [];
  selectPid: [pid: number];
  viewTLSEvent: [event: ObserverTLSEvent];
}>();

// Category filter
type Category = "all" | "process" | "file" | "network" | "syscall" | "ssl";
const activeCat = ref<Category>("all");
const categories: { key: Category; label: string; color: string; border: string; bg: string; icon: any }[] = [
  { key: "all", label: "All", color: "default", border: "#64748b", bg: "transparent", icon: CodeOutlined },
  { key: "process", label: "Process", color: "purple", border: "#8b5cf6", bg: "linear-gradient(90deg,#f5f3ff,#faf5ff)", icon: ApartmentOutlined },
  { key: "file", label: "File", color: "cyan", border: "#06b6d4", bg: "linear-gradient(90deg,#ecfeff,#eff6ff)", icon: FileTextOutlined },
  { key: "network", label: "Network", color: "orange", border: "#f59e0b", bg: "linear-gradient(90deg,#fff7ed,#fffbeb)", icon: GlobalOutlined },
  { key: "syscall", label: "Syscall", color: "geekblue", border: "#6366f1", bg: "linear-gradient(90deg,#eef2ff,#f0f9ff)", icon: CodeOutlined },
  { key: "ssl", label: "SSL/TLS", color: "green", border: "#10b981", bg: "linear-gradient(90deg,#ecfdf5,#f0fdfa)", icon: LockOutlined },
];

const classify = (e: ObserverEvent): Category => {
  const t = (e.type || "").toLowerCase();
  if (t.includes("exec")||t.includes("fork")||t.includes("clone")||t.includes("exit")) return "process";
  if (t.includes("open")||t.includes("read")||t.includes("write")||t.includes("mkdir")||t.includes("unlink")||t.includes("chmod")||t.includes("chown")||t.includes("rename")||t.includes("link")||t.includes("symlink")||t.includes("mknod")) return "file";
  if (t.includes("connect")||t.includes("bind")||t.includes("send")||t.includes("recv")||t.includes("socket")||t.includes("accept")||t.includes("dns")) return "network";
  if (t.includes("tls")||t.includes("ssl")||t.includes("http")) return "ssl";
  return "syscall";
};

const expanded = ref<Set<string>>(new Set());
const toggle = (key: string) => {
  const s = new Set(expanded.value);
  if (s.has(key)) s.delete(key); else s.add(key);
  expanded.value = s;
};

// Filtered events, newest first
const filteredEvents = computed(() => {
  let list = [...props.events];
  if (activeCat.value !== "all" && activeCat.value !== "ssl") {
    list = list.filter((e) => classify(e) === activeCat.value);
  }
  list.sort((a, b) => b.timestamp - a.timestamp);
  return list;
});

// Filtered TLS events for SSL category
const filteredTLSEvents = computed(() => {
  if (activeCat.value !== "all" && activeCat.value !== "ssl") return [];
  const list = [...(props.tlsEvents || [])];
  list.sort((a, b) => {
    const tsA = new Date(a.timestamp).getTime();
    const tsB = new Date(b.timestamp).getTime();
    return tsB - tsA;
  });
  return list;
});

// Combined for "all" view — interleave events and TLS events
const combinedList = computed(() => {
  if (activeCat.value === "ssl") return [];
  if (activeCat.value !== "all") return filteredEvents.value.map((e) => ({ kind: "event" as const, event: e, tls: null as null }));
  const events = filteredEvents.value.map((e) => ({ kind: "event" as const, event: e, tls: null as null }));
  const tls = filteredTLSEvents.value.slice(0, 50).map((e) => ({ kind: "tls" as const, event: null as null, tls: e }));
  const all = [...events, ...tls].sort((a, b) => {
    const tsA = a.kind === "event" ? a.event!.timestamp : new Date(a.tls!.timestamp).getTime();
    const tsB = b.kind === "event" ? b.event!.timestamp : new Date(b.tls!.timestamp).getTime();
    return tsB - tsA;
  });
  return all.slice(0, 500);
});

// Counts per category for badges
const catCounts = computed(() => {
  const c: Record<string, number> = { all: props.events.length + (props.tlsEvents?.length || 0) };
  for (const e of props.events) { const k = classify(e); c[k] = (c[k] || 0) + 1; }
  c["ssl"] = (c["ssl"] || 0) + (props.tlsEvents?.length || 0);
  return c;
});

const catInfo = (key: Category) => categories.find((c) => c.key === key)!;
const preview = (e: ObserverEvent): string => {
  const parts: string[] = [];
  if (e.comm) parts.push(e.comm);
  if (e.path) parts.push(e.path);
  if (e.extraInfo) parts.push(e.extraInfo);
  return parts.join(" → ") || e.type;
};

// TLS event helpers
const tlsPreview = (e: ObserverTLSEvent): string => {
  const parts: string[] = [];
  if (e.method) parts.push(e.method);
  if (e.host) parts.push(e.host);
  if (e.url) parts.push(e.url.length > 60 ? e.url.slice(0, 60) + "…" : e.url);
  return parts.join(" ") || e.type || "TLS Event";
};

const tlsTypeLabel = (e: ObserverTLSEvent): string => {
  switch (e.type) {
    case "http_request": return "REQ";
    case "http_response": return "RESP";
    case "sse_message": return "SSE";
    default: return e.type?.slice(0, 12) || "RAW";
  }
};

const tlsTypeColor = (e: ObserverTLSEvent): string => {
  switch (e.type) {
    case "http_request": return "blue";
    case "http_response": return "green";
    case "sse_message": return "purple";
    default: return "default";
  }
};

const formatTLSBodyPreview = (ev: ObserverTLSEvent, maxLen: number = 200): string => {
  // Use body if available
  if (ev.body) {
    const trimmed = ev.body.replace(/\s+/g, " ").trim();
    return trimmed.length > maxLen ? trimmed.slice(0, maxLen) + "…" : trimmed;
  }
  // Decode raw hex dump as text
  if (ev.raw_hex_dump) {
    try {
      const bytes: number[] = [];
      for (let i = 0; i < ev.raw_hex_dump.length - 1; i += 2) {
        const byte = parseInt(ev.raw_hex_dump.slice(i, i + 2), 16);
        if (isNaN(byte)) break;
        bytes.push(byte);
      }
      const text = bytes
        .map((b) => (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d ? String.fromCharCode(b) : ".")
        .join("");
      return text.length > maxLen ? text.slice(0, maxLen) + "…" : text;
    } catch { return ""; }
  }
  return "";
};
</script>

<template>
  <div class="timeline-root">
    <!-- Category filter tabs -->
    <div class="cat-tabs">
      <button
        v-for="cat in categories"
        :key="cat.key"
        class="cat-tab"
        :class="{ active: activeCat === cat.key }"
        @click="activeCat = cat.key"
      >
        <component :is="cat.icon" class="cat-icon" />
        {{ cat.label }}
        <span class="cat-count">{{ catCounts[cat.key] || 0 }}</span>
      </button>
    </div>

    <div class="timeline-toolbar">
      <span class="toolbar-count">
        {{ activeCat === 'ssl' ? filteredTLSEvents.length : activeCat === 'all' ? combinedList.length : filteredEvents.length }} events
      </span>
      <a-button size="small" type="link" danger @click="emit('clear')">
        <ClearOutlined /> Clear All
      </a-button>
    </div>

    <!-- SSL/TLS category: show TLS events -->
    <a-empty
      v-if="activeCat === 'ssl' && filteredTLSEvents.length === 0"
      description="No decrypted TLS events captured yet. Attach TLS probes to a process to see events."
      style="margin-top: 24px"
    />
    <div v-else-if="activeCat === 'ssl'" class="timeline-list">
      <div
        v-for="e in filteredTLSEvents"
        :key="e.key"
        class="event-block"
        style="border-left: 4px solid #10b981; background: linear-gradient(90deg,#ecfdf5,#f0fdfa)"
      >
        <div class="event-head" @click="toggle(e.key)">
          <span class="event-icon"><LockOutlined /></span>
          <a-tag :color="tlsTypeColor(e)" size="small">{{ tlsTypeLabel(e) }}</a-tag>
          <span class="event-type">{{ e.method || e.type }}</span>
          <span class="event-comm">{{ e.comm }}</span>
          <a-tag
            v-if="e.pid"
            color="processing"
            size="small"
            class="pid-tag"
            @click.stop="emit('selectPid', e.pid)"
          >PID {{ e.pid }}</a-tag>
          <span class="event-time">{{ e.timestamp?.slice(0, 19) || '' }}</span>
          <span class="expand-icon">
            <CaretDownOutlined v-if="expanded.has(e.key)" />
            <CaretRightOutlined v-else />
          </span>
        </div>

        <div class="event-preview" v-if="!expanded.has(e.key)">{{ tlsPreview(e) }}</div>

        <div v-if="expanded.has(e.key)" class="event-detail">
          <div class="detail-row" v-if="e.host"><span class="dl">Host</span><code>{{ e.host }}</code></div>
          <div class="detail-row" v-if="e.url"><span class="dl">URL</span><code>{{ e.url }}</code></div>
          <div class="detail-row" v-if="e.method"><span class="dl">Method</span><code>{{ e.method }}</code></div>
          <div class="detail-row" v-if="e.status"><span class="dl">Status</span><span>{{ e.status }}</span></div>
          <div class="detail-row"><span class="dl">Dir</span><span>{{ e.direction }}</span></div>
          <div class="detail-row"><span class="dl">Lib</span><code>{{ e.lib }}</code></div>
          <div class="detail-row" v-if="e.body || e.raw_hex_dump">
            <span class="dl">Body</span>
            <div class="tls-timeline-body">
              <pre>{{ formatTLSBodyPreview(e, 200) }}</pre>
            </div>
          </div>
          <div class="detail-row" v-if="e.redaction_state">
            <span class="dl">Redaction</span>
            <a-tag :color="e.redaction_state === 'sanitized' ? 'green' : 'orange'" size="small">{{ e.redaction_state }}</a-tag>
          </div>
          <a-button size="small" type="link" style="padding:0;margin-top:4px" @click.stop="emit('viewTLSEvent', e)">
            <EyeOutlined /> View Full Detail
          </a-button>
        </div>
      </div>
    </div>

    <!-- Non-SSL: show observer events (or combined in "all") -->
    <template v-else>
      <a-empty
        v-if="activeCat === 'all' ? combinedList.length === 0 : filteredEvents.length === 0"
        description="No events in this category"
        style="margin-top: 24px"
      />

      <div v-else class="timeline-list">
        <template v-if="activeCat === 'all'">
          <!-- Combined: show both ObserverEvent and TLS events interleaved -->
          <template v-for="item in combinedList" :key="item.kind === 'event' ? item.event!.key : item.tls!.key">
            <!-- TLS event -->
            <div
              v-if="item.kind === 'tls' && item.tls"
              class="event-block"
              style="border-left: 4px solid #10b981; background: linear-gradient(90deg,#ecfdf5,#f0fdfa)"
            >
              <div class="event-head" @click="toggle(item.tls.key)">
                <span class="event-icon"><LockOutlined /></span>
                <a-tag :color="tlsTypeColor(item.tls)" size="small">{{ tlsTypeLabel(item.tls) }}</a-tag>
                <span class="event-type">{{ item.tls.method || item.tls.type }}</span>
                <span class="event-comm">{{ item.tls.comm }}</span>
                <a-tag v-if="item.tls.pid" color="processing" size="small" class="pid-tag" @click.stop="emit('selectPid', item.tls.pid)">PID {{ item.tls.pid }}</a-tag>
                <span class="event-time">{{ item.tls.timestamp?.slice(0, 19) || '' }}</span>
                <span class="expand-icon">
                  <CaretDownOutlined v-if="expanded.has(item.tls.key)" />
                  <CaretRightOutlined v-else />
                </span>
              </div>
              <div class="event-preview" v-if="!expanded.has(item.tls.key)">{{ tlsPreview(item.tls) }}</div>
              <div v-if="expanded.has(item.tls.key)" class="event-detail">
                <div class="detail-row" v-if="item.tls.host"><span class="dl">Host</span><code>{{ item.tls.host }}</code></div>
                <div class="detail-row" v-if="item.tls.url"><span class="dl">URL</span><code>{{ item.tls.url }}</code></div>
                <div class="detail-row" v-if="item.tls.body || item.tls.raw_hex_dump"><span class="dl">Body</span><div class="tls-timeline-body"><pre>{{ formatTLSBodyPreview(item.tls, 150) }}</pre></div></div>
                <a-button size="small" type="link" style="padding:0;margin-top:4px" @click.stop="emit('viewTLSEvent', item.tls)"><EyeOutlined /> View Full Detail</a-button>
              </div>
            </div>

            <!-- Observer event -->
            <div
              v-else-if="item.kind === 'event' && item.event"
              class="event-block"
              :style="{
                borderLeft: `4px solid ${catInfo(classify(item.event)).border}`,
                background: catInfo(classify(item.event)).bg,
              }"
            >
              <div class="event-head" @click="toggle(item.event.key)">
                <span class="event-icon"><component :is="catInfo(classify(item.event)).icon" /></span>
                <a-tag :color="catInfo(classify(item.event)).color" size="small">{{ catInfo(classify(item.event)).label }}</a-tag>
                <span class="event-type">{{ item.event.type.toUpperCase() }}</span>
                <span class="event-comm">{{ item.event.comm }}</span>
                <a-tag v-if="item.event.pid" color="processing" size="small" class="pid-tag" @click.stop="emit('selectPid', item.event.pid)">PID {{ item.event.pid }}</a-tag>
                <span class="event-time">{{ item.event.time }}</span>
                <span class="expand-icon">
                  <CaretDownOutlined v-if="expanded.has(item.event.key)" />
                  <CaretRightOutlined v-else />
                </span>
              </div>
              <div class="event-preview" v-if="!expanded.has(item.event.key)">{{ preview(item.event) }}</div>
              <div v-if="expanded.has(item.event.key)" class="event-detail">
                <div class="detail-row" v-if="item.event.path"><span class="dl">Path</span><code>{{ item.event.path }}</code></div>
                <div class="detail-row" v-if="item.event.extraInfo"><span class="dl">Info</span><code>{{ item.event.extraInfo }}</code></div>
                <div class="detail-row" v-if="item.event.bytes"><span class="dl">Bytes</span><span>{{ item.event.bytes.toLocaleString() }}</span></div>
                <div class="detail-row" v-if="item.event.retval"><span class="dl">Retval</span><code>{{ item.event.retval }}</code></div>
                <div class="detail-row"><span class="dl">PPID</span><span>{{ item.event.ppid }}</span></div>
              </div>
            </div>
          </template>
        </template>

        <!-- Non-all, non-ssl: only ObserverEvent entries -->
        <template v-else>
          <div
            v-for="e in filteredEvents"
            :key="e.key"
            class="event-block"
            :style="{
              borderLeft: `4px solid ${catInfo(classify(e)).border}`,
              background: catInfo(classify(e)).bg,
            }"
          >
            <div class="event-head" @click="toggle(e.key)">
              <span class="event-icon"><component :is="catInfo(classify(e)).icon" /></span>
              <a-tag :color="catInfo(classify(e)).color" size="small">{{ catInfo(classify(e)).label }}</a-tag>
              <span class="event-type">{{ e.type.toUpperCase() }}</span>
              <span class="event-comm">{{ e.comm }}</span>
              <a-tag v-if="e.pid" color="processing" size="small" class="pid-tag" @click.stop="emit('selectPid', e.pid)">PID {{ e.pid }}</a-tag>
              <span class="event-time">{{ e.time }}</span>
              <span class="expand-icon">
                <CaretDownOutlined v-if="expanded.has(e.key)" />
                <CaretRightOutlined v-else />
              </span>
            </div>
            <div class="event-preview" v-if="!expanded.has(e.key)">{{ preview(e) }}</div>
            <div v-if="expanded.has(e.key)" class="event-detail">
              <div class="detail-row" v-if="e.path"><span class="dl">Path</span><code>{{ e.path }}</code></div>
              <div class="detail-row" v-if="e.extraInfo"><span class="dl">Info</span><code>{{ e.extraInfo }}</code></div>
              <div class="detail-row" v-if="e.bytes"><span class="dl">Bytes</span><span>{{ e.bytes.toLocaleString() }}</span></div>
              <div class="detail-row" v-if="e.retval"><span class="dl">Retval</span><code>{{ e.retval }}</code></div>
              <div class="detail-row"><span class="dl">PPID</span><span>{{ e.ppid }}</span></div>
            </div>
          </div>
        </template>
      </div>
    </template>
  </div>
</template>

<style scoped>
.timeline-root { display: flex; flex-direction: column; }

.cat-tabs {
  display: flex; gap: 2px; margin-bottom: 8px;
  background: #f5f5f5; border-radius: 6px; padding: 2px;
  overflow-x: auto;
}
.cat-tab {
  display: flex; align-items: center; gap: 3px;
  padding: 3px 10px; border: none; border-radius: 5px;
  background: transparent; cursor: pointer;
  font-size: 12px; color: #64748b; white-space: nowrap;
  transition: all 0.15s;
}
.cat-tab:hover { color: #334155; background: #e2e8f0; }
.cat-tab.active { background: #fff; color: #0f172a; font-weight: 600; box-shadow: 0 1px 2px rgba(0,0,0,.08); }
.cat-icon { font-size: 13px; }
.cat-count {
  background: #e2e8f0; color: #64748b; border-radius: 8px;
  padding: 0 5px; font-size: 10px; font-weight: 600; min-width: 16px; text-align: center;
}
.cat-tab.active .cat-count { background: #dbeafe; color: #1d4ed8; }

.timeline-toolbar {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 6px; padding-bottom: 4px;
  border-bottom: 1px solid #f0f0f0;
}
.toolbar-count { font-size: 12px; color: #888; }

.timeline-list { display: flex; flex-direction: column; gap: 6px; max-height: 520px; overflow-y: auto; }

.event-block {
  border-radius: 8px; padding: 6px 10px;
  box-shadow: 0 1px 3px rgba(15,23,42,.06);
  transition: box-shadow .15s;
}
.event-block:hover { box-shadow: 0 2px 6px rgba(15,23,42,.12); }

.event-head {
  display: flex; align-items: center; gap: 6px; cursor: pointer;
  user-select: none;
}
.event-icon { color: #64748b; font-size: 13px; display: flex; }
.event-type {
  font-family: ui-monospace,monospace; font-size: 11px;
  color: #475569; font-weight: 600; min-width: 70px;
}
.event-comm {
  font-family: ui-monospace,monospace; font-size: 12px;
  color: #1e293b; font-weight: 500; flex: 1;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
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

/* TLS timeline body */
.tls-timeline-body {
  flex: 1;
  min-width: 0;
}
.tls-timeline-body pre {
  background: #0f172a;
  color: #dbeafe;
  padding: 6px 10px;
  border-radius: 4px;
  font-size: 11px;
  line-height: 1.45;
  max-height: 120px;
  overflow: auto;
  margin: 2px 0 0;
  white-space: pre-wrap;
}
</style>
