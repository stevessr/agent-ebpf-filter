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
      url: "https://zcode.z.ai/en/docs/safety-confirm",
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
    "The open-source ZCode runtime keeps camelCase fields as its canonical hook contract and also emits snake_case compatibility aliases for Claude-style integrations.",
    "User hooks live in ~/.zcode/cli/config.json under hooks.events and require hooks.enabled=true.",
    "ZCode snapshots hook configuration at session startup. Start a new session after installing, removing, or editing hooks.",
    "agent-ebpf-filter installs async command hooks with empty stdout so native ZCode permission decisions remain unchanged; cgroup/BPF-LSM policies remain the OS-level enforcement boundary.",
    "The relay forwards telemetry to /hooks/event with a per-hook secret and uses its parent PID for kernel-event correlation when the ZCode payload has no PID.",
  ],
  events: [
    {
      name: "SessionStart",
      description: "New session initialization",
      matcher: "startup|resume|clear|compact",
      fields: [
        { name: "source", type: "string", description: "Session start source: startup | resume | clear | compact" },
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
        { name: "tool_use_id", type: "string", description: "Compatibility alias for canonical toolCallId" },
        { name: "riskLevel", type: "string", description: "Tool risk classification from the ZCode runtime" },
        { name: "sideEffectScope", type: "string", description: "Optional side-effect scope for the tool" },
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
        { name: "requestId", type: "string", description: "Permission request identifier" },
        { name: "reason", type: "string", description: "Why approval is required" },
        { name: "riskLevel", type: "string", description: "Tool risk classification" },
        { name: "sideEffectScope", type: "string", description: "Optional side-effect scope for the tool" },
      ],
    },
    {
      name: "PostToolUse",
      description: "After a tool succeeds",
      matcher: "tool name; * matches all",
      fields: [
        { name: "tool_response", type: "object", description: "Structured tool response compatibility alias" },
        { name: "toolResultPreview", type: "string", description: "Canonical bounded tool-result preview" },
        { name: "tool_use_id", type: "string", description: "Compatibility alias for canonical toolCallId" },
      ],
    },
    {
      name: "PostToolUseFailure",
      description: "After a tool fails",
      matcher: "tool name; * matches all",
      fields: [
        { name: "error", type: "string", description: "Compatibility failure message" },
        { name: "error_details", type: "object", description: "Structured failure details including the error type" },
        { name: "is_interrupt", type: "boolean", description: "Whether the failure was an interruption" },
      ],
    },
    {
      name: "Stop",
      description: "When the main model is about to finish",
      fields: [
        { name: "stop_hook_active", type: "boolean", description: "Whether a stop hook continuation is active" },
        { name: "last_assistant_message", type: "string", description: "Compatibility alias for responseText/responsePreview; agent-ebpf stores only safe metadata" },
        { name: "toolCallCount", type: "number", description: "Number of tool calls in the completed turn" },
      ],
    },
  ],
};
