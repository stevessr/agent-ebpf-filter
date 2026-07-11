import type { HookCliDoc } from "./types";

export const kiroHook: HookCliDoc = {
  id: "kiro",
  name: "Kiro CLI",
  sources: [
    {
      label: "Kiro CLI hooks docs",
      url: "https://kiro.dev/docs/cli/hooks/",
    },
    {
      label: "Kiro agent configuration reference",
      url: "https://kiro.dev/docs/cli/custom-agents/configuration-reference/",
    },
    {
      label: "Kiro custom agent creation docs",
      url: "https://kiro.dev/docs/cli/custom-agents/creating/",
    },
  ],
  commonFields: [
    {
      name: "session_id",
      type: "string",
      description: "Current Kiro session UUID",
    },
    { name: "cwd", type: "string", description: "Current working directory" },
    {
      name: "hook_event_name",
      type: "string",
      description: "Current hook event name",
    },
  ],
  notes: [
    "Kiro CLI defines hooks in agent configuration JSON files rather than a single global hooks file.",
    "This app creates and manages a derived agent at ~/.kiro/agents/agent-ebpf-hook.json, cloned from kiro_default, then points chat.defaultAgent at it during install.",
    "The previous Kiro default agent is restored on uninstall.",
    "For tool matchers, Kiro accepts canonical internal tool names like execute_bash / fs_read / fs_write / use_aws, aliases like shell / read / write / aws, namespaced MCP tools like @postgres/query, @builtin, or *.",
  ],
  events: [
    {
      name: "agentSpawn",
      description: "Runs when the agent is activated",
      notes: [
        "No tool context is provided. Successful STDOUT is added to the agent context.",
      ],
    },
    {
      name: "userPromptSubmit",
      description: "Runs when the user submits a prompt",
      fields: [
        {
          name: "prompt",
          type: "string",
          description: "Submitted prompt text",
        },
      ],
      notes: ["Successful STDOUT is added to the conversation context."],
    },
    {
      name: "preToolUse",
      description: "Runs before a tool executes",
      matcher:
        "execute_bash / shell, fs_read / read, fs_write / write, use_aws / aws, @mcp/tool, @builtin, *",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description: "Tool name or alias being executed",
        },
        {
          name: "tool_input",
          type: "object",
          description: "Tool-specific input payload",
        },
      ],
      notes: [
        "Exit code 2 blocks the tool and returns STDERR to the LLM. No matcher means all tools.",
      ],
    },
    {
      name: "postToolUse",
      description: "Runs after a tool executes",
      matcher:
        "execute_bash / shell, fs_read / read, fs_write / write, use_aws / aws, @mcp/tool, @builtin, *",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description: "Executed tool name",
        },
        {
          name: "tool_input",
          type: "object",
          description: "Tool-specific input payload",
        },
        {
          name: "tool_response",
          type: "object",
          description: "Tool execution result payload",
        },
      ],
    },
    {
      name: "stop",
      description: "Runs when the assistant finishes responding",
      notes: [
        "Stop hooks do not use matchers because they are not tied to a specific tool.",
      ],
    },
  ],
};
