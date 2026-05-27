<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue';
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
  HomeOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import FilePreviewDrawer from '../components/FilePreviewDrawer.vue';
import type { FilePreviewResponse } from '../types/filePreview';

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
  mimeType?: string;
  size?: number;
  modTime?: string;
}

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

const isImage = (entry: FileEntry) => {
  return entry.mimeType?.startsWith('image/');
};

const getImageUrl = (path: string) => {
  return `/system/download?path=${encodeURIComponent(path)}`;
};

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

const fetchHome = async () => {
  try {
    const res = await axios.get('/system/home');
    homePath.value = res.data.path;
    if (!route.query.path) {
      void navigateToPath(homePath.value);
    }
  } catch (err) {
    console.error('Failed to fetch home path', err);
  }
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
        path: path,
        offset: offset,
        limit: pageSize.value,
        showHidden: showHidden.value
      }
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

watch(showHidden, () => {
  currentPage.value = 1;
  void fetchEntries(currentPath.value, true);
});

const handlePageChange = (page: number, size: number) => {
  currentPage.value = page;
  pageSize.value = size;
  void fetchEntries(currentPath.value, true);
};

const paginatedEntries = computed(() => entries.value);

const listColumns = [
  { 
    title: 'Name', 
    dataIndex: 'name', 
    key: 'name', 
    sorter: (a: FileEntry, b: FileEntry) => a.name.localeCompare(b.name) 
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
    }
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

const addToRules = async (entry: FileEntry) => {
  try {
    await axios.post('/config/paths', { 
      path: entry.path,
      tag: selectedTag.value
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

const setExplorerTarget = async (path: string, preview = false) => {
  const query: Record<string, string> = { path };
  if (preview) {
    query.preview = '1';
  }
  const currentPathQuery = typeof route.query.path === 'string' ? route.query.path : '';
  const currentPreviewQuery = route.query.preview === '1' || route.query.preview === 'true';
  if (currentPathQuery === path && currentPreviewQuery === preview) {
    await openRouteTarget();
    return;
  }
  await router.replace({ path: '/explorer', query });
};

const navigateToPath = async (path: string) => {
  if (currentPath.value !== path) {
    currentPage.value = 1;
  }
  await setExplorerTarget(path, false);
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

const openRouteTarget = async () => {
  const targetPath = typeof route.query.path === 'string' && route.query.path.trim() ? route.query.path.trim() : homePath.value || '/';
  if (!targetPath) return;

  try {
    const isPreview = previewRequested.value;
    if (isPreview) {
      previewLoading.value = true;
    }
    const res = await axios.get(`/system/file-preview?path=${encodeURIComponent(targetPath)}`);
    const meta = res.data as FilePreviewResponse;
    const targetDir = meta.isDir ? meta.path : meta.parentDir || '/';

    // Only fetch entries if the directory has changed OR if list is empty
    if (currentPath.value !== targetDir || entries.value.length === 0) {
       await fetchEntries(targetDir);
    }

    selectedPath.value = meta.path;
    if (!meta.isDir && isPreview) {
      previewData.value = meta;
      showPreview.value = true;
      return;
    }

    if (meta.isDir || !isPreview) {
      showPreview.value = false;
      if (meta.isDir) {
        previewData.value = null;
      }
    }
  } catch (err: any) {
    if (!currentPath.value) {
      await fetchEntries(homePath.value || '/', true);
    }
  } finally {
    previewLoading.value = false;
  }
};

const previewRequested = computed(() => route.query.preview === '1' || route.query.preview === 'true');

watch(
  () => [route.query.path, route.query.preview],
  () => {
    void openRouteTarget();
  },
  { immediate: true },
);

onMounted(async () => {
  await fetchHome();
  fetchTags();
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 100%;">
    <a-alert
      type="info"
      show-icon
      style="margin-bottom: 16px;"
      message="Path tracking is exact-match"
      description="Adding a file or directory here tracks that exact path string only. Directory entries are not tracked recursively."
    />

    <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center; flex-wrap: wrap; gap: 16px;">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="crumb in pathBreadcrumbs" :key="crumb.path">
          <a @click.prevent="navigateToPath(crumb.path)" style="color: #374151; font-weight: 600;">{{ crumb.name }}</a>
        </a-breadcrumb-item>
      </a-breadcrumb>
      
      <div style="display: flex; align-items: center; gap: 12px;">
        <div v-if="viewMode === 'grid'" style="display: flex; align-items: center; gap: 8px; width: 140px; margin-right: 8px;">
          <span style="font-size: 12px; color: #666; white-space: nowrap;">Size:</span>
          <a-slider v-model:value="gridItemSize" :min="60" :max="240" :step="10" size="small" style="flex: 1;" />
        </div>

        <a-radio-group v-model:value="viewMode" size="small">
          <a-radio-button value="list"><UnorderedListOutlined /></a-radio-button>
          <a-radio-button value="grid"><AppstoreOutlined /></a-radio-button>
        </a-radio-group>

        <a-divider type="vertical" />

        <a-upload :customRequest="handleUpload" :showUploadList="false">
          <a-button size="small"><UploadOutlined /> Upload</a-button>
        </a-upload>

        <a-divider type="vertical" />

        <div style="display: flex; align-items: center; gap: 8px; background: #f5f5f5; padding: 4px 12px; border-radius: 4px;">
          <span style="font-size: 12px; color: #666;">Show Hidden</span>
          <a-switch v-model:checked="showHidden" size="small">
            <template #checkedChildren><EyeOutlined /></template>
            <template #unCheckedChildren><EyeInvisibleOutlined /></template>
          </a-switch>
        </div>

        <a-divider type="vertical" />
        
        <span style="font-size: 13px; color: #666;">Track as:</span>
        <a-select v-model:value="selectedTag" style="width: 150px">
          <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
        </a-select>
      </div>
    </div>

    <div style="margin-bottom: 16px; display: flex; gap: 8px;">
      <a-button @click="goUp" :disabled="currentPath === '/'" size="small">
        <template #icon><LeftOutlined /></template>
        Back
      </a-button>
      <a-button @click="navigateToPath(homePath)" size="small">
        <template #icon><HomeOutlined /></template>
        Home
      </a-button>
    </div>

    <div v-if="viewMode === 'list'" class="explorer-list">
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
            <div style="display: flex; align-items: center; gap: 8px; cursor: pointer;" 
                 @click="handleEntryClick(record)">
              <FolderOutlined v-if="record.isDir" style="color: #1890ff" />
              <div v-else-if="isImage(record)" style="width: 20px; height: 20px; display: flex; align-items: center; justify-content: center; overflow: hidden;">
                 <img :src="getImageUrl(record.path)" style="width: 100%; height: 100%; object-fit: cover; border-radius: 2px;" />
              </div>
              <FileOutlined v-else />
              <span :style="{ fontWeight: record.isDir ? 'bold' : 'normal', fontFamily: 'monospace', color: '#1f2937' }">{{ record.name }}</span>
            </div>
          </template>
          <template v-else-if="column.key === 'mimeType'">
            <span style="font-size: 12px; color: #666;">{{ record.isDir ? 'Directory' : (record.mimeType || 'unknown') }}</span>
          </template>
          <template v-else-if="column.key === 'size'">
            <span style="font-size: 12px; font-family: monospace;">{{ record.isDir ? '-' : formatBytes(record.size) }}</span>
          </template>
          <template v-else-if="column.key === 'modTime'">
            <span style="font-size: 12px;">{{ formatTime(record.modTime) }}</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <div style="display: flex; align-items: center; gap: 4px; justify-content: flex-end;">
              <a-button v-if="!record.isDir" type="link" size="small" @click.stop="previewFile(record.path)">
                <template #icon><EyeOutlined /></template>
              </a-button>
              <a-button v-if="!record.isDir" type="link" size="small" @click.stop="downloadFile(record.path)">
                <template #icon><DownloadOutlined /></template>
              </a-button>
              <a-button type="link" size="small" @click.stop="addToRules(record)">
                <template #icon><PlusOutlined /></template>
              </a-button>
            </div>
          </template>
        </template>
      </a-table>
    </div>

    <div v-else class="explorer-grid" :style="{ maxHeight: 'calc(100vh - 350px)', overflow: 'auto' }">
      <a-spin :spinning="loading">
        <div style="display: flex; flex-wrap: wrap; gap: 12px; padding: 8px;">
          <div v-for="item in paginatedEntries" :key="item.path" 
               class="explorer-grid-item"
               :class="{ 'is-selected': item.path === selectedPath }"
               :style="{ width: `${gridItemSize}px` }"
               @click="handleEntryClick(item)">
            <div class="explorer-grid-icon">
              <FolderOutlined v-if="item.isDir" :style="{ fontSize: `${Math.floor(gridItemSize * 0.35)}px`, color: '#1890ff' }" />
              <div v-else-if="isImage(item)" :style="{ width: `${Math.floor(gridItemSize * 0.5)}px`, height: `${Math.floor(gridItemSize * 0.5)}px` }" style="display: flex; align-items: center; justify-content: center; overflow: hidden; border: 1px solid #f0f0f0; border-radius: 4px; background: #fff;">
                 <img :src="getImageUrl(item.path)" style="width: 100%; height: 100%; object-fit: cover;" />
              </div>
              <FileOutlined v-else :style="{ fontSize: `${Math.floor(gridItemSize * 0.35)}px`, color: '#666' }" />
            </div>
            <div class="explorer-grid-name" :title="item.name" :style="{ fontSize: gridItemSize < 80 ? '10px' : '12px' }">{{ item.name }}</div>
            <div class="explorer-grid-actions">
               <a-dropdown>
                  <a-button type="text" size="small" @click.stop><PlusOutlined /></a-button>
                  <template #overlay>
                    <a-menu>
                      <a-menu-item v-if="!item.isDir" @click="previewFile(item.path)">Preview</a-menu-item>
                      <a-menu-item v-if="!item.isDir" @click="downloadFile(item.path)">Download</a-menu-item>
                      <a-menu-item @click="addToRules(item)">Track path</a-menu-item>
                    </a-menu>
                  </template>
               </a-dropdown>
            </div>
          </div>
        </div>
      </a-spin>
    </div>

    <div style="margin-top: 16px; display: flex; justify-content: flex-end;">
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
      title="Explorer File Preview"
    />
  </div>
</template>

<style scoped>
.explorer-grid-item {
  padding: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  border-radius: 4px;
  transition: all 0.2s;
  position: relative;
}
.explorer-grid-item:hover {
  background: #f0f7ff;
}
.explorer-grid-item.is-selected {
  background: #e6f4ff;
  border: 1px solid #91caff;
}
.explorer-grid-icon {
  margin-bottom: 4px;
}
.explorer-grid-name {
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
.explorer-grid-actions {
  position: absolute;
  top: 2px;
  right: 2px;
  opacity: 0;
}
.explorer-grid-item:hover .explorer-grid-actions {
  opacity: 1;
}
</style>
