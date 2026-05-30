import { ref } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';

import type {
  ExistingCommandCandidate, RemoteDatasetRow, RemoteDatasetResponse,
  LLMProductionDatasetResponse, LLMProductionDatasetRow,
  ClassicSecurityDatasetPreset,
} from '../../types/config';

import { classicSecurityDatasetPresets } from './mlPresets';
import type { TrainingPreset } from './mlPresets';
import { downloadJsonFile, downloadTextFile, buildLLMProductionJsonl, arrayBufferToBase64, splitCommandLine } from './mlUtils';

export function useConfigMLDataset(deps: {
  allSamples: ReturnType<typeof ref<Array<{ comm: string; args?: string[]; index?: number }>>>;
  fetchMLStatus: () => Promise<void>;
  fetchAllSamples: () => Promise<void>;
  fetchExistingCommandData: (silent?: boolean) => Promise<void>;
}) {
  // ── Existing Command Data ──
  const existingDataLimit = ref(200);
  const existingLabelMode = ref<'unlabeled' | 'heuristic'>('unlabeled');
  const existingCommandCandidates = ref<ExistingCommandCandidate[]>([]);
  const loadingExistingData = ref(false);
  const importingExistingData = ref(false);
  const existingDataSource = ref('');

  // ── Remote Dataset ──
  const remoteDatasetUrl = ref('');
  const remoteDatasetFormat = ref<'auto' | 'json' | 'jsonl' | 'csv' | 'tsv' | 'text'>('auto');
  const remoteDatasetLabelMode = ref<'preserve' | 'unlabeled' | 'heuristic'>('preserve');
  const remoteDatasetCleanSensitive = ref(true);
  const remoteDatasetLimit = ref(200);
  const loadingRemoteDataset = ref(false);
  const importingRemoteDataset = ref(false);
  const remoteDatasetPreview = ref<RemoteDatasetRow[]>([]);
  const remoteDatasetMeta = ref<RemoteDatasetResponse | null>(null);

  // ── LLM Production Dataset ──
  const llmProductionDatasetLimit = ref(500);
  const llmProductionAllowHeuristic = ref(false);
  const llmProductionDeduplicate = ref(true);
  const llmProductionLoading = ref(false);
  const llmProductionPreview = ref<LLMProductionDatasetRow[]>([]);
  const llmProductionMeta = ref<LLMProductionDatasetResponse | null>(null);

  // ── Import State ──
  const trainingDatasetImportInput = ref<HTMLInputElement | null>(null);
  const importingClassicDataset = ref(false);
  const dataMaskEnabled = ref(false);

  // ── Helpers ──
  const refreshTrainingDatasetViews = async () => {
    await deps.fetchMLStatus();
    await deps.fetchAllSamples();
    await deps.fetchExistingCommandData(true);
  };

  const resolveDatasetUrl = (input: string) => {
    const trimmed = input.trim();
    if (!trimmed) return '';
    if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed) || trimmed.startsWith('//')) {
      return trimmed;
    }
    if (trimmed.startsWith('/') || trimmed.startsWith('./') || trimmed.startsWith('../')) {
      return new URL(trimmed, window.location.origin).toString();
    }
    return trimmed;
  };

  const fetchExistingCommandData = async (silent = false) => {
    loadingExistingData.value = true;
    try {
      const res = await axios.get('/config/ml/existing-commands', { params: { limit: existingDataLimit.value } });
      existingCommandCandidates.value = res.data.candidates || [];
      existingDataSource.value = res.data.source || '';
      if (!silent) message.success(`拉取到 ${existingCommandCandidates.value.length} 条历史命令数据`);
    } catch (e: any) {
      message.error(e.response?.data?.error || '拉取已有命令数据失败');
    } finally { loadingExistingData.value = false; }
  };

  const importExistingCommandData = async () => {
    importingExistingData.value = true;
    try {
      const res = await axios.post('/config/ml/import-existing', { limit: existingDataLimit.value, labelMode: existingLabelMode.value });
      message.success(`导入 ${res.data.imported} 条，跳过 ${res.data.skipped} 条重复/无效数据`);
      await deps.fetchMLStatus(); await deps.fetchAllSamples(); await fetchExistingCommandData(true);
    } catch (e: any) {
      message.error(e.response?.data?.error || '导入已有命令数据失败');
    } finally { importingExistingData.value = false; }
  };

  const fetchRemoteDatasetPreview = async (silent = false) => {
    if (!remoteDatasetUrl.value.trim()) { message.warning('请输入数据集 URL'); return; }
    loadingRemoteDataset.value = true;
    try {
      const res = await axios.post<RemoteDatasetResponse>('/config/ml/datasets/pull', {
        url: resolveDatasetUrl(remoteDatasetUrl.value), format: remoteDatasetFormat.value,
        limit: remoteDatasetLimit.value, labelMode: remoteDatasetLabelMode.value,
        cleanSensitive: remoteDatasetCleanSensitive.value,
      });
      remoteDatasetMeta.value = res.data;
      remoteDatasetPreview.value = res.data.rows || [];
      if (!silent) message.success(`拉取到 ${res.data.total || 0} 条远程数据`);
    } catch (e: any) {
      if (!silent) message.error(e.response?.data?.error || '拉取远程数据集失败');
    } finally { loadingRemoteDataset.value = false; }
  };

  const importRemoteDatasetPayload = async (payload: {
    url?: string; content?: string; contentBase64?: string; sourceName?: string; importAll?: boolean;
    format?: 'auto' | 'json' | 'jsonl' | 'csv' | 'tsv' | 'text';
    labelMode?: 'preserve' | 'unlabeled' | 'heuristic' | 'block';
    cleanSensitive?: boolean;
  }, options?: { refreshViews?: boolean }) => {
    const url = resolveDatasetUrl(payload.url ?? ((payload.content || payload.contentBase64) ? '' : remoteDatasetUrl.value.trim()));
    const res = await axios.post<RemoteDatasetResponse>('/config/ml/datasets/import', {
      url, content: payload.content, contentBase64: payload.contentBase64,
      sourceName: payload.sourceName, format: payload.format ?? remoteDatasetFormat.value,
      limit: remoteDatasetLimit.value, labelMode: payload.labelMode ?? remoteDatasetLabelMode.value,
      importAll: payload.importAll ?? false,
      cleanSensitive: payload.cleanSensitive ?? remoteDatasetCleanSensitive.value,
    });
    remoteDatasetMeta.value = res.data;
    remoteDatasetPreview.value = res.data.rows || [];
    if (options?.refreshViews !== false) {
      await refreshTrainingDatasetViews();
    }
    return res;
  };

  const importRemoteDataset = async () => {
    if (!remoteDatasetUrl.value.trim()) { message.warning('请输入数据集 URL'); return; }
    importingRemoteDataset.value = true;
    try {
      const res = await importRemoteDatasetPayload({ url: remoteDatasetUrl.value.trim() });
      message.success(`导入 ${res.data.imported || 0} 条，跳过 ${res.data.skipped || 0} 条`);
    } catch (e: any) {
      message.error(e.response?.data?.error || '导入远程数据集失败');
    } finally { importingRemoteDataset.value = false; }
  };

  const importClassicDataset = async (preset: ClassicSecurityDatasetPreset) => {
    if (!preset.downloadUrl) { window.open(preset.pageUrl, '_blank'); return; }
    importingClassicDataset.value = true;
    try {
      const res = await importRemoteDatasetPayload({
        url: preset.downloadUrl,
        sourceName: preset.name,
        importAll: true,
        format: preset.format ?? 'auto',
        labelMode: preset.labelMode ?? remoteDatasetLabelMode.value,
        cleanSensitive: remoteDatasetCleanSensitive.value,
      });
      message.success(`已导入 ${preset.name}（${res.data.imported ?? res.data.total ?? 0} 条）`);
    } catch (e: any) {
      message.error(`导入 ${preset.name} 失败：${e.response?.data?.error || e.message}`);
    } finally { importingClassicDataset.value = false; }
  };

  const openClassicSecurityDatasetPage = (preset: ClassicSecurityDatasetPreset) => {
    window.open(preset.pageUrl, '_blank', 'noopener,noreferrer');
  };

  const copyClassicSecurityDatasetPage = async (preset: ClassicSecurityDatasetPreset) => {
    try { await navigator.clipboard.writeText(preset.pageUrl); message.success(`已复制 ${preset.name} 链接`); }
    catch (_) { message.error('复制链接失败'); }
  };

  const importClassicDatasetPayload = async (preset: ClassicSecurityDatasetPreset) => {
    if (!preset.downloadUrl) {
      throw new Error(`preset ${preset.name} does not provide a downloadUrl`);
    }
    return importRemoteDatasetPayload({
      url: preset.downloadUrl,
      sourceName: preset.name,
      importAll: true,
      format: preset.format ?? 'auto',
      labelMode: preset.labelMode ?? remoteDatasetLabelMode.value,
      cleanSensitive: remoteDatasetCleanSensitive.value,
    }, { refreshViews: false });
  };

  const importAllInternetDatasets = async () => {
    const downloadable = classicSecurityDatasetPresets.filter((preset) => Boolean(preset.downloadUrl));
    let importedRows = 0;
    let importedSets = 0;
    let skippedSets = classicSecurityDatasetPresets.length - downloadable.length;
    importingClassicDataset.value = true;
    try {
      for (const preset of downloadable) {
        try {
          const res = await importClassicDatasetPayload(preset);
          importedRows += res.data.imported ?? res.data.total ?? 0;
          importedSets += 1;
        } catch (_) {
          skippedSets += 1;
        }
      }
      await refreshTrainingDatasetViews();
      message.success(`互联网数据批量导入完成：${importedRows} 条，${importedSets} 个数据集，跳过 ${skippedSets} 个条目`);
    } catch (e: any) {
      message.error(e.response?.data?.error || e.message || '批量导入互联网数据失败');
    } finally {
      importingClassicDataset.value = false;
    }
  };

  const importPresetBatch = async (presets: TrainingPreset[], label: string) => {
    importingClassicDataset.value = true;
    let added = 0;
    let skipped = 0;
    try {
      for (const preset of presets) {
        const argsArray = preset.args ? splitCommandLine(preset.args) : [];
        const argsStr = argsArray.join(' ');
        if ((deps.allSamples.value ?? []).find((s) => s.comm === preset.comm && (s.args || []).join(' ') === argsStr)) {
          skipped++;
          continue;
        }
        try {
          const commandLine = [preset.comm, preset.args].filter((part) => part && part.trim()).join(' ');
          await axios.post('/config/ml/samples', {
            commandLine,
            comm: preset.comm,
            args: argsArray,
            label: preset.label,
          });
          added++;
        } catch (_) {
          skipped++;
        }
      }
      await refreshTrainingDatasetViews();
      message.success(`${label} 导入完成：新增 ${added} 条，跳过 ${skipped} 条`);
    } finally {
      importingClassicDataset.value = false;
    }
  };

  // ── Data Utilities ──
  const maskSensitiveData = (text: string): string => {
    if (!dataMaskEnabled.value || !text) return text;
    text = text.replace(/\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g, '***.***.***.**');
    text = text.replace(/\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b/g, '***@***.***');
    text = text.replace(/https?:\/\/[^\s]+/g, (url) => {
      const parts = url.split('/');
      return parts.length > 2 ? parts[0] + '//' + parts[2].replace(/[a-zA-Z0-9]/g, '*') + '/***' : url;
    });
    text = text.replace(/\/home\/[^\/\s]+/g, '/home/***');
    text = text.replace(/~\/[^\s]+/g, '~/***');
    text = text.replace(/(password|passwd|pwd|token|key|secret)[\s=:]+[^\s]+/gi, '$1=***');
    text = text.replace(/AKIA[0-9A-Z]{16}/g, 'AKIA****************');
    text = text.replace(/\/etc\/(passwd|shadow|sudoers)/g, '/etc/***');
    return text;
  };

  const fetchLLMProductionDataset = async (silent = false) => {
    llmProductionLoading.value = true;
    try {
      const res = await axios.post<LLMProductionDatasetResponse>('/config/ml/llm/production-dataset/pull', {
        limit: llmProductionDatasetLimit.value,
        allowHeuristic: llmProductionAllowHeuristic.value,
        deduplicate: llmProductionDeduplicate.value,
      });
      llmProductionMeta.value = res.data;
      llmProductionPreview.value = res.data.rows || [];
      if (!silent) {
        message.success(`已拉取 ${res.data.included || 0} 条 LLM 生产训练样本`);
      }
    } catch (e: any) {
      if (!silent) {
        message.error(e.response?.data?.error || '拉取 LLM 生产训练集失败');
      }
    } finally {
      llmProductionLoading.value = false;
    }
  };

  const exportLLMProductionDataset = async () => {
    if (llmProductionPreview.value.length === 0) {
      message.warning('没有可导出的 LLM 生产训练样本');
      return;
    }
    const jsonl = buildLLMProductionJsonl(llmProductionPreview.value);
    downloadTextFile('agent-ebpf-filter-llm-production-training.jsonl', jsonl, 'application/x-ndjson;charset=utf-8');
    message.success(`已导出 ${llmProductionPreview.value.length} 条 LLM 生产训练样本`);
  };

  const importTrainingDatasetFromFile = async (event: Event) => {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    importingRemoteDataset.value = true;
    try {
      const buffer = await file.arrayBuffer();
      if (buffer.byteLength === 0) { message.warning('所选文件为空'); return; }
      await importRemoteDatasetPayload({ contentBase64: arrayBufferToBase64(buffer), sourceName: file.name, importAll: true });
      message.success(`已导入本地数据集 ${file.name}`);
    } catch (e: any) { message.error(e.response?.data?.error || '导入本地数据集失败'); }
    finally { importingRemoteDataset.value = false; input.value = ''; }
  };

  const exportTrainingDataset = async () => {
    try {
      const res = await axios.get<RemoteDatasetResponse>('/config/ml/datasets/export');
      downloadJsonFile('agent-ebpf-filter-training-dataset.json', res.data);
      message.success(`已导出 ${res.data.total || 0} 条训练样本`);
    } catch (e: any) { message.error(e.response?.data?.error || '导出训练集失败'); }
  };

  const clearTrainingDataset = async () => {
    try {
      const res = await axios.delete('/config/ml/datasets');
      message.success(`已清空 ${res.data.cleared || 0} 条训练样本`);
      remoteDatasetMeta.value = null; remoteDatasetPreview.value = [];
      await deps.fetchMLStatus(); await deps.fetchAllSamples(); await fetchExistingCommandData(true);
    } catch (e: any) { message.error(e.response?.data?.error || '清空训练集失败'); }
  };

  return {
    // Existing command data
    existingDataLimit, existingLabelMode, existingCommandCandidates,
    loadingExistingData, importingExistingData, existingDataSource,
    fetchExistingCommandData, importExistingCommandData,
    // Remote dataset
    remoteDatasetUrl, remoteDatasetFormat, remoteDatasetLabelMode, remoteDatasetCleanSensitive, remoteDatasetLimit,
    loadingRemoteDataset, importingRemoteDataset, remoteDatasetPreview, remoteDatasetMeta,
    fetchRemoteDatasetPreview, importRemoteDataset, importRemoteDatasetPayload,
    // LLM production dataset
    llmProductionDatasetLimit, llmProductionAllowHeuristic, llmProductionDeduplicate,
    llmProductionLoading, llmProductionPreview, llmProductionMeta,
    fetchLLMProductionDataset, exportLLMProductionDataset,
    // Classic/internet datasets
    importingClassicDataset,
    importClassicDataset, openClassicSecurityDatasetPage, copyClassicSecurityDatasetPage,
    importAllInternetDatasets, importPresetBatch,
    // File import/export
    trainingDatasetImportInput, dataMaskEnabled,
    importTrainingDatasetFromFile, exportTrainingDataset, clearTrainingDataset,
    // Utilities
    maskSensitiveData, resolveDatasetUrl,
    refreshTrainingDatasetViews,
    downloadJsonFile, arrayBufferToBase64,
    openTrainingDatasetImportPicker: () => { trainingDatasetImportInput.value?.click(); },
  };
}
