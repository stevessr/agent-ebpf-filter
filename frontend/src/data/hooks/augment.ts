import type { HookCliDoc } from "./types";

export const augmentHook: HookCliDoc = {
  id: "augment",
  name: "Augment (Auggie CLI)",
  sources: [
    {
      label: "Augment hooks reference",
      url: "https://docs.augmentcode.com/cli/hooks",
    },
    {
      label: "Augment hooks examples",
      url: "https://docs.augmentcode.com/cli/hooks-examples",
    },
  ],
  commonFields: [
    {
      name: "hook_event_name",
      type: "string",
      description: "PreToolUse, PostToolUse, Stop, SessionStart, SessionEnd",
    },
    {
      name: "conversation_id",
      type: "string",
      description: "Unique identifier for the current conversation",
    },
    {
      name: "workspace_roots",
      type: "string[]",
      description: "Workspace root directories (usually one path)",
    },
  ],
  notes: [
    "Hooks are configured in ~/.augment/settings.json (user-level) or /etc/augment/settings.json (system-wide).",
    "PreToolUse, PostToolUse, and Stop hooks run synchronously - the agent waits for them.",
    "Hook scripts must use a .sh file extension and be executable. Any interpreter is allowed via shebang.",
    "PreToolUse exit code 2 blocks the tool; stderr is shown to the agent. Other non-zero codes are non-blocking.",
    "Auggie uses `timeout` (milliseconds, default 60000) instead of `async`.",
    'Matchers support regex (e.g. "launch-process|view") and the special "mcp:" prefix for MCP tools.',
  ],
  events: [
    {
      name: "PreToolUse",
      description:
        'Runs before a tool executes; can block via exit code 2 or permissionDecision: "deny"',
      matcher: "tool name (regex)",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description:
            "Tool name (e.g. launch-process, str-replace-editor, save-file, view, web-fetch)",
        },
        {
          name: "tool_input",
          type: "object",
          description: "Tool arguments; schema depends on the tool",
        },
      ],
    },
    {
      name: "PostToolUse",
      description:
        "Runs after a tool completes; cannot block but can inject context",
      matcher: "tool name (regex)",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description: "Executed tool name",
        },
        {
          name: "tool_input",
          type: "object",
          description: "Arguments sent to the tool",
        },
        {
          name: "tool_output",
          type: "string?",
          description: "Tool output if successful",
        },
        {
          name: "tool_error",
          type: "string?",
          description: "Error message if the tool failed",
        },
        {
          name: "file_changes",
          type: "object[]?",
          description:
            "For save-file/str-replace-editor/remove-files: path, changeType, content, oldContent",
        },
      ],
    },
    {
      name: "Stop",
      description:
        'Runs when the agent stops responding; can block stop with decision: "block"',
      fields: [
        {
          name: "agent_stop_cause",
          type: "string",
          description: '"end_turn" or "interrupted"',
        },
      ],
    },
    {
      name: "SessionStart",
      description:
        "Runs when a new Auggie session begins; stdout is injected as agent context",
      notes: ["Session events do not use matchers."],
    },
    {
      name: "SessionEnd",
      description: "Runs when an Auggie session ends",
      notes: ["Session events do not use matchers."],
    },
  ],
};
