<script setup lang="ts">
import { ref, onMounted } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { LinkOutlined, CheckCircleOutlined, DeleteOutlined, ThunderboltOutlined, SwapOutlined } from '@ant-design/icons-vue';

interface HookDef {
  id: string;
  name: string;
  description: string;
  target_cmd: string;
  hook_type: 'native' | 'wrapper';
  installed: boolean;
}

const hooks = ref<HookDef[]>([]);
const loading = ref(false);
// Track which hooks the user wants to force-use wrapper mode for.
const useWrapperOverride = ref<Record<string, boolean>>({});

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
              <b>Native Hook</b> (recommended): Injects directly into the agent CLI's own config (e.g. Claude Code's <code>~/.claude/settings.json</code>). Intercepts every tool call with zero shell overhead.<br/>
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
              <a-card size="small" hoverable style="border-radius: 6px;">
                <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px;">
                  <div>
                    <h3 style="margin: 0; font-size: 15px; font-weight: 600;">{{ item.name }}</h3>
                    <div style="font-family: monospace; font-size: 12px; color: #888; margin-top: 4px;">
                      cmd: <span style="background: #f0f0f0; padding: 2px 6px; border-radius: 3px;">{{ item.target_cmd }}</span>
                    </div>
                  </div>
                  <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 4px;">
                    <a-tag :color="item.installed ? 'success' : 'default'">
                      <template #icon><CheckCircleOutlined v-if="item.installed" /></template>
                      {{ item.installed ? 'Installed' : 'Not Installed' }}
                    </a-tag>
                    <a-tag :color="item.hook_type === 'native' ? 'blue' : 'orange'">
                      <template #icon>
                        <ThunderboltOutlined v-if="item.hook_type === 'native'" />
                        <SwapOutlined v-else />
                      </template>
                      {{ item.hook_type === 'native' ? 'Native Hook' : 'Wrapper Hook' }}
                    </a-tag>
                  </div>
                </div>

                <p style="font-size: 13px; color: #555; min-height: 36px; margin-bottom: 12px;">
                  {{ item.description }}
                </p>

                <!-- For native-capable CLIs, allow opting into wrapper mode -->
                <div v-if="item.hook_type === 'native' && !item.installed" style="margin-bottom: 12px;">
                  <a-checkbox v-model:checked="useWrapperOverride[item.id]">
                    <span style="font-size: 12px; color: #888;">Use wrapper alias instead</span>
                  </a-checkbox>
                </div>

                <div style="text-align: right; border-top: 1px solid #f0f0f0; padding-top: 12px;">
                  <a-button
                    :type="item.installed ? 'default' : 'primary'"
                    :danger="item.installed"
                    @click="toggleHook(item)"
                    :loading="loading"
                  >
                    <template #icon>
                      <DeleteOutlined v-if="item.installed" />
                      <LinkOutlined v-else />
                    </template>
                    {{ item.installed ? 'Uninstall' : 'Install Hook' }}
                  </a-button>
                </div>
              </a-card>
            </a-list-item>
          </template>
        </a-list>
      </a-card>
    </div>
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
