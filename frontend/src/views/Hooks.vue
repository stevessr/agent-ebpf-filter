<script setup lang="ts">
import { ref, onMounted } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { SettingOutlined, LinkOutlined, CheckCircleOutlined, DeleteOutlined } from '@ant-design/icons-vue';

interface HookDef {
  id: string;
  name: string;
  description: string;
  target_cmd: string;
  installed: boolean;
}

const hooks = ref<HookDef[]>([]);
const loading = ref(false);

const fetchHooks = async () => {
  loading.value = true;
  try {
    const res = await axios.get('/config/hooks');
    hooks.value = res.data;
  } catch (err) {
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
      install: !hook.installed
    });
    message.success(`${hook.installed ? 'Uninstalled' : 'Installed'} hook for ${hook.name}`);
    await fetchHooks(); // Refresh status
  } catch (err) {
    message.error(`Failed to ${hook.installed ? 'uninstall' : 'install'} hook`);
  } finally {
    loading.value = false;
  }
};

onMounted(() => {
  fetchHooks();
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
          message="About CLI Hooks"
          description="Installing a hook sets up an alias in your local shell configuration (~/.zshrc or ~/.bashrc) so that commands to popular AI CLIs (like Claude Code, Gemini CLI, Copilot) are transparently routed through agent-wrapper. This allows eBPF-Filter to apply security rules and tracking tags."
          type="info"
          show-icon
          style="margin-bottom: 24px;"
        />

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
                      Target: <span style="background: #f0f0f0; padding: 2px 6px; border-radius: 3px;">{{ item.target_cmd }}</span>
                    </div>
                  </div>
                  <a-tag :color="item.installed ? 'success' : 'default'">
                    <template #icon>
                      <CheckCircleOutlined v-if="item.installed" />
                    </template>
                    {{ item.installed ? 'Installed' : 'Not Installed' }}
                  </a-tag>
                </div>
                <p style="font-size: 13px; color: #555; height: 40px; margin-bottom: 16px;">
                  {{ item.description }}
                </p>
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
                    {{ item.installed ? 'Uninstall Hook' : 'Install Hook' }}
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
</style>