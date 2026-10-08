import catalog from "./harnessIconCatalog.json";

/** Exact CLI/Harness identifiers only: never infer a harness from a generic process (e.g. node or gh). */
export type HarnessIconKey = keyof typeof catalog;

const aliases = new Map<string, HarnessIconKey>();
for (const [key, entry] of Object.entries(catalog)) {
  for (const alias of entry.aliases) aliases.set(alias, key as HarnessIconKey);
}

/** Normalize paths, executable names and labels without matching arbitrary substrings. */
const normalize = (value: string): string =>
  value
    .trim()
    .toLowerCase()
    .replace(/\\/g, "/")
    .split("/")
    .pop()!
    .replace(/\.exe$/, "")
    .replace(/[\s_.]+/g, "-")
    .replace(/-+/g, "-");

/** Prefer a stable hook ID; fall back to a recognized display name/command. */
export function resolveHarnessIcon(
  harness?: string | null,
  command?: string | null,
): HarnessIconKey | null {
  for (const value of [harness, command]) {
    if (!value) continue;
    const key = aliases.get(normalize(value));
    if (key) return key;
  }
  return null;
}
