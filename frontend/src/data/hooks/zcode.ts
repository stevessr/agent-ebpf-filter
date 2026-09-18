import type { HookCliDoc } from "./types";

export const zcodeHook: HookCliDoc = {
  id: "zcode",
  name: "ZCode",
  sources: [
    {
      label: "ZCode hooks documentation",
      url: "https://zcode.z.ai/en/docs/hooks",
    },
    {
      label: "ZCode safety confirmation documentation",
      url: "https://zcode.z.ai/en/docs/safety-confirmation",
    },
  ],
  commonFields: [
    {
      name: "session_id",
      type: "string",
      description: "ZCode session identifier",
    },
    {
      name: "transcript_path",
      type: "string",
      description: "Temporary JSONL transcript path exposed for the hook run",
    },
    {
      name: "cwd",
      type: "string",
      description: "Current workspace directory",
    },
    {
      name: "permission_mode",
      type: "string",
      description: "Active ZCode permission mode",
    },
  ],
  notes: [
    "User hooks live in ~/.zcode/cli/config.json under hooks.events and require hooks.enabled=true.",
    "ZCode snapshots hook configuration at session startup. Start a new session after installing, removing, or editing hooks.",
    "agent-ebpf-filter installs async command hooks with empty stdout so native ZCode permission decisions remain unchanged; cgroup/BPF-LSM policies remain the OS-level enforcement boundary.",
    "The relay forwards telemetry to /hooks/event with a per-hook secret and uses its parent PID for kernel-event correlation when the ZCode payload has no PID.",
  ],
  events: [
    {
      name: "SessionStart",
      description: "New session initialization",
      matcher: "startup|clear|compact",
      fields: [
        { name: "source", type: "string", description: "Session start source" },
        { name: "agent_type", type: "string", description: "Optional active agent type" },
        { name: "model", type: "string", description: "Optional model identifier" },
      ],
    },
    {
      name: "UserPromptSubmit",
      description: "Before the user prompt is sent to the main model",
      fields: [
        { name: "prompt", type: "string", description: "Submitted user prompt; agent-ebpf stores only safe metadata" },
      ],
    },
    {
      name: "PreToolUse",
      description: "Before a tool executes",
      matcher: "tool name; * matches all",
      fields: [
        { name: "tool_name", type: "string", description: "Requested tool name" },
        { name: "tool_input", type: "object", description: "Structured tool arguments" },
        { name: "tool_use_id", type: "string", description: "Tool call identifier" },
      ],
    },
    {
      name: "PermissionRequest",
      description: "When ZCode would ask the user for a tool permission decision",
      matcher: "tool name; * matches all",
      fields: [
        { name: "tool_name", type: "string", description: "Requested tool name" },
        { name: "tool_input", type: "object", description: "Structured tool arguments" },
        { name: "permission_suggestions", type: "object", description: "Optional permission suggestions" },
      ],
    },
    {
      name: "PostToolUse",
      description: "After a tool succeeds",
      matcher: "tool name; * matches all",
      fields: [
        { name: "tool_response", type: "object", description: "Structured tool response" },
        { name: "tool_use_id", type: "string", description: "Completed tool call identifier" },
      ],
    },
    {
      name: "PostToolUseFailure",
      description: "After a tool fails",
      matcher: "tool name; * matches all",
      fields: [
        { name: "error", type: "string", description: "Tool failure description" },
        { name: "is_interrupt", type: "boolean", description: "Whether the failure was an interruption" },
      ],
    },
    {
      name: "Stop",
      description: "When the main model is about to finish",
      fields: [
        { name: "stop_hook_active", type: "boolean", description: "Whether a stop hook continuation is active" },
        { name: "last_assistant_message", type: "string", description: "Last assistant message; agent-ebpf stores only safe metadata" },
      ],
    },
  ],
};
