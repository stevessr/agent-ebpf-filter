<script setup lang="ts">
import { computed } from "vue";
import HarnessIcon from "../common/HarnessIcon.vue";
import { useDashboardSnapshot } from "../../composables/dashboard/useDashboardSnapshot";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { buildDashboardTriage } from "../../composables/dashboard/dashboardTriage";

const props = defineProps<{
  events: AgentEvent[];
  isConnected: boolean;
  isPaused: boolean;
  capacity: number;
  evictedRecords: number;
}>();
const emit = defineEmits<{
  inspect: [event: AgentEvent];
  raw: [];
  pause: [];
}>();
const snapshot = useDashboardSnapshot(() => props.events);
const summary = computed(() => buildDashboardTriage(snapshot.events.value));
const updated = computed(() =>
  snapshot.updatedAt.value
    ? new Date(snapshot.updatedAt.value).toLocaleTimeString()
    : "",
);
const columns = [
  { title: "优先级 / 依据", key: "priority", width: 180 },
  { title: "进程与活动", key: "activity", width: 190 },
  { title: "目标", key: "target", ellipsis: true },
  { title: "出现次数", dataIndex: "count", width: 105 },
  { title: "证据", key: "action", width: 100 },
];
</script>

<template>
  <section class="triage" aria-label="自动整理事件">
    <header class="triage-header">
      <div>
        <h2>先看重点，再看证据</h2>
        <p>
          自动归组重复活动，优先显示阻断、告警和失败调用。不会自动改变安全策略。
        </p>
      </div>
      <a-space wrap>
        <a-badge
          :status="isConnected ? 'success' : 'error'"
          :text="isConnected ? '已连接' : '未连接'"
        />
        <a-button @click="emit('pause')">{{
          isPaused ? "恢复接收" : "暂停接收"
        }}</a-button>
        <a-button @click="emit('raw')">查看事件明细</a-button>
      </a-space>
    </header>
    <a-alert
      v-if="isPaused"
      type="warning"
      show-icon
      message="接收已暂停；暂停期间的实时事件不会进入本页，也不会自动补回。"
    />
    <div class="triage-metrics">
      <div>
        <span>优先检查</span
        ><strong>{{ summary.counts.attention.toLocaleString() }}</strong
        ><small>阻断 / 告警 / 高风险评分</small>
      </div>
      <div>
        <span>失败调用</span
        ><strong>{{ summary.counts.failed.toLocaleString() }}</strong
        ><small>负返回值，不自动判定为威胁</small>
      </div>
      <div>
        <span>活动分组</span
        ><strong>{{ summary.groupCount.toLocaleString() }}</strong
        ><small
          >折叠 {{ summary.foldedRecords.toLocaleString() }} 条重复记录</small
        >
      </div>
      <div>
        <span>当前窗口</span
        ><strong>{{ summary.total.toLocaleString() }}</strong
        ><small
          >{{ summary.records.toLocaleString() }} 条记录 / 容量
          {{ capacity.toLocaleString() }}</small
        >
      </div>
    </div>
    <p class="scope-note">
      范围：本页内存窗口（含近期历史），不受明细筛选影响；并非全量历史审计。摘要每秒更新，最近
      {{ updated || "等待数据" }}。本次页面会话因容量限制移出
      {{ evictedRecords.toLocaleString() }} 条记录。
    </p>
    <a-alert
      v-if="summary.records && !summary.counts.attention"
      type="info"
      show-icon
      message="当前窗口未发现上述优先检查项；不代表系统没有风险或数据采集完整。"
    />
    <a-table
      class="triage-table"
      :columns="columns"
      :data-source="summary.groups"
      row-key="key"
      size="small"
      :scroll="{ x: 720 }"
      :pagination="{ pageSize: 10, showSizeChanger: false }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'priority'"
          ><a-tag
            :color="
              record.priority === 'attention'
                ? 'red'
                : record.priority === 'failed'
                  ? 'orange'
                  : 'default'
            "
            >{{
              record.priority === "attention"
                ? "优先检查"
                : record.priority === "failed"
                  ? "失败调用"
                  : "常规"
            }}</a-tag
          ><small class="reason">{{ record.reason }}</small></template
        >
        <template v-else-if="column.key === 'activity'"
          ><strong class="activity-comm"><HarnessIcon :harness="record.latest.comm" :size="16" />{{ record.latest.comm || "未知进程" }}</strong
          ><small class="reason"
            >PID {{ record.latest.pid }} · {{ record.latest.type }}</small
          ></template
        >
        <template v-else-if="column.key === 'target'"
          ><span :title="record.latest.netEndpoint || record.latest.path">{{
            record.latest.netEndpoint ||
            record.latest.path ||
            record.latest.toolName ||
            "—"
          }}</span></template
        >
        <template v-else-if="column.key === 'action'"
          ><a-button
            type="link"
            size="small"
            @click="emit('inspect', record.latest)"
            >最新样本</a-button
          ></template
        >
      </template>
      <template #emptyText>等待事件；请检查连接、追踪配置及采集状态。</template>
    </a-table>
    <p class="scope-note">
      按优先级、出现次数排序，最多展示前 50
      组。组内工具参数等可能不同，“最新样本”不是所有记录；完整窗口请切换事件明细。
    </p>
  </section>
</template>

<style scoped>
.activity-comm { display: inline-flex; align-items: center; gap: 6px; }
.triage {
  padding: 20px;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #fff;
}
.triage-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}
h2 {
  font-size: 20px;
  margin: 0 0 6px;
}
p,
small {
  color: #64748b;
}
.triage-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin: 20px 0 12px;
}
.triage-metrics > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
  background: #f8fafc;
  padding: 16px;
  border-radius: 8px;
}
.triage-metrics strong {
  font-size: 28px;
  font-variant-numeric: tabular-nums;
}
.scope-note {
  font-size: 12px;
  margin: 12px 0;
}
.reason {
  display: block;
}
.triage-table {
  margin-top: 16px;
}
@media (max-width: 850px) {
  .triage-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 480px) {
  .triage {
    padding: 12px;
  }
  .triage-metrics {
    grid-template-columns: 1fr;
  }
}
</style>
