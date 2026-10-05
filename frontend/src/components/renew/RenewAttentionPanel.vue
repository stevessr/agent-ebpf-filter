<script setup lang="ts">
import {
  AlertOutlined,
  CheckCircleFilled,
} from "@ant-design/icons-vue";

import type { AgentEvent } from "../../composables/dashboard/useDashboard";

defineProps<{
  events: AgentEvent[];
  describeEvent: (event: AgentEvent) => string;
  eventTime: (event: AgentEvent) => string;
  eventLabel: (event: AgentEvent) => string;
}>();

const emit = defineEmits<{
  openEvents: [];
  openEvent: [eventId: string];
}>();
</script>

<template>
  <section class="renew-panel renew-panel--attention">
    <div class="renew-panel__header">
      <div>
        <h2>需要关注</h2>
        <p>只有真的值得处理时才强调颜色。</p>
      </div>
      <AlertOutlined />
    </div>

    <div v-if="events.length" class="renew-attention-list">
      <button
        v-for="event in events"
        :key="event.key"
        class="renew-attention-item"
        @click="event.eventId && emit('openEvent', event.eventId)"
      >
        <div>
          <strong>{{ eventLabel(event) }}</strong>
          <span>{{ eventTime(event) }}</span>
        </div>
        <p>{{ describeEvent(event) }}</p>
      </button>
    </div>

    <div v-else class="renew-empty renew-empty--compact">
      <CheckCircleFilled />
      <strong>没有待处理异常</strong>
      <span>阻断、高风险和语义告警会集中显示在这里。</span>
    </div>
  </section>
</template>
