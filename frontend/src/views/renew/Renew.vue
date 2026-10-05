<script setup lang="ts">
import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { HARNESS_LABELS } from "../../composables/renew/harness";
import RenewExplorer from "../../components/renew/RenewExplorer.vue";
import RenewRules from "../../components/renew/RenewRules.vue";

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

type Section =
  "overview" | "monitoring" | "events" | "network" | "processes" | "rules";
const route = useRoute();
const router = useRouter();
const activeSection = computed<Section>(() => {
  const section = String(route.params.section || "overview");
  return [
    "overview",
    "monitoring",
    "events",
    "network",
    "processes",
    "rules",
  ].includes(section)
    ? (section as Section)
    : "overview";
});
const openSection = (section: Section) =>
  void router.push({
    name: "Renew",
    params: { section: section === "overview" ? undefined : section },
    query: route.query,
  });

const {
  events,
  harness,
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
  ready,
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

const handleToggleRuntime = (key: RenewRuntimeToggleKey, enabled: boolean) => {
  void setRuntimeEnabled(key, enabled).catch(() => {});
};
</script>

<template>
  <div class="renew-shell">
    <RenewSidebar
      :active-section="activeSection"
      @section="openSection"
      @navigate="go"
    />

    <main class="renew-main">
      <div class="renew-harness-bar">
        <label
          >Harness
          <select v-model="harness" aria-label="Harness 筛选">
            <option value="all">全部 harness</option>
            <option
              v-for="(label, key) in HARNESS_LABELS"
              :key="key"
              :value="key"
            >
              {{ label }}
            </option>
          </select></label
        >
        <span>按工具身份区分，不按模型供应商猜测；系统资源为全机指标。</span>
      </div>
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
            @open-events="openSection('events')"
          />

          <RenewAttentionPanel
            :events="attentionEvents"
            :describe-event="describeEvent"
            :event-time="eventTime"
            :event-label="eventLabel"
            @open-event="loadEventDetail"
            @open-events="openSection('events')"
          />

          <RenewAgentSessions :sessions="activeAgents" />

          <RenewSystemSnapshot
            :processes="topProcesses"
            :destinations="topDestinations"
            :tracked-process-count="trackedProcesses.length"
            @open-processes="openSection('processes')"
            @open-network="openSection('network')"
          />
        </div>
      </template>

      <RenewExplorer
        v-else-if="
          activeSection === 'events' ||
          activeSection === 'network' ||
          activeSection === 'processes'
        "
        :section="activeSection"
        :events="events"
        :processes="processes"
        :connected="isConnected"
        :has-older="hasOlder"
        :history-loading="historyLoading"
        @open-event="loadEventDetail"
        @load-older="loadOlder"
      />
      <RenewRules v-else-if="activeSection === 'rules'" />
      <template v-else>
        <p class="renew-scope-note">
          共享配置：监控开关对后端所有 harness
          生效，上方筛选只影响事件与进程展示。
        </p>
        <RenewMonitoringPanel
          :modules="modules"
          :profiles="profiles"
          :enabled-module-keys="enabledModuleKeys"
          :active-profile-key="activeProfileKey"
          :stats-interval-ms="statsIntervalMs"
          :collector-health="collectorHealth"
          :applying="applying"
          :loading="loading"
          :ready="ready"
          :error="error"
          :overhead="overhead"
          :runtime-enabled="runtimeEnabled"
          @apply-profile="handleApplyProfile"
          @toggle-module="handleToggleModule"
          @toggle-runtime="handleToggleRuntime"
          @update-stats-interval="setStatsInterval"
          @refresh="refreshMonitoring"
        />
      </template>
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
