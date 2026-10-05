import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";

import { useDashboard } from "../dashboard/useDashboard";
import { useMonitorData } from "../monitor/useMonitorData";
import {
  describeEvent,
  eventLabel,
  eventTime,
  eventTone,
} from "./eventPresentation";
import { useRenewEventSummary } from "./useRenewEventSummary";

export function useRenewDashboard() {
  const router = useRouter();
  const search = ref("");
  const onlyAgents = ref(true);

  const { events, isConnected, isPaused } = useDashboard();
  const {
    processes,
    systemStats,
    trackedProcesses,
    formatBytesWithUnit,
    fetchTrackedComms,
    setup,
    teardown,
  } = useMonitorData();

  const eventSummary = useRenewEventSummary(events, search, onlyAgents);

  const topProcesses = computed(() =>
    [...processes.value].sort((a, b) => b.cpu - a.cpu).slice(0, 5),
  );

  const connectionLabel = computed(() =>
    isConnected.value ? "实时采集中" : "等待采集端",
  );

  const formatRate = (bytes: number) => `${formatBytesWithUnit(bytes)}/s`;

  const go = (name: string, params?: Record<string, string>) => {
    void router.push({ name, params });
  };

  onMounted(() => {
    setup();
    void fetchTrackedComms();
  });

  onUnmounted(() => {
    teardown();
  });

  return {
    events,
    isConnected,
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
    ...eventSummary,
  };
}
