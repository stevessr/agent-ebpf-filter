export interface HookDef {
  id: string;
  name: string;
  description: string;
  target_cmd: string;
  hook_type: 'native' | 'wrapper';
  installed: boolean;
}
