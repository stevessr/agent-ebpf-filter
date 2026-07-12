import { ref, computed, watch, onUnmounted } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import type {
  MLStatusState,
  MLLlmConfig,
  MLLlmBatchEntry,
  MLLlmBatchResponse,
  MLTrainingHistoryEntry,
  MLCommandSafetyResult,
  SampleEntry,
  MLBuiltinModelCatalogItem,
  MLCRuntimeStatus,
} from "../../types/config";

import {
  defaultMLBuiltinModelCatalog,
  findMLBuiltinModel,
} from "../../data/mlModelCatalog";
import {
  safetyNetHighRiskPresets,
  classicSecurityDatasetPresets,
  syntheticExpansionPresets,
  highRiskPresets,
} from "./mlPresets";
import type { TrainingPreset } from "./mlPresets";
export {
  safetyNetHighRiskPresets,
  classicSecurityDatasetPresets,
  highRiskPresets,
  syntheticExpansionPresets,
} from "./mlPresets";
import {
  LLM_SCORING_STORAGE_KEY,
  MAX_LLM_BATCH_SCORE_LIMIT,
  MAX_LLM_TIMEOUT_SECONDS,
  defaultLLMScoringConfig,
  readStoredLLMScoringConfig,
  pickLLMScoringConfigForStorage,
  getLabelColor,
  splitCommandLine,
  riskLevelColor,
  riskMeterColor,
} from "./mlUtils";
import { useAutoTune, type AutoTuneDeps } from "./useAutoTune";
import { useConfigMLDataset } from "./useConfigMLDataset";
import { useMLSampleActions } from "./useMLSampleActions";
import { autoTunePublicApi, mlSampleActionsPublicApi } from "./mlPublicApi";

export interface MLThresholds {
  blockConfidenceThreshold: number;
  mlMinConfidence: number;
  ruleOverridePriority: number;
  lowAnomalyThreshold: number;
  highAnomalyThreshold: number;
}

interface GuardedFetchOptions {
  signal?: AbortSignal;
  isCurrent?: () => boolean;
}

export function useConfigML() {
  // ── ML Status ──
  const mlEnabled = ref(false);
  const modelType = ref<string>("random_forest");
  const builtinModelCatalog = ref<MLBuiltinModelCatalogItem[]>(
    defaultMLBuiltinModelCatalog,
  );
  const selectedBuiltinModel = computed(() =>
    findMLBuiltinModel(builtinModelCatalog.value, modelType.value),
  );
  const modelBaseType = computed(
    () => selectedBuiltinModel.value?.base || modelType.value,
  );
  const cudaAvailable = ref(false);
  const cudaInfo = ref("");
  const cudaMemUsedMB = ref(0);
  const cudaMemTotalMB = ref(0);
  const mlCRuntime = ref<MLCRuntimeStatus | null>(null);
  const cancellingTraining = ref(false);
  const mlStatus = ref<MLStatusState>({
    model_type: "random_forest",
    model_loaded: false,
    num_trees: 0,
    num_samples: 0,
    num_labeled_samples: 0,
    last_trained: "",
    test_accuracy: 0,
    model_path: "",
    training_in_progress: false,
    training_progress: 0,
    train_accuracy: 0,
    validation_accuracy: 0,
    train_samples: 0,
    validation_samples: 0,
    validation_split_ratio: 0.2,
    training_readiness: null,
    llm_review: null,
  });
  const trainingModel = ref(false);
  const feedbackComm = ref("");
  const feedbackAction = ref("accepted");
  const mlThresholds = ref<MLThresholds>({
    blockConfidenceThreshold: 0.85,
    mlMinConfidence: 0.6,
    ruleOverridePriority: 100,
    lowAnomalyThreshold: 0.3,
    highAnomalyThreshold: 0.7,
  });
  const mlTrainingConfig = ref({ validationSplitRatio: 0.2 });
  const llmScoringConfig = ref<MLLlmConfig>({
    ...defaultLLMScoringConfig(),
    ...(readStoredLLMScoringConfig() || {}),
  });
  const llmBatchConfig = ref({
    source: "validation" as "training" | "validation",
    limit: 20,
    onlyUnlabeled: false,
    applyLabels: false,
  });
  const llmBatchResponse = ref<MLLlmBatchResponse | null>(null);
  const llmBatchLoading = ref(false);
  const llmBatchStartedAt = ref(0);
  const llmBatchElapsed = ref("0s");
  const llmBatchProgressTimer = ref<ReturnType<typeof setInterval> | null>(
    null,
  );
  const trainingLogs = ref<{ time: string; message: string }[]>([]);
  const wsActive = ref(false);
  const logPollTimer = ref<ReturnType<typeof setTimeout> | null>(null);
  let logPollActive = false;
  let logPollGeneration = 0;
  let logPollController: AbortController | null = null;
  let logCompletionController: AbortController | null = null;
  let configMLDisposed = false;
  let mlStatusEpoch = 0;
  const llmConfigReady = ref(false);
  const llmConfigApplyingRemote = ref(false);
  const llmConfigSyncTimer = ref<ReturnType<typeof setTimeout> | null>(null);
  const llmConfigSyncPromise = ref<Promise<void> | null>(null);
  const llmConfigSyncInFlight = ref(false);
  const llmConfigSyncQueued = ref(false);
  const llmSaveStatus = ref<"idle" | "saving" | "saved" | "error">("idle");
  const llmStorageTimer = ref<ReturnType<typeof setTimeout> | null>(null);
  const trainingHistory = ref<MLTrainingHistoryEntry[]>([]);
  const hyperParams = ref({ numTrees: 31, maxDepth: 8, minSamplesLeaf: 5 });

  // ── Sample Data ──
  const allSamples = ref<SampleEntry[]>([]);
  const loadingSamples = ref(false);
  const sampleTablePageSize = ref(15);
  const sampleSearchText = ref("");

  // ── Manual Samples ──
  const sampleCommandLine = ref("");
  const sampleLabel = ref("BLOCK");
  const submittingSample = ref(false);

  // ── Backtest ──
  const backtestCommandLine = ref("");
  const backtesting = ref(false);
  const backtestResult = ref<MLCommandSafetyResult | null>(null);

  // ── Dataset Management (extracted composable) ──
  const dataset = useConfigMLDataset({
    allSamples,
    fetchMLStatus: async () => {
      await fetchMLStatus();
    },
    fetchAllSamples: async () => {
      await sampleActions.fetchAllSamples();
    },
    fetchExistingCommandData: async (silent?: boolean) => {
      await dataset.fetchExistingCommandData(silent);
    },
  });

  // ── Helpers ──
  const applyMLStatusData = (data: any) => {
    mlEnabled.value = data.mlEnabled ?? data.ml_enabled ?? false;
    modelType.value = data.modelType ?? data.model_type ?? modelType.value;
    cudaAvailable.value = data.cudaAvailable ?? data.cuda_available ?? false;
    cudaInfo.value = data.cudaInfo ?? data.cuda_info ?? "";
    cudaMemUsedMB.value = data.cudaMemUsedMB ?? data.cuda_mem_used_mb ?? 0;
    cudaMemTotalMB.value = data.cudaMemTotalMB ?? data.cuda_mem_total_mb ?? 0;
    mlCRuntime.value = data.cRuntime ?? data.c_runtime ?? mlCRuntime.value;
    mlStatus.value.model_type = modelType.value;
    mlStatus.value.model_loaded =
      data.modelLoaded ?? data.model_loaded ?? false;
    mlStatus.value.num_trees = data.numTrees ?? data.num_trees ?? 0;
    mlStatus.value.num_samples = data.numSamples ?? data.num_samples ?? 0;
    mlStatus.value.num_labeled_samples =
      data.numLabeledSamples ?? data.num_labeled_samples ?? 0;
    mlStatus.value.last_trained = data.lastTrained ?? data.last_trained ?? "";
    mlStatus.value.test_accuracy = data.testAccuracy ?? data.test_accuracy ?? 0;
    mlStatus.value.model_path = data.modelPath ?? data.model_path ?? "";
    mlStatus.value.training_in_progress =
      data.trainingInProgress ?? data.training_in_progress ?? false;
    mlStatus.value.training_progress =
      data.trainingProgress ?? data.training_progress ?? 0;
    mlStatus.value.train_accuracy =
      data.trainAccuracy ?? data.train_accuracy ?? 0;
    mlStatus.value.validation_accuracy =
      data.validationAccuracy ?? data.validation_accuracy ?? 0;
    mlStatus.value.train_samples = data.trainSamples ?? data.train_samples ?? 0;
    mlStatus.value.validation_samples =
      data.validationSamples ?? data.validation_samples ?? 0;
    mlStatus.value.validation_split_ratio =
      data.validationSplitRatio ??
      data.validation_split_ratio ??
      mlStatus.value.validation_split_ratio ??
      0.2;
    mlStatus.value.training_readiness =
      data.trainingReadiness ?? data.training_readiness ?? null;
    mlStatus.value.llm_review = data.llmReview ?? data.llm_review ?? null;
    const remoteBuiltinModels = data.builtinModels ?? data.builtin_models;
    if (Array.isArray(remoteBuiltinModels) && remoteBuiltinModels.length > 0) {
      builtinModelCatalog.value = remoteBuiltinModels;
    }
    applyAutoTuneStatus(data);

    const mlConfig = data.mlConfig ?? data.ml_config ?? {};
    if (mlConfig) {
      llmConfigApplyingRemote.value = true;
      try {
        if (mlConfig.modelType) modelType.value = mlConfig.modelType;
        mlTrainingConfig.value.validationSplitRatio =
          mlConfig.validationSplitRatio ??
          mlConfig.validation_split_ratio ??
          mlStatus.value.validation_split_ratio ??
          0.2;
        llmScoringConfig.value.enabled =
          mlConfig.llmEnabled ??
          mlConfig.llm_enabled ??
          llmScoringConfig.value.enabled;
        llmScoringConfig.value.baseUrl =
          mlConfig.llmBaseUrl ??
          mlConfig.llm_base_url ??
          llmScoringConfig.value.baseUrl;
        llmScoringConfig.value.apiKeyConfigured =
          mlConfig.llmApiKeyConfigured ??
          mlConfig.llm_api_key_configured ??
          llmScoringConfig.value.apiKeyConfigured;
        llmScoringConfig.value.model =
          mlConfig.llmModel ??
          mlConfig.llm_model ??
          llmScoringConfig.value.model;
        llmScoringConfig.value.timeoutSeconds =
          mlConfig.llmTimeoutSeconds ??
          mlConfig.llm_timeout_seconds ??
          llmScoringConfig.value.timeoutSeconds;
        llmScoringConfig.value.temperature =
          mlConfig.llmTemperature ??
          mlConfig.llm_temperature ??
          llmScoringConfig.value.temperature;
        llmScoringConfig.value.maxTokens =
          mlConfig.llmMaxTokens ??
          mlConfig.llm_max_tokens ??
          llmScoringConfig.value.maxTokens;
        llmScoringConfig.value.systemPrompt =
          mlConfig.llmSystemPrompt ??
          mlConfig.llm_system_prompt ??
          llmScoringConfig.value.systemPrompt;
        applyStoredLLMScoringConfig();
      } finally {
        llmConfigApplyingRemote.value = false;
      }
    }
    if (Array.isArray(data.trainingLogs)) {
      trainingLogs.value = data.trainingLogs;
    }
  };

  const applyMLStatusResponse = (data: any) => {
    if (configMLDisposed) return;
    mlStatusEpoch++;
    applyMLStatusData(data);
  };

  const fetchAndApplyMLStatus = async (
    options: GuardedFetchOptions = {},
  ): Promise<any | null> => {
    const statusEpoch = ++mlStatusEpoch;
    const isCurrent = () =>
      !configMLDisposed &&
      statusEpoch === mlStatusEpoch &&
      !options.signal?.aborted &&
      (options.isCurrent?.() ?? true);
    try {
      const res = await axios.get("/config/ml/status", {
        signal: options.signal,
      });
      if (!isCurrent()) return null;
      applyMLStatusData(res.data);
      return res.data;
    } catch (_) {
      return null;
    }
  };

  const stopLogPolling = () => {
    logPollActive = false;
    logPollGeneration++;
    logPollController?.abort();
    logPollController = null;
    logCompletionController?.abort();
    logCompletionController = null;
    if (logPollTimer.value !== null) {
      clearTimeout(logPollTimer.value);
      logPollTimer.value = null;
    }
  };

  const runLogPoll = async (generation: number) => {
    if (
      !logPollActive ||
      configMLDisposed ||
      wsActive.value ||
      generation !== logPollGeneration
    )
      return;
    logPollTimer.value = null;
    const controller = new AbortController();
    logPollController?.abort();
    logPollController = controller;
    try {
      const wasRunning = mlStatus.value.training_in_progress;
      const data = await fetchAndApplyMLStatus({
        signal: controller.signal,
        isCurrent: () =>
          logPollActive && !wsActive.value && generation === logPollGeneration,
      });
      if (data && wasRunning && !mlStatus.value.training_in_progress) {
        stopLogPolling();
        const completionGeneration = logPollGeneration;
        if (configMLDisposed) return;
        const completionController = new AbortController();
        logCompletionController = completionController;
        const completionIsCurrent = () =>
          !configMLDisposed && completionGeneration === logPollGeneration;
        try {
          await fetchMLStatus({
            signal: completionController.signal,
            isCurrent: completionIsCurrent,
          });
          if (!completionIsCurrent()) return;
          await sampleActions.fetchAllSamples({
            signal: completionController.signal,
            isCurrent: completionIsCurrent,
          });
        } finally {
          if (logCompletionController === completionController) {
            logCompletionController = null;
          }
        }
        return;
      }
    } catch (_) {
      if (controller.signal.aborted) return;
    } finally {
      if (logPollController === controller) logPollController = null;
    }
    if (
      logPollActive &&
      !configMLDisposed &&
      !wsActive.value &&
      generation === logPollGeneration
    ) {
      logPollTimer.value = setTimeout(() => void runLogPoll(generation), 1000);
    }
  };

  const startLogPolling = () => {
    if (wsActive.value || logPollActive || configMLDisposed) return;
    logCompletionController?.abort();
    logCompletionController = null;
    logPollActive = true;
    const generation = ++logPollGeneration;
    logPollTimer.value = setTimeout(() => void runLogPoll(generation), 1000);
  };

  watch(wsActive, (active) => {
    if (active) {
      stopLogPolling();
    } else if (trainingModel.value || mlStatus.value.training_in_progress) {
      startLogPolling();
    }
  });

  const fetchMLStatus = async (options: GuardedFetchOptions = {}) => {
    const isCurrent = () =>
      !configMLDisposed &&
      !options.signal?.aborted &&
      (options.isCurrent?.() ?? true);
    let fetchedOk = false;
    try {
      const data = await fetchAndApplyMLStatus(options);
      if (!data || !isCurrent()) return;
      if (data.blockConfidenceThreshold !== undefined) {
        mlThresholds.value.blockConfidenceThreshold =
          data.blockConfidenceThreshold ?? 0.85;
        mlThresholds.value.mlMinConfidence = data.mlMinConfidence ?? 0.6;
        mlThresholds.value.ruleOverridePriority =
          data.ruleOverridePriority ?? 100;
        mlThresholds.value.lowAnomalyThreshold =
          data.lowAnomalyThreshold ?? 0.3;
        mlThresholds.value.highAnomalyThreshold =
          data.highAnomalyThreshold ?? 0.7;
      }
      if (data.hyperParams) {
        hyperParams.value.numTrees = data.hyperParams.numTrees ?? 31;
        hyperParams.value.maxDepth = data.hyperParams.maxDepth ?? 8;
        hyperParams.value.minSamplesLeaf = data.hyperParams.minSamplesLeaf ?? 5;
      }
      await fetchTrainingHistory(options);
      if (!isCurrent()) return;
      fetchedOk = true;
    } catch (_) {
    } finally {
      if (isCurrent()) {
        if (!llmConfigReady.value) {
          llmConfigReady.value = true;
        }
        if (fetchedOk) {
          queueLLMScoringConfigAutosave();
        }
      }
    }
  };

  // ── Auto-Tune ──
  const _atDeps: AutoTuneDeps = {
    modelType,
    builtinModelCatalog,
    modelBaseType,
    mlTrainingConfig,
    hyperParams,
    wsActive,
    fetchMLStatus,
    fetchMLStatusData: fetchAndApplyMLStatus,
  };
  const autoTune = useAutoTune(_atDeps);
  const {
    autoTuneMode,
    modelTuneSelectedTypes,
    modelTuneParamSearch,
    modelTuneApplyBest,
    modelTuneResponse,
    modelTuneBest,
    modelTuneRecommendedTypes,
    autoTuneXAxis,
    autoTuneYAxis,
    autoTuneGridSize,
    autoTuneGranularity,
    autoTuneMetric,
    autoTuneMinX,
    autoTuneMaxX,
    autoTuneMinY,
    autoTuneMaxY,
    autoTuneLoading,
    autoTuneInProgress,
    autoTuneProgress,
    autoTuneCompleted,
    autoTuneTotal,
    autoTuneMessage,
    autoTuneError,
    autoTuneJobId,
    autoTuneResponse,
    autoTuneSelectedCell,
    autoTuneAxisOptions,
    autoTuneAxisLabel,
    autoTuneMetricLabel,
    autoTuneMetricFormat,
    autoTuneGranularityLabel,
    autoTuneScore,
    autoTuneHeatmapOptions,
    autoTuneHeatmapSeries,
    autoTuneBestCell,
    runAutoTune,
    runModelTune,
    applyModelTuneBest,
    applyAutoTuneCell,
    applyAutoTuneStatus,
    stopAutoTunePolling,
  } = autoTune;

  const fetchTrainingHistory = async (options: GuardedFetchOptions = {}) => {
    try {
      const res = await axios.get("/config/ml/history", {
        signal: options.signal,
      });
      if (
        configMLDisposed ||
        options.signal?.aborted ||
        !(options.isCurrent?.() ?? true)
      )
        return;
      trainingHistory.value = res.data.history || [];
    } catch (_) {}
  };

  const trainingChartOptions = computed(() => ({
    chart: {
      type: "line" as const,
      height: 280,
      toolbar: { show: false },
      animations: { enabled: true },
    },
    stroke: { curve: "smooth" as const, width: 2 },
    xaxis: { type: "datetime" as const, labels: { format: "HH:mm" } },
    yaxis: [
      {
        title: { text: "Accuracy" },
        min: 0,
        max: 1,
        labels: { formatter: (v: number) => (v * 100).toFixed(0) + "%" },
      },
      {
        seriesName: "Samples",
        opposite: true,
        title: { text: "Samples" },
        min: 0,
      },
    ],
    tooltip: { x: { format: "yyyy-MM-dd HH:mm" } },
    legend: { position: "top" as const },
    colors: ["#52c41a", "#1890ff", "#faad14"],
  }));

  const trainingChartSeries = computed(() => {
    if (!trainingHistory.value.length) return [];
    return [
      {
        name: "Train Accuracy",
        type: "line",
        data: trainingHistory.value.map((h) => ({
          x: new Date(h.timestamp).getTime(),
          y: h.trainAccuracy ?? h.accuracy,
        })),
      },
      {
        name: "Validation Accuracy",
        type: "line",
        data: trainingHistory.value.map((h) => ({
          x: new Date(h.timestamp).getTime(),
          y: h.validationAccuracy ?? h.accuracy,
        })),
      },
      {
        name: "Samples",
        type: "line",
        data: trainingHistory.value.map((h) => ({
          x: new Date(h.timestamp).getTime(),
          y: h.numSamples,
        })),
      },
    ];
  });

  const boundedLLMTimeoutSeconds = () =>
    Math.min(
      Math.max(llmScoringConfig.value.timeoutSeconds || 45, 5),
      MAX_LLM_TIMEOUT_SECONDS,
    );

  const buildThresholdRuntimePayload = () => {
    const payload: Record<string, any> = {
      enabled: true,
      modelType: modelType.value,
      blockConfidenceThreshold: mlThresholds.value.blockConfidenceThreshold,
      mlMinConfidence: mlThresholds.value.mlMinConfidence,
      ruleOverridePriority: mlThresholds.value.ruleOverridePriority,
      lowAnomalyThreshold: mlThresholds.value.lowAnomalyThreshold,
      highAnomalyThreshold: mlThresholds.value.highAnomalyThreshold,
      modelPath: mlStatus.value.model_path || "",
      autoTrain: true,
      trainInterval: "24h",
      minSamplesForTraining: 1000,
      activeLearningEnabled: false,
      featureHistorySize: 100,
      numTrees: hyperParams.value.numTrees,
      maxDepth: hyperParams.value.maxDepth,
      minSamplesLeaf: hyperParams.value.minSamplesLeaf,
      validationSplitRatio: mlTrainingConfig.value.validationSplitRatio,
      llmEnabled: llmScoringConfig.value.enabled,
      llmBaseUrl: llmScoringConfig.value.baseUrl,
      llmModel: llmScoringConfig.value.model,
      llmTimeoutSeconds: boundedLLMTimeoutSeconds(),
      llmTemperature: llmScoringConfig.value.temperature,
      llmMaxTokens: llmScoringConfig.value.maxTokens,
      llmSystemPrompt: llmScoringConfig.value.systemPrompt,
    };
    const apiKey = llmScoringConfig.value.apiKey.trim();
    if (apiKey) {
      payload.llmApiKey = apiKey;
    }
    return payload;
  };

  const buildLLMRuntimePayload = () => {
    const payload: Record<string, any> = {
      llmEnabled: llmScoringConfig.value.enabled,
      llmBaseUrl: llmScoringConfig.value.baseUrl,
      llmModel: llmScoringConfig.value.model,
      llmTimeoutSeconds: boundedLLMTimeoutSeconds(),
      llmTemperature: llmScoringConfig.value.temperature,
      llmMaxTokens: llmScoringConfig.value.maxTokens,
      llmSystemPrompt: llmScoringConfig.value.systemPrompt,
    };
    const apiKey = llmScoringConfig.value.apiKey.trim();
    if (apiKey) {
      payload.llmApiKey = apiKey;
    }
    return payload;
  };

  const persistLLMScoringConfigToStorage = () => {
    if (typeof window === "undefined") return;
    try {
      window.localStorage.setItem(
        LLM_SCORING_STORAGE_KEY,
        JSON.stringify(pickLLMScoringConfigForStorage(llmScoringConfig.value)),
      );
    } catch (_) {}
  };

  const applyStoredLLMScoringConfig = () => {
    const stored = readStoredLLMScoringConfig();
    if (!stored) return false;
    if (stored.enabled !== undefined)
      llmScoringConfig.value.enabled = stored.enabled;
    if (stored.baseUrl !== undefined)
      llmScoringConfig.value.baseUrl = stored.baseUrl;
    if (stored.apiKey !== undefined)
      llmScoringConfig.value.apiKey = stored.apiKey;
    if (stored.model !== undefined) llmScoringConfig.value.model = stored.model;
    if (stored.timeoutSeconds !== undefined)
      llmScoringConfig.value.timeoutSeconds = stored.timeoutSeconds;
    if (stored.temperature !== undefined)
      llmScoringConfig.value.temperature = stored.temperature;
    if (stored.maxTokens !== undefined)
      llmScoringConfig.value.maxTokens = stored.maxTokens;
    if (stored.systemPrompt !== undefined)
      llmScoringConfig.value.systemPrompt = stored.systemPrompt;
    return true;
  };

  const syncLLMScoringConfigToBackend = async () => {
    if (llmConfigSyncPromise.value) {
      llmConfigSyncQueued.value = true;
      return llmConfigSyncPromise.value;
    }
    if (llmConfigSyncTimer.value) {
      clearTimeout(llmConfigSyncTimer.value);
      llmConfigSyncTimer.value = null;
    }
    llmConfigSyncInFlight.value = true;
    llmSaveStatus.value = "saving";
    const runSync = async () => {
      try {
        do {
          llmConfigSyncQueued.value = false;
          await axios.put("/config/runtime", buildLLMRuntimePayload());
        } while (llmConfigSyncQueued.value);
        llmSaveStatus.value = "saved";
        setTimeout(() => {
          if (llmSaveStatus.value === "saved") llmSaveStatus.value = "idle";
        }, 2000);
      } catch (e: any) {
        llmSaveStatus.value = "error";
        message.error(e.response?.data?.error || "LLM 配置保存失败");
      } finally {
        llmConfigSyncInFlight.value = false;
        llmConfigSyncPromise.value = null;
      }
    };
    llmConfigSyncPromise.value = runSync();
    return llmConfigSyncPromise.value;
  };

  const queueLLMScoringConfigAutosave = () => {
    if (!llmConfigReady.value || llmConfigApplyingRemote.value) return;
    // Debounce localStorage write (300ms to avoid writing on every keystroke)
    if (llmStorageTimer.value) clearTimeout(llmStorageTimer.value);
    llmStorageTimer.value = setTimeout(() => {
      llmStorageTimer.value = null;
      persistLLMScoringConfigToStorage();
    }, 300);
    // Debounce backend sync (600ms)
    if (llmConfigSyncTimer.value) clearTimeout(llmConfigSyncTimer.value);
    llmConfigSyncTimer.value = setTimeout(() => {
      llmConfigSyncTimer.value = null;
      void syncLLMScoringConfigToBackend();
    }, 600);
  };

  const saveLLMConfigNow = async () => {
    // Flush debounce timers immediately
    if (llmStorageTimer.value) {
      clearTimeout(llmStorageTimer.value);
      llmStorageTimer.value = null;
    }
    persistLLMScoringConfigToStorage();
    if (llmConfigSyncTimer.value) {
      clearTimeout(llmConfigSyncTimer.value);
      llmConfigSyncTimer.value = null;
    }
    await syncLLMScoringConfigToBackend();
  };

  const flushLLMScoringConfigAutosave = async () => {
    if (llmStorageTimer.value) {
      clearTimeout(llmStorageTimer.value);
      llmStorageTimer.value = null;
    }
    persistLLMScoringConfigToStorage();
    if (llmConfigSyncTimer.value) {
      clearTimeout(llmConfigSyncTimer.value);
      llmConfigSyncTimer.value = null;
    }
    await syncLLMScoringConfigToBackend();
  };

  const persistRuntimeMLConfig = async (payload: Record<string, any>) => {
    const res = await axios.put("/config/runtime", payload);
    try {
      await fetchMLStatus();
    } catch (_) {}
    return res.data;
  };

  const submitFeedback = async () => {
    if (!feedbackComm.value) return;
    try {
      const res = await axios.post("/config/ml/feedback", {
        comm: feedbackComm.value,
        userAction: feedbackAction.value,
      });
      message.success(`Feedback applied: ${res.data.matched} samples labeled`);
      feedbackComm.value = "";
      await fetchMLStatus();
    } catch (_: any) {
      message.error("Failed to submit feedback");
    }
  };

  const saveMLThresholds = async () => {
    try {
      await persistRuntimeMLConfig(buildThresholdRuntimePayload());
      message.success("ML settings saved");
    } catch (_) {
      message.error("Failed to save thresholds");
    }
  };

  const applySelectedModelDefaults = () => {
    const defaults = selectedBuiltinModel.value?.defaults;
    if (!defaults) return;
    hyperParams.value.numTrees =
      defaults.numTrees ?? hyperParams.value.numTrees;
    hyperParams.value.maxDepth =
      defaults.maxDepth ?? hyperParams.value.maxDepth;
    hyperParams.value.minSamplesLeaf =
      defaults.minSamplesLeaf ?? hyperParams.value.minSamplesLeaf;
  };

  const saveMLModelType = async () => {
    applySelectedModelDefaults();
    await saveMLThresholds();
  };

  watch(
    llmScoringConfig,
    () => {
      if (llmConfigApplyingRemote.value) return;
      persistLLMScoringConfigToStorage();
      if (!llmConfigReady.value) return;
      queueLLMScoringConfigAutosave();
    },
    { deep: true },
  );

  watch(
    () => llmBatchConfig.value.source,
    (source) => {
      if (source !== "training") llmBatchConfig.value.applyLabels = false;
    },
  );

  const llmBatchCanApplyLabels = computed(
    () => llmBatchConfig.value.source === "training",
  );
  const boundedLLMBatchLimit = () =>
    Math.min(
      Math.max(1, llmBatchConfig.value.limit || 20),
      MAX_LLM_BATCH_SCORE_LIMIT,
    );

  const llmBatchPreviewSubjects = computed(() => {
    const limit = boundedLLMBatchLimit();
    const candidates = allSamples.value.filter((sample) => {
      if (!llmBatchConfig.value.onlyUnlabeled) return true;
      return !sample.label || sample.label === "-";
    });
    return candidates.slice(0, limit);
  });

  const llmBatchProgress = computed(() => {
    const total =
      llmBatchResponse.value?.total ||
      llmBatchPreviewSubjects.value.length ||
      llmBatchConfig.value.limit ||
      0;
    const scored = llmBatchResponse.value?.scored || 0;
    return {
      total,
      scored,
      percent: total > 0 ? Math.round((scored / total) * 100) : 0,
    };
  });

  const stopLLMBatchProgressTimer = () => {
    if (llmBatchProgressTimer.value) {
      clearInterval(llmBatchProgressTimer.value);
      llmBatchProgressTimer.value = null;
    }
  };

  const startLLMBatchProgressTimer = () => {
    stopLLMBatchProgressTimer();
    llmBatchStartedAt.value = Date.now();
    llmBatchElapsed.value = "0s";
    llmBatchProgressTimer.value = setInterval(() => {
      const sec = Math.floor((Date.now() - llmBatchStartedAt.value) / 1000);
      llmBatchElapsed.value =
        sec < 60 ? `${sec}s` : `${Math.floor(sec / 60)}m${sec % 60}s`;
    }, 1000);
  };

  const runLLMBatchScore = async () => {
    llmBatchLoading.value = true;
    llmBatchResponse.value = null;
    startLLMBatchProgressTimer();
    try {
      try {
        await flushLLMScoringConfigAutosave();
      } catch (e: any) {
        message.error(
          e.response?.data?.error ||
            "LLM 配置自动保存失败，请先检查 Base URL / Model / API Key",
        );
        return;
      }
      const res = await axios.post<MLLlmBatchResponse>(
        "/config/ml/llm/batch-score",
        {
          source: llmBatchConfig.value.source,
          limit: boundedLLMBatchLimit(),
          onlyUnlabeled: llmBatchConfig.value.onlyUnlabeled,
          applyLabels:
            llmBatchConfig.value.applyLabels && llmBatchCanApplyLabels.value,
        },
      );
      llmBatchResponse.value = res.data;
      if (res.data.review) mlStatus.value.llm_review = res.data.review;
      if (res.data.applied > 0) {
        await fetchMLStatus();
        await sampleActions.fetchAllSamples();
      }
      message.success(
        `LLM 打分完成：${res.data.scored}/${res.data.total}，平均风险 ${(res.data.averageRiskScore ?? 0).toFixed(1)}`,
      );
    } catch (e: any) {
      message.error(e.response?.data?.error || "LLM 批量打分失败");
    } finally {
      llmBatchLoading.value = false;
      stopLLMBatchProgressTimer();
    }
  };

  const llmBatchRowKey = (record: MLLlmBatchEntry) =>
    record.index !== undefined
      ? String(record.index)
      : `${record.commandLine}:${record.recommendedAction}:${record.riskScore}:${record.confidence}`;

  const sampleActions = useMLSampleActions({
    allSamples,
    loadingSamples,
    sampleSearchText,
    sampleCommandLine,
    sampleLabel,
    submittingSample,
    backtestCommandLine,
    backtesting,
    backtestResult,
    cancellingTraining,
    trainingModel,
    trainingLogs,
    hyperParams,
    llmScoringConfig,
    dataset,
    fetchMLStatus,
    saveMLThresholds,
    startLogPolling,
    stopLogPolling,
    splitCommandLine,
    isDisposed: () => configMLDisposed,
  });
  onUnmounted(() => {
    configMLDisposed = true;
    sampleActions.dispose();
    stopLogPolling();
    stopAutoTunePolling();
    stopLLMBatchProgressTimer();
    if (llmStorageTimer.value) {
      clearTimeout(llmStorageTimer.value);
      llmStorageTimer.value = null;
    }
    if (llmConfigSyncTimer.value) {
      clearTimeout(llmConfigSyncTimer.value);
      llmConfigSyncTimer.value = null;
    }
  });

  return {
    mlEnabled,
    mlStatus,
    trainingModel,
    feedbackComm,
    feedbackAction,
    mlThresholds,
    mlTrainingConfig,
    llmScoringConfig,
    llmBatchConfig,
    llmBatchResponse,
    llmBatchLoading,
    llmBatchElapsed,
    llmBatchPreviewSubjects,
    llmBatchProgress,
    trainingLogs,
    wsActive,
    logPollTimer,
    llmSaveStatus,
    saveLLMConfigNow,
    modelType,
    builtinModelCatalog,
    selectedBuiltinModel,
    modelBaseType,
    ...autoTunePublicApi(autoTune),
    autoTuneAxisOptions,
    cudaAvailable,
    cudaInfo,
    cudaMemUsedMB,
    cudaMemTotalMB,
    mlCRuntime,
    cancellingTraining,
    trainingHistory,
    hyperParams,
    allSamples,
    loadingSamples,
    sampleTablePageSize,
    sampleSearchText,
    sampleCommandLine,
    sampleLabel,
    submittingSample,
    backtestCommandLine,
    backtesting,
    backtestResult,
    applyMLStatusResponse,
    startLogPolling,
    stopLogPolling,
    fetchMLStatus,
    fetchTrainingHistory,
    trainingChartOptions,
    trainingChartSeries,
    submitFeedback,
    saveMLThresholds,
    saveMLModelType,
    applySelectedModelDefaults,
    runLLMBatchScore,
    llmBatchRowKey,
    llmBatchCanApplyLabels,
    getLabelColor,
    splitCommandLine,
    importAllSafetyNetPresets: async () => {
      await dataset.importPresetBatch(
        safetyNetHighRiskPresets,
        "Safety Net 预设",
      );
    },
    riskLevelColor,
    riskMeterColor,
    ...mlSampleActionsPublicApi(sampleActions),
    // Dataset management (from extracted composable)
    ...dataset,
  };
}
