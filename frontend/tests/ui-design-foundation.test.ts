import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { uiDesignTokens } from "../src/config/uiDesignTokens";
import { resolveWorkbenchShortcut } from "../src/composables/workbench/navigationShortcuts";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");
const styles = read("../src/style.css");
const workbench = read("../src/components/workbench/workbench.css");
const desktopTheme = read("../../desktop/renew/native_theme.go");
const app = read("../src/App.vue");
const sideNav = read("../src/components/layout/AppSideNav.vue");
const tabs = read("../src/components/layout/AppWorkbenchTabs.vue");
const dockSource = read("../src/components/workbench/DockNode.vue");
const persistenceSource = read("../src/composables/workbench/useDockWorkbench.ts");

test("shared colors stay aligned across Web CSS, Ant Design and native Desktop", () => {
  for (const mode of ["light", "dark"] as const) {
    for (const [token, value] of Object.entries(uiDesignTokens[mode])) {
      expect(styles).toContain(value);
      if (token !== "accent") expect(desktopTheme).toContain(value);
    }
  }
  expect(app).toContain("uiDesignTokens");
  expect(uiDesignTokens.fontSize).toBe(16);
  expect(styles).toContain("--surface-0: var(--ui-canvas)");
  expect(styles).toContain("--text-primary: var(--ui-text-strong)");
  expect(styles).toContain("--border: var(--ui-border)");
});

test("shell responds to narrow screens and keyboard and motion preferences", () => {
  expect(workbench).toContain("@media (max-width: 720px)");
  expect(workbench).toContain(".ide-sidebar-scrim");
  expect(workbench).toContain("prefers-reduced-motion: reduce");
  expect(workbench).toContain("forced-colors: active");
  expect(app).toContain('aria-controls="ide-sidebar"');
  expect(app).toContain('v-show="sidebar" id="ide-sidebar"');
  expect(sideNav).toContain("var(--ui-accent)");
  expect(sideNav).toContain("var(--ui-surface-soft)");
  expect(tabs).toContain("var(--ui-surface)");
  expect(app).toContain(':aria-expanded="sidebar"');
  expect(app).toContain('window.addEventListener("keydown", onWorkbenchKeydown)');
  expect(app).toContain('window.removeEventListener("keydown", onWorkbenchKeydown)');
  expect(app).toContain(".monaco-editor, .xterm, .wterm");
  expect(app).toContain('narrowNavigationMedia = window.matchMedia("(max-width: 720px)")');
  expect(app).toContain('narrowNavigationMedia.addEventListener("change", onNarrowNavigationChanged)');
  expect(app).toContain('if (narrowNavigationMedia.matches) sidebar.value = false');
  expect(app).toContain("resolveWorkbenchShortcut({");
  expect(app).toContain('inEditorOrTerminal: inside(".monaco-editor, .xterm, .wterm")');
  expect(app).toContain('import { useRouter, type RouteLocationRaw } from "vue-router"');
  expect(app).toContain('function openNavigationItem(route: RouteLocationRaw)');
  expect(app).toContain('@click="openNavigationItem(item.defaultRoute)"');
  expect(app).toContain(':aria-pressed="activeGroup === g.key && sidebar"');
  expect(workbench).not.toContain(":root { font-size: 13px; }");
  expect(workbench).not.toMatch(/(?:font-size|font):\s*(?:0|[1-9]|10)px/);
  expect(workbench).not.toMatch(/font-size:\s*0\s*;/);
  expect(workbench).not.toMatch(/font-weight:\s*(?:600|700)/);
  expect(workbench).not.toMatch(/(?:linear|radial|conic)-gradient\s*\(/);
  expect(workbench).not.toContain("box-shadow: 10px");
  expect(workbench).toContain("background: transparent;");
  expect(app).not.toContain("◈");
  expect(app).toContain('ref="navigationToggle"');
});

test("navigation shortcuts respect editable surfaces and event ownership", () => {
  const globalSearch = { key: "k", ctrlKey: true, narrow: true, sidebarOpen: false };
  expect(resolveWorkbenchShortcut(globalSearch)).toBe("focus-search");
  expect(resolveWorkbenchShortcut({ ...globalSearch, ctrlKey: false, metaKey: true })).toBe("focus-search");
  expect(resolveWorkbenchShortcut({ ...globalSearch, inEditableField: true })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...globalSearch, inEditorOrTerminal: true })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...globalSearch, defaultPrevented: true })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...globalSearch, ctrlKey: false, key: "k" })).toBeNull();
});

test("Escape dismisses only an open narrow overlay, even from shell search", () => {
  const narrow = { key: "Escape", narrow: true, sidebarOpen: true };
  expect(resolveWorkbenchShortcut(narrow)).toBe("close-overlay");
  expect(resolveWorkbenchShortcut({ ...narrow, inEditableField: true, inSidebarSearch: true })).toBe("close-overlay");
  expect(resolveWorkbenchShortcut({ ...narrow, inEditableField: true })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...narrow, narrow: false })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...narrow, sidebarOpen: false })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...narrow, inEditorOrTerminal: true })).toBeNull();
  expect(resolveWorkbenchShortcut({ ...narrow, defaultPrevented: true })).toBeNull();
});

test("dock tabs expose accessible keyboard and panel semantics", () => {
  expect(dockSource).toContain('role="tablist"');
  expect(dockSource).toContain('aria-orientation="horizontal"');
  expect(dockSource).toContain(':tabindex="node.active === tab.id ? 0 : -1"');
  expect(dockSource).toContain('dockTabNavigationIndex(event.key, index, props.node.tabs.length)');
  expect(dockSource).toContain('role="tabpanel"');
  expect(dockSource).toContain(':aria-labelledby="node.active ?');
  expect(workbench).toContain('.dock-content[role="tabpanel"]:focus-visible');
});
test("layout changes use trailing writes and flush on navigation away", () => {
  expect(persistenceSource).toContain('createTrailingWrite(');
  expect(persistenceSource).toContain('window.addEventListener("pagehide", persistence.flush)');
  expect(persistenceSource).toContain('document.addEventListener("visibilitychange", onVisibilityChange)');
});
