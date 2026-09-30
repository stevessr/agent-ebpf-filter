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
    "The wrapper preserves dsh's launcher/app argument boundary and keeps profile app arguments verbatim.",
    "Audit labels include only bounded launcher metadata (profile, plugin operation, or config-dump mode); prompts, package names, and patch paths are not copied into those labels.",
    "Manage dsh profiles, bundles, compatibility exemptions, and plugins with dsh itself.",
  ],
  events: [],
};
