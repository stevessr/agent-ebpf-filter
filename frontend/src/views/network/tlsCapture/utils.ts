import type {
  TLSCaptureRule,
  TLSIgnoreRule,
  TLSPlaintextEvent,
} from "../../../types/tls";
import { TLS_IGNORE_RULES_STORAGE_KEY } from "./constants";

export type TLSRuleListField =
  | "comms"
  | "hosts"
  | "methods"
  | "libraries"
  | "directions";

export type TLSIgnoreRuleListField =
  | "comms"
  | "hosts"
  | "urls"
  | "methods"
  | "libraries"
  | "directions"
  | "statusCodes";

export const loadTLSIgnoreRules = (): TLSIgnoreRule[] => {
  try {
    const raw = localStorage.getItem(TLS_IGNORE_RULES_STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
};

export const saveTLSIgnoreRulesToStorage = (rules: TLSIgnoreRule[]) => {
  try {
    localStorage.setItem(TLS_IGNORE_RULES_STORAGE_KEY, JSON.stringify(rules));
  } catch {
    localStorage.removeItem(TLS_IGNORE_RULES_STORAGE_KEY);
  }
};

export const formatTLSBytes = (bytes?: number) => {
  const value = Number(bytes || 0);
  if (!value) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(value) / Math.log(1024)),
    units.length - 1,
  );
  return `${(value / Math.pow(1024, index)).toFixed(1)} ${units[index]}`;
};

export const formatTLSTimestamp = (timestamp?: string) => {
  if (!timestamp) return "—";
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? timestamp : date.toLocaleString();
};

export const isTLSRequestEvent = (event: TLSPlaintextEvent) =>
  event.type === "http_request";

export const isTLSResponseEvent = (event: TLSPlaintextEvent) =>
  event.type === "http_response" || event.type === "sse_message";

export const isTLSDisplayEvent = (event: TLSPlaintextEvent) =>
  isTLSRequestEvent(event) || isTLSResponseEvent(event);

export const tlsDirectionLabel = (direction?: string) =>
  direction === "send" ? "Request" : direction === "recv" ? "Response" : "—";

export const tlsDirectionColor = (direction?: string) =>
  direction === "send" ? "green" : direction === "recv" ? "blue" : "default";

export const tlsPacketTypeLabel = (event: TLSPlaintextEvent) => {
  if (event.type === "http_request") return "HTTP Request";
  if (event.type === "http_response") return "HTTP Response";
  if (event.type === "sse_message") return "SSE Response";
  return "—";
};

export const tlsPacketTypeColor = (event: TLSPlaintextEvent) =>
  event.type === "sse_message"
    ? "cyan"
    : isTLSRequestEvent(event)
      ? "green"
      : isTLSResponseEvent(event)
        ? "blue"
        : "default";

export const splitTLSRuleValues = (value: string) =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

export const joinTLSRuleValues = (values?: string[]) =>
  (values || []).join(", ");

export const updateTLSRuleValues = (
  rule: TLSCaptureRule,
  field: TLSRuleListField,
  value: string,
) => {
  rule[field] = splitTLSRuleValues(value);
};

export const updateTLSIgnoreRuleValues = (
  rule: TLSIgnoreRule,
  field: TLSIgnoreRuleListField,
  value: string,
) => {
  rule[field] = splitTLSRuleValues(value);
};

export const matchesTLSIgnoreRule = (
  rule: TLSIgnoreRule,
  event: TLSPlaintextEvent,
): boolean => {
  if (!rule.enabled) return false;
  if (
    rule.comms?.length &&
    !rule.comms.some((comm) =>
      (event.comm || "").toLowerCase().includes(comm.toLowerCase()),
    )
  )
    return false;
  if (
    rule.hosts?.length &&
    !rule.hosts.some((host) =>
      (event.host || "").toLowerCase().includes(host.toLowerCase()),
    )
  )
    return false;
  if (
    rule.urls?.length &&
    !rule.urls.some((url) =>
      (event.url || "").toLowerCase().includes(url.toLowerCase()),
    )
  )
    return false;
  if (
    rule.methods?.length &&
    !rule.methods.some(
      (method) => (event.method || "").toLowerCase() === method.toLowerCase(),
    )
  )
    return false;
  if (
    rule.libraries?.length &&
    !rule.libraries.some(
      (library) => (event.lib || "").toLowerCase() === library.toLowerCase(),
    )
  )
    return false;
  if (
    rule.directions?.length &&
    !rule.directions.some(
      (direction) =>
        (event.direction || "").toLowerCase() === direction.toLowerCase(),
    )
  )
    return false;
  if (
    rule.statusCodes?.length &&
    !rule.statusCodes.some((status) => String(event.status || "") === status)
  )
    return false;
  return true;
};

export const evaluateTLSFilter = (
  expression: string,
  event: TLSPlaintextEvent,
): boolean => {
  if (!expression) return true;
  const orParts = splitTLSFilterExpression(expression, "|");
  if (orParts.length > 1)
    return orParts.some((part) => evaluateTLSFilter(part, event));
  const andParts = splitTLSFilterExpression(expression, "&");
  if (andParts.length > 1)
    return andParts.every((part) => evaluateTLSFilter(part, event));

  const operators = [">=", "<=", "!=", "=", ">", "<", "~"];
  for (const operator of operators) {
    const index = expression.indexOf(operator);
    if (index < 0) continue;
    const field = expression.slice(0, index).trim();
    const expected = unescapeTLSFilterValue(
      expression.slice(index + operator.length).trim(),
    );
    const eventData: Record<string, unknown> = {
      ...event,
      data_type: event.data_type || detectTLSDataType(event),
      is_handshake: event.is_handshake || false,
      truncated: event.truncated || false,
    };
    return compareTLSFilterValue(eventData[field], operator, expected);
  }
  return false;
};

const splitTLSFilterExpression = (expression: string, separator: string) => {
  const parts: string[] = [];
  let depth = 0;
  let current = "";
  for (const character of expression) {
    if (character === "(") depth++;
    else if (character === ")") depth--;
    if (character === separator && depth === 0) {
      parts.push(current.trim());
      current = "";
    } else {
      current += character;
    }
  }
  if (current.trim()) parts.push(current.trim());
  return parts;
};

const compareTLSFilterValue = (
  fieldValue: unknown,
  operator: string,
  expected: string,
) => {
  if (fieldValue === undefined || fieldValue === null) return false;
  const numericValue = Number(fieldValue);
  const numericExpected = Number(expected);
  const compareAsNumber =
    !Number.isNaN(numericValue) &&
    !Number.isNaN(numericExpected) &&
    typeof fieldValue === "number";
  switch (operator) {
    case "=":
    case "exact":
      return compareAsNumber
        ? numericValue === numericExpected
        : String(fieldValue) === expected;
    case "!=":
    case "not_equal":
      return compareAsNumber
        ? numericValue !== numericExpected
        : String(fieldValue) !== expected;
    case ">":
    case "gt":
      return numericValue > numericExpected;
    case "<":
    case "lt":
      return numericValue < numericExpected;
    case ">=":
    case "gte":
      return numericValue >= numericExpected;
    case "<=":
    case "lte":
      return numericValue <= numericExpected;
    case "~":
    case "contains":
      return String(fieldValue).toLowerCase().includes(expected.toLowerCase());
    default:
      return false;
  }
};

const unescapeTLSFilterValue = (value: string) =>
  value
    .replace(/\\r/g, "\r")
    .replace(/\\n/g, "\n")
    .replace(/\\t/g, "\t")
    .replace(/\\\\/g, "\\")
    .replace(/\\"/g, '"');

const detectTLSDataType = (event: TLSPlaintextEvent) => {
  const body = event.body || "";
  if (!body) return event.data_type || "empty";
  if (body.startsWith("HTTP/")) return "http_response";
  if (/^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) /.test(body))
    return "http_request";
  if (body.startsWith("data:") || body.startsWith("event:")) return "sse";
  const trimmed = body.trim();
  if (
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"))
  )
    return "json";
  return "text";
};

export const buildTLSCurlCommand = (event: TLSPlaintextEvent) => {
  const target =
    event.host && (event.url || "").startsWith("/")
      ? `https://${event.host}${event.url}`
      : event.url || "https://example.invalid";
  const parts = ["curl", "-X", event.method || "GET"];
  Object.entries(event.headers || {}).forEach(([key, value]) => {
    if (value !== "***REDACTED***") parts.push("-H", `${key}: ${value}`);
  });
  if (event.body) parts.push("--data", event.body);
  parts.push(target);
  return parts.map((part) => `'${part.replaceAll("'", "'\\''")}'`).join(" ");
};
