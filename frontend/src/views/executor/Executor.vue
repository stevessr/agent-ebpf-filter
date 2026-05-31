<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import axios from 'axios';
import { PlayCircleOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import RemoteWrapperTerminal from '../../components/terminal/RemoteWrapperTerminal.vue';
import LocalShellTerminal from '../../components/terminal/LocalShellTerminal.vue';
import PathNavigatorDrawer from '../../components/explorer/PathNavigatorDrawer.vue';
import ExecutorLaunchEnvTab from '../../components/executor/ExecutorLaunchEnvTab.vue';
import type { ShellSessionCreateRequest, ShellSessionInfo } from '../../types/shell';
import { useLaunchEnv } from '../../composables/executor/useLaunchEnv';
import { useCodingLauncher, CODING_PRESET_OPTIONS } from '../../composables/executor/useCodingLauncher';
import { useScriptLauncher, SCRIPT_LANGUAGE_OPTIONS } from '../../composables/executor/useScriptLauncher';
import { dirname, resolvePythonInterpreter } from '../../composables/executor/useLauncherUtils';

type ExecutorTabKey = 'shell' | 'remote' | 'tmux' | 'scripts';
type ScriptTabKey = 'runner' | 'launch-env';
type PathPickerTarget = 'coding-workdir' | 'script-path' | 'script-workdir' | 'python-venv';

type LocalShellManagerExpose = {
  upsertSession: (session: ShellSessionInfo) => void;
  openSession: (sessionId: string) => void;
  refreshSessions: () => Promise<void>;
};

const shellManagerRef = ref<LocalShellManagerExpose | null>(null);
const tmuxManagerRef = ref<LocalShellManagerExpose | null>(null);

const route = useRoute();
const router = useRouter();

const normalizeMainTab = (tab: unknown): ExecutorTabKey => {
  if (tab === 'remote' || tab === 'tmux' || tab === 'scripts') return tab;
  if (tab === 'launch-env') return 'scripts';
  return 'shell';
};

const normalizeScriptTab = (tab: unknown): ScriptTabKey => (
  tab === 'launch-env' ? 'launch-env' : 'runner'
);

const activeTabKey = ref<ExecutorTabKey>(normalizeMainTab(route.params.tab));
const scriptTabKey = ref<ScriptTabKey>(
  route.params.tab === 'launch-env' ? 'launch-env' : normalizeScriptTab(route.params.subtab),
);

watch(
  () => [route.params.tab, route.params.subtab],
  ([tab, subtab]) => {
    if (tab === 'launch-env') {
      activeTabKey.value = 'scripts';
      scriptTabKey.value = 'launch-env';
      return;
    }
    activeTabKey.value = normalizeMainTab(tab);
    if (activeTabKey.value === 'scripts') {
      scriptTabKey.value = normalizeScriptTab(subtab);
    }
  },
);

watch(activeTabKey, (val) => {
  if (val !== route.params.tab) {
    router.replace({
      name: 'Executor',
      params: { tab: val, subtab: val === 'scripts' && scriptTabKey.value !== 'runner' ? scriptTabKey.value : undefined },
    });
  }
});

watch(scriptTabKey, (val) => {
  if (activeTabKey.value !== 'scripts') return;
  const subtab = val === 'runner' ? undefined : val;
  if (route.params.tab !== 'scripts' || route.params.subtab !== subtab) {
    router.replace({ name: 'Executor', params: { tab: 'scripts', subtab } });
  }
});

const isTmuxSession = (session: ShellSessionInfo) => {
  const kind = (session.kind || '').trim().toLowerCase();
  if (kind === 'tmux') return true;
  return (session.shell || '').trim().toLowerCase() === 'tmux' || (session.command || '').trim().toLowerCase() === 'tmux';
};

const routeSessionToManager = (session: ShellSessionInfo) => {
  if (isTmuxSession(session)) {
    tmuxManagerRef.value?.upsertSession(session);
    return 'tmux' as ExecutorTabKey;
  }
  shellManagerRef.value?.upsertSession(session);
  return 'shell' as ExecutorTabKey;
};

const focusSessionInManager = (session: ShellSessionInfo, manager?: ExecutorTabKey) => {
  const targetManager = manager || routeSessionToManager(session);
  if (manager) {
    if (manager === 'tmux') tmuxManagerRef.value?.upsertSession(session);
    else shellManagerRef.value?.upsertSession(session);
  }
  if (targetManager === 'tmux') tmuxManagerRef.value?.openSession(session.id);
  else shellManagerRef.value?.openSession(session.id);
  activeTabKey.value = targetManager;
};

const createShellSession = async (
  payload: ShellSessionCreateRequest,
  successMessage: string,
  manager: ExecutorTabKey = 'shell',
) => {
  const res = await axios.post('/shell-sessions', payload);
  const session = res.data as ShellSessionInfo;
  focusSessionInManager(session, manager);
  message.success(successMessage);
  return session;
};

const launchEnv = useLaunchEnv();
const { launchEnvRecord, launchEnvPreview } = launchEnv;

const {
  codingPreset, codingCustomCommand, codingExtraArgs,
  codingSessionName, codingWorkDir, codingUseTmux,
  codingLaunching, codingCommandPreview, launchCodingCli,
} = useCodingLauncher(createShellSession, () => launchEnvRecord.value);

const {
  scriptLanguage, scriptPath, scriptWorkDir, pythonVenv,
  scriptArgs, scriptLaunching, scriptArgsPlaceholder,
  scriptCommandPreview, launchScript,
} = useScriptLauncher(
  (payload, msg) => createShellSession(payload, msg, 'shell'),
  () => launchEnvRecord.value,
);

// Path picker
const pathPickerOpen = ref(false);
const pathPickerTarget = ref<PathPickerTarget>('coding-workdir');

const getPathPickerInitialPath = computed(() => {
  switch (pathPickerTarget.value) {
    case 'coding-workdir': return codingWorkDir.value.trim() || '/';
    case 'script-path': return scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    case 'script-workdir': return scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    case 'python-venv': return pythonVenv.value.trim() || scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    default: return '/';
  }
});

const getPathPickerTitle = computed(() => {
  switch (pathPickerTarget.value) {
    case 'coding-workdir': return 'Pick coding CLI workdir';
    case 'script-path': return 'Pick script file';
    case 'script-workdir': return 'Pick script workdir';
    case 'python-venv': return 'Pick Python venv directory';
    default: return 'Pick path';
  }
});

const getPathPickerMode = computed(() => pathPickerTarget.value === 'script-path' ? 'file' : 'directory');

const setPathPickerTarget = (target: PathPickerTarget) => {
  pathPickerTarget.value = target;
  pathPickerOpen.value = true;
};

const applyPickedPath = (path: string) => {
  const normalized = path.trim();
  if (!normalized) return;
  switch (pathPickerTarget.value) {
    case 'coding-workdir': codingWorkDir.value = normalized; break;
    case 'script-path':
      scriptPath.value = normalized;
      if (!scriptWorkDir.value.trim()) scriptWorkDir.value = dirname(normalized);
      break;
    case 'script-workdir': scriptWorkDir.value = normalized; break;
    case 'python-venv': pythonVenv.value = normalized; break;
  }
};
</script>

<template>
  <div style="padding: 24px; background: #f0f2f5; min-height: 100%;">
    <a-tabs v-model:activeKey="activeTabKey" type="card" size="large" :destroyInactiveTabPane="false">
      <a-tab-pane key="shell" tab="Shell Manager">
        <a-space direction="vertical" :size="16" style="width: 100%;">
          <a-card title="Interactive Shell Manager (wterm)" :bordered="false">
            <template #extra>
              <a-tag color="blue">multi-session PTY</a-tag>
            </template>
            <LocalShellTerminal
              ref="shellManagerRef"
              session-kind-filter="non-tmux"
              :default-env="launchEnvRecord"
            />
          </a-card>
        </a-space>
      </a-tab-pane>

      <a-tab-pane key="remote" tab="Remote Executor">
        <RemoteWrapperTerminal
          :active="activeTabKey === 'remote'"
          :default-env="launchEnvRecord"
        />
      </a-tab-pane>

      <a-tab-pane key="tmux" tab="Tmux">
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :xl="10">
            <a-card title="Launch coding CLI in tmux" :bordered="false">
              <a-alert type="info" show-icon style="margin-bottom: 16px;"
                message="This launcher starts the coding CLI inside tmux by default."
                description="The launched session appears only in the Tmux tab so shell and tmux management stay separate."
              />
              <a-form layout="vertical">
                <a-form-item label="CLI preset">
                  <a-select v-model:value="codingPreset" :options="CODING_PRESET_OPTIONS" />
                </a-form-item>
                <a-form-item v-if="codingPreset === 'custom'" label="Custom CLI command">
                  <a-input v-model:value="codingCustomCommand" placeholder="codex, claude, gemini, or any executable in PATH" />
                </a-form-item>
                <a-form-item label="Extra args">
                  <a-input v-model:value="codingExtraArgs" placeholder="e.g. --model gpt-5.5 --help" />
                </a-form-item>
                <a-form-item label="tmux session name">
                  <a-input v-model:value="codingSessionName" placeholder="coding-cli" />
                </a-form-item>
                <a-form-item label="Workdir">
                  <a-input-search v-model:value="codingWorkDir" placeholder="defaults to backend workdir if empty" enter-button="Browse" @search="setPathPickerTarget('coding-workdir')" />
                </a-form-item>
                <a-form-item label="Launch mode">
                  <a-space>
                    <a-switch v-model:checked="codingUseTmux" />
                    <span>{{ codingUseTmux ? 'tmux wrapper' : 'direct command' }}</span>
                  </a-space>
                </a-form-item>
                <a-alert type="success" show-icon style="margin-bottom: 16px;" :message="codingCommandPreview" />
                <a-button type="primary" :loading="codingLaunching" block @click="launchCodingCli">
                  <template #icon><PlayCircleOutlined /></template>
                  Launch coding CLI
                </a-button>
                <a-alert type="info" show-icon style="margin-top: 16px;" message="Tmux shortcut tools stay available in the session pane after you open the launched session." />
              </a-form>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="14">
            <a-card title="Tmux Workbench" :bordered="false">
              <LocalShellTerminal
                ref="tmuxManagerRef"
                manager-title="Tmux Session Manager"
                session-kind-filter="tmux"
                :show-create-panel="false"
                :show-tmux-quick-actions="true"
                :default-env="launchEnvRecord"
              />
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <a-tab-pane key="scripts" tab="Script Runner">
        <a-tabs v-model:activeKey="scriptTabKey" size="small" :destroyInactiveTabPane="false">
          <a-tab-pane key="runner" tab="Run Script">
            <a-row :gutter="[16, 16]">
              <a-col :xs="24" :xl="10">
                <a-card title="Launch script" :bordered="false">
                  <a-alert type="info" show-icon style="margin-bottom: 16px;"
                    message="This launcher starts Python, Node.js, Ruby, sh, pwsh, Deno, or Bun scripts in a dedicated backend shell session."
                    description="System environment is the default. For Python, you can optionally point at a venv directory and the launcher will use its interpreter."
                  />
                  <a-form layout="vertical">
                    <a-form-item label="Language">
                      <a-select v-model:value="scriptLanguage" :options="SCRIPT_LANGUAGE_OPTIONS" />
                    </a-form-item>
                    <a-form-item label="Script file">
                      <a-input-search v-model:value="scriptPath" placeholder="/path/to/script.py, app.js, script.rb, or script.ps1" enter-button="Browse" @search="setPathPickerTarget('script-path')" />
                    </a-form-item>
                    <a-form-item label="Workdir">
                      <a-input-search v-model:value="scriptWorkDir" placeholder="defaults to script parent directory" enter-button="Browse" @search="setPathPickerTarget('script-workdir')" />
                    </a-form-item>
                    <a-form-item v-if="scriptLanguage === 'python'" label="Python venv directory">
                      <a-input-search v-model:value="pythonVenv" placeholder="Leave empty for system Python" enter-button="Browse" @search="setPathPickerTarget('python-venv')" />
                    </a-form-item>
                    <a-form-item label="Script args">
                      <a-input v-model:value="scriptArgs" :placeholder="scriptArgsPlaceholder" />
                    </a-form-item>
                    <a-alert v-if="scriptLanguage === 'deno'" type="info" show-icon style="margin-bottom: 16px;" message="For Deno, put runtime flags before `--`, then script arguments after `--`." />
                    <a-alert type="success" show-icon style="margin-bottom: 16px;" :message="scriptCommandPreview" />
                    <a-button type="primary" :loading="scriptLaunching" block @click="launchScript">
                      <template #icon><PlayCircleOutlined /></template>
                      Launch script
                    </a-button>
                  </a-form>
                </a-card>
              </a-col>
              <a-col :xs="24" :xl="14">
                <a-card title="Runtime notes" :bordered="false">
                  <a-space direction="vertical" :size="12" style="width: 100%;">
                    <a-descriptions bordered size="small" :column="1">
                      <a-descriptions-item label="Default environment"><span>System + launch env overrides</span></a-descriptions-item>
                      <a-descriptions-item label="Python interpreter"><span>{{ resolvePythonInterpreter(pythonVenv) }}</span></a-descriptions-item>
                      <a-descriptions-item label="Current launch"><span>{{ scriptCommandPreview }}</span></a-descriptions-item>
                      <a-descriptions-item label="Workdir fallback"><span>{{ scriptWorkDir.trim() || (scriptPath.trim() ? dirname(scriptPath) : 'script parent') }}</span></a-descriptions-item>
                      <a-descriptions-item label="Launch env"><span>{{ launchEnvPreview }}</span></a-descriptions-item>
                    </a-descriptions>
                    <a-alert type="warning" show-icon message="Browse to the script file first, then optionally set a venv for Python." />
                    <a-alert type="info" show-icon message="The launched script session will show up in the Shell Manager tab for detach/reattach." />
                  </a-space>
                </a-card>
              </a-col>
            </a-row>
          </a-tab-pane>

          <a-tab-pane key="launch-env" tab="Launch Env">
            <ExecutorLaunchEnvTab :launch-env="launchEnv" />
          </a-tab-pane>
        </a-tabs>
      </a-tab-pane>
    </a-tabs>

    <PathNavigatorDrawer
      v-model:open="pathPickerOpen"
      :title="getPathPickerTitle"
      :initial-path="getPathPickerInitialPath"
      :pick-mode="getPathPickerMode"
      @confirm="applyPickedPath"
    />
  </div>
</template>

<style scoped>
.executor-env__value {
  display: inline-block;
  max-width: 100%;
  word-break: break-all;
  white-space: normal;
  color: #333;
}
</style>
