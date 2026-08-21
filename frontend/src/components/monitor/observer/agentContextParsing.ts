// Pure parsing/grouping helpers for AgentContextPanel.
// Extracted from the component so display state stays in the SFC.
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  CodeOutlined,
  MessageOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from "@ant-design/icons-vue";
import type { ObserverTLSEvent } from "../../../composables/monitor/useProcessObserver";
export const hexToText = (hex: string): string => {
  // Robust hex decoder: strips whitespace, skips non-hex chars (don't break!)
  const clean = hex.replace(/[^0-9a-fA-F]/g, "");
  const bytes: number[] = [];
  for (let i = 0; i < clean.length - 1; i += 2) {
    const byte = parseInt(clean.slice(i, i + 2), 16);
    if (!isNaN(byte)) bytes.push(byte);
  }
  return bytes
    .map((b) =>
      (b >= 0x20 && b < 0x7f) || b === 0x0a || b === 0x0d
        ? String.fromCharCode(b)
        : ".",
    )
    .join("");
};

export const getRawText = (ev: ObserverTLSEvent): string =>
  ev.body || (ev.raw_hex_dump ? hexToText(ev.raw_hex_dump) : "");

// Strip HTTP chunked transfer encoding markers (standalone hex followed by \r\n)
export const stripChunkedEncoding = (text: string): string =>
  text.replace(/^[0-9a-fA-F]{1,8}\s*$/gm, "").replace(/\n{3,}/g, "\n\n");

export const tryParseJSON = (text: string): any | null => {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
};

export const formatJSON = (obj: any): string => {
  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
};

// ── SSE parser (handles framed SSE + bare JSON + chunked + truncated) ────

export const NOOP_TYPES = new Set(["ping", "message_stop"]);

export interface SSEParsed {
  type: string;
  index?: number;
  data: any;
  delta?: any;
  content_block?: any;
  message?: any;
  usage?: any;
}

export interface ContentBlock {
  type: string;
  mergedText: string;
  toolName?: string;
  toolId?: string;
  toolInput?: any;
  image?: {
    src?: string;
    mediaType?: string;
    approxBytes?: number;
    folded?: boolean;
    httpUrl?: string;
  };
}

export interface MergedGroup {
  id: string;
  events: ObserverTLSEvent[];
  startTime: string;
  endTime: string;
  totalSize: number;
  messageRole?: string;
  messageModel?: string;
  messageId?: string;
  contentBlocks: ContentBlock[];
  rawMerged: string;
  usage?: {
    input_tokens?: number;
    output_tokens?: number;
    cache_read_input_tokens?: number;
    cache_creation_input_tokens?: number;
  };
}

// Depth-aware split of concatenated JSON: {a}{b} → [{a}, {b}]
export const splitConcatenatedJSON = (text: string): any[] => {
  const results: any[] = [];
  let depth = 0,
    start = -1,
    inString = false,
    escaped = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (ch === "\\") {
      escaped = true;
      continue;
    }
    if (ch === '"') {
      inString = !inString;
      continue;
    }
    if (inString) continue;
    if (ch === "{") {
      if (depth === 0) start = i;
      depth++;
    } else if (ch === "}") {
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
export const extractPartialJSON = (text: string): string | null => {
  const m = text.match(/"partial_json"\s*:\s*"((?:[^"\\]|\\.)*)"/);
  return m ? JSON.parse(`"${m[1]}"`) : null; // unescape via JSON.parse
};

// Parse one SSE event's data field into SSEParsed objects
export const parseOneSSEData = (evType: string, data: string): SSEParsed[] => {
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
    return [
      {
        type: evType || "content_block_delta",
        data: {},
        delta: { type: "input_json_delta", partial_json: pj },
      },
    ];
  }
  return [];
};

// Main parser: handles both SSE-framed text and bare concatenated JSON
export const parseRawSSE = (text: string): SSEParsed[] => {
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
        type: obj.type || "",
        index: obj.index,
        data: obj,
        delta: obj.delta,
        content_block: obj.content_block,
        message: obj.message,
        usage: obj.usage || obj.message?.usage,
      });
    }
    return result;
  }

  return result;
};

// ── Content block ────────────────────────────────────────────────────────

export const IMAGE_FOLDED_PREFIX = "__IMAGE_FOLDED__:";
export const describeAgentImageItem = (
  item: any,
): {
  mediaType?: string;
  approxBytes?: number;
  src?: string;
  httpUrl?: string;
  folded?: boolean;
} | null => {
  if (!item || typeof item !== "object") return null;
  const t = item.type;
  if (t === "image") {
    const src = item.source || {};
    const mediaType = src.media_type || "image";
    const data: string = src.data || "";
    if (typeof data === "string" && data.startsWith(IMAGE_FOLDED_PREFIX)) {
      const rest = data.slice(IMAGE_FOLDED_PREFIX.length);
      const [m, n] = rest.split(":");
      return { mediaType: m, approxBytes: Number(n) || 0, folded: true };
    }
    if (src.type === "base64" && data) {
      // data URI form for <img> rendering (may be truncated by capture; we still try)
      return {
        mediaType,
        approxBytes: Math.floor((data.length * 3) / 4),
        src: `data:${mediaType};base64,${data}`,
      };
    }
    return { mediaType, folded: !data };
  }
  if (t === "image_url") {
    const iu = item.image_url || {};
    const url: string = iu.url || "";
    if (!url) return { folded: true };
    if (url.startsWith(IMAGE_FOLDED_PREFIX)) {
      const rest = url.slice(IMAGE_FOLDED_PREFIX.length);
      const [m, n] = rest.split(":");
      return { mediaType: m, approxBytes: Number(n) || 0, folded: true };
    }
    if (url.startsWith("data:")) {
      const m = url.slice(5).split(",")[0]?.split(";")[0];
      return { mediaType: m, src: url, folded: false };
    }
    if (/^https?:\/\//i.test(url)) {
      return { httpUrl: url, folded: false };
    }
    return { folded: true };
  }
  return null;
};

// Push an image content block (or append to existing text block list). Returns
// the content block to push, or null when the item is not an image.
export const pushImageBlock = (blocks: ContentBlock[], item: any) => {
  const img = describeAgentImageItem(item);
  if (!img) return false;
  const approx = img.approxBytes ?? 0;
  const media = img.mediaType || "image";
  const label = img.folded
    ? `[image ${media} ${approx > 0 ? `~${formatBytesFromInt(approx)}` : "captured partially"}]`
    : `[image ${media}]`;
  blocks.push({
    type: "image",
    mergedText: label,
    image: {
      src: img.src,
      mediaType: media,
      approxBytes: approx,
      folded: img.folded,
      httpUrl: img.httpUrl,
    },
  });
  return true;
};

export const formatBytesFromInt = (n: number): string => {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1);
  return `${(n / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

export const normalizeBlockType = (t: string): string => {
  if (t === "thinking_delta" || t === "thinking") return "thinking";
  if (t === "text_delta" || t === "text") return "text";
  if (t === "input_json_delta" || t === "tool_use") return "tool_use";
  if (t === "signature_delta") return "signature";
  if (t === "citations_delta") return "citations";
  return t;
};

export const mergeContentBlocks = (
  parsedEvents: SSEParsed[],
): ContentBlock[] => {
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
        toolName: cb.name,
        toolId: cb.id,
        toolInput: cb.input,
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
          const pj =
            delta.partial_json || delta.input_json_delta?.partial_json || "";
          existing.mergedText += pj;
          existing.toolInput = tryParseJSON(existing.mergedText);
        } else if (delta.signature || delta.signature_delta) {
          /* signature — ignore */
        } else if (delta.citations || delta.citations_delta) {
          /* citations — ignore for now */
        }
      } else {
        // New block — use normalized type from delta (skip signature/citations as no-ops)
        const deltaType = delta.type || "text";
        if (deltaType === "signature_delta" || deltaType === "citations_delta")
          continue;
        const text =
          delta.text ||
          delta.thinking ||
          delta.partial_json ||
          delta.input_json_delta?.partial_json ||
          "";
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
      else
        blocks.set(key, {
          type: "text",
          mergedText: p.data.choices[0].delta.content,
        });
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
export const extractMessageMeta = (
  events: SSEParsed[],
): { role?: string; model?: string; id?: string; usage?: any } => {
  let meta: {
    role?: string;
    model?: string;
    id?: string;
    usage?: Record<string, number>;
  } = {};
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
export const usageEntries = (
  u: any,
): { key: string; label: string; value: number }[] => {
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
export const looksLikeSSEJSON = (text: string): boolean => {
  const t = text.trim();
  return (
    t.startsWith('{"type":"') &&
    /"(message_start|content_block|ping|message_delta|message_stop)"/.test(
      t.slice(0, 100),
    )
  );
};

// ── Build groups ─────────────────────────────────────────────────────────

let _gid = 0;
export const nextId = () => `g${_gid++}`;

export const buildGroups = (
  events: ObserverTLSEvent[],
  dir: "send" | "recv",
): MergedGroup[] => {
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
                  const resultText =
                    typeof item.content === "string"
                      ? item.content
                      : Array.isArray(item.content)
                        ? item.content
                            .map((c: any) => c.text || JSON.stringify(c))
                            .join("\n")
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
        const meta = [`${ev.method || "GET"} ${ev.url || "/"}`];
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
        id: nextId(),
        events: [ev],
        startTime: ev.timestamp,
        endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: blocks,
        rawMerged: raw,
      });
      i++;
      continue;
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
                    const resultText =
                      typeof item.content === "string"
                        ? item.content
                        : Array.isArray(item.content)
                          ? item.content
                              .map((c: any) => c.text || JSON.stringify(c))
                              .join("\n")
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
        } else if (ev.body) {
          blocks.push({ type: "request_body", mergedText: ev.body });
        } else if (raw) {
          blocks.push({
            type: "raw",
            mergedText: raw.includes("HTTP/2")
              ? raw
              : "[HTTP/2 binary — see hex]",
          });
        }
      } else if (raw) {
        blocks.push({
          type: "raw",
          mergedText:
            "[HTTP/2 binary — see hex]\n\nRaw hex decode:\n" +
            raw.slice(0, 500),
        });
      }
      groups.push({
        id: nextId(),
        events: [ev],
        startTime: ev.timestamp,
        endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: blocks,
        rawMerged: raw,
      });
      i++;
      continue;
    }

    // HTTP response — individual
    if (ev.type === "http_response") {
      const raw = getRawText(ev);
      groups.push({
        id: nextId(),
        events: [ev],
        startTime: ev.timestamp,
        endTime: ev.timestamp,
        totalSize: ev.body_size || ev.captured_len,
        contentBlocks: raw ? [{ type: "response_body", mergedText: raw }] : [],
        rawMerged: raw,
      });
      i++;
      continue;
    }

    // DOWNSTREAM only: SSE stream merging (SSE is server→client protocol)
    if (
      dir === "recv" &&
      (ev.type === "sse_message" ||
        (ev.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(ev))))
    ) {
      const batch: ObserverTLSEvent[] = [ev];
      let j = i + 1;
      while (j < events.length) {
        const next = events[j];
        if (
          next.type === "sse_message" ||
          (next.type === "tls_plaintext" && looksLikeSSEJSON(getRawText(next)))
        ) {
          batch.push(next);
          j++;
        } else break;
      }
      const allParsed: SSEParsed[] = [];
      for (const be of batch) allParsed.push(...parseRawSSE(getRawText(be)));
      const meta = extractMessageMeta(allParsed);
      groups.push({
        id: nextId(),
        events: batch,
        startTime: batch[0].timestamp,
        endTime: batch[batch.length - 1].timestamp,
        totalSize: batch.reduce(
          (s, e) => s + (e.body_size || e.captured_len),
          0,
        ),
        messageRole: meta.role,
        messageModel: meta.model,
        messageId: meta.id,
        usage: meta.usage,
        contentBlocks:
          allParsed.length > 0 ? mergeContentBlocks(allParsed) : [],
        rawMerged: allParsed.length > 0 ? "" : batch.map(getRawText).join("\n"),
      });
      i = j;
      continue;
    }

    // Raw — group consecutive non-SSE raw events
    const batch: ObserverTLSEvent[] = [ev];
    let k = i + 1;
    while (k < events.length && events[k].type === "tls_plaintext") {
      // For upstream: merge all consecutive raw (may be fragmented HTTP body)
      // For downstream: only merge if NOT SSE-looking
      if (dir === "recv" && looksLikeSSEJSON(getRawText(events[k]))) break;
      batch.push(events[k]);
      k++;
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
                  const resultText =
                    typeof item.content === "string"
                      ? item.content
                      : Array.isArray(item.content)
                        ? item.content
                            .map((c: any) => c.text || JSON.stringify(c))
                            .join("\n")
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
      id: nextId(),
      events: batch,
      startTime: batch[0].timestamp,
      endTime: batch[batch.length - 1].timestamp,
      totalSize: batch.reduce((s, e) => s + (e.body_size || e.captured_len), 0),
      contentBlocks: blocks,
      rawMerged: rawBody,
    });
    i = k;
  }
  return groups;
};

// ── Display helpers ──────────────────────────────────────────────────────

export const blockDisplayText = (b: ContentBlock): string => {
  if (
    b.toolInput &&
    typeof b.toolInput === "object" &&
    Object.keys(b.toolInput).length > 0
  ) {
    try {
      return JSON.stringify(b.toolInput, null, 2);
    } catch {
      /* fall through */
    }
  }
  const parsed = tryParseJSON(b.mergedText);
  if (parsed && typeof parsed === "object" && Object.keys(parsed).length > 0) {
    try {
      return JSON.stringify(parsed, null, 2);
    } catch {
      /* fall through */
    }
  }
  return b.mergedText;
};

export const formatBytes = (bytes: number): string => {
  if (!bytes) return "0 B";
  const u = ["B", "KB", "MB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    u.length - 1,
  );
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};
export const formatTime = (ts: string): string => {
  try {
    return new Date(ts).toLocaleTimeString();
  } catch {
    return ts?.slice(11, 19) || "";
  }
};
export const formatTimeRange = (s: string, e: string): string =>
  s === e ? formatTime(s) : `${formatTime(s)} → ${formatTime(e)}`;

export const formatTokens = (u: any): { input: number; output: number } => {
  if (!u || typeof u !== "object") return { input: 0, output: 0 };
  const input =
    (typeof u.input_tokens === "number" ? u.input_tokens : 0) +
    (typeof u.cache_read_input_tokens === "number"
      ? u.cache_read_input_tokens
      : 0) +
    (typeof u.cache_creation_input_tokens === "number"
      ? u.cache_creation_input_tokens
      : 0);
  const output = typeof u.output_tokens === "number" ? u.output_tokens : 0;
  return { input, output };
};

export const blockTokens = (
  g: MergedGroup,
): { input: number; output: number } => formatTokens(g.usage);

export const blockLabel = (t: string) => {
  const m: Record<string, string> = {
    text: "Text",
    tool_use: "Tool Use",
    tool_result: "Tool Result",
    thinking: "Thinking",
    signature: "Signature",
    citations: "Citations",
    request_body: "Request Body",
    response_body: "Response Body",
    raw: "Raw Data",
    data: "SSE Data",
    http2_frame: "HTTP/2 Frame",
    http2_preface: "HTTP/2 Preface",
  };
  return m[t] || t;
};

export const firstTypeLabel = (g: MergedGroup): string => {
  const ev = g.events[0];
  if (!ev) return "?";
  if (ev.type === "http_request") return ev.method || "REQ";
  if (ev.type === "http_response") return `HTTP ${ev.status || ""}`;
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

export const blockColor = (t: string) => {
  const m: Record<string, string> = {
    text: "green",
    tool_use: "orange",
    tool_result: "volcano",
    thinking: "purple",
    signature: "geekblue",
    citations: "cyan",
    request_body: "blue",
    response_body: "cyan",
  };
  return m[t] || "default";
};

export const blockIcon = (t: string) => {
  if (t === "text") return MessageOutlined;
  if (t === "tool_use") return ToolOutlined;
  if (t === "thinking") return ThunderboltOutlined;
  if (t === "request_body") return ArrowUpOutlined;
  if (t === "response_body") return ArrowDownOutlined;
  return CodeOutlined;
};
