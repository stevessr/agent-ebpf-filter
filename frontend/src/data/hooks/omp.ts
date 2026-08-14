import type { HookCliDoc } from "./types";

export const ompHook: HookCliDoc = {
  id: "omp",
  name: "Oh My Pi",
  sources: [
    {
      label: "Oh My Pi configuration",
      url: "https://github.com/can1357/oh-my-pi/blob/main/docs/config-usage.md",
    },
    {
      label: "Oh My Pi hook capability",
      url: "https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/capability/hook.ts",
    },
    {
      label: "Oh My Pi extension modules",
      url: "https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/capability/extension-module.ts",
    },
    {
      label: "Oh My Pi native discovery",
      url: "https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/discovery/builtin.ts",
    },
  ],
  commonFields: [
    { name: "session_id", type: "string", description: "Current Oh My Pi session id" },
    { name: "cwd", type: "string", description: "Working directory" },
    { name: "tool_name", type: "string", description: "Tool name for tool events" },
    { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
  ],
  notes: [
    "Oh My Pi discovers TypeScript extension modules from the active agent directory's extensions/ folder.",
    "Oh My Pi's hooks/pre and hooks/post directories are shell-script hooks; this integration uses the TypeScript extension API for session and tool events.",
    "With OMP_PROFILE set (or legacy PI_PROFILE when OMP_PROFILE is unset), the backend resolves the profile-scoped agent directory below the configured .omp root.",
    "PI_CONFIG_DIR changes the .omp root; PI_CODING_AGENT_DIR relocates the default agent directory and is ignored for named profiles.",
    "The generated integration emits only session metadata and tool-call metadata; prompts and tool results are not forwarded as raw text.",
  ],
  events: [
    {
      name: "session_start",
      description: "When an Oh My Pi session starts or resumes",
      fields: [
        { name: "session_id", type: "string", description: "Current session id" },
        { name: "cwd", type: "string", description: "Working directory" },
        { name: "reason", type: "string", description: "Session start reason when supplied" },
      ],
    },
    {
      name: "tool_call",
      description: "Before Oh My Pi invokes a tool",
      fields: [
        { name: "tool_name", type: "string", description: "Tool being invoked" },
        { name: "tool_input", type: "object", description: "Tool input supplied by Oh My Pi" },
        { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
      ],
    },
    {
      name: "tool_result",
      description: "After an Oh My Pi tool invocation completes",
      fields: [
        { name: "tool_name", type: "string", description: "Tool that completed" },
        { name: "tool_call_id", type: "string", description: "Tool-call identifier" },
        { name: "tool_result.is_error", type: "boolean", description: "Whether Oh My Pi reported an error" },
      ],
      notes: ["The integration intentionally sends only the error bit, not raw tool output."],
    },
  ],
};
