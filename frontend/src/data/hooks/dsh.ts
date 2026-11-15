import type { HookCliDoc } from "./types";

export const dshHook: HookCliDoc = {
  id: "dsh",
  name: "DeepSeek Harness",
  sources: [
    {
      label: "DeepSeek Harness repository",
      url: "https://github.com/deepseek-ai/deepseek-harness",
    },
  ],
  notes: [
    "Native Cordis plugin: registers session/created and session/event observers via the DSH_HOME home patch (default ~/.dsh/cordis.patch.yml), across profiles.",
    "Restart dsh after installation. DSH_HOME must match the backend environment. Existing profile patches and user comments are preserved; unsupported YAML layouts are rejected rather than rewritten.",
    "Metadata only: PID, session, cwd, tool name, call ID and error flag. No arguments, prompts, Code Mode source, or result bodies are sent. This observes events and does not block tools.",
    "Code Mode / PTC: tool/ptc-dispatch-start and tool/ptc-dispatch emit separate nested lifecycle signals with hashed parent correlation in the backend. Parallel nested calls are not treated as exclusive PID context.",
    "High-fanout PTC callbacks use a bounded asynchronous relay queue. If it overflows, telemetry_dropped exposes the drop count rather than implying a complete audit trace.",
    "Uninstall removes only the managed patch block and plugin. Existing dsh wrapper aliases are not changed automatically.",
    "For opt-in process/PTY interception and userspace Inspector network capture, install the separate dsh-exec provider hook. This does not replace native lifecycle telemetry.",
  ],
  events: [
    { name: "session_start", description: "Cordis session/created", fields: [
      { name: "session_id", type: "string", description: "Harness session ID" },
      { name: "pid", type: "number", description: "Harness process PID" },
    ] },
    { name: "tool_call", description: "session/event: tool/call", fields: [
      { name: "tool_name", type: "string", description: "Tool name" },
      { name: "tool_call_id", type: "string", description: "Call correlation ID" },
    ] },
    { name: "ptc_dispatch_start", description: "session/event: tool/ptc-dispatch-start (Code Mode nested call started)", fields: [
      { name: "tool_name", type: "string", description: "Nested tool name" },
      { name: "tool_call_id", type: "string", description: "Nested sub-call ID (hashed in backend event)" },
      { name: "parent_tool_call_id", type: "string", description: "Parent run_code call ID (hashed in backend ExtraInfo)" },
      { name: "root_tool_call_id", type: "string", description: "Root model-requested call ID (hashed in backend ExtraInfo)" },
    ] },
    { name: "ptc_dispatch", description: "session/event: tool/ptc-dispatch (Code Mode nested call settled)", fields: [
      { name: "tool_name", type: "string", description: "Nested tool name" },
      { name: "tool_call_id", type: "string", description: "Nested sub-call ID" },
      { name: "tool_result.is_error", type: "boolean", description: "Nested call error status" },
    ] },
    { name: "tool_result", description: "session/event: tool/result", fields: [
      { name: "tool_call_id", type: "string", description: "Call correlation ID" },
      { name: "tool_result.is_error", type: "boolean", description: "Tool error flag" },
    ] },
  ],
};
