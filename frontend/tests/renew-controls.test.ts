import { expect, test, afterEach } from "bun:test";
import axios from "axios";
import { useRenewMonitoringControls } from "../src/composables/renew/useRenewMonitoringControls";
import { RENEW_MONITORING_MODULES } from "../src/composables/renew/monitoringPresets";
const originalGet = axios.get;
const originalPut = axios.put;
afterEach(() => {
  axios.get = originalGet;
  axios.put = originalPut;
});
const runtime = () => ({
  disabledEventTypes: [],
  loopDetection: { enabled: true },
  signalProcessing: { enabled: true },
  researchProcessing: { enabled: false },
  tlsCaptureEnabled: false,
  logPersistenceEnabled: true,
});
function mockState() {
  let state: any = runtime();
  const updates: any[] = [];
  axios.get = (async (url: string) => ({
    data:
      url === "/config/event-types"
        ? { disabled_event_types: state.disabledEventTypes }
        : url === "/config/runtime"
          ? { runtime: state }
          : { captureHealthy: true },
  })) as any;
  axios.put = (async (_url: string, patch: any) => {
    updates.push(patch);
    state = { ...state, ...patch };
    return { data: { runtime: state } };
  }) as any;
  return updates;
}
test("controls unavailable until config loads; failed load never claims all enabled", async () => {
  const c = useRenewMonitoringControls();
  expect(c.ready.value).toBe(false);
  expect(c.enabledModuleKeys.value).toEqual([]);
  axios.get = (async () => {
    throw new Error("offline");
  }) as any;
  await c.fetchState();
  await c.setModuleEnabled("process", false);
  expect(c.ready.value).toBe(false);
  expect(c.error.value).toBe("offline");
});
test("module switch persists backend state and survives reloading config", async () => {
  mockState();
  const c = useRenewMonitoringControls();
  await c.fetchState();
  await c.setModuleEnabled("process", false);
  expect(c.enabledModuleKeys.value).not.toContain("process");
  expect(
    RENEW_MONITORING_MODULES[0]!.eventTypes.every((t) =>
      c.disabledEventTypes.value.has(t),
    ),
  ).toBe(true);
  await c.fetchState();
  expect(c.enabledModuleKeys.value).not.toContain("process");
  await c.setModuleEnabled("process", true);
  expect(c.enabledModuleKeys.value).toContain("process");
});
test("failed mutation preserves confirmed switch position and shows error", async () => {
  mockState();
  const c = useRenewMonitoringControls();
  await c.fetchState();
  axios.put = (async () => {
    throw new Error("403 forbidden");
  }) as any;
  await expect(c.setModuleEnabled("process", false)).rejects.toThrow();
  expect(c.enabledModuleKeys.value).toContain("process");
  expect(c.applying.value).toBe(false);
  expect(c.error.value).toBe("403 forbidden");
});
test("profile is one update, preserves unrelated disabled types and opt-in TLS", async () => {
  const updates = mockState();
  const c = useRenewMonitoringControls();
  await c.fetchState();
  c.disabledEventTypes.value.add(9999);
  await c.applyProfile("daily");
  expect(updates).toHaveLength(1);
  expect(updates[0].disabledEventTypes).toContain(9999);
  expect(updates[0]).not.toHaveProperty("tlsCaptureEnabled");
  expect(c.runtimeEnabled("tlsCapture")).toBe(false);
  expect(c.activeProfileKey.value).toBe("daily");
});
test("rapid writes are ignored while one update is in flight", async () => {
  mockState();
  const c = useRenewMonitoringControls();
  await c.fetchState();
  let release: any;
  let calls = 0;
  axios.put = (async (_u: any, patch: any) => {
    calls++;
    await new Promise((r) => (release = r));
    return { data: { runtime: { ...runtime(), ...patch } } };
  }) as any;
  const first = c.setModuleEnabled("process", false);
  await c.setModuleEnabled("network", false);
  expect(calls).toBe(1);
  release();
  await first;
});
