import type { HookCliDoc } from "./types";

export const copilotHook: HookCliDoc = {
  id: "copilot",
  name: "GitHub Copilot CLI",
  sources: [
    {
      label: "GitHub Copilot CLI hooks reference",
      url: "https://docs.github.com/en/enterprise-cloud/latest/copilot/reference/copilot-cli-reference/cli-command-reference",
    },
    {
      label: "GitHub Copilot CLI hooks tutorial",
      url: "https://docs.github.com/en/copilot/tutorials/copilot-cli-hooks",
    },
  ],
  notes: [
    "Copilot CLI supports camelCase event names and PascalCase VS Code-compatible aliases.",
    "This page uses the CLI-native camelCase names by default.",
  ],
  events: [
    {
      name: "sessionStart",
      aliases: ["SessionStart"],
      description: "When a new or resumed session begins",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description:
            "Unix ms in camelCase form, ISO string in PascalCase form",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "source",
          type: "string",
          description: "startup, resume, or new",
        },
        {
          name: "initialPrompt / initial_prompt",
          type: "string",
          description: "Optional initial prompt",
        },
      ],
    },
    {
      name: "sessionEnd",
      aliases: ["SessionEnd"],
      description: "When the session terminates",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "reason",
          type: "string",
          description: "complete, error, abort, timeout, or user_exit",
        },
      ],
    },
    {
      name: "userPromptSubmitted",
      aliases: ["UserPromptSubmit"],
      description: "When the user submits a prompt",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "prompt",
          type: "string",
          description: "Submitted prompt text",
        },
      ],
    },
    {
      name: "preToolUse",
      aliases: ["PreToolUse"],
      description: "Before a tool executes",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "toolName / tool_name",
          type: "string",
          description: "Tool name",
        },
        {
          name: "toolArgs / tool_input",
          type: "unknown",
          description: "Tool arguments",
        },
      ],
    },
    {
      name: "postToolUse",
      aliases: ["PostToolUse"],
      description: "After a tool completes successfully",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "toolName / tool_name",
          type: "string",
          description: "Tool name",
        },
        {
          name: "toolArgs / tool_input",
          type: "unknown",
          description: "Tool arguments",
        },
        {
          name: "toolResult / tool_result",
          type: "object",
          description: "Tool result payload",
        },
      ],
    },
    {
      name: "postToolUseFailure",
      aliases: ["PostToolUseFailure"],
      description: "After a tool completes with failure",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "toolName / tool_name",
          type: "string",
          description: "Tool name",
        },
        {
          name: "toolArgs / tool_input",
          type: "unknown",
          description: "Tool arguments",
        },
        { name: "error", type: "string", description: "Failure string" },
      ],
    },
    {
      name: "agentStop",
      aliases: ["Stop"],
      description: "When the main agent finishes a turn",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "transcriptPath / transcript_path",
          type: "string",
          description: "Transcript path",
        },
        {
          name: "stopReason / stop_reason",
          type: "string",
          description: "Current stop reason, typically end_turn",
        },
      ],
    },
    {
      name: "subagentStart",
      aliases: ["SubagentStart"],
      description: "When a subagent is spawned",
      fields: [
        { name: "sessionId", type: "string", description: "Session id" },
        {
          name: "timestamp",
          type: "number",
          description: "Unix timestamp in milliseconds",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "transcriptPath",
          type: "string",
          description: "Transcript path",
        },
        { name: "agentName", type: "string", description: "Subagent name" },
        {
          name: "agentDisplayName",
          type: "string",
          description: "Optional display name",
        },
        {
          name: "agentDescription",
          type: "string",
          description: "Optional description",
        },
      ],
    },
    {
      name: "subagentStop",
      aliases: ["SubagentStop"],
      description: "When a subagent completes",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "transcriptPath / transcript_path",
          type: "string",
          description: "Transcript path",
        },
        {
          name: "agentName / agent_name",
          type: "string",
          description: "Subagent name",
        },
        {
          name: "agentDisplayName / agent_display_name",
          type: "string",
          description: "Optional display name",
        },
        {
          name: "stopReason / stop_reason",
          type: "string",
          description: "Stop reason, typically end_turn",
        },
      ],
    },
    {
      name: "preCompact",
      aliases: ["PreCompact"],
      description: "Before context compaction begins",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "transcriptPath / transcript_path",
          type: "string",
          description: "Transcript path",
        },
        { name: "trigger", type: "string", description: "manual or auto" },
        {
          name: "customInstructions / custom_instructions",
          type: "string",
          description: "Compaction instructions",
        },
      ],
    },
    {
      name: "permissionRequest",
      aliases: ["PermissionRequest"],
      description: "Before a permission dialog is shown",
      matcher: "toolName",
      notes: [
        "The CLI hooks reference documents decision control for this event; the payload schema is aligned with tool-level permission checks.",
      ],
    },
    {
      name: "errorOccurred",
      aliases: ["ErrorOccurred"],
      description: "When an error occurs during execution",
      fields: [
        {
          name: "sessionId / session_id",
          type: "string",
          description: "Session id",
        },
        {
          name: "timestamp",
          type: "number | string",
          description: "Unix ms or ISO string",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "error",
          type: "object",
          description: "Error object with message, name, and optional stack",
        },
        {
          name: "errorContext / error_context",
          type: "string",
          description: "model_call, tool_execution, system, or user_input",
        },
        {
          name: "recoverable",
          type: "boolean",
          description: "Whether recovery is possible",
        },
      ],
    },
    {
      name: "notification",
      aliases: ["Notification"],
      description: "When the CLI emits a system notification",
      matcher: "notification_type",
      fields: [
        { name: "sessionId", type: "string", description: "Session id" },
        {
          name: "timestamp",
          type: "number",
          description: "Unix timestamp in milliseconds",
        },
        { name: "cwd", type: "string", description: "Working directory" },
        {
          name: "hook_event_name",
          type: "string",
          description: "Notification",
        },
        { name: "message", type: "string", description: "Notification text" },
        {
          name: "title",
          type: "string",
          description: "Optional short title",
        },
        {
          name: "notification_type",
          type: "string",
          description:
            "shell_completed, shell_detached_completed, agent_completed, agent_idle, permission_prompt, or elicitation_dialog",
        },
      ],
    },
  ],
};
