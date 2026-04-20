<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import axios from 'axios';
import { PlusOutlined, TagOutlined, AppstoreOutlined, FolderOutlined, ExportOutlined, ImportOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

interface TrackedItem {
  comm?: string;
  path?: string;
  tag: string;
}

const tags = ref<string[]>([]);
const trackedItems = ref<TrackedItem[]>([]);
const trackedPaths = ref<TrackedItem[]>([]);

const newTagName = ref('');
const newCommName = ref('');
const newCommTag = ref('');
const newPathName = ref('');
const newPathTag = ref('');

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
    if (tags.value.length > 0) {
      if (!newCommTag.value) newCommTag.value = tags.value[0];
      if (!newPathTag.value) newPathTag.value = tags.value[0];
    }
  } catch (err) {
    message.error('Failed to fetch tags');
  }
};

const fetchTrackedComms = async () => {
  try {
    const res = await axios.get('/config/comms');
    trackedItems.value = res.data;
  } catch (err) {
    message.error('Failed to fetch tracked commands');
  }
};

const fetchTrackedPaths = async () => {
  try {
    const res = await axios.get('/config/paths');
    trackedPaths.value = res.data;
  } catch (err) {
    message.error('Failed to fetch tracked paths');
  }
};

const exportConfig = async () => {
  try {
    const res = await axios.get('/config/export');
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(res.data, null, 2));
    const downloadAnchorNode = document.createElement('a');
    downloadAnchorNode.setAttribute("href", dataStr);
    downloadAnchorNode.setAttribute("download", "agent-ebpf-config.json");
    document.body.appendChild(downloadAnchorNode);
    downloadAnchorNode.click();
    downloadAnchorNode.remove();
    message.success('Configuration exported');
  } catch (err) {
    message.error('Failed to export configuration');
  }
};

const importConfig = async (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  
  const reader = new FileReader();
  reader.onload = async (e) => {
    try {
      const config = JSON.parse(e.target?.result as string);
      await axios.post('/config/import', config);
      message.success('Configuration imported successfully');
      fetchTags();
      fetchTrackedComms();
      fetchTrackedPaths();
    } catch (err) {
      message.error('Failed to import configuration: invalid JSON');
    }
  };
  reader.readAsText(file);
};

const addTag = async () => {
  if (!newTagName.value) return;
  try {
    await axios.post('/config/tags', { name: newTagName.value });
    message.success(`Tag "${newTagName.value}" created`);
    newTagName.value = '';
    fetchTags();
  } catch (err) {
    message.error('Failed to create tag');
  }
};

const addComm = async () => {
  if (!newCommName.value || !newCommTag.value) return;
  try {
    await axios.post('/config/comms', { 
      comm: newCommName.value,
      tag: newCommTag.value
    });
    message.success(`Added ${newCommName.value} to ${newCommTag.value}`);
    newCommName.value = '';
    fetchTrackedComms();
  } catch (err) {
    message.error('Failed to add tracked command');
  }
};

const removeComm = async (comm: string) => {
  try {
    await axios.delete(`/config/comms/${comm}`);
    message.success(`Removed ${comm}`);
    fetchTrackedComms();
  } catch (err) {
    message.error('Failed to remove tracked command');
  }
};

const addPath = async () => {
  if (!newPathName.value || !newPathTag.value) return;
  try {
    await axios.post('/config/paths', { 
      path: newPathName.value,
      tag: newPathTag.value
    });
    message.success(`Added path ${newPathName.value}`);
    newPathName.value = '';
    fetchTrackedPaths();
  } catch (err) {
    message.error('Failed to add tracked path');
  }
};

const removePath = async (path: string) => {
  try {
    // encodeURIComponent is important for paths
    await axios.delete(`/config/paths/${path}`);
    message.success(`Removed path ${path}`);
    fetchTrackedPaths();
  } catch (err) {
    message.error('Failed to remove tracked path');
  }
};

const groupedTrackedItems = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedItems.value.forEach(item => {
    if (!groups[item.tag]) groups[item.tag] = [];
    if (item.comm) groups[item.tag].push(item.comm);
  });
  return groups;
});

const groupedTrackedPaths = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedPaths.value.forEach(item => {
    if (!groups[item.tag]) groups[item.tag] = [];
    if (item.path) groups[item.tag].push(item.path);
  });
  return groups;
});

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta', 'Git': 'orange', 'Build Tool': 'cyan',
    'Package Manager': 'green', 'Runtime': 'blue', 'System Tool': 'geekblue', 
    'Network Tool': 'purple', 'Security': 'red'
  };
  return colors[tag] || 'default';
};

onMounted(() => {
  fetchTags();
  fetchTrackedComms();
  fetchTrackedPaths();
});
</script>

<template>
  <div style="padding: 24px; background: #fff; min-height: 100%;">
    <a-row :gutter="[24, 24]">
      <!-- Tag Management -->
      <a-col :span="24">
        <a-card title="Tag Management" size="small">
          <template #extra>
            <div style="display: flex; gap: 8px; align-items: center;">
              <input type="file" ref="fileInput" @change="importConfig" style="display: none" accept=".json" />
              <a-button size="small" @click="() => ($refs.fileInput as any).click()">
                <template #icon><ImportOutlined /></template>
                Import
              </a-button>
              <a-button size="small" @click="exportConfig">
                <template #icon><ExportOutlined /></template>
                Export
              </a-button>
              <a-divider type="vertical" />
              <TagOutlined />
            </div>
          </template>
          <div style="display: flex; gap: 16px; align-items: flex-start;">
            <div style="width: 300px;">
              <a-input-group compact>
                <a-input v-model:value="newTagName" style="width: calc(100% - 40px)" placeholder="New tag name" @pressEnter="addTag" />
                <a-button type="primary" @click="addTag"><template #icon><PlusOutlined /></template></a-button>
              </a-input-group>
            </div>
            <div style="display: flex; flex-wrap: wrap; gap: 8px;">
              <a-tag v-for="tag in tags" :key="tag" :color="getCategoryColor(tag)">{{ tag }}</a-tag>
            </div>
          </div>
        </a-card>
      </a-col>

      <!-- Command Management -->
      <a-col :span="12">
        <a-card title="Tracked Executables">
          <template #extra><AppstoreOutlined /></template>
          <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px;">
            <div style="display: flex; gap: 8px;">
              <a-input v-model:value="newCommName" placeholder="Binary name (e.g. gcc)" style="flex: 2" />
              <a-select v-model:value="newCommTag" style="flex: 1" placeholder="Tag">
                <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
              </a-select>
              <a-button type="primary" @click="addComm"><PlusOutlined /></a-button>
            </div>
          </div>

          <div v-for="(comms, tag) in groupedTrackedItems" :key="tag" style="margin-bottom: 12px;">
            <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
            <div style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tag v-for="comm in comms" :key="comm" closable @close.prevent="removeComm(comm)" :color="getCategoryColor(tag as string)">
                {{ comm }}
              </a-tag>
            </div>
          </div>
        </a-card>
      </a-col>

      <!-- Path Management -->
      <a-col :span="12">
        <a-card title="Tracked File Paths">
          <template #extra><FolderOutlined /></template>
          <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px;">
            <div style="display: flex; gap: 8px;">
              <a-input v-model:value="newPathName" placeholder="Absolute path (e.g. /etc/shadow)" style="flex: 2" />
              <a-select v-model:value="newPathTag" style="flex: 1" placeholder="Tag">
                <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
              </a-select>
              <a-button type="primary" @click="addPath"><PlusOutlined /></a-button>
            </div>
          </div>

          <div v-for="(paths, tag) in groupedTrackedPaths" :key="tag" style="margin-bottom: 12px;">
            <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
            <div style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tag v-for="p in paths" :key="p" closable @close.prevent="removePath(p)" :color="getCategoryColor(tag as string)">
                {{ p }}
              </a-tag>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>
