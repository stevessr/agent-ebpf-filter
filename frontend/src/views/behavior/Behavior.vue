<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { RadarChartOutlined, PauseOutlined, CaretRightOutlined, ReloadOutlined, ArrowRightOutlined, CloseOutlined, ExperimentOutlined } from "@ant-design/icons-vue";
import { webLocale } from "../../i18n";
import BehaviorOrbit from "../../components/behavior/BehaviorOrbit.vue";
import BehaviorTelemetry from "../../components/behavior/BehaviorTelemetry.vue";
import BehaviorSelectionDial from "../../components/behavior/BehaviorSelectionDial.vue";
import BehaviorAttentionNavigator from "../../components/behavior/BehaviorAttentionNavigator.vue";
import BehaviorReplayTransport from "../../components/behavior/BehaviorReplayTransport.vue";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { useDashboardStream } from "../../composables/dashboard/useDashboardStream";
import { formatTraceSummary } from "../../composables/dashboard/dashboardHelpers";
import { buildBehaviorSnapshot, createBehaviorDemo, laneForEvent, isAttentionEvent, type BehaviorLane } from "../../composables/behavior/behaviorModel";

const BehaviorThree = defineAsyncComponent(() => import("../../components/behavior/BehaviorThree.vue"));
const route = useRoute();
const visualMode = ref<"orbit" | "3d">(route.query.view === "3d" ? "3d" : "orbit");
watch(() => route.query.view, value => { if (value === "3d" || value === "orbit") visualMode.value = value; });
const router = useRouter();
const text = (zh: string, en: string) => webLocale.value === "en-US" ? en : zh;
const events = shallowRef<AgentEvent[]>([]);
const isConnected = ref(false);
const isPaused = ref(false);
const maxEvents = ref(3000);
const demo = ref(false);
const now = ref(Date.now());
const wallNow = ref(Date.now());
const windowMs = ref(60000);
const pid = ref<number | undefined>();
const lane = ref<BehaviorLane | "all">("all");
const attentionOnly = ref(false);
const selected = shallowRef<AgentEvent | null>(null);
const demoEvents = shallowRef<AgentEvent[]>([]);
const viewedKeys = shallowRef<string[]>([]);
const replaySpeed = ref(1);
const replayLoop = ref(true);
const navigatorOpen = ref(true);
let demoStart = 0;
let timer: ReturnType<typeof setInterval> | undefined;
const stream = useDashboardStream({ events, isConnected, isPaused, maxEvents, getFilteredEvents: () => events.value });
const { recoveryState, queuedRecords, droppedRecords, evictedRecords } = stream;
const availableEvents = computed(() => demo.value ? demoEvents.value.filter(e => (e.receivedAtMs ?? 0) <= now.value) : events.value);
const allSnapshot = computed(() => buildBehaviorSnapshot(availableEvents.value, now.value, windowMs.value));
const snapshot = computed(() => pid.value === undefined ? allSnapshot.value : buildBehaviorSnapshot(availableEvents.value, now.value, windowMs.value, pid.value));
const orbitEvents = computed(() => snapshot.value.events.filter(e => !attentionOnly.value || isAttentionEvent(e)));
const visibleEvents = computed(() => orbitEvents.value.filter(e => lane.value === "all" || laneForEvent(e) === lane.value));
const recentEvents = computed(() => visibleEvents.value.slice(0, 12));
const attentionScope = computed(() => snapshot.value.events.filter(event => lane.value === "all" || laneForEvent(event) === lane.value));
const dialCounts = computed(() => ({ all: snapshot.value.events.length, file: snapshot.value.lanes.find(item => item.key === "file")?.count ?? 0, network: snapshot.value.lanes.find(item => item.key === "network")?.count ?? 0, process: snapshot.value.lanes.find(item => item.key === "process")?.count ?? 0, other: snapshot.value.lanes.find(item => item.key === "other")?.count ?? 0, attention: snapshot.value.attentionCount }));
const replayPosition = computed(() => Math.max(0, Math.min(60, (now.value - demoStart) / 1000)));
const replayMarkers = computed(() => demoEvents.value.filter(event => isAttentionEvent(event) && (pid.value === undefined || event.pid === pid.value) && (lane.value === "all" || laneForEvent(event) === lane.value)).map(event => ({ seconds: ((event.receivedAtMs ?? demoStart) - demoStart) / 1000, event })).sort((a,b) => a.seconds-b.seconds));
const selectedOutside = computed(() => !!selected.value && !snapshot.value.events.some(event => event.key === selected.value?.key));
const lastEventAt = computed(() => events.value.reduce((latest, event) => Math.max(latest, event.receivedAtMs ?? 0), 0));
const sourceStatus = computed(() => {
  if (demo.value) return text("演示回放", "Demo replay");
  if (!isConnected.value) return text("实时流未连接", "Stream disconnected");
  if (recoveryState.value === "error") return text("历史补取失败", "History recovery failed");
  if (recoveryState.value === "loading") return text("正在补取记录", "Recovering history");
  if (lastEventAt.value > wallNow.value + 30000) return text("事件时钟异常", "Event clock skew");
  if (!lastEventAt.value || wallNow.value - lastEventAt.value > 15000) return text("已连接 · 等待新事件", "Connected · awaiting events");
  return text("实时流已连接", "Event stream connected");
});
const sourceWarning = computed(() => demo.value || !isConnected.value || recoveryState.value === "error" || !lastEventAt.value || wallNow.value - lastEventAt.value > 15000 || lastEventAt.value > wallNow.value + 30000);
const statusNote = computed(() => {
  if (demo.value) return text("合成数据回放，与真实采集事件独立。", "Synthetic replay, separate from captured events.");
  if (!isConnected.value) return text("未连接采集器，保留的事件仅供查看。连接后自动恢复。", "Collector disconnected. Retained events are historical; reconnection is automatic.");
  if (recoveryState.value === "error") return text("历史补取失败，可能缺少事件；可重试补取。", "History recovery failed; events may be missing. Retry recovery.");
  return text("仅展示当前有界窗口；空窗口不代表系统安全。", "A bounded observation window. An empty window does not establish safety.");
});
const laneName = (key: string) => ({ all: text("全部行为", "All behavior"), file: text("文件访问", "File access"), network: text("网络通信", "Network I/O"), process: text("进程执行", "Process execution"), other: text("其他信号", "Other signals") })[key] || key;
const centerLabel = computed(() => pid.value === undefined ? text("全部进程", "All processes") : "PID " + pid.value);
const setLane = (key: BehaviorLane) => { lane.value = lane.value === key ? "all" : key; };
const selectEvent = (event: AgentEvent) => {
  selected.value = event;
  if (!viewedKeys.value.includes(event.key)) viewedKeys.value = [...viewedKeys.value.slice(-2999), event.key];
};
const chooseDial = (selection: { lane: BehaviorLane | "all"; attentionOnly: boolean }) => {
  lane.value = selection.lane; attentionOnly.value = selection.attentionOnly; selected.value = null;
};
const seekDemo = (seconds: number, event?: AgentEvent) => {
  if (!demo.value || !Number.isFinite(seconds)) return;
  isPaused.value = true;
  now.value = demoStart + Math.max(0, Math.min(60, seconds)) * 1000;
  if (event) selectEvent(event); else selected.value = null;
};
const togglePause = () => {
  isPaused.value = !isPaused.value;
  if (!isPaused.value && !demo.value) now.value = Date.now();
};
const switchSource = (useDemo: boolean) => {
  isPaused.value = false;
  selected.value = null;
  pid.value = undefined;
  lane.value = "all";
  attentionOnly.value = false;
  viewedKeys.value = [];
  demo.value = useDemo;
  if (useDemo) {
    stream.stopStream();
    demoStart = Date.now() - 60000;
    demoEvents.value = createBehaviorDemo(demoStart + 60000);
    now.value = demoStart + 36000;
  } else {
    now.value = Date.now();
    stream.startStream();
  }
};
watch(() => route.query.demo, (value, previous) => {
  if (value === "1" && !demo.value) switchSource(true);
  else if (previous === "1" && value !== "1" && demo.value) switchSource(false);
});
const resetFocus = () => { pid.value = undefined; lane.value = "all"; attentionOnly.value = false; };
const openTrace = () => {
  if (!selected.value || demo.value) return;
  void router.push({ name: "ExecutionGraph", params: { tab: "topology" }, query: { pid: String(selected.value.pid) } });
};
const detailFields = computed(() => {
  const e = selected.value;
  if (!e) return [];
  return [
    { label: text("来源", "Source"), value: demo.value ? text("合成演示", "Synthetic demo") : text("采集事件", "Captured event") },
    { label: text("进程", "Process"), value: e.comm + " · PID " + e.pid },
    { label: text("父进程", "Parent process"), value: String(e.ppid) },
    { label: text("事件类型", "Event type"), value: e.type },
    { label: text("记录时间", "Recorded at"), value: e.receivedAtMs ? new Date(e.receivedAtMs).toLocaleString(webLocale.value) : e.time },
    { label: text("目标", "Target"), value: e.netEndpoint || e.path || "—" },
    { label: text("风险分数", "Risk score"), value: e.riskScore === undefined ? text("未记录", "Not recorded") : String(e.riskScore) },
    { label: text("策略决策", "Policy decision"), value: e.decision || text("未记录", "Not recorded") },
    { label: text("返回值", "Return value"), value: e.retval === undefined ? text("未记录", "Not recorded") : String(e.retval) },
    { label: text("事件标识", "Event identity"), value: e.eventId || e.key },
    ...(e.traceId ? [{ label: "Trace ID", value: e.traceId }] : []),
  ];
});
const timeLabel = (event: AgentEvent) => new Date(event.receivedAtMs ?? 0).toLocaleTimeString(webLocale.value, { hour12: false });
watch([pid, lane, attentionOnly, windowMs], () => { selected.value = null; });
onMounted(() => {
  switchSource(route.query.demo === "1");
  timer = setInterval(() => {
    wallNow.value = Date.now();
    if (isPaused.value) return;
    if (demo.value) {
      now.value += 1000 * replaySpeed.value;
      if (now.value > demoStart + 60000) {
        if (replayLoop.value) now.value = demoStart;
        else { now.value = demoStart + 60000; isPaused.value = true; }
      }
    } else now.value = wallNow.value;
  }, 1000);
});
onUnmounted(() => { if (timer) clearInterval(timer); stream.stopStream(); });
</script>

<template>
  <main class="behavior-studio" data-i18n-ignore>
    <header class="behavior-header">
      <div class="behavior-title"><div class="title-icon"><RadarChartOutlined /></div><div><h1>{{ text('行为可视化', 'Behavior Observatory') }}</h1><p>{{ text('让每一次行动，都留下可见的轨迹。', 'Every action leaves a visible trace.') }}</p></div></div>
      <div class="source-switch" role="group" :aria-label="text('事件来源', 'Event source')">
        <button :aria-pressed="!demo" @click="switchSource(false)">{{ text('实时观测', 'Live observation') }}</button>
        <button :aria-pressed="demo" @click="switchSource(true)"><ExperimentOutlined /> {{ text('演示回放', 'Demo replay') }}</button>
      </div>
    </header>

    <div class="behavior-status" role="status">
      <span class="source-indicator" :class="{ warning: sourceWarning }"><i />{{ sourceStatus }}</span>
      <span v-if="isPaused" class="pause-indicator">{{ text('显示已暂停', 'Presentation paused') }}<template v-if="!demo"> · {{ queuedRecords }} {{ text('条待显示', 'queued') }}</template></span>
      <span class="status-note">{{ statusNote }}</span>
      <button v-if="!demo && recoveryState === 'error'" class="text-button" @click="stream.loadRecentEvents()"><ReloadOutlined /> {{ text('重试', 'Retry') }}</button>
    </div>
    <p v-if="!demo && stream.continuityGap.value" class="stream-notice">{{ text('此会话曾断线或解码失败。仅补取最近 1000 条，无法保证间隙完整。', 'This session had a connection or decoding gap. Recovery fetches only the latest 1,000 records; continuity is not guaranteed.') }}</p>

    <div class="behavior-controls">
      <div class="control-filters">
        <label>{{ text('进程', 'Process') }}<select v-model="pid" :aria-label="text('按进程聚焦', 'Focus by process')"><option :value="undefined">{{ text('全部进程', 'All processes') }}</option><option v-for="process in allSnapshot.processes" :key="process.pid" :value="process.pid">{{ process.comm }} · {{ process.pid }}</option><option v-if="pid !== undefined && !allSnapshot.processes.some(p => p.pid === pid)" :value="pid">PID {{ pid }}</option></select></label>
        <label>{{ text('时间窗', 'Window') }}<select v-model="windowMs" :aria-label="text('观测时间窗', 'Observation window')"><option :value="30000">30s</option><option :value="60000">60s</option><option :value="300000">5min</option></select></label>
        <button class="attention-filter" :aria-pressed="attentionOnly" @click="attentionOnly = !attentionOnly"><span class="attention-symbol" />{{ text('仅需关注', 'Attention only') }}<code>{{ snapshot.attentionCount }}</code></button>
      </div>
      <button class="pause-button" :aria-pressed="isPaused" @click="togglePause"><CaretRightOutlined v-if="isPaused" /><PauseOutlined v-else />{{ isPaused ? text('继续', 'Resume') : text('暂停显示', 'Pause display') }}</button>
    </div>

    <BehaviorReplayTransport v-if="demo" :position="replayPosition" :paused="isPaused" :speed="replaySpeed" :loop="replayLoop" :markers="replayMarkers" @seek="seekDemo" @toggle="togglePause" @speed="replaySpeed = $event" @loop="replayLoop = !replayLoop" />
    <details class="behavior-navigation" :open="navigatorOpen" @toggle="navigatorOpen = ($event.target as HTMLDetailsElement).open">
      <summary>{{ text('行为导航台', 'Behavior navigation deck') }}<span>{{ text('转盘选择 · 优先核查 · 时间定位', 'Rotary selection · priority review · time navigation') }}</span></summary>
      <div class="behavior-navigation-grid"><BehaviorSelectionDial :counts="dialCounts" :active-lane="lane" :attention-only="attentionOnly" @choose="chooseDial" /><BehaviorAttentionNavigator :events="attentionScope" :now="now" :window-ms="windowMs" :selected-key="selected?.key" :viewed-keys="viewedKeys" @inspect="selectEvent" /></div>
    </details>
    <div class="behavior-workspace">
      <section class="orbit-panel">
        <div class="panel-heading"><h2>{{ visualMode === '3d' ? text('3D 行为轨道', '3D behavior orbit') : text('行为轨道', 'Behavior orbit') }}</h2><div class="visual-mode" role="group" :aria-label="text('可视化模式', 'Visualization mode')"><button :aria-pressed="visualMode === 'orbit'" @click="visualMode = 'orbit'">{{ text('平面轨道', 'Flat orbit') }}</button><button :aria-pressed="visualMode === '3d'" @click="visualMode = '3d'">{{ text('3D 空间', '3D space') }}</button></div></div>
        <BehaviorThree v-if="visualMode === '3d'" :status-label="sourceStatus + (stream.continuityGap.value ? ' · ' + text('曾发生数据间隙', 'Data continuity gap') : '') + (isPaused ? ' · ' + text('显示已暂停', 'Presentation paused') : '')" :source-warning="sourceWarning || stream.continuityGap.value" :playback-rate="demo ? replaySpeed : 1" :events="orbitEvents" :now="now" :window-ms="windowMs" :selected-key="selected?.key" :active-lane="lane" :paused="isPaused || (!demo && !isConnected)" :center-label="centerLabel" @select="selectEvent" @lane="setLane" />
        <BehaviorOrbit v-else :events="orbitEvents" :now="now" :window-ms="windowMs" :selected-key="selected?.key" :active-lane="lane" :paused="isPaused || (!demo && !isConnected)" :center-label="centerLabel" @select="selectEvent" @lane="setLane" />
        <div class="orbit-footer"><span>{{ allSnapshot.processes.length }} {{ text('个进程', 'processes') }} · {{ snapshot.events.length }} {{ text('条窗口记录', 'window records') }}</span><span>{{ text('轨道最多显示 96 个节点', 'Orbit shows up to 96 nodes') }}</span></div>
      </section>

      <aside class="evidence-panel">
        <template v-if="selected">
          <div class="panel-heading"><h2>{{ text('事件证据', 'Event evidence') }}</h2><button class="icon-button" :aria-label="text('关闭事件证据', 'Close event evidence')" @click="selected = null"><CloseOutlined /></button></div>
          <div class="event-inspector">
            <span class="event-kind" :class="{ warning: isAttentionEvent(selected) }">{{ isAttentionEvent(selected) ? text('需关注', 'Needs attention') : text('已观测', 'Observed') }}</span>
            <h3 data-i18n-ignore>{{ selected.type }}</h3>
            <p v-if="selectedOutside" class="evidence-note">{{ text('这条证据不在当前时间窗或筛选范围内，原始详情仍保留。', 'This evidence is outside the current time window or filters; its original details are retained.') }}</p>
            <p class="trace-summary" data-i18n-ignore>{{ formatTraceSummary(selected) }}</p>
            <dl><template v-for="field in detailFields" :key="field.label"><dt>{{ field.label }}</dt><dd data-i18n-ignore>{{ field.value }}</dd></template></dl>
            <p class="evidence-note">{{ text('策略决策不等于已确认的内核阻断，请结合返回值和审计证据判断。', 'A policy decision alone does not confirm kernel enforcement. Inspect return values and audit evidence.') }}</p>
            <button v-if="!demo" class="trace-button" @click="openTrace">{{ text('查看进程执行追踪', 'Open process trace') }}<ArrowRightOutlined /></button>
          </div>
        </template>
        <template v-else>
          <div class="panel-heading"><h2>{{ text('事件流', 'Event stream') }}</h2><span>{{ visibleEvents.length }} {{ text('条', 'events') }}</span></div>
          <div class="stream-filter"><span>{{ laneName(lane) }}<template v-if="pid !== undefined"> · PID {{ pid }}</template></span><button v-if="lane !== 'all' || pid !== undefined || attentionOnly" class="text-button" @click="resetFocus">{{ text('重置', 'Reset') }}</button></div>
          <div v-if="recentEvents.length" class="event-list">
            <button v-for="event in recentEvents" :key="event.key" class="stream-event" @click="selectEvent(event)">
              <span class="event-row"><span class="event-operation" :class="{ warning: isAttentionEvent(event) }"><i :class="{ diamond: isAttentionEvent(event) }" />{{ event.type }}</span><time>{{ timeLabel(event) }}</time></span>
              <span class="event-process">{{ event.comm }} <code>{{ event.pid }}</code><span v-if="isAttentionEvent(event)" class="attention-label">{{ text('需关注', 'Attention') }}</span></span>
              <span class="event-target">{{ event.netEndpoint || event.path || event.extraInfo || '—' }}</span>
            </button>
          </div>
          <div v-else class="stream-empty"><RadarChartOutlined /><h3>{{ text('等待行为信号', 'Awaiting behavior signals') }}</h3><p>{{ demo ? text('当前筛选下没有事件，试试其他轨道或进程。', 'No events match. Try another lane or process.') : text('连接采集器后，事件将在这里汇流。也可以先体验演示回放。', 'Events appear here when a collector connects. You can also explore the demo replay.') }}</p><button v-if="!demo" class="trace-button" @click="switchSource(true)">{{ text('体验演示回放', 'Explore demo replay') }}<ArrowRightOutlined /></button><button v-else class="text-button" @click="resetFocus">{{ text('重置筛选', 'Reset filters') }}</button></div>
        </template>
      </aside>
    </div>

    <BehaviorTelemetry :snapshot="snapshot" :window-ms="windowMs" />
    <footer class="behavior-footnote"><span>{{ text('仅展示已接收的事件；不持久化事件内容。', 'Shows received events only; event content is not persisted by this view.') }}</span><span v-if="demo">{{ text('演示进度', 'Replay position') }} {{ Math.floor((now - demoStart) / 1000) }} / 60s</span><span v-else>{{ text('缓存上限', 'Buffer limit') }} {{ maxEvents }} · {{ text('累计淘汰', 'Evicted') }} {{ evictedRecords + droppedRecords }}</span></footer>
  </main>
</template>

<style scoped>
.behavior-studio { width: 100%; min-height: 100%; box-sizing: border-box; padding: 24px; color: var(--text-primary); background: var(--surface-0); container-type: inline-size; }
.behavior-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; margin-bottom: 20px; }
.behavior-title { display: flex; align-items: center; gap: 13px; }
.title-icon { width: 42px; height: 42px; display: grid; place-items: center; border: .5px solid var(--border); color: var(--text-accent); border-radius: 12px; background: var(--surface-2); font-size: 23px; }
h1 { font-size: 22px; font-weight: 500; margin: 0; line-height: 1.5; }
.behavior-title p { margin: 2px 0 0; font-size: 12px; color: var(--text-muted); }
button, select { font-family: inherit; font-size: 12px; }
button { cursor: pointer; }
.source-switch { display: flex; flex-shrink: 0; padding: 3px; background: var(--surface-1); border: .5px solid var(--border); border-radius: 8px; }
.source-switch button { display: flex; gap: 6px; align-items: center; min-height: 32px; border: .5px solid transparent; border-radius: 6px; padding: 5px 13px; color: var(--text-secondary); background: transparent; }
.source-switch button[aria-pressed="true"] { background: var(--surface-2); color: var(--text-accent); border-color: var(--border); }
.behavior-status { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; font-size: 12px; }
.source-indicator { display: inline-flex; gap: 7px; align-items: center; color: var(--text-accent); }
.source-indicator i { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.source-indicator.warning, .pause-indicator { color: var(--text-warning); }
.status-note { color: var(--text-muted); font-size: 11px; }
.stream-notice { font-size: 11px; color: var(--text-warning); margin: 8px 0 0; }
.behavior-controls { margin: 18px 0 14px; display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.control-filters { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
.control-filters label { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--text-secondary); }
select { max-width: 210px; background: var(--surface-2); color: var(--text-primary); border: .5px solid var(--border); border-radius: 8px; padding: 7px 10px; height: 36px; }
.attention-filter, .pause-button { display: inline-flex; align-items: center; gap: 8px; min-height: 36px; padding: 6px 12px; background: var(--surface-2); color: var(--text-secondary); border: .5px solid var(--border); border-radius: 8px; }
.attention-filter code { font: 11px var(--font-mono); padding-left: 4px; }
.attention-filter[aria-pressed="true"] { background: var(--bg-warning); color: var(--text-warning); border-color: var(--text-warning); }
.attention-symbol { width: 6px; height: 6px; background: var(--text-warning); transform: rotate(45deg); }
.pause-button { flex-shrink: 0; }
.pause-button[aria-pressed="true"] { border-color: var(--text-accent); color: var(--text-accent); }
.behavior-navigation { margin-bottom: 14px; }
.behavior-navigation > summary { display: flex; align-items: center; gap: 12px; padding: 10px 2px; color: var(--text-primary); font-size: 13px; cursor: pointer; }
.behavior-navigation > summary span { font-size: 11px; color: var(--text-muted); }
.behavior-navigation > summary:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
.behavior-navigation-grid { display: grid; grid-template-columns: 250px minmax(0,1fr); gap: 14px; align-items: center; }
@container (max-width: 800px) { .behavior-navigation-grid { grid-template-columns: minmax(0,1fr); } .behavior-navigation > summary { flex-wrap: wrap; gap: 5px; } }
.behavior-workspace { display: grid; grid-template-columns: minmax(0, 1fr) 270px; gap: 14px; margin-bottom: 14px; }
.orbit-panel, .evidence-panel { background: var(--surface-2); border: .5px solid var(--border); border-radius: 12px; overflow: hidden; min-width: 0; }
.orbit-panel { display: flex; flex-direction: column; justify-content: space-between; }
.panel-heading { min-height: 44px; padding: 0 16px; display: flex; align-items: center; justify-content: space-between; gap: 8px; border-bottom: .5px solid var(--border); }
h2 { font-size: 13px; font-weight: 500; margin: 0; }
.panel-heading > span { font-size: 11px; color: var(--text-muted); }
.visual-mode { display: flex; gap: 3px; }
.visual-mode button { border: .5px solid transparent; border-radius: 6px; min-height: 30px; padding: 4px 10px; background: transparent; color: var(--text-muted); font-size: 11px; }
.visual-mode button[aria-pressed="true"] { color: var(--text-accent); background: var(--bg-accent); border-color: var(--border); }
.orbit-footer { display: flex; justify-content: space-between; gap: 12px; padding: 10px 16px; border-top: .5px solid var(--border); font: 11px var(--font-mono); color: var(--text-muted); }
.evidence-panel { display: flex; flex-direction: column; max-height: 600px; }
.stream-filter { display: flex; justify-content: space-between; align-items: center; gap: 8px; padding: 10px 16px; font-size: 11px; color: var(--text-muted); background: var(--surface-0); min-height: 18px; }
.event-list { flex: 1; overflow-y: auto; min-height: 0; scrollbar-width: thin; }
.stream-event { display: flex; flex-direction: column; width: 100%; padding: 13px 16px; text-align: left; border: 0; border-bottom: .5px solid var(--border); background: transparent; color: var(--text-primary); gap: 5px; }
.stream-event:hover { background: var(--bg-accent); }
.event-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.event-operation { display: inline-flex; align-items: center; gap: 7px; font: 500 12px var(--font-mono); color: var(--text-accent); overflow: hidden; text-overflow: ellipsis; }
.event-operation i { flex-shrink: 0; width: 5px; height: 5px; background: currentColor; border-radius: 50%; }
.event-operation i.diamond { border-radius: 0; transform: rotate(45deg); }
.event-operation.warning { color: var(--text-warning); }
time { font: 11px var(--font-mono); color: var(--text-muted); flex-shrink: 0; }
.event-process { display: flex; align-items: center; gap: 8px; font-size: 12px; }
.event-process code { font-size: 11px; color: var(--text-muted); }
.attention-label { margin-left: auto; color: var(--text-warning); font-size: 11px; }
.event-target { font: 11px/1.6 var(--font-mono); color: var(--text-muted); overflow: hidden; white-space: nowrap; text-overflow: ellipsis; max-width: 100%; }
.stream-empty { flex: 1; padding: 45px 22px; display: flex; flex-direction: column; align-items: flex-start; justify-content: center; gap: 12px; }
.stream-empty > .anticon { font-size: 32px; color: var(--text-muted); }
.stream-empty h3 { margin: 0; font-size: 16px; font-weight: 500; }
.stream-empty p { margin: 0; font-size: 13px; color: var(--text-muted); line-height: 1.8; }
.text-button { display: inline-flex; align-items: center; gap: 6px; border: 0; padding: 2px 0; background: transparent; color: var(--text-accent); font-size: 11px; }
.icon-button { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 6px; background: transparent; color: var(--text-muted); }
.icon-button:hover { background: var(--surface-1); }
.event-inspector { padding: 18px 16px; overflow-y: auto; font-size: 12px; }
.event-kind { display: inline-block; border: .5px solid var(--border); padding: 2px 7px; border-radius: 4px; color: var(--text-accent); font-size: 11px; }
.event-kind.warning { color: var(--text-warning); border-color: var(--text-warning); }
.event-inspector h3 { margin: 10px 0; font: 500 16px var(--font-mono); overflow-wrap: anywhere; }
.trace-summary { font: 11px/1.7 var(--font-mono); color: var(--text-secondary); background: var(--surface-0); border: .5px solid var(--border); padding: 10px; border-radius: 8px; overflow-wrap: anywhere; }
dl { margin: 16px 0; display: grid; gap: 4px; }
dt { color: var(--text-muted); font-size: 11px; }
dd { margin: 0 0 9px; overflow-wrap: anywhere; font-family: var(--font-mono); }
.evidence-note { color: var(--text-warning); font-size: 11px; line-height: 1.8; }
.trace-button { display: inline-flex; align-items: center; justify-content: space-between; gap: 12px; border: .5px solid var(--border); border-radius: 8px; padding: 9px 12px; min-height: 36px; background: var(--surface-0); color: var(--text-accent); font-size: 12px; }
.behavior-footnote { display: flex; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 14px; color: var(--text-muted); font-size: 11px; }
button:focus-visible, select:focus-visible { outline: 2px solid var(--text-accent); outline-offset: 2px; }
@container (max-width: 800px) { .behavior-workspace { grid-template-columns: minmax(0, 1fr); } .evidence-panel { max-height: 390px; } .event-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); } .stream-empty { padding: 24px; } }
@container (max-width: 620px) { .behavior-header { flex-direction: column; align-items: flex-start; gap: 14px; } .behavior-controls { align-items: flex-end; } .control-filters { gap: 8px; } .control-filters label { flex-wrap: wrap; } select { max-width: 150px; } .status-note { flex-basis: 100%; } .event-list { grid-template-columns: minmax(0, 1fr); } .panel-heading > span { max-width: 55%; text-align: right; } .orbit-footer { flex-wrap: wrap; } }
@media (max-width: 720px) { .behavior-studio { padding: 14px; } }
@media (forced-colors: active) { .attention-symbol, .event-operation i, .source-indicator i { background: ButtonText; } button[aria-pressed="true"] { outline: 1px solid Highlight; } }
</style>
