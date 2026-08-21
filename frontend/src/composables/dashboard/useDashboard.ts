import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import axios from "axios";
import { message } from "ant-design-vue";

import {
  canPreviewEventPath,
  type FilePreviewResponse,
} from "../../types/filePreview";
import {
  minColumnWidths,
  pageSizeOptions,
  eventTypeLabelMap,
  selectableEventTypes,
  eventCategories,
  categoryTabs,
  syscallCatLabels,
  syscallCatColors,
  parseSyscallNr,
  syscallCategory,
  syscallDisplayName,
  builtinFilterRules,
  baseColumns,
  type AgentEvent,
  type BuiltinFilterRule,
} from "./dashboardConstants";
import {
  getTagColor,
  getCategoryColor,
  formatTraceSummary,
} from "./dashboardHelpers";
import { useDashboardStream } from "./useDashboardStream";

export type { AgentEvent, BuiltinFilterRule };

type DisplayedAgentEvent = AgentEvent & {
  mergeSignature?: string;
  lastReceivedAtMs?: number;
};

type BuiltinFilterState = Record<string, boolean>;

type ResizableColumnKey =
  | "time"
  | "tag"
  | "pid"
  | "comm"
  | "type"
  | "path"
  | "action";

export function useDashboard() {
  const events = ref<AgentEvent[]>([]);
  const isConnected = ref(false);
  const isPaused = ref(false);
  const showDetails = ref(false);
  const selectedEvent = ref<AgentEvent | null>(null);
  const showPreview = ref(false);
  const previewLoading = ref(false);
  const previewData = ref<FilePreviewResponse | null>(null);
  const selectedTags = ref<string[]>([]);
  const selectedTypes = ref<number[]>([]);
  const timeFilter = ref("");
  const pidFilter = ref("");
  const commandFilter = ref("");
  const pathFilter = ref("");
  const isDeduplicated = ref(false);
  const hideUnknown = ref(true);
  const activeHeaderFilter = ref<string | null>(null);
  const tags = ref<string[]>([]);
  const currentPage = ref(1);
  const pageSize = ref(20);
  const tableWrapperRef = ref<HTMLElement | null>(null);
  const tableContentWidth = ref(0);
  const router = useRouter();
  const route = useRoute();
  let resizeObserver: ResizeObserver | null = null;
  let cleanupColumnResize: (() => void) | null = null;
  let recentRowTimer: number | null = null;

  const maxEvents = ref(5000);
  const maxEventsOptions = ["2000", "5000", "10000", "20000", "50000"];

  const EVENT_MERGE_WINDOW_MS = 5000;

  // ── Stream composable ──

  const stream = useDashboardStream({
    events,
    isConnected,
    isPaused,
    maxEvents,
    getFilteredEvents: () => filteredEvents.value,
  });

  const {
    historyLoaded,
    startStream,
    stopStream,
    resetStreamState,
    clearEvents,
    exportEvents,
    exportEventsCSV,
    markRecentRowsRef,
  } = stream;

  // Wire up markRecentRows into the stream composable
  const markRecentRows = (keys: string[]) => {
    if (keys.length === 0) return;

    const nextKeys = new Set(recentRowKeys.value);
    for (const key of keys) {
      nextKeys.add(key);
    }
    recentRowKeys.value = nextKeys;

    if (recentRowTimer !== null) {
      window.clearTimeout(recentRowTimer);
    }
    recentRowTimer = window.setTimeout(() => {
      recentRowKeys.value = new Set();
      recentRowTimer = null;
    }, 320);
  };

  markRecentRowsRef.value = markRecentRows;

  // ── Stream direction / show-all-rows preferences ──

  const STREAM_DIRECTION_STORAGE_KEY = "dashboard.streamDirection";
  const SHOW_ALL_ROWS_STORAGE_KEY = "dashboard.showAllRows";
  const BUILTIN_FILTER_STATE_STORAGE_KEY = "dashboard.builtinFilters";
  const streamDirection = ref<"top" | "bottom">(getStoredStreamDirection());
  const showAllRows = ref(getStoredShowAllRows());

  function getStoredStreamDirection(): "top" | "bottom" {
    if (typeof window === "undefined") return "top";
    return window.localStorage.getItem(STREAM_DIRECTION_STORAGE_KEY) ===
      "bottom"
      ? "bottom"
      : "top";
  }

  function getStoredShowAllRows(): boolean {
    if (typeof window === "undefined") return false;
    return window.localStorage.getItem(SHOW_ALL_ROWS_STORAGE_KEY) === "true";
  }

  const columnWidths = ref<Record<ResizableColumnKey, number>>({
    time: 120,
    tag: 120,
    pid: 96,
    comm: 150,
    type: 140,
    path: 180,
    action: 80,
  });

  // Event category sets for tab filtering

  const activeTab = ref<string>("all");
  const netDirFilter = ref<string>("all");
  const syscallCatFilter = ref<string>("all");

  const syncTabFromRoute = () => {
    const tab = route.params.tab as string | undefined;
    const resolved =
      tab && categoryTabs.some((t) => t.key === tab) ? tab : "all";
    if (activeTab.value !== resolved) {
      activeTab.value = resolved;
    }
  };

  syncTabFromRoute();

  const onTabChange = (key: string) => {
    const resolved = categoryTabs.some((tab) => tab.key === key) ? key : "all";
    activeTab.value = resolved;
    void router.replace({
      name: "Dashboard",
      params: resolved === "all" ? {} : { tab: resolved },
      query: route.query,
      hash: route.hash,
    });
  };

  function createDefaultBuiltinFilterState(): BuiltinFilterState {
    return Object.fromEntries(
      builtinFilterRules.map((rule) => [rule.id, true]),
    ) as BuiltinFilterState;
  }

  function getStoredBuiltinFilterState(): BuiltinFilterState {
    const defaults = createDefaultBuiltinFilterState();
    if (typeof window === "undefined") return defaults;
    try {
      const raw = window.localStorage.getItem(BUILTIN_FILTER_STATE_STORAGE_KEY);
      if (!raw) return defaults;
      const parsed = JSON.parse(raw) as Record<string, unknown>;
      return builtinFilterRules.reduce(
        (state, rule) => {
          state[rule.id] =
            typeof parsed[rule.id] === "boolean"
              ? (parsed[rule.id] as boolean)
              : defaults[rule.id];
          return state;
        },
        { ...defaults } as BuiltinFilterState,
      );
    } catch {
      return defaults;
    }
  }

  const builtinFilterState = ref<BuiltinFilterState>(
    getStoredBuiltinFilterState(),
  );

  watch(
    builtinFilterState,
    (state) => {
      if (typeof window === "undefined") return;
      window.localStorage.setItem(
        BUILTIN_FILTER_STATE_STORAGE_KEY,
        JSON.stringify(state),
      );
    },
    { deep: true },
  );

  const activeBuiltinFilterRules = computed(() =>
    builtinFilterRules.filter(
      (rule) => builtinFilterState.value[rule.id] !== false,
    ),
  );

  const builtinFilterSummary = computed(() => {
    const labels = activeBuiltinFilterRules.value.map((rule) => rule.label);
    return labels.length > 0
      ? labels.join(" · ")
      : "No built-in filters enabled";
  });

  const shouldKeepBuiltinEvent = (event: AgentEvent) =>
    !activeBuiltinFilterRules.value.some((rule) => rule.test(event));

  const setBuiltinFiltersEnabled = (enabled: boolean) => {
    builtinFilterState.value = Object.fromEntries(
      builtinFilterRules.map((rule) => [rule.id, enabled]),
    ) as BuiltinFilterState;
  };

  const tagOptions = computed(() =>
    tags.value.map((tag) => ({
      label: tag,
      value: tag,
    })),
  );

  const eventTypeOptions = computed(() =>
    selectableEventTypes.map((eventType) => ({
      label: (eventTypeLabelMap[eventType] || String(eventType)).toUpperCase(),
      value: eventType,
    })),
  );

  const fetchTags = async () => {
    try {
      const res = await axios.get("/config/tags");
      tags.value = res.data;
    } catch (err) {
      console.error("Failed to fetch tags", err);
    }
  };

  // Built-in filters are always applied first and can be toggled per rule.
  const builtinFilteredEvents = computed(() =>
    events.value.filter((event) => shouldKeepBuiltinEvent(event)),
  );

  // Events with built-in + user filters only (used for stats bars)
  const tabFilteredEvents = computed(() => {
    let result = builtinFilteredEvents.value;
    if (selectedTags.value.length) {
      const activeTags = new Set(selectedTags.value);
      result = result.filter((e) => activeTags.has(e.tag));
    }
    if (selectedTypes.value.length) {
      const activeTypes = new Set(selectedTypes.value);
      result = result.filter(
        (e) => e.eventType !== undefined && activeTypes.has(e.eventType),
      );
    }
    const timeQuery = timeFilter.value.trim().toLowerCase();
    if (timeQuery)
      result = result.filter((e) => e.time.toLowerCase().includes(timeQuery));
    const pidQuery = pidFilter.value.trim();
    const commQuery = commandFilter.value.trim().toLowerCase();
    const pathQuery = pathFilter.value.trim().toLowerCase();
    if (pidQuery)
      result = result.filter((e) => String(e.pid).includes(pidQuery));
    if (commQuery)
      result = result.filter((e) => e.comm.toLowerCase().includes(commQuery));
    if (pathQuery)
      result = result.filter((e) => e.path.toLowerCase().includes(pathQuery));
    if (isDeduplicated.value) {
      const seen = new Set();
      result = result.filter((e) => {
        const id = `${e.type}-${e.comm}-${e.path}`;
        if (seen.has(id)) return false;
        seen.add(id);
        return true;
      });
    }
    // Category filtering: skip for "all" and "filter" tabs
    if (activeTab.value !== "all" && activeTab.value !== "filter") {
      const categorySet = eventCategories[activeTab.value];
      if (categorySet) {
        result = result.filter(
          (e) => e.eventType !== undefined && categorySet.has(e.eventType),
        );
      }
    }
    if (hideUnknown.value) result = result.filter((e) => e.tag !== "Unknown");
    return result;
  });

  // Full filtered events including sub-filters
  const filteredEvents = computed(() => {
    let result = tabFilteredEvents.value;
    // In "filter" tab or the tab-specific tab, apply sub-filters
    const isFilterTab = activeTab.value === "filter";
    if ((isFilterTab || activeTab.value === "network") && netDirFilter.value !== "all") {
      result = result.filter(
        (e) => (e.netDirection || "unknown") === netDirFilter.value,
      );
    }
    if ((isFilterTab || activeTab.value === "syscall") && syscallCatFilter.value !== "all") {
      result = result.filter(
        (e) =>
          syscallCategory(parseSyscallNr(e.extraInfo)) ===
          syscallCatFilter.value,
      );
    }
    return streamDirection.value === "bottom" ? [...result].reverse() : result;
  });

  const createEventMergeSignature = (event: AgentEvent) =>
    [
      event.eventType ?? "",
      event.type,
      event.tag,
      event.pid,
      event.ppid,
      event.uid,
      event.comm,
      event.path,
      event.netDirection ?? "",
      event.netEndpoint ?? "",
      event.netFamily ?? "",
      event.netBytes ?? "",
      event.retval ?? "",
      event.extraInfo ?? "",
      event.extraPath ?? "",
      event.bytes ?? "",
      event.mode ?? "",
      event.domain ?? "",
      event.sockType ?? "",
      event.protocol ?? "",
      event.uidArg ?? "",
      event.gidArg ?? "",
    ]
      .map((value) => String(value))
      .join("");

  const mergeEventsWithinWindow = (list: AgentEvent[]) => {
    const merged: DisplayedAgentEvent[] = [];
    const groupsBySignature = new Map<string, DisplayedAgentEvent>();

    for (const event of list) {
      const signature = createEventMergeSignature(event);
      const eventReceivedAtMs = event.receivedAtMs ?? 0;
      const currentGroup = groupsBySignature.get(signature);

      if (
        currentGroup &&
        currentGroup.lastReceivedAtMs !== undefined &&
        Math.abs(eventReceivedAtMs - currentGroup.lastReceivedAtMs) <=
          EVENT_MERGE_WINDOW_MS
      ) {
        currentGroup.occurrenceCount = (currentGroup.occurrenceCount ?? 1) + 1;
        currentGroup.lastReceivedAtMs = eventReceivedAtMs;
        if (event.durationNs !== undefined) {
          currentGroup.durationNs = event.durationNs;
        }
        continue;
      }

      const nextGroup: DisplayedAgentEvent = {
        ...event,
        occurrenceCount: 1,
        mergeSignature: signature,
        lastReceivedAtMs: eventReceivedAtMs,
      };
      merged.push(nextGroup);
      groupsBySignature.set(signature, nextGroup);
    }

    return merged.map(
      ({ mergeSignature, lastReceivedAtMs, ...event }) => event,
    );
  };

  const displayedEvents = computed(() =>
    mergeEventsWithinWindow(filteredEvents.value),
  );

  // Stats use tabFilteredEvents (pre-sub-filter) to avoid zeroing out
  const networkDirStats = computed(() => {
    const isNetworkOrFilter =
      activeTab.value === "network" || activeTab.value === "filter";
    const list = isNetworkOrFilter ? tabFilteredEvents.value : [];
    const dirs: Record<string, number> = {
      outgoing: 0,
      incoming: 0,
      listening: 0,
      unknown: 0,
    };
    for (const e of list) {
      const d = e.netDirection || "unknown";
      if (d in dirs) dirs[d]++;
      else dirs.unknown++;
    }
    return dirs as { outgoing: number; incoming: number; listening: number; unknown: number };
  });

  const syscallCatStats = computed(() => {
    const isSyscallOrFilter =
      activeTab.value === "syscall" || activeTab.value === "filter";
    const list = isSyscallOrFilter ? tabFilteredEvents.value : [];
    const cats: Record<string, number> = {};
    for (const e of list) {
      const cat = syscallCategory(parseSyscallNr(e.extraInfo));
      cats[cat] = (cats[cat] || 0) + 1;
    }
    return cats;
  });

  const tablePagination = computed(() => {
    if (showAllRows.value) {
      return false;
    }
    return {
      current: currentPage.value,
      pageSize: pageSize.value,
      total: displayedEvents.value.length,
      showSizeChanger: true,
      pageSizeOptions,
      showTotal: (total: number, range: [number, number]) =>
        `${range[0]}-${range[1]} / ${total}`,
    };
  });

  const handleTableChange = (pagination: {
    current?: number;
    pageSize?: number;
  }) => {
    if (showAllRows.value) return;
    currentPage.value = pagination.current ?? 1;
    pageSize.value = pagination.pageSize ?? pageSize.value;
  };

  const recentRowKeys = ref<Set<string>>(new Set());

  const getRowClassName = (record: AgentEvent, index: number) => {
    const classes = [index % 2 === 0 ? "excel-row-even" : "excel-row-odd"];
    if (recentRowKeys.value.has(record.key)) {
      classes.push(
        streamDirection.value === "bottom"
          ? "excel-row-enter-bottom"
          : "excel-row-enter-top",
      );
    }
    return classes.join(" ");
  };

  const hasHeaderFilter = (key: string | number | symbol) =>
    ["time", "tag", "pid", "comm", "type", "path"].includes(String(key));

  const isResizableColumn = (key: string | number | symbol) =>
    (
      ["time", "tag", "pid", "comm", "type", "path", "action"] as const
    ).includes(String(key) as ResizableColumnKey);

  const getFilterPopupContainer = (triggerNode: HTMLElement) =>
    (triggerNode.closest(".excel-filter-popover") as HTMLElement | null) ??
    document.body;

  const computePathWidth = () => {
    const fixedWidth = (
      ["time", "tag", "pid", "comm", "type", "action"] as const
    ).reduce((total, key) => total + columnWidths.value[key], 0);
    const availableWidth =
      tableContentWidth.value > 0 ? tableContentWidth.value : 0;
    const remainingWidth =
      availableWidth > 0
        ? Math.max(minColumnWidths.path, availableWidth - fixedWidth - 12)
        : columnWidths.value.path;
    return Math.max(
      minColumnWidths.path,
      columnWidths.value.path,
      remainingWidth,
    );
  };

  const tableColumns = computed(() =>
    baseColumns.map((column) => {
      if (column.key === "path") {
        return { ...column, width: computePathWidth() };
      }
      if (column.key in columnWidths.value) {
        return {
          ...column,
          width: columnWidths.value[column.key as ResizableColumnKey],
        };
      }
      return column;
    }),
  );

  const handleTableResize = (entries: ResizeObserverEntry[]) => {
    const entry = entries[0];
    if (!entry) return;
    tableContentWidth.value = entry.contentRect.width;
  };

  const startColumnResize = (key: string, event: MouseEvent) => {
    if (!isResizableColumn(key)) return;
    event.preventDefault();

    const resizeKey = key as ResizableColumnKey;

    const startX = event.clientX;
    const startWidth = columnWidths.value[resizeKey];
    const minWidth = minColumnWidths[resizeKey];

    const onMouseMove = (moveEvent: MouseEvent) => {
      const nextWidth = Math.max(
        minWidth,
        startWidth + moveEvent.clientX - startX,
      );
      columnWidths.value[resizeKey] = nextWidth;
    };

    const stopResize = () => {
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", stopResize);
      document.documentElement.classList.remove("excel-resizing");
      cleanupColumnResize = null;
    };

    cleanupColumnResize?.();
    document.documentElement.classList.add("excel-resizing");
    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", stopResize);
    cleanupColumnResize = stopResize;
  };

  const toggleHeaderFilter = (key: string | number | symbol) => {
    const filterKey = String(key);
    activeHeaderFilter.value =
      activeHeaderFilter.value === filterKey ? null : filterKey;
  };

  const closeHeaderFilter = () => {
    activeHeaderFilter.value = null;
  };

  const handleDocumentClick = (event: MouseEvent) => {
    if (!activeHeaderFilter.value) return;
    const target = event.target;
    if (!(target instanceof Element)) return;
    if (
      target.closest(".excel-filter-popover") ||
      target.closest(".excel-header-filter-trigger")
    ) {
      return;
    }
    closeHeaderFilter();
  };

  const isHeaderFilterActive = (key: string | number | symbol) => {
    switch (String(key)) {
      case "time":
        return Boolean(timeFilter.value.trim());
      case "tag":
        return selectedTags.value.length > 0;
      case "pid":
        return Boolean(pidFilter.value.trim());
      case "comm":
        return Boolean(commandFilter.value.trim());
      case "type":
        return selectedTypes.value.length > 0;
      case "path":
        return Boolean(pathFilter.value.trim());
      default:
        return false;
    }
  };

  const clearHeaderFilter = (key: string | number | symbol) => {
    switch (String(key)) {
      case "time":
        timeFilter.value = "";
        break;
      case "tag":
        selectedTags.value = [];
        break;
      case "pid":
        pidFilter.value = "";
        break;
      case "comm":
        commandFilter.value = "";
        break;
      case "type":
        selectedTypes.value = [];
        break;
      case "path":
        pathFilter.value = "";
        break;
    }
  };

  const clearAllFilters = () => {
    selectedTags.value = [];
    selectedTypes.value = [];
    timeFilter.value = "";
    pidFilter.value = "";
    commandFilter.value = "";
    pathFilter.value = "";
    isDeduplicated.value = false;
    hideUnknown.value = true;
    netDirFilter.value = "all";
    syscallCatFilter.value = "all";
    builtinFilterState.value = createDefaultBuiltinFilterState();
  };

  watch(
    [
      selectedTags,
      selectedTypes,
      timeFilter,
      pidFilter,
      commandFilter,
      pathFilter,
      isDeduplicated,
      hideUnknown,
    ],
    () => {
      if (showAllRows.value) return;
      currentPage.value = 1;
    },
  );

  watch(
    () => route.params.tab,
    () => {
      syncTabFromRoute();
      if (!showAllRows.value) {
        currentPage.value = 1;
      }
    },
  );

  watch(streamDirection, (direction) => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(STREAM_DIRECTION_STORAGE_KEY, direction);
    }
    if (showAllRows.value) return;
    currentPage.value =
      direction === "bottom"
        ? Math.max(1, Math.ceil(displayedEvents.value.length / pageSize.value))
        : 1;
  });

  watch(showAllRows, (enabled) => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(
        SHOW_ALL_ROWS_STORAGE_KEY,
        enabled ? "true" : "false",
      );
    }
    if (enabled) {
      currentPage.value = 1;
      return;
    }
    const maxPage = Math.max(
      1,
      Math.ceil(displayedEvents.value.length / pageSize.value),
    );
    currentPage.value = streamDirection.value === "bottom" ? maxPage : 1;
  });

  watch(
    [() => displayedEvents.value.length, pageSize, streamDirection],
    ([total]) => {
      if (showAllRows.value) return;
      const maxPage = Math.max(1, Math.ceil(total / pageSize.value));
      if (streamDirection.value === "bottom") {
        currentPage.value = maxPage;
        return;
      }
      if (currentPage.value > maxPage) {
        currentPage.value = maxPage;
      }
    },
  );

  const openDetails = (record: AgentEvent) => {
    selectedEvent.value = { ...record };
    showDetails.value = true;
  };

  const formatDetailValue = (value: number | string | undefined | null) => {
    if (value === undefined || value === null || value === "") {
      return "—";
    }
    return String(value);
  };

  const selectedTraceSummary = computed(() =>
    formatTraceSummary(selectedEvent.value),
  );

  const canInteractWithPath = (record: AgentEvent) =>
    canPreviewEventPath(record);

  const previewPath = async (path: string) => {
    previewLoading.value = true;
    try {
      const res = await axios.get(
        `/system/file-preview?path=${encodeURIComponent(path)}`,
      );
      previewData.value = res.data as FilePreviewResponse;
      showPreview.value = true;
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { error?: string } } };
      message.error(axiosErr?.response?.data?.error || "Failed to preview file");
    } finally {
      previewLoading.value = false;
    }
  };

  const previewRecordPath = (record: AgentEvent) => {
    if (!canInteractWithPath(record)) return;
    void previewPath(record.path);
  };

  const openInExplorer = (record: AgentEvent) => {
    if (!canInteractWithPath(record)) return;
    void router.push({
      path: "/explorer",
      query: {
        path: record.path,
        preview: "1",
      },
    });
  };

  const resetDashboardRuntimeState = () => {
    // Reset UI/filter state
    events.value = [];
    isConnected.value = false;
    isPaused.value = false;
    showDetails.value = false;
    selectedEvent.value = null;
    showPreview.value = false;
    previewLoading.value = false;
    previewData.value = null;
    selectedTags.value = [];
    selectedTypes.value = [];
    timeFilter.value = "";
    pidFilter.value = "";
    commandFilter.value = "";
    pathFilter.value = "";
    isDeduplicated.value = false;
    hideUnknown.value = true;
    activeHeaderFilter.value = null;
    tags.value = [];
    currentPage.value = 1;
    pageSize.value = 20;
    maxEvents.value = 5000;
    netDirFilter.value = "all";
    syscallCatFilter.value = "all";
    tableContentWidth.value = 0;
    columnWidths.value = {
      time: 120,
      tag: 120,
      pid: 96,
      comm: 150,
      type: 140,
      path: 180,
      action: 80,
    };
    recentRowKeys.value = new Set();
    if (recentRowTimer !== null) {
      window.clearTimeout(recentRowTimer);
      recentRowTimer = null;
    }
    // Reset stream state
    resetStreamState();
  };

  onMounted(() => {
    resetDashboardRuntimeState();
    streamDirection.value = getStoredStreamDirection();
    showAllRows.value = getStoredShowAllRows();
    builtinFilterState.value = getStoredBuiltinFilterState();
    startStream();
    fetchTags();
    document.addEventListener("click", handleDocumentClick);
    if (tableWrapperRef.value && typeof ResizeObserver !== "undefined") {
      resizeObserver = new ResizeObserver(handleTableResize);
      resizeObserver.observe(tableWrapperRef.value);
    }
  });

  onUnmounted(() => {
    document.removeEventListener("click", handleDocumentClick);
    resetDashboardRuntimeState();
    resizeObserver?.disconnect();
    resizeObserver = null;
    cleanupColumnResize?.();
    cleanupColumnResize = null;
  });

  return {
    events,
    isConnected,
    isPaused,
    showDetails,
    selectedEvent,
    showPreview,
    previewLoading,
    previewData,
    selectedTags,
    selectedTypes,
    timeFilter,
    pidFilter,
    commandFilter,
    pathFilter,
    isDeduplicated,
    hideUnknown,
    activeHeaderFilter,
    tags,
    currentPage,
    pageSize,
    tableWrapperRef,
    streamDirection,
    showAllRows,
    builtinFilterRules,
    builtinFilterState,
    builtinFilterSummary,
    setBuiltinFiltersEnabled,
    maxEvents,
    maxEventsOptions,
    activeTab,
    netDirFilter,
    syscallCatFilter,
    categoryTabs,
    networkDirStats,
    syscallCatStats,
    syscallCatLabels,
    syscallCatColors,
    tableColumns,
    tablePagination,
    pageSizeOptions,
    eventTypeOptions,
    tagOptions,
    displayedEvents,
    openDetails,
    formatDetailValue,
    selectedTraceSummary,
    canInteractWithPath,
    previewRecordPath,
    openInExplorer,
    getTagColor,
    getCategoryColor,
    getRowClassName,
    onTabChange,
    handleTableChange,
    toggleHeaderFilter,
    clearHeaderFilter,
    isHeaderFilterActive,
    hasHeaderFilter,
    isResizableColumn,
    startColumnResize,
    getFilterPopupContainer,
    clearEvents,
    exportEvents,
    exportEventsCSV,
    syscallDisplayName,
    clearAllFilters,
  };
}
