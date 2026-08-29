import { expect, test } from "bun:test";
import { AgentSightStdioStreamDecoder } from "../src/utils/agentsight/stdio_stream";
import type { AgentSightEvent } from "../src/utils/agentsight/types";

const encoder = new TextEncoder();

function frame(payload: unknown) {
  const body = JSON.stringify(payload);
  return `Content-Length: ${encoder.encode(body).byteLength}\r\n\r\n${body}`;
}

function event(data: string, timestamp: number): AgentSightEvent {
  return {
    id: `e-${timestamp}`,
    timestamp,
    source: "stdio",
    rawSource: "stdio",
    pid: 77,
    ppid: 1,
    comm: "lsp-server",
    eventType: "STDIO",
    traceId: "",
    spanId: "",
    redactionState: "",
    title: "stdio",
    data: { data, direction: "read", fd: 1, fd_role: "stdout" },
    raw: data,
  };
}

test("reassembles when capture splits inside Content-Length header name", () => {
  const decoder = new AgentSightStdioStreamDecoder();
  const raw = frame({ jsonrpc: "2.0", id: 31, method: "workspace/executeCommand" });
  const split = 10; // "Content-Le"

  const first = decoder.decode(event(raw.slice(0, split), 10));
  expect(first.framed).toBe(true);
  expect(first.incompleteFrame).toBe(true);
  expect(first.pendingBytes).toBe(split);

  const second = decoder.decode(event(raw.slice(split), 20));
  expect(second.reassembled).toBe(true);
  expect(second.protocol).toBe("lsp");
  expect(second.method).toBe("workspace/executeCommand");
  expect(second.id).toBe("31");
  expect(decoder.pendingStreamCount()).toBe(0);
});

test("expires stale partial stdio state instead of joining unrelated bytes", () => {
  const decoder = new AgentSightStdioStreamDecoder({ ttlMs: 5 });
  const raw = frame({ jsonrpc: "2.0", id: 32, method: "shutdown" });
  const split = raw.indexOf("\r\n\r\n") + 5;

  decoder.decode(event(raw.slice(0, split), 10));
  expect(decoder.pendingStreamCount()).toBe(1);

  const tail = decoder.decode(event(raw.slice(split), 20));
  expect(tail.reassembled).not.toBe(true);
  expect(tail.method).toBeUndefined();
  expect(decoder.pendingStreamCount()).toBe(0);
});
