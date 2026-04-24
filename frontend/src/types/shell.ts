export type ShellMode = 'auto' | 'system' | 'fish' | 'zsh' | 'bash' | 'ash' | 'sh' | 'custom';

export interface ShellConfig {
  mode: ShellMode;
  customPath: string;
}

export interface ShellSessionCreateRequest {
  shell: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  label?: string;
  workDir?: string;
  cols?: number;
  rows?: number;
}

export interface ShellSessionInfo {
  id: string;
  label?: string;
  shell: string;
  shellPath: string;
  command?: string;
  args?: string[];
  workDir: string;
  pid: number;
  status: string;
  attached: boolean;
  createdAt: string;
  updatedAt: string;
  lastError?: string;
}
