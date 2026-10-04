<script setup lang="ts">
import type { ProcessInfo } from "../../composables/monitor/useMonitorData";
import type { RenewDestinationSummary } from "../../composables/renew/types";

defineProps<{
  processes: ProcessInfo[];
  destinations: RenewDestinationSummary[];
  trackedProcessCount: number;
}>();

const emit = defineEmits<{
  openProcesses: [];
  openNetwork: [];
}>();
</script>

<template>
  <section class="renew-panel">
    <div class="renew-panel__header">
      <div>
        <h2>系统速览</h2>
        <p>日常只保留最常看的进程与网络目标。</p>
      </div>
    </div>

    <div class="renew-snapshot">
      <div>
        <h3>高 CPU 进程</h3>
        <button
          v-for="process in processes"
          :key="process.pid"
          class="renew-row"
          @click="emit('openProcesses')"
        >
          <span>{{ process.name }}</span>
          <strong>{{ process.cpu.toFixed(1) }}%</strong>
        </button>
        <span v-if="!processes.length" class="renew-muted">等待系统指标…</span>
      </div>

      <div>
        <h3>常见网络目标</h3>
        <button
          v-for="item in destinations"
          :key="item.endpoint"
          class="renew-row"
          @click="emit('openNetwork')"
        >
          <span>{{ item.endpoint }}</span>
          <strong>{{ item.count }}</strong>
        </button>
        <span v-if="!destinations.length" class="renew-muted">暂无网络事件</span>
      </div>
    </div>

    <div v-if="trackedProcessCount" class="renew-tracked">
      当前跟踪 {{ trackedProcessCount }} 个已配置进程
    </div>
  </section>
</template>
