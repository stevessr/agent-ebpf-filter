import { describe, expect, test } from "bun:test";
import { ref } from "vue";

import {
  activateRowOnKeyboard,
  focusOnDrawerOpened,
  isTextEntryTarget,
  shouldFocusSearchOnSlash,
} from "../src/composables/renew/keyboard";

// bun test runs without a DOM, so the helpers are exercised against the exact
// shapes they read (tagName / isContentEditable / focus / select) rather than
// real elements.
const elementStub = (props: Record<string, unknown> = {}) =>
  props as unknown as EventTarget;

const eventStub = (
  key: string,
  target: EventTarget | null = null,
  init: KeyboardEventInit = {},
) => {
  const state = { prevented: false };
  const event = {
    key,
    target,
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    ...init,
    preventDefault: () => {
      state.prevented = true;
    },
  } as unknown as KeyboardEvent;
  return { event, state };
};

describe("renew keyboard operability", () => {
  test("Enter and Space activate a row, other keys do not", () => {
    for (const key of ["Enter", " "]) {
      let calls = 0;
      const { event, state } = eventStub(key);
      activateRowOnKeyboard(event, () => calls++);
      expect(calls).toBe(1);
      // Enter/Space must suppress the browser default so Space does not scroll.
      expect(state.prevented).toBe(true);
    }

    let calls = 0;
    for (const key of ["a", "Escape", "ArrowDown", "Tab"]) {
      const { event, state } = eventStub(key);
      activateRowOnKeyboard(event, () => calls++);
      expect(state.prevented).toBe(false);
    }
    expect(calls).toBe(0);
  });

  test("drawer focus moves only on open and only when a body exists", () => {
    let focused = 0;
    const body = elementStub({
      focus: () => {
        focused++;
      },
    }) as unknown as HTMLElement;

    focusOnDrawerOpened(false, body);
    expect(focused).toBe(0);

    focusOnDrawerOpened(true, null);
    expect(focused).toBe(0);

    focusOnDrawerOpened(true, body);
    expect(focused).toBe(1);
  });

  test("text-entry targets are recognised for slash suppression", () => {
    for (const tag of ["input", "INPUT", "textarea", "select"]) {
      expect(isTextEntryTarget(elementStub({ tagName: tag }))).toBe(true);
    }
    expect(isTextEntryTarget(elementStub({ tagName: "span" }))).toBe(false);
    expect(
      isTextEntryTarget(
        elementStub({ tagName: "div", isContentEditable: true }),
      ),
    ).toBe(true);
    expect(isTextEntryTarget(null)).toBe(false);
  });

  test("slash focuses search only outside text entry and without modifiers", () => {
    let focused = 0;
    let selected = 0;
    const searchBox = elementStub({
      tagName: "input",
      focus: () => {
        focused++;
      },
      select: () => {
        selected++;
      },
    }) as unknown as HTMLElement;
    const search = ref<HTMLElement | null>(searchBox);

    const { event, state } = eventStub("/");
    expect(shouldFocusSearchOnSlash(event, search)).toBe(true);
    expect(focused).toBe(1);
    expect(selected).toBe(1);
    expect(state.prevented).toBe(true);

    // Typing "/" inside a text box must insert a slash, not steal focus.
    focused = 0;
    const typing = eventStub("/", elementStub({ tagName: "input" }));
    expect(shouldFocusSearchOnSlash(typing.event, search)).toBe(false);
    expect(focused).toBe(0);
    expect(typing.state.prevented).toBe(false);

    // Modified slashes are browser shortcuts and must pass through untouched.
    for (const modifier of ["ctrlKey", "metaKey", "altKey"] as const) {
      const modified = eventStub("/", null, { [modifier]: true });
      expect(shouldFocusSearchOnSlash(modified.event, search)).toBe(false);
      expect(modified.state.prevented).toBe(false);
    }
    expect(focused).toBe(0);

    // No search box mounted: nothing to focus, nothing swallowed.
    const empty = eventStub("/");
    expect(shouldFocusSearchOnSlash(empty.event, ref(null))).toBe(false);
    expect(empty.state.prevented).toBe(false);
  });
});
