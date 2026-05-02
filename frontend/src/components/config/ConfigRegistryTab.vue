<script setup lang="ts">
import { ref } from 'vue';
import {
  TagOutlined, AppstoreOutlined, FolderOutlined,
  ExportOutlined, ImportOutlined, PlusOutlined,
} from '@ant-design/icons-vue';
import PathNavigatorDrawer from '../PathNavigatorDrawer.vue';
import { getCategoryColor, type useConfigRegistry } from '../../composables/useConfigRegistry';

const props = defineProps<{
  registry: ReturnType<typeof useConfigRegistry>;
}>();

const {
  tags,
  newTagName, newCommName, newCommTag, newPathName, newPathTag, newPrefixValue, newPrefixTag,
  pathPickerOpen, pathPickerTarget,
  addTag, addComm, removeComm, toggleCommDisabled,
  addPath, removePath, addPrefix, removePrefix,
  exportConfig, importConfig, clearAllConfig,
  openPathPicker, handlePathPicked, openImportPicker,
  groupedTrackedItems, groupedTrackedPaths, groupedTrackedPrefixes,
  importFileInput,
} = props.registry;

void importFileInput;

const registryTabKey = ref('tags');
</script>

<template>
  <a-tabs v-model:activeKey="registryTabKey" size="small">
    <a-tab-pane key="tags" tab="Tags & Global">
      <a-row :gutter="[24, 24]">
        <a-col :span="24">
          <a-card title="Global Registry & Actions" size="small">
            <template #extra>
              <div style="display: flex; gap: 8px; align-items: center">
                <input type="file" ref="importFileInput" @change="importConfig" style="display: none" accept=".json" />
                <a-button size="small" @click="openImportPicker"><ImportOutlined /> Import</a-button>
                <a-button size="small" @click="exportConfig"><ExportOutlined /> Export</a-button>
                <a-popconfirm title="Are you sure you want to clear all configurations?" @confirm="clearAllConfig">
                  <a-button size="small" danger>Clear All</a-button>
                </a-popconfirm>
                <a-divider type="vertical" />
                <TagOutlined />
              </div>
            </template>
            <div style="display: flex; flex-direction: column; gap: 16px">
              <div style="display: flex; gap: 8px; align-items: center">
                <span style="color: #888; font-size: 13px; width: 80px">Add Tag:</span>
                <div style="display: flex; width: 320px">
                  <a-input v-model:value="newTagName" placeholder="New tag name..." @pressEnter="addTag"
                    style="border-top-right-radius: 0; border-bottom-right-radius: 0" />
                  <a-button type="primary" @click="addTag"
                    style="border-top-left-radius: 0; border-bottom-left-radius: 0">
                    <PlusOutlined />
                  </a-button>
                </div>
              </div>
              <div style="display: flex; gap: 8px; align-items: flex-start">
                <span style="color: #888; font-size: 13px; width: 80px; margin-top: 4px">Registered:</span>
                <div style="display: flex; flex-wrap: wrap; gap: 8px; flex: 1">
                  <a-tag v-for="tag in tags" :key="tag" :color="getCategoryColor(tag)">{{ tag }}</a-tag>
                </div>
              </div>
            </div>
          </a-card>
        </a-col>
        <a-col :span="24">
          <a-alert type="info" show-icon
            message="Tags are used to categorize tracked processes, paths, and prefixes. They provide semantic context in the Monitor and Network views." />
        </a-col>
      </a-row>
    </a-tab-pane>

    <a-tab-pane key="binaries" tab="Tracked Binaries">
      <a-row :gutter="[24, 24]">
        <a-col :span="24">
          <a-card title="Tracked Executables" size="small">
            <template #extra><AppstoreOutlined /></template>
            <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px">
              <a-input v-model:value="newCommName" placeholder="Binary name (e.g. curl, git, python)" style="flex: 2" />
              <a-select v-model:value="newCommTag" style="flex: 1" placeholder="Assign Tag">
                <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
              </a-select>
              <a-button type="primary" @click="addComm"><PlusOutlined /> Add</a-button>
            </div>
            <a-row :gutter="[16, 16]">
              <a-col v-for="(comms, tag) in groupedTrackedItems" :key="tag" :xs="24" :md="12" :xl="8">
                <div style="padding: 12px; border: 1px solid #f0f0f0; border-radius: 8px; height: 100%">
                  <div style="margin-bottom: 8px; border-bottom: 1px solid #f5f5f5; padding-bottom: 4px">
                    <a-typography-text strong>{{ tag }}</a-typography-text>
                  </div>
                  <div style="display: flex; flex-wrap: wrap; gap: 6px">
                    <a-tag v-for="entry in comms" :key="entry.comm" closable @close.prevent="removeComm(entry.comm!)"
                      :color="entry.disabled ? 'default' : getCategoryColor(tag)">
                      <span
                        :style="entry.disabled ? 'text-decoration: line-through; opacity: 0.55; cursor: pointer;' : 'cursor: pointer;'"
                        @click.stop="toggleCommDisabled(entry.comm!, entry.disabled || false)">{{ entry.comm }}</span>
                      <span v-if="entry.disabled" style="margin-left: 4px; font-size: 10px; opacity: 0.7;">off</span>
                    </a-tag>
                  </div>
                </div>
              </a-col>
            </a-row>
          </a-card>
        </a-col>
      </a-row>
    </a-tab-pane>

    <a-tab-pane key="paths" tab="Paths & Prefixes">
      <a-row :gutter="[24, 24]">
        <a-col :xs="24" :lg="12">
          <a-card title="Exact File Paths" size="small">
            <template #extra>
              <a-space>
                <a-tooltip title="Browse files">
                  <FolderOutlined style="cursor: pointer; color: #1890ff;" @click="openPathPicker('exact')" />
                </a-tooltip>
                <FolderOutlined />
              </a-space>
            </template>
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
                <a-tag v-for="p in paths" :key="p.path" closable @close.prevent="removePath(p.path!)" :color="getCategoryColor(tag)">{{ p.path }}</a-tag>
              </div>
            </div>
          </a-card>
        </a-col>
        <a-col :xs="24" :lg="12">
          <a-card title="Path Prefixes (LPM)" size="small">
            <template #extra>
              <a-space>
                <a-tooltip title="Browse directories">
                  <FolderOutlined style="cursor: pointer; color: #1890ff;" @click="openPathPicker('prefix')" />
                </a-tooltip>
                <FolderOutlined />
              </a-space>
            </template>
            <div style="margin-bottom: 16px; background: #fafafa; padding: 12px; border-radius: 8px; display: flex; gap: 8px;">
              <a-input v-model:value="newPrefixValue" placeholder="Path prefix (e.g. /etc)" style="flex: 2" />
              <a-select v-model:value="newPrefixTag" style="flex: 1" placeholder="Tag">
                <a-select-option v-for="tag in tags" :key="tag" :value="tag">{{ tag }}</a-select-option>
              </a-select>
              <a-button type="primary" @click="addPrefix"><PlusOutlined /></a-button>
            </div>
            <a-alert type="info" show-icon style="margin-bottom: 12px;"
              message="Prefix matching applies to descendant paths." />
            <div v-for="(prefixes, tag) in groupedTrackedPrefixes" :key="tag" style="margin-bottom: 12px;">
              <div style="margin-bottom: 4px;"><a-typography-text strong>{{ tag }}</a-typography-text></div>
              <div style="display: flex; flex-wrap: wrap; gap: 6px;">
                <a-tag v-for="prefix in prefixes" :key="prefix.prefix" closable @close.prevent="removePrefix(prefix.prefix!)" :color="getCategoryColor(tag)">{{ prefix.prefix }}</a-tag>
              </div>
            </div>
          </a-card>
        </a-col>
      </a-row>
    </a-tab-pane>
  </a-tabs>

  <PathNavigatorDrawer v-model:open="pathPickerOpen"
    :title="pathPickerTarget === 'exact' ? 'Pick File' : 'Pick Directory'"
    :pick-mode="pathPickerTarget === 'exact' ? 'file' : 'directory'"
    @confirm="handlePathPicked" />
</template>
