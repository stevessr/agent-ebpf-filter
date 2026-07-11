export interface TrackedItem {
  comm?: string;
  path?: string;
  prefix?: string;
  tag: string;
  disabled?: boolean;
}
export interface WrapperRule {
  comm: string;
  action: string;
  rewritten_cmd: string[];
  regex?: string;
  replacement?: string;
  priority?: number;
}
