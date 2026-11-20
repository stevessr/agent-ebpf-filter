/** Browser acceptance using a local transport fixture; no live kernel or credentials.
 * Run with Bun, PLAYWRIGHT_MODULE and a running project frontend.
 */
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import { pb } from "../src/pb/tracker_pb.js";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright-core");
const base = process.env.BEHAVIOR_URL || "http://127.0.0.1:5173";
const out = process.env.BEHAVIOR_SCREENSHOTS || "../reports/behavior";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium", headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1400 }, colorScheme: "light" });
const errors = [];
page.on("pageerror", error => errors.push(error.message));
let sockets = [];
let failHistory = false;
let initialAt = Date.now();
const makeEvent = (id, n, type = "openat", pid = 42) => ({
  eventId: id, ingestTimestampNs: (initialAt + n) * 1e6,
  legacyEvent: { pid, ppid: 1, eventType: type === "network_connect" ? pb.EventType.NETWORK_CONNECT : pb.EventType.OPENAT,
    type, comm: pid === 42 ? "node" : "python", path: "/fixture/" + id, tag: "Agent" },
});
const send = (events) => sockets.at(-1).send(Buffer.from(pb.EventEnvelopeBatch.encode({ envelopes: events }).finish()));
const count = async () => Number((await page.locator(".core-count").textContent()).replaceAll(",", ""));
const poll = async (fn, label) => {
  const deadline = Date.now() + 15000;
  while (!(await fn())) {
    assert.ok(Date.now() < deadline, "Timed out: " + label);
    await new Promise(resolve => setTimeout(resolve, 40));
  }
};
const shot = (name) => page.screenshot({ path: out + "/" + name + ".png" });
try {
  await page.route("**/*", async route => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    if (!["xhr", "fetch"].includes(req.resourceType()) || path.startsWith("/src/") || path.startsWith("/node_modules/") || path.startsWith("/@")) return route.continue();
    if (path === "/events/recent") {
      if (failHistory) return route.fulfill({ status: 503, json: { error: "fixture offline" } });
      return route.fulfill({ contentType: "application/x-protobuf", body: Buffer.from(pb.EventHistoryResponse.encode({ events: [{ envelope: makeEvent("history", -1000) }] }).finish()) });
    }
    return route.fulfill({ json: {} });
  });
  await page.routeWebSocket("**/ws/envelopes*", socket => sockets.push(socket));
  await page.routeWebSocket(/\/ws(?:\?|$|\/tls-capture)/, () => {});
  await page.goto(base + "/behavior?demo=1");
  await page.locator(".behavior-studio").waitFor();
  await page.getByRole("button", { name: "暂停显示", exact: false }).click();
  const demoCount = await count();
  assert.ok(demoCount > 0);
  await page.waitForTimeout(1100);
  assert.equal(await count(), demoCount, "pause freezes replay");
  await page.locator(".stream-event").first().click();
  assert.match(await page.locator(".event-inspector").textContent(), /合成演示/);
  assert.equal(await page.locator(".trace-button").count(), 0, "demo cannot navigate synthetic PID into live evidence");
  await page.getByRole("button", { name: "关闭事件证据" }).click();
  await page.getByLabel("按进程聚焦").selectOption("4101");
  assert.ok(await count() < demoCount);
  await page.locator(".stream-event").first().click();
  assert.match(await page.locator(".event-inspector").textContent(), /PID 4101/);
  await page.getByRole("button", { name: "关闭事件证据" }).click();
  await page.getByLabel("按进程聚焦").selectOption({ label: "全部进程" });
  await page.getByRole("button", { name: /仅需关注/ }).click();
  assert.equal(await page.locator(".orbit-event:not(.attention)").count(), 0);
  await page.getByRole("button", { name: /仅需关注/ }).click();
  await page.getByLabel("观测时间窗").selectOption("30000");
  assert.ok(await count() < demoCount);
  await page.getByLabel("观测时间窗").selectOption("60000");
  await shot("web-light");
  await page.emulateMedia({ colorScheme: "dark" });
  await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
  assert.notEqual(await page.locator(".orbit-panel").evaluate(el => getComputedStyle(el).backgroundColor), "rgb(255, 255, 255)");
  await shot("web-dark");
  await page.getByLabel("Language / 语言").selectOption("en-US");
  await page.getByRole("heading", { name: "Behavior Observatory", exact: true }).waitFor();
  await page.getByRole("button", { name: "Resume", exact: false }).click();
  await page.emulateMedia({ reducedMotion: "reduce" });
  assert.equal(await page.locator(".channel-signal").first().evaluate(el => getComputedStyle(el).animationName), "none");
  for (const width of [375, 768, 1280, 1920]) {
    await page.setViewportSize({ width, height: 1080 });
    await page.waitForTimeout(100);
    assert.ok(await page.locator(".behavior-studio").evaluate(el => el.scrollWidth <= el.clientWidth + 1), "No horizontal overflow at " + width);
    await shot("web-" + width);
    await page.locator(".behavior-footnote").scrollIntoViewIfNeeded();
    assert.ok(await page.locator(".behavior-telemetry").isVisible(), "Telemetry remains reachable at " + width);
    await page.locator(".behavior-title").scrollIntoViewIfNeeded();
  }
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.emulateMedia({ forcedColors: "active" });
  await shot("web-high-contrast");
  await page.emulateMedia({ forcedColors: "none", reducedMotion: "no-preference" });
  await page.getByRole("button", { name: "Live observation", exact: true }).click();
  await poll(async () => sockets.length > 0 && await count() === 1, "history catch-up");
  await page.getByRole("button", { name: "Pause display", exact: false }).click();
  send([makeEvent("live-paused", 0), makeEvent("history", -1000)]);
  await poll(async () => (await page.locator(".pause-indicator").textContent()).includes("1 queued"), "bounded paused reception");
  assert.equal(await count(), 1);
  await page.getByRole("button", { name: "Resume", exact: false }).click();
  await poll(async () => await count() === 2, "resume drains without history duplicate");
  await page.locator(".orbit-event").first().focus();
  await page.keyboard.press("Enter");
  await page.locator(".event-inspector").waitFor();
  assert.match(await page.locator(".event-inspector").textContent(), /Captured event/);
  await page.getByRole("button", { name: "Close event evidence" }).click();
  failHistory = true;
  sockets.at(-1).close();
  await poll(async () => (await page.locator(".behavior-status").textContent()).includes("History recovery failed"), "failed history is visible after reconnect");
  assert.equal(await count(), 2);
  failHistory = false;
  await page.getByRole("button", { name: "Retry", exact: false }).click();
  await poll(async () => !(await page.locator(".behavior-status").textContent()).includes("History recovery failed"), "history retry recovers");
  assert.equal(await count(), 2);
  assert.deepEqual(errors, []);
  console.log("PASS: replay isolation, pause/resume, PID/window/attention filters, keyboard evidence, live protobuf history/deduplication, reconnect/retry, themes, English, reduced motion, high contrast, 375/768/1280/1920 widths");
} finally { await browser.close(); }
