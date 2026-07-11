import type { HookCliDoc } from "./types";

export const antigravityHook: HookCliDoc = {
  id: "antigravity",
  name: "Antigravity CLI",
  sources: [
    {
      label: "Google Antigravity hooks documentation",
      url: "https://www.antigravity.google/docs/hooks",
    },
    {
      label: "Antigravity CLI plugins documentation",
      url: "https://www.antigravity.google/docs/cli-features",
    },
    {
      label: "Antigravity CLI settings documentation",
      url: "https://www.antigravity.google/docs/cli-using",
    },
  ],
  commonFields: [
    {
      name: "conversationId",
      type: "string",
      description: "Active Antigravity agent conversation UUID",
    },
    {
      name: "workspacePaths",
      type: "string[]",
      description: "Mounted workspace directories for the conversation",
    },
    {
      name: "transcriptPath",
      type: "string",
      description: "Path to transcript.jsonl for the conversation",
    },
    {
      name: "artifactDirectoryPath",
      type: "string",
      description: "Directory containing artifacts and screenshots",
    },
  ],
  notes: [
    "Antigravity CLI discovers hooks from plugin customization directories such as ~/.gemini/antigravity-cli/plugins/<plugin>/hooks.json.",
    "The backend installs an agent-ebpf plugin with plugin.json plus hooks.json instead of editing settings.json.",
    "Antigravity hook handlers must read camelCase JSON from stdin and return JSON on stdout; the generated relay script returns allow / empty responses after forwarding telemetry.",
    "PreToolUse and PostToolUse use regex matchers against tool names such as run_command, view_file, write_to_file, replace_file_content, and browser_.*; the default installed matcher is * for all tools.",
  ],
  events: [
    {
      name: "PreToolUse",
      description:
        "Before a tool executes; can allow, deny, ask, or force_ask",
      matcher: "tool name regex",
      fields: [
        {
          name: "toolCall.name",
          type: "string",
          description: "Tool name such as run_command",
        },
        {
          name: "toolCall.args",
          type: "object",
          description:
            "Tool arguments; run_command uses CommandLine, Cwd, WaitMsBeforeAsync, and related fields",
        },
        {
          name: "stepIdx",
          type: "number",
          description: "0-based step index in the trajectory",
        },
      ],
      notes: [
        "The generated relay returns decision=allow so agent-ebpf observes without changing Antigravity permissions.",
      ],
    },
    {
      name: "PostToolUse",
      description: "After a tool completes; returns an empty JSON object",
      matcher: "tool name regex",
      fields: [
        {
          name: "stepIdx",
          type: "number",
          description: "0-based completed step index",
        },
        {
          name: "error",
          type: "string",
          description: "Runtime error string if the tool failed",
        },
      ],
    },
    {
      name: "PreInvocation",
      description: "Before the model is called; can inject trajectory steps",
      fields: [
        {
          name: "invocationNum",
          type: "number",
          description: "Current model invocation sequence number",
        },
        {
          name: "initialNumSteps",
          type: "number",
          description: "Trajectory length before invocation",
        },
      ],
    },
    {
      name: "PostInvocation",
      description:
        "After tool calls finish; can inject steps or terminate / force_continue",
      fields: [
        {
          name: "invocationNum",
          type: "number",
          description: "Current model invocation sequence number",
        },
        {
          name: "initialNumSteps",
          type: "number",
          description: "Trajectory length before invocation",
        },
      ],
    },
    {
      name: "Stop",
      description: "When the execution loop terminates",
      fields: [
        {
          name: "executionNum",
          type: "number",
          description: "Execution attempt sequence number",
        },
        {
          name: "terminationReason",
          type: "string",
          description:
            "Reason such as model_stop, max_steps_exceeded, or error",
        },
        {
          name: "fullyIdle",
          type: "boolean",
          description:
            "Whether all background commands or tasks are finished",
        },
      ],
    },
  ],
};
