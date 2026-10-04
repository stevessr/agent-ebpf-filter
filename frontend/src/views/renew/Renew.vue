<script setup lang="ts">
import RenewActivityPanel from "../../components/renew/RenewActivityPanel.vue";
import RenewAgentSessions from "../../components/renew/RenewAgentSessions.vue";
import RenewAttentionPanel from "../../components/renew/RenewAttentionPanel.vue";
import RenewHeader from "../../components/renew/RenewHeader.vue";
import RenewMetrics from "../../components/renew/RenewMetrics.vue";
import RenewSidebar from "../../components/renew/RenewSidebar.vue";
import RenewSystemSnapshot from "../../components/renew/RenewSystemSnapshot.vue";
import RenewToolbar from "../../components/renew/RenewToolbar.vue";
import { useRenewDashboard } from "../../composables/renew/useRenewDashboard";
import "./renew.css";

const {
  events,
  isConnected,
  isPaused,
  processes,
  systemStats,
  trackedProcesses,
  search,
  onlyAgents,
  recentEvents,
  attentionEvents,
  attentionCount,
  blockedCount,
  agentEventCount,
  activeAgents,
  topProcesses,
  topDestinations,
  connectionLabel,
  alertTone,
  formatBytesWithUnit,
  formatRate,
  describeEvent,
  eventTime,
  eventTone,
  eventLabel,
  go,
} = useRenewDashboard();
</script>

<template>
  <div class="renew-shell">
    <RenewSidebar @navigate="go" />

    <main class="renew-main">
      <RenewHeader
        :is-connected="isConnected"
        :is-paused="isPaused"
        :connection-label="connectionLabel"
        @toggle-pause="isPaused = !isPaused"
      />

      <RenewMetrics
        :is-connected="isConnected"
        :event-count="events.length"
        :agent-event-count="agentEventCount"
        :cpu-total="systemStats.cpuTotal"
        :process-count="processes.length"
        :mem-percent="systemStats.memPercent"
        :mem-used="systemStats.memUsed"
        :mem-total="systemStats.memTotal"
        :attention-count="attentionCount"
        :blocked-count="blockedCount"
        :alert-tone="alertTone"
        :format-bytes="formatBytesWithUnit"
      />

      <RenewToolbar
        :search="search"
        :only-agents="onlyAgents"
        :net-recv="systemStats.totalNetRecv"
        :net-sent="systemStats.totalNetSent"
        :format-rate="formatRate"
        @update:search="search = $event"
        @update:only-agents="onlyAgents = $event"
      />

      <div class="renew-grid">
        <RenewActivityPanel
          :events="recentEvents"
          :describe-event="describeEvent"
          :event-time="eventTime"
          :event-tone="eventTone"
          :event-label="eventLabel"
          @open-events="go('Dashboard')"
        />

        <RenewAttentionPanel
          :events="attentionEvents"
          :describe-event="describeEvent"
          :event-time="eventTime"
          :event-label="eventLabel"
          @open-events="go('Dashboard')"
        />

        <RenewAgentSessions :sessions="activeAgents" />

        <RenewSystemSnapshot
          :processes="topProcesses"
          :destinations="topDestinations"
          :tracked-process-count="trackedProcesses.length"
          @open-processes="go('Monitor', { tab: 'processes' })"
          @open-network="go('NetworkFlow', { tab: 'overview' })"
        />
      </div>
    </main>
  </div>
</template>
