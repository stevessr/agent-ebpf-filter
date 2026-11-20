/** Local synthetic replay acceptance for priority review, rotary selection and 3D controls. */
import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright-core");
const base=process.env.BEHAVIOR_URL||"http://127.0.0.1:5173";
const out=process.env.BEHAVIOR_SCREENSHOTS||"../reports/behavior";
await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_PATH||"/usr/bin/chromium",headless:true,args:["--use-gl=angle","--use-angle=swiftshader","--enable-unsafe-swiftshader"]});
const page=await browser.newPage({viewport:{width:1600,height:1400},colorScheme:"dark",reducedMotion:"reduce"});
const errors=[];page.on("pageerror",error=>errors.push(error.message));
async function poll(fn,label){const until=Date.now()+15000;while(!(await fn())){assert.ok(Date.now()<until,"Timed out: "+label);await page.waitForTimeout(40);}}
try{
 await page.route("**/*",route=>{const request=route.request(),path=new URL(request.url()).pathname;if(!["xhr","fetch"].includes(request.resourceType())||path.startsWith("/src/")||path.startsWith("/node_modules/")||path.startsWith("/@"))return route.continue();return route.fulfill({json:{}});});
 await page.routeWebSocket("**/ws/**",()=>{});
 await page.goto(base+"/behavior?demo=1&view=3d");
 await page.locator(".behavior-studio").waitFor();await page.locator(".pause-button").click();
 await page.locator('.behavior-three[data-ready="true"]').waitFor();
 const dial=page.locator(".behavior-selection-dial");
 const attention=page.locator(".attention-navigator");
 const risks=await attention.locator(".attention-card").evaluateAll(items=>items.map(item=>Number(item.dataset.risk)));
 assert.ok(risks.length>0);assert.deepEqual(risks,[...risks].sort((a,b)=>b-a));
 const totalAttention=Number(await dial.locator('[data-option="attention"] small').textContent());
 const summary=await attention.locator("header>div").first().textContent();
 assert.ok(summary.includes(String(totalAttention)),"grouping retains all attention record counts");
 const evidenceKey=await attention.locator(".attention-card").first().getAttribute("data-event-id");
 await attention.locator(".attention-card").first().click();
 assert.ok((await page.locator(".event-inspector").textContent()).includes(evidenceKey));
 assert.match(await attention.locator(".attention-card").first().textContent(),/证据已查看/);
 await page.getByRole("button",{name:"关闭事件证据"}).click();
 await page.getByLabel("关注事件排序").selectOption("recent");
 await page.getByLabel("关注事件排序").selectOption("risk");
 await attention.locator('.attention-bucket:not([disabled])').first().click();
 await page.locator(".event-inspector").waitFor();
 await page.getByRole("button",{name:"关闭事件证据"}).click();
 await dial.locator('[data-option="file"]').click();
 assert.equal(await dial.locator('[data-option="file"]').getAttribute("aria-pressed"),"true");
 await dial.locator('[data-option="file"]').focus();await page.keyboard.press("ArrowRight");
 assert.equal(await dial.getAttribute("data-pending"),"network");
 assert.equal(await dial.locator('[data-option="file"]').getAttribute("aria-pressed"),"true","preview does not apply until confirmed");
 await page.keyboard.press("Enter");
 assert.equal(await dial.locator('[data-option="network"]').getAttribute("aria-pressed"),"true","Enter applies preview rather than the old focused button");
 const db=await dial.boundingBox();await page.mouse.move(db.x+db.width/2,db.y+db.height/2);await page.mouse.wheel(0,80);
 await poll(async()=>await dial.getAttribute("data-pending")==="process","wheel preview");
 await dial.locator(".dial-confirm").focus();await page.keyboard.press("Escape");
 assert.equal(await dial.getAttribute("data-pending"),"network");
 await page.keyboard.press("Home");await page.keyboard.press("Enter");
 assert.equal(await dial.locator('[data-option="all"]').getAttribute("aria-pressed"),"true");
 await page.keyboard.press("End");await page.keyboard.press("Space");
 assert.equal(await page.locator(".attention-filter").getAttribute("aria-pressed"),"true");
 await dial.locator('[data-option="all"]').click();
 const face=await dial.locator(".dial-face").boundingBox();const cx=face.x+face.width/2,cy=face.y+face.height/2,r=face.width*.485;
 await page.mouse.move(cx+r,cy);await page.mouse.down();
 for(let step=1;step<=6;step++){const a=step*Math.PI/12;await page.mouse.move(cx+Math.cos(a)*r,cy+Math.sin(a)*r);}
 await page.mouse.up();
 const dragged=await dial.getAttribute("data-pending");
 assert.notEqual(dragged,"all");assert.equal(await dial.locator('[data-option="'+dragged+'"]').getAttribute("aria-pressed"),"true","drag release applies selected segment");
 await dial.locator('[data-option="all"]').click();
 const slider=page.getByLabel("回放进度");
 await slider.evaluate(el=>{el.value="48";el.dispatchEvent(new Event("input",{bubbles:true}));});
 assert.equal(await slider.inputValue(),"48");assert.equal(await page.locator(".pause-button").getAttribute("aria-pressed"),"true");
 await page.getByLabel("回放速度").selectOption("2");await page.getByRole("button",{name:"播放回放",exact:false}).click();
 await page.waitForTimeout(1150);await page.getByRole("button",{name:"暂停回放",exact:false}).click();
 assert.ok(Number(await slider.inputValue())>=50,"2x playback advances the actual timeline");
 await page.getByRole("button",{name:"上一关注时刻",exact:true}).click();
 await page.locator(".event-inspector").waitFor();
 assert.equal(await page.locator(".pause-button").getAttribute("aria-pressed"),"true");
 await page.getByRole("button",{name:"关闭事件证据"}).click();
 const canvas=page.locator('canvas[data-renderer="three"]');await canvas.scrollIntoViewIfNeeded();
 const perspective=await canvas.screenshot();await page.getByLabel("3D 视角").selectOption("top");
 assert.notDeepEqual(await canvas.screenshot(),perspective,"camera preset changes actual rendering");
 const flatLayers=await canvas.screenshot();await page.getByRole("button",{name:"分层展开",exact:true}).click();
 assert.notDeepEqual(await canvas.screenshot(),flatLayers,"expanded lanes change actual rendering");
 await page.getByRole("button",{name:"事件尾迹",exact:true}).click();assert.equal(await page.getByRole("button",{name:"事件尾迹",exact:true}).getAttribute("aria-pressed"),"false");
 await page.locator(".three-keyboard summary").click();await page.locator(".three-keyboard button").first().click();
 assert.equal(await page.getByRole("button",{name:"聚焦选中",exact:true}).isEnabled(),true);
 await page.getByRole("button",{name:"聚焦选中",exact:true}).click();
 const fullscreen=page.getByRole("button",{name:"专注全屏",exact:true});
 if(await fullscreen.isEnabled()){
  await fullscreen.click();await poll(()=>page.evaluate(()=>!!document.fullscreenElement),"fullscreen entry");
  assert.ok((await page.locator(".three-source").textContent()).includes("演示回放"),"fullscreen retains source/pause semantics");
  await page.getByRole("button",{name:"退出全屏",exact:true}).click();await poll(()=>page.evaluate(()=>!document.fullscreenElement),"fullscreen exit");
 }
 await page.getByRole("button",{name:"关闭事件证据"}).click();
 await page.getByRole("button",{name:/重置视角/}).click();
 if(await page.getByRole("button",{name:"事件尾迹",exact:true}).getAttribute("aria-pressed")==="false")await page.getByRole("button",{name:"事件尾迹",exact:true}).click();
 await page.locator(".three-keyboard summary").click();
 await page.setViewportSize({width:1920,height:1900});await page.locator(".behavior-title").scrollIntoViewIfNeeded();
 await page.screenshot({path:out+"/navigation-dark.png"});
 await page.emulateMedia({colorScheme:"light"});await page.waitForFunction(()=>document.documentElement.dataset.theme==="light");await page.screenshot({path:out+"/navigation-light.png"});
 await page.getByLabel("Language / 语言").selectOption("en-US");
 assert.ok(await page.getByRole("heading",{name:"Priority review",exact:true}).isVisible());
 for(const width of [375,768,1280,1920]){
  await page.setViewportSize({width,height:1100});await page.waitForTimeout(80);
  assert.ok(await page.locator(".behavior-studio").evaluate(el=>el.scrollWidth<=el.clientWidth+1),"no horizontal overflow at "+width);
 }
 assert.ok(await dial.locator(".dial-rotor").evaluate(el=>parseFloat(getComputedStyle(el).transitionDuration)<=.001),"reduced motion removes perceptible wheel animation");
 assert.deepEqual(errors,[]);
 console.log("PASS: grouped attention review/time peaks, evidence viewed semantics, dial click/keyboard/wheel/drag, timeline seeking/speed/chapters, real 3D presets/layers/trails/focus/fullscreen, source labels, bilingual themes, reduced motion and responsive widths");
}finally{await browser.close();}
