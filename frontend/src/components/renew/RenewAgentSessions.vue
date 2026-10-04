<script setup lang="ts">
import type { RenewAgentSession } from "../../composables/renew/types";

defineProps<{
  sessions: RenewAgentSession[];
}>();
</script>

<template>
  <section class="renew-panel">
    <div class="renew-panel__header">
      <div>
        <h2>Agent 会话</h2>
        <p>按 run / conversation / 根进程归并。</p>
      </div>
    </div>

    <div v-if="sessions.length" class="renew-session-list">
      <div v-for="agent in sessions" :key="agent.key" class="renew-session">
        <div class="renew-session__icon">
          {{ agent.label.slice(0, 1).toUpperCase() }}
        </div>
        <div class="renew-session__body">
          <div>
            <strong>{{ agent.label }}</strong>
            <span>{{ agent.events }} 个动作</span>
          </div>
          <p>{{ agent.lastAction }}</p>
        </div>
        <span v-if="agent.alerts" class="renew-session__alert">
          {{ agent.alerts }}
        </span>
      </div>
    </div>

    <div v-else class="renew-empty renew-empty--compact">
      <span>暂无可归并的 Agent 会话</span>
    </div>
  </section>
</template>
