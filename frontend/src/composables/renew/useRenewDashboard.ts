import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";

import { useMonitorData } from "../monitor/useMonitorData";
import {
  describeEvent,
  eventLabel,
  eventTime,
  eventTone,
} from "./eventPresentation";
import { useRenewEventFeed } from "./useRenewEventFeed";
import { useRenewEventSummary } from "./useRenewEventSummary";
import { useRenewMonitoringControls } from "./useRenewMonitoringControls";

export function useRenewDashboard() {
  const router = useRouter();
  const search = ref("");
  const onlyAgents = ref(true);
  const isPaused = ref(false);

  const monitoring = useRenewMonitoringControls();
  const feed = useRenewEventFeed(isPaused);

  const {
    processes,
    systemStats,
    trackedProcesses,
    formatBytesWithUnit,
    fetchTrackedComms,
    setup,
    teardown,
  } = useMonitorData(monitoring.statsIntervalMs);

  const eventSummary = useRenewEventSummary(
    feed.events,
    search,
    onlyAgents,
  );

  const topProcesses = computed(() =>
    [...processes.value].sort((a, b) => b.cpu - a.cpu).slice(0, 5),
  );

  const connectionLabel = computed(() =>
    feed.isConnected.value ? "实时采集中" : "等待采集端",
  );

  const formatRate = (bytes: number) => `${formatBytesWithUnit(bytes)}/s`;

  const go = (name: string, params?: Record<string, string>) => {
    void router.push({ name, params });
  };

  const refreshMonitoring = async () => {
    await Promise.allSettled([
      monitoring.fetchState(),
      monitoring.fetchCollectorHealth(),
    ]);
  };

  onMounted(() => {
    setup();
    feed.start();
    void fetchTrackedComms();
    void refreshMonitoring();
  });

  onUnmounted(() => {
    feed.stop();
    teardown();
  });

  return {
    events: feed.events,
    isConnected: feed.isConnected,
    historyLoading: feed.historyLoading,
    hasOlder: feed.hasOlder,
    isPaused,
    processes,
    systemStats,
    trackedProcesses,
    search,
    onlyAgents,
    topProcesses,
    connectionLabel,
    formatBytesWithUnit,
    formatRate,
    describeEvent,
    eventTime,
    eventTone,
    eventLabel,
    go,
    detailLoading: feed.detailLoading,
    selectedEventDetail: feed.selectedEventDetail,
    selectedEventID: feed.selectedEventID,
    loadEventDetail: feed.loadEventDetail,
    loadOlder: feed.loadOlder,
    closeEventDetail: feed.closeEventDetail,
    refreshMonitoring,
    monitoring,
    ...eventSummary,
  };
}
