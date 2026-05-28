<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue';
import ShellTerminalPane from './ShellTerminalPane.vue';
import type { ShellSessionInfo } from '../../types/shell';
import { isTmuxSession } from '../../utils/tmux';
import { useShellSessions, SHELL_MODE_OPTIONS } from '../../composables/executor/useShellSessions';

const props = withDefaults(defineProps<{
  managerTitle?: string;
  sessionKindFilter?: 'all' | 'tmux' | 'non-tmux';
  showCreatePanel?: boolean;
  showTmuxQuickActions?: boolean;
  defaultEnv?: Record<string, string>;
}>(), {
  managerTitle: 'Terminal Session Manager',
  sessionKindFilter: 'all',
  showCreatePanel: true,
  showTmuxQuickActions: false,
});

const {
  defaultShellMode, defaultCustomShellPath,
  sessionsLoading, sessionError, creating,
  activeTabKey, wsConnected,
  isTmuxFilteredView, isNonTmuxFilteredView,
  filteredSessions, tmuxQuickShortcuts,
  defaultShellRequest, canCreateSession, shellSelectionLabel,
  sessionMap, filteredSessionMap, sessionColumns,
  openSessions, runningSessionCount,
  formatDateTime, shellStatusColor, attachedColor, isSessionOpen,
  upsertSession, refreshSessions, openSession, focusOrOpenSession,
  detachSession, handleTabEdit, closeBackendSession,
  sendTmuxShortcut, createSession, cleanupSessions,
  connectShellSessionsWS, disconnectShellSessionsWS,
  sessionLabel, tabLabel,
} = useShellSessions(props.sessionKindFilter);

const refreshNow = () => { void refreshSessions(); };

const doCreateSession = () => { void createSession(props.defaultEnv); };

defineExpose({ upsertSession, openSession, refreshSessions });

onMounted(() => { connectShellSessionsWS(); });
onBeforeUnmount(() => { disconnectShellSessionsWS(); });
</script>

<template>
  <div class="shell-manager">
    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xxl="10">
        <a-card :title="managerTitle" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="blue">{{ filteredSessions.length }} listed</a-tag>
              <a-tag color="green">{{ openSessions.length }} open</a-tag>
              <a-button size="small" @click="cleanupSessions">Cleanup Sessions</a-button>
              <a-button size="small" :loading="sessionsLoading" @click="refreshNow">Refresh</a-button>
            </a-space>
          </template>

          <a-alert type="info" show-icon style="margin-bottom: 16px" message="Detach vs. close" description="Closing a tab only detaches the frontend. The backend shell keeps running until you click Close backend." />

          <template v-if="showCreatePanel">
            <a-form layout="vertical">
              <a-row :gutter="12">
                <a-col :span="14">
                  <a-form-item label="Default shell">
                    <a-select v-model:value="defaultShellMode" :options="SHELL_MODE_OPTIONS" style="width: 100%" />
                  </a-form-item>
                </a-col>
                <a-col :span="10">
                  <a-form-item label="Create">
                    <a-button type="primary" :loading="creating" :disabled="!canCreateSession" block @click="doCreateSession">New Session</a-button>
                  </a-form-item>
                </a-col>
              </a-row>
              <a-form-item v-if="defaultShellMode === 'custom'" label="Custom shell path">
                <a-input v-model:value="defaultCustomShellPath" placeholder="/usr/bin/fish" allow-clear />
              </a-form-item>
              <a-alert v-if="defaultShellMode === 'custom' && !defaultShellRequest" type="warning" show-icon message="Custom shell path is required" style="margin-bottom: 12px" />
              <div class="shell-manager__summary">
                <a-tag color="purple">{{ shellSelectionLabel }}</a-tag>
                <span class="shell-manager__summary-text">New sessions will be created with the selected shell and then attached in a tab.</span>
              </div>
            </a-form>
          </template>
          <template v-else>
            <a-alert v-if="isTmuxFilteredView" type="info" show-icon message="Tmux session view" description="Use the tmux launcher on the left to create a coding CLI. Open a session here to use tmux quick shortcuts." />
            <a-alert v-else-if="isNonTmuxFilteredView" type="info" show-icon message="Shell session view" description="This area lists interactive shell and script sessions only. Tmux sessions are managed separately in the Tmux tab." />
          </template>

          <a-divider v-if="showCreatePanel || isTmuxFilteredView || isNonTmuxFilteredView" />

          <a-alert v-if="sessionError" type="warning" show-icon :message="sessionError" style="margin-bottom: 12px" />

          <a-table
            :data-source="filteredSessions" :columns="sessionColumns" :loading="sessionsLoading"
            :pagination="false" size="small" row-key="id" :scroll="{ x: 1100 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'session'">
                <div class="shell-manager__session-cell" :title="`${sessionLabel(record)} → ${record.shellPath || 'unresolved'}\n${record.workDir}`">
                  <div class="shell-manager__session-title">#{{ record.id }}</div>
                  <div class="shell-manager__session-badges">
                    <a-tag v-if="isTmuxSession(record)" color="purple">tmux</a-tag>
                    <a-tag v-else-if="record.kind && record.kind !== 'shell'" color="blue">{{ record.kind }}</a-tag>
                  </div>
                  <div class="shell-manager__session-subtitle shell-manager__session-subtitle--ellipsis">{{ sessionLabel(record) }} → {{ record.shellPath || 'unresolved' }}</div>
                  <div class="shell-manager__session-subtitle shell-manager__session-subtitle--ellipsis">{{ record.workDir }}</div>
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'pid'"><code>{{ record.pid }}</code></template>
              <template v-else-if="column.dataIndex === 'status'">
                <a-space wrap :size="6">
                  <a-tag :color="shellStatusColor(record.status)">{{ record.status }}</a-tag>
                  <a-tag :color="attachedColor(record.attached)">{{ record.attached ? 'attached' : 'detached' }}</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.dataIndex === 'updatedAt'">{{ formatDateTime(record.updatedAt) }}</template>
              <template v-else-if="column.dataIndex === 'actions'">
                <div class="shell-manager__actions">
                  <a-space wrap :size="8">
                    <a-button size="small" type="primary" :disabled="!isSessionOpen(record.id) && (record.status !== 'running' || record.attached)" @click="focusOrOpenSession(record)">{{ isSessionOpen(record.id) ? 'Focus' : record.attached ? 'Busy' : 'Attach' }}</a-button>
                    <a-button size="small" :disabled="!isSessionOpen(record.id)" @click="detachSession(record.id)">Detach</a-button>
                    <a-button size="small" danger @click="closeBackendSession(record.id)">Close</a-button>
                  </a-space>
                  <a-space v-if="showTmuxQuickActions && isTmuxSession(record)" class="shell-manager__tmux-tools" wrap :size="6">
                    <a-tag color="purple">tmux</a-tag>
                    <a-button v-for="shortcut in tmuxQuickShortcuts" :key="shortcut.key" size="small" :disabled="record.status !== 'running'" @click="sendTmuxShortcut(record.id, shortcut.sequence, shortcut.label)">{{ shortcut.label }}</a-button>
                  </a-space>
                </div>
              </template>
            </template>
            <template #emptyText>
              <a-empty :description="isTmuxFilteredView ? 'No tmux sessions yet' : isNonTmuxFilteredView ? 'No shell sessions yet' : 'No backend shell sessions yet'" />
            </template>
          </a-table>
        </a-card>
      </a-col>

      <a-col :xs="24" :xxl="14">
        <a-card :title="isTmuxFilteredView ? 'Active Tmux Tabs' : 'Active Terminal Tabs'" :bordered="false">
          <template #extra>
            <a-space :size="8">
              <a-tag color="green">{{ openSessions.length }} active</a-tag>
              <a-tag color="blue">{{ runningSessionCount }} running</a-tag>
            </a-space>
          </template>
          <template v-if="openSessions.length > 0">
            <a-tabs v-model:activeKey="activeTabKey" type="editable-card" :hideAdd="true" :destroyInactiveTabPane="false" @edit="handleTabEdit">
              <a-tab-pane v-for="session in openSessions" :key="session.id" :tab="tabLabel(session)" :closable="true">
                <ShellTerminalPane :session="session" :active="activeTabKey === session.id" @detach="detachSession(session.id)" @close-session="closeBackendSession(session.id)" />
              </a-tab-pane>
            </a-tabs>
          </template>
          <template v-else>
            <a-empty description="No attached terminal tabs yet. Use Attach from the table or create a new session." />
          </template>
        </a-card>
      </a-col>
    </a-row>
  </div>
</template>

<style scoped>
.shell-manager { display: flex; flex-direction: column; gap: 16px; }
.shell-manager__summary { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.shell-manager__summary-text { color: #666; font-size: 13px; }
.shell-manager__session-cell { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.shell-manager__session-title { font-weight: 600; line-height: 1.4; }
.shell-manager__session-badges { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.shell-manager__session-subtitle { color: #666; font-size: 12px; line-height: 1.4; }
.shell-manager__session-subtitle--ellipsis { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.shell-manager__actions { display: flex; flex-direction: column; gap: 8px; }
.shell-manager__tmux-tools { align-items: center; }
</style>
