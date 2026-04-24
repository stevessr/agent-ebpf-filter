<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

import ShellTerminalPane from './ShellTerminalPane.vue';
import type { ShellConfig, ShellMode, ShellSessionCreateRequest, ShellSessionInfo } from '../types/shell';

const SHELL_STORAGE_KEY = 'executor-shell-config';

const shellModeOptions = [
  { label: 'Auto (fish → zsh → bash → ash → sh)', value: 'auto' },
  { label: 'System shell ($SHELL)', value: 'system' },
  { label: 'fish', value: 'fish' },
  { label: 'zsh', value: 'zsh' },
  { label: 'bash', value: 'bash' },
  { label: 'ash', value: 'ash' },
  { label: 'sh', value: 'sh' },
  { label: 'Custom path', value: 'custom' },
] as const;

const normalizeShellMode = (value: unknown): ShellMode => {
  const candidate = String(value || '').trim().toLowerCase();
  if (
    candidate === 'auto' ||
    candidate === 'system' ||
    candidate === 'env' ||
    candidate === 'fish' ||
    candidate === 'zsh' ||
    candidate === 'bash' ||
    candidate === 'ash' ||
    candidate === 'sh' ||
    candidate === 'custom'
  ) {
    return candidate === 'env' ? 'system' : (candidate as ShellMode);
  }
  return 'auto';
};

const loadShellConfig = (): ShellConfig => {
  try {
    const parsed = JSON.parse(localStorage.getItem(SHELL_STORAGE_KEY) || '{}') as Partial<ShellConfig>;
    return {
      mode: normalizeShellMode(parsed.mode),
      customPath: typeof parsed.customPath === 'string' ? parsed.customPath : '',
    };
  } catch {
    return { mode: 'auto', customPath: '' };
  }
};

const persistShellConfig = () => {
  const payload: ShellConfig = {
    mode: defaultShellMode.value,
    customPath: defaultCustomShellPath.value,
  };
  localStorage.setItem(SHELL_STORAGE_KEY, JSON.stringify(payload));
};

const initialShellConfig = loadShellConfig();
const defaultShellMode = ref<ShellMode>(initialShellConfig.mode);
const defaultCustomShellPath = ref(initialShellConfig.customPath);

const sessions = ref<ShellSessionInfo[]>([]);
const sessionsLoading = ref(false);
const sessionError = ref('');
const creating = ref(false);

const openSessionIds = ref<string[]>([]);
const activeTabKey = ref('');

let refreshTimer: number | null = null;

watch([defaultShellMode, defaultCustomShellPath], persistShellConfig, { immediate: true });

const defaultShellRequest = computed(() => {
  if (defaultShellMode.value === 'custom') {
    return defaultCustomShellPath.value.trim();
  }
  return defaultShellMode.value;
});

const canCreateSession = computed(() => {
  if (defaultShellMode.value === 'custom') {
    return defaultShellRequest.value.length > 0;
  }
  return true;
});

const shellSelectionLabel = computed(() => {
  switch (defaultShellMode.value) {
    case 'auto':
      return 'Auto: fish → zsh → bash → ash → sh';
    case 'system':
      return 'System: $SHELL';
    case 'custom':
      return defaultShellRequest.value ? `Custom: ${defaultShellRequest.value}` : 'Custom: unset';
    default:
      return defaultShellMode.value;
  }
});

const sessionMap = computed(() => new Map(sessions.value.map((session) => [session.id, session] as const)));

const sessionColumns = [
  { title: 'Session', dataIndex: 'session', key: 'session' },
  { title: 'PID', dataIndex: 'pid', key: 'pid' },
  { title: 'Status', dataIndex: 'status', key: 'status' },
  { title: 'Updated', dataIndex: 'updatedAt', key: 'updatedAt' },
  { title: 'Actions', dataIndex: 'actions', key: 'actions' },
];

const openSessions = computed(() =>
  openSessionIds.value
    .map((id) => sessionMap.value.get(id))
    .filter((session): session is ShellSessionInfo => Boolean(session)),
);

const runningSessionCount = computed(() => sessions.value.filter((session) => session.status === 'running').length);

const formatDateTime = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
};

const shellStatusColor = (status: string) => {
  switch (status) {
    case 'running':
      return 'success';
    case 'exited':
      return 'warning';
    case 'closed':
      return 'default';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
};

const attachedColor = (attached: boolean) => (attached ? 'success' : 'default');

const isSessionOpen = (sessionId: string) => openSessionIds.value.includes(sessionId);

const syncOpenTabs = () => {
  const availableIds = new Set(sessions.value.map((session) => session.id));
  openSessionIds.value = openSessionIds.value.filter((id) => availableIds.has(id));
  if (!openSessionIds.value.includes(activeTabKey.value)) {
    activeTabKey.value = openSessionIds.value[0] || '';
  }
};

const upsertSession = (session: ShellSessionInfo) => {
  const index = sessions.value.findIndex((item) => item.id === session.id);
  if (index >= 0) {
    sessions.value = sessions.value.map((item) => (item.id === session.id ? session : item));
  } else {
    sessions.value = [session, ...sessions.value];
  }
  syncOpenTabs();
};

const removeSessionLocally = (sessionId: string) => {
  sessions.value = sessions.value.filter((session) => session.id !== sessionId);
  openSessionIds.value = openSessionIds.value.filter((id) => id !== sessionId);
  if (activeTabKey.value === sessionId) {
    activeTabKey.value = openSessionIds.value[0] || '';
  }
};

const refreshSessions = async () => {
  if (sessionsLoading.value) return;

  sessionsLoading.value = true;
  sessionError.value = '';
  try {
    const res = await axios.get('/shell-sessions');
    sessions.value = Array.isArray(res.data) ? (res.data as ShellSessionInfo[]) : [];
    syncOpenTabs();
  } catch (err: any) {
    sessionError.value = err?.response?.data?.error || err?.message || 'Failed to load shell sessions';
  } finally {
    sessionsLoading.value = false;
  }
};

const openSession = (sessionId: string) => {
  if (!sessionId) return;
  if (!openSessionIds.value.includes(sessionId)) {
    openSessionIds.value = [...openSessionIds.value, sessionId];
  }
  activeTabKey.value = sessionId;
};

const focusOrOpenSession = (session: ShellSessionInfo) => {
  if (isSessionOpen(session.id)) {
    activeTabKey.value = session.id;
    return;
  }

  if (session.attached) {
    message.warning(`Session #${session.id} is already attached elsewhere`);
    return;
  }

  if (session.status !== 'running') {
    message.warning(`Session #${session.id} is not running`);
    return;
  }

  openSession(session.id);
};

const detachSession = (sessionId: string) => {
  if (!isSessionOpen(sessionId)) return;
  openSessionIds.value = openSessionIds.value.filter((id) => id !== sessionId);
  if (activeTabKey.value === sessionId) {
    activeTabKey.value = openSessionIds.value[0] || '';
  }
};

const handleTabEdit = (targetKey: string | number | MouseEvent, action: 'add' | 'remove') => {
  if (action !== 'remove') return;
  detachSession(String(targetKey));
};

const closeBackendSession = async (sessionId: string) => {
  try {
    detachSession(sessionId);
    await axios.delete(`/shell-sessions/${sessionId}`);
    removeSessionLocally(sessionId);
    message.success(`Closed session #${sessionId}`);
    await refreshSessions();
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to close session');
    await refreshSessions();
  }
};

const createSession = async () => {
  if (!canCreateSession.value) {
    message.error('Please provide a custom shell path');
    return;
  }

  creating.value = true;
  try {
    const payload: ShellSessionCreateRequest = {
      shell: defaultShellRequest.value || 'auto',
      cols: 100,
      rows: 32,
    };
    const res = await axios.post('/shell-sessions', payload);
    const session = res.data as ShellSessionInfo;
    upsertSession(session);
    openSession(session.id);
    message.success(`Created shell session #${session.id}`);
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to create session');
  } finally {
    creating.value = false;
  }
};

const refreshNow = () => {
  void refreshSessions();
};

const sessionLabel = (session: ShellSessionInfo) => session.label || session.shell || 'auto';
const tabLabel = (session: ShellSessionInfo) => `#${session.id} · ${sessionLabel(session)}`;

defineExpose({
  upsertSession,
  openSession,
  refreshSessions,
});

onMounted(() => {
  void refreshSessions();
  refreshTimer = window.setInterval(() => {
    void refreshSessions();
  }, 4000);
});

onBeforeUnmount(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer);
    refreshTimer = null;
  }
});
</script>

<template>
  <div class="shell-manager">
    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xxl="10">
        <a-card title="Terminal Session Manager" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="blue">{{ openSessions.length }} open</a-tag>
              <a-button size="small" :loading="sessionsLoading" @click="refreshNow">
                Refresh
              </a-button>
            </a-space>
          </template>

          <a-alert
            type="info"
            show-icon
            style="margin-bottom: 16px"
            message="Detach vs. close"
            description="Closing a tab only detaches the frontend. The backend shell keeps running until you click Close backend."
          />

          <a-form layout="vertical">
            <a-row :gutter="12">
              <a-col :span="14">
                <a-form-item label="Default shell">
                  <a-select
                    v-model:value="defaultShellMode"
                    :options="shellModeOptions"
                    style="width: 100%"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="10">
                <a-form-item label="Create">
                  <a-button
                    type="primary"
                    :loading="creating"
                    :disabled="!canCreateSession"
                    block
                    @click="createSession"
                  >
                    New Session
                  </a-button>
                </a-form-item>
              </a-col>
            </a-row>

            <a-form-item v-if="defaultShellMode === 'custom'" label="Custom shell path">
              <a-input
                v-model:value="defaultCustomShellPath"
                placeholder="/usr/bin/fish"
                allow-clear
              />
            </a-form-item>

            <a-alert
              v-if="defaultShellMode === 'custom' && !defaultShellRequest"
              type="warning"
              show-icon
              message="Custom shell path is required"
              style="margin-bottom: 12px"
            />

            <div class="shell-manager__summary">
              <a-tag color="purple">{{ shellSelectionLabel }}</a-tag>
              <span class="shell-manager__summary-text">
                New sessions will be created with the selected shell and then attached in a tab.
              </span>
            </div>
          </a-form>

          <a-divider />

          <a-alert
            v-if="sessionError"
            type="warning"
            show-icon
            :message="sessionError"
            style="margin-bottom: 12px"
          />

          <a-table
            :data-source="sessions"
            :columns="sessionColumns"
            :loading="sessionsLoading"
            :pagination="false"
            size="small"
            row-key="id"
            :scroll="{ x: 900 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'session'">
                <div class="shell-manager__session-cell" :title="`${sessionLabel(record)} → ${record.shellPath || 'unresolved'}\n${record.workDir}`">
                  <div class="shell-manager__session-title">#{{ record.id }}</div>
                  <div class="shell-manager__session-subtitle shell-manager__session-subtitle--ellipsis">
                    {{ sessionLabel(record) }} → {{ record.shellPath || 'unresolved' }}
                  </div>
                  <div class="shell-manager__session-subtitle shell-manager__session-subtitle--ellipsis">
                    {{ record.workDir }}
                  </div>
                </div>
              </template>

              <template v-else-if="column.dataIndex === 'pid'">
                <code>{{ record.pid }}</code>
              </template>

              <template v-else-if="column.dataIndex === 'status'">
                <a-space wrap :size="6">
                  <a-tag :color="shellStatusColor(record.status)">{{ record.status }}</a-tag>
                  <a-tag :color="attachedColor(record.attached)">
                    {{ record.attached ? 'attached' : 'detached' }}
                  </a-tag>
                </a-space>
              </template>

              <template v-else-if="column.dataIndex === 'updatedAt'">
                {{ formatDateTime(record.updatedAt) }}
              </template>

              <template v-else-if="column.dataIndex === 'actions'">
                <a-space wrap :size="8">
                  <a-button
                    size="small"
                    type="primary"
                    :disabled="!isSessionOpen(record.id) && (record.status !== 'running' || record.attached)"
                    @click="focusOrOpenSession(record)"
                  >
                    {{ isSessionOpen(record.id) ? 'Focus' : record.attached ? 'Busy' : 'Attach' }}
                  </a-button>
                  <a-button
                    size="small"
                    :disabled="!isSessionOpen(record.id)"
                    @click="detachSession(record.id)"
                  >
                    Detach
                  </a-button>
                  <a-button size="small" danger @click="closeBackendSession(record.id)">
                    Close
                  </a-button>
                </a-space>
              </template>
            </template>

            <template #emptyText>
              <a-empty description="No backend shell sessions yet" />
            </template>

          </a-table>
        </a-card>
      </a-col>

      <a-col :xs="24" :xxl="14">
        <a-card title="Active Terminal Tabs" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="green">{{ openSessions.length }} active</a-tag>
              <a-tag color="blue">{{ runningSessionCount }} running</a-tag>
            </a-space>
          </template>

          <template v-if="openSessions.length > 0">
            <a-tabs
              v-model:activeKey="activeTabKey"
              type="editable-card"
              :hideAdd="true"
              :destroyInactiveTabPane="false"
              @edit="handleTabEdit"
            >
              <a-tab-pane
                v-for="session in openSessions"
                :key="session.id"
                :tab="tabLabel(session)"
                :closable="true"
              >
                <ShellTerminalPane
                  :session="session"
                  :active="activeTabKey === session.id"
                  @detach="detachSession(session.id)"
                  @close-session="closeBackendSession(session.id)"
                />
              </a-tab-pane>
            </a-tabs>
          </template>
          <template v-else>
            <a-empty
              description="No attached terminal tabs yet. Use Attach from the table or create a new session."
            />
          </template>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.shell-manager {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.shell-manager__summary {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.shell-manager__summary-text {
  color: #666;
  font-size: 13px;
}

.shell-manager__session-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.shell-manager__session-title {
  font-weight: 600;
  line-height: 1.4;
}

.shell-manager__session-subtitle {
  color: #666;
  font-size: 12px;
  line-height: 1.4;
}

.shell-manager__session-subtitle--ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
