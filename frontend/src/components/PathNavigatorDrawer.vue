<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  CheckOutlined,
  FileOutlined,
  FolderOutlined,
  LeftOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue';

interface PathEntry {
  name: string;
  isDir: boolean;
  path: string;
}

type PickMode = 'directory' | 'file' | 'any';

const props = withDefaults(defineProps<{
  open: boolean;
  title: string;
  initialPath?: string;
  pickMode?: PickMode;
  confirmLabel?: string;
}>(), {
  initialPath: '/',
  pickMode: 'directory',
  confirmLabel: '',
});

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void;
  (event: 'confirm', path: string): void;
}>();

const currentPath = ref('/');
const entries = ref<PathEntry[]>([]);
const loading = ref(false);
const selectedPath = ref('');
const jumpPath = ref('/');

const normalizedPath = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed) return '/';
  if (trimmed === '/') return '/';
  return trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
};

const parentPath = (value: string) => {
  const normalized = normalizedPath(value);
  if (normalized === '/') return '/';
  const parts = normalized.split('/').filter(Boolean);
  parts.pop();
  return parts.length ? `/${parts.join('/')}` : '/';
};

const pickMode = computed(() => props.pickMode);
const canSelectFiles = computed(() => pickMode.value !== 'directory');
const canSelectCurrent = computed(() => pickMode.value !== 'file');
const confirmText = computed(() => props.confirmLabel || (pickMode.value === 'file' ? 'Use file' : pickMode.value === 'directory' ? 'Use folder' : 'Use path'));

const breadcrumbs = computed(() => {
  const path = currentPath.value || '/';
  const parts = path.split('/').filter(Boolean);
  const crumbs = [{ label: 'Root', path: '/' }];
  let current = '';
  for (const part of parts) {
    current += `/${part}`;
    crumbs.push({ label: part, path: current });
  }
  return crumbs;
});

const loadPath = async (path: string, keepSelection = false) => {
  const target = normalizedPath(path);
  loading.value = true;
  try {
    const res = await axios.get('/system/ls', { params: { path: target } });
    entries.value = Array.isArray(res.data)
      ? [...res.data].sort((a: PathEntry, b: PathEntry) => {
          if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
          return a.name.localeCompare(b.name);
        })
      : [];
    currentPath.value = target;
    jumpPath.value = target;
    if (!keepSelection) {
      selectedPath.value = pickMode.value === 'directory' ? target : '';
    }
  } catch (err: any) {
    message.error(err?.response?.data?.error || 'Failed to read directory');
  } finally {
    loading.value = false;
  }
};

const openEntry = async (entry: PathEntry) => {
  if (entry.isDir) {
    await loadPath(entry.path);
    return;
  }
  if (canSelectFiles.value) {
    selectedPath.value = entry.path;
  }
};

const chooseEntry = (entry: PathEntry) => {
  if (entry.isDir) {
    selectedPath.value = pickMode.value === 'directory' ? entry.path : '';
    void loadPath(entry.path, false);
    return;
  }
  if (canSelectFiles.value) {
    selectedPath.value = entry.path;
  }
};

const useCurrentPath = () => {
  if (!canSelectCurrent.value) return;
  emit('confirm', pickMode.value === 'directory' ? currentPath.value : (selectedPath.value || currentPath.value));
  emit('update:open', false);
};

const useSelectedPath = () => {
  const value = selectedPath.value || (pickMode.value === 'directory' ? currentPath.value : '');
  if (!value) {
    message.warning('Please select a path first');
    return;
  }
  emit('confirm', value);
  emit('update:open', false);
};

const handleConfirm = () => {
  if (pickMode.value === 'file' && !selectedPath.value) {
    message.warning('Please select a file first');
    return;
  }
  if (pickMode.value === 'directory') {
    useCurrentPath();
    return;
  }
  if (selectedPath.value) {
    useSelectedPath();
    return;
  }
  if (currentPath.value) {
    emit('confirm', currentPath.value);
    emit('update:open', false);
  }
};

const goUp = async () => {
  await loadPath(parentPath(currentPath.value));
};

const jumpToPath = async () => {
  await loadPath(jumpPath.value);
};

const isHighlighted = (entry: PathEntry) => entry.path === selectedPath.value;

watch(
  () => props.open,
  (visible) => {
    if (visible) {
      void loadPath(props.initialPath || '/', false);
    }
  },
  { immediate: true },
);

watch(
  () => props.initialPath,
  (value) => {
    if (props.open) {
      void loadPath(value || '/', false);
    }
  },
);
</script>

<template>
  <a-drawer
    :open="open"
    :title="title"
    width="760"
    :destroyOnClose="false"
    @close="emit('update:open', false)"
  >
    <a-space direction="vertical" :size="12" style="width: 100%;">
      <a-input-search
        v-model:value="jumpPath"
        placeholder="/path/to/folder-or-file"
        enter-button="Go"
        @search="jumpToPath"
      />

      <a-space wrap :size="8">
        <a-tag color="blue">Current: {{ currentPath }}</a-tag>
        <a-tag v-if="pickMode === 'directory'" color="green">Directory mode</a-tag>
        <a-tag v-else-if="pickMode === 'file'" color="purple">File mode</a-tag>
        <a-tag v-else color="cyan">Any path mode</a-tag>
      </a-space>

      <a-breadcrumb>
        <a-breadcrumb-item v-for="crumb in breadcrumbs" :key="crumb.path">
          <a @click="void loadPath(crumb.path)">{{ crumb.label }}</a>
        </a-breadcrumb-item>
      </a-breadcrumb>

      <a-space>
        <a-button @click="goUp" :disabled="currentPath === '/'">
          <template #icon>
            <LeftOutlined />
          </template>
          Up
        </a-button>
        <a-button @click="() => void loadPath(currentPath)">
          <template #icon>
            <SearchOutlined />
          </template>
          Refresh
        </a-button>
      </a-space>

      <a-spin :spinning="loading">
        <a-list
          bordered
          :data-source="entries"
          :locale="{ emptyText: 'No entries found' }"
          style="max-height: 60vh; overflow: auto;"
        >
          <template #renderItem="{ item }">
            <a-list-item :class="{ 'path-picker__selected': isHighlighted(item) }">
              <div class="path-picker__row" @click="chooseEntry(item)">
                <div class="path-picker__name">
                  <FolderOutlined v-if="item.isDir" style="color: #1890ff;" />
                  <FileOutlined v-else style="color: #8c8c8c;" />
                  <span>{{ item.name }}</span>
                </div>
                <a-space>
                  <a-tag :color="item.isDir ? 'blue' : 'default'">
                    {{ item.isDir ? 'Dir' : 'File' }}
                  </a-tag>
                  <a-button size="small" @click.stop="openEntry(item)">
                    {{ item.isDir ? 'Open' : 'Select' }}
                  </a-button>
                </a-space>
              </div>
            </a-list-item>
          </template>
        </a-list>
      </a-spin>

      <a-divider style="margin: 8px 0;" />

      <a-space style="justify-content: space-between; width: 100%;" align="center">
        <span style="color: #666;">
          {{ pickMode === 'directory'
            ? 'Choose the current folder or navigate deeper before confirming.'
            : pickMode === 'file'
              ? 'Navigate to the target file, then confirm the selected file.'
              : 'Pick a file or keep the current directory.'
          }}
        </span>
        <a-space>
          <a-button @click="emit('update:open', false)">Cancel</a-button>
          <a-button type="primary" @click="handleConfirm">
            <template #icon>
              <CheckOutlined />
            </template>
            {{ confirmText }}
          </a-button>
        </a-space>
      </a-space>
    </a-space>
  </a-drawer>
</template>

<style scoped>
.path-picker__row {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  cursor: pointer;
}

.path-picker__name {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
  word-break: break-all;
}

:deep(.path-picker__selected) {
  background: #f0f7ff;
}
</style>
