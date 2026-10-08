import { describe, expect, test } from "bun:test";
import {
  dock,
  group,
  groups,
  validLayout,
  prune,
} from "../src/composables/workbench/layout";
import type { DockTab, DockNode } from "../src/composables/workbench/layout";
const tab = (id: string): DockTab => ({ id, path: "/dashboard", title: id });
describe("IDE docking layout", () => {
  test("UI identifiers also work on insecure LAN HTTP origins", () => {
    const descriptor = Object.getOwnPropertyDescriptor(
      globalThis.crypto,
      "randomUUID",
    );
    Object.defineProperty(globalThis.crypto, "randomUUID", {
      value: undefined,
      configurable: true,
    });
    try {
      const first = group(),
        second = group();
      expect(first.id.startsWith("dock-")).toBe(true);
      expect(first.id).not.toBe(second.id);
    } finally {
      if (descriptor)
        Object.defineProperty(globalThis.crypto, "randomUUID", descriptor);
      else Reflect.deleteProperty(globalThis.crypto, "randomUUID");
    }
  });
  test("all four edges create the correct ordered split", () => {
    for (const edge of ["left", "right", "top", "bottom"] as const) {
      const a = group([tab("a")]);
      const root = dock(a, tab("b"), a.id, edge);
      expect(root.kind).toBe("split");
      if (root.kind !== "split") throw new Error("missing split");
      expect(root.axis).toBe(
        ["left", "right"].includes(edge) ? "horizontal" : "vertical",
      );
      expect(groups(root)[0]!.tabs[0]!.id).toBe(
        ["left", "top"].includes(edge) ? "b" : "a",
      );
      expect(validLayout(root)).toBe(true);
    }
  });
  test("moving to center collapses the empty source and activates the moved tab", () => {
    const a = group([tab("a")]);
    const root = dock(a, tab("b"), a.id, "right");
    const moved = dock(root, tab("b"), a.id, "center");
    expect(moved.kind).toBe("group");
    expect(groups(moved)[0]!.active).toBe("b");
    expect(groups(moved)[0]!.tabs.map((t) => t.id)).toEqual(["a", "b"]);
  });
  test("same-group split moves a tab without duplication or mutation", () => {
    const a = group([tab("a"), tab("b")]);
    const root = dock(a, tab("b"), a.id, "bottom");
    expect(a.tabs.length).toBe(2);
    expect(groups(root).map((g) => g.tabs.length)).toEqual([1, 1]);
    expect(dock(group([tab("a")]), tab("a"), "unknown", "left").kind).toBe(
      "group",
    );
    const single = group([tab("x")]);
    expect(dock(single, tab("x"), single.id, "left")).toBe(single);
  });
  test("nested splits survive serialization and recursively prune empty groups", () => {
    const a = group([tab("a")]);
    let root = dock(a, tab("b"), a.id, "right");
    root = dock(root, tab("c"), groups(root)[1]!.id, "bottom");
    expect(groups(root).length).toBe(3);
    expect(validLayout(JSON.parse(JSON.stringify(root)))).toBe(true);
    groups(root)[1]!.tabs = [];
    const next = prune(root)!;
    expect(groups(next).length).toBe(2);
  });
  test("rejects duplicate IDs, bad ratios, external paths and excessive depth", () => {
    const a = group([tab("a")]);
    expect(validLayout(a)).toBe(true);
    expect(validLayout({ ...a, tabs: [tab("a"), tab("a")] })).toBe(false);
    expect(
      validLayout({ ...a, tabs: [{ ...tab("a"), path: "//evil.test" }] }),
    ).toBe(false);
    expect(validLayout({ ...a, active: "missing" })).toBe(false);
    let root: DockNode = a;
    for (let i = 0; i < 10; i++)
      root = {
        kind: "split",
        id: `s${i}`,
        axis: "horizontal",
        ratio: 0.5,
        first: root,
        second: group([tab(`b${i}`)]),
      };
    expect(validLayout(root)).toBe(false);
    expect(
      validLayout({
        kind: "split",
        id: "bad",
        axis: "vertical",
        ratio: NaN,
        first: a,
        second: group(),
      }),
    ).toBe(false);
  });
});
