import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { uiDesignTokens } from "../src/config/uiDesignTokens";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");
const styles = read("../src/style.css");
const workbench = read("../src/components/workbench/workbench.css");
const desktopTheme = read("../../desktop/renew/native_theme.go");
const app = read("../src/App.vue");

test("shared colors stay aligned across Web CSS, Ant Design and native Desktop", () => {
  for (const mode of ["light", "dark"] as const) {
    for (const [token, value] of Object.entries(uiDesignTokens[mode])) {
      expect(styles).toContain(value);
      if (token !== "accent") expect(desktopTheme).toContain(value);
    }
  }
  expect(app).toContain("uiDesignTokens");
});

test("shell responds to narrow screens and keyboard and motion preferences", () => {
  expect(workbench).toContain("@media (max-width: 720px)");
  expect(workbench).toContain(".ide-sidebar-scrim");
  expect(workbench).toContain("prefers-reduced-motion: reduce");
  expect(workbench).toContain("forced-colors: active");
  expect(app).toContain('aria-controls="ide-sidebar"');
  expect(app).toContain(':aria-expanded="sidebar"');
  expect(app).toContain('window.addEventListener("keydown", onWorkbenchKeydown)');
  expect(app).toContain('window.removeEventListener("keydown", onWorkbenchKeydown)');
});
