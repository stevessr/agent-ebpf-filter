import { computed, onUnmounted, ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import type {
  ResearchArtifactRef,
  ResearchCreateSessionRequest,
  ResearchEventsResponse,
  ResearchEvent,
  ResearchResults,
  ResearchSecurityEvaluationReport,
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
type SecurityEvaluationExportFormat = "json" | "jsonl" | "csv";
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

const safeSecurityEvaluationExportName = (
  session: ResearchSession | null,
  format: SecurityEvaluationExportFormat,
) => {
  const base = (session?.name || session?.id || "research-session")
    .trim()
    .replace(/[^a-z0-9._-]+/gi, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
  return `${base || "research-session"}-security-evaluation.${format}`;
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
  const exportingSecurityEvaluation = ref(false);
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
  const securityEvaluationMode = ref<"combined" | "builtin" | "session">(
    "combined",
  );
  const securityEvaluationLimit = ref(5000);
  const securityEvaluationIncludeLLM = ref(false);
  const securityEvaluationLabelPolicy =
    ref("decision_then_heuristic");

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
  let taskPollGeneration = 0;
  let taskRequestController: AbortController | null = null;
  let taskSubmitGeneration = 0;
  let taskSubmitController: AbortController | null = null;
  let taskCancelGeneration = 0;
  let taskCancelController: AbortController | null = null;
  let sessionsRequestGeneration = 0;
  let sessionsRequestController: AbortController | null = null;
  let eventsRequestGeneration = 0;
  let eventsRequestController: AbortController | null = null;
  let resultsRequestGeneration = 0;
  let resultsRequestController: AbortController | null = null;
  let taskPollingDisposed = false;

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
  const securityEvaluation = computed<ResearchSecurityEvaluationReport | null>(
    () => results.value?.securityEvaluation || null,
  );
  const securityEvaluationPreviewSamples = computed(() =>
    (securityEvaluation.value?.samples || []).slice(0, 100),
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

  const refreshSessions = async (silent = false, guard = () => true) => {
    sessionsRequestController?.abort();
    const controller = new AbortController();
    sessionsRequestController = controller;
    const generation = ++sessionsRequestGeneration;
    loadingSessions.value = true;
    try {
      const res = await axios.get<ResearchSessionListResponse>(
        "/research/sessions",
        { signal: controller.signal },
      );
      if (generation !== sessionsRequestGeneration || !guard()) return;
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
      if (controller.signal.aborted || generation !== sessionsRequestGeneration || !guard()) return;
      if (!silent) message.error(e.response?.data?.error || "刷新研究会话失败");
    } finally {
      if (generation === sessionsRequestGeneration) {
        loadingSessions.value = false;
        sessionsRequestController = null;
      }
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
      stopTaskPolling();
      stopTaskSubmission();
      stopTaskCancellation();
      sessions.value = [res.data, ...sessions.value.filter((s) => s.id !== res.data.id)];
      selectedSessionId.value = res.data.id;
      activeTask.value = null;
      eventOffset.value = 0;
      eventsTotal.value = 0;
      events.value = [];
      results.value = null;
      researchTrainingDataset.value = null;
      researchTrainingImportResult.value = null;
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
    stopTaskPolling();
    stopTaskSubmission();
    stopTaskCancellation();
    activeTask.value = null;
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
      stopTaskPolling();
      stopTaskSubmission();
      stopTaskCancellation();
      message.success("研究会话已删除");
      const removed = selectedSessionId.value;
      sessions.value = sessions.value.filter((session) => session.id !== removed);
      selectedSessionId.value = sessions.value[0]?.id || "";
      activeTask.value = null;
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
    taskPollGeneration++;
    taskRequestController?.abort();
    taskRequestController = null;
    if (taskPollTimer !== null) {
      window.clearTimeout(taskPollTimer);
      taskPollTimer = null;
    }
  };

  const stopTaskSubmission = () => {
    taskSubmitGeneration++;
    taskSubmitController?.abort();
    taskSubmitController = null;
    submittingTask.value = false;
  };

  const stopTaskCancellation = () => {
    taskCancelGeneration++;
    taskCancelController?.abort();
    taskCancelController = null;
  };

  const taskPollIsCurrent = (generation: number) =>
    !taskPollingDisposed && generation === taskPollGeneration;

  const refreshTask = async (taskId: string, generation: number) => {
    if (!taskPollIsCurrent(generation)) return;
    const controller = new AbortController();
    taskRequestController?.abort();
    taskRequestController = controller;
    try {
      const res = await axios.get<ResearchTask>(
        `/research/tasks/${encodeURIComponent(taskId)}`,
        { signal: controller.signal },
      );
      if (!taskPollIsCurrent(generation)) return;
      activeTask.value = res.data;
      if (terminalTaskStatuses.has(res.data.status)) {
        taskPollTimer = null;
        await refreshSessions(true, () => taskPollIsCurrent(generation));
        if (!taskPollIsCurrent(generation)) return;
        if (selectedSessionId.value) {
          await Promise.all([
            fetchEvents(true, () => taskPollIsCurrent(generation)),
            fetchResults(true, () => taskPollIsCurrent(generation)),
          ]);
          if (!taskPollIsCurrent(generation)) return;
        }
        if (res.data.status === "succeeded") {
          message.success(`任务完成：${res.data.action}`);
        } else if (res.data.status === "failed") {
          message.error(res.data.error || `任务失败：${res.data.action}`);
        }
        if (taskPollIsCurrent(generation)) taskPollGeneration++;
        return;
      }
      taskPollTimer = window.setTimeout(() => {
        void refreshTask(taskId, generation);
      }, 1200);
    } catch (e: any) {
      if (controller.signal.aborted || !taskPollIsCurrent(generation)) return;
      taskPollGeneration++;
      taskPollTimer = null;
      message.error(e.response?.data?.error || "刷新研究任务失败");
    } finally {
      if (taskRequestController === controller) {
        taskRequestController = null;
      }
    }
  };

  const submitTask = async (
    action: ResearchTaskAction,
    overrides: Partial<ResearchTaskRequest> = {},
  ) => {
    if (!ensureSelectedSession()) return;
    stopTaskPolling();
    stopTaskSubmission();
    stopTaskCancellation();
    submittingTask.value = true;
    const pollGeneration = taskPollGeneration;
    const submitGeneration = taskSubmitGeneration;
    const controller = new AbortController();
    taskSubmitController = controller;
    try {
      const payload: ResearchTaskRequest = {
        action,
        limit: createForm.value.limit,
        ...overrides,
      };
      const res = await axios.post<ResearchTask>(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/tasks`,
        payload,
        { signal: controller.signal },
      );
      if (
        submitGeneration !== taskSubmitGeneration ||
        !taskPollIsCurrent(pollGeneration)
      ) return;
      activeTask.value = res.data;
      message.success(`已提交研究任务：${action}`);
      void refreshTask(res.data.taskId, pollGeneration);
    } catch (e: any) {
      if (
        controller.signal.aborted ||
        submitGeneration !== taskSubmitGeneration ||
        !taskPollIsCurrent(pollGeneration)
      ) return;
      message.error(e.response?.data?.error || "提交研究任务失败");
    } finally {
      if (taskSubmitController === controller) taskSubmitController = null;
      if (submitGeneration === taskSubmitGeneration) {
        submittingTask.value = false;
      }
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

  const runSecurityEvaluation = async () => {
    await submitTask("security_eval", {
      evaluationMode: securityEvaluationMode.value,
      labelPolicy: securityEvaluationLabelPolicy.value,
      includeLLM: securityEvaluationIncludeLLM.value,
      limit: Math.max(1, securityEvaluationLimit.value || createForm.value.limit || 5000),
      sourceFilter: buildFilterFromForm(),
      timeRange: buildTimeRangeFromForm(),
    });
  };

  const cancelActiveTask = async () => {
    const taskId = activeTask.value?.taskId;
    if (!taskId) return;
    stopTaskCancellation();
    const generation = taskCancelGeneration;
    const pollGeneration = taskPollGeneration;
    const sessionId = selectedSessionId.value;
    const controller = new AbortController();
    taskCancelController = controller;
    try {
      const res = await axios.post<ResearchTask>(
        `/research/tasks/${encodeURIComponent(taskId)}/cancel`,
        undefined,
        { signal: controller.signal },
      );
      if (
        generation !== taskCancelGeneration ||
        !taskPollIsCurrent(pollGeneration) ||
        selectedSessionId.value !== sessionId ||
        activeTask.value?.taskId !== taskId
      ) return;
      activeTask.value = res.data;
      stopTaskPolling();
      message.success("已请求取消研究任务");
    } catch (e: any) {
      if (
        controller.signal.aborted ||
        generation !== taskCancelGeneration ||
        !taskPollIsCurrent(pollGeneration)
      ) return;
      message.error(e.response?.data?.error || "取消研究任务失败");
    } finally {
      if (taskCancelController === controller) taskCancelController = null;
    }
  };

  const fetchEvents = async (silent = false, guard = () => true) => {
    const sessionId = selectedSessionId.value;
    if (!sessionId) return;
    eventsRequestController?.abort();
    const controller = new AbortController();
    eventsRequestController = controller;
    const generation = ++eventsRequestGeneration;
    loadingEvents.value = true;
    try {
      const res = await axios.get<ResearchEventsResponse>(
        `/research/sessions/${encodeURIComponent(sessionId)}/events`,
        {
          signal: controller.signal,
          params: {
            limit: eventLimit.value,
            offset: eventOffset.value,
            q: eventSearch.value.trim() || undefined,
          },
        },
      );
      if (
        generation !== eventsRequestGeneration ||
        selectedSessionId.value !== sessionId ||
        !guard()
      ) return;
      events.value = res.data.events || [];
      eventsTotal.value = res.data.total || 0;
      if (!silent) message.success(`已加载 ${events.value.length} 条事件`);
    } catch (e: any) {
      if (
        controller.signal.aborted ||
        generation !== eventsRequestGeneration ||
        selectedSessionId.value !== sessionId ||
        !guard()
      ) return;
      if (!silent) message.error(e.response?.data?.error || "加载研究事件失败");
    } finally {
      if (generation === eventsRequestGeneration) {
        loadingEvents.value = false;
        eventsRequestController = null;
      }
    }
  };

  const fetchResults = async (silent = false, guard = () => true) => {
    const sessionId = selectedSessionId.value;
    if (!sessionId) return;
    resultsRequestController?.abort();
    const controller = new AbortController();
    resultsRequestController = controller;
    const generation = ++resultsRequestGeneration;
    loadingResults.value = true;
    try {
      const res = await axios.get<ResearchResults>(
        `/research/sessions/${encodeURIComponent(sessionId)}/results`,
        { signal: controller.signal },
      );
      if (
        generation !== resultsRequestGeneration ||
        selectedSessionId.value !== sessionId ||
        !guard()
      ) return;
      results.value = res.data;
      if (!silent) message.success("已刷新研究聚合结果");
    } catch (e: any) {
      if (
        controller.signal.aborted ||
        generation !== resultsRequestGeneration ||
        selectedSessionId.value !== sessionId ||
        !guard()
      ) return;
      results.value = null;
      if (!silent) message.error(e.response?.data?.error || "加载研究结果失败");
    } finally {
      if (generation === resultsRequestGeneration) {
        loadingResults.value = false;
        resultsRequestController = null;
      }
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

  const downloadSecurityEvaluation = async (
    format: SecurityEvaluationExportFormat,
  ) => {
    if (!ensureSelectedSession()) return;
    exportingSecurityEvaluation.value = true;
    const backendFormat =
      format === "json"
        ? "security-json"
        : format === "jsonl"
          ? "security-jsonl"
          : "security-csv";
    try {
      const res = await axios.get(
        `/research/sessions/${encodeURIComponent(selectedSessionId.value)}/export`,
        {
          params: { format: backendFormat },
          responseType: "blob",
        },
      );
      const filename =
        parseFilename(res.headers["content-disposition"]) ||
        safeSecurityEvaluationExportName(selectedSession.value, format);
      downloadBlob(res.data, filename);
      message.success(`已下载安全评测 ${format.toUpperCase()}`);
    } catch (e: any) {
      message.error(
        e.response?.data?.error || "下载安全评测失败，请先运行安全评测任务",
      );
    } finally {
      exportingSecurityEvaluation.value = false;
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

  onUnmounted(() => {
    taskPollingDisposed = true;
    stopTaskPolling();
    stopTaskSubmission();
    stopTaskCancellation();
    sessionsRequestGeneration++;
    sessionsRequestController?.abort();
    eventsRequestGeneration++;
    eventsRequestController?.abort();
    resultsRequestGeneration++;
    resultsRequestController?.abort();
  });

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
    exportingSecurityEvaluation,
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
    securityEvaluationMode,
    securityEvaluationLimit,
    securityEvaluationIncludeLLM,
    securityEvaluationLabelPolicy,
    securityEvaluation,
    createForm,
    hasEvents,
    canPageBack,
    canPageForward,
    researchTrainingPreviewSamples,
    securityEvaluationPreviewSamples,
    refreshSessions,
    createSession,
    selectSession,
    deleteSelectedSession,
    buildSession,
    scanRecent,
    exportBundle,
    resetSession,
    compareRecentWindows,
    runSecurityEvaluation,
    cancelActiveTask,
    fetchEvents,
    fetchResults,
    pageEvents,
    downloadArtifact,
    downloadSecurityEvaluation,
    fetchResearchTrainingDataset,
    importResearchTrainingDataset,
    downloadResearchTrainingDataset,
    formatBytes,
    statusColor,
    riskColor,
    formatTime,
  };
}
