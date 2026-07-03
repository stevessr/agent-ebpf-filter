import { ref, computed } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import { setStoredApiToken } from "../../utils/requestContext";
import { useFeatureManifest } from "./useFeatureManifest";
import type {
  RuntimeSettings,
  RuntimeConfigResponse,
  CollectorHealthResponse,
  OTelHealthResponse,
  DomainForwardRoute,
  DomainForwardProxySettings,
  KernelRiskFeedbackSettings,
  LoopDetectionSettings,
  LoopDetectionStatus,
  ResearchProcessingSettings,
  ResearchProcessingStatus,
  DomainForwardProxyStatus,
  TracepointBootstrapStatus,
} from "../../types/config";

interface EditableHeaderRow {
  id: string;
  key: string;
  value: string;
}

interface EditableDomainForwardRoute extends DomainForwardRoute {
  id: string;
}

let editableRowSequence = 0;
const nextEditableRowId = (prefix: string) =>
  `${prefix}-${Date.now()}-${editableRowSequence++}`;

const defaultDomainForwardProxy = (): DomainForwardProxySettings => ({
  enabled: false,
  httpPort: 80,
  httpsPort: 443,
  defaultScheme: "https",
  allowAnyHost: false,
  dnsResolver: "",
  dialTimeoutSeconds: 10,
  certFile: "",
  keyFile: "",
  routes: [],
});

const defaultKernelRiskFeedback = (): KernelRiskFeedbackSettings => ({
  enabled: false,
  minRiskScore: 85,
  enforceNetwork: true,
  enforceFileNames: true,
  enforceExec: true,
  maxActionsPerMinute: 30,
});

const defaultLoopDetection = (): LoopDetectionSettings => ({
  enabled: false,
  windowSeconds: 30,
  repeatThreshold: 5,
  maxContexts: 512,
  queueSize: 2048,
  emitSemanticAlerts: true,
});

const defaultLoopDetectionStatus = (): LoopDetectionStatus => ({
  enabled: false,
  settings: defaultLoopDetection(),
  queueLen: 0,
  queueCap: 0,
  consumedTotal: 0,
  findingsTotal: 0,
  droppedTotal: 0,
  windowCount: 0,
  recentFindings: [],
  updatedAt: "",
});

const defaultResearchProcessing = (): ResearchProcessingSettings => ({
  enabled: false,
  maxEvents: 5000,
  queueSize: 2048,
  timelineBucketSeconds: 60,
  topK: 20,
  recentSamples: 25,
  artifactRetentionDays: 14,
  maxSessionEvents: 50000,
  exportFormats: "jsonl,csv,bundle",
});

const defaultResearchProcessingStatus = (): ResearchProcessingStatus => ({
  enabled: false,
  settings: defaultResearchProcessing(),
  queueLen: 0,
  queueCap: 0,
  consumedTotal: 0,
  droppedTotal: 0,
  bufferedTotal: 0,
  updatedAt: "",
  summary: {
    total: 0,
    bySource: [],
    byType: [],
    byComm: [],
    byPid: [],
    byTrace: [],
    timeline: [],
    topProcesses: [],
    topTraces: [],
    recentSamples: [],
    generatedTimestamp: 0,
    generatedTime: "",
  },
});

const defaultTracepointBootstrapStatus = (): TracepointBootstrapStatus => ({
  kernelRelease: "unknown",
  compiledCount: 0,
  attachedCount: 0,
  skippedCount: 0,
  skippedTracepoints: [],
  status: "unknown",
  message: "Tracepoint bootstrap has not been observed yet.",
});

const toFiniteNumber = (value: unknown, fallback: number) => {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
};

const normalizeTracepointBootstrapStatus = (
  value?: Partial<TracepointBootstrapStatus>,
): TracepointBootstrapStatus => {
  const defaults = defaultTracepointBootstrapStatus();
  const skippedTracepoints = Array.isArray(value?.skippedTracepoints)
    ? value.skippedTracepoints.filter(
        (tracepoint): tracepoint is string => typeof tracepoint === "string",
      )
    : [];
  const validStatuses: TracepointBootstrapStatus["status"][] = [
    "unknown",
    "ready",
    "partial",
    "error",
  ];
  const status = validStatuses.includes(
    value?.status as TracepointBootstrapStatus["status"],
  )
    ? (value?.status as TracepointBootstrapStatus["status"])
    : defaults.status;

  return {
    ...defaults,
    ...value,
    kernelRelease: value?.kernelRelease || defaults.kernelRelease,
    compiledCount: toFiniteNumber(value?.compiledCount, defaults.compiledCount),
    attachedCount: toFiniteNumber(value?.attachedCount, defaults.attachedCount),
    skippedCount: toFiniteNumber(value?.skippedCount, skippedTracepoints.length),
    skippedTracepoints,
    status,
    message: value?.message || defaults.message,
  };
};

const normalizeDomainForwardProxy = (
  value?: Partial<DomainForwardProxySettings>,
): DomainForwardProxySettings => {
  const defaults = defaultDomainForwardProxy();
  const scheme = value?.defaultScheme === "http" ? "http" : "https";
  return {
    ...defaults,
    ...value,
    defaultScheme: scheme,
    httpPort: Number(value?.httpPort || defaults.httpPort),
    httpsPort: Number(value?.httpsPort || defaults.httpsPort),
    dialTimeoutSeconds: Number(
      value?.dialTimeoutSeconds || defaults.dialTimeoutSeconds,
    ),
    routes: Array.isArray(value?.routes) ? value.routes : [],
  };
};

const normalizeKernelRiskFeedback = (
  value?: Partial<KernelRiskFeedbackSettings>,
): KernelRiskFeedbackSettings => {
  const defaults = defaultKernelRiskFeedback();
  return {
    ...defaults,
    ...value,
    minRiskScore: Number(value?.minRiskScore || defaults.minRiskScore),
    maxActionsPerMinute: Number(
      value?.maxActionsPerMinute || defaults.maxActionsPerMinute,
    ),
  };
};

const normalizeLoopDetection = (
  value?: Partial<LoopDetectionSettings>,
): LoopDetectionSettings => {
  const defaults = defaultLoopDetection();
  return {
    ...defaults,
    ...value,
    windowSeconds: Number(value?.windowSeconds || defaults.windowSeconds),
    repeatThreshold: Number(
      value?.repeatThreshold || defaults.repeatThreshold,
    ),
    maxContexts: Number(value?.maxContexts || defaults.maxContexts),
    queueSize: Number(value?.queueSize || defaults.queueSize),
    emitSemanticAlerts:
      value?.emitSemanticAlerts ?? defaults.emitSemanticAlerts,
  };
};

const normalizeLoopDetectionStatus = (
  value?: Partial<LoopDetectionStatus>,
): LoopDetectionStatus => {
  const defaults = defaultLoopDetectionStatus();
  return {
    ...defaults,
    ...value,
    settings: normalizeLoopDetection(value?.settings),
    recentFindings: Array.isArray(value?.recentFindings)
      ? value.recentFindings
      : [],
  };
};

const normalizeResearchProcessing = (
  value?: Partial<ResearchProcessingSettings>,
): ResearchProcessingSettings => {
  const defaults = defaultResearchProcessing();
  return {
    ...defaults,
    ...value,
    maxEvents: Number(value?.maxEvents || defaults.maxEvents),
    queueSize: Number(value?.queueSize || defaults.queueSize),
    timelineBucketSeconds: Number(
      value?.timelineBucketSeconds || defaults.timelineBucketSeconds,
    ),
    topK: Number(value?.topK || defaults.topK),
    recentSamples: Number(value?.recentSamples || defaults.recentSamples),
    artifactRetentionDays: Number(
      value?.artifactRetentionDays || defaults.artifactRetentionDays,
    ),
    maxSessionEvents: Number(
      value?.maxSessionEvents || defaults.maxSessionEvents,
    ),
    exportFormats: String(value?.exportFormats || defaults.exportFormats),
  };
};

const normalizeResearchProcessingStatus = (
  value?: Partial<ResearchProcessingStatus>,
): ResearchProcessingStatus => {
  const defaults = defaultResearchProcessingStatus();
  const summary = value?.summary || defaults.summary;
  return {
    ...defaults,
    ...value,
    settings: normalizeResearchProcessing(value?.settings),
    summary: {
      ...defaults.summary,
      ...summary,
      bySource: Array.isArray(summary.bySource) ? summary.bySource : [],
      byType: Array.isArray(summary.byType) ? summary.byType : [],
      byComm: Array.isArray(summary.byComm) ? summary.byComm : [],
      byPid: Array.isArray(summary.byPid) ? summary.byPid : [],
      byTrace: Array.isArray(summary.byTrace) ? summary.byTrace : [],
      timeline: Array.isArray(summary.timeline) ? summary.timeline : [],
      topProcesses: Array.isArray(summary.topProcesses)
        ? summary.topProcesses
        : [],
      topTraces: Array.isArray(summary.topTraces) ? summary.topTraces : [],
      recentSamples: Array.isArray(summary.recentSamples)
        ? summary.recentSamples
        : [],
    },
  };
};

export function useConfigRuntime() {
  const featureManifest = useFeatureManifest();
  const runtimeSettings = ref<RuntimeSettings>({
    logPersistenceEnabled: false,
    logFilePath: "",
    accessToken: "",
    maxEventCount: 1500,
    maxEventAge: "0",
    shellSessionsEnabled: false,
    systemRunEnabled: false,
    hookManagementEnabled: false,
    policyManagementEnabled: false,
    otlpEnabled: false,
    otlpEndpoint: "",
    otlpServiceName: "agent-ebpf-filter",
    otlpHeaders: {},
    tlsCaptureEnabled: false,
    kernelRiskFeedback: defaultKernelRiskFeedback(),
    loopDetection: defaultLoopDetection(),
    researchProcessing: defaultResearchProcessing(),
    domainForwardProxy: defaultDomainForwardProxy(),
  });
  const mcpEndpoint = ref("");
  const authHeaderName = ref("X-API-KEY");
  const bearerAuthHeaderName = ref("Authorization: Bearer");
  const persistedEventLogPath = ref("");
  const persistedEventLogAlive = ref(false);
  const otlpHeaderRows = ref<EditableHeaderRow[]>([]);
  const domainForwardRoutes = ref<EditableDomainForwardRoute[]>([]);
  const collectorHealth = ref<CollectorHealthResponse>({
    collectorMapAvailable: false,
    ringbufEventsTotal: 0,
    ringbufDroppedTotal: 0,
    ringbufReserveFailedTotal: 0,
    ringbufZeroCopyDecodeTotal: 0,
    ringbufCopyDecodeTotal: 0,
    eventsByTypeTotal: {},
    eventsByPidTotal: {},
    agentSightCountersTotal: {},
    backendQueueLen: 0,
    wsClients: 0,
    persistAppendLatencyNs: 0,
    kernelRiskEvaluationsTotal: 0,
    kernelRiskAlertsTotal: 0,
    kernelRiskBlocksTotal: 0,
    kernelRiskLastEvalLatencyNs: 0,
    kernelRiskFeedbackApplied: 0,
    kernelRiskFeedbackDropped: 0,
    captureHealthy: true,
  });
  const bootstrapHealth = ref<TracepointBootstrapStatus>(
    defaultTracepointBootstrapStatus(),
  );
  const otelHealth = ref<OTelHealthResponse>({
    enabled: false,
    ready: false,
    endpoint: "",
    serviceName: "",
    queueLen: 0,
    activeRunSpans: 0,
    activeTaskSpans: 0,
    activeToolSpans: 0,
    exportedSpans: 0,
    droppedEvents: 0,
  });
  const domainForwardStatus = ref<DomainForwardProxyStatus>({
    enabled: false,
    httpRunning: false,
    httpsRunning: false,
    httpPort: 80,
    httpsPort: 443,
    routeCount: 0,
    allowAnyHost: false,
    updatedAt: "",
  });
  const loopDetectionStatus = ref<LoopDetectionStatus>(
    defaultLoopDetectionStatus(),
  );
  const researchProcessingStatus = ref<ResearchProcessingStatus>(
    defaultResearchProcessingStatus(),
  );

  const syncApiToken = (token: string) => {
    const normalized = token.trim();
    if (typeof window === "undefined") return;
    if (!normalized) {
      setStoredApiToken("");
      return;
    }
    setStoredApiToken(normalized);
    axios.defaults.headers.common["X-API-KEY"] = normalized;
    axios.defaults.headers.common.Authorization = `Bearer ${normalized}`;
  };

  const applyRuntimeResponse = (data: RuntimeConfigResponse) => {
    runtimeSettings.value = {
      logPersistenceEnabled: data.runtime.logPersistenceEnabled,
      logFilePath: data.runtime.logFilePath,
      accessToken: data.runtime.accessToken,
      maxEventCount: data.runtime.maxEventCount ?? 1500,
      maxEventAge: data.runtime.maxEventAge ?? "0",
      shellSessionsEnabled: Boolean(data.runtime.shellSessionsEnabled),
      systemRunEnabled: Boolean(data.runtime.systemRunEnabled),
      hookManagementEnabled: Boolean(data.runtime.hookManagementEnabled),
      policyManagementEnabled: Boolean(data.runtime.policyManagementEnabled),
      otlpEnabled: Boolean(data.runtime.otlpEnabled),
      otlpEndpoint: data.runtime.otlpEndpoint || "",
      otlpServiceName: data.runtime.otlpServiceName || "agent-ebpf-filter",
      otlpHeaders: { ...(data.runtime.otlpHeaders || {}) },
      tlsCaptureEnabled: Boolean(data.runtime.tlsCaptureEnabled),
      kernelRiskFeedback: normalizeKernelRiskFeedback(
        data.runtime.kernelRiskFeedback,
      ),
      loopDetection: normalizeLoopDetection(data.runtime.loopDetection),
      researchProcessing: normalizeResearchProcessing(
        data.runtime.researchProcessing,
      ),
      domainForwardProxy: normalizeDomainForwardProxy(
        data.runtime.domainForwardProxy,
      ),
    };
    otlpHeaderRows.value = Object.entries(
      runtimeSettings.value.otlpHeaders || {},
    ).map(([key, value]) => ({
      id: nextEditableRowId("otlp-header"),
      key,
      value: String(value ?? ""),
    }));
    domainForwardRoutes.value = (
      runtimeSettings.value.domainForwardProxy.routes || []
    ).map((route) => ({
      id: nextEditableRowId("domain-route"),
      host: route.host || "",
      upstream: route.upstream || "",
      certFile: route.certFile || "",
      keyFile: route.keyFile || "",
    }));
    mcpEndpoint.value = data.mcpEndpoint;
    authHeaderName.value = data.authHeaderName;
    bearerAuthHeaderName.value = data.bearerAuthHeaderName;
    persistedEventLogPath.value = data.persistedEventLogPath;
    persistedEventLogAlive.value = data.persistedEventLogAlive;
    syncApiToken(data.runtime.accessToken);
  };

  const fetchRuntime = async () => {
    const [
      runtimeRes,
      bootstrapRes,
      collectorRes,
      otelRes,
      domainForwardRes,
      loopDetectionRes,
      researchProcessingRes,
      featureRes,
    ] = await Promise.allSettled([
      axios.get("/config/runtime"),
      axios.get("/system/bootstrap-health"),
      axios.get("/system/collector-health"),
      axios.get("/system/otel-health"),
      axios.get("/system/domain-forward/status"),
      axios.get("/system/loop-detection/status"),
      axios.get("/system/research-processing/status"),
      featureManifest.fetchFeatureManifest(),
    ]);
    if (runtimeRes.status === "fulfilled") {
      applyRuntimeResponse(runtimeRes.value.data as RuntimeConfigResponse);
    } else {
      console.error("Failed to fetch runtime config");
    }
    if (bootstrapRes.status === "fulfilled") {
      bootstrapHealth.value = normalizeTracepointBootstrapStatus(
        bootstrapRes.value.data as Partial<TracepointBootstrapStatus>,
      );
    } else {
      console.error("Failed to fetch bootstrap health");
    }
    if (collectorRes.status === "fulfilled") {
      collectorHealth.value = collectorRes.value
        .data as CollectorHealthResponse;
    } else {
      console.error("Failed to fetch collector health");
    }
    if (otelRes.status === "fulfilled") {
      otelHealth.value = otelRes.value.data as OTelHealthResponse;
    } else {
      console.error("Failed to fetch OTLP health");
    }
    if (domainForwardRes.status === "fulfilled") {
      domainForwardStatus.value = domainForwardRes.value
        .data as DomainForwardProxyStatus;
    } else {
      console.error("Failed to fetch domain forward proxy status");
    }
    if (loopDetectionRes.status === "fulfilled") {
      loopDetectionStatus.value = normalizeLoopDetectionStatus(
        loopDetectionRes.value.data as Partial<LoopDetectionStatus>,
      );
    } else {
      console.error("Failed to fetch loop detection status");
    }
    if (researchProcessingRes.status === "fulfilled") {
      researchProcessingStatus.value = normalizeResearchProcessingStatus(
        researchProcessingRes.value.data as Partial<ResearchProcessingStatus>,
      );
    } else {
      console.error("Failed to fetch research processing status");
    }
    if (featureRes.status === "rejected") {
      console.error("Failed to fetch feature manifest");
    }
  };

  const fetchCollectorHealth = async () => {
    const [
      bootstrapRes,
      collectorRes,
      otelRes,
      domainForwardRes,
      loopDetectionRes,
      researchProcessingRes,
    ] =
      await Promise.allSettled([
        axios.get("/system/bootstrap-health"),
        axios.get("/system/collector-health"),
        axios.get("/system/otel-health"),
        axios.get("/system/domain-forward/status"),
        axios.get("/system/loop-detection/status"),
        axios.get("/system/research-processing/status"),
        featureManifest.fetchFeatureManifest(),
      ]);
    if (bootstrapRes.status === "fulfilled") {
      bootstrapHealth.value = normalizeTracepointBootstrapStatus(
        bootstrapRes.value.data as Partial<TracepointBootstrapStatus>,
      );
    } else {
      console.error("Failed to fetch bootstrap health");
    }
    if (collectorRes.status === "fulfilled") {
      collectorHealth.value = collectorRes.value
        .data as CollectorHealthResponse;
    } else {
      console.error("Failed to fetch collector health");
    }
    if (otelRes.status === "fulfilled") {
      otelHealth.value = otelRes.value.data as OTelHealthResponse;
    } else {
      console.error("Failed to fetch OTLP health");
    }
    if (domainForwardRes.status === "fulfilled") {
      domainForwardStatus.value = domainForwardRes.value
        .data as DomainForwardProxyStatus;
    } else {
      console.error("Failed to fetch domain forward proxy status");
    }
    if (loopDetectionRes.status === "fulfilled") {
      loopDetectionStatus.value = normalizeLoopDetectionStatus(
        loopDetectionRes.value.data as Partial<LoopDetectionStatus>,
      );
    } else {
      console.error("Failed to fetch loop detection status");
    }
    if (researchProcessingRes.status === "fulfilled") {
      researchProcessingStatus.value = normalizeResearchProcessingStatus(
        researchProcessingRes.value.data as Partial<ResearchProcessingStatus>,
      );
    } else {
      console.error("Failed to fetch research processing status");
    }
  };

  const parseOTLPHeaders = () => {
    return otlpHeaderRows.value.reduce<Record<string, string>>(
      (acc, row, index) => {
        const key = row.key.trim();
        const value = row.value.trim();
        if (!key && !value) return acc;
        if (!key) {
          throw new Error(`OTLP header #${index + 1} requires a name`);
        }
        acc[key] = value;
        return acc;
      },
      {},
    );
  };

  const parseDomainForwardRoutes = () => {
    return domainForwardRoutes.value.reduce<DomainForwardRoute[]>(
      (acc, route, index) => {
        const host = String(route.host || "").trim();
        const upstream = String(route.upstream || "").trim();
        const certFile = String(route.certFile || "").trim();
        const keyFile = String(route.keyFile || "").trim();
        if (!host && !upstream && !certFile && !keyFile) return acc;
        if (!host) {
          throw new Error(`Domain forward route #${index + 1} requires a host`);
        }
        acc.push({ host, upstream, certFile, keyFile });
        return acc;
      },
      [],
    );
  };

  const addOTLPHeaderRow = () => {
    otlpHeaderRows.value.push({
      id: nextEditableRowId("otlp-header"),
      key: "",
      value: "",
    });
  };

  const removeOTLPHeaderRow = (id: string) => {
    otlpHeaderRows.value = otlpHeaderRows.value.filter((row) => row.id !== id);
  };

  const addDomainForwardRoute = () => {
    domainForwardRoutes.value.push({
      id: nextEditableRowId("domain-route"),
      host: "",
      upstream: "",
      certFile: "",
      keyFile: "",
    });
  };

  const removeDomainForwardRoute = (id: string) => {
    domainForwardRoutes.value = domainForwardRoutes.value.filter(
      (route) => route.id !== id,
    );
  };

  const saveRuntime = async () => {
    try {
      const otlpHeaders = parseOTLPHeaders();
      const domainForwardRoutes = parseDomainForwardRoutes();
      const res = await axios.put("/config/runtime", {
        logPersistenceEnabled: runtimeSettings.value.logPersistenceEnabled,
        logFilePath: runtimeSettings.value.logFilePath,
        maxEventCount: runtimeSettings.value.maxEventCount,
        maxEventAge: runtimeSettings.value.maxEventAge,
        shellSessionsEnabled: runtimeSettings.value.shellSessionsEnabled,
        systemRunEnabled: runtimeSettings.value.systemRunEnabled,
        hookManagementEnabled: runtimeSettings.value.hookManagementEnabled,
        policyManagementEnabled: runtimeSettings.value.policyManagementEnabled,
        otlpEnabled: runtimeSettings.value.otlpEnabled,
        otlpEndpoint: runtimeSettings.value.otlpEndpoint,
        otlpServiceName: runtimeSettings.value.otlpServiceName,
        otlpHeaders,
        tlsCaptureEnabled: runtimeSettings.value.tlsCaptureEnabled,
        kernelRiskFeedback: runtimeSettings.value.kernelRiskFeedback,
        loopDetection: runtimeSettings.value.loopDetection,
        researchProcessing: runtimeSettings.value.researchProcessing,
        domainForwardProxy: {
          ...runtimeSettings.value.domainForwardProxy,
          routes: domainForwardRoutes,
        },
      });
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
      await fetchCollectorHealth();
      message.success("Runtime settings saved");
    } catch (error: any) {
      message.error(error?.message || "Failed to save runtime settings");
    }
  };

  const fetchLoopDetectionStatus = async () => {
    try {
      const res = await axios.get("/system/loop-detection/status");
      loopDetectionStatus.value = normalizeLoopDetectionStatus(
        res.data as Partial<LoopDetectionStatus>,
      );
    } catch (_) {
      message.error("Failed to fetch loop detection status");
    }
  };

  const runLoopDetectionScan = async (limit = 500) => {
    try {
      await axios.post("/system/loop-detection/task", {
        action: "scan_recent",
        limit,
      });
      await fetchLoopDetectionStatus();
      message.success("Loop detection scan queued");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to queue loop detection scan",
      );
    }
  };

  const resetLoopDetection = async () => {
    try {
      await axios.post("/system/loop-detection/task", { action: "reset" });
      await fetchLoopDetectionStatus();
      message.success("Loop detection state reset");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to reset loop detection",
      );
    }
  };

  const fetchResearchProcessingStatus = async () => {
    try {
      const res = await axios.get("/system/research-processing/status");
      researchProcessingStatus.value = normalizeResearchProcessingStatus(
        res.data as Partial<ResearchProcessingStatus>,
      );
    } catch (_) {
      message.error("Failed to fetch research processing status");
    }
  };

  const runResearchProcessingScan = async (limit = 1000) => {
    try {
      await axios.post("/system/research-processing/task", {
        action: "scan_recent",
        limit,
      });
      await fetchResearchProcessingStatus();
      message.success("Backend research processing scan queued");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error ||
          "Failed to queue backend research processing scan",
      );
    }
  };

  const resetResearchProcessing = async () => {
    try {
      await axios.post("/system/research-processing/task", { action: "reset" });
      await fetchResearchProcessingStatus();
      message.success("Backend research processing state reset");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error ||
          "Failed to reset backend research processing",
      );
    }
  };

  const rotateAccessToken = async () => {
    try {
      const res = await axios.post("/config/access-token");
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
      await fetchCollectorHealth();
      message.success("Access token regenerated");
    } catch (_) {
      message.error("Failed to regenerate access token");
    }
  };

  const clearInMemoryEvents = async () => {
    try {
      await axios.post("/data/clear-events-memory");
      message.success("In-memory events cleared");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to clear memory events",
      );
    }
  };

  const clearPersistedLog = async () => {
    try {
      await axios.post("/data/clear-events-persisted");
      message.success("Persisted event log truncated");
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to truncate log");
    }
  };

  const clearAllEvents = async () => {
    try {
      await axios.post("/data/clear-events");
      message.success("All events cleared");
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to clear events");
    }
  };

  const copyText = async (text: string, successMessage: string) => {
    const value = text.trim();
    if (!value) {
      message.warning("Nothing to copy");
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      message.success(successMessage);
    } catch (_) {
      message.error("Failed to copy to clipboard");
    }
  };

  const mcpQueryEndpoint = computed(() => {
    if (!mcpEndpoint.value) return "";
    if (!runtimeSettings.value.accessToken.trim()) {
      return `${mcpEndpoint.value}?key=$API_KEY`;
    }
    return `${mcpEndpoint.value}?key=${encodeURIComponent(runtimeSettings.value.accessToken)}`;
  });

  const mcpQueryEndpointTemplate = computed(() => {
    if (!mcpEndpoint.value) return "";
    return `${mcpEndpoint.value}?key=$API_KEY`;
  });

  return {
    runtimeSettings,
    otlpHeaderRows,
    otelHealth,
    domainForwardRoutes,
    domainForwardStatus,
    loopDetectionStatus,
    researchProcessingStatus,
    mcpEndpoint,
    authHeaderName,
    bearerAuthHeaderName,
    persistedEventLogPath,
    persistedEventLogAlive,
    bootstrapHealth,
    collectorHealth,
    featureManifest,
    syncApiToken,
    applyRuntimeResponse,
    fetchRuntime,
    fetchCollectorHealth,
    fetchLoopDetectionStatus,
    runLoopDetectionScan,
    resetLoopDetection,
    fetchResearchProcessingStatus,
    runResearchProcessingScan,
    resetResearchProcessing,
    saveRuntime,
    addOTLPHeaderRow,
    removeOTLPHeaderRow,
    addDomainForwardRoute,
    removeDomainForwardRoute,
    rotateAccessToken,
    clearInMemoryEvents,
    clearPersistedLog,
    clearAllEvents,
    copyText,
    mcpQueryEndpoint,
    mcpQueryEndpointTemplate,
  };
}
