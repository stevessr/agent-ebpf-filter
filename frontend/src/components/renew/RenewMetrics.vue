<script setup lang="ts">
import type { RenewTone } from "../../composables/renew/types";

defineProps<{
  isConnected: boolean;
  eventCount: number;
  agentEventCount: number;
  cpuTotal: number;
  processCount: number;
  memPercent: number;
  memUsed: number;
  memTotal: number;
  attentionCount: number;
  blockedCount: number;
  alertTone: RenewTone;
  formatBytes: (bytes: number) => string;
}>();
</script>

<template>
  <section class="renew-metrics" aria-label="System overview">
    <article class="renew-metric">
      <div class="renew-metric__label">采集状态</div>
      <div class="renew-metric__value">
        {{ isConnected ? "Online" : "Offline" }}
      </div>
      <div class="renew-metric__meta">
        当前缓冲区 {{ eventCount }} 条 · Agent {{ agentEventCount }} 条
      </div>
    </article>

    <article class="renew-metric">
      <div class="renew-metric__label">CPU</div>
      <div class="renew-metric__value">{{ cpuTotal.toFixed(1) }}%</div>
      <div class="renew-meter">
        <span :style="{ width: Math.min(100, cpuTotal) + '%' }" />
      </div>
      <div class="renew-metric__meta">{{ processCount }} 个进程</div>
    </article>

    <article class="renew-metric">
      <div class="renew-metric__label">内存</div>
      <div class="renew-metric__value">{{ memPercent.toFixed(1) }}%</div>
      <div class="renew-meter">
        <span :style="{ width: Math.min(100, memPercent) + '%' }" />
      </div>
      <div class="renew-metric__meta">
        {{ formatBytes(memUsed) }} / {{ formatBytes(memTotal) }}
      </div>
    </article>

    <article class="renew-metric" :class="`renew-metric--${alertTone}`">
      <div class="renew-metric__label">需要关注</div>
      <div class="renew-metric__value">{{ attentionCount }}</div>
      <div class="renew-metric__meta">
        已阻断 {{ blockedCount }} ·
        {{ attentionCount ? "建议查看" : "暂时正常" }}
      </div>
    </article>
  </section>
</template>
