<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import axios from 'axios';
import { CodeOutlined, PlayCircleOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import ShellTerminalPane from './ShellTerminalPane.vue';
import type { ShellSessionCreateRequest, ShellSessionInfo } from '../types/shell';

const RECENT_COMMANDS_STORAGE_KEY = 'recent_cmds';

const props = withDefaults(defineProps<{
  active?: boolean;
  defaultEnv?: Record<string, string>;
}>(), {
  active: false,
});

const command = ref('');
const args = ref('');
const recentCommands = ref<string[]>(
  JSON.parse(localStorage.getItem(RECENT_COMMANDS_STORAGE_KEY) || '[]'),
);
const launching = ref(false);
const session = ref<ShellSessionInfo | null>(null);

const splitArgs = (input: string) => {
  const output: string[] = [];
  let current = '';
  let quote: '"' | '\'' | null = null;
  let escaped = false;

  for (const char of input.trim()) {
    if (escaped) {
      current += char;
      escaped = false;
      continue;
    }

    if (char === '\\') {
      escaped = true;
      continue;
    }

    if (quote) {
      if (char === quote) {
        quote = null;
        continue;
      }
      current += char;
      continue;
    }

    if (char === '"' || char === '\'') {
      quote = char;
      continue;
    }

    if (/\s/.test(char)) {
      if (current) {
        output.push(current);
        current = '';
      }
      continue;
    }

    current += char;
  }

  if (escaped) {
    current += '\\';
  }

  if (current) {
    output.push(current);
  }

  return output;
};

const persistRecentCommands = () => {
  localStorage.setItem(RECENT_COMMANDS_STORAGE_KEY, JSON.stringify(recentCommands.value));
};

const useRecent = (cmdStr: string) => {
  const parts = splitArgs(cmdStr);
  command.value = parts[0] || '';
  args.value = parts.slice(1).join(' ');
};

const launchPreview = computed(() => {
  const executable = command.value.trim();
  if (!executable) {
    return 'Enter a command to launch it through agent-wrapper';
  }
  const argList = splitArgs(args.value);
  return ['agent-wrapper', executable, ...argList].join(' ');
});

const closeSession = async () => {
  const current = session.value;
  if (!current) {
    return;
  }

  session.value = null;
  try {
    await axios.delete(`/shell-sessions/${current.id}`);
  } catch (err: any) {
    const status = err?.response?.status;
    if (status !== 404) {
      message.error(err?.response?.data?.error || err?.message || 'Failed to close remote terminal');
    }
  }
};

const launchCommand = async () => {
  const executable = command.value.trim();
  if (!executable) {
    message.error('Please enter an executable');
    return;
  }

  const argList = splitArgs(args.value);
  launching.value = true;
  try {
    if (session.value) {
      await closeSession();
    }

    const payload: ShellSessionCreateRequest = {
      shell: 'wrapper',
      command: 'agent-wrapper',
      args: [executable, ...argList],
      cols: 100,
      rows: 32,
      label: `wrapper: ${executable}`,
      kind: 'wrapper',
      env: props.defaultEnv && Object.keys(props.defaultEnv).length > 0
        ? { ...props.defaultEnv }
        : undefined,
    };

    const res = await axios.post('/shell-sessions', payload);
    const created = res.data as ShellSessionInfo;

    if (!props.active) {
      try {
        await axios.delete(`/shell-sessions/${created.id}`);
      } catch {
        // Best-effort cleanup if the user already left the tab.
      }
      return;
    }

    session.value = created;

    const full = `${executable} ${args.value}`.trim();
    if (!recentCommands.value.includes(full)) {
      recentCommands.value.unshift(full);
      recentCommands.value = recentCommands.value.slice(0, 10);
      persistRecentCommands();
    }
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to launch remote terminal');
  } finally {
    launching.value = false;
  }
};

interface WrapperEventRecord {
  receivedAt: string;
  event: {
    pid: number;
    comm: string;
    type: string;
    tag: string;
    path: string;
  };
}

const recentEvents = ref<WrapperEventRecord[]>([]);
let eventsPollTimer: number | null = null;

const fetchRecentEvents = async () => {
  if (!props.active) return;
  try {
    const res = await axios.get('/events/recent', {
      params: { type: 'wrapper_intercept', limit: 20 },
    });
    recentEvents.value = (res.data.events || []).reverse();
  } catch {
    // Silently ignore poll errors
  }
};

const startEventsPolling = () => {
  stopEventsPolling();
  fetchRecentEvents();
  eventsPollTimer = window.setInterval(fetchRecentEvents, 3000);
};

const stopEventsPolling = () => {
  if (eventsPollTimer !== null) {
    clearInterval(eventsPollTimer);
    eventsPollTimer = null;
  }
};

const formatEventTime = (iso: string) => {
  const d = new Date(iso);
  return d.toLocaleTimeString();
};

const closeTemporaryTerminal = async () => {
  if (!session.value) return;
  await closeSession();
};

watch(
  () => props.active,
  (active) => {
    if (!active) {
      void closeSession();
      stopEventsPolling();
    } else {
      startEventsPolling();
    }
  },
);

onBeforeUnmount(() => {
  void closeSession();
  stopEventsPolling();
});
</script>

<template>
  <a-row :gutter="[16, 16]">
    <a-col :xs="24" :xl="10">
      <a-card title="Remote Executor (via Wrapper)" :bordered="false">
        <template #extra>
          <a-tag color="blue">ephemeral PTY</a-tag>
        </template>

        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 16px;"
          message="Commands run through agent-wrapper in a temporary terminal."
          description="Leaving this tab destroys the backend PTY session automatically."
        />

        <a-form layout="vertical">
          <a-form-item label="Executable">
            <a-input
              v-model:value="command"
              placeholder="e.g. ls, python, git"
              @pressEnter="launchCommand"
            >
              <template #prefix>
                <CodeOutlined />
              </template>
            </a-input>
          </a-form-item>

          <a-form-item label="Arguments">
            <a-input
              v-model:value="args"
              placeholder="e.g. -la /tmp"
              @pressEnter="launchCommand"
            />
          </a-form-item>

          <a-alert
            type="success"
            show-icon
            style="margin-bottom: 16px;"
            :message="launchPreview"
          />

          <a-button
            type="primary"
            :loading="launching"
            block
            @click="launchCommand"
          >
            <template #icon>
              <PlayCircleOutlined />
            </template>
            Launch temporary terminal
          </a-button>

          <a-divider orientation="left">Recent Commands</a-divider>
          <a-list size="small" :data-source="recentCommands">
            <template #renderItem="{ item }">
              <a-list-item>
                <code style="cursor: pointer; color: #1890ff" @click="useRecent(item)">{{ item }}</code>
              </a-list-item>
            </template>
            <template v-if="recentCommands.length === 0" #header>
              <div style="text-align: center; color: #999;">No recent commands</div>
            </template>
          </a-list>
        </a-form>
      </a-card>
    </a-col>

    <a-col :xs="24" :xl="14">
      <a-card title="Active Wrapper Terminal" :bordered="false">
        <template #extra>
          <a-space :size="8">
            <a-tag v-if="session" color="green">live</a-tag>
            <a-tag v-else color="default">idle</a-tag>
            <a-button size="small" :disabled="!session" @click="closeTemporaryTerminal">
              Close temp terminal
            </a-button>
          </a-space>
        </template>

        <template v-if="session">
          <ShellTerminalPane
            :session="session"
            :active="active"
            :show-detach="false"
            @close-session="closeSession"
          />
        </template>
        <template v-else>
          <a-empty
            description="Launch a command to open a temporary wrapper terminal. Switch away from this tab and it will be destroyed automatically."
          />
        </template>
      </a-card>

      <a-card title="Recent Wrapper Events (eBPF)" :bordered="false" style="margin-top: 16px;">
        <template #extra>
          <a-tag color="orange">live</a-tag>
        </template>
        <a-table
          :data-source="recentEvents"
          :columns="[
            { title: 'Time', dataIndex: 'receivedAt', key: 'receivedAt' },
            { title: 'Command', dataIndex: ['event', 'comm'], key: 'comm' },
            { title: 'Args/Path', dataIndex: ['event', 'path'], key: 'path', ellipsis: true },
            { title: 'Tag', dataIndex: ['event', 'tag'], key: 'tag' },
          ]"
          :pagination="false"
          size="small"
          row-key="receivedAt"
          :scroll="{ x: true }"
          :locale="{ emptyText: 'Waiting for wrapper events...' }"
        >
        <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'receivedAt'">
              <span style="font-size: 12px; white-space: nowrap;">{{ formatEventTime(record.receivedAt) }}</span>
            </template>
            <template v-else-if="column.key === 'comm'">
              <code>{{ record.event.comm }}</code>
            </template>
            <template v-else-if="column.key === 'path'">
              <span style="font-size: 12px;">{{ record.event.path }}</span>
            </template>
            <template v-else-if="column.key === 'tag'">
              <a-tag color="blue">{{ record.event.tag }}</a-tag>
            </template>
          </template>
        </a-table>
      </a-card>
    </a-col>
  </a-row>
</template>
