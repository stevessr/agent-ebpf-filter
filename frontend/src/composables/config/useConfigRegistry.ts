import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import type { TrackedItem } from '../types/config';

const CATEGORY_COLORS: Record<string, string> = {
  red: 'red', orange: 'orange', gold: 'gold', green: 'green', cyan: 'cyan',
  blue: 'blue', purple: 'purple', magenta: 'magenta', pink: 'pink',
};

let colorIdx = 0;
const assignedColors: Record<string, string> = {};

export function getCategoryColor(tag: string | undefined): string {
  const key = tag || 'default';
  if (assignedColors[key]) return assignedColors[key];
  const keys = Object.keys(CATEGORY_COLORS);
  const color = CATEGORY_COLORS[keys[colorIdx % keys.length]] || 'default';
  assignedColors[key] = color;
  colorIdx++;
  return color;
}

export function useConfigRegistry() {
  // ── State ──
  const tags = ref<string[]>([]);
  const trackedItems = ref<TrackedItem[]>([]);
  const trackedPaths = ref<TrackedItem[]>([]);
  const trackedPrefixes = ref<TrackedItem[]>([]);

  // Input fields
  const newTagName = ref('');
  const newCommName = ref('');
  const newCommTag = ref('');
  const newPathName = ref('');
  const newPathTag = ref('');
  const newPrefixValue = ref('');
  const newPrefixTag = ref('');
  const importFileInput = ref<HTMLInputElement | null>(null);

  // Path picker
  const pathPickerOpen = ref(false);
  const pathPickerTarget = ref<"exact" | "prefix">("exact");

  // ── Fetch Functions ──
  const fetchTags = async () => {
    try {
      const res = await axios.get('/config/tags');
      tags.value = res.data;
      if (tags.value.length > 0) {
        if (!newCommTag.value) newCommTag.value = tags.value[0];
        if (!newPathTag.value) newPathTag.value = tags.value[0];
      }
    } catch (_) {}
  };

  const fetchTrackedComms = async () => {
    try {
      const res = await axios.get('/config/comms');
      trackedItems.value = res.data;
    } catch (_) {}
  };

  const fetchTrackedPaths = async () => {
    try {
      const res = await axios.get('/config/paths');
      trackedPaths.value = res.data;
    } catch (_) {}
  };

  const fetchTrackedPrefixes = async () => {
    try {
      const res = await axios.get('/config/prefixes');
      trackedPrefixes.value = res.data;
    } catch (_) {}
  };

  // ── CRUD: Tags ──
  const addTag = async () => {
    if (!newTagName.value) return;
    try {
      await axios.post('/config/tags', { name: newTagName.value });
      message.success(`Tag "${newTagName.value}" created`);
      newTagName.value = '';
      fetchTags();
    } catch (_) {
      message.error('Failed to create tag');
    }
  };

  // ── CRUD: Comms ──
  const addComm = async () => {
    if (!newCommName.value || !newCommTag.value) return;
    try {
      await axios.post('/config/comms', {
        comm: newCommName.value,
        tag: newCommTag.value,
      });
      message.success(`Added ${newCommName.value}`);
      newCommName.value = '';
      fetchTrackedComms();
    } catch (_) {
      message.error('Failed to add command');
    }
  };

  const removeComm = async (comm: string) => {
    try {
      await axios.delete(`/config/comms/${comm}`);
      message.success(`Removed ${comm}`);
      fetchTrackedComms();
    } catch (_) {}
  };

  const toggleCommDisabled = async (comm: string, disabled: boolean) => {
    try {
      if (disabled) {
        await axios.delete(`/config/comms/${comm}/disable`);
        message.success(`Re-enabled ${comm}`);
      } else {
        await axios.post(`/config/comms/${comm}/disable`);
        message.success(`Disabled ${comm}`);
      }
      fetchTrackedComms();
    } catch (_) {
      message.error(`Failed to toggle ${comm}`);
    }
  };

  // ── CRUD: Paths ──
  const addPath = async () => {
    if (!newPathName.value || !newPathTag.value) return;
    try {
      await axios.post('/config/paths', {
        path: newPathName.value,
        tag: newPathTag.value,
      });
      message.success(`Added path ${newPathName.value}`);
      newPathName.value = '';
      fetchTrackedPaths();
    } catch (_) {}
  };

  const removePath = async (path: string) => {
    try {
      await axios.delete(`/config/paths/${path}`);
      message.success(`Removed path ${path}`);
      fetchTrackedPaths();
    } catch (_) {}
  };

  // ── CRUD: Prefixes ──
  const addPrefix = async () => {
    if (!newPrefixValue.value || !newPrefixTag.value) return;
    try {
      await axios.post('/config/prefixes', {
        prefix: newPrefixValue.value,
        tag: newPrefixTag.value,
      });
      message.success(`Added prefix ${newPrefixValue.value}`);
      newPrefixValue.value = '';
      fetchTrackedPrefixes();
    } catch (_) {
      message.error('Failed to add prefix');
    }
  };

  const removePrefix = async (prefix: string) => {
    try {
      await axios.delete('/config/prefixes', { params: { prefix } });
      message.success(`Removed prefix ${prefix}`);
      fetchTrackedPrefixes();
    } catch (_) {
      message.error('Failed to remove prefix');
    }
  };

  // ── Import/Export ──
  const exportConfig = async () => {
    try {
      const res = await axios.get('/config/export');
      const blob = new Blob([JSON.stringify(res.data, null, 2)], {
        type: 'application/json;charset=utf-8',
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `agent-ebpf-config-${new Date().toISOString().slice(0, 10)}.json`;
      link.click();
      URL.revokeObjectURL(url);
      message.success('Configuration exported');
    } catch (_) {
      message.error('Failed to export configuration');
    }
  };

  const importConfig = async (event: Event) => {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      const data = JSON.parse(text);
      await axios.post('/config/import', data);
      message.success('Configuration imported successfully');
      fetchTags();
      fetchTrackedComms();
      fetchTrackedPaths();
      fetchTrackedPrefixes();
    } catch (_) {
      message.error('Failed to import configuration');
    } finally {
      input.value = '';
    }
  };

  const clearAllConfig = async () => {
    try {
      for (const item of trackedItems.value) {
        if (item.comm) await axios.delete(`/config/comms/${item.comm}`);
      }
      for (const item of trackedPaths.value) {
        if (item.path) await axios.delete(`/config/paths/${item.path}`);
      }
      for (const item of trackedPrefixes.value) {
        if (item.prefix) await axios.delete('/config/prefixes', { params: { prefix: item.prefix } });
      }
      for (const comm of Object.keys({})) {
        await axios.delete(`/config/rules/${comm}`);
      }
      await axios.delete('/config/tags');
      message.success('All config cleared');
      fetchTags();
      fetchTrackedComms();
      fetchTrackedPaths();
      fetchTrackedPrefixes();
    } catch (_) {
      message.error('Failed to clear config');
    }
  };

  // ── Path Picker ──
  const openPathPicker = (target: "exact" | "prefix") => {
    pathPickerTarget.value = target;
    pathPickerOpen.value = true;
  };

  const handlePathPicked = (path: string) => {
    if (pathPickerTarget.value === 'exact') {
      newPathName.value = path;
    } else {
      newPrefixValue.value = path;
    }
  };

  // ── Computed: Grouped items ──
  const groupedTrackedItems = computed(() => {
    const groups: Record<string, TrackedItem[]> = {};
    for (const item of trackedItems.value) {
      const tag = item.tag || 'untagged';
      if (!groups[tag]) groups[tag] = [];
      groups[tag].push(item);
    }
    return groups;
  });

  const groupedTrackedPaths = computed(() => {
    const groups: Record<string, TrackedItem[]> = {};
    for (const item of trackedPaths.value) {
      const tag = item.tag || 'untagged';
      if (!groups[tag]) groups[tag] = [];
      groups[tag].push(item);
    }
    return groups;
  });

  const groupedTrackedPrefixes = computed(() => {
    const groups: Record<string, TrackedItem[]> = {};
    for (const item of trackedPrefixes.value) {
      const tag = item.tag || 'untagged';
      if (!groups[tag]) groups[tag] = [];
      groups[tag].push(item);
    }
    return groups;
  });

  return {
    openImportPicker: () => { importFileInput.value?.click(); },
    tags, trackedItems, trackedPaths, trackedPrefixes,
    newTagName, newCommName, newCommTag, newPathName, newPathTag, newPrefixValue, newPrefixTag,
    importFileInput,
    pathPickerOpen, pathPickerTarget,
    fetchTags, fetchTrackedComms, fetchTrackedPaths, fetchTrackedPrefixes,
    addTag, addComm, removeComm, toggleCommDisabled,
    addPath, removePath, addPrefix, removePrefix,
    exportConfig, importConfig, clearAllConfig,
    openPathPicker, handlePathPicked,
    groupedTrackedItems, groupedTrackedPaths, groupedTrackedPrefixes,
  };
}
