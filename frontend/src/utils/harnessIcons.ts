import catalog from "./harnessIconCatalog.json";

/**
 * Identity mapping, not process classification. Match known CLI/harness names
 * exactly and never mistake a generic runtime or API provider for a harness.
 */
export type HarnessIconKey = keyof typeof catalog;

function normalize(value: string): string {
  const basename = value.trim().toLowerCase().replace(/\\/g, "/").split("/").pop() ?? "";
  return basename
    .replace(/\.(?:exe|appimage|cmd|bat)$/i, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

const aliases = new Map<string, HarnessIconKey>();
for (const [id, entry] of Object.entries(catalog)) {
  for (const alias of [id, ...entry.aliases]) {
    const normalized = normalize(alias);
    if (!normalized) throw new Error(`Empty harness alias for ${id}`);
    const previous = aliases.get(normalized);
    if (previous && previous !== id) {
      throw new Error(`Harness icon alias collision: ${alias} (${previous}, ${id})`);
    }
    aliases.set(normalized, id as HarnessIconKey);
  }
}

const packageMarkers: readonly [RegExp, HarnessIconKey][] = [
  [/(?:^|[/\\])@anthropic-ai[/\\]claude-code(?:[/\\]|\s|$)/i, "claude"],
  [/(?:^|[/\\])@google[/\\]gemini-cli(?:[/\\]|\s|$)/i, "gemini"],
  [/(?:^|[/\\])@openai[/\\]codex(?:[/\\]|\s|$)/i, "codex"],
  [/(?:^|[/\\])@github[/\\]copilot(?:[/\\]|\s|$)/i, "copilot"],
];

/** Resolve an explicit hook ID, display label or executable name. */
export function resolveHarnessIcon(
  harness?: string | null,
  command?: string | null,
  commandLine?: string | null,
): HarnessIconKey | null {
  for (const value of [harness, command]) {
    if (!value) continue;
    const key = aliases.get(normalize(value));
    if (key) return key;
  }

  // Process command lines can expose a known executable or an installed CLI
  // package. Ignore arbitrary arguments: "node --title codex" is not Codex.
  if (commandLine) {
    const executable = commandLine.trim().match(/^(?:"([^"]+)"|'([^']+)'|(\S+))/)?.slice(1).find(Boolean);
    if (executable) {
      const key = aliases.get(normalize(executable));
      if (key) return key;
    }
    for (const [pattern, key] of packageMarkers) {
      if (pattern.test(commandLine)) return key;
    }
  }
  return null;
}
