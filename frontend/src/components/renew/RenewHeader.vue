<script setup lang="ts">
import {
  PauseCircleOutlined,
  PlayCircleOutlined,
} from "@ant-design/icons-vue";

defineProps<{
  isConnected: boolean;
  isPaused: boolean;
  connectionLabel: string;
}>();

const emit = defineEmits<{
  togglePause: [];
}>();
</script>

<template>
  <header class="renew-header">
    <div>
      <div class="renew-eyebrow">日常监控</div>
      <h1>今天的 Agent 活动</h1>
      <p>把内核事件整理成日常可读的状态、异常和最近动作。</p>
    </div>
    <div class="renew-header__actions">
      <div class="renew-live" :class="{ 'renew-live--offline': !isConnected }">
        <span class="renew-live__dot" />
        {{ connectionLabel }}
      </div>
      <button
        class="renew-icon-button"
        :title="isPaused ? '继续事件流' : '暂停事件流'"
        @click="emit('togglePause')"
      >
        <PlayCircleOutlined v-if="isPaused" />
        <PauseCircleOutlined v-else />
      </button>
    </div>
  </header>
</template>
