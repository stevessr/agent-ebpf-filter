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
    "Agent eBPF installs a DeepSeek Harness bundle that replaces the canonical ctx.subprocess provider, so both spawn() and spawnTerminal() cross the policy boundary.",
    "The subprocess provider preserves exact argv, cwd, stdio, cancellation, output and PTY semantics from dsh-subprocess-local; dsh plugin/launcher operations remain owned by dsh.",
    "DeepSeek Harness network plaintext is collected from the official experimental Inspector/CDP Network API on loopback, not from eBPF TLS uprobes.",
    "Inspector payloads are sanitized by the existing URL/header/body redaction pipeline before entering the Network capture store.",
  ],
  events: [],
};
