<script setup lang="ts">
import { ref, watch, computed } from "vue";
import axios from "axios";
import {
  FolderOutlined,
  FileOutlined,
  LeftOutlined,
  HomeOutlined,
  CheckOutlined,
} from "@ant-design/icons-vue";

interface FileEntry {
  name: string;
  isDir: boolean;
  path: string;
}

const props = withDefaults(
  defineProps<{
    open: boolean;
    startPath?: string;
    directoryOnly?: boolean;
  }>(),
  {
    startPath: "/",
    directoryOnly: false,
  },
);

const emit = defineEmits<{
  "update:open": [value: boolean];
  select: [path: string];
}>();

const currentPath = ref("/");
const entries = ref<FileEntry[]>([]);
const loading = ref(false);
const homePath = ref("/");

const fetchEntries = async (path: string) => {
  loading.value = true;
  try {
    const res = await axios.get("/system/ls", {
      params: { path, showHidden: false, limit: 200 },
    });
    const items: FileEntry[] = res.data.items || [];
    // Sort: dirs first, then alphabetically
    items.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    entries.value = items;
    currentPath.value = path;
  } catch {
    entries.value = [];
  } finally {
    loading.value = false;
  }
};

const fetchHome = async () => {
  try {
    const res = await axios.get("/system/home");
    homePath.value = res.data.path || "/";
  } catch {
    homePath.value = "/";
  }
};

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      fetchHome().then(() => {
        fetchEntries(props.startPath || homePath.value || "/");
      });
    }
  },
);

const goUp = () => {
  const parts = currentPath.value.split("/").filter((p) => p);
  parts.pop();
  const parent = "/" + parts.join("/");
  fetchEntries(parent || "/");
};

const navigateTo = (path: string) => {
  fetchEntries(path);
};

const selectEntry = (entry: FileEntry) => {
  if (props.directoryOnly && !entry.isDir) return;
  if (entry.isDir) {
    fetchEntries(entry.path);
  } else {
    emit("select", entry.path);
    emit("update:open", false);
  }
};

const selectCurrentDir = () => {
  emit("select", currentPath.value);
  emit("update:open", false);
};

const pathBreadcrumbs = computed(() => {
  const parts = currentPath.value.split("/").filter((p) => p);
  const crumbs = [{ name: "/", path: "/" }];
  let cur = "";
  parts.forEach((p) => {
    cur += "/" + p;
    crumbs.push({ name: p, path: cur });
  });
  return crumbs;
});
</script>

<template>
  <a-modal
    :open="open"
    title="Browse Filesystem"
    width="620px"
    :footer="null"
    @cancel="emit('update:open', false)"
    @update:open="emit('update:open', $event)"
  >
    <!-- Breadcrumb -->
    <div class="fpb-breadcrumb">
      <a-breadcrumb>
        <a-breadcrumb-item v-for="crumb in pathBreadcrumbs" :key="crumb.path">
          <a @click.prevent="navigateTo(crumb.path)" class="fpb-crumb-link">
            {{ crumb.name }}
          </a>
        </a-breadcrumb-item>
      </a-breadcrumb>
    </div>

    <!-- Nav buttons -->
    <div class="fpb-nav">
      <a-button size="small" @click="goUp" :disabled="currentPath === '/'">
        <template #icon><LeftOutlined /></template>
        Back
      </a-button>
      <a-button size="small" @click="navigateTo(homePath)">
        <template #icon><HomeOutlined /></template>
        Home
      </a-button>
      <a-button
        v-if="directoryOnly"
        size="small"
        type="primary"
        @click="selectCurrentDir"
        style="margin-left: auto"
      >
        <CheckOutlined /> Select This Directory
      </a-button>
    </div>

    <!-- File list -->
    <div class="fpb-list">
      <a-spin :spinning="loading">
        <a-empty
          v-if="!loading && entries.length === 0"
          description="Empty directory"
          style="padding: 24px"
        />
        <div v-else class="fpb-entries">
          <div
            v-for="entry in entries"
            :key="entry.path"
            class="fpb-entry"
            :class="{
              'is-dir': entry.isDir,
              'is-disabled': directoryOnly && !entry.isDir,
            }"
            role="button"
            :tabindex="directoryOnly && !entry.isDir ? -1 : 0"
            :aria-disabled="directoryOnly && !entry.isDir"
            :aria-label="`${entry.isDir ? 'Open directory' : 'Select file'} ${entry.name}`"
            @click="selectEntry(entry)"
            @keydown.enter.prevent="selectEntry(entry)"
            @keydown.space.prevent="selectEntry(entry)"
          >
            <FolderOutlined v-if="entry.isDir" class="fpb-icon dir-icon" />
            <FileOutlined v-else class="fpb-icon file-icon" />
            <span class="fpb-name">{{ entry.name }}</span>
            <span v-if="entry.isDir" class="fpb-hint">Click to open</span>
            <span v-else-if="!directoryOnly" class="fpb-hint">Click to select</span>
          </div>
        </div>
      </a-spin>
    </div>
  </a-modal>
</template>

<style scoped>
.fpb-breadcrumb {
  margin-bottom: 10px;
}
.fpb-crumb-link {
  color: #374151;
  font-weight: 600;
  font-size: 12px;
}
.fpb-nav {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}
.fpb-list {
  max-height: 360px;
  overflow-y: auto;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}
.fpb-entries {
  display: flex;
  flex-direction: column;
}
.fpb-entry {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  cursor: pointer;
  transition: background 0.1s;
  border-bottom: 1px solid #f5f5f5;
}
.fpb-entry:last-child {
  border-bottom: none;
}
.fpb-entry:hover {
  background: #f0f5ff;
}
.fpb-entry.is-disabled {
  opacity: 0.4;
  cursor: default;
}
.fpb-entry.is-disabled:hover {
  background: transparent;
}
.fpb-icon {
  font-size: 16px;
  flex-shrink: 0;
}
.dir-icon {
  color: #1890ff;
}
.file-icon {
  color: #4b5563;
}
.fpb-name {
  font-family: ui-monospace, monospace;
  font-size: 13px;
  color: #1f2937;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fpb-hint {
  margin-left: auto;
  font-size: 11px;
  color: #9ca3af;
}
</style>
