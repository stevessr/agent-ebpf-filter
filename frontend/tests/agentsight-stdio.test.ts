import { describe, expect, test } from "bun:test";
import { buildProcessTree } from "../src/utils/agentsight/processes";
import {
  decodeContentLengthFrames,
  decodeStdioMessage,
} from "../src/utils/agentsight/stdio";
import { AgentSightStdioStreamDecoder } from "../src/utils/agentsight/stdio_stream";
import type { AgentSightEvent } from "../src/utils/agentsight/types";

const encoder = new TextEncoder();

function frame(payload: unknown) {
  const body = JSON.stringify(payload);
  return `Content-Length: ${encoder.encode(body).byteLength}\r\n\r\n${body}`;
}

function stdioEvent(
  data: Record<string, any>,
  timestamp = 1,
  pid = 42,
): AgentSightEvent {
  return {
    id: `event-${timestamp}-${data.fd ?? "x"}`,
    timestamp,
    source: "stdio",
    rawSource: "stdio",
    pid,
    ppid: 1,
    comm: "language-server",
    eventType: "STDIO",
    traceId: "",
    spanId: "",
    redactionState: "",
    title: "stdio",
    data: {
      direction: "write",
      fd_role: "stdin",
      fd: 0,
      ...data,
    },
    raw: data,
  };
}

describe("Content-Length stdio framing", () => {
  test("uses UTF-8 byte length for CJK and emoji payloads", () => {
    const raw = frame({
      jsonrpc: "2.0",
      id: 7,
      method: "textDocument/hover",
      params: { text: "鱼🐟" },
    });
    const decoded = decodeContentLengthFrames(raw);
    expect(decoded.framed).toBe(true);
    expect(decoded.incomplete).toBe(false);
    expect(decoded.error).toBeUndefined();
    expect(decoded.payloads).toHaveLength(1);
    expect(JSON.parse(decoded.payloads[0]).params.text).toBe("鱼🐟");
    expect(decoded.consumedBytes).toBe(encoder.encode(raw).byteLength);
  });

  test("parses concatenated LSP frames without losing boundaries", () => {
    const raw =
      frame({ jsonrpc: "2.0", method: "initialized", params: {} }) +
      frame({ jsonrpc: "2.0", id: 9, method: "shutdown" });
    const decoded = decodeStdioMessage({ data: raw, direction: "write" });
    expect(decoded.protocol).toBe("lsp");
    expect(decoded.frameCount).toBe(2);
    expect(decoded.parsedMessages).toHaveLength(2);
    expect(decoded.incompleteFrame).toBe(false);
  });

  test("distinguishes MCP from generic JSON-RPC", () => {
    const mcp = decodeStdioMessage({
      data: frame({
        jsonrpc: "2.0",
        id: 1,
        method: "tools/call",
        params: { name: "search", arguments: { text: "hello" } },
      }),
    });
    expect(mcp.protocol).toBe("mcp");
    expect(mcp.toolName).toBe("search");

    const generic = decodeStdioMessage({
      data: frame({ jsonrpc: "2.0", id: 2, method: "custom/run" }),
    });
    expect(generic.protocol).toBe("jsonrpc");
  });
});

describe("stateful stdio stream reassembly", () => {
  test("reassembles a frame split across capture events", () => {
    const decoder = new AgentSightStdioStreamDecoder();
    const raw = frame({
      jsonrpc: "2.0",
      id: 11,
      method: "textDocument/completion",
      params: { position: { line: 3, character: 4 } },
    });
    const split = raw.indexOf("\r\n\r\n") + 6;

    const first = decoder.decode(stdioEvent({ data: raw.slice(0, split) }, 10));
    expect(first.incompleteFrame).toBe(true);
    expect(first.pendingBytes).toBeGreaterThan(0);
    expect(decoder.pendingStreamCount()).toBe(1);

    const second = decoder.decode(stdioEvent({ data: raw.slice(split) }, 20));
    expect(second.reassembled).toBe(true);
    expect(second.incompleteFrame).toBe(false);
    expect(second.protocol).toBe("lsp");
    expect(second.method).toBe("textDocument/completion");
    expect(second.id).toBe("11");
    expect(second.pendingBytes).toBe(0);
    expect(decoder.pendingStreamCount()).toBe(0);
  });

  test("retains only the incomplete suffix after complete plus partial frames", () => {
    const decoder = new AgentSightStdioStreamDecoder();
    const firstFrame = frame({
      jsonrpc: "2.0",
      method: "initialized",
      params: {},
    });
    const secondFrame = frame({
      jsonrpc: "2.0",
      method: "textDocument/didOpen",
      params: { textDocument: { uri: "file:///tmp/a.ts", text: "hello" } },
    });
    const split = Math.floor(secondFrame.length / 2);

    const first = decoder.decode(
      stdioEvent({ data: firstFrame + secondFrame.slice(0, split) }, 10),
    );
    expect(first.frameCount).toBe(1);
    expect(first.parsedMessages[0].method).toBe("initialized");
    expect(first.incompleteFrame).toBe(true);

    const second = decoder.decode(
      stdioEvent({ data: secondFrame.slice(split) }, 20),
    );
    expect(second.reassembled).toBe(true);
    expect(second.frameCount).toBe(1);
    expect(second.parsedMessages).toHaveLength(1);
    expect(second.parsedMessages[0].method).toBe("textDocument/didOpen");
    expect(
      second.parsedMessages.some((message) => message.method === "initialized"),
    ).toBe(false);
  });

  test("keeps independent fd streams isolated", () => {
    const decoder = new AgentSightStdioStreamDecoder();
    const a = frame({ jsonrpc: "2.0", id: "a", method: "workspace/symbol" });
    const b = frame({ jsonrpc: "2.0", id: "b", method: "textDocument/hover" });
    const aSplit = Math.floor(a.length / 2);
    const bSplit = Math.floor(b.length / 2);

    decoder.decode(stdioEvent({ fd: 1, data: a.slice(0, aSplit) }, 10));
    decoder.decode(stdioEvent({ fd: 2, data: b.slice(0, bSplit) }, 11));
    expect(decoder.pendingStreamCount()).toBe(2);

    const bDone = decoder.decode(stdioEvent({ fd: 2, data: b.slice(bSplit) }, 12));
    expect(bDone.id).toBe("b");
    expect(bDone.method).toBe("textDocument/hover");
    expect(decoder.pendingStreamCount()).toBe(1);

    const aDone = decoder.decode(stdioEvent({ fd: 1, data: a.slice(aSplit) }, 13));
    expect(aDone.id).toBe("a");
    expect(aDone.method).toBe("workspace/symbol");
    expect(decoder.pendingStreamCount()).toBe(0);
  });

  test("drops pending state after a collector truncation gap", () => {
    const decoder = new AgentSightStdioStreamDecoder();
    const raw = frame({ jsonrpc: "2.0", id: 4, method: "workspace/configuration" });
    const split = Math.floor(raw.length / 2);

    decoder.decode(stdioEvent({ data: raw.slice(0, split) }, 10));
    expect(decoder.pendingStreamCount()).toBe(1);

    const gap = decoder.decode(
      stdioEvent({ data: "dropped bytes", truncated: true }, 20),
    );
    expect(gap.reassemblyReset).toContain("truncated");
    expect(decoder.pendingStreamCount()).toBe(0);

    const tail = decoder.decode(stdioEvent({ data: raw.slice(split) }, 30));
    expect(tail.reassembled).not.toBe(true);
    expect(tail.method).toBeUndefined();
  });

  test("fails closed when a pending frame exceeds the bounded cache", () => {
    const decoder = new AgentSightStdioStreamDecoder({
      maxPendingBytesPerStream: 256,
      maxTotalPendingBytes: 512,
    });
    const partial = `Content-Length: 1000\r\n\r\n${"x".repeat(300)}`;
    const decoded = decoder.decode(stdioEvent({ data: partial }, 10));
    expect(decoded.reassemblyReset).toContain("limit");
    expect(decoder.pendingStreamCount()).toBe(0);
    expect(decoder.pendingByteCount()).toBe(0);
  });

  test("process tree uses the chronological stream decoder", () => {
    const raw = frame({
      jsonrpc: "2.0",
      id: 21,
      method: "textDocument/definition",
    });
    const split = Math.floor(raw.length / 2);
    const tree = buildProcessTree([
      stdioEvent({ data: raw.slice(split) }, 20),
      stdioEvent({ data: raw.slice(0, split) }, 10),
    ]);
    expect(tree).toHaveLength(1);
    expect(tree[0].events).toHaveLength(2);
    expect(tree[0].events[1].metadata.stdio_reassembled).toBe(true);
    expect(tree[0].events[1].metadata.rpc_method).toBe(
      "textDocument/definition",
    );
  });
});
