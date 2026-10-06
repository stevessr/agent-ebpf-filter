import type { Ref } from "vue";

/**
 * Renew keyboard operability helpers.
 *
 * Renew lists are rendered as real <button> elements wherever possible, so this
 * module only covers the cases a native button cannot express: activating a
 * non-interactive row container, and keeping the drawer predictable for
 * keyboard and screen-reader users.
 */

/** Enter/Space activates a row that is not itself a <button>. */
export const activateRowOnKeyboard = (
  event: KeyboardEvent,
  action: () => void,
) => {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  action();
};

/**
 * Move focus into the drawer body once it finishes opening.
 *
 * a-drawer animates in, so focusing on open would land on a container that is
 * still transitioning. `afterOpenChange` fires after the transition and only
 * on open, which avoids stealing focus on close.
 */
export const focusOnDrawerOpened = (
  open: boolean,
  body: HTMLElement | null,
) => {
  if (!open || !body) return;
  body.focus();
};

/** True when a keyboard event originated inside a text-entry control. */
export const isTextEntryTarget = (target: EventTarget | null) => {
  const element = target as HTMLElement | null;
  if (!element || typeof element.tagName !== "string") return false;
  const tag = element.tagName.toLowerCase();
  return (
    tag === "input" ||
    tag === "textarea" ||
    tag === "select" ||
    element.isContentEditable === true
  );
};

/**
 * "/" focuses the Renew search box, the convention operators expect from
 * log/dashboard tools. Skipped while typing so it never steals a keystroke,
 * and skipped when a modifier is held so browser shortcuts still work.
 */
export const shouldFocusSearchOnSlash = (
  event: KeyboardEvent,
  search: Ref<HTMLElement | null>,
) => {
  if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey) {
    return false;
  }
  if (isTextEntryTarget(event.target)) return false;
  const element = search.value;
  if (!element) return false;
  event.preventDefault();
  element.focus();
  const textEntry = element as HTMLInputElement | HTMLTextAreaElement;
  textEntry.select?.();
  return true;
};
