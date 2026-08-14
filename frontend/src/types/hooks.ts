export type HookConfigFormat = "json" | "toml" | "typescript";

export interface HookDef {
	id: string;
	name: string;
	description: string;
	target_cmd: string;
	hook_type: "native" | "wrapper";
	config_format?: HookConfigFormat;
	installed: boolean;
}
