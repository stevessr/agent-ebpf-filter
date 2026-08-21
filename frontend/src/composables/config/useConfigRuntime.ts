import { ref, computed } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import { setStoredApiToken } from "../../utils/requestContext";
import { useFeatureManifest } from "./useFeatureManifest";
import {
  defaultDomainForwardProxy,
  defaultKernelRiskFeedback,
  defaultLoopDetection,
  defaultLoopDetectionStatus,
  defaultResearchProcessing,
  defaultResearchProcessingStatus,
  defaultSignalProgramLogWriterStatus,
  defaultSignalProcessing,
  defaultSignalProcessingStatus,
  defaultTracepointBootstrapStatus,
  nextEditableRowId,
  normalizeDomainForwardProxy,
  normalizeKernelRiskFeedback,
  normalizeLoopDetection,
  normalizeLoopDetectionStatus,
  normalizeResearchProcessing,
  normalizeResearchProcessingStatus,
  normalizeSignalProgramLogWriterStatus,
  normalizeSignalProcessing,
  normalizeSignalProcessingStatus,
  normalizeTracepointBootstrapStatus,
  type EditableDomainForwardRoute,
  type EditableHeaderRow,
} from "./runtimeSettings";
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
  SignalCondition,
  SignalProgramLogStatus,
  SignalProgramLogsResponse,
  SignalProgramLogWriterStatus,
  SignalProcessingSettings,
  SignalProcessingStatus,
  SignalRule,
  SignalRuleTestResponse,
  DomainForwardProxyStatus,
  TracepointBootstrapStatus,
} from "../../types/config";

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
    signalProcessing: defaultSignalProcessing(),
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
    semanticStateEntriesByKind: {},
    semanticStateEntries: 0,
    semanticStateMaxEntries: 0,
    semanticStateExpiredEvictionsTotal: 0,
    semanticStateCapacityEvictionsTotal: 0,
    semanticStateTruncatedValuesTotal: 0,
    semanticStateIgnoredOversizedMetadataTotal: 0,
    semanticStateLastSweepAt: "",
    toolBaselineTools: 0,
    toolBaselineSamples: 0,
    toolBaselineMaxTools: 0,
    toolBaselineMaxSamples: 0,
    toolBaselineMaxSamplesPerTool: 0,
    toolBaselineObservationsTotal: 0,
    toolBaselineDriftsTotal: 0,
    toolBaselineExpiredEvictionsTotal: 0,
    toolBaselineCapacityEvictionsTotal: 0,
    toolBaselineTruncatedValuesTotal: 0,
    toolBaselineLastSweepAt: "",
    backendQueueLen: 0,
    wsClients: 0,
    persistAppendLatencyNs: 0,
    capturedArchivedTotal: 0,
    capturedPersistedTotal: 0,
    capturedPersistErrorsTotal: 0,
    persistWriterActive: false,
    persistWriterStopping: false,
    persistQueueLen: 0,
    persistQueueCap: 0,
    persistPending: 0,
    persistGenerationEnqueued: 0,
    persistGenerationPersisted: 0,
    persistGenerationFailed: 0,
    persistGenerationDropped: 0,
    persistWriterLastFlushedAt: "",
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
    queueCap: 0,
    enqueuedEvents: 0,
    processedEvents: 0,
    activeRunSpans: 0,
    activeTaskSpans: 0,
    activeToolSpans: 0,
    maxRunSpans: 0,
    maxTaskSpans: 0,
    maxToolSpans: 0,
    evictedRunSpans: 0,
    evictedTaskSpans: 0,
    evictedToolSpans: 0,
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
  const signalProcessingStatus = ref<SignalProcessingStatus>(
    defaultSignalProcessingStatus(),
  );
  const signalProgramLogs = ref<SignalProgramLogStatus[]>([]);
  const signalProgramLogWriterStatus = ref<SignalProgramLogWriterStatus>(
    defaultSignalProgramLogWriterStatus(),
  );

  const applySignalProgramLogsResponse = (
    value?: Partial<SignalProgramLogsResponse>,
  ) => {
    signalProgramLogs.value = Array.isArray(value?.logs) ? value.logs : [];
    signalProgramLogWriterStatus.value = normalizeSignalProgramLogWriterStatus(
      value?.writer,
    );
  };

  const syncApiToken = (token: string) => {
    const normalized = token.trim();
    if (typeof window === "undefined") return;
    if (!normalized) {
      setStoredApiToken("");
      return;
    }
    setStoredApiToken(normalized);
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
      signalProcessing: normalizeSignalProcessing(
        data.runtime.signalProcessing,
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
      signalProcessingRes,
      signalProgramLogsRes,
      featureRes,
    ] = await Promise.allSettled([
      axios.get("/config/runtime"),
      axios.get("/system/bootstrap-health"),
      axios.get("/system/collector-health"),
      axios.get("/system/otel-health"),
      axios.get("/system/domain-forward/status"),
      axios.get("/system/loop-detection/status"),
      axios.get("/system/research-processing/status"),
      axios.get("/system/signals/status"),
      axios.get("/system/signals/program-logs"),
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
    if (signalProcessingRes.status === "fulfilled") {
      signalProcessingStatus.value = normalizeSignalProcessingStatus(
        signalProcessingRes.value.data as Partial<SignalProcessingStatus>,
      );
    } else {
      console.error("Failed to fetch signal processing status");
    }
    if (signalProgramLogsRes.status === "fulfilled") {
      applySignalProgramLogsResponse(
        signalProgramLogsRes.value.data as SignalProgramLogsResponse,
      );
    } else {
      console.error("Failed to fetch signal program logs");
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
      signalProcessingRes,
      signalProgramLogsRes,
    ] = await Promise.allSettled([
      axios.get("/system/bootstrap-health"),
      axios.get("/system/collector-health"),
      axios.get("/system/otel-health"),
      axios.get("/system/domain-forward/status"),
      axios.get("/system/loop-detection/status"),
      axios.get("/system/research-processing/status"),
      axios.get("/system/signals/status"),
      axios.get("/system/signals/program-logs"),
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
    if (signalProcessingRes.status === "fulfilled") {
      signalProcessingStatus.value = normalizeSignalProcessingStatus(
        signalProcessingRes.value.data as Partial<SignalProcessingStatus>,
      );
    } else {
      console.error("Failed to fetch signal processing status");
    }
    if (signalProgramLogsRes.status === "fulfilled") {
      applySignalProgramLogsResponse(
        signalProgramLogsRes.value.data as SignalProgramLogsResponse,
      );
    } else {
      console.error("Failed to fetch signal program logs");
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
        signalProcessing: runtimeSettings.value.signalProcessing,
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

  const fetchSignalProcessingStatus = async () => {
    try {
      const res = await axios.get("/system/signals/status");
      signalProcessingStatus.value = normalizeSignalProcessingStatus(
        res.data as Partial<SignalProcessingStatus>,
      );
    } catch (_) {
      message.error("Failed to fetch signal processing status");
    }
  };

  const runSignalProcessingScan = async (limit = 1000) => {
    try {
      await axios.post("/system/signals/task", {
        action: "scan_recent",
        limit,
      });
      await fetchSignalProcessingStatus();
      message.success("Signal scan queued");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to queue signal scan",
      );
    }
  };

  const resetSignalProcessing = async () => {
    try {
      await axios.post("/system/signals/task", { action: "reset" });
      await fetchSignalProcessingStatus();
      message.success("Signal state reset");
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to reset signals");
    }
  };

  const expireSignalProcessing = async () => {
    try {
      await axios.post("/system/signals/task", { action: "expire" });
      await fetchSignalProcessingStatus();
      message.success("Signal TTL eviction queued");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to queue signal TTL eviction",
      );
    }
  };

  const fetchSignalProgramLogs = async () => {
    try {
      const res = await axios.get("/system/signals/program-logs");
      applySignalProgramLogsResponse(res.data as SignalProgramLogsResponse);
    } catch (_) {
      message.error("Failed to fetch signal program logs");
    }
  };

  const downloadSignalProgramLog = async (program: string) => {
    const normalized = program.trim();
    if (!normalized) {
      message.warning("Program is empty");
      return;
    }
    try {
      const res = await axios.get("/system/signals/program-logs/download", {
        params: { program: normalized },
        responseType: "blob",
      });
      const blob = new Blob([res.data], {
        type: "application/octet-stream",
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${normalized.replace(/[^a-zA-Z0-9_.-]+/g, "_")}.pb.gzlog`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      message.success("Signal program log download started");
    } catch (err: any) {
      message.error(
        err?.response?.data?.error || "Failed to download signal program log",
      );
    }
  };

  const testSignalRule = async (
    rule: SignalRule,
    limit = 500,
  ): Promise<SignalRuleTestResponse | null> => {
    try {
      const res = await axios.post("/system/signals/rules/test", {
        rule,
        limit,
      });
      const result = res.data as SignalRuleTestResponse;
      message.success(
        `Signal rule matched ${result.matchedTotal}/${result.scannedTotal} recent events`,
      );
      return result;
    } catch (err: any) {
      message.error(err?.response?.data?.error || "Failed to test signal rule");
      return null;
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
    signalProcessingStatus,
    signalProgramLogs,
    signalProgramLogWriterStatus,
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
    fetchSignalProcessingStatus,
    runSignalProcessingScan,
    resetSignalProcessing,
    expireSignalProcessing,
    fetchSignalProgramLogs,
    downloadSignalProgramLog,
    testSignalRule,
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
