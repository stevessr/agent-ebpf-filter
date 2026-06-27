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

const props = withDefaults(defineProps<{
  events?: ObserverEvent[];
  selectedPid: number | null;
}>(), {
  events: () => [],
});

const emit = defineEmits<{
  clear: [];
  selectPid: [pid: number];
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

// Filtered, newest first
const filtered = computed(() => {
  let list = [...props.events];
  if (activeCat.value !== "all") list = list.filter((e) => classify(e) === activeCat.value);
  list.sort((a, b) => b.timestamp - a.timestamp);
  return list;
});

// Counts per category for badges
const catCounts = computed(() => {
  const c: Record<string, number> = { all: props.events.length };
  for (const e of props.events) { const k = classify(e); c[k] = (c[k] || 0) + 1; }
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
      <span class="toolbar-count">{{ filtered.length }} events</span>
      <a-button size="small" type="link" danger @click="emit('clear')">
        <ClearOutlined /> Clear All
      </a-button>
    </div>

    <a-empty
      v-if="filtered.length === 0"
      description="No events in this category"
      style="margin-top: 24px"
    />

    <div v-else class="timeline-list">
      <div
        v-for="e in filtered"
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
          <a-tag
            v-if="e.pid"
            color="processing"
            size="small"
            class="pid-tag"
            @click.stop="emit('selectPid', e.pid)"
          >PID {{ e.pid }}</a-tag>
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
    </div>
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
</style>
