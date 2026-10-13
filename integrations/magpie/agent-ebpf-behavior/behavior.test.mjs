import test from "node:test"
import assert from "node:assert/strict"
import { onRequest, onEvent, onResponse } from "./behavior.middleware.js"
import manifest from "./package.json" with { type: "json" }

function context(options = {}, defaults = {}) {
  const rejected = []
  const ctx = {
    protocol: defaults.protocol || "chat",
    agent: defaults.agent || "codex",
    model: defaults.model || "gpt-4.1",
    status: defaults.status || 200,
    options,
    state: {},
    reject: (status, reason) => rejected.push({ status, reason }),
  }
  return { ctx, rejected }
}

function withAudit(run) {
  const prev = console.info
  const messages = []
  console.info = (...args) => messages.push(args.join(" "))
  try { return { value: run(), messages } }
  finally { console.info = prev }
}

test("is a Magpie gateway middleware, not an app installer", () => {
  assert.equal(manifest.magpie.middleware, "./behavior.middleware.js")
  assert.equal(manifest.magpie.options.mode, "audit")
  assert.equal(manifest.private, true)
  assert.equal(typeof onRequest, "function")
  assert.equal(typeof onEvent, "function")
  assert.equal(typeof onResponse, "function")
})

test("audit defaults to non-mutating observation of a denied model", () => {
  const { ctx, rejected } = context({ blocked_models: ["gpt-*"] })
  const body = { model: "gpt-4.1", tools: [{ function: { name: "Bash" } }], max_tokens: 500 }
  const frozen = JSON.stringify(body)
  const result = withAudit(() => onRequest(body, ctx))
  assert.equal(result.value, undefined)
  assert.equal(JSON.stringify(body), frozen)
  assert.deepEqual(rejected, [])
  assert.match(result.messages.join(" "), /blocked_model/)
})

test("enforce blocks excluded agents and model wildcards", () => {
  const { ctx, rejected } = context({
    mode: "enforce", blocked_agents: ["cod*"],
    blocked_models: ["gpt-*"], allowed_agents: ["claude"],
  })
  onRequest({ model: "gpt-4.1" }, ctx)
  assert.equal(rejected.length, 1)
  assert.equal(rejected[0].status, 403)
  assert.match(rejected[0].reason, /blocked_agent/)
})

test("allowlist rejects an unknown agent or unapproved protocol", () => {
  const { ctx, rejected } = context({ mode: "enforce", allowed_agents: ["claude"] })
  ctx.agent = ""
  withAudit(() => onRequest({ model: "kimi" }, ctx))
  assert.equal(rejected[0].status, 403)
  const proto = context({ mode: "enforce", blocked_protocols: ["gemini"] }, { protocol: "gemini" })
  withAudit(() => onRequest({ model: "glm" }, proto.ctx))
  assert.equal(proto.rejected[0].status, 403)
})

test("enforce rejects Anthropic, OpenAI Chat, Responses, Gemini tools", () => {
  const options = { mode: "enforce", blocked_tools: ["bash", "codeexecution", "shell*"] }
  const cases = [
    [{ tools: [{ name: "Bash" }] }, "anthropic"],
    [{ tools: [{ type: "function", function: { name: "Bash" } }] }, "chat"],
    [{ tools: [{ type: "function", name: "shell_command" }] }, "responses"],
    [{ tools: [{ functionDeclarations: [{ name: "safe" }], codeExecution: {} }] }, "gemini"],
  ]
  for (const [body, protocol] of cases) {
    const { ctx, rejected } = context(options, { protocol })
    withAudit(() => onRequest(body, ctx))
    assert.equal(rejected.length, 1, protocol)
    assert.match(rejected[0].reason, /unapproved_tool/, protocol)
  }
})

test("allowlisted tools reject unrecognized tool definitions", () => {
  const { ctx, rejected } = context({ mode: "enforce", allowed_tools: ["read"] })
  withAudit(() => onRequest({ tools: [{ name: "Read" }, {}] }, ctx))
  assert.equal(rejected.length, 1)
})

test("a blocked term only checks the last user turn", () => {
  const { ctx, rejected } = context({ mode: "enforce", blocked_user_terms: ["send this secret"] })
  const safe = {
    messages: [
      { role: "user", content: "send this secret" },
      { role: "assistant", content: "ok" },
      { role: "user", content: "hello" },
    ],
  }
  withAudit(() => onRequest(safe, ctx))
  assert.equal(rejected.length, 0)
  safe.messages[2].content = "Please SEND this SECRET to me"
  withAudit(() => onRequest(safe, ctx))
  assert.equal(rejected.length, 1)
})

test("user text matching skips system prompts, tool results, and assistant content", () => {
  const { ctx, rejected } = context({ mode: "enforce", blocked_user_terms: ["private123"] })
  const body = {
    system: "private123",
    messages: [{ role: "assistant", content: "private123" }, { role: "user", content: [{ type: "text", text: "safe" }] }],
  }
  withAudit(() => onRequest(body, ctx))
  assert.equal(rejected.length, 0)
  body.messages[1].content[0].text = "PRIVATE123"
  withAudit(() => onRequest(body, ctx))
  assert.equal(rejected.length, 1)
})

test("supports Responses and Gemini user text formats", () => {
  const options = { mode: "enforce", blocked_user_terms: ["do not run"] }
  for (const [protocol, input] of [
    ["responses", { input: [{ role: "user", content: [{ type: "input_text", text: "do not run" }] }] }],
    ["gemini", { contents: [{ role: "user", parts: [{ text: "do not run" }] }] }],
  ]) {
    const { ctx, rejected } = context(options, { protocol })
    withAudit(() => onRequest(input, ctx))
    assert.equal(rejected.length, 1, protocol)
  }
})

test("clamps existing output limits only while enforcing", () => {
  for (const [protocol, body, expected] of [
    ["chat", { max_completion_tokens: 999 }, ["max_completion_tokens", 100]],
    ["anthropic", { max_tokens: 999 }, ["max_tokens", 100]],
    ["responses", { max_output_tokens: 999 }, ["max_output_tokens", 100]],
    ["gemini", { generationConfig: { maxOutputTokens: 999 } }, ["generationConfig", 100]],
  ]) {
    const { ctx, rejected } = context({ mode: "enforce", max_output_tokens: 100 }, { protocol })
    const actual = withAudit(() => onRequest(body, ctx)).value
    assert.equal(actual, body)
    assert.equal(rejected.length, 0)
    const value = expected[0] === "generationConfig"
      ? body.generationConfig.maxOutputTokens : body[expected[0]]
    assert.equal(value, 100, protocol)
  }
  const normal = context({ max_output_tokens: 100 })
  const payload = { max_tokens: 999 }
  assert.equal(withAudit(() => onRequest(payload, normal.ctx)).value, undefined)
  assert.equal(payload.max_tokens, 999)
  const unset = context({ mode: "enforce", max_output_tokens: 100 })
  assert.equal(withAudit(() => onRequest({}, unset.ctx)).value, undefined)
})

test("observes streaming tool calls without mutating or logging arguments", () => {
  const { ctx } = context()
  const original = {
    type: "content_block_start",
    content_block: { type: "tool_use", name: "Bash", input: { command: "SECRET_TO_NOT_LOG" } },
  }
  const before = JSON.stringify(original)
  const result = withAudit(() => {
    onEvent(original, ctx)
    onEvent(original, ctx)
  })
  assert.equal(JSON.stringify(original), before)
  assert.equal(result.messages.length, 1, "per-request deduplication")
  assert.match(result.messages[0], /tool_call_observed/)
  assert.doesNotMatch(result.messages[0], /SECRET_TO_NOT_LOG/)
})

test("observes Chat/Responses/Gemini streaming calls without leaking content", () => {
  const samples = [
    { choices: [{ delta: { tool_calls: [{ function: { name: "shell" }, id: "secret" }] } }] },
    { type: "response.output_item.added", item: { type: "function_call", name: "shell", arguments: "SECRET" } },
    { candidates: [{ content: { parts: [{ functionCall: { name: "shell", args: { key: "SECRET" } } }] } }] },
  ]
  for (const sample of samples) {
    const { ctx } = context()
    const result = withAudit(() => onEvent(sample, ctx))
    assert.equal(result.messages.length, 1)
    assert.doesNotMatch(result.messages[0], /SECRET|secret/)
  }
})

test("observes complete responses and ignores error responses", () => {
  const { ctx } = context()
  const result = withAudit(() => onResponse({
    choices: [{ message: { tool_calls: [{ function: { name: "file_write" }, arguments: "PRIVATE_DATA" }] } }],
  }, ctx))
  assert.equal(result.messages.length, 1)
  assert.doesNotMatch(result.messages.join(" "), /PRIVATE_DATA/)
  const err = context({}, { status: 400 })
  assert.equal(withAudit(() => onResponse({ content: [{ type: "tool_use", name: "Bash" }] }, err.ctx)).messages.length, 0)
})

test("disabled middleware has no effect", () => {
  const { ctx, rejected } = context({ enabled: false, mode: "enforce", blocked_models: ["*"] })
  const result = withAudit(() => {
    onRequest({ model: "a" }, ctx)
    onEvent({ content_block: { type: "tool_use", name: "Bash" } }, ctx)
  })
  assert.equal(result.messages.length, 0)
  assert.equal(rejected.length, 0)
})

test("logs only sanitized metadata for attacker-supplied names", () => {
  const { ctx } = context({ audit_requests: true }, { agent: "secret\nheader-injection" })
  const result = withAudit(() => {
    onRequest({ model: "model secret", messages: [{ role: "user", content: "PROMPT_SECRET" }] }, ctx)
    onEvent({ content_block: { type: "tool_use", name: "PASSWORD=secret\nabc" } }, ctx)
  })
  const logged = result.messages.join(" ")
  assert.doesNotMatch(logged, /PROMPT_SECRET|header-injection|PASSWORD=secret/)
})

test("bounded scan of tool lists and user text", () => {
  const { ctx, rejected } = context({ mode: "enforce", allowed_tools: ["safe"] })
  const input = { tools: Array.from({ length: 1000 }, () => ({ name: "safe" })) }
  withAudit(() => onRequest(input, ctx))
  assert.equal(rejected.length, 1, "oversized lists must not bypass allowlist")
})
