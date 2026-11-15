/**
 * Real Chromium harness graph smoke. Uses a deterministic WebSocket fixture,
 * never talks to a production tracker or kernel.
 *
 * Called from the existing WebUI Playwright workflow (Chromium is already
 * installed there); test both GPU rendering and an intentionally denied
 * WebGL context to guarantee the SVG fallback is usable.
 */
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || "playwright");
const base = process.env.WEBUI_URL || "http://127.0.0.1:5173";
const outdir = path.resolve("artifacts/webui-i18n/shard-0");
fs.mkdirSync(outdir, { recursive: true });

const fixture = {
  eventCount: 4,
  source: "fixture",
  nodeCounts: { agent_run: 1, process: 1, syscall: 1, policy_alert: 1 },
  nodes: [
    { id: "run:demo", kind: "agent_run", label: "Demo run" },
    { id: "proc:42", kind: "process", label: "fixture-agent", pid: 42 },
    { id: "evt:getpid", kind: "syscall", label: "getpid", metadata: { type: "getpid" } },
    { id: "alert:1", kind: "policy_alert", label: "BLOCK", riskScore: 96,
      metadata: { decision: "BLOCK" } },
  ],
  edges: [
    { id: "a", source: "run:demo", target: "proc:42", kind: "contains" },
    { id: "b", source: "proc:42", target: "evt:getpid", kind: "observed" },
    { id: "c", source: "proc:42", target: "alert:1", kind: "alerted" },
  ],
};
const browser = await chromium.launch({
  headless: true,
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
});
const results = [];
try {
  for (const disabledWebGL of [false, true]) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    if (disabledWebGL) {
      await page.addInitScript(() => {
        const original = HTMLCanvasElement.prototype.getContext;
        HTMLCanvasElement.prototype.getContext = function (kind, ...args) {
          if (["webgl", "webgl2", "experimental-webgl"].includes(String(kind)))
            return null;
          return original.call(this, kind, ...args);
        };
      });
    }
    await page.routeWebSocket("**/ws/**", () => {});
    await page.routeWebSocket("**/ws/events/graph*", (socket) => {
      // Let the Vue client install its onmessage listener before delivery.
      setTimeout(() => socket.send(JSON.stringify(fixture)), 500);
    });
    try {
      await page.goto(new URL("/execution-graph/topology", base).toString(), {
        waitUntil: "domcontentloaded", timeout: 30000,
      });
      await page.locator(".execution-graph-page").waitFor({ timeout: 25000 });
      await page.getByText(/已折叠 1 节点/).first().waitFor({ timeout: 25000 });
      // Offscreen graph rendering is intentionally suspended; bring the
      // canvas into view before asserting its GPU state.
      const canvasHost = page.locator(".harness-3d, .execution-graph-canvas").first();
      await canvasHost.scrollIntoViewIfNeeded();
      await page.waitForFunction(() => {
        const overlay = document.querySelector(".harness-3d-overlay");
        const svgNodes = document.querySelectorAll(".execution-graph-svg g.execution-node").length;
        return (overlay && overlay.textContent?.includes("3 nodes")) || svgNodes === 3;
      }, null, { timeout: 15000 });

      const hasWebGLCanvas = await page.locator(".harness-3d canvas").count() > 0;
      const hasSvgCanvas = await page.locator(".execution-graph-canvas svg").count() > 0;
      assert(hasWebGLCanvas || hasSvgCanvas, "no usable 3D or SVG canvas");
      if (disabledWebGL) {
        assert(!hasWebGLCanvas && hasSvgCanvas, "WebGL-disabled view did not fall back to SVG");
      }
      if (hasWebGLCanvas) {
        const overlay = await page.locator(".harness-3d-overlay").innerText();
        assert(overlay.includes("3 nodes"), `noise filter not applied to GPU: ${overlay}`);
      } else {
        const count = await page.locator(".execution-graph-svg g.execution-node").count();
        assert.equal(count, 3, "D3 fallback should render filtered nodes");
      }

      const selector = page.locator('[title*="仅隐藏低风险的展示杂音"]');
      await selector.click();
      await page.locator(".ant-select-item-option")
        .filter({ hasText: "完整证据" }).last().click();
      await page.waitForFunction(() =>
        localStorage.getItem("agent-ebpf.execution-graph.display")?.includes('"noise":"off"'),
      );
      // The dropdown lives above the canvas; scrolling to it can suspend
      // the offscreen 3D redraw until the canvas is visible once more.
      await page.locator(".harness-3d, .execution-graph-canvas").first()
        .scrollIntoViewIfNeeded();
      await page.waitForFunction(() => {
        const overlay = document.querySelector(".harness-3d-overlay");
        const svgNodes = document.querySelectorAll(".execution-graph-svg g.execution-node").length;
        return (overlay && overlay.textContent?.includes("4 nodes")) || svgNodes === 4;
      }, null, { timeout: 15000 });

      if (await page.locator(".harness-3d-overlay").count()) {
        assert((await page.locator(".harness-3d-overlay").innerText()).includes("4 nodes"),
          "full evidence 3D should keep separate raw routine event");
      } else {
        assert.equal(await page.locator(".execution-graph-svg g.execution-node").count(), 4,
          "full evidence 2D should keep all event nodes");
      }
      await page.getByText("2D 拓扑", { exact: true }).first().click();
      await page.locator(".execution-graph-canvas").scrollIntoViewIfNeeded();
      await page.waitForFunction(() =>
        document.querySelectorAll(".execution-graph-svg g.execution-node").length === 4,
      null, { timeout: 15000 });

      const name = disabledWebGL ? "harness-webgl-fallback" : "harness-gpu";
      await page.screenshot({ path: path.join(outdir, name + ".png") });
      assert.deepEqual(errors, [], `browser page errors: ${errors.join(" | ")}`);
      results.push({ mode: name, rendered: hasWebGLCanvas ? "webgl" : "svg",
        nodes: 4, fallbackVerified: disabledWebGL });
    } catch (error) {
      await page.screenshot({
        path: path.join(outdir, disabledWebGL
          ? "harness-webgl-fallback-failure.png" : "harness-gpu-failure.png"),
        fullPage: true,
      }).catch(() => {});
      throw error;
    } finally {
      await context.close();
    }
  }
  fs.writeFileSync(path.join(outdir, "harness-3d-verdict.json"),
    JSON.stringify({ status: "pass", results }, null, 2) + "\n");
  console.log("Harness Three.js / SVG Playwright acceptance: PASS", results);
} finally {
  await browser.close();
}
