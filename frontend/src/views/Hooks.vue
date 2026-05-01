<script setup lang="ts">
import { ref, onMounted } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { LinkOutlined } from '@ant-design/icons-vue';

import HookCard from '../components/hooks/HookCard.vue';
import HookConfigModal from '../components/hooks/HookConfigModal.vue';
import type { HookDef } from '../types/hooks';

const hooks = ref<HookDef[]>([]);
const loading = ref(false);
// Track which hooks the user wants to force-use wrapper mode for.
const useWrapperOverride = ref<Record<string, boolean>>({});

const showEditModal = ref(false);
const editingHook = ref<HookDef | null>(null);

const fetchHooks = async () => {
  loading.value = true;
  try {
    const res = await axios.get('/config/hooks');
    hooks.value = res.data;
  } catch {
    message.error('Failed to fetch hooks');
  } finally {
    loading.value = false;
  }
};

const openEditModal = (hook: HookDef) => {
  editingHook.value = hook;
  showEditModal.value = true;
};

const toggleHook = async (hook: HookDef) => {
  try {
    loading.value = true;
    await axios.post('/config/hooks', {
      id: hook.id,
      install: !hook.installed,
      use_wrapper: useWrapperOverride.value[hook.id] ?? false,
    });
    message.success(`${hook.installed ? 'Uninstalled' : 'Installed'} hook for ${hook.name}`);
    await fetchHooks();
  } catch {
    message.error(`Failed to ${hook.installed ? 'uninstall' : 'install'} hook`);
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  void fetchHooks();
});
</script>

<template>
  <div style="background: #f0f2f5; padding: 24px; min-height: 100%;">
    <div style="max-width: 1000px; margin: 0 auto;">
      <a-card :bordered="false" style="border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.05);">
        <template #title>
          <div style="display: flex; align-items: center; gap: 8px;">
            <LinkOutlined style="color: #1890ff; font-size: 18px;" />
            <span style="font-size: 16px; font-weight: bold;">AI CLI Interception Hooks</span>
          </div>
        </template>
        <template #extra>
          <a-button @click="fetchHooks" :loading="loading" size="small">Refresh</a-button>
        </template>

        <a-alert
          message="Hook Modes"
          type="info"
          show-icon
          style="margin-bottom: 24px;"
        >
          <template #description>
            <div>
              <b>Native Hook</b> (recommended): Injects directly into the agent CLI's own config (e.g. Claude Code's <code>~/.claude/settings.json</code> or Augment's <code>~/.augment/settings.json</code>). Intercepts every tool call with zero shell overhead.<br/>
              <b>Wrapper Hook</b>: Adds a shell alias so the CLI is transparently routed through <code>agent-wrapper</code>. Works for any CLI but requires a shell reload.
            </div>
          </template>
        </a-alert>

        <a-list
          :grid="{ gutter: 16, xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }"
          :dataSource="hooks"
          :loading="loading"
        >
          <template #renderItem="{ item }">
            <a-list-item>
              <HookCard
                :hook="item"
                :use-wrapper="useWrapperOverride[item.id] ?? false"
                :loading="loading"
                @update:use-wrapper="(value) => (useWrapperOverride[item.id] = value)"
                @toggle="toggleHook"
                @edit="openEditModal"
              />
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </div>

    <HookConfigModal
      v-model:open="showEditModal"
      :hook="editingHook"
      @saved="fetchHooks"
    />
  </div>
</template>

<style scoped>
code {
  background: #f0f0f0;
  padding: 1px 4px;
  border-radius: 3px;
  font-size: 12px;
}
</style>
