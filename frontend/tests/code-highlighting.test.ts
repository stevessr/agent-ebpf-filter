import { describe, expect, test } from "bun:test";
import { fileURLToPath } from "node:url";

// Isolate the theme singleton from tests that install their own window stub.
function checkPreview(source: string) {
  const result = Bun.spawnSync([
    process.execPath, "--eval",
    `import assert from "node:assert/strict";
     import { highlightPreviewCode, normalizePreviewLanguage } from "./src/utils/codeHighlighting";
     ${source}`,
  ], { cwd: fileURLToPath(new URL("..", import.meta.url)) });
  expect(result.stderr.toString()).toBe("");
  expect(result.exitCode).toBe(0);
}

describe("code preview highlighting", () => {
  test("normalizes aliases and rejects inherited object keys", () => {
    checkPreview(`
      assert.equal(normalizePreviewLanguage(" .TS "), "typescript");
      assert.equal(normalizePreviewLanguage("py"), "python");
      for (const language of [undefined, "", "unknown", "constructor", "__proto__", "hasOwnProperty"]) {
        assert.equal(normalizePreviewLanguage(language), "text");
      }
    `);
  });

  test("unknown languages render escaped plain text", () => {
    checkPreview(`
      const html = await highlightPreviewCode('<script>alert("x")</script>', "constructor", false);
      assert.match(html, /(?:&lt;|&#x3C;)script/i);
      assert.ok(!html.includes("<script>"));
      assert.ok(html.includes("github-light"));
    `);
  });

  test("concurrent previews load grammars and honor each requested theme", () => {
    checkPreview(`
      const results = await Promise.all([
        highlightPreviewCode("const answer = 42", "js", true),
        highlightPreviewCode("const answer = 43", "javascript", false),
        highlightPreviewCode("print(44)", "python", true),
      ]);
      assert.ok(results[0].includes("github-dark"));
      assert.ok(results[1].includes("github-light"));
      assert.ok(results[2].includes("github-dark"));
      for (const html of results) assert.ok(html.includes("<span"));
    `);
  });
});
