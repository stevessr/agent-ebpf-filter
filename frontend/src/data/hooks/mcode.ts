import type { HookCliDoc } from "./types";

export const mcodeHook: HookCliDoc = {
  id: "mcode",
  name: "MiniMax Code",
  sources: [{ label: "MiniMax Code repository", url: "https://github.com/MiniMax-AI/minimax-code" }],
  notes: [
    "The mcode integration uses an optional agent-wrapper shell alias, not an invented native hook configuration.",
    "The mcode launcher serves TUI, headless exec, and ACP. The public source then sets process.title to minimax-code, so both mcode and minimax-code are tracked; child processes inherit tracking through the existing PID lineage mechanism.",
    "MiniMax Code supports custom providers. A request to MiniMax does not by itself identify MiniMax Code, and the CLI is not limited to MiniMax's API host.",
  ],
  events: [],
};
