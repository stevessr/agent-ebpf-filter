import { collectTextSlots, DEFAULT_LABELS, maskDeterministic, redactText } from "./redaction_core.js";

function report(ctx, code, bytes) {
  if (typeof ctx?.report === "function") ctx.report(code, bytes);
}

function localEndpoint(value) {
  const u = new URL(value ?? "http://127.0.0.1:8765");
  if (u.protocol !== "http:" || !["127.0.0.1", "[::1]", "localhost"].includes(u.hostname) ||
      u.username || u.password || u.search || u.hash || (u.pathname !== "/" && u.pathname !== "")) {
    throw new Error("Guard endpoint must be loopback HTTP without credentials, query or path");
  }
  return new URL("/scan", u);
}

// Runs in Bun before gateway routing; the synchronous companion middleware
// supplies regex coverage even when this best-effort async hook is skipped.
export async function onRequest(body, ctx = {}) {
  const options = ctx.options ?? {};
  const action = ["audit", "redact"].includes(options.action) ? options.action : "redact";
  const labels = Array.isArray(options.labels) ? options.labels : DEFAULT_LABELS;
  const candidate = JSON.parse(JSON.stringify(body));
  const { slots, overLimit } = collectTextSlots(candidate, options);
  const bytes = slots.reduce((n, s) => n + new TextEncoder().encode(s.text).length, 0);
  if (overLimit) { report(ctx, "invalid", bytes); return; }
  if (!slots.length) { report(ctx, "empty", 0); return; }
  const regexBackup = () => {
    if (action === "redact") {
      maskDeterministic(candidate, options);
      return candidate;
    }
    return undefined;
  };
  let url;
  try { url = localEndpoint(options.serverUrl); }
  catch { report(ctx, "invalid", bytes); return regexBackup(); }
  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        ...(options.authToken ? { authorization: `Bearer ${options.authToken}` } : {}),
      },
      body: JSON.stringify({ texts: slots.map(s => s.text) }),
      signal: ctx.signal,
    });
    if (!response.ok) throw new Error(`Local scanner HTTP ${response.status}`);
    const result = await response.json();
    if (!Array.isArray(result.spans) || result.spans.length !== slots.length ||
        !result.spans.every(a => Array.isArray(a))) throw new Error("Invalid scanner response");
    let matched = 0;
    for (let i = 0; i < slots.length; i++) {
      const masked = redactText(slots[i].text, result.spans[i], labels);
      slots[i].parent[slots[i].key] = masked.text;
      matched += masked.count;
    }
    report(ctx, "ready", bytes);
    if (action === "audit") return;
    // The model may miss obvious credential patterns; apply regex backup.
    maskDeterministic(candidate, options);
    return candidate;
  } catch {
    report(ctx, "unavailable", bytes);
    return regexBackup();
  }
}
