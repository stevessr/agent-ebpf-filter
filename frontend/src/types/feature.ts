export type FeatureID =
  | "shell_sessions"
  | "system_run"
  | "hooks"
  | "policy_management"
  | "tls_capture"
  | "otlp"
  | "domain_forward"
  | "ml"
  | "plugins"
  | "sandbox_cgroup"
  | "sandbox_lsm"
  | "network_export"
  | "agentsight";

export type FeatureDangerLevel = "low" | "medium" | "high" | "critical";

export type FeatureStatus =
  | "compiled-out"
  | "runtime-disabled"
  | "enabled"
  | "unknown";

export interface FeatureManifestEntry {
  id: FeatureID;
  name: string;
  compiledIn: boolean;
  runtimeEnabled: boolean;
  runtimeGate?: string;
  authRequired: boolean;
  routePrefixes: string[];
  dangerLevel: FeatureDangerLevel;
  buildTag: string;
  compatibilityAliases?: string[];
}

export interface FeatureManifestResponse {
  features: FeatureManifestEntry[];
}

export const ALL_FEATURE_IDS: FeatureID[] = [
  "shell_sessions",
  "system_run",
  "hooks",
  "policy_management",
  "tls_capture",
  "otlp",
  "domain_forward",
  "ml",
  "plugins",
  "sandbox_cgroup",
  "sandbox_lsm",
  "network_export",
  "agentsight",
];
