import type { HookCliDoc } from "./types";

export const mcodeHook: HookCliDoc = {
  id: "mcode",
  name: "MiniMax Code",
  sources: [{ label: "MiniMax Code repository", url: "https://github.com/MiniMax-AI/minimax-code" }],
  notes: [
    "The mcode integration uses an optional agent-wrapper shell alias, not an invented native hook configuration.",
    "The CLI, headless exec mode, and ACP mode can run through the same tracked mcode process; child processes inherit tracking through the existing PID lineage mechanism.",
    "MiniMax Code supports custom providers. A request to MiniMax does not by itself identify MiniMax Code, and the CLI is not limited to MiniMax's API host.",
  ],
  events: [],
};
