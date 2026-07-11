import type { HookCliDoc } from "./types";

export const geminiHook: HookCliDoc = {
  id: "gemini",
  name: "Gemini CLI",
  sources: [
    {
      label: "Gemini CLI hooks guide",
      url: "https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/writing-hooks.md",
    },
    {
      label: "Gemini CLI configuration reference",
      url: "https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md",
    },
  ],
  notes: [
    "The current official writing guide is example-driven; some events are documented as configurable but their full input schema is not enumerated there.",
    "Fields below for Gemini are limited to what the official examples explicitly use.",
  ],
  events: [
    {
      name: "SessionStart",
      description: "When a session starts",
      matcher: "source",
      notes: [
        "Official examples show matcher values like startup; the current writing guide does not enumerate extra input fields for this event.",
      ],
    },
    {
      name: "SessionEnd",
      description: "When a session ends",
      matcher: "source",
      notes: [
        "The configuration reference documents the event, but the writing guide does not list its input schema.",
      ],
    },
    {
      name: "BeforeAgent",
      description: "Before the agent loop starts",
      matcher: "*",
      notes: [
        "The official example injects additional context here but does not consume documented input fields.",
      ],
    },
    {
      name: "AfterAgent",
      description: "After the agent loop completes",
      matcher: "*",
      fields: [
        {
          name: "prompt_response",
          type: "string",
          description:
            "Agent response text used in the official validation example",
        },
      ],
    },
    {
      name: "BeforeModel",
      description: "Before an LLM request is sent",
      matcher: "*",
      notes: [
        "The configuration reference documents this event; field-level schema is not enumerated in the current hooks guide.",
      ],
    },
    {
      name: "AfterModel",
      description: "After an LLM response is received",
      matcher: "*",
      fields: [
        {
          name: "llm_request",
          type: "object",
          description: "LLM request payload",
        },
        {
          name: "llm_response",
          type: "object",
          description: "LLM response payload",
        },
      ],
    },
    {
      name: "BeforeToolSelection",
      description: "Before available tools are selected",
      matcher: "*",
      fields: [
        {
          name: "llm_request",
          type: "object",
          description: "Request payload containing messages",
        },
        {
          name: "llm_request.messages",
          type: "array",
          description: "Messages array used to infer tool intent",
        },
      ],
    },
    {
      name: "BeforeTool",
      description: "Before a tool runs",
      matcher: "tool name",
      fields: [
        {
          name: "tool_name",
          type: "string",
          description: "Tool name used in the quick-start logger",
        },
        {
          name: "tool_input",
          type: "object",
          description:
            "Tool arguments; examples read fields like content and new_string",
        },
      ],
    },
    {
      name: "Notification",
      description: "When Gemini CLI emits a notification",
      matcher: "notification type",
      notes: [
        "The configuration reference documents Notification, but the current hooks guide does not enumerate its input schema.",
      ],
    },
    {
      name: "PreCompress",
      aliases: ["PreCompress"],
      description: "Before chat history compression",
      matcher: "trigger",
      notes: [
        "The configuration reference documents this event, but the current hooks guide does not enumerate its input schema.",
      ],
    },
  ],
};
