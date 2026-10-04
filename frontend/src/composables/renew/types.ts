export type RenewTone = "normal" | "warning" | "danger";

export interface RenewAgentSession {
  key: string;
  label: string;
  events: number;
  alerts: number;
  lastSeen: number;
  lastAction: string;
}

export interface RenewDestinationSummary {
  endpoint: string;
  count: number;
}
