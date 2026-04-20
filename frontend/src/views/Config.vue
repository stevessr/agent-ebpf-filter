<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import axios from 'axios';
import { 
  PlusOutlined, 
  TagOutlined, 
  AppstoreOutlined, 
  FolderOutlined, 
  ExportOutlined, 
  ImportOutlined, 
  SafetyCertificateOutlined,
  SwapOutlined,
  StopOutlined,
  AlertOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

interface TrackedItem {
  comm?: string;
  path?: string;
  tag: string;
}

interface WrapperRule {
  comm: string;
  action: string;
  rewritten_cmd: string[];
}

const tags = ref<string[]>([]);
const trackedItems = ref<TrackedItem[]>([]);
const trackedPaths = ref<TrackedItem[]>([]);
const wrapperRules = ref<Record<string, WrapperRule>>({});

const newTagName = ref('');
const newCommName = ref('');
const newCommTag = ref('');
const newPathName = ref('');
const newPathTag = ref('');

// Wrapper rule state
const newRuleComm = ref('');
const newRuleAction = ref('BLOCK');
const newRuleRewritten = ref('');

const fetchTags = async () => {
  try {
    const res = await axios.get('/config/tags');
    tags.value = res.data;
    if (tags.value.length > 0) {
      if (!newCommTag.value) newCommTag.value = tags.value[0];
      if (!newPathTag.value) newPathTag.value = tags.value[0];
    }
  } catch (err) {}
};

const fetchTrackedComms = async () => {
  try {
    const res = await axios.get('/config/comms');
    trackedItems.value = res.data;
  } catch (err) {}
};

const fetchTrackedPaths = async () => {
  try {
    const res = await axios.get('/config/paths');
    trackedPaths.value = res.data;
  } catch (err) {}
};

const fetchRules = async () => {
  try {
    const res = await axios.get('/config/rules');
    wrapperRules.value = res.data;
  } catch (err) {}
};

const addTag = async () => {
  if (!newTagName.value) return;
  try {
    await axios.post('/config/tags', { name: newTagName.value });
    message.success(`Tag "${newTagName.value}" created`);
    newTagName.value = '';
    fetchTags();
  } catch (err) { message.error('Failed to create tag'); }
};

const addComm = async () => {
  if (!newCommName.value || !newCommTag.value) return;
  try {
    await axios.post('/config/comms', { comm: newCommName.value, tag: newCommTag.value });
    message.success(`Added ${newCommName.value}`);
    newCommName.value = '';
    fetchTrackedComms();
  } catch (err) { message.error('Failed to add command'); }
};

const removeComm = async (comm: string) => {
  try {
    await axios.delete(`/config/comms/${comm}`);
    message.success(`Removed ${comm}`);
    fetchTrackedComms();
  } catch (err) {}
};

const addPath = async () => {
  if (!newPathName.value || !newPathTag.value) return;
  try {
    await axios.post('/config/paths', { path: newPathName.value, tag: newPathTag.value });
    message.success(`Added path ${newPathName.value}`);
    newPathName.value = '';
    fetchTrackedPaths();
  } catch (err) {}
};

const removePath = async (path: string) => {
  try {
    await axios.delete(`/config/paths/${path}`);
    message.success(`Removed path ${path}`);
    fetchTrackedPaths();
  } catch (err) {}
};

const saveRule = async () => {
  if (!newRuleComm.value) return;
  try {
    const rule: WrapperRule = {
      comm: newRuleComm.value,
      action: newRuleAction.value,
      rewritten_cmd: newRuleAction.value === 'REWRITE' ? newRuleRewritten.value.split(' ').filter(s => s) : []
    };
    await axios.post('/config/rules', rule);
    message.success('Rule saved');
    newRuleComm.value = '';
    fetchRules();
  } catch (err) {}
};

const deleteRule = async (comm: string) => {
  try {
    await axios.delete(`/config/rules/${comm}`);
    message.success('Rule deleted');
    fetchRules();
  } catch (err) {}
};

const exportConfig = async () => {
  try {
    const res = await axios.get('/config/export');
    const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(res.data, null, 2));
    const link = document.createElement('a');
    link.setAttribute("href", dataStr);
    link.setAttribute("download", "agent-ebpf-config.json");
    link.click();
  } catch (err) {}
};

const importConfig = async (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = async (e) => {
    try {
      const config = JSON.parse(e.target?.result as string);
      await axios.post('/config/import', config);
      message.success('Imported');
      fetchTags(); fetchTrackedComms(); fetchTrackedPaths(); fetchRules();
    } catch (err) {}
  };
  reader.readAsText(file);
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
    'Network Tool': 'purple', 'Security': 'red', 'Wrapper': 'gold'
  };
  return colors[tag] || 'default';
};

onMounted(() => {
  fetchTags(); fetchTrackedComms(); fetchTrackedPaths(); fetchRules();
});
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%;">
    <a-row :gutter="[24, 24]">
      <!-- Tag Management -->
      <a-col :span="24">
        <a-card title="Global Registry" size="small">
          <template #extra>
            <div style="display: flex; gap: 8px; align-items: center;">
              <input type="file" ref="fileInput" @change="importConfig" style="display: none" accept=".json" />
              <a-button size="small" @click="() => ($refs.fileInput as any).click()"><ImportOutlined /> Import</a-button>
              <a-button size="small" @click="exportConfig"><ExportOutlined /> Export</a-button>
              <a-divider type="vertical" />
              <TagOutlined />
            </div>
          </template>
          <div style="display: flex; gap: 16px; align-items: center;">
            <a-input-group compact style="width: 300px;">
              <a-input v-model:value="newTagName" style="width: calc(100% - 40px)" placeholder="New tag" @pressEnter="addTag" />
              <a-button type="primary" @click="addTag"><PlusOutlined /></a-button>
            </a-input-group>
            <div style="display: flex; flex-wrap: wrap; gap: 8px;">
              <a-tag v-for="tag in tags" :key="tag" :color="getCategoryColor(tag)">{{ tag }}</a-tag>
            </div>
          </div>
        </a-card>
      </a-col>

      <!-- Interceptor / Wrapper Rules -->
      <a-col :span="24">
        <a-card title="Wrapper Security Policies" size="small">
          <template #extra><SafetyCertificateOutlined /></template>
          <div style="margin-bottom: 16px; background: #fafafa; padding: 16px; border-radius: 8px;">
            <a-row :gutter="16" align="middle">
              <a-col :span="6">
                <a-input v-model:value="newRuleComm" placeholder="Command to intercept (e.g. rm)" />
              </a-col>
              <a-col :span="4">
                <a-select v-model:value="newRuleAction" style="width: 100%">
                  <a-select-option value="BLOCK">Block Execution</a-select-option>
                  <a-select-option value="REWRITE">Rewrite Command</a-select-option>
                  <a-select-option value="ALERT">Alert Only</a-select-option>
                </a-select>
              </a-col>
              <a-col :span="10">
                <a-input v-if="newRuleAction === 'REWRITE'" v-model:value="newRuleRewritten" placeholder="Rewritten command (e.g. ls -la)" />
                <span v-else style="color: #999; font-size: 12px;">Intercepts and blocks or warns when the command is called via agent-wrapper</span>
              </a-col>
              <a-col :span="4" style="text-align: right;">
                <a-button type="primary" @click="saveRule"><PlusOutlined /> Add Policy</a-button>
              </a-col>
            </a-row>
          </div>

          <a-table :dataSource="Object.values(wrapperRules)" size="small" rowKey="comm" :pagination="false">
            <a-table-column title="Intercepted Command" dataIndex="comm" key="comm">
              <template #default="{ text }"><code>{{ text }}</code></template>
            </a-table-column>
            <a-table-column title="Action" dataIndex="action" key="action">
              <template #default="{ text }">
                <a-tag :color="text === 'BLOCK' ? 'red' : (text === 'REWRITE' ? 'blue' : 'orange')">
                  <component :is="text === 'BLOCK' ? StopOutlined : (text === 'REWRITE' ? SwapOutlined : AlertOutlined)" />
                  {{ text }}
                </a-tag>
              </template>
            </a-table-column>
            <a-table-column title="Rewritten To" dataIndex="rewritten_cmd" key="rewritten_cmd">
              <template #default="{ text }">
                <code v-if="text && text.length">{{ text.join(' ') }}</code>
                <span v-else>-</span>
              </template>
            </a-table-column>
            <a-table-column title="Remove" key="action" width="100px">
              <template #default="{ record }">
                <a-button type="link" danger @click="deleteRule(record.comm)">Delete</a-button>
              </template>
            </a-table-column>
          </a-table>
        </a-card>
      </a-col>

      <!-- Standard Tracking -->
      <a-col :span="12">
        <a-card title="Tracked Executables" size="small">
          <template #extra><AppstoreOutlined /></template>
          <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px;">
            <a-input v-model:value="newCommName" placeholder="Binary name" style="flex: 2" />
            <a-select v-model:value="newCommTag" style="flex: 1" placeholder="Tag">
              <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
            </a-select>
            <a-button type="primary" @click="addComm"><PlusOutlined /></a-button>
          </div>
          <div v-for="(comms, tag) in groupedTrackedItems" :key="tag" style="margin-bottom: 12px;">
            <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
            <div style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tag v-for="comm in comms" :key="comm" closable @close.prevent="removeComm(comm)" :color="getCategoryColor(tag as string)">{{ comm }}</a-tag>
            </div>
          </div>
        </a-card>
      </a-col>

      <a-col :span="12">
        <a-card title="Tracked File Paths" size="small">
          <template #extra><FolderOutlined /></template>
          <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px;">
            <a-input v-model:value="newPathName" placeholder="Absolute path" style="flex: 2" />
            <a-select v-model:value="newPathTag" style="flex: 1" placeholder="Tag">
              <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
            </a-select>
            <a-button type="primary" @click="addPath"><PlusOutlined /></a-button>
          </div>
          <div v-for="(paths, tag) in groupedTrackedPaths" :key="tag" style="margin-bottom: 12px;">
            <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
            <div style="display: flex; flex-wrap: wrap; gap: 6px;">
              <a-tag v-for="p in paths" :key="p" closable @close.prevent="removePath(p)" :color="getCategoryColor(tag as string)">{{ p }}</a-tag>
            </div>
          </div>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
:deep(.ant-card) { border-radius: 8px; }
code { font-family: monospace; background: #eee; padding: 2px 4px; border-radius: 4px; }
</style>
