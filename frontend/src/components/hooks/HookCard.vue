<script setup lang="ts">
import { computed } from 'vue';
import {
  LinkOutlined,
  CheckCircleOutlined,
  DeleteOutlined,
  ThunderboltOutlined,
  SwapOutlined,
  EditOutlined,
} from '@ant-design/icons-vue';
import type { HookDef } from '../../types/hooks';
import { getHookCliDoc } from '../../data/hookCatalog';

const props = defineProps<{
  hook: HookDef;
  useWrapper: boolean;
  loading: boolean;
}>();

const emit = defineEmits<{
  (e: 'update:useWrapper', value: boolean): void;
  (e: 'toggle', hook: HookDef): void;
  (e: 'edit', hook: HookDef): void;
}>();

const docSourceUrl = computed(() => getHookCliDoc(props.hook.id)?.sources?.[0]?.url || '');

const wrapperOverride = computed({
  get: () => props.useWrapper,
  set: (value: boolean) => emit('update:useWrapper', value),
});
</script>

<template>
  <a-card size="small" hoverable style="border-radius: 6px;">
    <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px;">
      <div>
        <h3 style="margin: 0; font-size: 15px; font-weight: 600;">{{ hook.name }}</h3>
        <div style="font-family: monospace; font-size: 12px; color: #888; margin-top: 4px;">
          cmd:
          <span style="background: #f0f0f0; padding: 2px 6px; border-radius: 3px;">{{ hook.target_cmd }}</span>
        </div>
      </div>
      <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px;">
        <a-tag :color="hook.installed ? 'success' : 'default'">
          <template #icon><CheckCircleOutlined v-if="hook.installed" /></template>
          {{ hook.installed ? 'Installed' : 'Not Installed' }}
        </a-tag>
        <a-tag :color="hook.hook_type === 'native' ? 'blue' : 'orange'">
          <template #icon>
            <ThunderboltOutlined v-if="hook.hook_type === 'native'" />
            <SwapOutlined v-else />
          </template>
          {{ hook.hook_type === 'native' ? 'Native Hook' : 'Wrapper Hook' }}
        </a-tag>
      </div>
    </div>

    <p style="font-size: 13px; color: #555; min-height: 36px; margin-bottom: 12px;">
      {{ hook.description }}
    </p>

    <div v-if="hook.hook_type === 'native' && !hook.installed" style="margin-bottom: 12px;">
      <a-checkbox v-model:checked="wrapperOverride">
        <span style="font-size: 12px; color: #888;">Use wrapper alias instead</span>
      </a-checkbox>
    </div>

    <div
      style="text-align: right; border-top: 1px solid #f0f0f0; padding-top: 12px; display: flex; justify-content: flex-end; gap: 8px;"
    >
      <a-button v-if="docSourceUrl" size="small" :href="docSourceUrl" target="_blank">
        <template #icon><LinkOutlined /></template>
        Docs
      </a-button>
      <a-button v-if="hook.hook_type === 'native'" size="small" @click="emit('edit', hook)">
        <template #icon><EditOutlined /></template>
        Edit Config
      </a-button>
      <a-button
        :type="hook.installed ? 'default' : 'primary'"
        :danger="hook.installed"
        @click="emit('toggle', hook)"
        :loading="loading"
        size="small"
      >
        <template #icon>
          <DeleteOutlined v-if="hook.installed" />
          <LinkOutlined v-else />
        </template>
        {{ hook.installed ? 'Uninstall' : 'Install Hook' }}
      </a-button>
    </div>
  </a-card>
</template>
