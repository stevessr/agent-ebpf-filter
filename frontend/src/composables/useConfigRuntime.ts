import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  setStoredApiToken,
} from '../utils/requestContext';
import type { RuntimeSettings, RuntimeConfigResponse, CollectorHealthResponse, OTelHealthResponse } from '../types/config';

export function useConfigRuntime() {
  const runtimeSettings = ref<RuntimeSettings>({
    logPersistenceEnabled: false,
    logFilePath: '',
    accessToken: '',
    maxEventCount: 1500,
    maxEventAge: '0',
    shellSessionsEnabled: false,
    systemRunEnabled: false,
    hookManagementEnabled: false,
    policyManagementEnabled: false,
    otlpEnabled: false,
    otlpEndpoint: '',
    otlpServiceName: 'agent-ebpf-filter',
    otlpHeaders: {},
    tlsCaptureEnabled: false,
  });
  const mcpEndpoint = ref('');
  const authHeaderName = ref('X-API-KEY');
  const bearerAuthHeaderName = ref('Authorization: Bearer');
  const persistedEventLogPath = ref('');
  const persistedEventLogAlive = ref(false);
  const otlpHeadersText = ref('{}');
  const collectorHealth = ref<CollectorHealthResponse>({
    collectorMapAvailable: false,
    ringbufEventsTotal: 0,
    ringbufDroppedTotal: 0,
    ringbufReserveFailedTotal: 0,
    eventsByTypeTotal: {},
    backendQueueLen: 0,
    wsClients: 0,
    persistAppendLatencyNs: 0,
    captureHealthy: true,
  });
  const otelHealth = ref<OTelHealthResponse>({
    enabled: false,
    ready: false,
    endpoint: '',
    serviceName: '',
    queueLen: 0,
    activeRunSpans: 0,
    activeTaskSpans: 0,
    activeToolSpans: 0,
    exportedSpans: 0,
    droppedEvents: 0,
  });

  const syncApiToken = (token: string) => {
    const normalized = token.trim();
    if (typeof window === 'undefined') return;
    if (!normalized) {
      setStoredApiToken('');
      return;
    }
    setStoredApiToken(normalized);
    axios.defaults.headers.common['X-API-KEY'] = normalized;
    axios.defaults.headers.common.Authorization = `Bearer ${normalized}`;
  };

  const applyRuntimeResponse = (data: RuntimeConfigResponse) => {
    runtimeSettings.value = {
      logPersistenceEnabled: data.runtime.logPersistenceEnabled,
      logFilePath: data.runtime.logFilePath,
      accessToken: data.runtime.accessToken,
      maxEventCount: data.runtime.maxEventCount ?? 1500,
      maxEventAge: data.runtime.maxEventAge ?? '0',
      shellSessionsEnabled: Boolean(data.runtime.shellSessionsEnabled),
      systemRunEnabled: Boolean(data.runtime.systemRunEnabled),
      hookManagementEnabled: Boolean(data.runtime.hookManagementEnabled),
      policyManagementEnabled: Boolean(data.runtime.policyManagementEnabled),
      otlpEnabled: Boolean(data.runtime.otlpEnabled),
      otlpEndpoint: data.runtime.otlpEndpoint || '',
      otlpServiceName: data.runtime.otlpServiceName || 'agent-ebpf-filter',
      otlpHeaders: { ...(data.runtime.otlpHeaders || {}) },
      tlsCaptureEnabled: Boolean(data.runtime.tlsCaptureEnabled),
    };
    otlpHeadersText.value = JSON.stringify(runtimeSettings.value.otlpHeaders || {}, null, 2);
    mcpEndpoint.value = data.mcpEndpoint;
    authHeaderName.value = data.authHeaderName;
    bearerAuthHeaderName.value = data.bearerAuthHeaderName;
    persistedEventLogPath.value = data.persistedEventLogPath;
    persistedEventLogAlive.value = data.persistedEventLogAlive;
    syncApiToken(data.runtime.accessToken);
  };

  const fetchRuntime = async () => {
    try {
      const [runtimeRes, collectorRes, otelRes] = await Promise.all([
        axios.get('/config/runtime'),
        axios.get('/system/collector-health'),
        axios.get('/system/otel-health'),
      ]);
      collectorHealth.value = collectorRes.data as CollectorHealthResponse;
      otelHealth.value = otelRes.data as OTelHealthResponse;
      applyRuntimeResponse(runtimeRes.data as RuntimeConfigResponse);
    } catch (_) {
      console.error('Failed to fetch runtime config');
    }
  };

  const fetchCollectorHealth = async () => {
    try {
      const [collectorRes, otelRes] = await Promise.all([
        axios.get('/system/collector-health'),
        axios.get('/system/otel-health'),
      ]);
      collectorHealth.value = collectorRes.data as CollectorHealthResponse;
      otelHealth.value = otelRes.data as OTelHealthResponse;
    } catch (_) {
      console.error('Failed to fetch collector health');
    }
  };

  const parseOTLPHeaders = () => {
    const raw = otlpHeadersText.value.trim();
    if (!raw) return {};
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('OTLP headers must be a JSON object');
      }
      return Object.entries(parsed).reduce<Record<string, string>>((acc, [key, value]) => {
        const normalizedKey = String(key || '').trim();
        if (!normalizedKey) return acc;
        acc[normalizedKey] = String(value ?? '').trim();
        return acc;
      }, {});
    } catch (error: any) {
      throw new Error(error?.message || 'Invalid OTLP headers JSON');
    }
  };

  const saveRuntime = async () => {
    try {
      const otlpHeaders = parseOTLPHeaders();
      const res = await axios.put('/config/runtime', {
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
      });
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
      await fetchCollectorHealth();
      message.success('Runtime settings saved');
    } catch (error: any) {
      message.error(error?.message || 'Failed to save runtime settings');
    }
  };

  const rotateAccessToken = async () => {
    try {
      const res = await axios.post('/config/access-token');
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
      await fetchCollectorHealth();
      message.success('Access token regenerated');
    } catch (_) {
      message.error('Failed to regenerate access token');
    }
  };

  const clearInMemoryEvents = async () => {
    try {
      await axios.post('/data/clear-events-memory');
      message.success('In-memory events cleared');
    } catch (err: any) {
      message.error(err?.response?.data?.error || 'Failed to clear memory events');
    }
  };

  const clearPersistedLog = async () => {
    try {
      await axios.post('/data/clear-events-persisted');
      message.success('Persisted event log truncated');
    } catch (err: any) {
      message.error(err?.response?.data?.error || 'Failed to truncate log');
    }
  };

  const clearAllEvents = async () => {
    try {
      await axios.post('/data/clear-events');
      message.success('All events cleared');
    } catch (err: any) {
      message.error(err?.response?.data?.error || 'Failed to clear events');
    }
  };

  const copyText = async (text: string, successMessage: string) => {
    const value = text.trim();
    if (!value) {
      message.warning('Nothing to copy');
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      message.success(successMessage);
    } catch (_) {
      message.error('Failed to copy to clipboard');
    }
  };

  const mcpQueryEndpoint = computed(() => {
    if (!mcpEndpoint.value) return '';
    if (!runtimeSettings.value.accessToken.trim()) {
      return `${mcpEndpoint.value}?key=$API_KEY`;
    }
    return `${mcpEndpoint.value}?key=${encodeURIComponent(runtimeSettings.value.accessToken)}`;
  });

  const mcpQueryEndpointTemplate = computed(() => {
    if (!mcpEndpoint.value) return '';
    return `${mcpEndpoint.value}?key=$API_KEY`;
  });

  return {
    runtimeSettings,
    otlpHeadersText, otelHealth,
    mcpEndpoint, authHeaderName, bearerAuthHeaderName,
    persistedEventLogPath, persistedEventLogAlive, collectorHealth,
    syncApiToken, applyRuntimeResponse, fetchRuntime, fetchCollectorHealth, saveRuntime,
    rotateAccessToken, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
    copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
  };
}
