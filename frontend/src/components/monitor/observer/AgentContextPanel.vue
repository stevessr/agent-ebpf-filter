<script setup lang="ts">
import { computed, ref } from "vue";
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  MergeCellsOutlined,
  EyeOutlined,
  CaretDownOutlined,
  CaretRightOutlined,
  CodeOutlined,
  ToolOutlined,
  MessageOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons-vue";
import type { ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";

const props = defineProps<{
  events: ObserverTLSEvent[];
}>();

const emit = defineEmits<{
  viewEvent: [event: ObserverTLSEvent];
}>();

// ── Hex decoder ──────────────────────────────────────────────────────────
const hexToText = (hex: string, maxLen: number = 2000): string => {
  try {
    const bytes: number[] = [];
    for (let i = 0; i < hex.length - 1; i += 2) {
      const byte = parseInt(hex.slice(i, i + 2), 16);
      if (isNaN(byte)) break;
      bytes.push(byte);
    }
    return bytes
      .map((b) => (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d ? String.fromCharCode(b) : ".")
      .join("").slice(0, maxLen);
  } catch { return ""; }
};

const getRawText = (ev: ObserverTLSEvent): string =>
  ev.body || (ev.raw_hex_dump ? hexToText(ev.raw_hex_dump, 2000) : "");

const tryParseJSON = (text: string): any | null => {
  try { return JSON.parse(text); } catch { return null; }
};

const formatJSON = (obj: any, maxLen: number = 3000): string => {
  try {
    const s = JSON.stringify(obj, null, 2);
    return s.length > maxLen ? s.slice(0, maxLen) + "\n… [truncated]" : s;
  } catch { return String(obj); }
};

// ── SSE parser (handles both framed SSE and bare concatenated JSON) ──────
interface SSEParsed { type: string; index?: number; data: any; delta?: any; content_block?: any; message?: any; }
const NOOP_TYPES = new Set(["ping", "message_stop"]);

// Split concatenated JSON objects: {"a":1}{"b":2} → [{"a":1}, {"b":2}]
const splitConcatenatedJSON = (text: string): any[] => {
  const results: any[] = [];
  // Find all top-level { } objects
  let depth = 0, start = -1;
  let inString = false, escaped = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (escaped) { escaped = false; continue; }
    if (ch === '\\') { escaped = true; continue; }
    if (ch === '"') { inString = !inString; continue; }
    if (inString) continue;
    if (ch === '{') {
      if (depth === 0) start = i;
      depth++;
    } else if (ch === '}') {
      depth--;
      if (depth === 0 && start >= 0) {
        const obj = tryParseJSON(text.slice(start, i + 1));
        if (obj) results.push(obj);
        start = -1;
      }
    }
  }
  return results;
};

// Parse raw text from an event into SSE data objects
// Handles: SSE-framed ("event:...\ndata:..."), bare concatenated JSON, or single JSON
const parseRawSSE = (text: string): SSEParsed[] => {
  const result: SSEParsed[] = [];

  // Try SSE-framed first
  let sseEvent = "", sseData = "";
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (t.startsWith("event:")) sseEvent = t.slice(6).trim();
    else if (t.startsWith("data:")) sseData += t.slice(5).trim();
  }
  if (sseData) {
    // Could be single JSON or concatenated
    const objs = splitConcatenatedJSON(sseData);
    if (objs.length > 0) {
      for (const obj of objs) {
        const ev = sseEvent || obj.type || "";
        result.push({ type: ev, index: obj.index, data: obj, delta: obj.delta, content_block: obj.content_block, message: obj.message });
      }
      return result;
    }
  }

  // Try bare concatenated JSON (or single JSON)
  const objs = splitConcatenatedJSON(text);
  if (objs.length > 0) {
    for (const obj of objs) {
      result.push({ type: obj.type || "", index: obj.index, data: obj, delta: obj.delta, content_block: obj.content_block, message: obj.message });
    }
    return result;
  }

  return result;
};

// ── Content block ────────────────────────────────────────────────────────
interface ContentBlock {
  type: string; mergedText: string; toolName?: string; toolId?: string; toolInput?: any;
}

interface MergedGroup {
  id: string; events: ObserverTLSEvent[];
  startTime: string; endTime: string; totalSize: number;
  messageRole?: string; messageModel?: string; messageId?: string;
  contentBlocks: ContentBlock[]; rawMerged: string;
}

// ── Content block merging (handles Anthropic + OpenAI + generic) ─────────
const mergeContentBlocks = (parsedEvents: SSEParsed[]): ContentBlock[] => {
  const blocks = new Map<string, ContentBlock>();

  for (const p of parsedEvents) {
    const evType = p.type;
    if (!evType || NOOP_TYPES.has(evType)) continue;

    // message_start → metadata
    if (evType === "message_start") continue;
    if (evType === "message_delta") continue;

    // content_block_start
    if (evType === "content_block_start" && p.content_block) {
      const cb = p.content_block;
      const key = `cb-${p.index ?? blocks.size}`;
      blocks.set(key, {
        type: cb.type || "unknown",
        mergedText: cb.text || cb.thinking || "",
        toolName: cb.name, toolId: cb.id, toolInput: cb.input,
      });
      continue;
    }

    // content_block_delta — accumulate text / thinking / partial_json / input_json_delta
    if (evType === "content_block_delta" && p.delta) {
      const key = `cb-${p.index ?? blocks.size}`;
      const existing = blocks.get(key);
      const delta = p.delta;
      if (existing) {
        if (delta.text) existing.mergedText += delta.text;
        else if (delta.thinking) existing.mergedText += delta.thinking;
        else if (delta.partial_json || delta.input_json_delta?.partial_json) {
          const pj = delta.partial_json || delta.input_json_delta?.partial_json || "";
          existing.mergedText += pj;
          existing.toolInput = tryParseJSON(existing.mergedText);
        }
        else if (delta.signature_delta) { /* ignore */ }
      } else {
        const text = delta.text || delta.thinking ||
          delta.partial_json || delta.input_json_delta?.partial_json || "";
        blocks.set(key, { type: delta.type || "text", mergedText: text });
      }
      continue;
    }

    // content_block_stop
    if (evType === "content_block_stop") continue;

    // OpenAI-style: {"choices":[{"delta":{"content":"..."}}]}
    if (p.data?.choices?.[0]?.delta?.content) {
      const key = "text-delta";
      const ex = blocks.get(key);
      if (ex) ex.mergedText += p.data.choices[0].delta.content;
      else blocks.set(key, { type: "text", mergedText: p.data.choices[0].delta.content });
      continue;
    }

    // Unknown event type — store as raw data
    const key = `other-${blocks.size}`;
    const text = formatJSON(p.data, 2000);
    const ex = blocks.get(key);
    if (ex) ex.mergedText += "\n" + text;
    else blocks.set(key, { type: "data", mergedText: text });
  }

  return Array.from(blocks.values());
};

const extractMessageMeta = (events: SSEParsed[]): { role?: string; model?: string; id?: string } => {
  for (const p of events) {
    if (p.type === "message_start" && p.message) {
      return { role: p.message.role, model: p.message.model, id: p.message.id };
    }
  }
  return {};
};

// ── Check if raw text looks like SSE JSON ───────────────────────────────
const looksLikeSSEJSON = (text: string): boolean => {
  const t = text.trim();
  return t.startsWith('{"type":"') &&
    /"(message_start|content_block|ping|message_delta|message_stop)"/.test(t.slice(0, 100));
};

// ── Build groups ─────────────────────────────────────────────────────────
const streamGroups = computed(() => {
  const sorted = [...props.events].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );
  return {
    send: buildGroups(sorted.filter((e) => e.direction === "send")),
    recv: buildGroups(sorted.filter((e) => e.direction !== "send")),
  };
});

let _gid = 0;
const nextId = () => `g${_gid++}`;

const buildGroups = (events: ObserverTLSEvent[]): MergedGroup[] => {
  _gid = 0;
  if (events.length === 0) return [];
  const groups: MergedGroup[] = [];
  let i = 0;

  while (i < events.length) {
    const ev = events[i];

    // HTTP request — individual with parsed body
    if (ev.type === "http_request") {
      const raw = getRawText(ev);
      const json = tryParseJSON(raw);
      groups.push({
        id: nextId(), events: [ev],
        startTime: ev.timestamp, endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: json
          ? [{ type: "request_body", mergedText: formatJSON(json, 4000) }]
          : raw ? [{ type: "request_body", mergedText: raw.slice(0, 3000) }] : [],
        rawMerged: raw,
      });
      i++; continue;
    }

    // HTTP response — individual
    if (ev.type === "http_response") {
      const raw = getRawText(ev);
      groups.push({
        id: nextId(), events: [ev],
        startTime: ev.timestamp, endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: raw ? [{ type: "response_body", mergedText: raw.slice(0, 3000) }] : [],
        rawMerged: raw,
      });
      i++; continue;
    }

    // SSE stream + raw events that look like SSE JSON — merge all together
    if (ev.type === "sse_message" || (ev.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(ev)))) {
      const batch: ObserverTLSEvent[] = [ev];
      let j = i + 1;
      while (j < events.length) {
        const next = events[j];
        if (next.type === "sse_message" || (next.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(next)))) {
          batch.push(next); j++;
        } else {
          break;
        }
      }

      // Parse all events into SSE data objects
      const allParsed: SSEParsed[] = [];
      for (const be of batch) {
        const text = getRawText(be);
        allParsed.push(...parseRawSSE(text));
      }

      const meta = extractMessageMeta(allParsed);
      groups.push({
        id: nextId(), events: batch,
        startTime: batch[0].timestamp, endTime: batch[batch.length - 1].timestamp,
        totalSize: batch.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
        messageRole: meta.role, messageModel: meta.model, messageId: meta.id,
        contentBlocks: allParsed.length > 0 ? mergeContentBlocks(allParsed) : [],
        rawMerged: allParsed.length > 0 ? "" : batch.map(getRawText).join("\n"),
      });
      i = j; continue;
    }

    // Raw — group consecutive non-SSE raw events
    const batch: ObserverTLSEvent[] = [ev];
    let k = i + 1;
    while (k < events.length &&
           events[k].type === "tls_plaintext" &&
           !looksLikeSSEJSON(getRawText(events[k]))) {
      batch.push(events[k]); k++;
    }
    const rawBody = batch.map(getRawText).filter(Boolean).join("\n");
    groups.push({
      id: nextId(), events: batch,
      startTime: batch[0].timestamp, endTime: batch[batch.length - 1].timestamp,
      totalSize: batch.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
      contentBlocks: rawBody ? [{ type: "raw", mergedText: rawBody.slice(0, 3000) }] : [],
      rawMerged: rawBody,
    });
    i = k;
  }
  return groups;
};

// ── Display helpers ──────────────────────────────────────────────────────
const blockIcon = (t: string) => {
  if (t === "text") return MessageOutlined;
  if (t === "tool_use") return ToolOutlined;
  if (t === "thinking") return ThunderboltOutlined;
  if (t === "request_body") return ArrowUpOutlined;
  if (t === "response_body") return ArrowDownOutlined;
  return CodeOutlined;
};
const blockLabel = (t: string) => {
  const m: Record<string,string> = { text:"Text", tool_use:"Tool Use", thinking:"Thinking", request_body:"Request Body", response_body:"Response Body", raw:"Raw Data", data:"SSE Data" };
  return m[t] || t;
};
const blockColor = (t: string) => {
  const m: Record<string,string> = { text:"green", tool_use:"orange", thinking:"purple", request_body:"blue", response_body:"cyan" };
  return m[t] || "default";
};
const firstTypeLabel = (g: MergedGroup): string => {
  const ev = g.events[0];
  if (!ev) return "?";
  if (ev.type === "http_request") return ev.method || "REQ";
  if (ev.type === "http_response") return `HTTP ${ev.status||""}`;
  if (ev.type === "sse_message") return "SSE Stream";
  return ev.type;
};

const formatBytes = (bytes: number): string => {
  if (!bytes) return "0 B";
  const u = ["B", "KB", "MB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), u.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};
const formatTime = (ts: string): string => {
  try { return new Date(ts).toLocaleTimeString(); } catch { return ts?.slice(11, 19) || ""; }
};
const formatTimeRange = (s: string, e: string): string =>
  s === e ? formatTime(s) : `${formatTime(s)} → ${formatTime(e)}`;

// ── Stats & state ────────────────────────────────────────────────────────
const stats = computed(() => ({
  sendCount: streamGroups.value.send.length,
  recvCount: streamGroups.value.recv.length,
  sendBytes: streamGroups.value.send.reduce((s, g) => s + g.totalSize, 0),
  recvBytes: streamGroups.value.recv.reduce((s, g) => s + g.totalSize, 0),
}));

const expanded = ref<Set<string>>(new Set());
const toggle = (id: string) => {
  const s = new Set(expanded.value);
  if (s.has(id)) s.delete(id); else s.add(id);
  expanded.value = s;
};
</script>

<template>
  <div class="ac-root">
    <div class="ac-stats">
      <div class="ac-stat-item send"><ArrowUpOutlined /><span class="ac-stat-label">Upstream</span><span class="ac-stat-val">{{ stats.sendCount }} groups</span><span class="ac-stat-size">{{ formatBytes(stats.sendBytes) }}</span></div>
      <div class="ac-stat-item recv"><ArrowDownOutlined /><span class="ac-stat-label">Downstream</span><span class="ac-stat-val">{{ stats.recvCount }} groups</span><span class="ac-stat-size">{{ formatBytes(stats.recvBytes) }}</span></div>
    </div>

    <div class="ac-columns">
      <!-- UPSTREAM -->
      <div class="ac-col ac-send-col">
        <div class="ac-col-header"><ArrowUpOutlined style="color:#f59e0b" /><span>Upstream (Agent → Server)</span></div>
        <a-empty v-if="streamGroups.send.length===0" description="No upstream data" style="padding:24px" />
        <div v-else class="ac-list">
          <div v-for="g in streamGroups.send" :key="g.id" class="ac-card" :class="{'ac-sse':g.events[0]?.type==='sse_message'}">
            <!-- head -->
            <div class="ac-head" @click="toggle(g.id)">
              <span class="ac-h-icon"><CaretDownOutlined v-if="expanded.has(g.id)" /><CaretRightOutlined v-else /></span>
              <a-tag :color="blockColor(g.contentBlocks[0]?.type||'raw')" size="small">{{ firstTypeLabel(g) }}</a-tag>
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{ g.messageRole }}</span>
                <span v-if="g.messageModel" class="ac-model">{{ g.messageModel }}</span>
                <span v-if="g.contentBlocks.length>1" class="ac-cbc">{{ g.contentBlocks.length }} blocks</span>
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{ g.events[0].host }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{ formatTimeRange(g.startTime,g.endTime) }}</span>
              <a-button size="small" type="link" class="ac-view" @click.stop="emit('viewEvent',g.events[0])"><EyeOutlined /></a-button>
            </div>
            <div v-if="g.events.length>1 && g.events[0]?.type==='sse_message'" class="ac-merged"><MergeCellsOutlined /> {{ g.events.length }} SSE → {{ g.contentBlocks.length }} block{{g.contentBlocks.length!==1?'s':''}}</div>
            <!-- body -->
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta"><span class="ac-k">ID</span><code>{{ g.messageId }}</code></div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <div v-for="(b,bi) in g.contentBlocks" :key="bi" class="ac-block" :class="`ac-b-${b.type}`">
                  <div class="ac-b-head"><component :is="blockIcon(b.type)" class="ac-b-icon" /><a-tag :color="blockColor(b.type)" size="small">{{ blockLabel(b.type) }}</a-tag><span v-if="b.toolName" class="ac-tn">{{ b.toolName }}</span><span v-if="b.toolId" class="ac-tid">{{ b.toolId }}</span><span class="ac-bsz">{{ formatBytes(b.mergedText.length) }}</span></div>
                  <div class="ac-b-body"><pre>{{ b.mergedText.slice(0,4000) }}{{ b.mergedText.length>4000?'\n… [truncated]':'' }}</pre></div>
                </div>
              </div>
              <div v-else-if="g.rawMerged" class="ac-b-body"><pre>{{ g.rawMerged.slice(0,2000) }}</pre></div>
            </div>
          </div>
        </div>
      </div>

      <!-- DOWNSTREAM -->
      <div class="ac-col ac-recv-col">
        <div class="ac-col-header"><ArrowDownOutlined style="color:#06b6d4" /><span>Downstream (Server → Agent)</span></div>
        <a-empty v-if="streamGroups.recv.length===0" description="No downstream data" style="padding:24px" />
        <div v-else class="ac-list">
          <div v-for="g in streamGroups.recv" :key="g.id" class="ac-card" :class="{'ac-sse':g.events[0]?.type==='sse_message'}">
            <div class="ac-head" @click="toggle(g.id)">
              <span class="ac-h-icon"><CaretDownOutlined v-if="expanded.has(g.id)" /><CaretRightOutlined v-else /></span>
              <a-tag :color="blockColor(g.contentBlocks[0]?.type||'raw')" size="small">{{ firstTypeLabel(g) }}</a-tag>
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{ g.messageRole }}</span>
                <span v-if="g.messageModel" class="ac-model">{{ g.messageModel }}</span>
                <span v-if="g.contentBlocks.length>1" class="ac-cbc">{{ g.contentBlocks.length }} blocks</span>
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{ g.events[0].host }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{ formatTimeRange(g.startTime,g.endTime) }}</span>
              <a-button size="small" type="link" class="ac-view" @click.stop="emit('viewEvent',g.events[0])"><EyeOutlined /></a-button>
            </div>
            <div v-if="g.events.length>1 && g.events[0]?.type==='sse_message'" class="ac-merged"><MergeCellsOutlined /> {{ g.events.length }} SSE → {{ g.contentBlocks.length }} block{{g.contentBlocks.length!==1?'s':''}}</div>
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta"><span class="ac-k">ID</span><code>{{ g.messageId }}</code></div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <div v-for="(b,bi) in g.contentBlocks" :key="bi" class="ac-block" :class="`ac-b-${b.type}`">
                  <div class="ac-b-head"><component :is="blockIcon(b.type)" class="ac-b-icon" /><a-tag :color="blockColor(b.type)" size="small">{{ blockLabel(b.type) }}</a-tag><span v-if="b.toolName" class="ac-tn">{{ b.toolName }}</span><span v-if="b.toolId" class="ac-tid">{{ b.toolId }}</span><span class="ac-bsz">{{ formatBytes(b.mergedText.length) }}</span></div>
                  <div class="ac-b-body"><pre>{{ b.mergedText.slice(0,4000) }}{{ b.mergedText.length>4000?'\n… [truncated]':'' }}</pre></div>
                </div>
              </div>
              <div v-else-if="g.rawMerged" class="ac-b-body"><pre>{{ g.rawMerged.slice(0,2000) }}</pre></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ac-root{display:flex;flex-direction:column;gap:12px}
.ac-stats{display:flex;gap:16px;padding:8px 12px;background:#f8fafc;border-radius:6px;border:1px solid #e2e8f0}
.ac-stat-item{display:flex;align-items:center;gap:6px;font-size:12px}
.ac-stat-item.send{color:#d97706}.ac-stat-item.recv{color:#059669}
.ac-stat-label{font-weight:600;color:#475569}.ac-stat-val{color:#64748b}
.ac-stat-size{font-family:ui-monospace,monospace;font-weight:600;color:#334155}
.ac-columns{display:grid;grid-template-columns:1fr 1fr;gap:12px;max-height:560px}
.ac-col{display:flex;flex-direction:column;overflow-y:auto;border:1px solid #e2e8f0;border-radius:8px;background:#fafafa}
.ac-send-col{border-left:3px solid #f59e0b}.ac-recv-col{border-left:3px solid #06b6d4}
.ac-col-header{display:flex;align-items:center;gap:6px;padding:8px 12px;font-size:12px;font-weight:600;color:#334155;background:#fff;border-bottom:1px solid #e2e8f0;position:sticky;top:0;z-index:1}
.ac-list{display:flex;flex-direction:column;gap:4px;padding:6px}
.ac-card{background:#fff;border:1px solid #e2e8f0;border-radius:6px;cursor:pointer;transition:box-shadow .15s}
.ac-card:hover{box-shadow:0 1px 4px rgba(0,0,0,.08)}
.ac-card.ac-sse{border-color:#c084fc;background:#faf5ff}
.ac-head{display:flex;align-items:center;gap:6px;padding:6px 10px}
.ac-h-icon{font-size:9px;color:#94a3b8}
.ac-h-meta{display:flex;align-items:center;gap:6px;flex:1;overflow:hidden}
.ac-role{font-family:ui-monospace,monospace;font-size:11px;color:#7c3aed;font-weight:600}
.ac-model{font-size:10px;color:#94a3b8;font-family:ui-monospace,monospace}
.ac-cbc{font-size:10px;color:#64748b}
.ac-h-host{font-size:10px;color:#64748b;font-family:ui-monospace,monospace;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ac-h-size{font-size:10px;color:#94a3b8;font-family:ui-monospace,monospace}
.ac-h-time{font-size:10px;color:#94a3b8;font-family:ui-monospace,monospace}
.ac-view{padding:0;font-size:13px}
.ac-merged{padding:2px 10px 6px;font-size:10px;color:#7c3aed;font-weight:500;display:flex;align-items:center;gap:4px}
.ac-body{padding:6px 10px 10px;border-top:1px solid #f0f0f0}
.ac-meta{display:flex;gap:6px;font-size:11px;margin-bottom:6px;align-items:baseline}
.ac-k{color:#94a3b8;text-transform:uppercase;min-width:30px;font-size:10px}
.ac-meta code{font-family:ui-monospace,monospace;font-size:10px;color:#0f172a;word-break:break-all}
.ac-blocks{display:flex;flex-direction:column;gap:8px}
.ac-block{border:1px solid #e2e8f0;border-radius:6px;overflow:hidden}
.ac-b-text{border-color:#86efac}.ac-b-tool_use{border-color:#fdba74}.ac-b-thinking{border-color:#c4b5fd}.ac-b-request_body{border-color:#93c5fd}.ac-b-response_body{border-color:#67e8f9}
.ac-b-head{display:flex;align-items:center;gap:5px;padding:4px 8px;background:#f8fafc;border-bottom:1px solid #f0f0f0;font-size:11px}
.ac-b-icon{font-size:12px;color:#64748b}
.ac-tn{font-family:ui-monospace,monospace;font-weight:600;color:#d97706;font-size:11px}
.ac-tid{font-family:ui-monospace,monospace;font-size:9px;color:#94a3b8}
.ac-bsz{margin-left:auto;font-size:9px;color:#94a3b8}
.ac-b-body pre{background:#0f172a;color:#dbeafe;padding:10px;border-radius:0;font-size:11px;line-height:1.55;max-height:300px;overflow:auto;margin:0;white-space:pre-wrap}
</style>
