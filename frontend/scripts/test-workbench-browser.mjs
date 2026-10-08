/** Browser interaction regression; requires agent-browser and a running frontend. */
import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
const cli = process.env.AGENT_BROWSER_CLI || "agent-browser";
const session = `ide-regression-${process.pid}`;
const base = process.env.WORKBENCH_URL || "http://127.0.0.1:5173";
function command(...args) {
  const result = spawnSync(cli, ["--session", session, "--json", ...args], {
    encoding: "utf8",
    timeout: 60000,
  });
  if (result.error) throw result.error;
  const response = JSON.parse(result.stdout.trim());
  assert.equal(response.success, true, response.error || result.stderr);
  return response.data;
}
const evaluate = (code) => command("eval", code).result;
const settle = () =>
  evaluate("new Promise(resolve => setTimeout(resolve, 150))");
const count = () => evaluate('document.querySelectorAll(".dock-group").length');
const layout = () =>
  evaluate('JSON.parse(localStorage.getItem("agent.ide.layout.v1"))');
function click(selector) {
  command("click", selector);
  settle();
}
function drag(source, destination, x = 0.5, y = 0.5) {
  // HTML5 DataTransfer is exercised through the same event handlers as a native drag.
  evaluate(`(async () => {
    const source = document.querySelector(${JSON.stringify(source)}), destination = document.querySelector(${JSON.stringify(destination)});
    if (!source || !destination) throw new Error('missing drag surface');
    const dataTransfer = new DataTransfer(), rect = destination.getBoundingClientRect();
    const init = { bubbles:true, cancelable:true, dataTransfer, clientX:rect.x+rect.width*${x}, clientY:rect.y+rect.height*${y} };
    source.dispatchEvent(new DragEvent('dragstart',init));
    await new Promise(r => setTimeout(r,30));
    destination.dispatchEvent(new DragEvent('dragover',init));
    await new Promise(r => setTimeout(r,30));
    destination.dispatchEvent(new DragEvent('drop',init));
    source.dispatchEvent(new DragEvent('dragend',init));
  })()`);
  settle();
}
try {
  const launch = process.env.CHROMIUM_PATH
    ? ["--executable-path", process.env.CHROMIUM_PATH]
    : [];
  command(...launch, "open", `${base}/dashboard`);
  evaluate('localStorage.removeItem("agent.ide.layout.v1")');
  command("reload");
  command("wait", ".triage");
  assert.equal(count(), 1);
  click('[aria-label="左右分屏"]');
  assert.equal(count(), 2);
  click('[aria-label="上下分屏"]');
  assert.equal(count(), 3);
  assert.equal(layout().first.axis, "vertical");
  // Close the bottom leaf: its empty branch must collapse back to two panes.
  click(".dock-split.vertical .dock-branch:last-child .tab-close");
  assert.equal(count(), 2);
  command("focus", '[role="separator"]');
  command("press", "ArrowRight");
  assert.equal(layout().ratio, 0.55);
  const box = evaluate(
    `(() => { const e = document.querySelector('[role=separator]'), r = e.getBoundingClientRect(), p = e.parentElement.getBoundingClientRect(); return { x:r.x+r.width/2, y:r.y+r.height/2, next:p.x+p.width*.6 }; })()`,
  );
  // Keyboard resize above also makes the splitter usable without a pointing device.
  assert.ok(box);
  command(
    "mouse",
    "move",
    String(Math.round(box.x)),
    String(Math.round(box.y)),
  );
  command("mouse", "down");
  command(
    "mouse",
    "move",
    String(Math.round(box.next)),
    String(Math.round(box.y)),
  );
  command("mouse", "up");
  assert.ok(Math.abs(layout().ratio - 0.6) < 0.01);
  const resizedRatio = layout().ratio;
  command("reload");
  command("wait", ".triage");
  assert.equal(count(), 2);
  assert.equal(layout().ratio, resizedRatio);
  // Center merge removes the emptied source pane; no duplicate tab IDs survive.
  command(
    "drag",
    ".dock-branch:last-child .dock-tab",
    ".dock-branch:first-child .dock-tabs",
  );
  settle();
  assert.equal(count(), 1);
  assert.equal(layout().tabs.length, 2);
  // Edge drag of a tab creates another group.
  drag(".dock-tab:last-child", ".dock-group", 0.95, 0.5);
  assert.equal(count(), 2);
  // Drag navigation directly to an edge to open a distinct tool in a new split.
  drag(
    '.ide-nav-item[title^="网络 ·"]',
    ".dock-branch:last-child .dock-group",
    0.5,
    0.95,
  );
  assert.equal(count(), 3);
  assert.equal(layout().second.axis, "vertical");
  // Click configuration in the network pane, then navigate inside its isolated route scope.
  click(".ide-activity > button:last-child");
  command("wait", ".dock-group.focused .ant-tabs");
  evaluate(
    `Array.from(document.querySelectorAll('.dock-group.focused .ant-tabs-tab')).find(e => e.innerText.includes('Runtime')).click()`,
  );
  settle();
  const paths = evaluate(
    `Array.from(document.querySelectorAll('.dock-breadcrumb-path')).map(e=>e.innerText)`,
  );
  assert.ok(paths.includes("/config/runtime"), JSON.stringify(paths));
  assert.equal(paths.filter((p) => p === "/dashboard").length, 2);
  command("reload");
  command("wait", ".dock-group.focused .ant-tabs");
  assert.equal(count(), 3);
  assert.ok(
    evaluate(
      `document.querySelector('.dock-group.focused .dock-content').innerText.includes('Runtime')`,
    ),
  );
  command("fill", '[aria-label="搜索工作台功能"]', "不存在的功能");
  assert.equal(
    evaluate('document.querySelectorAll(".ide-nav-item").length'),
    0,
  );
  command("fill", '[aria-label="搜索工作台功能"]', "");
  // Verify the narrow shell keeps its viewport and navigation can be hidden.
  command("set", "viewport", "600", "800");
  click('[aria-label="显示 / 隐藏导航"]');
  assert.equal(evaluate('document.querySelectorAll(".ide-sidebar").length'), 0);
  assert.ok(
    evaluate(
      'document.querySelector(".ide-workbench").getBoundingClientRect().width <= 600',
    ),
  );
  const errors = command("errors");
  assert.ok(!errors.errors?.length, JSON.stringify(errors));
  console.log(
    "PASS: split, tab/navigation drag, merge/prune, pointer/keyboard resize, persistence, independent routes, search, narrow viewport; no uncaught browser errors.",
  );
} finally {
  command("close");
}
