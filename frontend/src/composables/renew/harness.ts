import type { AgentEvent } from "../dashboard/dashboardConstants";
import type { ProcessInfo } from "../monitor/useMonitorData";

export const HARNESS_LABELS = {
  codex: "Codex",
  claude: "Claude Code",
  gemini: "Gemini CLI",
  dsh: "DeepSeek Harness",
  copilot: "GitHub Copilot",
  cursor: "Cursor",
  opencode: "OpenCode",
  pi: "Pi",
  omp: "Oh My Pi",
  kiro: "Kiro CLI",
  augment: "Augment",
  antigravity: "Antigravity CLI",
  zcode: "ZCode",
  mcode: "MiniMax Code",
  unknown: "未识别",
} as const;
export type Harness = keyof typeof HARNESS_LABELS;
export type HarnessFilter = Harness | "all";

// Match tool identities, not API providers, file paths accessed, or generic node/bun.
export function identifyHarness(identity: string): Harness {
  const name = identity.trim().toLowerCase();
  const aliases: Record<string, Harness> = {
    codex: "codex",
    "codex-code-mode": "codex",
    "codex-code-mode-host": "codex",
    claude: "claude",
    "claude code": "claude",
    "claude-code": "claude",
    gemini: "gemini",
    "gemini cli": "gemini",
    "gemini-cli": "gemini",
    dsh: "dsh",
    "deepseek-harness": "dsh",
    "deepseek harness": "dsh",
    copilot: "copilot",
    "github copilot": "copilot",
    cursor: "cursor",
    "cursor-agent": "cursor",
    opencode: "opencode",
    pi: "pi",
    "pi-coding-agent": "pi",
    omp: "omp",
    "oh my pi": "omp",
    "oh-my-pi": "omp",
    kiro: "kiro",
    "kiro-cli": "kiro",
    "kiro cli": "kiro",
    augment: "augment",
    auggie: "augment",
    "augment (auggie cli)": "augment",
    agy: "antigravity",
    antigravity: "antigravity",
    "antigravity cli": "antigravity",
    zcode: "zcode",
    "zcode.appimage": "zcode",
    mcode: "mcode",
    "minimax code": "mcode",
  };
  return Object.hasOwn(aliases, name) ? aliases[name]! : "unknown";
}
export function eventHarness(event: AgentEvent): Harness {
  if (event.harness && Object.hasOwn(HARNESS_LABELS, event.harness))
    return event.harness as Harness;
  const tagged = identifyHarness(event.tag);
  return tagged !== "unknown" ? tagged : identifyHarness(event.comm);
}
// Descendant kernel events may have a generic tag. Correlate only by explicit
// run/conversation AND root context, never by a live PID or accessed file path.
export function attributeEvents(events: AgentEvent[]): AgentEvent[] {
  const contextKey = (event: AgentEvent) => {
    if (!event.agentRunId && !event.conversationId) return "";
    return JSON.stringify([
      event.agentRunId || "",
      event.conversationId || "",
      event.rootAgentPid || 0,
    ]);
  };
  const identities = new Map<string, Set<Harness>>();
  for (const event of events) {
    const key = contextKey(event);
    const harness = eventHarness(event);
    if (!key || harness === "unknown") continue;
    const found = identities.get(key) || new Set<Harness>();
    found.add(harness);
    identities.set(key, found);
  }
  return events.map((event) => {
    const harness = eventHarness(event);
    if (harness !== "unknown") return event;
    const found = identities.get(contextKey(event));
    return found?.size === 1 ? { ...event, harness: [...found][0] } : event;
  });
}

export function processHarness(process: ProcessInfo): Harness {
  const direct = identifyHarness(process.name);
  if (direct !== "unknown") return direct;
  // Only inspect executable/script arguments of known interpreters. A grep or
  // shell command mentioning a harness package is not itself that harness.
  if (!/^(node|bun|python[\d.]*)$/.test(process.name)) return "unknown";
  const argv = (process.cmdline || "").trim().split(/\s+/);
  if (
    argv
      .slice(1)
      .some((arg) => ["-e", "--eval", "-p", "--print", "-c"].includes(arg))
  )
    return "unknown";
  const script = argv.slice(1).find((arg) => !arg.startsWith("-"));
  const args = script ? [script] : [];
  for (const arg of args) {
    if (/(?:^|\/)@openai\/codex\//.test(arg)) return "codex";
    if (/(?:^|\/)@(?:anthropic-ai|cometix)\/claude-code\//.test(arg))
      return "claude";
    if (/(?:^|\/)@mariozechner\/pi-coding-agent\//.test(arg)) return "pi";
    if (/(?:^|\/)@oh-my-pi\//.test(arg)) return "omp";
    if (/(?:^|\/)@google\/gemini-cli\//.test(arg)) return "gemini";
    if (/(?:^|\/)(?:deepseek-harness|dsh)\/(?:[^\s]+\/)*[^\s]+$/.test(arg))
      return "dsh";
  }
  return "unknown";
}
export function attributeProcesses(processes: ProcessInfo[]) {
  const byPid = new Map(processes.map((p) => [p.pid, p]));
  return processes.map((process) => {
    let current: ProcessInfo | undefined = process;
    const seen = new Set<number>();
    let harness: Harness = "unknown";
    let rootPid = process.pid;
    while (current && !seen.has(current.pid)) {
      seen.add(current.pid);
      const found = processHarness(current);
      if (found !== "unknown") {
        harness = found;
        rootPid = current.pid;
        break;
      }
      const parent = byPid.get(current.ppid);
      if (
        parent &&
        current.createTime &&
        parent.createTime > current.createTime
      )
        break;
      current = parent;
    }
    return { ...process, harness, rootPid };
  });
}
export function sessionKey(event: AgentEvent) {
  // Never coalesce unrelated tools that happen to use the same run ID.
  const context = [event.agentRunId, event.conversationId]
    .filter(Boolean)
    .join(":");
  const harness = eventHarness(event);
  return `${harness}:${context || `pid:${event.rootAgentPid || event.pid}`}${harness === "unknown" && context ? `:pid:${event.rootAgentPid || event.pid}` : ""}`;
}
