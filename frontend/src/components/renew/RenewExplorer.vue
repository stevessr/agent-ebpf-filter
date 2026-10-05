<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import type { ProcessInfo } from "../../composables/monitor/useMonitorData";
import {
  HARNESS_LABELS,
  eventHarness,
  sessionKey,
  type Harness,
} from "../../composables/renew/harness";
import {
  describeEvent,
  eventTime,
  eventLabel,
} from "../../composables/renew/eventPresentation";

const props = defineProps<{
  section: "events" | "network" | "processes";
  events: AgentEvent[];
  processes: (ProcessInfo & { harness: Harness; rootPid: number })[];
  hasOlder: boolean;
  historyLoading: boolean;
  connected: boolean;
}>();
const emit = defineEmits<{ openEvent: [id: string]; loadOlder: [] }>();
const query = ref("");
const type = ref("");
const session = ref("");
const limit = ref(50);
const selectedPid = ref<number | null>(null);
const title = computed(
  () => ({ events: "事件", network: "网络", processes: "进程" })[props.section],
);
const baseEvents = computed(() =>
  props.events.filter(
    (e) =>
      props.section !== "network" ||
      Boolean(
        e.netEndpoint ||
        e.netDirection ||
        e.type.includes("network") ||
        e.type === "dns",
      ),
  ),
);
const types = computed(() =>
  [...new Set(baseEvents.value.map((e) => e.type))].sort(),
);
const sessions = computed(() => [...new Set(baseEvents.value.map(sessionKey))]);
const rows = computed(() =>
  baseEvents.value.filter(
    (e) =>
      (!type.value || e.type === type.value) &&
      (!session.value || sessionKey(e) === session.value) &&
      (!query.value.trim() ||
        [
          e.comm,
          e.tag,
          e.pid,
          e.path,
          e.netEndpoint,
          e.domain,
          e.agentRunId,
          e.conversationId,
          e.decision,
        ]
          .join(" ")
          .toLowerCase()
          .includes(query.value.trim().toLowerCase())),
  ),
);
const processRows = computed(() =>
  props.processes
    .filter((p) =>
      [p.name, p.pid, p.cmdline, p.user]
        .join(" ")
        .toLowerCase()
        .includes(query.value.trim().toLowerCase()),
    )
    .sort((a, b) => b.cpu - a.cpu),
);
const selected = computed(() =>
  props.processes.find((p) => p.pid === selectedPid.value),
);
const hasFilters = computed(() =>
  Boolean(query.value.trim() || type.value || session.value),
);
const clearFilters = () => {
  query.value = "";
  type.value = "";
  session.value = "";
};
const destinations = computed(() => {
  const result = new Map<
    string,
    {
      key: string;
      endpoint: string;
      harness: Harness;
      count: number;
      bytes: number;
      pids: Set<number>;
    }
  >();
  for (const e of rows.value) {
    if (!e.netEndpoint && !e.domain) continue;
    const harness = eventHarness(e);
    const endpoint = e.netEndpoint || e.domain || "";
    const key = `${harness}:${endpoint}`;
    const d = result.get(key) || {
      key,
      endpoint,
      harness,
      count: 0,
      bytes: 0,
      pids: new Set<number>(),
    };
    d.count++;
    d.bytes += e.netBytes || 0;
    d.pids.add(e.pid);
    result.set(key, d);
  }
  return [...result.values()].sort((a, b) => b.count - a.count);
});
watch(
  () => props.section,
  () => {
    query.value = "";
    type.value = "";
    session.value = "";
    limit.value = 50;
    selectedPid.value = null;
  },
);
watch([query, type, session], () => {
  limit.value = 50;
});
watch(types, (values) => {
  if (type.value && !values.includes(type.value)) type.value = "";
});
watch(sessions, (values) => {
  if (session.value && !values.includes(session.value)) session.value = "";
});
</script>
<template>
  <div class="renew-explorer">
    <header class="renew-monitoring__header">
      <div>
        <div class="renew-eyebrow">RENEW · {{ title }}</div>
        <h1>{{ title }}</h1>
        <p>
          {{
            section === "processes"
              ? "实时进程快照；子进程按当前父链归属 Agent 工具。点击查看命令行。"
              : "紧凑事件摘要；点击按需读取完整详情。可按 Agent 工具、会话与类型分别查看。"
          }}
        </p>
      </div>
      <span class="renew-chip">{{
        connected ? "实时采集中" : "采集端离线 · 显示已加载数据"
      }}</span>
    </header>
    <div class="renew-filterbar">
      <input
        v-model="query"
        :aria-label="`${title}搜索`"
        placeholder="搜索 PID、命令、路径或目标…"
      />
      <template v-if="section !== 'processes'">
        <select v-model="type" aria-label="事件类型">
          <option value="">全部类型</option>
          <option v-for="t in types" :key="t">{{ t }}</option>
        </select>
        <select v-model="session" aria-label="会话筛选">
          <option value="">全部会话</option>
          <option v-for="s in sessions" :key="s">{{ s }}</option>
        </select>
        <span>{{ rows.length }} 条已加载摘要</span>
        <button v-if="hasFilters" class="renew-link" @click="clearFilters">
          清除筛选
        </button>
      </template
      ><span v-else>{{ processRows.length }} 个进程</span>
    </div>
    <section v-if="section === 'network'" class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>访问目标</h2>
          <p>按 Agent 工具分组；计数与字节仅统计已加载摘要，不是全机流量。</p>
        </div>
      </div>
      <div class="renew-table-wrap">
        <table class="renew-table">
          <thead>
            <tr>
              <th>Agent 工具</th>
              <th>目标</th>
              <th>进程 PID</th>
              <th>事件</th>
              <th>摘要字节</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in destinations.slice(0, limit)" :key="d.key">
              <td>{{ HARNESS_LABELS[d.harness] }}</td>
              <td>{{ d.endpoint }}</td>
              <td>{{ [...d.pids].join(", ") }}</td>
              <td>{{ d.count }}</td>
              <td>{{ d.bytes.toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="!destinations.length" class="renew-empty">
          当前筛选没有网络目标
        </p>
      </div>
    </section>
    <section v-if="section === 'processes'" class="renew-panel">
      <div class="renew-table-wrap">
        <table class="renew-table">
          <thead>
            <tr>
              <th>进程</th>
              <th>Harness</th>
              <th>PID / PPID</th>
              <th>CPU</th>
              <th>内存</th>
              <th>用户</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in processRows.slice(0, limit)" :key="p.pid">
              <td>
                <button class="renew-link" @click="selectedPid = p.pid">
                  {{ p.name }}
                </button>
              </td>
              <td>
                {{ HARNESS_LABELS[p.harness]
                }}<small v-if="p.rootPid !== p.pid">
                  · 根 PID {{ p.rootPid }}</small
                >
              </td>
              <td>{{ p.pid }} / {{ p.ppid }}</td>
              <td>{{ p.cpu.toFixed(1) }}%</td>
              <td>{{ (p.mem / 1048576).toFixed(1) }} MiB</td>
              <td>{{ p.user }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="!processRows.length" class="renew-empty">
          当前筛选没有进程；实时快照需要采集端连接。
        </p>
      </div>
      <div v-if="selected" class="renew-process-detail">
        <button class="renew-link" @click="selectedPid = null">收起详情</button>
        <h2>{{ selected.name }} · PID {{ selected.pid }}</h2>
        <p>
          {{ HARNESS_LABELS[selected.harness] }} · 根 PID {{ selected.rootPid }}
        </p>
        <pre>{{ selected.cmdline || "后端未提供命令行" }}</pre>
      </div>
    </section>
    <section v-else class="renew-panel">
      <div class="renew-panel__header">
        <div>
          <h2>{{ section === "network" ? "网络活动" : "事件流" }}</h2>
          <p>Agent 工具与会话分别标记，未可靠识别的记录不会猜测归属。</p>
        </div>
      </div>
      <div class="renew-activity-list">
        <button
          v-for="e in rows.slice(0, limit)"
          :key="e.key"
          class="renew-activity"
          @click="e.eventId && emit('openEvent', e.eventId)"
        >
          <span class="renew-activity__body"
            ><span class="renew-activity__top"
              ><strong
                >{{ HARNESS_LABELS[eventHarness(e)] }} · {{ e.comm }}</strong
              ><span>{{ eventTime(e) }}</span></span
            ><span class="renew-activity__text">{{ describeEvent(e) }}</span
            ><span class="renew-activity__meta"
              >PID {{ e.pid }} · {{ e.type }} ·
              {{
                e.agentRunId ||
                e.conversationId ||
                `根 PID ${e.rootAgentPid || e.pid}`
              }}</span
            ></span
          ><span class="renew-chip">{{ eventLabel(e) }}</span>
        </button>
      </div>
      <p v-if="!rows.length" class="renew-empty">当前筛选没有匹配事件</p>
    </section>
    <div class="renew-panel__pager">
      <button
        v-if="
          (section === 'processes' ? processRows.length : rows.length) >
            limit ||
          (section === 'network' && destinations.length > limit)
        "
        class="renew-link"
        @click="limit += 50"
      >
        展开更多（每次 50 条）</button
      ><button
        v-if="section !== 'processes' && hasOlder"
        class="renew-link"
        :disabled="historyLoading"
        @click="emit('loadOlder')"
      >
        {{ historyLoading ? "正在读取…" : "加载更早记录" }}
      </button>
    </div>
  </div>
</template>
