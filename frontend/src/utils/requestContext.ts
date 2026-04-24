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
