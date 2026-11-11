// This module is dependency-free to load both in Magpie's moejs middleware
// (no Node/Bun APIs) and in Bun's async gateway hooks.
export const DEFAULT_LABELS = [
  "private_person", "private_address", "phone", "email", "account",
  "payment_card", "common_secret",
];

const TEXT_KEYS = new Set([
  "text", "content", "input", "prompt", "instructions", "system", "parts",
  "message", "messages", "contents", "output", "arguments", "tool_result",
  "tool_results", "function_call", "function_response", "result",
]);
// Never rewrite routing, tool declarations, signed blobs or metadata. Magpie
// rejects a hook that modifies tools/tool_choice, so these must be excluded.
const SKIP_KEYS = new Set([
  "tools", "tool_choice", "functions", "model", "stream", "name", "role",
  "type", "id", "call_id", "tool_call_id", "cache_control", "signature",
  "thought_signature", "mime_type", "media_type", "image_url", "file_id",
  "data", "inline_data", "schema", "parameters", "response_format",
]);

export function collectTextSlots(body, options = {}) {
  const maxTexts = Number.isInteger(options.maxTexts) ? Math.min(Math.max(options.maxTexts, 1), 256) : 64;
  const maxTextChars = Number.isInteger(options.maxTextChars) ? Math.min(Math.max(options.maxTextChars, 1), 65536) : 16384;
  const slots = [];
  let overLimit = false;
  const visit = (value, parent, key, depth, allowed) => {
    if (depth > 30 || slots.length > maxTexts) { overLimit = true; return; }
    if (typeof value === "string") {
      if (!allowed || !parent) return;
      if (value.length > maxTextChars) { overLimit = true; return; }
      slots.push({ parent, key, text: value });
      return;
    }
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) {
      for (let i = 0; i < value.length; i++) visit(value[i], value, i, depth + 1, allowed);
      return;
    }
    for (const k of Object.keys(value)) {
      if (SKIP_KEYS.has(k)) continue;
      // The root is not a free-form map: traverse only recognized content
      // containers. Within tool arguments/results, unknown keys are text.
      const nestedAllowed = allowed || TEXT_KEYS.has(k);
      if (!nestedAllowed) continue;
      visit(value[k], value, k, depth + 1, nestedAllowed);
    }
  };
  visit(body, null, "", 0, false);
  if (slots.length > maxTexts) overLimit = true;
  return { slots: slots.slice(0, maxTexts), overLimit };
}

// Fast deterministic backup. Deliberately conservative: the local model does
// contextual NER; these regexes only cover obvious credentials and email.
const PATTERNS = [
  { label: "common_secret", re: /\b(?:sk-[A-Za-z0-9_-]{12,}|gh[pousr]_[A-Za-z0-9_]{16,}|AKIA[A-Z0-9]{16})\b/g },
  { label: "common_secret", re: /\b(?:api[_-]?key|access[_-]?token|password|passwd|secret)\s*[:=]\s*["']?([A-Za-z0-9_./+\-=]{8,})/gi, group: 1 },
  { label: "email", re: /\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi },
];

export function regexSpans(text, labels = DEFAULT_LABELS) {
  const selected = new Set(labels);
  const spans = [];
  for (const { label, re, group } of PATTERNS) {
    if (!selected.has(label)) continue;
    re.lastIndex = 0;
    let m;
    while ((m = re.exec(text)) !== null) {
      const value = group ? m[group] : m[0];
      if (!value) continue;
      const start = m.index + (group ? m[0].lastIndexOf(value) : 0);
      spans.push({ start, end: start + value.length, label, score: 1 });
    }
  }
  return spans;
}

export function redactText(text, spans, labels = DEFAULT_LABELS) {
  const selected = new Set(labels);
  const candidates = spans.filter(s =>
    selected.has(s.label) && Number.isInteger(s.start) && Number.isInteger(s.end) &&
    s.start >= 0 && s.end > s.start && s.end <= text.length &&
    Number.isFinite(s.score) && s.score >= 0 && s.score <= 1
  ).sort((a, b) => a.start - b.start || b.end - a.end || b.score - a.score);
  const kept = [];
  let end = 0;
  for (const s of candidates) {
    if (s.start < end) continue;
    kept.push(s);
    end = s.end;
  }
  if (!kept.length) return { text, count: 0 };
  let result = "";
  let cursor = 0;
  for (const s of kept) {
    result += text.slice(cursor, s.start) + `[REDACTED:${s.label}]`;
    cursor = s.end;
  }
  return { text: result + text.slice(cursor), count: kept.length };
}

export function maskDeterministic(body, options = {}) {
  const labels = Array.isArray(options.labels) ? options.labels : DEFAULT_LABELS;
  const { slots, overLimit } = collectTextSlots(body, options);
  let matches = 0;
  for (const slot of slots) {
    const masked = redactText(slot.text, regexSpans(slot.text, labels), labels);
    slot.parent[slot.key] = masked.text;
    matches += masked.count;
  }
  return { matches, overLimit };
}
