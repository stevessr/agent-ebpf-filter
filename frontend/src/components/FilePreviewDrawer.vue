<script setup lang="ts">
import { computed, ref, watchEffect } from 'vue';
import { createHighlighter, type Highlighter } from 'shiki';
import type { FilePreviewResponse } from '../types/filePreview';

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

const videoUrl = computed(() => {
  if (!props.preview?.path) return '';
  return `/system/download?path=${encodeURIComponent(props.preview.path)}`;
});

watchEffect(async () => {
  if (props.preview?.previewType === 'text' && props.preview.content) {
    highlightLoading.value = true;
    try {
      const lang = props.preview.language || 'text';
      const hl = await getHighlighter();
      
      if (!hl.getLoadedLanguages().includes(lang)) {
        try {
          await hl.loadLanguage(lang as any);
        } catch (e) {
          console.warn(`Language ${lang} not supported by shiki`);
        }
      }

      highlightedHtml.value = hl.codeToHtml(props.preview.content, {
        lang: hl.getLoadedLanguages().includes(lang) ? lang : 'text',
        theme: 'github-dark',
      });
    } catch (err) {
      console.error('Failed to highlight code', err);
      highlightedHtml.value = '';
    } finally {
      highlightLoading.value = false;
    }
  } else {
    highlightedHtml.value = '';
  }
});
</script>

<template>
  <a-drawer v-model:open="drawerOpen" :title="title" width="85vw">
    <a-spin :spinning="loading">
      <a-empty v-if="!preview && !loading" description="No preview available" />

      <template v-else-if="preview">
        <a-descriptions bordered :column="2" size="small" style="margin-bottom: 16px;">
          <a-descriptions-item label="Path" :span="2">
            <a-typography-text code style="word-break: break-all;">{{ preview.path }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Type">
            <a-tag :color="preview.isDir ? 'blue' : 'default'">{{ preview.previewType.toUpperCase() }}</a-tag>
            <a-tag v-if="preview.mimeType">{{ preview.mimeType }}</a-tag>
            <a-tag v-if="preview.truncated" color="orange">TRUNCATED</a-tag>
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
            controls 
            autoplay
            style="width: 100%; max-height: 70vh; border-radius: 8px; background: #000;"
            :src="videoUrl">
            Your browser does not support the video tag.
          </video>
        </div>

        <div v-else-if="preview.previewType === 'text'" class="file-preview-drawer__content">
          <div style="display: flex; justify-content: flex-end; margin-bottom: 8px; gap: 8px; align-items: center;">
            <span style="font-size: 12px; color: #888;">Language: {{ preview.language }}</span>
            <a-checkbox v-model:checked="wordWrap" size="small">Word Wrap</a-checkbox>
          </div>
          <a-spin :spinning="highlightLoading">
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
