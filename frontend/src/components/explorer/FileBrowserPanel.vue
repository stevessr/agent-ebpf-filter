<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import {
  FolderOutlined,
  FileOutlined,
  LeftOutlined,
  PlusOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  UnorderedListOutlined,
  AppstoreOutlined,
  DownloadOutlined,
  UploadOutlined,
  HomeOutlined,
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import FilePreviewDrawer from './FilePreviewDrawer.vue';
import type { FilePreviewResponse } from '../../types/filePreview';

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
  mimeType?: string;
  size?: number;
  modTime?: string;
}

const props = withDefaults(defineProps<{
  routePath?: string;
  actionLabel?: string;
  actionType?: 'track' | 'emit';
  showTrackingControls?: boolean;
  showUpload?: boolean;
  alertMessage?: string;
  alertDescription?: string;
  fileActionOnly?: boolean;
  previewTitle?: string;
}>(), {
  routePath: '',
  actionLabel: 'Track path',
  actionType: 'track',
  showTrackingControls: true,
  showUpload: true,
  alertMessage: 'Path tracking is exact-match',
  alertDescription: 'Adding a file or directory here tracks that exact path string only. Directory entries are not tracked recursively.',
  fileActionOnly: false,
  previewTitle: 'Explorer File Preview',
});

const emit = defineEmits<{
  action: [entry: FileEntry];
}>();

const currentPath = ref('');
const entries = ref<FileEntry[]>([]);
const loading = ref(false);
const tags = ref<string[]>([]);
const selectedTag = ref('Security');
const showHidden = ref(false);
const viewMode = ref<'list' | 'grid'>('grid');
const selectedPath = ref('');
const previewLoading = ref(false);
const showPreview = ref(false);
const previewData = ref<FilePreviewResponse | null>(null);
const homePath = ref('/');
const gridItemSize = ref(100);
const route = useRoute();
const router = useRouter();

const pageSize = ref(50);
const currentPage = ref(1);
const totalItems = ref(0);

const hasRouteSync = computed(() => Boolean(props.routePath));
const previewRequested = computed(() => route.query.preview === '1' || route.query.preview === 'true');
const paginatedEntries = computed(() => entries.value);

const isImage = (entry: FileEntry) => entry.mimeType?.startsWith('image/');
const getImageUrl = (path: string) => `/system/download?path=${encodeURIComponent(path)}`;

const formatBytes = (value: number | undefined) => {
  if (value === undefined) return '-';
  if (value === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(value) / Math.log(k));
  return parseFloat((value / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const formatTime = (time: string | undefined) => {
  if (!time) return '-';
  return new Date(time).toLocaleString();
};

const fetchEntries = async (path: string, force = false) => {
  if (!force && currentPath.value === path && entries.value.length > 0) {
    return;
  }
  loading.value = true;
  try {
    const offset = (currentPage.value - 1) * pageSize.value;
    const res = await axios.get('/system/ls', {
      params: {
        path,
        offset,
        limit: pageSize.value,
        showHidden: showHidden.value,
      },
    });
    entries.value = res.data.items || [];
    totalItems.value = res.data.total || 0;
    currentPath.value = path;
  } catch (err) {
    message.error('Failed to read directory');
  } finally {
    loading.value = false;
  }
};

const openTargetPath = async (targetPath: string, preview = false) => {
  if (!targetPath) return;

  try {
    if (preview) {
      previewLoading.value = true;
    }
    const res = await axios.get(`/system/file-preview?path=${encodeURIComponent(targetPath)}`);
    const meta = res.data as FilePreviewResponse;
    const targetDir = meta.isDir ? meta.path : meta.parentDir || '/';

    if (currentPath.value !== targetDir || entries.value.length === 0) {
      await fetchEntries(targetDir);
    }

    selectedPath.value = meta.path;
    if (!meta.isDir && preview) {
      previewData.value = meta;
      showPreview.value = true;
      return;
    }

    if (meta.isDir || !preview) {
      showPreview.value = false;
      if (meta.isDir) {
        previewData.value = null;
      }
    }
  } catch (err) {
    if (!currentPath.value) {
      await fetchEntries(homePath.value || '/', true);
    }
  } finally {
    previewLoading.value = false;
  }
};

const openRouteTarget = async () => {
  const targetPath = typeof route.query.path === 'string' && route.query.path.trim() ? route.query.path.trim() : homePath.value || '/';
  await openTargetPath(targetPath, previewRequested.value);
};

const setExplorerTarget = async (path: string, preview = false) => {
  if (!hasRouteSync.value) {
    await openTargetPath(path, preview);
    return;
  }

  const query: Record<string, string> = { path };
  if (preview) {
    query.preview = '1';
  }
  const currentPathQuery = typeof route.query.path === 'string' ? route.query.path : '';
  const currentPreviewQuery = route.query.preview === '1' || route.query.preview === 'true';
  if (route.path === props.routePath && currentPathQuery === path && currentPreviewQuery === preview) {
    await openRouteTarget();
    return;
  }
  await router.replace({ path: props.routePath, query });
};

const navigateToPath = async (path: string) => {
  if (currentPath.value !== path) {
    currentPage.value = 1;
  }
  await setExplorerTarget(path, false);
};

const fetchHome = async () => {
  try {
    const res = await axios.get('/system/home');
    homePath.value = res.data.path;
    if (hasRouteSync.value) {
      if (!route.query.path) {
        await navigateToPath(homePath.value);
      } else {
        await openRouteTarget();
      }
      return;
    }
    await fetchEntries(homePath.value || '/', true);
  } catch (err) {
    console.error('Failed to fetch home path', err);
  }
};

watch(showHidden, () => {
  currentPage.value = 1;
  void fetchEntries(currentPath.value, true);
});

watch(
  () => [route.path, route.query.path, route.query.preview],
  () => {
    if (hasRouteSync.value && route.path === props.routePath) {
      void openRouteTarget();
    }
  },
);

const handlePageChange = (page: number, size: number) => {
  currentPage.value = page;
  pageSize.value = size;
  void fetchEntries(currentPath.value, true);
};

const listColumns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
    sorter: (a: FileEntry, b: FileEntry) => a.name.localeCompare(b.name),
  },
  {
    title: 'Type',
    dataIndex: 'mimeType',
    key: 'mimeType',
    sorter: (a: FileEntry, b: FileEntry) => (a.mimeType || '').localeCompare(b.mimeType || ''),
    filters: [
      { text: 'Directory', value: 'dir' },
      { text: 'Image', value: 'image' },
      { text: 'Application', value: 'application' },
      { text: 'Text', value: 'text' },
    ],
    onFilter: (value: string, record: FileEntry) => {
      if (value === 'dir') return record.isDir;
      if (value === 'image') return record.mimeType?.startsWith('image/');
      if (value === 'application') return record.mimeType?.startsWith('application/');
      if (value === 'text') return record.mimeType?.startsWith('text/');
      return true;
    },
  },
  { title: 'Size', dataIndex: 'size', key: 'size', align: 'right' as const, sorter: (a: FileEntry, b: FileEntry) => (a.size || 0) - (b.size || 0) },
  { title: 'Modified', dataIndex: 'modTime', key: 'modTime', sorter: (a: FileEntry, b: FileEntry) => (a.modTime || '').localeCompare(b.modTime || '') },
  { title: 'Action', key: 'action', width: 220, align: 'right' as const },
];

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
  } catch (err) {}
};

const goUp = () => {
  const parts = currentPath.value.split('/').filter(p => p);
  parts.pop();
  const parent = '/' + parts.join('/');
  void navigateToPath(parent || '/');
};

const runEntryAction = async (entry: FileEntry) => {
  if (props.fileActionOnly && entry.isDir) {
    message.warning('Select a file to continue');
    return;
  }
  if (props.actionType === 'emit') {
    emit('action', entry);
    return;
  }
  try {
    await axios.post('/config/paths', {
      path: entry.path,
      tag: selectedTag.value,
    });
    message.success(`Added ${entry.name} to tracking`);
  } catch (err) {
    message.error('Failed to add path to rules');
  }
};

const pathBreadcrumbs = computed(() => {
  const parts = currentPath.value.split('/').filter(p => p);
  const crumbs = [{ name: 'Root', path: '/' }];
  let current = '';
  parts.forEach(p => {
    current += '/' + p;
    crumbs.push({ name: p, path: current });
  });
  return crumbs;
});

const previewFile = async (path: string) => {
  await setExplorerTarget(path, true);
};

const handleEntryClick = async (entry: FileEntry) => {
  if (entry.isDir && currentPath.value !== entry.path) {
    currentPage.value = 1;
  }
  await setExplorerTarget(entry.path, !entry.isDir);
};

const downloadFile = (path: string) => {
  window.open(`/system/download?path=${encodeURIComponent(path)}`, '_blank');
};

const handleUpload = async (info: any) => {
  const { file } = info;
  const formData = new FormData();
  formData.append('file', file);
  try {
    await axios.post(`/system/upload?path=${encodeURIComponent(currentPath.value)}`, formData);
    message.success(`File ${file.name} uploaded`);
    void fetchEntries(currentPath.value);
  } catch (err) {
    message.error('Upload failed');
  }
};

onMounted(async () => {
  await fetchHome();
  if (props.showTrackingControls) {
    fetchTags();
  }
});
</script>

<template>
  <div class="file-browser-panel">
    <a-alert
      v-if="alertMessage || alertDescription"
      type="info"
      show-icon
      class="file-browser-alert"
      :message="alertMessage"
      :description="alertDescription"
    />

    <div class="file-browser-header">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="crumb in pathBreadcrumbs" :key="crumb.path">
          <a @click.prevent="navigateToPath(crumb.path)" class="file-browser-breadcrumb">{{ crumb.name }}</a>
        </a-breadcrumb-item>
      </a-breadcrumb>

      <div class="file-browser-tools">
        <div v-if="viewMode === 'grid'" class="file-browser-size-control">
          <span>Size:</span>
          <a-slider v-model:value="gridItemSize" :min="60" :max="240" :step="10" size="small" class="file-browser-size-slider" />
        </div>

        <a-radio-group v-model:value="viewMode" size="small">
          <a-radio-button value="list"><UnorderedListOutlined /></a-radio-button>
          <a-radio-button value="grid"><AppstoreOutlined /></a-radio-button>
        </a-radio-group>

        <a-divider v-if="showUpload" type="vertical" />

        <a-upload v-if="showUpload" :customRequest="handleUpload" :showUploadList="false">
          <a-button size="small"><UploadOutlined /> Upload</a-button>
        </a-upload>

        <a-divider type="vertical" />

        <div class="file-browser-hidden-toggle">
          <span>Show Hidden</span>
          <a-switch v-model:checked="showHidden" size="small">
            <template #checkedChildren><EyeOutlined /></template>
            <template #unCheckedChildren><EyeInvisibleOutlined /></template>
          </a-switch>
        </div>

        <template v-if="showTrackingControls">
          <a-divider type="vertical" />
          <span class="file-browser-track-label">Track as:</span>
          <a-select v-model:value="selectedTag" class="file-browser-tag-select">
            <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
          </a-select>
        </template>
      </div>
    </div>

    <div class="file-browser-nav">
      <a-button @click="goUp" :disabled="currentPath === '/'" size="small">
        <template #icon><LeftOutlined /></template>
        Back
      </a-button>
      <a-button @click="navigateToPath(homePath)" size="small">
        <template #icon><HomeOutlined /></template>
        Home
      </a-button>
    </div>

    <div v-if="viewMode === 'list'" class="file-browser-list">
      <a-table
        :loading="loading"
        :dataSource="paginatedEntries"
        :columns="listColumns"
        row-key="path"
        size="small"
        :pagination="false"
        :scroll="{ y: 'calc(100vh - 400px)' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <div class="file-browser-name-cell" @click="handleEntryClick(record)">
              <FolderOutlined v-if="record.isDir" class="file-browser-folder-icon" />
              <div v-else-if="isImage(record)" class="file-browser-list-image">
                <img :src="getImageUrl(record.path)" />
              </div>
              <FileOutlined v-else />
              <span class="file-browser-entry-name" :class="{ 'is-dir': record.isDir }">{{ record.name }}</span>
            </div>
          </template>
          <template v-else-if="column.key === 'mimeType'">
            <span class="file-browser-muted">{{ record.isDir ? 'Directory' : (record.mimeType || 'unknown') }}</span>
          </template>
          <template v-else-if="column.key === 'size'">
            <span class="file-browser-mono">{{ record.isDir ? '-' : formatBytes(record.size) }}</span>
          </template>
          <template v-else-if="column.key === 'modTime'">
            <span class="file-browser-muted">{{ formatTime(record.modTime) }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <div class="file-browser-actions">
              <a-button v-if="!record.isDir" type="link" size="small" @click.stop="previewFile(record.path)">
                <template #icon><EyeOutlined /></template>
              </a-button>
              <a-button v-if="!record.isDir" type="link" size="small" @click.stop="downloadFile(record.path)">
                <template #icon><DownloadOutlined /></template>
              </a-button>
              <a-button type="link" size="small" :disabled="fileActionOnly && record.isDir" @click.stop="runEntryAction(record)">
                <template #icon><PlusOutlined /></template>
                {{ actionLabel }}
              </a-button>
            </div>
          </template>
        </template>
      </a-table>
    </div>

    <div v-else class="file-browser-grid">
      <a-spin :spinning="loading">
        <div class="file-browser-grid-inner">
          <div
            v-for="item in paginatedEntries"
            :key="item.path"
            class="file-browser-grid-item"
            :class="{ 'is-selected': item.path === selectedPath }"
            :style="{ width: `${gridItemSize}px` }"
            @click="handleEntryClick(item)"
          >
            <div class="file-browser-grid-icon">
              <FolderOutlined v-if="item.isDir" :style="{ fontSize: `${Math.floor(gridItemSize * 0.35)}px`, color: '#1890ff' }" />
              <div v-else-if="isImage(item)" :style="{ width: `${Math.floor(gridItemSize * 0.5)}px`, height: `${Math.floor(gridItemSize * 0.5)}px` }" class="file-browser-grid-image">
                <img :src="getImageUrl(item.path)" />
              </div>
              <FileOutlined v-else :style="{ fontSize: `${Math.floor(gridItemSize * 0.35)}px`, color: '#666' }" />
            </div>
            <div class="file-browser-grid-name" :title="item.name" :style="{ fontSize: gridItemSize < 80 ? '10px' : '12px' }">{{ item.name }}</div>
            <div class="file-browser-grid-actions">
              <a-dropdown>
                <a-button type="text" size="small" @click.stop><PlusOutlined /></a-button>
                <template #overlay>
                  <a-menu>
                    <a-menu-item v-if="!item.isDir" @click="previewFile(item.path)">Preview</a-menu-item>
                    <a-menu-item v-if="!item.isDir" @click="downloadFile(item.path)">Download</a-menu-item>
                    <a-menu-item :disabled="fileActionOnly && item.isDir" @click="runEntryAction(item)">{{ actionLabel }}</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </div>
          </div>
        </div>
      </a-spin>
    </div>

    <div class="file-browser-pagination">
      <a-pagination
        v-model:current="currentPage"
        v-model:pageSize="pageSize"
        :total="totalItems"
        show-size-changer
        @change="handlePageChange"
      />
    </div>

    <FilePreviewDrawer
      v-model:open="showPreview"
      :loading="previewLoading"
      :preview="previewData"
      :title="previewTitle"
    />
  </div>
</template>

<style scoped>
.file-browser-panel {
  background: #fff;
}

.file-browser-alert {
  margin-bottom: 16px;
}

.file-browser-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 16px;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}

.file-browser-breadcrumb {
  color: #374151;
  font-weight: 600;
}

.file-browser-tools {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.file-browser-size-control {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 140px;
  margin-right: 8px;
}

.file-browser-size-control span,
.file-browser-hidden-toggle span,
.file-browser-track-label {
  font-size: 12px;
  color: #666;
  white-space: nowrap;
}

.file-browser-size-slider {
  flex: 1;
}

.file-browser-hidden-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f5f5f5;
  padding: 4px 12px;
  border-radius: 4px;
}

.file-browser-tag-select {
  width: 150px;
}

.file-browser-nav {
  margin-bottom: 16px;
  display: flex;
  gap: 8px;
}

.file-browser-grid {
  max-height: calc(100vh - 350px);
  overflow: auto;
}

.file-browser-grid-inner {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 8px;
}

.file-browser-grid-item {
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  border-radius: 4px;
  transition: all 0.2s;
  position: relative;
}

.file-browser-grid-item:hover {
  background: #f0f7ff;
}

.file-browser-grid-item.is-selected {
  background: #e6f4ff;
  border: 1px solid #91caff;
}

.file-browser-grid-icon {
  margin-bottom: 4px;
}

.file-browser-grid-name {
  font-size: 12px;
  text-align: center;
  word-break: break-all;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.2;
  height: 2.4em;
  font-family: monospace;
}

.file-browser-grid-actions {
  position: absolute;
  top: 2px;
  right: 2px;
  opacity: 0;
}

.file-browser-grid-item:hover .file-browser-grid-actions {
  opacity: 1;
}

.file-browser-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.file-browser-folder-icon {
  color: #1890ff;
}

.file-browser-list-image,
.file-browser-grid-image {
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 4px;
  background: #fff;
}

.file-browser-list-image {
  width: 20px;
  height: 20px;
}

.file-browser-grid-image {
  border: 1px solid #f0f0f0;
}

.file-browser-list-image img,
.file-browser-grid-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.file-browser-entry-name {
  font-family: monospace;
  color: #1f2937;
}

.file-browser-entry-name.is-dir {
  font-weight: bold;
}

.file-browser-muted {
  font-size: 12px;
  color: #666;
}

.file-browser-mono {
  font-size: 12px;
  font-family: monospace;
}

.file-browser-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  justify-content: flex-end;
}

.file-browser-pagination {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
</style>
