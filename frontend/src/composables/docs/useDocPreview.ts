import { computed, nextTick, ref, watch } from "vue";
import MarkdownIt from "markdown-it";
import { message } from "ant-design-vue";
import type { LinuxReferenceEntry } from "../../data/linuxReferenceCatalog";

export interface OutlineItem {
  id: string;
  level: 1 | 2 | 3 | 4;
  text: string;
}

const markdown = new MarkdownIt({
  html: true,
  linkify: true,
  breaks: true,
  typographer: true,
});

export const slugify = (text: string) =>
  text
    .trim()
    .toLowerCase()
    .replace(/['"`]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "section";

export const escapeSelector = (value: string) => {
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(value);
  }
  return value.replace(/([!"#$%&'()*+,./:;<=>?@[\\\]^`{|}~])/g, "\\$1");
};

export const sanitizeAndOutline = (html: string, entryUrl: string) => {
  if (typeof window === "undefined" || typeof DOMParser === "undefined") {
    return { html, outline: [] as OutlineItem[] };
  }

  const parser = new DOMParser();
  const doc = parser.parseFromString(html, "text/html");
  const root = doc.body;
  const headingCounts = new Map<string, number>();
  const outline: OutlineItem[] = [];

  root.querySelectorAll("script, style, iframe, object, embed, noscript").forEach((node) => node.remove());

  root.querySelectorAll("*").forEach((element) => {
    Array.from(element.attributes).forEach((attr) => {
      const attrName = attr.name.toLowerCase();
      const attrValue = attr.value.trim();

      if (attrName.startsWith("on")) {
        element.removeAttribute(attr.name);
        return;
      }

      if (attrName === "style") {
        element.removeAttribute(attr.name);
        return;
      }

      if (
        (attrName === "href" || attrName === "src") &&
        attrValue.toLowerCase().startsWith("javascript:")
      ) {
        element.removeAttribute(attr.name);
      }
    });
  });

  root.querySelectorAll("a[href]").forEach((anchor) => {
    const href = anchor.getAttribute("href")?.trim() ?? "";
    if (!href) return;

    if (href.startsWith("#")) {
      anchor.setAttribute("href", href);
      anchor.setAttribute("data-doc-anchor", href.slice(1));
      return;
    }

    try {
      const resolved = new URL(href, entryUrl);
      if (resolved.protocol === "http:" || resolved.protocol === "https:") {
        anchor.setAttribute("href", resolved.toString());
        anchor.setAttribute("target", "_blank");
        anchor.setAttribute("rel", "noopener noreferrer");
      }
    } catch {
      // Keep the original link if it cannot be resolved.
    }
  });

  root.querySelectorAll("img[src]").forEach((img) => {
    const src = img.getAttribute("src")?.trim() ?? "";
    if (!src) return;

    try {
      const resolved = new URL(src, entryUrl);
      if (resolved.protocol === "http:" || resolved.protocol === "https:") {
        img.setAttribute("src", resolved.toString());
        img.setAttribute("loading", "lazy");
      }
    } catch {
      // Leave the image source untouched when it cannot be resolved.
    }
  });

  root.querySelectorAll("h1, h2, h3, h4").forEach((heading) => {
    const level = Number(heading.tagName.slice(1)) as OutlineItem["level"];
    const text = heading.textContent?.trim() || "Section";
    const baseId = slugify(text);
    const nextCount = (headingCounts.get(baseId) ?? 0) + 1;
    headingCounts.set(baseId, nextCount);
    const id = nextCount === 1 ? baseId : `${baseId}-${nextCount}`;

    heading.setAttribute("id", id);
    outline.push({ id, level, text });
  });

  return {
    html: root.innerHTML,
    outline,
  };
};

export const copyText = async (text: string, successMessage = "Copied") => {
  try {
    if (!text) return;

    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      const textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.readOnly = true;
      textarea.style.position = "fixed";
      textarea.style.opacity = "0";
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
    }

    message.success(successMessage);
  } catch {
    message.error("Failed to copy text");
  }
};

export const openDocs = (url: string) => {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
};

export const openSnapshot = (path: string) => {
  if (!path) return;
  window.open(path, "_blank", "noopener,noreferrer");
};

export function useDocPreview() {
  const activeTab = ref<"preview" | "outline" | "source">("preview");
  const loading = ref(false);
  const loadError = ref("");
  const sourceText = ref("");
  const renderedHtml = ref("");
  const outlineItems = ref<OutlineItem[]>([]);
  const previewScrollRef = ref<HTMLDivElement | null>(null);
  const reloadNonce = ref(0);

  const sourceLineCount = computed(() =>
    sourceText.value ? sourceText.value.split(/\r?\n/).length : 0,
  );

  const releaseText = computed(() => '');
  const kindText = computed(() => '');

  const resetPreviewState = () => {
    sourceText.value = "";
    renderedHtml.value = "";
    outlineItems.value = [];
    loadError.value = "";
  };

  const reloadCurrent = () => {
    reloadNonce.value += 1;
  };

  const loadContent = async (entry: LinuxReferenceEntry, signal: AbortSignal) => {
    const path = entry.localPath;
    activeTab.value = "preview";
    loading.value = true;
    resetPreviewState();

    try {
      const response = await fetch(path, { signal });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status} ${response.statusText}`.trim());
      }

      const raw = await response.text();
      if (signal.aborted) return;

      sourceText.value = raw;
      const rendered = markdown.render(raw);
      const { html, outline } = sanitizeAndOutline(rendered, entry.url);
      renderedHtml.value = html;
      outlineItems.value = outline;
      loadError.value = "";
    } catch (error) {
      if (signal.aborted) return;
      loadError.value =
        error instanceof Error ? error.message : "Failed to load the cached snapshot.";
    } finally {
      if (!signal.aborted) {
        loading.value = false;
      }
    }
  };

  const scrollToHeading = async (id: string) => {
    activeTab.value = "preview";
    await nextTick();

    const container = previewScrollRef.value;
    if (!container) return;

    const target = container.querySelector<HTMLElement>(`#${escapeSelector(id)}`);
    if (!target) return;

    const containerRect = container.getBoundingClientRect();
    const targetRect = target.getBoundingClientRect();
    const top = container.scrollTop + (targetRect.top - containerRect.top) - 12;

    container.scrollTo({
      top: Math.max(top, 0),
      behavior: "smooth",
    });
  };

  const onPreviewClick = (event: MouseEvent) => {
    const target = event.target as HTMLElement | null;
    const anchor = target?.closest("a[href]") as HTMLAnchorElement | null;
    if (!anchor) return;

    const href = anchor.getAttribute("href")?.trim() ?? "";
    if (!href.startsWith("#")) return;

    event.preventDefault();
    void scrollToHeading(href.slice(1));
  };

  return {
    activeTab,
    loading,
    loadError,
    sourceText,
    renderedHtml,
    outlineItems,
    previewScrollRef,
    reloadNonce,
    sourceLineCount,
    resetPreviewState,
    reloadCurrent,
    loadContent,
    scrollToHeading,
    onPreviewClick,
  };
}
