<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  MergeCellsOutlined,
  EyeOutlined,
  CaretDownOutlined,
  CaretRightOutlined,
} from "@ant-design/icons-vue";
import type { ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";
import {
  buildGroups,
  blockColor,
  blockDisplayText,
  blockIcon,
  blockLabel,
  blockTokens,
  firstTypeLabel,
  formatBytes,
  formatTime,
  formatTimeRange,
  formatTokens,
  usageEntries,
} from "./agentContextParsing";
import type { ContentBlock, MergedGroup } from "./agentContextParsing";
const props = defineProps<{
  events: ObserverTLSEvent[];
}>();

const emit = defineEmits<{
  viewEvent: [event: ObserverTLSEvent];
}>();

const streamGroups = computed(() => {
  const sorted = [...props.events].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );
  // Case-insensitive direction match (backend uses lowercase "send"/"recv")
  const isSend = (e: ObserverTLSEvent) => e.direction?.toLowerCase() === "send";
  return {
    send: buildGroups(sorted.filter(isSend), "send"),
    recv: buildGroups(
      sorted.filter((e) => !isSend(e)),
      "recv",
    ),
  };
});

function readStoredCap(key: string, fallback: number): number {
  try {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    const n = parseInt(v, 10);
    return Number.isFinite(n) && n >= 0 ? n : fallback;
  } catch {
    return fallback;
  }
}

// ── Raw visibility mode (persisted) ───────────────────────────────────────
// "show": show everything
// "skip": skip groups that are ALL raw, hide raw blocks within context groups
// "hide": show all groups, hide raw/data blocks within each
const RAW_MODE_KEY = "observe-raw-mode";
type RawMode = "show" | "skip" | "hide";
const rawModeOptions = [
  { value: "show" as RawMode, label: "Show all" },
  { value: "skip" as RawMode, label: "Skip raw" },
  { value: "hide" as RawMode, label: "Hide raw" },
];
function readStoredRawMode(key: string, fallback: RawMode): RawMode {
  try {
    const v = localStorage.getItem(key);
    if (v === "show" || v === "skip" || v === "hide") return v;
    return fallback;
  } catch {
    return fallback;
  }
}
const rawMode = ref<RawMode>(readStoredRawMode(RAW_MODE_KEY, "skip"));
const CONTEXT_BLOCK_TYPES = new Set([
  "text",
  "thinking",
  "tool_use",
  "tool_result",
  "request_body",
  "response_body",
  "signature",
  "citations",
]);

// Persist rawMode
watch(rawMode, (v) => {
  try {
    localStorage.setItem(RAW_MODE_KEY, v);
  } catch {
    /* ignore */
  }
});

// ── Scrollback cap (synced with composable via localStorage) ──────────────
const TLS_CAP_KEY = "observe-tls-cap";
const capPresets = [1000, 5000, 10000, 50000, 0];
const capOptions = capPresets.map((n) => ({
  value: n,
  label: n === 0 ? "∞" : n >= 1000 ? `${(n / 1000).toFixed(0)}k` : String(n),
}));

const tlsCap = ref(readStoredCap(TLS_CAP_KEY, 50000));
const onCapChange = (v: number) => {
  tlsCap.value = v;
  try {
    localStorage.setItem(TLS_CAP_KEY, String(v));
  } catch {
    /* ignore */
  }
};

// Filter groups based on rawMode
// "show" — all groups visible
// "skip" / "hide" — all groups visible; raw blocks hidden per-block via blockVisible.
//   Difference is at buffer level: skip prunes raw events after each conversation
//   wave, hide caches them indefinitely. Display is identical.
const filteredSendGroups = computed(() => streamGroups.value.send);
const filteredRecvGroups = computed(() => streamGroups.value.recv);

// ── Stats & state ────────────────────────────────────────────────────────
const stats = computed(() => {
  const byDir = (dir: string) =>
    props.events.filter((e) => e.direction?.toLowerCase() === dir);
  const sendRaw = byDir("send");
  const recvRaw = byDir("recv");
  const countByType = (evs: ObserverTLSEvent[]) => {
    const m: Record<string, number> = {};
    for (const e of evs) {
      const t = e.type || "?";
      m[t] = (m[t] || 0) + 1;
    }
    return m;
  };
  return {
    sendCount: streamGroups.value.send.length,
    recvCount: streamGroups.value.recv.length,
    sendBytes: streamGroups.value.send.reduce((s, g) => s + g.totalSize, 0),
    recvBytes: streamGroups.value.recv.reduce((s, g) => s + g.totalSize, 0),
    // Debug: raw event counts
    sendRawN: sendRaw.length,
    recvRawN: recvRaw.length,
    sendTypes: countByType(sendRaw),
    recvTypes: countByType(recvRaw),
    sendWithBody: sendRaw.filter((e) => e.body || e.raw_hex_dump).length,
    sendBodySizes: sendRaw.map(
      (e) => e.body_size || e.body?.length || e.raw_hex_dump?.length || 0,
    ),
  };
});

const expanded = ref<Set<string>>(new Set());
const blockExpanded = ref<Set<string>>(new Set());
const toggle = (id: string) => {
  const s = new Set(expanded.value);
  if (s.has(id)) s.delete(id);
  else s.add(id);
  expanded.value = s;
};
const toggleBlock = (blockId: string) => {
  const s = new Set(blockExpanded.value);
  if (s.has(blockId)) s.delete(blockId);
  else s.add(blockId);
  blockExpanded.value = s;
};
const blockId = (gid: string, bi: number) => `${gid}-b${bi}`;

// Determine if a block should be visible under current rawMode
const blockVisible = (b: ContentBlock): boolean =>
  rawMode.value === "show" || CONTEXT_BLOCK_TYPES.has(b.type);

// Only "show" mode reveals raw fallback bodies — "hide" and "skip" suppress them
const showRawFallback = (g: MergedGroup): boolean =>
  rawMode.value === "show" && !!g.rawMerged;

// Token display helpers per block — show input/output at block level
</script>
<template>
  <div class="ac-root">
    <div class="ac-stats">
      <div class="ac-stat-item send">
        <ArrowUpOutlined /><span class="ac-stat-label">Upstream</span
        ><span class="ac-stat-val">{{ stats.sendCount }} groups</span
        ><span class="ac-stat-size">{{ formatBytes(stats.sendBytes) }}</span
        ><a-tooltip placement="bottom"
          ><template #title
            ><span style="font-family: monospace; font-size: 11px"
              >Raw events: {{ stats.sendRawN }} (body:
              {{ stats.sendWithBody }})<br />{{
                JSON.stringify(stats.sendTypes)
              }}</span
            ></template
          ><span
            class="ac-diag-dot"
            :class="stats.sendRawN > 0 ? 'ac-diag-ok' : 'ac-diag-warn'"
            >●</span
          ></a-tooltip
        >
      </div>
      <div class="ac-stat-item recv">
        <ArrowDownOutlined /><span class="ac-stat-label">Downstream</span
        ><span class="ac-stat-val">{{ stats.recvCount }} groups</span
        ><span class="ac-stat-size">{{ formatBytes(stats.recvBytes) }}</span
        ><a-tooltip placement="bottom"
          ><template #title
            ><span style="font-family: monospace; font-size: 11px"
              >Raw events: {{ stats.recvRawN }}<br />{{
                JSON.stringify(stats.recvTypes)
              }}</span
            ></template
          ><span
            class="ac-diag-dot"
            :class="stats.recvRawN > 0 ? 'ac-diag-ok' : 'ac-diag-warn'"
            >●</span
          ></a-tooltip
        >
      </div>
      <div class="ac-stat-item ac-hide-raw-toggle">
        <a-select
          v-model:value="rawMode"
          size="small"
          style="width: 100px"
          :options="rawModeOptions"
        />
      </div>
      <div class="ac-stat-item ac-cap-ctl">
        <span class="ac-cap-label">Scrollback</span>
        <a-select
          v-model:value="tlsCap"
          size="small"
          style="width: 90px"
          :options="capOptions"
          @change="onCapChange"
        />
      </div>
    </div>

    <div class="ac-columns">
      <!-- UPSTREAM -->
      <div class="ac-col ac-send-col">
        <div class="ac-col-header">
          <ArrowUpOutlined style="color: #f59e0b" /><span
            >Upstream (Agent → Server)</span
          >
        </div>
        <a-empty
          v-if="filteredSendGroups.length === 0"
          description="No upstream data"
          style="padding: 24px"
        />
        <div v-else class="ac-list">
          <div
            v-for="g in filteredSendGroups"
            :key="g.id"
            class="ac-card"
            :class="{
              'ac-sse':
                g.events[0]?.type === 'sse_message' ||
                g.contentBlocks.length > 1 ||
                g.contentBlocks.some(
                  (b) => b.type === 'thinking' || b.type === 'tool_use',
                ),
            }"
          >
            <!-- head -->
            <div
              class="ac-head"
              role="button"
              tabindex="0"
              :aria-expanded="expanded.has(g.id)"
              :aria-label="`${expanded.has(g.id) ? 'Collapse' : 'Expand'} upstream context group`"
              @click="toggle(g.id)"
              @keydown.enter.self.prevent="toggle(g.id)"
              @keydown.space.self.prevent="toggle(g.id)"
            >
              <span class="ac-h-icon"
                ><CaretDownOutlined
                  v-if="expanded.has(g.id)" /><CaretRightOutlined v-else
              /></span>
              <a-tag
                :color="blockColor(g.contentBlocks[0]?.type || 'raw')"
                size="small"
                >{{ firstTypeLabel(g) }}</a-tag
              >
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{
                  g.messageRole
                }}</span>
                <span v-if="g.messageModel" class="ac-model">{{
                  g.messageModel
                }}</span>
                <span
                  v-if="usageEntries(g.usage || {}).length"
                  class="ac-tok-inline"
                  >{{
                    usageEntries(g.usage!)
                      .reduce((s, e) => s + e.value, 0)
                      .toLocaleString()
                  }}
                  tok</span
                >
                <span v-if="g.contentBlocks.length > 1" class="ac-cbc"
                  >{{ g.contentBlocks.length }} blocks</span
                >
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{
                g.events[0].host
              }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{
                formatTimeRange(g.startTime, g.endTime)
              }}</span>
              <a-button
                size="small"
                type="link"
                class="ac-view"
                @click.stop="emit('viewEvent', g.events[0])"
                ><EyeOutlined
              /></a-button>
            </div>
            <div
              v-if="
                g.events.length > 1 &&
                (g.events[0]?.type === 'sse_message' ||
                  g.contentBlocks.length > 0)
              "
              class="ac-merged"
            >
              <MergeCellsOutlined /> {{ g.events.length }} SSE →
              {{ g.contentBlocks.length }} block{{
                g.contentBlocks.length !== 1 ? "s" : ""
              }}
            </div>
            <!-- body -->
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta">
                <span class="ac-k">ID</span><code>{{ g.messageId }}</code>
              </div>
              <div
                v-if="g.usage && usageEntries(g.usage).length"
                class="ac-tokens"
              >
                <span
                  v-for="e in usageEntries(g.usage)"
                  :key="e.key"
                  class="ac-tok"
                  :class="{
                    'ac-tok-cache': e.key.includes('cache'),
                    'ac-tok-out': e.key.includes('output'),
                  }"
                >
                  <span class="ac-tok-label">{{ e.label }}</span>
                  <span class="ac-tok-val">{{ e.value.toLocaleString() }}</span>
                </span>
              </div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <template v-for="(b, bi) in g.contentBlocks" :key="bi">
                  <div
                    v-if="blockVisible(b)"
                    class="ac-block"
                    :class="`ac-b-${b.type}`"
                  >
                    <div
                      class="ac-b-head"
                      role="button"
                      tabindex="0"
                      :aria-expanded="blockExpanded.has(blockId(g.id, bi))"
                      :aria-label="`${blockExpanded.has(blockId(g.id, bi)) ? 'Collapse' : 'Expand'} upstream content block`"
                      style="cursor: pointer"
                      @click.stop="toggleBlock(blockId(g.id, bi))"
                      @keydown.enter.stop.prevent="
                        toggleBlock(blockId(g.id, bi))
                      "
                      @keydown.space.stop.prevent="
                        toggleBlock(blockId(g.id, bi))
                      "
                    >
                      <span class="ac-expand-icon"
                        ><CaretDownOutlined
                          v-if="
                            blockExpanded.has(blockId(g.id, bi))
                          " /><CaretRightOutlined v-else
                      /></span>
                      <component
                        :is="blockIcon(b.type)"
                        class="ac-b-icon"
                      /><a-tag :color="blockColor(b.type)" size="small">{{
                        blockLabel(b.type)
                      }}</a-tag
                      ><span v-if="b.toolName" class="ac-tn">{{
                        b.toolName
                      }}</span
                      ><span v-if="b.toolId" class="ac-tid">{{
                        b.toolId
                      }}</span>
                      <span
                        v-if="blockTokens(g).input"
                        class="ac-b-tok ac-b-tok-in"
                        title="Input tokens"
                        >{{
                          blockTokens(g).input.toLocaleString()
                        }}&thinsp;in</span
                      >
                      <span
                        v-if="blockTokens(g).output"
                        class="ac-b-tok ac-b-tok-out"
                        title="Output tokens"
                        >{{
                          blockTokens(g).output.toLocaleString()
                        }}&thinsp;out</span
                      >
                      <span class="ac-bsz">{{
                        formatBytes(b.mergedText.length)
                      }}</span>
                    </div>
                    <div
                      v-if="blockExpanded.has(blockId(g.id, bi))"
                      class="ac-b-body"
                    >
                      <pre>{{ blockDisplayText(b) }}</pre>
                    </div>
                  </div>
                </template>
              </div>
              <div
                v-else-if="g.rawMerged && showRawFallback(g)"
                class="ac-b-body"
              >
                <pre>{{ g.rawMerged }}</pre>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- DOWNSTREAM -->
      <div class="ac-col ac-recv-col">
        <div class="ac-col-header">
          <ArrowDownOutlined style="color: #06b6d4" /><span
            >Downstream (Server → Agent)</span
          >
        </div>
        <a-empty
          v-if="filteredRecvGroups.length === 0"
          description="No downstream data"
          style="padding: 24px"
        />
        <div v-else class="ac-list">
          <div
            v-for="g in filteredRecvGroups"
            :key="g.id"
            class="ac-card"
            :class="{
              'ac-sse':
                g.events[0]?.type === 'sse_message' ||
                g.contentBlocks.length > 1 ||
                g.contentBlocks.some(
                  (b) => b.type === 'thinking' || b.type === 'tool_use',
                ),
            }"
          >
            <div
              class="ac-head"
              role="button"
              tabindex="0"
              :aria-expanded="expanded.has(g.id)"
              :aria-label="`${expanded.has(g.id) ? 'Collapse' : 'Expand'} downstream context group`"
              @click="toggle(g.id)"
              @keydown.enter.self.prevent="toggle(g.id)"
              @keydown.space.self.prevent="toggle(g.id)"
            >
              <span class="ac-h-icon"
                ><CaretDownOutlined
                  v-if="expanded.has(g.id)" /><CaretRightOutlined v-else
              /></span>
              <a-tag
                :color="blockColor(g.contentBlocks[0]?.type || 'raw')"
                size="small"
                >{{ firstTypeLabel(g) }}</a-tag
              >
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{
                  g.messageRole
                }}</span>
                <span v-if="g.messageModel" class="ac-model">{{
                  g.messageModel
                }}</span>
                <span
                  v-if="usageEntries(g.usage || {}).length"
                  class="ac-tok-inline"
                  >{{
                    usageEntries(g.usage!)
                      .reduce((s, e) => s + e.value, 0)
                      .toLocaleString()
                  }}
                  tok</span
                >
                <span v-if="g.contentBlocks.length > 1" class="ac-cbc"
                  >{{ g.contentBlocks.length }} blocks</span
                >
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{
                g.events[0].host
              }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{
                formatTimeRange(g.startTime, g.endTime)
              }}</span>
              <a-button
                size="small"
                type="link"
                class="ac-view"
                @click.stop="emit('viewEvent', g.events[0])"
                ><EyeOutlined
              /></a-button>
            </div>
            <div
              v-if="
                g.events.length > 1 &&
                (g.events[0]?.type === 'sse_message' ||
                  g.contentBlocks.length > 0)
              "
              class="ac-merged"
            >
              <MergeCellsOutlined /> {{ g.events.length }} SSE →
              {{ g.contentBlocks.length }} block{{
                g.contentBlocks.length !== 1 ? "s" : ""
              }}
            </div>
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta">
                <span class="ac-k">ID</span><code>{{ g.messageId }}</code>
              </div>
              <div
                v-if="g.usage && usageEntries(g.usage).length"
                class="ac-tokens"
              >
                <span
                  v-for="e in usageEntries(g.usage)"
                  :key="e.key"
                  class="ac-tok"
                  :class="{
                    'ac-tok-cache': e.key.includes('cache'),
                    'ac-tok-out': e.key.includes('output'),
                  }"
                >
                  <span class="ac-tok-label">{{ e.label }}</span>
                  <span class="ac-tok-val">{{ e.value.toLocaleString() }}</span>
                </span>
              </div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <template v-for="(b, bi) in g.contentBlocks" :key="bi">
                  <div
                    v-if="blockVisible(b)"
                    class="ac-block"
                    :class="`ac-b-${b.type}`"
                  >
                    <div
                      class="ac-b-head"
                      role="button"
                      tabindex="0"
                      :aria-expanded="blockExpanded.has(blockId(g.id, bi))"
                      :aria-label="`${blockExpanded.has(blockId(g.id, bi)) ? 'Collapse' : 'Expand'} downstream content block`"
                      style="cursor: pointer"
                      @click.stop="toggleBlock(blockId(g.id, bi))"
                      @keydown.enter.stop.prevent="
                        toggleBlock(blockId(g.id, bi))
                      "
                      @keydown.space.stop.prevent="
                        toggleBlock(blockId(g.id, bi))
                      "
                    >
                      <span class="ac-expand-icon"
                        ><CaretDownOutlined
                          v-if="
                            blockExpanded.has(blockId(g.id, bi))
                          " /><CaretRightOutlined v-else
                      /></span>
                      <component
                        :is="blockIcon(b.type)"
                        class="ac-b-icon"
                      /><a-tag :color="blockColor(b.type)" size="small">{{
                        blockLabel(b.type)
                      }}</a-tag
                      ><span v-if="b.toolName" class="ac-tn">{{
                        b.toolName
                      }}</span
                      ><span v-if="b.toolId" class="ac-tid">{{ b.toolId }}</span
                      ><span class="ac-bsz">{{
                        formatBytes(b.mergedText.length)
                      }}</span>
                    </div>
                    <div
                      v-if="blockExpanded.has(blockId(g.id, bi))"
                      class="ac-b-body"
                    >
                      <pre>{{ blockDisplayText(b) }}</pre>
                    </div>
                  </div>
                </template>
              </div>
              <div
                v-else-if="g.rawMerged && showRawFallback(g)"
                class="ac-b-body"
              >
                <pre>{{ g.rawMerged }}</pre>
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
.ac-stat-item.send {
  color: #d97706;
}
.ac-stat-item.recv {
  color: #059669;
}
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
.ac-hide-raw-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  padding-left: 12px;
  border-left: 1px solid #e2e8f0;
}
.ac-cap-ctl {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
}
.ac-cap-label {
  font-size: 10px;
  color: #64748b;
  font-weight: 500;
  white-space: nowrap;
}
.ac-diag-dot {
  font-size: 8px;
  cursor: help;
  margin-left: 2px;
}
.ac-diag-ok {
  color: #22c55e;
}
.ac-diag-warn {
  color: #f97316;
}
.ac-columns {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.ac-col {
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fafafa;
}
.ac-send-col {
  border-left: 3px solid #f59e0b;
}
.ac-recv-col {
  border-left: 3px solid #06b6d4;
}
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
.ac-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 6px;
}
.ac-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  cursor: pointer;
  transition: box-shadow 0.15s;
}
.ac-card:hover {
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}
.ac-card.ac-sse {
  border-color: #c084fc;
  background: #faf5ff;
}
.ac-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
}
.ac-h-icon {
  font-size: 9px;
  color: #64748b;
}
.ac-h-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  overflow: hidden;
}
.ac-role {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: #7c3aed;
  font-weight: 600;
}
.ac-model {
  font-size: 10px;
  color: #64748b;
  font-family: ui-monospace, monospace;
}
.ac-tok-inline {
  font-size: 10px;
  color: #0891b2;
  font-family: ui-monospace, monospace;
  font-weight: 600;
  background: #ecfeff;
  padding: 0 4px;
  border-radius: 3px;
  border: 1px solid #a5f3fc;
}
.ac-cbc {
  font-size: 10px;
  color: #64748b;
}
.ac-h-host {
  font-size: 10px;
  color: #64748b;
  font-family: ui-monospace, monospace;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ac-h-size {
  font-size: 10px;
  color: #64748b;
  font-family: ui-monospace, monospace;
}
.ac-h-time {
  font-size: 10px;
  color: #64748b;
  font-family: ui-monospace, monospace;
}
.ac-view {
  padding: 0;
  font-size: 13px;
}
.ac-merged {
  padding: 2px 10px 6px;
  font-size: 10px;
  color: #7c3aed;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 4px;
}
.ac-body {
  padding: 6px 10px 10px;
  border-top: 1px solid #f0f0f0;
}
.ac-meta {
  display: flex;
  gap: 6px;
  font-size: 11px;
  margin-bottom: 6px;
  align-items: baseline;
}
.ac-k {
  color: #64748b;
  text-transform: uppercase;
  min-width: 30px;
  font-size: 10px;
}
.ac-meta code {
  font-family: ui-monospace, monospace;
  font-size: 10px;
  color: #0f172a;
  word-break: break-all;
}
.ac-tokens {
  display: flex;
  gap: 10px;
  padding: 4px 8px;
  background: #f0fdfa;
  border-radius: 4px;
  border: 1px solid #ccfbf1;
  margin-bottom: 6px;
  flex-wrap: wrap;
}
.ac-tok {
  display: flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
}
.ac-tok-label {
  color: #64748b;
  font-size: 10px;
}
.ac-tok-val {
  font-family: ui-monospace, monospace;
  font-weight: 700;
  color: #0f766e;
  font-size: 12px;
}
.ac-tok-cache .ac-tok-val {
  color: #7c3aed;
}
.ac-blocks {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.ac-block {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  overflow: hidden;
}
.ac-b-text {
  border-color: #86efac;
}
.ac-b-tool_use {
  border-color: #fdba74;
}
.ac-b-thinking {
  border-color: #c4b5fd;
}
.ac-b-request_body {
  border-color: #93c5fd;
}
.ac-b-response_body {
  border-color: #67e8f9;
}
.ac-b-head {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 4px 8px;
  background: #f8fafc;
  border-bottom: 1px solid #f0f0f0;
  font-size: 11px;
}
.ac-b-icon {
  font-size: 12px;
  color: #64748b;
}
.ac-tn {
  font-family: ui-monospace, monospace;
  font-weight: 600;
  color: #d97706;
  font-size: 11px;
}
.ac-tid {
  font-family: ui-monospace, monospace;
  font-size: 9px;
  color: #64748b;
}
.ac-bsz {
  margin-left: auto;
  font-size: 9px;
  color: #64748b;
}
.ac-b-tok {
  font-size: 9px;
  font-family: ui-monospace, monospace;
  font-weight: 600;
  padding: 0 3px;
  border-radius: 2px;
  white-space: nowrap;
}
.ac-b-tok-in {
  color: #0f766e;
  background: #f0fdfa;
  border: 1px solid #ccfbf1;
}
.ac-b-tok-out {
  color: #0891b2;
  background: #ecfeff;
  border: 1px solid #a5f3fc;
}
.ac-b-body pre {
  background: #0f172a;
  color: #dbeafe;
  padding: 10px;
  border-radius: 0;
  font-size: 11px;
  line-height: 1.55;
  margin: 0;
  white-space: pre-wrap;
}
</style>
