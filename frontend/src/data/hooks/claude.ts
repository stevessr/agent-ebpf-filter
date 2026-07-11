import type { HookCliDoc } from "./types";

export const claudeHook: HookCliDoc = {
  id: "claude",
  name: "Claude Code",
  sources: [
    {
      label: "Anthropic Claude Code Hooks reference",
      url: "https://code.claude.com/docs/en/hooks",
    },
  ],
  commonFields: [
    {
      name: "session_id",
      type: "string",
      description: "Current Claude session id",
    },
    {
      name: "transcript_path",
      type: "string | null",
      description: "Transcript path for the session",
    },
    { name: "cwd", type: "string", description: "Current working directory" },
    {
      name: "hook_event_name",
      type: "string",
      description: "Current hook event name",
    },
  ],
  notes: [
    "Claude Code supports many more hook events than the current backend autoinstall target.",
    "For PreToolUse and PostToolUse, tool-specific fields under tool_input/tool_response depend on the tool being invoked.",
  ],
  events: [
    {
      name: "SessionStart",
      description: "When a session begins or resumes",
      matcher: "source",
      fields: [
        {
          name: "source",
          type: "string",
          description: "startup, resume, clear, compact",
        },
        {
          name: "model",
          type: "string",
          description: "Active Claude model id",
        },
        {
          name: "agent_type",
          type: "string",
          description: "Present when starting Claude with a named agent",
        },
      ],
    },
    {
      name: "InstructionsLoaded",
      description: "When CLAUDE.md or .claude/rules files are loaded",
    },
    {
      name: "UserPromptSubmit",
      description: "Before Claude processes a user prompt",
      fields: [
        {
          name: "prompt",
          type: "string",
          description: "Submitted prompt text",
        },
      ],
    },
    {
      name: "UserPromptExpansion",
      description: "When a slash command expands into a prompt",
    },
    {
      name: "PreToolUse",
      description: "Before a tool call executes",
      matcher: "tool name",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description: "Tool being invoked",
        },
        {
          name: "tool_input",
          type: "object",
          description: "Tool arguments; schema depends on the tool",
        },
        {
          name: "tool_use_id",
          type: "string",
          description: "Unique tool call id",
        },
      ],
    },
    {
      name: "PermissionRequest",
      description: "When a permission dialog is about to be shown",
      matcher: "tool name",
      fields: [
        { name: "tool_name", type: "string", description: "Tool name" },
        { name: "tool_input", type: "object", description: "Tool arguments" },
        {
          name: "permission_suggestions",
          type: "array",
          description:
            "Optional “always allow” suggestions shown in the dialog",
        },
      ],
    },
    {
      name: "PermissionDenied",
      description: "When the auto mode classifier denies a tool call",
    },
    {
      name: "PostToolUse",
      description: "After a tool call succeeds",
      matcher: "tool name",
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
          name: "tool_response",
          type: "object",
          description: "Successful tool response payload",
        },
        {
          name: "tool_use_id",
          type: "string",
          description: "Unique tool call id",
        },
      ],
    },
    {
      name: "PostToolUseFailure",
      description: "After a tool call fails",
      matcher: "tool name",
    },
    {
      name: "Notification",
      description: "When Claude Code emits a notification",
      matcher: "notification_type",
      fields: [
        { name: "message", type: "string", description: "Notification text" },
        {
          name: "title",
          type: "string",
          description: "Optional notification title",
        },
        {
          name: "notification_type",
          type: "string",
          description: "Which notification fired",
        },
      ],
    },
    {
      name: "SubagentStart",
      description: "When a subagent is spawned",
      fields: [
        {
          name: "agent_id",
          type: "string",
          description: "Unique subagent id",
        },
        {
          name: "agent_type",
          type: "string",
          description: "Agent name / matcher value",
        },
      ],
    },
    {
      name: "SubagentStop",
      description: "When a subagent finishes",
      fields: [
        {
          name: "stop_hook_active",
          type: "boolean",
          description:
            "Whether continuation is already active because of a stop hook",
        },
        {
          name: "agent_id",
          type: "string",
          description: "Unique subagent id",
        },
        {
          name: "agent_type",
          type: "string",
          description: "Agent name / matcher value",
        },
        {
          name: "agent_transcript_path",
          type: "string",
          description: "Subagent transcript path",
        },
        {
          name: "last_assistant_message",
          type: "string",
          description: "Last assistant message text",
        },
      ],
    },
    {
      name: "TaskCreated",
      description: "When a task is being created",
      fields: [
        { name: "task_id", type: "string", description: "Task id" },
        { name: "task_subject", type: "string", description: "Task subject" },
        {
          name: "task_description",
          type: "string",
          description: "Optional task description",
        },
        {
          name: "teammate_name",
          type: "string",
          description: "Optional teammate name",
        },
        {
          name: "team_name",
          type: "string",
          description: "Optional team name",
        },
      ],
    },
    { name: "TaskCompleted", description: "When a task is marked completed" },
    {
      name: "Stop",
      description: "When Claude finishes responding",
      fields: [
        {
          name: "stop_hook_active",
          type: "boolean",
          description:
            "True when Claude is already continuing because of a stop hook",
        },
        {
          name: "last_assistant_message",
          type: "string",
          description: "Final assistant response text",
        },
      ],
    },
    {
      name: "StopFailure",
      description: "When a turn ends because of an API error",
      fields: [
        {
          name: "error",
          type: "string",
          description: "Error type / matcher value",
        },
        {
          name: "error_details",
          type: "object",
          description: "Optional structured error details",
        },
        {
          name: "last_assistant_message",
          type: "string",
          description: "Optional latest assistant message",
        },
      ],
    },
    {
      name: "TeammateIdle",
      description: "When an agent-team teammate is about to go idle",
      fields: [
        {
          name: "teammate_name",
          type: "string",
          description: "Teammate identifier",
        },
        { name: "team_name", type: "string", description: "Agent team name" },
      ],
    },
    {
      name: "ConfigChange",
      description: "When a configuration file changes during a session",
    },
    {
      name: "CwdChanged",
      description: "When the working directory changes",
      fields: [
        {
          name: "old_cwd",
          type: "string",
          description: "Previous working directory",
        },
        {
          name: "new_cwd",
          type: "string",
          description: "New working directory",
        },
      ],
    },
    {
      name: "FileChanged",
      description: "When a watched file changes on disk",
      fields: [
        {
          name: "file_path",
          type: "string",
          description: "Absolute path to the changed file",
        },
        {
          name: "event",
          type: "string",
          description: "change, add, or unlink",
        },
      ],
    },
    {
      name: "WorktreeCreate",
      description: "When a worktree is about to be created",
    },
    {
      name: "WorktreeRemove",
      description: "When a worktree is removed",
      fields: [
        {
          name: "worktree_path",
          type: "string",
          description: "Absolute path to the worktree",
        },
      ],
    },
    {
      name: "PreCompact",
      description: "Before context compaction starts",
      matcher: "trigger",
      fields: [
        { name: "trigger", type: "string", description: "manual or auto" },
        {
          name: "custom_instructions",
          type: "string",
          description: "User-provided compact instructions",
        },
      ],
    },
    {
      name: "PostCompact",
      description: "After context compaction completes",
      fields: [
        { name: "trigger", type: "string", description: "manual or auto" },
        {
          name: "compact_summary",
          type: "string",
          description: "Generated compaction summary",
        },
      ],
    },
    {
      name: "SessionEnd",
      description: "When a session terminates",
      fields: [
        {
          name: "reason",
          type: "string",
          description: "Why the session ended",
        },
      ],
    },
    {
      name: "Elicitation",
      description: "When an MCP server requests user input",
      fields: [
        {
          name: "mcp_server_name",
          type: "string",
          description: "MCP server name",
        },
        { name: "message", type: "string", description: "Displayed prompt" },
        {
          name: "mode",
          type: "string",
          description: "Optional elicitation mode",
        },
        { name: "url", type: "string", description: "Optional related URL" },
        {
          name: "elicitation_id",
          type: "string",
          description: "Optional elicitation id",
        },
        {
          name: "requested_schema",
          type: "object",
          description: "Optional requested input schema",
        },
      ],
    },
    {
      name: "ElicitationResult",
      description: "After a user answers an MCP elicitation",
    },
  ],
};
