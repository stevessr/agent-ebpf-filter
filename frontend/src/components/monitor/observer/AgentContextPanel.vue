<script setup lang="ts">
import { computed, ref, watch } from "vue";
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
const hexToText = (hex: string): string => {
  // Robust hex decoder: strips whitespace, skips non-hex chars (don't break!)
  const clean = hex.replace(/[^0-9a-fA-F]/g, "");
  const bytes: number[] = [];
  for (let i = 0; i < clean.length - 1; i += 2) {
    const byte = parseInt(clean.slice(i, i + 2), 16);
    if (!isNaN(byte)) bytes.push(byte);
  }
  return bytes
    .map((b) => (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d ? String.fromCharCode(b) : ".")
    .join("");
};

const getRawText = (ev: ObserverTLSEvent): string =>
  ev.body || (ev.raw_hex_dump ? hexToText(ev.raw_hex_dump) : "");

// Strip HTTP chunked transfer encoding markers (standalone hex followed by \r\n)
const stripChunkedEncoding = (text: string): string =>
  text.replace(/^[0-9a-fA-F]{1,8}\s*$/gm, "").replace(/\n{3,}/g, "\n\n");

const tryParseJSON = (text: string): any | null => {
  try { return JSON.parse(text); } catch { return null; }
};

const formatJSON = (obj: any): string => {
  try { return JSON.stringify(obj, null, 2); } catch { return String(obj); }
};

// ── SSE parser (handles framed SSE + bare JSON + chunked + truncated) ────
interface SSEParsed { type: string; index?: number; data: any; delta?: any; content_block?: any; message?: any; usage?: any; }
const NOOP_TYPES = new Set(["ping", "message_stop"]);

// Depth-aware split of concatenated JSON: {a}{b} → [{a}, {b}]
const splitConcatenatedJSON = (text: string): any[] => {
  const results: any[] = [];
  let depth = 0, start = -1, inString = false, escaped = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (escaped) { escaped = false; continue; }
    if (ch === '\\') { escaped = true; continue; }
    if (ch === '"') { inString = !inString; continue; }
    if (inString) continue;
    if (ch === '{') { if (depth === 0) start = i; depth++; }
    else if (ch === '}') {
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

// Try to extract partial_json value from truncated JSON via regex
const extractPartialJSON = (text: string): string | null => {
  const m = text.match(/"partial_json"\s*:\s*"((?:[^"\\]|\\.)*)"/);
  return m ? JSON.parse(`"${m[1]}"`) : null; // unescape via JSON.parse
};

// Parse one SSE event's data field into SSEParsed objects
const parseOneSSEData = (evType: string, data: string): SSEParsed[] => {
  const objs = splitConcatenatedJSON(data);
  if (objs.length > 0) {
    return objs.map((obj) => ({
      type: evType || obj.type || "",
      index: obj.index,
      data: obj,
      delta: obj.delta,
      content_block: obj.content_block,
      message: obj.message,
      usage: obj.usage || obj.message?.usage,
    }));
  }
  // Truncated JSON — try to salvage partial_json
  const pj = extractPartialJSON(data);
  if (pj) {
    return [{
      type: evType || "content_block_delta",
      data: {},
      delta: { type: "input_json_delta", partial_json: pj },
    }];
  }
  return [];
};

// Main parser: handles both SSE-framed text and bare concatenated JSON
const parseRawSSE = (text: string): SSEParsed[] => {
  const clean = stripChunkedEncoding(text);
  const result: SSEParsed[] = [];

  // Strategy 1: split by SSE "event:" boundaries
  const eventSections = clean.split(/^event:\s*/m).filter(Boolean);
  if (eventSections.length > 0 && clean.includes("event:")) {
    for (const section of eventSections) {
      const lines = section.split("\n");
      let evType = "";
      let data = "";
      for (const line of lines) {
        const t = line.trim();
        if (!evType && t.length > 0 && !t.startsWith("data:")) {
          // First non-empty, non-data line after "event:" split IS the event type
          evType = t;
        } else if (t.startsWith("data:")) {
          data += t.slice(5).trim();
        }
      }
      if (data) {
        result.push(...parseOneSSEData(evType, data));
      }
    }
    if (result.length > 0) return result;
  }

  // Strategy 2: bare concatenated JSON
  const objs = splitConcatenatedJSON(clean);
  if (objs.length > 0) {
    for (const obj of objs) {
      result.push({
        type: obj.type || "", index: obj.index,
        data: obj, delta: obj.delta,
        content_block: obj.content_block, message: obj.message,
        usage: obj.usage || obj.message?.usage,
      });
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
  usage?: { input_tokens?: number; output_tokens?: number; cache_read_input_tokens?: number; cache_creation_input_tokens?: number };
}

// ── Content block merging (handles Anthropic + OpenAI + generic) ─────────
// Normalize delta types to canonical block types (per Anthropic SSE spec)
const normalizeBlockType = (t: string): string => {
  if (t === "thinking_delta" || t === "thinking") return "thinking";
  if (t === "text_delta" || t === "text") return "text";
  if (t === "input_json_delta" || t === "tool_use") return "tool_use";
  if (t === "signature_delta") return "signature";
  if (t === "citations_delta") return "citations";
  return t;
};

const mergeContentBlocks = (parsedEvents: SSEParsed[]): ContentBlock[] => {
  const blocks = new Map<string, ContentBlock>();

  for (const p of parsedEvents) {
    const evType = p.type;
    if (!evType || NOOP_TYPES.has(evType)) continue;

    if (evType === "message_start" || evType === "message_delta") continue;

    // content_block_start — register block with canonical type
    if (evType === "content_block_start" && p.content_block) {
      const cb = p.content_block;
      const key = `cb-${p.index ?? blocks.size}`;
      blocks.set(key, {
        type: normalizeBlockType(cb.type || "unknown"),
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
        else if (delta.signature || delta.signature_delta) { /* signature — ignore */ }
        else if (delta.citations || delta.citations_delta) { /* citations — ignore for now */ }
      } else {
        // New block — use normalized type from delta (skip signature/citations as no-ops)
        const deltaType = delta.type || "text";
        if (deltaType === "signature_delta" || deltaType === "citations_delta") continue;
        const text = delta.text || delta.thinking ||
          delta.partial_json || delta.input_json_delta?.partial_json || "";
        blocks.set(key, {
          type: normalizeBlockType(deltaType),
          mergedText: text,
        });
      }
      continue;
    }

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
    const text = formatJSON(p.data);
    const ex = blocks.get(key);
    if (ex) ex.mergedText += "\n" + text;
    else blocks.set(key, { type: "data", mergedText: text });
  }

  return Array.from(blocks.values());
};

// Extract message metadata + merge usage from BOTH message_start and message_delta.
// Per Anthropic SSE spec:
//   message_start.message.usage → input_tokens, cache_*, initial output_tokens (usually 0)
//   message_delta.usage → cumulative output_tokens (final, non-zero)
const extractMessageMeta = (events: SSEParsed[]): { role?: string; model?: string; id?: string; usage?: any } => {
  let meta: { role?: string; model?: string; id?: string; usage?: Record<string,number> } = {};
  for (const p of events) {
    if (p.type === "message_start" && p.message) {
      meta.role = p.message.role;
      meta.model = p.message.model;
      meta.id = p.message.id;
      // Take input-side tokens from message_start (output_tokens here is always 0)
      if (p.message.usage) {
        meta.usage = { ...meta.usage };
        for (const [k, v] of Object.entries(p.message.usage)) {
          if (typeof v === "number") meta.usage[k] = v;
        }
      }
    }
    // message_delta carries cumulative final output_tokens — overwrites the 0 from message_start
    if ((p.type === "message_delta" || p.type === "message_stop") && p.usage) {
      meta.usage = { ...meta.usage };
      for (const [k, v] of Object.entries(p.usage)) {
        if (typeof v === "number") meta.usage[k] = v;
      }
    }
  }
  return meta;
};

// Format usage object into displayable key-value pairs (camelCase→Title Case)
const usageEntries = (u: any): { key: string; label: string; value: number }[] => {
  if (!u || typeof u !== "object") return [];
  return Object.entries(u)
    .filter(([, v]) => typeof v === "number")
    .map(([k, v]) => ({
      key: k,
      label: k.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase()),
      value: v as number,
    }));
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
  // Case-insensitive direction match (backend uses lowercase "send"/"recv")
  const isSend = (e: ObserverTLSEvent) =>
    e.direction?.toLowerCase() === "send";
  return {
    send: buildGroups(sorted.filter(isSend), "send"),
    recv: buildGroups(sorted.filter((e) => !isSend(e)), "recv"),
  };
});

let _gid = 0;
const nextId = () => `g${_gid++}`;

const buildGroups = (events: ObserverTLSEvent[], dir: "send" | "recv"): MergedGroup[] => {
  _gid = 0;
  if (events.length === 0) return [];
  const groups: MergedGroup[] = [];
  let i = 0;

  while (i < events.length) {
    const ev = events[i];

    // HTTP request — parse body, extract tool_results from messages array
    if (ev.type === "http_request") {
      const raw = getRawText(ev);
      const json = tryParseJSON(raw);
      const blocks: ContentBlock[] = [];

      if (json) {
        const messages = json.messages;
        if (Array.isArray(messages)) {
          for (const msg of messages) {
            const content = msg.content;
            if (msg.role === "user" && Array.isArray(content)) {
              for (const item of content) {
                if (item.type === "tool_result") {
                  const resultText = typeof item.content === "string"
                    ? item.content
                    : Array.isArray(item.content)
                      ? item.content.map((c: any) => c.text || JSON.stringify(c)).join("\n")
                      : formatJSON(item.content || item);
                  blocks.push({
                    type: "tool_result",
                    mergedText: resultText,
                    toolId: item.tool_use_id,
                  });
                }
              }
            }
          }
        }
        if (blocks.length === 0) {
          blocks.push({ type: "request_body", mergedText: formatJSON(json) });
        }
      } else if (raw) {
        blocks.push({ type: "request_body", mergedText: raw });
      } else {
        // No body available — show HTTP metadata summary
        const meta = [`${ev.method || 'GET'} ${ev.url || '/'}`];
        if (ev.host) meta.push(`Host: ${ev.host}`);
        if (ev.headers && Object.keys(ev.headers).length) {
          for (const [k, v] of Object.entries(ev.headers)) {
            if (k !== "host") meta.push(`${k}: ${v}`);
          }
        }
        if (meta.length > 1) {
          blocks.push({ type: "request_body", mergedText: meta.join("\n") });
        }
      }

      groups.push({
        id: nextId(), events: [ev],
        startTime: ev.timestamp, endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: blocks, rawMerged: raw,
      });
      i++; continue;
    }

    // HTTP/2 — connection preface or data frames (non-HTTP/1.1 protocol)
    if (ev.type === "http2_preface" || ev.type === "http2_frame") {
      const raw = getRawText(ev);
      const blocks: ContentBlock[] = [];
      // For HTTP/2 frames, the backend may have extracted DATA frame body text
      if (ev.body) {
        const json = tryParseJSON(ev.body);
        if (json && typeof json === "object") {
          const messages = json.messages;
          if (Array.isArray(messages)) {
            for (const msg of messages) {
              const content = msg.content;
              if (msg.role === "user" && Array.isArray(content)) {
                for (const item of content) {
                  if (item.type === "tool_result") {
                    const resultText = typeof item.content === "string"
                      ? item.content
                      : Array.isArray(item.content)
                        ? item.content.map((c: any) => c.text || JSON.stringify(c)).join("\n")
                        : formatJSON(item.content || item);
                    blocks.push({ type: "tool_result", mergedText: resultText, toolId: item.tool_use_id });
                  }
                }
              }
            }
          }
          if (blocks.length === 0) {
            blocks.push({ type: "request_body", mergedText: formatJSON(json) });
          }
        } else if (ev.body) {
          blocks.push({ type: "request_body", mergedText: ev.body });
        } else if (raw) {
          blocks.push({ type: "raw", mergedText: raw.includes("HTTP/2") ? raw : "[HTTP/2 binary — see hex]" });
        }
      } else if (raw) {
        blocks.push({ type: "raw", mergedText: "[HTTP/2 binary — see hex]\n\nRaw hex decode:\n" + raw.slice(0, 500) });
      }
      groups.push({
        id: nextId(), events: [ev],
        startTime: ev.timestamp, endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: blocks, rawMerged: raw,
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
        contentBlocks: raw ? [{ type: "response_body", mergedText: raw }] : [],
        rawMerged: raw,
      });
      i++; continue;
    }

    // DOWNSTREAM only: SSE stream merging (SSE is server→client protocol)
    if (dir === "recv" && (ev.type === "sse_message" || (ev.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(ev))))) {
      const batch: ObserverTLSEvent[] = [ev];
      let j = i + 1;
      while (j < events.length) {
        const next = events[j];
        if (next.type === "sse_message" || (next.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(next)))) {
          batch.push(next); j++;
        } else break;
      }
      const allParsed: SSEParsed[] = [];
      for (const be of batch) allParsed.push(...parseRawSSE(getRawText(be)));
      const meta = extractMessageMeta(allParsed);
      groups.push({
        id: nextId(), events: batch,
        startTime: batch[0].timestamp, endTime: batch[batch.length - 1].timestamp,
        totalSize: batch.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
        messageRole: meta.role, messageModel: meta.model, messageId: meta.id, usage: meta.usage,
        contentBlocks: allParsed.length > 0 ? mergeContentBlocks(allParsed) : [],
        rawMerged: allParsed.length > 0 ? "" : batch.map(getRawText).join("\n"),
      });
      i = j; continue;
    }

    // Raw — group consecutive non-SSE raw events
    const batch: ObserverTLSEvent[] = [ev];
    let k = i + 1;
    while (k < events.length && events[k].type === "tls_plaintext") {
      // For upstream: merge all consecutive raw (may be fragmented HTTP body)
      // For downstream: only merge if NOT SSE-looking
      if (dir === "recv" && looksLikeSSEJSON(getRawText(events[k]))) break;
      batch.push(events[k]); k++;
    }

    // Try to extract structured content from raw text
    const rawBody = batch.map(getRawText).filter(Boolean).join("\n");
    const blocks: ContentBlock[] = [];

    // Upstream raw data: comprehensive JSON extraction
    if (dir === "send" && rawBody) {
      // 1. Try complete JSON parse first (single POST body)
      const parsed = tryParseJSON(rawBody);
      if (parsed && typeof parsed === "object") {
        const messages = parsed.messages;
        if (Array.isArray(messages)) {
          for (const msg of messages) {
            const content = msg.content;
            if (msg.role === "user" && Array.isArray(content)) {
              for (const item of content) {
                if (item.type === "tool_result") {
                  const resultText = typeof item.content === "string"
                    ? item.content
                    : Array.isArray(item.content)
                      ? item.content.map((c: any) => c.text || JSON.stringify(c)).join("\n")
                      : formatJSON(item.content || item);
                  blocks.push({ type: "tool_result", mergedText: resultText, toolId: item.tool_use_id });
                }
              }
            }
          }
        }
        if (blocks.length === 0) {
          blocks.push({ type: "request_body", mergedText: formatJSON(parsed) });
        }
      } else {
        // 2. Try split concatenated JSON (fragmented across BF captures)
        const objs = splitConcatenatedJSON(rawBody);
        if (objs.length > 0) {
          for (const obj of objs) {
            blocks.push({ type: "request_body", mergedText: formatJSON(obj) });
          }
        } else {
          // 3. Fall back to raw text (non-JSON body)
          blocks.push({ type: "raw", mergedText: rawBody });
        }
      }
    } else if (rawBody) {
      blocks.push({ type: "raw", mergedText: rawBody });
    }

    groups.push({
      id: nextId(), events: batch,
      startTime: batch[0].timestamp, endTime: batch[batch.length - 1].timestamp,
      totalSize: batch.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
      contentBlocks: blocks, rawMerged: rawBody,
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
  const m: Record<string,string> = { text:"Text", tool_use:"Tool Use", tool_result:"Tool Result", thinking:"Thinking", signature:"Signature", citations:"Citations", request_body:"Request Body", response_body:"Response Body", raw:"Raw Data", data:"SSE Data", http2_frame:"HTTP/2 Frame", http2_preface:"HTTP/2 Preface" };
  return m[t] || t;
};
const blockColor = (t: string) => {
  const m: Record<string,string> = { text:"green", tool_use:"orange", tool_result:"volcano", thinking:"purple", signature:"geekblue", citations:"cyan", request_body:"blue", response_body:"cyan" };
  return m[t] || "default";
};
const firstTypeLabel = (g: MergedGroup): string => {
  const ev = g.events[0];
  if (!ev) return "?";
  if (ev.type === "http_request") return ev.method || "REQ";
  if (ev.type === "http_response") return `HTTP ${ev.status||""}`;
  if (ev.type === "http2_preface") return "H2 Preface";
  if (ev.type === "http2_frame") return "H2 Frame";
  // For SSE streams (including raw events with SSE JSON), derive label from content blocks
  if (g.contentBlocks.length > 0) {
    const types = [...new Set(g.contentBlocks.map((b) => blockLabel(b.type)))];
    if (types.length === 1) return types[0];
    return types.join("+");
  }
  return ev.type === "sse_message" ? "SSE Stream" : ev.type;
};

// Get display text for a content block — NO truncation, full content always shown
const blockDisplayText = (b: ContentBlock): string => {
  if (b.toolInput && typeof b.toolInput === "object" && Object.keys(b.toolInput).length > 0) {
    try { return JSON.stringify(b.toolInput, null, 2); } catch { /* fall through */ }
  }
  const parsed = tryParseJSON(b.mergedText);
  if (parsed && typeof parsed === "object" && Object.keys(parsed).length > 0) {
    try { return JSON.stringify(parsed, null, 2); } catch { /* fall through */ }
  }
  return b.mergedText;
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

// ── Persistence helpers (define BEFORE use — const arrows are not hoisted) ──
function readStoredCap(key: string, fallback: number): number {
  try {
    const v = localStorage.getItem(key);
    if (v === null) return fallback;
    const n = parseInt(v, 10);
    return Number.isFinite(n) && n >= 0 ? n : fallback;
  } catch { return fallback; }
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
  } catch { return fallback; }
}
const rawMode = ref<RawMode>(readStoredRawMode(RAW_MODE_KEY, "skip"));
const CONTEXT_BLOCK_TYPES = new Set(["text", "thinking", "tool_use", "tool_result", "request_body", "response_body", "signature", "citations"]);

// Persist rawMode
watch(rawMode, (v) => {
  try { localStorage.setItem(RAW_MODE_KEY, v); } catch { /* ignore */ }
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
  try { localStorage.setItem(TLS_CAP_KEY, String(v)); } catch { /* ignore */ }
};

// Filter groups based on rawMode
const filteredSendGroups = computed(() => {
  const gs = streamGroups.value.send;
  if (rawMode.value === "show") return gs;
  // "skip": drop groups that are entirely raw (no context blocks)
  if (rawMode.value === "skip") return gs.filter((g) => g.contentBlocks.some((b) => CONTEXT_BLOCK_TYPES.has(b.type)));
  // "hide": show all groups, raw blocks hidden per-block via blockVisible
  return gs;
});
const filteredRecvGroups = computed(() => {
  const gs = streamGroups.value.recv;
  if (rawMode.value === "show") return gs;
  if (rawMode.value === "skip") return gs.filter((g) => g.contentBlocks.some((b) => CONTEXT_BLOCK_TYPES.has(b.type)));
  return gs;
});

// ── Stats & state ────────────────────────────────────────────────────────
const stats = computed(() => {
  const byDir = (dir: string) => props.events.filter((e) => e.direction?.toLowerCase() === dir);
  const sendRaw = byDir("send");
  const recvRaw = byDir("recv");
  const countByType = (evs: ObserverTLSEvent[]) => {
    const m: Record<string,number> = {};
    for (const e of evs) { const t = e.type || "?"; m[t] = (m[t]||0)+1; }
    return m;
  };
  return {
    sendCount: streamGroups.value.send.length,
    recvCount: streamGroups.value.recv.length,
    sendBytes: streamGroups.value.send.reduce((s, g) => s + g.totalSize, 0),
    recvBytes: streamGroups.value.recv.reduce((s, g) => s + g.totalSize, 0),
    filteredSendCount: filteredSendGroups.value.length,
    filteredRecvCount: filteredRecvGroups.value.length,
    allCount: streamGroups.value.send.length + streamGroups.value.recv.length,
    filteredAllCount: filteredSendGroups.value.length + filteredRecvGroups.value.length,
    // Debug: raw event counts
    sendRawN: sendRaw.length,
    recvRawN: recvRaw.length,
    sendTypes: countByType(sendRaw),
    recvTypes: countByType(recvRaw),
    sendWithBody: sendRaw.filter(e => e.body || e.raw_hex_dump).length,
    sendBodySizes: sendRaw.map(e => e.body_size || e.body?.length || e.raw_hex_dump?.length || 0),
  };
});

const expanded = ref<Set<string>>(new Set());
const blockExpanded = ref<Set<string>>(new Set());
const toggle = (id: string) => {
  const s = new Set(expanded.value);
  if (s.has(id)) s.delete(id); else s.add(id);
  expanded.value = s;
};
const toggleBlock = (blockId: string) => {
  const s = new Set(blockExpanded.value);
  if (s.has(blockId)) s.delete(blockId); else s.add(blockId);
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
const formatTokens = (u: any): { input: number; output: number } => {
  if (!u || typeof u !== "object") return { input: 0, output: 0 };
  const input = (typeof u.input_tokens === "number" ? u.input_tokens : 0) +
    (typeof u.cache_read_input_tokens === "number" ? u.cache_read_input_tokens : 0) +
    (typeof u.cache_creation_input_tokens === "number" ? u.cache_creation_input_tokens : 0);
  const output = typeof u.output_tokens === "number" ? u.output_tokens : 0;
  return { input, output };
};

const blockTokens = (g: MergedGroup): { input: number; output: number } =>
  formatTokens(g.usage);
</script>

<template>
  <div class="ac-root">
    <div class="ac-stats">
      <div class="ac-stat-item send"><ArrowUpOutlined /><span class="ac-stat-label">Upstream</span><span class="ac-stat-val">{{ stats.sendCount }} groups</span><span class="ac-stat-size">{{ formatBytes(stats.sendBytes) }}</span><a-tooltip placement="bottom"><template #title><span style="font-family:monospace;font-size:11px">Raw events: {{ stats.sendRawN }} (body: {{ stats.sendWithBody }})<br/>{{ JSON.stringify(stats.sendTypes) }}</span></template><span class="ac-diag-dot" :class="stats.sendRawN>0?'ac-diag-ok':'ac-diag-warn'">●</span></a-tooltip></div>
      <div class="ac-stat-item recv"><ArrowDownOutlined /><span class="ac-stat-label">Downstream</span><span class="ac-stat-val">{{ stats.recvCount }} groups</span><span class="ac-stat-size">{{ formatBytes(stats.recvBytes) }}</span><a-tooltip placement="bottom"><template #title><span style="font-family:monospace;font-size:11px">Raw events: {{ stats.recvRawN }}<br/>{{ JSON.stringify(stats.recvTypes) }}</span></template><span class="ac-diag-dot" :class="stats.recvRawN>0?'ac-diag-ok':'ac-diag-warn'">●</span></a-tooltip></div>
      <div class="ac-stat-item ac-hide-raw-toggle">
        <a-select v-model:value="rawMode" size="small" style="width:100px" :options="rawModeOptions" />
      </div>
      <div class="ac-stat-item ac-cap-ctl">
        <span class="ac-cap-label">Scrollback</span>
        <a-select v-model:value="tlsCap" size="small" style="width:90px" :options="capOptions" @change="onCapChange" />
      </div>
    </div>

    <div class="ac-columns">
      <!-- UPSTREAM -->
      <div class="ac-col ac-send-col">
        <div class="ac-col-header"><ArrowUpOutlined style="color:#f59e0b" /><span>Upstream (Agent → Server)</span></div>
        <a-empty v-if="filteredSendGroups.length===0" description="No upstream data" style="padding:24px" />
        <div v-else class="ac-list">
          <div v-for="g in filteredSendGroups" :key="g.id" class="ac-card" :class="{'ac-sse':g.events[0]?.type==='sse_message' || g.contentBlocks.length>1 || g.contentBlocks.some(b=>b.type==='thinking'||b.type==='tool_use')}">
            <!-- head -->
            <div class="ac-head" @click="toggle(g.id)">
              <span class="ac-h-icon"><CaretDownOutlined v-if="expanded.has(g.id)" /><CaretRightOutlined v-else /></span>
              <a-tag :color="blockColor(g.contentBlocks[0]?.type||'raw')" size="small">{{ firstTypeLabel(g) }}</a-tag>
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{ g.messageRole }}</span>
                <span v-if="g.messageModel" class="ac-model">{{ g.messageModel }}</span>
                <span v-if="usageEntries(g.usage||{}).length" class="ac-tok-inline">{{ usageEntries(g.usage!).reduce((s,e) => s + e.value, 0).toLocaleString() }} tok</span>
                <span v-if="g.contentBlocks.length>1" class="ac-cbc">{{ g.contentBlocks.length }} blocks</span>
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{ g.events[0].host }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{ formatTimeRange(g.startTime,g.endTime) }}</span>
              <a-button size="small" type="link" class="ac-view" @click.stop="emit('viewEvent',g.events[0])"><EyeOutlined /></a-button>
            </div>
            <div v-if="g.events.length>1 && (g.events[0]?.type==='sse_message' || g.contentBlocks.length>0)" class="ac-merged"><MergeCellsOutlined /> {{ g.events.length }} SSE → {{ g.contentBlocks.length }} block{{g.contentBlocks.length!==1?'s':''}}</div>
            <!-- body -->
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta"><span class="ac-k">ID</span><code>{{ g.messageId }}</code></div>
              <div v-if="g.usage && usageEntries(g.usage).length" class="ac-tokens">
                <span v-for="e in usageEntries(g.usage)" :key="e.key" class="ac-tok" :class="{ 'ac-tok-cache': e.key.includes('cache'), 'ac-tok-out': e.key.includes('output') }">
                  <span class="ac-tok-label">{{ e.label }}</span>
                  <span class="ac-tok-val">{{ e.value.toLocaleString() }}</span>
                </span>
              </div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <template v-for="(b,bi) in g.contentBlocks" :key="bi">
                  <div v-if="blockVisible(b)" class="ac-block" :class="`ac-b-${b.type}`">
                    <div class="ac-b-head" @click.stop="toggleBlock(blockId(g.id, bi))" style="cursor:pointer">
                      <span class="ac-expand-icon"><CaretDownOutlined v-if="blockExpanded.has(blockId(g.id,bi))" /><CaretRightOutlined v-else /></span>
                      <component :is="blockIcon(b.type)" class="ac-b-icon" /><a-tag :color="blockColor(b.type)" size="small">{{ blockLabel(b.type) }}</a-tag><span v-if="b.toolName" class="ac-tn">{{ b.toolName }}</span><span v-if="b.toolId" class="ac-tid">{{ b.toolId }}</span>
                      <span v-if="blockTokens(g).input" class="ac-b-tok ac-b-tok-in" title="Input tokens">{{ blockTokens(g).input.toLocaleString() }}&thinsp;in</span>
                      <span v-if="blockTokens(g).output" class="ac-b-tok ac-b-tok-out" title="Output tokens">{{ blockTokens(g).output.toLocaleString() }}&thinsp;out</span>
                      <span class="ac-bsz">{{ formatBytes(b.mergedText.length) }}</span>
                    </div>
                    <div v-if="blockExpanded.has(blockId(g.id,bi))" class="ac-b-body"><pre>{{ blockDisplayText(b) }}</pre></div>
                  </div>
                </template>
              </div>
              <div v-else-if="g.rawMerged && showRawFallback(g)" class="ac-b-body"><pre>{{ g.rawMerged }}</pre></div>
            </div>
          </div>
        </div>
      </div>

      <!-- DOWNSTREAM -->
      <div class="ac-col ac-recv-col">
        <div class="ac-col-header"><ArrowDownOutlined style="color:#06b6d4" /><span>Downstream (Server → Agent)</span></div>
        <a-empty v-if="filteredRecvGroups.length===0" description="No downstream data" style="padding:24px" />
        <div v-else class="ac-list">
          <div v-for="g in filteredRecvGroups" :key="g.id" class="ac-card" :class="{'ac-sse':g.events[0]?.type==='sse_message' || g.contentBlocks.length>1 || g.contentBlocks.some(b=>b.type==='thinking'||b.type==='tool_use')}">
            <div class="ac-head" @click="toggle(g.id)">
              <span class="ac-h-icon"><CaretDownOutlined v-if="expanded.has(g.id)" /><CaretRightOutlined v-else /></span>
              <a-tag :color="blockColor(g.contentBlocks[0]?.type||'raw')" size="small">{{ firstTypeLabel(g) }}</a-tag>
              <span class="ac-h-meta">
                <span v-if="g.messageRole" class="ac-role">{{ g.messageRole }}</span>
                <span v-if="g.messageModel" class="ac-model">{{ g.messageModel }}</span>
                <span v-if="usageEntries(g.usage||{}).length" class="ac-tok-inline">{{ usageEntries(g.usage!).reduce((s,e) => s + e.value, 0).toLocaleString() }} tok</span>
                <span v-if="g.contentBlocks.length>1" class="ac-cbc">{{ g.contentBlocks.length }} blocks</span>
              </span>
              <span class="ac-h-host" v-if="g.events[0]?.host">{{ g.events[0].host }}</span>
              <span class="ac-h-size">{{ formatBytes(g.totalSize) }}</span>
              <span class="ac-h-time">{{ formatTimeRange(g.startTime,g.endTime) }}</span>
              <a-button size="small" type="link" class="ac-view" @click.stop="emit('viewEvent',g.events[0])"><EyeOutlined /></a-button>
            </div>
            <div v-if="g.events.length>1 && (g.events[0]?.type==='sse_message' || g.contentBlocks.length>0)" class="ac-merged"><MergeCellsOutlined /> {{ g.events.length }} SSE → {{ g.contentBlocks.length }} block{{g.contentBlocks.length!==1?'s':''}}</div>
            <div v-if="expanded.has(g.id)" class="ac-body">
              <div v-if="g.messageId" class="ac-meta"><span class="ac-k">ID</span><code>{{ g.messageId }}</code></div>
              <div v-if="g.usage && usageEntries(g.usage).length" class="ac-tokens">
                <span v-for="e in usageEntries(g.usage)" :key="e.key" class="ac-tok" :class="{ 'ac-tok-cache': e.key.includes('cache'), 'ac-tok-out': e.key.includes('output') }">
                  <span class="ac-tok-label">{{ e.label }}</span>
                  <span class="ac-tok-val">{{ e.value.toLocaleString() }}</span>
                </span>
              </div>
              <div v-if="g.contentBlocks.length" class="ac-blocks">
                <template v-for="(b,bi) in g.contentBlocks" :key="bi">
                  <div v-if="blockVisible(b)" class="ac-block" :class="`ac-b-${b.type}`">
                    <div class="ac-b-head" @click.stop="toggleBlock(blockId(g.id, bi))" style="cursor:pointer">
                      <span class="ac-expand-icon"><CaretDownOutlined v-if="blockExpanded.has(blockId(g.id,bi))" /><CaretRightOutlined v-else /></span>
                      <component :is="blockIcon(b.type)" class="ac-b-icon" /><a-tag :color="blockColor(b.type)" size="small">{{ blockLabel(b.type) }}</a-tag><span v-if="b.toolName" class="ac-tn">{{ b.toolName }}</span><span v-if="b.toolId" class="ac-tid">{{ b.toolId }}</span><span class="ac-bsz">{{ formatBytes(b.mergedText.length) }}</span>
                    </div>
                    <div v-if="blockExpanded.has(blockId(g.id,bi))" class="ac-b-body"><pre>{{ blockDisplayText(b) }}</pre></div>
                  </div>
                </template>
              </div>
              <div v-else-if="g.rawMerged && showRawFallback(g)" class="ac-b-body"><pre>{{ g.rawMerged }}</pre></div>
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
.ac-hide-raw-toggle{display:flex;align-items:center;gap:4px;margin-left:auto;padding-left:12px;border-left:1px solid #e2e8f0}
.ac-cap-ctl{display:flex;align-items:center;gap:4px;margin-left:auto}
.ac-cap-label{font-size:10px;color:#94a3b8;font-weight:500;white-space:nowrap}
.ac-diag-dot{font-size:8px;cursor:help;margin-left:2px}
.ac-diag-ok{color:#22c55e}.ac-diag-warn{color:#f97316}
.ac-columns{display:grid;grid-template-columns:1fr 1fr;gap:12px}
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
.ac-tok-inline{font-size:10px;color:#0891b2;font-family:ui-monospace,monospace;font-weight:600;background:#ecfeff;padding:0 4px;border-radius:3px;border:1px solid #a5f3fc}
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
.ac-tokens{display:flex;gap:10px;padding:4px 8px;background:#f0fdfa;border-radius:4px;border:1px solid #ccfbf1;margin-bottom:6px;flex-wrap:wrap}
.ac-tok{display:flex;align-items:center;gap:3px;font-size:11px}
.ac-tok-label{color:#64748b;font-size:10px}
.ac-tok-val{font-family:ui-monospace,monospace;font-weight:700;color:#0f766e;font-size:12px}
.ac-tok-cache .ac-tok-val{color:#7c3aed}
.ac-blocks{display:flex;flex-direction:column;gap:8px}
.ac-block{border:1px solid #e2e8f0;border-radius:6px;overflow:hidden}
.ac-b-text{border-color:#86efac}.ac-b-tool_use{border-color:#fdba74}.ac-b-thinking{border-color:#c4b5fd}.ac-b-request_body{border-color:#93c5fd}.ac-b-response_body{border-color:#67e8f9}
.ac-b-head{display:flex;align-items:center;gap:5px;padding:4px 8px;background:#f8fafc;border-bottom:1px solid #f0f0f0;font-size:11px}
.ac-b-icon{font-size:12px;color:#64748b}
.ac-tn{font-family:ui-monospace,monospace;font-weight:600;color:#d97706;font-size:11px}
.ac-tid{font-family:ui-monospace,monospace;font-size:9px;color:#94a3b8}
.ac-bsz{margin-left:auto;font-size:9px;color:#94a3b8}
.ac-b-tok{font-size:9px;font-family:ui-monospace,monospace;font-weight:600;padding:0 3px;border-radius:2px;white-space:nowrap}
.ac-b-tok-in{color:#0f766e;background:#f0fdfa;border:1px solid #ccfbf1}
.ac-b-tok-out{color:#0891b2;background:#ecfeff;border:1px solid #a5f3fc}
.ac-b-body pre{background:#0f172a;color:#dbeafe;padding:10px;border-radius:0;font-size:11px;line-height:1.55;margin:0;white-space:pre-wrap}
</style>
