import type { HookCliDoc } from "./types";

export const dshExecHook: HookCliDoc = {
  id: "dsh-exec",
  name: "DeepSeek Harness subprocess policy",
  sources: [
    { label: "DeepSeek Harness repository", url: "https://github.com/deepseek-ai/deepseek-harness" },
  ],
  notes: [
    "Optional independent provider for the canonical Harness subprocess capability; it wraps spawn and spawnTerminal before the official local provider executes commands.",
    "Requires the compatible Harness plugin package, an installed agent-wrapper, and a reachable backend Unix socket. It does not replace the native dsh Cordis session/tool metadata hook.",
    "Installation and removal are managed by dsh plugin per profile and never modify the shell alias or the native Cordis patch.",
    "The official experimental Inspector/CDP Network integration connects only when requested, using loopback, and sanitizes secrets before storing events.",
  ],
  events: [],
};
