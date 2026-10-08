import { describe, expect, test } from "bun:test";

import { buildGroups } from "../src/components/monitor/observer/agentContextParsing";

const event = (overrides: Record<string, unknown> = {}) =>
  ({
    key: "tls-test",
    timestamp: "2026-10-05T00:00:00Z",
    pid: 42,
    tgid: 42,
    comm: "codex",
    direction: "send",
    lib: "codex-reqwest",
    function: "websocket_request",
    captured_len: 0,
    original_len: 0,
    type: "websocket_request",
    method: "WEBSOCKET",
    url: "wss://api.openai.com/v1/responses",
    host: "api.openai.com",
    status: 0,
    ...overrides,
  }) as any;

describe("Agent Context upstream parsing", () => {
  test("keeps Responses WebSocket requests visible as request context", () => {
    const body =
      '{"type":"response.create","stream_id":"main","input":[{"role":"user","content":[{"type":"input_text","text":"hello upstream"}]}]}';
    const groups = buildGroups(
      [event({ body, body_size: body.length, protocol_event: "response.create" })],
      "send",
    );

    expect(groups).toHaveLength(1);
    expect(groups[0].contentBlocks).toHaveLength(1);
    expect(groups[0].contentBlocks[0].type).toBe("request_body");
    expect(groups[0].contentBlocks[0].mergedText).toContain("hello upstream");
  });

  test("keeps response.steer input on the upstream side", () => {
    const body =
      '{"type":"response.steer","previous_response_id":"resp_1","input":"make it shorter"}';
    const groups = buildGroups(
      [
        event({
          body,
          body_size: body.length,
          protocol_event: "response.steer",
          previous_response_id: "resp_1",
        }),
      ],
      "send",
    );

    expect(groups[0].contentBlocks[0].mergedText).toContain("make it shorter");
    expect(groups[0].messageId).toBe("resp_1");
  });

  test("losslessly rejoins fragmented TLS writes split inside a JSON string", () => {
    const first =
      '{"type":"response.create","input":[{"role":"user","content":"hel';
    const second = 'lo"}]}';

    const groups = buildGroups(
      [
        event({
          key: "frag-1",
          type: "tls_plaintext",
          function: "SSL_write",
          body: first,
          body_size: first.length,
        }),
        event({
          key: "frag-2",
          type: "tls_plaintext",
          function: "SSL_write",
          body: second,
          body_size: second.length,
        }),
      ],
      "send",
    );

    expect(groups).toHaveLength(1);
    expect(groups[0].contentBlocks[0].type).toBe("request_body");
    expect(groups[0].contentBlocks[0].mergedText).toContain('"hello"');
  });
});
