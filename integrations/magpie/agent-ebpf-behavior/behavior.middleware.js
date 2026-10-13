// Agent eBPF Filter policy for yetone/magpie's gateway middleware.
//
// This code runs in Magpie's restricted moejs runtime: no fetch, timers,
// Node.js, filesystem, or kernel privileges. It is intentionally a pure,
// bounded request guard plus metadata-only audit. System-level enforcement
// remains in the Agent eBPF Filter backend/eBPF/agent-wrapper.
//
// Hook semantics: undefined = unchanged, ctx.reject(403, ...) = preflight
// denial. Events and responses are observed, not mutated or suppressed.

const MAX_RULES = 128
const MAX_TOOLS = 256
const MAX_TEXT_CHARS = 64 * 1024

function list(value) {
  if (!Array.isArray(value)) return []
  const out = []
  for (let i = 0; i < value.length && out.length < MAX_RULES; i++) {
    if (typeof value[i] === "string") {
      const name = value[i].trim().toLowerCase()
      if (name && name.length <= 160) out.push(name)
    }
  }
  return out
}

// Exact match or a single trailing wildcard, e.g. "claude-*". No regex
// supplied by a configuration or a request is executed.
function matches(rules, input) {
  if (typeof input !== "string") return false
  const value = input.toLowerCase()
  for (const rule of rules) {
    if (rule === "*" || rule === value) return true
    if (rule.endsWith("*") && value.startsWith(rule.slice(0, -1))) return true
  }
  return false
}

function object(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function toolName(tool) {
  if (!object(tool)) return ""
  if (typeof tool.name === "string") return tool.name
  if (object(tool.function) && typeof tool.function.name === "string") return tool.function.name
  if (typeof tool.type === "string" && tool.type !== "function") return tool.type
  return ""
}

// Tool categories are HEURISTICS based only on declared names, not on tool
// arguments or runtime effects. For mandatory security use Agent eBPF
// Filter's process, file and network enforcement instead.
function toolCategory(name) {
  if (typeof name !== "string") return "other"
  const n = name.toLowerCase().replace(/[^a-z0-9]/g, "")
  if (/^(bash|shell|shellcommand|exec|execcommand|execute|runcommand|terminal|codeexecution|pythonrepl|runshell|runterminal)$/.test(n))
    return "process"
  if (/^(write|edit|multiedit|applypatch|writefile|editfile|createfile|deletefile|removefile|renamefile|movefile|mkdir|filewrite|fswrite)$/.test(n))
    return "file_write"
  if (/^(read|readfile|listfiles|glob|grep|searchfiles|fileread|fsread)$/.test(n))
    return "file_read"
  if (/^(webfetch|websearch|googlesearch|fetchurl|httpget|browse|browser|searchweb|networkrequest|fetch)$/.test(n))
    return "network"
  return "other"
}

function inputTools(body) {
  // Magpie currently supports Anthropic, Chat, Responses and Gemini. The
  // first three use one definition per tool; Gemini may group function
  // declarations, or expose standalone built-ins like codeExecution.
  const tools = Array.isArray(body.tools) ? body.tools : []
  const out = []
  let overflow = tools.length > MAX_TOOLS
  for (let i = 0; i < tools.length && i < MAX_TOOLS; i++) {
    const tool = tools[i]
    if (out.length >= MAX_TOOLS) {
      overflow = true
      break
    }
    if (object(tool) && Array.isArray(tool.functionDeclarations)) {
      if (tool.functionDeclarations.length > MAX_TOOLS - out.length) overflow = true
      for (const d of tool.functionDeclarations.slice(0, MAX_TOOLS - out.length)) {
        out.push(toolName(d))
      }
      // Gemini built-in tools (e.g. googleSearch, codeExecution).
      for (const key of Object.keys(tool)) {
        if (key !== "functionDeclarations") {
          if (out.length >= MAX_TOOLS) overflow = true
          else out.push(key)
        }
      }
    } else if (object(tool)) {
      const name = toolName(tool)
      if (name) {
        out.push(name)
      } else if (object(tool.functionDeclarations)) {
        out.push("")
      } else {
        const keys = Object.keys(tool)
        if (keys.length === 0) out.push("")
        for (const key of keys) {
          if (out.length >= MAX_TOOLS) { overflow = true; break }
          if (key === "functionDeclarations") out.push("")
          else out.push(key)
        }
      }
    } else {
      out.push("")
    }
  }
  if (overflow) out.push("") // unknown/unchecked extra tools => fail closed
  return out
}

// Strings are deliberately not sent to any other process. The scan is
// bounded, and enforcing rules must reject oversized text instead of
// silently ignoring blocked terms at the end.
function requestedText(body, protocol) {
  let messages = []
  if (protocol === "gemini") {
    messages = Array.isArray(body.contents) ? body.contents : []
  } else if (protocol === "responses") {
    if (typeof body.input === "string") {
      return { text: body.input.slice(0, MAX_TEXT_CHARS), truncated: body.input.length > MAX_TEXT_CHARS }
    }
    messages = Array.isArray(body.input) ? body.input : []
  } else {
    messages = Array.isArray(body.messages) ? body.messages : []
  }
  let remaining = MAX_TEXT_CHARS
  let result = ""
  let truncated = false
  // Only the user's latest turn, not system prompts, tool results or prior
  // assistant responses. This does not replace kernel/process monitoring.
  for (let i = messages.length - 1; i >= 0 && remaining > 0; i--) {
    const message = messages[i]
    if (!object(message) || (message.role !== "user" && !(protocol === "gemini" && !message.role))) continue
    const parts = typeof message.content === "string"
      ? [message.content]
      : Array.isArray(message.content)
        ? message.content
        : Array.isArray(message.parts) ? message.parts : []
    for (const part of parts) {
      const value = typeof part === "string"
        ? part
        : object(part) && typeof part.text === "string" &&
          (protocol === "gemini" || part.type === "text" || part.type === "input_text") ? part.text : ""
      if (value) {
        if (value.length > remaining) truncated = true
        const chunk = value.slice(0, remaining)
        result += chunk
        remaining -= chunk.length
      }
    }
    break
  }
  return { text: result, truncated }
}

function label(value) {
  // Avoid logging user-controlled strings, newlines or prompt content.
  if (typeof value !== "string") return "unknown"
  return /^[a-z0-9_.:/-]{1,80}$/i.test(value) ? value : "unknown"
}

function audit(ctx, type, code, detail) {
  if (ctx.options && ctx.options.audit === false) return
  // Magpie caps middleware logging to 60 lines/min; never include prompts,
  // tool arguments, API keys, model outputs or other request content.
  console.info("[agent-ebpf-filter] " + JSON.stringify({
    event: type,
    code: code,
    agent: label(ctx.agent),
    protocol: label(ctx.protocol),
    tool: label(detail),
  }))
}

// Malformed and overlong rules in enforce mode are rejected rather than
// silently ignored. In audit mode we keep observing without disruption.
const POLICY_LIST_FIELDS = [
  "allowed_agents", "blocked_agents", "allowed_models", "blocked_models",
  "allowed_tools", "blocked_tools", "blocked_protocols", "blocked_user_terms",
  "blocked_tool_categories", "allowed_tool_categories",
]

function invalidOptions(o) {
  for (const key of POLICY_LIST_FIELDS) {
    if (o[key] === undefined) continue
    if (!Array.isArray(o[key]) || o[key].length > MAX_RULES) return true
    for (const item of o[key]) {
      if (typeof item !== "string" || item.trim().length === 0 || item.length > 160) return true
    }
  }
  const categories = ["process", "file_write", "file_read", "network", "other"]
  for (const key of ["blocked_tool_categories", "allowed_tool_categories"]) {
    if (o[key] === undefined) continue
    for (const name of o[key]) {
      if (typeof name !== "string" || !categories.includes(name.toLowerCase())) return true
    }
  }
  if (o.max_output_tokens !== undefined && o.max_output_tokens !== 0 &&
      (!Number.isSafeInteger(o.max_output_tokens) ||
       o.max_output_tokens < 1 || o.max_output_tokens > 131072)) return true
  return false
}

function tokenCap(body, protocol, max) {
  if (!Number.isSafeInteger(max) || max < 1 || max > 131072) return false
  // Only clamp a limit the client sent. Introducing a missing limit might
  // break model-specific protocols (e.g. chat max_completion_tokens).
  const keys = protocol === "responses"
    ? ["max_output_tokens"]
    : protocol === "chat"
      ? ["max_completion_tokens", "max_tokens"]
      : ["max_tokens", "max_output_tokens"]
  let changed = false
  for (const key of keys) {
    if (typeof body[key] === "number" && body[key] > max) {
      body[key] = max
      changed = true
    }
  }
  if (protocol === "gemini" && object(body.generationConfig)) {
    const n = body.generationConfig.maxOutputTokens
    if (typeof n === "number" && n > max) {
      body.generationConfig.maxOutputTokens = max
      changed = true
    }
  }
  return changed
}

// Strict action checks happen BEFORE the model is called. Unsupported
// execution details (filesystem, subprocess and network) are enforced by
// Agent eBPF Filter itself, not by this gateway plugin.
export function onRequest(body, ctx) {
  if (!object(body) || !ctx || !object(ctx)) return
  const o = object(ctx.options) ? ctx.options : {}
  if (o.enabled === false) return
  const enforce = o.mode === "enforce"
  if (enforce && invalidOptions(o)) {
    audit(ctx, "deny", "invalid_policy", "")
    ctx.reject(403, "Agent eBPF Filter / Magpie policy denied request: invalid_policy")
    return
  }

  const protocol = typeof ctx.protocol === "string" ? ctx.protocol : ""
  const agent = typeof ctx.agent === "string" ? ctx.agent : ""
  const model = typeof body.model === "string" ? body.model
    : typeof ctx.model === "string" ? ctx.model : ""
  const violations = []
  const allowedAgents = list(o.allowed_agents)
  const allowedModels = list(o.allowed_models)
  const allowedTools = list(o.allowed_tools)
  const blockedAgents = list(o.blocked_agents)
  const blockedModels = list(o.blocked_models)
  const blockedProtocols = list(o.blocked_protocols)
  const blockedTools = list(o.blocked_tools)
  const blockedCategories = list(o.blocked_tool_categories)
  const allowedCategories = list(o.allowed_tool_categories)

  if (matches(blockedAgents, agent)) violations.push("blocked_agent")
  if (allowedAgents.length && !matches(allowedAgents, agent)) violations.push("unapproved_agent")
  if (matches(blockedProtocols, protocol)) violations.push("blocked_protocol")
  if (matches(blockedModels, model)) violations.push("blocked_model")
  if (allowedModels.length && !matches(allowedModels, model)) violations.push("unapproved_model")

  // Explicit/forced tool_choice can name a tool independently of tools[].
  // Include it so it cannot evade allow/deny checks.
  const tools = inputTools(body)
  if (object(body.tool_choice)) {
    const choice = body.tool_choice
    const choiceType = typeof choice.type === "string" ? choice.type : ""
    if (choiceType === "auto" || choiceType === "none" ||
        choiceType === "any" || choiceType === "required") {
      // No individual tool name is forced.
    } else {
      const forced = toolName(choice)
      tools.push(forced)
    }
  } else if (typeof body.tool_choice === "string" &&
             !["auto", "none", "any", "required"].includes(body.tool_choice)) {
    tools.push(body.tool_choice)
  }
  const geminiChoice = object(body.toolConfig) && object(body.toolConfig.functionCallingConfig)
    ? body.toolConfig.functionCallingConfig : null
  if (geminiChoice && Array.isArray(geminiChoice.allowedFunctionNames)) {
    for (const name of geminiChoice.allowedFunctionNames.slice(0, MAX_TOOLS)) tools.push(name)
    if (geminiChoice.allowedFunctionNames.length > MAX_TOOLS) tools.push("")
  }
  if ((blockedTools.length || allowedTools.length) && tools.includes("")) {
    violations.push("uninspectable_tool")
  }
  for (const name of tools) {
    if ((blockedTools.length && matches(blockedTools, name)) ||
        (allowedTools.length && !matches(allowedTools, name))) {
      violations.push("unapproved_tool")
      break
    }
    const category = toolCategory(name)
    if ((blockedCategories.length && matches(blockedCategories, category)) ||
        (allowedCategories.length && !matches(allowedCategories, category))) {
      violations.push("unapproved_tool_category")
      break
    }
  }

  const terms = list(o.blocked_user_terms)
  if (terms.length) {
    const inspected = requestedText(body, protocol)
    if (terms.some((term) => inspected.text.toLowerCase().includes(term))) {
      violations.push("blocked_user_term")
    } else if (inspected.truncated) {
      violations.push("uninspectable_user_text")
    }
  }

  if (violations.length) {
    audit(ctx, enforce ? "deny" : "observe", violations[0], "")
    if (enforce) {
      ctx.reject(403, "Agent eBPF Filter / Magpie policy denied request: " + violations[0])
      return
    }
  } else if (o.audit_requests === true) {
    audit(ctx, "request", "allowed", "")
  }

  if (!enforce) return
  if (tokenCap(body, protocol, o.max_output_tokens)) {
    audit(ctx, "limit", "output_tokens_capped", "")
    return body
  }
}

// A streaming tool invocation is visible but cannot be retroactively
// forbidden at this stage. No output is changed; use eBPF/wrapper to
// enforce actual process, filesystem and network operations.
function emittedTools(event) {
  const out = []
  if (object(event) && object(event.content_block) && event.content_block.type === "tool_use") {
    out.push(toolName(event.content_block))
  }
  if (object(event) && object(event.item) && event.item.type === "function_call") {
    out.push(toolName(event.item))
  }
  if (object(event) && Array.isArray(event.choices)) {
    for (const choice of event.choices.slice(0, 8)) {
      const delta = object(choice) ? choice.delta : null
      if (object(delta) && Array.isArray(delta.tool_calls)) {
        for (const tool of delta.tool_calls.slice(0, 16)) {
          if (object(tool) && object(tool.function) && tool.function.name) out.push(toolName(tool))
        }
      }
    }
  }
  if (object(event) && Array.isArray(event.candidates)) {
    for (const c of event.candidates.slice(0, 8)) {
      const parts = c && c.content && Array.isArray(c.content.parts) ? c.content.parts : []
      for (const part of parts.slice(0, 16)) {
        if (object(part) && object(part.functionCall)) out.push(toolName(part.functionCall))
      }
    }
  }
  return out
}

function observeToolCalls(body, ctx) {
  const o = object(ctx.options) ? ctx.options : {}
  if (o.enabled === false || o.audit_tool_calls === false) return
  const names = emittedTools(body)
  if (object(body) && Array.isArray(body.content)) {
    for (const item of body.content.slice(0, 32)) {
      if (object(item) && item.type === "tool_use") names.push(toolName(item))
    }
  }
  if (object(body) && Array.isArray(body.output)) {
    for (const item of body.output.slice(0, 32)) {
      if (object(item) && item.type === "function_call") names.push(toolName(item))
    }
  }
  if (object(body) && Array.isArray(body.choices)) {
    for (const choice of body.choices.slice(0, 8)) {
      const calls = choice && choice.message && Array.isArray(choice.message.tool_calls)
        ? choice.message.tool_calls : []
      for (const item of calls.slice(0, 32)) names.push(toolName(item))
    }
  }
  const state = object(ctx.state) ? ctx.state : {}
  if (!object(state.seen_tools)) state.seen_tools = {}
  for (const name of names.slice(0, 32)) {
    const safe = label(name)
    if (safe === "unknown" || state.seen_tools[safe]) continue
    if (Object.keys(state.seen_tools).length >= 32) break
    state.seen_tools[safe] = true
    audit(ctx, "tool_call_observed", "model_proposed_tool", safe)
  }
}

export function onEvent(event, ctx) {
  if (ctx && object(ctx)) observeToolCalls(event, ctx)
}

export function onResponse(body, ctx) {
  if (ctx && object(ctx) && !(ctx.status >= 400)) observeToolCalls(body, ctx)
}
