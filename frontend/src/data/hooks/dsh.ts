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
    "Metadata only: PID, session, cwd, tool name, call ID and error flag. No arguments, prompts or result bodies are sent. This observes events and does not block tools.",
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
    { name: "tool_result", description: "session/event: tool/result", fields: [
      { name: "tool_call_id", type: "string", description: "Call correlation ID" },
      { name: "tool_result.is_error", type: "boolean", description: "Tool error flag" },
    ] },
  ],
};
