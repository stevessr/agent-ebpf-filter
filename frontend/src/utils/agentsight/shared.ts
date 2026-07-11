import { EVENT_TYPE_NAMES } from "./constants";

export function readAny<T = any>(obj: any, keys: string[], fallback?: T): T {
  if (!obj || typeof obj !== "object") return fallback as T;
  for (const key of keys) {
    if (obj[key] !== undefined && obj[key] !== null) return obj[key] as T;
  }
  return fallback as T;
}

export function firstNonEmpty(...values: any[]) {
  for (const value of values) {
    if (value !== undefined && value !== null && String(value).trim() !== "")
      return value;
  }
  return "";
}

export function toNumber(value: any, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function eventTypeName(value: any, fallback: string) {
  if (typeof value === "number") return EVENT_TYPE_NAMES[value] || fallback;
  if (typeof value === "string" && value.trim() !== "") return value;
  return fallback;
}

export function parseTimestampMs(value: any, fallback = Date.now()) {
  if (typeof value === "number") {
    if (value > 1_000_000_000_000_000) return Math.floor(value / 1_000_000);
    if (value > 10_000_000_000_000) return Math.floor(value / 1_000);
    return Math.floor(value);
  }
  if (typeof value === "string" && value.trim() !== "") {
    const numeric = Number(value);
    if (Number.isFinite(numeric)) return parseTimestampMs(numeric, fallback);
    const parsed = Date.parse(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

export function stableID(prefix: string, parts: any[]) {
  return `${prefix}-${parts
    .map((value) =>
      String(value ?? "")
        .replace(/\s+/g, "_")
        .slice(0, 80),
    )
    .join("-")}`;
}


export function safeJsonParse(value: string): any | null {
  if (!value.trim()) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}
