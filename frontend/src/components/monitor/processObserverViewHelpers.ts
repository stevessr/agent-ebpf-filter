import { h } from "vue";
import type { ObserverTLSEvent, ProcessTreeNode } from "../../composables/monitor/useProcessObserver";

export const subTabKeys = [
  "selection",
  "timeline",
  "flamegraph",
  "tree",
  "network",
  "syscalls",
  "file-access",
  "resources",
  "ssl",
  "agent-context",
] as const;
export type SubTabKey = (typeof subTabKeys)[number];

export const TAB_STORAGE_KEY = "observe-active-tab";

export const getRouteParam = (param: unknown): string | undefined =>
  Array.isArray(param)
    ? String(param[0] ?? "") || undefined
    : typeof param === "string"
      ? param
      : undefined;

export const normalizeObserveTab = (tab: unknown): SubTabKey => {
  if (
    typeof tab === "string" &&
    (subTabKeys as readonly string[]).includes(tab)
  ) {
    return tab as SubTabKey;
  }
  try {
    const stored = localStorage.getItem(TAB_STORAGE_KEY) as SubTabKey | null;
    if (stored && (subTabKeys as readonly string[]).includes(stored)) {
      return stored;
    }
  } catch {
    // Ignore unavailable browser storage.
  }
  return "selection";
};

export const formatBytes = (bytes: number): string => {
  if (!bytes || bytes === 0) return "—";
  const u = ["B", "KB", "MB", "GB"];
  const i = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    u.length - 1,
  );
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

// Format body preview for table display
export const formatTLSBodyPreview = (body: string | undefined, maxLen: number = 120): string => {
  if (!body) return "";
  const trimmed = body.replace(/\s+/g, " ").trim();
  return trimmed.length > maxLen ? trimmed.slice(0, maxLen) + "…" : trimmed;
};

// Hex to text decoder for raw TLS events
export const hexToText = (hex: string, maxLen: number = 100): string => {
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

// Best available preview from any TLS event (body first, then hex decode)
export const bestTLSPreview = (ev: ObserverTLSEvent, maxLen: number = 100): string => {
  if (ev.body) return formatTLSBodyPreview(ev.body, maxLen);
  if (ev.raw_hex_dump) return hexToText(ev.raw_hex_dump, maxLen);
  return "";
};

// Check if body looks like JSON
export const looksLikeJSON = (body: string | undefined): boolean => {
  if (!body) return false;
  const t = body.trim();
  return (t.startsWith("{") || t.startsWith("[")) && t.length > 2;
};

// TLS event detail modal

export const classifySSLLib = (libName: string): { type: string; color: string; tagColor: string } => {
  const name = libName.toLowerCase();
  if (name.includes("go-crypto") || name.includes("crypto/tls")) return { type: "Go crypto/tls", color: "#00ADD8", tagColor: "cyan" };
  if (name.includes("openssl") || name.includes("libssl") || name.includes("libcrypto")) return { type: "OpenSSL", color: "#1677ff", tagColor: "blue" };
  if (name.includes("gnutls")) return { type: "GnuTLS", color: "#10b981", tagColor: "green" };
  if (name.includes("nss3") || name.includes("libnss")) return { type: "NSS", color: "#f59e0b", tagColor: "orange" };
  if (name.includes("mbedtls")) return { type: "Mbed TLS", color: "#8b5cf6", tagColor: "purple" };
  if (name.includes("wolfssl")) return { type: "WolfSSL", color: "#ef4444", tagColor: "red" };
  if (name.includes("boringssl")) return { type: "BoringSSL", color: "#6366f1", tagColor: "geekblue" };
  if (name) return { type: "SSL Library", color: "#64748b", tagColor: "default" };
  return { type: "Detected", color: "#94a3b8", tagColor: "default" };
};

export const sslAttachmentColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 65 },
  { title: "Comm", key: "comm", width: 110, ellipsis: true },
  { title: "Library Path", dataIndex: "library_name", key: "lib", ellipsis: true },
  { title: "Type", key: "libType", width: 100 },
  { title: "Binary", dataIndex: "binary_path", key: "bin", ellipsis: true },
  { title: "Status", key: "status", width: 80 },
];

// ── Table columns ────────────────────────────────────────────────────────

export const networkFlowColumns = [
  { title: "Proto", dataIndex: "protocol", key: "protocol", width: 60 },
  {
    title: "Src",
    dataIndex: "srcIp",
    key: "srcIp",
    width: 130,
    ellipsis: true,
  },
  {
    title: "Dst",
    dataIndex: "dstIp",
    key: "dstIp",
    width: 130,
    ellipsis: true,
  },
  { title: "Port", dataIndex: "dstPort", key: "dstPort", width: 65 },
  { title: "Svc", dataIndex: "dstService", key: "dstService", width: 90 },
  {
    title: "In",
    dataIndex: "bytesIn",
    key: "bytesIn",
    width: 75,
    align: "right" as const,
  },
  {
    title: "Out",
    dataIndex: "bytesOut",
    key: "bytesOut",
    width: 75,
    align: "right" as const,
  },
];

export const tcpConnColumns = [
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 100 },
  { title: "State", dataIndex: "state", key: "state", width: 80 },
  { title: "Local", key: "local", width: 150, ellipsis: true },
  { title: "Remote", key: "remote", width: 150, ellipsis: true },
];

export const eventColumns = [
  { title: "Time", dataIndex: "time", key: "time", width: 90 },
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 100, ellipsis: true },
  { title: "Type", dataIndex: "type", key: "type", width: 95 },
  { title: "Path", dataIndex: "path", key: "path", ellipsis: true },
  {
    title: "Bytes",
    dataIndex: "bytes",
    key: "bytes",
    width: 80,
    align: "right" as const,
  },
];

export const tlsColumns = [
  { title: "Time", dataIndex: "timestamp", key: "timestamp", width: 90 },
  { title: "PID", dataIndex: "pid", key: "pid", width: 60 },
  { title: "Comm", dataIndex: "comm", key: "comm", width: 90, ellipsis: true },
  { title: "Dir", dataIndex: "direction", key: "direction", width: 50 },
  { title: "Type", key: "evType", width: 95 },
  { title: "Host", dataIndex: "host", key: "host", width: 140, ellipsis: true },
  { title: "URL", dataIndex: "url", key: "url", ellipsis: true },
  { title: "Body Preview", key: "bodyPreview", ellipsis: true },
  {
    title: "Size",
    dataIndex: "captured_len",
    key: "size",
    width: 70,
    align: "right" as const,
  },
  { title: "", key: "actions", width: 36 },
];

export const createExpandedTLSRowRender = (
  openTLSDetail: (record: ObserverTLSEvent) => void,
) => (record: ObserverTLSEvent) => {
  const items: any[] = [];

  // Headers section
  if (record.headers && Object.keys(record.headers).length > 0) {
    const headerTags = Object.entries(record.headers).map(([k, v]) => {
      const isRedacted = v === "***REDACTED***";
      return h("div", { class: "tls-header-row" }, [
        h("span", { class: "tls-header-key" }, k),
        h("span", { class: isRedacted ? "tls-header-val-redacted" : "tls-header-val" }, v),
      ]);
    });
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, `Headers (${Object.keys(record.headers).length})`),
        h("div", { class: "tls-headers-box" }, headerTags),
      ]),
    );
  }

  // Agent context section
  if (record.vendor || record.agent_run_id || record.task_id || record.message_role) {
    const ctxItems: any[] = [];
    if (record.vendor) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Vendor:"), h("a-tag", { color: "geekblue", size: "small" }, () => record.vendor)]));
    if (record.message_role) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Role:"), h("code", {}, record.message_role)]));
    if (record.agent_run_id) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Run:"), h("code", {}, record.agent_run_id)]));
    if (record.task_id) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Task:"), h("code", {}, record.task_id)]));
    if (record.prompt_digest) ctxItems.push(h("div", { class: "tls-ctx-row" }, [h("span", { class: "tls-ctx-key" }, "Prompt:"), h("code", {}, record.prompt_digest.slice(0, 32) + "…")]));
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, "Agent Context"),
        ...ctxItems,
      ]),
    );
  }

  // Body section
  if (record.body) {
    const bodyContent = record.body.length > 3000
      ? record.body.slice(0, 3000) + "\n… [truncated]"
      : record.body;
    const isJSON = looksLikeJSON(record.body);
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, `Body (${record.body_size || record.body.length} bytes)${record.truncated ? ' — truncated' : ''}`),
        h("pre", { class: isJSON ? "tls-body-json" : "tls-body-text" }, bodyContent),
        h("a-button", {
          size: "small", type: "link",
          onClick: () => openTLSDetail(record),
        }, { default: () => "View Full Detail" }),
      ]),
    );
  }

  // SSE info
  if (record.sse_event || record.sse_data_count) {
    items.push(
      h("div", { class: "tls-expand-section" }, [
        h("div", { class: "tls-expand-label" }, "SSE"),
        h("div", { class: "tls-sse-info" }, [
          record.sse_event ? h("span", {}, `Event: ${record.sse_event}`) : null,
          record.sse_data_count ? h("span", { style: { marginLeft: "8px" } }, `Data parts: ${record.sse_data_count}`) : null,
        ].filter(Boolean)),
      ]),
    );
  }

  if (items.length === 0) return null;
  return h("div", { class: "tls-expand-content" }, items);
};

export const collectAllPids = (nodes: ProcessTreeNode[]): number[] =>
  nodes.flatMap((node) => [node.pid, ...collectAllPids(node.children)]);
