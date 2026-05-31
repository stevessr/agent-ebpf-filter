<script setup lang="ts">
import type { WorkbenchTabState } from '../../types/navigation';

const props = defineProps<{
  tabs: WorkbenchTabState[];
  activeKey: string;
}>();

const emit = defineEmits<{
  change: [key: string | number];
  edit: [targetKey: string | number | MouseEvent, action: 'add' | 'remove'];
}>();

const handleChange = (key: string | number) => {
  emit('change', key);
};

const handleEdit = (targetKey: string | number | MouseEvent, action: 'add' | 'remove') => {
  emit('edit', targetKey, action);
};

const routeTitle = (tab: WorkbenchTabState) => {
  const route = tab.lastRoute;
  if (typeof route === 'string') return route;
  if ('path' in route && route.path) return route.path;
  const name = 'name' in route ? String(route.name || tab.title) : tab.title;
  const params = 'params' in route && route.params ? JSON.stringify(route.params) : '';
  return params === '{}' || !params ? name : `${name} ${params}`;
};
</script>

<template>
  <div class="app-workbench-tabs">
    <a-tabs
      type="editable-card"
      size="small"
      :hideAdd="true"
      :activeKey="props.activeKey"
      @change="handleChange"
      @edit="handleEdit"
    >
      <a-tab-pane
        v-for="tab in props.tabs"
        :key="tab.key"
        :closable="tab.closable"
      >
        <template #tab>
          <span class="app-workbench-tabs__label" :title="routeTitle(tab)">{{ tab.title }}</span>
        </template>
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
.app-workbench-tabs {
  padding: 8px 12px 0;
  background: #fff;
  border-bottom: 1px solid #f0f0f0;
}

.app-workbench-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 0;
}

.app-workbench-tabs :deep(.ant-tabs-tab) {
  border-radius: 8px 8px 0 0 !important;
}

.app-workbench-tabs__label {
  display: inline-block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
  white-space: nowrap;
}
</style>
