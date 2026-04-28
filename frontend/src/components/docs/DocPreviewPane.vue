<script setup lang="ts">
import { computed, ref, watch } from "vue";
import MarkdownIt from "markdown-it";
import { message } from "ant-design-vue";
import {
  CopyOutlined,
  FileOutlined,
  LinkOutlined,
  ReloadOutlined,
} from "@ant-design/icons-vue";
import type { LinuxReferenceEntry } from "../../data/linuxReferenceCatalog";

const props = defineProps<{
  entry: LinuxReferenceEntry | null;
}>();

const markdown = new MarkdownIt({
  html: true,
  linkify: true,
  breaks: true,
  typographer: true,
});

const loading = ref(false);
const sourceText = ref("");
const loadError = ref("");

const copyText = async (text: string) => {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      message.success("Link copied");
      return;
    }

    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.readOnly = true;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
    message.success("Link copied");
  } catch (err) {
    message.error("Failed to copy link");
  }
};

const openUrl = (url: string) => {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
};

const loadPreview = async () => {
  const entry = props.entry;
  if (!entry) {
    sourceText.value = "";
    loadError.value = "";
    return;
  }

  loading.value = true;
  loadError.value = "";

  try {
    const response = await fetch(entry.localPath, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status} ${response.statusText}`);
    }
    sourceText.value = await response.text();
  } catch (err) {
    sourceText.value = "";
    loadError.value = err instanceof Error ? err.message : String(err);
  } finally {
    loading.value = false;
  }
};

watch(() => props.entry?.localPath, loadPreview, { immediate: true });

const renderedBodyHtml = computed(() => {
  if (!props.entry || !sourceText.value) return "";
  return markdown.render(sourceText.value);
});

const previewSrcDoc = computed(() => {
  if (!props.entry) return "";

  const body = renderedBodyHtml.value || "<p>No content loaded.</p>";
  const escapedBase = props.entry.url.replace(/"/g, "&quot;");

  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <base href="${escapedBase}">
    <style>
      :root {
        color-scheme: light;
      }
      html, body {
        margin: 0;
        padding: 0;
        background: #ffffff;
        color: #1f2937;
        font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
        font-size: 14px;
        line-height: 1.7;
      }
      body {
        padding: 20px 22px 28px;
        overflow: auto;
      }
      a {
        color: #1677ff;
        text-decoration: underline;
        pointer-events: none;
      }
      img, svg, video {
        max-width: 100%;
      }
      pre, code {
        font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
      }
      pre {
        padding: 12px 14px;
        overflow: auto;
        white-space: pre-wrap;
        word-break: break-word;
        border-radius: 8px;
        background: #0f172a;
        color: #e2e8f0;
      }
      code {
        color: #b91c1c;
        background: rgba(148, 163, 184, 0.16);
        padding: 0 4px;
        border-radius: 4px;
      }
      blockquote {
        margin: 1em 0;
        padding: 0.25em 1em;
        border-left: 4px solid #dbeafe;
        background: #f8fbff;
        color: #334155;
      }
      table {
        width: 100%;
        border-collapse: collapse;
        margin: 1em 0;
      }
      th, td {
        border: 1px solid #e5e7eb;
        padding: 8px 10px;
        text-align: left;
        vertical-align: top;
      }
      th {
        background: #f8fafc;
        font-weight: 600;
      }
      h1, h2, h3, h4 {
        line-height: 1.3;
        margin-top: 1.4em;
      }
      h1 { font-size: 28px; }
      h2 { font-size: 22px; }
      h3 { font-size: 18px; }
      h4 { font-size: 16px; }
      ul, ol {
        padding-left: 1.4em;
      }
      .local-snapshot-note {
        margin: 0 0 18px;
        padding: 10px 12px;
        border-radius: 8px;
        background: #eff6ff;
        color: #1e3a8a;
        border: 1px solid #bfdbfe;
      }
    </style>
  </head>
  <body>
    <div class="local-snapshot-note">
      Local snapshot: ${props.entry.release}
    </div>
    ${body}
  </body>
</html>`;
});

const refreshPreview = () => {
  void loadPreview();
};
</script>

<template>
  <a-card size="small" class="docs-preview-card">
    <template #title>
      <span style="display: inline-flex; align-items: center; gap: 8px;">
        <FileOutlined />
        Doc Preview
      </span>
    </template>

    <template #extra>
      <a-tag color="gold" v-if="entry">{{ entry.release }}</a-tag>
      <a-tag v-if="entry" :color="entry.kind === 'syscall' ? 'blue' : 'green'">
        {{ entry.kind === "syscall" ? "syscall" : "eBPF helper" }}
      </a-tag>
    </template>

    <a-empty
      v-if="!entry"
      description="Select a syscall or helper on the left to preview its local snapshot."
      style="padding: 44px 0;"
    />

    <template v-else>
      <a-descriptions bordered size="small" :column="1" style="margin-bottom: 16px;">
        <a-descriptions-item label="Name">
          <a-typography-text code>{{ entry.name }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Category">
          {{ entry.category }}
        </a-descriptions-item>
        <a-descriptions-item label="Synopsis">
          <code style="white-space: normal;">{{ entry.synopsis }}</code>
        </a-descriptions-item>
        <a-descriptions-item label="Local snapshot">
          <a-typography-text code style="word-break: break-all;">{{ entry.localPath }}</a-typography-text>
        </a-descriptions-item>
        <a-descriptions-item label="Source">
          <a-typography-link :href="entry.url" target="_blank" rel="noopener noreferrer">
            {{ entry.url }}
          </a-typography-link>
        </a-descriptions-item>
      </a-descriptions>

      <a-alert
        v-if="loadError"
        type="warning"
        show-icon
        :message="`Unable to load local snapshot: ${loadError}`"
        style="margin-bottom: 12px;"
      />

      <a-space wrap style="margin-bottom: 12px;">
        <a-button type="primary" @click="openUrl(entry.localPath)">
          <FileOutlined /> Open snapshot
        </a-button>
        <a-button @click="openUrl(entry.url)">
          <LinkOutlined /> Open source
        </a-button>
        <a-button @click="copyText(entry.url)">
          <CopyOutlined /> Copy source URL
        </a-button>
        <a-button @click="refreshPreview" :loading="loading">
          <ReloadOutlined /> Reload
        </a-button>
      </a-space>

      <a-spin :spinning="loading">
        <iframe
          v-if="previewSrcDoc"
          class="docs-preview-frame"
          :srcdoc="previewSrcDoc"
          title="Linux docs preview"
        />
        <a-empty
          v-else
          description="No preview content available yet."
          style="padding: 44px 0;"
        />
      </a-spin>
    </template>
  </a-card>
</template>

<style scoped>
.docs-preview-card {
  position: sticky;
  top: 16px;
}

.docs-preview-frame {
  width: 100%;
  height: min(74vh, 920px);
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
}

.docs-preview-card :deep(.ant-descriptions-item-label) {
  width: 140px;
  white-space: nowrap;
}
</style>
