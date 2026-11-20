<script setup lang="ts">
import { computed } from "vue";
import { webLocale } from "../../i18n";
import type { BehaviorSnapshot } from "../../composables/behavior/behaviorModel";
const props = defineProps<{ snapshot: BehaviorSnapshot; windowMs: number }>();
const text = (zh: string, en: string) => webLocale.value === "en-US" ? en : zh;
const peak = computed(() => Math.max(1, ...props.snapshot.histogram));
const sparkline = computed(() => props.snapshot.histogram.map((value, i) => (i * 280 / 29) + "," + (63 - value / peak.value * 52)).join(" "));
const laneName = (key: string) => ({ file: text("文件", "Files"), network: text("网络", "Network"), process: text("进程", "Processes"), other: text("其他", "Other") })[key];
const bytes = computed(() => {
  const n = props.snapshot.byteCount;
  return n >= 1048576 ? (n / 1048576).toFixed(1) + " MiB" : n >= 1024 ? (n / 1024).toFixed(1) + " KiB" : n + " B";
});
</script>
<template>
  <div class="behavior-telemetry">
    <section class="telemetry-card">
      <h3>{{ text('事件速率', 'Event rate') }} <span>{{ text('最近 5 秒', 'Last 5 seconds') }}</span></h3>
      <div class="metric-number">{{ snapshot.rate.toFixed(1) }}<small> {{ text('次 / 秒', 'events / s') }}</small></div>
      <svg viewBox="0 0 280 70" preserveAspectRatio="none" class="rate-chart" role="img" :aria-label="text('窗口内事件速率趋势', 'Event activity trend in the window')"><line x1="0" y1="64" x2="280" y2="64" /><polyline :points="sparkline" /></svg>
    </section>
    <section class="telemetry-card">
      <h3>{{ text('活动密度', 'Activity density') }} <span>{{ windowMs / 1000 }}s</span></h3>
      <div class="density-chart" role="img" :aria-label="text('30 个时间桶的事件数量', 'Event counts across 30 time buckets')"><div v-for="(count, i) in snapshot.histogram" :key="i" class="density-column" :style="{ height: Math.max(3, count / peak * 100) + '%', opacity: count ? 1 : .2 }"><span>{{ count }}</span></div></div>
      <div class="chart-axis"><span>−{{ windowMs / 1000 }}s</span><span>{{ text('现在', 'Now') }}</span></div>
    </section>
    <section class="telemetry-card">
      <h3>{{ text('行为构成', 'Behavior mix') }} <span>{{ snapshot.events.length }} {{ text('条', 'events') }}</span></h3>
      <div class="lane-breakdown"><div v-for="lane in snapshot.lanes" :key="lane.key"><span>{{ laneName(lane.key) }}</span><meter :min="0" :max="Math.max(1, snapshot.events.length)" :value="lane.count" :aria-label="laneName(lane.key)" /><code>{{ lane.count }}</code></div></div>
    </section>
    <section class="telemetry-card evidence-metric">
      <h3>{{ text('证据摘要', 'Evidence summary') }}</h3>
      <div><span>{{ text('需关注事件', 'Needs attention') }}</span><strong :class="{ warning: snapshot.attentionCount > 0 }">{{ snapshot.attentionCount }}</strong></div>
      <div><span>{{ text('已记录字节', 'Recorded bytes') }}</span><code>{{ bytes }}</code></div>
      <p>{{ text('风险与决策仅表示记录的证据。', 'Risk and decisions reflect recorded evidence.') }}</p>
    </section>
  </div>
</template>
<style scoped>
.behavior-telemetry { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.telemetry-card { min-width: 0; padding: 16px; background: var(--surface-2); border: .5px solid var(--border); border-radius: 12px; }
h3 { display: flex; justify-content: space-between; align-items: center; gap: 8px; margin: 0 0 12px; font-size: 13px; font-weight: 500; }
h3 span { font-size: 11px; color: var(--text-muted); font-weight: 400; }
.metric-number { font: 400 27px var(--font-mono); }
.metric-number small { font: 11px var(--font-sans); color: var(--text-muted); }
.rate-chart { width: 100%; height: 60px; margin-top: 8px; overflow: visible; }
.rate-chart line { stroke: var(--border); stroke-width: .5; }
.rate-chart polyline { fill: none; stroke: var(--text-accent); stroke-width: 1.5; vector-effect: non-scaling-stroke; }
.density-chart { height: 88px; display: flex; align-items: end; gap: 3px; padding-top: 8px; }
.density-column { flex: 1; min-width: 1px; background: var(--text-accent); position: relative; }
.density-column span { display: none; }
.chart-axis { display: flex; justify-content: space-between; margin-top: 6px; font: 11px var(--font-mono); color: var(--text-muted); }
.lane-breakdown { display: grid; gap: 9px; }
.lane-breakdown > div { display: grid; grid-template-columns: 52px 1fr 30px; align-items: center; gap: 8px; font-size: 11px; color: var(--text-secondary); }
meter { appearance: none; width: 100%; height: 5px; border: 0; background: var(--surface-1); }
meter::-webkit-meter-bar { background: var(--surface-1); height: 5px; border: 0; border-radius: 0; }
meter::-webkit-meter-optimum-value { background: var(--text-accent); }
meter::-moz-meter-bar { background: var(--text-accent); }
.lane-breakdown code { text-align: right; }
.evidence-metric > div { display: flex; align-items: center; justify-content: space-between; margin: 8px 0; gap: 8px; font-size: 12px; color: var(--text-secondary); }
.evidence-metric strong { font: 500 25px var(--font-mono); color: var(--text-primary); }
.evidence-metric strong.warning { color: var(--text-warning); }
.evidence-metric code { color: var(--text-primary); }
.evidence-metric p { margin: 12px 0 0; color: var(--text-muted); font-size: 11px; }
@container (max-width: 900px) { .behavior-telemetry { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@container (max-width: 480px) { .behavior-telemetry { grid-template-columns: minmax(0, 1fr); } }
@media (forced-colors: active) { .density-column { background: LinkText; } .rate-chart polyline { stroke: LinkText; } }
</style>
