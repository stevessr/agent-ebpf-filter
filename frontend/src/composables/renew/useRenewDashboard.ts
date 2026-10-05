import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import { useMonitorData } from "../monitor/useMonitorData";
import {
  describeEvent,
  eventLabel,
  eventTime,
  eventTone,
} from "./eventPresentation";
import {
  HARNESS_LABELS,
  attributeProcesses,
  attributeEvents,
  eventHarness,
  type HarnessFilter,
} from "./harness";
import { useRenewEventFeed } from "./useRenewEventFeed";
import { useRenewEventSummary } from "./useRenewEventSummary";
import { useRenewMonitoringControls } from "./useRenewMonitoringControls";

export function useRenewDashboard() {
  const router = useRouter();
  const route = useRoute();
  const search = ref("");
  const onlyAgents = ref(true);
  const readHarness = (): HarnessFilter => {
    const value = String(route.query.harness || "all");
    return Object.hasOwn(HARNESS_LABELS, value)
      ? (value as HarnessFilter)
      : "all";
  };
  const harness = ref<HarnessFilter>(readHarness());
  watch(
    () => route.query.harness,
    () => {
      harness.value = readHarness();
    },
  );
  watch(harness, (value) => {
    if (value === readHarness()) return;
    void router.replace({
      query: { ...route.query, harness: value === "all" ? undefined : value },
    });
  });
  const isPaused = ref(false);

  const monitoring = useRenewMonitoringControls();
  const feed = useRenewEventFeed(isPaused);

  const {
    processes: allProcesses,
    systemStats,
    trackedProcesses: allTrackedProcesses,
    formatBytesWithUnit,
    fetchTrackedComms,
    setup,
    teardown,
  } = useMonitorData(monitoring.statsIntervalMs);

  const processes = computed(() =>
    attributeProcesses(allProcesses.value).filter(
      (p) => harness.value === "all" || p.harness === harness.value,
    ),
  );
  const trackedProcesses = computed(() =>
    allTrackedProcesses.value.filter((p) =>
      processes.value.some((q) => q.pid === p.pid),
    ),
  );
  const events = computed(() =>
    attributeEvents(feed.events.value).filter(
      (event) =>
        harness.value === "all" || eventHarness(event) === harness.value,
    ),
  );

  const eventSummary = useRenewEventSummary(events, search, onlyAgents);

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
    events,
    harness,
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
