// Deep-link contract tests for useRenewDashboard's ?event= query-param sync.
// Uses a REAL vue-router (createRouter + createMemoryHistory). useRenewDashboard
// calls useRoute/useRouter, which resolve via Vue injection; we activate the
// composable inside app.runWithContext() after app.use(router) so the router's
// app-level provides (routerKey / routeLocationKey) are visible. No hand-rolled
// route stub. axios.get is swapped with a counting fake (same technique as
// tests/renew-controls.test.ts).
import { afterEach, expect, test } from "bun:test";
import { createApp, inject, type Ref } from "vue";
import {
  createMemoryHistory,
  createRouter,
  routeLocationKey,
  type RouteLocationNormalizedLoaded,
  type Router,
} from "vue-router";
import axios from "axios";
import { useRenewDashboard } from "../src/composables/renew/useRenewDashboard";

// Structural view of the dashboard surface under test; useRenewDashboard's
// return value satisfies it without a cast.
interface DashboardHandle {
  selectedEventID: Ref<string>;
  harness: Ref<string>;
  openEvent: (eventId: string) => void;
  closeEventDetail: () => void;
}

const originalGet = axios.get;
afterEach(() => {
  axios.get = originalGet;
});

function mockDetailEndpoint(): string[] {
  const calls: string[] = [];
  axios.get = (async (url: string) => {
    calls.push(String(url));
    return { data: { eventId: "whatever", note: "full record" } };
    // AxiosResponse requires status/headers/etc.; the composable only reads
    // `response.data`, so a structural fake is impossible without this cast.
  }) as unknown as typeof axios.get;
  return calls;
}

function createRouterFor(initialUrl: string): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/:section?",
        name: "Renew",
        component: { render: () => null },
      },
    ],
  });
  router.push(initialUrl);
  return router;
}

// Activates useRenewDashboard inside a real router context (app-level provides
// resolved through app.runWithContext), and hands back the live reactive route.
function setup(initialUrl: string): {
  dash: DashboardHandle;
  route: RouteLocationNormalizedLoaded;
  router: Router;
} {
  const router = createRouterFor(initialUrl);
  const app = createApp({ render: () => null });
  app.use(router);
  let dash: DashboardHandle | undefined;
  let route: RouteLocationNormalizedLoaded | undefined;
  app.runWithContext(() => {
    dash = useRenewDashboard();
    route = inject(routeLocationKey);
  });
  return { dash: dash!, route: route!, router };
}

// Resolves when the next vue-router navigation settles; used instead of
// wall-clock waits because the composable's URL writes are promise-based.
function nextNavigation(router: Router): Promise<void> {
  const { promise, resolve } = Promise.withResolvers<void>();
  const stop = router.afterEach(() => {
    stop();
    resolve();
  });
  return promise;
}

// Flushes the Vue scheduler (watch callbacks run on a microtask after the
// navigation commits) without touching the real clock.
async function flushWatches(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

test("cold load with ?event=<id> opens exactly that event once", async () => {
  const calls = mockDetailEndpoint();
  const { dash, router } = setup("/renew?event=evt-1");
  await router.isReady();
  await flushWatches();
  expect(dash.selectedEventID.value).toBe("evt-1");
  expect(calls.filter((u) => u.includes("/events/detail/")).length).toBe(1);
  expect(calls[0]).toContain(encodeURIComponent("evt-1"));
});

test("openEvent writes ?event= into the URL and selects the event", async () => {
  mockDetailEndpoint();
  const { dash, route, router } = setup("/renew");
  await router.isReady();
  await flushWatches();
  expect(dash.selectedEventID.value).toBe("");

  dash.openEvent("evt-2");
  await nextNavigation(router);
  await flushWatches();
  expect(route.query.event).toBe("evt-2");
  expect(dash.selectedEventID.value).toBe("evt-2");
});

test("closeEventDetail removes ?event= but preserves harness and other params", async () => {
  mockDetailEndpoint();
  const { dash, route, router } = setup(
    "/renew?harness=claude&event=evt-3&foo=bar",
  );
  await router.isReady();
  await flushWatches();
  expect(dash.selectedEventID.value).toBe("evt-3");

  dash.closeEventDetail();
  await nextNavigation(router);
  await flushWatches();
  expect(route.query.event).toBeUndefined();
  expect(route.query.harness).toBe("claude");
  expect(route.query.foo).toBe("bar");
});

test("URL change to another event loads the new event (watcher path)", async () => {
  const calls = mockDetailEndpoint();
  const { dash, route, router } = setup("/renew?event=evt-4");
  await router.isReady();
  await flushWatches();
  expect(dash.selectedEventID.value).toBe("evt-4");

  await router.push("/renew?event=evt-9");
  await flushWatches();
  expect(route.query.event).toBe("evt-9");
  expect(dash.selectedEventID.value).toBe("evt-9");
  expect(calls.filter((u) => u.includes("evt-9")).length).toBe(1);
});

test("blank or whitespace ?event= clears the drawer without throwing", async () => {
  const calls = mockDetailEndpoint();
  const { dash, route, router } = setup("/renew?event=");
  await router.isReady();
  await flushWatches();
  expect(dash.selectedEventID.value).toBe("");
  expect(calls.length).toBe(0);

  // Mid-session: a whitespace-only ?event= must clear an open drawer.
  const {
    dash: dash2,
    route: route2,
    router: router2,
  } = setup("/renew?event=evt-5");
  await router2.isReady();
  await flushWatches();
  expect(dash2.selectedEventID.value).toBe("evt-5");

  await router2.push("/renew?event=%20%20");
  await flushWatches();
  expect(route2.query.event).toBe("  ");
  expect(dash2.selectedEventID.value).toBe("");
});

test("re-selecting the id already in the URL does not re-fetch", async () => {
  const calls = mockDetailEndpoint();
  const { dash, route, router } = setup("/renew?event=evt-6");
  await router.isReady();
  await flushWatches();
  expect(calls.length).toBe(1);

  dash.openEvent("evt-6");
  await flushWatches();
  // Guard: same id already selected AND already in the URL → no second fetch,
  // and the redundant router.replace must not touch the query either.
  expect(calls.length).toBe(1);
  expect(dash.selectedEventID.value).toBe("evt-6");
  expect(route.query.event).toBe("evt-6");
});

test("harness param is untouched by event sync", async () => {
  mockDetailEndpoint();
  const { dash, route, router } = setup("/renew?harness=claude");
  await router.isReady();
  await flushWatches();
  const handle = dash;
  expect(handle.harness.value).toBe("claude");
  expect(route.query.event).toBeUndefined();
  expect(route.query.harness).toBe("claude");

  dash.openEvent("evt-7");
  await nextNavigation(router);
  await flushWatches();
  expect(route.query.harness).toBe("claude");
  expect(route.query.event).toBe("evt-7");
});
