import { ref, computed } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import {
  setStoredApiToken,
} from '../utils/requestContext';
import type { RuntimeSettings, RuntimeConfigResponse } from '../types/config';

export function useConfigRuntime() {
  const runtimeSettings = ref<RuntimeSettings>({
    logPersistenceEnabled: false,
    logFilePath: '',
    accessToken: '',
    maxEventCount: 1500,
    maxEventAge: '0',
  });
  const mcpEndpoint = ref('');
  const authHeaderName = ref('X-API-KEY');
  const bearerAuthHeaderName = ref('Authorization: Bearer');
  const persistedEventLogPath = ref('');
  const persistedEventLogAlive = ref(false);

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
    };
    mcpEndpoint.value = data.mcpEndpoint;
    authHeaderName.value = data.authHeaderName;
    bearerAuthHeaderName.value = data.bearerAuthHeaderName;
    persistedEventLogPath.value = data.persistedEventLogPath;
    persistedEventLogAlive.value = data.persistedEventLogAlive;
    syncApiToken(data.runtime.accessToken);
  };

  const fetchRuntime = async () => {
    try {
      const res = await axios.get('/config/runtime');
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
    } catch (_) {
      console.error('Failed to fetch runtime config');
    }
  };

  const saveRuntime = async () => {
    try {
      const res = await axios.put('/config/runtime', {
        logPersistenceEnabled: runtimeSettings.value.logPersistenceEnabled,
        logFilePath: runtimeSettings.value.logFilePath,
        maxEventCount: runtimeSettings.value.maxEventCount,
        maxEventAge: runtimeSettings.value.maxEventAge,
      });
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
      message.success('Runtime settings saved');
    } catch (_) {
      message.error('Failed to save runtime settings');
    }
  };

  const rotateAccessToken = async () => {
    try {
      const res = await axios.post('/config/access-token');
      applyRuntimeResponse(res.data as RuntimeConfigResponse);
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
    mcpEndpoint, authHeaderName, bearerAuthHeaderName,
    persistedEventLogPath, persistedEventLogAlive,
    syncApiToken, applyRuntimeResponse, fetchRuntime, saveRuntime,
    rotateAccessToken, clearInMemoryEvents, clearPersistedLog, clearAllEvents,
    copyText, mcpQueryEndpoint, mcpQueryEndpointTemplate,
  };
}
