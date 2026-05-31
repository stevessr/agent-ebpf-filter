import { computed, ref } from 'vue';
import { message } from 'ant-design-vue';
import type { ShellSessionCreateRequest, ShellSessionInfo } from '../../types/shell';
import { splitArgs, sanitizeTmuxSessionName } from './useLauncherUtils';

export type CodingPresetKey = 'codex' | 'claude' | 'gemini' | 'custom';

export const CODING_PRESET_OPTIONS: Array<{ label: string; value: CodingPresetKey; command: string }> = [
  { label: 'Codex', value: 'codex', command: 'codex' },
  { label: 'Claude Code', value: 'claude', command: 'claude' },
  { label: 'Gemini CLI', value: 'gemini', command: 'gemini' },
  { label: 'Custom', value: 'custom', command: '' },
];

export function useCodingLauncher(
  createSession: (payload: ShellSessionCreateRequest, successMessage: string, manager: 'shell' | 'tmux') => Promise<ShellSessionInfo | undefined>,
  getLaunchEnvRecord: () => Record<string, string>,
) {
  const codingPreset = ref<CodingPresetKey>('codex');
  const codingCustomCommand = ref('');
  const codingExtraArgs = ref('');
  const codingSessionName = ref('coding');
  const codingWorkDir = ref('');
  const codingUseTmux = ref(true);
  const codingLaunching = ref(false);

  const getSelectedCodingCommand = () => {
    if (codingPreset.value === 'custom') {
      return codingCustomCommand.value.trim();
    }
    return CODING_PRESET_OPTIONS.find((option) => option.value === codingPreset.value)?.command || '';
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
            env: getLaunchEnvRecord(),
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
            env: getLaunchEnvRecord(),
          };

      await createSession(payload, `Launched coding CLI: ${cliCommand}`, codingUseTmux.value ? 'tmux' : 'shell');
    } catch (err: any) {
      message.error(err?.response?.data?.error || err?.message || 'Failed to launch coding CLI');
    } finally {
      codingLaunching.value = false;
    }
  };

  return {
    codingPreset,
    codingCustomCommand,
    codingExtraArgs,
    codingSessionName,
    codingWorkDir,
    codingUseTmux,
    codingLaunching,
    codingCommandPreview,
    launchCodingCli,
  };
}
