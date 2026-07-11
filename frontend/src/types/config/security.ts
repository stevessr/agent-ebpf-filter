export type SecurityRuleAction = "BLOCK" | "ALERT";
export interface SecurityRulePreset {
  comm: string;
  action: SecurityRuleAction;
  priority: number;
  source: string;
  summary: string;
}
export interface ExternalRuleSource {
  id: string;
  name: string;
  description: string;
  url: string;
  format: "json" | "yaml" | "markdown";
  sourceAttribution: string;
  category: "agent-security" | "community" | "owasp";
}
export interface SyscallDef {
  type: number;
  name: string;
  desc: string;
}
export interface SyscallGroup {
  key: string;
  title: string;
  icon: string;
  color: string;
  syscalls: SyscallDef[];
}
