import { expect, test, afterEach, afterAll } from "bun:test";
import axios from "axios";
import { message as antdMessage } from "ant-design-vue";
import { renewToast } from "../src/composables/renew/notify";
import { useRenewRules } from "../src/composables/renew/useRenewRules";
import { useRenewMonitoringControls } from "../src/composables/renew/useRenewMonitoringControls";

// The composables confirm through `renewToast`, whose method is swapped for a
// recorder. Module-level `mock.module` is avoided on purpose: it would leak into
// the other renew test files sharing this bun process.
const originalToast = renewToast.success;
const successCalls: string[] = [];
renewToast.success = (content: string) => {
  successCalls.push(content);
};
afterAll(() => {
  renewToast.success = originalToast;
});

const originalGet = axios.get;
const originalPost = axios.post;
const originalPut = axios.put;
const originalDelete = axios.delete;
afterEach(() => {
  successCalls.length = 0;
  axios.get = originalGet;
  axios.post = originalPost;
  axios.put = originalPut;
  axios.delete = originalDelete;
});

const axiosError = (text: string) => {
  const cause = new Error(text);
  // Test fixture: axios rejections carry the backend text under
  // `response.data.error`, which is exactly what both composables read.
  const withResponse = cause as Error & {
    response: { data: { error: string } };
  };
  withResponse.response = { data: { error: text } };
  return withResponse;
};

type PostCall = { url: string; payload: Record<string, unknown> };
type DeleteCall = { url: string };
type RulesOverrides = {
  post?: (url: string, payload: Record<string, unknown>) => Promise<unknown>;
  delete?: (url: string) => Promise<unknown>;
};

const mockRulesApi = (
  rules: Record<string, unknown>,
  overrides: RulesOverrides = {},
) => {
  const posts: PostCall[] = [];
  const deletes: DeleteCall[] = [];
  axios.get = (async () => ({ data: rules })) as unknown as typeof axios.get;
  axios.post = (async (url: string, payload: Record<string, unknown>) => {
    posts.push({ url, payload });
    return overrides.post ? overrides.post(url, payload) : { data: {} };
  }) as unknown as typeof axios.post;
  axios.delete = (async (url: string) => {
    deletes.push({ url });
    return overrides.delete ? overrides.delete(url) : { data: {} };
  }) as unknown as typeof axios.delete;
  return { posts, deletes };
};

// useRenewRules loads in onMounted, which never fires outside a component, so the
// tests drive load() themselves before mutating.
const loadedRules = async () => {
  const rules = useRenewRules();
  await rules.load();
  return rules;
};

test("successful rule save posts the payload and confirms the command name", async () => {
  const { posts } = mockRulesApi({});
  const rules = await loadedRules();
  rules.comm.value = " curl ";
  rules.action.value = "ALERT";
  rules.priority.value = 7;

  await rules.save();

  expect(posts).toHaveLength(1);
  expect(posts[0]!.url).toBe("/config/rules");
  expect(posts[0]!.payload).toEqual({
    comm: "curl",
    action: "ALERT",
    regex: "",
    replacement: "",
    priority: 7,
    rewritten_cmd: [],
  });
  expect(successCalls.join("|")).toContain("curl");
  expect(rules.error.value).toBe("");
});

test("successful rule delete confirms the removed command", async () => {
  const { deletes } = mockRulesApi({});
  const rules = await loadedRules();
  rules.pendingDelete.value = "nc open";

  await rules.remove();

  expect(deletes).toHaveLength(1);
  expect(deletes[0]!.url).toBe("/config/rules/nc%20open");
  expect(successCalls.join("|")).toContain("nc open");
  expect(rules.error.value).toBe("");
});

test("failed save surfaces the backend error and stays silent about success", async () => {
  mockRulesApi(
    {},
    {
      post: async () => {
        throw axiosError("规则已存在");
      },
    },
  );
  const rules = await loadedRules();
  rules.comm.value = "curl";

  await rules.save();

  expect(rules.error.value).toBe("规则已存在");
  expect(successCalls).toHaveLength(0);
});

test("failed delete surfaces the backend error and stays silent about success", async () => {
  mockRulesApi(
    {},
    {
      delete: async () => {
        throw axiosError("规则不存在");
      },
    },
  );
  const rules = await loadedRules();
  rules.pendingDelete.value = "curl";

  await rules.remove();

  expect(rules.error.value).toBe("规则不存在");
  expect(successCalls).toHaveLength(0);
});

type RuntimeState = {
  disabledEventTypes: number[];
  loopDetection: { enabled: boolean };
  signalProcessing: { enabled: boolean };
  researchProcessing: { enabled: boolean };
  tlsCaptureEnabled: boolean;
  logPersistenceEnabled: boolean;
};
type RuntimeOverrides = {
  put?: (
    url: string,
    patch: Record<string, unknown>,
  ) => Promise<{ data: Record<string, unknown> }>;
};

const mockRuntimeApi = (overrides: RuntimeOverrides = {}) => {
  const updates: Record<string, unknown>[] = [];
  const state: RuntimeState = {
    disabledEventTypes: [],
    loopDetection: { enabled: true },
    signalProcessing: { enabled: true },
    researchProcessing: { enabled: false },
    tlsCaptureEnabled: false,
    logPersistenceEnabled: true,
  };
  axios.get = (async () => ({
    data: { runtime: state },
  })) as unknown as typeof axios.get;
  axios.put = (async (_url: string, patch: Record<string, unknown>) => {
    if (overrides.put) return overrides.put(_url, patch);
    updates.push(patch);
    Object.assign(state, patch);
    return { data: { runtime: state } };
  }) as unknown as typeof axios.put;
  return updates;
};

test("monitoring module and runtime toggles confirm success", async () => {
  mockRuntimeApi();
  const controls = useRenewMonitoringControls();
  await controls.fetchState();

  await controls.setModuleEnabled("process", false);
  expect(successCalls.join("|")).toContain("process");

  await controls.setRuntimeEnabled("tlsCapture", true);
  expect(successCalls.join("|")).toContain("tlsCapture");

  expect(controls.error.value).toBe("");
});

test("applying a monitoring profile confirms success", async () => {
  const updates = mockRuntimeApi();
  const controls = useRenewMonitoringControls();
  await controls.fetchState();

  await controls.applyProfile("daily");

  expect(updates).toHaveLength(1);
  expect(successCalls.join("|")).toContain("daily");
  expect(controls.error.value).toBe("");
});

test("failed monitoring mutation surfaces the error, rejects, and never confirms", async () => {
  mockRuntimeApi({
    put: async () => {
      throw axiosError("配置校验失败");
    },
  });
  const controls = useRenewMonitoringControls();
  await controls.fetchState();

  await expect(controls.setModuleEnabled("process", false)).rejects.toThrow();
  await expect(
    controls.setRuntimeEnabled("tlsCapture", true),
  ).rejects.toThrow();
  await expect(controls.applyProfile("daily")).rejects.toThrow();

  expect(controls.error.value).toBe("配置校验失败");
  expect(successCalls).toHaveLength(0);
  // Switch positions stay at their last confirmed state, so the template renders
  // backend truth rather than the rejected toggle.
  expect(controls.enabledModuleKeys.value).toContain("process");
  expect(controls.runtimeEnabled("tlsCapture")).toBe(false);
});

test("setStatsInterval stays local: no request, no toast", async () => {
  const updates = mockRuntimeApi();
  const controls = useRenewMonitoringControls();
  await controls.fetchState();

  controls.setStatsInterval(30_000);

  expect(updates).toHaveLength(0);
  expect(successCalls).toHaveLength(0);
  expect(controls.statsIntervalMs.value).toBe(30_000);
});

test("renewToast delivers inside a DOM and stays silent outside one", () => {
  // Guards the seam itself: antd mounts its holder into `document`, so the toast
  // must be dropped rather than thrown outside a DOM.
  const antdSurface = antdMessage as unknown as {
    success: (c: string) => void;
  };
  const realAntSuccess = antdSurface.success;
  // `originalToast` is the real guarded implementation; `renewToast.success`
  // is the recorder installed above.
  const guardedSuccess = originalToast;
  const antdCalls: string[] = [];
  antdSurface.success = (content: string) => {
    antdCalls.push(content);
  };
  const globals = globalThis as { document?: unknown };
  const realDocument = globals.document;

  globals.document = {};
  guardedSuccess("规则已保存：curl");
  expect(antdCalls).toEqual(["规则已保存：curl"]);

  delete globals.document;
  expect(() => guardedSuccess("规则已删除：nc")).not.toThrow();
  expect(antdCalls).toHaveLength(1);

  if (realDocument === undefined) delete globals.document;
  else globals.document = realDocument;
  antdSurface.success = realAntSuccess;
  renewToast.success = (content: string) => {
    successCalls.push(content);
  };
});
