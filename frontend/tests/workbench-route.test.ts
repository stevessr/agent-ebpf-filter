import { describe, expect, test } from "bun:test";
import {
  createRouter,
  createMemoryHistory,
  loadRouteLocation,
} from "vue-router";
import { defineComponent } from "vue";
import { resolvePanelRoute } from "../src/composables/workbench/resolvePanelRoute";
import { FRONTEND_BUILD_FEATURE_MODE } from "../src/config/featureFlags";
const View = defineComponent({ template: "<div>panel</div>" });
const router = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", redirect: "/dashboard" },
      { path: "/dashboard", name: "Dashboard", component: async () => View },
      { path: "/config/:tab?", name: "Config", component: async () => View },
      {
        path: "/legacy/:tab?",
        redirect: (to) => ({ name: "Config", params: { tab: to.params.tab } }),
      },
      {
        path: "/tls-capture",
        component: async () => View,
        meta: { feature: "tls_capture" },
      },
      {
        path: "/feature-unavailable",
        name: "FeatureUnavailable",
        component: View,
      },
    ],
  });
describe("isolated panel routing", () => {
  test("resolves root and legacy functional redirects before rendering", () => {
    const r = router();
    expect(resolvePanelRoute(r, "/").fullPath).toBe("/dashboard");
    expect(resolvePanelRoute(r, "/legacy/runtime").fullPath).toBe(
      "/config/runtime",
    );
  });
  test("resolves relative query navigation against the pane, not global URL", async () => {
    const r = router();
    await r.push("/dashboard");
    const left = resolvePanelRoute(r, "/config/runtime");
    const right = resolvePanelRoute(r, "/config/security");
    expect(
      resolvePanelRoute(r, { query: { inspect: "1" } }, left).fullPath,
    ).toBe("/config/runtime?inspect=1");
    expect(right.fullPath).toBe("/config/security");
    expect(r.currentRoute.value.fullPath).toBe("/dashboard");
  });
  test("restored inactive route explicitly loads its lazy component", async () => {
    const r = router();
    await r.push("/config/runtime");
    const loaded = await loadRouteLocation(resolvePanelRoute(r, "/dashboard"));
    expect(loaded.matched[0]?.components?.default).toBe(View);
  });
  test("explicit pane routes respect build feature gates", () => {
    const result = resolvePanelRoute(router(), "/tls-capture");
    if (FRONTEND_BUILD_FEATURE_MODE === "core") {
      expect(result.name).toBe("FeatureUnavailable");
      expect(result.query.feature).toBe("tls_capture");
    } else expect(result.path).toBe("/tls-capture");
  });
});
