/** Local transport fixture, not a live-kernel test. Requires a running Vite
 * frontend and playwright-core (or PLAYWRIGHT_MODULE pointing to its entry).
 * Run with Bun so the generated protobuf module resolves its extensionless import.
 */
import assert from "node:assert/strict";
import { pb } from "../src/pb/tracker_pb.js";
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE || "playwright-core"
);
const base = process.env.DASHBOARD_URL || "http://127.0.0.1:5173";
const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium",
  headless: true,
});
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));
let sockets = [];
let historyCalls = 0;
let failHistory = false;
const makeEvent = (id, n) => ({
  eventId: id,
  ingestTimestampNs: (1_700_000_000_000 + n) * 1e6,
  legacyEvent: {
    pid: 42,
    eventType: 1,
    type: "openat",
    comm: "node",
    path: `/fixture/${id}`,
    tag: "agent",
  },
});
let history = [makeEvent("initial", 1)];
const send = (socket, events) =>
  socket.send(
    Buffer.from(pb.EventEnvelopeBatch.encode({ envelopes: events }).finish()),
  );
const waitFor = async (fn, label) => {
  const deadline = Date.now() + 15000;
  while (!(await fn())) {
    if (Date.now() > deadline) throw new Error(`Timed out: ${label}`);
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
};
try {
  // All API traffic is fixture-only; no backend credentials or mutations.
  await page.route("**/*", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (
      !["xhr", "fetch"].includes(request.resourceType()) ||
      path.startsWith("/src/") ||
      path.startsWith("/node_modules/") ||
      path.startsWith("/@")
    )
      return route.continue();
    if (path === "/events/recent") {
      historyCalls++;
      assert.equal(new URL(request.url()).searchParams.get("limit"), "1000");
      if (failHistory)
        return route.fulfill({
          status: 503,
          json: { error: "fixture unavailable" },
        });
      // Mirror the backend's UseProtoNames JSON fallback on manual retry.
      if (historyCalls === 4)
        return route.fulfill({
          json: {
            events: history.map((event) => ({
              Envelope: {
                event_id: event.eventId,
                ingest_timestamp_ns: String(event.ingestTimestampNs),
                legacy_event: event.legacyEvent,
              },
            })),
          },
        });
      return route.fulfill({
        contentType: "application/x-protobuf",
        body: Buffer.from(
          pb.EventHistoryResponse.encode({
            events: history.map((envelope) => ({ envelope })),
          }).finish(),
        ),
      });
    }
    if (path === "/data/clear-events-memory")
      return route.fulfill({ status: 403, json: { error: "fixture denied" } });
    if (path === "/config/tags") return route.fulfill({ json: [] });
    return route.fulfill({ json: {} });
  });
  await page.routeWebSocket("**/ws/envelopes*", (socket) => {
    sockets.push(socket);
  });
  // No other application socket may reach an actual backend during the fixture.
  await page.routeWebSocket(/\/ws(?:\?|$|\/tls-capture)/, () => {});
  await page.goto(`${base}/dashboard`);
  await page.locator(".triage").waitFor();
  const count = async () =>
    Number(
      (
        await page
          .locator(".triage-metrics > div")
          .last()
          .locator("strong")
          .innerText()
      ).replaceAll(",", ""),
    );
  await waitFor(
    async () => (await count()) === 1 && sockets.length === 1,
    "initial snapshot",
  );
  await page.getByRole("button", { name: "暂停显示", exact: true }).click();
  send(sockets[0], [makeEvent("paused", 2)]);
  await page
    .locator(".dashboard-stream-notice")
    .filter({ hasText: "待显示 1 条" })
    .waitFor();
  assert.equal(await count(), 1);
  await page.getByRole("button", { name: "恢复显示", exact: true }).click();
  await waitFor(async () => (await count()) === 2, "resume buffered event");

  history = [...history, makeEvent("paused", 2), makeEvent("missed", 3)];
  sockets[0].close({ code: 1012, reason: "fixture restart" });
  await waitFor(
    () => sockets.length === 2 && historyCalls === 2,
    "automatic reconnect and refetch",
  );
  send(sockets[1], [makeEvent("missed", 3), makeEvent("live", 4)]);
  await waitFor(
    async () => (await count()) === 4,
    "stable-ID dedup across reconnect overlap",
  );
  assert.match(
    await page.locator(".dashboard-stream-notice").innerText(),
    /无法保证间隙完整/,
  );

  failHistory = true;
  sockets[1].close({ code: 1012, reason: "fixture failure" });
  await page
    .getByRole("button", { name: "重试补取", exact: true })
    .waitFor({ timeout: 15000 });
  assert.equal(await count(), 4);
  history.push(makeEvent("live", 4));
  failHistory = false;
  await page.getByRole("button", { name: "重试补取", exact: true }).click();
  await waitFor(
    async () =>
      historyCalls === 4 &&
      (await page
        .getByRole("button", { name: "重试补取", exact: true })
        .count()) === 0,
    "manual retry",
  );
  assert.equal(await count(), 4);
  await page.getByRole("button", { name: "查看事件明细", exact: true }).click();
  assert.match(
    await page.locator(".dashboard-stream-notice").innerText(),
    /无法保证间隙完整/,
  );
  await page.getByRole("button", { name: "Clear Events", exact: true }).click();
  await page
    .getByText("Failed to clear events; local events retained", { exact: true })
    .waitFor();
  assert.match(
    await page.locator(".dashboard-page").innerText(),
    /Total Events: 4/,
  );
  await page.setViewportSize({ width: 600, height: 800 });
  assert.equal(
    await page.locator(".dashboard-stream-notice").isVisible(),
    true,
  );
  await page.screenshot({
    path: process.env.DASHBOARD_SCREENSHOT || "/tmp/agent-dashboard-stream.png",
    fullPage: true,
  });
  assert.deepEqual(errors, []);
  console.log(
    "PASS: real Chromium UI → fixture WebSocket/protobuf/history → bounded pause/resume → automatic reconnect/dedup → failed snapshot/retry → failed clear retains evidence → raw/narrow continuity notice; no uncaught browser errors.",
  );
} catch (error) {
  console.error({
    sockets: sockets.length,
    historyCalls,
    errors,
    body: (await page.locator("body").innerText()).slice(0, 5000),
  });
  throw error;
} finally {
  await browser.close();
}
