<script setup lang="ts">
import { ref, onMounted, computed } from 'vue';
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
  fetchEntries(parent);
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

onMounted(() => {
  fetchEntries('/');
  fetchTags();
});
</script>

<template>
  <div style="background: #fff; padding: 24px; min-height: 100%;">
    <div style="display: flex; justify-content: space-between; margin-bottom: 16px; align-items: center; flex-wrap: wrap; gap: 16px;">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="crumb in pathBreadcrumbs" :key="crumb.path">
          <a @click="fetchEntries(crumb.path)">{{ crumb.name }}</a>
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
        <a-list-item :style="{ opacity: item.name.startsWith('.') ? 0.6 : 1 }">
          <div style="display: flex; justify-content: space-between; width: 100%; align-items: center;">
            <div style="display: flex; align-items: center; gap: 8px; cursor: pointer; flex: 1" 
                 @click="item.isDir ? fetchEntries(item.path) : null">
              <FolderOutlined v-if="item.isDir" style="color: #1890ff" />
              <FileOutlined v-else />
              <span :style="{ fontWeight: item.isDir ? 'bold' : 'normal', fontFamily: 'monospace' }">{{ item.name }}</span>
            </div>
            <a-button type="link" size="small" @click="addToRules(item)">
              <template #icon><PlusOutlined /></template>
              Track {{ item.isDir ? 'Dir' : 'File' }}
            </a-button>
          </div>
        </a-list-item>
      </template>
    </a-list>
  </div>
</template>
