<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, triggerRef, watch } from 'vue';
import { createHighlighter, type Highlighter } from 'shiki';
import type { FilePreviewResponse } from '../../types/filePreview';
import { buildRequestHeaders } from '../../utils/requestContext';

let highlighterInstance: Highlighter | null = null;
const getHighlighter = async () => {
  if (!highlighterInstance) {
    highlighterInstance = await createHighlighter({
      themes: ['github-dark'],
      langs: ['cpp', 'python', 'javascript', 'typescript', 'go', 'rust', 'bash', 'json', 'yaml', 'sql', 'html', 'css', 'text'],
    });
  }
  return highlighterInstance;
};

const props = withDefaults(defineProps<{
  open: boolean;
  loading?: boolean;
  preview: FilePreviewResponse | null;
  title?: string;
}>(), {
  loading: false,
  title: 'File Preview',
});

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void;
}>();

const STREAM_CHUNK_FLUSH_LINES = 80;
const VIRTUAL_LINE_HEIGHT = 21;
const VIRTUAL_OVERSCAN_LINES = 40;
const VIRTUAL_DEFAULT_VIEWPORT_HEIGHT = 520;

const drawerOpen = computed({
  get: () => props.open,
  set: (value: boolean) => emit('update:open', value),
});

const formatBytes = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const base = 1024;
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(base)), units.length - 1);
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
};

const formattedModTime = computed(() => {
  if (!props.preview?.modTime) return '—';
  const date = new Date(props.preview.modTime);
  return Number.isNaN(date.getTime()) ? props.preview.modTime : date.toLocaleString();
});

const highlightedHtml = ref('');
const highlightLoading = ref(false);
const wordWrap = ref(true);
const streamLoading = ref(false);
const streamError = ref('');
const streamBytesLoaded = ref(0);
const streamLines = shallowRef<string[]>([]);
const virtualScrollTop = ref(0);
const virtualViewportHeight = ref(VIRTUAL_DEFAULT_VIEWPORT_HEIGHT);
const virtualScrollerRef = ref<HTMLElement | null>(null);
let streamAbortController: AbortController | null = null;
let streamLineCarry = '';
let pendingStreamLineCount = 0;
let highlightRunId = 0;

const isStreamingText = computed(() => (
  props.open &&
  props.preview?.previewType === 'text' &&
  props.preview.streamable === true
));
const streamProgressPercent = computed(() => {
  const size = props.preview?.size || 0;
  if (!size) return 0;
  return Math.min(100, Math.round((streamBytesLoaded.value / size) * 100));
});
const virtualTotalLines = computed(() => streamLines.value.length);
const virtualTotalHeight = computed(() => Math.max(virtualTotalLines.value * VIRTUAL_LINE_HEIGHT, virtualViewportHeight.value));
const virtualStartLine = computed(() => Math.max(0, Math.floor(virtualScrollTop.value / VIRTUAL_LINE_HEIGHT) - VIRTUAL_OVERSCAN_LINES));
const virtualVisibleLineCount = computed(() => Math.ceil(virtualViewportHeight.value / VIRTUAL_LINE_HEIGHT) + VIRTUAL_OVERSCAN_LINES * 2);
const virtualEndLine = computed(() => Math.min(virtualTotalLines.value, virtualStartLine.value + virtualVisibleLineCount.value));
const virtualTopOffset = computed(() => virtualStartLine.value * VIRTUAL_LINE_HEIGHT);
const virtualRenderedLines = computed(() => streamLines.value.slice(virtualStartLine.value, virtualEndLine.value));
const virtualStatus = computed(() => {
  if (!isStreamingText.value) return '';
  if (!virtualTotalLines.value) return streamLoading.value ? 'Loading first slice…' : 'No text loaded';
  return `Rendering lines ${virtualStartLine.value + 1}-${virtualEndLine.value} of ${virtualTotalLines.value}`;
});

const videoUrl = computed(() => {
  if (!props.preview?.path) return '';
  return `/system/download?path=${encodeURIComponent(props.preview.path)}`;
});

const stopStreaming = () => {
  if (streamAbortController) {
    streamAbortController.abort();
    streamAbortController = null;
  }
  streamLoading.value = false;
};

const resetStreamState = () => {
  stopStreaming();
  streamError.value = '';
  streamBytesLoaded.value = 0;
  streamLineCarry = '';
  pendingStreamLineCount = 0;
  virtualScrollTop.value = 0;
  streamLines.value = [];
};

const syncVirtualViewport = async () => {
  await nextTick();
  const el = virtualScrollerRef.value;
  if (!el) return;
  virtualViewportHeight.value = el.clientHeight || VIRTUAL_DEFAULT_VIEWPORT_HEIGHT;
};

const handleVirtualScroll = (event: Event) => {
  const el = event.currentTarget as HTMLElement;
  virtualScrollTop.value = el.scrollTop;
  virtualViewportHeight.value = el.clientHeight || VIRTUAL_DEFAULT_VIEWPORT_HEIGHT;
};

const flushStreamLines = (force = false) => {
  if (!force && pendingStreamLineCount < STREAM_CHUNK_FLUSH_LINES) return;
  pendingStreamLineCount = 0;
  triggerRef(streamLines);
};

const appendStreamText = (text: string) => {
  if (!text) return;
  const parts = (streamLineCarry + text).split(/\r?\n/);
  streamLineCarry = parts.pop() ?? '';
  if (!parts.length) return;
  streamLines.value.push(...parts);
  pendingStreamLineCount += parts.length;
  flushStreamLines();
};

const finishStreamLines = () => {
  if (streamLineCarry || !streamLines.value.length) {
    streamLines.value.push(streamLineCarry.replace(/^﻿/, ''));
  }
  streamLineCarry = '';
  flushStreamLines(true);
};

const normalizeTextEncoding = (encoding?: string) => {
  const value = (encoding || 'utf-8').toLowerCase();
  if (value === 'utf-16le' || value === 'utf-16be') return value;
  return 'utf-8';
};

const streamDecoderEncoding = computed(() => normalizeTextEncoding(props.preview?.encoding));

const streamTextPreview = async (path: string) => {
  resetStreamState();
  void syncVirtualViewport();
  const controller = new AbortController();
  streamAbortController = controller;
  streamLoading.value = true;

  try {
    const response = await fetch(`/system/file-preview/stream?path=${encodeURIComponent(path)}`, {
      headers: buildRequestHeaders(),
      signal: controller.signal,
    });
    if (!response.ok) {
      throw new Error(`Stream failed: ${response.status}`);
    }
    if (!response.body) {
      throw new Error('Streaming is not supported by this browser');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder(streamDecoderEncoding.value);
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      streamBytesLoaded.value += value.byteLength;
      appendStreamText(decoder.decode(value, { stream: true }));
    }
    appendStreamText(decoder.decode());
    finishStreamLines();
  } catch (err) {
    if ((err as Error).name !== 'AbortError') {
      streamError.value = (err as Error).message || 'Failed to stream file preview';
    }
  } finally {
    if (streamAbortController === controller) {
      streamAbortController = null;
      streamLoading.value = false;
    }
  }
};

watch(() => props.open, (isOpen) => {
  if (!isOpen) {
    highlightedHtml.value = '';
    highlightLoading.value = false;
    resetStreamState();
    return;
  }
  void syncVirtualViewport();
});

watch(
  () => [props.open, props.preview?.path, props.preview?.previewType, props.preview?.streamable] as const,
  ([isOpen, path, previewType, streamable]) => {
    if (isOpen && path && previewType === 'text' && streamable) {
      void streamTextPreview(path);
      return;
    }
    resetStreamState();
  },
  { immediate: true },
);

watch(
  () => [props.preview?.path, props.preview?.previewType, props.preview?.language, props.preview?.content, props.preview?.streamable] as const,
  async ([, previewType, language, content, streamable]) => {
    const runId = ++highlightRunId;
    if (previewType === 'text' && content && !streamable) {
      highlightLoading.value = true;
      try {
        const lang = language || 'text';
        const hl = await getHighlighter();

        if (!hl.getLoadedLanguages().includes(lang)) {
          try {
            await hl.loadLanguage(lang as any);
          } catch (e) {
            console.warn(`Language ${lang} not supported by shiki`);
          }
        }

        if (runId !== highlightRunId) return;
        highlightedHtml.value = hl.codeToHtml(content, {
          lang: hl.getLoadedLanguages().includes(lang) ? lang : 'text',
          theme: 'github-dark',
        });
      } catch (err) {
        console.error('Failed to highlight code', err);
        if (runId === highlightRunId) highlightedHtml.value = '';
      } finally {
        if (runId === highlightRunId) highlightLoading.value = false;
      }
    } else {
      highlightedHtml.value = '';
      highlightLoading.value = false;
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  resetStreamState();
});
</script>

<template>
  <a-drawer v-model:open="drawerOpen" :title="title" width="85vw" destroyOnClose>
    <a-spin :spinning="loading">
      <a-empty v-if="!preview && !loading" description="No preview available" />

      <template v-else-if="preview">
        <a-descriptions bordered :column="2" size="small" style="margin-bottom: 16px;">
          <a-descriptions-item label="Path" :span="2">
            <a-typography-text code style="word-break: break-all; color: #111827;">{{ preview.path }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Type">
            <a-tag :color="preview.isDir ? 'blue' : 'default'">{{ preview.previewType.toUpperCase() }}</a-tag>
            <a-tag v-if="preview.mimeType">{{ preview.mimeType }}</a-tag>
            <a-tag v-if="preview.streamable" color="green">VIRTUAL STREAM</a-tag>
            <a-tag v-else-if="preview.truncated" color="orange">TRUNCATED</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Size">{{ formatBytes(preview.size) }}</a-descriptions-item>
          <a-descriptions-item label="Mode">{{ preview.mode || '—' }}</a-descriptions-item>
          <a-descriptions-item label="Modified">{{ formattedModTime }}</a-descriptions-item>
        </a-descriptions>

        <a-alert
          v-if="preview.previewType === 'directory'"
          type="info"
          show-icon
          message="Directory selected"
          description="Directories can be jumped to, but not inline-previewed as file content."
        />

        <div v-else-if="preview.previewType === 'image'" class="file-preview-drawer__content">
          <a-alert
            v-if="!preview.dataUrl && preview.content"
            type="info"
            show-icon
            :message="preview.content"
          />
          <img
            v-else-if="preview.dataUrl"
            :src="preview.dataUrl"
            :alt="preview.name"
            class="file-preview-drawer__image"
          />
        </div>

        <div v-else-if="preview.previewType === 'video'" class="file-preview-drawer__content">
          <video
            v-if="preview.mimeType.startsWith('video/')"
            controls
            autoplay
            style="width: 100%; max-height: 70vh; border-radius: 8px; background: #000;"
            :src="videoUrl">
            Your browser does not support the video tag.
          </video>
          <div v-else-if="preview.mimeType.startsWith('audio/')" style="padding: 40px; background: #f0f2f5; border-radius: 8px; text-align: center;">
            <div style="margin-bottom: 16px; font-size: 48px;">🎵</div>
            <audio controls autoplay style="width: 100%;" :src="videoUrl">
              Your browser does not support the audio element.
            </audio>
            <div style="margin-top: 12px; color: #666; font-size: 14px;">{{ preview.name }}</div>
          </div>
        </div>

        <div v-else-if="preview.previewType === 'text'" class="file-preview-drawer__content">
          <div class="file-preview-drawer__text-toolbar">
            <span>Language: {{ preview.language || 'text' }}</span>
            <template v-if="preview.streamable">
              <span>{{ formatBytes(streamBytesLoaded) }} / {{ formatBytes(preview.size) }}</span>
              <span>{{ virtualStatus }}</span>
              <a-progress :percent="streamProgressPercent" size="small" class="file-preview-drawer__progress" />
              <a-button v-if="streamLoading" size="small" @click="stopStreaming">Stop</a-button>
            </template>
            <a-checkbox v-if="!isStreamingText" v-model:checked="wordWrap" size="small">Word Wrap</a-checkbox>
          </div>
          <a-alert
            v-if="isStreamingText"
            type="info"
            show-icon
            message="Virtualized stream preview"
            description="The file is sliced into lines and only the visible window is rendered into DOM. Word wrap is disabled for stable virtual scrolling."
            style="margin-bottom: 12px;"
          />
          <a-alert
            v-if="streamError"
            type="error"
            show-icon
            :message="streamError"
            style="margin-bottom: 12px;"
          />
          <div
            v-if="isStreamingText"
            ref="virtualScrollerRef"
            class="file-preview-drawer__virtual-scroller"
            @scroll="handleVirtualScroll"
          >
            <div class="file-preview-drawer__virtual-spacer" :style="{ height: `${virtualTotalHeight}px` }">
              <pre
                class="file-preview-drawer__virtual-pre"
                :style="{ transform: `translateY(${virtualTopOffset}px)` }"
              ><template v-for="(line, index) in virtualRenderedLines" :key="virtualStartLine + index">{{ line }}{{ '\n' }}</template></pre>
            </div>
          </div>
          <a-spin v-else :spinning="highlightLoading">
            <div
              v-if="highlightedHtml"
              class="file-preview-drawer__shiki"
              :class="{ 'is-wrapped': wordWrap }"
              v-html="highlightedHtml">
            </div>
            <pre v-else class="file-preview-drawer__pre" :class="{ 'is-wrapped': wordWrap }">{{ preview.content }}</pre>
          </a-spin>
        </div>

        <div v-else class="file-preview-drawer__content">
          <a-alert
            type="warning"
            show-icon
            message="Binary file preview"
            description="Showing a limited hex dump."
            style="margin-bottom: 12px;"
          />
          <pre class="file-preview-drawer__pre">{{ preview.content || 'Binary preview unavailable.' }}</pre>
        </div>
      </template>
    </a-spin>
  </a-drawer>
</template>

<style scoped>
.file-preview-drawer__content {
  max-width: 100%;
}

.file-preview-drawer__text-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  font-size: 12px;
  color: #888;
}

.file-preview-drawer__progress {
  width: 180px;
}

.file-preview-drawer__pre,
.file-preview-drawer__shiki :deep(pre) {
  margin: 0;
  padding: 16px;
  max-height: calc(100vh - 280px);
  overflow: auto;
  border-radius: 8px;
  background: #0f172a !important;
  color: #e2e8f0;
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}

.file-preview-drawer__virtual-scroller {
  height: calc(100vh - 330px);
  min-height: 360px;
  overflow: auto;
  border-radius: 8px;
  background: #0f172a;
}

.file-preview-drawer__virtual-spacer {
  position: relative;
  min-width: max-content;
}

.file-preview-drawer__virtual-pre {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  margin: 0;
  padding: 16px;
  color: #e2e8f0;
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 13px;
  line-height: 21px;
  white-space: pre;
  overflow: visible;
}

.file-preview-drawer__shiki :deep(.line) {
  display: block;
  min-height: 1.5em;
}

.is-wrapped,
.is-wrapped :deep(pre) {
  white-space: pre-wrap !important;
  word-break: break-all !important;
  overflow-wrap: anywhere !important;
}

.file-preview-drawer__image {
  display: block;
  max-width: 100%;
  max-height: calc(100vh - 260px);
  margin: 0 auto;
  border-radius: 8px;
  border: 1px solid #f0f0f0;
  background: #fafafa;
}
</style>
