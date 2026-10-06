import { expect, test, afterEach } from "bun:test";
import axios from "axios";
import {
  deriveRenewEnforcementTargets,
  parseEndpointIP,
  useRenewEnforcement,
} from "../src/composables/renew/useRenewEnforcement";

const originalGet = axios.get;
const originalPost = axios.post;
afterEach(() => {
  axios.get = originalGet;
  axios.post = originalPost;
});

const POLICY_GATE_MESSAGE =
  "策略管理未启用：请在配置中开启 policy_management 后再执行阻断操作";

// ── parseEndpointIP：host:port 拆分 ──
test("parseEndpointIP splits IPv4 host:port and keeps bare hosts portless", () => {
  expect(parseEndpointIP("1.2.3.4:443")).toEqual({ ip: "1.2.3.4", port: 443 });
  expect(parseEndpointIP("1.2.3.4")).toEqual({ ip: "1.2.3.4", port: null });
  expect(parseEndpointIP(" 1.2.3.4:8080 ")).toEqual({
    ip: "1.2.3.4",
    port: 8080,
  });
});

test("parseEndpointIP handles bare and bracketed IPv6 without splitting mid-address", () => {
  expect(parseEndpointIP("::1")).toEqual({ ip: "::1", port: null });
  expect(parseEndpointIP("[::1]:443")).toEqual({ ip: "::1", port: 443 });
  expect(parseEndpointIP("2001:db8::1")).toEqual({
    ip: "2001:db8::1",
    port: null,
  });
  // 裸 IPv6 自身含多个冒号：不得在最后一个冒号处拆开
  expect(parseEndpointIP("fe80:0:0:0:0:0:0:1")).toEqual({
    ip: "fe80:0:0:0:0:0:0:1",
    port: null,
  });
});

test("parseEndpointIP rejects DNS names and malformed endpoints", () => {
  expect(parseEndpointIP("api.example.com")).toBeNull();
  expect(parseEndpointIP("api.example.com:443")).toBeNull();
  expect(parseEndpointIP("")).toBeNull();
  expect(parseEndpointIP("1.2.3.4:https")).toBeNull();
  expect(parseEndpointIP("1.2.3.4:70000")).toBeNull();
  expect(parseEndpointIP("999.1.1.1:80")).toBeNull();
});

// ── deriveRenewEnforcementTargets：从事件详情推导可用处置动作 ──
test("deriveRenewEnforcementTargets offers IP/port for network events and exec path for absolute paths", () => {
  expect(
    deriveRenewEnforcementTargets(
      { net_endpoint: "10.0.0.5:443", path: "/usr/bin/curl" },
      "network",
    ),
  ).toEqual({ ip: "10.0.0.5", port: 443, execPath: "/usr/bin/curl" });
  // 端点为域名：不提供 IP 阻断，避免把主机名发给 IP 端点
  expect(
    deriveRenewEnforcementTargets(
      { net_endpoint: "api.example.com:443" },
      "network",
    ),
  ).toEqual({ ip: null, port: null, execPath: null });
  // 无端点时回退到 dst_ip/dst_port
  expect(
    deriveRenewEnforcementTargets(
      { dst_ip: "10.0.0.6", dst_port: 8080 },
      "network",
    ),
  ).toEqual({ ip: "10.0.0.6", port: 8080, execPath: null });
  // 相对路径或空事件：无处置动作
  expect(deriveRenewEnforcementTargets({ path: "curl" }, "process")).toEqual({
    ip: null,
    port: null,
    execPath: null,
  });
  expect(deriveRenewEnforcementTargets(null, "network")).toEqual({
    ip: null,
    port: null,
    execPath: null,
  });
});

// ── useRenewEnforcement：状态拉取与处置动作 ──
const statusPayload = () => ({
  available: true,
  blockedIPs: ["10.0.0.5"],
  blockedPorts: [443],
});

function mockBackend(
  overrides: {
    status?: () => unknown;
    lsm?: () => unknown;
    runtime?: () => unknown;
  } = {},
) {
  const posts: Array<{ url: string; body: Record<string, unknown> }> = [];
  let statusFetches = 0;
  axios.get = (async (url: string) => {
    if (url === "/sandbox/cgroup/status") {
      statusFetches += 1;
      return { data: overrides.status ? overrides.status() : statusPayload() };
    }
    if (url === "/sandbox/lsm/status") {
      return {
        data: overrides.lsm
          ? overrides.lsm()
          : { available: true, blockedExecPaths: ["/usr/bin/curl"] },
      };
    }
    if (url === "/config/runtime") {
      return {
        data: overrides.runtime
          ? overrides.runtime()
          : { runtime: { policyManagementEnabled: true } },
      };
    }
    throw new Error(`unexpected GET ${url}`);
  }) as any;
  axios.post = (async (url: string, body: Record<string, unknown>) => {
    posts.push({ url, body });
    return { data: {} };
  }) as any;
  return {
    posts,
    statusFetchCount: () => statusFetches,
  };
}

test("fetchStatus populates blocked lists, availability and policy gate", async () => {
  mockBackend();
  const c = useRenewEnforcement();
  expect(c.blockedIPs.value).toEqual([]);
  expect(c.available.value).toBe(false);
  await c.fetchStatus();
  expect(c.blockedIPs.value).toEqual(["10.0.0.5"]);
  expect(c.blockedPorts.value).toEqual([443]);
  expect(c.blockedExecPaths.value).toEqual(["/usr/bin/curl"]);
  expect(c.available.value).toBe(true);
  expect(c.policyManagementEnabled.value).toBe(true);
  expect(c.error.value).toBe("");
});

test("toggle blocks unknown targets and unblocks listed ones with the right JSON body", async () => {
  const { posts } = mockBackend();
  const c = useRenewEnforcement();
  await c.fetchStatus();

  // 10.0.0.5 已在 blockedIPs：走 unblock-ip；10.0.0.7 不在：走 block-ip
  await c.toggleIP("10.0.0.5");
  await c.toggleIP("10.0.0.7");
  expect(posts).toContainEqual({
    url: "/sandbox/cgroup/unblock-ip",
    body: { ip: "10.0.0.5" },
  });
  expect(posts).toContainEqual({
    url: "/sandbox/cgroup/block-ip",
    body: { ip: "10.0.0.7" },
  });

  await c.togglePort(443);
  await c.togglePort(8080);
  expect(posts).toContainEqual({
    url: "/sandbox/cgroup/unblock-port",
    body: { port: 443 },
  });
  expect(posts).toContainEqual({
    url: "/sandbox/cgroup/block-port",
    body: { port: 8080 },
  });

  await c.toggleExecPath("/usr/bin/curl");
  await c.toggleExecPath("/usr/bin/nc");
  expect(posts).toContainEqual({
    url: "/sandbox/lsm/unblock-exec-path",
    body: { path: "/usr/bin/curl" },
  });
  expect(posts).toContainEqual({
    url: "/sandbox/lsm/block-exec-path",
    body: { path: "/usr/bin/nc" },
  });
});

test("403 policy_management produces the specific message and leaves lists untouched", async () => {
  mockBackend();
  const c = useRenewEnforcement();
  await c.fetchStatus();
  expect(c.policyManagementEnabled.value).toBe(true);

  axios.post = (async () => {
    throw {
      response: {
        status: 403,
        data: {
          error: "policy_management is disabled",
          feature: "policy_management",
        },
      },
    };
  }) as any;

  await c.toggleIP("10.0.0.9");
  expect(c.error.value).toBe(POLICY_GATE_MESSAGE);
  // 列表不因失败而改变
  expect(c.blockedIPs.value).toEqual(["10.0.0.5"]);
  expect(c.blockedPorts.value).toEqual([443]);

  // 门控关闭时不再发请求
  axios.get = (async (url: string) => {
    if (url === "/config/runtime") {
      return { data: { runtime: { policyManagementEnabled: false } } };
    }
    throw new Error(`unexpected GET ${url}`);
  }) as any;
  await c.fetchStatus();
  expect(c.policyManagementEnabled.value).toBe(false);
  const before = c.blockedIPs.value.length;
  await c.toggleIP("10.0.0.9");
  expect(c.blockedIPs.value.length).toBe(before);
  expect(c.error.value).toBe("");
});

test("failing cgroup status never throws and keeps available false", async () => {
  mockBackend({
    status: () => {
      throw new Error("offline backend");
    },
    lsm: () => {
      throw new Error("offline backend");
    },
    runtime: () => {
      throw new Error("offline backend");
    },
  });
  const c = useRenewEnforcement();
  await c.fetchStatus();
  expect(c.available.value).toBe(false);
  expect(c.blockedIPs.value).toEqual([]);
  expect(c.blockedPorts.value).toEqual([]);
  expect(c.blockedExecPaths.value).toEqual([]);
  expect(c.policyManagementEnabled.value).toBe(false);
  expect(c.error.value).not.toBe("");
});

test("a successful action re-fetches status so the list reflects backend truth", async () => {
  const { statusFetchCount } = mockBackend();
  const c = useRenewEnforcement();
  await c.fetchStatus();
  const before = statusFetchCount();
  await c.toggleIP("10.0.0.7");
  expect(statusFetchCount()).toBeGreaterThan(before);
});
