import {
  decodeContentLengthFrames,
  decodeStdioMessage,
} from "./stdio";
import type { AgentSightEvent, DecodedStdioMessage } from "./types";

const utf8Encoder = new TextEncoder();
const utf8Decoder = new TextDecoder("utf-8");

const defaultStdioStreamTTL = 30_000;
const defaultMaxStdioStreams = 256;
const defaultMaxPendingBytesPerStream = 1024 * 1024;
const defaultMaxTotalPendingBytes = 8 * 1024 * 1024;

interface PendingStdioStream {
  pending: Uint8Array;
  lastSeen: number;
}

export interface AgentSightStdioStreamDecoderOptions {
  ttlMs?: number;
  maxStreams?: number;
  maxPendingBytesPerStream?: number;
  maxTotalPendingBytes?: number;
}

function rawStdioPayload(data: Record<string, any>) {
  if (typeof data?.data === "string") return data.data;
  if (typeof data?.payload === "string") return data.payload;
  return "";
}

function stdioKeyPart(value: unknown) {
  return String(value ?? "").slice(0, 256);
}

export function agentSightStdioStreamKey(event: AgentSightEvent) {
  const data = event.data || {};
  const direction = stdioKeyPart(data.direction || data.stream).toUpperCase();
  const fd =
    typeof data.fd === "number" || typeof data.fd === "string"
      ? stdioKeyPart(data.fd)
      : "?";
  const fdRole = stdioKeyPart(data.fd_role || data.fdRole || data.stream || "stdio");
  const fdTarget = stdioKeyPart(data.fd_target || data.fdTarget);
  return `${event.pid}|${fd}|${direction}|${fdRole}|${fdTarget}`;
}

function concatBytes(left: Uint8Array, right: Uint8Array) {
  if (left.length === 0) return right.slice();
  if (right.length === 0) return left.slice();
  const combined = new Uint8Array(left.length + right.length);
  combined.set(left, 0);
  combined.set(right, left.length);
  return combined;
}

function appendSummary(decoded: DecodedStdioMessage, suffix: string) {
  if (!suffix) return;
  decoded.summary = `${decoded.summary} · ${suffix}`;
}

// Stateful, bounded LSP/MCP/JSON-RPC Content-Length reassembly. The decoder is
// intentionally scoped to one chronological Agent Sight processing pass (for
// example buildProcessTree) rather than being a module singleton, which keeps
// React/Vue recomputation deterministic and avoids replaying stale fragments.
export class AgentSightStdioStreamDecoder {
  private readonly streams = new Map<string, PendingStdioStream>();
  private totalPendingBytes = 0;
  private readonly ttlMs: number;
  private readonly maxStreams: number;
  private readonly maxPendingBytesPerStream: number;
  private readonly maxTotalPendingBytes: number;

  constructor(options: AgentSightStdioStreamDecoderOptions = {}) {
    this.ttlMs = Math.max(1, options.ttlMs ?? defaultStdioStreamTTL);
    this.maxStreams = Math.max(1, options.maxStreams ?? defaultMaxStdioStreams);
    this.maxPendingBytesPerStream = Math.max(
      256,
      options.maxPendingBytesPerStream ?? defaultMaxPendingBytesPerStream,
    );
    this.maxTotalPendingBytes = Math.max(
      this.maxPendingBytesPerStream,
      options.maxTotalPendingBytes ?? defaultMaxTotalPendingBytes,
    );
  }

  private deleteStream(key: string) {
    const previous = this.streams.get(key);
    if (!previous) return false;
    this.totalPendingBytes -= previous.pending.byteLength;
    this.streams.delete(key);
    return true;
  }

  private evictOldest() {
    let oldestKey: string | undefined;
    let oldestTimestamp = Number.POSITIVE_INFINITY;
    for (const [key, state] of this.streams) {
      if (state.lastSeen < oldestTimestamp) {
        oldestTimestamp = state.lastSeen;
        oldestKey = key;
      }
    }
    if (oldestKey === undefined) return false;
    return this.deleteStream(oldestKey);
  }

  private cleanupExpired(now: number) {
    for (const [key, state] of this.streams) {
      if (now - state.lastSeen > this.ttlMs) this.deleteStream(key);
    }
  }

  private storePending(key: string, pending: Uint8Array, lastSeen: number) {
    this.deleteStream(key);
    if (
      pending.byteLength === 0 ||
      pending.byteLength > this.maxPendingBytesPerStream ||
      pending.byteLength > this.maxTotalPendingBytes
    ) {
      return false;
    }

    while (this.streams.size >= this.maxStreams) {
      if (!this.evictOldest()) break;
    }
    while (
      this.totalPendingBytes + pending.byteLength >
      this.maxTotalPendingBytes
    ) {
      if (!this.evictOldest()) break;
    }
    if (
      this.streams.size >= this.maxStreams ||
      this.totalPendingBytes + pending.byteLength > this.maxTotalPendingBytes
    ) {
      return false;
    }

    const copy = pending.slice();
    this.streams.set(key, { pending: copy, lastSeen });
    this.totalPendingBytes += copy.byteLength;
    return true;
  }

  decode(event: AgentSightEvent): DecodedStdioMessage {
    const data = event.data || {};
    const key = agentSightStdioStreamKey(event);
    const timestamp = Number.isFinite(event.timestamp)
      ? event.timestamp
      : Date.now();
    this.cleanupExpired(timestamp);

    const previous = this.streams.get(key);
    const rawPayload = rawStdioPayload(data);
    const segment = utf8Encoder.encode(rawPayload);

    // Once the collector reports truncation there is a known byte gap. Never
    // concatenate future data onto the old partial frame because that can turn
    // unrelated bytes into syntactically valid but semantically false JSON.
    if (Boolean(data.truncated)) {
      const reset = this.deleteStream(key);
      const decoded = decodeStdioMessage(data);
      decoded.streamKey = key;
      if (reset) {
        decoded.reassemblyReset = "collector truncated the stdio stream";
        appendSummary(decoded, "reassembly reset");
      }
      return decoded;
    }

    const previousBytes = previous?.pending.byteLength ?? 0;
    const combined = previous
      ? concatBytes(previous.pending, segment)
      : segment.slice();
    if (previous) this.deleteStream(key);

    if (
      combined.byteLength > this.maxPendingBytesPerStream ||
      combined.byteLength > this.maxTotalPendingBytes
    ) {
      const decoded = decodeStdioMessage(data);
      decoded.streamKey = key;
      decoded.reassemblyReset = "stdio reassembly buffer limit exceeded";
      decoded.framingError = decoded.reassemblyReset;
      appendSummary(decoded, "reassembly overflow");
      return decoded;
    }

    const combinedText = utf8Decoder.decode(combined);
    const framing = decodeContentLengthFrames(combinedText);

    if (!framing.framed) {
      const decoded = decodeStdioMessage(data);
      decoded.streamKey = key;
      if (previousBytes > 0) {
        decoded.reassemblyReset = "stdio stream lost Content-Length framing";
        decoded.framingError = decoded.reassemblyReset;
        appendSummary(decoded, "reassembly reset");
      }
      return decoded;
    }

    const decoded = decodeStdioMessage({
      ...data,
      data: combinedText,
      payload: combinedText,
      len: combined.byteLength,
      size: combined.byteLength,
      truncated: false,
    });
    decoded.streamKey = key;
    decoded.reassembled = previousBytes > 0;
    if (decoded.reassembled) decoded.reassembledBytes = combined.byteLength;

    if (framing.error) {
      decoded.reassemblyReset = framing.error;
      decoded.pendingBytes = 0;
      appendSummary(decoded, "reassembly reset");
      return decoded;
    }

    if (framing.incomplete) {
      const start = Math.min(
        Math.max(0, framing.consumedBytes),
        combined.byteLength,
      );
      const pending = combined.slice(start);
      if (!this.storePending(key, pending, timestamp)) {
        decoded.reassemblyReset = "stdio pending frame exceeds bounded cache";
        decoded.framingError = decoded.reassemblyReset;
        decoded.pendingBytes = 0;
        appendSummary(decoded, "reassembly overflow");
        return decoded;
      }
      decoded.pendingBytes = pending.byteLength;
    } else {
      decoded.pendingBytes = 0;
    }

    if (decoded.reassembled) {
      appendSummary(decoded, `reassembled ${combined.byteLength}B`);
    }
    return decoded;
  }

  pendingStreamCount() {
    return this.streams.size;
  }

  pendingByteCount() {
    return this.totalPendingBytes;
  }
}
