export const API_TOKEN_STORAGE_KEY = 'agent-ebpf.apiToken';
export const CLUSTER_TARGET_STORAGE_KEY = 'agent-ebpf.clusterTarget';
export const LOCAL_CLUSTER_TARGET = 'local';
export const CLUSTER_TARGET_CHANGED_EVENT = 'agent-ebpf:cluster-target-changed';

const isClient = () => typeof window !== 'undefined';

export const normalizeClusterTarget = (target?: string | null) => {
  const value = (target ?? '').trim();
  return value || LOCAL_CLUSTER_TARGET;
};

export const isLocalClusterTarget = (target?: string | null) => normalizeClusterTarget(target) === LOCAL_CLUSTER_TARGET;

export const getStoredApiToken = () => {
  if (!isClient()) return '';
  return window.localStorage.getItem(API_TOKEN_STORAGE_KEY)?.trim() ?? '';
};

export const setStoredApiToken = (token: string) => {
  if (!isClient()) return;
  const normalized = token.trim();
  if (!normalized) {
    window.localStorage.removeItem(API_TOKEN_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(API_TOKEN_STORAGE_KEY, normalized);
};

export const getStoredClusterTarget = () => {
  if (!isClient()) return LOCAL_CLUSTER_TARGET;
  return normalizeClusterTarget(window.localStorage.getItem(CLUSTER_TARGET_STORAGE_KEY));
};

export const setStoredClusterTarget = (target: string) => {
  if (!isClient()) return;
  const normalized = normalizeClusterTarget(target);
  window.localStorage.setItem(CLUSTER_TARGET_STORAGE_KEY, normalized);
  window.dispatchEvent(new CustomEvent(CLUSTER_TARGET_CHANGED_EVENT, { detail: { target: normalized } }));
};

export const buildRequestHeaders = () => {
  const headers: Record<string, string> = {};
  const token = getStoredApiToken();
  if (token) {
    headers['X-API-KEY'] = token;
    headers.Authorization = `Bearer ${token}`;
  }
  const clusterTarget = getStoredClusterTarget();
  if (clusterTarget && !isLocalClusterTarget(clusterTarget)) {
    headers['X-Cluster-Target'] = clusterTarget;
  }
  return headers;
};

import axios from 'axios';

// Proto-aware fetch: requests protobuf binary, decodes with provided decode function.
// Falls back to JSON if the server doesn't support proto.
export const fetchProto = async <T>(url: string, decode: (data: Uint8Array) => T): Promise<T> => {
  const headers = buildRequestHeaders();
  headers['Accept'] = 'application/x-protobuf, application/json;q=0.9';
  const res = await axios.get(url, { headers, responseType: 'arraybuffer' });
  const contentType = String(res.headers['content-type'] ?? '');
  if (contentType.includes('application/x-protobuf')) {
    return decode(new Uint8Array(res.data));
  }
  // Backward-compatible JSON fallback
  const text = new TextDecoder().decode(res.data);
  return JSON.parse(text) as T;
};

export const buildWebSocketUrl = (path: string, params: Record<string, string | number | undefined> = {}) => {
  if (!isClient()) {
    return path;
  }
  const url = new URL(path, window.location.origin);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const clusterTarget = getStoredClusterTarget();
  if (clusterTarget && !isLocalClusterTarget(clusterTarget)) {
    url.searchParams.set('cluster', clusterTarget);
  }
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && `${value}` !== '') {
      url.searchParams.set(key, String(value));
    }
  });
  return url.toString();
};
