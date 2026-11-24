<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { webLocale } from "../../i18n";
import type { BehaviorLane } from "../../composables/behavior/behaviorModel";

interface DialCounts { all: number; file: number; network: number; process: number; other: number; attention: number }
type DialLane = BehaviorLane | "all";
type OptionKey = keyof DialCounts;
interface Selection { lane: DialLane; attentionOnly: boolean }
interface DialOption { key: OptionKey; lane: DialLane; attentionOnly: boolean; zh: string; en: string; color: "blue" | "amber" }

const props = defineProps<{ counts: DialCounts; activeLane: DialLane; attentionOnly: boolean }>();
const emit = defineEmits<{ choose: [selection: Selection] }>();
const options: readonly DialOption[] = [
  { key: "all", lane: "all", attentionOnly: false, zh: "全部", en: "All", color: "blue" },
  { key: "file", lane: "file", attentionOnly: false, zh: "文件", en: "Files", color: "blue" },
  { key: "network", lane: "network", attentionOnly: false, zh: "网络", en: "Network", color: "blue" },
  { key: "process", lane: "process", attentionOnly: false, zh: "进程", en: "Process", color: "blue" },
  { key: "other", lane: "other", attentionOnly: false, zh: "其他", en: "Other", color: "blue" },
  { key: "attention", lane: "all", attentionOnly: true, zh: "需关注", en: "Attention", color: "amber" },
];
const text = (zh: string, en: string) => webLocale.value === "en-US" ? en : zh;
const keyFromProps = () => props.attentionOnly ? "attention" : props.activeLane;
const pending = ref<OptionKey>(keyFromProps());
const applied = ref<OptionKey>(keyFromProps());
const rotation = ref(-options.findIndex(option => option.key === keyFromProps()) * 60);
const root = ref<HTMLElement | null>(null);
const dragging = ref<{ pointerId: number; lastAngle: number; startRotation: number; startPending: OptionKey } | null>(null);
const pendingOption = computed(() => options.find(option => option.key === pending.value)!);
const pendingName = computed(() => text(pendingOption.value.zh, pendingOption.value.en));
const displayCount = computed(() => props.counts[pendingOption.value.key].toLocaleString(webLocale.value));
const rotorStyle = computed(() => ({ transform: "rotate(" + rotation.value + "deg)" }));
const countFor = (option: DialOption) => props.counts[option.key];
function optionStyle(index: number) {
  const angle = (index * 60 + rotation.value) * Math.PI / 180 - Math.PI / 2;
  return { left: ((115 + Math.cos(angle) * 78) / 230 * 100) + "%", top: ((115 + Math.sin(angle) * 78) / 230 * 100) + "%" };
}
function sectorPath(index: number): string {
  const center = index * 60 - 90;
  const start = (center - 30) * Math.PI / 180;
  const end = (center + 30) * Math.PI / 180;
  const point = (radius: number, angle: number) => (115 + Math.cos(angle) * radius) + " " + (115 + Math.sin(angle) * radius);
  return "M " + point(46, start) + " L " + point(108, start) + " A 108 108 0 0 1 " + point(108, end) + " L " + point(46, end) + " A 46 46 0 0 0 " + point(46, start) + " Z";
}
function rotateTo(key: OptionKey): void {
  const target = -options.findIndex(option => option.key === key) * 60;
  rotation.value = target + Math.round((rotation.value - target) / 360) * 360;
}
function setPending(key: OptionKey): void { pending.value = key; rotateTo(key); }
function apply(key: OptionKey): void {
  const option = options.find(item => item.key === key)!;
  pending.value = key; applied.value = key; rotateTo(key);
  emit("choose", { lane: option.lane, attentionOnly: option.attentionOnly });
}
function confirmPending(): void { apply(pending.value); }
let wheelRemainder = 0;
function onWheel(event: WheelEvent): void {
  const delta = Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX;
  if (delta === 0) return;
  if (wheelRemainder !== 0 && Math.sign(delta) !== Math.sign(wheelRemainder)) wheelRemainder = 0;
  wheelRemainder += delta;
  if (Math.abs(wheelRemainder) >= 40) {
    stepPending(Math.sign(wheelRemainder));
    wheelRemainder = 0;
  }
}
function restoreApplied(): void { setPending(applied.value); }
function stepPending(delta: number): void {
  const index = options.findIndex(option => option.key === pending.value);
  setPending(options[(index + delta + options.length) % options.length]!.key);
}
function onKeydown(event: KeyboardEvent): void {
  if (event.altKey || event.ctrlKey || event.metaKey) return;
  if (event.key === "ArrowLeft" || event.key === "ArrowUp") { event.preventDefault(); stepPending(-1); }
  else if (event.key === "ArrowRight" || event.key === "ArrowDown") { event.preventDefault(); stepPending(1); }
  else if (event.key === "Home") { event.preventDefault(); setPending(options[0]!.key); }
  else if (event.key === "End") { event.preventDefault(); setPending(options[5]!.key); }
  else if (event.key === "Enter" || event.key === " ") { event.preventDefault(); confirmPending(); }
  else if (event.key === "Escape") { event.preventDefault(); if (dragging.value) cancelDrag(); restoreApplied(); }
}
function angleAt(event: PointerEvent): number {
  const bounds = root.value!.querySelector<HTMLElement>(".dial-face")!.getBoundingClientRect();
  return Math.atan2(event.clientY - (bounds.top + bounds.height / 2), event.clientX - (bounds.left + bounds.width / 2)) * 180 / Math.PI;
}
function onPointerDown(event: PointerEvent): void {
  if (dragging.value) return;
  if (event.button !== 0 || !(event.target instanceof Element) || event.target.closest("button") || !event.target.closest(".dial-face")) return;
  event.preventDefault();
  const target = event.currentTarget as HTMLElement;
  dragging.value = { pointerId: event.pointerId, lastAngle: angleAt(event), startRotation: rotation.value, startPending: pending.value };
  target.setPointerCapture(event.pointerId);
  window.addEventListener("blur", cancelDrag, { once: true });
}
function onPointerMove(event: PointerEvent): void {
  const state = dragging.value;
  if (!state || event.pointerId !== state.pointerId) return;
  const angle = angleAt(event);
  let delta = angle - state.lastAngle;
  if (delta > 180) delta -= 360;
  if (delta < -180) delta += 360;
  rotation.value += delta;
  state.lastAngle = angle;
}
function finishDrag(event: PointerEvent): void {
  const state = dragging.value;
  if (!state || event.pointerId !== state.pointerId) return;
  dragging.value = null;
  window.removeEventListener("blur", cancelDrag);
  const index = ((Math.round(-rotation.value / 60) % options.length) + options.length) % options.length;
  const key = options[index]!.key;
  setPending(key); apply(key);
  const target = event.currentTarget as HTMLElement;
  if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId);
}
function cancelDrag(): void {
  const state = dragging.value;
  if (!state) return;
  dragging.value = null;
  window.removeEventListener("blur", cancelDrag);
  rotation.value = state.startRotation;
  setPending(state.startPending);
  if (root.value?.hasPointerCapture(state.pointerId)) root.value.releasePointerCapture(state.pointerId);
}
function onPointerCancel(event: PointerEvent): void { if (dragging.value?.pointerId === event.pointerId) cancelDrag(); }
watch(() => [props.activeLane, props.attentionOnly] as const, () => {
  if (dragging.value) cancelDrag();
  applied.value = keyFromProps();
  setPending(applied.value);
});
onUnmounted(() => { window.removeEventListener("blur", cancelDrag); if (dragging.value) cancelDrag(); });
</script>

<template>
  <section ref="root" class="behavior-selection-dial" :data-pending="pending" :class="{ 'is-dragging': dragging }"
    :aria-label="text('行为类型选择转盘', 'Behavior category selection dial')" @keydown="onKeydown"
    @pointerdown="onPointerDown" @pointermove="onPointerMove" @pointerup="finishDrag" @pointercancel="onPointerCancel" @lostpointercapture="onPointerCancel" @wheel.prevent="onWheel">
    <div class="dial-face" role="group" :aria-label="text('六类行为筛选', 'Six behavior filters')">
      <svg class="dial-diagram" viewBox="0 0 230 230" aria-hidden="true">
        <g class="dial-rotor" :style="rotorStyle">
          <circle class="dial-track" cx="115" cy="115" r="107" />
          <path v-for="(option, index) in options" :key="option.key" :d="sectorPath(index)" :class="['dial-sector', option.color, { selected: pending === option.key, applied: applied === option.key }]" />
          <circle class="dial-inner-track" cx="115" cy="115" r="44" />
          <line v-for="index in 6" :key="index" class="dial-divider" :x1="115 + Math.cos((index * 60 - 120) * Math.PI / 180) * 46" :y1="115 + Math.sin((index * 60 - 120) * Math.PI / 180) * 46" :x2="115 + Math.cos((index * 60 - 120) * Math.PI / 180) * 108" :y2="115 + Math.sin((index * 60 - 120) * Math.PI / 180) * 108" />
          <line v-for="tick in 72" :key="tick" :class="['dial-tick', { major: tick % 6 === 0 }]" :x1="115 + Math.cos((tick * 5 - 90) * Math.PI / 180) * (tick % 6 === 0 ? 109 : 111)" :y1="115 + Math.sin((tick * 5 - 90) * Math.PI / 180) * (tick % 6 === 0 ? 109 : 111)" :x2="115 + Math.cos((tick * 5 - 90) * Math.PI / 180) * 114" :y2="115 + Math.sin((tick * 5 - 90) * Math.PI / 180) * 114" />
        </g>
      </svg>
      <span class="dial-pointer" aria-hidden="true" />
      <button v-for="(option, index) in options" :key="option.key" type="button" class="dial-option"
        :class="[option.color, { selected: pending === option.key, applied: applied === option.key }]" :style="optionStyle(index)"
        :data-option="option.key" :aria-label="text(option.zh, option.en) + ', ' + countFor(option).toLocaleString(webLocale) + ', ' + (pending === option.key ? text('预选，按 Enter 或 Space 确认', 'Previewed. Press Enter or Space to confirm') : applied === option.key ? text('当前已应用', 'Currently applied') : text('未选中', 'Not selected'))"
        :aria-pressed="applied === option.key" :aria-current="pending === option.key ? 'true' : undefined" @click="apply(option.key)">
        <span>{{ text(option.zh, option.en) }}</span><small>{{ countFor(option).toLocaleString(webLocale) }}</small>
      </button>
      <button type="button" class="dial-confirm" :aria-label="text('确认筛选：', 'Apply filter: ') + pendingName + ', ' + displayCount" @click="confirmPending">
        <span class="confirm-label">{{ pendingName }}</span><strong>{{ displayCount }}</strong><small>{{ text('确认筛选', 'Apply filter') }}</small>
      </button>
    </div>
    <p class="dial-hint">{{ text('点击选项应用 · 滚轮或方向键预选 · 拖动后释放应用', 'Click to apply · Wheel or arrows to preview · Drag and release to apply') }}</p>
  </section>
</template>

<style scoped>
.behavior-selection-dial { width: 100%; max-width: 250px; color: var(--text-primary); font-family: var(--font-sans); user-select: none; }
.dial-face { width: min(230px, 100%); aspect-ratio: 1; position: relative; margin: 0 auto; touch-action: none; }
.dial-diagram { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; pointer-events: none; }
.dial-rotor { transform-origin: 115px 115px; transition: transform 180ms ease-out; }
.dial-track, .dial-inner-track { fill: var(--surface-0); stroke: var(--border); stroke-width: 1; }
.dial-inner-track { fill: var(--surface-1); stroke-dasharray: 2 3; }
.dial-sector { fill: var(--surface-1); stroke: var(--border); stroke-width: .6; transition: fill 140ms ease, stroke 140ms ease; }
.dial-sector.blue.selected { fill: var(--bg-accent); stroke: var(--text-accent); stroke-width: 1.5; }
.dial-sector.amber.selected { fill: color-mix(in srgb, var(--text-warning) 13%, var(--surface-1)); stroke: var(--text-warning); stroke-width: 1.5; }
.dial-sector.applied:not(.selected) { stroke: var(--text-secondary); }
.dial-divider { stroke: var(--border); stroke-width: .7; pointer-events: none; }
.dial-tick { stroke: var(--border); stroke-width: .55; }
.dial-tick.major { stroke: var(--text-secondary); stroke-width: .9; }
.dial-pointer { position: absolute; top: 1px; left: 50%; width: 0; height: 0; border-left: 5px solid transparent; border-right: 5px solid transparent; border-top: 11px solid var(--text-accent); transform: translateX(-50%); z-index: 3; pointer-events: none; }
.dial-option { position: absolute; z-index: 2; width: 64px; min-height: 35px; padding: 3px 2px; transform: translate(-50%, -50%); display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 1px; border: 1px solid transparent; border-radius: 7px; background: transparent; color: var(--text-secondary); font: 500 11px/1.15 var(--font-sans); cursor: pointer; }
.dial-option small { color: var(--text-muted); font: 11px/1.1 var(--font-mono); }
.dial-option.blue.selected { color: var(--text-accent); }
.dial-option.amber.selected { color: var(--text-warning); }
.dial-option.applied { border-color: var(--border-strong); }
.dial-option:hover { background: var(--surface-2); }
.dial-option:focus-visible, .dial-confirm:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
.dial-confirm { position: absolute; z-index: 4; left: 50%; top: 50%; width: 78px; height: 78px; padding: 5px 3px; transform: translate(-50%, -50%); display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 2px; border: 1px solid var(--border-strong); border-radius: 50%; background: var(--surface-0); color: var(--text-primary); cursor: pointer; }
.confirm-label { max-width: 68px; color: var(--text-secondary); font: 500 11px/1.15 var(--font-sans); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.dial-confirm strong { max-width: 70px; color: var(--text-primary); font: 500 18px/1.1 var(--font-mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.dial-confirm small { color: var(--text-accent); font: 11px/1.1 var(--font-sans); }
.dial-confirm:hover { border-color: var(--text-accent); }
.dial-hint { margin: 7px 0 0; text-align: center; color: var(--text-muted); font-size: 11px; line-height: 1.4; }
.is-dragging .dial-rotor, .is-dragging .dial-option { transition: none; }
.is-dragging .dial-face { cursor: grabbing; }
@media (prefers-reduced-motion: reduce) { .dial-rotor, .dial-sector { transition: none; } }
@media (forced-colors: active) {
  .dial-track, .dial-inner-track { stroke: ButtonText; }
  .dial-sector { fill: Canvas; stroke: ButtonText; }
  .dial-sector.selected { fill: Highlight; stroke: Highlight; }
  .dial-divider, .dial-tick { stroke: ButtonText; }
  .dial-option, .dial-confirm { border-color: ButtonText; }
  .dial-pointer { border-top-color: Highlight; }
  .dial-option.selected, .dial-confirm { color: ButtonText; }
}
</style>
