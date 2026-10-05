<script setup lang="ts">
import { ref } from "vue";

import RenewActivityPanel from "../../components/renew/RenewActivityPanel.vue";
import RenewAgentSessions from "../../components/renew/RenewAgentSessions.vue";
import RenewAttentionPanel from "../../components/renew/RenewAttentionPanel.vue";
import RenewEventDetailDrawer from "../../components/renew/RenewEventDetailDrawer.vue";
import RenewHeader from "../../components/renew/RenewHeader.vue";
import RenewMetrics from "../../components/renew/RenewMetrics.vue";
import RenewMonitoringPanel from "../../components/renew/RenewMonitoringPanel.vue";
import RenewSidebar from "../../components/renew/RenewSidebar.vue";
import RenewSystemSnapshot from "../../components/renew/RenewSystemSnapshot.vue";
import RenewToolbar from "../../components/renew/RenewToolbar.vue";
import type { RenewMonitoringProfileKey } from "../../composables/renew/monitoringPresets";
import type { RenewRuntimeToggleKey } from "../../composables/renew/useRenewMonitoringControls";
import { useRenewDashboard } from "../../composables/renew/useRenewDashboard";
import "./renew.css";

const activeSection = ref<"overview" | "monitoring">("overview");

const {
  events,
  isConnected,
  historyLoading,
  hasOlder,
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
  detailLoading,
  selectedEventDetail,
  selectedEventID,
  loadEventDetail,
  loadOlder,
  closeEventDetail,
  refreshMonitoring,
  monitoring,
} = useRenewDashboard();

const {
  modules,
  profiles,
  enabledModuleKeys,
  activeProfileKey,
  statsIntervalMs,
  collectorHealth,
  applying,
  loading,
  error,
  overhead,
  runtimeEnabled,
  applyProfile,
  setModuleEnabled,
  setRuntimeEnabled,
  setStatsInterval,
} = monitoring;

const handleApplyProfile = (key: RenewMonitoringProfileKey) => {
  void applyProfile(key).catch(() => {});
};

const handleToggleModule = (key: string, enabled: boolean) => {
  void setModuleEnabled(key, enabled).catch(() => {});
};

const handleToggleRuntime = (
  key: RenewRuntimeToggleKey,
  enabled: boolean,
) => {
  void setRuntimeEnabled(key, enabled).catch(() => {});
};
</script>

<template>
  <div class="renew-shell">
    <RenewSidebar
      :active-section="activeSection"
      @section="activeSection = $event"
      @navigate="go"
    />

    <main class="renew-main">
      <template v-if="activeSection === 'overview'">
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
            :has-older="hasOlder"
            :history-loading="historyLoading"
            @load-older="loadOlder"
            @open-event="loadEventDetail"
            @open-events="go('Dashboard')"
          />

          <RenewAttentionPanel
            :events="attentionEvents"
            :describe-event="describeEvent"
            :event-time="eventTime"
            :event-label="eventLabel"
            @open-event="loadEventDetail"
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
      </template>

      <RenewMonitoringPanel
        v-else
        :modules="modules"
        :profiles="profiles"
        :enabled-module-keys="enabledModuleKeys"
        :active-profile-key="activeProfileKey"
        :stats-interval-ms="statsIntervalMs"
        :collector-health="collectorHealth"
        :applying="applying"
        :loading="loading"
        :error="error"
        :overhead="overhead"
        :runtime-enabled="runtimeEnabled"
        @apply-profile="handleApplyProfile"
        @toggle-module="handleToggleModule"
        @toggle-runtime="handleToggleRuntime"
        @update-stats-interval="setStatsInterval"
        @refresh="refreshMonitoring"
      />
    </main>

    <RenewEventDetailDrawer
      :open="Boolean(selectedEventID)"
      :loading="detailLoading"
      :event-id="selectedEventID"
      :detail="selectedEventDetail"
      @close="closeEventDetail"
    />
  </div>
</template>