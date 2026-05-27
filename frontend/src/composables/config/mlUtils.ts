import type { MLLlmConfig, LLMProductionDatasetRow } from "../types/config";

export const LLM_SCORING_STORAGE_KEY = 'agent-ebpf-filter.ml.llm-scoring-config';

type StoredLLMScoringConfig = Pick<
  MLLlmConfig,
  'enabled' | 'baseUrl' | 'apiKey' | 'model' | 'timeoutSeconds' | 'temperature' | 'maxTokens' | 'systemPrompt'
>;

export const defaultLLMScoringConfig = (): MLLlmConfig => ({
  enabled: false,
  baseUrl: '',
  apiKey: '',
  apiKeyConfigured: false,
  model: '',
  timeoutSeconds: 45,
  temperature: 0,
  maxTokens: 256,
  systemPrompt: '',
});

export const readStoredLLMScoringConfig = (): Partial<StoredLLMScoringConfig> | null => {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(LLM_SCORING_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<StoredLLMScoringConfig>;
    if (!parsed || typeof parsed !== 'object') return null;
    return parsed;
  } catch {
    return null;
  }
};

export const pickLLMScoringConfigForStorage = (config: MLLlmConfig): StoredLLMScoringConfig => ({
  enabled: !!config.enabled,
  baseUrl: config.baseUrl || '',
  apiKey: config.apiKey || '',
  model: config.model || '',
  timeoutSeconds: Number.isFinite(config.timeoutSeconds) ? config.timeoutSeconds : 45,
  temperature: Number.isFinite(config.temperature) ? config.temperature : 0,
  maxTokens: Number.isFinite(config.maxTokens) ? config.maxTokens : 256,
  systemPrompt: config.systemPrompt || '',
});

export const downloadJsonFile = (filename: string, payload: unknown) => {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a'); link.href = url; link.download = filename; link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
};

export const downloadTextFile = (filename: string, content: string, mimeType = 'text/plain;charset=utf-8') => {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
};

export const llmProductionPayloadForRow = (row: LLMProductionDatasetRow) => ({
  messages: row.messages,
});

export const buildLLMProductionJsonl = (rows: LLMProductionDatasetRow[]) =>
  rows.map((row) => JSON.stringify(llmProductionPayloadForRow(row))).join('\n');

export const arrayBufferToBase64 = (buffer: ArrayBuffer) => {
  let binary = '';
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < bytes.length; i += 0x8000) binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return window.btoa(binary);
};

export const getLabelColor = (label: string) => {
  const m: Record<string, string> = {
    'BLOCK': 'red', 'ALERT': 'orange', 'ALLOW': 'green', 'REWRITE': 'blue', '-': 'default',
  };
  return m[label] || 'default';
};

export const splitCommandLine = (input: string): string[] => {
  const parts: string[] = [];
  let current = '';
  let inSingle = false, inDouble = false, escaped = false;
  const emit = () => { if (!current) return; parts.push(current); current = ''; };
  for (const ch of input.trim()) {
    if (escaped) { current += ch; escaped = false; }
    else if (ch === '\\' && !inSingle) { escaped = true; }
    else if (ch === "'" && !inDouble) { inSingle = !inSingle; }
    else if (ch === '"' && !inSingle) { inDouble = !inDouble; }
    else if (/\s/.test(ch) && !inSingle && !inDouble) { emit(); }
    else { current += ch; }
  }
  if (escaped) current += '\\';
  emit();
  return parts;
};

export const riskLevelColor = (level?: string) => {
  const m: Record<string, string> = { 'CRITICAL': '#cf1322', 'HIGH': '#d4380d', 'MEDIUM': '#d48806', 'LOW': '#389e0d', 'SAFE': '#52c41a' };
  return (level && m[level]) || '#666';
};

export const riskMeterColor = (score: number) => {
  if (score >= 80) return '#cf1322'; if (score >= 60) return '#d4380d';
  if (score >= 40) return '#d48806'; if (score >= 20) return '#389e0d'; return '#52c41a';
};
