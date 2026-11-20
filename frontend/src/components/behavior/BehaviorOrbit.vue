<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useTemplateRef } from "vue";
import { webLocale } from "../../i18n";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { laneForEvent, isAttentionEvent, type BehaviorLane } from "../../composables/behavior/behaviorModel";

const props = defineProps<{
  events: readonly AgentEvent[];
  now: number;
  windowMs: number;
  selectedKey?: string;
  activeLane: BehaviorLane | "all";
  paused: boolean;
  centerLabel: string;
}>();
const emit = defineEmits<{ select: [event: AgentEvent]; lane: [lane: BehaviorLane] }>();
const root = useTemplateRef<HTMLElement>("orbitRoot");
const compact = ref(true);
let observer: ResizeObserver | undefined;
onMounted(() => {
  observer = new ResizeObserver(entries => { compact.value = (entries[0]?.contentRect.width ?? 1000) < 780; });
  if (root.value) observer.observe(root.value);
});
onUnmounted(() => observer?.disconnect());
const text = (zh: string, en: string) => webLocale.value === "en-US" ? en : zh;
const lanes = computed(() => [
  { key: "file" as const, name: text("文件访问", "File access"), code: "01", radius: 146 },
  { key: "network" as const, name: text("网络通信", "Network I/O"), code: "02", radius: 189 },
  { key: "process" as const, name: text("进程执行", "Process execution"), code: "03", radius: 232 },
  { key: "other" as const, name: text("其他信号", "Other signals"), code: "04", radius: 275 },
]);
const point = (radius: number, angle: number) => ({
  x: 326 + Math.cos(angle * Math.PI / 180) * radius,
  y: 318 + Math.sin(angle * Math.PI / 180) * radius,
});
const ticks = Array.from({ length: 61 }, (_, index) => {
  const angle = -80 + index * 5;
  return { index, start: point(290, angle), end: point(index % 5 === 0 ? 302 : 295, angle) };
});
const marks = computed(() => {
  // SVG stays bounded even when the shared stream retains thousands of records.
  const buckets = lanes.value.map(lane => props.events.filter(e => laneForEvent(e) === lane.key).slice(0, 24));
  return buckets.flatMap((events, laneIndex) => events.map((event, index) => {
    const lane = lanes.value[laneIndex]!;
    const age = Math.max(0, Math.min(1, (props.now - (event.receivedAtMs ?? props.now)) / props.windowMs));
    const angle = -80 + age * 300;
    const radius = lane.radius + ((index % 3) - 1) * 5;
    return { event, lane: lane.key, attention: isAttentionEvent(event), ...point(radius, angle), tail: point(radius - 9, angle) };
  }));
});
const channels = computed(() => lanes.value.map((lane, index) => {
  const events = props.events.filter(e => laneForEvent(e) === lane.key);
  const y = 90 + index * 142;
  const anchor = point(lane.radius, -24 + index * 14);
  return { ...lane, events, latest: events[0], y,
    path: "M " + anchor.x + " " + anchor.y + " C 640 " + anchor.y + ", 609 " + (y + 43) + ", 680 " + (y + 43) };
}));
const shortTarget = (event?: AgentEvent) => {
  if (!event) return text("等待事件", "Awaiting events");
  const value = event.netEndpoint || event.path || event.extraInfo || event.comm;
  return value.length > 31 ? "…" + value.slice(-30) : value;
};
</script>

<template>
  <div ref="orbitRoot" class="behavior-orbit" :class="{ 'is-paused': paused, compact }">
    <svg :viewBox="compact ? '0 0 650 650' : '0 0 970 650'" class="orbit-svg" :aria-label="text('行为轨道，按时间和类型展示事件', 'Behavior orbit showing events by age and category')" role="group">
      <g class="orbit-guides" aria-hidden="true">
        <line x1="16" y1="318" x2="636" y2="318" />
        <line x1="326" y1="10" x2="326" y2="626" />
        <circle cx="326" cy="318" r="303" />
        <line v-for="tick in ticks" :key="tick.index" :x1="tick.start.x" :y1="tick.start.y" :x2="tick.end.x" :y2="tick.end.y" :class="{ major: tick.index % 5 === 0 }" />
        <text x="373" y="21">{{ text('现在', 'Now') }}</text>
        <text x="617" y="366">−{{ Math.round(windowMs / 4000) }}s</text>
        <text x="270" y="637">−{{ Math.round(windowMs / 2000) }}s</text>
        <text x="9" y="280">−{{ Math.round(windowMs * .75 / 1000) }}s</text>
      </g>
      <g v-for="lane in lanes" :key="lane.key" class="orbit-track" :class="{ dimmed: activeLane !== 'all' && activeLane !== lane.key }" aria-hidden="true">
        <circle cx="326" cy="318" :r="lane.radius - 12" class="track-boundary" />
        <circle cx="326" cy="318" :r="lane.radius" class="track-line" />
        <circle cx="326" cy="318" :r="lane.radius + 12" class="track-boundary" />
        <text :x="326 - lane.radius + 15" y="321">{{ lane.code }}</text>
      </g>
      <g v-for="channel in channels" v-show="!compact" :key="channel.key" :class="['orbit-channel', { dimmed: activeLane !== 'all' && activeLane !== channel.key }]">
        <path :d="channel.path" class="channel-bed" />
        <path v-if="channel.latest" :d="channel.path" class="channel-signal" :class="{ attention: channel.events.some(isAttentionEvent), quiet: now - (channel.latest.receivedAtMs ?? 0) > 5000 }" />
        <circle cx="680" :cy="channel.y + 43" r="4" class="channel-terminal" />
        <foreignObject x="691" :y="channel.y" width="264" height="113">
          <button class="channel-card" :aria-pressed="activeLane === channel.key" @click="emit('lane', channel.key)">
            <span class="channel-heading"><span>{{ channel.code }} / {{ channel.name }}</span><strong>{{ channel.events.length }}</strong></span>
            <span class="channel-call" data-i18n-ignore>{{ channel.latest?.type || '—' }}</span>
            <span class="channel-target" data-i18n-ignore>{{ shortTarget(channel.latest) }}</span>
          </button>
        </foreignObject>
      </g>
      <g aria-hidden="true" class="orbit-core">
        <circle cx="326" cy="318" r="109" class="core-outer" />
        <circle cx="326" cy="318" r="100" class="core-inner" />
        <text x="326" y="282" class="core-label">{{ centerLabel }}</text>
        <text x="326" y="330" class="core-count">{{ events.length.toLocaleString() }}</text>
        <text x="326" y="358" class="core-caption">{{ compact ? text('条事件', 'events') : text('窗口内事件', 'events in window') }}</text>
        <line x1="299" y1="377" x2="353" y2="377" />
        <text x="326" y="402" class="core-clock">{{ Math.round(windowMs / 1000) }}s</text>
      </g>
      <g class="orbit-events">
        <g v-for="mark in marks" :key="mark.event.key" class="orbit-event" :class="{ selected: selectedKey === mark.event.key, attention: mark.attention, dimmed: activeLane !== 'all' && activeLane !== mark.lane }"
          role="button" tabindex="0" :aria-pressed="selectedKey === mark.event.key"
          :aria-label="[mark.event.comm, mark.event.type, mark.event.netEndpoint || mark.event.path, mark.attention ? text('需关注', 'Needs attention') : ''].join(' ')"
          @click="emit('select', mark.event)" @keydown.enter.prevent="emit('select', mark.event)" @keydown.space.prevent="emit('select', mark.event)">
          <title>{{ mark.event.comm }} · {{ mark.event.type }} · {{ mark.event.netEndpoint || mark.event.path }}</title>
          <circle :cx="mark.x" :cy="mark.y" r="11" class="event-hit" />
          <line :x1="mark.tail.x" :y1="mark.tail.y" :x2="mark.x" :y2="mark.y" class="event-stem" />
          <rect v-if="mark.attention" :x="mark.x - 4" :y="mark.y - 4" width="8" height="8" :transform="'rotate(45 ' + mark.x + ' ' + mark.y + ')'" class="event-dot" />
          <circle v-else :cx="mark.x" :cy="mark.y" r="3" class="event-dot" />
          <circle :cx="mark.x" :cy="mark.y" r="8" class="event-selection" />
        </g>
      </g>
    </svg>
    <div v-if="compact" class="compact-channels"><button v-for="channel in channels" :key="channel.key" :aria-pressed="activeLane === channel.key" @click="emit('lane', channel.key)"><span>{{ channel.code }} / {{ channel.name }}</span><strong>{{ channel.events.length }}</strong></button></div>
    <div class="orbit-key"><span><i class="key-dot" />{{ text('已观测事件', 'Observed event') }}</span><span><i class="key-diamond" />{{ text('需关注', 'Needs attention') }}</span><span>{{ text('越靠近起点，事件越新', 'Newer events sit near Now') }}</span></div>
  </div>
</template>

<style scoped>
.behavior-orbit { min-width: 0; container-type: inline-size; }
.orbit-svg { width: 100%; height: auto; max-height: 440px; display: block; overflow: visible; }
.compact .orbit-svg { max-height: 420px; }
.compact-channels { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 6px; padding: 4px 14px 12px; }
.compact-channels button { display: flex; flex-direction: column; gap: 6px; text-align: left; font: 11px var(--font-sans); color: var(--text-secondary); padding: 9px; background: var(--surface-0); border: .5px solid var(--border); border-radius: 8px; cursor: pointer; min-height: 58px; }
.compact-channels strong { font: 500 19px var(--font-mono); color: var(--text-accent); }
.compact-channels button[aria-pressed="true"] { background: var(--bg-accent); border-color: var(--text-accent); }
.compact-channels button:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
.orbit-guides { fill: var(--text-muted); font: 17px var(--font-mono); }
.orbit-guides circle, .orbit-guides line { fill: none; stroke: var(--border); stroke-width: .65; }
.orbit-guides .major { stroke: var(--text-muted); }
.orbit-guides text { stroke: none; }
.orbit-track { transition: opacity .2s; }
.track-boundary { fill: none; stroke: var(--border); stroke-width: .5; }
.track-line { fill: none; stroke: var(--border-strong); stroke-width: .7; stroke-dasharray: 1 5; opacity: .6; }
.orbit-track text { fill: var(--text-muted); font: 17px var(--font-mono); }
.channel-bed { fill: none; stroke: var(--border); stroke-width: 5; }
.channel-signal { fill: none; stroke: var(--text-accent); stroke-width: 1.5; stroke-dasharray: 5 12; animation: flow 2.5s linear infinite; }
.channel-signal.attention { stroke: var(--text-warning); }
.channel-signal.quiet { animation: none; stroke-dasharray: none; opacity: .45; }
.channel-terminal { fill: var(--surface-2); stroke: var(--text-muted); }
.channel-card { width: 100%; height: 105px; display: flex; flex-direction: column; text-align: left; gap: 7px; padding: 12px 15px; border: .5px solid var(--border); border-left: 2px solid var(--text-accent); border-radius: 8px; background: var(--surface-2); color: var(--text-primary); cursor: pointer; font-family: var(--font-sans); }
.channel-card:hover, .channel-card[aria-pressed="true"] { background: var(--bg-accent); border-color: var(--text-accent); }
.channel-heading { display: flex; justify-content: space-between; align-items: center; gap: 8px; font-size: 17px; color: var(--text-secondary); }
.channel-heading strong { font: 500 26px var(--font-mono); color: var(--text-accent); }
.channel-call { font: 500 20px var(--font-mono); }
.channel-target { font: 17px var(--font-mono); color: var(--text-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.core-outer { fill: var(--surface-0); stroke: var(--border); stroke-width: 1; }
.core-inner { fill: var(--surface-2); stroke: var(--border); stroke-dasharray: 3 5; }
.orbit-core text { text-anchor: middle; }
.core-label { fill: var(--text-secondary); font: 18px var(--font-mono); }
.core-count { fill: var(--text-primary); font: 400 45px var(--font-mono); letter-spacing: -2px; }
.core-caption { fill: var(--text-muted); font: 18px var(--font-sans); }
.core-clock { fill: var(--text-accent); font: 18px var(--font-mono); }
.orbit-core line { stroke: var(--border-strong); }
.event-dot { fill: var(--text-accent); stroke: var(--surface-2); stroke-width: 1; }
.event-stem { stroke: var(--text-accent); stroke-width: 1; opacity: .6; }
.attention .event-dot { fill: var(--text-warning); }
.attention .event-stem { stroke: var(--text-warning); }
.event-hit { fill: transparent; }
.orbit-event { cursor: pointer; outline: none; }
.event-selection { fill: none; stroke: var(--text-primary); stroke-width: 1.5; opacity: 0; }
.orbit-event:hover .event-selection, .orbit-event:focus-visible .event-selection, .orbit-event.selected .event-selection { opacity: 1; }
.dimmed { opacity: .2; }
.orbit-key { display: flex; flex-wrap: wrap; justify-content: center; align-items: center; gap: 18px; padding: 4px 12px 14px; color: var(--text-muted); font-size: 11px; }
.orbit-key span { display: inline-flex; gap: 7px; align-items: center; }
.key-dot { width: 5px; height: 5px; border-radius: 50%; background: var(--text-accent); }
.key-diamond { width: 6px; height: 6px; transform: rotate(45deg); background: var(--text-warning); }
.channel-card:focus-visible { outline: 2px solid var(--text-accent); outline-offset: -3px; }
.is-paused .channel-signal { animation-play-state: paused; }
@keyframes flow { to { stroke-dashoffset: -68; } }
@media (prefers-reduced-motion: reduce) { .channel-signal { animation: none; } .orbit-track { transition: none; } }
@container (max-width: 450px) { .compact-channels { grid-template-columns: repeat(2, minmax(0, 1fr)); } .compact .orbit-svg { max-height: 330px; } .orbit-guides, .orbit-track text { font-size: 28px; } .core-label, .core-caption, .core-clock { font-size: 26px; } .core-count { font-size: 60px; } }
@media (forced-colors: active) { .event-dot { fill: LinkText; } .attention .event-dot { fill: Mark; } .channel-card { border: 1px solid ButtonText; } .channel-signal { stroke: LinkText; } }
</style>
