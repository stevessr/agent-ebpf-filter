import type { ShellSessionInfo } from '../types/shell';

export interface TmuxShortcut {
  key: string;
  label: string;
  sequence: string;
  danger?: boolean;
}

export const TMUX_SHORTCUTS: TmuxShortcut[] = [
  { key: 'detach-client', label: 'Detach tmux', sequence: '\x02d' },
  { key: 'new-window', label: 'New window', sequence: '\x02c' },
  { key: 'split-vertical', label: 'Split vertical', sequence: '\x02%' },
  { key: 'split-horizontal', label: 'Split horizontal', sequence: '\x02"' },
  { key: 'next-window', label: 'Next window', sequence: '\x02n' },
  { key: 'prev-window', label: 'Prev window', sequence: '\x02p' },
  { key: 'zoom-pane', label: 'Zoom pane', sequence: '\x02z' },
  { key: 'kill-pane', label: 'Kill pane', sequence: '\x02x', danger: true },
  { key: 'kill-window', label: 'Kill window', sequence: '\x02&', danger: true },
];

export const isTmuxSession = (session: Pick<ShellSessionInfo, 'kind' | 'shell' | 'command' | 'label'>) => {
  const kind = (session.kind || '').trim().toLowerCase();
  if (kind === 'tmux') return true;
  if ((session.shell || '').trim().toLowerCase() === 'tmux') return true;
  if ((session.command || '').trim().toLowerCase() === 'tmux') return true;
  return (session.label || '').trim().toLowerCase().startsWith('tmux:');
};
