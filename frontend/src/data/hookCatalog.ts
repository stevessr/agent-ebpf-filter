export interface HookFieldDoc {
  name: string;
  type?: string;
  description: string;
}

export interface HookEventDoc {
  name: string;
  aliases?: string[];
  description: string;
  matcher?: string;
  fields?: HookFieldDoc[];
  notes?: string[];
}

export interface HookSourceDoc {
  label: string;
  url: string;
}

export interface HookCliDoc {
  id: string;
  name: string;
  sources: HookSourceDoc[];
  commonFields?: HookFieldDoc[];
  notes?: string[];
  events: HookEventDoc[];
}

export const hookCatalog: Record<string, HookCliDoc> = {
  claude: {
    id: 'claude',
    name: 'Claude Code',
    sources: [
      {
        label: 'Anthropic Claude Code Hooks reference',
        url: 'https://code.claude.com/docs/en/hooks',
      },
    ],
    commonFields: [
      { name: 'session_id', type: 'string', description: 'Current Claude session id' },
      { name: 'transcript_path', type: 'string | null', description: 'Transcript path for the session' },
      { name: 'cwd', type: 'string', description: 'Current working directory' },
      { name: 'hook_event_name', type: 'string', description: 'Current hook event name' },
    ],
    notes: [
      'Claude Code supports many more hook events than the current backend autoinstall target.',
      'For PreToolUse and PostToolUse, tool-specific fields under tool_input/tool_response depend on the tool being invoked.',
    ],
    events: [
      { name: 'SessionStart', description: 'When a session begins or resumes', matcher: 'source', fields: [{ name: 'source', type: 'string', description: 'startup, resume, clear, compact' }, { name: 'model', type: 'string', description: 'Active Claude model id' }, { name: 'agent_type', type: 'string', description: 'Present when starting Claude with a named agent' }] },
      { name: 'InstructionsLoaded', description: 'When CLAUDE.md or .claude/rules files are loaded' },
      { name: 'UserPromptSubmit', description: 'Before Claude processes a user prompt', fields: [{ name: 'prompt', type: 'string', description: 'Submitted prompt text' }] },
      { name: 'UserPromptExpansion', description: 'When a slash command expands into a prompt' },
      { name: 'PreToolUse', description: 'Before a tool call executes', matcher: 'tool name', fields: [{ name: 'tool_name', type: 'string', description: 'Tool being invoked' }, { name: 'tool_input', type: 'object', description: 'Tool arguments; schema depends on the tool' }, { name: 'tool_use_id', type: 'string', description: 'Unique tool call id' }] },
      { name: 'PermissionRequest', description: 'When a permission dialog is about to be shown', matcher: 'tool name', fields: [{ name: 'tool_name', type: 'string', description: 'Tool name' }, { name: 'tool_input', type: 'object', description: 'Tool arguments' }, { name: 'permission_suggestions', type: 'array', description: 'Optional “always allow” suggestions shown in the dialog' }] },
      { name: 'PermissionDenied', description: 'When the auto mode classifier denies a tool call' },
      { name: 'PostToolUse', description: 'After a tool call succeeds', matcher: 'tool name', fields: [{ name: 'tool_name', type: 'string', description: 'Executed tool name' }, { name: 'tool_input', type: 'object', description: 'Arguments sent to the tool' }, { name: 'tool_response', type: 'object', description: 'Successful tool response payload' }, { name: 'tool_use_id', type: 'string', description: 'Unique tool call id' }] },
      { name: 'PostToolUseFailure', description: 'After a tool call fails', matcher: 'tool name' },
      { name: 'Notification', description: 'When Claude Code emits a notification', matcher: 'notification_type', fields: [{ name: 'message', type: 'string', description: 'Notification text' }, { name: 'title', type: 'string', description: 'Optional notification title' }, { name: 'notification_type', type: 'string', description: 'Which notification fired' }] },
      { name: 'SubagentStart', description: 'When a subagent is spawned', fields: [{ name: 'agent_id', type: 'string', description: 'Unique subagent id' }, { name: 'agent_type', type: 'string', description: 'Agent name / matcher value' }] },
      { name: 'SubagentStop', description: 'When a subagent finishes', fields: [{ name: 'stop_hook_active', type: 'boolean', description: 'Whether continuation is already active because of a stop hook' }, { name: 'agent_id', type: 'string', description: 'Unique subagent id' }, { name: 'agent_type', type: 'string', description: 'Agent name / matcher value' }, { name: 'agent_transcript_path', type: 'string', description: 'Subagent transcript path' }, { name: 'last_assistant_message', type: 'string', description: 'Last assistant message text' }] },
      { name: 'TaskCreated', description: 'When a task is being created', fields: [{ name: 'task_id', type: 'string', description: 'Task id' }, { name: 'task_subject', type: 'string', description: 'Task subject' }, { name: 'task_description', type: 'string', description: 'Optional task description' }, { name: 'teammate_name', type: 'string', description: 'Optional teammate name' }, { name: 'team_name', type: 'string', description: 'Optional team name' }] },
      { name: 'TaskCompleted', description: 'When a task is marked completed' },
      { name: 'Stop', description: 'When Claude finishes responding', fields: [{ name: 'stop_hook_active', type: 'boolean', description: 'True when Claude is already continuing because of a stop hook' }, { name: 'last_assistant_message', type: 'string', description: 'Final assistant response text' }] },
      { name: 'StopFailure', description: 'When a turn ends because of an API error', fields: [{ name: 'error', type: 'string', description: 'Error type / matcher value' }, { name: 'error_details', type: 'object', description: 'Optional structured error details' }, { name: 'last_assistant_message', type: 'string', description: 'Optional latest assistant message' }] },
      { name: 'TeammateIdle', description: 'When an agent-team teammate is about to go idle', fields: [{ name: 'teammate_name', type: 'string', description: 'Teammate identifier' }, { name: 'team_name', type: 'string', description: 'Agent team name' }] },
      { name: 'ConfigChange', description: 'When a configuration file changes during a session' },
      { name: 'CwdChanged', description: 'When the working directory changes', fields: [{ name: 'old_cwd', type: 'string', description: 'Previous working directory' }, { name: 'new_cwd', type: 'string', description: 'New working directory' }] },
      { name: 'FileChanged', description: 'When a watched file changes on disk', fields: [{ name: 'file_path', type: 'string', description: 'Absolute path to the changed file' }, { name: 'event', type: 'string', description: 'change, add, or unlink' }] },
      { name: 'WorktreeCreate', description: 'When a worktree is about to be created' },
      { name: 'WorktreeRemove', description: 'When a worktree is removed', fields: [{ name: 'worktree_path', type: 'string', description: 'Absolute path to the worktree' }] },
      { name: 'PreCompact', description: 'Before context compaction starts', matcher: 'trigger', fields: [{ name: 'trigger', type: 'string', description: 'manual or auto' }, { name: 'custom_instructions', type: 'string', description: 'User-provided compact instructions' }] },
      { name: 'PostCompact', description: 'After context compaction completes', fields: [{ name: 'trigger', type: 'string', description: 'manual or auto' }, { name: 'compact_summary', type: 'string', description: 'Generated compaction summary' }] },
      { name: 'SessionEnd', description: 'When a session terminates', fields: [{ name: 'reason', type: 'string', description: 'Why the session ended' }] },
      { name: 'Elicitation', description: 'When an MCP server requests user input', fields: [{ name: 'mcp_server_name', type: 'string', description: 'MCP server name' }, { name: 'message', type: 'string', description: 'Displayed prompt' }, { name: 'mode', type: 'string', description: 'Optional elicitation mode' }, { name: 'url', type: 'string', description: 'Optional related URL' }, { name: 'elicitation_id', type: 'string', description: 'Optional elicitation id' }, { name: 'requested_schema', type: 'object', description: 'Optional requested input schema' }] },
      { name: 'ElicitationResult', description: 'After a user answers an MCP elicitation' },
    ],
  },
  gemini: {
    id: 'gemini',
    name: 'Gemini CLI',
    sources: [
      {
        label: 'Gemini CLI hooks guide',
        url: 'https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/writing-hooks.md',
      },
      {
        label: 'Gemini CLI configuration reference',
        url: 'https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md',
      },
    ],
    notes: [
      'The current official writing guide is example-driven; some events are documented as configurable but their full input schema is not enumerated there.',
      'Fields below for Gemini are limited to what the official examples explicitly use.',
    ],
    events: [
      { name: 'SessionStart', description: 'When a session starts', matcher: 'source', notes: ['Official examples show matcher values like startup; the current writing guide does not enumerate extra input fields for this event.'] },
      { name: 'SessionEnd', description: 'When a session ends', matcher: 'source', notes: ['The configuration reference documents the event, but the writing guide does not list its input schema.'] },
      { name: 'BeforeAgent', description: 'Before the agent loop starts', matcher: '*', notes: ['The official example injects additional context here but does not consume documented input fields.'] },
      { name: 'AfterAgent', description: 'After the agent loop completes', matcher: '*', fields: [{ name: 'prompt_response', type: 'string', description: 'Agent response text used in the official validation example' }] },
      { name: 'BeforeModel', description: 'Before an LLM request is sent', matcher: '*', notes: ['The configuration reference documents this event; field-level schema is not enumerated in the current hooks guide.'] },
      { name: 'AfterModel', description: 'After an LLM response is received', matcher: '*', fields: [{ name: 'llm_request', type: 'object', description: 'LLM request payload' }, { name: 'llm_response', type: 'object', description: 'LLM response payload' }] },
      { name: 'BeforeToolSelection', description: 'Before available tools are selected', matcher: '*', fields: [{ name: 'llm_request', type: 'object', description: 'Request payload containing messages' }, { name: 'llm_request.messages', type: 'array', description: 'Messages array used to infer tool intent' }] },
      { name: 'BeforeTool', description: 'Before a tool runs', matcher: 'tool name', fields: [{ name: 'tool_name', type: 'string', description: 'Tool name used in the quick-start logger' }, { name: 'tool_input', type: 'object', description: 'Tool arguments; examples read fields like content and new_string' }] },
      { name: 'Notification', description: 'When Gemini CLI emits a notification', matcher: 'notification type', notes: ['The configuration reference documents Notification, but the current hooks guide does not enumerate its input schema.'] },
      { name: 'PreCompress', aliases: ['PreCompress'], description: 'Before chat history compression', matcher: 'trigger', notes: ['The configuration reference documents this event, but the current hooks guide does not enumerate its input schema.'] },
    ],
  },
  codex: {
    id: 'codex',
    name: 'Codex',
    sources: [
      {
        label: 'OpenAI Codex hooks docs',
        url: 'https://developers.openai.com/codex/hooks',
      },
    ],
    commonFields: [
      { name: 'session_id', type: 'string', description: 'Current session / thread id' },
      { name: 'transcript_path', type: 'string | null', description: 'Transcript path, if any' },
      { name: 'cwd', type: 'string', description: 'Working directory' },
      { name: 'hook_event_name', type: 'string', description: 'Current hook event name' },
      { name: 'model', type: 'string', description: 'Active model slug' },
    ],
    notes: [
      'Hooks are behind the [features].codex_hooks = true feature flag in ~/.codex/config.toml.',
      'Codex discovers hooks.json next to active config layers; the global user-level file is ~/.codex/hooks.json.',
      'The current Codex runtime only emits Bash for PreToolUse / PostToolUse / PermissionRequest matcher filtering.',
      'Several parsed output fields are documented as not yet supported and fail open today.',
    ],
    events: [
      { name: 'SessionStart', description: 'When a Codex session starts or resumes', matcher: 'source', fields: [{ name: 'source', type: 'string', description: 'startup or resume' }] },
      { name: 'PreToolUse', description: 'Before a Bash command runs', matcher: 'tool_name', fields: [{ name: 'turn_id', type: 'string', description: 'Active Codex turn id' }, { name: 'tool_name', type: 'string', description: 'Currently always Bash' }, { name: 'tool_use_id', type: 'string', description: 'Tool-call id for this invocation' }, { name: 'tool_input.command', type: 'string', description: 'Shell command Codex is about to run' }], notes: ['Official docs say allow/ask are parsed but not supported yet; deny is the reliable current control path.'] },
      { name: 'PermissionRequest', description: 'When Codex is about to ask for approval', matcher: 'tool_name', fields: [{ name: 'turn_id', type: 'string', description: 'Active Codex turn id' }, { name: 'tool_name', type: 'string', description: 'Currently always Bash' }, { name: 'tool_input.command', type: 'string', description: 'Shell command tied to the approval request' }, { name: 'tool_input.description', type: 'string | null', description: 'Human-readable approval reason, when present' }] },
      { name: 'PostToolUse', description: 'After a Bash command runs', matcher: 'tool_name', fields: [{ name: 'turn_id', type: 'string', description: 'Active Codex turn id' }, { name: 'tool_name', type: 'string', description: 'Currently always Bash' }, { name: 'tool_use_id', type: 'string', description: 'Tool-call id for this invocation' }, { name: 'tool_input.command', type: 'string', description: 'Shell command Codex just ran' }, { name: 'tool_response', type: 'JSON', description: 'Bash tool output payload; often a JSON string today' }] },
      { name: 'UserPromptSubmit', description: 'Before a user prompt is sent', fields: [{ name: 'turn_id', type: 'string', description: 'Active Codex turn id' }, { name: 'prompt', type: 'string', description: 'Prompt about to be sent' }] },
      { name: 'Stop', description: 'When Codex finishes a turn', fields: [{ name: 'turn_id', type: 'string', description: 'Active Codex turn id' }, { name: 'stop_hook_active', type: 'boolean', description: 'Whether Stop has already continued this turn' }, { name: 'last_assistant_message', type: 'string | null', description: 'Latest assistant message text' }] },
    ],
  },
  kiro: {
    id: 'kiro',
    name: 'Kiro CLI',
    sources: [
      {
        label: 'Kiro CLI hooks docs',
        url: 'https://kiro.dev/docs/cli/hooks/',
      },
      {
        label: 'Kiro agent configuration reference',
        url: 'https://kiro.dev/docs/cli/custom-agents/configuration-reference/',
      },
      {
        label: 'Kiro custom agent creation docs',
        url: 'https://kiro.dev/docs/cli/custom-agents/creating/',
      },
    ],
    commonFields: [
      { name: 'session_id', type: 'string', description: 'Current Kiro session UUID' },
      { name: 'cwd', type: 'string', description: 'Current working directory' },
      { name: 'hook_event_name', type: 'string', description: 'Current hook event name' },
    ],
    notes: [
      'Kiro CLI defines hooks in agent configuration JSON files rather than a single global hooks file.',
      'This app creates and manages a derived agent at ~/.kiro/agents/agent-ebpf-hook.json, cloned from kiro_default, then points chat.defaultAgent at it during install.',
      'The previous Kiro default agent is restored on uninstall.',
      'For tool matchers, Kiro accepts canonical internal tool names like execute_bash / fs_read / fs_write / use_aws, aliases like shell / read / write / aws, namespaced MCP tools like @postgres/query, @builtin, or *.',
    ],
    events: [
      { name: 'agentSpawn', description: 'Runs when the agent is activated', notes: ['No tool context is provided. Successful STDOUT is added to the agent context.'] },
      { name: 'userPromptSubmit', description: 'Runs when the user submits a prompt', fields: [{ name: 'prompt', type: 'string', description: 'Submitted prompt text' }], notes: ['Successful STDOUT is added to the conversation context.'] },
      { name: 'preToolUse', description: 'Runs before a tool executes', matcher: 'execute_bash / shell, fs_read / read, fs_write / write, use_aws / aws, @mcp/tool, @builtin, *', fields: [{ name: 'tool_name', type: 'string', description: 'Tool name or alias being executed' }, { name: 'tool_input', type: 'object', description: 'Tool-specific input payload' }], notes: ['Exit code 2 blocks the tool and returns STDERR to the LLM. No matcher means all tools.'] },
      { name: 'postToolUse', description: 'Runs after a tool executes', matcher: 'execute_bash / shell, fs_read / read, fs_write / write, use_aws / aws, @mcp/tool, @builtin, *', fields: [{ name: 'tool_name', type: 'string', description: 'Executed tool name' }, { name: 'tool_input', type: 'object', description: 'Tool-specific input payload' }, { name: 'tool_response', type: 'object', description: 'Tool execution result payload' }] },
      { name: 'stop', description: 'Runs when the assistant finishes responding', notes: ['Stop hooks do not use matchers because they are not tied to a specific tool.'] },
    ],
  },
  copilot: {
    id: 'copilot',
    name: 'GitHub Copilot CLI',
    sources: [
      {
        label: 'GitHub Copilot CLI hooks reference',
        url: 'https://docs.github.com/en/enterprise-cloud/latest/copilot/reference/copilot-cli-reference/cli-command-reference',
      },
      {
        label: 'GitHub Copilot CLI hooks tutorial',
        url: 'https://docs.github.com/en/copilot/tutorials/copilot-cli-hooks',
      },
    ],
    notes: [
      'Copilot CLI supports camelCase event names and PascalCase VS Code-compatible aliases.',
      'This page uses the CLI-native camelCase names by default.',
    ],
    events: [
      { name: 'sessionStart', aliases: ['SessionStart'], description: 'When a new or resumed session begins', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms in camelCase form, ISO string in PascalCase form' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'source', type: 'string', description: 'startup, resume, or new' }, { name: 'initialPrompt / initial_prompt', type: 'string', description: 'Optional initial prompt' }] },
      { name: 'sessionEnd', aliases: ['SessionEnd'], description: 'When the session terminates', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'reason', type: 'string', description: 'complete, error, abort, timeout, or user_exit' }] },
      { name: 'userPromptSubmitted', aliases: ['UserPromptSubmit'], description: 'When the user submits a prompt', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'prompt', type: 'string', description: 'Submitted prompt text' }] },
      { name: 'preToolUse', aliases: ['PreToolUse'], description: 'Before a tool executes', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'toolName / tool_name', type: 'string', description: 'Tool name' }, { name: 'toolArgs / tool_input', type: 'unknown', description: 'Tool arguments' }] },
      { name: 'postToolUse', aliases: ['PostToolUse'], description: 'After a tool completes successfully', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'toolName / tool_name', type: 'string', description: 'Tool name' }, { name: 'toolArgs / tool_input', type: 'unknown', description: 'Tool arguments' }, { name: 'toolResult / tool_result', type: 'object', description: 'Tool result payload' }] },
      { name: 'postToolUseFailure', aliases: ['PostToolUseFailure'], description: 'After a tool completes with failure', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'toolName / tool_name', type: 'string', description: 'Tool name' }, { name: 'toolArgs / tool_input', type: 'unknown', description: 'Tool arguments' }, { name: 'error', type: 'string', description: 'Failure string' }] },
      { name: 'agentStop', aliases: ['Stop'], description: 'When the main agent finishes a turn', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'transcriptPath / transcript_path', type: 'string', description: 'Transcript path' }, { name: 'stopReason / stop_reason', type: 'string', description: 'Current stop reason, typically end_turn' }] },
      { name: 'subagentStart', aliases: ['SubagentStart'], description: 'When a subagent is spawned', fields: [{ name: 'sessionId', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number', description: 'Unix timestamp in milliseconds' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'transcriptPath', type: 'string', description: 'Transcript path' }, { name: 'agentName', type: 'string', description: 'Subagent name' }, { name: 'agentDisplayName', type: 'string', description: 'Optional display name' }, { name: 'agentDescription', type: 'string', description: 'Optional description' }] },
      { name: 'subagentStop', aliases: ['SubagentStop'], description: 'When a subagent completes', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'transcriptPath / transcript_path', type: 'string', description: 'Transcript path' }, { name: 'agentName / agent_name', type: 'string', description: 'Subagent name' }, { name: 'agentDisplayName / agent_display_name', type: 'string', description: 'Optional display name' }, { name: 'stopReason / stop_reason', type: 'string', description: 'Stop reason, typically end_turn' }] },
      { name: 'preCompact', aliases: ['PreCompact'], description: 'Before context compaction begins', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'transcriptPath / transcript_path', type: 'string', description: 'Transcript path' }, { name: 'trigger', type: 'string', description: 'manual or auto' }, { name: 'customInstructions / custom_instructions', type: 'string', description: 'Compaction instructions' }] },
      { name: 'permissionRequest', aliases: ['PermissionRequest'], description: 'Before a permission dialog is shown', matcher: 'toolName', notes: ['The CLI hooks reference documents decision control for this event; the payload schema is aligned with tool-level permission checks.'] },
      { name: 'errorOccurred', aliases: ['ErrorOccurred'], description: 'When an error occurs during execution', fields: [{ name: 'sessionId / session_id', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number | string', description: 'Unix ms or ISO string' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'error', type: 'object', description: 'Error object with message, name, and optional stack' }, { name: 'errorContext / error_context', type: 'string', description: 'model_call, tool_execution, system, or user_input' }, { name: 'recoverable', type: 'boolean', description: 'Whether recovery is possible' }] },
      { name: 'notification', aliases: ['Notification'], description: 'When the CLI emits a system notification', matcher: 'notification_type', fields: [{ name: 'sessionId', type: 'string', description: 'Session id' }, { name: 'timestamp', type: 'number', description: 'Unix timestamp in milliseconds' }, { name: 'cwd', type: 'string', description: 'Working directory' }, { name: 'hook_event_name', type: 'string', description: 'Notification' }, { name: 'message', type: 'string', description: 'Notification text' }, { name: 'title', type: 'string', description: 'Optional short title' }, { name: 'notification_type', type: 'string', description: 'shell_completed, shell_detached_completed, agent_completed, agent_idle, permission_prompt, or elicitation_dialog' }] },
    ],
  },
};

export const getHookCliDoc = (hookId?: string | null) => {
  if (!hookId) return null;
  return hookCatalog[hookId] ?? null;
};

export const getHookEventDoc = (hookId: string | undefined, eventName: string | undefined) => {
  const cliDoc = getHookCliDoc(hookId);
  if (!cliDoc || !eventName) return null;
  const normalized = eventName.trim().toLowerCase();
  return cliDoc.events.find((event) => {
    if (event.name.toLowerCase() === normalized) return true;
    return (event.aliases || []).some((alias) => alias.toLowerCase() === normalized);
  }) ?? null;
};
