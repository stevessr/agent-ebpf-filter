<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
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
const streamedContent = ref('');
const streamLoading = ref(false);
const streamError = ref('');
const streamBytesLoaded = ref(0);
let streamAbortController: AbortController | null = null;
let highlightRunId = 0;

const isStreamingText = computed(() => (
  props.open &&
  props.preview?.previewType === 'text' &&
  props.preview.streamable === true
));
const textContent = computed(() => (isStreamingText.value ? streamedContent.value : props.preview?.content || ''));
const streamProgressPercent = computed(() => {
  const size = props.preview?.size || 0;
  if (!size) return 0;
  return Math.min(100, Math.round((streamBytesLoaded.value / size) * 100));
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
  streamedContent.value = '';
  streamError.value = '';
  streamBytesLoaded.value = 0;
};

const streamTextPreview = async (path: string) => {
  resetStreamState();
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
    const decoder = new TextDecoder('utf-8');
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      streamBytesLoaded.value += value.byteLength;
      streamedContent.value += decoder.decode(value, { stream: true });
    }
    streamedContent.value += decoder.decode();
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
  }
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
            <a-tag v-if="preview.streamable" color="green">STREAMING</a-tag>
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
              <a-progress :percent="streamProgressPercent" size="small" class="file-preview-drawer__progress" />
              <a-button v-if="streamLoading" size="small" @click="stopStreaming">Stop</a-button>
            </template>
            <a-checkbox v-model:checked="wordWrap" size="small">Word Wrap</a-checkbox>
          </div>
          <a-alert
            v-if="streamError"
            type="error"
            show-icon
            :message="streamError"
            style="margin-bottom: 12px;"
          />
          <a-spin :spinning="highlightLoading && !isStreamingText">
            <div
              v-if="highlightedHtml && !isStreamingText"
              class="file-preview-drawer__shiki"
              :class="{ 'is-wrapped': wordWrap }"
              v-html="highlightedHtml">
            </div>
            <pre v-else class="file-preview-drawer__pre" :class="{ 'is-wrapped': wordWrap }">{{ textContent }}</pre>
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
