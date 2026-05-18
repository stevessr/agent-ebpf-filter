import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  setStoredApiToken,
} from '../utils/requestContext';
import type {
  RuntimeSettings,
  RuntimeConfigResponse,
  CollectorHealthResponse,
  OTelHealthResponse,
  DomainForwardRoute,
  DomainForwardProxySettings,
  DomainForwardProxyStatus,
  TracepointBootstrapStatus,
} from '../types/config';

interface EditableHeaderRow {
  id: string;
  key: string;
  value: string;
}

interface EditableDomainForwardRoute extends DomainForwardRoute {
  id: string;
}

let editableRowSequence = 0;
const nextEditableRowId = (prefix: string) => `${prefix}-${Date.now()}-${editableRowSequence++}`;

const defaultDomainForwardProxy = (): DomainForwardProxySettings => ({
  enabled: false,
  httpPort: 80,
  httpsPort: 443,
  defaultScheme: 'https',
  allowAnyHost: false,
  dnsResolver: '',
  dialTimeoutSeconds: 10,
  certFile: '',
  keyFile: '',
  routes: [],
});

const normalizeDomainForwardProxy = (
  value?: Partial<DomainForwardProxySettings>,
): DomainForwardProxySettings => {
  const defaults = defaultDomainForwardProxy();
  const scheme = value?.defaultScheme === 'http' ? 'http' : 'https';
  return {
    ...defaults,
    ...value,
    defaultScheme: scheme,
    httpPort: Number(value?.httpPort || defaults.httpPort),
    httpsPort: Number(value?.httpsPort || defaults.httpsPort),
    dialTimeoutSeconds: Number(value?.dialTimeoutSeconds || defaults.dialTimeoutSeconds),
    routes: Array.isArray(value?.routes) ? value.routes : [],
  };
};

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
    domainForwardProxy: defaultDomainForwardProxy(),
  });
  const mcpEndpoint = ref('');
  const authHeaderName = ref('X-API-KEY');
  const bearerAuthHeaderName = ref('Authorization: Bearer');
  const persistedEventLogPath = ref('');
  const persistedEventLogAlive = ref(false);
  const otlpHeaderRows = ref<EditableHeaderRow[]>([]);
  const domainForwardRoutes = ref<EditableDomainForwardRoute[]>([]);
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
  const bootstrapHealth = ref<TracepointBootstrapStatus>({
    kernelRelease: 'unknown',
    compiledCount: 0,
    attachedCount: 0,
    skippedCount: 0,
    skippedTracepoints: [],
    status: 'unknown',
    message: 'Tracepoint bootstrap has not been observed yet.',
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
  const domainForwardStatus = ref<DomainForwardProxyStatus>({
    enabled: false,
    httpRunning: false,
    httpsRunning: false,
    httpPort: 80,
    httpsPort: 443,
    routeCount: 0,
    allowAnyHost: false,
    updatedAt: '',
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
      domainForwardProxy: normalizeDomainForwardProxy(data.runtime.domainForwardProxy),
    };
    otlpHeaderRows.value = Object.entries(runtimeSettings.value.otlpHeaders || {}).map(([key, value]) => ({
      id: nextEditableRowId('otlp-header'),
      key,
      value: String(value ?? ''),
    }));
    domainForwardRoutes.value = (runtimeSettings.value.domainForwardProxy.routes || []).map((route) => ({
      id: nextEditableRowId('domain-route'),
      host: route.host || '',
      upstream: route.upstream || '',
      certFile: route.certFile || '',
      keyFile: route.keyFile || '',
    }));
    mcpEndpoint.value = data.mcpEndpoint;
    authHeaderName.value = data.authHeaderName;
    bearerAuthHeaderName.value = data.bearerAuthHeaderName;
    persistedEventLogPath.value = data.persistedEventLogPath;
    persistedEventLogAlive.value = data.persistedEventLogAlive;
    syncApiToken(data.runtime.accessToken);
  };

  const fetchRuntime = async () => {
    const [runtimeRes, bootstrapRes, collectorRes, otelRes, domainForwardRes] = await Promise.allSettled([
      axios.get('/config/runtime'),
      axios.get('/system/bootstrap-health'),
      axios.get('/system/collector-health'),
      axios.get('/system/otel-health'),
      axios.get('/system/domain-forward/status'),
    ]);
    if (runtimeRes.status === 'fulfilled') {
      applyRuntimeResponse(runtimeRes.value.data as RuntimeConfigResponse);
    } else {
      console.error('Failed to fetch runtime config');
    }
    if (bootstrapRes.status === 'fulfilled') {
      bootstrapHealth.value = bootstrapRes.value.data as TracepointBootstrapStatus;
    } else {
      console.error('Failed to fetch bootstrap health');
    }
    if (collectorRes.status === 'fulfilled') {
      collectorHealth.value = collectorRes.value.data as CollectorHealthResponse;
    } else {
      console.error('Failed to fetch collector health');
    }
    if (otelRes.status === 'fulfilled') {
      otelHealth.value = otelRes.value.data as OTelHealthResponse;
    } else {
      console.error('Failed to fetch OTLP health');
    }
    if (domainForwardRes.status === 'fulfilled') {
      domainForwardStatus.value = domainForwardRes.value.data as DomainForwardProxyStatus;
    } else {
      console.error('Failed to fetch domain forward proxy status');
    }
  };

  const fetchCollectorHealth = async () => {
    const [bootstrapRes, collectorRes, otelRes, domainForwardRes] = await Promise.allSettled([
      axios.get('/system/bootstrap-health'),
      axios.get('/system/collector-health'),
      axios.get('/system/otel-health'),
      axios.get('/system/domain-forward/status'),
    ]);
    if (bootstrapRes.status === 'fulfilled') {
      bootstrapHealth.value = bootstrapRes.value.data as TracepointBootstrapStatus;
    } else {
      console.error('Failed to fetch bootstrap health');
    }
    if (collectorRes.status === 'fulfilled') {
      collectorHealth.value = collectorRes.value.data as CollectorHealthResponse;
    } else {
      console.error('Failed to fetch collector health');
    }
    if (otelRes.status === 'fulfilled') {
      otelHealth.value = otelRes.value.data as OTelHealthResponse;
    } else {
      console.error('Failed to fetch OTLP health');
    }
    if (domainForwardRes.status === 'fulfilled') {
      domainForwardStatus.value = domainForwardRes.value.data as DomainForwardProxyStatus;
    } else {
      console.error('Failed to fetch domain forward proxy status');
    }
  };

  const parseOTLPHeaders = () => {
    return otlpHeaderRows.value.reduce<Record<string, string>>((acc, row, index) => {
      const key = row.key.trim();
      const value = row.value.trim();
      if (!key && !value) return acc;
      if (!key) {
        throw new Error(`OTLP header #${index + 1} requires a name`);
      }
      acc[key] = value;
      return acc;
    }, {});
  };

  const parseDomainForwardRoutes = () => {
    return domainForwardRoutes.value.reduce<DomainForwardRoute[]>((acc, route, index) => {
      const host = String(route.host || '').trim();
      const upstream = String(route.upstream || '').trim();
      const certFile = String(route.certFile || '').trim();
      const keyFile = String(route.keyFile || '').trim();
      if (!host && !upstream && !certFile && !keyFile) return acc;
      if (!host) {
        throw new Error(`Domain forward route #${index + 1} requires a host`);
      }
      acc.push({ host, upstream, certFile, keyFile });
      return acc;
    }, []);
  };

  const addOTLPHeaderRow = () => {
    otlpHeaderRows.value.push({ id: nextEditableRowId('otlp-header'), key: '', value: '' });
  };

  const removeOTLPHeaderRow = (id: string) => {
    otlpHeaderRows.value = otlpHeaderRows.value.filter((row) => row.id !== id);
  };

  const addDomainForwardRoute = () => {
    domainForwardRoutes.value.push({
      id: nextEditableRowId('domain-route'),
      host: '',
      upstream: '',
      certFile: '',
      keyFile: '',
    });
  };

  const removeDomainForwardRoute = (id: string) => {
    domainForwardRoutes.value = domainForwardRoutes.value.filter((route) => route.id !== id);
  };

  const saveRuntime = async () => {
    try {
      const otlpHeaders = parseOTLPHeaders();
      const domainForwardRoutes = parseDomainForwardRoutes();
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
        domainForwardProxy: {
          ...runtimeSettings.value.domainForwardProxy,
          routes: domainForwardRoutes,
        },
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
    otlpHeaderRows, otelHealth, domainForwardRoutes, domainForwardStatus,
    mcpEndpoint, authHeaderName, bearerAuthHeaderName,
    persistedEventLogPath, persistedEventLogAlive, bootstrapHealth, collectorHealth,
    syncApiToken, applyRuntimeResponse, fetchRuntime, fetchCollectorHealth, saveRuntime,
    addOTLPHeaderRow, removeOTLPHeaderRow, addDomainForwardRoute, removeDomainForwardRoute,
    rotateAccessToken, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
    copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
  };
}
