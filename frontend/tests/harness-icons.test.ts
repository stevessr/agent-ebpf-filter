import { describe, expect, test } from "bun:test";
import catalog from "../src/utils/harnessIconCatalog.json";
import { resolveHarnessIcon } from "../src/utils/harnessIcons";

describe("Lobe harness icon identity", () => {
  test("every configured hook resolves to a stable icon", () => {
    for (const id of [
      "claude", "gemini", "codex", "dsh", "dsh-exec", "pi", "omp",
      "copilot", "kiro", "augment", "antigravity", "zcode", "mcode", "cursor",
    ]) {
      expect(resolveHarnessIcon(id)).not.toBeNull();
    }
    expect(resolveHarnessIcon("dsh-exec")).toBe("dsh");
  });

  test("every known catalog alias resolves to its own brand", () => {
    for (const [id, entry] of Object.entries(catalog)) {
      for (const alias of [id, ...entry.aliases]) {
        expect(resolveHarnessIcon(alias)).toBe(id);
      }
      expect(entry.assets.every((name) => name.endsWith(".svg"))).toBe(true);
    }
  });

  test("matches human-facing labels, appimages and executable paths", () => {
    expect(resolveHarnessIcon("Augment (Auggie CLI)")).toBe("augment");
    expect(resolveHarnessIcon("DeepSeek Harness · subprocess policy")).toBe("dsh");
    expect(resolveHarnessIcon("GitHub Copilot CLI")).toBe("copilot");
    expect(resolveHarnessIcon("Oh My Pi")).toBe("omp");
    expect(resolveHarnessIcon("MiniMax Code")).toBe("mcode");
    expect(resolveHarnessIcon("Antigravity CLI")).toBe("antigravity");
    expect(resolveHarnessIcon("/usr/local/bin/gemini")).toBe("gemini");
    expect(resolveHarnessIcon("C:\\Tools\\claude.exe")).toBe("claude");
    expect(resolveHarnessIcon("/opt/ZCode.AppImage")).toBe("zcode");
    expect(resolveHarnessIcon("/opt/agent/bin/kilo-code")).toBe("kilocode");
  });

  test("uses explicit executable or installed package identity, not arbitrary args", () => {
    expect(resolveHarnessIcon("node", "", "node /opt/node_modules/@anthropic-ai/claude-code/cli.js")).toBe("claude");
    expect(resolveHarnessIcon("node", "", "node /opt/node_modules/@google/gemini-cli/dist/index.js")).toBe("gemini");
    expect(resolveHarnessIcon("unknown", "", "/usr/bin/codex --sandbox")).toBe("codex");
    expect(resolveHarnessIcon("unknown", "", '"/usr/local/bin/claude" --help')).toBe("claude");
    expect(resolveHarnessIcon("node", "", "node --title codex")).toBeNull();
    expect(resolveHarnessIcon("node", "", "node /tmp/code-buddy-test.js")).toBeNull();
  });

  test("never maps providers or lookalike binaries to an unrelated harness", () => {
    for (const invalid of ["gh", "node", "python", "openai", "deepseek", "minimax", "claude-malware", "z-code", ""]) {
      expect(resolveHarnessIcon(invalid)).toBeNull();
    }
    expect(resolveHarnessIcon("zencoder")).toBe("zencoder");
    expect(resolveHarnessIcon("zcode")).toBe("zcode"); // distinct products
    expect(resolveHarnessIcon("unknown", "Codex")).toBe("codex");
  });
});
