<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch } from "vue";
import { ReloadOutlined, AimOutlined } from "@ant-design/icons-vue";
import { webLocale } from "../../i18n";
import { systemIsDark } from "../../composables/useSystemTheme";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { BEHAVIOR_LANES, isAttentionEvent, laneForEvent, type BehaviorLane } from "../../composables/behavior/behaviorModel";
import BehaviorOrbit from "./BehaviorOrbit.vue";
import { BehaviorThreeScene, type ThreePalette } from "./behaviorThreeScene";
import { boundedThreeEvents } from "./behaviorThreeLayout";

const props = defineProps<{
  events: readonly AgentEvent[]; now: number; windowMs: number; selectedKey?: string;
  activeLane: BehaviorLane | "all"; paused: boolean; centerLabel: string; playbackRate?: number; statusLabel?: string; sourceWarning?: boolean;
}>();
const emit = defineEmits<{ select: [event: AgentEvent]; lane: [lane: BehaviorLane] }>();
const text = (zh: string, en: string) => webLocale.value === "en-US" ? en : zh;
const root = useTemplateRef<HTMLElement>("threeRoot");
const host = useTemplateRef<HTMLElement>("threeHost");
const stage = useTemplateRef<HTMLElement>("threeStage");
const failure = ref(false);
const ready = ref(false);
const autoOrbit = ref(true);
const reducedMotion = ref(false);
const expanded = ref(false);
const showTrails = ref(true);
const cameraView = ref<"perspective" | "top" | "side">("perspective");
const fullscreen = ref(false);
const fullscreenSupported = ref(false);
const fullscreenError = ref(false);
const canFocus = computed(() => !!props.selectedKey && boundedThreeEvents(props.events, props.selectedKey).some(mark => mark.event.key === props.selectedKey));
const hover = shallowRef<{ event: AgentEvent; x: number; y: number } | null>(null);
let engine: BehaviorThreeScene | null = null;
let resizeObserver: ResizeObserver | undefined;
let intersectionObserver: IntersectionObserver | undefined;
let motionMedia: MediaQueryList | undefined;
let contrastMedia: MediaQueryList | undefined;
let inViewport = true;
let mounted = false;
const laneName = (lane: BehaviorLane) => ({ file: text("文件访问", "File access"), network: text("网络通信", "Network I/O"), process: text("进程执行", "Process execution"), other: text("其他信号", "Other signals") })[lane];
const lanes = computed(() => BEHAVIOR_LANES.map((key, index) => ({ key, index, count: props.events.filter(event => laneForEvent(event) === key).length })));
const attentionCount = computed(() => props.events.filter(isAttentionEvent).length);
const keyboardEvents = computed(() => props.events.filter(event => props.activeLane === "all" || laneForEvent(event) === props.activeLane).slice(0, 8));
function palette(): ThreePalette {
  const css = getComputedStyle(document.documentElement);
  const color = (token: string, fallback: string) => css.getPropertyValue(token).trim() || fallback;
  if (contrastMedia?.matches) return { canvas: "#000000", surface: "#000000", border: "#ffffff", text: "#ffffff", accent: "#74c0fc", warning: "#ffdf00" };
  return {
    canvas: color("--ui-canvas", "#f8fafc"), surface: color("--ui-surface", "#ffffff"),
    border: color("--ui-border", "#d8e2f0"), text: color("--ui-text-strong", "#1f2937"),
    accent: color("--ui-accent", "#2563eb"), warning: color("--text-warning", "#92400e"),
  };
}
function sync(): void {
  engine?.update({ events: props.events, now: props.now, windowMs: props.windowMs, activeLane: props.activeLane,
    selectedKey: props.selectedKey, paused: props.paused, reducedMotion: reducedMotion.value, autoOrbit: autoOrbit.value, expanded: expanded.value, showTrails: showTrails.value, playbackRate: props.playbackRate });
}
function setActive(): void { engine?.setActive(inViewport && !document.hidden); }
function resetView(): void { autoOrbit.value = false; cameraView.value = "perspective"; engine?.reset(); }
function applyView(): void { autoOrbit.value = false; engine?.setView(cameraView.value); }
function focusSelected(): void { autoOrbit.value = false; engine?.focusSelected(); }
function fullscreenChanged(): void { fullscreen.value = document.fullscreenElement === root.value; }
async function toggleFullscreen(): Promise<void> {
  fullscreenError.value = false;
  try { if (document.fullscreenElement === root.value) await document.exitFullscreen(); else await root.value?.requestFullscreen(); }
  catch { fullscreenError.value = true; }
}
function disposeScene(): void {
  resizeObserver?.disconnect(); resizeObserver = undefined;
  intersectionObserver?.disconnect(); intersectionObserver = undefined;
  if (engine) {
    engine.renderer.domElement.removeEventListener("webglcontextlost", lostContext);
    engine.dispose(); engine = null;
  }
  ready.value = false; hover.value = null;
}
function lostContext(event: Event): void {
  event.preventDefault(); failure.value = true; disposeScene();
}
async function initialize(): Promise<void> {
  disposeScene(); failure.value = false;
  await nextTick();
  if (!mounted || !host.value || !stage.value) return;
  try {
    engine = new BehaviorThreeScene(host.value, palette(), event => emit("select", event), (event, x, y) => { hover.value = event ? { event, x: Math.max(8, x), y } : null; });
    engine.renderer.domElement.setAttribute("aria-label", text("3D 行为轨道，拖动旋转，方向键调整视角，加减缩放，R 重置", "3D behavior orbit. Drag or use arrow keys to rotate, plus/minus to zoom, R to reset"));
    engine.renderer.domElement.addEventListener("webglcontextlost", lostContext);
    resizeObserver = new ResizeObserver(entries => { const rect = entries[0]?.contentRect; if (rect) engine?.resize(rect.width, rect.height); });
    resizeObserver.observe(host.value);
    intersectionObserver = new IntersectionObserver(entries => { inViewport = entries[0]?.isIntersecting ?? true; setActive(); }, { threshold: .01 });
    intersectionObserver.observe(stage.value);
    engine.resize(host.value.clientWidth, host.value.clientHeight);
    sync(); setActive(); ready.value = true;
  } catch {
    failure.value = true; disposeScene();
  }
}
function mediaChanged(): void {
  reducedMotion.value = motionMedia?.matches ?? false;
  engine?.setPalette(palette()); sync();
}
watch(() => [props.events, props.now, props.windowMs, props.activeLane, props.selectedKey, props.paused, autoOrbit.value, expanded.value, showTrails.value, props.playbackRate], sync);
watch([systemIsDark, webLocale], async () => {
  await nextTick();
  engine?.setPalette(palette());
  engine?.renderer.domElement.setAttribute("aria-label", text("3D 行为轨道，拖动旋转，方向键调整视角，加减缩放，R 重置", "3D behavior orbit. Drag or use arrow keys to rotate, plus/minus to zoom, R to reset"));
});
onMounted(() => {
  mounted = true;
  fullscreenSupported.value = !!document.fullscreenEnabled;
  document.addEventListener("fullscreenchange", fullscreenChanged);
  motionMedia = window.matchMedia("(prefers-reduced-motion: reduce)");
  contrastMedia = window.matchMedia("(forced-colors: active)");
  reducedMotion.value = motionMedia.matches;
  motionMedia.addEventListener("change", mediaChanged); contrastMedia.addEventListener("change", mediaChanged);
  document.addEventListener("visibilitychange", setActive);
  void initialize();
});
onUnmounted(() => {
  mounted = false; disposeScene();
  motionMedia?.removeEventListener("change", mediaChanged); contrastMedia?.removeEventListener("change", mediaChanged);
  document.removeEventListener("visibilitychange", setActive);
  document.removeEventListener("fullscreenchange", fullscreenChanged);
});
</script>

<template>
  <div ref="threeRoot" class="behavior-three" :data-ready="ready" :data-paused="paused" :data-reduced-motion="reducedMotion" :data-expanded="expanded" :data-camera-view="cameraView">
    <div v-if="failure" class="three-fallback" role="status"><span>{{ text('3D 渲染不可用，已显示平面轨道。', '3D rendering is unavailable. Showing the flat orbit.') }}</span><button @click="initialize"><ReloadOutlined /> {{ text('重试 3D', 'Retry 3D') }}</button></div>
    <template v-if="!failure">
      <div class="three-source" :class="{ warning: sourceWarning }" role="status">{{ statusLabel }}</div>
      <div class="three-tools">
        <span>{{ text('拖动旋转 · 滚轮缩放 · 点击事件', 'Drag to orbit · scroll to zoom · select an event') }}</span>
        <div><select v-model="cameraView" :aria-label="text('3D 视角', '3D camera view')" @change="applyView"><option value="perspective">{{ text('透视', 'Perspective') }}</option><option value="top">{{ text('俯视', 'Top') }}</option><option value="side">{{ text('侧视', 'Side') }}</option></select><button :aria-pressed="expanded" @click="expanded = !expanded">{{ text('分层展开', 'Expand layers') }}</button><button :aria-pressed="showTrails" @click="showTrails = !showTrails">{{ text('事件尾迹', 'Event trails') }}</button><button :disabled="!canFocus" @click="focusSelected">{{ text('聚焦选中', 'Focus selection') }}</button><button :disabled="!fullscreenSupported" @click="toggleFullscreen">{{ fullscreen ? text('退出全屏', 'Exit fullscreen') : text('专注全屏', 'Focus fullscreen') }}</button><button :aria-pressed="autoOrbit && !reducedMotion" :disabled="reducedMotion" @click="autoOrbit = !autoOrbit">{{ text('自动环绕', 'Auto orbit') }}</button><button @click="resetView"><AimOutlined /> {{ text('重置视角', 'Reset view') }}</button></div>
      </div>
      <p v-if="fullscreenError" class="three-fullscreen-error" role="status">{{ text('浏览器未允许全屏，可继续使用当前画布。', 'Fullscreen was not allowed; the current canvas remains available.') }}</p>
      <div ref="threeStage" class="three-stage">
        <div ref="threeHost" class="three-host" />
        <div class="three-window"><span>{{ centerLabel }}</span><strong>{{ events.length.toLocaleString() }}</strong><small>{{ text('窗口内事件', 'events in window') }} · {{ windowMs / 1000 }}s</small></div>
        <div class="three-caption"><span class="caption-dot" />{{ text('圆点：已观测', 'Sphere: observed') }}<span class="caption-diamond" />{{ text('菱形：需关注', 'Diamond: attention') }} {{ attentionCount }}</div>
        <div v-if="hover" class="three-tooltip" :style="{ left: hover.x + 'px', top: hover.y + 'px' }" data-i18n-ignore><strong>{{ hover.event.comm }} · {{ hover.event.type }}</strong><span>{{ hover.event.netEndpoint || hover.event.path || '—' }}</span></div>
        <div v-if="!events.length" class="three-empty">{{ text('等待行为信号，空窗口不代表安全。', 'Awaiting behavior signals. An empty window does not establish safety.') }}</div>
      </div>
      <div class="three-lanes" role="group" :aria-label="text('3D 行为分类', '3D behavior categories')"><button v-for="item in lanes" :key="item.key" :aria-pressed="activeLane === item.key" @click="emit('lane', item.key)"><span><code>0{{ item.index + 1 }}</code> {{ laneName(item.key) }}</span><strong>{{ item.count }}</strong></button></div>
      <details class="three-keyboard"><summary>{{ text('键盘选择事件', 'Select events with the keyboard') }}</summary><div><button v-for="event in keyboardEvents" :key="event.key" :aria-pressed="selectedKey === event.key" @click="emit('select', event)" data-i18n-ignore>{{ event.comm }} · {{ event.type }} · {{ event.pid }}<span v-if="isAttentionEvent(event)"> ◆</span></button></div></details>
    </template>
    <BehaviorOrbit v-else v-bind="props" @select="emit('select', $event)" @lane="emit('lane', $event)" />
  </div>
</template>
<style scoped>
.behavior-three { min-width: 0; }
.three-tools { display: flex; gap: 12px; align-items: center; justify-content: space-between; padding: 10px 14px; border-bottom: .5px solid var(--border); }
.three-tools > span { font-size: 11px; color: var(--text-muted); }
.three-tools > div { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; min-width: 0; }
.three-tools select { font: 11px var(--font-sans); color: var(--text-secondary); background: var(--surface-2); border: .5px solid var(--border); border-radius: 8px; padding: 6px; min-height: 32px; }
.three-tools select:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
.three-source { display: none; font-size: 12px; color: var(--text-accent); padding: 5px 0 10px; }
.three-source.warning { color: var(--text-warning); }
.behavior-three:fullscreen .three-source { display: block; }
.three-fullscreen-error { margin: 0; padding: 8px 14px; font-size: 11px; color: var(--text-warning); }
.behavior-three:fullscreen { display: flex; flex-direction: column; width: 100dvw; height: 100dvh; background: var(--surface-0); color: var(--text-primary); padding: 16px; box-sizing: border-box; }
.behavior-three:fullscreen .three-stage { height: auto; min-height: 0; flex: 1; }
.behavior-three:fullscreen .three-keyboard { display: none; }
button { font: 12px var(--font-sans); border: .5px solid var(--border); border-radius: 8px; background: var(--surface-2); color: var(--text-secondary); padding: 7px 10px; cursor: pointer; min-height: 32px; }
button[aria-pressed="true"] { color: var(--text-accent); background: var(--bg-accent); border-color: var(--text-accent); }
button:disabled { cursor: default; opacity: .5; }
button:focus-visible, summary:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
.three-stage { position: relative; min-height: 360px; height: clamp(360px, 38vw, 500px); background: var(--surface-0); overflow: hidden; }
.three-host { position: absolute; inset: 0; }
.three-host :deep(canvas) { width: 100%; height: 100%; display: block; cursor: grab; outline: 0; touch-action: none; }
.three-host :deep(canvas:focus-visible) { outline: 2px solid var(--text-accent); outline-offset: -3px; }
.three-window { position: absolute; top: 18px; left: 18px; display: flex; flex-direction: column; gap: 3px; pointer-events: none; }
.three-window > span { font: 11px var(--font-mono); color: var(--text-muted); }
.three-window > strong { font: 400 34px var(--font-mono); color: var(--text-primary); }
.three-window > small { font-size: 11px; color: var(--text-muted); }
.three-caption { position: absolute; bottom: 13px; left: 18px; display: flex; flex-wrap: wrap; align-items: center; gap: 7px; pointer-events: none; font-size: 11px; color: var(--text-secondary); }
.caption-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--text-accent); }
.caption-diamond { margin-left: 8px; width: 7px; height: 7px; transform: rotate(45deg); background: var(--text-warning); }
.three-tooltip { position: absolute; pointer-events: none; max-width: 235px; background: var(--surface-2); border: .5px solid var(--border); border-radius: 8px; padding: 9px 11px; display: grid; gap: 5px; font: 11px var(--font-mono); overflow-wrap: anywhere; }
.three-tooltip strong { font-weight: 500; }
.three-tooltip span { color: var(--text-muted); }
.three-lanes { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 7px; padding: 12px 14px; }
.three-lanes button { display: flex; align-items: center; justify-content: space-between; gap: 8px; min-height: 46px; }
.three-lanes button > span { text-align: left; font-size: 11px; }
.three-lanes code { color: var(--text-muted); }
.three-lanes strong { font: 500 19px var(--font-mono); color: var(--text-accent); }
.three-keyboard { padding: 0 14px 12px; font-size: 11px; color: var(--text-muted); }
.three-keyboard summary { cursor: pointer; padding: 4px 0; }
.three-keyboard > div { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 7px; }
.three-keyboard button { font-size: 11px; text-align: left; }
.three-empty { position: absolute; bottom: 45px; left: 18px; right: 18px; color: var(--text-warning); font-size: 12px; pointer-events: none; }
.three-fallback { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px; padding: 12px 14px; color: var(--text-warning); font-size: 12px; }
@container (max-width: 620px) { .three-tools { align-items: flex-start; flex-direction: column; gap: 8px; } .three-lanes { grid-template-columns: repeat(2, minmax(0, 1fr)); } .three-stage { height: 380px; } .three-window { top: 12px; left: 12px; } }
@media (forced-colors: active) { button { border: 1px solid ButtonText; } }
</style>
