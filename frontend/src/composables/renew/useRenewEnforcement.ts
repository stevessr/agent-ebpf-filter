import { ref, type Ref } from "vue";
import axios from "axios";
import type { RuntimeConfigResponse } from "../../types/config";

import { backendError } from "./notify";

export interface RenewEnforcementTarget {
  ip: string;
  // null when the endpoint carries no usable port
  port: number | null;
}

export interface RenewEnforcementTargets {
  ip: string | null;
  port: number | null;
  execPath: string | null;
}

const IPV4_PATTERN = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;

export const isIPv4Literal = (host: string): boolean => {
  const match = IPV4_PATTERN.exec(host);
  if (!match) return false;
  return match.slice(1).every((octet) => {
    const value = Number(octet);
    return (
      value >= 0 &&
      value <= 255 &&
      String(value) === octet.replace(/^0+(?=\d)/, "")
    );
  });
};

const isIPv6Group = (group: string) =>
  group.length >= 1 && group.length <= 4 && /^[0-9a-fA-F]+$/.test(group);

export const isIPv6Literal = (host: string): boolean => {
  if (!host.includes(":")) return false;
  const bare = host.split("%")[0]!;
  // IPv4-mapped tail like ::ffff:192.168.0.1
  const lastColon = bare.lastIndexOf(":");
  if (lastColon !== -1 && bare.slice(lastColon + 1).includes(".")) {
    if (!isIPv4Literal(bare.slice(lastColon + 1))) return false;
    return isIPv6Literal(bare.slice(0, lastColon + 1));
  }
  if (bare.includes(":::")) return false;
  const doubleColon = bare.includes("::");
  if (!doubleColon && (bare.startsWith(":") || bare.endsWith(":"))) {
    // single leading/trailing colon is never valid
    return false;
  }
  const groups = bare.split(":").filter((group) => group !== "");
  if (!groups.every(isIPv6Group)) return false;
  // "::" collapses at least one zero group; without it all 8 must be present
  return doubleColon ? groups.length <= 7 : groups.length === 8;
};

export const isIPLiteral = (host: string): boolean =>
  isIPv4Literal(host) || isIPv6Literal(host);

const validPort = (port: number): boolean =>
  Number.isInteger(port) && port >= 1 && port <= 65535;

const portFromString = (value: string): number | null => {
  if (!/^\d{1,5}$/.test(value)) return null;
  const port = Number(value);
  return validPort(port) ? port : null;
};

/**
 * Host:port splitter for event endpoints.
 *
 * - `1.2.3.4:443` -> { ip: "1.2.3.4", port: 443 }
 * - `1.2.3.4`     -> { ip: "1.2.3.4", port: null }
 * - `[::1]:443`   -> { ip: "::1", port: 443 }
 * - `2001:db8::1` -> { ip: "2001:db8::1", port: null } (bare IPv6 is never
 *   split on its interior colons)
 * - DNS names / garbage -> null (no IP target; the caller must never block a
 *   hostname through the IP endpoint)
 */
export const parseEndpointIP = (
  endpoint: string,
): RenewEnforcementTarget | null => {
  const value = endpoint.trim();
  if (!value) return null;

  let host: string;
  let port: number | null = null;

  if (value.startsWith("[")) {
    const close = value.indexOf("]");
    if (close === -1) return null;
    host = value.slice(1, close);
    const rest = value.slice(close + 1);
    if (rest) {
      if (!rest.startsWith(":")) return null;
      port = portFromString(rest.slice(1));
      if (port === null) return null;
    }
  } else {
    const firstColon = value.indexOf(":");
    if (firstColon === -1) {
      host = value;
    } else if (value.indexOf(":", firstColon + 1) === -1) {
      // exactly one colon: host:port (an IPv6 literal always has >= 2 colons)
      host = value.slice(0, firstColon);
      port = portFromString(value.slice(firstColon + 1));
      if (port === null) return null;
    } else {
      // bare IPv6 with interior colons: no port suffix
      host = value;
    }
  }

  if (!isIPLiteral(host)) return null;
  return { ip: host, port };
};

type AnyRecord = Record<string, unknown>;

const asRecord = (value: unknown): AnyRecord | null =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as AnyRecord)
    : null;

const textValue = (value: unknown): string =>
  typeof value === "string" ? value.trim() : "";

const firstText = (detail: AnyRecord | null, ...keys: string[]): string => {
  if (!detail) return "";
  for (const key of keys) {
    const value = textValue(detail[key]);
    if (value) return value;
  }
  return "";
};

const firstNumber = (detail: AnyRecord | null, ...keys: string[]): unknown => {
  if (!detail) return undefined;
  for (const key of keys) {
    if (detail[key] != null && detail[key] !== "") return detail[key];
  }
  return undefined;
};

/**
 * Applicable enforcement actions for one event detail. Only concrete IP
 * targets (never DNS names), valid ports, and absolute executable paths are
 * offered; every field may be null.
 */
export const deriveRenewEnforcementTargets = (
  detail: Record<string, unknown> | null,
  category: string,
): RenewEnforcementTargets => {
  const record = asRecord(detail);
  if (!record) return { ip: null, port: null, execPath: null };

  let ip: string | null = null;
  let port: number | null = null;

  const endpoint = firstText(record, "net_endpoint", "NetEndpoint", "endpoint");
  if (endpoint) {
    // A DNS name or malformed endpoint offers nothing at all — never send a
    // hostname to the IP blocklist.
    const parsed = parseEndpointIP(endpoint);
    if (parsed) {
      ip = parsed.ip;
      port = parsed.port;
    }
  } else if (category === "network") {
    const dstIp = firstText(record, "dst_ip", "DstIp");
    if (dstIp && isIPLiteral(dstIp)) {
      ip = dstIp;
      const rawPort = firstNumber(record, "dst_port", "DstPort");
      const numeric = Number(rawPort);
      if (Number.isFinite(numeric) && validPort(numeric)) port = numeric;
    }
  }

  const path = firstText(record, "path", "Path");
  const execPath = path.startsWith("/") ? path : null;

  if (!ip && !execPath) return { ip: null, port: null, execPath: null };
  return { ip, port, execPath };
};

export interface RenewEnforcement {
  blockedIPs: Ref<string[]>;
  blockedPorts: Ref<number[]>;
  blockedExecPaths: Ref<string[]>;
  available: Ref<boolean>;
  busy: Ref<boolean>;
  error: Ref<string>;
  policyManagementEnabled: Ref<boolean>;
  fetchStatus: () => Promise<void>;
  toggleIP: (ip: string) => Promise<void>;
  togglePort: (port: number) => Promise<void>;
  toggleExecPath: (path: string) => Promise<void>;
}

const POLICY_GATE_MESSAGE =
  "策略管理未启用：请在配置中开启 policy_management 后再执行阻断操作";

export function useRenewEnforcement(): RenewEnforcement {
  const blockedIPs = ref<string[]>([]);
  const blockedPorts = ref<number[]>([]);
  const blockedExecPaths = ref<string[]>([]);
  const available = ref(false);
  const busy = ref(false);
  const error = ref("");
  const policyManagementEnabled = ref(false);

  const fetchRuntimeGate = async () => {
    // Tolerated: without the gate answer we keep actions disabled, not broken.
    try {
      const res = await axios.get<RuntimeConfigResponse>("/config/runtime");
      policyManagementEnabled.value = Boolean(
        res.data?.runtime?.policyManagementEnabled,
      );
    } catch {
      policyManagementEnabled.value = false;
    }
  };

  const fetchCgroupStatus = async () => {
    try {
      const res = await axios.get("/sandbox/cgroup/status");
      available.value = Boolean(res.data?.available);
      blockedIPs.value = res.data?.blockedIPs || [];
      blockedPorts.value = res.data?.blockedPorts || [];
    } catch (cause) {
      available.value = false;
      blockedIPs.value = [];
      blockedPorts.value = [];
      if (!error.value) {
        error.value = backendError(cause, "无法加载 cgroup 沙箱状态");
      }
    }
  };

  const fetchLsmStatus = async () => {
    try {
      const res = await axios.get("/sandbox/lsm/status");
      blockedExecPaths.value = res.data?.blockedExecPaths || [];
    } catch {
      // LSM may be unavailable without the cgroup sandbox; the exec-path
      // action just stays in the unblocked state.
      blockedExecPaths.value = [];
    }
  };

  const fetchStatus = async () => {
    error.value = "";
    await Promise.all([fetchCgroupStatus(), fetchLsmStatus()]);
    await fetchRuntimeGate();
  };

  const actionErrorText = (cause: unknown): string => {
    if (cause && typeof cause === "object" && "response" in cause) {
      const response = cause.response;
      if (
        response &&
        typeof response === "object" &&
        "status" in response &&
        "data" in response &&
        response.status === 403
      ) {
        const data = response.data;
        if (
          data &&
          typeof data === "object" &&
          "feature" in data &&
          data.feature === "policy_management"
        ) {
          return POLICY_GATE_MESSAGE;
        }
      }
    }
    return backendError(cause, "处置操作失败；请确认后端可用");
  };

  const postEnforcementAction = async (
    path: string,
    payload: Record<string, unknown>,
    successText: string,
  ) => {
    error.value = "";
    if (!policyManagementEnabled.value || busy.value) return;
    busy.value = true;
    try {
      await axios.post(path, payload);
      // The drawer renders the authoritative status line with the refreshed
      // state; the composable stays DOM-free.
      await fetchStatus();
    } catch (cause) {
      const text = actionErrorText(cause);
      error.value = text;
    } finally {
      busy.value = false;
    }
  };

  const toggleIP = (ip: string) => {
    const target = ip.trim();
    if (!target) return Promise.resolve();
    const blocked = blockedIPs.value.includes(target);
    return postEnforcementAction(
      blocked ? "/sandbox/cgroup/unblock-ip" : "/sandbox/cgroup/block-ip",
      { ip: target },
      blocked ? `已解除 IP ${target} 的阻断` : `已阻断 IP ${target}`,
    );
  };

  const togglePort = (port: number) => {
    if (!validPort(port)) return Promise.resolve();
    const blocked = blockedPorts.value.includes(port);
    return postEnforcementAction(
      blocked ? "/sandbox/cgroup/unblock-port" : "/sandbox/cgroup/block-port",
      { port },
      blocked ? `已解除端口 ${port} 的阻断` : `已阻断端口 ${port}`,
    );
  };

  const toggleExecPath = (path: string) => {
    const target = path.trim();
    if (!target.startsWith("/")) return Promise.resolve();
    const blocked = blockedExecPaths.value.includes(target);
    return postEnforcementAction(
      blocked
        ? "/sandbox/lsm/unblock-exec-path"
        : "/sandbox/lsm/block-exec-path",
      { path: target },
      blocked ? `已解除执行阻断：${target}` : `已在内核层阻断执行：${target}`,
    );
  };

  return {
    blockedIPs,
    blockedPorts,
    blockedExecPaths,
    available,
    busy,
    error,
    policyManagementEnabled,
    fetchStatus,
    toggleIP,
    togglePort,
    toggleExecPath,
  };
}
