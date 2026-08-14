import type { HookCliDoc } from "./types";

export const dshHook: HookCliDoc = {
  id: "dsh",
  name: "DeepSeek Harness",
  sources: [
    {
      label: "DeepSeek Harness repository",
      url: "https://github.com/deepseek-ai/deepseek-harness",
    },
  ],
  notes: [
    "dsh is supported through the agent-wrapper command shim; this integration does not invent a generic dsh hook configuration file.",
    "Manage dsh profiles, bundles, and plugins with dsh itself.",
  ],
  events: [],
};
