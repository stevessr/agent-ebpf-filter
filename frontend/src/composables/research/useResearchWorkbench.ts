import { computed, onUnmounted, ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import type {
  ResearchArtifactRef,
  ResearchCreateSessionRequest,
  ResearchEventsResponse,
  ResearchEvent,
  ResearchResults,
  ResearchSession,
  ResearchSessionListResponse,
  ResearchSourceFilter,
  ResearchTask,
  ResearchTaskRequest,
  ResearchTrainingDataset,
  ResearchTrainingImportResponse,
  ResearchTrainingLabelPolicy,
  ResearchTimeRange,
} from "../../types/config";

type ResearchTaskAction = ResearchTaskRequest["action"];
type ExportFormat = "jsonl" | "csv" | "bundle" | "json";
type TrainingExportFormat = "jsonl" | "csv";

const terminalTaskStatuses = new Set(["succeeded", "failed", "canceled"]);

const splitList = (input: string) =>
  input
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);

const makeDefaultSessionName = () => {
  const now = new Date();
  const stamp = now.toISOString().replace(/\.\d{3}Z$/, "Z");
  return `Research ${stamp}`;
};

const parseFilename = (contentDisposition?: string) => {
  if (!contentDisposition) return "";
  const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match?.[1]) return decodeURIComponent(utf8Match[1]);
  const quotedMatch = contentDisposition.match(/filename="?([^";]+)"?/i);
  return quotedMatch?.[1] || "";
};

const safeExportName = (session: ResearchSession | null, format: ExportFormat) => {
  const base = (session?.name || session?.id || "research-session")
    .trim()
    .replace(/[^a-z0-9._-]+/gi, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  const suffix = format === "bundle" ? "zip" : format;
  return `${base || "research-session"}.${suffix}`;
};

const safeTrainingExportName = (
  session: ResearchSession | null,
  format: TrainingExportFormat,
) => {
  const base = (session?.name || session?.id || "research-session")
    .trim()
    .replace(/[^a-z0-9._-]+/gi, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return `${base || "research-session"}-training.${format}`;
};

const downloadBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
};

export function useResearchWorkbench() {
  const sessions = ref<ResearchSession[]>([]);
  const selectedSessionId = ref("");
  const events = ref<ResearchEvent[]>([]);
  const results = ref<ResearchResults | null>(null);
  const activeTask = ref<ResearchTask | null>(null);
  const loadingSessions = ref(false);
  const creatingSession = ref(false);
  const deletingSession = ref(false);
  const loadingEvents = ref(false);
  const loadingResults = ref(false);
  const submittingTask = ref(false);
  const exportingArtifact = ref(false);
  const loadingResearchTraining = ref(false);
  const importingResearchTraining = ref(false);
  const exportingResearchTraining = ref(false);
  const eventSearch = ref("");
  const eventLimit = ref(100);
  const eventOffset = ref(0);
  const eventsTotal = ref(0);
  const compareWindowHours = ref(1);
  const researchTrainingLabelPolicy =
    ref<ResearchTrainingLabelPolicy>("decision");
  const researchTrainingImportLimit = ref(0);
  const researchTrainingDataset = ref<ResearchTrainingDataset | null>(null);
  const researchTrainingImportResult =
    ref<ResearchTrainingImportResponse | null>(null);

  const createForm = ref({
    name: makeDefaultSessionName(),
    description: "",
    tags: "research,training",
    query: "",
    sources: "",
    eventTypes: "",
    comms: "",
    limit: 5000,
    sinceMinutes: 0,
  });

  let taskPollTimer: number | null = null;

  const selectedSession = computed(
    () =>
      sessions.value.find((session) => session.id === selectedSessionId.value) ||
      null,
  );

  const artifactRefs = computed<ResearchArtifactRef[]>(() =>
    Object.values(selectedSession.value?.artifactRefs || {}),
  );

  const hasEvents = computed(() => events.value.length > 0);
  const canPageBack = computed(() => eventOffset.value > 0);
  const canPageForward = computed(
    () => eventOffset.value + eventLimit.value < eventsTotal.value,
  );
  const researchTrainingPreviewSamples = computed(() =>
    (researchTrainingDataset.value?.samples || []).slice(0, 20),
  );

  const buildFilterFromForm = (): ResearchSourceFilter => {
    const filter: ResearchSourceFilter = {};
    const sources = splitList(createForm.value.sources);
    const eventTypes = splitList(createForm.value.eventTypes);
    const comms = splitList(createForm.value.comms);
    if (sources.length) filter.sources = sources;
    if (eventTypes.length) filter.eventTypes = eventTypes;
    if (comms.length) filter.comms = comms;
    if (createForm.value.query.trim()) filter.query = createForm.value.query.trim();
    if (createForm.value.limit > 0) filter.limit = createForm.value.limit;
    return filter;
  };

  const buildTimeRangeFromForm = (): ResearchTimeRange => {
    const minutes = Math.max(0, createForm.value.sinceMinutes || 0);
    if (minutes <= 0) return {};
    const since = Date.now() - minutes * 60_000;
    return { since };
  };

  const refreshSessions = async (silent = false) => {
    loadingSessions.value = true;
    try {
      const res = await axios.get<ResearchSessionListResponse>(
        "/research/sessions",
      );
      sessions.value = res.data.sessions || [];
      if (
        selectedSessionId.value &&
        !sessions.value.some((session) => session.id === selectedSessionId.value)
      ) {
        selectedSessionId.value = "";
      }
      if (!selectedSessionId.value && sessions.value.length > 0) {
        selectedSessionId.value = sessions.value[0].id;
      }
      if (!silent) message.success(`已刷新 ${sessions.value.length} 个研究会话`);
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error || "刷新研究会话失败");
    } finally {
      loadingSessions.value = false;
    }
  };

  const createSession = async () => {
    const name = createForm.value.name.trim() || makeDefaultSessionName();
    creatingSession.value = true;
    try {
      const payload: ResearchCreateSessionRequest = {
        name,
        description: createForm.value.description.trim(),
        tags: splitList(createForm.value.tags),
        sourceFilter: buildFilterFromForm(),
        timeRange: buildTimeRangeFromForm(),
      };
      const res = await axios.post<ResearchSession>("/research/sessions", payload);
      sessions.value = [res.data, ...sessions.value.filter((s) => s.id !== res.data.id)];
      selectedSessionId.value = res.data.id;
      createForm.value.name = makeDefaultSessionName();
      message.success(`已创建研究会话：${res.data.name}`);
    } catch (e: any) {
      message.error(e.response?.data?.error || "创建研究会话失败");
    } finally {
      creatingSession.value = false;
    }
  };

  const ensureSelectedSession = () => {
    if (selectedSessionId.value) return true;
    message.warning("请先选择或创建 Research Session");
    return false;
  };

  const selectSession = async (id: string) => {
    selectedSessionId.value = id;
    eventOffset.value = 0;
    events.value = [];
    results.value = null;
    researchTrainingDataset.value = null;
    researchTrainingImportResult.value = null;
    await Promise.all([fetchEvents(true), fetchResults(true)]);
  };

  const deleteSelectedSession = async () => {
    if (!ensureSelectedSession()) return;
    deletingSession.value = true;
    try {
      await axios.delete(`/research/sessions/${encodeURIComponent(selectedSessionId.value)}`);
      message.success("研究会话已删除");
      const removed = selectedSessionId.value;
      sessions.value = sessions.value.filter((session) => session.id !== removed);
      selectedSessionId.value = sessions.value[0]?.id || "";
      events.value = [];
      results.value = null;
      researchTrainingDataset.value = null;
      researchTrainingImportResult.value = null;
      eventsTotal.value = 0;
    } catch (e: any) {
      message.error(e.response?.data?.error || "删除研究会话失败");
    } finally {
      deletingSession.value = false;
    }
  };

  const stopTaskPolling = () => {
    if (taskPollTimer !== null) {
      window.clearTimeout(taskPollTimer);
      taskPollTimer = null;
    }
  };

  const refreshTask = async (taskId: string) => {
    try {
      const res = await axios.get<ResearchTask>(
        `/research/tasks/${encodeURIComponent(taskId)}`,
      );
      activeTask.value = res.data;
      if (terminalTaskStatuses.has(res.data.status)) {
        stopTaskPolling();
        await refreshSessions(true);
        if (selectedSessionId.value) {
          await Promise.all([fetchEvents(true), fetchResults(true)]);
        }
        if (res.data.status === "succeeded") {
          message.success(`任务完成：${res.data.action}`);
        } else if (res.data.status === "failed") {
          message.error(res.data.error || `任务失败：${res.data.action}`);
        }
        return;
      }
      taskPollTimer = window.setTimeout(() => {
        void refreshTask(taskId);
      }, 1200);
    } catch (e: any) {
      stopTaskPolling();
      message.error(e.response?.data?.error || "刷新研究任务失败");
    }
  };

  const submitTask = async (
    action: ResearchTaskAction,
    overrides: Partial<ResearchTaskRequest> = {},
  ) => {
    if (!ensureSelectedSession()) return;
    submittingTask.value = true;
    stopTaskPolling();
    try {
      const payload: ResearchTaskRequest = {
        action,
        limit: createForm.value.limit,
        ...overrides,
      };
      const res = await axios.post<ResearchTask>(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/tasks`,
        payload,
      );
      activeTask.value = res.data;
      message.success(`已提交研究任务：${action}`);
      void refreshTask(res.data.taskId);
    } catch (e: any) {
      message.error(e.response?.data?.error || "提交研究任务失败");
    } finally {
      submittingTask.value = false;
    }
  };

  const buildSession = async () => {
    await submitTask("build_session", {
      sourceFilter: buildFilterFromForm(),
      timeRange: buildTimeRangeFromForm(),
    });
  };

  const scanRecent = async () => {
    await submitTask("scan_recent", {
      sourceFilter: buildFilterFromForm(),
      timeRange: buildTimeRangeFromForm(),
    });
  };

  const exportBundle = async () => {
    await submitTask("export_bundle", {
      formats: ["jsonl", "csv", "bundle"],
    });
  };

  const resetSession = async () => {
    await submitTask("reset_session");
  };

  const compareRecentWindows = async () => {
    const hours = Math.max(1, compareWindowHours.value || 1);
    const windowMs = hours * 60 * 60 * 1000;
    const now = Date.now();
    await submitTask("compare_windows", {
      leftWindow: { since: now - 2 * windowMs, until: now - windowMs },
      rightWindow: { since: now - windowMs, until: now },
    });
  };

  const cancelActiveTask = async () => {
    const taskId = activeTask.value?.taskId;
    if (!taskId) return;
    try {
      const res = await axios.post<ResearchTask>(
        `/research/tasks/${encodeURIComponent(taskId)}/cancel`,
      );
      activeTask.value = res.data;
      stopTaskPolling();
      message.success("已请求取消研究任务");
    } catch (e: any) {
      message.error(e.response?.data?.error || "取消研究任务失败");
    }
  };

  const fetchEvents = async (silent = false) => {
    if (!selectedSessionId.value) return;
    loadingEvents.value = true;
    try {
      const res = await axios.get<ResearchEventsResponse>(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/events`,
        {
          params: {
            limit: eventLimit.value,
            offset: eventOffset.value,
            q: eventSearch.value.trim() || undefined,
          },
        },
      );
      events.value = res.data.events || [];
      eventsTotal.value = res.data.total || 0;
      if (!silent) message.success(`已加载 ${events.value.length} 条事件`);
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error || "加载研究事件失败");
    } finally {
      loadingEvents.value = false;
    }
  };

  const fetchResults = async (silent = false) => {
    if (!selectedSessionId.value) return;
    loadingResults.value = true;
    try {
      const res = await axios.get<ResearchResults>(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/results`,
      );
      results.value = res.data;
      if (!silent) message.success("已刷新研究聚合结果");
    } catch (e: any) {
      results.value = null;
      if (!silent) message.error(e.response?.data?.error || "加载研究结果失败");
    } finally {
      loadingResults.value = false;
    }
  };

  const pageEvents = async (direction: "prev" | "next") => {
    const delta = direction === "next" ? eventLimit.value : -eventLimit.value;
    eventOffset.value = Math.max(0, eventOffset.value + delta);
    await fetchEvents(true);
  };

  const downloadArtifact = async (format: ExportFormat) => {
    if (!ensureSelectedSession()) return;
    exportingArtifact.value = true;
    try {
      const res = await axios.get(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/export`,
        {
          params: { format },
          responseType: "blob",
        },
      );
      const filename =
        parseFilename(res.headers["content-disposition"]) ||
        safeExportName(selectedSession.value, format);
      downloadBlob(res.data, filename);
      message.success(`已下载 ${format.toUpperCase()} 研究产物`);
    } catch (e: any) {
      message.error(e.response?.data?.error || "下载研究产物失败，请先生成导出包");
    } finally {
      exportingArtifact.value = false;
    }
  };

  const fetchResearchTrainingDataset = async (silent = false) => {
    if (!ensureSelectedSession()) return;
    loadingResearchTraining.value = true;
    try {
      const res = await axios.get<ResearchTrainingDataset>(
        `/research/sessions/${encodeURIComponent(
          selectedSessionId.value,
        )}/training`,
        {
          params: {
            format: "json",
            labelPolicy: researchTrainingLabelPolicy.value,
          },
        },
      );
      researchTrainingDataset.value = res.data;
      researchTrainingImportResult.value = null;
      if (!silent) {
        message.success(
          `已生成 ${res.data.sampleCount || 0} 条训练样本，已标注 ${res.data.labeledCount || 0} 条`,
        );
      }
    } catch (e: any) {
      if (!silent) {
        message.error(e.response?.data?.error || "生成 Research 训练集失败");
      }
    } finally {
      loadingResearchTraining.value = false;
    }
  };

  const importResearchTrainingDataset = async () => {
    if (!ensureSelectedSession()) return;
    importingResearchTraining.value = true;
    try {
      const res = await axios.post<ResearchTrainingImportResponse>(
        `/research/sessions/${encodeURIComponent(
          selectedSessionId.value,
        )}/training/import`,
        {
          labelPolicy: researchTrainingLabelPolicy.value,
          limit: Math.max(0, researchTrainingImportLimit.value || 0),
        },
      );
      researchTrainingImportResult.value = res.data;
      message.success(
        `训练样本导入完成：新增 ${res.data.imported || 0} 条，跳过 ${res.data.skipped || 0} 条`,
      );
    } catch (e: any) {
      message.error(e.response?.data?.error || "导入 Research 训练样本失败");
    } finally {
      importingResearchTraining.value = false;
    }
  };

  const downloadResearchTrainingDataset = async (
    format: TrainingExportFormat,
  ) => {
    if (!ensureSelectedSession()) return;
    exportingResearchTraining.value = true;
    try {
      const res = await axios.get(
        `/research/sessions/${encodeURIComponent(
          selectedSessionId.value,
        )}/training`,
        {
          params: {
            format,
            labelPolicy: researchTrainingLabelPolicy.value,
          },
          responseType: "blob",
        },
      );
      downloadBlob(
        res.data,
        safeTrainingExportName(selectedSession.value, format),
      );
      message.success(`已下载 Research 训练集 ${format.toUpperCase()}`);
    } catch (e: any) {
      message.error(e.response?.data?.error || "下载 Research 训练集失败");
    } finally {
      exportingResearchTraining.value = false;
    }
  };

  const formatBytes = (bytes?: number) => {
    const value = bytes || 0;
    if (value < 1024) return `${value} B`;
    if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
    return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  };

  const statusColor = (status?: string) => {
    switch ((status || "").toLowerCase()) {
      case "ready":
      case "succeeded":
        return "green";
      case "running":
      case "building":
      case "queued":
        return "processing";
      case "failed":
      case "error":
        return "red";
      case "canceled":
        return "orange";
      case "empty":
        return "default";
      default:
        return "blue";
    }
  };

  const riskColor = (risk?: number) => {
    const score = risk || 0;
    if (score >= 90) return "red";
    if (score >= 60) return "orange";
    if (score > 0) return "gold";
    return "default";
  };

  const formatTime = (value?: string | number) => {
    if (!value) return "—";
    const date = typeof value === "number" ? new Date(value) : new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return date.toLocaleString();
  };

  onUnmounted(stopTaskPolling);

  return {
    sessions,
    selectedSessionId,
    selectedSession,
    events,
    results,
    activeTask,
    artifactRefs,
    loadingSessions,
    creatingSession,
    deletingSession,
    loadingEvents,
    loadingResults,
    submittingTask,
    exportingArtifact,
    loadingResearchTraining,
    importingResearchTraining,
    exportingResearchTraining,
    eventSearch,
    eventLimit,
    eventOffset,
    eventsTotal,
    compareWindowHours,
    researchTrainingLabelPolicy,
    researchTrainingImportLimit,
    researchTrainingDataset,
    researchTrainingImportResult,
    createForm,
    hasEvents,
    canPageBack,
    canPageForward,
    researchTrainingPreviewSamples,
    refreshSessions,
    createSession,
    selectSession,
    deleteSelectedSession,
    buildSession,
    scanRecent,
    exportBundle,
    resetSession,
    compareRecentWindows,
    cancelActiveTask,
    fetchEvents,
    fetchResults,
    pageEvents,
    downloadArtifact,
    fetchResearchTrainingDataset,
    importResearchTrainingDataset,
    downloadResearchTrainingDataset,
    formatBytes,
    statusColor,
    riskColor,
    formatTime,
  };
}
