import { describe, expect, test } from "bun:test";
import { resolveHarnessIcon } from "../src/utils/harnessIcons";

describe("Lobe harness icon identity", () => {
  test("maps native hooks, wrappers and subprocess variants", () => {
    expect(resolveHarnessIcon("claude")).toBe("claude");
    expect(resolveHarnessIcon("codex-code-mode-host")).toBe("codex");
    expect(resolveHarnessIcon("dsh-exec")).toBe("dsh");
    expect(resolveHarnessIcon("DeepSeek Harness")).toBe("dsh");
    expect(resolveHarnessIcon("omp")).toBe("omp");
    expect(resolveHarnessIcon("mcode")).toBe("mcode");
    expect(resolveHarnessIcon("copilot")).toBe("copilot");
    expect(resolveHarnessIcon("Antigravity CLI")).toBe("antigravity");
    expect(resolveHarnessIcon("cursor-agent")).toBe("cursor");
  });

  test("recognizes CLI executable paths without guessing providers", () => {
    expect(resolveHarnessIcon("/usr/local/bin/gemini")).toBe("gemini");
    expect(resolveHarnessIcon("C:\\Tools\\claude.exe")).toBe("claude");
    expect(resolveHarnessIcon("unknown", "Codex")).toBe("codex");
    expect(resolveHarnessIcon("gh")).toBeNull();
    expect(resolveHarnessIcon("node")).toBeNull();
    expect(resolveHarnessIcon("python")).toBeNull();
    expect(resolveHarnessIcon("claude-malware")).toBeNull();
    expect(resolveHarnessIcon("")).toBeNull();
  });
});
