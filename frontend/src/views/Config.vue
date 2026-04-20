<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import axios from 'axios';
import { PlusOutlined, DeleteOutlined, TagOutlined, AppstoreOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

interface TrackedItem {
  comm: string;
  tag: string;
}

const tags = ref<string[]>([]);
const trackedItems = ref<TrackedItem[]>([]);
const newTagName = ref('');
const newCommName = ref('');
const newCommTag = ref('');

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
    if (tags.value.length > 0 && !newCommTag.value) {
      newCommTag.value = tags.value[0];
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

const groupedTrackedItems = computed(() => {
  const groups: Record<string, string[]> = {};
  trackedItems.value.forEach(item => {
    if (!groups[item.tag]) groups[item.tag] = [];
    groups[item.tag].push(item.comm);
  });
  return groups;
});

const getCategoryColor = (tag: string) => {
  const colors: Record<string, string> = {
    'AI Agent': 'magenta', 'Git': 'orange', 'Build Tool': 'cyan',
    'Package Manager': 'green', 'Runtime': 'blue', 'System Tool': 'geekblue', 'Network Tool': 'purple',
  };
  return colors[tag] || 'default';
};

onMounted(() => {
  fetchTags();
  fetchTrackedComms();
});
</script>

<template>
  <div style="padding: 24px; background: #fff; min-height: 100%;">
    <a-row :gutter="24">
      <!-- Tag Management -->
      <a-col :span="8">
        <a-card title="Tag Management">
          <template #extra><TagOutlined /></template>
          <div style="margin-bottom: 16px;">
            <a-input-group compact>
              <a-input v-model:value="newTagName" style="width: calc(100% - 40px)" placeholder="New tag name" @pressEnter="addTag" />
              <a-button type="primary" @click="addTag"><template #icon><PlusOutlined /></template></a-button>
            </a-input-group>
          </div>
          <a-list :dataSource="tags" size="small" bordered>
            <template #renderItem="{ item }">
              <a-list-item>
                <a-tag :color="getCategoryColor(item)">{{ item }}</a-tag>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-col>

      <!-- Command Management -->
      <a-col :span="16">
        <a-card title="Tracked Executables">
          <template #extra><AppstoreOutlined /></template>
          <div style="margin-bottom: 24px; background: #fafafa; padding: 16px; border-radius: 8px;">
            <div style="display: flex; gap: 8px;">
              <a-input v-model:value="newCommName" placeholder="Executable (e.g. gcc, custom-binary)" style="flex: 2" />
              <a-select v-model:value="newCommTag" style="flex: 1" placeholder="Select Tag">
                <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
              </a-select>
              <a-button type="primary" @click="addComm">
                <template #icon><PlusOutlined /></template>
                Add Filter
              </a-button>
            </div>
          </div>

          <div v-for="(comms, tag) in groupedTrackedItems" :key="tag" style="margin-bottom: 16px;">
            <a-divider orientation="left" style="margin: 8px 0">
              <a-tag :color="getCategoryColor(tag as string)">{{ tag }}</a-tag>
            </a-divider>
            <div style="display: flex; flex-wrap: wrap; gap: 8px; padding-left: 8px;">
              <a-tag v-for="comm in comms" :key="comm" closable @close.prevent="removeComm(comm)">
                {{ comm }}
              </a-tag>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>
