<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import axios from 'axios';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SettingOutlined,
} from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import RemoteWrapperTerminal from '../components/RemoteWrapperTerminal.vue';
import LocalShellTerminal from '../components/LocalShellTerminal.vue';
import PathNavigatorDrawer from '../components/PathNavigatorDrawer.vue';
import type { ShellSessionCreateRequest, ShellSessionInfo } from '../types/shell';

type ExecutorTabKey = 'shell' | 'remote' | 'tmux' | 'scripts' | 'launch-env';
type PathPickerTarget = 'coding-workdir' | 'script-path' | 'script-workdir' | 'python-venv';

type LocalShellManagerExpose = {
  upsertSession: (session: ShellSessionInfo) => void;
  openSession: (sessionId: string) => void;
  refreshSessions: () => Promise<void>;
};

type CodingPresetKey = 'codex' | 'claude' | 'gemini' | 'custom';
type ScriptLanguage = 'python' | 'node' | 'ruby' | 'sh' | 'pwsh' | 'deno' | 'bun';
type ScriptLaunchPlan = {
  command: string;
  args: string[];
  preview: string;
};
type LaunchEnvEntry = {
  id: string;
  key: string;
  value: string;
  enabled: boolean;
};
type LaunchEnvProfile = {
  id: string;
  name: string;
  entries: LaunchEnvEntry[];
};
type DetectedLaunchEnvEntry = {
  key: string;
  value: string;
};

const LAUNCH_ENV_STORAGE_KEY = 'executor_launch_env_v2';
const LAUNCH_ENV_LEGACY_KEY = 'executor_launch_env';

const shellManagerRef = ref<LocalShellManagerExpose | null>(null);
const tmuxManagerRef = ref<LocalShellManagerExpose | null>(null);
const activeTabKey = ref<ExecutorTabKey>('shell');

const codingPreset = ref<CodingPresetKey>('codex');
const codingCustomCommand = ref('');
const codingExtraArgs = ref('');
const codingSessionName = ref('coding');
const codingWorkDir = ref('');
const codingUseTmux = ref(true);
const codingLaunching = ref(false);

const scriptLanguage = ref<ScriptLanguage>('python');
const scriptPath = ref('');
const scriptWorkDir = ref('');
const pythonVenv = ref('');
const scriptArgs = ref('');
const scriptLaunching = ref(false);

const pathPickerOpen = ref(false);
const pathPickerTarget = ref<PathPickerTarget>('coding-workdir');

const profiles = ref<LaunchEnvProfile[]>(loadProfiles());
const activeProfileId = ref<string>(localStorage.getItem('executor_active_profile_id') || profiles.value[0]?.id || '');
const activeProfile = computed(() => profiles.value.find(p => p.id === activeProfileId.value) || profiles.value[0]);
const launchEnvEntries = computed({
  get: () => activeProfile.value?.entries || [],
  set: (val) => {
    const p = profiles.value.find(p => p.id === activeProfileId.value);
    if (p) {
      p.entries = val;
    }
  }
});

const newLaunchEnvKey = ref('');
const newLaunchEnvValue = ref('');
const detectedLaunchEnvEntries = ref<DetectedLaunchEnvEntry[]>([]);
const detectedLaunchEnvSearch = ref('');
const detectedLaunchEnvLoading = ref(false);
const detectedLaunchEnvError = ref('');

const profileRenameModalOpen = ref(false);
const profileRenameValue = ref('');
const profileRenameId = ref('');

function loadProfiles(): LaunchEnvProfile[] {
  try {
    const raw = JSON.parse(localStorage.getItem(LAUNCH_ENV_STORAGE_KEY) || 'null') as unknown;
    if (Array.isArray(raw) && raw.length > 0) {
      return raw as LaunchEnvProfile[];
    }

    // Try migrating legacy data
    const legacy = JSON.parse(localStorage.getItem(LAUNCH_ENV_LEGACY_KEY) || '[]') as LaunchEnvEntry[];
    if (Array.isArray(legacy) && legacy.length > 0) {
      return [{
        id: 'default',
        name: 'Default Profile',
        entries: legacy
      }];
    }

    return [{
      id: 'default',
      name: 'Default Profile',
      entries: []
    }];
  } catch {
    return [{
      id: 'default',
      name: 'Default Profile',
      entries: []
    }];
  }
}

function persistProfiles() {
  localStorage.setItem(LAUNCH_ENV_STORAGE_KEY, JSON.stringify(profiles.value));
  localStorage.setItem('executor_active_profile_id', activeProfileId.value);
}

function addNewProfile() {
  const id = `profile-${Date.now()}`;
  profiles.value.push({
    id,
    name: `New Profile ${profiles.value.length + 1}`,
    entries: []
  });
  activeProfileId.value = id;
}

function copyProfile(profile: LaunchEnvProfile) {
  const id = `profile-${Date.now()}`;
  profiles.value.push({
    id,
    name: `${profile.name} (Copy)`,
    entries: JSON.parse(JSON.stringify(profile.entries)) as LaunchEnvEntry[]
  });
  activeProfileId.value = id;
}

function deleteProfile(id: string) {
  if (profiles.value.length <= 1) {
    message.warning('Cannot delete the last profile');
    return;
  }
  const index = profiles.value.findIndex(p => p.id === id);
  if (index >= 0) {
    profiles.value.splice(index, 1);
    if (activeProfileId.value === id) {
      activeProfileId.value = profiles.value[0].id;
    }
  }
}

function openRenameModal(profile: LaunchEnvProfile) {
  profileRenameId.value = profile.id;
  profileRenameValue.value = profile.name;
  profileRenameModalOpen.value = true;
}

function applyRename() {
  const p = profiles.value.find(p => p.id === profileRenameId.value);
  if (p && profileRenameValue.value.trim()) {
    p.name = profileRenameValue.value.trim();
  }
  profileRenameModalOpen.value = false;
}

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

const basename = (path: string) => {
  const normalized = path.trim().replace(/\/+$/, '');
  if (!normalized || normalized === '/') return '/';
  const index = normalized.lastIndexOf('/');
  return index >= 0 ? normalized.slice(index + 1) || normalized : normalized;
};

const dirname = (path: string) => {
  const normalized = path.trim().replace(/\/+$/, '');
  if (!normalized || normalized === '/') return '/';
  const index = normalized.lastIndexOf('/');
  if (index <= 0) return '/';
  return normalized.slice(0, index);
};

const sanitizeTmuxSessionName = (value: string) => {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^[-.]+|[-.]+$/g, '');
  return slug || 'coding-cli';
};

const getSelectedCodingCommand = () => {
  if (codingPreset.value === 'custom') {
    return codingCustomCommand.value.trim();
  }
  return codingPresetOptions.find((option) => option.value === codingPreset.value)?.command || '';
};

const resolvePythonInterpreter = (venvPath: string) => {
  const normalized = venvPath.trim().replace(/\/+$/, '');
  if (!normalized) return 'python3';
  if (
    normalized.endsWith('/python') ||
    normalized.endsWith('/python3') ||
    normalized.endsWith('/python.exe')
  ) {
    return normalized;
  }
  return `${normalized}/bin/python`;
};

const splitRuntimeAndScriptArgs = (input: string) => {
  const tokens = splitArgs(input);
  const separatorIndex = tokens.indexOf('--');
  if (separatorIndex < 0) {
    return {
      runtimeArgs: [] as string[],
      scriptArgs: tokens,
    };
  }
  return {
    runtimeArgs: tokens.slice(0, separatorIndex),
    scriptArgs: tokens.slice(separatorIndex + 1),
  };
};

const resolveScriptLaunchPlan = (
  language: ScriptLanguage,
  venvPath: string,
  scriptPath: string,
  rawArgs: string,
): ScriptLaunchPlan => {
  const script = scriptPath.trim();
  const scriptDisplay = script || '<script>';
  const tokens = splitArgs(rawArgs);

  switch (language) {
    case 'python':
      return {
        command: resolvePythonInterpreter(venvPath),
        args: [scriptDisplay, ...tokens],
        preview: [resolvePythonInterpreter(venvPath), scriptDisplay, ...tokens].join(' '),
      };
    case 'node':
      return {
        command: 'node',
        args: [scriptDisplay, ...tokens],
        preview: ['node', scriptDisplay, ...tokens].join(' '),
      };
    case 'ruby':
      return {
        command: 'ruby',
        args: [scriptDisplay, ...tokens],
        preview: ['ruby', scriptDisplay, ...tokens].join(' '),
      };
    case 'sh':
      return {
        command: 'sh',
        args: [scriptDisplay, ...tokens],
        preview: ['sh', scriptDisplay, ...tokens].join(' '),
      };
    case 'pwsh':
      {
        const { runtimeArgs, scriptArgs } = splitRuntimeAndScriptArgs(rawArgs);
        return {
          command: 'pwsh',
          args: [...runtimeArgs, '-File', scriptDisplay, ...scriptArgs],
          preview: ['pwsh', ...runtimeArgs, '-File', scriptDisplay, ...scriptArgs].join(' '),
        };
      }
    case 'deno': {
      const { runtimeArgs, scriptArgs } = splitRuntimeAndScriptArgs(rawArgs);
      return {
        command: 'deno',
        args: ['run', ...runtimeArgs, scriptDisplay, ...scriptArgs],
        preview: ['deno', 'run', ...runtimeArgs, scriptDisplay, ...scriptArgs].join(' '),
      };
    }
    case 'bun':
      return {
        command: 'bun',
        args: [scriptDisplay, ...tokens],
        preview: ['bun', scriptDisplay, ...tokens].join(' '),
      };
    default:
      return {
        command: resolvePythonInterpreter(venvPath),
        args: [scriptDisplay, ...tokens],
        preview: [resolvePythonInterpreter(venvPath), scriptDisplay, ...tokens].join(' '),
      };
  }
};

const scriptArgsPlaceholder = computed(() => {
  switch (scriptLanguage.value) {
    case 'deno':
      return '--allow-read -- --foo bar';
    case 'pwsh':
      return '-ExecutionPolicy Bypass -- --foo bar';
    case 'bun':
      return '--foo bar';
    default:
      return '--debug --foo bar';
  }
});

const getPathPickerInitialPath = computed(() => {
  switch (pathPickerTarget.value) {
    case 'coding-workdir':
      return codingWorkDir.value.trim() || '/';
    case 'script-path':
      return scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    case 'script-workdir':
      return scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    case 'python-venv':
      return pythonVenv.value.trim() || scriptWorkDir.value.trim() || dirname(scriptPath.value) || '/';
    default:
      return '/';
  }
});

const getPathPickerTitle = computed(() => {
  switch (pathPickerTarget.value) {
    case 'coding-workdir':
      return 'Pick coding CLI workdir';
    case 'script-path':
      return 'Pick script file';
    case 'script-workdir':
      return 'Pick script workdir';
    case 'python-venv':
      return 'Pick Python venv directory';
    default:
      return 'Pick path';
  }
});

const getPathPickerMode = computed(() => {
  switch (pathPickerTarget.value) {
    case 'script-path':
      return 'file';
    case 'coding-workdir':
    case 'script-workdir':
    case 'python-venv':
      return 'directory';
    default:
      return 'directory';
  }
});

const setPathPickerTarget = (target: PathPickerTarget) => {
  pathPickerTarget.value = target;
  pathPickerOpen.value = true;
};

const applyPickedPath = (path: string) => {
  const normalized = path.trim();
  if (!normalized) return;

  switch (pathPickerTarget.value) {
    case 'coding-workdir':
      codingWorkDir.value = normalized;
      break;
    case 'script-path':
      scriptPath.value = normalized;
      if (!scriptWorkDir.value.trim()) {
        scriptWorkDir.value = dirname(normalized);
      }
      break;
    case 'script-workdir':
      scriptWorkDir.value = normalized;
      break;
    case 'python-venv':
      pythonVenv.value = normalized;
      break;
  }
};

const isValidLaunchEnvKey = (key: string) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);

const launchEnvEntriesCount = computed(() => {
  return launchEnvEntries.value.filter(
    (entry) => entry.enabled && isValidLaunchEnvKey(entry.key.trim()),
  ).length;
});

const launchEnvEntryKeys = computed(() =>
  new Set(
    launchEnvEntries.value
      .map((entry) => entry.key.trim())
      .filter((key) => Boolean(key)),
  ),
);

const launchEnvRecord = computed<Record<string, string>>(() => {
  const env: Record<string, string> = {};
  for (const entry of launchEnvEntries.value) {
    const key = entry.key.trim();
    if (!entry.enabled || !key || !isValidLaunchEnvKey(key)) continue;
    env[key] = entry.value;
  }
  return env;
});

const launchEnvPreview = computed(() => {
  const entries = Object.entries(launchEnvRecord.value);
  if (!entries.length) {
    return 'No launch environment overrides configured';
  }
  return entries.map(([key, value]) => `${key}=${value || '""'}`).join('  ');
});

const launchEnvScope = computed(() => {
  const count = launchEnvEntriesCount.value;
  if (!count) return 'Applies to all Executor launches';
  return `${count} active variable${count === 1 ? '' : 's'} applied to Remote, Shell, Tmux, and Script launches`;
});

const launchEnvColumns = [
  { title: 'Enabled', key: 'enabled', dataIndex: 'enabled' },
  { title: 'Key', key: 'key', dataIndex: 'key' },
  { title: 'Value', key: 'value', dataIndex: 'value' },
  { title: 'Action', key: 'action', dataIndex: 'action' },
];

const detectedLaunchEnvColumns = [
  { title: 'Key', key: 'key', dataIndex: 'key' },
  { title: 'Value', key: 'value', dataIndex: 'value' },
  { title: 'Action', key: 'action', dataIndex: 'action' },
];

const filteredDetectedLaunchEnvEntries = computed(() => {
  const query = detectedLaunchEnvSearch.value.trim().toLowerCase();
  if (!query) {
    return detectedLaunchEnvEntries.value;
  }
  return detectedLaunchEnvEntries.value.filter((entry) => {
    return entry.key.toLowerCase().includes(query) || entry.value.toLowerCase().includes(query);
  });
});

watch([profiles, activeProfileId], persistProfiles, { deep: true });

onMounted(() => {
  void refreshDetectedLaunchEnvEntries();
});

const addLaunchEnvEntry = () => {
  const key = newLaunchEnvKey.value.trim();
  const value = newLaunchEnvValue.value;
  if (!key) {
    message.error('Please enter an environment variable name');
    return;
  }
  if (!isValidLaunchEnvKey(key)) {
    message.error('Environment variable names should look like FOO or FOO_BAR');
    return;
  }

  launchEnvEntries.value = [
    {
      id: `launch-env-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      key,
      value,
      enabled: true,
    },
    ...launchEnvEntries.value,
  ];
  newLaunchEnvKey.value = '';
  newLaunchEnvValue.value = '';
};

const removeLaunchEnvEntry = (id: string) => {
  launchEnvEntries.value = launchEnvEntries.value.filter((entry) => entry.id !== id);
};

const clearDisabledLaunchEnvEntries = () => {
  launchEnvEntries.value = launchEnvEntries.value.filter((entry) => entry.enabled);
};

const isLaunchEnvImported = (key: string) => launchEnvEntryKeys.value.has(key.trim());

const importDetectedLaunchEnvEntry = (entry: DetectedLaunchEnvEntry) => {
  const key = entry.key.trim();
  if (!key || !isValidLaunchEnvKey(key)) {
    return;
  }

  const next: LaunchEnvEntry = {
    id: `launch-env-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    key,
    value: entry.value,
    enabled: true,
  };

  const index = launchEnvEntries.value.findIndex((item) => item.key.trim() === key);
  if (index >= 0) {
    const current = launchEnvEntries.value[index];
    launchEnvEntries.value = [
      ...launchEnvEntries.value.slice(0, index),
      {
        ...current,
        key,
        value: entry.value,
        enabled: true,
      },
      ...launchEnvEntries.value.slice(index + 1),
    ];
    return;
  }

  launchEnvEntries.value = [next, ...launchEnvEntries.value];
};

const importAllDetectedLaunchEnvEntries = () => {
  for (const entry of filteredDetectedLaunchEnvEntries.value) {
    importDetectedLaunchEnvEntry(entry);
  }
};

const refreshDetectedLaunchEnvEntries = async () => {
  detectedLaunchEnvLoading.value = true;
  detectedLaunchEnvError.value = '';
  try {
    const res = await axios.get('/system/env');
    const items = Array.isArray(res.data?.items) ? (res.data.items as DetectedLaunchEnvEntry[]) : [];
    detectedLaunchEnvEntries.value = items
      .map((item) => ({
        key: String(item?.key || '').trim(),
        value: String(item?.value ?? ''),
      }))
      .filter((item) => Boolean(item.key));
  } catch (err: any) {
    detectedLaunchEnvError.value = err?.response?.data?.error || err?.message || 'Failed to load detected env vars';
  } finally {
    detectedLaunchEnvLoading.value = false;
  }
};

const isTmuxSession = (session: ShellSessionInfo) => {
  const kind = (session.kind || '').trim().toLowerCase();
  if (kind === 'tmux') return true;
  return (session.shell || '').trim().toLowerCase() === 'tmux' || (session.command || '').trim().toLowerCase() === 'tmux';
};

const routeSessionToManager = (session: ShellSessionInfo) => {
  if (isTmuxSession(session)) {
    tmuxManagerRef.value?.upsertSession(session);
    return 'tmux';
  }
  shellManagerRef.value?.upsertSession(session);
  return 'shell';
};

const focusSessionInManager = (session: ShellSessionInfo, manager?: ExecutorTabKey) => {
  const targetManager = manager || routeSessionToManager(session);
  if (manager) {
    if (manager === 'tmux') {
      tmuxManagerRef.value?.upsertSession(session);
    } else {
      shellManagerRef.value?.upsertSession(session);
    }
  }

  if (targetManager === 'tmux') {
    tmuxManagerRef.value?.openSession(session.id);
  } else {
    shellManagerRef.value?.openSession(session.id);
  }

  activeTabKey.value = targetManager;
};

const createShellSession = async (
  payload: ShellSessionCreateRequest,
  successMessage: string,
  manager: ExecutorTabKey,
) => {
  const res = await axios.post('/shell-sessions', payload);
  const session = res.data as ShellSessionInfo;
  focusSessionInManager(session, manager);
  message.success(successMessage);
  return session;
};

const codingCommandPreview = computed(() => {
  const cliCommand = getSelectedCodingCommand();
  if (!cliCommand) return 'Select a coding CLI command first';
  const cliArgs = splitArgs(codingExtraArgs.value);
  if (codingUseTmux.value) {
    const tmuxArgs = ['new-session', '-A', '-s', sanitizeTmuxSessionName(codingSessionName.value || cliCommand)];
    if (codingWorkDir.value.trim()) {
      tmuxArgs.push('-c', codingWorkDir.value.trim());
    }
    tmuxArgs.push('--', cliCommand, ...cliArgs);
    return `tmux ${tmuxArgs.join(' ')}`;
  }
  return [cliCommand, ...cliArgs].join(' ');
});

const launchCodingCli = async () => {
  const cliCommand = getSelectedCodingCommand();
  if (!cliCommand) {
    message.error('Please choose a coding CLI command');
    return;
  }

  codingLaunching.value = true;
  try {
    const cliArgs = splitArgs(codingExtraArgs.value);
    const workDir = codingWorkDir.value.trim();
    const payload: ShellSessionCreateRequest = codingUseTmux.value
      ? {
          shell: 'tmux',
          command: 'tmux',
          args: [
            'new-session',
            '-A',
            '-s',
            sanitizeTmuxSessionName(codingSessionName.value || cliCommand),
            ...(workDir ? ['-c', workDir] : []),
            '--',
            cliCommand,
            ...cliArgs,
          ],
          workDir,
          cols: 100,
          rows: 32,
          label: `tmux: ${cliCommand}`,
          kind: 'tmux',
          env: launchEnvRecord.value,
        }
      : {
          shell: cliCommand,
          command: cliCommand,
          args: cliArgs,
          workDir,
          cols: 100,
          rows: 32,
          label: `cli: ${cliCommand}`,
          kind: 'shell',
          env: launchEnvRecord.value,
        };

    await createShellSession(payload, `Launched coding CLI: ${cliCommand}`, codingUseTmux.value ? 'tmux' : 'shell');
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to launch coding CLI');
  } finally {
    codingLaunching.value = false;
  }
};

const scriptCommandPreview = computed(() => {
  return resolveScriptLaunchPlan(
    scriptLanguage.value,
    pythonVenv.value,
    scriptPath.value,
    scriptArgs.value,
  ).preview;
});

const launchScript = async () => {
  const script = scriptPath.value.trim();
  if (!script) {
    message.error('Please choose a script file');
    return;
  }

  const workDir = scriptWorkDir.value.trim() || dirname(script);
  const launchPlan = resolveScriptLaunchPlan(
    scriptLanguage.value,
    pythonVenv.value,
    script,
    scriptArgs.value,
  );

  scriptLaunching.value = true;
  try {
    const payload: ShellSessionCreateRequest = {
      shell: scriptLanguage.value,
      command: launchPlan.command,
      args: launchPlan.args,
      workDir,
      cols: 100,
      rows: 32,
      label: `${scriptLanguage.value}: ${basename(script)}`,
      kind: 'script',
      env: launchEnvRecord.value,
    };
    await createShellSession(payload, `Launched ${scriptLanguage.value} script: ${basename(script)}`, 'shell');
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to launch script');
  } finally {
    scriptLaunching.value = false;
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
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 16px;"
                message="This launcher starts the coding CLI inside tmux by default."
                description="The launched session appears only in the Tmux tab so shell and tmux management stay separate."
              />

              <a-form layout="vertical">
                <a-form-item label="CLI preset">
                  <a-select v-model:value="codingPreset" :options="codingPresetOptions" />
                </a-form-item>

                <a-form-item v-if="codingPreset === 'custom'" label="Custom CLI command">
                  <a-input
                    v-model:value="codingCustomCommand"
                    placeholder="codex, claude, gemini, or any executable in PATH"
                  />
                </a-form-item>

                <a-form-item label="Extra args">
                  <a-input
                    v-model:value="codingExtraArgs"
                    placeholder="e.g. --model gpt-5.5 --help"
                  />
                </a-form-item>

                <a-form-item label="tmux session name">
                  <a-input
                    v-model:value="codingSessionName"
                    placeholder="coding-cli"
                  />
                </a-form-item>

                <a-form-item label="Workdir">
                  <a-input-search
                    v-model:value="codingWorkDir"
                    placeholder="defaults to backend workdir if empty"
                    enter-button="Browse"
                    @search="setPathPickerTarget('coding-workdir')"
                  />
                </a-form-item>

                <a-form-item label="Launch mode">
                  <a-space>
                    <a-switch v-model:checked="codingUseTmux" />
                    <span>{{ codingUseTmux ? 'tmux wrapper' : 'direct command' }}</span>
                  </a-space>
                </a-form-item>

                <a-alert
                  type="success"
                  show-icon
                  style="margin-bottom: 16px;"
                  :message="codingCommandPreview"
                />

                <a-button
                  type="primary"
                  :loading="codingLaunching"
                  block
                  @click="launchCodingCli"
                >
                  <template #icon>
                    <PlayCircleOutlined />
                  </template>
                  Launch coding CLI
                </a-button>

                <a-alert
                  type="info"
                  show-icon
                  style="margin-top: 16px;"
                  message="Tmux shortcut tools stay available in the session pane after you open the launched session."
                />
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
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :xl="10">
            <a-card title="Launch script" :bordered="false">
              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 16px;"
                message="This launcher starts Python, Node.js, Ruby, sh, pwsh, Deno, or Bun scripts in a dedicated backend shell session."
                description="System environment is the default. For Python, you can optionally point at a venv directory and the launcher will use its interpreter."
              />

              <a-form layout="vertical">
                <a-form-item label="Language">
                  <a-select v-model:value="scriptLanguage" :options="scriptLanguageOptions" />
                </a-form-item>

                <a-form-item label="Script file">
                  <a-input-search
                    v-model:value="scriptPath"
                    placeholder="/path/to/script.py, app.js, script.rb, or script.ps1"
                    enter-button="Browse"
                    @search="setPathPickerTarget('script-path')"
                  />
                </a-form-item>

                <a-form-item label="Workdir">
                  <a-input-search
                    v-model:value="scriptWorkDir"
                    placeholder="defaults to script parent directory"
                    enter-button="Browse"
                    @search="setPathPickerTarget('script-workdir')"
                  />
                </a-form-item>

                <a-form-item v-if="scriptLanguage === 'python'" label="Python venv directory">
                  <a-input-search
                    v-model:value="pythonVenv"
                    placeholder="Leave empty for system Python"
                    enter-button="Browse"
                    @search="setPathPickerTarget('python-venv')"
                  />
                </a-form-item>

                <a-form-item label="Script args">
                  <a-input
                    v-model:value="scriptArgs"
                    :placeholder="scriptArgsPlaceholder"
                  />
                </a-form-item>

                <a-alert
                  v-if="scriptLanguage === 'deno'"
                  type="info"
                  show-icon
                  style="margin-bottom: 16px;"
                  message="For Deno, put runtime flags before `--`, then script arguments after `--`."
                />

                <a-alert
                  type="success"
                  show-icon
                  style="margin-bottom: 16px;"
                  :message="scriptCommandPreview"
                />

                <a-button
                  type="primary"
                  :loading="scriptLaunching"
                  block
                  @click="launchScript"
                >
                  <template #icon>
                    <PlayCircleOutlined />
                  </template>
                  Launch script
                </a-button>
              </a-form>
            </a-card>
          </a-col>

          <a-col :xs="24" :xl="14">
            <a-card title="Runtime notes" :bordered="false">
              <a-space direction="vertical" :size="12" style="width: 100%;">
                <a-descriptions bordered size="small" :column="1">
                  <a-descriptions-item label="Default environment">
                    <span>System</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Python interpreter">
                    <span>{{ resolvePythonInterpreter(pythonVenv) }}</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Current launch">
                    <span>{{ scriptCommandPreview }}</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Workdir fallback">
                    <span>{{ scriptWorkDir.trim() || (scriptPath.trim() ? dirname(scriptPath) : 'script parent') }}</span>
                  </a-descriptions-item>
                </a-descriptions>
                <a-alert
                  type="warning"
                  show-icon
                  message="Browse to the script file first, then optionally set a venv for Python."
                />
                <a-alert
                  type="info"
                  show-icon
                  message="The launched script session will show up in the Shell Manager tab for detach/reattach."
                />
              </a-space>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>

      <a-tab-pane key="launch-env" tab="Launch Env">
        <a-row :gutter="[16, 16]">
          <a-col :span="24">
            <a-card title="Profile Management" :bordered="false" style="margin-bottom: 16px;">
              <template #extra>
                <a-button type="primary" size="small" @click="addNewProfile">
                  <template #icon><PlusOutlined /></template>
                  New Profile
                </a-button>
              </template>
              <a-space wrap :size="12">
                <template v-for="profile in profiles" :key="profile.id">
                  <a-card-grid
                    :style="{
                      width: '280px',
                      padding: '12px',
                      textAlign: 'left',
                      cursor: 'pointer',
                      border: activeProfileId === profile.id ? '2px solid #1890ff' : '1px solid #f0f0f0',
                      boxShadow: activeProfileId === profile.id ? '0 0 8px rgba(24,144,255,0.2)' : 'none',
                      borderRadius: '4px',
                      background: activeProfileId === profile.id ? '#e6f7ff' : '#fff'
                    }"
                    @click="activeProfileId = profile.id"
                  >
                    <div style="display: flex; justify-content: space-between; align-items: flex-start;">
                      <div style="flex: 1; min-width: 0;">
                        <div style="font-weight: 600; margin-bottom: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" :title="profile.name">
                          {{ profile.name }}
                        </div>
                        <div style="font-size: 12px; color: #666;">
                          {{ profile.entries.length }} variables
                        </div>
                      </div>
                      <a-dropdown :trigger="['click']" @click.stop>
                        <SettingOutlined style="cursor: pointer; color: #1890ff;" />
                        <template #overlay>
                          <a-menu>
                            <a-menu-item key="rename" @click="openRenameModal(profile)">
                              <template #icon><EditOutlined /></template>
                              Rename
                            </a-menu-item>
                            <a-menu-item key="copy" @click="copyProfile(profile)">
                              <template #icon><CopyOutlined /></template>
                              Duplicate
                            </a-menu-item>
                            <a-menu-divider />
                            <a-menu-item key="delete" danger @click="deleteProfile(profile.id)">
                              <template #icon><DeleteOutlined /></template>
                              Delete
                            </a-menu-item>
                          </a-menu>
                        </template>
                      </a-dropdown>
                    </div>
                  </a-card-grid>
                </template>
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :xl="11">
            <a-card :title="`Variables in: ${activeProfile.name}`" :bordered="false">
              <template #extra>
                <a-space :size="8">
                  <a-tag color="green">{{ launchEnvEntriesCount }} active</a-tag>
                  <a-button size="small" @click="clearDisabledLaunchEnvEntries">
                    Clear disabled
                  </a-button>
                </a-space>
              </template>

              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 16px;"
                message="These key/value pairs are injected into every launch action from Executor."
                description="Remote Executor, Shell Manager, tmux coding launches, and script runners all receive the enabled variables."
              />

              <a-form layout="vertical">
                <a-row :gutter="12">
                  <a-col :xs="24" :md="8">
                    <a-form-item label="Key">
                      <a-input
                        v-model:value="newLaunchEnvKey"
                        placeholder="FOO_BAR"
                        @pressEnter="addLaunchEnvEntry"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-form-item label="Value">
                      <a-input
                        v-model:value="newLaunchEnvValue"
                        placeholder="value"
                        @pressEnter="addLaunchEnvEntry"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="4" style="display: flex; align-items: flex-end;">
                    <a-button type="primary" block @click="addLaunchEnvEntry">
                      <template #icon>
                        <PlusOutlined />
                      </template>
                      Add
                    </a-button>
                  </a-col>
                </a-row>
              </a-form>

              <a-table
                :data-source="launchEnvEntries"
                :columns="launchEnvColumns"
                :pagination="false"
                size="small"
                row-key="id"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'enabled'">
                    <a-switch v-model:checked="record.enabled" />
                  </template>
                  <template v-else-if="column.key === 'key'">
                    <a-input v-model:value="record.key" placeholder="ENV_NAME" allow-clear />
                  </template>
                  <template v-else-if="column.key === 'value'">
                    <a-input v-model:value="record.value" placeholder="value" allow-clear />
                  </template>
                  <template v-else-if="column.key === 'action'">
                    <a-button size="small" danger @click="removeLaunchEnvEntry(record.id)">
                      <template #icon>
                        <DeleteOutlined />
                      </template>
                      Delete
                    </a-button>
                  </template>
                </template>
              </a-table>
            </a-card>
          </a-col>

          <a-col :xs="24" :xl="13">
            <a-card title="Launch env preview" :bordered="false">
              <template #extra>
                <a-space :size="8">
                  <SettingOutlined />
                  <span>Local browser persistence</span>
                </a-space>
              </template>

              <a-space direction="vertical" :size="12" style="width: 100%;">
                <a-descriptions bordered size="small" :column="1">
                  <a-descriptions-item label="Active variables">
                    <span>{{ launchEnvEntriesCount }}</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Scope">
                    <span>{{ launchEnvScope }}</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Preview">
                    <span>{{ launchEnvPreview }}</span>
                  </a-descriptions-item>
                </a-descriptions>

                <a-alert
                  type="warning"
                  show-icon
                  message="Environment variable names should use the usual shell style: FOO or FOO_BAR."
                />
                <a-alert
                  type="info"
                  show-icon
                  message="Because this is stored in your browser, each workstation/browser can keep its own launch env profile."
                />
              </a-space>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" style="margin-top: 16px;">
          <a-col :span="24">
            <a-card title="Detected environment from backend" :bordered="false">
              <template #extra>
                <a-space :size="8">
                  <a-tag color="blue">{{ filteredDetectedLaunchEnvEntries.length }} visible</a-tag>
                  <a-tag color="default">{{ detectedLaunchEnvEntries.length }} detected</a-tag>
                  <a-button size="small" :loading="detectedLaunchEnvLoading" @click="refreshDetectedLaunchEnvEntries">
                    <template #icon>
                      <ReloadOutlined />
                    </template>
                    Refresh
                  </a-button>
                  <a-button
                    size="small"
                    type="primary"
                    :disabled="filteredDetectedLaunchEnvEntries.length === 0"
                    @click="importAllDetectedLaunchEnvEntries"
                  >
                    Import visible
                  </a-button>
                </a-space>
              </template>

              <a-alert
                type="info"
                show-icon
                style="margin-bottom: 16px;"
                message="This list is read from the backend runtime environment. Backend configuration vars such as AGENT_*, GIN_MODE, DISABLE_AUTH, SUDO_*, and PKEXEC_UID are hidden."
              />

              <a-alert
                v-if="detectedLaunchEnvError"
                type="warning"
                show-icon
                style="margin-bottom: 16px;"
                :message="detectedLaunchEnvError"
              />

              <a-input-search
                v-model:value="detectedLaunchEnvSearch"
                placeholder="Filter detected env by key or value"
                enter-button="Search"
                style="margin-bottom: 16px;"
              />

              <a-table
                :data-source="filteredDetectedLaunchEnvEntries"
                :columns="detectedLaunchEnvColumns"
                :loading="detectedLaunchEnvLoading"
                :pagination="false"
                row-key="key"
                size="small"
              >
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'key'">
                    <code>{{ record.key }}</code>
                  </template>
                  <template v-else-if="column.key === 'value'">
                    <span class="executor-env__value" :title="record.value">
                      {{ record.value || '—' }}
                    </span>
                  </template>
                  <template v-else-if="column.key === 'action'">
                    <a-space>
                      <a-tag v-if="isLaunchEnvImported(record.key)" color="green">Imported</a-tag>
                      <a-button size="small" @click="importDetectedLaunchEnvEntry(record)">
                        {{ isLaunchEnvImported(record.key) ? 'Update' : 'Use' }}
                      </a-button>
                    </a-space>
                  </template>
                </template>

                <template #emptyText>
                  <a-empty description="No backend runtime env vars detected" />
                </template>
              </a-table>
            </a-card>
          </a-col>
        </a-row>
      </a-tab-pane>
    </a-tabs>

    <PathNavigatorDrawer
      v-model:open="pathPickerOpen"
      :title="getPathPickerTitle"
      :initial-path="getPathPickerInitialPath"
      :pick-mode="getPathPickerMode"
      @confirm="applyPickedPath"
    />

    <a-modal
      v-model:open="profileRenameModalOpen"
      title="Rename Profile"
      @ok="applyRename"
    >
      <a-form layout="vertical">
        <a-form-item label="Profile Name">
          <a-input v-model:value="profileRenameValue" placeholder="Enter profile name" @pressEnter="applyRename" />
        </a-form-item>
      </a-form>
    </a-modal>
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
