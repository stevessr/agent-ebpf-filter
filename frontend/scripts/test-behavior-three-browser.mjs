/** Real Three.js/WebGL browser acceptance with synthetic HTTP/WebSocket fixtures. */
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import * as THREE from "three";
import { pb } from "../src/pb/tracker_pb.js";
import { THREE_CAMERA, THREE_TARGET, eventAngle, worldOrbitPoint } from "../src/components/behavior/behaviorThreeLayout.ts";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright-core");
const base = process.env.BEHAVIOR_URL || "http://127.0.0.1:5173";
const out = process.env.BEHAVIOR_SCREENSHOTS || "../reports/behavior";
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || "/usr/bin/chromium", headless: true, args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"] });
const page = await browser.newPage({ viewport: { width: 1600, height: 1200 }, colorScheme: "dark" });
const errors = []; page.on("pageerror", error => errors.push(error.message));
const fixedNow = Date.now();
const history = { eventId: "event-00", ingestTimestampNs: (fixedNow - 1000) * 1e6, legacyEvent: { pid: 42, ppid: 1, comm: "node", type: "openat", eventType: pb.EventType.OPENAT, path: "/fixture/three.ts" } };
const configure = async p => {
 await p.route("**/*", async route => {
  const request = route.request(), path = new URL(request.url()).pathname;
  if (!["xhr","fetch"].includes(request.resourceType()) || path.startsWith("/src/") || path.startsWith("/node_modules/") || path.startsWith("/@")) return route.continue();
  if (path === "/events/recent") return route.fulfill({contentType:"application/x-protobuf",body:Buffer.from(pb.EventHistoryResponse.encode({events:[{envelope:history}]}).finish())});
  return route.fulfill({json:{}});
 });
 await p.routeWebSocket("**/ws/**",()=>{});
};
const ready = () => page.locator('.behavior-three[data-ready="true"]').waitFor();
try {
 await configure(page);
 await page.goto(base + "/behavior?demo=1&view=3d"); await ready();
 const canvas = page.locator('canvas[data-renderer="three"]');
 assert.ok(Number(await canvas.getAttribute("data-node-count")) > 0);
 assert.ok(Number(await canvas.getAttribute("data-node-count")) <= 96);
 assert.ok(await canvas.evaluate(el => !!el.getContext("webgl2")), "uses an actual WebGL2 context");
 await page.getByRole("button", {name:/暂停显示/}).click();
 await page.getByRole("button", {name:/重置视角/}).click();
 const count = await page.locator(".three-window strong").textContent();
 const beforePause = await canvas.screenshot();
 await page.waitForTimeout(350);
 assert.equal(await page.locator(".three-window strong").textContent(),count);
 assert.deepEqual(await canvas.screenshot(),beforePause,"pause freezes scene, camera and replay");
 await page.screenshot({path:out+"/three-dark.png"});
 await canvas.focus(); await page.keyboard.press("ArrowRight");
 assert.notDeepEqual(await canvas.screenshot(),beforePause,"keyboard orbits the actual scene while paused");
 await page.getByRole("button",{name:/重置视角/}).click();
 await page.getByLabel("按进程聚焦").selectOption("4101");
 assert.ok(Number(await canvas.getAttribute("data-node-count")) < Number(count));
 await page.getByLabel("按进程聚焦").selectOption({label:"全部进程"});
 await page.locator(".three-lanes button").nth(1).click();
 assert.equal(await page.locator(".three-lanes button").nth(1).getAttribute("aria-pressed"),"true");
 await page.locator(".three-lanes button").nth(1).click();
 await page.locator(".three-keyboard summary").click();
 await page.locator(".three-keyboard button").first().click();
 assert.match(await page.locator(".event-inspector").textContent(),/合成演示/);
 await page.getByRole("button",{name:"关闭事件证据"}).click();
 await page.emulateMedia({colorScheme:"light"});
 await page.waitForFunction(()=>document.documentElement.dataset.theme==="light");
 await page.waitForTimeout(100);
 await page.screenshot({path:out+"/three-light.png"});
 await page.emulateMedia({reducedMotion:"reduce"});
 await page.waitForFunction(()=>document.querySelector(".behavior-three")?.dataset.reducedMotion==="true");
 assert.equal(await page.getByRole("button",{name:"自动环绕"}).isDisabled(),true);
 await page.emulateMedia({colorScheme:"dark", reducedMotion:"no-preference"});
 await page.getByLabel("Language / 语言").selectOption("en-US");
 for (const width of [375,768,1280,1920]) {
  await page.setViewportSize({width,height:1100});await page.waitForTimeout(100);
  assert.ok(await page.locator(".behavior-studio").evaluate(el=>el.scrollWidth<=el.clientWidth+1),"no overflow at "+width);
 }
 await page.setViewportSize({width:1600,height:1200});
 await page.clock.setFixedTime(new Date(fixedNow));
 await page.getByRole("button",{name:"Live observation",exact:true}).click();
 await page.waitForFunction(()=>document.querySelector(".three-window strong")?.textContent==="1");
 await page.getByRole("button",{name:/Pause display/}).click();
 await page.getByLabel("Observation window").selectOption("300000");
 await page.getByRole("button",{name:/Reset view/}).click();
 const box = await canvas.boundingBox(); assert.ok(box);
 const camera = new THREE.PerspectiveCamera(42,box.width/box.height,.1,100);
 camera.position.set(...THREE_CAMERA);camera.lookAt(...THREE_TARGET);camera.updateMatrixWorld();
 const point = new THREE.Vector3(...worldOrbitPoint(0,eventAngle(fixedNow,300000,fixedNow-1000),-.025)).project(camera);
 await page.mouse.click(box.x+(point.x+1)*box.width/2,box.y+(1-point.y)*box.height/2);
 await page.locator(".event-inspector").waitFor();
 assert.match(await page.locator(".event-inspector").textContent(),/event-00/ ,"raycast selects the rendered evidence node");
 await page.getByRole("button",{name:"Close event evidence"}).click();
 await page.evaluate(()=>{history.pushState({},"","/behavior?demo=0&view=3d");window.dispatchEvent(new PopStateEvent("popstate"));});
 await page.waitForTimeout(100);
 await page.evaluate(()=>{history.pushState({},"","/behavior?demo=1&view=3d");window.dispatchEvent(new PopStateEvent("popstate"));});
 await page.waitForFunction(()=>Number(document.querySelector(".three-window strong")?.textContent)>1);
 assert.equal(await page.getByRole("button",{name:"Demo replay",exact:false}).getAttribute("aria-pressed"),"true","direct demo link updates restored pane");
 await canvas.evaluate(el=>el.dispatchEvent(new Event("webglcontextlost",{cancelable:true})));
 await page.locator(".three-fallback").waitFor(); await page.locator(".orbit-svg").waitFor();
 await page.getByRole("button",{name:/Retry 3D/}).click();await ready();
 for(let i=0;i<3;i++){
  await page.getByRole("button",{name:"Flat orbit",exact:true}).click();
  assert.equal(await page.locator('canvas[data-renderer="three"]').count(),0);
  await page.getByRole("button",{name:"3D space",exact:true}).click();await ready();
  assert.equal(await page.locator('canvas[data-renderer="three"]').count(),1);
 }
 assert.deepEqual(errors,[]);
 console.log("PASS: real WebGL2 scene, bounded marks, paused scene, keyboard camera, PID/lane filtering, evidence, light/dark, reduced motion, four viewport sizes, raycast picking, context loss/retry, repeated mount cleanup");
} finally { await browser.close(); }
