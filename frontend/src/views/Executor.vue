<script setup lang="ts">
import { ref, onMounted } from 'vue';
import axios from 'axios';
import { 
  PlusOutlined, 
  PlayCircleOutlined,
  CommandLineOutlined,
  CodeOutlined
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

const command = ref('');
const args = ref('');
const loading = ref(false);
const recentCommands = ref<string[]>(JSON.parse(localStorage.getItem('recent_cmds') || '[]'));

const runCommand = async () => {
  if (!command.value) return;
  loading.value = true;
  try {
    const argList = args.value.split(' ').filter(s => s);
    const res = await axios.post('/system/run', {
      comm: command.value,
      args: argList
    });
    message.success(`Started process PID: ${res.data.pid}`);
    
    // Save to recent
    const full = `${command.value} ${args.value}`.trim();
    if (!recentCommands.value.includes(full)) {
      recentCommands.value.unshift(full);
      recentCommands.value = recentCommands.value.slice(0, 10);
      localStorage.setItem('recent_cmds', JSON.stringify(recentCommands.value));
    }
  } catch (err: any) {
    message.error(`Failed to run: ${err.response?.data?.error || err.message}`);
  } finally {
    loading.value = false;
  }
};

const useRecent = (cmdStr: string) => {
  const parts = cmdStr.split(' ');
  command.value = parts[0];
  args.value = parts.slice(1).join(' ');
};
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%;">
    <a-card title="Remote Executor (via Wrapper)" :bordered="false">
      <template #extra><CommandLineOutlined /></template>
      <p style="color: #666; margin-bottom: 24px;">
        Execute commands on the host system. All commands are automatically routed through the 
        <code>agent-wrapper</code> to enforce security policies and track activities.
      </p>

      <a-form layout="vertical">
        <a-row :gutter="16">
          <a-col :span="8">
            <a-form-item label="Executable">
              <a-input v-model:value="command" placeholder="e.g. ls, python, git" @pressEnter="runCommand">
                <template #prefix><CodeOutlined /></template>
              </a-input>
            </a-form-item>
Col          </a-col>
          <a-col :span="12">
            <a-form-item label="Arguments">
              <a-input v-model:value="args" placeholder="e.g. -la /tmp" @pressEnter="runCommand" />
            </a-form-item>
          </a-col>
          <a-col :span="4" style="display: flex; align-items: flex-end; padding-bottom: 24px;">
            <a-button type="primary" :loading="loading" @click="runCommand" block>
              <template #icon><PlayCircleOutlined /></template>
              Run
            </a-button>
          </a-col>
        </a-row>
      </a-form>

      <a-divider orientation="left">Recent Commands</a-divider>
      <a-list size="small" :dataSource="recentCommands">
        <template #renderItem="{ item }">
          <a-list-item>
            <code style="cursor: pointer; color: #1890ff" @click="useRecent(item)">{{ item }}</code>
          </a-list-item>
        </template>
        <template v-if="recentCommands.length === 0" #header>
          <div style="text-align: center; color: #999;">No recent commands</div>
        </template>
      </a-list>
    </a-card>
  </div>
</template>
