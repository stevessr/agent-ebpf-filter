import { expect, test } from "bun:test";
import { dockTabNavigationIndex, dockSplitterRatio } from "../src/composables/workbench/keyboardNavigation";
import { createTrailingWrite } from "../src/composables/workbench/trailingWrite";

test("horizontal dock tabs wrap, jump home/end and ignore unrelated keys", () => {
  expect(dockTabNavigationIndex("ArrowRight", 2, 3)).toBe(0);
  expect(dockTabNavigationIndex("ArrowLeft", 0, 3)).toBe(2);
  expect(dockTabNavigationIndex("Home", 2, 3)).toBe(0);
  expect(dockTabNavigationIndex("End", 0, 3)).toBe(2);
  expect(dockTabNavigationIndex("ArrowDown", 0, 3)).toBeNull();
  expect(dockTabNavigationIndex("ArrowRight", 0, 0)).toBeNull();
  expect(dockTabNavigationIndex("Home", -1, 3)).toBeNull();
});

test("dock split resizing honors orientation and 15–85% bounds", () => {
  expect(dockSplitterRatio("horizontal", "ArrowRight", 0.5)).toBe(0.55);
  expect(dockSplitterRatio("horizontal", "ArrowLeft", 0.15)).toBe(0.15);
  expect(dockSplitterRatio("horizontal", "ArrowUp", 0.5)).toBeNull();
  expect(dockSplitterRatio("vertical", "ArrowDown", 0.84)).toBe(0.85);
  expect(dockSplitterRatio("vertical", "ArrowUp", 0.5)).toBe(0.45);
  expect(dockSplitterRatio("vertical", "ArrowRight", 0.5)).toBeNull();
  expect(dockSplitterRatio("horizontal", "ArrowLeft", Number.NaN)).toBeNull();
});

test("rapid layout changes coalesce and flush persists once", () => {
  const pending = new Map<number, () => void>();
  let id = 0;
  const timers = {
    set(fn: () => void, ms: number): ReturnType<typeof setTimeout> {
      expect(ms).toBe(250);
      pending.set(++id, fn);
      return id as unknown as ReturnType<typeof setTimeout>;
    },
    clear(handle: ReturnType<typeof setTimeout>) {
      pending.delete(handle as unknown as number);
    },
  };
  let writes = 0;
  const writer = createTrailingWrite(() => { writes++; }, 250, timers);
  for (let i = 0; i < 100; i++) writer.mark();
  expect(pending.size).toBe(1);
  expect(writes).toBe(0);
  [...pending.values()][0]!();
  expect(writes).toBe(1);
  writer.flush();
  expect(writes).toBe(1);
  writer.mark();
  writer.flush();
  expect(writes).toBe(2);
  expect(pending.size).toBe(0);
  writer.mark();
  writer.cancel();
  expect(pending.size).toBe(0);
  expect(writes).toBe(2);
});
