import { ref } from "vue";
import { enToZh, zhToEn } from "./messages";

export type WebLocale = "zh-CN" | "en-US";
const STORAGE_KEY = "agent.webui.locale.v1";
const readSavedLocale = (): WebLocale => {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "en-US" ? "en-US" : "zh-CN";
  } catch {
    return "zh-CN";
  }
};

export const webLocale = ref<WebLocale>(readSavedLocale());

export const setWebLocale = (next: WebLocale): void => {
  webLocale.value = next;
  try {
    window.localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Private windows and locked-down browsers still allow in-session changes.
  }
  applyDocumentLocale();
};

export const applyDocumentLocale = (): void => {
  if (typeof document !== "undefined") {
    document.documentElement.lang = webLocale.value;
    document.title = webLocale.value === "en-US"
      ? "Agent eBPF · Observability Studio"
      : "Agent eBPF · 观测工作台";
  }
};

/** UI-only translation. Unknown strings remain unchanged; never translate event data. */
export const translateUI = (value: string): string => {
  const trimmed = value.trim();
  if (!trimmed) return value;
  const dict = webLocale.value === "en-US" ? zhToEn : enToZh;
  let translated = dict[trimmed];
  if (!translated && webLocale.value === "en-US") {
    const panels = trimmed.match(/^(\d+)\s*个面板$/);
    if (panels) translated = `${panels[1]} panes`;
  }
  if (!translated || translated === trimmed) return value;
  const start = value.indexOf(trimmed);
  return value.slice(0, start) + translated + value.slice(start + trimmed.length);
};

// This bridge keeps existing Vue 3 components functional during gradual
// migration to explicit $t calls. It only translates exact, cataloged UI text
// and accessible labels; editor content, logs, JSON, and table records are ignored.
// New components should use translateUI() or $t directly.
const ignoredSelector =
  "pre, code, textarea, script, style, svg, [contenteditable], [data-i18n-ignore], .xterm, .ant-table-tbody";
const attributes = ["title", "placeholder", "aria-label", "alt", "aria-description"];
const textSource = new WeakMap<Text, { source: string; rendered: string }>();
const attrSource = new WeakMap<Element, Map<string, { source: string; rendered: string }>>();

const eligible = (element: Element | null): boolean =>
  !!element && !element.closest(ignoredSelector);

const translateText = (node: Text): void => {
  if (!eligible(node.parentElement)) return;
  const current = node.nodeValue || "";
  const prior = textSource.get(node);
  const source = prior && prior.rendered === current ? prior.source : current;
  const rendered = translateUI(source);
  textSource.set(node, { source, rendered });
  if (current !== rendered) node.nodeValue = rendered;
};

const translateAttributes = (element: Element): void => {
  if (!eligible(element)) return;
  let originals = attrSource.get(element);
  if (!originals) {
    originals = new Map();
    attrSource.set(element, originals);
  }
  for (const attribute of attributes) {
    const current = element.getAttribute(attribute);
    if (current === null) continue;
    const prior = originals.get(attribute);
    const source = prior && prior.rendered === current ? prior.source : current;
    const rendered = translateUI(source);
    originals.set(attribute, { source, rendered });
    if (current !== rendered) element.setAttribute(attribute, rendered);
  }
};

const translateTree = (root: Node): void => {
  if (root.nodeType === Node.TEXT_NODE) {
    translateText(root as Text);
    return;
  }
  if (root.nodeType !== Node.ELEMENT_NODE) return;
  const element = root as Element;
  if (!eligible(element)) return;
  translateAttributes(element);
  const walker = document.createTreeWalker(
    element, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT,
    { acceptNode(node) {
      if (node.nodeType === Node.ELEMENT_NODE && !eligible(node as Element))
        return NodeFilter.FILTER_REJECT;
      return NodeFilter.FILTER_ACCEPT;
    } },
  );
  while (walker.nextNode()) {
    const node = walker.currentNode;
    if (node.nodeType === Node.TEXT_NODE) translateText(node as Text);
    else translateAttributes(node as Element);
  }
};

export const mountLegacyUITranslations = (): (() => void) => {
  applyDocumentLocale();
  if (typeof document === "undefined") return () => {};
  translateTree(document.body);

  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.type === "characterData") translateTree(mutation.target);
      else if (mutation.type === "attributes") translateTree(mutation.target);
      else for (const node of mutation.addedNodes) translateTree(node);
    }
  });
  observer.observe(document.body, {
    subtree: true, childList: true, characterData: true,
    attributes: true, attributeFilter: attributes,
  });
  return () => observer.disconnect();
};

/** Refresh cataloged labels after switching language, including AntD portals. */
export const refreshLegacyUITranslations = (): void => {
  applyDocumentLocale();
  if (typeof document !== "undefined") translateTree(document.body);
};

applyDocumentLocale();
