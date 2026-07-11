import type { HookCliDoc } from "./types";

export const codexHook: HookCliDoc = {
  id: "codex",
  name: "Codex",
  sources: [
    {
      label: "OpenAI Codex hooks docs",
      url: "https://developers.openai.com/codex/hooks",
    },
  ],
  commonFields: [
    {
      name: "session_id",
      type: "string",
      description: "Current session / thread id",
    },
    {
      name: "transcript_path",
      type: "string | null",
      description: "Transcript path, if any",
    },
    { name: "cwd", type: "string", description: "Working directory" },
    {
      name: "hook_event_name",
      type: "string",
      description: "Current hook event name",
    },
    { name: "model", type: "string", description: "Active model slug" },
  ],
  notes: [
    "Hooks are behind the [features].codex_hooks = true feature flag in ~/.codex/config.toml.",
    "Codex discovers hooks.json next to active config layers; the global user-level file is ~/.codex/hooks.json.",
    "The current Codex runtime only emits Bash for PreToolUse / PostToolUse / PermissionRequest matcher filtering.",
    "Several parsed output fields are documented as not yet supported and fail open today.",
  ],
  events: [
    {
      name: "SessionStart",
      description: "When a Codex session starts or resumes",
      matcher: "source",
      fields: [
        { name: "source", type: "string", description: "startup or resume" },
      ],
    },
    {
      name: "PreToolUse",
      description: "Before a Bash command runs",
      matcher: "tool_name",
      fields: [
        {
          name: "turn_id",
          type: "string",
          description: "Active Codex turn id",
        },
        {
          name: "tool_name",
          type: "string",
          description: "Currently always Bash",
        },
        {
          name: "tool_use_id",
          type: "string",
          description: "Tool-call id for this invocation",
        },
        {
          name: "tool_input.command",
          type: "string",
          description: "Shell command Codex is about to run",
        },
      ],
      notes: [
        "Official docs say allow/ask are parsed but not supported yet; deny is the reliable current control path.",
      ],
    },
    {
      name: "PermissionRequest",
      description: "When Codex is about to ask for approval",
      matcher: "tool_name",
      fields: [
        {
          name: "turn_id",
          type: "string",
          description: "Active Codex turn id",
        },
        {
          name: "tool_name",
          type: "string",
          description: "Currently always Bash",
        },
        {
          name: "tool_input.command",
          type: "string",
          description: "Shell command tied to the approval request",
        },
        {
          name: "tool_input.description",
          type: "string | null",
          description: "Human-readable approval reason, when present",
        },
      ],
    },
    {
      name: "PostToolUse",
      description: "After a Bash command runs",
      matcher: "tool_name",
      fields: [
        {
          name: "turn_id",
          type: "string",
          description: "Active Codex turn id",
        },
        {
          name: "tool_name",
          type: "string",
          description: "Currently always Bash",
        },
        {
          name: "tool_use_id",
          type: "string",
          description: "Tool-call id for this invocation",
        },
        {
          name: "tool_input.command",
          type: "string",
          description: "Shell command Codex just ran",
        },
        {
          name: "tool_response",
          type: "JSON",
          description: "Bash tool output payload; often a JSON string today",
        },
      ],
    },
    {
      name: "UserPromptSubmit",
      description: "Before a user prompt is sent",
      fields: [
        {
          name: "turn_id",
          type: "string",
          description: "Active Codex turn id",
        },
        {
          name: "prompt",
          type: "string",
          description: "Prompt about to be sent",
        },
      ],
    },
    {
      name: "Stop",
      description: "When Codex finishes a turn",
      fields: [
        {
          name: "turn_id",
          type: "string",
          description: "Active Codex turn id",
        },
        {
          name: "stop_hook_active",
          type: "boolean",
          description: "Whether Stop has already continued this turn",
        },
        {
          name: "last_assistant_message",
          type: "string | null",
          description: "Latest assistant message text",
        },
      ],
    },
  ],
};
