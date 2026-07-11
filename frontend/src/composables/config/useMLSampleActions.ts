import { computed, type Ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";
import type {
  MLCommandSafetyResult,
  MLLlmConfig,
  SampleEntry,
} from "../../types/config";
import {
  highRiskPresets,
  safetyNetHighRiskPresets,
  syntheticExpansionPresets,
} from "./mlPresets";
import type { useConfigMLDataset } from "./useConfigMLDataset";

interface GuardedFetchOptions {
  signal?: AbortSignal;
  isCurrent?: () => boolean;
}

export function useMLSampleActions(options: {
  allSamples: Ref<SampleEntry[]>;
  loadingSamples: Ref<boolean>;
  sampleSearchText: Ref<string>;
  sampleCommandLine: Ref<string>;
  sampleLabel: Ref<string>;
  submittingSample: Ref<boolean>;
  backtestCommandLine: Ref<string>;
  backtesting: Ref<boolean>;
  backtestResult: Ref<MLCommandSafetyResult | null>;
  cancellingTraining: Ref<boolean>;
  trainingModel: Ref<boolean>;
  trainingLogs: Ref<{ time: string; message: string }[]>;
  hyperParams: Ref<{ numTrees: number; maxDepth: number; minSamplesLeaf: number }>;
  llmScoringConfig: Ref<MLLlmConfig>;
  dataset: ReturnType<typeof useConfigMLDataset>;
  fetchMLStatus: () => Promise<unknown>;
  saveMLThresholds: () => Promise<unknown>;
  startLogPolling: () => void;
  stopLogPolling: () => void;
  splitCommandLine: (commandLine: string) => string[];
  isDisposed: () => boolean;
}) {
  const {
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
    isDisposed,
  } = options;
  let samplesFetchGeneration = 0;

  // ── Sample CRUD ──
  const filteredSamples = computed(() => {
    if (!sampleSearchText.value.trim()) return allSamples.value;
    const search = sampleSearchText.value.toLowerCase();
    return allSamples.value.filter(
      (s) =>
        (s.commandLine || "").toLowerCase().includes(search) ||
        s.comm.toLowerCase().includes(search) ||
        (s.args || []).join(" ").toLowerCase().includes(search),
    );
  });

  const existingDuplicateCount = computed(
    () =>
      dataset.existingCommandCandidates.value.filter(
        (item: any) => item.duplicate,
      ).length,
  );
  const importableExistingCount = computed(
    () =>
      dataset.existingCommandCandidates.value.length -
      existingDuplicateCount.value,
  );

  const fetchAllSamples = async (options: GuardedFetchOptions = {}) => {
    const generation = ++samplesFetchGeneration;
    const isCurrent = () =>
      !isDisposed() &&
      generation === samplesFetchGeneration &&
      !options.signal?.aborted &&
      (options.isCurrent?.() ?? true);
    loadingSamples.value = true;
    try {
      const res = await axios.get("/config/ml/samples", {
        signal: options.signal,
      });
      if (!isCurrent()) return;
      allSamples.value = res.data.samples || [];
    } catch (_) {
    } finally {
      if (generation === samplesFetchGeneration) {
        loadingSamples.value = false;
      }
    }
  };

  const labelSample = async (index: number, label: string) => {
    try {
      await axios.put("/config/ml/samples/label", { index, label });
      const entry = allSamples.value.find((s) => s.index === index);
      if (entry) {
        entry.label = label;
        entry.userLabel = "manual-index";
      }
      message.success(`Sample #${index} labeled as ${label}`);
    } catch (_: any) {
      message.error("Failed to label sample");
    }
  };

  const deleteSample = async (index: number) => {
    try {
      await axios.delete(`/config/ml/samples/${index}`);
      allSamples.value = allSamples.value.filter((s) => s.index !== index);
      message.success(`Sample #${index} deleted`);
      await fetchMLStatus();
    } catch (_: any) {
      message.error("Failed to delete sample");
    }
  };

  const updateAnomaly = async (index: number, anomalyScore: number) => {
    try {
      await axios.put("/config/ml/samples/anomaly", { index, anomalyScore });
    } catch (_: any) {
      message.error("Failed to update anomaly score");
    }
  };

  const cancelTraining = async () => {
    cancellingTraining.value = true;
    try {
      await axios.post("/config/ml/train/cancel");
      message.info("已发送中止请求");
    } catch (e: any) {
      message.error(e.response?.data?.error || "取消失败");
    } finally {
      cancellingTraining.value = false;
    }
  };

  const trainWithParams = async () => {
    trainingModel.value = true;
    trainingLogs.value = [];
    try {
      await saveMLThresholds();
      startLogPolling();
      const res = await axios.post("/config/ml/train", {
        numTrees: hyperParams.value.numTrees,
        maxDepth: hyperParams.value.maxDepth,
        minSamplesLeaf: hyperParams.value.minSamplesLeaf,
      });
      message.success(
        `Model trained: accuracy=${(res.data.accuracy * 100).toFixed(1)}%, ${res.data.numTrees} trees`,
      );
      await fetchMLStatus();
      await fetchAllSamples();
    } catch (e: any) {
      message.error(e.response?.data?.error || "Training failed");
    } finally {
      trainingModel.value = false;
      stopLogPolling();
    }
  };

  // ── Manual Sample Submission ──
  const submitManualSample = async () => {
    if (!sampleCommandLine.value.trim()) return;
    const commands = sampleCommandLine.value
      .trim()
      .split("|")
      .map((c) => c.trim())
      .filter((c) => c);
    if (commands.length === 0) return;
    submittingSample.value = true;
    let addedCount = 0;
    try {
      for (const cmdStr of commands) {
        const parts = splitCommandLine(cmdStr);
        if (parts.length === 0) continue;
        const comm = parts[0],
          args = parts.slice(1),
          argsStr = args.join(" ");
        const duplicate = allSamples.value.find(
          (s) => s.comm === comm && (s.args || []).join(" ") === argsStr,
        );
        if (duplicate) {
          message.warning(`样本已存在：${comm} (Index #${duplicate.index})`);
          continue;
        }
        await axios.post("/config/ml/samples", {
          commandLine: cmdStr,
          comm,
          args,
          label: sampleLabel.value,
        });
        addedCount++;
      }
      if (addedCount > 0) {
        message.success(`已添加 ${addedCount} 个样本 → ${sampleLabel.value}`);
        sampleCommandLine.value = "";
        await fetchMLStatus();
        await fetchAllSamples();
      }
    } catch (e: any) {
      message.error(e.response?.data?.error || "Failed to add sample");
    } finally {
      submittingSample.value = false;
    }
  };

  const addPresetSample = async (preset: {
    comm: string;
    args: string;
    label: string;
  }) => {
    const argsArray = preset.args ? splitCommandLine(preset.args) : [];
    const argsStr = argsArray.join(" ");
    const duplicate = allSamples.value.find(
      (s) => s.comm === preset.comm && (s.args || []).join(" ") === argsStr,
    );
    if (duplicate) {
      message.warning(`样本已存在：${preset.comm} (Index #${duplicate.index})`);
      return;
    }
    try {
      const commandLine = [preset.comm, preset.args]
        .filter((part) => part && part.trim())
        .join(" ");
      await axios.post("/config/ml/samples", {
        commandLine,
        comm: preset.comm,
        args: argsArray,
        label: preset.label,
      });
      message.success(`Preset added: ${preset.comm} → ${preset.label}`);
      await fetchMLStatus();
      await fetchAllSamples();
    } catch (_: any) {
      message.error("Failed to add preset");
    }
  };

  const importAllHighRiskPresets = async () => {
    await dataset.importPresetBatch(highRiskPresets, "高危行为预设");
  };

  const importAllSyntheticPresets = async () => {
    await dataset.importPresetBatch(syntheticExpansionPresets, "合成扩增样本");
  };

  const importExpandedTrainingCorpus = async () => {
    await dataset.importPresetBatch(highRiskPresets, "高危行为预设");
    await dataset.importPresetBatch(
      safetyNetHighRiskPresets,
      "Safety Net 预设",
    );
    await dataset.importPresetBatch(syntheticExpansionPresets, "合成扩增样本");
    await dataset.importSELinuxPolicyDataset();
    await dataset.importAllInternetDatasets();
    await trainWithParams();
  };

  // ── Command Safety Assessment ──
  const runBacktest = async () => {
    if (!backtestCommandLine.value.trim()) return;
    backtesting.value = true;
    backtestResult.value = null;
    try {
      backtestResult.value = (
        await axios.post("/config/ml/assess", {
          commandLine: backtestCommandLine.value,
        })
      ).data;
    } catch (e: any) {
      message.error(e.response?.data?.error || "命令安全性判断失败");
    } finally {
      backtesting.value = false;
    }
  };

  const runBacktestPreset = async (comm: string, argsStr: string) => {
    backtestCommandLine.value = `${comm} ${argsStr || ""}`.trim();
    await runBacktest();
  };

  const llmApiKeyStatus = computed(() => {
    if (llmScoringConfig.value.apiKey.trim()) {
      return { text: "Key 已自动保存", color: "green" };
    }
    if (llmScoringConfig.value.apiKeyConfigured) {
      return { text: "Key 已配置", color: "green" };
    }
    return { text: "Key 未配置", color: "default" };
  });

  const dispose = () => {
    samplesFetchGeneration++;
  };

  return {
    filteredSamples,
    existingDuplicateCount,
    importableExistingCount,
    fetchAllSamples,
    labelSample,
    deleteSample,
    updateAnomaly,
    cancelTraining,
    trainWithParams,
    submitManualSample,
    addPresetSample,
    importAllHighRiskPresets,
    importAllSyntheticPresets,
    importExpandedTrainingCorpus,
    runBacktest,
    runBacktestPreset,
    llmApiKeyStatus,
    dispose,
  };
}
