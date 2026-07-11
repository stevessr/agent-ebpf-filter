import type { MergedTransaction, TLSPlaintextEvent } from "../../types/tls";

/* ──── Type filter chips ──── */
export const typeFilters = [
  { label: "All", value: "all" },
  { label: "Fetch/XHR", value: "xhr" },
  { label: "SSE", value: "sse" },
  { label: "JSON", value: "json" },
  { label: "LLM", value: "llm" },
];

/* ──── Helpers ──── */
export const isRequestEvent = (e: TLSPlaintextEvent) => e.type === "http_request";
export const isResponseEvent = (e: TLSPlaintextEvent) =>
  e.type === "http_response" || e.type === "sse_message";

export const extractPathname = (url?: string): string => {
  if (!url) return "/";
  try {
    // Handle both absolute and relative URLs
    if (url.startsWith("http://") || url.startsWith("https://")) {
      const u = new URL(url);
      const path = u.pathname + u.search;
      return path || "/";
    }
    return url;
  } catch {
    return url;
  }
};

export const buildFullUrl = (event: TLSPlaintextEvent): string => {
  if (!event.url) return "";
  if (
    event.url.startsWith("http://") ||
    event.url.startsWith("https://")
  )
    return event.url;
  if (event.host) return `https://${event.host}${event.url}`;
  return event.url;
};

/** Build once per transaction update instead of normalizing large bodies per keystroke. */
export const buildTransactionSearchText = (tx: MergedTransaction): string =>
  [
    tx.name,
    tx.fullUrl,
    tx.host,
    tx.method,
    tx.comm,
    tx.status,
    tx.request?.body,
    tx.response?.body,
    JSON.stringify(tx.request?.headers || {}),
    JSON.stringify(tx.response?.headers || {}),
  ]
    .filter((value) => value !== undefined && value !== null)
    .join("\n")
    .toLowerCase();

export const createTransactionSearchIndex = (
  transactions: readonly MergedTransaction[],
): WeakMap<MergedTransaction, string> => {
  const index = new WeakMap<MergedTransaction, string>();
  for (const tx of transactions) index.set(tx, buildTransactionSearchText(tx));
  return index;
};

export const activateOnKeyboard = (event: KeyboardEvent, action: () => void) => {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  action();
};

export const formatBytes = (bytes?: number): string => {
  const n = Number(bytes || 0);
  if (!n) return "—";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
};

export const formatTime = (ms?: number): string => {
  if (ms === undefined || ms === null) return "—";
  if (ms < 1) return "<1 ms";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
};

export const formatTimestamp = (ts?: string): string => {
  if (!ts) return "—";
  try {
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return ts;
    return d.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    });
  } catch {
    return ts;
  }
};

export const statusClass = (status?: number): string => {
  if (!status) return "status-pending";
  if (status >= 200 && status < 300) return "status-ok";
  if (status >= 300 && status < 400) return "status-redirect";
  return "status-error";
};

export const shortType = (ct?: string): string => {
  if (!ct) return "";
  const lower = ct.toLowerCase();
  if (lower.includes("json")) return "json";
  if (lower.includes("html")) return "html";
  if (lower.includes("javascript") || lower.includes("ecmascript"))
    return "js";
  if (lower.includes("css")) return "css";
  if (lower.includes("xml")) return "xml";
  if (lower.includes("text/plain")) return "text";
  if (lower.includes("form")) return "form";
  if (lower.includes("event-stream")) return "sse";
  if (lower.includes("image")) return "img";
  if (lower.includes("font")) return "font";
  return ct.split("/").pop()?.split(";")[0] || "";
};

export const getMethodColor = (method?: string): string => {
  switch ((method || "").toUpperCase()) {
    case "GET":
      return "#188038";
    case "POST":
      return "#1967d2";
    case "PUT":
      return "#e37400";
    case "PATCH":
      return "#9334e6";
    case "DELETE":
      return "#d93025";
    default:
      return "#5f6368";
  }
};

export const isJson = (body?: string): boolean => {
  if (!body) return false;
  const t = body.trim();
  return (
    (t.startsWith("{") && t.endsWith("}")) ||
    (t.startsWith("[") && t.endsWith("]"))
  );
};

export const formatBody = (body?: string): string => {
  if (!body) return "";
  if (isJson(body)) {
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      return body;
    }
  }
  return body;
};

export const truncateBody = (body?: string, maxLen = 8000): string => {
  const formatted = formatBody(body);
  if (formatted.length > maxLen) return formatted.slice(0, maxLen) + "\n\n… [truncated]";
  return formatted;
};
