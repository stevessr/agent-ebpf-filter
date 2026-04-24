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
  EyeInvisibleOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import FilePreviewDrawer from '../components/FilePreviewDrawer.vue';
import type { FilePreviewResponse } from '../types/filePreview';

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
}

const currentPath = ref('/');
const entries = ref<FileEntry[]>([]);
const loading = ref(false);
const tags = ref<string[]>([]);
const selectedTag = ref('Security');
const showHidden = ref(false);
const selectedPath = ref('');
const previewLoading = ref(false);
const showPreview = ref(false);
const previewData = ref<FilePreviewResponse | null>(null);
const route = useRoute();
const router = useRouter();

const fetchEntries = async (path: string) => {
  loading.value = true;
  try {
    const res = await axios.get(`/system/ls?path=${encodeURIComponent(path)}`);
    entries.value = res.data.sort((a: FileEntry, b: FileEntry) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    currentPath.value = path;
  } catch (err) {
    message.error('Failed to read directory');
  } finally {
    loading.value = false;
  }
};

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

const filteredEntries = computed(() => {
  if (showHidden.value) return entries.value;
  return entries.value.filter(e => !e.name.startsWith('.'));
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
  await setExplorerTarget(path, false);
};

const handleEntryClick = async (entry: FileEntry) => {
  await setExplorerTarget(entry.path, !entry.isDir);
};

const openRouteTarget = async () => {
  const targetPath = typeof route.query.path === 'string' && route.query.path.trim() ? route.query.path.trim() : '/';
  const previewRequested = route.query.preview === '1' || route.query.preview === 'true';

  try {
    previewLoading.value = previewRequested;
    const res = await axios.get(`/system/file-preview?path=${encodeURIComponent(targetPath)}`);
    const meta = res.data as FilePreviewResponse;
    const targetDir = meta.isDir ? meta.path : meta.parentDir || '/';

    await fetchEntries(targetDir);

    selectedPath.value = meta.path;
    if (!meta.isDir && previewRequested) {
      previewData.value = meta;
      showPreview.value = true;
      return;
    }

    if (meta.isDir || !previewRequested) {
      showPreview.value = false;
      if (meta.isDir) {
        previewData.value = null;
      }
    }
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to open target path');
    if (!currentPath.value) {
      await fetchEntries('/');
    }
  } finally {
    previewLoading.value = false;
  }
};

watch(
  () => [route.query.path, route.query.preview],
  () => {
    void openRouteTarget();
  },
  { immediate: true },
);

onMounted(() => {
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
          <a @click="navigateToPath(crumb.path)">{{ crumb.name }}</a>
        </a-breadcrumb-item>
      </a-breadcrumb>
      
      <div style="display: flex; align-items: center; gap: 12px;">
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

    <div style="margin-bottom: 16px;">
      <a-button @click="goUp" :disabled="currentPath === '/'" size="small">
        <template #icon><LeftOutlined /></template>
        Back
      </a-button>
    </div>

    <a-list :loading="loading" bordered :dataSource="filteredEntries" size="small" :style="{ maxHeight: 'calc(100vh - 300px)', overflow: 'auto' }">
      <template #renderItem="{ item }">
        <a-list-item :style="{ opacity: item.name.startsWith('.') ? 0.6 : 1, background: item.path === selectedPath ? '#f0f7ff' : 'transparent' }">
          <div style="display: flex; justify-content: space-between; width: 100%; align-items: center;">
            <div style="display: flex; align-items: center; gap: 8px; cursor: pointer; flex: 1" 
                 @click="handleEntryClick(item)">
              <FolderOutlined v-if="item.isDir" style="color: #1890ff" />
              <FileOutlined v-else />
              <span :style="{ fontWeight: item.isDir ? 'bold' : 'normal', fontFamily: 'monospace' }">{{ item.name }}</span>
            </div>
            <div style="display: flex; align-items: center; gap: 4px;">
              <a-button v-if="!item.isDir" type="link" size="small" @click.stop="previewFile(item.path)">
                <template #icon><EyeOutlined /></template>
                Preview
              </a-button>
              <a-button type="link" size="small" @click.stop="addToRules(item)">
                <template #icon><PlusOutlined /></template>
                Track exact path
              </a-button>
            </div>
          </div>
        </a-list-item>
      </template>
    </a-list>

    <FilePreviewDrawer
      v-model:open="showPreview"
      :loading="previewLoading"
      :preview="previewData"
      title="Explorer File Preview"
    />
  </div>
</template>
