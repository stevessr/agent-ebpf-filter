import { computed, ref } from 'vue';

const STORAGE_KEY = 'agent-ebpf.agentsight.locale';
type Locale = 'en' | 'zh';

type AgentSightMessageKey =
  | 'title'
  | 'compatMessage'
  | 'compatDescription'
  | 'traceImport'
  | 'uploadTrace'
  | 'importedRecords'
  | 'clearImported'
  | 'pastePlaceholder'
  | 'importPasted'
  | 'log'
  | 'timeline'
  | 'processTree'
  | 'metrics'
  | 'refresh'
  | 'command'
  | 'pid'
  | 'traceId'
  | 'source'
  | 'eventType'
  | 'redaction'
  | 'clear'
  | 'time'
  | 'type'
  | 'trace'
  | 'summary'
  | 'zoom'
  | 'window'
  | 'visible'
  | 'lanes'
  | 'events';

const messages: Record<Locale, Record<AgentSightMessageKey, string>> = {
  en: {
    title: 'AgentSight',
    compatMessage: 'AgentSight compatibility view',
    compatDescription: 'This page consumes unified EventEnvelope history and exposes Log, Timeline, Process Tree, and Metrics views for TLS, HTTP/SSE, process, policy, wrapper, hook, MCP, stdio, and system events.',
    traceImport: 'Trace import',
    uploadTrace: 'Upload trace/log',
    importedRecords: 'imported records cached locally',
    clearImported: 'Clear imported',
    pastePlaceholder: 'Paste JSON, JSONL, or AgentSight sample trace records here',
    importPasted: 'Import pasted trace',
    log: 'Log',
    timeline: 'Timeline',
    processTree: 'Process Tree',
    metrics: 'Metrics',
    refresh: 'Refresh',
    command: 'Command',
    pid: 'PID',
    traceId: 'Trace ID',
    source: 'Source',
    eventType: 'Event type',
    redaction: 'Redaction',
    clear: 'Clear',
    time: 'Time',
    type: 'Type',
    trace: 'Trace',
    summary: 'Summary',
    zoom: 'Zoom',
    window: 'Window',
    visible: 'visible',
    lanes: 'lanes',
    events: 'events',
  },
  zh: {
    title: 'AgentSight',
    compatMessage: 'AgentSight 兼容视图',
    compatDescription: '此页面消费统一 EventEnvelope 历史与实时流，提供 Log、Timeline、Process Tree、Metrics 视图，覆盖 TLS、HTTP/SSE、进程、策略、wrapper、hook、MCP、stdio 和 system 事件。',
    traceImport: 'Trace 导入',
    uploadTrace: '上传 trace/log',
    importedRecords: '条导入记录已本地缓存',
    clearImported: '清空导入',
    pastePlaceholder: '在此粘贴 JSON、JSONL 或 AgentSight 示例 trace 记录',
    importPasted: '导入粘贴 trace',
    log: '日志',
    timeline: '时间线',
    processTree: '进程树',
    metrics: '指标',
    refresh: '刷新',
    command: '命令',
    pid: 'PID',
    traceId: 'Trace ID',
    source: '来源',
    eventType: '事件类型',
    redaction: '脱敏',
    clear: '清空',
    time: '时间',
    type: '类型',
    trace: 'Trace',
    summary: '摘要',
    zoom: '缩放',
    window: '窗口',
    visible: '可见',
    lanes: '泳道',
    events: '事件',
  },
};

const storedLocale = () => {
  if (typeof window === 'undefined') return 'en';
  return window.localStorage.getItem(STORAGE_KEY) === 'zh' ? 'zh' : 'en';
};

export function useAgentSightI18n() {
  const locale = ref<Locale>(storedLocale());
  const localeOptions = [
    { label: 'English', value: 'en' },
    { label: '中文', value: 'zh' },
  ];
  const t = computed(() => messages[locale.value]);
  const setLocale = (value: Locale) => {
    locale.value = value;
    if (typeof window !== 'undefined') window.localStorage.setItem(STORAGE_KEY, value);
  };
  return { locale, localeOptions, t, setLocale };
}
