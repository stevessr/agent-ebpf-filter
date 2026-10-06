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

  // Deep-link sync between the selected event detail and ?event=, mirroring the
  // harness sync above: read*() reads route.query.X, a watch on route.query.X
  // writes into state, and openEvent/closeEventDetail write the query back.
  const readEventID = (): string => String(route.query.event ?? "").trim();
  const syncEventFromQuery = () => {
    const value = readEventID();
    if (!value) {
      // 空白/非法的 ?event= 只清空抽屉，不抛错
      if (feed.selectedEventID.value) feed.closeEventDetail();
      return;
    }
    // 同一个事件已在 URL 与状态中：不重复加载
    if (feed.selectedEventID.value === value) return;
    void feed.loadEventDetail(value);
  };
  // 冷加载/分享链接/F5：挂载时若 ?event= 存在则直接载入该事件详情
  syncEventFromQuery();
  watch(() => route.query.event, syncEventFromQuery);

  const openEvent = (eventId: string) => {
    const normalized = eventId.trim();
    if (!normalized) return;
    // 已选中且 URL 一致时不重复拉取
    if (
      feed.selectedEventID.value === normalized &&
      readEventID() === normalized
    ) {
      return;
    }
    void feed.loadEventDetail(normalized);
    void router.replace({
      query: { ...route.query, event: normalized },
    });
  };

  const closeEventDetail = () => {
    feed.closeEventDetail();
    void router.replace({
      query: { ...route.query, event: undefined },
    });
  };

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
    openEvent,
    loadOlder: feed.loadOlder,
    closeEventDetail,
    refreshMonitoring,
    monitoring,
    ...eventSummary,
  };
}
