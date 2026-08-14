import type { HookCliDoc } from "./types";

export const piHook: HookCliDoc = {
  id: "pi",
  name: "Pi",
  sources: [
    {
      label: "Pi extensions documentation",
      url: "https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md",
    },
    {
      label: "Pi settings documentation",
      url: "https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/settings.md",
    },
  ],
  commonFields: [
    { name: "session_id", type: "string", description: "Current Pi session id" },
    { name: "cwd", type: "string", description: "Working directory" },
    { name: "tool_name", type: "string", description: "Tool name for tool events" },
    { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
  ],
  notes: [
    "Pi discovers TypeScript extensions from ~/.pi/agent/extensions/ and project .pi/extensions/.",
    "The generated integration emits only session metadata and tool-call metadata; prompts and tool results are not forwarded as raw text.",
  ],
  events: [
    {
      name: "session_start",
      description: "When a Pi session starts or resumes",
      fields: [
        { name: "session_id", type: "string", description: "Current Pi session id" },
        { name: "cwd", type: "string", description: "Working directory" },
        { name: "reason", type: "string", description: "Session start reason when supplied" },
      ],
    },
    {
      name: "tool_call",
      description: "Before Pi invokes a tool",
      fields: [
        { name: "tool_name", type: "string", description: "Tool being invoked" },
        { name: "tool_input", type: "object", description: "Tool input supplied by Pi" },
        { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
      ],
    },
    {
      name: "tool_result",
      description: "After a Pi tool invocation completes",
      fields: [
        { name: "tool_name", type: "string", description: "Tool that completed" },
        { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
        { name: "tool_result.is_error", type: "boolean", description: "Whether Pi reported an error" },
      ],
      notes: ["The integration intentionally sends only the error bit, not raw tool output."],
    },
  ],
};
