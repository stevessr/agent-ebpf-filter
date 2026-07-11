import type { SelectProps } from "ant-design-vue";

export type TLSManualHookType =
  | "executable"
  | "go"
  | "openssl"
  | "gnutls"
  | "nss";

export type TLSExecutableLibraryHint = "auto" | "openssl" | "gnutls" | "nss";

export const TLS_IGNORE_RULES_STORAGE_KEY = "agent-ebpf.tls.ignoreRules";
export const TLS_CAPTURE_EVENT_LIMIT = 500;
export const TLS_CAPTURE_RECONNECT_DELAY_MS = 3_000;
export const TLS_CAPTURE_STATUS_REFRESH_MS = 5_000;
export const TLS_CAPTURE_STATUS_TIMEOUT_MS = 10_000;
export const TLS_CAPTURE_DEFAULT_QUEUE_CAPACITY = 64;

export const TLS_MANUAL_HOOK_OPTIONS: SelectProps["options"] = [
  { label: "Executable / CLI bin", value: "executable" },
  { label: "Go TLS binary", value: "go" },
  { label: "OpenSSL libssl", value: "openssl" },
  { label: "GnuTLS library", value: "gnutls" },
  { label: "NSS / NSPR library", value: "nss" },
];

export const TLS_EXECUTABLE_LIBRARY_OPTIONS: SelectProps["options"] = [
  { label: "Auto detect", value: "auto" },
  { label: "OpenSSL", value: "openssl" },
  { label: "GnuTLS", value: "gnutls" },
  { label: "NSS / NSPR", value: "nss" },
];

export const TLS_RULE_SCOPE_OPTIONS: SelectProps["options"] = [
  { label: "Agent CLI tag", value: "agent_cli_tag" },
  { label: "Custom", value: "custom" },
];

export const TLS_DIRECTION_OPTIONS: SelectProps["options"] = [
  { label: "All directions", value: "all" },
  { label: "Send", value: "send" },
  { label: "Recv", value: "recv" },
];

export const TLS_BUILTIN_COMMANDS = [
  "node",
  "deno",
  "bun",
  "codex",
  "claude",
  "gemini",
];
