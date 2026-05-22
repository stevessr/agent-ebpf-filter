export type VisualTrigger =
  | "process"
  | "file_open"
  | "mkdir"
  | "file_create"
  | "rmdir"
  | "symlink"
  | "unlink"
  | "socket_connect"
  | "inode_mknod"
  | "file_mprotect"
  | "inode_rename";

export type VisualAction = "BLOCK" | "ALERT" | "KILL";

export type VisualMapMode = "NONE" | "COUNTER" | "BLOCKLIST";

export type VisualMapKey = "uid" | "pid" | "comm";

export type VisualConditionField =
  | "comm"
  | "pid"
  | "uid"
  | "basename"
  | "port"
  | "ipv4"
  | "gid";

export interface VisualCondition {
  id: string;
  type: "CONDITION";
  field: VisualConditionField;
  operator: "==" | "!=" | "starts_with" | "ends_with";
  value: string;
}

export interface VisualLogicGroup {
  id: string;
  type: "AND" | "OR";
  children: Array<VisualLogicGroup | VisualCondition>;
}

export type VisualLogicNode = VisualLogicGroup | VisualCondition;

export interface TriggerOption {
  value: VisualTrigger;
  label: string;
  icon: any;
  color: string;
}

export interface VisualWorkspaceSnapshot {
  version: 1;
  trigger: VisualTrigger;
  action: VisualAction;
  conditions: VisualLogicGroup;
  mapMode: VisualMapMode;
  mapKey: VisualMapKey;
  mapLimit: number;
  pluginId?: string;
  pluginName?: string;
  description?: string;
  nodeLayout?: VisualNodeLayout;
  wireStates?: VisualWireStates;
}

export interface VisualRecipe extends VisualWorkspaceSnapshot {
  id: string;
  name: string;
  description: string;
  tags: string[];
}

export type VisualValidationSeverity = "error" | "warning" | "info";

export interface VisualValidationIssue {
  id: string;
  severity: VisualValidationSeverity;
  title: string;
  detail?: string;
}

export interface VisualNodePosition {
  x: number;
  y: number;
}

export type VisualFlowNodeId =
  | "trigger"
  | "condition"
  | "map"
  | "action"
  | "code"
  | "compile";

export type VisualWireId =
  | "trigger-condition"
  | "condition-map"
  | "map-action"
  | "condition-code"
  | "map-code"
  | "action-compile"
  | "code-compile";

export type VisualNodeLayout = Record<string, VisualNodePosition>;

export type VisualWireStates = Partial<Record<VisualWireId, boolean>>;
