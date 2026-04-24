<script setup lang="ts">
import { computed } from 'vue';

import type { FilePreviewResponse } from '../types/filePreview';

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
</script>

<template>
  <a-drawer v-model:open="drawerOpen" :title="title" width="720">
    <a-spin :spinning="loading">
      <a-empty v-if="!preview && !loading" description="No preview available" />

      <template v-else-if="preview">
        <a-descriptions bordered :column="1" size="small" style="margin-bottom: 16px;">
          <a-descriptions-item label="Path">
            <a-typography-text code style="word-break: break-all;">{{ preview.path }}</a-typography-text>
          </a-descriptions-item>
          <a-descriptions-item label="Parent">
            <a-typography-text code style="word-break: break-all;">{{ preview.parentDir }}</a-typography-text>
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

        <div v-else-if="preview.previewType === 'text'" class="file-preview-drawer__content">
          <pre class="file-preview-drawer__pre">{{ preview.content }}</pre>
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

.file-preview-drawer__pre {
  margin: 0;
  padding: 12px;
  max-height: calc(100vh - 280px);
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  border-radius: 8px;
  background: #0f172a;
  color: #e2e8f0;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  font-size: 12px;
  line-height: 1.55;
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
