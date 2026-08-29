import { safeJsonParse } from "./shared";
import type {
  AgentSightStdioProtocol,
  DecodedStdioMessage,
} from "./types";

const utf8Encoder = new TextEncoder();
const utf8Decoder = new TextDecoder("utf-8");

export interface ContentLengthDecodeResult {
  framed: boolean;
  payloads: string[];
  incomplete: boolean;
  // Number of UTF-8 wire bytes that belong to complete frames (and optional
  // leading separators). Stateful stream decoding can retain only the suffix
  // beginning here instead of replaying already-emitted frames.
  consumedBytes: number;
  error?: string;
}

function truncateText(value: string, limit = 96) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized.length <= limit
    ? normalized
    : `${normalized.slice(0, limit - 3)}...`;
}

function stringifyId(value: unknown) {
  if (value === null || value === undefined) return undefined;
  return String(value);
}

function findHeaderTerminator(bytes: Uint8Array, start: number) {
  for (let index = start; index < bytes.length - 1; index++) {
    if (
      index + 3 < bytes.length &&
      bytes[index] === 13 &&
      bytes[index + 1] === 10 &&
      bytes[index + 2] === 13 &&
      bytes[index + 3] === 10
    ) {
      return { index, length: 4 };
    }
    if (bytes[index] === 10 && bytes[index + 1] === 10) {
      return { index, length: 2 };
    }
  }
  return null;
}

function parseContentLength(header: string) {
  for (const rawLine of header.split(/\r?\n/)) {
    const separator = rawLine.indexOf(":");
    if (separator < 0) continue;
    const name = rawLine.slice(0, separator).trim().toLowerCase();
    if (name !== "content-length") continue;
    const value = rawLine.slice(separator + 1).trim();
    if (!/^\d+$/.test(value)) return { value: 0, valid: false };
    const parsed = Number(value);
    return {
      value: parsed,
      valid: Number.isSafeInteger(parsed) && parsed >= 0,
    };
  }
  return null;
}

function looksLikeContentLengthPrefix(bytes: Uint8Array, start = 0) {
  const sample = utf8Decoder
    .decode(bytes.slice(start, Math.min(bytes.length, start + 64)))
    .trimStart()
    .toLowerCase();
  return sample.startsWith("content-length");
}

// LSP, MCP-over-stdio and DAP-style transports commonly use byte-counted
// Content-Length framing. Parse it against UTF-8 bytes rather than JavaScript
// string length so CJK/emoji payloads do not shift the next frame boundary.
export function decodeContentLengthFrames(
  rawPayload: string,
): ContentLengthDecodeResult {
  const bytes = utf8Encoder.encode(rawPayload);
  const payloads: string[] = [];
  let cursor = 0;
  let framed = false;
  let incomplete = false;
  let error: string | undefined;

  while (cursor < bytes.length) {
    while (
      cursor < bytes.length &&
      (bytes[cursor] === 13 || bytes[cursor] === 10)
    ) {
      cursor++;
    }
    if (cursor >= bytes.length) break;

    // Keep cursor at the beginning of the current frame until its payload is
    // complete. When a capture event ends mid-header or mid-payload this is
    // exactly the suffix the incremental decoder must retain.
    const frameStart = cursor;
    const terminator = findHeaderTerminator(bytes, cursor);
    if (!terminator) {
      if (framed || looksLikeContentLengthPrefix(bytes, cursor)) {
        framed = true;
        incomplete = true;
        cursor = frameStart;
      }
      break;
    }

    const header = utf8Decoder.decode(bytes.slice(cursor, terminator.index));
    const contentLength = parseContentLength(header);
    if (!contentLength) {
      if (!framed && payloads.length === 0) {
        return {
          framed: false,
          payloads: [],
          incomplete: false,
          consumedBytes: 0,
        };
      }
      error = "framed stdio header is missing Content-Length";
      cursor = frameStart;
      break;
    }
    framed = true;
    if (!contentLength.valid) {
      error = "invalid Content-Length value";
      cursor = frameStart;
      break;
    }

    const payloadStart = terminator.index + terminator.length;
    const payloadEnd = payloadStart + contentLength.value;
    if (payloadEnd > bytes.length) {
      incomplete = true;
      cursor = frameStart;
      break;
    }
    payloads.push(utf8Decoder.decode(bytes.slice(payloadStart, payloadEnd)));
    cursor = payloadEnd;
  }

  return {
    framed,
    payloads,
    incomplete,
    consumedBytes: framed && !incomplete && !error ? bytes.length : cursor,
    error,
  };
}

function extractToolName(parsedPayload: any) {
  const toolName = parsedPayload?.params?.name;
  return typeof toolName === "string" && toolName.length > 0
    ? toolName
    : undefined;
}

function extractStdioPreview(
  parsedPayload: any,
  kind: DecodedStdioMessage["kind"],
) {
  if (!parsedPayload || typeof parsedPayload !== "object") return undefined;
  if (kind === "request" || kind === "notification") {
    const args = parsedPayload.params?.arguments;
    if (typeof args?.text === "string" && args.text.length > 0)
      return truncateText(args.text);
    const text = parsedPayload.params?.textDocument?.text;
    if (typeof text === "string" && text.length > 0) return truncateText(text);
    const method = parsedPayload.method;
    if (
      method === "tools/call" &&
      typeof parsedPayload.params?.name === "string"
    )
      return parsedPayload.params.name;
    if (method === "tools/list") return "list tools";
    if (method === "initialize")
      return parsedPayload.params?.clientInfo?.name || "initialize";
  }
  if (kind === "response") {
    const content = parsedPayload.result?.content;
    if (Array.isArray(content) && typeof content[0]?.text === "string")
      return truncateText(content[0].text);
    if (Array.isArray(parsedPayload.result?.tools))
      return `${parsedPayload.result.tools.length} tools`;
    if (typeof parsedPayload.result?.protocolVersion === "string")
      return parsedPayload.result.protocolVersion;
  }
  if (kind === "error" && typeof parsedPayload.error?.message === "string")
    return truncateText(parsedPayload.error.message);
  return undefined;
}

function isLspMessage(message: any) {
  if (!message || typeof message !== "object") return false;
  const method = typeof message.method === "string" ? message.method : "";
  if (
    ["initialized", "shutdown", "exit"].includes(method) ||
    [
      "textDocument/",
      "notebookDocument/",
      "workspace/",
      "window/",
      "client/",
      "telemetry/",
      "$/",
    ].some((prefix) => method.startsWith(prefix))
  ) {
    return true;
  }
  if (method === "initialize") {
    const params = message.params;
    return Boolean(
      params &&
        (params.processId !== undefined ||
          params.rootUri !== undefined ||
          params.rootPath !== undefined ||
          params.workspaceFolders !== undefined),
    );
  }
  return false;
}

function isMcpMessage(message: any) {
  if (!message || typeof message !== "object") return false;
  const method = typeof message.method === "string" ? message.method : "";
  if (
    [
      "tools/",
      "resources/",
      "prompts/",
      "sampling/",
      "roots/",
      "completion/",
      "logging/",
      "notifications/",
    ].some((prefix) => method.startsWith(prefix))
  ) {
    return true;
  }
  if (method === "initialize") {
    return typeof message.params?.protocolVersion === "string";
  }
  return typeof message.result?.protocolVersion === "string";
}

function classifyStdioProtocol(
  parsedMessages: any[],
  rawPayload: string,
): AgentSightStdioProtocol {
  if (parsedMessages.some(isLspMessage)) return "lsp";
  if (parsedMessages.some(isMcpMessage)) return "mcp";
  if (
    parsedMessages.some(
      (message) =>
        message &&
        typeof message === "object" &&
        (message.jsonrpc === "2.0" ||
          message.method !== undefined ||
          message.result !== undefined ||
          message.error !== undefined),
    )
  ) {
    return "jsonrpc";
  }
  return rawPayload.trim() ? "text" : "unknown";
}

function protocolLabel(protocol: AgentSightStdioProtocol) {
  switch (protocol) {
    case "lsp":
      return "LSP";
    case "mcp":
      return "MCP";
    case "jsonrpc":
      return "JSON-RPC";
    default:
      return "";
  }
}

export function isStdioSource(source: string) {
  return (
    String(source || "")
      .toLowerCase()
      .trim() === "stdio"
  );
}

export function decodeStdioMessage(data: any): DecodedStdioMessage {
  const rawPayload =
    typeof data?.data === "string"
      ? data.data
      : typeof data?.payload === "string"
        ? data.payload
        : "";
  const framing = decodeContentLengthFrames(rawPayload);
  const payloadTexts = framing.framed ? framing.payloads : [rawPayload];
  const parsedMessages = payloadTexts
    .map((payload) => safeJsonParse(payload))
    .filter((value) => value !== null && typeof value === "object");
  const parsedPayload = parsedMessages[0] ?? null;
  let framingError = framing.error;
  if (framing.framed && framing.payloads.length > parsedMessages.length) {
    framingError ||= "one or more framed payloads contain invalid JSON";
  }

  const protocol = classifyStdioProtocol(parsedMessages, rawPayload);
  const direction = String(data?.direction || data?.stream || "").toUpperCase();
  const fdRole = String(
    data?.fd_role ||
      data?.fdRole ||
      data?.stream ||
      (data?.fd !== undefined ? `fd ${data.fd}` : "stdio"),
  );
  const fdTarget = String(data?.fd_target || data?.fdTarget || "");
  const fd = typeof data?.fd === "number" ? data.fd : null;
  const measuredLength = utf8Encoder.encode(rawPayload).byteLength;
  const length = Number(data?.len || data?.size || measuredLength);
  const truncated = Boolean(data?.truncated);
  let kind: DecodedStdioMessage["kind"] = rawPayload.trim()
    ? "text"
    : "unknown";
  let method: string | undefined;
  let id: string | undefined;
  if (parsedPayload && typeof parsedPayload === "object") {
    method =
      typeof parsedPayload.method === "string"
        ? parsedPayload.method
        : undefined;
    id = stringifyId(parsedPayload.id);
    if (method) kind = id ? "request" : "notification";
    else if (parsedPayload.result !== undefined) kind = "response";
    else if (parsedPayload.error !== undefined) kind = "error";
    else kind = "unknown";
  }
  const toolName = extractToolName(parsedPayload);
  const preview = parsedPayload
    ? extractStdioPreview(parsedPayload, kind)
    : framing.framed && framing.incomplete
      ? "incomplete framed message"
      : truncateText(rawPayload);
  const label = protocolLabel(protocol);
  const protocolPrefix = label ? `${label} ` : "";
  const title =
    kind === "request"
      ? method === "tools/call" && toolName
        ? `${protocolPrefix}tools/call ${toolName}`
        : method
          ? `${protocolPrefix}${method}`
          : `${protocolPrefix}stdio request`
      : kind === "notification"
        ? method
          ? `${protocolPrefix}${method}`
          : `${protocolPrefix}stdio notification`
        : kind === "response"
          ? id
            ? `${protocolPrefix}response #${id}`
            : `${protocolPrefix}stdio response`
          : kind === "error"
            ? id
              ? `${protocolPrefix}error #${id}`
              : `${protocolPrefix}stdio error`
            : framing.framed && framing.incomplete
              ? `${protocolPrefix}partial frame`
              : kind === "text"
                ? "stdio text"
                : "stdio event";
  const directionLabel = direction || "STDIO";
  const role = fdRole || "fd";
  const protocolSummary = label ? ` ${label}` : "";
  let summary = `${directionLabel} ${role}${protocolSummary}`;
  if (kind === "request" || kind === "notification") {
    summary = `${directionLabel} ${role}${protocolSummary} ${method || "message"}${toolName && method === "tools/call" ? ` ${toolName}` : ""}${preview && preview !== toolName ? ` · ${preview}` : ""}`;
  } else if (kind === "response" || kind === "error") {
    summary = `${directionLabel} ${role}${protocolSummary} ${kind}${id ? ` #${id}` : ""}${preview ? ` · ${preview}` : ""}`;
  } else if (preview) {
    summary = `${directionLabel} ${role}${protocolSummary} · ${preview}`;
  }
  if (framing.payloads.length > 1) {
    summary += ` · ${framing.payloads.length} frames`;
  } else if (framing.incomplete) {
    summary += " · partial";
  }
  if (framingError) {
    summary += " · framing error";
  }

  return {
    direction,
    fdRole,
    fdTarget,
    fd,
    length,
    truncated,
    rawPayload,
    parsedPayload,
    parsedMessages,
    protocol,
    framed: framing.framed,
    frameCount: framing.payloads.length,
    incompleteFrame: framing.incomplete,
    framingError,
    kind,
    method,
    id,
    toolName,
    preview,
    title,
    summary,
  };
}

export function formatStdioExpandedContent(decoded: DecodedStdioMessage) {
  const sections = [
    `Direction: ${decoded.direction || "UNKNOWN"}`,
    `FD Role: ${decoded.fdRole || "unknown"}`,
    `Protocol: ${decoded.protocol}`,
    `Framing: ${decoded.framed ? "Content-Length" : "unframed"}`,
    `Frames: ${decoded.frameCount}`,
  ];
  if (decoded.fd !== null) sections.push(`FD: ${decoded.fd}`);
  if (decoded.fdTarget) sections.push(`Target: ${decoded.fdTarget}`);
  sections.push(`Kind: ${decoded.kind}`);
  if (decoded.method) sections.push(`Method: ${decoded.method}`);
  if (decoded.toolName) sections.push(`Tool: ${decoded.toolName}`);
  if (decoded.id) sections.push(`Message ID: ${decoded.id}`);
  sections.push(`Length: ${decoded.length}`);
  sections.push(`Truncated: ${decoded.truncated ? "yes" : "no"}`);
  if (decoded.reassembled) {
    sections.push(`Reassembled: yes (${decoded.reassembledBytes || 0} bytes)`);
  }
  if (decoded.pendingBytes) sections.push(`Pending bytes: ${decoded.pendingBytes}`);
  if (decoded.reassemblyReset) sections.push(`Reassembly reset: ${decoded.reassemblyReset}`);
  if (decoded.incompleteFrame) sections.push("Incomplete frame: yes");
  if (decoded.framingError) sections.push(`Framing error: ${decoded.framingError}`);

  const payload =
    decoded.parsedMessages.length > 1
      ? JSON.stringify(decoded.parsedMessages, null, 2)
      : decoded.parsedPayload !== null
        ? JSON.stringify(decoded.parsedPayload, null, 2)
        : decoded.rawPayload;
  return `${sections.join("\n")}\n\nPayload\n-------\n${payload}`;
}
