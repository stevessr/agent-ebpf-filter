<script setup lang="ts">
import { computed, ref } from 'vue';
import axios from 'axios';
import { CodeOutlined, PlayCircleOutlined } from '@ant-design/icons-vue';
import { message } from 'ant-design-vue';

import LocalShellTerminal from '../components/LocalShellTerminal.vue';
import PathNavigatorDrawer from '../components/PathNavigatorDrawer.vue';
import type { ShellSessionCreateRequest, ShellSessionInfo } from '../types/shell';

type ExecutorTabKey = 'shell' | 'tmux' | 'scripts';
type PathPickerTarget = 'coding-workdir' | 'script-path' | 'script-workdir' | 'python-venv';

type LocalShellManagerExpose = {
  upsertSession: (session: ShellSessionInfo) => void;
  openSession: (sessionId: string) => void;
  refreshSessions: () => Promise<void>;
};

type CodingPresetKey = 'codex' | 'claude' | 'gemini' | 'custom';
type ScriptLanguage = 'python' | 'node' | 'ruby' | 'sh' | 'pwsh';

const shellManagerRef = ref<LocalShellManagerExpose | null>(null);
const tmuxManagerRef = ref<LocalShellManagerExpose | null>(null);
const activeTabKey = ref<ExecutorTabKey>('shell');

const command = ref('');
const args = ref('');
const loading = ref(false);
const recentCommands = ref<string[]>(
  JSON.parse(localStorage.getItem('recent_cmds') || '[]'),
);

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

const codingPresetOptions: Array<{ label: string; value: CodingPresetKey; command: string }> = [
  { label: 'Codex', value: 'codex', command: 'codex' },
  { label: 'Claude Code', value: 'claude', command: 'claude' },
  { label: 'Gemini CLI', value: 'gemini', command: 'gemini' },
  { label: 'Custom', value: 'custom', command: '' },
];

const scriptLanguageOptions: Array<{ label: string; value: ScriptLanguage }> = [
  { label: 'Python', value: 'python' },
  { label: 'Node.js', value: 'node' },
  { label: 'Ruby', value: 'ruby' },
  { label: 'Shell (sh)', value: 'sh' },
  { label: 'PowerShell (pwsh)', value: 'pwsh' },
];

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

const resolveScriptInterpreter = (language: ScriptLanguage, venvPath: string) => {
  switch (language) {
    case 'python':
      return resolvePythonInterpreter(venvPath);
    case 'node':
      return 'node';
    case 'ruby':
      return 'ruby';
    case 'sh':
      return 'sh';
    case 'pwsh':
      return 'pwsh';
    default:
      return 'python3';
  }
};

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

const runCommand = async () => {
  const executable = command.value.trim();
  if (!executable) return;

  loading.value = true;
  try {
    const argList = splitArgs(args.value);
    const res = await axios.post('/system/run', {
      comm: executable,
      args: argList,
    });

    message.success(`Started process PID: ${res.data.pid}`);

    const full = `${executable} ${args.value}`.trim();
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
  const parts = splitArgs(cmdStr);
  command.value = parts[0] || '';
  args.value = parts.slice(1).join(' ');
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
        };

    await createShellSession(payload, `Launched coding CLI: ${cliCommand}`, codingUseTmux.value ? 'tmux' : 'shell');
  } catch (err: any) {
    message.error(err?.response?.data?.error || err?.message || 'Failed to launch coding CLI');
  } finally {
    codingLaunching.value = false;
  }
};

const scriptCommandPreview = computed(() => {
  const script = scriptPath.value.trim();
  const scriptArgsList = splitArgs(scriptArgs.value);
  const interpreter = resolveScriptInterpreter(scriptLanguage.value, pythonVenv.value);
  if (!script) {
    return `${interpreter} <script>`;
  }
  return [interpreter, script, ...scriptArgsList].join(' ');
});

const launchScript = async () => {
  const script = scriptPath.value.trim();
  if (!script) {
    message.error('Please choose a script file');
    return;
  }

  const workDir = scriptWorkDir.value.trim() || dirname(script);
  const interpreter = resolveScriptInterpreter(scriptLanguage.value, pythonVenv.value);

  scriptLaunching.value = true;
  try {
    const payload: ShellSessionCreateRequest = {
      shell: scriptLanguage.value,
      command: interpreter,
      args: [script, ...splitArgs(scriptArgs.value)],
      workDir,
      cols: 100,
      rows: 32,
      label: `${scriptLanguage.value}: ${basename(script)}`,
      kind: 'script',
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
          <a-card title="Remote Executor (via Wrapper)" :bordered="false">
            <template #extra>
              <CodeOutlined />
            </template>

            <p style="color: #666; margin-bottom: 24px;">
              Execute commands on the host system. All commands are automatically routed through the
              <code>agent-wrapper</code> to enforce security policies and track activities.
            </p>

            <a-form layout="vertical">
              <a-row :gutter="16">
                <a-col :span="8">
                  <a-form-item label="Executable">
                    <a-input
                      v-model:value="command"
                      placeholder="e.g. ls, python, git"
                      @pressEnter="runCommand"
                    >
                      <template #prefix>
                        <CodeOutlined />
                      </template>
                    </a-input>
                  </a-form-item>
                </a-col>

                <a-col :span="12">
                  <a-form-item label="Arguments">
                    <a-input
                      v-model:value="args"
                      placeholder="e.g. -la /tmp"
                      @pressEnter="runCommand"
                    />
                  </a-form-item>
                </a-col>

                <a-col
                  :span="4"
                  style="display: flex; align-items: flex-end; padding-bottom: 24px;"
                >
                  <a-button type="primary" :loading="loading" @click="runCommand" block>
                    <template #icon>
                      <PlayCircleOutlined />
                    </template>
                    Run
                  </a-button>
                </a-col>
              </a-row>
            </a-form>

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
          </a-card>

          <a-card title="Interactive Shell Manager (wterm)" :bordered="false">
            <template #extra>
              <a-tag color="blue">multi-session PTY</a-tag>
            </template>

            <LocalShellTerminal ref="shellManagerRef" session-kind-filter="non-tmux" />
          </a-card>
        </a-space>
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
                message="This launcher starts Python, Node.js, Ruby, sh, or pwsh scripts in a dedicated backend shell session."
                description="System environment is the default. For Python, you can optionally point at a venv directory and the launcher will use its interpreter."
              />

              <a-form layout="vertical">
                <a-form-item label="Language">
                  <a-select v-model:value="scriptLanguage" :options="scriptLanguageOptions" />
                </a-form-item>

                <a-form-item label="Script file">
                  <a-input-search
                    v-model:value="scriptPath"
                    placeholder="/path/to/script.py or app.js"
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
                    placeholder="e.g. --debug --foo bar"
                  />
                </a-form-item>

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
                    <span>{{ resolveScriptInterpreter('python', pythonVenv) }}</span>
                  </a-descriptions-item>
                  <a-descriptions-item label="Current interpreter">
                    <span>{{ resolveScriptInterpreter(scriptLanguage, pythonVenv) }}</span>
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
