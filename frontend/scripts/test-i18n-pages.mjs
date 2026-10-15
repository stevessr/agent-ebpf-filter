/** Chromium acceptance checks for each WebUI route in both locales and themes.
 * A failed page does not skip later pages; every route writes a JSON verdict and screenshots.
 */
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const playwrightPath = process.env.PLAYWRIGHT_MODULE || "playwright";
const { chromium } = require(playwrightPath);
const base = process.env.WEBUI_URL || "http://127.0.0.1:5173";
const shard = Number(process.env.WEBUI_SHARD || 0);
const shardCount = Number(process.env.WEBUI_SHARD_COUNT || 4);
assert(Number.isInteger(shard) && shard >= 0 && shard < shardCount);

export const routes = [
  ["dashboard", "/dashboard"],
  ["monitor-dashboard", "/monitor/dashboard/cpu"],
  ["monitor-processes", "/monitor/processes"],
  ["monitor-systemd", "/monitor/systemd"],
  ["monitor-sensors", "/monitor/sensors"],
  ["monitor-tracing", "/monitor/tracing"],
  ["network", "/network"],
  ["network-flow", "/network-flow/overview"],
  ["tls-capture", "/tls-capture"],
  ["execution-topology", "/execution-graph/topology"],
  ["execution-behavior", "/execution-graph/behavior"],
  ["execution-recording", "/execution-graph/recording"],
  ["research", "/research"],
  ["explorer", "/explorer"],
  ["executor", "/executor/shell"],
  ["hooks", "/hooks"],
  ["ml", "/ml/status"],
  ["plugins-list", "/plugins/list"],
  ["plugins-visual", "/plugins/visual"],
  ["config-registry", "/config/registry"],
  ["config-security", "/config/security"],
  ["config-runtime", "/config/runtime"],
  ["config-system", "/config/system"],
  ["signals", "/settings/signals"],
  ["feature-unavailable", "/feature-unavailable"],
];

const selected = routes.filter((_, i) => i % shardCount === shard);
const outdir = path.resolve("artifacts/webui-i18n", `shard-${shard}`);
fs.mkdirSync(outdir, { recursive: true });
const browser = await chromium.launch({ headless: true });
const failures = [];
const results = [];

const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const htmlRoot = (page) => page.locator(".ide-workbench");
const navDashboard = (page) => page.locator(".ide-nav-item").filter({ hasText: /^(Dashboard|仪表盘)/ }).first();
const screenshot = async (page, name, locale, appearance) =>
  page.screenshot({
    path: path.join(outdir, `${name}-${locale}-${appearance}.png`),
    fullPage: true,
    animations: "disabled",
  });

for (const [name, route] of selected) {
  const errors = [];
  const verdict = { name, route, status: "pass", steps: [], untranslated: [] };
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    colorScheme: "light",
    locale: "zh-CN",
  });
  const page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  try {
    await page.goto(new URL(route, base).toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
    await htmlRoot(page).waitFor({ state: "visible", timeout: 25000 });
    await page.locator(".dock-content").first().waitFor({ state: "visible", timeout: 25000 });
    await page.locator(".pane-loading").first().waitFor({ state: "hidden", timeout: 25000 }).catch(() => {});
    await page.waitForTimeout(350);

    const picker = page.locator('[data-testid="locale-switch"]');
    assert.equal(await picker.count(), 1, "missing language selector");
    await picker.selectOption("en-US");
    await page.waitForFunction(() => document.documentElement.lang === "en-US");
    await page.waitForFunction(() => {
      const t = Array.from(document.querySelectorAll(".ide-nav-item")).map(e => e.textContent?.trim());
      return t.includes("Dashboard");
    }, null, { timeout: 7000 });
    assert.equal(await picker.inputValue(), "en-US");
    assert.ok((await navDashboard(page).textContent())?.includes("Dashboard"), "English navigation missing");
    verdict.steps.push("English UI & AntD locale");
    await screenshot(page, name, "en", "light");

    // Audit untranslated legacy UI copy. Do not fail on dynamic event data or server output.
    const untranslated = await page.evaluate(() =>
      Array.from(document.querySelectorAll(".ide-titlebar, .ide-sidebar, .dock-tabs, .dock-breadcrumb"))
        .flatMap((section) => [...section.querySelectorAll("*")])
        .filter((e) => !e.children.length && /[\u3400-\u9fff]/.test(e.textContent || ""))
        .map((e) => e.textContent?.trim())
        .filter(Boolean).slice(0, 30)
    );
    verdict.untranslated = [...new Set(untranslated)].filter((value) => value !== "简体中文" && value !== "语言" && value !== "Language / 语言");
    assert.equal(verdict.untranslated.length, 0, `untranslated shell: ${verdict.untranslated.join(", ")}`);

    await page.emulateMedia({ colorScheme: "dark" });
    await page.waitForFunction(() => document.documentElement.dataset.theme === "dark");
    await screenshot(page, name, "en", "dark");
    verdict.steps.push("English dark theme");

    await page.emulateMedia({ colorScheme: "light" });
    await page.waitForFunction(() => document.documentElement.dataset.theme === "light");
    await picker.selectOption("zh-CN");
    await page.waitForFunction(() => document.documentElement.lang === "zh-CN");
    await page.waitForFunction(() =>
      Array.from(document.querySelectorAll(".ide-nav-item")).some(e => e.textContent?.trim() === "仪表盘")
    );
    await screenshot(page, name, "zh", "light");
    verdict.steps.push("Chinese UI & light theme");

    await page.reload({ waitUntil: "domcontentloaded" });
    await page.locator('[data-testid="locale-switch"]').waitFor();
    assert.equal(await page.locator('[data-testid="locale-switch"]').inputValue(), "zh-CN", "locale not persisted");
    await page.locator('[data-testid="locale-switch"]').selectOption("en-US");
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.locator('[data-testid="locale-switch"]').waitFor();
    assert.equal(await page.locator('[data-testid="locale-switch"]').inputValue(), "en-US", "English locale not persisted");
    assert.equal(await page.locator("html").getAttribute("lang"), "en-US");
    verdict.steps.push("Locale persistence across reload");

    const width = await page.evaluate(() => document.querySelector(".ide-workbench")?.getBoundingClientRect().width || 0);
    assert(width > 0 && width <= 1440 + 2, `Invalid viewport width ${width}`);
    assert.deepEqual(errors, [], "uncaught browser exceptions");
    verdict.steps.push("Viewport & browser errors");
  } catch (error) {
    verdict.status = "fail";
    verdict.error = String(error?.stack || error);
    verdict.browserErrors = errors;
    failures.push({ name, error: verdict.error });
    try { await screenshot(page, name, "failure", "snapshot"); } catch {}
  } finally {
    fs.writeFileSync(path.join(outdir, `${name}.json`), JSON.stringify(verdict, null, 2));
    results.push(verdict);
    await context.close();
  }
  console.log(`[${verdict.status.toUpperCase()}] ${name} ${route}: ${verdict.steps.join(", ")}`);
  if (verdict.status === "fail") console.error(verdict.error);
}

await browser.close();
fs.writeFileSync(path.join(outdir, "summary.json"), JSON.stringify(results, null, 2));
if (process.env.GITHUB_STEP_SUMMARY) {
  const rows = results.map(r =>
    `| ${r.name} | ${r.status === "pass" ? "✅" : "❌"} | ${r.steps.join(" · ")} |`
  );
  fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY,
    `\n### WebUI bilingual acceptance — shard ${shard}\n\n| Page | Result | Checked |\n|---|---|---|\n${rows.join("\n")}\n`);
}
if (failures.length) {
  console.error(`Failed ${failures.length}/${results.length} routes:`, failures);
  process.exitCode = 1;
} else {
  console.log(`All ${results.length} pages passed bilingual, theme and persistence acceptance.`);
}
