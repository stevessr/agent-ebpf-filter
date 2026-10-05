<script setup lang="ts">
import { ArrowRightOutlined, CheckCircleFilled } from "@ant-design/icons-vue";

import type { AgentEvent } from "../../composables/dashboard/useDashboard";
import { HARNESS_LABELS, eventHarness } from "../../composables/renew/harness";
import type { RenewTone } from "../../composables/renew/types";

defineProps<{
  events: AgentEvent[];
  describeEvent: (event: AgentEvent) => string;
  eventTime: (event: AgentEvent) => string;
  eventTone: (event: AgentEvent) => RenewTone;
  eventLabel: (event: AgentEvent) => string;
  hasOlder: boolean;
  historyLoading: boolean;
}>();

const emit = defineEmits<{
  openEvents: [];
  openEvent: [eventId: string];
  loadOlder: [];
}>();
</script>

<template>
  <section class="renew-panel renew-panel--activity">
    <div class="renew-panel__header">
      <div>
        <h2>最近活动</h2>
        <p>默认隐藏底层字段，只保留“谁做了什么”。</p>
      </div>
      <button class="renew-link" @click="emit('openEvents')">
        完整事件流 <ArrowRightOutlined />
      </button>
    </div>

    <div v-if="events.length" class="renew-activity-list">
      <button
        v-for="event in events"
        :key="event.key"
        class="renew-activity"
        @click="event.eventId && emit('openEvent', event.eventId)"
      >
        <span
          class="renew-activity__status"
          :class="`is-${eventTone(event)}`"
        />
        <span class="renew-activity__body">
          <span class="renew-activity__top">
            <strong>{{
              `${HARNESS_LABELS[eventHarness(event)]} · ${event.comm || event.tag}`
            }}</strong>
            <span>{{ eventTime(event) }}</span>
          </span>
          <span class="renew-activity__text">{{ describeEvent(event) }}</span>
          <span class="renew-activity__meta">
            PID {{ event.pid }}
            <template v-if="event.toolName"> · {{ event.toolName }}</template>
            <template v-if="event.riskScore">
              · risk {{ event.riskScore }}</template
            >
          </span>
        </span>
        <span class="renew-activity__badge" :class="`is-${eventTone(event)}`">
          {{ eventLabel(event) }}
        </span>
      </button>
    </div>

    <div v-else class="renew-empty">
      <CheckCircleFilled />
      <strong>还没有匹配的 Agent 活动</strong>
      <span>启动已接入的 Agent 后，这里会自动出现行为摘要。</span>
    </div>

    <div v-if="hasOlder" class="renew-panel__pager">
      <button
        class="renew-link"
        :disabled="historyLoading"
        @click="emit('loadOlder')"
      >
        {{ historyLoading ? "正在读取后端历史…" : "加载更早记录" }}
      </button>
      <span>只加载紧凑摘要；完整内容仍按单条详情读取。</span>
    </div>
  </section>
</template>
