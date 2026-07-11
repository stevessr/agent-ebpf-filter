import { safeJsonParse } from "./shared";
import type { DecodedStdioMessage } from "./types";

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
  const parsedPayload = safeJsonParse(rawPayload);
  const direction = String(data?.direction || data?.stream || "").toUpperCase();
  const fdRole = String(
    data?.fd_role ||
      data?.fdRole ||
      data?.stream ||
      (data?.fd !== undefined ? `fd ${data.fd}` : "stdio"),
  );
  const fdTarget = String(data?.fd_target || data?.fdTarget || "");
  const fd = typeof data?.fd === "number" ? data.fd : null;
  const length = Number(data?.len || data?.size || 0);
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
    : truncateText(rawPayload);
  const title =
    kind === "request"
      ? method === "tools/call" && toolName
        ? `tools/call ${toolName}`
        : method
          ? `request ${method}`
          : "stdio request"
      : kind === "notification"
        ? method
          ? `notification ${method}`
          : "stdio notification"
        : kind === "response"
          ? id
            ? `response #${id}`
            : "stdio response"
          : kind === "error"
            ? id
              ? `error #${id}`
              : "stdio error"
            : kind === "text"
              ? "stdio text"
              : "stdio event";
  const directionLabel = direction || "STDIO";
  const role = fdRole || "fd";
  let summary = `${directionLabel} ${role}`;
  if (kind === "request" || kind === "notification") {
    summary = `${directionLabel} ${role} ${method || "message"}${toolName && method === "tools/call" ? ` ${toolName}` : ""}${preview && preview !== toolName ? ` · ${preview}` : ""}`;
  } else if (kind === "response" || kind === "error") {
    summary = `${directionLabel} ${role} ${kind}${id ? ` #${id}` : ""}${preview ? ` · ${preview}` : ""}`;
  } else if (preview) {
    summary = `${directionLabel} ${role} · ${preview}`;
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
  ];
  if (decoded.fd !== null) sections.push(`FD: ${decoded.fd}`);
  if (decoded.fdTarget) sections.push(`Target: ${decoded.fdTarget}`);
  sections.push(`Kind: ${decoded.kind}`);
  if (decoded.method) sections.push(`Method: ${decoded.method}`);
  if (decoded.toolName) sections.push(`Tool: ${decoded.toolName}`);
  if (decoded.id) sections.push(`Message ID: ${decoded.id}`);
  sections.push(`Length: ${decoded.length}`);
  sections.push(`Truncated: ${decoded.truncated ? "yes" : "no"}`);
  const payload =
    decoded.parsedPayload !== null
      ? JSON.stringify(decoded.parsedPayload, null, 2)
      : decoded.rawPayload;
  return `${sections.join("\n")}\n\nPayload\n-------\n${payload}`;
}
