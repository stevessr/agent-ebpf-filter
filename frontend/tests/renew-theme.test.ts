import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const renewDir = join(import.meta.dir, "../src");
const cssPath = join(renewDir, "views/renew/renew.css");
const stylePath = join(renewDir, "components/renew/RenewEventDetailDrawer.vue");
const visualPath = join(renewDir, "components/renew/RenewVisualOverview.vue");

const css = readFileSync(cssPath, "utf8");

// Light palette is the leading :root block; dark palette the one nested in the
// prefers-color-scheme query. Aliases (`--x: var(--y)`) inherit per-scheme
// automatically, so only literal-valued tokens need a dark counterpart.
const LIGHT_BLOCK = css.match(/^:root \{([\s\S]*?)^\}/m)?.[1] ?? "";
const DARK_BLOCK =
  css.match(
    /@media \(prefers-color-scheme: dark\) \{\n  :root \{([\s\S]*?)^\s{2}\}/m,
  )?.[1] ?? "";

const literalTokens = (block: string) =>
  new Set(
    [...block.matchAll(/^\s*(--renew-[a-z0-9-]+)\s*:\s*([^;]+);/gim)]
      .filter((match) => !match[2]!.trim().startsWith("var("))
      .map((match) => match[1]!),
  );

const tokensIn = (text: string) =>
  new Set(
    [...text.matchAll(/var\((--renew-[a-z0-9-]+)\)/g)].map(
      (match) => match[1]!,
    ),
  );

// Color literals are only allowed inside the palette blocks themselves.
const outsideTokenBlocks = css.replace(LIGHT_BLOCK, "").replace(DARK_BLOCK, "");

describe("renew theme tokens", () => {
  test("light palette declares the shared surface/status tokens", () => {
    const light = literalTokens(LIGHT_BLOCK);
    for (const token of [
      "--renew-bg",
      "--renew-panel",
      "--renew-text",
      "--renew-accent",
      "--renew-success",
      "--renew-warning",
      "--renew-danger",
    ]) {
      expect({ token, declared: light.has(token) }).toEqual({
        token,
        declared: true,
      });
    }
  });

  test("dark palette covers every literal-valued light token", () => {
    const light = literalTokens(LIGHT_BLOCK);
    const dark = literalTokens(DARK_BLOCK);
    expect(light.size).toBeGreaterThan(30);
    for (const token of light) {
      expect({ token, inDark: dark.has(token) }).toEqual({
        token,
        inDark: true,
      });
    }
  });

  test("every token used by renew styles is declared", () => {
    const declared = new Set([
      ...literalTokens(LIGHT_BLOCK),
      ...[
        ...LIGHT_BLOCK.matchAll(/^\s*(--renew-[a-z0-9-]+)\s*:\s*var\(/gim),
      ].map((match) => match[1]!),
    ]);
    const styleText = [
      css,
      readFileSync(stylePath, "utf8"),
      readFileSync(visualPath, "utf8"),
    ].join("\n");
    for (const token of tokensIn(styleText)) {
      expect({ token, declared: declared.has(token) }).toEqual({
        token,
        declared: true,
      });
    }
  });

  test("no raw hex colors or rgba() literals outside the palette blocks", () => {
    for (const [path, text] of [
      [cssPath, outsideTokenBlocks],
      [stylePath, readFileSync(stylePath, "utf8")],
      [visualPath, readFileSync(visualPath, "utf8")],
    ] as const) {
      const hex = text.match(/#[0-9a-f]{3,8}\b/gi) ?? [];
      const rgba = text.match(/rgba?\(\s*[\d.]/g) ?? [];
      expect({ path, hex, rgba }).toEqual({ path, hex: [], rgba: [] });
    }
  });
});
