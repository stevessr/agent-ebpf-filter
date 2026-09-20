import type { HookCliDoc } from "./types";

export const zcodeHook: HookCliDoc = {
  id: "zcode",
  name: "ZCode",
  sources: [{ label: "ZCode documentation", url: "https://zcode.z.ai/en/docs/agents" }],
  notes: [
    "The CLI integration uses an optional agent-wrapper shell alias; it does not install unverified native hooks in the ZCode desktop application.",
    "Tracked process lineage observes desktop and child processes independently of the shell alias, when executable identity can be established.",
    "ZCode can use different model providers: network vendor fingerprints do not establish the calling harness.",
  ],
  events: [],
};
