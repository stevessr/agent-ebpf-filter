<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import MarkdownIt from "markdown-it";
import { message } from "ant-design-vue";
import {
  CopyOutlined,
  FileOutlined,
  LinkOutlined,
  OrderedListOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type { LinuxReferenceEntry } from "../../data/linuxReferenceCatalog";

interface OutlineItem {
  id: string;
  level: 1 | 2 | 3 | 4;
  text: string;
}

const props = withDefaults(
  defineProps<{
    open: boolean;
    entry: LinuxReferenceEntry | null;
  }>(),
  {
    open: false,
  },
);

const emit = defineEmits<{
  (event: "update:open", value: boolean): void;
}>();

const drawerOpen = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value),
});

const activeTab = ref<"preview" | "outline" | "source">("preview");
const loading = ref(false);
const loadError = ref("");
const sourceText = ref("");
const renderedHtml = ref("");
const outlineItems = ref<OutlineItem[]>([]);
const previewScrollRef = ref<HTMLDivElement | null>(null);
const reloadNonce = ref(0);

const markdown = new MarkdownIt({
  html: true,
  linkify: true,
  breaks: true,
  typographer: true,
});

const sourceLineCount = computed(() =>
  sourceText.value ? sourceText.value.split(/\r?\n/).length : 0,
);

const releaseText = computed(() => props.entry?.release ?? "");
const kindText = computed(() =>
  props.entry?.kind === "syscall"
    ? "syscall"
    : props.entry?.kind === "helper"
      ? "eBPF helper"
      : "",
);

const resetPreviewState = () => {
  sourceText.value = "";
  renderedHtml.value = "";
  outlineItems.value = [];
  loadError.value = "";
};

const openDocs = (url: string) => {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
};

const openSnapshot = (path: string) => {
  if (!path) return;
  window.open(path, "_blank", "noopener,noreferrer");
};

const copyText = async (text: string, successMessage = "Copied") => {
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

const reloadCurrent = () => {
  reloadNonce.value += 1;
};

const slugify = (text: string) =>
  text
    .trim()
    .toLowerCase()
    .replace(/['"`]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "section";

const escapeSelector = (value: string) => {
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(value);
  }

  return value.replace(/([!"#$%&'()*+,./:;<=>?@[\\\]^`{|}~])/g, "\\$1");
};

const sanitizeAndOutline = (html: string, entry: LinuxReferenceEntry) => {
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
      const resolved = new URL(href, entry.url);
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
      const resolved = new URL(src, entry.url);
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

watch(
  [() => props.entry?.localPath, () => props.open, reloadNonce],
  async ([path, isOpen], _oldValues, onCleanup) => {
    if (!isOpen || !props.entry || !path) {
      return;
    }

    const entry = props.entry;
    const controller = new AbortController();
    onCleanup(() => controller.abort());

    activeTab.value = "preview";
    loading.value = true;
    resetPreviewState();

    try {
      const response = await fetch(path, { signal: controller.signal });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status} ${response.statusText}`.trim());
      }

      const raw = await response.text();
      if (controller.signal.aborted) return;

      sourceText.value = raw;
      const rendered = markdown.render(raw);
      const { html, outline } = sanitizeAndOutline(rendered, entry);
      renderedHtml.value = html;
      outlineItems.value = outline;
      loadError.value = "";
    } catch (error) {
      if (controller.signal.aborted) return;
      loadError.value =
        error instanceof Error ? error.message : "Failed to load the cached snapshot.";
    } finally {
      if (!controller.signal.aborted) {
        loading.value = false;
      }
    }
  },
  { immediate: true },
);

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
</script>

<template>
  <a-drawer
    v-model:open="drawerOpen"
    rootClassName="docs-preview-drawer"
    :title="entry ? `Document Preview · ${entry.name}` : 'Document Preview'"
    width="92vw"
    :drawerStyle="{ background: 'var(--docs-surface)' }"
    :contentWrapperStyle="{ background: 'var(--docs-surface)' }"
    :headerStyle="{ background: 'var(--docs-surface)' }"
    :bodyStyle="{ padding: '16px 20px 20px', background: 'var(--docs-surface)' }"
    :maskStyle="{ backgroundColor: 'rgba(15, 23, 42, 0.42)' }"
    destroyOnClose
  >
    <a-empty
      v-if="!entry"
      description="Select a syscall or eBPF helper first, then open the popup preview."
      style="padding: 72px 0;"
    />

    <template v-else>
      <div class="docs-preview-shell">
        <div class="docs-preview-meta">
          <div class="docs-preview-meta__head">
            <div>
              <div class="docs-preview-meta__name">{{ entry.name }}</div>
              <div class="docs-preview-meta__summary">{{ entry.summary }}</div>
            </div>

            <a-space size="small" wrap>
              <a-tag color="gold">{{ releaseText }}</a-tag>
              <a-tag :color="entry.kind === 'syscall' ? 'blue' : 'green'">{{ kindText }}</a-tag>
              <a-tag color="purple">{{ entry.category }}</a-tag>
            </a-space>
          </div>

          <div class="docs-preview-meta__synopsis">
            <code>{{ entry.synopsis }}</code>
          </div>

          <div class="docs-preview-meta__details">
            <span>Source: {{ entry.source }}</span>
            <span>•</span>
            <span>Cached path: <code class="docs-preview-inline-code">{{ entry.localPath }}</code></span>
            <span>•</span>
            <span>{{ sourceLineCount }} lines</span>
            <span>•</span>
            <span>{{ outlineItems.length }} headings</span>
          </div>
        </div>

        <a-space wrap class="docs-preview-actions">
          <a-button size="small" @click="openSnapshot(entry.localPath)">
            <FileOutlined /> Open snapshot
          </a-button>
          <a-button size="small" @click="openDocs(entry.url)">
            <LinkOutlined /> Open source
          </a-button>
          <a-button size="small" @click="copyText(entry.url, 'Source URL copied')">
            <CopyOutlined /> Copy source URL
          </a-button>
          <a-button size="small" @click="reloadCurrent">
            <ReloadOutlined /> Reload
          </a-button>
        </a-space>

        <a-tabs v-model:activeKey="activeTab" class="docs-preview-tabs">
          <a-tab-pane key="preview" tab="Preview">
            <div ref="previewScrollRef" class="docs-preview-scroll" @click="onPreviewClick">
              <a-spin :spinning="loading">
                <a-alert
                  v-if="loadError"
                  :message="loadError"
                  type="error"
                  show-icon
                  style="margin-bottom: 12px;"
                >
                  <template #description>
                    <a-space wrap>
                      <span>The local snapshot could not be loaded.</span>
                      <a-button size="small" @click="reloadCurrent">Try again</a-button>
                    </a-space>
                  </template>
                </a-alert>

                <a-empty
                  v-else-if="!renderedHtml"
                  description="Preview content will appear here after the local snapshot is rendered."
                  style="padding: 72px 0;"
                />

                <article v-else class="docs-preview-rendered" v-html="renderedHtml" />
              </a-spin>
            </div>
          </a-tab-pane>

          <a-tab-pane key="outline">
            <template #tab>
              <span class="docs-preview-tab-title">
                <OrderedListOutlined />
                <span>Outline</span>
              </span>
            </template>

            <div class="docs-outline-scroll">
              <a-empty
                v-if="!outlineItems.length"
                description="This snapshot does not expose any headings."
                style="padding: 72px 0;"
              />

              <div v-else class="docs-outline-list">
                <button
                  v-for="item in outlineItems"
                  :key="item.id"
                  class="docs-outline-item"
                  type="button"
                  :style="{ paddingInlineStart: `${(item.level - 1) * 16}px` }"
                  @click="scrollToHeading(item.id)"
                >
                  <span class="docs-outline-item__label">{{ item.text }}</span>
                  <span class="docs-outline-level">h{{ item.level }}</span>
                </button>
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="source" tab="Source">
            <div class="docs-source-scroll">
              <div class="docs-source-toolbar">
                <a-space wrap>
                  <a-tag color="default">{{ sourceLineCount }} lines</a-tag>
                  <a-tag color="default">{{ sourceText.length }} chars</a-tag>
                </a-space>

                <a-button size="small" @click="copyText(sourceText, 'Snapshot source copied')">
                  <CopyOutlined /> Copy snapshot
                </a-button>
              </div>

              <pre class="docs-source-pre">{{ sourceText || "No source content loaded yet." }}</pre>
            </div>
          </a-tab-pane>
        </a-tabs>
      </div>
    </template>
  </a-drawer>
</template>

<style scoped>
:global(.docs-preview-drawer) {
  --docs-surface: #ffffff;
  --docs-surface-2: #f8fafc;
  --docs-border: #e5e7eb;
  --docs-text: #334155;
  --docs-text-strong: #0f172a;
  --docs-text-muted: #64748b;
  --docs-link: #1677ff;
  --docs-link-hover: #0958d9;
  --docs-code-bg: #f3f4f6;
  --docs-code-text: #111827;
  --docs-outline-bg: rgba(22, 119, 255, 0.1);
  --docs-outline-text: #1677ff;
}

@media (prefers-color-scheme: dark) {
  :global(.docs-preview-drawer) {
    --docs-surface: #0f172a;
    --docs-surface-2: #111827;
    --docs-border: #243042;
    --docs-text: #cbd5e1;
    --docs-text-strong: #f8fafc;
    --docs-text-muted: #94a3b8;
    --docs-link: #7ab7ff;
    --docs-link-hover: #9cc4ff;
    --docs-code-bg: #1f2937;
    --docs-code-text: #e5e7eb;
    --docs-outline-bg: rgba(96, 165, 250, 0.14);
    --docs-outline-text: #93c5fd;
  }
}

:global(.docs-preview-drawer .ant-drawer-content) {
  background: var(--docs-surface);
}

:global(.docs-preview-drawer .ant-drawer-header) {
  background: var(--docs-surface);
  border-bottom: 1px solid var(--docs-border);
}

:global(.docs-preview-drawer .ant-drawer-title) {
  color: var(--docs-text-strong);
}

:global(.docs-preview-drawer .ant-drawer-close) {
  color: var(--docs-text-muted);
}

:global(.docs-preview-drawer .ant-drawer-body) {
  background: var(--docs-surface);
  color: var(--docs-text);
}

.docs-preview-shell {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: calc(100vh - 140px);
}

.docs-preview-meta {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.docs-preview-meta__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.docs-preview-meta__name {
  font-size: 22px;
  font-weight: 700;
  color: var(--docs-text-strong);
  line-height: 1.2;
}

.docs-preview-meta__summary {
  margin-top: 6px;
  color: var(--docs-text);
}

.docs-preview-meta__synopsis code,
.docs-preview-inline-code {
  font-family: var(--mono);
}

.docs-preview-meta__synopsis code {
  display: inline-flex;
  width: 100%;
  box-sizing: border-box;
  white-space: normal;
  line-height: 1.5;
  color: var(--docs-code-text);
  background: var(--docs-code-bg);
  border: 1px solid var(--docs-border);
}

.docs-preview-meta__details {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  color: var(--docs-text-muted);
  font-size: 12px;
}

.docs-preview-actions {
  margin-bottom: 2px;
}

.docs-preview-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 12px;
}

.docs-preview-tab-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.docs-preview-scroll,
.docs-outline-scroll,
.docs-source-scroll {
  max-height: calc(100vh - 320px);
  overflow: auto;
  padding-right: 4px;
}

.docs-preview-rendered {
  color: var(--docs-text);
  line-height: 1.75;
}

.docs-preview-rendered :deep(h1),
.docs-preview-rendered :deep(h2),
.docs-preview-rendered :deep(h3),
.docs-preview-rendered :deep(h4) {
  color: var(--docs-text-strong);
  scroll-margin-top: 16px;
  line-height: 1.3;
}

.docs-preview-rendered :deep(h1) {
  font-size: 1.85rem;
  margin: 0 0 12px;
}

.docs-preview-rendered :deep(h2) {
  font-size: 1.5rem;
  margin: 24px 0 12px;
}

.docs-preview-rendered :deep(h3) {
  font-size: 1.18rem;
  margin: 20px 0 10px;
}

.docs-preview-rendered :deep(h4) {
  font-size: 1rem;
  margin: 18px 0 8px;
}

.docs-preview-rendered :deep(p),
.docs-preview-rendered :deep(ul),
.docs-preview-rendered :deep(ol),
.docs-preview-rendered :deep(blockquote),
.docs-preview-rendered :deep(pre),
.docs-preview-rendered :deep(table) {
  margin: 0 0 12px;
}

.docs-preview-rendered :deep(a) {
  color: var(--docs-link);
  text-decoration: none;
}

.docs-preview-rendered :deep(a:hover) {
  color: var(--docs-link-hover);
  text-decoration: underline;
}

.docs-preview-rendered :deep(blockquote) {
  border-inline-start: 4px solid var(--docs-border);
  padding-inline-start: 12px;
  color: var(--docs-text);
  background: var(--docs-surface-2);
}

.docs-preview-rendered :deep(pre) {
  overflow: auto;
  border: 1px solid var(--docs-border);
  border-radius: 10px;
  padding: 12px;
  background: var(--docs-code-bg);
}

.docs-preview-rendered :deep(pre code) {
  padding: 0;
  background: transparent;
  white-space: pre;
}

.docs-preview-rendered :deep(code) {
  font-family: var(--mono);
  font-size: 0.95em;
  color: var(--docs-code-text);
  background: var(--docs-code-bg);
  border: 1px solid var(--docs-border);
}

.docs-preview-rendered :deep(table) {
  display: block;
  width: max-content;
  min-width: 100%;
  overflow: auto;
  border-collapse: collapse;
}

.docs-preview-rendered :deep(th),
.docs-preview-rendered :deep(td) {
  border: 1px solid var(--docs-border);
  padding: 6px 10px;
  vertical-align: top;
}

.docs-preview-rendered :deep(th) {
  color: var(--docs-text-strong);
  background: var(--docs-surface-2);
}

.docs-preview-rendered :deep(img) {
  max-width: 100%;
}

.docs-preview-rendered :deep(mark) {
  padding: 0 2px;
  border-radius: 3px;
  background: #fff2a8;
  color: inherit;
}

.docs-outline-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.docs-outline-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  border: 1px solid var(--docs-border);
  border-radius: 10px;
  padding: 10px 12px;
  background: var(--docs-surface);
  color: var(--docs-text-strong);
  text-align: left;
  cursor: pointer;
}

.docs-outline-item:hover {
  border-color: var(--docs-outline-text);
  background: var(--docs-outline-bg);
}

.docs-outline-item__label {
  min-width: 0;
  flex: 1;
  white-space: normal;
  word-break: break-word;
}

.docs-outline-level {
  flex: 0 0 auto;
  padding: 0 8px;
  border-radius: 999px;
  background: var(--docs-outline-bg);
  color: var(--docs-outline-text);
  font-size: 12px;
  line-height: 20px;
}

.docs-source-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.docs-source-pre {
  margin: 0;
  padding: 16px;
  border: 1px solid var(--docs-border);
  border-radius: 12px;
  background: var(--docs-code-bg);
  color: var(--docs-code-text);
  font-family: var(--mono);
  font-size: 13px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 1199px) {
  .docs-preview-shell {
    min-height: auto;
  }

  .docs-preview-scroll,
  .docs-outline-scroll,
  .docs-source-scroll {
    max-height: none;
  }

  .docs-preview-meta__head,
  .docs-source-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
