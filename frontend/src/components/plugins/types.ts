export interface VisualCondition {
  id: string;
  type: "CONDITION";
  field: "comm" | "pid" | "uid" | "basename" | "port" | "ipv4" | "gid";
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
  value: string;
  label: string;
  icon: any;
  color: string;
}
